// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OTPVerificationRepositoryImpl implements OTPVerificationRepository interface
type OTPVerificationRepositoryImpl struct {
	*BaseRepository[models.OTPVerification, models.OTPVerificationFilter]
}

// NewOTPVerificationRepository creates a new OTP verification repository
func NewOTPVerificationRepository(db *gorm.DB) OTPVerificationRepository {
	return &OTPVerificationRepositoryImpl{
		BaseRepository: NewBaseRepository[models.OTPVerification, models.OTPVerificationFilter](db),
	}
}

// ByID retrieves an OTP verification by its ID with preloaded relationships
func (r *OTPVerificationRepositoryImpl) ByID(ctx context.Context, id uint) (*models.OTPVerification, error) {
	db := r.getDB(ctx)

	var otp models.OTPVerification
	err := db.Preload("Customer").
		Last(&otp, id).Error
	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find OTP verification by ID %d: %w", id, err)
	}

	return &otp, nil
}

// ByCustomerAndType retrieves OTP verifications for a customer and specific type
func (r *OTPVerificationRepositoryImpl) ByCustomerAndType(ctx context.Context, customerID uint, otpType string) ([]*models.OTPVerification, error) {
	filter := models.OTPVerificationFilter{
		CustomerID: &customerID,
		OTPType:    &otpType,
	}

	otps, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find OTPs by customer and type: %w", err)
	}

	return otps, nil
}

// ByTargetAndType retrieves the latest OTP verification for a target and type
func (r *OTPVerificationRepositoryImpl) ByTargetAndType(ctx context.Context, targetValue, otpType string) (*models.OTPVerification, error) {
	db := r.getDB(ctx)

	var otp models.OTPVerification
	err := db.Where("target_value = ? AND otp_type = ?", targetValue, otpType).
		Order("created_at DESC").
		Last(&otp).Error

	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find OTP by target and type: %w", err)
	}

	return &otp, nil
}

// ListActiveOTPs retrieves all active (pending and non-expired) OTPs for a customer
func (r *OTPVerificationRepositoryImpl) ListActiveOTPs(ctx context.Context, customerID uint) ([]*models.OTPVerification, error) {
	filter := models.OTPVerificationFilter{
		CustomerID: &customerID,
		Status:     utils.ToPtr(models.OTPStatusPending),
		IsActive:   utils.ToPtr(true), // This will filter non-expired pending OTPs
	}

	otps, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list active OTPs: %w", err)
	}

	return otps, nil
}

// ExpireOldOTPs marks old OTPs as expired for a customer and type (insert-only approach)
func (r *OTPVerificationRepositoryImpl) ExpireOldOTPs(ctx context.Context, customerID uint, otpType string) error {
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

	// Find all pending OTPs for this customer and type
	var oldOTPs []models.OTPVerification
	err = db.Where("customer_id = ? AND otp_type = ? AND status = ?",
		customerID, otpType, models.OTPStatusPending).
		Find(&oldOTPs).Error

	if err != nil {
		return fmt.Errorf("failed to find old OTPs: %w", err)
	}

	// Create new expired records for each old OTP (immutable approach)
	for _, oldOTP := range oldOTPs {
		expiredOTP := models.OTPVerification{
			CorrelationID: oldOTP.CorrelationID, // Use same correlation ID
			CustomerID:    oldOTP.CustomerID,
			OTPCode:       oldOTP.OTPCode,
			OTPType:       oldOTP.OTPType,
			TargetValue:   oldOTP.TargetValue,
			Status:        models.OTPStatusExpired,
			AttemptsCount: oldOTP.AttemptsCount,
			MaxAttempts:   oldOTP.MaxAttempts,
			CreatedAt:     oldOTP.CreatedAt,
			ExpiresAt:     time.Now(), // Mark as expired now
			IPAddress:     oldOTP.IPAddress,
			UserAgent:     oldOTP.UserAgent,
		}

		err = db.Create(&expiredOTP).Error
		if err != nil {
			return fmt.Errorf("failed to create expired OTP record: %w", err)
		}
	}

	return nil
}

// ByFilter retrieves OTP verifications based on filter criteria
func (r *OTPVerificationRepositoryImpl) ByFilter(ctx context.Context, filter models.OTPVerificationFilter, orderBy string, limit, offset int) ([]*models.OTPVerification, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.OTPVerification{})

	// Apply filters based on provided values
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}

	if filter.CorrelationID != nil {
		query = query.Where("correlation_id = ?", *filter.CorrelationID)
	}

	if filter.CustomerID != nil {
		query = query.Where("customer_id = ?", *filter.CustomerID)
	}

	if filter.OTPType != nil {
		query = query.Where("otp_type = ?", *filter.OTPType)
	}

	if filter.OTPCode != nil {
		query = query.Where("otp_code = ?", *filter.OTPCode)
	}

	if filter.TargetValue != nil {
		query = query.Where("target_value = ?", *filter.TargetValue)
	}

	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}

	if filter.IPAddress != nil {
		query = query.Where("ip_address = ?", *filter.IPAddress)
	}

	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", *filter.CreatedBefore)
	}

	if filter.ExpiresAfter != nil {
		query = query.Where("expires_at >= ?", *filter.ExpiresAfter)
	}

	if filter.ExpiresBefore != nil {
		query = query.Where("expires_at <= ?", *filter.ExpiresBefore)
	}

	// Special handling for IsActive - filter non-expired pending OTPs
	if filter.IsActive != nil && *filter.IsActive {
		query = query.Where("status = ? AND expires_at > ?", models.OTPStatusPending, time.Now())
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

	var otps []*models.OTPVerification
	err := query.Find(&otps).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find OTP verifications by filter: %w", err)
	}

	return otps, nil
}

// Count returns the number of OTP verifications matching the filter
func (r *OTPVerificationRepositoryImpl) Count(ctx context.Context, filter models.OTPVerificationFilter) (int64, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.OTPVerification{})

	// Apply filters based on provided values
	if filter.ID != nil {
		query = query.Where("id = ?", *filter.ID)
	}

	if filter.CorrelationID != nil {
		query = query.Where("correlation_id = ?", *filter.CorrelationID)
	}

	if filter.CustomerID != nil {
		query = query.Where("customer_id = ?", *filter.CustomerID)
	}

	if filter.OTPType != nil {
		query = query.Where("otp_type = ?", *filter.OTPType)
	}

	if filter.OTPCode != nil {
		query = query.Where("otp_code = ?", *filter.OTPCode)
	}

	if filter.TargetValue != nil {
		query = query.Where("target_value = ?", *filter.TargetValue)
	}

	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}

	if filter.IPAddress != nil {
		query = query.Where("ip_address = ?", *filter.IPAddress)
	}

	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", *filter.CreatedBefore)
	}

	if filter.ExpiresAfter != nil {
		query = query.Where("expires_at >= ?", *filter.ExpiresAfter)
	}

	if filter.ExpiresBefore != nil {
		query = query.Where("expires_at <= ?", *filter.ExpiresBefore)
	}

	// Special handling for IsActive - filter non-expired pending OTPs
	if filter.IsActive != nil && *filter.IsActive {
		query = query.Where("status = ? AND expires_at > ?", models.OTPStatusPending, time.Now())
	}

	var count int64
	err := query.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count OTP verifications: %w", err)
	}

	return count, nil
}

// Exists checks if any OTP verification matching the filter exists
func (r *OTPVerificationRepositoryImpl) Exists(ctx context.Context, filter models.OTPVerificationFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetLatestByCorrelationID retrieves the latest OTP record for a given correlation ID
func (r *OTPVerificationRepositoryImpl) GetLatestByCorrelationID(ctx context.Context, correlationID uuid.UUID) (*models.OTPVerification, error) {
	db := r.getDB(ctx)

	var otp models.OTPVerification
	err := db.Where("correlation_id = ?", correlationID).
		Order("id DESC").
		First(&otp).Error

	if err != nil {
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find latest OTP by correlation ID: %w", err)
	}

	return &otp, nil
}

// GetHistoryByCorrelationID retrieves all OTP records for a given correlation ID (full history)
func (r *OTPVerificationRepositoryImpl) GetHistoryByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.OTPVerification, error) {
	db := r.getDB(ctx)

	var otps []*models.OTPVerification
	err := db.Where("correlation_id = ?", correlationID).
		Order("id DESC").
		Find(&otps).Error

	if err != nil {
		return nil, fmt.Errorf("failed to find OTP history by correlation ID: %w", err)
	}

	return otps, nil
}
