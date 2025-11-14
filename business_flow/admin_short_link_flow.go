package businessflow

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// AdminShortLinkFlow provides use cases for admin short link creation
// The admin uploads a CSV with a 'long_link' column and provides a short_link_domain parameter
// For each row with a valid long_link, a short link will be generated and inserted
// The generated short URL format is: <short_link_domain>/s/<uid>
// Note: This flow assumes a public redirect route exists at /s/:uid
// Note: UID collisions are extremely unlikely; a random base62 string of length 10 is used
// This function skips rows with empty long_link values
// It returns a summary with counts and the created short links
// Validations are minimal; consumers may validate long_link formats if needed
// Domain should include scheme (https://) if desired by the caller
// If not provided with a scheme, https:// will be prefixed automatically
type AdminShortLinkFlow interface {
	CreateShortLinksFromCSV(ctx context.Context, csvReader io.Reader, shortLinkDomain string) (*dto.AdminCreateShortLinksResponse, error)
	DownloadShortLinksCSV(ctx context.Context, scenarioID uint) (string, []byte, error)
	DownloadShortLinksWithClicksCSV(ctx context.Context, scenarioID uint) (string, []byte, error)
}

type AdminShortLinkFlowImpl struct {
	repo repository.ShortLinkRepository
}

func NewAdminShortLinkFlow(repo repository.ShortLinkRepository) AdminShortLinkFlow {
	return &AdminShortLinkFlowImpl{repo: repo}
}

func (f *AdminShortLinkFlowImpl) CreateShortLinksFromCSV(ctx context.Context, csvReader io.Reader, shortLinkDomain string) (*dto.AdminCreateShortLinksResponse, error) {
	if csvReader == nil {
		return nil, NewBusinessError("VALIDATION_ERROR", "CSV file is required", nil)
	}
	shortLinkDomain = normalizeDomain(shortLinkDomain)
	if shortLinkDomain == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "short_link_domain is required", nil)
	}

	reader := csv.NewReader(bufio.NewReader(csvReader))
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, NewBusinessError("CSV_READ_ERROR", "Failed to read CSV header", err)
	}

	colIndex := map[string]int{}
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	longIdx, ok := colIndex["long_link"]
	if !ok {
		return nil, NewBusinessError("CSV_HEADER_ERROR", "CSV must contain a 'long_link' column", nil)
	}

	lastScenarioID, err := f.repo.GetLastScenarioID(ctx)
	if err != nil {
		return nil, NewBusinessError("FETCH_SCENARIO_ID_FAILED", "Failed to determine next scenario id", err)
	}
	newScenarioID := lastScenarioID + 1

	rows := make([]*models.ShortLink, 0, 256)
	created := 0
	skipped := 0
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, NewBusinessError("CSV_READ_ERROR", "Failed to read CSV row", err)
		}
		if longIdx >= len(rec) {
			skipped++
			continue
		}
		longLink := strings.TrimSpace(rec[longIdx])
		if longLink == "" {
			skipped++
			continue
		}

		uid := generateRandomBase62(10)
		shortURL := fmt.Sprintf("%s/s/%s", shortLinkDomain, uid)
		scenarioID := newScenarioID
		rows = append(rows, &models.ShortLink{
			UID:         uid,
			CampaignID:  nil,
			ClientID:    nil,
			PhoneNumber: nil,
			ScenarioID:  &scenarioID,
			LongLink:    longLink,
			ShortLink:   shortURL,
		})
		created++
	}

	if len(rows) == 0 {
		return &dto.AdminCreateShortLinksResponse{
			Message:    "No valid rows to create",
			TotalRows:  created + skipped,
			Created:    0,
			Skipped:    skipped,
			ScenarioID: newScenarioID,
		}, nil
	}

	if err := f.repo.SaveBatch(ctx, rows); err != nil {
		return nil, NewBusinessError("CREATE_SHORT_LINKS_FAILED", "Failed to create short links", err)
	}

	return &dto.AdminCreateShortLinksResponse{
		Message:    "Short links created",
		TotalRows:  created + skipped,
		Created:    created,
		Skipped:    skipped,
		ScenarioID: newScenarioID,
	}, nil
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	// remove trailing slashes
	return strings.TrimRight(domain, "/")
}

func generateRandomBase62(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range n {
		var rb [1]byte
		_, _ = rand.Read(rb[:])
		b[i] = letters[int(rb[0])%len(letters)]
	}
	return string(b)
}

func (f *AdminShortLinkFlowImpl) DownloadShortLinksCSV(ctx context.Context, scenarioID uint) (string, []byte, error) {
	if scenarioID == 0 {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_id must be greater than 0", nil)
	}

	filter := models.ShortLinkFilter{ScenarioID: &scenarioID}
	rows, err := f.repo.ByFilter(ctx, filter, "id ASC", 0, 0)
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links", err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	alreadyFlushed := false
	defer func() {
		if !alreadyFlushed {
			w.Flush()
			alreadyFlushed = true
		}
	}()

	// Header: all current columns in short_links
	header := []string{
		"id",
		"uid",
		"campaign_id",
		"client_id",
		"scenario_id",
		"phone_number",
		"long_link",
		"short_link",
		"created_at",
		"updated_at",
	}
	if err := w.Write(header); err != nil {
		return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV header", err)
	}

	for _, r := range rows {
		campaignID := ""
		if r.CampaignID != nil {
			campaignID = strconv.FormatUint(uint64(*r.CampaignID), 10)
		}
		clientID := ""
		if r.ClientID != nil {
			clientID = strconv.FormatUint(uint64(*r.ClientID), 10)
		}
		scenario := ""
		if r.ScenarioID != nil {
			scenario = strconv.FormatUint(uint64(*r.ScenarioID), 10)
		}
		phone := ""
		if r.PhoneNumber != nil {
			phone = *r.PhoneNumber
		}

		record := []string{
			strconv.FormatUint(uint64(r.ID), 10),
			r.UID,
			campaignID,
			clientID,
			scenario,
			phone,
			r.LongLink,
			r.ShortLink,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if err := w.Write(record); err != nil {
			return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV row", err)
		}
	}

	filename := fmt.Sprintf("short_links_scenario_%d.csv", scenarioID)
	if !alreadyFlushed {
		w.Flush()
		alreadyFlushed = true
	}
	return filename, buf.Bytes(), nil
}

func (f *AdminShortLinkFlowImpl) DownloadShortLinksWithClicksCSV(ctx context.Context, scenarioID uint) (string, []byte, error) {
	if scenarioID == 0 {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_id must be greater than 0", nil)
	}

	rows, err := f.repo.ListByScenarioWithClicks(ctx, scenarioID, "id ASC")
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links with clicks", err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	alreadyFlushed := false
	defer func() {
		if !alreadyFlushed {
			w.Flush()
			alreadyFlushed = true
		}
	}()

	header := []string{
		"id",
		"uid",
		"campaign_id",
		"client_id",
		"scenario_id",
		"phone_number",
		"long_link",
		"short_link",
		"created_at",
		"updated_at",
	}
	if err := w.Write(header); err != nil {
		return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV header", err)
	}

	for _, r := range rows {
		campaignID := ""
		if r.CampaignID != nil {
			campaignID = strconv.FormatUint(uint64(*r.CampaignID), 10)
		}
		clientID := ""
		if r.ClientID != nil {
			clientID = strconv.FormatUint(uint64(*r.ClientID), 10)
		}
		scenario := ""
		if r.ScenarioID != nil {
			scenario = strconv.FormatUint(uint64(*r.ScenarioID), 10)
		}
		phone := ""
		if r.PhoneNumber != nil {
			phone = *r.PhoneNumber
		}

		record := []string{
			strconv.FormatUint(uint64(r.ID), 10),
			r.UID,
			campaignID,
			clientID,
			scenario,
			phone,
			r.LongLink,
			r.ShortLink,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if err := w.Write(record); err != nil {
			return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV row", err)
		}
	}

	filename := fmt.Sprintf("short_links_with_clicks_scenario_%d.csv", scenarioID)
	if !alreadyFlushed {
		w.Flush()
		alreadyFlushed = true
	}
	return filename, buf.Bytes(), nil
}
