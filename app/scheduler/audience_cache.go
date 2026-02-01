package scheduler

import (
	"context"
	"errors"

	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// AudienceSelection represents the persisted selection snapshot for a customer/hash pair.
type AudienceSelection struct {
	ID            uint
	CorrelationID string
	IDs           map[int64]struct{}
}

// AudienceCache is a database-backed cache of previously used audience IDs per customer/tags hash.
type AudienceCache struct {
	repo repository.AudienceSelectionRepository
}

func NewAudienceCache(repo repository.AudienceSelectionRepository) *AudienceCache {
	return &AudienceCache{repo: repo}
}

// GetOrCreate returns the current selection for the provided customer/tags hash and loads IDs for the active version.
func (c *AudienceCache) Latest(ctx context.Context, customerID uint, tagsHash string) (*AudienceSelection, error) {
	if c == nil || c.repo == nil {
		return nil, errors.New("audience cache repository not configured")
	}
	row, err := c.repo.Latest(ctx, customerID, tagsHash)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	sel := &AudienceSelection{
		ID:            row.ID,
		CorrelationID: row.CorrelationID,
		IDs:           make(map[int64]struct{}, len(row.AudienceIDs)),
	}
	for _, id := range row.AudienceIDs {
		sel.IDs[id] = struct{}{}
	}
	return sel, nil
}

// AddIDs persists the provided audience IDs for the active selection version and updates the in-memory map.
func (c *AudienceCache) SaveWithMerge(ctx context.Context, customerID uint, tagsHash string, correlationID string, ids []int64) (*AudienceSelection, error) {
	if c == nil || c.repo == nil {
		return nil, errors.New("audience cache repository not configured")
	}
	row, err := c.repo.InsertWithMerge(ctx, customerID, tagsHash, correlationID, ids)
	if err != nil {
		return nil, err
	}
	sel := &AudienceSelection{
		ID:            row.ID,
		CorrelationID: row.CorrelationID,
		IDs:           make(map[int64]struct{}, len(row.AudienceIDs)),
	}
	for _, id := range row.AudienceIDs {
		sel.IDs[id] = struct{}{}
	}
	return sel, nil
}

// SaveSnapshot inserts a new audience selection row without merging with previous snapshots.
func (c *AudienceCache) SaveSnapshot(ctx context.Context, customerID uint, tagsHash string, correlationID string, ids []int64) (*AudienceSelection, error) {
	if c == nil || c.repo == nil {
		return nil, errors.New("audience cache repository not configured")
	}
	row, err := c.repo.InsertSnapshot(ctx, customerID, tagsHash, correlationID, ids)
	if err != nil {
		return nil, err
	}
	sel := &AudienceSelection{
		ID:            row.ID,
		CorrelationID: row.CorrelationID,
		IDs:           make(map[int64]struct{}, len(row.AudienceIDs)),
	}
	for _, id := range row.AudienceIDs {
		sel.IDs[id] = struct{}{}
	}
	return sel, nil
}
