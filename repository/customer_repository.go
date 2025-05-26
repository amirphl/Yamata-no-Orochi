// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

// CustomerRepositoryImpl implements CustomerRepository interface
type CustomerRepositoryImpl struct {
	*BaseRepository[models.Customer, models.CustomerFilter]
}

// NewCustomerRepository creates a new customer repository
func NewCustomerRepository(db *gorm.DB) CustomerRepository {
	return &CustomerRepositoryImpl{
		BaseRepository: NewBaseRepository[models.Customer, models.CustomerFilter](db),
	}
}

// ByID retrieves a customer by its ID with preloaded relationships
func (r *CustomerRepositoryImpl) ByID(ctx context.Context, id uint) (*models.Customer, error) {
	db := r.getDB(ctx)

	var customer models.Customer
	err := db.Preload("AccountType").
		Preload("ReferrerAgency").
		Last(&customer, id).Error
	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find customer by ID %d: %w", id, err)
	}

	return &customer, nil
}

// ByEmail retrieves a customer by email address
func (r *CustomerRepositoryImpl) ByEmail(ctx context.Context, email string) (*models.Customer, error) {
	filter := models.CustomerFilter{Email: &email}
	customers, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find customer by email: %w", err)
	}

	if len(customers) == 0 {
		return nil, nil
	}

	return customers[0], nil
}

// ByMobile retrieves a customer by mobile number
func (r *CustomerRepositoryImpl) ByMobile(ctx context.Context, mobile string) (*models.Customer, error) {
	filter := models.CustomerFilter{RepresentativeMobile: &mobile}
	customers, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find customer by mobile: %w", err)
	}

	if len(customers) == 0 {
		return nil, nil
	}

	return customers[0], nil
}

// ByUUID retrieves a customer by UUID
func (r *CustomerRepositoryImpl) ByUUID(ctx context.Context, uuid string) (*models.Customer, error) {
	parsedUUID, err := utils.ParseUUID(uuid)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID format: %w", err)
	}

	filter := models.CustomerFilter{UUID: &parsedUUID}
	customers, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find customer by UUID: %w", err)
	}

	if len(customers) == 0 {
		return nil, nil
	}

	return customers[0], nil
}

// ByAgencyRefererCode retrieves a customer by agency referer code
func (r *CustomerRepositoryImpl) ByAgencyRefererCode(ctx context.Context, agencyRefererCode int64) (*models.Customer, error) {
	filter := models.CustomerFilter{AgencyRefererCode: &agencyRefererCode}
	customers, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find customer by agency referer code: %w", err)
	}

	if len(customers) == 0 {
		return nil, nil
	}

	return customers[0], nil
}

// ByNationalID retrieves a customer by national ID
func (r *CustomerRepositoryImpl) ByNationalID(ctx context.Context, nationalID string) (*models.Customer, error) {
	filter := models.CustomerFilter{NationalID: &nationalID}
	customers, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find customer by national ID: %w", err)
	}

	if len(customers) == 0 {
		return nil, nil
	}

	return customers[0], nil
}

// ListByAgency retrieves all customers associated with a specific agency
func (r *CustomerRepositoryImpl) ListByAgency(ctx context.Context, agencyID uint) ([]*models.Customer, error) {
	filter := models.CustomerFilter{
		ReferrerAgencyID: &agencyID,
		IsActive:         utils.ToPtr(true), // Active customers only
	}

	customers, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list customers by agency: %w", err)
	}

	return customers, nil
}

// ListActiveCustomers retrieves active customers with pagination
func (r *CustomerRepositoryImpl) ListActiveCustomers(ctx context.Context, limit, offset int) ([]*models.Customer, error) {
	db := r.getDB(ctx)

	var customers []*models.Customer
	err := db.Where("is_active = ?", true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("AccountType").
		Find(&customers).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list active customers: %w", err)
	}

	return customers, nil
}

// ByFilter retrieves customers based on filter criteria
func (r *CustomerRepositoryImpl) ByFilter(ctx context.Context, filter models.CustomerFilter, orderBy string, limit, offset int) ([]*models.Customer, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.Customer{})

	// Apply filters based on provided values
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}

	if filter.UUID != nil {
		query = query.Where("uuid = ?", *filter.UUID)
	}

	if filter.AccountTypeID != nil {
		query = query.Where("account_type_id = ?", *filter.AccountTypeID)
	}

	if filter.Email != nil {
		query = query.Where("email = ?", *filter.Email)
	}

	if filter.RepresentativeMobile != nil {
		query = query.Where("representative_mobile = ?", *filter.RepresentativeMobile)
	}

	if filter.CompanyName != nil {
		query = query.Where("company_name = ?", *filter.CompanyName)
	}

	if filter.NationalID != nil {
		query = query.Where("national_id = ?", *filter.NationalID)
	}

	if filter.AgencyRefererCode != nil {
		query = query.Where("agency_referer_code = ?", *filter.AgencyRefererCode)
	}

	if filter.IsEmailVerified != nil {
		query = query.Where("is_email_verified = ?", *filter.IsEmailVerified)
	}

	if filter.IsMobileVerified != nil {
		query = query.Where("is_mobile_verified = ?", *filter.IsMobileVerified)
	}

	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", *filter.CreatedBefore)
	}

	if filter.LastLoginAfter != nil {
		query = query.Where("last_login_at >= ?", *filter.LastLoginAfter)
	}

	if filter.LastLoginBefore != nil {
		query = query.Where("last_login_at <= ?", *filter.LastLoginBefore)
	}

	// Special handling for AccountTypeName - join with account_types table
	if filter.AccountTypeName != nil {
		query = query.Joins("JOIN account_types ON customers.account_type_id = account_types.id").
			Where("account_types.type_name = ?", *filter.AccountTypeName)
	}

	// Apply ordering (default to id DESC)
	if orderBy == "" {
		orderBy = "id DESC"
	}
	query = query.Order(orderBy)

	// Apply pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var customers []*models.Customer
	err := query.Preload("AccountType").Find(&customers).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find customers by filter: %w", err)
	}

	return customers, nil
}

// Count returns the number of customers matching the filter
func (r *CustomerRepositoryImpl) Count(ctx context.Context, filter models.CustomerFilter) (int64, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.Customer{})

	// Apply filters based on provided values
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}

	if filter.UUID != nil {
		query = query.Where("uuid = ?", *filter.UUID)
	}

	if filter.AccountTypeID != nil {
		query = query.Where("account_type_id = ?", *filter.AccountTypeID)
	}

	if filter.Email != nil {
		query = query.Where("email = ?", *filter.Email)
	}

	if filter.RepresentativeMobile != nil {
		query = query.Where("representative_mobile = ?", *filter.RepresentativeMobile)
	}

	if filter.CompanyName != nil {
		query = query.Where("company_name = ?", *filter.CompanyName)
	}

	if filter.NationalID != nil {
		query = query.Where("national_id = ?", *filter.NationalID)
	}

	if filter.AgencyRefererCode != nil {
		query = query.Where("agency_referer_code = ?", *filter.AgencyRefererCode)
	}

	if filter.IsEmailVerified != nil {
		query = query.Where("is_email_verified = ?", *filter.IsEmailVerified)
	}

	if filter.IsMobileVerified != nil {
		query = query.Where("is_mobile_verified = ?", *filter.IsMobileVerified)
	}

	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", *filter.CreatedBefore)
	}

	if filter.LastLoginAfter != nil {
		query = query.Where("last_login_at >= ?", *filter.LastLoginAfter)
	}

	if filter.LastLoginBefore != nil {
		query = query.Where("last_login_at <= ?", *filter.LastLoginBefore)
	}

	// Special handling for AccountTypeName - join with account_types table
	if filter.AccountTypeName != nil {
		query = query.Joins("JOIN account_types ON customers.account_type_id = account_types.id").
			Where("account_types.type_name = ?", *filter.AccountTypeName)
	}

	var count int64
	err := query.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count customers: %w", err)
	}

	return count, nil
}

// Exists checks if any customer matching the filter exists
func (r *CustomerRepositoryImpl) Exists(ctx context.Context, filter models.CustomerFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// UpdatePassword updates the password hash for an existing customer
// This is a special case that allows updating the password while maintaining referential integrity
func (r *CustomerRepositoryImpl) UpdatePassword(ctx context.Context, customerID uint, passwordHash string) error {
	db := r.getDB(ctx)

	// Use direct SQL update to change only the password hash
	result := db.Model(&models.Customer{}).
		Where("id = ?", customerID).
		Update("password_hash", passwordHash)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("customer not found with ID: %d", customerID)
	}

	return nil
}

// UpdateVerificationStatus updates verification fields for an existing customer
// This is a special case that allows updating verification status while maintaining referential integrity
func (r *CustomerRepositoryImpl) UpdateVerificationStatus(ctx context.Context, customerID uint, isMobileVerified, isEmailVerified *bool, mobileVerifiedAt, emailVerifiedAt *time.Time) error {
	db := r.getDB(ctx)

	updates := make(map[string]interface{})

	if isMobileVerified != nil {
		updates["is_mobile_verified"] = *isMobileVerified
	}
	if isEmailVerified != nil {
		updates["is_email_verified"] = *isEmailVerified
	}
	if mobileVerifiedAt != nil {
		updates["mobile_verified_at"] = *mobileVerifiedAt
	}
	if emailVerifiedAt != nil {
		updates["email_verified_at"] = *emailVerifiedAt
	}

	if len(updates) == 0 {
		return nil // No updates needed
	}

	result := db.Model(&models.Customer{}).
		Where("id = ?", customerID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update verification status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("customer not found with ID: %d", customerID)
	}

	return nil
}
