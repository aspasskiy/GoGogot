package tools

import (
	"context"
	"sort"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/store"
	"github.com/aspasskiy/gogogot/internal/tools/memory"
	"github.com/aspasskiy/gogogot/internal/tools/recall"
	"github.com/aspasskiy/gogogot/internal/tools/skills"
	systemtools "github.com/aspasskiy/gogogot/internal/tools/system"
	"github.com/aspasskiy/gogogot/internal/tools/types"
	webtools "github.com/aspasskiy/gogogot/internal/tools/web"

	"github.com/rs/zerolog/log"
)

type Registry struct {
	tt map[string]types.Tool
}

func NewRegistry(st store.Store, braveAPIKey string, searchFn domain.ChatSearchFunc, extra ...types.Tool) *Registry {
	all := []types.Tool{
		systemtools.BashTool(),
		systemtools.EditFileTool(),
		systemtools.SystemInfoTool(),
		systemtools.FileSearchTool(),
	}
	all = append(all, systemtools.FileTools()...)
	all = append(all, webtools.WebSearchTool(braveAPIKey))
	all = append(all, webtools.WebFetchTool())
	all = append(all, webtools.WebRequestTool())
	all = append(all, webtools.WebDownloadTool())
	all = append(all, memory.MemoryTools(st)...)
	all = append(all, recall.RecallTool(searchFn))
	all = append(all, skills.SkillTools(st)...)
	all = append(all, extra...)

	r := &Registry{tt: make(map[string]types.Tool, len(all))}
	for _, t := range all {
		r.tt[t.Name] = t
	}
	return r
}

func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) types.Result {
	t, ok := r.tt[name]
	if !ok {
		log.Warn().Str("name", name).Msg("tool dispatch: unknown tool")
		return types.Errf("unknown tool: %s", name)
	}
	return t.Handler(ctx, input)
}

// Definitions returns the tool definitions sorted by name. A stable order keeps
// the request prefix identical between calls, which prompt caching depends on.
func (r *Registry) Definitions() []domain.ToolDef {
	out := make([]domain.ToolDef, 0, len(r.tt))
	for _, t := range r.tt {
		out = append(out, domain.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Required:    t.Required,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Lookup(name string) (types.Tool, bool) {
	t, ok := r.tt[name]
	return t, ok
}
