// Package businessflow contains the core business logic and use cases for campaign workflows
package businessflow

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// CampaignFlow handles the campaign business logic
type CampaignFlow interface {
	CreateCampaign(ctx context.Context, req *dto.CreateCampaignRequest, metadata *ClientMetadata) (*dto.CreateCampaignResponse, error)
	UpdateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, metadata *ClientMetadata) (*dto.UpdateCampaignResponse, error)
	CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error)
	CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error)
	CalculateCampaignCostV2(ctx context.Context, req *dto.CalculateCampaignCostV2Request, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error)
	ListCampaigns(ctx context.Context, req *dto.ListCampaignsRequest, metadata *ClientMetadata) (*dto.ListCampaignsResponse, error)
	GetLastInitiatedCampaign(ctx context.Context, customerID uint, metadata *ClientMetadata) (*dto.GetLastInitiatedCampaignResponse, error)
	GetPagePrices(ctx context.Context) (*dto.GetPagePricesResponse, error)
	ListAudienceSpec(ctx context.Context, platform *string) (*dto.ListAudienceSpecResponse, error)
	GetApprovedRunningSummary(ctx context.Context, customerID uint) (*dto.CampaignsSummaryResponse, error)
	CancelCampaign(ctx context.Context, req *dto.CancelCampaignRequest, metadata *ClientMetadata) (*dto.CancelCampaignResponse, error)
	CloneCampaign(ctx context.Context, req *dto.CloneCampaignRequest, metadata *ClientMetadata) (*dto.CloneCampaignResponse, error)
	ExportCampaignReport(ctx context.Context, campaignID string) ([]byte, error)
	ExportCampaignClickReport(ctx context.Context, campaignUUID string) ([]byte, error)
	SendCampaignTestMessage(ctx context.Context, req *dto.SendCampaignTestMessageRequest, metadata *ClientMetadata) (*dto.SendCampaignTestMessageResponse, error)
}

// CampaignFlowImpl implements the campaign business flow
type CampaignFlowImpl struct {
	campaignRepo          repository.CampaignRepository
	bundleRepo            repository.BundleRepository
	shortLinkRepo         repository.ShortLinkRepository
	customerRepo          repository.CustomerRepository
	multimediaRepo        repository.MultimediaAssetRepository
	platformSettingsRepo  repository.PlatformSettingsRepository
	walletRepo            repository.WalletRepository
	balanceSnapshotRepo   repository.BalanceSnapshotRepository
	transactionRepo       repository.TransactionRepository
	auditRepo             repository.AuditLogRepository
	lineNumberRepo        repository.LineNumberRepository
	segmentPriceRepo      repository.SegmentPriceFactorRepository
	platformBaseRepo      repository.PlatformBasePriceRepository
	pagePriceRepo         repository.PagePriceRepository
	processedCampaignRepo repository.ProcessedCampaignRepository
	smsStatusResultRepo   repository.SMSStatusResultRepository
	shortLinkClickRepo    repository.ShortLinkClickRepository
	notifier              services.NotificationService
	adminConfig           config.AdminConfig
	cacheConfig           config.CacheConfig
	botConfig             config.BotConfig
	payamSMSConfig        config.PayamSMSConfig
	baleConfig            config.BaleConfig
	rubikaConfig          config.RubikaConfig
	splusConfig           config.SplusConfig
	rc                    *redis.Client
	db                    *gorm.DB
}

const (
	minCampaignBudget            = uint64(100_000)
	maxCampaignBudget            = uint64(160_000_000)
	defaultSegmentPriceFactor    = 1.0
	defaultLineNumberPriceFactor = 1.0
	undeliveredRefundDelay       = 72 * time.Hour
)

var tehranLoc *time.Location

var allowedShortLinkDomains = []string{"jo1n.ir", "joinsahel.ir"}

// NewCampaignFlow creates a new campaign flow instance
func NewCampaignFlow(
	campaignRepo repository.CampaignRepository,
	bundleRepo repository.BundleRepository,
	shortLinkRepo repository.ShortLinkRepository,
	customerRepo repository.CustomerRepository,
	multimediaRepo repository.MultimediaAssetRepository,
	platformSettingsRepo repository.PlatformSettingsRepository,
	walletRepo repository.WalletRepository,
	balanceSnapshotRepo repository.BalanceSnapshotRepository,
	transactionRepo repository.TransactionRepository,
	auditRepo repository.AuditLogRepository,
	lineNumberRepo repository.LineNumberRepository,
	segmentPriceRepo repository.SegmentPriceFactorRepository,
	platformBaseRepo repository.PlatformBasePriceRepository,
	pagePriceRepo repository.PagePriceRepository,
	processedCampaignRepo repository.ProcessedCampaignRepository,
	smsStatusResultRepo repository.SMSStatusResultRepository,
	shortLinkClickRepo repository.ShortLinkClickRepository,
	db *gorm.DB,
	rc *redis.Client,
	notifier services.NotificationService,
	adminConfig config.AdminConfig,
	cacheConfig config.CacheConfig,
	botConfig config.BotConfig,
	payamSMSConfig config.PayamSMSConfig,
	baleConfig config.BaleConfig,
	rubikaConfig config.RubikaConfig,
	splusConfig config.SplusConfig,
) CampaignFlow {
	return &CampaignFlowImpl{
		campaignRepo:          campaignRepo,
		bundleRepo:            bundleRepo,
		shortLinkRepo:         shortLinkRepo,
		customerRepo:          customerRepo,
		multimediaRepo:        multimediaRepo,
		platformSettingsRepo:  platformSettingsRepo,
		walletRepo:            walletRepo,
		balanceSnapshotRepo:   balanceSnapshotRepo,
		transactionRepo:       transactionRepo,
		auditRepo:             auditRepo,
		lineNumberRepo:        lineNumberRepo,
		segmentPriceRepo:      segmentPriceRepo,
		platformBaseRepo:      platformBaseRepo,
		pagePriceRepo:         pagePriceRepo,
		processedCampaignRepo: processedCampaignRepo,
		smsStatusResultRepo:   smsStatusResultRepo,
		shortLinkClickRepo:    shortLinkClickRepo,
		notifier:              notifier,
		adminConfig:           adminConfig,
		cacheConfig:           cacheConfig,
		botConfig:             botConfig,
		payamSMSConfig:        payamSMSConfig,
		baleConfig:            baleConfig,
		rubikaConfig:          rubikaConfig,
		splusConfig:           splusConfig,
		rc:                    rc,
		db:                    db,
	}
}

// CreateCampaign handles the complete campaign creation process
func (s *CampaignFlowImpl) CreateCampaign(ctx context.Context, req *dto.CreateCampaignRequest, metadata *ClientMetadata) (*dto.CreateCampaignResponse, error) {
	// Validate business rules
	if err := s.validateCreateCampaignRequest(ctx, req); err != nil {
		return nil, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}

	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}

	shortLinkDomain, err := sanitizeShortLinkDomain(req.ShortLinkDomain)
	if err != nil {
		return nil, NewBusinessError("SHORT_LINK_DOMAIN_INVALID", "Invalid short link domain", err)
	}
	req.ShortLinkDomain = shortLinkDomain

	category, job, err := sanitizeCategoryAndJob(customer.AccountType.TypeName, req.Category, req.Job, true)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}
	req.Category = category
	req.Job = job

	sanitizedPlatform, err := sanitizeCampaignPlatform(req.Platform)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}
	req.Platform = &sanitizedPlatform

	if err := s.ensureCreateCampaignRefs(
		ctx,
		req.CustomerID,
		req.BundleID,
		req.Phase,
		req.LineNumber,
		req.Level3s,
		sanitizedPlatform,
		req.MediaUUID,
		req.TargetAudienceExcelFileUUID,
		req.PlatformSettingsID,
	); err != nil {
		return nil, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}

	// Use transaction for atomicity
	var campaign *models.Campaign

	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		var err error
		campaign, err = s.createCampaign(txCtx, req, &customer)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Campaign creation failed: %s", err.Error())
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignCreationFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CAMPAIGN_CREATION_FAILED", "Campaign creation failed", err)
	}

	// Log successful creation
	msg := fmt.Sprintf("Campaign created successfully: %s", campaign.UUID.String())
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignCreated, msg, true, nil, metadata)

	// Build resp
	resp := &dto.CreateCampaignResponse{
		Message:   "Campaign created successfully",
		ID:        campaign.ID,
		UUID:      campaign.UUID.String(),
		Status:    string(campaign.Status),
		CreatedAt: campaign.CreatedAt.Format(time.RFC3339),
	}

	return resp, nil
}

// UpdateCampaign handles the campaign update process
func (s *CampaignFlowImpl) UpdateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, metadata *ClientMetadata) (*dto.UpdateCampaignResponse, error) {
	// Validate business rules
	if err := s.validateUpdateCampaignRequest(req); err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}

	// Get customer
	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}

	// Get existing campaign
	campaign, err := getCampaign(ctx, s.campaignRepo, req.UUID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}

	// Check if campaign can be updated (only initiated campaigns can be updated)
	if !canUpdateCampaign(campaign.Status) {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_NOT_ALLOWED", "Campaign cannot be updated in current status", ErrCampaignUpdateNotAllowed)
	}

	// if req.ScheduleAt == nil && campaign.Spec.ScheduleAt == nil {
	// 	req.ScheduleAt = utils.ToPtr(utils.UTCNow().Add(time.Hour))
	// 	campaign.Spec.ScheduleAt = req.ScheduleAt
	// }

	// Validate schedule time must be at least 10 minutes in the future
	// scheduleTime := req.ScheduleAt
	// if scheduleTime == nil {
	// 	scheduleTime = campaign.Spec.ScheduleAt
	// }
	// if scheduleTime != nil && !scheduleTime.IsZero() {
	// 	if scheduleTime.Before(utils.UTCNow().Add(10 * time.Minute)) {
	// 		return nil, NewBusinessError("INVALID_SCHEDULE_TIME", "Schedule time must be at least 10 minutes in the future", ErrScheduleTimeTooSoon)
	// 	}
	// }

	sanitizedShortLinkDomain, err := sanitizeShortLinkDomain(req.ShortLinkDomain)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}
	req.ShortLinkDomain = sanitizedShortLinkDomain

	sanitizedCategory, sanitizedJob, err := sanitizeCategoryAndJob(customer.AccountType.TypeName, req.Category, req.Job, true)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}
	req.Category = sanitizedCategory
	req.Job = sanitizedJob

	sanitizedPlatform, err := sanitizeCampaignPlatform(req.Platform)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}
	req.Platform = &sanitizedPlatform

	finalize := req.Finalize != nil && *req.Finalize

	ensureCampaignSpecDefaults(&campaign.Spec)
	if err := s.ensureUpdateCampaignRefs(
		ctx,
		customer.ID,
		req.BundleID,
		req.Phase,
		req.LineNumber,
		req.Level3s,
		*req.Platform,
		req.MediaUUID,
		req.TargetAudienceExcelFileUUID,
		req.PlatformSettingsID,
		finalize,
	); err != nil {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_VALIDATION_FAILED", "Campaign update validation failed", err)
	}

	// TODO:
	// Title, Sex, City, Adlink, Content, Budget: DTO validation
	// Level1, Level2s, Level3s, Tags, Sex, City: Ensure exist in file/database/cache
	// Adlink: validate URL format and length
	// Content: validate link anchor text and length
	// Schedule time must be in future and at least 10 minutes from now
	// Line number: validate exist in database and is available for the campaign schedule time (not reserved by another campaign)
	// * Move line number and segment price factor queries to ensureCreateCampaignRefs function

	// Use transaction for atomicity
	err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		// Update campaign
		if err := s.updateCampaign(txCtx, req, &campaign); err != nil {
			return err
		}

		// retrieve campaign again (last state)
		campaign, err := getCampaign(txCtx, s.campaignRepo, req.UUID, req.CustomerID)
		if err != nil {
			return err
		}

		capacity, err := s.CalculateCampaignCapacity(txCtx, &dto.CalculateCampaignCapacityRequest{
			CampaignID: campaign.ID,
			CustomerID: campaign.CustomerID,
		}, metadata)
		if err != nil {
			return err
		}
		usingTargetAudienceExcelFile := campaign.Spec.TargetAudienceExcelFileUUID != nil && strings.TrimSpace(*campaign.Spec.TargetAudienceExcelFileUUID) != ""
		if !usingTargetAudienceExcelFile && capacity.Capacity < utils.MinAcceptableCampaignCapacity {
			return ErrInsufficientCampaignCapacity
		}

		if req.Finalize != nil && *req.Finalize {
			if err := s.canFinalizeCampaign(txCtx, campaign, customer); err != nil {
				return err
			}

			lineNumberPriceFactor := defaultLineNumberPriceFactor
			if campaign.Spec.Platform == models.CampaignPlatformSMS {
				lineNumberPriceFactor, err = s.fetchLineNumberPriceFactor(txCtx, campaign.Spec.LineNumber)
				if err != nil {
					return err
				}
			}

			segmentPriceFactor := defaultSegmentPriceFactor
			usingTargetAudienceExcelFile := campaign.Spec.TargetAudienceExcelFileUUID != nil && *campaign.Spec.TargetAudienceExcelFileUUID != ""
			if usingTargetAudienceExcelFile {
				segmentPriceFactor = defaultSegmentPriceFactor
			} else {
				segmentPriceFactor, err = s.fetchSegmentPriceFactor(txCtx, campaign.Spec.Level3s, sanitizedPlatform)
				if err != nil {
					return err
				}
			}

			pbp, err := s.platformBaseRepo.LatestByPlatform(txCtx, campaign.Spec.Platform)
			if err != nil {
				return err
			}
			if pbp == nil {
				return ErrPlatformBasePriceNotFound
			}
			pp, err := s.pagePriceRepo.LatestByPlatform(txCtx, campaign.Spec.Platform)
			if err != nil {
				return err
			}
			if pp == nil {
				return ErrPagePriceNotFound
			}

			cost, err := s.CalculateCampaignCost(txCtx, &dto.CalculateCampaignCostRequest{
				CampaignID: campaign.ID,
				CustomerID: campaign.CustomerID,
			}, metadata)
			if err != nil {
				return err
			}

			campaign.Status = models.CampaignStatusWaitingForApproval
			campaign.NumAudience = utils.ToPtr(cost.NumTargetAudience)
			campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
			if err := s.campaignRepo.Update(txCtx, campaign); err != nil {
				return err
			}

			// Fetch wallet free balance
			wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
			if err != nil {
				return err
			}
			// Update customer with wallet reference
			customer.Wallet = &wallet

			latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
			if err != nil {
				return err
			}

			availableBalance := latestBalance.FreeBalance + latestBalance.CreditBalance
			if availableBalance < cost.TotalCost {
				return ErrInsufficientFunds
			}

			newFreeBalance := latestBalance.FreeBalance
			newCreditBalance := latestBalance.CreditBalance
			remaining := cost.TotalCost

			if remaining <= newFreeBalance {
				newFreeBalance -= remaining
			} else {
				remaining -= newFreeBalance
				newFreeBalance = 0
				newCreditBalance -= remaining
			}
			newFrozenBalance := latestBalance.FrozenBalance + cost.TotalCost

			numPages := s.calculateParts(
				campaign.Spec.Content,
				campaign.Spec.AdLink,
				campaign.Spec.ShortLinkDomain,
				sanitizedPlatform,
			)

			// Build metadata with full campaign spec
			meta := map[string]any{
				"source":                   "campaign_update",
				"operation":                "reserve_budget",
				"campaign_id":              campaign.ID,
				"amount":                   cost.TotalCost,
				"currency":                 utils.TomanCurrency,
				"campaign_spec":            campaign.Spec,
				"base_price":               pbp.Price,
				"page_price":               pp.Price,
				"num_pages":                numPages,
				"line_number_price_factor": lineNumberPriceFactor,
				"segment_price_factor":     segmentPriceFactor,
			}
			metaBytes, _ := json.Marshal(meta)

			corrID := uuid.New()

			newSnapshot := &models.BalanceSnapshot{
				UUID:               uuid.New(),
				CorrelationID:      corrID,
				WalletID:           wallet.ID,
				CustomerID:         customer.ID,
				FreeBalance:        newFreeBalance,
				FrozenBalance:      newFrozenBalance,
				CreditBalance:      newCreditBalance,
				LockedBalance:      latestBalance.LockedBalance,
				SpentOnCampaign:    latestBalance.SpentOnCampaign,
				AgencyShareWithTax: latestBalance.AgencyShareWithTax,
				TotalBalance:       newFreeBalance + newFrozenBalance + newCreditBalance + latestBalance.LockedBalance + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
				Reason:             "campaign_budget_reserved_waiting_for_approval",
				Description:        fmt.Sprintf("Budget reserved for campaign %d", campaign.ID),
				Metadata:           metaBytes,
				CreatedAt:          utils.UTCNow(),
				UpdatedAt:          utils.UTCNow(),
			}
			if err := s.balanceSnapshotRepo.Save(txCtx, newSnapshot); err != nil {
				return err
			}

			beforeMap, err := latestBalance.GetBalanceMap()
			if err != nil {
				return err
			}
			afterMap, err := newSnapshot.GetBalanceMap()
			if err != nil {
				return err
			}

			freezeTx := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: corrID,
				Type:          models.TransactionTypeFreeze,
				Status:        models.TransactionStatusCompleted,
				Amount:        cost.TotalCost,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Campaign budget reserved: %d Tomans for campaign %d", cost.TotalCost, campaign.ID),
				Metadata:      metaBytes,
				CreatedAt:     utils.UTCNow(),
				UpdatedAt:     utils.UTCNow(),
			}
			if err := s.transactionRepo.Save(txCtx, freezeTx); err != nil {
				return err
			}

			// Notify admins about new campaign awaiting approval
			if s.notifier != nil {
				subject := campaign.UUID.String()
				if campaign.Spec.Title != nil {
					subject = *campaign.Spec.Title
				}
				msg := fmt.Sprintf("New campaign pending approval:\n%s", subject)
				for _, mobile := range s.adminConfig.ActiveMobiles() {
					_ = s.notifier.SendSMS(txCtx, mobile, msg, nil)
				}
				// TODO: Resend?
			}
		}

		return nil
	})

	if err != nil {
		errMsg := fmt.Sprintf("Campaign update failed for campaign %d: %s", campaign.ID, err.Error())
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignUpdateFailed, errMsg, false, &errMsg, metadata)

		return nil, NewBusinessError("CAMPAIGN_UPDATE_FAILED", "Campaign update failed", err)
	}

	// Log successful update
	msg := fmt.Sprintf("Campaign updated successfully: %d", campaign.ID)
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignUpdated, msg, true, nil, metadata)

	// Build resp
	resp := &dto.UpdateCampaignResponse{
		Message: "Campaign updated successfully",
	}

	return resp, nil
}

// CloneCampaign clones an existing campaign for the same customer with a fresh identity and reset state.
func (s *CampaignFlowImpl) CloneCampaign(ctx context.Context, req *dto.CloneCampaignRequest, metadata *ClientMetadata) (*dto.CloneCampaignResponse, error) {
	if req == nil || strings.TrimSpace(req.UUID) == "" {
		return nil, NewBusinessError("CLONE_CAMPAIGN_VALIDATION_FAILED", "Campaign UUID is required", ErrCampaignUUIDRequired)
	}

	customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "Failed to lookup customer", err)
	}

	src, err := getCampaign(ctx, s.campaignRepo, req.UUID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}

	ensureCampaignSpecDefaults(&src.Spec)
	src.Spec.ScheduleAt = nil // Clear schedule to avoid cloning campaigns with past schedule times

	clone := models.Campaign{
		UUID:        uuid.New(),
		CustomerID:  src.CustomerID,
		Status:      models.CampaignStatusInitiated,
		Spec:        src.Spec,
		Comment:     nil,
		Statistics:  json.RawMessage(`{}`),
		NumAudience: utils.ToPtr(uint64(0)),
		BundleID:    src.BundleID,
		Phase:       src.Phase,
	}

	if err := s.campaignRepo.Save(ctx, &clone); err != nil {
		errMsg := fmt.Sprintf("Campaign clone failed from %s: %v", src.UUID.String(), err)
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignCreationFailed, errMsg, false, &errMsg, metadata)
		return nil, NewBusinessError("CAMPAIGN_CLONE_FAILED", "Campaign clone failed", err)
	}

	msg := fmt.Sprintf("Campaign cloned from %s to %s", src.UUID.String(), clone.UUID.String())
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignCreated, msg, true, nil, metadata)

	return &dto.CloneCampaignResponse{
		Message:   "Campaign cloned successfully",
		ID:        clone.ID,
		UUID:      clone.UUID.String(),
		Status:    string(clone.Status),
		CreatedAt: clone.CreatedAt.Format(time.RFC3339),
	}, nil
}

// CancelCampaign allows a customer to cancel their own campaign and refunds budget according to current campaign status.
func (s *CampaignFlowImpl) CancelCampaign(ctx context.Context, req *dto.CancelCampaignRequest, metadata *ClientMetadata) (*dto.CancelCampaignResponse, error) {
	// NOTE: Idempotency
	if req == nil || req.CampaignID == 0 {
		return nil, NewBusinessError("CANCEL_CAMPAIGN_VALIDATION_FAILED", "campaign_id is required", ErrCampaignNotFound)
	}

	if !s.tryAcquireFlowLock(ctx, fmt.Sprintf("cancel_campaign:%d", req.CustomerID), 20*time.Second) {
		return nil, NewBusinessError("CANCEL_CAMPAIGN_BUSY", "cancel campaign request is already in progress", ErrInvalidState)
	}

	var campaign *models.Campaign

	err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		var err error
		campaign, err = s.campaignRepo.ByID(txCtx, req.CampaignID)
		if err != nil {
			return err
		}
		if campaign == nil {
			return ErrCampaignNotFound
		}
		if campaign.CustomerID != req.CustomerID {
			return ErrCampaignAccessDenied
		}
		if !canCancelCampaign(campaign.Status) {
			return ErrCampaignNotWaitingForApproval
		}

		customer, err := getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
		if err != nil {
			return err
		}

		wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
		if err != nil {
			return err
		}
		latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
		if err != nil {
			return err
		}

		switch campaign.Status {
		case models.CampaignStatusWaitingForApproval:
			freezeTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
				CustomerID: &campaign.CustomerID,
				CampaignID: &campaign.ID,
				Source:     utils.ToPtr("campaign_update"),
				Operation:  utils.ToPtr("reserve_budget"),
				Type:       utils.ToPtr(models.TransactionTypeFreeze),
				Status:     utils.ToPtr(models.TransactionStatusCompleted),
			}, "id DESC", 0, 0)
			if err != nil {
				return err
			}
			if len(freezeTxs) == 0 {
				return ErrFreezeTransactionNotFound
			}
			if len(freezeTxs) > 1 {
				return ErrMultipleFreezeTransactionsFound
			}
			freezeTx := freezeTxs[0]

			amount := freezeTx.Amount
			if latestBalance.FrozenBalance < amount {
				return ErrInsufficientFunds
			}

			meta := map[string]any{
				"source":      "campaign_cancel",
				"operation":   "cancel_campaign_refund_frozen",
				"campaign_id": campaign.ID,
				"comment":     req.Comment,
			}
			metaBytes, _ := json.Marshal(meta)

			// TODO:
			// newFrozen := latestBalance.FrozenBalance - amount
			// // Restore free/credit split exactly as it was taken during the freeze.
			// freeRefund, creditRefund := computeFreezeRefundSplit(freezeTx, amount)
			// newFree := latestBalance.FreeBalance + freeRefund
			// newCredit := latestBalance.CreditBalance + creditRefund

			// newSnap := &models.BalanceSnapshot{
			// 	UUID:               uuid.New(),
			// 	CorrelationID:      freezeTx.CorrelationID,
			// 	WalletID:           wallet.ID,
			// 	CustomerID:         customer.ID,
			// 	FreeBalance:        newFree,
			// 	FrozenBalance:      newFrozen,
			// 	LockedBalance:      latestBalance.LockedBalance,
			// 	CreditBalance:      newCredit,
			// 	SpentOnCampaign:    latestBalance.SpentOnCampaign,
			// 	AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			// 	TotalBalance:       newFree + newFrozen + latestBalance.LockedBalance + newCredit + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
			// 	Reason:             "campaign_cancelled_budget_refund",
			// 	Description:        fmt.Sprintf("Refund reserved budget for cancelled campaign %d", campaign.ID),
			// 	Metadata:           metaBytes,
			// }

			newFrozen := latestBalance.FrozenBalance - amount
			newCredit := latestBalance.CreditBalance + amount

			newSnap := &models.BalanceSnapshot{
				UUID:               uuid.New(),
				CorrelationID:      freezeTx.CorrelationID,
				WalletID:           wallet.ID,
				CustomerID:         customer.ID,
				FreeBalance:        latestBalance.FreeBalance,
				FrozenBalance:      newFrozen,
				LockedBalance:      latestBalance.LockedBalance,
				CreditBalance:      newCredit,
				SpentOnCampaign:    latestBalance.SpentOnCampaign,
				AgencyShareWithTax: latestBalance.AgencyShareWithTax,
				TotalBalance:       latestBalance.FreeBalance + newFrozen + latestBalance.LockedBalance + newCredit + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
				Reason:             "campaign_cancelled_budget_refund",
				Description:        fmt.Sprintf("Refund reserved budget for cancelled campaign %d", campaign.ID),
				Metadata:           metaBytes,
			}
			if err := s.balanceSnapshotRepo.Save(txCtx, newSnap); err != nil {
				return err
			}

			beforeMap, err := latestBalance.GetBalanceMap()
			if err != nil {
				return err
			}
			afterMap, err := newSnap.GetBalanceMap()
			if err != nil {
				return err
			}

			refundTx := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: freezeTx.CorrelationID,
				Type:          models.TransactionTypeRefund,
				Status:        models.TransactionStatusCompleted,
				Amount:        amount,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Refund reserved budget for cancelled campaign %d", campaign.ID),
				Metadata:      metaBytes,
			}
			if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
				return err
			}
		case models.CampaignStatusApproved:
			if campaign.Spec.ScheduleAt == nil || campaign.Spec.ScheduleAt.IsZero() {
				return ErrCampaignNotWaitingForApproval
			}

			now := utils.UTCNow()
			if campaign.Spec.ScheduleAt.Before(now.Add(10 * time.Minute)) {
				return ErrScheduleTimeTooSoon
			}

			debitTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
				CustomerID: &campaign.CustomerID,
				CampaignID: &campaign.ID,
				Source:     utils.ToPtr("admin_campaign_approve"),
				Operation:  utils.ToPtr("approve_campaign_budget_consume"),
				Type:       utils.ToPtr(models.TransactionTypeFee),
				Status:     utils.ToPtr(models.TransactionStatusCompleted),
			}, "id DESC", 0, 0)
			if err != nil {
				return err
			}
			if len(debitTxs) == 0 {
				return ErrCampaignDebitTransactionNotFound
			}
			if len(debitTxs) > 1 {
				return ErrMultipleCampaignDebitTransactionsFound
			}
			debitTx := debitTxs[0]

			amount := debitTx.Amount
			if latestBalance.SpentOnCampaign < amount {
				return ErrInsufficientFunds
			}

			// TODO:
			// // Recover the original FreeBalance/CreditBalance split by looking up the
			// // freeze transaction that shares the same CorrelationID as the debit tx.
			// debitCorrID := debitTx.CorrelationID
			// origFreezeTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
			// 	CorrelationID: &debitCorrID,
			// 	CustomerID:    &campaign.CustomerID,
			// 	Type:          utils.ToPtr(models.TransactionTypeFreeze),
			// 	Status:        utils.ToPtr(models.TransactionStatusCompleted),
			// }, "id ASC", 1, 0)
			// if err != nil {
			// 	return err
			// }
			// var freeRefund, creditRefund uint64
			// if len(origFreezeTxs) > 0 {
			// 	freeRefund, creditRefund = computeFreezeRefundSplit(origFreezeTxs[0], amount)
			// } else {
			// 	// No freeze tx found; conservatively return all to free balance.
			// 	freeRefund = amount
			// }

			meta := map[string]any{
				"source":      "campaign_cancel",
				"operation":   "cancel_campaign_refund_spent_after_approval",
				"campaign_id": campaign.ID,
				"comment":     req.Comment,
			}
			metaBytes, _ := json.Marshal(meta)

			// TODO:
			// newFree := latestBalance.FreeBalance + freeRefund
			// newCredit := latestBalance.CreditBalance + creditRefund
			newCredit := latestBalance.CreditBalance + amount
			newSpentOnCampaign := latestBalance.SpentOnCampaign - amount

			// TODO:
			// newSnap := &models.BalanceSnapshot{
			// 	UUID:               uuid.New(),
			// 	CorrelationID:      debitTx.CorrelationID,
			// 	WalletID:           wallet.ID,
			// 	CustomerID:         customer.ID,
			// 	FreeBalance:        newFree,
			// 	FrozenBalance:      latestBalance.FrozenBalance,
			// 	LockedBalance:      latestBalance.LockedBalance,
			// 	CreditBalance:      newCredit,
			// 	SpentOnCampaign:    newSpentOnCampaign,
			// 	AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			// 	TotalBalance:       newFree + latestBalance.FrozenBalance + latestBalance.LockedBalance + newCredit + newSpentOnCampaign + latestBalance.AgencyShareWithTax,
			// 	Reason:             "campaign_cancelled_budget_refund_after_approval",
			// 	Description:        fmt.Sprintf("Refund spent budget for cancelled campaign %d after approval", campaign.ID),
			// 	Metadata:           metaBytes,
			// }
			newSnap := &models.BalanceSnapshot{
				UUID:               uuid.New(),
				CorrelationID:      debitTx.CorrelationID,
				WalletID:           wallet.ID,
				CustomerID:         customer.ID,
				FreeBalance:        latestBalance.FreeBalance,
				FrozenBalance:      latestBalance.FrozenBalance,
				LockedBalance:      latestBalance.LockedBalance,
				CreditBalance:      newCredit,
				SpentOnCampaign:    newSpentOnCampaign,
				AgencyShareWithTax: latestBalance.AgencyShareWithTax,
				TotalBalance:       latestBalance.FreeBalance + latestBalance.FrozenBalance + latestBalance.LockedBalance + newCredit + newSpentOnCampaign + latestBalance.AgencyShareWithTax,
				Reason:             "campaign_cancelled_budget_refund_after_approval",
				Description:        fmt.Sprintf("Refund spent budget for cancelled campaign %d after approval", campaign.ID),
				Metadata:           metaBytes,
			}
			if err := s.balanceSnapshotRepo.Save(txCtx, newSnap); err != nil {
				return err
			}

			beforeMap, err := latestBalance.GetBalanceMap()
			if err != nil {
				return err
			}
			afterMap, err := newSnap.GetBalanceMap()
			if err != nil {
				return err
			}

			refundTx := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: debitTx.CorrelationID,
				Type:          models.TransactionTypeRefund,
				Status:        models.TransactionStatusCompleted,
				Amount:        amount,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Refund spent budget for cancelled campaign %d after approval", campaign.ID),
				Metadata:      metaBytes,
			}
			if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
				return err
			}
		default:
			return ErrCampaignNotWaitingForApproval
		}

		campaign.Status = models.CampaignStatusCancelled
		if req.Comment != nil && strings.TrimSpace(*req.Comment) != "" {
			comment := strings.TrimSpace(*req.Comment)
			campaign.Comment = &comment
		}
		campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
		if err := s.campaignRepo.Update(txCtx, *campaign); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, NewBusinessError("CANCEL_CAMPAIGN_FAILED", "Failed to cancel campaign", err)
	}

	return &dto.CancelCampaignResponse{
		Message: "Campaign cancelled successfully",
	}, nil
}

type campaignReportRow struct {
	AudienceProfileUID string
	Status             string
	Clicked            string
}

type campaignStatistics struct {
	TrackingResults []campaignStatisticsTrackingResult `json:"trackingResults"`
}

type campaignStatisticsTrackingResult struct {
	AudienceProfileUID    *string `json:"audienceProfileUID"`
	TrackingID            string  `json:"trackingID"`
	TotalParts            *int64  `json:"totalParts"`
	TotalDeliveredParts   *int64  `json:"totalDeliveredParts"`
	TotalUndeliveredParts *int64  `json:"totalUndeliveredParts"`
	TotalUnknownParts     *int64  `json:"totalUnknownParts"`
	Status                *string `json:"status"`
}

var campaignReportHeaders = []string{
	"Audience Profile UID",
	"Status",
	"Clicked",
}

func (s *CampaignFlowImpl) ExportCampaignReport(ctx context.Context, campaignUUID string) ([]byte, error) {
	campaignUUID = strings.TrimSpace(campaignUUID)
	if campaignUUID == "" {
		return nil, NewBusinessError("CAMPAIGN_UUID_REQUIRED", "campaign uuid is required", ErrCampaignUUIDRequired)
	}
	parsedCampaignUUID, err := uuid.Parse(campaignUUID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_UUID_INVALID", "campaign uuid is invalid", ErrCampaignUUIDRequired)
	}
	campaignUUID = parsedCampaignUUID.String()

	customerID, ok := ctx.Value(utils.CustomerIDKey).(uint)
	if !ok || customerID == 0 {
		return nil, NewBusinessError("MISSING_CUSTOMER_ID", "customer id is required", ErrCustomerNotFound)
	}

	customer, err := getCustomer(ctx, s.customerRepo, customerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_LOOKUP_FAILED", "failed to lookup customer", err)
	}
	auditFailure := func(message string, e error) {
		errMsg := e.Error()
		_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignReportExportFailed, message, false, &errMsg, nil)
	}

	campaign, err := getCampaign(ctx, s.campaignRepo, campaignUUID, customerID)
	if err != nil {
		auditFailure(fmt.Sprintf("Campaign report export failed for campaign UUID %s", campaignUUID), err)
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "failed to lookup campaign", err)
	}

	rows := make([]campaignReportRow, 0)
	if len(campaign.Statistics) > 0 {
		var stats campaignStatistics
		if err := json.Unmarshal(campaign.Statistics, &stats); err != nil {
			auditFailure(fmt.Sprintf("Campaign report export failed for campaign %s", campaign.UUID.String()), err)
			return nil, NewBusinessError("CAMPAIGN_STATISTICS_PARSE_FAILED", "failed to parse campaign statistics", err)
		}

		trackingResults := stats.TrackingResults
		rows = make([]campaignReportRow, 0, len(trackingResults))
		for _, tr := range trackingResults {
			rows = append(rows, campaignReportRow{
				AudienceProfileUID: stringValue(tr.AudienceProfileUID),
				Status:             deriveCampaignExportStatus(tr),
				Clicked:            "", // TODO: Populate clicked status from short link click tracking.
			})
		}
	} else {
		// Keep empty rows when campaign has no statistics yet.
		rows = make([]campaignReportRow, 0)
	}

	reportBytes, err := buildCampaignReportExcel(rows)
	if err != nil {
		auditFailure(fmt.Sprintf("Campaign report export failed for campaign %s", campaign.UUID.String()), err)
		return nil, NewBusinessError("CAMPAIGN_REPORT_EXPORT_FAILED", "failed to generate campaign report", err)
	}

	msg := fmt.Sprintf("Campaign report exported for %s", campaign.UUID.String())
	_ = s.createAuditLog(ctx, &customer, models.AuditActionCampaignReportExported, msg, true, nil, nil)

	return reportBytes, nil
}

// CalculateCampaignCapacity handles the campaign capacity calculation process
func (s *CampaignFlowImpl) CalculateCampaignCapacity(ctx context.Context, req *dto.CalculateCampaignCapacityRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCapacityResponse, error) {
	if err := s.validateCalculateCampaignCapacityRequest(req); err != nil {
		return nil, NewBusinessError("CALCULATE_CAMPAIGN_CAPACITY_VALIDATION_FAILED", "Campaign capacity calculation validation failed", err)
	}

	campaign, err := getCampaignByID(ctx, s.campaignRepo, req.CampaignID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}
	ensureCampaignSpecDefaults(&campaign.Spec)

	usingTargetAudienceExcelFile := campaign.Spec.TargetAudienceExcelFileUUID != nil && strings.TrimSpace(*campaign.Spec.TargetAudienceExcelFileUUID) != ""
	if !usingTargetAudienceExcelFile {
		if campaign.Spec.Level1 == nil {
			return nil, NewBusinessError("CALCULATE_CAMPAIGN_CAPACITY_VALIDATION_FAILED", "Campaign capacity calculation validation failed", ErrCampaignLevel1Required)
		}
		if len(campaign.Spec.Level2s) == 0 {
			return nil, NewBusinessError("CALCULATE_CAMPAIGN_CAPACITY_VALIDATION_FAILED", "Campaign capacity calculation validation failed", ErrCampaignLevel2sRequired)
		}
		if len(campaign.Spec.Level3s) == 0 {
			return nil, NewBusinessError("CALCULATE_CAMPAIGN_CAPACITY_VALIDATION_FAILED", "Campaign capacity calculation validation failed", ErrCampaignLevel3sRequired)
		}
		if len(campaign.Spec.Tags) == 0 {
			return nil, NewBusinessError("CALCULATE_CAMPAIGN_CAPACITY_VALIDATION_FAILED", "Campaign capacity calculation validation failed", ErrCampaignTagsRequired)
		}
	}
	if usingTargetAudienceExcelFile {
		count, err := s.CountTargetAudienceFromExcelFile(ctx, campaign.CustomerID, strings.TrimSpace(*campaign.Spec.TargetAudienceExcelFileUUID))
		if err != nil {
			switch {
			case errors.Is(err, os.ErrNotExist):
				return nil, NewBusinessError("EXCEL_MEDIA_NOT_FOUND", "Excel file not found", ErrCampaignTargetAudienceExcelMediaNotFound)
			case errors.Is(err, ErrCampaignTargetAudienceExcelFileInvalid):
				return nil, NewBusinessError("EXCEL_FILE_INVALID", "Excel file is invalid", ErrCampaignTargetAudienceExcelFileInvalid)
			default:
				return nil, NewBusinessError("EXCEL_CAPACITY_READ_FAILED", "Failed to read excel audience", err)
			}
		}
		return &dto.CalculateCampaignCapacityResponse{
			Message:               "Campaign capacity calculated successfully",
			Capacity:              count,
			AudienceGradeCapacity: map[string]uint64{audienceGradeA: 0, audienceGradeB: 0, audienceGradeC: 0},
		}, nil
	}

	if csvCapacity, foundAllLevel3s, err := calculateCampaignCapacityFromCSV(campaign.Spec.Platform, campaign.Spec.Level3s, campaign.Spec.AudienceGrades); err == nil {
		if !foundAllLevel3s {
			return &dto.CalculateCampaignCapacityResponse{
				Message:               "Campaign capacity calculated successfully",
				Capacity:              0,
				AudienceGradeCapacity: csvCapacity.AudienceGradeCapacity,
			}, nil
		}
		return &dto.CalculateCampaignCapacityResponse{
			Message:               "Campaign capacity calculated successfully",
			Capacity:              csvCapacity.TotalCapacity,
			AudienceGradeCapacity: csvCapacity.AudienceGradeCapacity,
		}, nil
	}

	// Fetch audience spec (from cache or file)
	specResp, err := s.ListAudienceSpec(ctx, &campaign.Spec.Platform)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_FAILED", "Failed to load audience spec", err)
	}

	var capacity uint64

	// Build a set of requested tags for quick lookup
	tagSet := make(map[string]struct{}, len(campaign.Spec.Tags))
	for _, t := range campaign.Spec.Tags {
		if t != "" {
			tagSet[t] = struct{}{}
		}
	}

	// Sum available audience only for requested Level1/Level2/Level3 keys
	// Respect provided tags: if tags set is empty, count all items; otherwise only items with matching tags.
	if campaign.Spec.Level1 != nil {
		l1k := *campaign.Spec.Level1
		l1map, ok := specResp.Spec[l1k]
		if ok {
			// prepare lookups for requested level2s and level3s
			level2Set := make(map[string]struct{}, len(campaign.Spec.Level2s))
			for _, l2 := range campaign.Spec.Level2s {
				if l2 != "" {
					level2Set[l2] = struct{}{}
				}
			}
			level3Set := make(map[string]struct{}, len(campaign.Spec.Level3s))
			for _, l3 := range campaign.Spec.Level3s {
				if l3 != "" {
					level3Set[l3] = struct{}{}
				}
			}

			for l2k, node := range l1map {
				// skip level2s not requested
				if len(level2Set) > 0 {
					if _, ok := level2Set[l2k]; !ok {
						continue
					}
				}
				if len(node.Items) == 0 && len(node.Metadata) == 0 {
					continue
				}
				for l3k, item := range node.Items {
					// skip level3s not requested
					if len(level3Set) > 0 {
						if _, ok := level3Set[l3k]; !ok {
							continue
						}
					}
					if len(tagSet) == 0 {
						capacity += uint64(item.AvailableAudience)
						continue
					}
					matched := false
					for _, it := range item.Tags {
						if _, ok := tagSet[it]; ok {
							matched = true
							break
						}
					}
					if matched {
						capacity += uint64(item.AvailableAudience)
					}
				}
			}
		}
	}

	return &dto.CalculateCampaignCapacityResponse{
		Message:               "Campaign capacity calculated successfully",
		Capacity:              capacity,
		AudienceGradeCapacity: map[string]uint64{audienceGradeA: 0, audienceGradeB: 0, audienceGradeC: 0},
	}, nil
}

// CalculateCampaignCost handles the campaign cost calculation process
func (s *CampaignFlowImpl) CalculateCampaignCost(ctx context.Context, req *dto.CalculateCampaignCostRequest, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error) {
	if req.CampaignID == 0 {
		return nil, NewBusinessError("CALCULATE_CAMPAIGN_COST_VALIDATION_FAILED", "Campaign cost calculation validation failed", ErrCampaignNotFound)
	}

	campaign, err := getCampaignByID(ctx, s.campaignRepo, req.CampaignID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}

	pricePerMsg, availableCapacity, err := s.computeCostInputs(ctx, campaign, metadata)
	if err != nil {
		return nil, err
	}

	numTargetAudience := availableCapacity
	budget := campaign.Spec.Budget
	if req.Budget != nil {
		budget = req.Budget
	}
	if budget != nil {
		numTargetAudience = uint64(math.Min(float64(availableCapacity), float64(*budget)/float64(pricePerMsg)))
	}

	totalCost := pricePerMsg * numTargetAudience

	if req.CustomerID != 0 {
		customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
		if err == nil && s.adminConfig.HasMobile(customer.RepresentativeMobile) {
			totalCost = 0
		}
	}

	return &dto.CalculateCampaignCostResponse{
		Message:           "Campaign cost calculated successfully",
		TotalCost:         totalCost,
		NumTargetAudience: numTargetAudience,
		MaxTargetAudience: availableCapacity,
	}, nil
}

// CalculateCampaignCostV2 calculates required cost for desired num_messages
// and caps num_messages by available audience capacity.
func (s *CampaignFlowImpl) CalculateCampaignCostV2(ctx context.Context, req *dto.CalculateCampaignCostV2Request, metadata *ClientMetadata) (*dto.CalculateCampaignCostResponse, error) {
	campaign, err := getCampaignByID(ctx, s.campaignRepo, req.CampaignID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}

	pricePerMsg, availableCapacity, err := s.computeCostInputs(ctx, campaign, metadata)
	if err != nil {
		return nil, err
	}

	numTargetAudience := req.NumMessages
	if numTargetAudience > availableCapacity {
		numTargetAudience = availableCapacity
	}

	totalCost := pricePerMsg * numTargetAudience

	if req.CustomerID != 0 {
		customer, err := getCustomer(ctx, s.customerRepo, req.CustomerID)
		if err == nil && s.adminConfig.HasMobile(customer.RepresentativeMobile) {
			totalCost = 0
		}
	}

	return &dto.CalculateCampaignCostResponse{
		Message:           "Campaign cost calculated successfully",
		TotalCost:         totalCost,
		NumTargetAudience: numTargetAudience,
		MaxTargetAudience: availableCapacity,
	}, nil
}

func (s *CampaignFlowImpl) computeCostInputs(
	ctx context.Context,
	campaign models.Campaign,
	metadata *ClientMetadata,
) (uint64, uint64, error) {
	platform, err := sanitizeCampaignPlatform(&campaign.Spec.Platform)
	if err != nil {
		return 0, 0, NewBusinessError("CAMPAIGN_VALIDATION_FAILED", "Campaign validation failed", err)
	}

	if platform == models.CampaignPlatformSMS && campaign.Spec.LineNumber == nil {
		return 0, 0, NewBusinessError("LINE_NUMBER_REQUIRED", "Line number is required for SMS campaigns", ErrCampaignLineNumberRequired)
	}

	usingTargetAudienceExcelFile := campaign.Spec.TargetAudienceExcelFileUUID != nil && strings.TrimSpace(*campaign.Spec.TargetAudienceExcelFileUUID) != ""
	if len(campaign.Spec.Level3s) == 0 && !usingTargetAudienceExcelFile {
		return 0, 0, NewBusinessError("LEVEL3_REQUIRED", "At least one level3 option or target audience Excel file is required for cost calculation", ErrLevel3Required)
	}

	// Pricing constants
	numParts := s.calculateParts(
		campaign.Spec.Content,
		campaign.Spec.AdLink,
		campaign.Spec.ShortLinkDomain,
		platform,
	)

	lineNumberFactor := defaultLineNumberPriceFactor
	if platform == models.CampaignPlatformSMS && campaign.Spec.LineNumber != nil && strings.TrimSpace(*campaign.Spec.LineNumber) != "" {
		var err error
		lineNumberFactor, err = s.fetchLineNumberPriceFactor(ctx, campaign.Spec.LineNumber)
		if err != nil {
			return 0, 0, NewBusinessError("LINE_NUMBER_PRICE_FACTOR_FETCH_FAILED", "Failed to fetch line number price factor", err)
		}
	}

	segmentPriceFactor := defaultSegmentPriceFactor
	if len(campaign.Spec.Level3s) > 0 && !usingTargetAudienceExcelFile {
		maxFactor, err := s.fetchSegmentPriceFactor(ctx, campaign.Spec.Level3s, platform)
		if err != nil {
			if errors.Is(err, ErrSegmentPriceFactorNotFound) {
				s.notifyMissingSegmentPriceFactor(campaign.Spec.Level3s)
				return 0, 0, NewBusinessError("SEGMENT_PRICE_FACTOR_NOT_FOUND", "Segment price factor not found for provided level3 options", ErrSegmentPriceFactorNotFound)
			}
			return 0, 0, NewBusinessError("SEGMENT_PRICE_FACTOR_FETCH_FAILED", "Failed to fetch segment price factors", err)
		}
		if maxFactor == 0 {
			s.notifyMissingSegmentPriceFactor(campaign.Spec.Level3s)
			return 0, 0, NewBusinessError("SEGMENT_PRICE_FACTOR_NOT_FOUND", "Segment price factor not found for provided level3 options", ErrSegmentPriceFactorNotFound)
		}
		segmentPriceFactor = maxFactor
	}

	pricePerMsg := uint64(0)

	pbp, err := s.platformBaseRepo.LatestByPlatform(ctx, platform)
	if err != nil {
		return 0, 0, NewBusinessError("PLATFORM_BASE_PRICE_FETCH_FAILED", "Failed to fetch platform base price", err)
	}
	if pbp == nil {
		return 0, 0, NewBusinessError("PLATFORM_BASE_PRICE_NOT_FOUND", "Platform base price not found for platform "+platform, ErrPlatformBasePriceNotFound)
	}
	pp, err := s.pagePriceRepo.LatestByPlatform(ctx, platform)
	if err != nil {
		return 0, 0, NewBusinessError("PAGE_PRICE_FETCH_FAILED", "Failed to fetch page price", err)
	}
	if pp == nil {
		return 0, 0, NewBusinessError("PAGE_PRICE_NOT_FOUND", "Page price not found for platform "+platform, ErrPagePriceNotFound)
	}
	pagePrice := float64(pp.Price)
	numPages := float64(numParts)
	if platform == models.CampaignPlatformSMS {
		pricePerMsg = pbp.Price*uint64(lineNumberFactor*numPages) + uint64(segmentPriceFactor*pagePrice)
	} else {
		pricePerMsg = pbp.Price*uint64(1*1) + uint64(segmentPriceFactor*pagePrice)
	}

	// Calculate campaign capacity (target audience size)
	capacityResp, err := s.CalculateCampaignCapacity(ctx, &dto.CalculateCampaignCapacityRequest{
		CampaignID: campaign.ID,
		CustomerID: campaign.CustomerID,
	}, metadata)
	if err != nil {
		return 0, 0, NewBusinessError("CAPACITY_CALCULATION_FAILED", "Failed to calculate campaign capacity", err)
	}
	return pricePerMsg, capacityResp.Capacity, nil
}

func (s *CampaignFlowImpl) fetchLineNumberPriceFactor(ctx context.Context, lineNumber *string) (float64, error) {
	if lineNumber == nil || strings.TrimSpace(*lineNumber) == "" {
		return 0, ErrLineNumberNotFound
	}

	ln, err := s.lineNumberRepo.ByValue(ctx, *lineNumber)
	if err != nil {
		return 0, err
	}
	if ln == nil {
		return 0, ErrLineNumberNotFound
	}
	if !utils.IsTrue(ln.IsActive) {
		return 0, ErrLineNumberNotActive
	}

	return ln.PriceFactor, nil
}

func (s *CampaignFlowImpl) fetchSegmentPriceFactor(ctx context.Context, level3s []string, platform string) (float64, error) {
	factors, err := s.segmentPriceRepo.LatestByLevel3sForPlatform(ctx, level3s, platform)
	if err != nil {
		return 0, err
	}
	maxFactor := float64(0)
	for _, l3 := range level3s {
		if f, ok := factors[l3]; ok && f > maxFactor {
			maxFactor = f
		}
	}
	if maxFactor == 0 {
		return 0, ErrSegmentPriceFactorNotFound
	}

	return maxFactor, nil
}

func (s *CampaignFlowImpl) notifyMissingSegmentPriceFactor(level3s []string) {
	if s.notifier == nil {
		return
	}
	msg := fmt.Sprintf("Segment price factor missing for level3: %s", strings.Join(level3s, ","))
	go func() {
		for _, mobile := range s.adminConfig.ActiveMobiles() {
			_ = s.notifier.SendSMS(context.Background(), mobile, msg, nil)
		}
	}()
}

// campaignDisplayEnrichments holds computed pricing and settings data for building a GetCampaignResponse.
type campaignDisplayEnrichments struct {
	linePriceFactor      *float64
	segmentPriceFactor   *float64
	platformSettingsName *string
}

// fetchCampaignDisplayEnrichments computes linePriceFactor, segmentPriceFactor, and platformSettingsName for a campaign.
func (s *CampaignFlowImpl) fetchCampaignDisplayEnrichments(ctx context.Context, c *models.Campaign) (campaignDisplayEnrichments, error) {
	var e campaignDisplayEnrichments

	lpf, err := s.resolveLinePriceFactor(ctx, c.ID, c.Spec.LineNumber)
	if err != nil {
		return e, err
	}
	e.linePriceFactor = lpf

	spf, err := s.resolveSegmentPriceFactor(ctx, c.ID, c.Spec.Level3s, c.Spec.Platform)
	if err != nil {
		return e, err
	}
	e.segmentPriceFactor = spf

	name, err := s.resolvePlatformSettingsName(ctx, c.Spec.PlatformSettingsID)
	if err != nil {
		return e, err
	}
	e.platformSettingsName = name

	return e, nil
}

// resolveSegmentPriceFactor reads segment price factor from transaction metadata,
// falling back to the segment price repo if not found.
func (s *CampaignFlowImpl) resolveSegmentPriceFactor(ctx context.Context, campaignID uint, level3s []string, platform string) (*float64, error) {
	spf, err := s.readSegmentPriceFromMetadata(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if spf != nil {
		return spf, nil
	}
	if len(level3s) == 0 {
		return nil, nil
	}
	factor, err := s.fetchSegmentPriceFactor(ctx, level3s, platform)
	if err != nil {
		if errors.Is(err, ErrSegmentPriceFactorNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &factor, nil
}

// readSegmentPriceFromMetadata reads the segment_price_factor from the latest
// campaign finalization transaction metadata.
func (s *CampaignFlowImpl) readSegmentPriceFromMetadata(ctx context.Context, campaignID uint) (*float64, error) {
	source := "campaign_update"
	operation := "reserve_budget"
	txs, err := s.transactionRepo.ByFilter(ctx, models.TransactionFilter{
		CampaignID: &campaignID,
		Source:     &source,
		Operation:  &operation,
	}, "id DESC", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(txs) == 0 || len(txs[0].Metadata) == 0 {
		return nil, nil
	}
	var meta map[string]any
	if err := json.Unmarshal(txs[0].Metadata, &meta); err != nil {
		return nil, nil
	}
	return parseMetadataFloat(meta["segment_price_factor"]), nil
}

// resolveLinePriceFactor reads line price factor from transaction metadata,
// falling back to the line number repo if not found.
func (s *CampaignFlowImpl) resolveLinePriceFactor(ctx context.Context, campaignID uint, lineNumber *string) (*float64, error) {
	lpf, err := s.readLinePriceFromMetadata(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if lpf != nil {
		return lpf, nil
	}
	if lineNumber == nil {
		return nil, nil
	}
	ln, err := s.lineNumberRepo.ByValue(ctx, *lineNumber)
	if err != nil {
		return nil, err
	}
	if ln == nil {
		return nil, nil
	}
	return &ln.PriceFactor, nil
}

// readLinePriceFromMetadata reads the line_number_price_factor from the latest
// campaign finalization transaction metadata.
func (s *CampaignFlowImpl) readLinePriceFromMetadata(ctx context.Context, campaignID uint) (*float64, error) {
	source := "campaign_update"
	operation := "reserve_budget"
	txs, err := s.transactionRepo.ByFilter(ctx, models.TransactionFilter{
		CampaignID: &campaignID,
		Source:     &source,
		Operation:  &operation,
	}, "id DESC", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(txs) == 0 || len(txs[0].Metadata) == 0 {
		return nil, nil
	}
	var meta map[string]any
	if err := json.Unmarshal(txs[0].Metadata, &meta); err != nil {
		return nil, nil
	}
	return parseMetadataFloat(meta["line_number_price_factor"]), nil
}

// resolvePlatformSettingsName returns the Name of a platform settings record by ID.
func (s *CampaignFlowImpl) resolvePlatformSettingsName(ctx context.Context, id *uint) (*string, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	settings, err := s.platformSettingsRepo.ByID(ctx, *id)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, nil
	}
	return settings.Name, nil
}

func campaignPhasePtr(phase models.CampaignPhase) *string {
	if !phase.Valid() {
		return nil
	}
	value := phase.String()
	return &value
}

func campaignPhaseOrDefault(phase *string) models.CampaignPhase {
	if phase == nil {
		return models.CampaignPhaseExecution
	}
	trimmed := strings.TrimSpace(*phase)
	campaignPhase := models.CampaignPhase(trimmed)
	if !campaignPhase.Valid() {
		return models.CampaignPhaseExecution
	}
	return campaignPhase
}

func validateCampaignPhaseInput(phase *string, required bool) error {
	if phase == nil {
		if required {
			return ErrCampaignPhaseRequired
		}
		return nil
	}

	trimmed := strings.TrimSpace(*phase)
	if trimmed == "" {
		if required {
			return ErrCampaignPhaseRequired
		}
		return ErrCampaignPhaseInvalid
	}

	if !models.CampaignPhase(trimmed).Valid() {
		return ErrCampaignPhaseInvalid
	}

	return nil
}

// buildCampaignResponse constructs a GetCampaignResponse from a campaign model and its display enrichments.
func buildCampaignResponse(c *models.Campaign, e campaignDisplayEnrichments) dto.GetCampaignResponse {
	ensureCampaignSpecDefaults(&c.Spec)

	var bundleTitle *string
	if c.Bundle != nil {
		bundleTitle = &c.Bundle.Title
	}

	return dto.GetCampaignResponse{
		ID:                          c.ID,
		UUID:                        c.UUID.String(),
		Status:                      c.Status.String(),
		CreatedAt:                   c.CreatedAt,
		UpdatedAt:                   c.UpdatedAt,
		Title:                       c.Spec.Title,
		Level1:                      c.Spec.Level1,
		Level2s:                     c.Spec.Level2s,
		Level3s:                     c.Spec.Level3s,
		Tags:                        c.Spec.Tags,
		Sex:                         c.Spec.Sex,
		City:                        c.Spec.City,
		AdLink:                      c.Spec.AdLink,
		Content:                     c.Spec.Content,
		ShortLinkDomain:             c.Spec.ShortLinkDomain,
		Category:                    c.Spec.Category,
		Job:                         c.Spec.Job,
		ScheduleAt:                  c.Spec.ScheduleAt,
		LineNumber:                  c.Spec.LineNumber,
		MediaUUID:                   c.Spec.MediaUUID,
		PlatformSettingsID:          c.Spec.PlatformSettingsID,
		Platform:                    c.Spec.Platform,
		LinePriceFactor:             e.linePriceFactor,
		SegmentPriceFactor:          e.segmentPriceFactor,
		PlatformSettingsName:        e.platformSettingsName,
		Budget:                      c.Spec.Budget,
		NumAudience:                 c.NumAudience,
		Comment:                     c.Comment,
		BundleID:                    c.BundleID,
		BundleTitle:                 bundleTitle,
		Phase:                       campaignPhasePtr(c.Phase),
		AudienceGrades:              campaignAudienceGradesOrDefault(c.Spec.AudienceGrades),
		TargetAudienceExcelFileUUID: c.Spec.TargetAudienceExcelFileUUID,
	}
}

// ListCampaigns retrieves user's campaigns with pagination, ordering and filters
func (s *CampaignFlowImpl) ListCampaigns(ctx context.Context, req *dto.ListCampaignsRequest, metadata *ClientMetadata) (*dto.ListCampaignsResponse, error) {
	var err error
	defer func() {
		if err != nil {
			err = NewBusinessError("LIST_CAMPAIGNS_FAILED", "Failed to list campaigns", err)
		}
	}()

	// Validate customer
	_, err = getCustomer(ctx, s.customerRepo, req.CustomerID)
	if err != nil {
		return nil, err
	}

	// if s.tryAcquireFlowLock(ctx, fmt.Sprintf("list_campaigns_expire:%d", req.CustomerID), 20*time.Second) {
	// 	if err := s.expireCustomerCampaigns(ctx, req.CustomerID); err != nil {
	// 		return nil, err
	// 	}
	// }

	if s.tryAcquireFlowLock(ctx, fmt.Sprintf("list_campaigns_reconcile_refund:%d", req.CustomerID), 20*time.Second) {
		if err := s.reconcileUndeliveredCampaignRefunds(ctx, req.CustomerID); err != nil {
			return nil, err
		}
	}

	// Normalize pagination
	page := max(1, req.Page)
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	// Build filter
	filter := models.CampaignFilter{CustomerID: &req.CustomerID}
	if req.Filter != nil {
		if req.Filter.Title != nil {
			title := strings.TrimSpace(*req.Filter.Title)
			if title != "" {
				filter.Title = &title
			}
		}
		if req.Filter.Status != nil {
			statusValue := strings.TrimSpace(*req.Filter.Status)
			status := models.CampaignStatus(statusValue)
			if status.Valid() {
				filter.Status = &status
			}
		}
		if req.Filter.BundleID != nil && *req.Filter.BundleID > 0 {
			filter.BundleID = req.Filter.BundleID
		}
		if req.Filter.Phase != nil {
			phaseValue := strings.TrimSpace(*req.Filter.Phase)
			phase := models.CampaignPhase(phaseValue)
			if phase.Valid() {
				filter.Phase = &phase
			}
		}
	}

	// Order by
	orderBy := "updated_at DESC"
	switch req.OrderBy {
	case "oldest":
		orderBy = "updated_at ASC"
	case "newest":
		orderBy = "updated_at DESC"
	}

	// Count total
	total64, err := s.campaignRepo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Fetch rows
	rows, err := s.campaignRepo.ByFilter(ctx, filter, orderBy, limit, offset)
	if err != nil {
		return nil, err
	}

	// Precompute click counts per campaign
	campaignIDs := make([]uint, 0, len(rows))
	for _, c := range rows {
		campaignIDs = append(campaignIDs, c.ID)
	}
	clickCounts, err := s.campaignRepo.AggregateClickCountsByCampaignIDs(ctx, campaignIDs)
	if err != nil {
		return nil, err
	}

	// Map to response items
	items := make([]dto.GetCampaignResponse, 0, len(rows))
	for _, c := range rows {
		var statsMap map[string]any
		if len(c.Statistics) > 0 {
			_ = json.Unmarshal(c.Statistics, &statsMap)
		}
		clicks := clickCounts[c.ID]
		totalClicks := clicks
		// computeClickRate returns nil when aggregatedTotalSent is 0 so callers can
		// distinguish "campaign not yet executed" from "0% click-through rate".
		// Note: reconcileUndeliveredCampaignRefunds (called above) only appends
		// "undeliveredRefund*" keys to Statistics; it never overwrites
		// aggregatedTotalSent, so the value read here is always the authoritative
		// delivery count set by the campaign scheduler.
		clickRate := computeClickRate(clicks, parseAggregatedTotalSentFromMap(statsMap))

		enrichments, err := s.fetchCampaignDisplayEnrichments(ctx, c)
		if err != nil {
			return nil, err
		}

		item := buildCampaignResponse(c, enrichments)
		item.Statistics = statsMap
		item.ClickRate = clickRate
		item.TotalClicks = &totalClicks

		items = append(items, item)
	}

	// Build pagination
	totalPages := int((total64 + int64(limit) - 1) / int64(limit))

	return &dto.ListCampaignsResponse{
		Message: "Campaigns retrieved successfully",
		Items:   items,
		Pagination: dto.PaginationInfo{
			Total:      total64,
			Page:       page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	}, nil
}

// GetLastInitiatedCampaign retrieves the most recent initiated or in-progress campaign for the given customer.
func (s *CampaignFlowImpl) GetLastInitiatedCampaign(ctx context.Context, customerID uint, metadata *ClientMetadata) (*dto.GetLastInitiatedCampaignResponse, error) {
	_, err := getCustomer(ctx, s.customerRepo, customerID)
	if err != nil {
		return nil, NewBusinessError("GET_LAST_INITIATED_CAMPAIGN_FAILED", "Failed to get last initiated campaign", err)
	}

	fetchLatest := func(status models.CampaignStatus) (*models.Campaign, error) {
		rows, err := s.campaignRepo.ByFilter(ctx, models.CampaignFilter{
			CustomerID: &customerID,
			Status:     &status,
		}, "created_at DESC", 1, 0)
		if err != nil {
			return nil, err
		}

		if len(rows) == 0 {
			return nil, nil
		}

		return rows[0], nil
	}

	initiatedCampaign, err := fetchLatest(models.CampaignStatusInitiated)
	if err != nil {
		return nil, NewBusinessError("GET_LAST_INITIATED_CAMPAIGN_FAILED", "Failed to get last initiated campaign", err)
	}

	inProgressCampaign, err := fetchLatest(models.CampaignStatusInProgress)
	if err != nil {
		return nil, NewBusinessError("GET_LAST_INITIATED_CAMPAIGN_FAILED", "Failed to get last initiated campaign", err)
	}

	var c *models.Campaign
	switch {
	case initiatedCampaign == nil && inProgressCampaign == nil:
		return &dto.GetLastInitiatedCampaignResponse{
			Message: "No initiated or in-progress campaign found",
			Item:    nil,
		}, nil
	case initiatedCampaign == nil:
		c = inProgressCampaign
	case inProgressCampaign == nil:
		c = initiatedCampaign
	default:
		if inProgressCampaign.CreatedAt.After(initiatedCampaign.CreatedAt) {
			c = inProgressCampaign
		} else {
			c = initiatedCampaign
		}
	}
	enrichments, err := s.fetchCampaignDisplayEnrichments(ctx, c)
	if err != nil {
		return nil, NewBusinessError("GET_LAST_INITIATED_CAMPAIGN_FAILED", "Failed to get last initiated campaign", err)
	}

	item := buildCampaignResponse(c, enrichments)

	return &dto.GetLastInitiatedCampaignResponse{
		Message: "Last initiated campaign retrieved successfully",
		Item:    &item,
	}, nil
}

// validateCreateCampaignRequest validates the campaign creation request
func (s *CampaignFlowImpl) validateCreateCampaignRequest(ctx context.Context, req *dto.CreateCampaignRequest) error {
	usingTargetAudienceFromExcelFile := req.TargetAudienceExcelFileUUID != nil && strings.TrimSpace(*req.TargetAudienceExcelFileUUID) != ""

	if req.CustomerID == 0 {
		return ErrCustomerNotFound
	}
	if req.BundleID == nil || *req.BundleID == 0 {
		return ErrBundleNotFound
	}
	if err := validateCampaignPhaseInput(req.Phase, true); err != nil {
		return err
	}
	if req.Title == nil || (req.Title != nil && *req.Title == "") {
		return ErrCampaignTitleRequired
	}
	if req.Content != nil && *req.Content == "" {
		return ErrCampaignContentRequired
	}
	if !usingTargetAudienceFromExcelFile {
		if req.Level1 == nil || (req.Level1 != nil && *req.Level1 == "") {
			return ErrCampaignLevel1Required
		}
		if req.Level2s == nil || (req.Level2s != nil && len(req.Level2s) == 0) {
			return ErrCampaignLevel2sRequired
		}
		if req.Level3s == nil || (req.Level3s != nil && len(req.Level3s) == 0) {
			return ErrCampaignLevel3sRequired
		}
		if req.Tags == nil || (req.Tags != nil && len(req.Tags) == 0) {
			return ErrCampaignTagsRequired
		}
	} else {
		media, err := s.multimediaRepo.ByUUID(ctx, strings.TrimSpace(*req.TargetAudienceExcelFileUUID))
		if err != nil {
			return err
		}
		if media == nil {
			return ErrCampaignTargetAudienceExcelMediaNotFound
		}
	}
	if req.LineNumber != nil && *req.LineNumber == "" {
		return ErrCampaignLineNumberRequired
	}
	if req.Budget != nil {
		if *req.Budget <= 0 {
			return ErrCampaignBudgetRequired
		}
		if *req.Budget < minCampaignBudget || *req.Budget > maxCampaignBudget {
			return ErrCampaignBudgetOutOfRange
		}
	}
	if req.Sex != nil && *req.Sex == "" {
		return ErrCampaignSexRequired
	}
	if req.City != nil && len(req.City) == 0 {
		return ErrCampaignCityRequired
	}
	if req.AdLink != nil && *req.AdLink == "" {
		return ErrCampaignAdLinkRequired
	}
	if req.Category != nil && strings.TrimSpace(*req.Category) == "" {
		return ErrAgencyCategoryJobRequired
	}
	if req.Job != nil && strings.TrimSpace(*req.Job) == "" {
		return ErrAgencyCategoryJobRequired
	}
	if req.Platform != nil && strings.TrimSpace(*req.Platform) == "" {
		return ErrCampaignPlatformRequired
	}

	// Validate schedule time must be at least 10 minutes in the future
	scheduleTime := req.ScheduleAt
	if scheduleTime != nil && !scheduleTime.IsZero() {
		if scheduleTime.Before(utils.UTCNow().Add(10 * time.Minute)) {
			return ErrScheduleTimeTooSoon
		}
		if !isScheduleWithinTehranWindow(*scheduleTime) {
			return ErrScheduleTimeOutsideWindow
		}
	}

	if len(req.City) > 0 {
		if slices.Contains(req.City, "") {
			return ErrCampaignCityRequired
		}
	}

	if !usingTargetAudienceFromExcelFile && len(req.Level2s) > 0 {
		if slices.Contains(req.Level2s, "") {
			return ErrCampaignLevel2sRequired
		}
	}

	if !usingTargetAudienceFromExcelFile && len(req.Level3s) > 0 {
		if slices.Contains(req.Level3s, "") {
			return ErrCampaignLevel3sRequired
		}
	}

	if !usingTargetAudienceFromExcelFile && len(req.Tags) > 0 {
		if slices.Contains(req.Tags, "") {
			return ErrCampaignTagsRequired
		}
	}
	if normalizedGrades, err := sanitizeAudienceGrades(req.AudienceGrades); err != nil {
		return err
	} else {
		req.AudienceGrades = normalizedGrades
	}

	if _, err := sanitizeShortLinkDomain(req.ShortLinkDomain); err != nil {
		return err
	}

	_, err := sanitizeCampaignPlatform(req.Platform)
	if err != nil {
		return err
	}

	return nil
}

func isScheduleWithinTehranWindow(t time.Time) bool {
	loc := getTehranLocation()
	tt := t.In(time.UTC).In(loc)
	minutes := tt.Hour()*60 + tt.Minute()
	return minutes >= 8*60 && minutes <= 21*60
}

func getTehranLocation() *time.Location {
	if tehranLoc == nil || tehranLoc.String() != "Asia/Tehran" {
		if loaded, err := time.LoadLocation("Asia/Tehran"); err == nil {
			tehranLoc = loaded
		} else {
			tehranLoc = time.FixedZone("Asia/Tehran", 3*3600+1800)
		}
	}
	return tehranLoc
}

// createCampaign creates the campaign in the database
func (s *CampaignFlowImpl) createCampaign(ctx context.Context, req *dto.CreateCampaignRequest, customer *models.Customer) (*models.Campaign, error) {
	shortLinkDomain, err := sanitizeShortLinkDomain(req.ShortLinkDomain)
	if err != nil {
		return nil, err
	}
	platform, err := sanitizeCampaignPlatform(req.Platform)
	if err != nil {
		return nil, err
	}

	// Build campaign spec
	spec := models.CampaignSpec{}

	if req.Title != nil && *req.Title != "" {
		spec.Title = req.Title
	}
	if req.Level1 != nil && *req.Level1 != "" {
		spec.Level1 = req.Level1
	}
	if len(req.Level2s) > 0 {
		spec.Level2s = req.Level2s
	}
	if len(req.Level3s) > 0 {
		spec.Level3s = req.Level3s
	}
	if req.TargetAudienceExcelFileUUID != nil && strings.TrimSpace(*req.TargetAudienceExcelFileUUID) != "" {
		excelFileUUID := strings.TrimSpace(*req.TargetAudienceExcelFileUUID)
		spec.TargetAudienceExcelFileUUID = &excelFileUUID
	}
	if len(req.Tags) > 0 {
		spec.Tags = req.Tags
	}
	if req.Sex != nil && *req.Sex != "" {
		spec.Sex = req.Sex
	}
	if len(req.City) > 0 {
		spec.City = req.City
	}
	if req.AdLink != nil && *req.AdLink != "" {
		spec.AdLink = req.AdLink
	}
	if req.Content != nil && *req.Content != "" {
		spec.Content = req.Content
	}
	spec.ShortLinkDomain = shortLinkDomain
	if req.Category != nil && *req.Category != "" {
		spec.Category = req.Category
	}
	if req.Job != nil && *req.Job != "" {
		spec.Job = req.Job
	}
	if req.ScheduleAt != nil {
		spec.ScheduleAt = req.ScheduleAt
	}
	if req.LineNumber != nil && *req.LineNumber != "" {
		spec.LineNumber = req.LineNumber
	}
	if req.MediaUUID != nil {
		spec.MediaUUID = req.MediaUUID
	}
	if req.PlatformSettingsID != nil && *req.PlatformSettingsID != 0 {
		spec.PlatformSettingsID = req.PlatformSettingsID
	}
	spec.Platform = platform
	if req.Budget != nil && *req.Budget != 0 {
		spec.Budget = req.Budget
	}
	spec.AudienceGrades = campaignAudienceGradesOrDefault(req.AudienceGrades)

	uid := uuid.New()

	// Save to database
	err = s.campaignRepo.Save(ctx, &models.Campaign{
		UUID:       uid,
		CustomerID: customer.ID,
		Status:     models.CampaignStatusInitiated,
		Spec:       spec,
		BundleID:   req.BundleID,
		Phase:      campaignPhaseOrDefault(req.Phase),
	})
	if err != nil {
		return nil, err
	}

	// Get the created campaign with ID
	c, err := s.campaignRepo.ByUUID(ctx, uid.String())
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (s *CampaignFlowImpl) ensureCampaignBundleAndPhase(ctx context.Context, customerID uint, bundleID *uint, phase *string, required bool) error {
	if required && (bundleID == nil || *bundleID == 0) {
		return ErrBundleNotFound
	}
	if err := validateCampaignPhaseInput(phase, required); err != nil {
		return err
	}
	if bundleID == nil || *bundleID == 0 {
		return nil
	}

	bundle, err := s.bundleRepo.ByID(ctx, *bundleID)
	if err != nil {
		return err
	}
	if bundle == nil {
		return ErrBundleNotFound
	}
	if bundle.CustomerID != customerID {
		return ErrBundleAccessDenied
	}

	return nil
}

func (s *CampaignFlowImpl) ensureCreateCampaignRefs(
	ctx context.Context,
	customerID uint,
	bundleID *uint,
	phase *string,
	lineNumber *string,
	level3s []string,
	platform string,
	mediaUUID *uuid.UUID,
	targetAudienceExcelFileUUID *string,
	platformSettingsID *uint,
) error {
	if err := s.ensureCampaignBundleAndPhase(ctx, customerID, bundleID, phase, true); err != nil {
		return err
	}

	if platform != models.CampaignPlatformSMS && lineNumber != nil {
		return ErrCampaignLineNumberNotApplicable
	}

	if platform == models.CampaignPlatformSMS && platformSettingsID != nil && *platformSettingsID != 0 {
		return ErrCampaignPlatformSettingNotApplicable
	}

	// if platform == models.CampaignPlatformSMS && lineNumber == nil {
	// 	return ErrCampaignLineNumberRequired
	// }

	if lineNumber != nil {
		_, err := s.fetchLineNumberPriceFactor(ctx, lineNumber)
		if err != nil {
			return err
		}
	}

	usingTargetAudienceFromExcelFile := targetAudienceExcelFileUUID != nil && strings.TrimSpace(*targetAudienceExcelFileUUID) != ""
	if len(level3s) > 0 && !usingTargetAudienceFromExcelFile {
		_, err := s.fetchSegmentPriceFactor(ctx, level3s, platform)
		if err != nil {
			return err
		}
	}

	if mediaUUID != nil {
		mediaRows, err := s.multimediaRepo.ByFilter(ctx, models.MultimediaAssetFilter{
			UUID:       mediaUUID,
			CustomerID: &customerID,
		}, "", 1, 0)
		if err != nil {
			return err
		}
		if len(mediaRows) == 0 {
			return ErrCampaignMediaNotFound
		}
	}

	if usingTargetAudienceFromExcelFile {
		count, err := s.CountTargetAudienceFromExcelFile(ctx, customerID, strings.TrimSpace(*targetAudienceExcelFileUUID))
		if err != nil {
			switch {
			case errors.Is(err, os.ErrNotExist):
				return ErrCampaignTargetAudienceExcelMediaNotFound
			case errors.Is(err, ErrCampaignTargetAudienceExcelFileInvalid):
				return ErrCampaignTargetAudienceExcelFileInvalid
			default:
				return err
			}
		}
		if count == 0 {
			return ErrCampaignTargetAudienceExcelFileInvalid
		}
	}

	// if platform != models.CampaignPlatformSMS && (platformSettingsID == nil || *platformSettingsID == 0) {
	// 	return ErrCampaignPlatformSettingRequired
	// }

	if platformSettingsID != nil && *platformSettingsID != 0 {
		platformFilter := platform
		settingsRows, err := s.platformSettingsRepo.ByFilter(ctx, models.PlatformSettingsFilter{
			ID:         platformSettingsID,
			CustomerID: &customerID,
			Platform:   &platformFilter,
			Status:     utils.ToPtr(models.PlatformSettingsStatusActive),
		}, "", 1, 0)
		if err != nil {
			return err
		}
		if len(settingsRows) == 0 {
			return ErrCampaignPlatformSettingNotFound
		}
	}

	return nil
}

func (s *CampaignFlowImpl) ensureUpdateCampaignRefs(
	ctx context.Context,
	customerID uint,
	bundleID *uint,
	phase *string,
	lineNumber *string,
	level3s []string,
	platform string,
	mediaUUID *uuid.UUID,
	targetAudienceExcelFileUUID *string,
	platformSettingsID *uint,
	finalize bool,
) error {
	if err := s.ensureCampaignBundleAndPhase(ctx, customerID, bundleID, phase, false); err != nil {
		return err
	}

	usingTargetAudienceExcelFile := targetAudienceExcelFileUUID != nil && strings.TrimSpace(*targetAudienceExcelFileUUID) != ""

	if finalize {
		// TODO: Nullify
		// if platform != models.CampaignPlatformSMS && lineNumber != nil {
		// 	return ErrCampaignLineNumberNotApplicable
		// }

		// if platform == models.CampaignPlatformSMS && platformSettingsID != nil && *platformSettingsID != 0 {
		// 	return ErrCampaignPlatformSettingNotApplicable
		// }

		if platform == models.CampaignPlatformSMS && lineNumber == nil {
			return ErrCampaignLineNumberRequired
		}

		if platform != models.CampaignPlatformSMS && (platformSettingsID == nil || *platformSettingsID == 0) {
			return ErrCampaignPlatformSettingRequired
		}

		if platform != models.CampaignPlatformSMS && platformSettingsID != nil && *platformSettingsID != 0 {
			settingsRows, err := s.platformSettingsRepo.ByFilter(ctx, models.PlatformSettingsFilter{
				ID:         platformSettingsID,
				CustomerID: &customerID,
				Platform:   &platform,
				Status:     utils.ToPtr(models.PlatformSettingsStatusActive),
			}, "", 1, 0)
			if err != nil {
				return err
			}
			if len(settingsRows) == 0 {
				return ErrCampaignPlatformSettingNotFound
			}
		}
	}

	if lineNumber != nil {
		_, err := s.fetchLineNumberPriceFactor(ctx, lineNumber)
		if err != nil {
			return err
		}
	}

	if len(level3s) > 0 && !usingTargetAudienceExcelFile {
		_, err := s.fetchSegmentPriceFactor(ctx, level3s, platform)
		if err != nil {
			return err
		}
	}

	if mediaUUID != nil {
		mediaRows, err := s.multimediaRepo.ByFilter(ctx, models.MultimediaAssetFilter{
			UUID:       mediaUUID,
			CustomerID: &customerID,
		}, "", 1, 0)
		if err != nil {
			return err
		}
		if len(mediaRows) == 0 {
			return ErrCampaignMediaNotFound
		}
	}

	if usingTargetAudienceExcelFile {
		count, err := s.CountTargetAudienceFromExcelFile(ctx, customerID, strings.TrimSpace(*targetAudienceExcelFileUUID))
		if err != nil {
			switch {
			case errors.Is(err, os.ErrNotExist):
				return ErrCampaignTargetAudienceExcelMediaNotFound
			case errors.Is(err, ErrCampaignTargetAudienceExcelFileInvalid):
				return ErrCampaignTargetAudienceExcelFileInvalid
			default:
				return err
			}
		}
		if count == 0 {
			return ErrCampaignTargetAudienceExcelFileInvalid
		}
	}

	return nil
}

// validateUpdateCampaignRequest validates the campaign update request
func (s *CampaignFlowImpl) validateUpdateCampaignRequest(req *dto.UpdateCampaignRequest) error {
	if req.UUID == "" {
		return ErrCampaignUUIDRequired
	}

	if req.CustomerID == 0 {
		return ErrCustomerNotFound
	}

	// At least one field should be provided for update
	hasUpdateFields := req.Title != nil || req.Level1 != nil || len(req.Level2s) > 0 || len(req.Level3s) > 0 ||
		req.BundleID != nil || req.Phase != nil ||
		req.TargetAudienceExcelFileUUID != nil || len(req.Tags) > 0 || req.AudienceGrades != nil || req.Sex != nil || len(req.City) > 0 ||
		req.AdLink != nil || req.Content != nil ||
		req.ScheduleAt != nil || req.LineNumber != nil || req.Budget != nil || req.ShortLinkDomain != nil ||
		req.Category != nil || req.Job != nil ||
		req.MediaUUID != nil || req.PlatformSettingsID != nil || req.Platform != nil

	if !hasUpdateFields {
		return ErrCampaignUpdateRequired
	}

	if req.Budget != nil {
		if *req.Budget <= 0 {
			return ErrCampaignBudgetRequired
		}
		if *req.Budget < minCampaignBudget || *req.Budget > maxCampaignBudget {
			return ErrCampaignBudgetOutOfRange
		}
	}
	if req.BundleID != nil && *req.BundleID == 0 {
		return ErrBundleNotFound
	}
	if normalizedGrades, err := sanitizeAudienceGrades(req.AudienceGrades); err != nil {
		return err
	} else {
		req.AudienceGrades = normalizedGrades
	}
	if err := validateCampaignPhaseInput(req.Phase, false); err != nil {
		return err
	}

	// if req.ScheduleAt != nil && !req.ScheduleAt.IsZero() {
	// 	if !isScheduleWithinTehranWindow(*req.ScheduleAt) {
	// 		return ErrScheduleTimeOutsideWindow
	// 	}
	// }

	return nil
}

// validateCalculateCampaignCapacityRequest validates the request
func (s *CampaignFlowImpl) validateCalculateCampaignCapacityRequest(req *dto.CalculateCampaignCapacityRequest) error {
	if req.CampaignID == 0 {
		return ErrCampaignNotFound
	}
	if req.CustomerID == 0 {
		return ErrCustomerNotFound
	}

	return nil
}

func (s *CampaignFlowImpl) canFinalizeCampaign(ctx context.Context, campaign models.Campaign, customer models.Customer) error {
	usingTargetAudienceExcelFile := campaign.Spec.TargetAudienceExcelFileUUID != nil && strings.TrimSpace(*campaign.Spec.TargetAudienceExcelFileUUID) != ""

	if campaign.Spec.Title == nil || *campaign.Spec.Title == "" {
		return ErrCampaignTitleRequired
	}
	if !usingTargetAudienceExcelFile {
		if campaign.Spec.Level1 == nil || *campaign.Spec.Level1 == "" {
			return ErrCampaignLevel1Required
		}
		if campaign.Spec.Level2s == nil {
			return ErrCampaignLevel2sRequired
		}
		if len(campaign.Spec.Level2s) == 0 {
			return ErrCampaignLevel2sRequired
		}
		if campaign.Spec.Level3s == nil {
			return ErrCampaignLevel3sRequired
		}
		if len(campaign.Spec.Level3s) == 0 {
			return ErrCampaignLevel3sRequired
		}
		if campaign.Spec.Tags == nil {
			return ErrCampaignTagsRequired
		}
		if len(campaign.Spec.Tags) == 0 {
			return ErrCampaignTagsRequired
		}
	}
	if campaign.Spec.Content == nil || *campaign.Spec.Content == "" {
		return ErrCampaignContentRequired
	}
	if campaign.Spec.ScheduleAt == nil || campaign.Spec.ScheduleAt.IsZero() {
		campaign.Spec.ScheduleAt = utils.ToPtr(utils.UTCNow().Add(20 * time.Minute))
		// return ErrScheduleTimeNotPresent
	}
	if campaign.Spec.ScheduleAt.Before(utils.UTCNow().Add(10 * time.Minute)) {
		return ErrScheduleTimeTooSoon
	}
	if !isScheduleWithinTehranWindow(*campaign.Spec.ScheduleAt) {
		return ErrScheduleTimeOutsideWindow
	}
	if campaign.Spec.Platform == models.CampaignPlatformSMS && (campaign.Spec.LineNumber == nil || *campaign.Spec.LineNumber == "") {
		return ErrCampaignLineNumberRequired
	}
	if campaign.Spec.Budget == nil || *campaign.Spec.Budget <= 0 {
		return ErrCampaignBudgetRequired
	}
	if _, err := sanitizeShortLinkDomain(campaign.Spec.ShortLinkDomain); err != nil {
		return err
	}
	if _, _, err := sanitizeCategoryAndJob(customer.AccountType.TypeName, campaign.Spec.Category, campaign.Spec.Job, true); err != nil {
		return err
	}
	if _, err := sanitizeCampaignPlatform(utils.ToPtr(campaign.Spec.Platform)); err != nil {
		return err
	}
	if err := s.ensureUpdateCampaignRefs(
		ctx,
		campaign.CustomerID,
		campaign.BundleID,
		campaignPhasePtr(campaign.Phase),
		campaign.Spec.LineNumber,
		campaign.Spec.Level3s,
		campaign.Spec.Platform,
		campaign.Spec.MediaUUID,
		campaign.Spec.TargetAudienceExcelFileUUID,
		campaign.Spec.PlatformSettingsID,
		true,
	); err != nil {
		return err
	}

	return nil
}

// updateCampaign updates the campaign in the database
func (s *CampaignFlowImpl) updateCampaign(ctx context.Context, req *dto.UpdateCampaignRequest, existingCampaign *models.Campaign) error {
	// Update campaign spec with new values
	spec := existingCampaign.Spec

	if req.Title != nil && *req.Title != "" {
		spec.Title = req.Title
	}
	if req.Level1 != nil && *req.Level1 != "" {
		spec.Level1 = req.Level1
	}
	if len(req.Level2s) > 0 {
		spec.Level2s = req.Level2s
	}
	if len(req.Level3s) > 0 {
		spec.Level3s = req.Level3s
	}
	if req.TargetAudienceExcelFileUUID != nil {
		excelFileUUID := strings.TrimSpace(*req.TargetAudienceExcelFileUUID)
		if excelFileUUID != "" {
			spec.TargetAudienceExcelFileUUID = &excelFileUUID
		} else {
			spec.TargetAudienceExcelFileUUID = nil
		}
	} else {
		spec.TargetAudienceExcelFileUUID = nil
	}
	if len(req.Tags) > 0 {
		spec.Tags = req.Tags
	}
	if req.Sex != nil && *req.Sex != "" {
		spec.Sex = req.Sex
	}
	if len(req.City) > 0 {
		spec.City = req.City
	}
	if req.AdLink != nil && *req.AdLink != "" {
		spec.AdLink = req.AdLink
	} else {
		spec.AdLink = nil
	}
	if req.Content != nil && *req.Content != "" {
		spec.Content = req.Content
	}
	if req.Category != nil && *req.Category != "" {
		spec.Category = req.Category
	}
	if req.Job != nil && *req.Job != "" {
		spec.Job = req.Job
	}
	if req.ShortLinkDomain != nil {
		spec.ShortLinkDomain = req.ShortLinkDomain
	} else {
		spec.ShortLinkDomain = nil
	}
	if req.ScheduleAt != nil {
		spec.ScheduleAt = req.ScheduleAt
	} else {
		spec.ScheduleAt = nil
	}
	if req.LineNumber != nil && *req.LineNumber != "" {
		spec.LineNumber = req.LineNumber
	} else {
		// TODO: Nullify?
	}
	if req.MediaUUID != nil {
		spec.MediaUUID = req.MediaUUID
	}
	if req.PlatformSettingsID != nil && *req.PlatformSettingsID != 0 {
		spec.PlatformSettingsID = req.PlatformSettingsID
	} else {
		// TODO: Nullify?
	}
	if req.Platform != nil {
		spec.Platform = *req.Platform
	}
	if req.Budget != nil && *req.Budget != 0 {
		spec.Budget = req.Budget
	}
	if req.AudienceGrades != nil {
		spec.AudienceGrades = campaignAudienceGradesOrDefault(req.AudienceGrades)
	}
	ensureCampaignSpecDefaults(&spec)

	// Update the campaign spec
	existingCampaign.Spec = spec
	if req.BundleID != nil {
		existingCampaign.BundleID = req.BundleID
		existingCampaign.Bundle = nil // Clear cached bundle to force reload with new ID
	}
	if req.Phase != nil {
		existingCampaign.Phase = campaignPhaseOrDefault(req.Phase)
	}
	existingCampaign.Status = models.CampaignStatusInProgress
	existingCampaign.UpdatedAt = utils.ToPtr(utils.UTCNow())

	// Save to database
	err := s.campaignRepo.Update(ctx, *existingCampaign)
	if err != nil {
		return err
	}

	return nil
}

func (s *CampaignFlowImpl) expireCustomerCampaigns(ctx context.Context, customerID uint) error {
	// NOTE: Idempotency
	cutoff := utils.UTCNow().Add(-6 * time.Hour)

	st := models.CampaignStatusWaitingForApproval
	rows, err := s.campaignRepo.ByFilter(ctx, models.CampaignFilter{
		CustomerID: &customerID,
		Status:     &st,
	}, "", 0, 0)
	if err != nil {
		return err
	}

	for _, c := range rows {
		if c.Spec.ScheduleAt == nil || c.Spec.ScheduleAt.IsZero() {
			continue
		}
		if !c.Spec.ScheduleAt.Before(cutoff) {
			continue
		}

		if err := repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
			campaign, err := s.campaignRepo.ByID(txCtx, c.ID)
			if err != nil {
				return err
			}
			if campaign == nil {
				return ErrCampaignNotFound
			}
			if campaign.CustomerID != customerID {
				return ErrCampaignAccessDenied
			}
			if campaign.Status != models.CampaignStatusWaitingForApproval {
				return nil
			}
			if campaign.Spec.ScheduleAt == nil || campaign.Spec.ScheduleAt.IsZero() || !campaign.Spec.ScheduleAt.Before(cutoff) {
				return nil
			}

			customer, err := getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
			if err != nil {
				return err
			}

			wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
			if err != nil {
				return err
			}
			latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
			if err != nil {
				return err
			}

			freezeTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
				CustomerID: &campaign.CustomerID,
				CampaignID: &campaign.ID,
				Source:     utils.ToPtr("campaign_update"),
				Operation:  utils.ToPtr("reserve_budget"),
				Type:       utils.ToPtr(models.TransactionTypeFreeze),
				Status:     utils.ToPtr(models.TransactionStatusCompleted),
			}, "id DESC", 0, 0)
			if err != nil {
				return err
			}
			if len(freezeTxs) == 0 {
				return ErrFreezeTransactionNotFound
			}
			if len(freezeTxs) > 1 {
				return ErrMultipleFreezeTransactionsFound
			}
			freezeTx := freezeTxs[0]

			amount := freezeTx.Amount
			if latestBalance.FrozenBalance < amount {
				return ErrInsufficientFunds
			}

			meta := map[string]any{
				"source":      "campaign_expire",
				"operation":   "expire_campaign_refund_frozen",
				"campaign_id": campaign.ID,
				"comment":     "campaign_auto_expired_due_to_schedule_time",
			}
			metaBytes, _ := json.Marshal(meta)

			// TODO:
			// newFrozen := latestBalance.FrozenBalance - amount
			// // Restore free/credit split exactly as it was taken during the freeze.
			// freeRefund, creditRefund := computeFreezeRefundSplit(freezeTx, amount)
			// newFree := latestBalance.FreeBalance + freeRefund
			// newCredit := latestBalance.CreditBalance + creditRefund

			// newSnap := &models.BalanceSnapshot{
			// 	UUID:               uuid.New(),
			// 	CorrelationID:      freezeTx.CorrelationID,
			// 	WalletID:           wallet.ID,
			// 	CustomerID:         customer.ID,
			// 	FreeBalance:        newFree,
			// 	FrozenBalance:      newFrozen,
			// 	LockedBalance:      latestBalance.LockedBalance,
			// 	CreditBalance:      newCredit,
			// 	SpentOnCampaign:    latestBalance.SpentOnCampaign,
			// 	AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			// 	TotalBalance:       newFree + newFrozen + latestBalance.LockedBalance + newCredit + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
			// 	Reason:             "campaign_expired_budget_refund",
			// 	Description:        fmt.Sprintf("Refund reserved budget for expired campaign %d", campaign.ID),
			// 	Metadata:           metaBytes,
			// }

			newFrozen := latestBalance.FrozenBalance - amount
			newCredit := latestBalance.CreditBalance + amount

			newSnap := &models.BalanceSnapshot{
				UUID:               uuid.New(),
				CorrelationID:      freezeTx.CorrelationID,
				WalletID:           wallet.ID,
				CustomerID:         customer.ID,
				FreeBalance:        latestBalance.FreeBalance,
				FrozenBalance:      newFrozen,
				LockedBalance:      latestBalance.LockedBalance,
				CreditBalance:      newCredit,
				SpentOnCampaign:    latestBalance.SpentOnCampaign,
				AgencyShareWithTax: latestBalance.AgencyShareWithTax,
				TotalBalance:       latestBalance.FreeBalance + newFrozen + latestBalance.LockedBalance + newCredit + latestBalance.SpentOnCampaign + latestBalance.AgencyShareWithTax,
				Reason:             "campaign_expired_budget_refund",
				Description:        fmt.Sprintf("Refund reserved budget for expired campaign %d", campaign.ID),
				Metadata:           metaBytes,
			}
			if err := s.balanceSnapshotRepo.Save(txCtx, newSnap); err != nil {
				return err
			}

			beforeMap, err := latestBalance.GetBalanceMap()
			if err != nil {
				return err
			}
			afterMap, err := newSnap.GetBalanceMap()
			if err != nil {
				return err
			}

			refundTx := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: freezeTx.CorrelationID,
				Type:          models.TransactionTypeRefund,
				Status:        models.TransactionStatusCompleted,
				Amount:        amount,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Refund reserved budget for expired campaign %d", campaign.ID),
				Metadata:      metaBytes,
			}
			if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
				return err
			}

			if err := s.campaignRepo.UpdateStatus(txCtx, campaign.ID, models.CampaignStatusExpired); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// reconcileUndeliveredCampaignRefunds runs a best-effort reconciliation pass to refund
// executed campaigns that under-delivered relative to their intended audience.
func (s *CampaignFlowImpl) reconcileUndeliveredCampaignRefunds(ctx context.Context, customerID uint) error {
	// NOTE: Idempotency

	if customerID == 0 {
		return nil
	}
	var auditCustomer *models.Customer
	if customer, err := getCustomer(ctx, s.customerRepo, customerID); err == nil {
		auditCustomer = &customer
	}

	status := models.CampaignStatusExecuted
	cutoff := utils.UTCNow().Add(-undeliveredRefundDelay)
	rows, err := s.campaignRepo.ByFilter(ctx, models.CampaignFilter{
		CustomerID:     &customerID,
		Status:         &status,
		ScheduleBefore: &cutoff,
	}, "id DESC", 0, 0)
	if err != nil {
		return err
	}

	for _, c := range rows {
		if c == nil {
			continue
		}
		if hasProcessedUndeliveredRefund(c.Statistics) {
			continue
		}
		if hasUndeliveredRefundError(c.Statistics) {
			continue
		}

		err = repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
			campaign, err := s.campaignRepo.ByID(txCtx, c.ID)
			if err != nil {
				return err
			}
			if campaign == nil {
				return nil
			}
			if campaign.Status != models.CampaignStatusExecuted {
				return nil
			}
			if campaign.Spec.ScheduleAt == nil || campaign.Spec.ScheduleAt.IsZero() {
				return nil
			}
			if campaign.Spec.ScheduleAt.After(utils.UTCNow().Add(-undeliveredRefundDelay)) {
				return nil
			}
			if campaign.NumAudience == nil || *campaign.NumAudience == 0 {
				log.Printf("reconcileUndeliveredCampaignRefunds: campaign %d has nil or zero num_audience, skipping refund", campaign.ID)
				return nil
			}

			// Serialize refund reconciliation per campaign to prevent duplicate refunds
			// under concurrent list/get requests.
			if tx, ok := txCtx.Value(repository.TxContextKey).(*gorm.DB); ok && tx != nil {
				if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(campaign.ID)).Error; err != nil {
					return err
				}
			}

			if hasProcessedUndeliveredRefund(campaign.Statistics) {
				return nil
			}
			if hasUndeliveredRefundError(campaign.Statistics) {
				return nil
			}

			existing, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
				CustomerID: &campaign.CustomerID,
				CampaignID: &campaign.ID,
				Source:     utils.ToPtr("campaign_partial_refund"),
				Operation:  utils.ToPtr("partial_undelivered_messages_refund"),
				Type:       utils.ToPtr(models.TransactionTypeRefund),
				Status:     utils.ToPtr(models.TransactionStatusCompleted),
			}, "id DESC", 1, 0)
			if err != nil {
				return err
			}
			if len(existing) > 0 {
				return nil
			}

			aggregatedTotalSent, ok := parseAggregatedTotalSent(campaign.Statistics)
			if !ok {
				return nil
			}
			if aggregatedTotalSent >= *campaign.NumAudience {
				return nil
			}

			missing := *campaign.NumAudience - aggregatedTotalSent
			if missing == 0 {
				return nil
			}

			var costPerMessage uint64
			if costPerMessage, ok = s.resolveCampaignCostPerMessageFromMetadata(txCtx, campaign); !ok {
				log.Printf("reconcileUndeliveredCampaignRefunds: cannot resolve cost per message for campaign %d, skipping refund reconciliation", campaign.ID)
				return nil
			}

			if costPerMessage == 0 {
				return nil
			}
			if missing > math.MaxUint64/costPerMessage {
				return fmt.Errorf("refund amount overflow for campaign=%d", campaign.ID)
			}
			refundAmount := missing * costPerMessage
			if refundAmount == 0 {
				return nil
			}

			debitTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
				CustomerID: &campaign.CustomerID,
				CampaignID: &campaign.ID,
				Source:     utils.ToPtr("admin_campaign_approve"),
				Operation:  utils.ToPtr("approve_campaign_budget_consume"),
				Type:       utils.ToPtr(models.TransactionTypeFee),
				Status:     utils.ToPtr(models.TransactionStatusCompleted),
			}, "id DESC", 1, 0)
			if err != nil {
				return err
			}
			if len(debitTxs) == 0 {
				return ErrCampaignDebitTransactionNotFound
			}
			debitTx := debitTxs[0]

			if debitTx.Amount < refundAmount {
				return fmt.Errorf("refund amount %d exceeds campaign debit amount %d for campaign=%d", refundAmount, debitTx.Amount, campaign.ID)
			}

			customer, err := getCustomer(txCtx, s.customerRepo, campaign.CustomerID)
			if err != nil {
				return err
			}
			wallet, err := getWallet(txCtx, s.walletRepo, campaign.CustomerID)
			if err != nil {
				return err
			}
			latestBalance, err := getLatestBalanceSnapshot(txCtx, s.walletRepo, wallet.ID)
			if err != nil {
				return err
			}
			if latestBalance.SpentOnCampaign < refundAmount {
				return ErrInsufficientFunds
			}

			// TODO:
			// // Recover the original FreeBalance/CreditBalance split by looking up the
			// // freeze transaction that shares the same CorrelationID as the debit tx.
			// debitCorrID := debitTx.CorrelationID
			// origFreezeTxs, err := s.transactionRepo.ByFilter(txCtx, models.TransactionFilter{
			// 	CorrelationID: &debitCorrID,
			// 	CustomerID:    &campaign.CustomerID,
			// 	Type:          utils.ToPtr(models.TransactionTypeFreeze),
			// 	Status:        utils.ToPtr(models.TransactionStatusCompleted),
			// }, "id ASC", 1, 0)
			// if err != nil {
			// 	return err
			// }
			// var freeRefund, creditRefund uint64
			// if len(origFreezeTxs) > 0 {
			// 	freeRefund, creditRefund = computeFreezeRefundSplit(origFreezeTxs[0], refundAmount)
			// } else {
			// 	// No freeze tx found; conservatively return all to free balance.
			// 	freeRefund = refundAmount
			// }

			meta := map[string]any{
				"source":                "campaign_partial_refund",
				"operation":             "partial_undelivered_messages_refund",
				"campaign_id":           campaign.ID,
				"scheduled_at":          campaign.Spec.ScheduleAt.UTC().Format(time.RFC3339),
				"num_audience":          *campaign.NumAudience,
				"aggregated_total_sent": aggregatedTotalSent,
				"missing_messages":      missing,
				"cost_per_message":      costPerMessage,
				"refund_amount":         refundAmount,
			}
			metaBytes, _ := json.Marshal(meta)

			// newFree := latestBalance.FreeBalance + freeRefund
			// newCredit := latestBalance.CreditBalance + creditRefund
			// newSpentOnCampaign := latestBalance.SpentOnCampaign - refundAmount

			// newSnap := &models.BalanceSnapshot{
			// 	UUID:               uuid.New(),
			// 	CorrelationID:      debitTx.CorrelationID,
			// 	WalletID:           wallet.ID,
			// 	CustomerID:         customer.ID,
			// 	FreeBalance:        newFree,
			// 	FrozenBalance:      latestBalance.FrozenBalance,
			// 	LockedBalance:      latestBalance.LockedBalance,
			// 	CreditBalance:      newCredit,
			// 	SpentOnCampaign:    newSpentOnCampaign,
			// 	AgencyShareWithTax: latestBalance.AgencyShareWithTax,
			// 	TotalBalance:       newFree + latestBalance.FrozenBalance + latestBalance.LockedBalance + newCredit + newSpentOnCampaign + latestBalance.AgencyShareWithTax,
			// 	Reason:             "campaign_partial_refund_for_undelivered_messages",
			// 	Description:        fmt.Sprintf("Refund undelivered messages for campaign %d", campaign.ID),
			// 	Metadata:           metaBytes,
			// }
			newCredit := latestBalance.CreditBalance + refundAmount
			newSpentOnCampaign := latestBalance.SpentOnCampaign - refundAmount

			newSnap := &models.BalanceSnapshot{
				UUID:               uuid.New(),
				CorrelationID:      debitTx.CorrelationID,
				WalletID:           wallet.ID,
				CustomerID:         customer.ID,
				FreeBalance:        latestBalance.FreeBalance,
				FrozenBalance:      latestBalance.FrozenBalance,
				LockedBalance:      latestBalance.LockedBalance,
				CreditBalance:      newCredit,
				SpentOnCampaign:    newSpentOnCampaign,
				AgencyShareWithTax: latestBalance.AgencyShareWithTax,
				TotalBalance:       latestBalance.FreeBalance + latestBalance.FrozenBalance + latestBalance.LockedBalance + newCredit + newSpentOnCampaign + latestBalance.AgencyShareWithTax,
				Reason:             "campaign_partial_refund_for_undelivered_messages",
				Description:        fmt.Sprintf("Refund undelivered messages for campaign %d", campaign.ID),
				Metadata:           metaBytes,
			}
			if err := s.balanceSnapshotRepo.Save(txCtx, newSnap); err != nil {
				return err
			}

			beforeMap, err := latestBalance.GetBalanceMap()
			if err != nil {
				return err
			}
			afterMap, err := newSnap.GetBalanceMap()
			if err != nil {
				return err
			}

			refundTx := &models.Transaction{
				UUID:          uuid.New(),
				CorrelationID: debitTx.CorrelationID,
				Type:          models.TransactionTypeRefund,
				Status:        models.TransactionStatusCompleted,
				Amount:        refundAmount,
				Currency:      utils.TomanCurrency,
				WalletID:      wallet.ID,
				CustomerID:    customer.ID,
				BalanceBefore: beforeMap,
				BalanceAfter:  afterMap,
				Description:   fmt.Sprintf("Partial refund for undelivered messages in campaign %d", campaign.ID),
				Metadata:      metaBytes,
			}
			if err := s.transactionRepo.Save(txCtx, refundTx); err != nil {
				return err
			}

			statsMap := map[string]any{}
			if len(campaign.Statistics) > 0 {
				_ = json.Unmarshal(campaign.Statistics, &statsMap)
			}
			statsMap["undeliveredRefundProcessed"] = true
			statsMap["undeliveredRefundProcessedAt"] = utils.UTCNow().Format(time.RFC3339)
			statsMap["undeliveredRefundAmount"] = refundAmount
			statsMap["undeliveredRefundMissingMessages"] = missing
			statsMap["undeliveredRefundCostPerMessage"] = costPerMessage
			statsMap["undeliveredRefundAggregatedTotalSent"] = aggregatedTotalSent
			statsBytes, _ := json.Marshal(statsMap)

			campaign.Statistics = statsBytes
			campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
			if err := s.campaignRepo.Update(txCtx, *campaign); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			errMsg := fmt.Sprintf("Undelivered refund reconciliation failed for campaign %d: %v", c.ID, err)
			if auditCustomer != nil {
				_ = s.createAuditLog(ctx, auditCustomer, models.AuditActionCampaignRefundReconcileFailed, errMsg, false, &errMsg, nil)
			}
			log.Printf("%s", errMsg)
			// Mark the campaign so it is never retried again. The refund transaction
			// was rolled back, so we persist the error flag in a separate (non-transactional) update.
			if markErr := s.markCampaignRefundError(ctx, c.ID, err); markErr != nil {
				log.Printf("reconcileUndeliveredCampaignRefunds: failed to mark refund error for campaign %d: %v", c.ID, markErr)
			}
			// Best-effort reconciliation: do not block listing/getting campaigns.
			continue
		}
	}

	return nil
}

// markCampaignRefundError writes an error flag into the campaign Statistics so
// reconcileUndeliveredCampaignRefunds skips it on future invocations.
// It runs outside any transaction because the failed refund transaction was already rolled back.
func (s *CampaignFlowImpl) markCampaignRefundError(ctx context.Context, campaignID uint, refundErr error) error {
	campaign, err := s.campaignRepo.ByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return nil
	}

	statsMap := map[string]any{}
	if len(campaign.Statistics) > 0 {
		_ = json.Unmarshal(campaign.Statistics, &statsMap)
	}
	statsMap["undeliveredRefundError"] = true
	statsMap["undeliveredRefundErrorAt"] = utils.UTCNow().Format(time.RFC3339)
	statsMap["undeliveredRefundErrorMsg"] = refundErr.Error()
	statsBytes, _ := json.Marshal(statsMap)

	campaign.Statistics = statsBytes
	campaign.UpdatedAt = utils.ToPtr(utils.UTCNow())
	return s.campaignRepo.Update(ctx, *campaign)
}

func (s *CampaignFlowImpl) tryAcquireFlowLock(ctx context.Context, suffix string, ttl time.Duration) bool {
	if s.rc == nil {
		return true
	}
	lockKey := redisKey(s.cacheConfig, "flow_lock:"+suffix)
	ok, err := s.rc.SetNX(ctx, lockKey, "1", ttl).Result()
	if err != nil {
		// Best-effort idempotency lock: continue when redis is unavailable.
		return true
	}
	return ok
}

func parseAggregatedTotalSent(stats json.RawMessage) (uint64, bool) {
	if len(stats) == 0 {
		return 0, false
	}
	var statsMap map[string]any
	if err := json.Unmarshal(stats, &statsMap); err != nil {
		return 0, false
	}
	v, ok := statsMap["aggregatedTotalSent"]
	if !ok {
		return 0, false
	}

	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil || f < 0 {
			return 0, false
		}
		return uint64(f), true
	default:
		return 0, false
	}
}

func hasProcessedUndeliveredRefund(stats json.RawMessage) bool {
	if len(stats) == 0 {
		return false
	}
	var statsMap map[string]any
	if err := json.Unmarshal(stats, &statsMap); err != nil {
		return false
	}
	raw, ok := statsMap["undeliveredRefundProcessed"]
	if !ok {
		return false
	}
	b, ok := raw.(bool)
	return ok && b
}

func hasUndeliveredRefundError(stats json.RawMessage) bool {
	if len(stats) == 0 {
		return false
	}
	var statsMap map[string]any
	if err := json.Unmarshal(stats, &statsMap); err != nil {
		return false
	}
	raw, ok := statsMap["undeliveredRefundError"]
	if !ok {
		return false
	}
	b, ok := raw.(bool)
	return ok && b
}

func (s *CampaignFlowImpl) resolveCampaignCostPerMessageFromMetadata(ctx context.Context, campaign *models.Campaign) (uint64, bool) {
	if campaign == nil {
		return 0, false
	}

	txs, err := s.transactionRepo.ByFilter(ctx, models.TransactionFilter{
		CustomerID: &campaign.CustomerID,
		CampaignID: &campaign.ID,
		Source:     utils.ToPtr("campaign_update"),
		Operation:  utils.ToPtr("reserve_budget"),
		Type:       utils.ToPtr(models.TransactionTypeFreeze),
		Status:     utils.ToPtr(models.TransactionStatusCompleted),
	}, "id DESC", 1, 0)
	if err != nil || len(txs) == 0 || len(txs[0].Metadata) == 0 {
		return 0, false
	}

	var meta map[string]any
	if err := json.Unmarshal(txs[0].Metadata, &meta); err != nil {
		return 0, false
	}

	basePrice, ok := parseMetadataUint64(meta["base_price"])
	if !ok || basePrice == 0 {
		pbp, err := s.platformBaseRepo.LatestByPlatform(ctx, campaign.Spec.Platform)
		if err != nil || pbp == nil || pbp.Price == 0 {
			return 0, false
		}
		basePrice = pbp.Price
	}
	pagePrice, ok := parseMetadataUint64(meta["page_price"])
	if !ok || pagePrice == 0 {
		pp, err := s.pagePriceRepo.LatestByPlatform(ctx, campaign.Spec.Platform)
		if err != nil || pp == nil || pp.Price == 0 {
			return 0, false
		}
		pagePrice = pp.Price
	}

	numPages, ok := parseMetadataUint64(meta["num_pages"])
	if !ok || numPages == 0 {
		numPages = s.calculateParts(
			campaign.Spec.Content,
			campaign.Spec.AdLink,
			campaign.Spec.ShortLinkDomain,
			campaign.Spec.Platform,
		)
	}

	f := parseMetadataFloat(meta["line_number_price_factor"])
	lineFactor := defaultLineNumberPriceFactor
	if f != nil && *f > 0 {
		lineFactor = *f
	}

	segmentFactor := defaultSegmentPriceFactor
	f = parseMetadataFloat(meta["segment_price_factor"])
	if f != nil && *f > 0 {
		segmentFactor = *f
	}

	pagePriceFloat := float64(pagePrice)
	numPagesFloat := float64(numPages)
	if campaign.Spec.Platform == models.CampaignPlatformSMS {
		return basePrice*uint64(lineFactor*numPagesFloat) + uint64(segmentFactor*pagePriceFloat), true
	}
	return basePrice*uint64(1*1) + uint64(segmentFactor*pagePriceFloat), true
}

func parseMetadataUint64(value any) (uint64, bool) {
	switch v := value.(type) {
	case float64:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case uint64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		if err != nil || f < 0 {
			return 0, false
		}
		return uint64(f), true
	default:
		return 0, false
	}
}

// computeFreezeRefundSplit determines how much of refundAmount should be restored
// to FreeBalance vs CreditBalance by reading the balance change recorded in the
// original freeze transaction.
//
// When a campaign budget is frozen, FreeBalance is drained first and any remainder
// is taken from CreditBalance. A refund must reverse that exact split so that a
// customer's real money (FreeBalance) is returned to FreeBalance and not silently
// converted to CreditBalance.
//
// The function applies "free-first" semantics: up to the amount originally taken
// from FreeBalance is restored there; any remaining refund goes to CreditBalance.
// If the balance snapshots cannot be parsed, the entire refundAmount is returned as
// freeRefund so no real money is lost.
func computeFreezeRefundSplit(freezeTx *models.Transaction, refundAmount uint64) (freeRefund, creditRefund uint64) {
	if refundAmount == 0 {
		return 0, 0
	}
	if len(freezeTx.BalanceBefore) == 0 || len(freezeTx.BalanceAfter) == 0 {
		// Cannot determine origin; conservatively return all to free balance.
		return refundAmount, 0
	}

	var balBefore, balAfter map[string]any
	if err := json.Unmarshal(freezeTx.BalanceBefore, &balBefore); err != nil {
		return refundAmount, 0
	}
	if err := json.Unmarshal(freezeTx.BalanceAfter, &balAfter); err != nil {
		return refundAmount, 0
	}

	freeBefore, _ := parseMetadataUint64(balBefore["free"])
	freeAfter, _ := parseMetadataUint64(balAfter["free"])

	// How much of the freeze came from FreeBalance.
	var freeReduced uint64
	if freeBefore > freeAfter {
		freeReduced = freeBefore - freeAfter
	}
	if freeReduced > refundAmount {
		freeReduced = refundAmount
	}

	return freeReduced, refundAmount - freeReduced
}

func (s *CampaignFlowImpl) GetPagePrices(ctx context.Context) (*dto.GetPagePricesResponse, error) {
	rows, err := s.pagePriceRepo.ListLatest(ctx)
	if err != nil {
		return nil, NewBusinessError("PAGE_PRICE_LIST_FAILED", "failed to list page prices", err)
	}

	items := make([]dto.PagePriceItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.PagePriceItem{
			Platform:  row.Platform,
			Price:     row.Price,
			CreatedAt: row.CreatedAt,
		})
	}
	slices.SortFunc(items, func(a, b dto.PagePriceItem) int {
		return strings.Compare(a.Platform, b.Platform)
	})

	return &dto.GetPagePricesResponse{
		Message: "Page prices retrieved successfully",
		Items:   items,
	}, nil
}

// ListAudienceSpec returns the current audience spec from cache or file
func (s *CampaignFlowImpl) ListAudienceSpec(ctx context.Context, platform *string) (*dto.ListAudienceSpecResponse, error) {
	// derive redis key and file path consistent with bot audience spec flow
	normalizedPlatform, err := normalizeAudienceSpecPlatformDefault(platform)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_PLATFORM_INVALID", "Invalid platform", err)
	}
	cacheKey := audienceSpecPlatformCacheKey(s.cacheConfig, normalizedPlatform)
	filePath := audienceSpecFilePath()
	hideTestLayer := s.shouldHideTestAudience(ctx)

	// try redis first
	if bs, err := s.rc.Get(ctx, cacheKey).Bytes(); err == nil && len(bs) > 0 {
		var out dto.AudienceSpec
		if err := json.Unmarshal(bs, &out); err == nil {
			if hideTestLayer {
				out = filterAudienceSpecLayer(out, "L1-test")
			}
			return &dto.ListAudienceSpecResponse{
				Message: "Audience spec retrieved from cache",
				Spec:    out,
			}, nil
		}
	}

	// Read existing JSON file (if any) for selected platform
	byPlatform, err := readAudienceSpecFileByPlatform(filePath)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_READ_FAILED", "Failed to read audience spec file", err)
	}
	current := byPlatform[normalizedPlatform]
	if current == nil {
		current = make(audienceSpecFile)
	}

	// Build DTO shape including level-2 metadata and only positive-availability items
	out := make(dto.AudienceSpec)
	for l1, l2map := range current {
		for l2k, node := range l2map {
			if node == nil {
				continue
			}
			// Collect items with AvailableAudience > 0
			items := make(map[string]dto.AudienceSpecItem)
			for l3k, leaf := range node.Items {
				if leaf.AvailableAudience > 0 {
					items[l3k] = dto.AudienceSpecItem{
						Tags:              leaf.Tags,
						AvailableAudience: leaf.AvailableAudience,
					}
				}
			}
			if len(items) == 0 && len(node.Metadata) == 0 {
				// Skip empty level2 without items and metadata
				continue
			}
			if _, ok := out[l1]; !ok {
				out[l1] = make(map[string]dto.AudienceSpecLevel2)
			}
			out[l1][l2k] = dto.AudienceSpecLevel2{
				Metadata: node.Metadata,
				Items:    items,
			}
		}
	}

	// Cache DTO JSON
	if bs, err := json.MarshalIndent(out, "", "  "); err == nil {
		_ = s.rc.Set(ctx, cacheKey, bs, 0).Err()
	}
	if hideTestLayer {
		out = filterAudienceSpecLayer(out, "L1-test")
	}

	return &dto.ListAudienceSpecResponse{
		Message: "Audience spec retrieved",
		Spec:    out,
	}, nil
}

func (s *CampaignFlowImpl) shouldHideTestAudience(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	customerID, ok := ctx.Value(utils.CustomerIDKey).(uint)
	if !ok || customerID == 0 {
		return true
	}
	customer, err := getCustomer(ctx, s.customerRepo, customerID)
	if err != nil {
		return true
	}
	return !s.adminConfig.HasMobile(customer.RepresentativeMobile)
}

func filterAudienceSpecLayer(spec dto.AudienceSpec, layer1 string) dto.AudienceSpec {
	if len(spec) == 0 {
		return spec
	}
	out := make(dto.AudienceSpec, len(spec))
	for l1, l2map := range spec {
		if l1 == layer1 {
			continue
		}
		out[l1] = l2map
	}
	return out
}

func (s *CampaignFlowImpl) GetApprovedRunningSummary(ctx context.Context, customerID uint) (*dto.CampaignsSummaryResponse, error) {
	if customerID == 0 {
		return nil, NewBusinessError("CUSTOMER_ID_REQUIRED", "customer_id must be greater than 0", ErrCustomerNotFound)
	}

	// Build counts using repository Count with combined filters
	custID := customerID
	statusApproved := models.CampaignStatusApproved
	statusRunning := models.CampaignStatusRunning

	approvedCount64, err := s.campaignRepo.Count(ctx, models.CampaignFilter{CustomerID: &custID, Status: &statusApproved})
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_COUNT_FAILED", "Failed to count approved campaigns", err)
	}
	runningCount64, err := s.campaignRepo.Count(ctx, models.CampaignFilter{CustomerID: &custID, Status: &statusRunning})
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_COUNT_FAILED", "Failed to count running campaigns", err)
	}

	approved := int(approvedCount64)
	running := int(runningCount64)
	resp := &dto.CampaignsSummaryResponse{
		Message:       "Campaigns summary retrieved",
		ApprovedCount: approved,
		RunningCount:  running,
		Total:         approved + running,
	}
	return resp, nil
}

// calculateParts calculates the number of SMS parts based on the effective
// character count after link substitution rules are applied.
// Non-SMS platforms always use a single part.
func (s *CampaignFlowImpl) calculateParts(content *string, adLink *string, shortLinkDomain *string, platform string) uint64 {
	if platform != models.CampaignPlatformSMS {
		return 1
	}

	if content == nil || *content == "" {
		return 1
	}

	// Count characters with proper weighting (English=1, others=2)
	charCount := s.countCharacters(*content, adLink, shortLinkDomain, platform)

	// Calculate SMS parts based on character count
	if charCount <= 70 {
		return 1
	} else if charCount <= 132 {
		return 2
	} else if charCount <= 198 {
		return 3
	} else if charCount <= 264 {
		return 4
	} else if charCount <= 330 {
		return 5
	}
	return 6 // More than 330 characters
}

// countCharacters counts characters after applying campaign link expansion rules.
func (s *CampaignFlowImpl) countCharacters(text string, adLink *string, shortLinkDomain *string, platform string) uint64 {
	if text == "" {
		if platform == models.CampaignPlatformSMS {
			return 6
		}
		return 0
	}

	textToCount := text

	hasAdLink := adLink != nil && strings.TrimSpace(*adLink) != ""
	hasShortLinkDomain := shortLinkDomain != nil && strings.TrimSpace(*shortLinkDomain) != ""

	switch {
	case hasAdLink && hasShortLinkDomain:
		shortLinkText := strings.TrimSpace(*shortLinkDomain)
		if !strings.HasPrefix(shortLinkText, "https://") {
			shortLinkText = "https://" + shortLinkText
		}
		textToCount = strings.ReplaceAll(textToCount, "{YOUR_LINK}", shortLinkText)
	case hasAdLink:
		resolvedAdLink := strings.TrimSpace(*adLink)
		if strings.Contains(resolvedAdLink, "{uid}") {
			resolvedAdLink = strings.ReplaceAll(resolvedAdLink, "{uid}", "123456")
		}
		textToCount = strings.ReplaceAll(textToCount, "{YOUR_LINK}", resolvedAdLink)
	}

	var count uint64
	for _, char := range textToCount {
		// Check if character is English (ASCII range 32-126)
		if char >= 32 && char <= 126 {
			count += 1 // English character
		} else {
			// Non-English character (Farsi, Arabic, etc.)
			// count += 2

			count += 1
		}
	}

	if platform == models.CampaignPlatformSMS {
		count += 6
	}

	return count
}

func sanitizeShortLinkDomain(domain *string) (*string, error) {
	if domain == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*domain)
	if trimmed == "" {
		return nil, ErrInvalidShortLinkDomain
	}
	if !slices.Contains(allowedShortLinkDomains, trimmed) {
		return nil, ErrInvalidShortLinkDomain
	}
	return &trimmed, nil
}

func sanitizeCategoryAndJob(accountType string, category, job *string, isMandatoryForAgency bool) (*string, *string, error) {
	var sanitizedCategory, sanitizedJob *string
	if category != nil {
		cat := strings.TrimSpace(*category)
		if cat == "" {
			return nil, nil, ErrAgencyCategoryJobRequired
		}
		sanitizedCategory = &cat
	}
	if job != nil {
		j := strings.TrimSpace(*job)
		if j == "" {
			return nil, nil, ErrAgencyCategoryJobRequired
		}
		sanitizedJob = &j
	}

	if accountType == models.AccountTypeMarketingAgency && isMandatoryForAgency {
		if sanitizedCategory == nil || sanitizedJob == nil {
			return nil, nil, ErrAgencyCategoryJobRequired
		}
	}

	return sanitizedCategory, sanitizedJob, nil
}

func sanitizeCampaignPlatform(platform *string) (string, error) {
	if platform == nil {
		return models.CampaignPlatformSMS, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*platform))
	if normalized == "" {
		return "", ErrCampaignPlatformRequired
	}
	if !models.IsValidCampaignPlatform(normalized) {
		return "", ErrCampaignPlatformInvalid
	}
	return normalized, nil
}

func ensureCampaignSpecDefaults(spec *models.CampaignSpec) {
	if spec == nil {
		return
	}
	if strings.TrimSpace(spec.Platform) == "" {
		spec.Platform = models.CampaignPlatformSMS
	}
	if spec.AudienceGrades == nil {
		spec.AudienceGrades = []string{"A", "B", "C"}
	}
}

func sanitizeAudienceGrades(grades []string) ([]string, error) {
	if grades == nil {
		return nil, nil
	}

	valid := []string{"A", "B", "C"}
	normalized := make([]string, 0, len(grades))
	seen := make(map[string]struct{}, len(grades))
	for _, grade := range grades {
		trimmed := strings.ToUpper(strings.TrimSpace(grade))
		if !slices.Contains(valid, trimmed) {
			return nil, ErrCampaignAudienceGradesInvalid
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized, nil
}

func campaignAudienceGradesOrDefault(grades []string) []string {
	if grades == nil {
		return []string{"A", "B", "C"}
	}
	return grades
}

// createAuditLog creates an audit log entry for the campaign operation
func (s *CampaignFlowImpl) createAuditLog(ctx context.Context, customer *models.Customer, action, description string, success bool, errorMsg *string, metadata *ClientMetadata) error {
	var customerID *uint
	if customer != nil {
		customerID = &customer.ID
	}

	ipAddress := ""
	userAgent := ""
	if metadata != nil {
		ipAddress = metadata.IPAddress
		userAgent = metadata.UserAgent
	}

	audit := &models.AuditLog{
		CustomerID:   customerID,
		Action:       action,
		Description:  &description,
		Success:      utils.ToPtr(success),
		IPAddress:    &ipAddress,
		UserAgent:    &userAgent,
		ErrorMessage: errorMsg,
	}

	// Extract request ID from context if available
	requestID := ctx.Value(utils.RequestIDKey)
	if requestID != nil {
		requestIDStr, ok := requestID.(string)
		if ok {
			audit.RequestID = &requestIDStr
		}
	}

	if err := s.auditRepo.Save(ctx, audit); err != nil {
		return err
	}

	return nil
}

func (s *CampaignFlowImpl) CountTargetAudienceFromExcelFile(ctx context.Context, customerID uint, targetAudienceExcelFileUUID string) (uint64, error) {
	asset, err := s.multimediaRepo.ByUUID(ctx, targetAudienceExcelFileUUID)
	if err != nil {
		return 0, err
	}
	if asset == nil || asset.CustomerID != customerID {
		return 0, os.ErrNotExist
	}

	cleanPath, err := sanitizeStoredMultimediaPath(asset.StoredPath)
	if err != nil {
		return 0, err
	}

	rowCount, err := countExcelRows(cleanPath, asset)
	if err != nil {
		return 0, err
	}
	if rowCount == 0 {
		return 0, ErrCampaignTargetAudienceExcelFileInvalid
	}
	return rowCount, nil
}

func countExcelRows(path string, asset *models.MultimediaAsset) (uint64, error) {
	f, err := excelize.OpenFile(path, excelize.Options{
		UnzipSizeLimit:    2 << 30, // 2GB
		UnzipXMLSizeLimit: 1 << 30, // 1GB
	})
	if err != nil {
		return 0, fmt.Errorf("%w: cannot open excel file: %v", ErrCampaignTargetAudienceExcelFileInvalid, err)
	}
	defer func() {
		_ = f.Close()
	}()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return 0, fmt.Errorf("%w: excel file has no sheets", ErrCampaignTargetAudienceExcelFileInvalid)
	}

	rows, err := f.Rows(sheets[0])
	if err != nil {
		return 0, fmt.Errorf("%w: cannot iterate rows: %v", ErrCampaignTargetAudienceExcelFileInvalid, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var count uint64
	for rows.Next() {
		count++
	}
	if err := rows.Error(); err != nil {
		return 0, fmt.Errorf("%w: failed while reading rows: %v", ErrCampaignTargetAudienceExcelFileInvalid, err)
	}

	if isExcelAsset(asset) && count > 0 {
		// Treat first row as header when a dedicated Excel audience file is uploaded.
		count--
	}

	return count, nil
}

func isExcelAsset(asset *models.MultimediaAsset) bool {
	if asset == nil {
		return false
	}
	ext := strings.ToLower(strings.TrimSpace(asset.Extension))
	if ext == ".xlsx" || ext == ".xlsm" || ext == ".xls" {
		return true
	}
	mime := strings.ToLower(strings.TrimSpace(asset.MimeType))
	return strings.Contains(mime, "spreadsheetml") || strings.Contains(mime, "ms-excel")
}

func sanitizeStoredMultimediaPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("invalid empty path")
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute path not allowed")
	}
	base := filepath.ToSlash(filepath.Clean(filepath.Join("data", "uploads", "multimedia")))
	if !strings.HasPrefix(cleaned, base) {
		return "", fmt.Errorf("path outside multimedia root")
	}
	return filepath.FromSlash(cleaned), nil
}

func buildCampaignReportExcel(rows []campaignReportRow) ([]byte, error) {
	xl := excelize.NewFile()
	defer func() { _ = xl.Close() }()

	sheetName := "Report"
	defaultSheet := xl.GetSheetName(0)
	if defaultSheet != sheetName {
		xl.SetSheetName(defaultSheet, sheetName)
	}

	if err := xl.SetSheetRow(sheetName, "A1", &campaignReportHeaders); err != nil {
		return nil, err
	}

	for i, row := range rows {
		record := []string{row.AudienceProfileUID, row.Status, row.Clicked}
		cellRef, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return nil, err
		}
		if err := xl.SetSheetRow(sheetName, cellRef, &record); err != nil {
			return nil, err
		}
	}

	if err := xl.SetColWidth(sheetName, "A", "C", 24); err != nil {
		return nil, err
	}

	buf, err := xl.WriteToBuffer()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func deriveCampaignExportStatus(result campaignStatisticsTrackingResult) string {
	totalParts := int64Value(result.TotalParts)
	totalDeliveredParts := int64Value(result.TotalDeliveredParts)
	totalUndeliveredParts := int64Value(result.TotalUndeliveredParts)

	if result.TotalParts != nil && result.TotalDeliveredParts != nil && totalDeliveredParts == totalParts {
		return "success"
	}
	if totalUndeliveredParts > 0 {
		return "failure"
	}
	return "inactive"
}

func int64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

// ExportCampaignClickReport builds a CSV with two columns - uid and clicked (true/false) -
// for all audience members targeted by the campaign. UIDs are fetched from the campaign's
// file-backed audience store, and click status is derived from short_link_clicks.
func (s *CampaignFlowImpl) ExportCampaignClickReport(ctx context.Context, campaignUUID string) ([]byte, error) {
	campaignUUID = strings.TrimSpace(campaignUUID)
	if campaignUUID == "" {
		return nil, NewBusinessError("CAMPAIGN_UUID_REQUIRED", "campaign uuid is required", ErrCampaignUUIDRequired)
	}
	parsed, err := uuid.Parse(campaignUUID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_UUID_INVALID", "campaign uuid is invalid", ErrCampaignUUIDRequired)
	}

	customerID, ok := ctx.Value(utils.CustomerIDKey).(uint)
	if !ok || customerID == 0 {
		return nil, NewBusinessError("MISSING_CUSTOMER_ID", "customer id is required", ErrCustomerNotFound)
	}

	campaign, err := getCampaign(ctx, s.campaignRepo, parsed.String(), customerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "failed to lookup campaign", err)
	}

	allUIDs, uidToCode, err := readCampaignAudienceUIDs(campaign.ID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewBusinessError("AUDIENCE_REPORT_NOT_AVAILABLE", "audience report data is not available (may have expired or not yet pushed)", nil)
		}
		return nil, NewBusinessError("AUDIENCE_UIDS_FETCH_FAILED", "failed to fetch audience UIDs", err)
	}
	if len(allUIDs) == 0 {
		return nil, NewBusinessError("AUDIENCE_REPORT_NOT_AVAILABLE", "audience report data is not available (may have expired or not yet pushed)", nil)
	}

	clickedCodes := make(map[string]struct{})
	if len(uidToCode) > 0 {
		codes, err := s.shortLinkClickRepo.DistinctShortLinkUIDsByCampaignID(ctx, campaign.ID)
		if err != nil {
			return nil, NewBusinessError("CLICKED_CODES_FETCH_FAILED", "failed to fetch clicked short-link codes", err)
		}
		for _, code := range codes {
			clickedCodes[code] = struct{}{}
		}
	}

	sort.Strings(allUIDs)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"uid", "clicked"}); err != nil {
		return nil, NewBusinessError("CSV_WRITE_FAILED", "failed to write csv header", err)
	}
	for _, uid := range allUIDs {
		clicked := "false"
		if code, ok := uidToCode[uid]; ok && code != "" {
			if _, wasClicked := clickedCodes[code]; wasClicked {
				clicked = "true"
			}
		}
		if err := w.Write([]string{uid, clicked}); err != nil {
			return nil, NewBusinessError("CSV_WRITE_FAILED", "failed to write csv row", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, NewBusinessError("CSV_FLUSH_FAILED", "failed to flush csv", err)
	}

	return buf.Bytes(), nil
}
