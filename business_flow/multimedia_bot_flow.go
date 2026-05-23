package businessflow

import (
	"context"
	"mime"
	"net/http"
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
		if os.IsNotExist(err) {
			return generateWhiteImagePreview()
		}
		return "", "", nil, err
	}

	filename := filepath.Base(cleanPath)
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = asset.MimeType
	}
	filename = ensureFilenameHasExtension(filename, contentType, data)
	if contentType == "" {
		contentType = detectContentType(data)
	}

	return filename, contentType, data, nil
}

func ensureFilenameHasExtension(filename, contentType string, data []byte) string {
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "media"
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext != "" {
		return name
	}
	if ext := inferExtension(contentType, data); ext != "" {
		return name + ext
	}
	return name
}

func detectContentType(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	n := len(data)
	if n > 512 {
		n = 512
	}
	return strings.TrimSpace(http.DetectContentType(data[:n]))
}

func inferExtension(contentType string, data []byte) string {
	trimmed := strings.ToLower(strings.TrimSpace(contentType))
	if trimmed == "" {
		trimmed = strings.ToLower(detectContentType(data))
	}

	switch {
	case strings.HasPrefix(trimmed, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(trimmed, "image/png"):
		return ".png"
	case strings.HasPrefix(trimmed, "image/gif"):
		return ".gif"
	case strings.HasPrefix(trimmed, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(trimmed, "audio/ogg"), strings.HasPrefix(trimmed, "application/ogg"):
		return ".ogg"
	case strings.HasPrefix(trimmed, "audio/opus"):
		return ".opus"
	default:
		return ""
	}
}
