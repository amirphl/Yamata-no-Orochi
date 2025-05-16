// Package adapters provides adapter functions to bridge different layers of the application
package adapters

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/handlers"
	businessflow "github.com/amirphl/Yamata-no-Orochi/business_flow"
)

// HandlerSignupFlowAdapter adapts business flow SignupFlow to handler SignupFlow
type HandlerSignupFlowAdapter struct {
	signupFlow businessflow.SignupFlow
}

// NewHandlerSignupFlowAdapter creates a new signup flow adapter
func NewHandlerSignupFlowAdapter(signupFlow businessflow.SignupFlow) handlers.SignupFlow {
	return &HandlerSignupFlowAdapter{signupFlow: signupFlow}
}

// InitiateSignup converts handler DTOs to business flow DTOs and calls the business flow
func (a *HandlerSignupFlowAdapter) InitiateSignup(ctx context.Context, req *handlers.SignupRequest, ipAddress, userAgent string) (*handlers.SignupResponse, error) {
	// Convert handler DTO to business flow DTO
	dtoReq := &dto.SignupRequest{
		AccountType:             req.AccountType,
		CompanyName:             req.CompanyName,
		NationalID:              req.NationalID,
		CompanyPhone:            req.CompanyPhone,
		CompanyAddress:          req.CompanyAddress,
		PostalCode:              req.CompanyPostalCode, // Map to correct field
		RepresentativeFirstName: req.RepresentativeFirstName,
		RepresentativeLastName:  req.RepresentativeLastName,
		RepresentativeMobile:    req.RepresentativeMobile,
		Email:                   req.Email,
		Password:                req.Password,
		ConfirmPassword:         req.ConfirmPassword,
		ReferrerAgencyCode:      req.ReferrerAgencyRefererCode, // Map to correct field
	}

	// Create client metadata
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business flow
	dtoResp, err := a.signupFlow.InitiateSignup(ctx, dtoReq, metadata)
	if err != nil {
		return nil, err
	}

	// Convert business flow DTO to handler DTO
	return &handlers.SignupResponse{
		Success:    true, // Always true if no error
		Message:    dtoResp.Message,
		CustomerID: dtoResp.CustomerID,
	}, nil
}

// VerifyOTP converts handler DTOs to business flow DTOs and calls the business flow
func (a *HandlerSignupFlowAdapter) VerifyOTP(ctx context.Context, req *handlers.OTPVerificationRequest, ipAddress, userAgent string) (*handlers.OTPVerificationResponse, error) {
	// Convert handler DTO to business flow DTO
	dtoReq := &dto.OTPVerificationRequest{
		CustomerID: req.CustomerID,
		OTPCode:    req.OTPCode,
		OTPType:    req.OTPType,
	}

	// Create client metadata
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business flow
	dtoResp, err := a.signupFlow.VerifyOTP(ctx, dtoReq, metadata)
	if err != nil {
		return nil, err
	}

	// Convert business flow DTO to handler DTO
	return &handlers.OTPVerificationResponse{
		Success:     true, // Always true if no error
		Message:     dtoResp.Message,
		AccessToken: dtoResp.Token, // Map to correct field
	}, nil
}

// ResendOTP delegates to the business flow
func (a *HandlerSignupFlowAdapter) ResendOTP(ctx context.Context, customerID uint, otpType string, ipAddress, userAgent string) error {
	// Create client metadata
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)
	return a.signupFlow.ResendOTP(ctx, customerID, otpType, metadata)
}

// HandlerLoginFlowAdapter adapts business flow LoginFlow to handler LoginFlow
type HandlerLoginFlowAdapter struct {
	loginFlow businessflow.LoginFlow
}

// NewHandlerLoginFlowAdapter creates a new login flow adapter
func NewHandlerLoginFlowAdapter(loginFlow businessflow.LoginFlow) handlers.LoginFlow {
	return &HandlerLoginFlowAdapter{loginFlow: loginFlow}
}

// Login converts handler DTOs to business flow DTOs and calls the business flow
func (a *HandlerLoginFlowAdapter) Login(ctx context.Context, req *handlers.LoginRequest, ipAddress, userAgent string) (*handlers.LoginResult, error) {
	// Convert handler DTO to business flow DTO
	dtoReq := &dto.LoginRequest{
		Identifier: req.Identifier,
		Password:   req.Password,
	}

	// Create client metadata
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business flow
	dtoResp, err := a.loginFlow.Login(ctx, dtoReq, metadata)
	if err != nil {
		return nil, err
	}

	// Convert business flow DTO to handler DTO
	return &handlers.LoginResult{
		Success:      dtoResp.Success,
		Customer:     dtoResp.Customer,
		AccountType:  dtoResp.AccountType,
		Session:      dtoResp.Session,
		ErrorCode:    dtoResp.ErrorCode,
		ErrorMessage: dtoResp.ErrorMessage,
	}, nil
}

// ForgotPassword converts handler DTOs to business flow DTOs and calls the business flow
func (a *HandlerLoginFlowAdapter) ForgotPassword(ctx context.Context, req *handlers.ForgotPasswordRequest, ipAddress, userAgent string) (*handlers.PasswordResetResult, error) {
	// Convert handler DTO to business flow DTO
	dtoReq := &dto.ForgotPasswordRequest{
		Identifier: req.Identifier,
	}

	// Create client metadata
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business flow
	dtoResp, err := a.loginFlow.ForgotPassword(ctx, dtoReq, metadata)
	if err != nil {
		return nil, err
	}

	// Convert business flow DTO to handler DTO
	return &handlers.PasswordResetResult{
		Success:      dtoResp.Success,
		CustomerID:   dtoResp.CustomerID,
		MaskedPhone:  dtoResp.MaskedPhone,
		OTPExpiry:    dtoResp.OTPExpiry,
		ErrorCode:    dtoResp.ErrorCode,
		ErrorMessage: dtoResp.ErrorMessage,
	}, nil
}

// ResetPassword converts handler DTOs to business flow DTOs and calls the business flow
func (a *HandlerLoginFlowAdapter) ResetPassword(ctx context.Context, req *handlers.ResetPasswordRequest, ipAddress, userAgent string) (*handlers.LoginResult, error) {
	// Convert handler DTO to business flow DTO
	dtoReq := &dto.ResetPasswordRequest{
		CustomerID:      req.CustomerID,
		OTPCode:         req.OTPCode,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	}

	// Create client metadata
	metadata := businessflow.NewClientMetadata(ipAddress, userAgent)

	// Call business flow
	dtoResp, err := a.loginFlow.ResetPassword(ctx, dtoReq, metadata)
	if err != nil {
		return nil, err
	}

	// Convert business flow DTO to handler DTO
	return &handlers.LoginResult{
		Success:      dtoResp.Success,
		Customer:     dtoResp.Customer,
		AccountType:  dtoResp.AccountType,
		Session:      dtoResp.Session,
		ErrorCode:    dtoResp.ErrorCode,
		ErrorMessage: dtoResp.ErrorMessage,
	}, nil
}
