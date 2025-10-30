// Package businessflow contains admin customer management operations
package businessflow

import (
	"context"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// AdminCustomerManagementFlow exposes admin customer management use cases
type AdminCustomerManagementFlow interface {
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
}

func NewAdminCustomerManagementFlow(
	customerRepo repository.CustomerRepository,
	campaignRepo repository.CampaignRepository,
	transactionRepo repository.TransactionRepository,
) AdminCustomerManagementFlow {
	return &AdminCustomerManagementFlowImpl{
		transactionRepo: transactionRepo,
		customerRepo:    customerRepo,
		campaignRepo:    campaignRepo,
	}
}

// GetCustomersShares returns aggregated shares per customer with optional date range
func (f *AdminCustomerManagementFlowImpl) GetCustomersShares(ctx context.Context, req *dto.AdminCustomersSharesRequest) (*dto.AdminCustomersSharesResponse, error) {
	var err error
	defer func() {
		if err != nil {
			err = NewBusinessError("GET_ADMIN_CUSTOMERS_SHARES_FAILED", "Failed to get customers shares", err)
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

	rows, err := f.transactionRepo.AggregateCustomersShares(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	items := make([]dto.AdminCustomersSharesItem, 0, len(rows))
	var sumAgency, sumSystem, sumTax, sumTotalSent uint64
	for _, r := range rows {
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
			TotalSent:          0,  // TODO
			ClickRate:          -1, // TODO
		})
		sumAgency += r.AgencyShareWithTax
		sumSystem += r.SystemShare
		sumTax += r.TaxShare
		sumTotalSent += 0 // TODO
	}

	return &dto.AdminCustomersSharesResponse{
		Message:               "Customers shares retrieved successfully",
		Items:                 items,
		SumAgencyShareWithTax: sumAgency,
		SumSystemShare:        sumSystem,
		SumTaxShare:           sumTax,
		SumTotalSent:          sumTotalSent,
	}, nil
}

// GetCustomerWithCampaigns retrieves full customer info plus their campaigns
func (f *AdminCustomerManagementFlowImpl) GetCustomerWithCampaigns(ctx context.Context, customerID uint) (*dto.AdminCustomerWithCampaignsResponse, error) {
	cust, err := f.customerRepo.ByID(ctx, customerID)
	if err != nil {
		return nil, NewBusinessError("GET_ADMIN_CUSTOMER_FAILED", "Failed to get customer", err)
	}
	if cust == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}

	campaigns, err := f.campaignRepo.ByCustomerID(ctx, customerID, 0, 0)
	if err != nil {
		return nil, NewBusinessError("GET_ADMIN_CUSTOMER_CAMPAIGNS_FAILED", "Failed to get customer campaigns", err)
	}

	resp := &dto.AdminCustomerWithCampaignsResponse{
		Message:   "Customer details retrieved successfully",
		Customer:  toAdminCustomerDetailDTO(*cust),
		Campaigns: make([]dto.AdminCustomerCampaignItem, 0, len(campaigns)),
	}
	for _, c := range campaigns {
		resp.Campaigns = append(resp.Campaigns, dto.AdminCustomerCampaignItem{
			CampaignID:     c.ID,
			Title:          c.Spec.Title,
			CreatedAt:      c.CreatedAt,
			ScheduleAt:     c.Spec.ScheduleAt,
			Status:         c.Status.String(),
			TotalSent:      0,  // TODO
			TotalDelivered: 0,  // TODO
			ClickRate:      -1, // TODO
		})
	}
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
		}
	}()

	cust, err := f.customerRepo.ByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if cust == nil {
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

	return &dto.AdminCustomerDiscountHistoryResponse{
		Message: "Customer discounts history retrieved successfully",
		Items:   items,
	}, nil
}

// SetCustomerActiveStatus sets the is_active flag based on request body
func (f *AdminCustomerManagementFlowImpl) SetCustomerActiveStatus(ctx context.Context, req *dto.AdminSetCustomerActiveStatusRequest) (*dto.AdminSetCustomerActiveStatusResponse, error) {
	if req == nil || req.CustomerID == 0 {
		return nil, NewBusinessError("VALIDATION_ERROR", "Invalid request", nil)
	}
	cust, err := f.customerRepo.ByID(ctx, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("SET_CUSTOMER_ACTIVE_STATUS_FAILED", "Failed to get customer", err)
	}
	if cust == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}
	if !req.IsActive {
		// TODO: Use account type name instead of representative names
		fullName := (cust.RepresentativeFirstName + " " + cust.RepresentativeLastName)
		if fullName == "System Account" || fullName == "Tax Collector" {
			return nil, NewBusinessError("FORBIDDEN_OPERATION", "System and Tax users cannot be deactivated", ErrAccountInactive)
		}
	}
	if cust.IsActive != nil && *cust.IsActive == req.IsActive {
		return &dto.AdminSetCustomerActiveStatusResponse{
			Message:  "No change required",
			IsActive: req.IsActive,
		}, nil
	}
	if err := f.customerRepo.UpdateActiveStatus(ctx, req.CustomerID, req.IsActive); err != nil {
		return nil, NewBusinessError("SET_CUSTOMER_ACTIVE_STATUS_FAILED", "Failed to update active status", err)
	}
	return &dto.AdminSetCustomerActiveStatusResponse{
		Message:  "Customer status updated successfully",
		IsActive: req.IsActive,
	}, nil
}
