package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

type stubSMSClient struct {
	fetchStatusFn func(ctx context.Context, token string, ids []string) (PayamStatusFetchResult, error)
}

func (s *stubSMSClient) SendBatch(ctx context.Context, sender string, items []PayamSMSItem) ([]PayamSMSResponseItem, error) {
	return nil, nil
}

func (s *stubSMSClient) GetToken(ctx context.Context) (string, error) {
	return "", nil
}

func (s *stubSMSClient) FetchStatus(ctx context.Context, token string, ids []string) (PayamStatusFetchResult, error) {
	if s.fetchStatusFn != nil {
		return s.fetchStatusFn(ctx, token, ids)
	}
	return PayamStatusFetchResult{}, nil
}

type stubSMSCampaignStatusJobRepo struct {
	updated []*models.CampaignStatusJob
}

func (s *stubSMSCampaignStatusJobRepo) ByFilter(ctx context.Context, filter any, orderBy string, limit, offset int) ([]*models.CampaignStatusJob, error) {
	return nil, nil
}

func (s *stubSMSCampaignStatusJobRepo) Save(ctx context.Context, entity *models.CampaignStatusJob) error {
	return nil
}

func (s *stubSMSCampaignStatusJobRepo) SaveBatch(ctx context.Context, entities []*models.CampaignStatusJob) error {
	return nil
}

func (s *stubSMSCampaignStatusJobRepo) Count(ctx context.Context, filter any) (int64, error) {
	return 0, nil
}

func (s *stubSMSCampaignStatusJobRepo) Exists(ctx context.Context, filter any) (bool, error) {
	return false, nil
}

func (s *stubSMSCampaignStatusJobRepo) ByID(ctx context.Context, id uint) (*models.CampaignStatusJob, error) {
	return nil, nil
}

func (s *stubSMSCampaignStatusJobRepo) ListDue(ctx context.Context, platform string, now time.Time, limit int) ([]*models.CampaignStatusJob, error) {
	return nil, nil
}

func (s *stubSMSCampaignStatusJobRepo) Update(ctx context.Context, job *models.CampaignStatusJob) error {
	clone := *job
	s.updated = append(s.updated, &clone)
	return nil
}

func TestSMSHandleStatusJobFetchFailureKeepsJobRetryable(t *testing.T) {
	t.Parallel()

	jobRepo := &stubSMSCampaignStatusJobRepo{}
	clientErr := errors.New("temporary status provider failure")
	s := &SMSCampaignScheduler{
		jobRepo: jobRepo,
		smsClient: &stubSMSClient{
			fetchStatusFn: func(ctx context.Context, token string, ids []string) (PayamStatusFetchResult, error) {
				if token != "token-1" {
					t.Fatalf("unexpected token: %q", token)
				}
				if len(ids) != 1 || ids[0] != "trk-1" {
					t.Fatalf("unexpected ids: %#v", ids)
				}
				raw := `{"error":"temporarily unavailable"}`
				return PayamStatusFetchResult{RawResponse: &raw}, clientErr
			},
		},
	}

	job := &models.CampaignStatusJob{
		ID:                  10,
		ProcessedCampaignID: 77,
		TrackingIDs:         []string{"trk-1"},
		RetryCount:          0,
	}

	err := s.handleStatusJob(context.Background(), job, "token-1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), clientErr.Error()) {
		t.Fatalf("expected provider error in return, got: %v", err)
	}
	if job.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got=%d", job.RetryCount)
	}
	if job.ExecutedAt != nil {
		t.Fatalf("expected executed_at to remain nil before max retries")
	}
	if len(jobRepo.updated) != 1 {
		t.Fatalf("expected one job update, got=%d", len(jobRepo.updated))
	}
	if job.RawProviderResponse == nil || *job.RawProviderResponse != `{"error":"temporarily unavailable"}` {
		t.Fatalf("expected raw provider response to be retained, got=%v", job.RawProviderResponse)
	}
}

func TestSMSHandleStatusJobFetchFailureMarksExecutedAtOnMaxRetry(t *testing.T) {
	t.Parallel()

	jobRepo := &stubSMSCampaignStatusJobRepo{}
	s := &SMSCampaignScheduler{
		jobRepo: jobRepo,
		smsClient: &stubSMSClient{
			fetchStatusFn: func(ctx context.Context, token string, ids []string) (PayamStatusFetchResult, error) {
				return PayamStatusFetchResult{}, errors.New("provider still down")
			},
		},
	}

	job := &models.CampaignStatusJob{
		ID:                  11,
		ProcessedCampaignID: 88,
		TrackingIDs:         []string{"trk-2"},
		RetryCount:          smsStatusJobMaxRetry - 1,
	}

	err := s.handleStatusJob(context.Background(), job, "token-2")
	if err == nil {
		t.Fatalf("expected error")
	}
	if job.RetryCount != smsStatusJobMaxRetry {
		t.Fatalf("expected retry_count=%d, got=%d", smsStatusJobMaxRetry, job.RetryCount)
	}
	if job.ExecutedAt == nil {
		t.Fatalf("expected executed_at to be set at retry limit")
	}
	if len(jobRepo.updated) != 1 {
		t.Fatalf("expected one job update, got=%d", len(jobRepo.updated))
	}
}

func TestBuildSMSProviderUpdateMissingResponse(t *testing.T) {
	t.Parallel()

	update := buildSMSProviderUpdate("trk-3", nil, nil)
	if update.TrackingID != "trk-3" {
		t.Fatalf("unexpected tracking id: %q", update.TrackingID)
	}
	if update.ErrorCode == nil || *update.ErrorCode != "MISSING_SEND_RESPONSE" {
		t.Fatalf("expected missing response error code, got=%v", update.ErrorCode)
	}
	if update.Description == nil || !strings.Contains(*update.Description, "trk-3") {
		t.Fatalf("expected missing response description to include tracking id, got=%v", update.Description)
	}
}
