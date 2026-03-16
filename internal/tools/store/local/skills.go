package local

import (
	"fmt"
	"gogogot/internal/tools/store"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

func (s *LocalStore) LoadSkills() ([]store.Skill, error) {
	return loadSkillsFromDir(s.SkillsDir())
}

func (s *LocalStore) CreateSkill(name, description, body string) (string, error) {
	if err := (&store.Skill{Name: name, Description: description}).Validate(); err != nil {
		return "", fmt.Errorf("failed to validate skill: %w", err)
	}
	safeName := sanitizeSkillName(name)
	skillDir := filepath.Join(s.SkillsDir(), safeName)

	if _, err := os.Stat(skillDir); err == nil {
		return "", fmt.Errorf("skill %q already exists", safeName)
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", err
	}

	content := formatSkillMd(name, description, body)
	skillMd := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMd, []byte(content), 0o644); err != nil {
		return "", err
	}
	return skillMd, nil
}

func (s *LocalStore) UpdateSkill(name, content string) error {
	safeName := sanitizeSkillName(name)
	skillMd := filepath.Join(s.SkillsDir(), safeName, "SKILL.md")

	if _, err := os.Stat(skillMd); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", safeName)
	}
	parsedName, parsedDesc := parseSkillFrontmatter(content)
	if parsedName == "" {
		parsedName = safeName
	}
	if err := (&store.Skill{Name: parsedName, Description: parsedDesc, FilePath: skillMd}).Validate(); err != nil {
		return err
	}
	return os.WriteFile(skillMd, []byte(content), 0o644)
}

func (s *LocalStore) DeleteSkill(name string) error {
	safeName := sanitizeSkillName(name)
	skillDir := filepath.Join(s.SkillsDir(), safeName)

	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", safeName)
	}
	return os.RemoveAll(skillDir)
}

func (s *LocalStore) ReadSkill(name string) (string, error) {
	safeName := sanitizeSkillName(name)
	data, err := os.ReadFile(filepath.Join(s.SkillsDir(), safeName, "SKILL.md"))
	if os.IsNotExist(err) {
		return "", fmt.Errorf("skill %q not found", safeName)
	}
	return string(data), err
}

// --- helpers ---

func loadSkillsFromDir(rootDir string) ([]store.Skill, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []store.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMd := filepath.Join(rootDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillMd)
		if err != nil {
			continue
		}
		name, desc := parseSkillFrontmatter(string(data))
		if name == "" {
			name = e.Name()
		}
		sk := store.Skill{
			Name:        name,
			Description: desc,
			FilePath:    skillMd,
			Dir:         filepath.Join(rootDir, e.Name()),
		}
		if err := sk.Validate(); err != nil {
			log.Warn().Err(err).Str("skill", sk.Name).Str("path", sk.FilePath).Msg("skills: skipping invalid skill")
			continue
		}
		out = append(out, sk)
	}
	return out, nil
}

func parseSkillFrontmatter(content string) (name, description string) {
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", ""
	}
	block := content[3 : 3+end]

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if key, val, ok := splitSkillYAMLLine(line); ok {
			switch key {
			case "name":
				name = val
			case "description":
				description = val
			}
		}
	}
	return name, description
}

func splitSkillYAMLLine(line string) (key, val string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	val = strings.TrimSpace(line[idx+1:])
	val = strings.Trim(val, `"'`)
	return key, val, true
}

func formatSkillMd(name, description, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", name))
	b.WriteString(fmt.Sprintf("description: %q\n", description))
	b.WriteString("---\n\n")
	if body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sanitizeSkillName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, name)
	return strings.Trim(name, "-")
}
