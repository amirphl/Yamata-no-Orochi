package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/repository"
)

type BundleTagEvaluationExecutor interface {
	ExecuteBundleTagEvaluationRun(ctx context.Context, runID int64) error
}

type BundleTagEvaluationScheduler struct {
	flow            BundleTagEvaluationExecutor
	readRepo        repository.BundleTagEvaluationReadRepository
	logger          *log.Logger
	pollInterval    time.Duration
	maxParallelRuns int

	mu       sync.Mutex
	inFlight map[int64]struct{}
}

func NewBundleTagEvaluationScheduler(
	flow BundleTagEvaluationExecutor,
	readRepo repository.BundleTagEvaluationReadRepository,
	logger *log.Logger,
	pollInterval time.Duration,
	maxParallelRuns int,
) *BundleTagEvaluationScheduler {
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}
	if maxParallelRuns <= 0 {
		maxParallelRuns = 1
	}
	if logger == nil {
		logger = log.Default()
	}
	return &BundleTagEvaluationScheduler{
		flow:            flow,
		readRepo:        readRepo,
		logger:          logger,
		pollInterval:    pollInterval,
		maxParallelRuns: maxParallelRuns,
		inFlight:        make(map[int64]struct{}),
	}
}

func (s *BundleTagEvaluationScheduler) Start(parent context.Context) func() {
	workerCtx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	var stopOnce sync.Once

	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		s.runOnce(workerCtx, &workers)
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				s.runOnce(workerCtx, &workers)
			}
		}
	}()
	return func() {
		stopOnce.Do(func() {
			cancel()
			workers.Wait()
		})
	}
}

func (s *BundleTagEvaluationScheduler) runOnce(parent context.Context, workers *sync.WaitGroup) {
	rows, err := s.readRepo.ListPendingRuns(parent, s.maxParallelRuns*2)
	if err != nil {
		s.logger.Printf("bundle tag evaluation scheduler: list pending runs failed: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	for _, row := range rows {
		if row == nil || row.EvaluationRunID == 0 {
			continue
		}
		if !s.tryMarkInFlight(row.EvaluationRunID) {
			continue
		}
		workers.Add(1)
		go func(runID int64) {
			defer workers.Done()
			defer s.unmarkInFlight(runID)

			ctx, cancel := context.WithTimeout(parent, 60*time.Minute)
			defer cancel()

			if err := s.flow.ExecuteBundleTagEvaluationRun(ctx, runID); err != nil {
				s.logger.Printf("bundle tag evaluation scheduler: execute run %d failed: %v", runID, err)
			}
		}(row.EvaluationRunID)

		if s.inFlightCount() >= s.maxParallelRuns {
			return
		}
	}
}

func (s *BundleTagEvaluationScheduler) tryMarkInFlight(runID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.inFlight) >= s.maxParallelRuns {
		return false
	}
	if _, exists := s.inFlight[runID]; exists {
		return false
	}
	s.inFlight[runID] = struct{}{}
	return true
}

func (s *BundleTagEvaluationScheduler) unmarkInFlight(runID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, runID)
}

func (s *BundleTagEvaluationScheduler) inFlightCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inFlight)
}
