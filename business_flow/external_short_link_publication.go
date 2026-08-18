package businessflow

import (
	"context"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

// ShortLinkMappingPublisher is implemented by the authenticated external-service client.
type ShortLinkMappingPublisher interface {
	UploadMappings(ctx context.Context, links []*models.ShortLink) error
}

type shortLinkPublicationRepository interface {
	ByUIDs(ctx context.Context, uids []string) ([]*models.ShortLink, error)
	MarkExternallyPublished(ctx context.Context, uids []string, publishedAt time.Time) error
}

func publishShortLinkMappings(
	ctx context.Context,
	repo shortLinkPublicationRepository,
	publisher ShortLinkMappingPublisher,
	links []*models.ShortLink,
) error {
	if publisher == nil || len(links) == 0 {
		return nil
	}
	uids := make([]string, 0, len(links))
	for _, link := range links {
		if link == nil || link.UID == "" {
			return fmt.Errorf("cannot publish an empty short-link mapping")
		}
		uids = append(uids, link.UID)
	}
	persisted, err := repo.ByUIDs(ctx, uids)
	if err != nil {
		return fmt.Errorf("reload persisted short-link mappings: %w", err)
	}
	if len(persisted) != len(uids) {
		return fmt.Errorf("reload persisted short-link mappings: got %d rows for %d codes", len(persisted), len(uids))
	}
	if err := publisher.UploadMappings(ctx, persisted); err != nil {
		return err
	}
	// The external acknowledgement is the send-safety boundary. If this local
	// marker write fails, the background publisher repeats the idempotent upload.
	_ = repo.MarkExternallyPublished(ctx, uids, time.Now().UTC())
	return nil
}
