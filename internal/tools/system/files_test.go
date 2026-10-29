package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathDetail(t *testing.T) {
	t.Run("returns_basename_of_path", func(t *testing.T) {
		got := pathDetail(map[string]any{"path": "/a/b/c/file.txt"})
		if got != "file.txt" {
			t.Errorf("got %q, want %q", got, "file.txt")
		}
	})

	t.Run("missing_path_returns_empty", func(t *testing.T) {
		if got := pathDetail(map[string]any{}); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("non_string_path_returns_empty", func(t *testing.T) {
		if got := pathDetail(map[string]any{"path": 42}); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hi there"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("returns_file_contents", func(t *testing.T) {
		r := readFile(context.Background(), map[string]any{"path": path})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if r.Output != "hi there" {
			t.Errorf("got %q, want %q", r.Output, "hi there")
		}
	})

	t.Run("missing_path_arg_errors", func(t *testing.T) {
		r := readFile(context.Background(), map[string]any{})
		if !r.IsErr {
			t.Errorf("expected error for missing path")
		}
	})

	t.Run("nonexistent_file_errors", func(t *testing.T) {
		r := readFile(context.Background(), map[string]any{"path": filepath.Join(dir, "nope")})
		if !r.IsErr {
			t.Errorf("expected error for missing file")
		}
		if !strings.Contains(r.Output, "read error") {
			t.Errorf("expected 'read error' prefix; got %q", r.Output)
		}
	})
}

func TestWriteFile(t *testing.T) {
	t.Run("writes_content_and_reports_byte_count", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")
		r := writeFile(context.Background(), map[string]any{
			"path":    path,
			"content": "hello",
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "wrote 5 bytes") {
			t.Errorf("output missing byte count: %q", r.Output)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "hello" {
			t.Errorf("file contents = %q (%v), want %q", got, err, "hello")
		}
	})

	t.Run("creates_parent_directories", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "a", "b", "c", "out.txt")
		r := writeFile(context.Background(), map[string]any{
			"path":    nested,
			"content": "nested",
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if _, err := os.Stat(nested); err != nil {
			t.Errorf("expected nested file created; stat err: %v", err)
		}
	})

	t.Run("empty_content_writes_zero_bytes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")
		r := writeFile(context.Background(), map[string]any{"path": path, "content": ""})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() != 0 {
			t.Errorf("size = %d (%v), want 0", info.Size(), err)
		}
	})

	t.Run("missing_path_errors", func(t *testing.T) {
		r := writeFile(context.Background(), map[string]any{"content": "x"})
		if !r.IsErr {
			t.Errorf("expected error for missing path")
		}
	})
}

func TestListFiles(t *testing.T) {
	t.Run("lists_files_with_dir_suffix", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644)
		os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

		r := listFiles(context.Background(), map[string]any{"path": dir})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "a.txt\n") {
			t.Errorf("missing a.txt in %q", r.Output)
		}
		if !strings.Contains(r.Output, "subdir/\n") {
			t.Errorf("missing subdir/ marker in %q", r.Output)
		}
	})

	t.Run("empty_directory_reports_empty", func(t *testing.T) {
		dir := t.TempDir()
		r := listFiles(context.Background(), map[string]any{"path": dir})
		if r.Output != "(empty directory)" {
			t.Errorf("got %q, want '(empty directory)'", r.Output)
		}
	})

	t.Run("nonexistent_path_errors", func(t *testing.T) {
		r := listFiles(context.Background(), map[string]any{"path": "/nope/does/not/exist"})
		if !r.IsErr {
			t.Errorf("expected error for missing dir")
		}
	})
}
