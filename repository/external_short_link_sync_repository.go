package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExternalShortLinkSyncRepositoryImpl struct {
	db *gorm.DB
}

func NewExternalShortLinkSyncRepository(db *gorm.DB) ExternalShortLinkSyncRepository {
	return &ExternalShortLinkSyncRepositoryImpl{db: db}
}

func (r *ExternalShortLinkSyncRepositoryImpl) Cursor(ctx context.Context, source string) (int64, error) {
	var state models.ExternalShortLinkSyncState
	err := r.db.WithContext(ctx).Where("source = ?", source).Take(&state).Error
	if err == nil {
		return state.LastClickID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}
	state = models.ExternalShortLinkSyncState{Source: source, LastClickID: 0, UpdatedAt: time.Now().UTC()}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return 0, err
	}
	return 0, nil
}

func (r *ExternalShortLinkSyncRepositoryImpl) ImportPage(
	ctx context.Context,
	source string,
	clicks []models.ExternalShortLinkClick,
	throughClickID int64,
) error {
	if throughClickID < 0 {
		return fmt.Errorf("external click cursor cannot be negative")
	}
	return WithTransaction(ctx, r.db, func(txCtx context.Context) error {
		tx := r.db.WithContext(txCtx)
		if current, ok := txCtx.Value(TxContextKey).(*gorm.DB); ok && current != nil {
			tx = current.WithContext(txCtx)
		}

		codes := make([]string, 0, len(clicks))
		seen := make(map[string]struct{}, len(clicks))
		for _, click := range clicks {
			if click.ClickID <= 0 || click.ShortCode == "" || click.LongURL == "" || click.ClickedAt.IsZero() {
				return fmt.Errorf("external click %d is missing required fields", click.ClickID)
			}
			if _, exists := seen[click.ShortCode]; !exists {
				seen[click.ShortCode] = struct{}{}
				codes = append(codes, click.ShortCode)
			}
		}

		linksByCode := make(map[string]models.ShortLink, len(codes))
		const lookupChunkSize = 5000
		for start := 0; start < len(codes); start += lookupChunkSize {
			end := min(start+lookupChunkSize, len(codes))
			var links []models.ShortLink
			if err := tx.Where("uid IN ?", codes[start:end]).Find(&links).Error; err != nil {
				return fmt.Errorf("lookup production short links: %w", err)
			}
			for _, link := range links {
				linksByCode[link.UID] = link
			}
		}

		rows := make([]*models.ShortLinkClick, 0, len(clicks))
		for _, external := range clicks {
			link, exists := linksByCode[external.ShortCode]
			if !exists {
				return fmt.Errorf("production short link for external code %q does not exist", external.ShortCode)
			}
			if link.LongLink != external.LongURL {
				return fmt.Errorf("external code %q destination does not match the immutable production mapping", external.ShortCode)
			}
			uid := external.ShortCode
			longURL := external.LongURL
			shortURL := external.ShortURL
			if shortURL == nil {
				shortURL = &link.ShortLink
			}
			campaignID := external.CampaignID
			if campaignID == nil {
				campaignID = link.CampaignID
			}
			clientID := external.ClientID
			if clientID == nil {
				clientID = link.ClientID
			}
			scenarioID := external.ScenarioID
			if scenarioID == nil {
				scenarioID = link.ScenarioID
			}
			scenarioName := external.ScenarioName
			if scenarioName == nil {
				scenarioName = link.ScenarioName
			}
			phoneNumber := external.PhoneNumber
			if phoneNumber == nil {
				phoneNumber = link.PhoneNumber
			}
			linkCreatedAt := external.LinkCreatedAt
			if linkCreatedAt == nil {
				linkCreatedAt = &link.CreatedAt
			}
			linkUpdatedAt := external.LinkUpdatedAt
			if linkUpdatedAt == nil {
				linkUpdatedAt = &link.UpdatedAt
			}
			rows = append(rows, &models.ShortLinkClick{
				ShortLinkID:        link.ID,
				UID:                &uid,
				CampaignID:         campaignID,
				ClientID:           clientID,
				ScenarioID:         scenarioID,
				ScenarioName:       scenarioName,
				PhoneNumber:        phoneNumber,
				LongLink:           &longURL,
				ShortLink:          shortURL,
				ShortLinkCreatedAt: linkCreatedAt,
				ShortLinkUpdatedAt: linkUpdatedAt,
				UserAgent:          external.UserAgent,
				IP:                 external.ClientIP,
				Referer:            external.Referer,
				Source:             source,
				ExternalClickID:    &external.ClickID,
				CreatedAt:          external.ClickedAt.UTC(),
			})
		}

		if len(rows) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "source"}, {Name: "external_click_id"}},
				DoNothing: true,
			}).CreateInBatches(rows, 500).Error; err != nil {
				return fmt.Errorf("insert external clicks: %w", err)
			}
		}

		state := models.ExternalShortLinkSyncState{
			Source:      source,
			LastClickID: throughClickID,
			UpdatedAt:   time.Now().UTC(),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "source"}},
			DoUpdates: clause.Assignments(map[string]any{
				"last_click_id": gorm.Expr("GREATEST(external_short_link_sync_state.last_click_id, EXCLUDED.last_click_id)"),
				"updated_at":    state.UpdatedAt,
			}),
		}).Create(&state).Error; err != nil {
			return fmt.Errorf("advance external click cursor: %w", err)
		}
		return nil
	})
}
