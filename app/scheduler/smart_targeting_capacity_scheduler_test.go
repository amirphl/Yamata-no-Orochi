package scheduler

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

type capacitySchedulerTestRepo struct {
	mu              sync.Mutex
	claimCalls      int
	cleanupCalls    int
	claimed         []*models.CampaignTargetingCapacityCalculation
	lastLimit       int
	lastStaleBefore time.Time
}

func (r *capacitySchedulerTestRepo) Save(context.Context, *models.CampaignTargetingCapacityCalculation) error {
	return nil
}

func (r *capacitySchedulerTestRepo) ByID(context.Context, int64) (*models.CampaignTargetingCapacityCalculation, error) {
	return nil, nil
}

func (r *capacitySchedulerTestRepo) LatestByCampaignID(context.Context, uint) (*models.CampaignTargetingCapacityCalculation, error) {
	return nil, nil
}

func (r *capacitySchedulerTestRepo) ClaimPending(_ context.Context, limit int, staleBefore, _ time.Time) ([]*models.CampaignTargetingCapacityCalculation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimCalls++
	r.lastLimit = limit
	r.lastStaleBefore = staleBefore
	rows := r.claimed
	r.claimed = nil
	return rows, nil
}

func (r *capacitySchedulerTestRepo) Complete(context.Context, int64, time.Time, int64, int64, int64, string, time.Time) error {
	return nil
}

func (r *capacitySchedulerTestRepo) Fail(context.Context, int64, time.Time, string, string, time.Time) error {
	return nil
}

func (r *capacitySchedulerTestRepo) DeleteExpiredCandidates(context.Context, time.Time, int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupCalls++
	return 0, nil
}

type capacitySchedulerTestExecutor struct {
	mu  sync.Mutex
	ids []int64
}

func (e *capacitySchedulerTestExecutor) ExecuteCampaignTargetingCapacityCalculation(_ context.Context, id int64, _ time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ids = append(e.ids, id)
	return nil
}

func TestSmartTargetingCapacitySchedulerUsesDurableClaimsAndCleanup(t *testing.T) {
	startedAt := time.Now().UTC()
	repo := &capacitySchedulerTestRepo{
		claimed: []*models.CampaignTargetingCapacityCalculation{{ID: 41, StartedAt: &startedAt}},
	}
	executor := &capacitySchedulerTestExecutor{}
	scheduler := NewSmartTargetingCapacityScheduler(executor, repo, log.New(io.Discard, "", 0), time.Hour, 1)

	var workers sync.WaitGroup
	scheduler.runOnce(t.Context(), &workers)
	workers.Wait()

	repo.mu.Lock()
	claimCalls, cleanupCalls, limit := repo.claimCalls, repo.cleanupCalls, repo.lastLimit
	leaseAge := time.Since(repo.lastStaleBefore)
	repo.mu.Unlock()
	if claimCalls != 1 || cleanupCalls != 1 || limit != 1 {
		t.Fatalf("claim/cleanup calls = %d/%d with limit %d, want 1/1 with limit 1", claimCalls, cleanupCalls, limit)
	}
	if leaseAge < smartTargetingCapacityLeaseDuration || leaseAge > smartTargetingCapacityLeaseDuration+time.Minute {
		t.Fatalf("stale lease age = %s, want approximately %s", leaseAge, smartTargetingCapacityLeaseDuration)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.ids) != 1 || executor.ids[0] != 41 {
		t.Fatalf("executed IDs = %v, want [41]", executor.ids)
	}
}
