package web

import (
	"context"
	"encoding/json"
	"fmt"
	"gogogot/internal/tools/types"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func WebSearchTool(braveAPIKey string) types.Tool {
	return types.Tool{
		Name:        "web_search",
		Label:       "Searching the web",
		Description: "Search the web for information using Brave Search. Returns top 5 results with title, URL, and description.",
		DetailFunc: func(input map[string]any) string {
			s, _ := input["query"].(string)
			return s
		},
		Parameters: map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query",
			},
		},
		Required: []string{"query"},
		Handler: func(ctx context.Context, input map[string]any) types.Result {
			return webSearch(ctx, input, braveAPIKey)
		},
	}
}

func webSearch(ctx context.Context, input map[string]any, apiKey string) types.Result {
	query, err := types.GetString(input, "query")
	if err != nil {
		return types.ErrResult(err)
	}
	if apiKey == "" {
		log.Warn().Msg("web_search: BRAVE_API_KEY not set")
		return types.Result{Output: "BRAVE_API_KEY not set — web search disabled", IsErr: true}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=5", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return types.Errf("request error: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.Errf("http error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return types.Errf("read body error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return types.Errf("brave API %d: %s", resp.StatusCode, string(body))
	}

	var br braveResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return types.Errf("json decode error: %v", err)
	}

	var sb strings.Builder
	for i, r := range br.Web.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	if sb.Len() == 0 {
		return types.Result{Output: "no results found"}
	}
	return types.Result{Output: sb.String()}
}
