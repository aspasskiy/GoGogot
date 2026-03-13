package web

import (
	"context"
	"fmt"
	"gogogot/internal/infra/utils"
	"gogogot/internal/tools/types"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	fetchTimeout = 15 * time.Second
	maxFetchBody = 512 * 1024
)

var defaultSelectors = []string{"article", "main", "[role=main]"}

var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true,
	"svg": true, "nav": true, "footer": true, "header": true,
}

func WebFetchTool() types.Tool {
	return types.Tool{
		Name:        "web_fetch",
		Label:       "Reading webpage",
		Description: "Fetch a web page and return its text content (HTML tags stripped). Use this to read documentation, articles, API responses, or any web page. For search results use web_search instead.",
		DetailFunc: func(input map[string]any) string {
			if raw, ok := input["url"].(string); ok {
				if u, err := neturl.Parse(raw); err == nil {
					return u.Host
				}
			}
			return ""
		},
		Parameters: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The full URL to fetch",
			},
			"selector": map[string]any{
				"type":        "string",
				"description": "Optional CSS selector to focus on (e.g. 'article', 'main', 'div.content', '#post-body'). Supports full CSS selectors. If omitted, auto-detects article/main content.",
			},
		},
		Required: []string{"url"},
		Handler:  webFetch,
	}
}

func webFetch(ctx context.Context, input map[string]any) types.Result {
	rawURL, err := types.GetString(input, "url")
	if err != nil {
		return types.ErrResult(err)
	}
	selector := types.GetStringOpt(input, "selector")

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return types.Errf("bad url: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SofieBot/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.Errf("http error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.Errf("HTTP %d for %s", resp.StatusCode, rawURL)
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/plain") || strings.Contains(ct, "application/json") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
		if err != nil {
			return types.Errf("read body error: %v", err)
		}
		return types.TruncateOutput(string(body))
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, maxFetchBody))
	if err != nil {
		return types.Errf("html parse error: %v", err)
	}

	doc.Find("script, style, noscript, svg").Remove()

	var root *goquery.Selection
	if selector != "" {
		root = doc.Find(selector)
		if root.Length() == 0 {
			return types.Result{Output: fmt.Sprintf("selector %q matched 0 elements", selector)}
		}
	} else {
		for _, s := range defaultSelectors {
			found := doc.Find(s)
			if found.Length() > 0 {
				root = found
				break
			}
		}
		if root == nil {
			root = doc.Find("body")
		}
	}

	text := extractGoqueryText(root)
	text = utils.CollapseWhitespace(text)

	if strings.TrimSpace(text) == "" {
		return types.Result{Output: "(page returned no readable text)"}
	}
	return types.TruncateOutput(text)
}

func extractGoqueryText(s *goquery.Selection) string {
	var sb strings.Builder
	s.Each(func(_ int, sel *goquery.Selection) {
		extractNodeText(sel, &sb)
	})
	return sb.String()
}

func extractNodeText(s *goquery.Selection, sb *strings.Builder) {
	tag := goquery.NodeName(s)
	if skipTags[tag] {
		return
	}

	switch tag {
	case "p", "div", "br", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "blockquote", "section":
		sb.WriteString("\n")
	}

	s.Contents().Each(func(_ int, child *goquery.Selection) {
		if goquery.NodeName(child) == "#text" {
			sb.WriteString(child.Text())
		} else {
			extractNodeText(child, sb)
		}
	})

	switch tag {
	case "p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "blockquote", "section":
		sb.WriteString("\n")
	}
}
