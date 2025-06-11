package dto

import (
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

// GetPaymentHistoryRequest represents the request to retrieve payment history
type GetPaymentHistoryRequest struct {
	CustomerID uint       `json:"customer_id" validate:"required"`    // Customer ID (from authenticated context)
	Page       uint       `json:"page" validate:"min=1"`              // Page number (1-based)
	PageSize   uint       `json:"page_size" validate:"min=1,max=100"` // Number of items per page
	StartDate  *time.Time `json:"start_date,omitempty"`               // Optional start date filter
	EndDate    *time.Time `json:"end_date,omitempty"`                 // Optional end date filter
	Type       *string    `json:"type,omitempty"`                     // Optional transaction type filter
	Status     *string    `json:"status,omitempty"`                   // Optional transaction status filter
}

// PaymentHistoryItem represents a single payment history item
type PaymentHistoryItem struct {
	UUID          string            `json:"uuid"`                   // Transaction UUID
	Type          string            `json:"type"`                   // Transaction type (deposit, withdrawal, etc.)
	Status        string            `json:"status"`                 // Transaction status
	Amount        uint64            `json:"amount"`                 // Amount in Tomans
	Currency      string            `json:"currency"`               // Currency (usually TMN)
	Description   string            `json:"description"`            // Human-readable description
	Operation     string            `json:"operation"`              // Operation name for display
	DateTime      time.Time         `json:"datetime"`               // When the transaction occurred
	ExternalRef   *string           `json:"external_ref,omitempty"` // External reference (e.g., Atipay reference)
	BalanceBefore map[string]uint64 `json:"balance_before"`         // Balance before transaction
	BalanceAfter  map[string]uint64 `json:"balance_after"`          // Balance after transaction
	Metadata      map[string]any    `json:"metadata,omitempty"`     // Additional transaction metadata
}

// PaymentHistoryResponse represents the response for payment history
type PaymentHistoryResponse struct {
	Items      []PaymentHistoryItem         `json:"items"`      // List of payment history items
	Pagination PaymentHistoryPaginationInfo `json:"pagination"` // Pagination information
	Summary    PaymentSummary               `json:"summary"`    // Summary statistics
}

// PaymentHistoryPaginationInfo represents pagination metadata for payment history
type PaymentHistoryPaginationInfo struct {
	CurrentPage uint `json:"current_page"` // Current page number
	PageSize    uint `json:"page_size"`    // Number of items per page
	TotalItems  uint `json:"total_items"`  // Total number of items
	TotalPages  uint `json:"total_pages"`  // Total number of pages
	HasNext     bool `json:"has_next"`     // Whether there's a next page
	HasPrevious bool `json:"has_previous"` // Whether there's a previous page
}

// PaymentSummary represents summary statistics for the payment history
type PaymentSummary struct {
	TotalDeposits    uint64 `json:"total_deposits"`    // Total amount deposited
	TotalWithdrawals uint64 `json:"total_withdrawals"` // Total amount withdrawn
	TotalFees        uint64 `json:"total_fees"`        // Total fees charged
	TotalRefunds     uint64 `json:"total_refunds"`     // Total refunds received
	NetAmount        int64  `json:"net_amount"`        // Net amount (deposits - withdrawals - fees + refunds)
	TransactionCount uint   `json:"transaction_count"` // Total number of transactions
}

// TransactionTypeDisplay maps transaction types to human-readable operation names
var TransactionTypeDisplay = map[models.TransactionType]string{
	models.TransactionTypeDeposit:    "Wallet Recharge",
	models.TransactionTypeWithdrawal: "Campaign Payment",
	models.TransactionTypeFreeze:     "Fund Freeze",
	models.TransactionTypeUnfreeze:   "Fund Unfreeze",
	models.TransactionTypeLock:       "Fund Lock",
	models.TransactionTypeUnlock:     "Fund Unlock",
	models.TransactionTypeRefund:     "Refund",
	models.TransactionTypeFee:        "Service Fee",
	models.TransactionTypeAdjustment: "Balance Adjustment",
}

// TransactionStatusDisplay maps transaction statuses to human-readable status names
var TransactionStatusDisplay = map[models.TransactionStatus]string{
	models.TransactionStatusPending:   "Pending",
	models.TransactionStatusCompleted: "Completed",
	models.TransactionStatusFailed:    "Failed",
	models.TransactionStatusCancelled: "Cancelled",
	models.TransactionStatusReversed:  "Reversed",
}
