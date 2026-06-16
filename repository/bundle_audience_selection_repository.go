package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

type BundleAudienceSelectionRepository interface {
	Latest(ctx context.Context, customerID uint, bundleID uint) (*models.BundleAudienceSelection, error)
	InsertWithMerge(ctx context.Context, customerID uint, bundleID uint, correlationID string, ids []int64) (*models.BundleAudienceSelection, error)
}

type BundleAudienceSelectionRepositoryImpl struct {
	DB *gorm.DB
}

func NewBundleAudienceSelectionRepository(db *gorm.DB) BundleAudienceSelectionRepository {
	return &BundleAudienceSelectionRepositoryImpl{DB: db}
}

func (r *BundleAudienceSelectionRepositoryImpl) getDB(ctx context.Context) *gorm.DB {
	return r.DB.WithContext(ctx)
}

// Latest returns the most recent selection snapshot for the given (customer_id, bundle_id) pair,
// or nil if no selection has been recorded yet for this bundle.
func (r *BundleAudienceSelectionRepositoryImpl) Latest(ctx context.Context, customerID uint, bundleID uint) (*models.BundleAudienceSelection, error) {
	db := r.getDB(ctx)
	var row models.BundleAudienceSelection
	err := db.Where("customer_id = ? AND bundle_id = ?", customerID, bundleID).
		Order("created_at DESC, id DESC").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// InsertWithMerge inserts a new selection snapshot that merges ids with the existing
// cumulative set for this bundle. The new row becomes the authoritative latest snapshot.
func (r *BundleAudienceSelectionRepositoryImpl) InsertWithMerge(ctx context.Context, customerID uint, bundleID uint, correlationID string, ids []int64) (*models.BundleAudienceSelection, error) {
	db := r.getDB(ctx)
	var inserted models.BundleAudienceSelection

	err := db.Transaction(func(tx *gorm.DB) error {
		merged := dedupeAndSort(ids)

		var latest models.BundleAudienceSelection
		err := tx.Where("customer_id = ? AND bundle_id = ?", customerID, bundleID).
			Order("created_at DESC, id DESC").
			First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && len(latest.AudienceIDs) > 0 {
			merged = dedupeAndSort(append(merged, []int64(latest.AudienceIDs)...))
		}

		row := models.BundleAudienceSelection{
			CustomerID:    customerID,
			BundleID:      bundleID,
			CorrelationID: correlationID,
			AudienceIDs:   merged,
			CreatedAt:     utils.UTCNow(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		inserted = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &inserted, nil
}
