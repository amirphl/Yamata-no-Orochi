package dto

import "time"

type AgencyCustomerReportFilter struct {
	StartDate *string `json:"start_date,omitempty"`
	EndDate   *string `json:"end_date,omitempty"`
	Name      *string `json:"name,omitempty" validate:"max=255"`
}

type AgencyCustomerReportRequest struct {
	AgencyID uint                        `json:"-"`
	OrderBy  string                      `json:"orderby"` // e.g., name_asc, name_desc, sent_desc, share_desc
	Filter   *AgencyCustomerReportFilter `json:"filter,omitempty"`
}

type AgencyCustomerReportItem struct {
	FirstName               string `json:"first_name"`
	LastName                string `json:"last_name"`
	CompanyName             string `json:"company_name"`
	TotalSent               uint64 `json:"total_sent"`
	TotalAgencyShareWithTax uint64 `json:"total_agency_share_with_tax"`
}

type AgencyCustomerReportResponse struct {
	Message                    string                     `json:"message"`
	Items                      []AgencyCustomerReportItem `json:"items"`
	SumTotalAgencyShareWithTax uint64                     `json:"sum_total_agency_share_with_tax"`
	SumTotalSent               uint64                     `json:"sum_total_sent"`
}

type ListAgencyActiveDiscountsFilter struct {
	Name *string `json:"name,omitempty" validate:"max=255"`
}

type ListAgencyActiveDiscountsRequest struct {
	AgencyID uint                             `json:"-"`
	Filter   *ListAgencyActiveDiscountsFilter `json:"filter,omitempty"`
}

type AgencyActiveDiscountItem struct {
	CustomerUUID string    `json:"customer_uuid"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	CompanyName  *string   `json:"company_name,omitempty"`
	Rate         float64   `json:"rate"`
	CreatedAt    time.Time `json:"created_at"`
}

type ListAgencyActiveDiscountsResponse struct {
	Message string                     `json:"message"`
	Items   []AgencyActiveDiscountItem `json:"items"`
}

type ListAgencyCustomerDiscountsRequest struct {
	AgencyID   uint `json:"-"`
	CustomerID uint `json:"-"`
}

type AgencyCustomerDiscountItem struct {
	DiscountRate       float64    `json:"discount_rate"`
	CreatedAt          time.Time  `json:"created_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	TotalSent          uint64     `json:"total_sent"`
	AgencyShareWithTax uint64     `json:"agency_share_with_tax"`
}

type ListAgencyCustomerDiscountsResponse struct {
	Message string                       `json:"message"`
	Items   []AgencyCustomerDiscountItem `json:"items"`
}

type CreateAgencyDiscountRequest struct {
	AgencyID   uint    `json:"-"`
	CustomerID uint    `json:"customer_id" validate:"required"`
	Name       string  `json:"name" validate:"required,max=255"`
	Rate       float64 `json:"rate" validate:"required,gte=0,lte=0.5"`
}

type CreateAgencyDiscountResponse struct {
	Message string  `json:"message"`
	Rate    float64 `json:"rate"`
}
