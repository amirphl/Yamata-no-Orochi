package dto

import "io"

// UploadMultimediaRequest contains upload details passed from handler to flow.
type UploadMultimediaRequest struct {
	CustomerID       uint      `json:"-"`
	OriginalFilename string    `json:"-"`
	FileSize         int64     `json:"-"`
	ContentType      string    `json:"-"`
	File             io.Reader `json:"-"`
}

// UploadMultimediaResponse represents a successful multimedia upload response.
type UploadMultimediaResponse struct {
	Message          string `json:"message"`
	UUID             string `json:"uuid"`
	MediaType        string `json:"media_type"`
	MimeType         string `json:"mime_type"`
	SizeBytes        int64  `json:"size_bytes"`
	OriginalFilename string `json:"original_filename"`
	CreatedAt        string `json:"created_at"`
}
