package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// PagePriceRepositoryImpl implements PagePriceRepository.
type PagePriceRepositoryImpl struct {
	*BaseRepository[models.PagePrice, struct{}]
}

func NewPagePriceRepository(db *gorm.DB) PagePriceRepository {
	return &PagePriceRepositoryImpl{
		BaseRepository: NewBaseRepository[models.PagePrice, struct{}](db),
	}
}

func (r *PagePriceRepositoryImpl) Insert(ctx context.Context, p *models.PagePrice) error {
	db := r.getDB(ctx)
	return db.Create(p).Error
}

func (r *PagePriceRepositoryImpl) LatestByPlatform(ctx context.Context, platform string) (*models.PagePrice, error) {
	db := r.getDB(ctx)
	var row models.PagePrice
	err := db.Where("platform = ?", platform).
		Order("created_at DESC, id DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *PagePriceRepositoryImpl) ListLatest(ctx context.Context) ([]*models.PagePrice, error) {
	db := r.getDB(ctx)
	var rows []*models.PagePrice
	err := db.Raw(`
		SELECT DISTINCT ON (platform) id, platform, price, created_by_admin_id, created_at
		FROM page_prices
		ORDER BY platform, created_at DESC, id DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
