package meta

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeleteMessageTemplateQuery(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotMethod string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod = req.Method
		gotPath = req.URL.Path
		gotQuery = req.URL.RawQuery
		gotAuth = req.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	client := &Client{
		httpClient: &http.Client{Transport: transport},
		apiVersion: "v21.0",
	}
	if err := client.DeleteMessageTemplate(context.Background(), "waba-1", "token-1", "promo_july", "999"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %s", gotMethod)
	}
	if gotPath != "/v21.0/waba-1/message_templates" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotQuery != "hsm_id=999&name=promo_july" && gotQuery != "name=promo_july&hsm_id=999" {
		t.Fatalf("query = %s", gotQuery)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("auth = %s", gotAuth)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
