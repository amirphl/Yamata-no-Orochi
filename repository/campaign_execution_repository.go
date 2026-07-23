package repository

import (
	"context"
	"errors"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

// LockCampaignForUpdate serializes status transitions and approval side
// effects for one campaign. The caller must provide the transaction through
// repository.WithTransaction's context.
func LockCampaignForUpdate(ctx context.Context, campaignID uint) error {
	tx, err := transactionForLock(ctx)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec("SELECT id FROM campaigns WHERE id = ? FOR UPDATE", campaignID).Error
}

func transactionForLock(ctx context.Context) (*gorm.DB, error) {
	tx, ok := ctx.Value(TxContextKey).(*gorm.DB)
	if !ok || tx == nil {
		return nil, errors.New("row lock requires an active transaction")
	}
	return tx, nil
}

func LockBundleForUpdate(ctx context.Context, bundleID uint) error {
	tx, err := transactionForLock(ctx)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec("SELECT id FROM bundles WHERE id = ? FOR UPDATE", bundleID).Error
}

func LockBundleForShare(ctx context.Context, bundleID uint) error {
	tx, err := transactionForLock(ctx)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec("SELECT id FROM bundles WHERE id = ? FOR SHARE", bundleID).Error
}

// ReleaseUnpreparedCampaign returns a scheduler claim to the durable approved
// queue only when no processed-campaign checkpoint exists. Once a checkpoint
// exists, delivery may have started and an automatic replay would be unsafe.
func ReleaseUnpreparedCampaign(ctx context.Context, db *gorm.DB, campaignID uint) error {
	return db.WithContext(ctx).Model(&models.Campaign{}).
		Where(`id = ? AND status = ? AND NOT EXISTS (
			SELECT 1 FROM processed_campaigns WHERE processed_campaigns.campaign_id = campaigns.id
		)`, campaignID, models.CampaignStatusRunning).
		Updates(map[string]any{"status": models.CampaignStatusApproved, "updated_at": utils.UTCNow()}).Error
}

// ReleaseStaleUnpreparedCampaigns recovers claims left behind by a process
// crash before the processed-campaign checkpoint. No provider delivery can
// have started before that checkpoint, so these rows are safe to retry.
func ReleaseStaleUnpreparedCampaigns(ctx context.Context, db *gorm.DB, staleBefore time.Time) (int64, error) {
	result := db.WithContext(ctx).Model(&models.Campaign{}).
		Where(`status = ? AND updated_at < ? AND NOT EXISTS (
			SELECT 1 FROM processed_campaigns WHERE processed_campaigns.campaign_id = campaigns.id
		)`, models.CampaignStatusRunning, staleBefore).
		Updates(map[string]any{"status": models.CampaignStatusApproved, "updated_at": utils.UTCNow()})
	return result.RowsAffected, result.Error
}

type BundleCampaignAllocation struct {
	CampaignID   uint                  `gorm:"column:campaign_id"`
	NumAudience  *uint64               `gorm:"column:num_audience"`
	Status       models.CampaignStatus `gorm:"column:status"`
	Materialized bool                  `gorm:"column:materialized"`
}

// ListBundleCampaignAllocations returns the stable, ordered standard/smart
// reservations used by exact-capacity deductions and fingerprint validation.
// Excel targeting is intentionally excluded: its recipients are explicitly
// reusable and never participate in the bundle audience-selection ledger.
func ListBundleCampaignAllocations(ctx context.Context, db *gorm.DB, bundleID, excludedCampaignID uint) ([]BundleCampaignAllocation, error) {
	queryDB := db.WithContext(ctx)
	if tx, ok := ctx.Value(TxContextKey).(*gorm.DB); ok && tx != nil {
		queryDB = tx.WithContext(ctx)
	}
	var rows []BundleCampaignAllocation
	err := bundleCampaignAllocationsQuery(queryDB, bundleID, excludedCampaignID).
		Find(&rows).Error
	return rows, err
}

func bundleCampaignAllocationsQuery(db *gorm.DB, bundleID, excludedCampaignID uint) *gorm.DB {
	return db.Table("campaigns").
		Select(`id AS campaign_id, num_audience, status,
			EXISTS (
				SELECT 1
				FROM bundle_audience_selections AS selection
				WHERE selection.campaign_id = campaigns.id
			) AS materialized`).
		Where("bundle_id = ? AND id <> ? AND status IN ?", bundleID, excludedCampaignID, []models.CampaignStatus{
			models.CampaignStatusApproved, models.CampaignStatusRunning, models.CampaignStatusExecuted,
		}).
		Where(`CASE
			WHEN LOWER(BTRIM(COALESCE(spec->>'audience_targeting_method', ''))) IN (?, ?, ?)
				THEN LOWER(BTRIM(spec->>'audience_targeting_method'))
			WHEN NULLIF(BTRIM(spec->>'target_audience_excel_file_uuid'), '') IS NOT NULL
				THEN ?
			ELSE ?
		END <> ?`,
			models.CampaignAudienceTargetingStandard,
			models.CampaignAudienceTargetingSmart,
			models.CampaignAudienceTargetingExcel,
			models.CampaignAudienceTargetingExcel,
			models.CampaignAudienceTargetingStandard,
			models.CampaignAudienceTargetingExcel,
		).
		Order("id ASC")
}
