// Package dto contains Data Transfer Objects for API request and response structures
package dto

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

// PhoneWithAdLink pairs a phone number with its specific ad link for short link allocation.
type PhoneWithAdLink struct {
	Phone  string  `json:"phone" validate:"required,max=20"`
	AdLink *string `json:"ad_link" validate:"omitempty,max=10000"`
}

// BotAllocateShortLinksRequest is used by scheduler/bots to allocate sequential short links
// with a distinct ad link per phone for a campaign.
type BotAllocateShortLinksRequest struct {
	CampaignID      uint              `json:"campaign_id" validate:"required"`
	Items           []PhoneWithAdLink `json:"items" validate:"required,min=1,dive"`
	ShortLinkDomain string            `json:"short_link_domain" validate:"required"`
}

// BotAllocateShortLinksResponse returns allocated codes in input order.
type BotAllocateShortLinksResponse struct {
	Message string   `json:"message"`
	Codes   []string `json:"codes"`
}

// BotAudienceUIDItem pairs an audience profile UID with its short-link code.
// Code is empty string when the campaign has no short link.
type BotAudienceUIDItem struct {
	UID  string `json:"uid"`
	Code string `json:"code"`
}

// BotPushAudienceUIDsRequest carries a batch of uid/code pairs for a campaign.
type BotPushAudienceUIDsRequest struct {
	Items []BotAudienceUIDItem `json:"items" validate:"required,min=1,dive"`
}

// BotPushAudienceUIDsResponse acknowledges a successful push.
type BotPushAudienceUIDsResponse struct {
	Message string `json:"message"`
}
