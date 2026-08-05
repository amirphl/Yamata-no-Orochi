package repository

import (
	"context"
	"errors"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// CampaignTargetingCapacityRepository keeps the calculation generation
// lifecycle small and explicit. Expensive query execution stays in the flow,
// where its input snapshot can be validated before completion.
type CampaignTargetingCapacityRepository interface {
	Save(ctx context.Context, row *models.CampaignTargetingCapacityCalculation) error
	ByID(ctx context.Context, id int64) (*models.CampaignTargetingCapacityCalculation, error)
	LatestByCampaignID(ctx context.Context, campaignID uint) (*models.CampaignTargetingCapacityCalculation, error)
	ClaimPending(ctx context.Context, limit int, staleBefore, at time.Time) ([]*models.CampaignTargetingCapacityCalculation, error)
	Complete(ctx context.Context, id int64, leaseStartedAt time.Time, eligible, deduction, usable int64, fingerprint string, at time.Time) error
	Fail(ctx context.Context, id int64, leaseStartedAt time.Time, code, message string, at time.Time) error
	DeleteExpiredCandidates(ctx context.Context, before time.Time, limit int) (int64, error)
}

var ErrCampaignTargetingCapacityStateConflict = errors.New("campaign targeting capacity calculation state changed")

type CampaignTargetingCapacityRepositoryImpl struct{ db *gorm.DB }

func NewCampaignTargetingCapacityRepository(db *gorm.DB) CampaignTargetingCapacityRepository {
	return &CampaignTargetingCapacityRepositoryImpl{db: db}
}

func (r *CampaignTargetingCapacityRepositoryImpl) getDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(TxContextKey).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *CampaignTargetingCapacityRepositoryImpl) Save(ctx context.Context, row *models.CampaignTargetingCapacityCalculation) error {
	return r.getDB(ctx).Create(row).Error
}

func (r *CampaignTargetingCapacityRepositoryImpl) ByID(ctx context.Context, id int64) (*models.CampaignTargetingCapacityCalculation, error) {
	var row models.CampaignTargetingCapacityCalculation
	if err := r.getDB(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *CampaignTargetingCapacityRepositoryImpl) LatestByCampaignID(ctx context.Context, campaignID uint) (*models.CampaignTargetingCapacityCalculation, error) {
	var row models.CampaignTargetingCapacityCalculation
	err := r.getDB(ctx).Where("campaign_id = ?", campaignID).Order("created_at DESC, id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ClaimPending atomically leases calculating rows across every application
// replica. Timed-out leases can be reclaimed after staleBefore.
func (r *CampaignTargetingCapacityRepositoryImpl) ClaimPending(ctx context.Context, limit int, staleBefore, at time.Time) ([]*models.CampaignTargetingCapacityCalculation, error) {
	if limit <= 0 {
		limit = 1
	}
	var rows []*models.CampaignTargetingCapacityCalculation
	query := `
WITH claimable AS (
    SELECT id
    FROM campaign_targeting_capacity_calculations
    WHERE status = ?
      AND (started_at IS NULL OR started_at < ?)
    ORDER BY created_at ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT ?
)
UPDATE campaign_targeting_capacity_calculations AS calculation
SET started_at = ?
FROM claimable
WHERE calculation.id = claimable.id
RETURNING calculation.*`
	return rows, r.getDB(ctx).Raw(query, models.CampaignTargetingCapacityCalculating, staleBefore, limit, at).Scan(&rows).Error
}

func (r *CampaignTargetingCapacityRepositoryImpl) Complete(ctx context.Context, id int64, leaseStartedAt time.Time, eligible, deduction, usable int64, fingerprint string, at time.Time) error {
	result := r.getDB(ctx).Model(&models.CampaignTargetingCapacityCalculation{}).
		Where("id = ? AND status = ? AND started_at = ?", id, models.CampaignTargetingCapacityCalculating, leaseStartedAt).
		Updates(map[string]any{
			"eligible_unique_audience_count": eligible,
			"approved_campaign_deduction":    deduction,
			"usable_unique_audience_count":   usable,
			"allocation_fingerprint":         fingerprint,
			"status":                         models.CampaignTargetingCapacityCalculated,
			"finished_at":                    at,
			"error_code":                     nil,
			"error_message":                  nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCampaignTargetingCapacityStateConflict
	}
	return nil
}

func (r *CampaignTargetingCapacityRepositoryImpl) Fail(ctx context.Context, id int64, leaseStartedAt time.Time, code, message string, at time.Time) error {
	result := r.getDB(ctx).Model(&models.CampaignTargetingCapacityCalculation{}).
		Where("id = ? AND status = ? AND started_at = ?", id, models.CampaignTargetingCapacityCalculating, leaseStartedAt).
		Updates(map[string]any{
			"status":        models.CampaignTargetingCapacityFailed,
			"finished_at":   at,
			"error_code":    code,
			"error_message": message,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCampaignTargetingCapacityStateConflict
	}
	return nil
}

func (r *CampaignTargetingCapacityRepositoryImpl) DeleteExpiredCandidates(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 10000
	}
	result := r.getDB(ctx).Exec(`
DELETE FROM campaign_targeting_candidate_stack
WHERE id IN (
    SELECT id
    FROM campaign_targeting_candidate_stack
    WHERE expires_at < ?
    ORDER BY expires_at ASC, id ASC
    LIMIT ?
)`, before, limit)
	return result.RowsAffected, result.Error
}
