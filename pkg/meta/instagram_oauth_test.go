package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildInstagramAuthorizeURL(t *testing.T) {
	got := BuildInstagramAuthorizeURL("app-id", "https://api.example/callback", "state-1", "")
	if !strings.Contains(got, "www.instagram.com/oauth/authorize") {
		t.Fatalf("unexpected url: %s", got)
	}
	for _, part := range []string{
		"client_id=app-id",
		"redirect_uri=",
		"response_type=code",
		"force_reauth=true",
		"scope=instagram_business_basic",
		"state=state-1",
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %s", part, got)
		}
	}
}

func TestValidateInstagramAccountType(t *testing.T) {
	if err := ValidateInstagramAccountType("BUSINESS"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstagramAccountType("MEDIA_CREATOR"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstagramAccountType("PERSONAL"); err == nil {
		t.Fatal("expected personal rejection")
	}
}

func TestParseInstagramCodeExchange_FlatNumericUserID(t *testing.T) {
	body := []byte(`{"access_token":"IGQVSx","user_id":17841400012345,"permissions":["instagram_business_basic","instagram_business_manage_messages"]}`)
	parsed, err := parseInstagramCodeExchange(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.AccessToken != "IGQVSx" || parsed.UserID != "17841400012345" {
		t.Fatalf("unexpected parse: %+v", parsed)
	}
	if !strings.Contains(parsed.Permissions, "instagram_business_basic") {
		t.Fatalf("unexpected permissions: %q", parsed.Permissions)
	}
}

func TestParseInstagramCodeExchange_Wrapped(t *testing.T) {
	body := []byte(`{"data":[{"access_token":"tok","user_id":"99","permissions":"instagram_business_basic"}]}`)
	parsed, err := parseInstagramCodeExchange(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.AccessToken != "tok" || parsed.UserID != "99" {
		t.Fatalf("unexpected parse: %+v", parsed)
	}
}

func TestRedactSensitiveJSON(t *testing.T) {
	out := redactSensitiveJSON([]byte(`{"access_token":"supersecrettoken","user_id":"1"}`))
	if strings.Contains(out, "supersecrettoken") {
		t.Fatalf("token leaked: %s", out)
	}
	if !strings.Contains(out, "user_id") {
		t.Fatalf("expected user_id retained: %s", out)
	}
}

func TestGetInstagramMediaPermalink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.URL.Path, "/18139494541428835") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"permalink": "https://www.instagram.com/p/ABC123/",
		})
	}))
	defer srv.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(srv.URL)
	permalink, err := client.GetInstagramMediaPermalink(context.Background(), "18139494541428835", "good-token")
	if err != nil {
		t.Fatal(err)
	}
	if permalink != "https://www.instagram.com/p/ABC123/" {
		t.Fatalf("permalink=%q", permalink)
	}
}

func TestSubscribeInstagramApp(t *testing.T) {
	var gotPath string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer srv.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(srv.URL)
	if err := client.SubscribeInstagramApp(context.Background(), "178414000", "good-token", "messages,comments"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/178414000/subscribed_apps" {
		t.Fatalf("path = %s", gotPath)
	}
	if !strings.Contains(gotQuery, "subscribed_fields=messages") {
		t.Fatalf("query = %s", gotQuery)
	}
}

func TestGetInstagramConversationParticipant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/17841474159701018/conversations") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("user_id") != "1750921879279189" {
			http.Error(w, "bad user", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"participants": map[string]any{
					"data": []map[string]string{
						{"id": "17841474159701018", "username": "visibel.ai"},
						{"id": "1750921879279189", "username": "syaamrizal"},
					},
				},
			}},
		})
	}))
	defer srv.Close()

	client := NewClient("v21.0")
	client.SetBaseURL(srv.URL)
	got, err := client.GetInstagramConversationParticipant(
		context.Background(), "17841474159701018", "1750921879279189", "good-token",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "syaamrizal" || got.ID != "1750921879279189" {
		t.Fatalf("got %#v", got)
	}
}
