package meta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendInstagramTextMessage(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer ig-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.URL.Path, "/178414000/messages") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"recipient_id": "1028733076273916",
			"message_id":   "mid.abc123",
		})
	}))
	defer srv.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(srv.URL)
	mid, err := client.SendInstagramTextMessage(context.Background(), "178414000", "ig-token", "1028733076273916", "Hello IG", "")
	if err != nil {
		t.Fatal(err)
	}
	if mid != "mid.abc123" {
		t.Fatalf("mid=%q", mid)
	}
	if !strings.Contains(gotBody, `"id":"1028733076273916"`) {
		t.Fatalf("body=%s", gotBody)
	}
	if !strings.Contains(gotBody, `"text":"Hello IG"`) {
		t.Fatalf("body=%s", gotBody)
	}
}

func TestSendInstagramTextMessage_WithReplyTo(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_ = json.NewEncoder(w).Encode(map[string]string{"message_id": "mid.noreply"})
	}))
	defer srv.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(srv.URL)
	mid, err := client.SendInstagramTextMessage(context.Background(), "178414000", "token", "123", "Hi", "mid-original")
	if err != nil {
		t.Fatal(err)
	}
	if mid != "mid.noreply" {
		t.Fatalf("mid=%q", mid)
	}
	if !strings.Contains(gotBody, "reply_to") {
		t.Fatalf("reply_to must be sent to Instagram API: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"messaging_type":"RESPONSE"`) {
		t.Fatalf("messaging_type RESPONSE required for reply: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"mid":"mid-original"`) {
		t.Fatalf("reply mid must be full value: %s", gotBody)
	}
}

func TestSendInstagramMediaMessage_UsesPublicURL(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type=%s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_ = json.NewEncoder(w).Encode(map[string]string{"message_id": "mid.media.1"})
	}))
	defer srv.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(srv.URL)
	mid, err := client.SendInstagramMediaMessage(
		context.Background(),
		"178414000",
		"token",
		"12345",
		"image",
		"https://cdn.example.com/photo.jpg",
		"",
		"mid.parent",
	)
	if err != nil {
		t.Fatal(err)
	}
	if mid != "mid.media.1" {
		t.Fatalf("mid=%q", mid)
	}
	if !strings.Contains(gotBody, `"type":"image"`) {
		t.Fatalf("body=%s", gotBody)
	}
	if !strings.Contains(gotBody, `"attachment"`) {
		t.Fatalf("images must use singular attachment: %s", gotBody)
	}
	if strings.Contains(gotBody, `"attachments"`) {
		t.Fatalf("plural attachments+url is rejected by IG Login API: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"url":"https://cdn.example.com/photo.jpg"`) {
		t.Fatalf("body=%s", gotBody)
	}
	if !strings.Contains(gotBody, `"mid":"mid.parent"`) {
		t.Fatalf("body=%s", gotBody)
	}
}

func TestValidateInstagramOutboundMIME(t *testing.T) {
	if err := ValidateInstagramOutboundMIME("image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstagramOutboundMIME("video/mp4"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstagramOutboundMIME("video/quicktime"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstagramOutboundMIME("image/webp"); err == nil {
		t.Fatal("expected webp rejection")
	}
	if err := ValidateInstagramOutboundMIME("video/3gpp"); err == nil {
		t.Fatal("expected 3gpp rejection")
	}
}

func TestSendInstagramMediaMessage_RejectsLocalhost(t *testing.T) {
	client := NewClient("v21.0")
	_, err := client.SendInstagramMediaMessage(
		context.Background(),
		"178414000",
		"token",
		"12345",
		"image",
		"http://127.0.0.1:8081/media/invalid",
		"",
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "publicly reachable") {
		t.Fatalf("expected public url error, got %v", err)
	}
}
