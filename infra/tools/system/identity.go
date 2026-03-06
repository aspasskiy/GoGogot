package system

import (
	"context"
	"fmt"
	"time"

	"gogogot/core/store"
	"gogogot/infra/tools"

	"github.com/rs/zerolog/log"
)

// OnTimezoneChange is called after user_write when the timezone in user.md changes.
// Set by the application to reload the scheduler, etc.
var OnTimezoneChange func(loc *time.Location)

func IdentityTools() []tools.Tool {
	return []tools.Tool{
		{
			Name:        "soul_read",
			Description: "Read your soul.md — your personality, values, and behavioral rules. This file defines who you are across all conversations.",
			Parameters:  map[string]any{},
			Handler:     soulRead,
		},
		{
			Name:        "soul_write",
			Description: "Write or update your soul.md — your identity file. Define your personality traits, communication style, core values, and behavioral rules. Read first with soul_read before updating to avoid losing information.",
			Parameters: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The full content for soul.md in markdown format",
				},
			},
			Required: []string{"content"},
			Handler:  soulWrite,
		},
		{
			Name:        "user_read",
			Description: "Read your user.md — everything you know about your owner. This file is loaded into your context automatically.",
			Parameters:  map[string]any{},
			Handler:     userRead,
		},
		{
			Name:        "user_write",
			Description: "Write or update your user.md — your owner's profile. Store their name, preferences, timezone, work context, communication style. Read first with user_read before updating to avoid losing information.",
			Parameters: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The full content for user.md in markdown format",
				},
			},
			Required: []string{"content"},
			Handler:  userWrite,
		},
	}
}

func soulRead(_ context.Context, _ map[string]any) tools.Result {
	content := store.ReadSoul()
	if content == "" {
		return tools.Result{Output: "(soul.md is empty — define your identity)"}
	}
	return tools.Result{Output: content}
}

func soulWrite(_ context.Context, input map[string]any) tools.Result {
	content, err := tools.GetString(input, "content")
	if err != nil {
		return tools.ErrResult(err)
	}
	if err := store.WriteSoul(content); err != nil {
		log.Error().Err(err).Msg("soul_write failed")
		return tools.Result{Output: "error writing soul.md: " + err.Error(), IsErr: true}
	}
	log.Info().Int("content_len", len(content)).Msg("soul_write")
	return tools.Result{Output: fmt.Sprintf("soul.md updated (%d bytes)", len(content))}
}

func userRead(_ context.Context, _ map[string]any) tools.Result {
	content := store.ReadUser()
	if content == "" {
		return tools.Result{Output: "(user.md is empty — learn about your owner)"}
	}
	return tools.Result{Output: content}
}

func userWrite(_ context.Context, input map[string]any) tools.Result {
	content, err := tools.GetString(input, "content")
	if err != nil {
		return tools.ErrResult(err)
	}

	oldTZ := store.LoadTimezone()

	if err := store.WriteUser(content); err != nil {
		log.Error().Err(err).Msg("user_write failed")
		return tools.Result{Output: "error writing user.md: " + err.Error(), IsErr: true}
	}

	newTZ := store.LoadTimezone()
	if newTZ.String() != oldTZ.String() && OnTimezoneChange != nil {
		OnTimezoneChange(newTZ)
		log.Info().Str("from", oldTZ.String()).Str("to", newTZ.String()).Msg("timezone changed via user_write")
	}

	log.Info().Int("content_len", len(content)).Msg("user_write")
	return tools.Result{Output: fmt.Sprintf("user.md updated (%d bytes)", len(content))}
}
