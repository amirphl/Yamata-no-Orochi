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

func TestNormalizeSchedulerScoreClassesRejectsDuplicate(t *testing.T) {
	if _, err := normalizeSchedulerScoreClasses([]string{"A", "a"}); err == nil {
		t.Fatal("duplicate score classes must fail closed at execution")
	}
	if got, err := normalizeSchedulerScoreClasses([]string{"c", "A"}); err != nil || len(got) != 2 || got[0] != "A" || got[1] != "C" {
		t.Fatalf("canonical classes = %v, %v", got, err)
	}
	if _, err := normalizeSchedulerScoreClasses([]string{"D"}); err == nil {
		t.Fatal("invalid score class must fail closed at execution")
	}
}

type capacitySchedulerTestRepo struct {
	mu              sync.Mutex
	claimCalls      int
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

func (r *capacitySchedulerTestRepo) ActiveByCampaignID(context.Context, uint) (*models.CampaignTargetingCapacityCalculation, error) {
	return nil, nil
}

func (r *capacitySchedulerTestRepo) LatestCalculatedByInput(context.Context, uint, string) (*models.CampaignTargetingCapacityCalculation, error) {
	return nil, nil
}

func (r *capacitySchedulerTestRepo) LatestByInput(context.Context, uint, string) (*models.CampaignTargetingCapacityCalculation, error) {
	return nil, nil
}

func (r *capacitySchedulerTestRepo) CurrentForExecution(context.Context, uint, uint, []int64, []string, int, time.Time) (*models.CampaignTargetingCapacityCalculation, error) {
	return nil, nil
}

func (r *capacitySchedulerTestRepo) Supersede(context.Context, int64, string, string, time.Time) error {
	return nil
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

func (r *capacitySchedulerTestRepo) Complete(context.Context, int64, time.Time, int64, int64, int64, int64, string, time.Time) error {
	return nil
}

func (r *capacitySchedulerTestRepo) Fail(context.Context, int64, time.Time, string, string, time.Time) error {
	return nil
}

type capacitySchedulerTestExecutor struct {
	mu    sync.Mutex
	ids   []int64
	block <-chan struct{}
}

func (e *capacitySchedulerTestExecutor) ExecuteCampaignTargetingCapacityCalculation(_ context.Context, id int64, _ time.Time) error {
	e.mu.Lock()
	e.ids = append(e.ids, id)
	block := e.block
	e.mu.Unlock()
	if block != nil {
		<-block
	}
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
	claimCalls, limit := repo.claimCalls, repo.lastLimit
	leaseAge := time.Since(repo.lastStaleBefore)
	repo.mu.Unlock()
	if claimCalls != 1 || limit != 1 {
		t.Fatalf("claim calls = %d with limit %d, want 1 with limit 1", claimCalls, limit)
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

func TestSmartTargetingCapacitySchedulerExecutesDuplicateClaimOnce(t *testing.T) {
	startedAt := time.Now().UTC()
	repo := &capacitySchedulerTestRepo{claimed: []*models.CampaignTargetingCapacityCalculation{
		{ID: 73, StartedAt: &startedAt}, {ID: 73, StartedAt: &startedAt},
	}}
	release := make(chan struct{})
	executor := &capacitySchedulerTestExecutor{block: release}
	scheduler := NewSmartTargetingCapacityScheduler(executor, repo, log.New(io.Discard, "", 0), time.Hour, 2)

	var workers sync.WaitGroup
	scheduler.runOnce(t.Context(), &workers)
	close(release)
	workers.Wait()

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.ids) != 1 || executor.ids[0] != 73 {
		t.Fatalf("executed IDs = %v, want one execution of 73", executor.ids)
	}
}
