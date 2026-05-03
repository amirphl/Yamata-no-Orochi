package businessflow

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// MultimediaBotFlow defines bot operations for multimedia retrieval.
type MultimediaBotFlow interface {
	DownloadMultimediaByBot(ctx context.Context, mediaUUID string) (string, string, []byte, error)
}

// MultimediaBotFlowImpl implements MultimediaBotFlow.
type MultimediaBotFlowImpl struct {
	multimediaRepo repository.MultimediaAssetRepository
}

// NewMultimediaBotFlow creates a new bot multimedia flow instance.
func NewMultimediaBotFlow(multimediaRepo repository.MultimediaAssetRepository) MultimediaBotFlow {
	return &MultimediaBotFlowImpl{
		multimediaRepo: multimediaRepo,
	}
}

func (f *MultimediaBotFlowImpl) DownloadMultimediaByBot(ctx context.Context, mediaUUID string) (string, string, []byte, error) {
	if mediaUUID == "" {
		return "", "", nil, NewBusinessError("INVALID_UUID", "media uuid is required", nil)
	}

	asset, err := f.multimediaRepo.ByUUID(ctx, mediaUUID)
	if err != nil {
		return "", "", nil, err
	}
	if asset == nil {
		return "", "", nil, NewBusinessError("MULTIMEDIA_NOT_FOUND", "multimedia not found", nil)
	}

	cleanPath, err := sanitizeMultimediaPath(asset.StoredPath)
	if err != nil {
		return "", "", nil, err
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", "", nil, err
	}

	filename := filepath.Base(cleanPath)
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = asset.MimeType
	}

	return filename, contentType, data, nil
}
