package system

import (
	"context"
	"fmt"

	"gogogot/infra/llm/types"
	"gogogot/infra/tools"

	"github.com/rs/zerolog/log"
)

type Registry struct {
	tt map[string]tools.Tool
}

func NewRegistry(tt []tools.Tool) *Registry {
	r := &Registry{tt: make(map[string]tools.Tool, len(tt))}
	for _, t := range tt {
		r.tt[t.Name] = t
		log.Debug().Str("name", t.Name).Msg("tool registered")
	}
	return r
}

func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) tools.Result {
	t, ok := r.tt[name]
	if !ok {
		log.Warn().Str("name", name).Msg("tool dispatch: unknown tool")
		return tools.Result{Output: fmt.Sprintf("unknown tool: %s", name), IsErr: true}
	}
	return t.Handler(ctx, input)
}

func (r *Registry) Definitions() []types.ToolDef {
	out := make([]types.ToolDef, 0, len(r.tt))
	for _, t := range r.tt {
		out = append(out, types.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Required:    t.Required,
		})
	}
	return out
}

func (r *Registry) All() []tools.Tool {
	out := make([]tools.Tool, 0, len(r.tt))
	for _, t := range r.tt {
		out = append(out, t)
	}
	return out
}

func (r *Registry) Register(t tools.Tool) {
	r.tt[t.Name] = t
	log.Debug().Str("name", t.Name).Msg("tool registered")
}
