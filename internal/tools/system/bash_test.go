package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteBash(t *testing.T) {
	t.Run("returns_stdout_for_simple_command", func(t *testing.T) {
		r := executeBash(context.Background(), map[string]any{"command": "echo hello"})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "hello") {
			t.Errorf("output %q missing 'hello'", r.Output)
		}
	})

	t.Run("captures_stderr_in_combined_output", func(t *testing.T) {
		r := executeBash(context.Background(), map[string]any{"command": "echo to-stderr 1>&2"})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "to-stderr") {
			t.Errorf("output %q missing stderr text", r.Output)
		}
	})

	t.Run("nonzero_exit_marks_iserr", func(t *testing.T) {
		r := executeBash(context.Background(), map[string]any{"command": "exit 7"})
		if !r.IsErr {
			t.Errorf("expected IsErr=true for nonzero exit")
		}
		if !strings.Contains(r.Output, "exit") {
			t.Errorf("output %q should mention exit", r.Output)
		}
	})

	t.Run("workdir_is_respected", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "sentinel.txt")
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		r := executeBash(context.Background(), map[string]any{
			"command": "ls",
			"workdir": dir,
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "sentinel.txt") {
			t.Errorf("ls in %s missing sentinel.txt; got %q", dir, r.Output)
		}
	})

	t.Run("missing_command_arg_errors", func(t *testing.T) {
		r := executeBash(context.Background(), map[string]any{})
		if !r.IsErr {
			t.Errorf("expected error for missing command")
		}
	})

	t.Run("empty_output_replaced_with_no_output_marker", func(t *testing.T) {
		r := executeBash(context.Background(), map[string]any{"command": "true"})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if r.Output != "(no output)" {
			t.Errorf("got %q, want '(no output)'", r.Output)
		}
	})

	t.Run("timeout_kills_long_command_and_returns_error", func(t *testing.T) {
		start := time.Now()
		r := executeBash(context.Background(), map[string]any{
			"command": "sleep 5",
			"timeout": float64(1),
		})
		elapsed := time.Since(start)
		if !r.IsErr {
			t.Errorf("expected IsErr=true for timeout")
		}
		if !strings.Contains(r.Output, "timed out") {
			t.Errorf("output %q missing 'timed out'", r.Output)
		}
		if elapsed > 4*time.Second {
			t.Errorf("timeout did not fire promptly; elapsed=%s", elapsed)
		}
	})

	t.Run("excessive_timeout_is_capped_at_max", func(t *testing.T) {
		// Use a quick command — we only verify the cap doesn't reject the call.
		r := executeBash(context.Background(), map[string]any{
			"command": "echo ok",
			"timeout": float64(99999),
		})
		if r.IsErr {
			t.Fatalf("unexpected error: %s", r.Output)
		}
		if !strings.Contains(r.Output, "ok") {
			t.Errorf("output %q missing 'ok'", r.Output)
		}
	})
}
