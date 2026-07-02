package message

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	maxImageBytes    int64 = 5 * 1024 * 1024
	maxVideoBytes    int64 = 16 * 1024 * 1024
	maxAudioBytes    int64 = 16 * 1024 * 1024
	maxDocumentBytes int64 = 100 * 1024 * 1024
)

type Attachment struct {
	StorageObjectID uuid.UUID `json:"storage_object_id"`
	MimeType        string    `json:"mime_type"`
	SizeBytes       int64     `json:"size_bytes"`
	Filename        string    `json:"filename"`
}

func ContentTypeFromMIME(mimeType string) (ContentType, error) {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp":
		return ContentImage, nil
	case "video/mp4", "video/3gpp":
		return ContentVideo, nil
	case "audio/aac", "audio/mp4", "audio/mpeg", "audio/amr", "audio/ogg", "audio/ogg; codecs=opus":
		return ContentAudio, nil
	case "application/pdf",
		"application/vnd.ms-excel",
		"application/vnd.ms-powerpoint",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ContentDocument, nil
	default:
		return "", fmt.Errorf("unsupported mime type")
	}
}

func ValidateAttachment(mimeType string, sizeBytes int64) (ContentType, error) {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if sizeBytes <= 0 {
		return "", fmt.Errorf("empty upload")
	}
	contentType, err := ContentTypeFromMIME(mimeType)
	if err != nil {
		return "", err
	}
	limit := maxSizeForContentType(contentType)
	if sizeBytes > limit {
		return "", fmt.Errorf("file exceeds allowed size")
	}
	return contentType, nil
}

func maxSizeForContentType(contentType ContentType) int64 {
	switch contentType {
	case ContentImage:
		return maxImageBytes
	case ContentVideo:
		return maxVideoBytes
	case ContentAudio:
		return maxAudioBytes
	case ContentDocument:
		return maxDocumentBytes
	default:
		return 0
	}
}

func WhatsAppMessageType(contentType ContentType) string {
	return string(contentType)
}
