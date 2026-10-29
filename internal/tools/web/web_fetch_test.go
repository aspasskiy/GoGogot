package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetch(t *testing.T) {
	t.Run("text_plain_passes_through_unchanged", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, "raw plain text body")
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{"url": srv.URL})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if r.Output != "raw plain text body" {
			t.Errorf("got %q, want raw passthrough", r.Output)
		}
	})

	t.Run("application_json_passes_through_unchanged", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"a":1}`)
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{"url": srv.URL})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if r.Output != `{"a":1}` {
			t.Errorf("got %q, want JSON passthrough", r.Output)
		}
	})

	t.Run("html_strips_tags_and_picks_article_by_default", func(t *testing.T) {
		const body = `<html><body>
			<nav>SKIP NAV</nav>
			<article><p>Real content here.</p><p>Second paragraph.</p></article>
			<footer>SKIP FOOTER</footer>
		</body></html>`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, body)
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{"url": srv.URL})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "Real content here.") {
			t.Errorf("missing article content in %q", r.Output)
		}
		if !strings.Contains(r.Output, "Second paragraph.") {
			t.Errorf("missing second paragraph in %q", r.Output)
		}
		if strings.Contains(r.Output, "SKIP NAV") {
			t.Errorf("nav text should not be included; got %q", r.Output)
		}
	})

	t.Run("custom_selector_targets_specific_element", func(t *testing.T) {
		const body = `<html><body>
			<article><p>Article body</p></article>
			<div id="custom"><p>Custom div content</p></div>
		</body></html>`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, body)
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{
			"url":      srv.URL,
			"selector": "#custom",
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "Custom div content") {
			t.Errorf("missing custom div content in %q", r.Output)
		}
		if strings.Contains(r.Output, "Article body") {
			t.Errorf("selector should exclude article; got %q", r.Output)
		}
	})

	t.Run("selector_matching_zero_elements_returns_message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<html><body><p>x</p></body></html>`)
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{
			"url":      srv.URL,
			"selector": ".does-not-exist",
		})
		if r.IsErr {
			t.Errorf("zero matches should not be IsErr")
		}
		if !strings.Contains(r.Output, "matched 0 elements") {
			t.Errorf("got %q, want 'matched 0 elements' message", r.Output)
		}
	})

	t.Run("falls_back_to_body_when_no_main_or_article", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<html><body><p>only body content</p></body></html>`)
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{"url": srv.URL})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "only body content") {
			t.Errorf("missing body content in %q", r.Output)
		}
	})

	t.Run("non_200_status_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		r := webFetch(context.Background(), map[string]any{"url": srv.URL})
		if !r.IsErr {
			t.Errorf("expected error for 404")
		}
		if !strings.Contains(r.Output, "404") {
			t.Errorf("output should mention 404; got %q", r.Output)
		}
	})

	t.Run("missing_url_errors", func(t *testing.T) {
		r := webFetch(context.Background(), map[string]any{})
		if !r.IsErr {
			t.Errorf("expected error for missing url")
		}
	})
}
