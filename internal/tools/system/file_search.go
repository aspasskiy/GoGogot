package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/tools/types"
)

const (
	searchTimeout    = 30 * time.Second
	defaultMaxResult = 30
)

func FileSearchTool() types.Tool {
	return types.Tool{
		Name:  "file_search",
		Label: "Searching files",
		Description: "Search file contents using regex patterns. Returns matching lines with file paths and line numbers. " +
			"Use for finding function definitions, variable usage, error messages, imports, or any text pattern across the codebase.",
		DetailFunc: func(input map[string]any) string {
			s, _ := input["pattern"].(string)
			return s
		},
		Parameters: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for (e.g. 'func main', 'TODO', 'import.*http')",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in (default: current directory)",
			},
			"include": map[string]any{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. '*.go', '*.ts', '*.yaml')",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of matching lines to return (default: 30)",
			},
		},
		Required: []string{"pattern"},
		Handler:  fileSearch,
	}
}

func fileSearch(ctx context.Context, input map[string]any) types.Result {
	pattern, err := types.GetString(input, "pattern")
	if err != nil {
		return types.ErrResult(err)
	}

	dir := types.GetStringOpt(input, "path")
	if dir == "" {
		dir = "."
	}

	include := types.GetStringOpt(input, "include")

	maxResults := defaultMaxResult
	if v, ok := input["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	args := buildSearchArgs(pattern, dir, include, maxResults)
	bin := pickSearchBin()

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return types.Errf("search timed out after %s", searchTimeout)
		}
		// grep/rg return exit 1 when no matches found
		if output == "" {
			return types.Result{Output: "no matches found"}
		}
	}

	if output == "" {
		return types.Result{Output: "no matches found"}
	}

	lines := strings.Split(output, "\n")
	if len(lines) > maxResults {
		output = strings.Join(lines[:maxResults], "\n")
		output += fmt.Sprintf("\n... (%d more lines truncated)", len(lines)-maxResults)
	}

	return types.TruncateOutput(output)
}

func pickSearchBin() string {
	if _, err := exec.LookPath("rg"); err == nil {
		return "rg"
	}
	return "grep"
}

func buildSearchArgs(pattern, dir, include string, maxResults int) []string {
	if _, err := exec.LookPath("rg"); err == nil {
		return buildRgArgs(pattern, dir, include, maxResults)
	}
	return buildGrepArgs(pattern, dir, include, maxResults)
}

func buildRgArgs(pattern, dir, include string, maxResults int) []string {
	args := []string{
		"-n", "--no-heading", "--color=never",
		fmt.Sprintf("--max-count=%d", maxResults),
	}
	if include != "" {
		args = append(args, "--glob", include)
	}
	args = append(args, pattern, dir)
	return args
}

func buildGrepArgs(pattern, dir, include string, maxResults int) []string {
	args := []string{"-rn", "--color=never", fmt.Sprintf("-m%d", maxResults)}
	if include != "" {
		args = append(args, "--include="+include)
	}
	args = append(args, pattern, dir)
	return args
}
