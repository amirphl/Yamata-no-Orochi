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
