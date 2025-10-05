package dto

import "time"

// AdminCustomersSharesRequest holds optional date filters for the report
type AdminCustomersSharesRequest struct {
	StartDate *string `json:"start_date,omitempty" validate:"omitempty"`
	EndDate   *string `json:"end_date,omitempty" validate:"omitempty"`
}

// AdminCustomersSharesItem represents a single row in the report
type AdminCustomersSharesItem struct {
	FirstName          string  `json:"first_name"`
	LastName           string  `json:"last_name"`
	FullName           string  `json:"full_name"`
	CompanyName        string  `json:"company_name"`
	ReferrerAgencyName string  `json:"referrer_agency_name"`
	AgencyShareWithTax uint64  `json:"agency_share_with_tax"`
	SystemShare        uint64  `json:"system_share"`
	TaxShare           uint64  `json:"tax_share"`
	TotalSent          uint64  `json:"total_sent"`
	ClickRate          float64 `json:"click_rate"`
}

// AdminCustomersSharesResponse is the API response for the report
type AdminCustomersSharesResponse struct {
	Message               string                     `json:"message"`
	Items                 []AdminCustomersSharesItem `json:"items"`
	SumAgencyShareWithTax uint64                     `json:"sum_agency_share_with_tax"`
	SumSystemShare        uint64                     `json:"sum_system_share"`
	SumTaxShare           uint64                     `json:"sum_tax_share"`
	SumTotalSent          uint64                     `json:"sum_total_sent"`
}

// AdminCustomerDetailDTO contains full customer info for admin
type AdminCustomerDetailDTO struct {
	ID                      uint       `json:"id"`
	UUID                    string     `json:"uuid"`
	AgencyRefererCode       string     `json:"agency_referer_code"`
	AccountTypeID           uint       `json:"account_type_id"`
	AccountTypeName         string     `json:"account_type_name"`
	CompanyName             *string    `json:"company_name,omitempty"`
	NationalID              *string    `json:"national_id,omitempty"`
	CompanyPhone            *string    `json:"company_phone,omitempty"`
	CompanyAddress          *string    `json:"company_address,omitempty"`
	PostalCode              *string    `json:"postal_code,omitempty"`
	RepresentativeFirstName string     `json:"representative_first_name"`
	RepresentativeLastName  string     `json:"representative_last_name"`
	RepresentativeMobile    string     `json:"representative_mobile"`
	Email                   string     `json:"email"`
	ShebaNumber             *string    `json:"sheba_number,omitempty"`
	ReferrerAgencyID        *uint      `json:"referrer_agency_id,omitempty"`
	IsEmailVerified         *bool      `json:"is_email_verified,omitempty"`
	IsMobileVerified        *bool      `json:"is_mobile_verified,omitempty"`
	IsActive                *bool      `json:"is_active,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at,omitempty"`
	EmailVerifiedAt         *time.Time `json:"email_verified_at,omitempty"`
	MobileVerifiedAt        *time.Time `json:"mobile_verified_at,omitempty"`
	LastLoginAt             *time.Time `json:"last_login_at,omitempty"`
}

// AdminCustomerCampaignItem summarizes a campaign for admin list
type AdminCustomerCampaignItem struct {
	Title          *string    `json:"title"`
	CreatedAt      time.Time  `json:"created_at"`
	ScheduleAt     *time.Time `json:"schedule_at,omitempty"`
	Status         string     `json:"status"`
	TotalSent      uint64     `json:"total_sent"`
	TotalDelivered uint64     `json:"total_delivered"`
	ClickRate      float64    `json:"click_rate"`
}

// AdminCustomerWithCampaignsResponse response payload
type AdminCustomerWithCampaignsResponse struct {
	Message   string                      `json:"message"`
	Customer  AdminCustomerDetailDTO      `json:"customer"`
	Campaigns []AdminCustomerCampaignItem `json:"campaigns"`
}
