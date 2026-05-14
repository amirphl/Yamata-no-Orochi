// Package businessflow contains admin customer management operations
package businessflow

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// AdminCustomerManagementFlow exposes admin customer management use cases
type AdminCustomerManagementFlow interface {
	ListCustomers(ctx context.Context) (*dto.AdminListCustomersResponse, error)
	GetCustomersShares(ctx context.Context, req *dto.AdminCustomersSharesRequest) (*dto.AdminCustomersSharesResponse, error)
	GetCustomerWithCampaigns(ctx context.Context, customerID uint) (*dto.AdminCustomerWithCampaignsResponse, error)
	GetCustomerDiscountsHistory(ctx context.Context, customerID uint) (*dto.AdminCustomerDiscountHistoryResponse, error)
	SetCustomerActiveStatus(ctx context.Context, req *dto.AdminSetCustomerActiveStatusRequest) (*dto.AdminSetCustomerActiveStatusResponse, error)
}

// AdminCustomerManagementFlowImpl implements AdminCustomerManagementFlow
type AdminCustomerManagementFlowImpl struct {
	transactionRepo repository.TransactionRepository
	customerRepo    repository.CustomerRepository
	campaignRepo    repository.CampaignRepository
	auditRepo       repository.AuditLogRepository
}

const (
	systemCustomerEmail = "system@yamata-no-orochi.com"
	taxCustomerEmail    = "tax@system.yamata-no-orochi.com"
)

func NewAdminCustomerManagementFlow(
	customerRepo repository.CustomerRepository,
	campaignRepo repository.CampaignRepository,
	transactionRepo repository.TransactionRepository,
	auditRepo repository.AuditLogRepository,
) AdminCustomerManagementFlow {
	return &AdminCustomerManagementFlowImpl{
		transactionRepo: transactionRepo,
		customerRepo:    customerRepo,
		campaignRepo:    campaignRepo,
		auditRepo:       auditRepo,
	}
}

// ListCustomers returns all customers except system and tax users.
func (f *AdminCustomerManagementFlowImpl) ListCustomers(ctx context.Context) (*dto.AdminListCustomersResponse, error) {
	customers, err := f.customerRepo.ByFilter(ctx, models.CustomerFilter{}, "id DESC", 0, 0)
	if err != nil {
		return nil, NewBusinessError("GET_ADMIN_CUSTOMERS_LIST_FAILED", "Failed to list customers", err)
	}

	items := make([]dto.AdminCustomerDetailDTO, 0, len(customers))
	for _, cust := range customers {
		if cust == nil || isSystemOrTaxCustomer(cust) {
			continue
		}
		items = append(items, toAdminCustomerDetailDTO(*cust))
	}

	resp := &dto.AdminListCustomersResponse{
		Message: "Customers retrieved successfully",
		Items:   items,
		Total:   uint64(len(items)),
	}
	logAdminAction(ctx, f.auditRepo, models.AuditActionAdminListCustomers, "Admin listed customers", true, nil, map[string]any{
		"total_returned": resp.Total,
	}, nil)
	return resp, nil
}

// GetCustomersShares returns aggregated shares per customer with optional date range
func (f *AdminCustomerManagementFlowImpl) GetCustomersShares(ctx context.Context, req *dto.AdminCustomersSharesRequest) (*dto.AdminCustomersSharesResponse, error) {
	var err error
	startStr := ""
	endStr := ""
	if req != nil && req.StartDate != nil {
		startStr = strings.TrimSpace(*req.StartDate)
	}
	if req != nil && req.EndDate != nil {
		endStr = strings.TrimSpace(*req.EndDate)
	}
	defer func() {
		if err != nil {
			err = NewBusinessError("GET_ADMIN_CUSTOMERS_SHARES_FAILED", "Failed to get customers shares", err)
			logAdminAction(ctx, f.auditRepo, models.AuditActionAdminViewCustomerShares, "Admin viewed customers shares", false, nil, map[string]any{
				"start_date": startStr,
				"end_date":   endStr,
			}, err)
		}
	}()

	var startDate *time.Time
	if req != nil && req.StartDate != nil && *req.StartDate != "" {
		st, e := time.Parse(time.RFC3339, *req.StartDate)
		if e != nil {
			return nil, NewBusinessError("VALIDATION_ERROR", "Invalid start_date format", e)
		}
		startDate = &st
	}
	var endDate *time.Time
	if req != nil && req.EndDate != nil && *req.EndDate != "" {
		et, e := time.Parse(time.RFC3339, *req.EndDate)
		if e != nil {
			return nil, NewBusinessError("VALIDATION_ERROR", "Invalid end_date format", e)
		}
		endDate = &et
	}
	if startDate != nil && endDate != nil && startDate.After(*endDate) {
		return nil, NewBusinessError("VALIDATION_ERROR", "start_date cannot be after end_date", ErrStartDateAfterEndDate)
	}

	rows, err := f.transactionRepo.AggregateCustomersShares(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	customerIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		customerIDs = append(customerIDs, r.CustomerID)
	}
	totalSentByCustomer, err := f.campaignRepo.AggregateTotalSentByCustomerIDs(ctx, customerIDs)
	if err != nil {
		return nil, err
	}
	clickCountsByCustomer, err := f.campaignRepo.AggregateClickCountsByCustomerIDs(ctx, customerIDs)
	if err != nil {
		return nil, err
	}

	items := make([]dto.AdminCustomersSharesItem, 0, len(rows))
	var sumAgency, sumSystem, sumTax, sumTotalSent uint64
	for _, r := range rows {
		totalSent := totalSentByCustomer[r.CustomerID]
		clicks := clickCountsByCustomer[r.CustomerID]
		clickRate := .0
		if totalSent > 0 {
			clickRate = float64(clicks) / float64(totalSent)
		}
		items = append(items, dto.AdminCustomersSharesItem{
			CustomerID:         r.CustomerID,
			FirstName:          r.FirstName,
			LastName:           r.LastName,
			FullName:           r.FullName,
			CompanyName:        r.CompanyName,
			ReferrerAgencyName: r.ReferrerAgencyName,
			AccountTypeName:    r.AccountTypeName,
			IsActive:           r.IsActive,
			AgencyShareWithTax: r.AgencyShareWithTax,
			SystemShare:        r.SystemShare,
			TaxShare:           r.TaxShare,
			TotalSent:          totalSent,
			ClickRate:          clickRate,
		})
		sumAgency += r.AgencyShareWithTax
		sumSystem += r.SystemShare
		sumTax += r.TaxShare
		sumTotalSent += totalSent
	}

	resp := &dto.AdminCustomersSharesResponse{
		Message:               "Customers shares retrieved successfully",
		Items:                 items,
		SumAgencyShareWithTax: sumAgency,
		SumSystemShare:        sumSystem,
		SumTaxShare:           sumTax,
		SumTotalSent:          sumTotalSent,
	}
	logAdminAction(ctx, f.auditRepo, models.AuditActionAdminViewCustomerShares, "Admin viewed customers shares", true, nil, map[string]any{
		"items":      len(items),
		"start_date": startStr,
		"end_date":   endStr,
	}, nil)
	return resp, nil
}

// GetCustomerWithCampaigns retrieves full customer info plus their campaigns
func (f *AdminCustomerManagementFlowImpl) GetCustomerWithCampaigns(ctx context.Context, customerID uint) (*dto.AdminCustomerWithCampaignsResponse, error) {
	customer, err := f.customerRepo.ByID(ctx, customerID)
	if err != nil {
		return nil, NewBusinessError("GET_ADMIN_CUSTOMER_FAILED", "Failed to get customer", err)
	}
	if customer == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}

	campaigns, err := f.campaignRepo.ByCustomerID(ctx, customerID, 0, 0)
	if err != nil {
		return nil, NewBusinessError("GET_ADMIN_CUSTOMER_CAMPAIGNS_FAILED", "Failed to get customer campaigns", err)
	}

	campaignIDs := make([]uint, 0, len(campaigns))
	for _, c := range campaigns {
		campaignIDs = append(campaignIDs, c.ID)
	}
	clickCounts, err := f.campaignRepo.AggregateClickCountsByCampaignIDs(ctx, campaignIDs)
	if err != nil {
		return nil, NewBusinessError("GET_ADMIN_CUSTOMER_CAMPAIGNS_FAILED", "Failed to get campaign clicks", err)
	}

	resp := &dto.AdminCustomerWithCampaignsResponse{
		Message:   "Customer details retrieved successfully",
		Customer:  toAdminCustomerDetailDTO(*customer),
		Campaigns: make([]dto.AdminCustomerCampaignItem, 0, len(campaigns)),
	}
	for _, c := range campaigns {
		var stats map[string]any
		if len(c.Statistics) > 0 {
			_ = json.Unmarshal(c.Statistics, &stats)
		}
		totalSent := uint64(0)
		totalDelivered := uint64(0)
		if v, ok := stats["aggregatedTotalSent"]; ok {
			switch n := v.(type) {
			case float64:
				if n > 0 {
					totalSent = uint64(n)
				}
			case int64:
				if n > 0 {
					totalSent = uint64(n)
				}
			case json.Number:
				if f, e := n.Float64(); e == nil && f > 0 {
					totalSent = uint64(f)
				}
			}
		}
		if v, ok := stats["aggregatedTotalDeliveredParts"]; ok {
			switch n := v.(type) {
			case float64:
				if n > 0 {
					totalDelivered = uint64(n)
				}
			case int64:
				if n > 0 {
					totalDelivered = uint64(n)
				}
			case json.Number:
				if f, e := n.Float64(); e == nil && f > 0 {
					totalDelivered = uint64(f)
				}
			}
		}
		clicks := clickCounts[c.ID]
		clickRate := 0.0
		if totalSent > 0 {
			clickRate = float64(clicks) / float64(totalSent)
		}
		resp.Campaigns = append(resp.Campaigns, dto.AdminCustomerCampaignItem{
			CampaignID:     c.ID,
			Title:          c.Spec.Title,
			CreatedAt:      c.CreatedAt,
			ScheduleAt:     c.Spec.ScheduleAt,
			Status:         c.Status.String(),
			LineNumber:     c.Spec.LineNumber,
			Level3s:        c.Spec.Level3s,
			NumAudience:    c.NumAudience,
			TotalSent:      totalSent,
			TotalDelivered: totalDelivered,
			ClickRate:      clickRate,
		})
	}
	logAdminAction(ctx, f.auditRepo, models.AuditActionAdminViewCustomer, "Admin fetched customer with campaigns", true, &customerID, map[string]any{
		"campaigns": len(campaigns),
	}, nil)
	return resp, nil
}

func toAdminCustomerDetailDTO(c models.Customer) dto.AdminCustomerDetailDTO {
	return dto.AdminCustomerDetailDTO{
		ID:                      c.ID,
		UUID:                    c.UUID.String(),
		AgencyRefererCode:       c.AgencyRefererCode,
		AccountTypeID:           c.AccountTypeID,
		AccountTypeName:         c.AccountType.TypeName,
		CompanyName:             c.CompanyName,
		NationalID:              c.NationalID,
		CompanyPhone:            c.CompanyPhone,
		CompanyAddress:          c.CompanyAddress,
		PostalCode:              c.PostalCode,
		RepresentativeFirstName: c.RepresentativeFirstName,
		RepresentativeLastName:  c.RepresentativeLastName,
		RepresentativeMobile:    c.RepresentativeMobile,
		Email:                   c.Email,
		ShebaNumber:             c.ShebaNumber,
		ReferrerAgencyID:        c.ReferrerAgencyID,
		IsEmailVerified:         c.IsEmailVerified,
		IsMobileVerified:        c.IsMobileVerified,
		IsActive:                c.IsActive,
		CreatedAt:               c.CreatedAt,
		UpdatedAt:               c.UpdatedAt,
		EmailVerifiedAt:         c.EmailVerifiedAt,
		MobileVerifiedAt:        c.MobileVerifiedAt,
		LastLoginAt:             c.LastLoginAt,
	}
}

// GetCustomerDiscountsHistory returns discounts used by a customer across agencies
func (f *AdminCustomerManagementFlowImpl) GetCustomerDiscountsHistory(ctx context.Context, customerID uint) (*dto.AdminCustomerDiscountHistoryResponse, error) {
	var err error
	defer func() {
		if err != nil {
			err = NewBusinessError("GET_ADMIN_CUSTOMER_DISCOUNTS_HISTORY_FAILED", "Failed to get customer discounts history", err)
			logAdminAction(ctx, f.auditRepo, models.AuditActionAdminViewCustomerDiscounts, "Admin viewed customer discount history", false, &customerID, nil, err)
		}
	}()

	customer, err := f.customerRepo.ByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, ErrCustomerNotFound
	}

	rows, err := f.transactionRepo.AggregateCustomerTransactionsByDiscounts(ctx, customerID, "share_desc")
	if err != nil {
		return nil, err
	}

	items := make([]dto.AdminCustomerDiscountHistoryItem, 0, len(rows))
	for _, v := range rows {
		items = append(items, dto.AdminCustomerDiscountHistoryItem{
			DiscountRate:       v.DiscountRate,
			CreatedAt:          v.CreatedAt,
			ExpiresAt:          v.ExpiresAt,
			TotalSent:          0, // TODO: integrate sent counts when available
			AgencyShareWithTax: v.AgencyShareWithTax,
		})
	}

	resp := &dto.AdminCustomerDiscountHistoryResponse{
		Message: "Customer discounts history retrieved successfully",
		Items:   items,
	}
	logAdminAction(ctx, f.auditRepo, models.AuditActionAdminViewCustomerDiscounts, "Admin viewed customer discount history", true, &customerID, map[string]any{
		"items": len(items),
	}, nil)
	return resp, nil
}

// SetCustomerActiveStatus sets the is_active flag based on request body
func (f *AdminCustomerManagementFlowImpl) SetCustomerActiveStatus(ctx context.Context, req *dto.AdminSetCustomerActiveStatusRequest) (*dto.AdminSetCustomerActiveStatusResponse, error) {
	if req == nil || req.CustomerID == 0 {
		return nil, NewBusinessError("VALIDATION_ERROR", "Invalid request", nil)
	}
	customer, err := f.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("SET_CUSTOMER_ACTIVE_STATUS_FAILED", "Failed to get customer", err)
	}
	if customer == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}
	prevActive := false
	if customer.IsActive != nil {
		prevActive = *customer.IsActive
	}
	if !req.IsActive {
		if isSystemOrTaxCustomer(customer) {
			return nil, NewBusinessError("FORBIDDEN_OPERATION", "System and Tax users cannot be deactivated", ErrAccountInactive)
		}
	}
	if customer.IsActive != nil && *customer.IsActive == req.IsActive {
		resp := &dto.AdminSetCustomerActiveStatusResponse{
			Message:  "No change required",
			IsActive: req.IsActive,
		}
		return resp, nil
	}
	if err := f.customerRepo.UpdateActiveStatus(ctx, req.CustomerID, req.IsActive); err != nil {
		logAdminAction(ctx, f.auditRepo, models.AuditActionAdminSetCustomerStatus, "Admin toggled customer active status", false, &req.CustomerID, map[string]any{
			"desired_active": req.IsActive,
			"previous":       prevActive,
		}, err)
		return nil, NewBusinessError("SET_CUSTOMER_ACTIVE_STATUS_FAILED", "Failed to update active status", err)
	}
	resp := &dto.AdminSetCustomerActiveStatusResponse{
		Message:  "Customer status updated successfully",
		IsActive: req.IsActive,
	}
	logAdminAction(ctx, f.auditRepo, models.AuditActionAdminSetCustomerStatus, "Admin toggled customer active status", true, &req.CustomerID, map[string]any{
		"desired_active": req.IsActive,
		"previous":       prevActive,
		"changed":        true,
	}, nil)
	return resp, nil
}

func isSystemOrTaxCustomer(cust *models.Customer) bool {
	if cust == nil {
		return false
	}
	fullName := strings.TrimSpace(cust.RepresentativeFirstName + " " + cust.RepresentativeLastName)
	return strings.EqualFold(cust.Email, systemCustomerEmail) ||
		strings.EqualFold(cust.Email, taxCustomerEmail) ||
		strings.EqualFold(fullName, "System Account") ||
		strings.EqualFold(fullName, "Tax Collector")
}
