package scheduler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	trackingCounterName   = "sms_tracking_id"
	trackingCounterHexLen = 16
	trackingCounterBits   = 16 * 4

	numJobsPerTick          = 100
	statusJobWorkerInterval = 5 * time.Minute

	// statusJobMaxRetry is the maximum number of times a status-check job is
	// retried before it is permanently marked as executed. Used by all platform
	// schedulers (Bale, Splus, SMS, …).
	statusJobMaxRetry = 3

	// audienceAppendBatchSize controls how many audience IDs are flushed to the
	// database per AppendAudienceData call inside the persistence transaction.
	// Used by all platform schedulers.
	audienceAppendBatchSize = 1000
)

type AudiencePhonesResult struct {
	Phones        []string
	IDs           []int64
	UIDs          []string
	Codes         []string
	SelectionID   uint
	MatchedUIDs   []string
	UnmatchedUIDs []string
}

func initSchedulerLogger(name string) (*log.Logger, *os.File, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		clean = "scheduler"
	}

	candidates := []string{"data", "/data"}
	for _, dir := range candidates {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		logPath := filepath.Join(dir, fmt.Sprintf("%s.log", clean))
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}

		// BUG FIX 5: io.MultiWriter(f) with a single argument is a no-op wrapper and,
		// critically, omits os.Stdout — so log lines were silently dropped from the
		// console/container output. Fixed by fanning out to both Stdout and the file.
		mw := io.MultiWriter(os.Stdout, f)
		l := log.New(mw, fmt.Sprintf("%s ", clean), log.LstdFlags|log.Lmicroseconds|log.LUTC)
		return l, f, nil
	}

	return nil, nil, fmt.Errorf("could not create %s log file in any candidate directory", clean)
}

func hasCampaignAdLink(link *string) bool {
	return link != nil && strings.TrimSpace(*link) != ""
}

func hasTargetAudienceExcelFileUUID(fileUUID *string) bool {
	return fileUUID != nil && strings.TrimSpace(*fileUUID) != ""
}

func fetchTargetAudienceUIDsFromExcel(ctx context.Context, botClient BotClient, jazzToken string, campaignID uint) ([]string, error) {
	data, err := botClient.DownloadTargetAudienceExcelFile(ctx, jazzToken, campaignID)
	if err != nil {
		return nil, err
	}

	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    2 << 30, // 2GB
		UnzipXMLSizeLimit: 1 << 30, // 1GB
	})
	if err != nil {
		return nil, fmt.Errorf("cannot open target audience excel file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("target audience excel file has no sheets")
	}

	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("cannot iterate target audience excel rows: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	uids := make([]string, 0)
	rowIndex := 0
	for rows.Next() {
		row, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to read target audience excel row: %w", err)
		}
		if rowIndex == 0 {
			rowIndex++
			continue // header row
		}
		if len(row) == 0 {
			rowIndex++
			continue
		}
		uid := strings.TrimSpace(row[0])
		if uid != "" {
			uids = append(uids, uid)
		}
		rowIndex++
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("failed while reading target audience excel rows: %w", err)
	}
	return uids, nil
}

func fetchAudiencePhonesByUIDs(
	ctx context.Context,
	logger *log.Logger,
	audRepo repository.AudienceProfileRepository,
	botClient BotClient,
	c dto.BotGetCampaignResponse,
	token string,
	inputUIDs []string,
	shortLinkDomain string,
) (*AudiencePhonesResult, error) {
	if len(inputUIDs) == 0 {
		return &AudiencePhonesResult{}, nil
	}

	uniqueUIDs := make([]string, 0, len(inputUIDs))
	seen := make(map[string]struct{}, len(inputUIDs))
	for _, raw := range inputUIDs {
		uid := strings.TrimSpace(raw)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		uniqueUIDs = append(uniqueUIDs, uid)
	}
	if len(uniqueUIDs) == 0 {
		return &AudiencePhonesResult{}, nil
	}

	profiles, err := audRepo.ByUIDs(ctx, uniqueUIDs)
	if err != nil {
		return nil, err
	}

	byUID := make(map[string]*models.AudienceProfile, len(profiles))
	for _, p := range profiles {
		if p == nil {
			continue
		}
		byUID[p.UID] = p
	}

	type matchedAudience struct {
		id    int64
		phone string
		uid   string
	}

	matched := make([]matchedAudience, 0, len(uniqueUIDs))
	unmatchedUIDs := make([]string, 0)
	for _, uid := range uniqueUIDs {
		profile, ok := byUID[uid]
		if !ok {
			unmatchedUIDs = append(unmatchedUIDs, uid)
			continue
		}
		if profile.PhoneNumber == nil || strings.TrimSpace(*profile.PhoneNumber) == "" {
			unmatchedUIDs = append(unmatchedUIDs, uid)
			continue
		}
		matched = append(matched, matchedAudience{
			id:    profile.ID,
			phone: strings.TrimSpace(*profile.PhoneNumber),
			uid:   uid,
		})
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].id < matched[j].id
	})

	phones := make([]string, 0, len(matched))
	ids := make([]int64, 0, len(matched))
	uids := make([]string, 0, len(matched))
	matchedUIDs := make([]string, 0, len(matched))
	for _, item := range matched {
		phones = append(phones, item.phone)
		ids = append(ids, item.id)
		uids = append(uids, item.uid)
		matchedUIDs = append(matchedUIDs, item.uid)
	}

	if !hasCampaignAdLink(c.AdLink) {
		logger.Printf("fetchAudiencePhonesByUIDs skipped short links generation: campaign_id=%d ad_link=empty", c.ID)
		return &AudiencePhonesResult{
			Phones:        phones,
			IDs:           ids,
			UIDs:          uids,
			Codes:         make([]string, len(phones)),
			MatchedUIDs:   matchedUIDs,
			UnmatchedUIDs: unmatchedUIDs,
		}, nil
	}
	if strings.TrimSpace(shortLinkDomain) == "" {
		logger.Printf("fetchAudiencePhonesByUIDs skipped short links generation: campaign_id=%d short_link_domain=empty", c.ID)
		return &AudiencePhonesResult{
			Phones:        phones,
			IDs:           ids,
			UIDs:          uids,
			Codes:         make([]string, len(phones)),
			MatchedUIDs:   matchedUIDs,
			UnmatchedUIDs: unmatchedUIDs,
		}, nil
	}

	items := make([]dto.PhoneWithAdLink, len(phones))
	for i, p := range phones {
		adLink := c.AdLink
		if adLink != nil && strings.Contains(*adLink, "{uid}") {
			resolved := strings.ReplaceAll(*adLink, "{uid}", matchedUIDs[i])
			adLink = &resolved
		}
		items[i] = dto.PhoneWithAdLink{Phone: p, AdLink: adLink}
	}
	codes, err := botClient.AllocateShortLinks(ctx, token, &dto.BotAllocateShortLinksRequest{
		CampaignID:      c.ID,
		Items:           items,
		ShortLinkDomain: shortLinkDomain,
	})
	if err != nil {
		logger.Printf("fetchAudiencePhonesByUIDs allocate short links failed: campaign_id=%d selected=%d err=%v", c.ID, len(phones), err)
		return nil, err
	}

	return &AudiencePhonesResult{
		Phones:        phones,
		IDs:           ids,
		UIDs:          uids,
		Codes:         codes,
		MatchedUIDs:   matchedUIDs,
		UnmatchedUIDs: unmatchedUIDs,
	}, nil
}

func allocateTrackingIDs(ctx context.Context, db *gorm.DB, count int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}

	var ids []string
	err := repository.WithTransaction(ctx, db, func(txCtx context.Context) error {
		db := db.WithContext(txCtx)
		if tx, ok := txCtx.Value(repository.TxContextKey).(*gorm.DB); ok && tx != nil {
			db = tx.WithContext(txCtx)
		}

		var counter models.SequenceCounter
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&counter, "name = ?", trackingCounterName).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			now := utils.UTCNow()
			counter = models.SequenceCounter{
				Name:      trackingCounterName,
				LastValue: strings.Repeat("0", trackingCounterHexLen),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := db.Create(&counter).Error; err != nil {
				return err
			}
		}

		last := strings.TrimSpace(counter.LastValue)
		if last == "" {
			last = strings.Repeat("0", trackingCounterHexLen)
		}
		if len(last) > trackingCounterHexLen {
			return fmt.Errorf("tracking counter exceeds %d hex chars", trackingCounterHexLen)
		}
		last = strings.Repeat("0", trackingCounterHexLen-len(last)) + strings.ToLower(last)
		base := new(big.Int)
		if _, ok := base.SetString(last, 16); !ok {
			return fmt.Errorf("invalid tracking counter value")
		}

		ids = make([]string, count)
		for i := 0; i < count; i++ {
			base.Add(base, big.NewInt(1))
			if base.BitLen() > trackingCounterBits {
				return fmt.Errorf("tracking counter overflow")
			}
			ids[i] = fmt.Sprintf("%0*x", trackingCounterHexLen, base)
		}

		counter.LastValue = ids[len(ids)-1]
		counter.UpdatedAt = utils.UTCNow()
		return db.Model(&models.SequenceCounter{}).
			Where("name = ?", counter.Name).
			Updates(map[string]any{
				"last_value": counter.LastValue,
				"updated_at": counter.UpdatedAt,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// retryBackoffDelay returns an exponential back-off duration for the given
// attempt index (0-based), starting at base and capped at max.
func retryBackoffDelay(attempt int, base, max time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := base
	for i := 0; i < attempt; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	return d
}

func hashTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	cp := make([]string, len(tags))
	copy(cp, tags)
	sort.Strings(cp)
	h := sha1.Sum([]byte(strings.Join(cp, ",")))
	return hex.EncodeToString(h[:])
}
