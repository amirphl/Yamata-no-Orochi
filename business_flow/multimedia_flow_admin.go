package businessflow

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/repository"
)

// MultimediaAdminFlow defines admin operations for multimedia retrieval.
type MultimediaAdminFlow interface {
	DownloadMultimediaByAdmin(ctx context.Context, mediaUUID string) (string, string, []byte, error)
	PreviewMultimediaByAdmin(ctx context.Context, mediaUUID string) (string, string, []byte, error)
}

// MultimediaAdminFlowImpl implements MultimediaAdminFlow.
type MultimediaAdminFlowImpl struct {
	multimediaRepo repository.MultimediaAssetRepository
}

// NewMultimediaAdminFlow creates a new admin multimedia flow instance.
func NewMultimediaAdminFlow(multimediaRepo repository.MultimediaAssetRepository) MultimediaAdminFlow {
	return &MultimediaAdminFlowImpl{
		multimediaRepo: multimediaRepo,
	}
}

func (f *MultimediaAdminFlowImpl) DownloadMultimediaByAdmin(ctx context.Context, mediaUUID string) (string, string, []byte, error) {
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

func (f *MultimediaAdminFlowImpl) PreviewMultimediaByAdmin(ctx context.Context, mediaUUID string) (string, string, []byte, error) {
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

	if asset.MediaType == "video" {
		return extractVideoThumbnail(cleanPath)
	}
	return generateImageThumbnail(cleanPath)
}
