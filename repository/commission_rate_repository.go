package repository

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// CommissionRateRepositoryImpl implements CommissionRateRepository interface
type CommissionRateRepositoryImpl struct {
	*BaseRepository[models.CommissionRate, models.CommissionRateFilter]
}

// NewCommissionRateRepository creates a new commission rate repository
func NewCommissionRateRepository(db *gorm.DB) CommissionRateRepository {
	return &CommissionRateRepositoryImpl{
		BaseRepository: NewBaseRepository[models.CommissionRate, models.CommissionRateFilter](db),
	}
}

// ByID finds a commission rate by ID
func (r *CommissionRateRepositoryImpl) ByID(ctx context.Context, id uint) (*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rate models.CommissionRate
	err := db.Last(&rate, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rate, nil
}

// ByUUID finds a commission rate by UUID
func (r *CommissionRateRepositoryImpl) ByUUID(ctx context.Context, uuid string) (*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rate models.CommissionRate
	err := db.Where("uuid = ?", uuid).Last(&rate).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rate, nil
}

// ByAgencyID finds commission rates by agency ID
func (r *CommissionRateRepositoryImpl) ByAgencyID(ctx context.Context, agencyID uint) ([]*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rates []*models.CommissionRate
	err := db.Where("agency_id = ?", agencyID).Order("created_at DESC").Find(&rates).Error
	if err != nil {
		return nil, err
	}
	return rates, nil
}

// ByAgencyAndTransactionType finds a commission rate by agency ID and transaction type
func (r *CommissionRateRepositoryImpl) ByAgencyAndTransactionType(ctx context.Context, agencyID uint, transactionType string) (*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rate models.CommissionRate
	err := db.Where("agency_id = ? AND transaction_type = ?", agencyID, transactionType).Last(&rate).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rate, nil
}

// GetActiveRates gets active commission rates
func (r *CommissionRateRepositoryImpl) GetActiveRates(ctx context.Context, limit, offset int) ([]*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rates []*models.CommissionRate

	query := db.Where("is_active = ?", true).Order("agency_id, transaction_type")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&rates).Error
	if err != nil {
		return nil, err
	}
	return rates, nil
}

// GetRatesByTransactionType gets commission rates by transaction type
func (r *CommissionRateRepositoryImpl) GetRatesByTransactionType(ctx context.Context, transactionType string, limit, offset int) ([]*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rates []*models.CommissionRate

	query := db.Where("transaction_type = ?", transactionType).Order("agency_id")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&rates).Error
	if err != nil {
		return nil, err
	}
	return rates, nil
}

// ByFilter retrieves commission rates based on filter criteria
func (r *CommissionRateRepositoryImpl) ByFilter(ctx context.Context, filter models.CommissionRateFilter, orderBy string, limit, offset int) ([]*models.CommissionRate, error) {
	db := r.getDB(ctx)
	var rates []*models.CommissionRate

	query := db.Model(&models.CommissionRate{})
	query = r.applyFilter(query, filter)

	if orderBy != "" {
		query = query.Order(orderBy)
	} else {
		query = query.Order("agency_id, transaction_type")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&rates).Error
	if err != nil {
		return nil, err
	}
	return rates, nil
}

// SaveBatch inserts multiple commission rates in a single transaction
func (r *CommissionRateRepositoryImpl) SaveBatch(ctx context.Context, rates []*models.CommissionRate) error {
	if len(rates) == 0 {
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

	err = db.CreateInBatches(rates, 100).Error
	if err != nil {
		return err
	}
	return nil
}

// Count returns the number of commission rates matching the filter
func (r *CommissionRateRepositoryImpl) Count(ctx context.Context, filter models.CommissionRateFilter) (int64, error) {
	db := r.getDB(ctx)
	var count int64

	query := db.Model(&models.CommissionRate{})
	query = r.applyFilter(query, filter)

	err := query.Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Exists checks if any commission rate matching the filter exists
func (r *CommissionRateRepositoryImpl) Exists(ctx context.Context, filter models.CommissionRateFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// applyFilter applies the filter to the query
func (r *CommissionRateRepositoryImpl) applyFilter(query *gorm.DB, filter models.CommissionRateFilter) *gorm.DB {
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}
	if filter.UUID != nil {
		query = query.Where("uuid = ?", *filter.UUID)
	}
	if filter.AgencyID != nil {
		query = query.Where("agency_id = ?", *filter.AgencyID)
	}
	if filter.TransactionType != nil {
		query = query.Where("transaction_type = ?", *filter.TransactionType)
	}
	if filter.Rate != nil {
		query = query.Where("rate = ?", *filter.Rate)
	}
	if filter.MinAmount != nil {
		query = query.Where("min_amount = ?", *filter.MinAmount)
	}
	if filter.MaxAmount != nil {
		query = query.Where("max_amount = ?", *filter.MaxAmount)
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}
	return query
}
