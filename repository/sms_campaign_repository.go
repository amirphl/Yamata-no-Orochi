package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

// SMSCampaignRepositoryImpl implements the SMSCampaignRepository interface
type SMSCampaignRepositoryImpl struct {
	*BaseRepository[models.SMSCampaign, models.SMSCampaignFilter]
}

// NewSMSCampaignRepository creates a new SMS campaign repository
func NewSMSCampaignRepository(db *gorm.DB) SMSCampaignRepository {
	return &SMSCampaignRepositoryImpl{
		BaseRepository: NewBaseRepository[models.SMSCampaign, models.SMSCampaignFilter](db),
	}
}

// Create creates a new SMS campaign
func (r *SMSCampaignRepositoryImpl) Create(ctx context.Context, campaign models.SMSCampaign) error {
	db, shouldCommit, err := r.getDBForWrite(ctx)
	if err != nil {
		return err
	}

	if shouldCommit {
		defer func() {
			if err != nil {
				db.Rollback()
			} else {
				db.Commit()
			}
		}()
	}

	// Set default values if not set
	if campaign.CreatedAt.IsZero() {
		campaign.CreatedAt = time.Now().UTC()
	}
	if campaign.Status == "" {
		campaign.Status = models.SMSCampaignStatusInitiated
	}

	err = db.Create(&campaign).Error
	if err != nil {
		return fmt.Errorf("failed to create SMS campaign: %w", err)
	}

	return nil
}

// ByID retrieves an SMS campaign by ID
func (r *SMSCampaignRepositoryImpl) ByID(ctx context.Context, id uint) (*models.SMSCampaign, error) {
	db := r.getDB(ctx)

	var campaign models.SMSCampaign
	err := db.Preload("Customer").
		Preload("Customer.AccountType").
		Last(&campaign, id).Error
	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find SMS campaign by ID %d: %w", id, err)
	}

	return &campaign, nil
}

// ByUUID retrieves an SMS campaign by UUID
func (r *SMSCampaignRepositoryImpl) ByUUID(ctx context.Context, uuid string) (*models.SMSCampaign, error) {
	parsedUUID, err := utils.ParseUUID(uuid)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID format: %w", err)
	}

	filter := models.SMSCampaignFilter{UUID: &parsedUUID}
	campaigns, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to find SMS campaign by UUID: %w", err)
	}

	if len(campaigns) == 0 {
		return nil, nil
	}

	return campaigns[0], nil
}

// ByCustomerID retrieves SMS campaigns by customer ID with pagination
func (r *SMSCampaignRepositoryImpl) ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.SMSCampaign, error) {
	filter := models.SMSCampaignFilter{CustomerID: &customerID}
	return r.ByFilter(ctx, filter, "created_at DESC", limit, offset)
}

// ByStatus retrieves SMS campaigns by status with pagination
func (r *SMSCampaignRepositoryImpl) ByStatus(ctx context.Context, status models.SMSCampaignStatus, limit, offset int) ([]*models.SMSCampaign, error) {
	filter := models.SMSCampaignFilter{Status: &status}
	return r.ByFilter(ctx, filter, "created_at DESC", limit, offset)
}

// Update updates an SMS campaign
func (r *SMSCampaignRepositoryImpl) Update(ctx context.Context, campaign models.SMSCampaign) error {
	db, shouldCommit, err := r.getDBForWrite(ctx)
	if err != nil {
		return err
	}

	if shouldCommit {
		defer func() {
			if err != nil {
				db.Rollback()
			} else {
				db.Commit()
			}
		}()
	}

	// Set updated_at timestamp
	now := time.Now().UTC()
	campaign.UpdatedAt = &now

	err = db.Save(&campaign).Error
	if err != nil {
		return fmt.Errorf("failed to update SMS campaign: %w", err)
	}

	return nil
}

// UpdateStatus updates only the status of an SMS campaign
func (r *SMSCampaignRepositoryImpl) UpdateStatus(ctx context.Context, id uint, status models.SMSCampaignStatus) error {
	db, shouldCommit, err := r.getDBForWrite(ctx)
	if err != nil {
		return err
	}

	if shouldCommit {
		defer func() {
			if err != nil {
				db.Rollback()
			} else {
				db.Commit()
			}
		}()
	}

	now := time.Now().UTC()
	err = db.Model(&models.SMSCampaign{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to update SMS campaign status: %w", err)
	}

	return nil
}

// CountByCustomerID counts SMS campaigns by customer ID
func (r *SMSCampaignRepositoryImpl) CountByCustomerID(ctx context.Context, customerID uint) (int, error) {
	filter := models.SMSCampaignFilter{CustomerID: &customerID}
	count, err := r.Count(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// CountByStatus counts SMS campaigns by status
func (r *SMSCampaignRepositoryImpl) CountByStatus(ctx context.Context, status models.SMSCampaignStatus) (int, error) {
	filter := models.SMSCampaignFilter{Status: &status}
	count, err := r.Count(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// GetPendingApproval retrieves campaigns waiting for approval
func (r *SMSCampaignRepositoryImpl) GetPendingApproval(ctx context.Context, limit, offset int) ([]*models.SMSCampaign, error) {
	status := models.SMSCampaignStatusWaitingForApproval
	filter := models.SMSCampaignFilter{Status: &status}
	return r.ByFilter(ctx, filter, "created_at ASC", limit, offset)
}

// GetScheduledCampaigns retrieves campaigns scheduled between the given times
func (r *SMSCampaignRepositoryImpl) GetScheduledCampaigns(ctx context.Context, from, to time.Time) ([]*models.SMSCampaign, error) {
	filter := models.SMSCampaignFilter{
		ScheduleAfter:  &from,
		ScheduleBefore: &to,
	}
	return r.ByFilter(ctx, filter, "schedule_at ASC", 0, 0)
}

// ByFilter retrieves SMS campaigns based on filter criteria
func (r *SMSCampaignRepositoryImpl) ByFilter(ctx context.Context, filter models.SMSCampaignFilter, orderBy string, limit, offset int) ([]*models.SMSCampaign, error) {
	db := r.getDB(ctx)

	var campaigns []*models.SMSCampaign
	query := r.applyFilter(db, filter)

	// Apply ordering
	if orderBy != "" {
		query = query.Order(orderBy)
	}

	// Apply pagination
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	// Preload relationships
	query = query.Preload("Customer").
		Preload("Customer.AccountType")

	err := query.Find(&campaigns).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find SMS campaigns by filter: %w", err)
	}

	return campaigns, nil
}

// Count returns the number of SMS campaigns matching the filter
func (r *SMSCampaignRepositoryImpl) Count(ctx context.Context, filter models.SMSCampaignFilter) (int64, error) {
	db := r.getDB(ctx)

	var count int64
	var campaign models.SMSCampaign
	query := r.applyFilter(db.Model(&campaign), filter)

	err := query.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count SMS campaigns: %w", err)
	}

	return count, nil
}

// Exists checks if any SMS campaign matching the filter exists
func (r *SMSCampaignRepositoryImpl) Exists(ctx context.Context, filter models.SMSCampaignFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// applyFilter applies filter conditions to the GORM query
func (r *SMSCampaignRepositoryImpl) applyFilter(db *gorm.DB, filter models.SMSCampaignFilter) *gorm.DB {
	if filter.ID != nil {
		db = db.Where("id = ?", *filter.ID)
	}
	if filter.UUID != nil {
		db = db.Where("uuid = ?", *filter.UUID)
	}
	if filter.CustomerID != nil {
		db = db.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.Status != nil {
		db = db.Where("status = ?", *filter.Status)
	}
	if filter.Title != nil {
		db = db.Where("spec->>'title' ILIKE ?", "%"+*filter.Title+"%")
	}
	if filter.Segment != nil {
		db = db.Where("spec->>'segment' = ?", *filter.Segment)
	}
	if filter.Sex != nil {
		db = db.Where("spec->>'sex' = ?", *filter.Sex)
	}
	if filter.City != nil {
		db = db.Where("spec->>'city' @> ?", fmt.Sprintf(`["%s"]`, *filter.City))
	}
	if filter.LineNumber != nil {
		db = db.Where("spec->>'line_number' = ?", *filter.LineNumber)
	}
	if filter.CreatedAfter != nil {
		db = db.Where("created_at >= ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		db = db.Where("created_at <= ?", *filter.CreatedBefore)
	}
	if filter.UpdatedAfter != nil {
		db = db.Where("updated_at >= ?", *filter.UpdatedAfter)
	}
	if filter.UpdatedBefore != nil {
		db = db.Where("updated_at <= ?", *filter.UpdatedBefore)
	}
	if filter.ScheduleAfter != nil {
		db = db.Where("spec->>'schedule_at' >= ?", filter.ScheduleAfter.Format(time.RFC3339))
	}
	if filter.ScheduleBefore != nil {
		db = db.Where("spec->>'schedule_at' <= ?", filter.ScheduleBefore.Format(time.RFC3339))
	}
	if filter.MinBudget != nil {
		db = db.Where("CAST(spec->>'budget' AS BIGINT) >= ?", *filter.MinBudget)
	}
	if filter.MaxBudget != nil {
		db = db.Where("CAST(spec->>'budget' AS BIGINT) <= ?", *filter.MaxBudget)
	}

	return db
}
