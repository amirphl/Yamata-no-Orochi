package dto

import "time"

type CreateBundleRequest struct {
	CustomerID            uint    `json:"-"`
	Title                 string  `json:"title" validate:"required,max=255"`
	Objective             string  `json:"objective" validate:"required,max=1023"`
	TargetAudiencePersona string  `json:"target_audience_persona" validate:"required,max=1023"`
	AdLink                *string `json:"adlink,omitempty" validate:"omitempty,max=2047"`
	ShortLinkDomain       *string `json:"short_link_domain,omitempty" validate:"omitempty,max=255"`
	Description           *string `json:"description,omitempty" validate:"omitempty,max=2047"`
	TargetCustomerName    *string `json:"target_customer_name,omitempty" validate:"omitempty,max=255"`
	Category              *string `json:"category,omitempty" validate:"omitempty,max=255"`
	Job                   *string `json:"job,omitempty" validate:"omitempty,max=255"`
}

type CreateBundleResponse struct {
	Message   string    `json:"message"`
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type GetBundleRequest struct {
	CustomerID uint `json:"-"`
	ID         uint `json:"id" validate:"required,min=1"`
}

type GetBundleResponse struct {
	Message string      `json:"message"`
	Item    *BundleItem `json:"item,omitempty"`
}

type ListBundlesFilter struct {
	Title              *string `json:"title,omitempty" validate:"omitempty,max=255"`
	TargetCustomerName *string `json:"target_customer_name,omitempty" validate:"omitempty,max=255"`
}

type ListBundlesRequest struct {
	CustomerID uint               `json:"-"`
	Page       int                `json:"page" validate:"omitempty,min=1,max=100"`
	Limit      int                `json:"limit" validate:"omitempty,min=1,max=100"`
	Filter     *ListBundlesFilter `json:"filter,omitempty" validate:"omitempty"`
}

type BundleItem struct {
	ID                    uint           `json:"id"`
	Title                 string         `json:"title"`
	Objective             string         `json:"objective"`
	TargetAudiencePersona string         `json:"target_audience_persona"`
	AdLink                *string        `json:"adlink,omitempty"`
	Description           *string        `json:"description,omitempty"`
	ShortLinkDomain       *string        `json:"short_link_domain,omitempty"`
	TargetCustomerName    *string        `json:"target_customer_name,omitempty"`
	Category              *string        `json:"job_category,omitempty"`
	Job                   *string        `json:"job,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	Statistics            map[string]any `json:"statistics,omitempty"`
	CustomerID            uint           `json:"customer_id"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

type ListBundlesResponse struct {
	Message    string         `json:"message"`
	Items      []BundleItem   `json:"items"`
	Pagination PaginationInfo `json:"pagination"`
}
