package catalog

import "testing"

func TestOpenAI_LoadsKnownModels(t *testing.T) {
	m := OpenAI()
	if len(m) == 0 {
		t.Fatalf("OpenAI() returned empty map")
	}
	def, ok := m["gpt-5.4"]
	if !ok {
		t.Fatalf("expected gpt-5.4 in OpenAI catalog; got %d models", len(m))
	}
	if def.ID != "gpt-5.4" || def.Label == "" {
		t.Errorf("gpt-5.4 entry malformed: %+v", def)
	}
	if def.ContextWindow <= 0 {
		t.Errorf("gpt-5.4 ContextWindow should be positive, got %d", def.ContextWindow)
	}
}

func TestAnthropic_LoadsKnownModels(t *testing.T) {
	m := Anthropic()
	if len(m) == 0 {
		t.Fatalf("Anthropic() returned empty map")
	}
	def, ok := m["claude-opus-4-6"]
	if !ok {
		t.Fatalf("expected claude-opus-4-6 in Anthropic catalog; got %d models", len(m))
	}
	if def.ContextWindow != 200000 {
		t.Errorf("claude-opus-4-6 ContextWindow = %d, want 200000", def.ContextWindow)
	}
	if !def.Vision {
		t.Errorf("claude-opus-4-6 should be vision-capable")
	}
	if def.InputPricePerM <= 0 || def.OutputPricePerM <= 0 {
		t.Errorf("claude-opus-4-6 pricing should be positive: in=%v out=%v",
			def.InputPricePerM, def.OutputPricePerM)
	}
}

func TestOpenRouter_LoadsAndNormalizes(t *testing.T) {
	m := OpenRouter()
	if len(m) == 0 {
		t.Fatalf("OpenRouter() returned empty map")
	}
	// OpenRouter pricing strings get scaled by 1M; verify at least one
	// entry has finite, non-negative pricing after normalization.
	var sawFinite bool
	for _, def := range m {
		if def.ID == "" {
			t.Errorf("OpenRouter entry with empty ID: %+v", def)
		}
		if def.InputPricePerM >= 0 && def.OutputPricePerM >= 0 {
			sawFinite = true
		}
	}
	if !sawFinite {
		t.Errorf("expected at least one OpenRouter entry with finite pricing")
	}
}

func TestOpenRouter_VisionDetection(t *testing.T) {
	m := OpenRouter()
	// At least one model in the catalog should declare image input.
	var sawVision bool
	for _, def := range m {
		if def.Vision {
			sawVision = true
			break
		}
	}
	if !sawVision {
		t.Errorf("expected at least one vision-capable OpenRouter model")
	}
}
