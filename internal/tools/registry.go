package tools

import (
	"context"
	"fmt"
	llmtypes "gogogot/internal/llm/types"
	"gogogot/internal/tools/store"
	system2 "gogogot/internal/tools/system"
	"gogogot/internal/tools/types"
	web2 "gogogot/internal/tools/web"

	"github.com/rs/zerolog/log"
)

type Registry struct {
	tt map[string]types.Tool
}

func NewRegistry(st *store.Store, braveAPIKey string, extra ...types.Tool) *Registry {
	all := []types.Tool{
		system2.BashTool(),
		system2.EditFileTool(),
		system2.SystemInfoTool(),
	}
	all = append(all, system2.FileTools()...)
	all = append(all, web2.WebSearchTool(braveAPIKey))
	all = append(all, web2.WebFetchTool())
	all = append(all, web2.WebRequestTool())
	all = append(all, web2.WebDownloadTool())
	all = append(all, st.MemoryTools()...)
	all = append(all, st.RecallTool())
	all = append(all, st.SkillTools()...)
	all = append(all, extra...)

	r := &Registry{tt: make(map[string]types.Tool, len(all))}
	for _, t := range all {
		r.tt[t.Name] = t
		log.Debug().Str("name", t.Name).Msg("tool registered")
	}
	return r
}

func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) types.Result {
	t, ok := r.tt[name]
	if !ok {
		log.Warn().Str("name", name).Msg("tool dispatch: unknown tool")
		return types.Result{Output: fmt.Sprintf("unknown tool: %s", name), IsErr: true}
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

func (r *Registry) Register(t types.Tool) {
	r.tt[t.Name] = t
	log.Debug().Str("name", t.Name).Msg("tool registered")
}
