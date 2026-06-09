package repository

import (
	"context"

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

func (r *platformBasePriceRepository) LatestByPlatform(ctx context.Context, platform string) (*models.PlatformBasePrice, error) {
	var pbp models.PlatformBasePrice
	err := r.db.WithContext(ctx).
		Where("platform = ?", platform).
		Order("id DESC").
		First(&pbp).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &pbp, nil
}
