package prompt

import (
	"strings"
	"testing"
	"time"
)

func TestSystemPrompt_ContainsCoreSections(t *testing.T) {
	got := SystemPrompt(PromptContext{
		TransportName: "telegram",
		ModelLabel:    "Claude Opus 4.6",
		Soul:          "I am a friendly assistant.",
		User:          "name: Alice\ntimezone: UTC",
	})
	// Section labels are stable contracts the agent relies on.
	wantContains := []string{
		"SAFETY:",
		"TOOL CALL STYLE:",
		"HOW TO WORK:",
		"USER INTERACTION:",
		"IDENTITY:",
		"MEMORY:",
		"CONVERSATION HISTORY:",
		"SKILLS:",
		"SELF-LEARNING:",
		"SYSTEM ACCESS:",
		"SELF-SCHEDULING:",
		"DOCKER AWARENESS:",
		"RUNTIME:",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("SystemPrompt missing section %q", s)
		}
	}
}

func TestSystemPrompt_SubstitutesTransportAndModel(t *testing.T) {
	got := SystemPrompt(PromptContext{
		TransportName: "discord",
		ModelLabel:    "Claude Sonnet 4.6",
	})
	if !strings.Contains(got, "discord") {
		t.Errorf("expected TransportName 'discord' in prompt, got:\n%s", got)
	}
	if !strings.Contains(got, "Claude Sonnet 4.6") {
		t.Errorf("expected ModelLabel in RUNTIME section, got:\n%s", got)
	}
}

func TestSystemPrompt_SanitizesNewlinesInContext(t *testing.T) {
	got := SystemPrompt(PromptContext{
		TransportName: "tele\ngram\rspoof",
		ModelLabel:    "model\nwith\nnewlines",
	})
	// Newlines inside injected context values must be stripped so the
	// prompt structure can't be hijacked by attacker-controlled metadata.
	if strings.Contains(got, "tele\ngram") || strings.Contains(got, "tele\rgram") {
		t.Errorf("TransportName newlines were not sanitized")
	}
	if strings.Contains(got, "model\nwith") {
		t.Errorf("ModelLabel newlines were not sanitized")
	}
}

func TestSystemPrompt_TimezoneDefaultsToUTC(t *testing.T) {
	got := SystemPrompt(PromptContext{TransportName: "x"})
	// With nil Timezone the runtime line should still render — the default
	// is time.UTC. Verify by formatting the same instant in UTC and
	// checking that "UTC" appears in the runtime section.
	if !strings.Contains(got, "RUNTIME:") {
		t.Fatalf("missing RUNTIME section")
	}
	if !strings.Contains(got, "UTC") {
		t.Errorf("expected 'UTC' in default-timezone runtime, got:\n%s", got)
	}
}

func TestSystemPrompt_TimezoneOverride(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York timezone unavailable: %v", err)
	}
	got := SystemPrompt(PromptContext{TransportName: "x", Timezone: loc})
	// The runtime line uses time.Format("2006-01-02 15:04 MST"), so the
	// timezone abbreviation (EST or EDT) should appear.
	if !strings.Contains(got, "EST") && !strings.Contains(got, "EDT") {
		t.Errorf("expected EST/EDT abbreviation in NY timezone runtime, got:\n%s", got)
	}
}

func TestSystemPrompt_InjectsSoulAndUserBlocks(t *testing.T) {
	got := SystemPrompt(PromptContext{
		TransportName: "x",
		Soul:          "I am Sage.",
		User:          "name: Bob",
	})
	if !strings.Contains(got, "<soul>") || !strings.Contains(got, "I am Sage.") || !strings.Contains(got, "</soul>") {
		t.Errorf("expected <soul>...I am Sage....</soul> block, got:\n%s", got)
	}
	if !strings.Contains(got, "<user>") || !strings.Contains(got, "name: Bob") || !strings.Contains(got, "</user>") {
		t.Errorf("expected <user>...name: Bob...</user> block, got:\n%s", got)
	}
	// Should NOT prompt for identity setup when both are supplied.
	if strings.Contains(got, "IDENTITY SETUP:") {
		t.Errorf("did not expect IDENTITY SETUP when soul and user are both provided")
	}
}

func TestSystemPrompt_IdentitySetupWhenSoulAndUserMissing(t *testing.T) {
	got := SystemPrompt(PromptContext{TransportName: "x"})
	if !strings.Contains(got, "IDENTITY SETUP:") {
		t.Errorf("expected IDENTITY SETUP block when soul/user empty, got:\n%s", got)
	}
	if !strings.Contains(got, "soul.md is empty") {
		t.Errorf("expected soul.md guidance in identity setup")
	}
	if !strings.Contains(got, "user.md is empty") {
		t.Errorf("expected user.md guidance in identity setup")
	}
}

func TestSystemPrompt_SkillsBlockEmptyVsPopulated(t *testing.T) {
	gotEmpty := SystemPrompt(PromptContext{TransportName: "x"})
	if !strings.Contains(gotEmpty, "No skills are currently available.") {
		t.Errorf("expected empty-skills message, got:\n%s", gotEmpty)
	}

	gotWith := SystemPrompt(PromptContext{
		TransportName: "x",
		SkillsBlock:   "<available_skills>\n<skill>foo</skill>\n</available_skills>",
	})
	if !strings.Contains(gotWith, "<available_skills>") {
		t.Errorf("expected SkillsBlock to be embedded, got:\n%s", gotWith)
	}
	if strings.Contains(gotWith, "No skills are currently available.") {
		t.Errorf("did not expect empty-skills message when SkillsBlock is provided")
	}
}

func TestSystemPrompt_NoNameClauseWhenSoulEmpty(t *testing.T) {
	got := SystemPrompt(PromptContext{TransportName: "x"})
	if !strings.Contains(got, "no name or identity yet") {
		t.Errorf("expected no-name-yet clause in core identity when Soul empty, got:\n%s", got)
	}

	gotWithSoul := SystemPrompt(PromptContext{TransportName: "x", Soul: "I am Sage."})
	if strings.Contains(gotWithSoul, "no name or identity yet") {
		t.Errorf("did NOT expect no-name clause when Soul is set")
	}
}

func TestScheduledTaskPrompt_WithoutSkill(t *testing.T) {
	got := ScheduledTaskPrompt("morning-news", "compile a digest", "")
	if !strings.Contains(got, "[Scheduled Task: morning-news]") {
		t.Errorf("missing task ID header: %s", got)
	}
	if !strings.Contains(got, "compile a digest") {
		t.Errorf("missing command body: %s", got)
	}
	if strings.Contains(got, "Skill:") {
		t.Errorf("did not expect Skill: line when skill is empty: %s", got)
	}
}

func TestScheduledTaskPrompt_WithSkill(t *testing.T) {
	got := ScheduledTaskPrompt("daily", "run report", "morning-report")
	if !strings.Contains(got, "Skill:") {
		t.Errorf("expected Skill: line when skill provided: %s", got)
	}
	if !strings.Contains(got, "morning-report") {
		t.Errorf("expected skill name to appear: %s", got)
	}
}
