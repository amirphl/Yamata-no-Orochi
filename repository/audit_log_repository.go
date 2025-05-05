// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// AuditLogRepositoryImpl implements AuditLogRepository interface
type AuditLogRepositoryImpl struct {
	*BaseRepository[models.AuditLog, models.AuditLogFilter]
}

// NewAuditLogRepository creates a new audit log repository
func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &AuditLogRepositoryImpl{
		BaseRepository: NewBaseRepository[models.AuditLog, models.AuditLogFilter](db),
	}
}

// ListByCustomer retrieves audit logs for a specific customer with pagination
func (r *AuditLogRepositoryImpl) ListByCustomer(ctx context.Context, customerID uint, limit, offset int) ([]*models.AuditLog, error) {
	db := r.getDB(ctx)

	var logs []*models.AuditLog
	err := db.Where("customer_id = ?", customerID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Customer").
		Find(&logs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs by customer: %w", err)
	}

	return logs, nil
}

// ListByAction retrieves audit logs for a specific action with pagination
func (r *AuditLogRepositoryImpl) ListByAction(ctx context.Context, action string, limit, offset int) ([]*models.AuditLog, error) {
	db := r.getDB(ctx)

	var logs []*models.AuditLog
	err := db.Where("action = ?", action).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Customer").
		Find(&logs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs by action: %w", err)
	}

	return logs, nil
}

// ListFailedActions retrieves all failed audit log entries with pagination
func (r *AuditLogRepositoryImpl) ListFailedActions(ctx context.Context, limit, offset int) ([]*models.AuditLog, error) {
	db := r.getDB(ctx)

	var logs []*models.AuditLog
	err := db.Where("success = ?", false).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Customer").
		Find(&logs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list failed audit logs: %w", err)
	}

	return logs, nil
}

// ListSecurityEvents retrieves security-related audit log entries with pagination
func (r *AuditLogRepositoryImpl) ListSecurityEvents(ctx context.Context, limit, offset int) ([]*models.AuditLog, error) {
	db := r.getDB(ctx)

	securityActions := []string{
		models.AuditActionLoginSuccess,
		models.AuditActionLoginFailed,
		models.AuditActionPasswordChanged,
		models.AuditActionAccountActivated,
		models.AuditActionAccountDeactivated,
		models.AuditActionOTPFailed,
	}

	var logs []*models.AuditLog
	err := db.Where("action IN ?", securityActions).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Customer").
		Find(&logs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list security audit logs: %w", err)
	}

	return logs, nil
}
