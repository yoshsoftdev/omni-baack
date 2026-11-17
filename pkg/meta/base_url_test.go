package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Every Graph call must honour SetBaseURL (META_GRAPH_BASE_URL); these two used to hardcode graph.facebook.com.
func TestSetBaseURL_AppliesToPhoneLookupAndSend(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.FAKE"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"display_phone_number":"62811"}`))
	}))
	defer srv.Close()

	c := NewClient("v21.0")
	c.SetBaseURL(srv.URL + "/")
	if _, err := c.GetPhoneNumber(context.Background(), "123", "tok"); err != nil {
		t.Fatal(err)
	}
	id, err := c.SendTextMessage(context.Background(), "123", "tok", "62812", "hi")
	if err != nil || id != "wamid.FAKE" {
		t.Fatalf("send = %q, %v", id, err)
	}
	want := []string{"GET /v21.0/123", "POST /v21.0/123/messages"}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}
