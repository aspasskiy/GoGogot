package tools

import (
	"context"
	llmtypes "github.com/aspasskiy/gogogot/internal/domain"
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

// ChatSearchFunc searches past chats by semantic relevance.
type ChatSearchFunc = store.ChatSearchFunc

func NewRegistry(st store.Store, braveAPIKey, tavilyAPIKey string, searchFn ChatSearchFunc, extra ...types.Tool) *Registry {
	all := []types.Tool{
		systemtools.BashTool(),
		systemtools.EditFileTool(),
		systemtools.SystemInfoTool(),
		systemtools.FileSearchTool(),
	}
	all = append(all, systemtools.FileTools()...)
	all = append(all, webtools.WebSearchTool(braveAPIKey))
	if tavilyAPIKey != "" {
		all = append(all, webtools.TavilySearchTool(tavilyAPIKey))
	}
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

func (r *Registry) Definitions() []llmtypes.ToolDef {
	out := make([]llmtypes.ToolDef, 0, len(r.tt))
	for _, t := range r.tt {
		out = append(out, llmtypes.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Required:    t.Required,
		})
	}
	return out
}

func (r *Registry) Lookup(name string) (types.Tool, bool) {
	t, ok := r.tt[name]
	return t, ok
}

func (r *Registry) Register(t types.Tool) {
	r.tt[t.Name] = t
}
