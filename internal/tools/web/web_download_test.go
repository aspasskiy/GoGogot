package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebDownload(t *testing.T) {
	t.Run("saves_body_to_explicit_path_and_reports_size", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			io.WriteString(w, "1234567890")
		}))
		defer srv.Close()

		dest := filepath.Join(t.TempDir(), "out.bin")
		r := webDownload(context.Background(), map[string]any{
			"url":  srv.URL,
			"path": dest,
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, dest) {
			t.Errorf("output should mention dest path %q; got %q", dest, r.Output)
		}
		if !strings.Contains(r.Output, "Content-Type: application/octet-stream") {
			t.Errorf("output missing Content-Type line: %q", r.Output)
		}
		got, err := os.ReadFile(dest)
		if err != nil || string(got) != "1234567890" {
			t.Errorf("downloaded file = %q (%v)", got, err)
		}
	})

	t.Run("creates_parent_directories", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "x")
		}))
		defer srv.Close()

		dest := filepath.Join(t.TempDir(), "deep", "a", "b", "file.txt")
		r := webDownload(context.Background(), map[string]any{
			"url":  srv.URL,
			"path": dest,
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if _, err := os.Stat(dest); err != nil {
			t.Errorf("expected nested file: %v", err)
		}
	})

	t.Run("non_200_response_errors_and_no_file_left_behind", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		dest := filepath.Join(t.TempDir(), "should-not-exist.bin")
		r := webDownload(context.Background(), map[string]any{
			"url":  srv.URL,
			"path": dest,
		})
		if !r.IsErr {
			t.Errorf("expected error for 500")
		}
		if _, err := os.Stat(dest); err == nil {
			t.Errorf("file should not be created on non-200 response")
		}
	})

	t.Run("default_path_uses_url_basename_under_tmp", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "data")
		}))
		defer srv.Close()

		// URL ends in /test-download-XYZ.bin so we can verify dest naming.
		u, _ := url.Parse(srv.URL)
		u.Path = "/test-download-phaseB-fixture.bin"

		expected := "/tmp/test-download-phaseB-fixture.bin"
		defer os.Remove(expected)

		r := webDownload(context.Background(), map[string]any{"url": u.String()})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, expected) {
			t.Errorf("output should mention %q; got %q", expected, r.Output)
		}
		got, err := os.ReadFile(expected)
		if err != nil || string(got) != "data" {
			t.Errorf("file at %q = %q (%v)", expected, got, err)
		}
	})

	t.Run("missing_url_errors", func(t *testing.T) {
		r := webDownload(context.Background(), map[string]any{})
		if !r.IsErr {
			t.Errorf("expected error for missing url")
		}
	})
}
