package businessflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"gorm.io/gorm"
)

// BotShortLinkFlow handles creation of short links by bot
// Validates inputs and persists records in a transaction for batch
// UID must be unique and provided by the caller
// CampaignID is optional (no FK)
type BotShortLinkFlow interface {
	CreateShortLink(ctx context.Context, req *dto.BotCreateShortLinkRequest) (*dto.BotCreateShortLinkResponse, error)
	CreateShortLinks(ctx context.Context, req *dto.BotCreateShortLinksRequest) (*dto.BotCreateShortLinksResponse, error)
	GenerateAndCreateShortLinks(ctx context.Context, campaignID uint, adLink string, phones []string, shortLinkDomain string) ([]string, error)
}

type BotShortLinkFlowImpl struct {
	shortRepo repository.ShortLinkRepository
	db        *gorm.DB
}

func NewBotShortLinkFlow(shortRepo repository.ShortLinkRepository, db *gorm.DB) BotShortLinkFlow {
	return &BotShortLinkFlowImpl{shortRepo: shortRepo, db: db}
}

func (s *BotShortLinkFlowImpl) CreateShortLink(ctx context.Context, req *dto.BotCreateShortLinkRequest) (*dto.BotCreateShortLinkResponse, error) {
	if req == nil {
		return nil, NewBusinessError("VALIDATION_ERROR", "Request body is required", nil)
	}
	if req.UID == "" || req.LongLink == "" || req.ShortLink == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "uid, long_link and short_link are required", nil)
	}

	lockShortLinkGen()
	defer unlockShortLinkGen()

	// read last scenario id from last short link from database and increment it
	lastScenarioID, err := s.shortRepo.GetLastScenarioID(ctx)
	if err != nil {
		return nil, NewBusinessError("FETCH_SCENARIO_ID_FAILED", "Failed to determine next scenario id", err)
	}
	newScenarioID := lastScenarioID + 1

	row := &models.ShortLink{
		UID:         req.UID,
		CampaignID:  req.CampaignID,
		ClientID:    req.ClientID,
		ScenarioID:  &newScenarioID,
		PhoneNumber: req.PhoneNumber,
		LongLink:    req.LongLink,
		ShortLink:   req.ShortLink,
	}
	if err := s.shortRepo.Save(ctx, row); err != nil {
		return nil, NewBusinessError("BOT_CREATE_SHORT_LINK_FAILED", "Failed to create short link", err)
	}
	return &dto.BotCreateShortLinkResponse{
		Message: "Short link created",
		Item:    mapShortLinkDTO(row),
	}, nil
}

func (s *BotShortLinkFlowImpl) CreateShortLinks(ctx context.Context, req *dto.BotCreateShortLinksRequest) (*dto.BotCreateShortLinksResponse, error) {
	if req == nil || len(req.Items) == 0 {
		return nil, NewBusinessError("VALIDATION_ERROR", "items must contain at least one element", nil)
	}

	lockShortLinkGen()
	defer unlockShortLinkGen()

	// read last scenario id from last short link from database and increment it
	lastScenarioID, err := s.shortRepo.GetLastScenarioID(ctx)
	if err != nil {
		return nil, NewBusinessError("FETCH_SCENARIO_ID_FAILED", "Failed to determine next scenario id", err)
	}
	newScenarioID := lastScenarioID + 1

	rows := make([]*models.ShortLink, 0, len(req.Items))
	for _, it := range req.Items {
		if it.UID == "" || it.LongLink == "" || it.ShortLink == "" {
			return nil, NewBusinessError("VALIDATION_ERROR", "uid, long_link and short_link are required for all items", nil)
		}
		rows = append(rows, &models.ShortLink{
			UID:         it.UID,
			CampaignID:  it.CampaignID,
			ClientID:    it.ClientID,
			ScenarioID:  &newScenarioID,
			PhoneNumber: it.PhoneNumber,
			LongLink:    it.LongLink,
			ShortLink:   it.ShortLink,
		})
	}

	// Persist in a single transaction for consistency
	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		return s.shortRepo.SaveBatch(txCtx, rows)
	}); err != nil {
		return nil, NewBusinessError("BOT_CREATE_SHORT_LINKS_FAILED", "Failed to create short links", err)
	}

	out := make([]dto.ShortLinkDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapShortLinkDTO(r))
	}
	return &dto.BotCreateShortLinksResponse{
		Message: "Short links created",
		Items:   out,
	}, nil
}

// GenerateAndCreateShortLinks generates sequential UIDs centrally and creates short links for phones
// Returns codes in the same order as phones.
func (s *BotShortLinkFlowImpl) GenerateAndCreateShortLinks(ctx context.Context, campaignID uint, adLink string, phones []string, shortLinkDomain string) ([]string, error) {
	adLink = strings.TrimSpace(adLink)
	if adLink == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "adLink is required", nil)
	}
	if len(phones) == 0 {
		return []string{}, nil
	}
	shortLinkDomain = normalizeDomain(shortLinkDomain)
	if shortLinkDomain == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "short_link_domain is required", nil)
	}

	lockShortLinkGen()
	defer unlockShortLinkGen()

	// compute starting UID sequence
	cutoff := time.Date(2025, 11, 10, 15, 45, 11, 401492000, time.UTC)
	lastUID, err := s.shortRepo.GetMaxUIDSince(ctx, cutoff)
	if err != nil {
		return nil, NewBusinessError("FETCH_MAX_UID_FAILED", "Failed to determine highest uid since cutoff", err)
	}
	var seq uint64
	if lastUID != "" {
		n, err := decodeBase36Compat(lastUID)
		if err != nil {
			return nil, NewBusinessError("INVALID_EXISTING_UID", "Found invalid uid in database", err)
		}
		seq = n + 1
	} else {
		seq = 0
	}

	codes := make([]string, len(phones))
	rows := make([]*models.ShortLink, 0, len(phones))
	// new scenario id
	lastScenarioID, err := s.shortRepo.GetLastScenarioID(ctx)
	if err != nil {
		return nil, NewBusinessError("FETCH_SCENARIO_ID_FAILED", "Failed to determine next scenario id", err)
	}
	newScenarioID := lastScenarioID + 1

	for i := range phones {
		uid, err := formatSequentialUIDCompat(seq)
		if err != nil {
			return nil, NewBusinessError("UID_SEQUENCE_EXHAUSTED", "No more UIDs available up to zzzzz", err)
		}
		seq++
		codes[i] = uid
		shortURL := fmt.Sprintf("%s/%s", shortLinkDomain, uid)
		p := phones[i]
		rows = append(rows, &models.ShortLink{
			UID:         uid,
			CampaignID:  &campaignID,
			ClientID:    nil,
			ScenarioID:  &newScenarioID,
			PhoneNumber: &p,
			LongLink:    adLink,
			ShortLink:   shortURL,
		})
	}

	if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		return s.shortRepo.SaveBatch(txCtx, rows)
	}); err != nil {
		return nil, NewBusinessError("BOT_CREATE_SHORT_LINKS_FAILED", "Failed to create short links", err)
	}

	return codes, nil
}

// duplicate minimal helpers (kept private to this file) for base36 sequential IDs
func decodeBase36Compat(s string) (uint64, error) {
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'z':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			v = int(c-'A') + 10
		default:
			return 0, fmt.Errorf("invalid base36 character: %q", c)
		}
		if v >= 36 {
			return 0, fmt.Errorf("invalid base36 value: %d", v)
		}
		n = n*36 + uint64(v)
	}
	return n, nil
}

func formatSequentialUIDCompat(seq uint64) (string, error) {
	s := encodeBase36Compat(seq)
	if len(s) < 4 {
		s = strings.Repeat("0", 4-len(s)) + s
	}
	if len(s) > 5 {
		return "", fmt.Errorf("sequence exhausted at %s", s)
	}
	return s, nil
}

func encodeBase36Compat(n uint64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 16)
	for n > 0 {
		r := n % 36
		buf = append(buf, digits[r])
		n /= 36
	}
	// reverse in place
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func mapShortLinkDTO(m *models.ShortLink) dto.ShortLinkDTO {
	return dto.ShortLinkDTO{
		ID:          m.ID,
		UID:         m.UID,
		CampaignID:  m.CampaignID,
		ClientID:    m.ClientID,
		PhoneNumber: m.PhoneNumber,
		LongLink:    m.LongLink,
		ShortLink:   m.ShortLink,
	}
}
