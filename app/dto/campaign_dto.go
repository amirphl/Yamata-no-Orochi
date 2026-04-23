package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateCampaignRequest represents the request to create a new campaign
type CreateCampaignRequest struct {
	CustomerID         uint       `json:"-"`
	Title              *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Level1             *string    `json:"level1,omitempty" validate:"omitempty,max=255"`
	Level2s            []string   `json:"level2s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Level3s            []string   `json:"level3s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Tags               []string   `json:"tags,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Sex                *string    `json:"sex,omitempty" validate:"omitempty,max=255"`
	City               []string   `json:"city,omitempty" validate:"omitempty,max=255,dive,max=255"`
	AdLink             *string    `json:"adlink,omitempty" validate:"omitempty,max=10000"`
	Content            *string    `json:"content,omitempty" validate:"omitempty,max=512,min=1"`
	ShortLinkDomain    *string    `json:"short_link_domain,omitempty" validate:"omitempty,max=255"`
	Category           *string    `json:"job_category,omitempty" validate:"omitempty,max=255"`
	Job                *string    `json:"job,omitempty" validate:"omitempty,max=255"`
	ScheduleAt         *time.Time `json:"scheduleat,omitempty"`
	LineNumber         *string    `json:"line_number,omitempty" validate:"omitempty,max=255"`
	MediaUUID          *uuid.UUID `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint      `json:"platform_settings_id,omitempty" validate:"omitempty,min=1"`
	Platform           *string    `json:"platform,omitempty" validate:"omitempty,oneof=sms rubika bale splus"`
	Budget             *uint64    `json:"budget,omitempty" validate:"omitempty"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty" validate:"omitempty,uuid4"`
}

// CreateCampaignResponse represents the response to create a new campaign
type CreateCampaignResponse struct {
	Message   string `json:"message"`
	ID        uint   `json:"id"`
	UUID      string `json:"uuid"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// UpdateCampaignRequest represents the request to update an existing campaign
type UpdateCampaignRequest struct {
	UUID               string     `json:"-"`
	CustomerID         uint       `json:"-"`
	Title              *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Level1             *string    `json:"level1,omitempty" validate:"omitempty,max=255"`
	Level2s            []string   `json:"level2s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Level3s            []string   `json:"level3s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Tags               []string   `json:"tags,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Sex                *string    `json:"sex,omitempty" validate:"omitempty,max=255"`
	City               []string   `json:"city,omitempty" validate:"omitempty,max=255,dive,max=255"`
	AdLink             *string    `json:"adlink,omitempty" validate:"omitempty,max=10000"`
	Content            *string    `json:"content,omitempty" validate:"omitempty,max=512,min=1"`
	ShortLinkDomain    *string    `json:"short_link_domain,omitempty" validate:"omitempty,max=255"`
	Category           *string    `json:"job_category,omitempty" validate:"omitempty,max=255"`
	Job                *string    `json:"job,omitempty" validate:"omitempty,max=255"`
	ScheduleAt         *time.Time `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber         *string    `json:"line_number,omitempty" validate:"omitempty,max=255"`
	MediaUUID          *uuid.UUID `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint      `json:"platform_settings_id,omitempty" validate:"omitempty,min=1"`
	Platform           *string    `json:"platform,omitempty" validate:"omitempty,oneof=sms rubika bale splus"`
	Budget             *uint64    `json:"budget,omitempty" validate:"omitempty"`
	Finalize           *bool      `json:"finalize,omitempty" validate:"omitempty"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty" validate:"omitempty,uuid4"`
}

// UpdateCampaignResponse represents the response to update an existing campaign
type UpdateCampaignResponse struct {
	Message string `json:"message"`
}

// CloneCampaignRequest represents the request to clone an existing campaign
type CloneCampaignRequest struct {
	UUID       string `json:"-" validate:"required,uuid4"`
	CustomerID uint   `json:"-"`
}

// CloneCampaignResponse represents the response after cloning a campaign
type CloneCampaignResponse struct {
	Message   string `json:"message"`
	ID        uint   `json:"id"`
	UUID      string `json:"uuid"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type CancelCampaignRequest struct {
	CampaignID uint    `json:"campaign_id" validate:"required"`
	CustomerID uint    `json:"-"`
	Comment    *string `json:"comment,omitempty" validate:"omitempty,max=500"`
}

type CancelCampaignResponse struct {
	Message string `json:"message"`
}

// GetCampaignRequest represents the request to get an existing campaign
type GetCampaignRequest struct {
	UUID       string `json:"-"`
	CustomerID uint   `json:"-"`
}

// GetCampaignResponse represents the campaign specification in responses
type GetCampaignResponse struct {
	ID                 uint           `json:"id"`
	UUID               string         `json:"uuid"`
	Status             string         `json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          *time.Time     `json:"updated_at,omitempty"`
	Title              *string        `json:"title,omitempty" validate:"omitempty"`
	Level1             *string        `json:"level1,omitempty" validate:"omitempty"`
	Level2s            []string       `json:"level2s,omitempty" validate:"omitempty"`
	Level3s            []string       `json:"level3s,omitempty" validate:"omitempty"`
	Tags               []string       `json:"tags,omitempty" validate:"omitempty"`
	Sex                *string        `json:"sex,omitempty" validate:"omitempty"`
	City               []string       `json:"city,omitempty" validate:"omitempty"`
	AdLink             *string        `json:"adlink,omitempty" validate:"omitempty"`
	Content            *string        `json:"content,omitempty" validate:"omitempty"`
	ShortLinkDomain    *string        `json:"short_link_domain,omitempty" validate:"omitempty"`
	Category           *string        `json:"job_category,omitempty" validate:"omitempty"`
	Job                *string        `json:"job,omitempty" validate:"omitempty"`
	ScheduleAt         *time.Time     `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber         *string        `json:"line_number,omitempty" validate:"omitempty"`
	MediaUUID          *uuid.UUID     `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint          `json:"platform_settings_id,omitempty"`
	Platform           string         `json:"platform"`
	LinePriceFactor    *float64       `json:"line_price_factor,omitempty"`
	Budget             *uint64        `json:"budget,omitempty" validate:"omitempty"`
	NumAudience        *uint64        `json:"num_audience,omitempty"`
	Comment            *string        `json:"comment,omitempty" validate:"omitempty"`
	Statistics         map[string]any `json:"statistics,omitempty"`
	ClickRate          *float64       `json:"click_rate,omitempty"`
	TotalClicks        *int64         `json:"total_clicks,omitempty"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty"`
}

// CalculateCampaignCapacityRequest represents the request to calculate the capacity of an campaign
type CalculateCampaignCapacityRequest struct {
	Title              *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Level1             *string    `json:"level1,omitempty" validate:"omitempty,max=255"`
	Level2s            []string   `json:"level2s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Level3s            []string   `json:"level3s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Tags               []string   `json:"tags,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Sex                *string    `json:"sex,omitempty" validate:"omitempty,max=255"`
	City               []string   `json:"city,omitempty" validate:"omitempty,max=255,dive,max=255"`
	AdLink             *string    `json:"adlink,omitempty" validate:"omitempty,max=10000"`
	Content            *string    `json:"content,omitempty" validate:"omitempty,max=512,min=1"`
	ScheduleAt         *time.Time `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber         *string    `json:"line_number,omitempty" validate:"omitempty,max=255"`
	MediaUUID          *uuid.UUID `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint      `json:"platform_settings_id,omitempty" validate:"omitempty,min=1"`
	Platform           *string    `json:"platform,omitempty" validate:"omitempty,oneof=sms rubika bale splus"`
	Budget             *uint64    `json:"budget,omitempty" validate:"omitempty"`
	ShortLinkDomain    *string    `json:"short_link_domain,omitempty" validate:"omitempty,max=255"`
	Category           *string    `json:"job_category,omitempty" validate:"omitempty,max=255"`
	Job                *string    `json:"job,omitempty" validate:"omitempty,max=255"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty" validate:"omitempty,uuid4"`
}

// CalculateCampaignCapacityResponse represents the response to calculate the capacity of an campaign
type CalculateCampaignCapacityResponse struct {
	Message  string `json:"message"`
	Capacity uint64 `json:"capacity"`
}

// CalculateCampaignCostRequest represents the request to calculate the cost of an campaign
type CalculateCampaignCostRequest struct {
	Title              *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Level1             *string    `json:"level1,omitempty" validate:"omitempty,max=255"`
	Level2s            []string   `json:"level2s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Level3s            []string   `json:"level3s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Tags               []string   `json:"tags,omitempty" validate:"omitempty"`
	Sex                *string    `json:"sex,omitempty" validate:"omitempty,max=255"`
	City               []string   `json:"city,omitempty" validate:"omitempty,max=255,dive,max=255"`
	AdLink             *string    `json:"adlink,omitempty" validate:"omitempty,max=10000"`
	Content            *string    `json:"content,omitempty" validate:"omitempty,max=512,min=1"`
	ScheduleAt         *time.Time `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber         *string    `json:"line_number,omitempty" validate:"omitempty,max=255"`
	MediaUUID          *uuid.UUID `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint      `json:"platform_settings_id,omitempty" validate:"omitempty,min=1"`
	Platform           *string    `json:"platform,omitempty" validate:"omitempty,oneof=sms rubika bale splus"`
	Budget             *uint64    `json:"budget,omitempty" validate:"omitempty"`
	ShortLinkDomain    *string    `json:"short_link_domain,omitempty" validate:"omitempty,max=255"`
	Category           *string    `json:"job_category,omitempty" validate:"omitempty,max=255"`
	Job                *string    `json:"job,omitempty" validate:"omitempty,max=255"`
	CustomerID         uint       `json:"-"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty" validate:"omitempty,uuid4"`
}

// CalculateCampaignCostV2Request represents the request to calculate required cost
// for a desired number of messages (capped by available audience capacity).
type CalculateCampaignCostV2Request struct {
	Title              *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Level1             *string    `json:"level1,omitempty" validate:"omitempty,max=255"`
	Level2s            []string   `json:"level2s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Level3s            []string   `json:"level3s,omitempty" validate:"omitempty,max=255,dive,max=255"`
	Tags               []string   `json:"tags,omitempty" validate:"omitempty"`
	Sex                *string    `json:"sex,omitempty" validate:"omitempty,max=255"`
	City               []string   `json:"city,omitempty" validate:"omitempty,max=255,dive,max=255"`
	AdLink             *string    `json:"adlink,omitempty" validate:"omitempty,max=10000"`
	Content            *string    `json:"content,omitempty" validate:"omitempty,max=512,min=1"`
	ScheduleAt         *time.Time `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber         *string    `json:"line_number,omitempty" validate:"omitempty,max=255"`
	MediaUUID          *uuid.UUID `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint      `json:"platform_settings_id,omitempty" validate:"omitempty,min=1"`
	Platform           *string    `json:"platform,omitempty" validate:"omitempty,oneof=sms rubika bale splus"`
	NumMessages        uint64     `json:"num_messages" validate:"required,gt=0"`
	ShortLinkDomain    *string    `json:"short_link_domain,omitempty" validate:"omitempty,max=255"`
	Category           *string    `json:"job_category,omitempty" validate:"omitempty,max=255"`
	Job                *string    `json:"job,omitempty" validate:"omitempty,max=255"`
	CustomerID         uint       `json:"-"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty" validate:"omitempty,uuid4"`
}

// CalculateCampaignCostResponse represents the response to calculate the cost of an campaign
type CalculateCampaignCostResponse struct {
	Message           string `json:"message"`
	TotalCost         uint64 `json:"total_cost"`
	NumTargetAudience uint64 `json:"msg_target"`
	MaxTargetAudience uint64 `json:"max_msg_target"`
}

// ListCampaignsFilter represents filter criteria for listing campaigns in request layer
type ListCampaignsFilter struct {
	Title  *string `json:"title,omitempty" validate:"omitempty,max=255"`
	Status *string `json:"status,omitempty" validate:"omitempty,max=255,oneof=initiated in_progress waiting_for_approval approved rejected cancelled running executed expired"`
}

// ListCampaignsRequest represents a paginated list request for user's campaigns
type ListCampaignsRequest struct {
	CustomerID uint                 `json:"-"`
	Page       int                  `json:"page" validate:"omitempty,min=1,max=100"`
	Limit      int                  `json:"limit" validate:"omitempty,min=1,max=100"`
	OrderBy    string               `json:"orderby" validate:"oneof=newest oldest"` // newest, oldest
	Filter     *ListCampaignsFilter `json:"filter,omitempty" validate:"omitempty"`
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

type GetLastInitiatedCampaignResponse struct {
	Message string               `json:"message"`
	Item    *GetCampaignResponse `json:"item,omitempty"`
}

// AdminListCampaignsFilter holds filters for admin campaign listing
type AdminListCampaignsFilter struct {
	Title     *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Status    *string    `json:"status,omitempty" validate:"omitempty,oneof=initiated in_progress waiting_for_approval approved rejected expired"`
	StartDate *time.Time `json:"start_date,omitempty" validate:"omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty" validate:"omitempty"`
}

// AdminGetCampaignResponse represents the campaign specification in responses
type AdminGetCampaignResponse struct {
	ID                    uint           `json:"id"`
	UUID                  string         `json:"uuid"`
	Status                string         `json:"status"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             *time.Time     `json:"updated_at,omitempty"`
	Title                 *string        `json:"title,omitempty" validate:"omitempty"`
	Level1                *string        `json:"level1,omitempty" validate:"omitempty"`
	Level2s               []string       `json:"level2s,omitempty" validate:"omitempty"`
	Level3s               []string       `json:"level3s,omitempty" validate:"omitempty"`
	Tags                  []string       `json:"tags,omitempty" validate:"omitempty"`
	Sex                   *string        `json:"sex,omitempty" validate:"omitempty"`
	City                  []string       `json:"city,omitempty" validate:"omitempty"`
	AdLink                *string        `json:"adlink,omitempty" validate:"omitempty"`
	Content               *string        `json:"content,omitempty" validate:"omitempty"`
	ShortLinkDomain       *string        `json:"short_link_domain,omitempty" validate:"omitempty"`
	Category              *string        `json:"job_category,omitempty" validate:"omitempty"`
	Job                   *string        `json:"job,omitempty" validate:"omitempty"`
	ScheduleAt            *time.Time     `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber            *string        `json:"line_number,omitempty" validate:"omitempty"`
	MediaUUID             *uuid.UUID     `json:"media_uuid,omitempty"`
	PlatformSettingsID    *uint          `json:"platform_settings_id,omitempty"`
	Platform              string         `json:"platform"`
	Budget                *uint64        `json:"budget,omitempty" validate:"omitempty"`
	Comment               *string        `json:"comment,omitempty" validate:"omitempty"`
	SegmentPriceFactor    float64        `json:"segment_price_factor,omitempty"`
	LineNumberPriceFactor float64        `json:"line_number_price_factor,omitempty"`
	Statistics            map[string]any `json:"statistics,omitempty"`
	TotalClicks           *int64         `json:"total_clicks,omitempty"`
	ClickRate             *float64       `json:"click_rate,omitempty"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty"`
}

// AdminListCampaignsResponse represents a paginated list of campaigns
type AdminListCampaignsResponse struct {
	Message string                     `json:"message"`
	Items   []AdminGetCampaignResponse `json:"items"`
}

// AdminApproveCampaignRequest represents admin approval input
type AdminApproveCampaignRequest struct {
	CampaignID uint    `json:"campaign_id" validate:"required"`
	Comment    *string `json:"comment,omitempty" validate:"omitempty,max=1000"`
}

// AdminApproveCampaignResponse represents admin approval result
type AdminApproveCampaignResponse struct {
	Message string `json:"message"`
}

// AdminRejectCampaignRequest represents admin rejection input
type AdminRejectCampaignRequest struct {
	CampaignID uint   `json:"campaign_id" validate:"required"`
	Comment    string `json:"comment" validate:"required,max=1000"`
}

// AdminRejectCampaignResponse represents admin rejection result
type AdminRejectCampaignResponse struct {
	Message string `json:"message"`
}

// AdminCancelCampaignRequest represents admin cancellation input
type AdminCancelCampaignRequest struct {
	CampaignID uint   `json:"campaign_id" validate:"required"`
	Comment    string `json:"comment" validate:"required,max=1000"`
}

// AdminCancelCampaignResponse represents admin cancellation result
type AdminCancelCampaignResponse struct {
	Message string `json:"message"`
}

// AdminRescheduleCampaignRequest represents admin reschedule input (schedule provided in Tehran time)
type AdminRescheduleCampaignRequest struct {
	CampaignID uint      `json:"campaign_id" validate:"required"`
	ScheduleAt time.Time `json:"schedule_at" validate:"required"`
}

// AdminRescheduleCampaignResponse represents admin reschedule result
type AdminRescheduleCampaignResponse struct {
	Message string `json:"message"`
}

type AdminRemoveAudienceSpecResponse struct {
	Message  string `json:"message"`
	Platform string `json:"platform"`
}

// BotGetCampaignResponse represents the campaign specification in responses
type BotGetCampaignResponse struct {
	ID                 uint                             `json:"id"`
	CustomerID         uint                             `json:"customer_id"`
	Status             string                           `json:"status"`
	CreatedAt          time.Time                        `json:"created_at"`
	UpdatedAt          *time.Time                       `json:"updated_at,omitempty"`
	Title              *string                          `json:"title,omitempty" validate:"omitempty"`
	Level1             *string                          `json:"level1,omitempty" validate:"omitempty"`
	Level2s            []string                         `json:"level2s,omitempty" validate:"omitempty"`
	Level3s            []string                         `json:"level3s,omitempty" validate:"omitempty"`
	Tags               []string                         `json:"tags,omitempty" validate:"omitempty"`
	Sex                *string                          `json:"sex,omitempty" validate:"omitempty"`
	City               []string                         `json:"city,omitempty" validate:"omitempty"`
	AdLink             *string                          `json:"adlink,omitempty" validate:"omitempty"`
	Content            *string                          `json:"content,omitempty" validate:"omitempty"`
	ShortLinkDomain    *string                          `json:"short_link_domain,omitempty" validate:"omitempty"`
	Category           *string                          `json:"job_category,omitempty" validate:"omitempty"`
	Job                *string                          `json:"job,omitempty" validate:"omitempty"`
	ScheduleAt         *time.Time                       `json:"scheduleat,omitempty" validate:"omitempty"`
	LineNumber         *string                          `json:"line_number,omitempty" validate:"omitempty"`
	MediaUUID          *uuid.UUID                       `json:"media_uuid,omitempty"`
	PlatformSettingsID *uint                            `json:"platform_settings_id,omitempty"`
	PlatformSettings   *BotCampaignPlatformSettingsSpec `json:"platform_settings,omitempty"`
	Platform           string                           `json:"platform"`
	Budget             *uint64                          `json:"budget,omitempty" validate:"omitempty"`
	Comment            *string                          `json:"comment,omitempty" validate:"omitempty"`
	NumAudiences       *uint64                          `json:"num_audiences"`

	TargetAudienceExcelFileUUID *string `json:"target_audience_excel_file_uuid,omitempty"`
}

type BotCampaignPlatformSettingsSpec struct {
	ID           uint           `json:"id"`
	Platform     string         `json:"platform"`
	Name         *string        `json:"name,omitempty"`
	Description  *string        `json:"description,omitempty"`
	MultimediaID *uint          `json:"multimedia_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Status       string         `json:"status"`
}

// BotListCampaignsResponse represents list of campaigns for bot
type BotListCampaignsResponse struct {
	Message string                   `json:"message"`
	Items   []BotGetCampaignResponse `json:"items"`
}

type AudienceSpecItem struct {
	Tags              []string `json:"tags"`
	AvailableAudience int      `json:"available_audience"`
}

type AudienceSpecLevel2 struct {
	Metadata map[string]any              `json:"metadata,omitempty"`
	Items    map[string]AudienceSpecItem `json:"items,omitempty"`
}

type AudienceSpec map[string]map[string]AudienceSpecLevel2

type ListAudienceSpecResponse struct {
	Message string       `json:"message"`
	Spec    AudienceSpec `json:"spec"`
}

type CampaignsSummaryResponse struct {
	Message       string `json:"message"`
	ApprovedCount int    `json:"approved_count"`
	RunningCount  int    `json:"running_count"`
	Total         int    `json:"total"`
}

// BotUpdateCampaignStatisticsRequest carries aggregated stats for a campaign
type BotUpdateCampaignStatisticsRequest struct {
	Statistics map[string]any `json:"statistics" validate:"required"`
}

type BotUpdateCampaignStatisticsResponse struct {
	Message string `json:"message"`
}
