package dto

import (
	"time"
)

// CreateCampaignRequest represents the request to create a new campaign
type CreateCampaignRequest struct {
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

// CreateCampaignResponse represents the response to create a new campaign
type CreateCampaignResponse struct {
	Message   string `json:"message"`
	UUID      string `json:"uuid"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// UpdateCampaignRequest represents the request to update an existing campaign
// TODO: validate length
type UpdateCampaignRequest struct {
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

// CalculateCampaignCapacityResponse represents the response to calculate the capacity of an campaign
type CalculateCampaignCapacityResponse struct {
	Message  string `json:"message"`
	Capacity uint64 `json:"capacity"`
}

// CalculateCampaignCostRequest represents the request to calculate the cost of an campaign
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

// CalculateCampaignCostResponse represents the response to calculate the cost of an campaign
type CalculateCampaignCostResponse struct {
	Message      string `json:"message"`
	Total        uint64 `json:"total"`
	MsgTarget    uint64 `json:"msg_target"`
	MaxMsgTarget uint64 `json:"max_msg_target"`
}

// ListCampaignsFilter represents filter criteria for listing campaigns in request layer
type ListCampaignsFilter struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}

// ListCampaignsRequest represents a paginated list request for user's campaigns
type ListCampaignsRequest struct {
	CustomerID uint                 `json:"-"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	OrderBy    string               `json:"orderby"` // newest, oldest
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
