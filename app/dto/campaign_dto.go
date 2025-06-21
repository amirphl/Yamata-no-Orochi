package dto

import (
	"time"
)

// CreateCampaignRequest represents the request to create a new campaign
type CreateCampaignRequest struct {
	CustomerID uint       `json:"-"`
	Title      *string    `json:"title,omitempty" validate:"max=255"`
	Segment    *string    `json:"segment,omitempty" validate:"max=255"`
	Subsegment []string   `json:"subsegment,omitempty" validate:"max=255,dive,max=255"`
	Sex        *string    `json:"sex,omitempty" validate:"max=255,oneof=male female all"`
	City       []string   `json:"city,omitempty" validate:"max=255,dive,max=255"`
	AdLink     *string    `json:"adlink,omitempty" validate:"max=10000"`
	Content    *string    `json:"content,omitempty" validate:"max=512,min=1"`
	ScheduleAt *time.Time `json:"schedule_at,omitempty"`
	LineNumber *string    `json:"line_number,omitempty" validate:"max=255"`
	Budget     *uint64    `json:"budget,omitempty" validate:"min=100000"`
}

// CreateCampaignResponse represents the response to create a new campaign
type CreateCampaignResponse struct {
	Message   string `json:"message"`
	UUID      string `json:"uuid"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// UpdateCampaignRequest represents the request to update an existing campaign
type UpdateCampaignRequest struct {
	UUID       string     `json:"-"`
	CustomerID uint       `json:"-"`
	Title      *string    `json:"title,omitempty" validate:"max=255"`
	Segment    *string    `json:"segment,omitempty" validate:"max=255"`
	Subsegment []string   `json:"subsegment,omitempty" validate:"max=255,dive,max=255"`
	Sex        *string    `json:"sex,omitempty" validate:"max=255,oneof=male female all"`
	City       []string   `json:"city,omitempty" validate:"max=255,dive,max=255"`
	AdLink     *string    `json:"adlink,omitempty" validate:"max=10000"`
	Content    *string    `json:"content,omitempty" validate:"max=512,min=1"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty" validate:"max=255"`
	Budget     *uint64    `json:"budget,omitempty" validate:"min=100000"`
	Finalize   *bool      `json:"finalize,omitempty"`
}

// UpdateCampaignResponse represents the response to update an existing campaign
type UpdateCampaignResponse struct {
	Message string `json:"message"`
}

// GetCampaignRequest represents the request to get an existing campaign
type GetCampaignRequest struct {
	UUID       string `json:"-"`
	CustomerID uint   `json:"-"`
}

// GetCampaignResponse represents the campaign specification in responses
type GetCampaignResponse struct {
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

// CalculateCampaignCapacityRequest represents the request to calculate the capacity of an campaign
type CalculateCampaignCapacityRequest struct {
	Title      *string    `json:"title,omitempty" validate:"max=255"`
	Segment    *string    `json:"segment,omitempty" validate:"max=255"`
	Subsegment []string   `json:"subsegment,omitempty" validate:"max=255,dive,max=255"`
	Sex        *string    `json:"sex,omitempty" validate:"max=255,oneof=male female all"`
	City       []string   `json:"city,omitempty" validate:"max=255,dive,max=255"`
	AdLink     *string    `json:"adlink,omitempty" validate:"max=10000"`
	Content    *string    `json:"content,omitempty" validate:"max=512,min=1"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty" validate:"max=255"`
	Budget     *uint64    `json:"budget,omitempty" validate:"min=100000"`
}

// CalculateCampaignCapacityResponse represents the response to calculate the capacity of an campaign
type CalculateCampaignCapacityResponse struct {
	Message  string `json:"message"`
	Capacity uint64 `json:"capacity"`
}

// CalculateCampaignCostRequest represents the request to calculate the cost of an campaign
type CalculateCampaignCostRequest struct {
	Title      *string    `json:"title,omitempty" validate:"max=255"`
	Segment    *string    `json:"segment,omitempty" validate:"max=255"`
	Subsegment []string   `json:"subsegment,omitempty" validate:"max=255,dive,max=255"`
	Sex        *string    `json:"sex,omitempty" validate:"max=255,oneof=male female all"`
	City       []string   `json:"city,omitempty" validate:"max=255,dive,max=255"`
	AdLink     *string    `json:"adlink,omitempty" validate:"max=10000"`
	Content    *string    `json:"content,omitempty" validate:"max=512,min=1"`
	ScheduleAt *time.Time `json:"scheduleat,omitempty"`
	LineNumber *string    `json:"line_number,omitempty" validate:"max=255"`
	Budget     *uint64    `json:"budget,omitempty" validate:"min=100000"`
}

// CalculateCampaignCostResponse represents the response to calculate the cost of an campaign
type CalculateCampaignCostResponse struct {
	Message      string `json:"message"`
	Total        uint64 `json:"total"`
	MsgTarget    uint64 `json:"msg_target"`
	MaxMsgTarget uint64 `json:"max_msg_target"`
}

// ListCampaignsFilter represents filter criteria for listing campaigns in request layer
type ListCampaignsFilter struct {
	Title  *string `json:"title,omitempty" validate:"max=255"`
	Status *string `json:"status,omitempty" validate:"max=255,oneof=initiated in_progress waiting_for_approval approved rejected"`
}

// ListCampaignsRequest represents a paginated list request for user's campaigns
type ListCampaignsRequest struct {
	CustomerID uint                 `json:"-"`
	Page       int                  `json:"page" validate:"min=1,max=100"`
	Limit      int                  `json:"limit" validate:"min=1,max=100"`
	OrderBy    string               `json:"orderby" validate:"oneof=newest oldest"` // newest, oldest
	Filter     *ListCampaignsFilter `json:"filter,omitempty"`
}

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"total_pages"`
}

// ListCampaignsResponse represents a paginated list of campaigns
type ListCampaignsResponse struct {
	Message    string                `json:"message"`
	Items      []GetCampaignResponse `json:"items"`
	Pagination PaginationInfo        `json:"pagination"`
}
