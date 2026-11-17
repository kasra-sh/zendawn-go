// Package openrouter fetches model metadata from the OpenRouter API.
package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	pageLimit      = 1000
	defaultTimeout = 15 * time.Second
)

// Model is one entry from GET /models.
type Model struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ContextLength int     `json:"context_length"`
	Pricing       Pricing `json:"pricing"`
	Benchmarks    *struct {
		ArtificialAnalysis *struct {
			CodingIndex *float64 `json:"coding_index"`
		} `json:"artificial_analysis"`
	} `json:"benchmarks"`
}

// Pricing holds string prices from OpenRouter (USD per token).
type Pricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// Spec is runtime metadata derived from a Model.
type Spec struct {
	ContextLength   int
	Name            string
	CodingIndex     *float64
	PromptPrice     string
	CompletionPrice string
}

// SpecFromModel maps API fields into Spec.
func SpecFromModel(m Model) Spec {
	s := Spec{
		ContextLength:   m.ContextLength,
		Name:            m.Name,
		PromptPrice:     m.Pricing.Prompt,
		CompletionPrice: m.Pricing.Completion,
	}
	if m.Benchmarks != nil && m.Benchmarks.ArtificialAnalysis != nil {
		s.CodingIndex = m.Benchmarks.ArtificialAnalysis.CodingIndex
	}
	return s
}

type listResponse struct {
	Data       []Model `json:"data"`
	TotalCount int     `json:"total_count"`
	Links      *struct {
		Next string `json:"next"`
	} `json:"links"`
}

// IsOpenRouterBase reports whether baseURL points at OpenRouter.
func IsOpenRouterBase(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return strings.Contains(strings.ToLower(baseURL), "openrouter.ai")
	}
	return strings.EqualFold(u.Host, "openrouter.ai") || strings.HasSuffix(strings.ToLower(u.Host), ".openrouter.ai")
}

// ListFreeZDRCodingChatModels returns free, ZDR, chat-completion models
// sorted by coding-high-to-low (server order). Paginated until complete.
func ListFreeZDRCodingChatModels(ctx context.Context, baseURL, apiKey string) ([]Model, error) {
	return listModels(ctx, baseURL, apiKey, url.Values{
		"zdr":               {"true"},
		"sort":              {"coding-high-to-low"},
		"max_price":         {"0"},
		"max_output_price":  {"0"},
		"endpoints":         {"/api/v1/chat/completions"},
		"limit":             {strconv.Itoa(pageLimit)},
	})
}

func listModels(ctx context.Context, baseURL, apiKey string, q url.Values) ([]Model, error) {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	client := &http.Client{Timeout: defaultTimeout}

	var all []Model
	offset := 0
	for {
		params := cloneValues(q)
		params.Set("offset", strconv.Itoa(offset))
		if params.Get("limit") == "" {
			params.Set("limit", strconv.Itoa(pageLimit))
		}
		limit, _ := strconv.Atoi(params.Get("limit"))
		if limit < 1 {
			limit = pageLimit
			params.Set("limit", strconv.Itoa(limit))
		}

		reqURL := base + "/models?" + params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		_ = res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("openrouter models: HTTP %d: %s", res.StatusCode, truncate(string(body), 200))
		}

		var page listResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("openrouter models: decode: %w", err)
		}
		if len(page.Data) == 0 {
			break
		}
		all = append(all, page.Data...)
		offset += len(page.Data)
		if page.TotalCount > 0 && len(all) >= page.TotalCount {
			break
		}
		if len(page.Data) < limit {
			break
		}
	}
	return all, nil
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vals := range v {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
