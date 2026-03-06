package system

import (
	"context"
	"fmt"
	"strings"

	"gogogot/core/skills"
	"gogogot/core/store"
	"gogogot/infra/tools"

	"github.com/rs/zerolog/log"
)

func SkillTools() []tools.Tool {
	return []tools.Tool{
		{
			Name:        "skill_list",
			Description: "List all available skills with their name and description. Skills are reusable procedural knowledge — workflows, integrations, how-to guides — that you created from past experience.",
			Parameters:  map[string]any{},
			Handler:     skillList,
		},
		{
			Name: "skill_read",
			Description: "Read the full content of a skill's SKILL.md. Use this when a skill matches the current task — " +
				"read it first, then follow the instructions inside.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (lowercase, hyphen-separated)",
				},
			},
			Required: []string{"name"},
			Handler:  skillRead,
		},
		{
			Name: "skill_create",
			Description: "Create a new skill to capture procedural knowledge for future reuse. " +
				"Use after solving a non-trivial problem — save the workflow so you can do it better next time. " +
				"The skill will appear in your available_skills on future conversations.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Short lowercase name with hyphens, e.g. deploy-docker, morning-digest",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "When to use this skill — include triggers and context so you can match it later",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Markdown instructions: commands, steps, examples, gotchas",
				},
			},
			Required: []string{"name", "description", "body"},
			Handler:  skillCreate,
		},
		{
			Name: "skill_update",
			Description: "Update an existing skill's SKILL.md with new content. " +
				"Read the skill first with skill_read, then write the improved version.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to update",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Full new content for SKILL.md (including frontmatter)",
				},
			},
			Required: []string{"name", "content"},
			Handler:  skillUpdate,
		},
		{
			Name:        "skill_delete",
			Description: "Delete a skill and its entire directory. Use when a skill is obsolete or incorrect.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to delete",
				},
			},
			Required: []string{"name"},
			Handler:  skillDelete,
		},
	}
}

func skillList(_ context.Context, _ map[string]any) tools.Result {
	loaded, err := skills.LoadAll(store.SkillsDir())
	if err != nil {
		return tools.Result{Output: "error listing skills: " + err.Error(), IsErr: true}
	}
	if len(loaded) == 0 {
		return tools.Result{Output: "(no skills yet — use skill_create to save procedural knowledge for reuse)"}
	}
	var b strings.Builder
	for _, s := range loaded {
		fmt.Fprintf(&b, "- %s: %s\n  path: %s\n", s.Name, s.Description, s.FilePath)
	}
	return tools.Result{Output: b.String()}
}

func skillRead(_ context.Context, input map[string]any) tools.Result {
	name, err := tools.GetString(input, "name")
	if err != nil {
		return tools.ErrResult(err)
	}
	content, err := skills.ReadSkill(store.SkillsDir(), name)
	if err != nil {
		return tools.Result{Output: err.Error(), IsErr: true}
	}
	return tools.Result{Output: content}
}

func skillCreate(_ context.Context, input map[string]any) tools.Result {
	name, err := tools.GetString(input, "name")
	if err != nil {
		return tools.ErrResult(err)
	}
	desc, err := tools.GetString(input, "description")
	if err != nil {
		return tools.ErrResult(err)
	}
	body, err := tools.GetString(input, "body")
	if err != nil {
		return tools.ErrResult(err)
	}

	path, err := skills.CreateSkill(store.SkillsDir(), name, desc, body)
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("skill_create failed")
		return tools.Result{Output: "error creating skill: " + err.Error(), IsErr: true}
	}
	log.Info().Str("name", name).Str("path", path).Msg("skill_create")
	return tools.Result{Output: fmt.Sprintf("skill %q created at %s — it will appear in your available_skills on future conversations", name, path)}
}

func skillUpdate(_ context.Context, input map[string]any) tools.Result {
	name, err := tools.GetString(input, "name")
	if err != nil {
		return tools.ErrResult(err)
	}
	content, err := tools.GetString(input, "content")
	if err != nil {
		return tools.ErrResult(err)
	}

	if err := skills.UpdateSkill(store.SkillsDir(), name, content); err != nil {
		return tools.Result{Output: "error updating skill: " + err.Error(), IsErr: true}
	}
	log.Info().Str("name", name).Msg("skill_update")
	return tools.Result{Output: fmt.Sprintf("skill %q updated", name)}
}

func skillDelete(_ context.Context, input map[string]any) tools.Result {
	name, err := tools.GetString(input, "name")
	if err != nil {
		return tools.ErrResult(err)
	}
	if err := skills.DeleteSkill(store.SkillsDir(), name); err != nil {
		return tools.Result{Output: "error deleting skill: " + err.Error(), IsErr: true}
	}
	log.Info().Str("name", name).Msg("skill_delete")
	return tools.Result{Output: fmt.Sprintf("skill %q deleted", name)}
}
