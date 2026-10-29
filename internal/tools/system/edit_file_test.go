package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFile(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		return path
	}

	t.Run("first_occurrence_replaced_by_default", func(t *testing.T) {
		path := write(t, "foo bar foo")
		r := editFile(context.Background(), map[string]any{
			"path":       path,
			"old_string": "foo",
			"new_string": "X",
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "replaced 1") {
			t.Errorf("output = %q, want 'replaced 1'", r.Output)
		}
		got, _ := os.ReadFile(path)
		if string(got) != "X bar foo" {
			t.Errorf("file = %q, want %q", got, "X bar foo")
		}
	})

	t.Run("replace_all_swaps_every_occurrence", func(t *testing.T) {
		path := write(t, "foo bar foo baz foo")
		r := editFile(context.Background(), map[string]any{
			"path":        path,
			"old_string":  "foo",
			"new_string":  "X",
			"replace_all": true,
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "replaced 3") {
			t.Errorf("output = %q, want 'replaced 3'", r.Output)
		}
		got, _ := os.ReadFile(path)
		if string(got) != "X bar X baz X" {
			t.Errorf("file = %q, want %q", got, "X bar X baz X")
		}
	})

	t.Run("missing_old_string_returns_iserr_and_does_not_modify_file", func(t *testing.T) {
		path := write(t, "alpha beta")
		r := editFile(context.Background(), map[string]any{
			"path":       path,
			"old_string": "gamma",
			"new_string": "delta",
		})
		if !r.IsErr {
			t.Errorf("expected IsErr=true for missing old_string")
		}
		got, _ := os.ReadFile(path)
		if string(got) != "alpha beta" {
			t.Errorf("file modified to %q, expected unchanged", got)
		}
	})

	t.Run("nonexistent_file_errors", func(t *testing.T) {
		r := editFile(context.Background(), map[string]any{
			"path":       "/nope/missing.txt",
			"old_string": "a",
			"new_string": "b",
		})
		if !r.IsErr {
			t.Errorf("expected error for missing file")
		}
	})

	t.Run("missing_required_args_error", func(t *testing.T) {
		path := write(t, "x")
		cases := []map[string]any{
			{"old_string": "a", "new_string": "b"},
			{"path": path, "new_string": "b"},
			{"path": path, "old_string": "a"},
		}
		for i, in := range cases {
			r := editFile(context.Background(), in)
			if !r.IsErr {
				t.Errorf("case %d expected error; got Output=%q", i, r.Output)
			}
		}
	})

	t.Run("new_string_can_be_empty_for_deletion", func(t *testing.T) {
		path := write(t, "delete-me-please")
		r := editFile(context.Background(), map[string]any{
			"path":       path,
			"old_string": "delete-me-please",
			"new_string": "",
		})
		if !r.IsErr {
			t.Errorf("empty new_string should error per GetString contract; got %q", r.Output)
		}
	})
}
