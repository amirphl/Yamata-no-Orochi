package repository

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SMSStatusResultRepositoryImpl implements SMSStatusResultRepository
type SMSStatusResultRepositoryImpl struct {
	*BaseRepository[models.SMSStatusResult, any]
}

func NewSMSStatusResultRepository(db *gorm.DB) SMSStatusResultRepository {
	return &SMSStatusResultRepositoryImpl{BaseRepository: NewBaseRepository[models.SMSStatusResult, any](db)}
}

func (r *SMSStatusResultRepositoryImpl) SaveBatch(ctx context.Context, rows []*models.SMSStatusResult) error {
	if len(rows) == 0 {
		return nil
	}
	db := r.getDB(ctx)
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "processed_campaign_id"}, {Name: "customer_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"job_id":                  clause.Expr("EXCLUDED.job_id"),
			"server_id":               clause.Expr("EXCLUDED.server_id"),
			"total_parts":             clause.Expr("EXCLUDED.total_parts"),
			"total_delivered_parts":   clause.Expr("EXCLUDED.total_delivered_parts"),
			"total_undelivered_parts": clause.Expr("EXCLUDED.total_undelivered_parts"),
			"total_unknown_parts":     clause.Expr("EXCLUDED.total_unknown_parts"),
			"status":                  clause.Expr("EXCLUDED.status"),
			"created_at":              clause.Expr("LEAST(sms_status_results.created_at, EXCLUDED.created_at)"),
		}),
	}).Create(&rows).Error
}

// ByFilter: no filter fields, just order/limit/offset
func (r *SMSStatusResultRepositoryImpl) ByFilter(ctx context.Context, _ any, orderBy string, limit, offset int) ([]*models.SMSStatusResult, error) {
	db := r.getDB(ctx)
	if orderBy != "" {
		db = db.Order(orderBy)
	}
	if limit > 0 {
		db = db.Limit(limit)
	}
	if offset > 0 {
		db = db.Offset(offset)
	}
	var rows []*models.SMSStatusResult
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SMSStatusResultRepositoryImpl) Count(ctx context.Context, _ any) (int64, error) {
	db := r.getDB(ctx)
	var count int64
	if err := db.Model(&models.SMSStatusResult{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

type SMSStatusAggregates struct {
	TotalSent                int64
	AggregatedTotalParts     int64
	AggregatedDeliveredParts int64
	AggregatedUndelivered    int64
	AggregatedUnknown        int64
}

func (r *SMSStatusResultRepositoryImpl) AggregateByCampaign(ctx context.Context, campaignID uint) (*SMSStatusAggregates, error) {
	db := r.getDB(ctx)
	var agg SMSStatusAggregates
	if err := db.Table("sms_status_results").
		Select(`
			COUNT(*) AS total_sent,
			COALESCE(SUM(total_parts),0) AS aggregated_total_parts,
			COALESCE(SUM(total_delivered_parts),0) AS aggregated_delivered_parts,
			COALESCE(SUM(total_undelivered_parts),0) AS aggregated_undelivered,
			COALESCE(SUM(total_unknown_parts),0) AS aggregated_unknown`).
		Where("campaign_id = ?", campaignID).
		Scan(&agg).Error; err != nil {
		return nil, err
	}
	return &agg, nil
}

func (r *SMSStatusResultRepositoryImpl) Exists(ctx context.Context, filter any) (bool, error) {
	c, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
