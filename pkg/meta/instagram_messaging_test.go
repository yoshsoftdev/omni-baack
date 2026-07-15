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

func TestSendInstagramTextMessage_IgnoresReplyTo(t *testing.T) {
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
	if strings.Contains(gotBody, "reply_to") {
		t.Fatalf("reply_to must not be sent to Instagram API: %s", gotBody)
	}
}
