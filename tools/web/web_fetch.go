package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gogogot/tools"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
)

const (
	fetchTimeout   = 15 * time.Second
	maxFetchBody   = 512 * 1024
	maxFetchOutput = 50 * 1024
)

var defaultSelectors = []string{"article", "main", "[role=main]"}

var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true,
	"svg": true, "nav": true, "footer": true, "header": true,
}

func WebFetchTool() tools.Tool {
	return tools.Tool{
		Name:        "web_fetch",
		Description: "Fetch a web page and return its text content (HTML tags stripped). Use this to read documentation, articles, API responses, or any web page. For search results use web_search instead.",
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

func webFetch(ctx context.Context, input map[string]any) tools.Result {
	rawURL, _ := input["url"].(string)
	if rawURL == "" {
		return tools.Result{Output: "url is required", IsErr: true}
	}
	selector, _ := input["selector"].(string)

	log.Debug().Str("url", rawURL).Str("selector", selector).Msg("web_fetch")

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return tools.Result{Output: fmt.Sprintf("bad url: %v", err), IsErr: true}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SofieBot/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debug().Str("url", rawURL).Err(err).Msg("web_fetch http error")
		return tools.Result{Output: fmt.Sprintf("http error: %v", err), IsErr: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tools.Result{Output: fmt.Sprintf("HTTP %d for %s", resp.StatusCode, rawURL), IsErr: true}
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/plain") || strings.Contains(ct, "application/json") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
		if err != nil {
			return tools.Result{Output: fmt.Sprintf("read body error: %v", err), IsErr: true}
		}
		return truncateResult(string(body))
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, maxFetchBody))
	if err != nil {
		return tools.Result{Output: fmt.Sprintf("html parse error: %v", err), IsErr: true}
	}

	doc.Find("script, style, noscript, svg").Remove()

	var root *goquery.Selection
	if selector != "" {
		root = doc.Find(selector)
		if root.Length() == 0 {
			return tools.Result{Output: fmt.Sprintf("selector %q matched 0 elements", selector)}
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
	text = collapseWhitespace(text)

	if strings.TrimSpace(text) == "" {
		return tools.Result{Output: "(page returned no readable text)"}
	}
	return truncateResult(text)
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

func truncateResult(s string) tools.Result {
	if len(s) > maxFetchOutput {
		return tools.Result{Output: s[:maxFetchOutput] + "\n... (content truncated)"}
	}
	return tools.Result{Output: s}
}

func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	prevEmpty := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !prevEmpty {
				out = append(out, "")
				prevEmpty = true
			}
			continue
		}
		out = append(out, trimmed)
		prevEmpty = false
	}
	return strings.Join(out, "\n")
}
