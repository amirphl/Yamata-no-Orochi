// Package dto contains Data Transfer Objects for API request and response structures
package dto

import "time"

// SignupRequest represents the signup form data
type SignupRequest struct {
	// Account type selection
	AccountType string `json:"account_type" validate:"required,oneof=individual independent_company marketing_agency"`

	// Company fields (required for independent_company and marketing_agency)
	CompanyName    *string `json:"company_name,omitempty" validate:"omitempty,max=60"`
	NationalID     *string `json:"national_id,omitempty" validate:"omitempty,len=11,numeric"`
	CompanyPhone   *string `json:"company_phone,omitempty" validate:"omitempty,min=10"`
	CompanyAddress *string `json:"company_address,omitempty" validate:"omitempty,max=255"`
	PostalCode     *string `json:"postal_code,omitempty" validate:"omitempty,len=10,numeric"`

	// Representative/Individual fields (required for all types)
	RepresentativeFirstName string `json:"representative_first_name" validate:"required,max=255,alpha_space"`
	RepresentativeLastName  string `json:"representative_last_name" validate:"required,max=255,alpha_space"`
	RepresentativeMobile    string `json:"representative_mobile" validate:"required,mobile_format"`

	// Common fields (required for all types)
	Email           string `json:"email" validate:"required,email,max=255"`
	Password        string `json:"password" validate:"required,min=8,password_strength"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=Password"`

	// Optional agency referral
	ReferrerAgencyCode *int64 `json:"referrer_agency_code,omitempty" validate:"omitempty"`
}

// SignupResponse represents the response after successful signup initiation
type SignupResponse struct {
	Message    string `json:"message"`
	CustomerID uint   `json:"customer_id"`
	OTPSent    bool   `json:"otp_sent"`
	OTPTarget  string `json:"otp_target"` // Mobile number (masked for security)
}

// OTPVerificationRequest represents the OTP verification request
type OTPVerificationRequest struct {
	CustomerID uint   `json:"customer_id" validate:"required"`
	OTPCode    string `json:"otp_code" validate:"required,len=6,numeric"`
	OTPType    string `json:"otp_type" validate:"required,oneof=mobile email"`
}

// OTPVerificationResponse represents the response after successful OTP verification
type OTPVerificationResponse struct {
	Message      string      `json:"message"`
	Token        string      `json:"token"`
	RefreshToken string      `json:"refresh_token"`
	Customer     CustomerDTO `json:"customer"`
}

// CustomerDTO represents customer data for API responses
type CustomerDTO struct {
	ID                      uint      `json:"id"`
	UUID                    string    `json:"uuid"`
	AccountType             string    `json:"account_type"`
	CompanyName             *string   `json:"company_name,omitempty"`
	NationalID              *string   `json:"national_id,omitempty"`
	CompanyPhone            *string   `json:"company_phone,omitempty"`
	CompanyAddress          *string   `json:"company_address,omitempty"`
	PostalCode              *string   `json:"postal_code,omitempty"`
	RepresentativeFirstName string    `json:"representative_first_name"`
	RepresentativeLastName  string    `json:"representative_last_name"`
	RepresentativeMobile    string    `json:"representative_mobile"`
	Email                   string    `json:"email"`
	IsEmailVerified         *bool     `json:"is_email_verified"`
	IsMobileVerified        *bool     `json:"is_mobile_verified"`
	IsActive                *bool     `json:"is_active"`
	CreatedAt               time.Time `json:"created_at"`
	ReferrerAgencyID        *uint     `json:"referrer_agency_id,omitempty"`
}

// ErrorResponse represents API error responses
type ErrorResponse struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"` // Field-specific validation errors
}

// SuccessResponse represents generic success responses
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
