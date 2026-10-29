package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebRequest(t *testing.T) {
	t.Run("get_returns_status_headers_and_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("server saw method %q, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"k":"v"}`)
		}))
		defer srv.Close()

		r := webRequest(context.Background(), map[string]any{"url": srv.URL})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "HTTP 200") {
			t.Errorf("missing status line in %q", r.Output)
		}
		if !strings.Contains(r.Output, "Content-Type: application/json") {
			t.Errorf("missing Content-Type header in %q", r.Output)
		}
		if !strings.Contains(r.Output, `{"k":"v"}`) {
			t.Errorf("missing body in %q", r.Output)
		}
	})

	t.Run("post_with_body_sets_default_content_type", func(t *testing.T) {
		var gotMethod, gotCT, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotCT = r.Header.Get("Content-Type")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		webRequest(context.Background(), map[string]any{
			"url":    srv.URL,
			"method": "post",
			"body":   `{"x":1}`,
		})

		if gotMethod != "POST" {
			t.Errorf("server saw method %q, want POST (uppercased)", gotMethod)
		}
		if gotCT != "application/json" {
			t.Errorf("server saw Content-Type %q, want application/json (default)", gotCT)
		}
		if gotBody != `{"x":1}` {
			t.Errorf("server saw body %q", gotBody)
		}
	})

	t.Run("custom_headers_override_default_content_type", func(t *testing.T) {
		var gotCT, gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCT = r.Header.Get("Content-Type")
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		webRequest(context.Background(), map[string]any{
			"url":    srv.URL,
			"method": "POST",
			"body":   "raw",
			"headers": map[string]any{
				"Content-Type":  "text/plain",
				"Authorization": "Bearer abc",
			},
		})

		if gotCT != "text/plain" {
			t.Errorf("Content-Type override not applied: got %q", gotCT)
		}
		if gotAuth != "Bearer abc" {
			t.Errorf("Authorization header not sent: got %q", gotAuth)
		}
	})

	t.Run("default_user_agent_is_set", func(t *testing.T) {
		var gotUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		webRequest(context.Background(), map[string]any{"url": srv.URL})
		if !strings.Contains(gotUA, "SofieBot") {
			t.Errorf("User-Agent %q missing SofieBot", gotUA)
		}
	})

	t.Run("4xx_response_marks_iserr_but_keeps_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, "missing resource")
		}))
		defer srv.Close()

		r := webRequest(context.Background(), map[string]any{"url": srv.URL})
		if !r.IsErr {
			t.Errorf("4xx should set IsErr=true")
		}
		if !strings.Contains(r.Output, "HTTP 404") {
			t.Errorf("missing HTTP 404 in %q", r.Output)
		}
		if !strings.Contains(r.Output, "missing resource") {
			t.Errorf("body should still be included in %q", r.Output)
		}
	})

	t.Run("missing_url_errors", func(t *testing.T) {
		r := webRequest(context.Background(), map[string]any{})
		if !r.IsErr {
			t.Errorf("expected error for missing url")
		}
	})

	t.Run("malformed_url_errors", func(t *testing.T) {
		r := webRequest(context.Background(), map[string]any{"url": "://not-a-url"})
		if !r.IsErr {
			t.Errorf("expected error for malformed url")
		}
	})
}
