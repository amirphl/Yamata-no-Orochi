package businessflow

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

type ProfileFlow interface {
	GetProfile(ctx context.Context, customerID uint) (*dto.GetProfileResponse, error)
}

type ProfileFlowImpl struct {
	customerRepo repository.CustomerRepository
}

func NewProfileFlow(customerRepo repository.CustomerRepository) ProfileFlow {
	return &ProfileFlowImpl{customerRepo: customerRepo}
}

func (f *ProfileFlowImpl) GetProfile(ctx context.Context, customerID uint) (*dto.GetProfileResponse, error) {
	if customerID == 0 {
		return nil, NewBusinessError("CUSTOMER_ID_REQUIRED", "customer_id must be greater than 0", ErrCustomerNotFound)
	}

	cust, err := f.customerRepo.ByID(ctx, customerID)
	if err != nil {
		return nil, NewBusinessError("CUSTOMER_FETCH_FAILED", "Failed to fetch customer", err)
	}
	if cust == nil {
		return nil, NewBusinessError("CUSTOMER_NOT_FOUND", "Customer not found", ErrCustomerNotFound)
	}

	resp := &dto.GetProfileResponse{Message: "Customer profile retrieved"}
	resp.Customer = mapCustomerToProfileDTO(cust)

	if cust.ReferrerAgency != nil {
		// Non-agency: parent agency name on customer and full agency spec as ParentAgency
		agencyName := agencyDisplayName(cust.ReferrerAgency)
		resp.Customer.ParentAgencyName = &agencyName
		resp.ParentAgency = mapAgencyToDTO(cust.ReferrerAgency)
	}

	return resp, nil
}

func mapCustomerToProfileDTO(c *models.Customer) dto.ProfileDTO {
	dtoOut := dto.ProfileDTO{
		ID:                      c.ID,
		UUID:                    c.UUID.String(),
		AccountType:             c.AccountType.TypeName,
		AccountTypeDisplayName:  c.AccountType.DisplayName,
		Email:                   c.Email,
		RepresentativeFirstName: c.RepresentativeFirstName,
		RepresentativeLastName:  c.RepresentativeLastName,
		RepresentativeMobile:    c.RepresentativeMobile,
		CompanyName:             c.CompanyName,
		NationalID:              c.NationalID,
		CompanyPhone:            c.CompanyPhone,
		CompanyAddress:          c.CompanyAddress,
		PostalCode:              c.PostalCode,
		ShebaNumber:             c.ShebaNumber,
		Category:                c.Category,
		Job:                     c.Job,
		IsActive:                c.IsActive,
		AgencyRefererCode:       c.AgencyRefererCode,
		ReferrerAgencyID:        c.ReferrerAgencyID,
		IsEmailVerified:         c.IsEmailVerified,
		IsMobileVerified:        c.IsMobileVerified,
		LastLoginAt:             c.LastLoginAt,
		CreatedAt:               c.CreatedAt,
		UpdatedAt:               c.UpdatedAt,
	}
	return dtoOut
}

func mapAgencyToDTO(a *models.Customer) *dto.AgencyProfileDTO {
	if a == nil {
		return nil
	}
	return &dto.AgencyProfileDTO{
		AgencyRefererCode: a.AgencyRefererCode,
		AccountType:       a.AccountType.TypeName,
		DisplayName:       a.AccountType.DisplayName,
		CompanyName:       a.CompanyName,
		IsActive:          a.IsActive,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}
}

func agencyDisplayName(c *models.Customer) string {
	if c == nil {
		return ""
	}
	if c.CompanyName != nil && *c.CompanyName != "" {
		return *c.CompanyName
	}
	// Fallback to representative name
	return c.RepresentativeFirstName + " " + c.RepresentativeLastName
}
