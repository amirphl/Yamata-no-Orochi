package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// AudienceProfileRepositoryImpl implements AudienceProfileRepository
type AudienceProfileRepositoryImpl struct {
	*BaseRepository[models.AudienceProfile, models.AudienceProfileFilter]
}

func NewAudienceProfileRepository(db *gorm.DB) AudienceProfileRepository {
	return &AudienceProfileRepositoryImpl{BaseRepository: NewBaseRepository[models.AudienceProfile, models.AudienceProfileFilter](db)}
}

func (r *AudienceProfileRepositoryImpl) ByID(ctx context.Context, id uint) (*models.AudienceProfile, error) {
	db := r.getDB(ctx)
	var ap models.AudienceProfile
	if err := db.Last(&ap, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ap, nil
}

func (r *AudienceProfileRepositoryImpl) ByUID(ctx context.Context, uid string) (*models.AudienceProfile, error) {
	rows, err := r.ByFilter(ctx, models.AudienceProfileFilter{UID: &uid}, "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *AudienceProfileRepositoryImpl) applyFilter(db *gorm.DB, f models.AudienceProfileFilter) *gorm.DB {
	if f.ID != nil {
		db = db.Where("id = ?", *f.ID)
	}
	if f.UID != nil {
		db = db.Where("uid = ?", *f.UID)
	}
	if f.PhoneNumber != nil {
		db = db.Where("phone_number = ?", *f.PhoneNumber)
	}
	if f.Tags != nil {
		db = db.Where("? = ANY (tags)", *f.Tags)
	}
	if f.Color != nil {
		db = db.Where("color = ?", *f.Color)
	}
	if f.CreatedAfter != nil {
		db = db.Where("created_at >= ?", *f.CreatedAfter)
	}
	if f.CreatedBefore != nil {
		db = db.Where("created_at < ?", *f.CreatedBefore)
	}
	return db
}

func (r *AudienceProfileRepositoryImpl) ByFilter(ctx context.Context, filter models.AudienceProfileFilter, orderBy string, limit, offset int) ([]*models.AudienceProfile, error) {
	db := r.getDB(ctx)
	base := r.applyFilter(db.Model(&models.AudienceProfile{}), filter)

	// Randomize first inside a subquery, then apply limit/offset on the randomized set
	randomized := base.Order("RANDOM()")
	if limit > 0 {
		randomized = randomized.Limit(limit)
	}
	if offset > 0 {
		randomized = randomized.Offset(offset)
	}

	// Outer query applies the final stable ordering for the selected subset
	outer := db.Table("(?) AS ap", randomized).Order("id ASC")

	var rows []*models.AudienceProfile
	if err := outer.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to find audience profiles by filter: %w", err)
	}
	return rows, nil
}

func (r *AudienceProfileRepositoryImpl) Count(ctx context.Context, filter models.AudienceProfileFilter) (int64, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.AudienceProfile{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *AudienceProfileRepositoryImpl) Exists(ctx context.Context, filter models.AudienceProfileFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
