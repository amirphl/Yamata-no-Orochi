// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/google/uuid"
)

// RepositoryContext key for transaction in context
type contextKey string

const TxContextKey contextKey = "tx"

type Repository[T any, F any] interface {
	ByFilter(ctx context.Context, filter F, orderBy string, limit, offset int) ([]*T, error)
	Save(ctx context.Context, entity *T) error
	SaveBatch(ctx context.Context, entities []*T) error
	Count(ctx context.Context, filter F) (int64, error)
	Exists(ctx context.Context, filter F) (bool, error)
}

// AccountTypeRepository defines operations for account types
type AccountTypeRepository interface {
	Repository[models.AccountType, models.AccountTypeFilter]
	ByID(ctx context.Context, id uint) (*models.AccountType, error)
	ByTypeName(ctx context.Context, typeName string) (*models.AccountType, error)
}

// AdminRepository defines operations for platform admins
type AdminRepository interface {
	Repository[models.Admin, models.AdminFilter]
	ByID(ctx context.Context, id uint) (*models.Admin, error)
	ByUUID(ctx context.Context, uuid string) (*models.Admin, error)
	ByUsername(ctx context.Context, username string) (*models.Admin, error)
}

// BotRepository defines operations for bots
type BotRepository interface {
	Repository[models.Bot, models.BotFilter]
	ByID(ctx context.Context, id uint) (*models.Bot, error)
	ByUUID(ctx context.Context, uuid string) (*models.Bot, error)
	ByUsername(ctx context.Context, username string) (*models.Bot, error)
}

// AudienceProfileRepository defines operations for audience profiles
type AudienceProfileRepository interface {
	Repository[models.AudienceProfile, models.AudienceProfileFilter]
	ByID(ctx context.Context, id uint) (*models.AudienceProfile, error)
	ByUID(ctx context.Context, uid string) (*models.AudienceProfile, error)
}

// LineNumberRepository defines operations for line numbers
type LineNumberRepository interface {
	Repository[models.LineNumber, models.LineNumberFilter]
	ByID(ctx context.Context, id uint) (*models.LineNumber, error)
	ByUUID(ctx context.Context, uuid string) (*models.LineNumber, error)
	ByValue(ctx context.Context, value string) (*models.LineNumber, error)
	Update(ctx context.Context, line *models.LineNumber) error
	UpdateBatch(ctx context.Context, lines []*models.LineNumber) error
}

// CustomerRepository defines operations for customers
type CustomerRepository interface {
	Repository[models.Customer, models.CustomerFilter]
	ByID(ctx context.Context, id uint) (*models.Customer, error)
	ByEmail(ctx context.Context, email string) (*models.Customer, error)
	ByMobile(ctx context.Context, mobile string) (*models.Customer, error)
	ByUUID(ctx context.Context, uuid string) (*models.Customer, error)
	ByAgencyRefererCode(ctx context.Context, agencyRefererCode string) (*models.Customer, error)
	ByNationalID(ctx context.Context, nationalID string) (*models.Customer, error)
	ListByAgency(ctx context.Context, agencyID uint) ([]*models.Customer, error)
	ListActiveCustomers(ctx context.Context, limit, offset int) ([]*models.Customer, error)
	UpdatePassword(ctx context.Context, customerID uint, passwordHash string) error
	UpdateVerificationStatus(ctx context.Context, customerID uint, isMobileVerified, isEmailVerified *bool, mobileVerifiedAt, emailVerifiedAt *time.Time) error
}

// OTPVerificationRepository defines operations for OTP verifications
type OTPVerificationRepository interface {
	Repository[models.OTPVerification, models.OTPVerificationFilter]
	ByID(ctx context.Context, id uint) (*models.OTPVerification, error)
	ByCustomerAndType(ctx context.Context, customerID uint, otpType string) ([]*models.OTPVerification, error)
	ByTargetAndType(ctx context.Context, targetValue, otpType string) (*models.OTPVerification, error)
	ListActiveOTPs(ctx context.Context, customerID uint) ([]*models.OTPVerification, error)
	ExpireOldOTPs(ctx context.Context, customerID uint, otpType string) error
	GetLatestByCorrelationID(ctx context.Context, correlationID uuid.UUID) (*models.OTPVerification, error)
	GetHistoryByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.OTPVerification, error)
}

// CustomerSessionRepository defines operations for customer sessions
type CustomerSessionRepository interface {
	Repository[models.CustomerSession, models.CustomerSessionFilter]
	ByID(ctx context.Context, id uint) (*models.CustomerSession, error)
	BySessionToken(ctx context.Context, token string) (*models.CustomerSession, error)
	ByRefreshToken(ctx context.Context, token string) (*models.CustomerSession, error)
	ListActiveSessionsByCustomer(ctx context.Context, customerID uint) ([]*models.CustomerSession, error)
	GetLatestByCorrelationID(ctx context.Context, correlationID uuid.UUID) (*models.CustomerSession, error)
	GetHistoryByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.CustomerSession, error)
	Update(ctx context.Context, session *models.CustomerSession) error
}

// AuditLogRepository defines operations for audit logs
type AuditLogRepository interface {
	Repository[models.AuditLog, models.AuditLogFilter]
	ByID(ctx context.Context, id uint) (*models.AuditLog, error)
	ListByCustomer(ctx context.Context, customerID uint, limit, offset int) ([]*models.AuditLog, error)
	ListByAction(ctx context.Context, action string, limit, offset int) ([]*models.AuditLog, error)
	ListFailedActions(ctx context.Context, limit, offset int) ([]*models.AuditLog, error)
	ListSecurityEvents(ctx context.Context, limit, offset int) ([]*models.AuditLog, error)
}

// CampaignRepository defines the interface for campaign data access
type CampaignRepository interface {
	Repository[models.Campaign, models.CampaignFilter]
	ByID(ctx context.Context, id uint) (*models.Campaign, error)
	ByUUID(ctx context.Context, uuid string) (*models.Campaign, error)
	ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.Campaign, error)
	ByStatus(ctx context.Context, status models.CampaignStatus, limit, offset int) ([]*models.Campaign, error)
	Update(ctx context.Context, campaign models.Campaign) error
	UpdateStatus(ctx context.Context, id uint, status models.CampaignStatus) error
	CountByCustomerID(ctx context.Context, customerID uint) (int, error)
	CountByStatus(ctx context.Context, status models.CampaignStatus) (int, error)
	GetPendingApproval(ctx context.Context, limit, offset int) ([]*models.Campaign, error)
	GetScheduledCampaigns(ctx context.Context, from, to time.Time) ([]*models.Campaign, error)
}

// WalletRepository defines the interface for wallet data access
type WalletRepository interface {
	Repository[models.Wallet, models.WalletFilter]
	ByID(ctx context.Context, id uint) (*models.Wallet, error)
	ByUUID(ctx context.Context, uuid string) (*models.Wallet, error)
	ByCustomerID(ctx context.Context, customerID uint) (*models.Wallet, error)
	SaveWithInitialSnapshot(ctx context.Context, wallet *models.Wallet) error
	GetCurrentBalance(ctx context.Context, walletID uint) (*models.BalanceSnapshot, error)
	GetBalanceAtTime(ctx context.Context, walletID uint, timestamp time.Time) (*models.BalanceSnapshot, error)
	GetBalanceHistory(ctx context.Context, walletID uint, limit, offset int) ([]*models.BalanceSnapshot, error)
}

// TransactionRepository defines the interface for transaction data access
type TransactionRepository interface {
	Repository[models.Transaction, models.TransactionFilter]
	ByID(ctx context.Context, id uint) (*models.Transaction, error)
	ByUUID(ctx context.Context, uuid string) (*models.Transaction, error)
	ByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.Transaction, error)
	ByWalletID(ctx context.Context, walletID uint, limit, offset int) ([]*models.Transaction, error)
	ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.Transaction, error)
	ByType(ctx context.Context, transactionType models.TransactionType, limit, offset int) ([]*models.Transaction, error)
	ByStatus(ctx context.Context, status models.TransactionStatus, limit, offset int) ([]*models.Transaction, error)
	ByExternalReference(ctx context.Context, externalReference string) (*models.Transaction, error)
	GetPendingTransactions(ctx context.Context, limit, offset int) ([]*models.Transaction, error)
	GetCompletedTransactions(ctx context.Context, limit, offset int) ([]*models.Transaction, error)
	// Reports
	AggregateAgencyTransactionsByCustomers(ctx context.Context, agencyID uint, nameLike string, startDate, endDate *time.Time, orderBy string) ([]*AgencyCustomerTransactionAggregate, error)
	AggregateAgencyTransactionsByDiscounts(ctx context.Context, agencyID uint, customerID uint, orderBy string) ([]*AgencyCustomerDiscountAggregate, error)
	AggregateCustomersShares(ctx context.Context, startDate, endDate *time.Time) ([]*CustomerShareAggregate, error)
}

// BalanceSnapshotRepository defines the interface for balance snapshot data access
type BalanceSnapshotRepository interface {
	Repository[models.BalanceSnapshot, models.BalanceSnapshotFilter]
	ByID(ctx context.Context, id uint) (*models.BalanceSnapshot, error)
	ByUUID(ctx context.Context, uuid string) (*models.BalanceSnapshot, error)
	ByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.BalanceSnapshot, error)
	ByWalletID(ctx context.Context, walletID uint, limit, offset int) ([]*models.BalanceSnapshot, error)
	ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.BalanceSnapshot, error)
	GetLatestByWalletID(ctx context.Context, walletID uint) (*models.BalanceSnapshot, error)
	GetLatestByWalletIDBeforeTime(ctx context.Context, walletID uint, timestamp time.Time) (*models.BalanceSnapshot, error)
}

// PaymentRequestRepository defines the interface for payment request data access
type PaymentRequestRepository interface {
	Repository[models.PaymentRequest, models.PaymentRequestFilter]
	Update(ctx context.Context, request *models.PaymentRequest) error
	ByID(ctx context.Context, id uint) (*models.PaymentRequest, error)
	ByUUID(ctx context.Context, uuid string) (*models.PaymentRequest, error)
	ByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.PaymentRequest, error)
	ByInvoiceNumber(ctx context.Context, invoiceNumber string) (*models.PaymentRequest, error)
	ByAtipayToken(ctx context.Context, atipayToken string) (*models.PaymentRequest, error)
	ByPaymentReference(ctx context.Context, paymentReference string) (*models.PaymentRequest, error)
	ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.PaymentRequest, error)
	ByWalletID(ctx context.Context, walletID uint, limit, offset int) ([]*models.PaymentRequest, error)
	ByStatus(ctx context.Context, status models.PaymentRequestStatus, limit, offset int) ([]*models.PaymentRequest, error)
	GetPendingRequests(ctx context.Context, limit, offset int) ([]*models.PaymentRequest, error)
	GetExpiredRequests(ctx context.Context, limit, offset int) ([]*models.PaymentRequest, error)
	GetCompletedRequests(ctx context.Context, limit, offset int) ([]*models.PaymentRequest, error)
}

// CommissionRateRepository defines the interface for commission rate data access
type CommissionRateRepository interface {
	Repository[models.CommissionRate, models.CommissionRateFilter]
	ByID(ctx context.Context, id uint) (*models.CommissionRate, error)
	ByUUID(ctx context.Context, uuid string) (*models.CommissionRate, error)
	ByAgencyID(ctx context.Context, agencyID uint) ([]*models.CommissionRate, error)
	ByAgencyAndTransactionType(ctx context.Context, agencyID uint, transactionType string) (*models.CommissionRate, error)
	GetActiveRates(ctx context.Context, limit, offset int) ([]*models.CommissionRate, error)
	GetRatesByTransactionType(ctx context.Context, transactionType string, limit, offset int) ([]*models.CommissionRate, error)
}

// AgencyCommissionRepository defines the interface for agency commission data access
type AgencyCommissionRepository interface {
	Repository[models.AgencyCommission, models.AgencyCommissionFilter]
	ByID(ctx context.Context, id uint) (*models.AgencyCommission, error)
	ByUUID(ctx context.Context, uuid string) (*models.AgencyCommission, error)
	ByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]*models.AgencyCommission, error)
	ByAgencyID(ctx context.Context, agencyID uint, limit, offset int) ([]*models.AgencyCommission, error)
	ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.AgencyCommission, error)
	ByWalletID(ctx context.Context, walletID uint, limit, offset int) ([]*models.AgencyCommission, error)
	ByType(ctx context.Context, commissionType models.CommissionType, limit, offset int) ([]*models.AgencyCommission, error)
	ByStatus(ctx context.Context, status models.CommissionStatus, limit, offset int) ([]*models.AgencyCommission, error)
	BySourceTransaction(ctx context.Context, sourceTransactionID uint) ([]*models.AgencyCommission, error)
	BySourceCampaign(ctx context.Context, sourceCampaignID uint) ([]*models.AgencyCommission, error)
	GetPendingCommissions(ctx context.Context, limit, offset int) ([]*models.AgencyCommission, error)
	GetPaidCommissions(ctx context.Context, limit, offset int) ([]*models.AgencyCommission, error)
	GetCommissionsByDateRange(ctx context.Context, from, to time.Time, limit, offset int) ([]*models.AgencyCommission, error)
}

// AgencyDiscountRepository defines the interface for agency discount data access
type AgencyDiscountRepository interface {
	Repository[models.AgencyDiscount, models.AgencyDiscountFilter]
	ByID(ctx context.Context, id uint) (*models.AgencyDiscount, error)
	ByUUID(ctx context.Context, uuid string) (*models.AgencyDiscount, error)
	ByAgencyAndCustomer(ctx context.Context, agencyID, customerID uint) ([]*models.AgencyDiscount, error)
	GetActiveDiscount(ctx context.Context, agencyID, customerID uint) (*models.AgencyDiscount, error)
	ListActiveDiscountsWithCustomer(ctx context.Context, agencyID uint, nameLike, orderBy string) ([]*AgencyDiscountWithCustomer, error)
	ExpireActiveByAgencyAndCustomer(ctx context.Context, agencyID, customerID uint, expiredAt time.Time) error
}

// TagRepository defines operations for tags
type TagRepository interface {
	Repository[models.Tag, models.TagFilter]
	ByID(ctx context.Context, id uint) (*models.Tag, error)
	ByName(ctx context.Context, name string) (*models.Tag, error)
	ListByNames(ctx context.Context, names []string) ([]*models.Tag, error)
}

// ProcessedCampaignRepository defines operations for processed campaigns
type ProcessedCampaignRepository interface {
	Repository[models.ProcessedCampaign, models.ProcessedCampaignFilter]
	ByID(ctx context.Context, id uint) (*models.ProcessedCampaign, error)
	ByCampaignID(ctx context.Context, campaignID uint) (*models.ProcessedCampaign, error)
	Update(ctx context.Context, pc *models.ProcessedCampaign) error
}

// SentSMSRepository defines operations for sent SMS rows
type SentSMSRepository interface {
	Repository[models.SentSMS, models.SentSMSFilter]
	ByID(ctx context.Context, id uint) (*models.SentSMS, error)
	ListByProcessedCampaign(ctx context.Context, processedCampaignID uint, limit, offset int) ([]*models.SentSMS, error)
}
