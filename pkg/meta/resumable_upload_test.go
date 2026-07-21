package meta

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadResumableMedia(t *testing.T) {
	var started, uploaded bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/uploads") && r.Method == http.MethodPost:
			started = true
			if r.URL.Query().Get("file_length") != "4" {
				t.Fatalf("file_length = %q", r.URL.Query().Get("file_length"))
			}
			if r.URL.Query().Get("file_type") != "image/jpeg" {
				t.Fatalf("file_type = %q", r.URL.Query().Get("file_type"))
			}
			_, _ = w.Write([]byte(`{"id":"upload:session-1"}`))
		case strings.Contains(r.URL.Path, "/upload:session-1") && r.Method == http.MethodPost:
			uploaded = true
			if r.Header.Get("Authorization") != "OAuth token" {
				t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("file_offset") != "0" {
				t.Fatalf("file_offset = %q", r.Header.Get("file_offset"))
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "jpeg" {
				t.Fatalf("body = %q", body)
			}
			_, _ = w.Write([]byte(`{"h":"4::abcHANDLE"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(server.URL)

	handle, err := client.UploadResumableMedia(
		context.Background(),
		"app-1",
		"token",
		"sample.jpg",
		"image/jpeg",
		strings.NewReader("jpeg"),
		4,
	)
	if err != nil {
		t.Fatalf("UploadResumableMedia: %v", err)
	}
	if handle != "4::abcHANDLE" {
		t.Fatalf("handle = %q", handle)
	}
	if !started || !uploaded {
		t.Fatalf("started=%v uploaded=%v", started, uploaded)
	}
}
