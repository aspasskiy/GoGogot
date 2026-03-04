package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gogogot/tools"

	"github.com/rs/zerolog/log"
)

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
	maxOutputSize      = 50 * 1024
)

func BashTool() tools.Tool {
	return tools.Tool{
		Name:        "bash",
		Description: "Execute a shell command and return stdout+stderr. Use for running programs, installing packages, git, docker, etc. Default timeout is 120s. For long-running commands (apt upgrade, docker build, large git clones) increase the timeout.",
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

func executeBash(ctx context.Context, input map[string]any) tools.Result {
	command, _ := input["command"].(string)
	if command == "" {
		return tools.Result{Output: "command is required", IsErr: true}
	}

	workdir, _ := input["workdir"].(string)

	timeout := defaultBashTimeout
	if v, ok := input["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	log.Debug().Str("command", command).Str("workdir", workdir).Dur("timeout", timeout).Msg("bash exec")

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

	truncated := false
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + "\n... (output truncated)"
		truncated = true
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warn().Str("command", command).Dur("timeout", timeout).Dur("elapsed", elapsed).Msg("bash timeout")
			return tools.Result{Output: fmt.Sprintf("command timed out after %s\n%s", timeout, output), IsErr: true}
		}
		log.Debug().Str("command", command).Err(err).Dur("elapsed", elapsed).Msg("bash exit error")
		return tools.Result{Output: fmt.Sprintf("exit error: %v\n%s", err, output), IsErr: true}
	}

	log.Debug().Str("command", command).Int("output_len", len(output)).Bool("truncated", truncated).Dur("elapsed", elapsed).Msg("bash success")

	if strings.TrimSpace(output) == "" {
		output = "(no output)"
	}
	return tools.Result{Output: output}
}
