// Package dto
package dto

type AdminDTO struct {
	ID        uint   `json:"id" example:"1"`
	UUID      string `json:"uuid" example:"f47ac10b-58cc-4372-a567-0e02b2c3d479"`
	Username  string `json:"username" example:"admin"`
	IsActive  *bool  `json:"is_active" example:"true"`
	CreatedAt string `json:"created_at" example:"2024-01-15T10:30:00Z"`
}

type AdminSessionDTO struct {
	AccessToken  string `json:"access_token" example:"jwt"`
	RefreshToken string `json:"refresh_token" example:"jwt"`
	ExpiresIn    int    `json:"expires_in" example:"3600"`
	TokenType    string `json:"token_type" example:"Bearer"`
	CreatedAt    string `json:"created_at" example:"2024-01-15T10:30:00Z"`
}

type AdminCaptchaInitResponse struct {
	ChallengeID       string `json:"challenge_id"`
	MasterImageBase64 string `json:"master_image_base64"`
	ThumbImageBase64  string `json:"thumb_image_base64"`
}

type AdminCaptchaVerifyRequest struct {
	ChallengeID string  `json:"challenge_id" validate:"required"`
	Username    string  `json:"username" validate:"required,min=3,max=255"`
	Password    string  `json:"password" validate:"required,min=8,max=100"`
	UserAngle   float64 `json:"user_angle" validate:"required"`
}

type AdminLoginResponse struct {
	Admin   AdminDTO        `json:"admin"`
	Session AdminSessionDTO `json:"session"`
}

// Admin short link CSV creation response
// Contains summary and the created short links
// Items use ShortLinkDTO defined in bot_dto.go for consistency
type AdminCreateShortLinksResponse struct {
	Message    string `json:"message"`
	TotalRows  int    `json:"total_rows"`
	Created    int    `json:"created"`
	Skipped    int    `json:"skipped"`
	ScenarioID uint   `json:"scenario_id"`
}

// Admin download short links request
// Posts scenario_id to retrieve CSV
type AdminDownloadShortLinksRequest struct {
	ScenarioID uint `json:"scenario_id" validate:"required,gt=0"`
}

// Admin download short links with clicks by scenario range [from, to)
// scenario_to must be greater than scenario_from
type AdminDownloadShortLinksRangeRequest struct {
	ScenarioFrom uint `json:"scenario_from" validate:"required,gt=0"`
	ScenarioTo   uint `json:"scenario_to" validate:"required,gt=0,gtfield=ScenarioFrom"`
}
