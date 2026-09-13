package businessflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

type publicationRepoStub struct {
	links  []*models.ShortLink
	marked bool
}

func (r *publicationRepoStub) ByUIDs(context.Context, []string) ([]*models.ShortLink, error) {
	return r.links, nil
}

func (r *publicationRepoStub) MarkExternallyPublished(context.Context, []string, time.Time) error {
	r.marked = true
	return nil
}

type publisherStub struct {
	err error
}

func (p publisherStub) UploadMappings(context.Context, []*models.ShortLink) error { return p.err }

func TestPublishShortLinkMappingsMarksOnlyAcknowledgedMappings(t *testing.T) {
	link := &models.ShortLink{ID: 1, UID: "abc1", LongLink: "https://example.com"}

	failedRepo := &publicationRepoStub{links: []*models.ShortLink{link}}
	if err := publishShortLinkMappings(context.Background(), failedRepo, publisherStub{err: errors.New("unavailable")}, []*models.ShortLink{link}); err == nil {
		t.Fatal("publishShortLinkMappings() error = nil, want upload failure")
	}
	if failedRepo.marked {
		t.Fatal("failed mapping was marked published")
	}

	successRepo := &publicationRepoStub{links: []*models.ShortLink{link}}
	if err := publishShortLinkMappings(context.Background(), successRepo, publisherStub{}, []*models.ShortLink{link}); err != nil {
		t.Fatalf("publishShortLinkMappings() error = %v", err)
	}
	if !successRepo.marked {
		t.Fatal("acknowledged mapping was not marked published")
	}
}
