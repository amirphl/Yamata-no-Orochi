// Package businessflow contains the core business logic and use cases for authentication workflows
package businessflow

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"golang.org/x/crypto/bcrypt"
)

// AdminFlow represents the admin authentication flow used by handlers
type AdminFlow interface {
	InitCaptcha(ctx context.Context) (*dto.AdminCaptchaInitResponse, error)
	Verify(ctx context.Context, req *dto.AdminCaptchaVerifyRequest, metadata *ClientMetadata) (*dto.AdminLoginResponse, error)
}

// AdminFlowImpl provides captcha-init and admin credential verification
type AdminFlowImpl struct {
	adminRepo    repository.AdminRepository
	tokenService services.TokenService
	captchaSvc   services.CaptchaService
}

func NewAdminFlow(adminRepo repository.AdminRepository, tokenService services.TokenService, captchaSvc services.CaptchaService) AdminFlow {
	return &AdminFlowImpl{
		adminRepo:    adminRepo,
		tokenService: tokenService,
		captchaSvc:   captchaSvc,
	}
}

func (af *AdminFlowImpl) InitCaptcha(ctx context.Context) (*dto.AdminCaptchaInitResponse, error) {
	if af.captchaSvc == nil {
		return nil, NewBusinessError("CAPTCHA_NOT_AVAILABLE", "Captcha service not available", ErrCacheNotAvailable)
	}
	ch, err := af.captchaSvc.GenerateRotate(ctx)
	if err != nil {
		return nil, NewBusinessError("CAPTCHA_INIT_FAILED", "Failed to initialize captcha", err)
	}
	return &dto.AdminCaptchaInitResponse{
		ChallengeID:       ch.ID,
		MasterImageBase64: ch.MasterImageBase64,
		ThumbImageBase64:  ch.ThumbImageBase64,
	}, nil
}

func (af *AdminFlowImpl) Verify(ctx context.Context, req *dto.AdminCaptchaVerifyRequest, metadata *ClientMetadata) (*dto.AdminLoginResponse, error) {
	// Validate request
	if req == nil {
		return nil, NewBusinessError("ADMIN_LOGIN_VALIDATION_FAILED", "Admin login validation failed", ErrAdminNotFound)
	}
	if len(req.Username) == 0 || len(req.Password) == 0 {
		return nil, NewBusinessError("ADMIN_LOGIN_VALIDATION_FAILED", "Admin login validation failed", ErrIncorrectPassword)
	}
	if len(req.ChallengeID) == 0 {
		return nil, NewBusinessError("CAPTCHA_INVALID", "Captcha challenge missing", ErrInvalidCaptcha)
	}

	// Verify captcha first
	if af.captchaSvc == nil || !af.captchaSvc.VerifyRotate(ctx, req.ChallengeID, req.UserAngle) {
		return nil, NewBusinessError("CAPTCHA_INVALID", "Captcha validation failed", ErrInvalidCaptcha)
	}

	// Lookup admin
	admin, err := af.adminRepo.ByUsername(ctx, req.Username)
	if err != nil {
		return nil, NewBusinessError("ADMIN_LOOKUP_FAILED", "Failed to lookup admin", err)
	}
	if admin == nil {
		return nil, NewBusinessError("ADMIN_NOT_FOUND", "Admin not found", ErrAdminNotFound)
	}
	if !utils.IsTrue(admin.IsActive) {
		return nil, NewBusinessError("ADMIN_INACTIVE", "Admin account is inactive", ErrAdminInactive)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		return nil, NewBusinessError("ADMIN_INCORRECT_PASSWORD", "Incorrect password", ErrIncorrectPassword)
	}

	// Generate admin tokens
	accessToken, refreshToken, err := af.tokenService.GenerateAdminTokens(admin.ID)
	if err != nil {
		return nil, NewBusinessError("TOKEN_GENERATION_FAILED", "Failed to generate tokens", err)
	}

	resp := &dto.AdminLoginResponse{
		Admin:   ToAdminDTOModel(*admin),
		Session: ToAdminSessionDTO(accessToken, refreshToken),
	}
	return resp, nil
}
