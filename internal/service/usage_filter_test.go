package service

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/cpa/dto/externalkeys"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/redact"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/repository/dto"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

type stubExternalAPIKeysFetcher struct {
	keys []string
	err  error
}

func (s stubExternalAPIKeysFetcher) FetchExternalAPIKeys(context.Context) (*response.ExternalAPIKeysResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &response.ExternalAPIKeysResult{
		Payload: externalkeys.ExternalAPIKeysResponse{ExternalAPIKeys: s.keys},
	}, nil
}

func TestUsageServiceGetUsageWithFilterDelegatesToFilteredSnapshot(t *testing.T) {
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "usage-service-filter.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{
		{EventKey: "event-1", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), TotalTokens: 10},
		{EventKey: "event-2", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC), TotalTokens: 20},
	}); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	start := time.Date(2026, 4, 16, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 4, 16, 10, 30, 0, 0, time.UTC)
	provider := NewUsageService(db)
	snapshot, err := provider.GetUsageWithFilter(context.Background(), servicedto.UsageFilter{StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("GetUsageWithFilter returned error: %v", err)
	}
	if snapshot.TotalRequests != 1 || snapshot.TotalTokens != 20 {
		t.Fatalf("expected service filter to keep only in-range event, got %+v", snapshot)
	}
}

func TestUsageServiceGetUsageOverviewDelegatesToFilteredOverview(t *testing.T) {
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "usage-service-overview.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)
	if _, err := repository.UpsertModelPriceSetting(db, dto.ModelPriceSettingInput{
		Model:                "claude-sonnet",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	}); err != nil {
		t.Fatalf("UpsertModelPriceSetting returned error: %v", err)
	}
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{
		{EventKey: "event-1", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), InputTokens: 1000, OutputTokens: 500, CachedTokens: 100, ReasoningTokens: 50, TotalTokens: 1650},
		{EventKey: "event-2", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC), InputTokens: 500, OutputTokens: 250, CachedTokens: 0, ReasoningTokens: 25, TotalTokens: 775},
	}); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	start := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 16, 23, 59, 59, 0, time.UTC)
	provider := NewUsageService(db)
	overview, err := provider.GetUsageOverview(context.Background(), servicedto.UsageFilter{Range: "24h", StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("GetUsageOverview returned error: %v", err)
	}
	if overview.Summary.RequestCount != 2 || overview.Summary.TokenCount != 2425 {
		t.Fatalf("expected overview summary counts, got %+v", overview.Summary)
	}
	if overview.Summary.WindowMinutes != 1440 {
		t.Fatalf("expected 24h overview to use exact 1440 minute window, got %+v", overview.Summary)
	}
	if overview.Series.Requests["2026-04-16T09:00:00Z"] != 1 || overview.Series.Requests["2026-04-16T10:00:00Z"] != 1 {
		t.Fatalf("expected hourly request series values, got %+v", overview.Series)
	}
	if math.Abs(overview.Series.Cost["2026-04-16T09:00:00Z"]-0.01023) > 0.000000001 || math.Abs(overview.Series.Cost["2026-04-16T10:00:00Z"]-0.00525) > 0.000000001 {
		t.Fatalf("expected hourly cost series values, got %+v", overview.Series)
	}
}

func TestUsageServiceGetUsageAnalysisIncludesZeroUsageExternalAPIKeys(t *testing.T) {
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "usage-service-analysis-keys.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{
		{EventKey: "event-1", APIGroupKey: "used-key", Model: "gpt-5.4-mini", Timestamp: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), TotalTokens: 10},
	}); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}
	if _, err := repository.UpsertAPIKeyNote(db, redact.APIAlias("unused-key"), "Customer Zero"); err != nil {
		t.Fatalf("UpsertAPIKeyNote returned error: %v", err)
	}

	provider := NewUsageService(db, stubExternalAPIKeysFetcher{keys: []string{"used-key", "unused-key", "unused-key", "  "}})
	analysis, err := provider.GetUsageAnalysis(context.Background(), servicedto.UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsageAnalysis returned error: %v", err)
	}

	byKey := map[string]servicedto.UsageAnalysisAPIStat{}
	for _, api := range analysis.APIs {
		byKey[api.APIKey] = api
	}
	if len(byKey) != 2 {
		t.Fatalf("expected used key plus zero-usage external key, got %+v", analysis.APIs)
	}
	if byKey["used-key"].TotalRequests != 1 || byKey["used-key"].TotalTokens != 10 {
		t.Fatalf("expected used key stats to remain intact, got %+v", byKey["used-key"])
	}
	zero := byKey["unused-key"]
	if zero.TotalRequests != 0 || zero.TotalTokens != 0 || len(zero.Models) != 0 || zero.Note != "Customer Zero" {
		t.Fatalf("expected zero-usage external key with note, got %+v", zero)
	}
}
