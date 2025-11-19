package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// ShortLinkWithClick is a projection row combining short link columns with click context fields
type ShortLinkWithClick struct {
	ID             uint      `gorm:"column:id"`
	UID            string    `gorm:"column:uid"`
	CampaignID     *uint     `gorm:"column:campaign_id"`
	ClientID       *uint     `gorm:"column:client_id"`
	ScenarioID     *uint     `gorm:"column:scenario_id"`
	PhoneNumber    *string   `gorm:"column:phone_number"`
	LongLink       string    `gorm:"column:long_link"`
	ShortLink      string    `gorm:"column:short_link"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	ClickUserAgent *string   `gorm:"column:click_user_agent"`
	ClickIP        *string   `gorm:"column:click_ip"`
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
		Select("short_links.*").
		Joins("JOIN short_link_clicks c ON c.short_link_id = short_links.id AND c.scenario_id = ?", scenarioID)
	if orderBy != "" {
		query = query.Order(orderBy)
	}
	var rows []*models.ShortLink
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *ShortLinkRepositoryImpl) ListWithClicksDetailsByScenario(ctx context.Context, scenarioID uint, orderBy string) ([]*ShortLinkWithClick, error) {
	db := r.getDB(ctx)
	q := db.Table("short_links").
		Select(`short_links.id,
			short_links.uid,
			short_links.campaign_id,
			short_links.client_id,
			short_links.scenario_id,
			short_links.phone_number,
			short_links.long_link,
			short_links.short_link,
			short_links.created_at,
			short_links.updated_at,
			c.user_agent AS click_user_agent,
			c.ip AS click_ip`).
		Joins("JOIN short_link_clicks c ON c.short_link_id = short_links.id AND c.scenario_id = ?", scenarioID)
	if orderBy != "" {
		q = q.Order(orderBy)
	}
	var out []*ShortLinkWithClick
	if err := q.Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ShortLinkRepositoryImpl) ListWithClicksDetailsByScenarioRange(ctx context.Context, scenarioFrom, scenarioTo uint, orderBy string) ([]*ShortLinkWithClick, error) {
	db := r.getDB(ctx)
	q := db.Table("short_links").
		Select(`short_links.id,
			short_links.uid,
			short_links.campaign_id,
			short_links.client_id,
			short_links.scenario_id,
			short_links.phone_number,
			short_links.long_link,
			short_links.short_link,
			short_links.created_at,
			short_links.updated_at,
			c.user_agent AS click_user_agent,
			c.ip AS click_ip`).
		Joins("JOIN short_link_clicks c ON c.short_link_id = short_links.id AND c.scenario_id >= ? AND c.scenario_id < ?", scenarioFrom, scenarioTo)
	if orderBy != "" {
		q = q.Order(orderBy)
	}
	var out []*ShortLinkWithClick
	if err := q.Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
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
