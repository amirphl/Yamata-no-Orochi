// Package dto contains Data Transfer Objects for API request and response structures
package dto

import (
	"time"
)

// LoginRequest represents the request payload for user login
type LoginRequest struct {
	Identifier string `json:"identifier" validate:"required,min=3,max=255" example:"user@example.com or +989123456789"`
	Password   string `json:"password" validate:"required,min=8,max=100" example:"SecurePass123!"`
}

// LoginResponse represents the successful login response
type LoginResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Login successful"`
	Data    struct {
		AccessToken  string    `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
		RefreshToken string    `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
		TokenType    string    `json:"token_type" example:"Bearer"`
		ExpiresIn    int       `json:"expires_in" example:"3600"`
		User         UserInfo  `json:"user"`
		ExpiresAt    time.Time `json:"expires_at" example:"2024-01-15T16:30:00Z"`
	} `json:"data"`
}

// UserInfo represents user information returned in login response
type UserInfo struct {
	ID                      uint   `json:"id" example:"123"`
	UUID                    string `json:"uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	Email                   string `json:"email" example:"user@example.com"`
	RepresentativeFirstName string `json:"representative_first_name" example:"John"`
	RepresentativeLastName  string `json:"representative_last_name" example:"Doe"`
	RepresentativeMobile    string `json:"representative_mobile" example:"+989123456789"`
	AccountType             string `json:"account_type" example:"individual"`
	CompanyName             string `json:"company_name,omitempty" example:"Tech Company Ltd"`
	IsActive                *bool  `json:"is_active" example:"true"`
	CreatedAt               string `json:"created_at" example:"2024-01-15T10:30:00Z"`
}

// ForgotPasswordRequest represents the request to initiate password reset
type ForgotPasswordRequest struct {
	Identifier string `json:"identifier" validate:"required,min=3,max=255" example:"user@example.com or +989123456789"`
}

// ForgotPasswordResponse represents the response after requesting password reset
type ForgotPasswordResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Password reset OTP sent to your mobile number"`
	Data    struct {
		CustomerID  uint   `json:"customer_id" example:"123"`
		MaskedPhone string `json:"masked_phone" example:"+9891234*****"`
		ExpiresIn   int    `json:"expires_in" example:"300"`
	} `json:"data"`
}

// ResetPasswordRequest represents the request to reset password with OTP
type ResetPasswordRequest struct {
	CustomerID      uint   `json:"customer_id" validate:"required" example:"123"`
	OTPCode         string `json:"otp_code" validate:"required,len=6,numeric" example:"123456"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=100,password_strength" example:"NewSecurePass123!"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword" example:"NewSecurePass123!"`
}

// ResetPasswordResponse represents the response after successful password reset
type ResetPasswordResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"New password saved"`
	Data    struct {
		PasswordChangedAt time.Time `json:"password_changed_at" example:"2024-01-15T16:30:00Z"`
	} `json:"data"`
}

// LoginErrorResponse represents error responses for login operations
type LoginErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"User not found"`
	Error   struct {
		Code    string `json:"code" example:"USER_NOT_FOUND"`
		Details string `json:"details,omitempty" example:"No user found with the provided email or mobile number"`
	} `json:"error"`
}

// Common error codes for login operations
const (
	ErrorUserNotFound      = "USER_NOT_FOUND"
	ErrorIncorrectPassword = "INCORRECT_PASSWORD"
	ErrorAccountInactive   = "ACCOUNT_INACTIVE"
	ErrorInvalidOTP        = "INVALID_OTP"
	ErrorOTPExpired        = "OTP_EXPIRED"
	ErrorTooManyAttempts   = "TOO_MANY_ATTEMPTS"
)

func (dto *LoginResponse) SetUserInfo(customerID uint, uuid, email, firstName, lastName, mobile, accountType string, companyName *string, isActive *bool, createdAt time.Time) {
	dto.Data.User = UserInfo{
		ID:                      customerID,
		UUID:                    uuid,
		Email:                   email,
		RepresentativeFirstName: firstName,
		RepresentativeLastName:  lastName,
		RepresentativeMobile:    mobile,
		AccountType:             accountType,
		IsActive:                isActive,
		CreatedAt:               createdAt.Format(time.RFC3339),
	}

	if companyName != nil {
		dto.Data.User.CompanyName = *companyName
	}
}

// MaskPhoneNumber masks the middle digits of a phone number for security
func MaskPhoneNumber(phone string) string {
	if len(phone) < 8 {
		return phone
	}

	// For numbers like +989123456789, show +9891234*****
	if len(phone) >= 10 {
		return phone[:7] + "*****"
	}

	// For shorter numbers, mask the middle part
	start := len(phone) / 3
	end := len(phone) - start
	masked := phone[:start] + "*****" + phone[end:]
	return masked
}
