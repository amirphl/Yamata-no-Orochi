package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

func NewPlatformBasePriceRepository(db *gorm.DB) PlatformBasePriceRepository {
	return &platformBasePriceRepository{db: db}
}

type platformBasePriceRepository struct {
	db *gorm.DB
}

func (r *platformBasePriceRepository) Insert(ctx context.Context, p *models.PlatformBasePrice) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *platformBasePriceRepository) UpdatePriceByPlatform(ctx context.Context, platform string, price uint64) error {
	result := r.db.WithContext(ctx).
		Model(&models.PlatformBasePrice{}).
		Where("platform = ? AND deleted_at IS NULL", platform).
		Update("price", price)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *platformBasePriceRepository) LatestByPlatform(ctx context.Context, platform string) (*models.PlatformBasePrice, error) {
	var pbp models.PlatformBasePrice
	err := r.db.WithContext(ctx).
		Where("platform = ?", platform).
		Order("id DESC").
		First(&pbp).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &pbp, nil
}

func (r *platformBasePriceRepository) List(ctx context.Context) ([]*models.PlatformBasePrice, error) {
	var rows []*models.PlatformBasePrice

	latestByPlatformSubQuery := r.db.WithContext(ctx).
		Model(&models.PlatformBasePrice{}).
		Select("platform, MAX(id) AS id").
		Where("deleted_at IS NULL").
		Group("platform")

	if err := r.db.WithContext(ctx).
		Model(&models.PlatformBasePrice{}).
		Joins("JOIN (?) AS latest ON latest.id = platform_base_prices.id", latestByPlatformSubQuery).
		Order("platform_base_prices.platform ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
