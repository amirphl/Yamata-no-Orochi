package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"gorm.io/gorm"
)

// CampaignRepositoryImpl implements the CampaignRepository interface
type CampaignRepositoryImpl struct {
	*BaseRepository[models.Campaign, models.CampaignFilter]
}

// NewCampaignRepository creates a new campaign repository
func NewCampaignRepository(db *gorm.DB) CampaignRepository {
	return &CampaignRepositoryImpl{
		BaseRepository: NewBaseRepository[models.Campaign, models.CampaignFilter](db),
	}
}

// ByID retrieves an campaign by ID
func (r *CampaignRepositoryImpl) ByID(ctx context.Context, id uint) (*models.Campaign, error) {
	db := r.getDB(ctx)

	var campaign models.Campaign
	err := db.Preload("Customer").
		Preload("Customer.AccountType").
		Last(&campaign, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &campaign, nil
}

// ByUUID retrieves an campaign by UUID
func (r *CampaignRepositoryImpl) ByUUID(ctx context.Context, uuid string) (*models.Campaign, error) {
	parsedUUID, err := utils.ParseUUID(uuid)
	if err != nil {
		return nil, err
	}

	filter := models.CampaignFilter{UUID: &parsedUUID}
	campaigns, err := r.ByFilter(ctx, filter, "", 0, 0)
	if err != nil {
		return nil, err
	}

	if len(campaigns) == 0 {
		return nil, nil
	}

	return campaigns[0], nil
}

// ByCustomerID retrieves campaigns by customer ID with pagination
func (r *CampaignRepositoryImpl) ByCustomerID(ctx context.Context, customerID uint, limit, offset int) ([]*models.Campaign, error) {
	filter := models.CampaignFilter{CustomerID: &customerID}
	return r.ByFilter(ctx, filter, "created_at DESC", limit, offset)
}

// ByStatus retrieves campaigns by status with pagination
func (r *CampaignRepositoryImpl) ByStatus(ctx context.Context, status models.CampaignStatus, limit, offset int) ([]*models.Campaign, error) {
	filter := models.CampaignFilter{Status: &status}
	return r.ByFilter(ctx, filter, "created_at DESC", limit, offset)
}

// Update updates an campaign
func (r *CampaignRepositoryImpl) Update(ctx context.Context, campaign models.Campaign) error {
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
	now := utils.UTCNow()
	campaign.UpdatedAt = &now

	err = db.Save(&campaign).Error
	if err != nil {
		return err
	}

	return nil
}

func (r *CampaignRepositoryImpl) UpdateStatistics(ctx context.Context, id uint, stats json.RawMessage) error {
	db := r.getDB(ctx)
	return db.Model(&models.Campaign{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"statistics": stats,
			"updated_at": utils.UTCNow(),
		}).Error
}

// UpdateStatus updates only the status of an campaign
func (r *CampaignRepositoryImpl) UpdateStatus(ctx context.Context, id uint, status models.CampaignStatus) error {
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

	now := utils.UTCNow()
	err = db.Model(&models.Campaign{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"updated_at": now,
		}).Error

	if err != nil {
		return err
	}

	return nil
}

// CountByCustomerID counts campaigns by customer ID
func (r *CampaignRepositoryImpl) CountByCustomerID(ctx context.Context, customerID uint) (int, error) {
	filter := models.CampaignFilter{CustomerID: &customerID}
	count, err := r.Count(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// CountByStatus counts campaigns by status
func (r *CampaignRepositoryImpl) CountByStatus(ctx context.Context, status models.CampaignStatus) (int, error) {
	filter := models.CampaignFilter{Status: &status}
	count, err := r.Count(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// GetPendingApproval retrieves campaigns waiting for approval
func (r *CampaignRepositoryImpl) GetPendingApproval(ctx context.Context, limit, offset int) ([]*models.Campaign, error) {
	status := models.CampaignStatusWaitingForApproval
	filter := models.CampaignFilter{Status: &status}
	return r.ByFilter(ctx, filter, "created_at ASC", limit, offset)
}

// GetScheduledCampaigns retrieves campaigns scheduled between the given times
func (r *CampaignRepositoryImpl) GetScheduledCampaigns(ctx context.Context, from, to time.Time) ([]*models.Campaign, error) {
	filter := models.CampaignFilter{
		ScheduleAfter:  &from,
		ScheduleBefore: &to,
	}
	return r.ByFilter(ctx, filter, "schedule_at ASC", 0, 0)
}

// ClickCounts returns a map of campaign_id -> distinct short_link_click uids
func (r *CampaignRepositoryImpl) ClickCounts(ctx context.Context, campaignIDs []uint) (map[uint]int64, error) {
	out := make(map[uint]int64)
	if len(campaignIDs) == 0 {
		return out, nil
	}
	type row struct {
		CampaignID uint
		Clicks     int64
	}
	var rows []row
	db := r.getDB(ctx)
	if err := db.Table("short_link_clicks").
		Select("campaign_id, COUNT(DISTINCT uid) AS clicks").
		Where("campaign_id IN ?", campaignIDs).
		Group("campaign_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CampaignID] = r.Clicks
	}
	return out, nil
}

// ByFilter retrieves campaigns based on filter criteria
func (r *CampaignRepositoryImpl) ByFilter(ctx context.Context, filter models.CampaignFilter, orderBy string, limit, offset int) ([]*models.Campaign, error) {
	db := r.getDB(ctx)

	var campaigns []*models.Campaign
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
		return nil, err
	}

	return campaigns, nil
}

// Count returns the number of campaigns matching the filter
func (r *CampaignRepositoryImpl) Count(ctx context.Context, filter models.CampaignFilter) (int64, error) {
	db := r.getDB(ctx)

	var count int64
	var campaign models.Campaign
	query := r.applyFilter(db.Model(&campaign), filter)

	err := query.Count(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
}

// Exists checks if any campaign matching the filter exists
func (r *CampaignRepositoryImpl) Exists(ctx context.Context, filter models.CampaignFilter) (bool, error) {
	count, err := r.Count(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// applyFilter applies filter conditions to the GORM query
func (r *CampaignRepositoryImpl) applyFilter(db *gorm.DB, filter models.CampaignFilter) *gorm.DB {
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
	if filter.Level1 != nil {
		db = db.Where("spec->>'level1' = ?", *filter.Level1)
	}
	if filter.Sex != nil {
		db = db.Where("spec->>'sex' = ?", *filter.Sex)
	}
	if filter.City != nil {
		db.Where("spec->'city' @> ?::jsonb", fmt.Sprintf(`["%s"]`, *filter.City))
	}
	if filter.LineNumber != nil {
		db = db.Where("spec->>'line_number' = ?", *filter.LineNumber)
	}
	if filter.CreatedAfter != nil {
		db = db.Where("created_at >= ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		db = db.Where("created_at < ?", *filter.CreatedBefore)
	}
	if filter.UpdatedAfter != nil {
		db = db.Where("updated_at > ?", *filter.UpdatedAfter)
	}
	if filter.UpdatedBefore != nil {
		db = db.Where("updated_at < ?", *filter.UpdatedBefore)
	}
	if filter.ScheduleAfter != nil {
		db = db.Where("(spec->>'schedule_at')::timestamptz > ?", *filter.ScheduleAfter)
	}
	if filter.ScheduleBefore != nil {
		db = db.Where("(spec->>'schedule_at')::timestamptz < ?", *filter.ScheduleBefore)
	}
	if filter.MinBudget != nil {
		db = db.Where("CAST(spec->>'budget' AS BIGINT) >= ?", *filter.MinBudget)
	}
	if filter.MaxBudget != nil {
		db = db.Where("CAST(spec->>'budget' AS BIGINT) <= ?", *filter.MaxBudget)
	}

	return db
}
