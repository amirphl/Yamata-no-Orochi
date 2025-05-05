// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// CustomerSessionRepositoryImpl implements CustomerSessionRepository interface
type CustomerSessionRepositoryImpl struct {
	*BaseRepository[models.CustomerSession, models.CustomerSessionFilter]
}

// NewCustomerSessionRepository creates a new customer session repository
func NewCustomerSessionRepository(db *gorm.DB) CustomerSessionRepository {
	return &CustomerSessionRepositoryImpl{
		BaseRepository: NewBaseRepository[models.CustomerSession, models.CustomerSessionFilter](db),
	}
}

// BySessionToken retrieves a session by session token
func (r *CustomerSessionRepositoryImpl) BySessionToken(ctx context.Context, token string) (*models.CustomerSession, error) {
	db := r.getDB(ctx)

	var session models.CustomerSession
	err := db.Where("session_token = ? AND is_active = ? AND expires_at > ?",
		token, true, time.Now()).
		Preload("Customer").
		First(&session).Error

	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find session by token: %w", err)
	}

	return &session, nil
}

// ByRefreshToken retrieves a session by refresh token
func (r *CustomerSessionRepositoryImpl) ByRefreshToken(ctx context.Context, token string) (*models.CustomerSession, error) {
	db := r.getDB(ctx)

	var session models.CustomerSession
	err := db.Where("refresh_token = ? AND is_active = ? AND expires_at > ?",
		token, true, time.Now()).
		Preload("Customer").
		First(&session).Error

	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find session by refresh token: %w", err)
	}

	return &session, nil
}

// ListActiveSessionsByCustomer retrieves all active sessions for a customer
func (r *CustomerSessionRepositoryImpl) ListActiveSessionsByCustomer(ctx context.Context, customerID uint) ([]*models.CustomerSession, error) {
	filter := models.CustomerSessionFilter{
		CustomerID: &customerID,
		IsActive:   &[]bool{true}[0],
	}

	sessions, err := r.ByFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sessions by customer: %w", err)
	}

	// Filter out expired sessions
	var activeSessions []*models.CustomerSession
	now := time.Now()
	for _, session := range sessions {
		if session.ExpiresAt.After(now) {
			activeSessions = append(activeSessions, session)
		}
	}

	return activeSessions, nil
}

// ExpireSession creates a new expired session record (insert-only approach)
func (r *CustomerSessionRepositoryImpl) ExpireSession(ctx context.Context, sessionID uint) error {
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

	// Find the session to expire
	var session models.CustomerSession
	err = db.Last(&session, sessionID).Error
	if err != nil {
		return fmt.Errorf("failed to find session to expire: %w", err)
	}

	// Create new expired session record
	expiredSession := models.CustomerSession{
		CustomerID:     session.CustomerID,
		SessionToken:   session.SessionToken + "_expired",
		RefreshToken:   nil, // Clear refresh token on expiration
		DeviceInfo:     session.DeviceInfo,
		IPAddress:      session.IPAddress,
		UserAgent:      session.UserAgent,
		IsActive:       false,
		CreatedAt:      session.CreatedAt,
		LastAccessedAt: time.Now(),
		ExpiresAt:      time.Now(), // Mark as expired now
	}

	err = db.Create(&expiredSession).Error
	if err != nil {
		return fmt.Errorf("failed to create expired session record: %w", err)
	}

	return nil
}

// ExpireAllCustomerSessions expires all sessions for a customer (insert-only approach)
func (r *CustomerSessionRepositoryImpl) ExpireAllCustomerSessions(ctx context.Context, customerID uint) error {
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

	// Find all active sessions for the customer
	var sessions []models.CustomerSession
	err = db.Where("customer_id = ? AND is_active = ?", customerID, true).
		Find(&sessions).Error

	if err != nil {
		return fmt.Errorf("failed to find customer sessions: %w", err)
	}

	// Create expired records for each session
	now := time.Now()
	for _, session := range sessions {
		expiredSession := models.CustomerSession{
			CustomerID:     session.CustomerID,
			SessionToken:   session.SessionToken + "_expired",
			RefreshToken:   nil, // Clear refresh token on expiration
			DeviceInfo:     session.DeviceInfo,
			IPAddress:      session.IPAddress,
			UserAgent:      session.UserAgent,
			IsActive:       false,
			CreatedAt:      session.CreatedAt,
			LastAccessedAt: now,
			ExpiresAt:      now, // Mark as expired now
		}

		err = db.Create(&expiredSession).Error
		if err != nil {
			return fmt.Errorf("failed to create expired session record: %w", err)
		}
	}

	return nil
}

// CleanupExpiredSessions creates cleanup records for naturally expired sessions
func (r *CustomerSessionRepositoryImpl) CleanupExpiredSessions(ctx context.Context) error {
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

	// Find all sessions that are naturally expired but still marked as active
	var expiredSessions []models.CustomerSession
	now := time.Now()
	err = db.Where("is_active = ? AND expires_at <= ?", true, now).
		Find(&expiredSessions).Error

	if err != nil {
		return fmt.Errorf("failed to find expired sessions: %w", err)
	}

	// Create cleanup records for each expired session
	for _, session := range expiredSessions {
		cleanupSession := models.CustomerSession{
			CustomerID:     session.CustomerID,
			SessionToken:   session.SessionToken + "_cleanup",
			RefreshToken:   nil, // Clear refresh token
			DeviceInfo:     session.DeviceInfo,
			IPAddress:      session.IPAddress,
			UserAgent:      session.UserAgent,
			IsActive:       false,
			CreatedAt:      session.CreatedAt,
			LastAccessedAt: session.LastAccessedAt,
			ExpiresAt:      session.ExpiresAt,
		}

		err = db.Create(&cleanupSession).Error
		if err != nil {
			return fmt.Errorf("failed to create cleanup session record: %w", err)
		}
	}

	return nil
}
