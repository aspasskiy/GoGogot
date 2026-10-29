package types

import (
	"errors"
	"strings"
	"testing"
)

func TestErrResult(t *testing.T) {
	r := ErrResult(errors.New("boom"))
	if !r.IsErr {
		t.Errorf("ErrResult IsErr = false, want true")
	}
	if r.Output != "boom" {
		t.Errorf("ErrResult Output = %q, want %q", r.Output, "boom")
	}
}

func TestErrf(t *testing.T) {
	r := Errf("code %d: %s", 42, "oops")
	if !r.IsErr {
		t.Errorf("Errf IsErr = false, want true")
	}
	if r.Output != "code 42: oops" {
		t.Errorf("Errf Output = %q, want %q", r.Output, "code 42: oops")
	}
}

func TestTruncateOutput(t *testing.T) {
	t.Run("short_passes_through_unchanged", func(t *testing.T) {
		r := TruncateOutput("hello world")
		if r.IsErr {
			t.Errorf("IsErr = true, want false")
		}
		if r.Output != "hello world" {
			t.Errorf("Output = %q, want %q", r.Output, "hello world")
		}
	})

	t.Run("at_limit_passes_through_unchanged", func(t *testing.T) {
		s := strings.Repeat("a", MaxOutputSize)
		r := TruncateOutput(s)
		if r.Output != s {
			t.Errorf("at-limit input was modified; got len %d, want %d", len(r.Output), len(s))
		}
	})

	t.Run("over_limit_is_truncated_with_marker", func(t *testing.T) {
		s := strings.Repeat("a", MaxOutputSize+100)
		r := TruncateOutput(s)
		if !strings.HasSuffix(r.Output, "\n... (truncated)") {
			t.Errorf("expected truncation marker; got tail %q", r.Output[len(r.Output)-30:])
		}
		if !strings.HasPrefix(r.Output, strings.Repeat("a", 64)) {
			t.Errorf("expected leading 'a's preserved")
		}
		want := MaxOutputSize + len("\n... (truncated)")
		if len(r.Output) != want {
			t.Errorf("truncated len = %d, want %d", len(r.Output), want)
		}
	})
}

func TestGetString(t *testing.T) {
	t.Run("present_string_returned", func(t *testing.T) {
		v, err := GetString(map[string]any{"k": "v"}, "k")
		if err != nil || v != "v" {
			t.Errorf("got (%q,%v); want (\"v\",nil)", v, err)
		}
	})

	t.Run("missing_key_errors", func(t *testing.T) {
		_, err := GetString(map[string]any{}, "missing")
		if err == nil {
			t.Errorf("expected error for missing key")
		}
	})

	t.Run("empty_string_errors", func(t *testing.T) {
		_, err := GetString(map[string]any{"k": ""}, "k")
		if err == nil {
			t.Errorf("expected error for empty string")
		}
	})

	t.Run("non_string_type_errors", func(t *testing.T) {
		_, err := GetString(map[string]any{"k": 42}, "k")
		if err == nil {
			t.Errorf("expected error for non-string value")
		}
	})
}

func TestGetStringOpt(t *testing.T) {
	if got := GetStringOpt(map[string]any{"k": "v"}, "k"); got != "v" {
		t.Errorf("present = %q, want %q", got, "v")
	}
	if got := GetStringOpt(map[string]any{}, "k"); got != "" {
		t.Errorf("missing = %q, want empty", got)
	}
	if got := GetStringOpt(map[string]any{"k": 42}, "k"); got != "" {
		t.Errorf("non-string = %q, want empty", got)
	}
}

func TestGetInt(t *testing.T) {
	t.Run("float64_returned_as_int", func(t *testing.T) {
		v, err := GetInt(map[string]any{"k": float64(42)}, "k")
		if err != nil || v != 42 {
			t.Errorf("got (%d,%v); want (42,nil)", v, err)
		}
	})

	t.Run("missing_key_errors", func(t *testing.T) {
		_, err := GetInt(map[string]any{}, "k")
		if err == nil {
			t.Errorf("expected error for missing key")
		}
	})

	t.Run("native_int_errors_per_json_convention", func(t *testing.T) {
		_, err := GetInt(map[string]any{"k": 42}, "k")
		if err == nil {
			t.Errorf("native int should error (JSON unmarshal yields float64)")
		}
	})

	t.Run("float_truncates_toward_zero", func(t *testing.T) {
		v, _ := GetInt(map[string]any{"k": float64(3.9)}, "k")
		if v != 3 {
			t.Errorf("got %d, want 3 (truncated)", v)
		}
	})
}

func TestGetBool(t *testing.T) {
	if !GetBool(map[string]any{"k": true}, "k") {
		t.Errorf("true value = false")
	}
	if GetBool(map[string]any{"k": false}, "k") {
		t.Errorf("false value = true")
	}
	if GetBool(map[string]any{}, "k") {
		t.Errorf("missing = true, want false")
	}
	if GetBool(map[string]any{"k": "true"}, "k") {
		t.Errorf("string \"true\" = true, want false (strict bool only)")
	}
}
