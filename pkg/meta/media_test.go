package meta

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadMediaAndSendMediaMessage(t *testing.T) {
	var uploadCalled, sendCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/media") && r.Method == http.MethodPost:
			uploadCalled = true
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || !strings.HasPrefix(mediaType, "multipart/form-data") {
				t.Fatalf("Content-Type = %q err=%v", r.Header.Get("Content-Type"), err)
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			var sawMessagingProduct, sawFile bool
			for {
				part, partErr := reader.NextPart()
				if partErr == io.EOF {
					break
				}
				if partErr != nil {
					t.Fatalf("NextPart() error = %v", partErr)
				}
				switch part.FormName() {
				case "messaging_product":
					sawMessagingProduct = true
					body, _ := io.ReadAll(part)
					if string(body) != "whatsapp" {
						t.Fatalf("messaging_product = %q", body)
					}
				case "file":
					sawFile = true
					if part.Header.Get("Content-Type") != "image/jpeg" {
						t.Fatalf("file Content-Type = %q", part.Header.Get("Content-Type"))
					}
				case "type":
					t.Fatalf("unexpected separate type field in upload request")
				}
			}
			if !sawMessagingProduct || !sawFile {
				t.Fatalf("sawMessagingProduct=%v sawFile=%v", sawMessagingProduct, sawFile)
			}
			_, _ = w.Write([]byte(`{"id":"meta-media-123"}`))
		case strings.Contains(r.URL.Path, "/messages") && r.Method == http.MethodPost:
			sendCalled = true
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.abc"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(server.URL)

	mediaID, err := client.UploadMedia(context.Background(), "phone-1", "token", "image/jpeg", "photo.jpg", strings.NewReader("jpeg-data"), 9)
	if err != nil {
		t.Fatalf("UploadMedia() error = %v", err)
	}
	if mediaID != "meta-media-123" {
		t.Fatalf("mediaID = %q", mediaID)
	}

	wamid, err := client.SendMediaMessage(context.Background(), "phone-1", "token", "+62123", "image", mediaID, "caption", "")
	if err != nil {
		t.Fatalf("SendMediaMessage() error = %v", err)
	}
	if wamid != "wamid.abc" {
		t.Fatalf("wamid = %q", wamid)
	}
	if !uploadCalled || !sendCalled {
		t.Fatalf("uploadCalled=%v sendCalled=%v", uploadCalled, sendCalled)
	}
}
