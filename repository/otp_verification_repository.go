// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
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

// ByCustomerAndType retrieves OTP verifications for a customer and specific type
func (r *OTPVerificationRepositoryImpl) ByCustomerAndType(ctx context.Context, customerID uint, otpType string) ([]*models.OTPVerification, error) {
	filter := models.OTPVerificationFilter{
		CustomerID: &customerID,
		OTPType:    &otpType,
	}

	otps, err := r.ByFilter(ctx, filter)
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
		Status:     &[]string{models.OTPStatusPending}[0],
		IsActive:   &[]bool{true}[0], // This will filter non-expired pending OTPs
	}

	otps, err := r.ByFilter(ctx, filter)
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
