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
	UUID string `json:"uuid"`
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
}
