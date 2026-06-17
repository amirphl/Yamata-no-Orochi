package scheduler

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// BundleAudienceSelection is the in-memory representation of a persisted bundle selection snapshot.
type BundleAudienceSelection struct {
	ID            uint
	CorrelationID string
	// IDs is the cumulative set of audience IDs already assigned across all campaigns in the bundle.
	IDs map[int64]struct{}
}

// BundleAudienceCache tracks cumulative audience IDs assigned across campaigns within a bundle.
// Uniqueness is scoped to (customer_id, bundle_id) rather than (customer_id, tags_hash).
type BundleAudienceCache struct {
	repo repository.BundleAudienceSelectionRepository
}

func NewBundleAudienceCache(repo repository.BundleAudienceSelectionRepository) *BundleAudienceCache {
	return &BundleAudienceCache{repo: repo}
}

// Latest returns the most recent selection for the given (customer_id, bundle_id), or nil if none exists.
func (c *BundleAudienceCache) Latest(ctx context.Context, customerID uint, bundleID uint) (*BundleAudienceSelection, error) {
	if c == nil || c.repo == nil {
		return nil, errors.New("bundle audience cache repository not configured")
	}
	row, err := c.repo.Latest(ctx, customerID, bundleID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	sel := &BundleAudienceSelection{
		ID:            row.ID,
		CorrelationID: row.CorrelationID,
		IDs:           make(map[int64]struct{}, len(row.AudienceIDs)),
	}
	for _, id := range row.AudienceIDs {
		sel.IDs[id] = struct{}{}
	}
	return sel, nil
}

// SaveWithMerge inserts a new snapshot merging ids with the existing cumulative set for the bundle.
func (c *BundleAudienceCache) SaveWithMerge(ctx context.Context, customerID uint, bundleID uint, correlationID string, ids []int64) (*BundleAudienceSelection, error) {
	if c == nil || c.repo == nil {
		return nil, errors.New("bundle audience cache repository not configured")
	}
	row, err := c.repo.InsertWithMerge(ctx, customerID, bundleID, correlationID, ids)
	if err != nil {
		return nil, err
	}
	sel := &BundleAudienceSelection{
		ID:            row.ID,
		CorrelationID: row.CorrelationID,
		IDs:           make(map[int64]struct{}, len(row.AudienceIDs)),
	}
	for _, id := range row.AudienceIDs {
		sel.IDs[id] = struct{}{}
	}
	return sel, nil
}
