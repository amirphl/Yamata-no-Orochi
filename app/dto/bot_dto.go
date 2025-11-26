// Package dto contains Data Transfer Objects for API request and response structures
package dto

// BotUpdateAudienceSpecRequest is used by bots to update available campaign spec
// color must be one of: black, white, pink
// tags must contain at least one non-empty element
type BotUpdateAudienceSpecRequest struct {
	Segment           string   `json:"segment" validate:"required,max=255"`
	Subsegment        string   `json:"subsegment" validate:"required,max=255"`
	Tags              []string `json:"tags" validate:"required,min=1,dive,required,max=255"`
	AvailableAudience int      `json:"available_audience" validate:"required,gte=0"`
}

// BotUpdateAudienceSpecResponse acknowledges a successful update
type BotUpdateAudienceSpecResponse struct {
	Message string `json:"message"`
}

// BotResetAudienceSpecRequest is used by bots to reset/delete audience spec
// This will completely remove the specified segment/subsegment from the audience spec
type BotResetAudienceSpecRequest struct {
	Segment    string `json:"segment" validate:"required,max=255"`
	Subsegment string `json:"subsegment" validate:"required,max=255"`
}

// BotResetAudienceSpecResponse acknowledges a successful reset/deletion
type BotResetAudienceSpecResponse struct {
	Message string `json:"message"`
}

// Bot DTOs for auth and listing (referenced by business flows)
// Minimal types used in flows; detailed types may live elsewhere

type BotDTO struct {
	ID        uint   `json:"id"`
	UUID      string `json:"uuid"`
	Username  string `json:"username"`
	IsActive  *bool  `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

type BotSessionDTO struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	CreatedAt    string `json:"created_at"`
}

type BotLoginRequest struct {
	Username string `json:"username" validate:"required,min=3,max=255"`
	Password string `json:"password" validate:"required,min=8,max=100"`
}

type BotLoginResponse struct {
	Bot     BotDTO        `json:"bot"`
	Session BotSessionDTO `json:"session"`
}

// Short Link creation DTOs for bot

type ShortLinkDTO struct {
	ID          uint    `json:"id"`
	UID         string  `json:"uid"`
	CampaignID  *uint   `json:"campaign_id,omitempty"`
	ClientID    *uint   `json:"client_id,omitempty"`
	PhoneNumber *string `json:"phone_number,omitempty"`
	LongLink    string  `json:"long_link"`
	ShortLink   string  `json:"short_link"`
}

type BotCreateShortLinkRequest struct {
	UID         string  `json:"uid" validate:"required,max=64"`
	CampaignID  *uint   `json:"campaign_id" validate:"omitempty"`
	ClientID    *uint   `json:"client_id" validate:"omitempty"`
	PhoneNumber *string `json:"phone_number" validate:"omitempty,max=20"`
	LongLink    string  `json:"long_link" validate:"required"`
	ShortLink   string  `json:"short_link" validate:"required"`
}

type BotCreateShortLinkResponse struct {
	Message string       `json:"message"`
	Item    ShortLinkDTO `json:"item"`
}

type BotCreateShortLinksRequest struct {
	Items []BotCreateShortLinkRequest `json:"items" validate:"required,min=1,dive"`
}

type BotCreateShortLinksResponse struct {
	Message string         `json:"message"`
	Items   []ShortLinkDTO `json:"items"`
}

// BotGenerateShortLinksRequest is used by scheduler/bots to allocate sequential short links centrally
// and persist them mapped to provided phones for a campaign.
type BotGenerateShortLinksRequest struct {
	CampaignID      uint     `json:"campaign_id" validate:"required"`
	AdLink          string   `json:"ad_link" validate:"required,max=10000"`
	Phones          []string `json:"phones" validate:"required,min=1,dive,required,max=20"`
	ShortLinkDomain string   `json:"short_link_domain" validate:"required"`
}

// BotGenerateShortLinksResponse returns allocated codes in order
type BotGenerateShortLinksResponse struct {
	Message string   `json:"message"`
	Codes   []string `json:"codes"`
}
