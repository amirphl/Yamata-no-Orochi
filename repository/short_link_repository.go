package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if f.PhoneNumber != nil {
		db = db.Where("phone_number = ?", *f.PhoneNumber)
	}
	if f.ClicksMin != nil {
		db = db.Where("clicks >= ?", *f.ClicksMin)
	}
	if f.ClicksMax != nil {
		db = db.Where("clicks <= ?", *f.ClicksMax)
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

func (r *ShortLinkRepositoryImpl) IncrementClicksByUID(ctx context.Context, uid string, userAgent *string, ip *string) error {
	db, shouldCommit, err := r.getDBForWrite(ctx)
	if err != nil {
		return err
	}
	if shouldCommit {
		defer func() {
			if err != nil {
				db.Rollback()
			} else {
				db.Commit()
			}
		}()
	}
	updates := map[string]any{
		"clicks":     gorm.Expr("clicks + ?", 1),
		"updated_at": utils.UTCNow(),
	}
	if userAgent != nil {
		updates["user_agent"] = *userAgent
	}
	if ip != nil {
		updates["ip"] = *ip
	}
	res := db.Model(&models.ShortLink{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("uid = ?", uid).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("short link not found")
	}
	return nil
}
