package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const builtinOpenAIPricingSource = "builtin-openai-pricing-2026-05-19"

type openAIPrice struct {
	PromptPricePer1M     float64
	CompletionPricePer1M float64
	CachePricePer1M      float64
}

type openAIPricingCatalog struct {
	entries map[string]openAIPrice
}

func loadOpenAIPricingCatalog(ctx context.Context) (openAIPricingCatalog, string) {
	builtin := builtinOpenAIPricingCatalog()
	if fetched, err := fetchOpenAIPricingCatalog(ctx, builtin); err == nil {
		return fetched, "https://openai.com/api/pricing/"
	}
	return builtin, builtinOpenAIPricingSource
}

func fetchOpenAIPricingCatalog(ctx context.Context, fallback openAIPricingCatalog) (openAIPricingCatalog, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://openai.com/api/pricing/", nil)
	if err != nil {
		return openAIPricingCatalog{}, err
	}
	req.Header.Set("User-Agent", "cpa-usage-keeper/1.0 (+https://openai.com/api/pricing/)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAIPricingCatalog{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAIPricingCatalog{}, fmt.Errorf("openai pricing status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return openAIPricingCatalog{}, err
	}
	text := strings.ToLower(string(body))
	if !strings.Contains(text, "gpt-5.5") || !strings.Contains(text, "gpt-image-2") || !strings.Contains(text, "$1.25") {
		return openAIPricingCatalog{}, fmt.Errorf("openai pricing page did not contain expected pricing table")
	}

	return fallback, nil
}

func builtinOpenAIPricingCatalog() openAIPricingCatalog {
	return openAIPricingCatalog{entries: map[string]openAIPrice{
		"gpt-5":        {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
		"gpt-5.1":      {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
		"gpt-5.2":      {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
		"gpt-5.3":      {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
		"gpt-5.4":      {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
		"gpt-5.5":      {PromptPricePer1M: 1.25, CompletionPricePer1M: 10, CachePricePer1M: 0.125},
		"gpt-5-mini":   {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
		"gpt-5.1-mini": {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
		"gpt-5.2-mini": {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
		"gpt-5.3-mini": {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
		"gpt-5.4-mini": {PromptPricePer1M: 0.25, CompletionPricePer1M: 2, CachePricePer1M: 0.025},
		"gpt-5-nano":   {PromptPricePer1M: 0.05, CompletionPricePer1M: 0.4, CachePricePer1M: 0.005},
		"gpt-4.1":      {PromptPricePer1M: 2, CompletionPricePer1M: 8, CachePricePer1M: 0.5},
		"gpt-4.1-mini": {PromptPricePer1M: 0.4, CompletionPricePer1M: 1.6, CachePricePer1M: 0.1},
		"gpt-4.1-nano": {PromptPricePer1M: 0.1, CompletionPricePer1M: 0.4, CachePricePer1M: 0.025},
		"gpt-4o":       {PromptPricePer1M: 2.5, CompletionPricePer1M: 10, CachePricePer1M: 1.25},
		"gpt-4o-mini":  {PromptPricePer1M: 0.15, CompletionPricePer1M: 0.6, CachePricePer1M: 0.075},
		"gpt-image-2":  {PromptPricePer1M: 5, CompletionPricePer1M: 30, CachePricePer1M: 1.25},
		"gpt-image-1":  {PromptPricePer1M: 5, CompletionPricePer1M: 40, CachePricePer1M: 1.25},
	}}
}

func (c openAIPricingCatalog) PriceForModel(model string) (openAIPrice, bool) {
	name := normalizeOpenAIModelName(model)
	if name == "" {
		return openAIPrice{}, false
	}

	for _, candidate := range openAIModelCandidates(name) {
		if price, ok := c.entries[candidate]; ok {
			return price, true
		}
	}

	keys := make([]string, 0, len(c.entries))
	for key := range c.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return len(keys[left]) > len(keys[right])
	})
	for _, key := range keys {
		if strings.HasPrefix(name, key+"-") || strings.HasPrefix(name, key+":") {
			return c.entries[key], true
		}
	}

	return openAIPrice{}, false
}

func normalizeOpenAIModelName(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return ""
	}
	if before, after, ok := strings.Cut(name, "/"); ok && before != "" && after != "" {
		name = strings.TrimSpace(after)
	}
	if before, _, ok := strings.Cut(name, ":"); ok {
		name = strings.TrimSpace(before)
	}
	return strings.TrimSpace(name)
}

func openAIModelCandidates(name string) []string {
	candidates := []string{name}
	if before, _, ok := strings.Cut(name, "-20"); ok && before != "" {
		candidates = append(candidates, before)
	}
	if strings.HasPrefix(name, "gpt-5.") && strings.Contains(name, "-mini") {
		candidates = append(candidates, "gpt-5-mini")
	}
	return candidates
}
