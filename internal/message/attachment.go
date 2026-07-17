package message

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

const (
	maxImageBytes    int64 = 5 * 1024 * 1024
	maxVideoBytes    int64 = 16 * 1024 * 1024
	maxAudioBytes    int64 = 16 * 1024 * 1024
	maxDocumentBytes int64 = 100 * 1024 * 1024

	// Instagram Messaging API limits (graph.instagram.com).
	maxInstagramImageBytes    int64 = 8 * 1024 * 1024
	maxInstagramVideoBytes    int64 = 25 * 1024 * 1024
	maxInstagramAudioBytes    int64 = 25 * 1024 * 1024
	maxInstagramDocumentBytes int64 = 25 * 1024 * 1024
)

type Attachment struct {
	StorageObjectID uuid.UUID `json:"storage_object_id"`
	MimeType        string    `json:"mime_type"`
	SizeBytes       int64     `json:"size_bytes"`
	Filename        string    `json:"filename"`
}

func normalizeMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func ContentTypeFromMIME(mimeType string) (ContentType, error) {
	mimeType = normalizeMIME(mimeType)
	switch mimeType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return ContentImage, nil
	case "video/mp4", "video/3gpp":
		return ContentVideo, nil
	case "audio/aac", "audio/mp4", "audio/mpeg", "audio/amr", "audio/ogg":
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

// ContentTypeFromMIMEForChannel maps MIME types using channel-specific allowlists.
func ContentTypeFromMIMEForChannel(mimeType string, channelType conversation.ChannelType) (ContentType, error) {
	if channelType == conversation.ChannelInstagram {
		return contentTypeFromInstagramMIME(mimeType)
	}
	return ContentTypeFromMIME(mimeType)
}

func contentTypeFromInstagramMIME(mimeType string) (ContentType, error) {
	mimeType = normalizeMIME(mimeType)
	switch mimeType {
	case "image/jpeg", "image/jpg", "image/png":
		return ContentImage, nil
	case "image/webp":
		return "", fmt.Errorf("instagram does not support WebP for outbound media; use JPEG or PNG")
	case "video/mp4", "video/ogg", "video/avi", "video/quicktime", "video/webm":
		return ContentVideo, nil
	case "video/3gpp":
		return "", fmt.Errorf("instagram does not support 3GPP video; use MP4, MOV, WebM, OGG, or AVI")
	case "audio/aac", "audio/mp4", "audio/mpeg", "audio/wav", "audio/x-wav", "audio/wave":
		return ContentAudio, nil
	case "application/pdf":
		return ContentDocument, nil
	default:
		if mimeType == "" {
			return "", fmt.Errorf("instagram media mime type is required")
		}
		return "", fmt.Errorf("instagram does not support mime type %s", mimeType)
	}
}

func ValidateAttachment(mimeType string, sizeBytes int64) (ContentType, error) {
	return ValidateAttachmentForChannel(mimeType, sizeBytes, conversation.ChannelWhatsApp)
}

func ValidateAttachmentForChannel(mimeType string, sizeBytes int64, channelType conversation.ChannelType) (ContentType, error) {
	if sizeBytes <= 0 {
		return "", fmt.Errorf("empty upload")
	}
	contentType, err := ContentTypeFromMIMEForChannel(mimeType, channelType)
	if err != nil {
		return "", err
	}
	limit := maxSizeForContentType(contentType, channelType)
	if sizeBytes > limit {
		return "", fmt.Errorf("file exceeds allowed size (%d MB max for this channel)", limit/(1024*1024))
	}
	return contentType, nil
}

func maxSizeForContentType(contentType ContentType, channelType conversation.ChannelType) int64 {
	if channelType == conversation.ChannelInstagram {
		switch contentType {
		case ContentImage:
			return maxInstagramImageBytes
		case ContentVideo:
			return maxInstagramVideoBytes
		case ContentAudio:
			return maxInstagramAudioBytes
		case ContentDocument:
			return maxInstagramDocumentBytes
		default:
			return 0
		}
	}
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
