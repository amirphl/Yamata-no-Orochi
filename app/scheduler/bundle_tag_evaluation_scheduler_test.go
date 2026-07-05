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

type restartTestExecutor struct {
	started  chan struct{}
	finished chan struct{}
	once     sync.Once
}

func (e *restartTestExecutor) ExecuteBundleTagEvaluationRun(ctx context.Context, _ int64) error {
	e.once.Do(func() { close(e.started) })
	<-ctx.Done()
	close(e.finished)
	return ctx.Err()
}

type restartTestReadRepository struct {
	runID int64
}

func (r *restartTestReadRepository) ByBundleID(context.Context, uint) (*models.CurrentBundleTagEvaluationStatus, error) {
	return nil, nil
}

func (r *restartTestReadRepository) ListByBundleIDs(context.Context, []uint) ([]*models.CurrentBundleTagEvaluationStatus, error) {
	return nil, nil
}

func (r *restartTestReadRepository) ByRunID(context.Context, int64) (*models.BundleTagEvaluationRunStatus, error) {
	return nil, nil
}

func (r *restartTestReadRepository) ListPendingRuns(context.Context, int) ([]*models.BundleTagEvaluationRunStatus, error) {
	return []*models.BundleTagEvaluationRunStatus{{EvaluationRunID: r.runID}}, nil
}

func (r *restartTestReadRepository) ListCurrentScoresByBundleID(context.Context, uint, int, int) ([]*models.CurrentBundleTagScore, error) {
	return nil, nil
}

func (r *restartTestReadRepository) CountCurrentScoresByBundleID(context.Context, uint) (int64, error) {
	return 0, nil
}

func TestBundleTagEvaluationSchedulerStopCancelsAndWaitsForRunningJobs(t *testing.T) {
	executor := &restartTestExecutor{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	scheduler := NewBundleTagEvaluationScheduler(
		executor,
		&restartTestReadRepository{runID: 1},
		log.New(io.Discard, "", 0),
		time.Hour,
		1,
	)
	stop := scheduler.Start(context.Background())

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not start the pending job")
	}

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("scheduler stop did not wait for cancellation-aware job shutdown")
	}
	select {
	case <-executor.finished:
	default:
		t.Fatal("scheduler returned before the running job observed cancellation")
	}
}
