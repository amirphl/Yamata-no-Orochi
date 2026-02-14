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

	// Deduplicate by conflict key to avoid ON CONFLICT hitting same row twice in one statement
	type aggKey struct {
		pcID       uint
		customerID string
	}
	seen := make(map[aggKey]struct{}, len(rows))
	deduped := make([]*models.SMSStatusResult, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := aggKey{pcID: row.ProcessedCampaignID, customerID: row.CustomerID}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, row)
	}
	if len(deduped) == 0 {
		return nil
	}

	db := r.getDB(ctx)
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "processed_campaign_id"}, {Name: "customer_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"job_id":                  clause.Expr{SQL: "EXCLUDED.job_id"},
			"server_id":               clause.Expr{SQL: "EXCLUDED.server_id"},
			"total_parts":             clause.Expr{SQL: "EXCLUDED.total_parts"},
			"total_delivered_parts":   clause.Expr{SQL: "EXCLUDED.total_delivered_parts"},
			"total_undelivered_parts": clause.Expr{SQL: "EXCLUDED.total_undelivered_parts"},
			"total_unknown_parts":     clause.Expr{SQL: "EXCLUDED.total_unknown_parts"},
			"status":                  clause.Expr{SQL: "EXCLUDED.status"},
			"created_at":              clause.Expr{SQL: "LEAST(sms_status_results.created_at, EXCLUDED.created_at)"},
		}),
	}).Create(&deduped).Error
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
	AggregatedTotalSent      int64
	AggregatedTotalParts     int64
	AggregatedDeliveredParts int64
	AggregatedUndelivered    int64
	AggregatedUnknown        int64
}

func (r *SMSStatusResultRepositoryImpl) AggregateByCampaign(ctx context.Context, processedCampaignID uint) (*SMSStatusAggregates, error) {
	// TODO: Aggregate on status too.
	// TODO: Query optimization: maintain a summary table updated on insert instead of aggregating on the fly.
	db := r.getDB(ctx)
	var agg SMSStatusAggregates
	if err := db.Table("sms_status_results").
		Select(`
			COUNT(*) AS aggregated_total_sent,
			COALESCE(SUM(total_parts),0) AS aggregated_total_parts,
			COALESCE(SUM(total_delivered_parts),0) AS aggregated_delivered_parts,
			COALESCE(SUM(total_undelivered_parts),0) AS aggregated_undelivered,
			COALESCE(SUM(total_unknown_parts),0) AS aggregated_unknown`).
		Where("processed_campaign_id = ?", processedCampaignID).
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
