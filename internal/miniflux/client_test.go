package miniflux

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idealland-apps/Blink/internal/config"
)

func TestListUnreadEntriesUsesTokenAndFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Auth-Token"); got != "token" {
			t.Fatalf("X-Auth-Token = %q", got)
		}
		if r.URL.Path != "/v1/entries" || r.URL.Query().Get("status") != "unread" || r.URL.Query().Get("limit") != "200" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `{"total":1,"entries":[{"id":7,"feed_id":3,"title":"Hello","url":"https://example.test/a","reading_time":2,"published_at":"2026-08-26T10:00:00Z","feed":{"id":3,"title":"Example","category":{"id":4,"title":"AI"}}}]}`)
	}))
	defer server.Close()

	client := New(config.Config{Provider: "miniflux", URL: server.URL, Token: "token"})
	entries, err := client.ListEntries(context.Background(), ListOptions{UnreadOnly: true, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != 7 || entries[0].Feed.Category.Title != "AI" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestListEntriesAcceptsAPIVersionInConfiguredURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/entries" {
			t.Fatalf("path = %q, want /v1/entries", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"total":0,"entries":[]}`)
	}))
	defer server.Close()

	client := New(config.Config{Provider: "miniflux", URL: server.URL + "/v1", Token: "token"})
	if _, err := client.ListEntries(context.Background(), ListOptions{UnreadOnly: true}); err != nil {
		t.Fatal(err)
	}
}

func TestMarkUnreadSendsUnreadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/entries/9" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"status":"unread"`) {
			t.Fatalf("body = %s", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(config.Config{Provider: "miniflux", URL: server.URL, Token: "token"})
	if err := client.MarkUnread(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateEntryUsesBasicAuthAndDoesNotLeakSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "anthony" || password != "password" {
			t.Fatalf("basic auth = %q/%q", user, password)
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/entries/9" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"starred":true`) {
			t.Fatalf("body = %s", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(config.Config{Provider: "miniflux", URL: server.URL, Username: "anthony", Password: "password"})
	if err := client.SetStarred(context.Background(), 9, true); err != nil {
		t.Fatal(err)
	}
}
