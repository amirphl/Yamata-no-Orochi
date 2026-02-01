package repository

import (
	"context"
	"errors"
	"sort"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

type AudienceSelectionRepository interface {
	Latest(ctx context.Context, customerID uint, tagsHash string) (*models.AudienceSelection, error)
	InsertWithMerge(ctx context.Context, customerID uint, tagsHash string, correlationID string, ids []int64) (*models.AudienceSelection, error)
	InsertSnapshot(ctx context.Context, customerID uint, tagsHash string, correlationID string, ids []int64) (*models.AudienceSelection, error)
}

type AudienceSelectionRepositoryImpl struct {
	DB *gorm.DB
}

func NewAudienceSelectionRepository(db *gorm.DB) AudienceSelectionRepository {
	return &AudienceSelectionRepositoryImpl{DB: db}
}

func (r *AudienceSelectionRepositoryImpl) getDB(ctx context.Context) *gorm.DB {
	return r.DB.WithContext(ctx)
}

func (r *AudienceSelectionRepositoryImpl) Latest(ctx context.Context, customerID uint, tagsHash string) (*models.AudienceSelection, error) {
	db := r.getDB(ctx)
	var row models.AudienceSelection
	err := db.Where("customer_id = ? AND tags_hash = ?", customerID, tagsHash).
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

func (r *AudienceSelectionRepositoryImpl) InsertWithMerge(ctx context.Context, customerID uint, tagsHash string, correlationID string, ids []int64) (*models.AudienceSelection, error) {
	db := r.getDB(ctx)
	var inserted models.AudienceSelection

	err := db.Transaction(func(tx *gorm.DB) error {
		currentIDs := dedupeAndSort(ids)
		var latest models.AudienceSelection
		err := tx.Where("customer_id = ? AND tags_hash = ?", customerID, tagsHash).
			Order("created_at DESC, id DESC").
			First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && len(latest.AudienceIDs) > 0 {
			currentIDs = dedupeAndSort(append(currentIDs, []int64(latest.AudienceIDs)...))
		}

		row := models.AudienceSelection{
			CustomerID:    customerID,
			TagsHash:      tagsHash,
			CorrelationID: correlationID,
			AudienceIDs:   currentIDs,
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

func (r *AudienceSelectionRepositoryImpl) InsertSnapshot(ctx context.Context, customerID uint, tagsHash string, correlationID string, ids []int64) (*models.AudienceSelection, error) {
	db := r.getDB(ctx)
	row := models.AudienceSelection{
		CustomerID:    customerID,
		TagsHash:      tagsHash,
		CorrelationID: correlationID,
		AudienceIDs:   dedupeAndSort(ids),
		CreatedAt:     utils.UTCNow(),
	}
	if err := db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func dedupeAndSort(ids []int64) []int64 {
	if len(ids) == 0 {
		return ids
	}
	m := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
