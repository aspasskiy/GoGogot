package tools

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"github.com/aspasskiy/gogogot/internal/tools/types"
	"strings"
)

func SkillTools(st store.Store) []types.Tool {
	return []types.Tool{
		{
			Name:  "skill_list",
			Label: "Listing skills",
			Description: "List all available skills with their name and description. Skills are reusable procedural knowledge — workflows, integrations, how-to guides — that you created from past experience.",
			Parameters:  map[string]any{},
			Handler: func(_ context.Context, _ map[string]any) types.Result {
				loaded, err := st.LoadSkills()
				if err != nil {
					return types.Result{Output: "error listing skills: " + err.Error(), IsErr: true}
				}
				if len(loaded) == 0 {
					return types.Result{Output: "(no skills yet — use skill_create to save procedural knowledge for reuse)"}
				}
				var b strings.Builder
				for _, sk := range loaded {
					fmt.Fprintf(&b, "- %s: %s\n  path: %s\n", sk.Name, sk.Description, sk.FilePath)
				}
				return types.Result{Output: b.String()}
			},
		},
		{
			Name:  "skill_read",
			Label: "Reading skill",
			Description: "Read the full content of a skill's SKILL.md. Use this when a skill matches the current task — " +
				"read it first, then follow the instructions inside.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (lowercase, hyphen-separated)",
				},
			},
			Required: []string{"name"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				content, err := st.ReadSkill(name)
				if err != nil {
					return types.Result{Output: err.Error(), IsErr: true}
				}
				return types.Result{Output: content}
			},
		},
		{
			Name:  "skill_create",
			Label: "Creating skill",
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
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				desc, err := types.GetString(input, "description")
				if err != nil {
					return types.ErrResult(err)
				}
				body, err := types.GetString(input, "body")
				if err != nil {
					return types.ErrResult(err)
				}

				path, err := st.CreateSkill(name, desc, body)
				if err != nil {
					return types.Result{Output: "error creating skill: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("skill %q created at %s — it will appear in your available_skills on future conversations", name, path)}
			},
		},
		{
			Name:  "skill_update",
			Label: "Updating skill",
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
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				content, err := types.GetString(input, "content")
				if err != nil {
					return types.ErrResult(err)
				}

				if err := st.UpdateSkill(name, content); err != nil {
					return types.Result{Output: "error updating skill: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("skill %q updated", name)}
			},
		},
		{
			Name:  "skill_delete",
			Label: "Deleting skill",
			Description: "Delete a skill and its entire directory. Use when a skill is obsolete or incorrect.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to delete",
				},
			},
			Required: []string{"name"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				if err := st.DeleteSkill(name); err != nil {
					return types.Result{Output: "error deleting skill: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("skill %q deleted", name)}
			},
		},
	}
}
