package dto

import (
	"time"
)

// CreateSMSCampaignRequest represents the request to create a new SMS campaign
type CreateSMSCampaignRequest struct {
	CustomerID uint       `json:"-"`
	Title      *string    `json:"title,omitempty"`
	Segment    *string    `json:"segment,omitempty"`
	Subsegment []string   `json:"subsegment,omitempty"`
	Sex        *string    `json:"sex,omitempty"`
	City       []string   `json:"city,omitempty"`
	AdLink     *string    `json:"adlink,omitempty"`
	Content    *string    `json:"content,omitempty"`
	ScheduleAt *time.Time `json:"schedule_at,omitempty"`
	LineNumber *string    `json:"line_number,omitempty"`
	Budget     *uint64    `json:"budget,omitempty"`
}

// CreateSMSCampaignResponse represents the response to create a new SMS campaign
type CreateSMSCampaignResponse struct {
	Message   string `json:"message"`
	UUID      string `json:"uuid"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// UpdateSMSCampaignRequest represents the request to update an existing SMS campaign
type UpdateSMSCampaignRequest struct {
	UUID       string     `json:"-"`
	CustomerID uint       `json:"-"`
	Title      *string    `json:"title,omitempty"`
	Segment    *string    `json:"segment,omitempty"`
	Subsegment []string   `json:"subsegment,omitempty"`
	Sex        *string    `json:"sex,omitempty"`
	City       []string   `json:"city,omitempty"`
	AdLink     *string    `json:"adlink,omitempty"`
	Content    *string    `json:"content,omitempty"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty"`
	Budget     *uint64    `json:"budget,omitempty"`
}

// UpdateSMSCampaignResponse represents the response to update an existing SMS campaign
type UpdateSMSCampaignResponse struct {
	Message string `json:"message"`
}

// GetSMSCampaignRequest represents the request to get an existing SMS campaign
type GetSMSCampaignRequest struct {
	UUID       string `json:"-"`
	CustomerID uint   `json:"-"`
}

// GetSMSCampaignResponse represents the SMS campaign specification in responses
type GetSMSCampaignResponse struct {
	UUID       string     `json:"uuid"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	Title      *string    `json:"title,omitempty"`
	Segment    *string    `json:"segment,omitempty"`
	Subsegment []string   `json:"subsegment,omitempty"`
	Sex        *string    `json:"sex,omitempty"`
	City       []string   `json:"city,omitempty"`
	AdLink     *string    `json:"adlink,omitempty"`
	Content    *string    `json:"content,omitempty"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty"`
	Budget     *uint64    `json:"budget,omitempty"`
	Comment    *string    `json:"comment,omitempty"`
}

// CalculateCampaignCapacityRequest represents the request to calculate the capacity of an SMS campaign
type CalculateCampaignCapacityRequest struct {
	Title      *string    `json:"title,omitempty"`
	Segment    *string    `json:"segment,omitempty"`
	Subsegment []string   `json:"subsegment,omitempty"`
	Sex        *string    `json:"sex,omitempty"`
	City       []string   `json:"city,omitempty"`
	AdLink     *string    `json:"adlink,omitempty"`
	Content    *string    `json:"content,omitempty"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty"`
	Budget     *uint64    `json:"budget,omitempty"`
}

// CalculateCampaignCapacityResponse represents the response to calculate the capacity of an SMS campaign
type CalculateCampaignCapacityResponse struct {
	Message  string `json:"message"`
	Capacity uint64 `json:"capacity"`
}

// CalculateCampaignCostRequest represents the request to calculate the cost of an SMS campaign
type CalculateCampaignCostRequest struct {
	Title      *string    `json:"title,omitempty"`
	Segment    *string    `json:"segment,omitempty"`
	Subsegment []string   `json:"subsegment,omitempty"`
	Sex        *string    `json:"sex,omitempty"`
	City       []string   `json:"city,omitempty"`
	AdLink     *string    `json:"adlink,omitempty"`
	Content    *string    `json:"content,omitempty"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty"`
	Budget     *uint64    `json:"budget,omitempty"`
}

// CalculateCampaignCostResponse represents the response to calculate the cost of an SMS campaign
type CalculateCampaignCostResponse struct {
	Message      string `json:"message"`
	SubTotal     uint64 `json:"sub_total"`
	Tax          uint64 `json:"tax"`
	Total        uint64 `json:"total"`
	MsgTarget    uint64 `json:"msg_target"`
	MaxMsgTarget uint64 `json:"max_msg_target"`
}

// GetWalletBalanceRequest represents the request to get user wallet balance
type GetWalletBalanceRequest struct {
	CustomerID uint `json:"-"`
}

// GetWalletBalanceResponse represents the response with user wallet balance information
type GetWalletBalanceResponse struct {
	Message             string `json:"message"`
	Free                uint64 `json:"free"`
	Locked              uint64 `json:"locked"`
	Frozen              uint64 `json:"frozen"`
	Total               uint64 `json:"total"`
	Currency            string `json:"currency"`
	LastUpdated         string `json:"last_updated"`
	PendingTransactions uint64 `json:"pending_transactions"`
	MinimumBalance      uint64 `json:"minimum_balance"`
	CreditLimit         uint64 `json:"credit_limit"`
	BalanceStatus       string `json:"balance_status"`
}

// ListSMSCampaignsFilter represents filter criteria for listing campaigns in request layer
type ListSMSCampaignsFilter struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}

// ListSMSCampaignsRequest represents a paginated list request for user's campaigns
type ListSMSCampaignsRequest struct {
	CustomerID uint                    `json:"-"`
	Page       int                     `json:"page"`
	Limit      int                     `json:"limit"`
	OrderBy    string                  `json:"orderby"` // newest, oldest
	Filter     *ListSMSCampaignsFilter `json:"filter,omitempty"`
}

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"total_pages"`
}

// ListSMSCampaignsResponse represents a paginated list of campaigns
type ListSMSCampaignsResponse struct {
	Message    string                   `json:"message"`
	Items      []GetSMSCampaignResponse `json:"items"`
	Pagination PaginationInfo           `json:"pagination"`
}
