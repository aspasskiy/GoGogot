package system

import (
	"context"
	"gogogot/internal/tools/types"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
)

func BashTool() types.Tool {
	return types.Tool{
		Name:        "bash",
		Label:       "Running command",
		Description: "Execute a shell command and return stdout+stderr. Use for running programs, installing packages, git, docker, etc. Default timeout is 120s. For long-running commands (apt upgrade, docker build, large git clones) increase the timeout.",
		DetailFunc: func(input map[string]any) string {
			s, _ := input["command"].(string)
			return s
		},
		Parameters: map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "Optional working directory for the command",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 120, max 600). Increase for long-running commands like apt upgrade or docker build.",
			},
		},
		Required: []string{"command"},
		Handler:  executeBash,
	}
}

func executeBash(ctx context.Context, input map[string]any) types.Result {
	command, err := types.GetString(input, "command")
	if err != nil {
		return types.ErrResult(err)
	}

	workdir := types.GetStringOpt(input, "workdir")

	timeout := defaultBashTimeout
	if v, ok := input["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	output := string(out)

	if len(output) > types.MaxOutputSize {
		output = output[:types.MaxOutputSize] + "\n... (truncated)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warn().Str("command", command).Dur("timeout", timeout).Dur("elapsed", elapsed).Msg("bash timeout")
			return types.Errf("command timed out after %s\n%s", timeout, output)
		}
		return types.Errf("exit error: %v\n%s", err, output)
	}

	if strings.TrimSpace(output) == "" {
		output = "(no output)"
	}
	return types.Result{Output: output}
}
