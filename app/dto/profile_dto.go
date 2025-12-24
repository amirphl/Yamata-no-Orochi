package dto

import "time"

type AgencyProfileDTO struct {
	AgencyRefererCode string    `json:"agency_referer_code"`
	AccountType       string    `json:"account_type"`
	DisplayName       string    `json:"display_name"`
	CompanyName       *string   `json:"company_name,omitempty"`
	IsActive          *bool     `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ProfileDTO struct {
	ID                      uint       `json:"id"`
	UUID                    string     `json:"uuid"`
	AccountType             string     `json:"account_type"`
	AccountTypeDisplayName  string     `json:"account_type_display_name"`
	Email                   string     `json:"email"`
	RepresentativeFirstName string     `json:"representative_first_name"`
	RepresentativeLastName  string     `json:"representative_last_name"`
	RepresentativeMobile    string     `json:"representative_mobile"`
	CompanyName             *string    `json:"company_name,omitempty"`
	NationalID              *string    `json:"national_id,omitempty"`
	CompanyPhone            *string    `json:"company_phone,omitempty"`
	CompanyAddress          *string    `json:"company_address,omitempty"`
	PostalCode              *string    `json:"postal_code,omitempty"`
	ShebaNumber             *string    `json:"sheba_number,omitempty"`
	Category                *string    `json:"category,omitempty"`
	Job                     *string    `json:"job,omitempty"`
	AgencyRefererCode       string     `json:"agency_referer_code"`
	ReferrerAgencyID        *uint      `json:"referrer_agency_id,omitempty"`
	IsActive                *bool      `json:"is_active"`
	IsEmailVerified         *bool      `json:"is_email_verified"`
	IsMobileVerified        *bool      `json:"is_mobile_verified"`
	LastLoginAt             *time.Time `json:"last_login_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	// Agency-specific/helpful fields
	AgencyID         *uint   `json:"agency_id,omitempty"`
	ParentAgencyName *string `json:"parent_agency_name,omitempty"`
}

type GetProfileResponse struct {
	Message      string            `json:"message"`
	Customer     ProfileDTO        `json:"customer"`
	ParentAgency *AgencyProfileDTO `json:"parent_agency,omitempty"`
}
