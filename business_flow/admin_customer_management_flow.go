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
			FirstName:          r.FirstName,
			LastName:           r.LastName,
			FullName:           r.FullName,
			CompanyName:        r.CompanyName,
			ReferrerAgencyName: r.ReferrerAgencyName,
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
