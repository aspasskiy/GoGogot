// Package bridge runs channel skills: a skill directory with a channel.json
// is started as a child process (the bridge) that connects one messaging
// platform to the agent. The bridge only moves messages; the core renders the
// UI (internal/channel/render), checks the sender is the owner, and handles
// commands. The wire format is documented for the agent in the bundled
// add-channel skill (internal/data/skills/bundled/add-channel/SKILL.md).
package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aspasskiy/gogogot/internal/channel/render"
)

// ManifestFile marks a skill directory as a channel.
const ManifestFile = "channel.json"

// Manifest is a channel skill's channel.json.
type Manifest struct {
	// Command runs the bridge from the skill directory, e.g. ["python3", "bridge.py"].
	Command []string `json:"command"`
	// Enabled defaults to true; set false to keep the skill without running it.
	Enabled *bool `json:"enabled,omitempty"`
	// RequiresEnv lists env vars that must be set for the bridge to start
	// (tokens, owner IDs). Missing ones keep the channel idle, not failed.
	RequiresEnv []string `json:"requires_env,omitempty"`
	// Owner is the platform user ID allowed to talk to the agent; OwnerEnv
	// names an env var holding it. One of them is required: messages from
	// any other sender are dropped by the core before they reach the agent.
	Owner    string `json:"owner,omitempty"`
	OwnerEnv string `json:"owner_env,omitempty"`

	// What the platform can display (see render.Caps).
	Format        string `json:"format,omitempty"` // markdown (default), plain, telegram_html
	MaxMessageLen int    `json:"max_message_len,omitempty"`
	CanEdit       bool   `json:"can_edit,omitempty"`
	Buttons       bool   `json:"buttons,omitempty"`
}

// LoadManifest reads dir/channel.json.
func LoadManifest(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	return m, m.validate()
}

func (m Manifest) validate() error {
	if len(m.Command) == 0 || strings.TrimSpace(m.Command[0]) == "" {
		return errors.New(`"command" is required, e.g. ["python3", "bridge.py"]`)
	}
	if m.Owner == "" && m.OwnerEnv == "" {
		return errors.New(`"owner" or "owner_env" is required: the core only lets the owner talk to the agent`)
	}
	switch render.Format(m.Format) {
	case "", render.FormatMarkdown, render.FormatPlain, render.FormatTelegramHTML:
	default:
		return fmt.Errorf("unknown format %q: want markdown, plain or telegram_html", m.Format)
	}
	return nil
}

// checkCommand verifies the bridge program exists, so a missing interpreter
// is reported once instead of as an endless crash loop.
func (m Manifest) checkCommand(dir string) error {
	prog := m.Command[0]
	if strings.ContainsRune(prog, os.PathSeparator) {
		if !filepath.IsAbs(prog) {
			prog = filepath.Join(dir, prog)
		}
		if _, err := os.Stat(prog); err != nil {
			return fmt.Errorf("bridge program %s not found", m.Command[0])
		}
		return nil
	}
	if _, err := exec.LookPath(prog); err != nil {
		return fmt.Errorf("%s not found in PATH (needed to run the bridge)", prog)
	}
	return nil
}

func (m Manifest) enabled() bool { return m.Enabled == nil || *m.Enabled }

// missingEnv lists required env vars (including owner_env) that are unset.
func (m Manifest) missingEnv() []string {
	vars := m.RequiresEnv
	if m.OwnerEnv != "" {
		vars = append(append([]string{}, vars...), m.OwnerEnv)
	}
	var missing []string
	seen := map[string]bool{}
	for _, v := range vars {
		if !seen[v] && os.Getenv(v) == "" {
			missing = append(missing, v)
		}
		seen[v] = true
	}
	return missing
}

func (m Manifest) owner() string {
	if m.Owner != "" {
		return m.Owner
	}
	return strings.TrimSpace(os.Getenv(m.OwnerEnv))
}

func (m Manifest) caps() render.Caps {
	f := render.Format(m.Format)
	if f == "" {
		f = render.FormatMarkdown
	}
	return render.Caps{Format: f, MaxLen: m.MaxMessageLen, CanEdit: m.CanEdit, Buttons: m.Buttons}
}
