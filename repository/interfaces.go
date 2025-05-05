// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

// RepositoryContext key for transaction in context
type contextKey string

const TxContextKey contextKey = "tx"

type Repository[T any, F any] interface {
	ByID(ctx context.Context, id uint) (*T, error)
	ByFilter(ctx context.Context, filter F) ([]*T, error)
	Save(ctx context.Context, entity *T) error
	SaveBatch(ctx context.Context, entities []*T) error
	Count(ctx context.Context, filter F) (int64, error)
	Exists(ctx context.Context, filter F) (bool, error)
}

// AccountTypeRepository defines operations for account types
type AccountTypeRepository interface {
	Repository[models.AccountType, models.AccountType]
	ByTypeName(ctx context.Context, typeName string) (*models.AccountType, error)
}

// CustomerRepository defines operations for customers
type CustomerRepository interface {
	Repository[models.Customer, models.CustomerFilter]
	ByEmail(ctx context.Context, email string) (*models.Customer, error)
	ByMobile(ctx context.Context, mobile string) (*models.Customer, error)
	ByNationalID(ctx context.Context, nationalID string) (*models.Customer, error)
	ListByAgency(ctx context.Context, agencyID uint) ([]*models.Customer, error)
	ListActiveCustomers(ctx context.Context, limit, offset int) ([]*models.Customer, error)
}

// OTPVerificationRepository defines operations for OTP verifications
type OTPVerificationRepository interface {
	Repository[models.OTPVerification, models.OTPVerificationFilter]
	ByCustomerAndType(ctx context.Context, customerID uint, otpType string) ([]*models.OTPVerification, error)
	ByTargetAndType(ctx context.Context, targetValue, otpType string) (*models.OTPVerification, error)
	ListActiveOTPs(ctx context.Context, customerID uint) ([]*models.OTPVerification, error)
	ExpireOldOTPs(ctx context.Context, customerID uint, otpType string) error
}

// CustomerSessionRepository defines operations for customer sessions
type CustomerSessionRepository interface {
	Repository[models.CustomerSession, models.CustomerSessionFilter]
	BySessionToken(ctx context.Context, token string) (*models.CustomerSession, error)
	ByRefreshToken(ctx context.Context, token string) (*models.CustomerSession, error)
	ListActiveSessionsByCustomer(ctx context.Context, customerID uint) ([]*models.CustomerSession, error)
	ExpireSession(ctx context.Context, sessionID uint) error
	ExpireAllCustomerSessions(ctx context.Context, customerID uint) error
	CleanupExpiredSessions(ctx context.Context) error
}

// AuditLogRepository defines operations for audit logs
type AuditLogRepository interface {
	Repository[models.AuditLog, models.AuditLogFilter]
	ListByCustomer(ctx context.Context, customerID uint, limit, offset int) ([]*models.AuditLog, error)
	ListByAction(ctx context.Context, action string, limit, offset int) ([]*models.AuditLog, error)
	ListFailedActions(ctx context.Context, limit, offset int) ([]*models.AuditLog, error)
	ListSecurityEvents(ctx context.Context, limit, offset int) ([]*models.AuditLog, error)
}
