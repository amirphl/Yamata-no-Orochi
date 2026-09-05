package repository

import (
	"context"
	"errors"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// CampaignStatusJobRepositoryImpl implements CampaignStatusJobRepository
type CampaignStatusJobRepositoryImpl struct {
	*BaseRepository[models.CampaignStatusJob, any]
	tableName string
}

func NewCampaignStatusJobRepository(db *gorm.DB) CampaignStatusJobRepository {
	return newCampaignStatusJobRepository(db, "campaign_status_jobs")
}

// NewPayamSMSStatusJobRepository persists Payam delivery polling jobs.
func NewPayamSMSStatusJobRepository(db *gorm.DB) CampaignStatusJobRepository {
	return newCampaignStatusJobRepository(db, "payam_sms_status_jobs")
}

// NewCandooSMSStatusJobRepository persists Candoo delivery polling jobs.
func NewCandooSMSStatusJobRepository(db *gorm.DB) CampaignStatusJobRepository {
	return newCampaignStatusJobRepository(db, "candoo_sms_status_jobs")
}

func newCampaignStatusJobRepository(db *gorm.DB, tableName string) *CampaignStatusJobRepositoryImpl {
	return &CampaignStatusJobRepositoryImpl{BaseRepository: NewBaseRepository[models.CampaignStatusJob, any](db), tableName: tableName}
}

func (r *CampaignStatusJobRepositoryImpl) table() string {
	if r.tableName == "" {
		return "campaign_status_jobs"
	}
	return r.tableName
}

func (r *CampaignStatusJobRepositoryImpl) Save(ctx context.Context, job *models.CampaignStatusJob) error {
	return r.getDB(ctx).Table(r.table()).Create(job).Error
}

func (r *CampaignStatusJobRepositoryImpl) ByID(ctx context.Context, id uint) (*models.CampaignStatusJob, error) {
	db := r.getDB(ctx).Table(r.table())
	var row models.CampaignStatusJob
	if err := db.Last(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ListDue returns jobs for one platform scheduled before or at 'now' with retry_count < 3.
func (r *CampaignStatusJobRepositoryImpl) ListDue(ctx context.Context, platform string, now time.Time, limit int) ([]*models.CampaignStatusJob, error) {
	if limit <= 0 {
		limit = 100
	}
	db := r.getDB(ctx)
	var rows []*models.CampaignStatusJob
	if err := db.Where("platform = ? AND scheduled_at <= ? AND retry_count < ? AND executed_at IS NULL", platform, now, 3).
		Order("scheduled_at ASC, id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CampaignStatusJobRepositoryImpl) SaveBatch(ctx context.Context, jobs []*models.CampaignStatusJob) error {
	if len(jobs) == 0 {
		return nil
	}
	return r.getDB(ctx).Table(r.table()).CreateInBatches(jobs, 1000).Error
}

func (r *CampaignStatusJobRepositoryImpl) Update(ctx context.Context, job *models.CampaignStatusJob) error {
	db := r.getDB(ctx)
	return db.Table(r.table()).Save(job).Error
}

// ByFilter: since no filter is defined, apply order/limit/offset only
func (r *CampaignStatusJobRepositoryImpl) ByFilter(ctx context.Context, _ any, orderBy string, limit, offset int) ([]*models.CampaignStatusJob, error) {
	db := r.getDB(ctx).Table(r.table())
	if orderBy != "" {
		db = db.Order(orderBy)
	}
	if limit > 0 {
		db = db.Limit(limit)
	}
	if offset > 0 {
		db = db.Offset(offset)
	}
	var rows []*models.CampaignStatusJob
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CampaignStatusJobRepositoryImpl) Count(ctx context.Context, _ any) (int64, error) {
	db := r.getDB(ctx).Table(r.table())
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *CampaignStatusJobRepositoryImpl) Exists(ctx context.Context, filter any) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
