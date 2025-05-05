// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"

	"github.com/amirphl/Yamata-no-Orochi/models"
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

// ByEmail retrieves a customer by email address
func (r *CustomerRepositoryImpl) ByEmail(ctx context.Context, email string) (*models.Customer, error) {
	filter := models.CustomerFilter{Email: &email}
	customers, err := r.ByFilter(ctx, filter)
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
	customers, err := r.ByFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find customer by mobile: %w", err)
	}

	if len(customers) == 0 {
		return nil, nil
	}

	return customers[0], nil
}

// ByNationalID retrieves a customer by national ID
func (r *CustomerRepositoryImpl) ByNationalID(ctx context.Context, nationalID string) (*models.Customer, error) {
	filter := models.CustomerFilter{NationalID: &nationalID}
	customers, err := r.ByFilter(ctx, filter)
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
		IsActive:         &[]bool{true}[0], // Active customers only
	}

	customers, err := r.ByFilter(ctx, filter)
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
