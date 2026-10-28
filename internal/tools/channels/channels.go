package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel/bridge"
	"github.com/aspasskiy/gogogot/internal/tools/types"
)

// ChannelTools let the agent manage channel skills: see which run, apply
// edits by restarting one, and read a bridge's logs. builtin lists the
// built-in channels running in this process, for channel_list.
func ChannelTools(m *bridge.Manager, builtin []string) []types.Tool {
	return []types.Tool{
		{
			Name:  "channel_list",
			Label: "Listing channels",
			Description: "List the channels the owner can reach you through: built-in ones and channel skills " +
				"(skills with a channel.json), with their state (running, crashed, missing_env, disabled, invalid, reserved) and why. " +
				"See the add-channel skill to connect a new interface.",
			Parameters: map[string]any{},
			Handler: func(context.Context, map[string]any) types.Result {
				var b strings.Builder
				for _, name := range builtin {
					fmt.Fprintf(&b, "- %s: running (built-in)\n", name)
				}
				for _, s := range m.List() {
					fmt.Fprintf(&b, "- %s: %s", s.Name, formatStatus(s))
					b.WriteByte('\n')
				}
				if b.Len() == 0 {
					return types.Result{Output: "(no channels)"}
				}
				return types.Result{Output: b.String()}
			},
		},
		{
			Name:  "channel_restart",
			Label: "Restarting channel",
			Description: "Apply changes to a channel skill: stop its bridge if running, re-read channel.json and start it again. " +
				"Use after creating, editing, enabling or disabling a channel skill. Waits up to 10s for the bridge to report ready " +
				"and returns its state; on failure read channel_logs. Restarting the channel you are talking through is safe " +
				"while the new bridge works: your current task keeps running and its replies wait for the new bridge (up to 10s).",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Channel skill name (its directory name under the skills dir)",
				},
			},
			Required: []string{"name"},
			DetailFunc: func(input map[string]any) string {
				return types.GetStringOpt(input, "name")
			},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				s, err := m.Reload(name)
				if err != nil {
					return types.ErrResult(err)
				}
				out := fmt.Sprintf("%s: %s", s.Name, formatStatus(s))
				if s.State == bridge.StateCrashed || s.State == bridge.StateStarting {
					if logs, err := m.Logs(name); err == nil && len(logs) > 0 {
						out += "\n\nlast log lines:\n" + strings.Join(tail(logs, 20), "\n")
					}
				}
				return types.Result{Output: out}
			},
		},
		{
			Name:        "channel_logs",
			Label:       "Reading channel logs",
			Description: "Show the last lines a channel skill's bridge wrote to stderr (up to 100). Use to debug a crashed or misbehaving channel.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Channel skill name",
				},
			},
			Required: []string{"name"},
			DetailFunc: func(input map[string]any) string {
				return types.GetStringOpt(input, "name")
			},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				logs, err := m.Logs(name)
				if err != nil {
					return types.ErrResult(err)
				}
				if len(logs) == 0 {
					return types.Result{Output: "(no log lines yet)"}
				}
				return types.TruncateOutput(strings.Join(logs, "\n"))
			},
		},
	}
}

func formatStatus(s bridge.Status) string {
	out := string(s.State)
	if s.State == bridge.StateRunning && s.PID != 0 {
		out += fmt.Sprintf(" (pid %d, up %s)", s.PID, time.Since(s.Since).Round(time.Second))
	}
	if s.Restarts > 0 {
		out += fmt.Sprintf(", %d restarts", s.Restarts)
	}
	if s.Detail != "" {
		out += " — " + s.Detail
	}
	return out
}

func tail(lines []string, n int) []string {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
}
