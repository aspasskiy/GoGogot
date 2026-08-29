package llm

import (
	"testing"
)

func TestResolveRequesty(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "test-key")

	// A raw vendor/model slug should resolve through Requesty.
	p, err := ResolveProvider("openai/gpt-4o-mini", "requesty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.BaseURL != "https://router.requesty.ai/v1" {
		t.Errorf("BaseURL = %q, want https://router.requesty.ai/v1", p.BaseURL)
	}
	if p.Model != "openai/gpt-4o-mini" {
		t.Errorf("Model = %q, want openai/gpt-4o-mini", p.Model)
	}
	if p.Format != "openai" {
		t.Errorf("Format = %q, want openai", p.Format)
	}
	if p.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", p.APIKey)
	}
}

func TestResolveRequestyMissingKey(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "")
	if _, err := ResolveProvider("openai/gpt-4o-mini", "requesty"); err == nil {
		t.Fatal("expected error when REQUESTY_API_KEY is unset")
	}
}

func TestResolveRequestyBareModel(t *testing.T) {
	t.Setenv("REQUESTY_API_KEY", "test-key")
	// A bare (non-slug) model that is not in a known catalog should error.
	if _, err := ResolveProvider("not-a-real-model", "requesty"); err == nil {
		t.Fatal("expected error for unknown bare model")
	}
}
