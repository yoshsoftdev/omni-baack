package meta_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestListMessageTemplatesPagination(t *testing.T) {
	pages := []string{
		`{"data":[{"id":"1","name":"a","language":"en_US","status":"APPROVED","components":[]}],"paging":{"after":"cursor-2"}}`,
		`{"data":[{"id":"2","name":"b","language":"en_US","status":"APPROVED","components":[]}],"paging":{"after":"cursor-3"}}`,
		`{"data":[{"id":"3","name":"c","language":"en_US","status":"APPROVED","components":[]}],"paging":{}}`,
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls >= len(pages) {
			t.Fatalf("unexpected extra page request")
		}
		_, _ = w.Write([]byte(pages[calls]))
		calls++
	}))
	defer server.Close()

	client := metapkg.NewClient("v21.0")
	client.SetBaseURL(server.URL + "/")

	templates, err := client.ListMessageTemplates(context.Background(), "waba", "token")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(templates) != 3 || calls != 3 {
		t.Fatalf("templates=%d calls=%d", len(templates), calls)
	}
}

func TestListMessageTemplatesPageDecodesAfterCursor(t *testing.T) {
	body := `{"data":[{"id":"1","name":"a","language":"en_US","status":"APPROVED","components":[]}],"paging":{"after":"next-cursor"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after") != "prev-cursor" {
			t.Fatalf("after=%s", r.URL.Query().Get("after"))
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := metapkg.NewClient("v21.0")
	client.SetBaseURL(server.URL + "/")
	page, next, err := client.ListMessageTemplatesPage(context.Background(), "waba", "token", "prev-cursor")
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(page) != 1 || next != "next-cursor" {
		t.Fatalf("page=%d next=%s", len(page), next)
	}
	var decoded metapkg.MessageTemplate
	if err := json.Unmarshal([]byte(`{"id":"1","name":"a","language":"en_US","status":"APPROVED","components":[]}`), &decoded); err != nil {
		t.Fatal(err)
	}
}
