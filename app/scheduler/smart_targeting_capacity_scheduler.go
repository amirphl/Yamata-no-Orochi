package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// SmartTargetingCapacityExecutor is intentionally small so the durable queue
// can be driven by the existing process scheduler without coupling scheduler
// code to the business-flow implementation.
type SmartTargetingCapacityExecutor interface {
	ExecuteCampaignTargetingCapacityCalculation(ctx context.Context, calculationID int64, leaseStartedAt time.Time) error
}

// SmartTargetingCapacityScheduler polls durable calculating generations. A
// process restart simply leaves the row pending for a later poll; the worker is
// idempotent because candidate insertion is keyed by calculation/audience.
type SmartTargetingCapacityScheduler struct {
	executor        SmartTargetingCapacityExecutor
	repo            repository.CampaignTargetingCapacityRepository
	logger          *log.Logger
	pollInterval    time.Duration
	maxParallelRuns int

	mu          sync.Mutex
	inFlight    map[int64]struct{}
	lastCleanup time.Time
}

const (
	smartTargetingCapacityJobTimeout    = 30 * time.Minute
	smartTargetingCapacityLeaseDuration = 35 * time.Minute
	smartTargetingCleanupInterval       = time.Hour
	smartTargetingCleanupBatchSize      = 10000
)

func NewSmartTargetingCapacityScheduler(executor SmartTargetingCapacityExecutor, repo repository.CampaignTargetingCapacityRepository, logger *log.Logger, pollInterval time.Duration, maxParallelRuns int) *SmartTargetingCapacityScheduler {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if maxParallelRuns <= 0 {
		maxParallelRuns = 2
	}
	if logger == nil {
		logger = log.Default()
	}
	return &SmartTargetingCapacityScheduler{
		executor: executor, repo: repo, logger: logger, pollInterval: pollInterval,
		maxParallelRuns: maxParallelRuns, inFlight: make(map[int64]struct{}),
	}
}

func (s *SmartTargetingCapacityScheduler) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	var once sync.Once
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()
		s.runOnce(ctx, &workers)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runOnce(ctx, &workers)
			}
		}
	}()
	return func() {
		once.Do(func() {
			cancel()
			workers.Wait()
		})
	}
}

func (s *SmartTargetingCapacityScheduler) runOnce(parent context.Context, workers *sync.WaitGroup) {
	if s.executor == nil || s.repo == nil {
		return
	}
	now := time.Now().UTC()
	s.maybeCleanup(parent, now)
	slots := s.availableSlots()
	if slots == 0 {
		return
	}
	rows, err := s.repo.ClaimPending(parent, slots, now.Add(-smartTargetingCapacityLeaseDuration), now)
	if err != nil {
		s.logger.Printf("smart targeting capacity scheduler: claim pending calculations failed: %v", err)
		return
	}
	for _, row := range rows {
		if row == nil || row.ID == 0 || row.StartedAt == nil || !s.tryMarkInFlight(row.ID) {
			continue
		}
		workers.Add(1)
		go func(id int64, leaseStartedAt time.Time) {
			defer workers.Done()
			defer s.unmarkInFlight(id)
			jobCtx, cancel := context.WithTimeout(parent, smartTargetingCapacityJobTimeout)
			defer cancel()
			if err := s.executor.ExecuteCampaignTargetingCapacityCalculation(jobCtx, id, leaseStartedAt); err != nil {
				s.logger.Printf("smart targeting capacity scheduler: calculation %d failed: %v", id, err)
			}
		}(row.ID, *row.StartedAt)
	}
}

func (s *SmartTargetingCapacityScheduler) availableSlots() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := s.maxParallelRuns - len(s.inFlight)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *SmartTargetingCapacityScheduler) maybeCleanup(ctx context.Context, now time.Time) {
	s.mu.Lock()
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < smartTargetingCleanupInterval {
		s.mu.Unlock()
		return
	}
	s.lastCleanup = now
	s.mu.Unlock()

	deleted, err := s.repo.DeleteExpiredCandidates(ctx, now, smartTargetingCleanupBatchSize)
	if err != nil {
		s.logger.Printf("smart targeting capacity scheduler: expired candidate cleanup failed: %v", err)
		return
	}
	if deleted > 0 {
		s.logger.Printf("smart targeting capacity scheduler: deleted %d expired candidates", deleted)
	}
}

func (s *SmartTargetingCapacityScheduler) tryMarkInFlight(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.inFlight) >= s.maxParallelRuns {
		return false
	}
	if _, exists := s.inFlight[id]; exists {
		return false
	}
	s.inFlight[id] = struct{}{}
	return true
}

func (s *SmartTargetingCapacityScheduler) unmarkInFlight(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, id)
}
