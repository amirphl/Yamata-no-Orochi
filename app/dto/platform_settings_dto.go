package dto

// CreatePlatformSettingsRequest represents payload for creating platform settings.
type CreatePlatformSettingsRequest struct {
	Name           *string `json:"name,omitempty" validate:"omitempty,max=255"`
	Description    *string `json:"description,omitempty"`
	MultimediaUUID *string `json:"multimedia_uuid,omitempty"`
	Platform       string  `json:"platform" validate:"required,oneof=rubika bale splus"`
	Status         *string `json:"status,omitempty" validate:"omitempty,oneof=initialized in-progress active inactive"`
}

// CreatePlatformSettingsResponse represents a successful creation response.
type CreatePlatformSettingsResponse struct {
	Message        string  `json:"message"`
	ID             uint    `json:"id"`
	Platform       string  `json:"platform"`
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	MultimediaUUID *string `json:"multimedia_uuid,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

// PlatformSettingsItem represents a platform settings row for listing.
type PlatformSettingsItem struct {
	ID             uint    `json:"id"`
	Platform       string  `json:"platform"`
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	MultimediaUUID *string `json:"multimedia_uuid,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// ListPlatformSettingsResponse represents a list of platform settings.
type ListPlatformSettingsResponse struct {
	Message string                 `json:"message"`
	Items   []PlatformSettingsItem `json:"items"`
}
