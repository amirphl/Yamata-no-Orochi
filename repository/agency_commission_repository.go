package repository

import (
	"context"
	"errors"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgencyCommissionRepositoryImpl implements AgencyCommissionRepository interface
type AgencyCommissionRepositoryImpl struct {
	*BaseRepository[models.AgencyCommission, models.AgencyCommissionFilter]
}

// NewAgencyCommissionRepository creates a new agency commission repository
func NewAgencyCommissionRepository(db *gorm.DB) AgencyCommissionRepository {
	return &AgencyCommissionRepositoryImpl{
		BaseRepository: NewBaseRepository[models.AgencyCommission, models.AgencyCommissionFilter](db),
	}
}

// ByID finds an agency commission by ID
func (r *AgencyCommissionRepositoryImpl) ByID(ctx context.Context, id uint) (*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commission models.AgencyCommission
	err := db.Last(&commission, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &commission, nil
}

// ByUUID finds an agency commission by UUID
func (r *AgencyCommissionRepositoryImpl) ByUUID(ctx context.Context, uuid string) (*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commission models.AgencyCommission
	err := db.Where("uuid = ?", uuid).Last(&commission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &commission, nil
}

// ByCorrelationID finds agency commissions by correlation ID
func (r *AgencyCommissionRepositoryImpl) ByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission
	err := db.Where("correlation_id = ?", correlationID).Order("created_at DESC").Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// ByAgencyID finds agency commissions by agency ID
func (r *AgencyCommissionRepositoryImpl) ByAgencyID(ctx context.Context, agencyID uint, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Where("agency_id = ?", agencyID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// ByCustomerID finds agency commissions by customer ID
func (r *AgencyCommissionRepositoryImpl) ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Where("customer_id = ?", customerID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// ByWalletID finds agency commissions by wallet ID
func (r *AgencyCommissionRepositoryImpl) ByWalletID(ctx context.Context, walletID uint, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Where("wallet_id = ?", walletID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// ByType finds agency commissions by type
func (r *AgencyCommissionRepositoryImpl) ByType(ctx context.Context, commissionType models.CommissionType, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Where("type = ?", commissionType).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// ByStatus finds agency commissions by status
func (r *AgencyCommissionRepositoryImpl) ByStatus(ctx context.Context, status models.CommissionStatus, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Where("status = ?", status).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// BySourceTransaction finds agency commissions by source transaction ID
func (r *AgencyCommissionRepositoryImpl) BySourceTransaction(ctx context.Context, sourceTransactionID uint) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission
	err := db.Where("source_transaction_id = ?", sourceTransactionID).Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// BySourceCampaign finds agency commissions by source campaign ID
func (r *AgencyCommissionRepositoryImpl) BySourceCampaign(ctx context.Context, sourceCampaignID uint) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission
	err := db.Where("source_campaign_id = ?", sourceCampaignID).Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// GetPendingCommissions gets pending agency commissions
func (r *AgencyCommissionRepositoryImpl) GetPendingCommissions(ctx context.Context, limit, offset int) ([]*models.AgencyCommission, error) {
	return r.ByStatus(ctx, models.CommissionStatusPending, limit, offset)
}

// GetPaidCommissions gets paid agency commissions
func (r *AgencyCommissionRepositoryImpl) GetPaidCommissions(ctx context.Context, limit, offset int) ([]*models.AgencyCommission, error) {
	return r.ByStatus(ctx, models.CommissionStatusPaid, limit, offset)
}

// GetCommissionsByDateRange gets agency commissions within a date range
func (r *AgencyCommissionRepositoryImpl) GetCommissionsByDateRange(ctx context.Context, from, to time.Time, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Where("created_at BETWEEN ? AND ?", from, to).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// ByFilter retrieves agency commissions based on filter criteria
func (r *AgencyCommissionRepositoryImpl) ByFilter(ctx context.Context, filter models.AgencyCommissionFilter, orderBy string, limit, offset int) ([]*models.AgencyCommission, error) {
	db := r.getDB(ctx)
	var commissions []*models.AgencyCommission

	query := db.Model(&models.AgencyCommission{})
	query = r.applyFilter(query, filter)

	if orderBy != "" {
		query = query.Order(orderBy)
	} else {
		query = query.Order("created_at DESC")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commissions).Error
	if err != nil {
		return nil, err
	}
	return commissions, nil
}

// Save inserts a new agency commission
func (r *AgencyCommissionRepositoryImpl) Save(ctx context.Context, commission *models.AgencyCommission) error {
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

	err = db.Create(commission).Error
	if err != nil {
		return err
	}
	return nil
}

// SaveBatch inserts multiple agency commissions in a single transaction
func (r *AgencyCommissionRepositoryImpl) SaveBatch(ctx context.Context, commissions []*models.AgencyCommission) error {
	if len(commissions) == 0 {
		return nil
	}

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

	err = db.CreateInBatches(commissions, 100).Error
	if err != nil {
		return err
	}
	return nil
}

// Count returns the number of agency commissions matching the filter
func (r *AgencyCommissionRepositoryImpl) Count(ctx context.Context, filter models.AgencyCommissionFilter) (int64, error) {
	db := r.getDB(ctx)
	var count int64

	query := db.Model(&models.AgencyCommission{})
	query = r.applyFilter(query, filter)

	err := query.Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Exists checks if any agency commission matching the filter exists
func (r *AgencyCommissionRepositoryImpl) Exists(ctx context.Context, filter models.AgencyCommissionFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// applyFilter applies the filter to the query
func (r *AgencyCommissionRepositoryImpl) applyFilter(query *gorm.DB, filter models.AgencyCommissionFilter) *gorm.DB {
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}
	if filter.UUID != nil {
		query = query.Where("uuid = ?", *filter.UUID)
	}
	if filter.CorrelationID != nil {
		query = query.Where("correlation_id = ?", *filter.CorrelationID)
	}
	if filter.AgencyID != nil {
		query = query.Where("agency_id = ?", *filter.AgencyID)
	}
	if filter.CustomerID != nil {
		query = query.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.WalletID != nil {
		query = query.Where("wallet_id = ?", *filter.WalletID)
	}
	if filter.Type != nil {
		query = query.Where("type = ?", *filter.Type)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.Amount != nil {
		query = query.Where("amount = ?", *filter.Amount)
	}
	if filter.SourceTransactionID != nil {
		query = query.Where("source_transaction_id = ?", *filter.SourceTransactionID)
	}
	if filter.SourceCampaignID != nil {
		query = query.Where("source_campaign_id = ?", *filter.SourceCampaignID)
	}
	if filter.PaymentTransactionID != nil {
		query = query.Where("payment_transaction_id = ?", *filter.PaymentTransactionID)
	}
	if filter.CreatedAfter != nil {
		query = query.Where("created_at > ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query = query.Where("created_at < ?", *filter.CreatedBefore)
	}
	if filter.PaidAfter != nil {
		query = query.Where("paid_at > ?", *filter.PaidAfter)
	}
	if filter.PaidBefore != nil {
		query = query.Where("paid_at < ?", *filter.PaidBefore)
	}
	return query
}
