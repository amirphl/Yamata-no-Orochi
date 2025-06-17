package repository

import (
	"context"
	"errors"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgencyCustomerTransactionSum is a report row for aggregated transaction amounts by customer under an agency
type AgencyCustomerTransactionSum struct {
	CustomerID              uint   `json:"customer_id"`
	FirstName               string `json:"first_name"`
	LastName                string `json:"last_name"`
	CompanyName             string `json:"company_name"`
	TotalAgencyShareWithTax uint64 `json:"total_agency_share_with_tax"`
}

// AgencyCustomerDiscountAggregate is a report row aggregating by discount for a given customer
type AgencyCustomerDiscountAggregate struct {
	AgencyDiscountID        uint64     `json:"agency_discount_id"`
	TotalAgencyShareWithTax uint64     `json:"total_agency_share_with_tax"`
	Rate                    float64    `json:"rate"`
	ExpiresAt               *time.Time `json:"expires_at"`
	CreatedAt               time.Time  `json:"created_at"`
}

// TransactionRepositoryImpl implements TransactionRepository interface
type TransactionRepositoryImpl struct {
	*BaseRepository[models.Transaction, models.TransactionFilter]
}

// NewTransactionRepository creates a new transaction repository
func NewTransactionRepository(db *gorm.DB) TransactionRepository {
	return &TransactionRepositoryImpl{
		BaseRepository: NewBaseRepository[models.Transaction, models.TransactionFilter](db),
	}
}

// ByID finds a transaction by ID
func (r *TransactionRepositoryImpl) ByID(ctx context.Context, id uint) (*models.Transaction, error) {
	db := r.getDB(ctx)
	var transaction models.Transaction
	err := db.Last(&transaction, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &transaction, nil
}

// ByUUID finds a transaction by UUID
func (r *TransactionRepositoryImpl) ByUUID(ctx context.Context, uuid string) (*models.Transaction, error) {
	db := r.getDB(ctx)
	var transaction models.Transaction
	err := db.Where("uuid = ?", uuid).Last(&transaction).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &transaction, nil
}

// ByCorrelationID finds transactions by correlation ID
func (r *TransactionRepositoryImpl) ByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.Transaction, error) {
	db := r.getDB(ctx)
	var transactions []*models.Transaction
	err := db.Where("correlation_id = ?", correlationID).Order("created_at DESC").Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// ByWalletID finds transactions by wallet ID
func (r *TransactionRepositoryImpl) ByWalletID(ctx context.Context, walletID uint, limit, offset int) ([]*models.Transaction, error) {
	db := r.getDB(ctx)
	var transactions []*models.Transaction

	query := db.Where("wallet_id = ?", walletID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// ByCustomerID finds transactions by customer ID
func (r *TransactionRepositoryImpl) ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.Transaction, error) {
	db := r.getDB(ctx)
	var transactions []*models.Transaction

	query := db.Where("customer_id = ?", customerID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// ByType finds transactions by type
func (r *TransactionRepositoryImpl) ByType(ctx context.Context, transactionType models.TransactionType, limit, offset int) ([]*models.Transaction, error) {
	db := r.getDB(ctx)
	var transactions []*models.Transaction

	query := db.Where("type = ?", transactionType).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// ByStatus finds transactions by status
func (r *TransactionRepositoryImpl) ByStatus(ctx context.Context, status models.TransactionStatus, limit, offset int) ([]*models.Transaction, error) {
	db := r.getDB(ctx)
	var transactions []*models.Transaction

	query := db.Where("status = ?", status).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// ByExternalReference finds a transaction by external reference
func (r *TransactionRepositoryImpl) ByExternalReference(ctx context.Context, externalReference string) (*models.Transaction, error) {
	db := r.getDB(ctx)
	var transaction models.Transaction
	err := db.Where("external_reference = ?", externalReference).Last(&transaction).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &transaction, nil
}

// GetPendingTransactions gets pending transactions
func (r *TransactionRepositoryImpl) GetPendingTransactions(ctx context.Context, limit, offset int) ([]*models.Transaction, error) {
	return r.ByStatus(ctx, models.TransactionStatusPending, limit, offset)
}

// GetCompletedTransactions gets completed transactions
func (r *TransactionRepositoryImpl) GetCompletedTransactions(ctx context.Context, limit, offset int) ([]*models.Transaction, error) {
	return r.ByStatus(ctx, models.TransactionStatusCompleted, limit, offset)
}

// ByFilter retrieves transactions based on filter criteria
func (r *TransactionRepositoryImpl) ByFilter(ctx context.Context, filter models.TransactionFilter, orderBy string, limit, offset int) ([]*models.Transaction, error) {
	db := r.getDB(ctx)
	var transactions []*models.Transaction

	query := db.Model(&models.Transaction{})
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

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// Save inserts a new transaction
func (r *TransactionRepositoryImpl) Save(ctx context.Context, transaction *models.Transaction) error {
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

	err = db.Create(transaction).Error
	if err != nil {
		return err
	}

	return nil
}

// SaveBatch inserts multiple transactions in a single transaction
func (r *TransactionRepositoryImpl) SaveBatch(ctx context.Context, transactions []*models.Transaction) error {
	if len(transactions) == 0 {
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

	err = db.CreateInBatches(transactions, 100).Error
	if err != nil {
		return err
	}

	return nil
}

// Count returns the number of transactions matching the filter
func (r *TransactionRepositoryImpl) Count(ctx context.Context, filter models.TransactionFilter) (int64, error) {
	db := r.getDB(ctx)
	var count int64

	query := db.Model(&models.Transaction{})
	query = r.applyFilter(query, filter)

	err := query.Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Exists checks if any transaction matching the filter exists
func (r *TransactionRepositoryImpl) Exists(ctx context.Context, filter models.TransactionFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// applyFilter applies the filter to the query
func (r *TransactionRepositoryImpl) applyFilter(query *gorm.DB, filter models.TransactionFilter) *gorm.DB {
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}
	if filter.UUID != nil {
		query = query.Where("uuid = ?", *filter.UUID)
	}
	if filter.CorrelationID != nil {
		query = query.Where("correlation_id = ?", *filter.CorrelationID)
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
	if filter.Currency != nil {
		query = query.Where("currency = ?", *filter.Currency)
	}
	if filter.WalletID != nil {
		query = query.Where("wallet_id = ?", *filter.WalletID)
	}
	if filter.CustomerID != nil {
		query = query.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.ExternalReference != nil {
		query = query.Where("external_reference = ?", *filter.ExternalReference)
	}
	if filter.CreatedAfter != nil {
		query = query.Where("created_at > ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query = query.Where("created_at < ?", *filter.CreatedBefore)
	}
	return query
}

// AggregateAgencyTransactionsByCustomers aggregates transaction amounts per customer under an agency based on metadata
func (r *TransactionRepositoryImpl) AggregateAgencyTransactionsByCustomers(ctx context.Context, agencyID uint, customerNameLike string, orderBy string) ([]*AgencyCustomerTransactionSum, error) {
	db := r.getDB(ctx)
	rows := make([]*AgencyCustomerTransactionSum, 0)

	allowed := map[string]string{
		"name_asc":   "first_name ASC, last_name ASC",
		"name_desc":  "first_name DESC, last_name DESC",
		"share_desc": "total_agency_share_with_tax DESC",
		"share_asc":  "total_agency_share_with_tax ASC",
	}

	order := allowed[orderBy]
	if order == "" {
		order = "agency_share_with_tax DESC"
	}

	query := db.
		Table("transactions t").
		Select("u.id as customer_id, u.representative_first_name as first_name, u.representative_last_name as last_name, u.company_name as company_name, COALESCE(SUM(t.amount),0) as total_agency_share_with_tax").
		Joins("JOIN customers u ON u.id = (t.metadata->>'customer_id')::bigint").
		Where("t.customer_id = ?", agencyID).
		Where("t.metadata->>'source' = ?", "payment_callback").
		Group("u.id, u.representative_first_name, u.representative_last_name, u.company_name").
		Order(order)

	if customerNameLike != "" {
		pattern := "%" + customerNameLike + "%"
		query = query.Where("u.representative_first_name like ? OR u.representative_last_name like ? OR u.company_name like ?", pattern, pattern, pattern)
	}

	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// AggregateAgencyTransactionsByDiscounts aggregates transactions by agency_discount_id for a given agency and customer
func (r *TransactionRepositoryImpl) AggregateAgencyTransactionsByDiscounts(ctx context.Context, agencyID uint, customerID uint, orderBy string) ([]*AgencyCustomerDiscountAggregate, error) {
	db := r.getDB(ctx)
	rows := make([]*AgencyCustomerDiscountAggregate, 0)

	allowed := map[string]string{
		"share_desc": "total_agency_share_with_tax DESC",
		"share_asc":  "total_agency_share_with_tax ASC",
		"id_desc":    "agency_discount_id DESC",
		"id_asc":     "agency_discount_id ASC",
	}
	order := allowed[orderBy]
	if order == "" {
		order = "total_agency_share_with_tax DESC"
	}

	query := db.
		Table("transactions t").
		Select("COALESCE((t.metadata->>'agency_discount_id')::bigint, 0) as agency_discount_id, COALESCE(SUM(t.amount),0) as total_agency_share_with_tax, r.rate as rate, r.expires_at as expires_at, r.created_at as created_at").
		Joins("JOIN agency_discounts r ON r.id = (t.metadata->>'agency_discount_id')::bigint").
		Where("t.customer_id = ?", agencyID).
		Where("(t.metadata->>'customer_id')::bigint = ?", customerID).
		Where("t.metadata->>'source' = ?", "payment_callback").
		Group("(t.metadata->>'agency_discount_id')::bigint").
		Order(order)

	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
