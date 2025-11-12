package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// ShortLinkRepositoryImpl implements ShortLinkRepository
type ShortLinkRepositoryImpl struct {
	*BaseRepository[models.ShortLink, models.ShortLinkFilter]
}

func NewShortLinkRepository(db *gorm.DB) ShortLinkRepository {
	return &ShortLinkRepositoryImpl{BaseRepository: NewBaseRepository[models.ShortLink, models.ShortLinkFilter](db)}
}

func (r *ShortLinkRepositoryImpl) ByID(ctx context.Context, id uint) (*models.ShortLink, error) {
	db := r.getDB(ctx)
	var row models.ShortLink
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *ShortLinkRepositoryImpl) ByUID(ctx context.Context, uid string) (*models.ShortLink, error) {
	filter := models.ShortLinkFilter{UID: &uid}
	rows, err := r.ByFilter(ctx, filter, "id DESC", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *ShortLinkRepositoryImpl) applyFilter(db *gorm.DB, f models.ShortLinkFilter) *gorm.DB {
	if f.ID != nil {
		db = db.Where("id = ?", *f.ID)
	}
	if f.UID != nil {
		db = db.Where("uid = ?", *f.UID)
	}
	if f.CampaignID != nil {
		db = db.Where("campaign_id = ?", *f.CampaignID)
	}
	if f.ClientID != nil {
		db = db.Where("client_id = ?", *f.ClientID)
	}
	if f.ScenarioID != nil {
		db = db.Where("scenario_id = ?", *f.ScenarioID)
	}
	if f.PhoneNumber != nil {
		db = db.Where("phone_number = ?", *f.PhoneNumber)
	}
	if f.CreatedAfter != nil {
		db = db.Where("created_at >= ?", *f.CreatedAfter)
	}
	if f.CreatedBefore != nil {
		db = db.Where("created_at < ?", *f.CreatedBefore)
	}
	return db
}

func (r *ShortLinkRepositoryImpl) ByFilter(ctx context.Context, filter models.ShortLinkFilter, orderBy string, limit, offset int) ([]*models.ShortLink, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.ShortLink{}), filter)
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []*models.ShortLink
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *ShortLinkRepositoryImpl) Count(ctx context.Context, filter models.ShortLinkFilter) (int64, error) {
	db := r.getDB(ctx)
	query := r.applyFilter(db.Model(&models.ShortLink{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *ShortLinkRepositoryImpl) Exists(ctx context.Context, filter models.ShortLinkFilter) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}

func (r *ShortLinkRepositoryImpl) ListByScenarioWithClicks(ctx context.Context, scenarioID uint, orderBy string) ([]*models.ShortLink, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.ShortLink{}).
		Where("scenario_id = ?", scenarioID).
		Where("EXISTS (SELECT 1 FROM short_link_clicks c WHERE c.short_link_id = short_links.id)")
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	var rows []*models.ShortLink
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *ShortLinkRepositoryImpl) GetLastScenarioID(ctx context.Context) (uint, error) {
	db := r.getDB(ctx)
	var max sql.NullInt64
	if err := db.Model(&models.ShortLink{}).Select("MAX(scenario_id)").Scan(&max).Error; err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return uint(max.Int64), nil
}
