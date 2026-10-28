package render

import (
	"fmt"
	"strings"

	"github.com/aspasskiy/gogogot/internal/transport"
)

var phaseEmoji = map[transport.Phase]string{
	transport.PhaseThinking: "\U0001f9e0",
	transport.PhasePlanning: "\U0001f4cb",
	transport.PhaseTool:     "\U0001f527",
	transport.PhaseWorking:  "⚡",
	transport.PhaseMessage:  "\U0001f4ac",
}

// formatStatus renders the live status message as plain text.
func formatStatus(s transport.AgentStatus) string {
	var parts []string

	if len(s.Plan) > 0 {
		parts = append(parts, formatPlanLine(s.Plan))
	}

	if s.Percent != nil {
		parts = append(parts, formatProgressBar(*s.Percent))
	}

	emoji := phaseEmoji[s.Phase]
	if emoji == "" {
		emoji = "⏳"
	}

	label := s.Detail
	if label == "" {
		switch s.Phase {
		case transport.PhaseThinking:
			label = "Thinking"
		case transport.PhasePlanning:
			label = "Planning"
		default:
			label = s.Tool
		}
	}
	if label != "" {
		if s.Phase == transport.PhaseMessage {
			runes := []rune(label)
			if len(runes) > 300 {
				label = string(runes[:300]) + "…"
			}
			parts = append(parts, emoji+" "+label)
		} else {
			parts = append(parts, emoji+" "+label+"...")
		}
	}

	if len(parts) == 0 {
		return emoji + " Working..."
	}
	return strings.Join(parts, "\n")
}

func formatProgressBar(pct int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	const width = 10
	filled := pct * width / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s %d%%", bar, pct)
}

func formatPlanLine(tasks []transport.PlanTask) string {
	if len(tasks) == 0 {
		return ""
	}
	completed := 0
	var activeTitle string
	var icons []string
	for _, t := range tasks {
		switch t.Status {
		case transport.TaskCompleted:
			completed++
			icons = append(icons, "✅")
		case transport.TaskInProgress:
			icons = append(icons, "▸")
			if activeTitle == "" {
				activeTitle = t.Title
			}
		default:
			icons = append(icons, "○")
		}
	}

	line := fmt.Sprintf("\U0001f4cb %d/%d %s", completed, len(tasks), strings.Join(icons, ""))
	if activeTitle != "" {
		line += "\n▸ " + activeTitle
	}
	return line
}
