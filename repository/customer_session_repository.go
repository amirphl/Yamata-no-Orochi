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
		Last(&session).Error

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
		Last(&session).Error

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

	sessions, err := r.ByFilter(ctx, filter, "", 0, 0)
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
		CorrelationID:  session.CorrelationID, // Use same correlation ID
		CustomerID:     session.CustomerID,
		SessionToken:   session.SessionToken + "_expired",
		RefreshToken:   nil, // Clear refresh token on expiration
		DeviceInfo:     session.DeviceInfo,
		IPAddress:      session.IPAddress,
		UserAgent:      session.UserAgent,
		IsActive:       utils.ToPtr(false),
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
			CorrelationID:  session.CorrelationID, // Use same correlation ID
			CustomerID:     session.CustomerID,
			SessionToken:   session.SessionToken + "_expired",
			RefreshToken:   nil, // Clear refresh token on expiration
			DeviceInfo:     session.DeviceInfo,
			IPAddress:      session.IPAddress,
			UserAgent:      session.UserAgent,
			IsActive:       utils.ToPtr(false),
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
			IsActive:       utils.ToPtr(false),
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

// ByFilter retrieves customer sessions based on filter criteria
func (r *CustomerSessionRepositoryImpl) ByFilter(ctx context.Context, filter models.CustomerSessionFilter, orderBy string, limit, offset int) ([]*models.CustomerSession, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.CustomerSession{})

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

	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
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

	if filter.AccessedAfter != nil {
		query = query.Where("last_accessed_at >= ?", *filter.AccessedAfter)
	}

	if filter.AccessedBefore != nil {
		query = query.Where("last_accessed_at <= ?", *filter.AccessedBefore)
	}

	// Special handling for IsExpired - filter expired sessions
	if filter.IsExpired != nil && *filter.IsExpired {
		query = query.Where("expires_at <= ?", time.Now())
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

	var sessions []*models.CustomerSession
	err := query.Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find customer sessions by filter: %w", err)
	}

	return sessions, nil
}

// Count returns the number of customer sessions matching the filter
func (r *CustomerSessionRepositoryImpl) Count(ctx context.Context, filter models.CustomerSessionFilter) (int64, error) {
	db := r.getDB(ctx)
	query := db.Model(&models.CustomerSession{})

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

	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
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

	if filter.AccessedAfter != nil {
		query = query.Where("last_accessed_at >= ?", *filter.AccessedAfter)
	}

	if filter.AccessedBefore != nil {
		query = query.Where("last_accessed_at <= ?", *filter.AccessedBefore)
	}

	// Special handling for IsExpired - filter expired sessions
	if filter.IsExpired != nil && *filter.IsExpired {
		query = query.Where("expires_at <= ?", time.Now())
	}

	var count int64
	err := query.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count customer sessions: %w", err)
	}

	return count, nil
}

// Exists checks if any customer session matching the filter exists
func (r *CustomerSessionRepositoryImpl) Exists(ctx context.Context, filter models.CustomerSessionFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetLatestByCorrelationID retrieves the latest session record for a given correlation ID
func (r *CustomerSessionRepositoryImpl) GetLatestByCorrelationID(ctx context.Context, correlationID uuid.UUID) (*models.CustomerSession, error) {
	db := r.getDB(ctx)

	var session models.CustomerSession
	err := db.Where("correlation_id = ?", correlationID).
		Order("id DESC").
		First(&session).Error

	if err != nil {
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find latest session by correlation ID: %w", err)
	}

	return &session, nil
}

// GetHistoryByCorrelationID retrieves all session records for a given correlation ID (full history)
func (r *CustomerSessionRepositoryImpl) GetHistoryByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.CustomerSession, error) {
	db := r.getDB(ctx)

	var sessions []*models.CustomerSession
	err := db.Where("correlation_id = ?", correlationID).
		Order("id DESC").
		Find(&sessions).Error

	if err != nil {
		return nil, fmt.Errorf("failed to find session history by correlation ID: %w", err)
	}

	return sessions, nil
}
