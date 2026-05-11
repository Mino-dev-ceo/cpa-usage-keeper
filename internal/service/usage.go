package service

import (
	"context"
	"fmt"
	"strings"

	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/redact"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	servicedto "cpa-usage-keeper/internal/service/dto"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type usageService struct {
	db                     *gorm.DB
	externalAPIKeysFetcher externalAPIKeysFetcher
}

const maxAPIKeyNoteLength = 80

type externalAPIKeysFetcher interface {
	FetchExternalAPIKeys(context.Context) (*response.ExternalAPIKeysResult, error)
}

func NewUsageService(db *gorm.DB, fetchers ...externalAPIKeysFetcher) UsageProvider {
	var fetcher externalAPIKeysFetcher
	if len(fetchers) > 0 {
		fetcher = fetchers[0]
	}
	return &usageService{db: db, externalAPIKeysFetcher: fetcher}
}

func (s *usageService) GetUsageWithFilter(_ context.Context, filter servicedto.UsageFilter) (*repodto.StatisticsSnapshot, error) {
	return repository.BuildUsageSnapshotWithFilter(s.db, repodto.UsageQueryFilter{
		Range:     filter.Range,
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
}

// Usage 页面里的 Overview tab 只下传时间窗口，仓储层负责构建 overview 聚合。
func (s *usageService) GetUsageOverview(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageOverviewSnapshot, error) {
	overview, err := repository.BuildUsageOverviewWithFilter(s.db, repodto.UsageQueryFilter{
		Range:     filter.Range,
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}
	return &servicedto.UsageOverviewSnapshot{
		Usage: overview.Usage,
		Summary: servicedto.UsageOverviewSummary{
			RequestCount:    overview.Summary.RequestCount,
			TokenCount:      overview.Summary.TokenCount,
			WindowMinutes:   overview.Summary.WindowMinutes,
			RPM:             overview.Summary.RPM,
			TPM:             overview.Summary.TPM,
			TotalCost:       overview.Summary.TotalCost,
			CostAvailable:   overview.Summary.CostAvailable,
			CachedTokens:    overview.Summary.CachedTokens,
			ReasoningTokens: overview.Summary.ReasoningTokens,
		},
		Series:       mapUsageOverviewSeries(overview.Series),
		HourlySeries: mapUsageOverviewSeries(overview.HourlySeries),
		DailySeries:  mapUsageOverviewSeries(overview.DailySeries),
		Health: servicedto.UsageOverviewHealth{
			TotalSuccess:  overview.Health.TotalSuccess,
			TotalFailure:  overview.Health.TotalFailure,
			SuccessRate:   overview.Health.SuccessRate,
			Rows:          overview.Health.Rows,
			Columns:       overview.Health.Columns,
			BucketSeconds: overview.Health.BucketSeconds,
			WindowStart:   overview.Health.WindowStart,
			WindowEnd:     overview.Health.WindowEnd,
			BlockDetails: func() []servicedto.UsageOverviewHealthBlock {
				blocks := make([]servicedto.UsageOverviewHealthBlock, 0, len(overview.Health.BlockDetails))
				for _, block := range overview.Health.BlockDetails {
					blocks = append(blocks, servicedto.UsageOverviewHealthBlock{
						StartTime: block.StartTime,
						EndTime:   block.EndTime,
						Success:   block.Success,
						Failure:   block.Failure,
						Rate:      block.Rate,
					})
				}
				return blocks
			}(),
		},
	}, nil
}

func mapUsageOverviewSeries(series repodto.UsageOverviewSeriesRecord) servicedto.UsageOverviewSeries {
	models := make(map[string]servicedto.UsageOverviewSeries, len(series.Models))
	for model, modelSeries := range series.Models {
		models[model] = mapUsageOverviewSeries(modelSeries)
	}
	return servicedto.UsageOverviewSeries{
		Requests:        series.Requests,
		Tokens:          series.Tokens,
		RPM:             series.RPM,
		TPM:             series.TPM,
		Cost:            series.Cost,
		InputTokens:     series.InputTokens,
		OutputTokens:    series.OutputTokens,
		CachedTokens:    series.CachedTokens,
		ReasoningTokens: series.ReasoningTokens,
		Models:          models,
	}
}

// Usage 页面里的 Request Event Log tab 下传分页和列表筛选条件。
func (s *usageService) ListUsageEvents(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageEventsPage, error) {
	page, err := repository.ListUsageEventsWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
		Limit:     filter.Limit,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
		Offset:    filter.Offset,
		Model:     filter.Model,
		Source:    filter.Source,
		AuthIndex: filter.AuthIndex,
		Result:    filter.Result,
	})
	if err != nil {
		return nil, err
	}
	result := make([]servicedto.UsageEventRecord, 0, len(page.Events))
	for _, row := range page.Events {
		result = append(result, servicedto.UsageEventRecord{
			ID:              row.ID,
			Timestamp:       row.Timestamp,
			APIGroupKey:     row.APIGroupKey,
			Model:           row.Model,
			AuthType:        row.AuthType,
			Provider:        row.Provider,
			Source:          row.Source,
			AuthIndex:       row.AuthIndex,
			Failed:          row.Failed,
			LatencyMS:       row.LatencyMS,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			ReasoningTokens: row.ReasoningTokens,
			CachedTokens:    row.CachedTokens,
			TotalTokens:     row.TotalTokens,
		})
	}
	return &servicedto.UsageEventsPage{Events: result, Models: page.Models, TotalCount: page.TotalCount, Page: page.Page, PageSize: page.PageSize, TotalPages: page.TotalPages}, nil
}

// Usage 页面里的 Request Event Log tab 的 model 筛选项只按当前时间窗口加载候选值。
func (s *usageService) ListUsageEventFilterOptions(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageEventFilterOptions, error) {
	options, err := repository.ListUsageEventFilterOptionsWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}
	return &servicedto.UsageEventFilterOptions{Models: options.Models}, nil
}

// Usage 页面里的 Analysis tab 只下传时间窗口，仓储层负责按 API 和 model 聚合。
func (s *usageService) GetUsageAnalysis(ctx context.Context, filter servicedto.UsageFilter) (*servicedto.UsageAnalysisSnapshot, error) {
	apiRows, modelRows, err := repository.ListUsageAnalysisWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}

	notes, err := s.apiKeyNoteMap()
	if err != nil {
		return nil, err
	}

	apis := make([]servicedto.UsageAnalysisAPIStat, 0, len(apiRows))
	existingAPIAliases := make(map[string]struct{}, len(apiRows))
	for _, row := range apiRows {
		models := make([]servicedto.UsageAnalysisModelStat, 0, len(row.Models))
		for _, model := range row.Models {
			models = append(models, servicedto.UsageAnalysisModelStat{
				Model:              model.Model,
				TotalRequests:      model.TotalRequests,
				SuccessCount:       model.SuccessCount,
				FailureCount:       model.FailureCount,
				TotalTokens:        model.TotalTokens,
				InputTokens:        model.InputTokens,
				OutputTokens:       model.OutputTokens,
				ReasoningTokens:    model.ReasoningTokens,
				CachedTokens:       model.CachedTokens,
				TotalLatencyMS:     model.TotalLatencyMS,
				LatencySampleCount: model.LatencySampleCount,
			})
		}
		existingAPIAliases[redact.APIAlias(row.APIGroupKey)] = struct{}{}
		apis = append(apis, servicedto.UsageAnalysisAPIStat{
			APIKey:          row.APIGroupKey,
			DisplayName:     row.DisplayName,
			Note:            notes[redact.APIAlias(row.APIGroupKey)],
			TotalRequests:   row.TotalRequests,
			SuccessCount:    row.SuccessCount,
			FailureCount:    row.FailureCount,
			TotalTokens:     row.TotalTokens,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			ReasoningTokens: row.ReasoningTokens,
			CachedTokens:    row.CachedTokens,
			Models:          models,
		})
	}
	apis = append(apis, s.zeroUsageExternalAPIKeys(ctx, notes, existingAPIAliases)...)

	models := make([]servicedto.UsageAnalysisModelStat, 0, len(modelRows))
	for _, row := range modelRows {
		models = append(models, servicedto.UsageAnalysisModelStat{
			Model:              row.Model,
			TotalRequests:      row.TotalRequests,
			SuccessCount:       row.SuccessCount,
			FailureCount:       row.FailureCount,
			TotalTokens:        row.TotalTokens,
			InputTokens:        row.InputTokens,
			OutputTokens:       row.OutputTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CachedTokens:       row.CachedTokens,
			TotalLatencyMS:     row.TotalLatencyMS,
			LatencySampleCount: row.LatencySampleCount,
		})
	}

	return &servicedto.UsageAnalysisSnapshot{APIs: apis, Models: models}, nil
}

func (s *usageService) zeroUsageExternalAPIKeys(ctx context.Context, notes map[string]string, existingAliases map[string]struct{}) []servicedto.UsageAnalysisAPIStat {
	if s.externalAPIKeysFetcher == nil {
		return nil
	}
	result, err := s.externalAPIKeysFetcher.FetchExternalAPIKeys(ctx)
	if err != nil {
		logrus.WithError(err).Warn("failed to sync external api keys for usage analysis")
		return nil
	}
	if result == nil {
		return nil
	}

	rows := make([]servicedto.UsageAnalysisAPIStat, 0, len(result.Payload.ExternalAPIKeys))
	for _, value := range result.Payload.ExternalAPIKeys {
		apiKey := strings.TrimSpace(value)
		if apiKey == "" {
			continue
		}
		alias := redact.APIAlias(apiKey)
		if _, ok := existingAliases[alias]; ok {
			continue
		}
		existingAliases[alias] = struct{}{}
		rows = append(rows, servicedto.UsageAnalysisAPIStat{
			APIKey:      apiKey,
			DisplayName: apiKey,
			Note:        notes[alias],
			Models:      []servicedto.UsageAnalysisModelStat{},
		})
	}
	return rows
}

func (s *usageService) ListAPIKeyNotes(context.Context) ([]servicedto.APIKeyNote, error) {
	notes, err := repository.ListAPIKeyNotes(s.db)
	if err != nil {
		return nil, err
	}
	result := make([]servicedto.APIKeyNote, 0, len(notes))
	for _, note := range notes {
		result = append(result, servicedto.APIKeyNote{
			APIAlias: note.APIAlias,
			Note:     note.Note,
		})
	}
	return result, nil
}

func (s *usageService) UpsertAPIKeyNote(_ context.Context, apiAlias string, note string) (servicedto.APIKeyNote, error) {
	normalizedAlias, err := normalizeAPIKeyNoteAlias(apiAlias)
	if err != nil {
		return servicedto.APIKeyNote{}, err
	}
	normalizedNote, err := normalizeAPIKeyNoteText(note)
	if err != nil {
		return servicedto.APIKeyNote{}, err
	}
	if normalizedNote == "" {
		if err := repository.DeleteAPIKeyNote(s.db, normalizedAlias); err != nil {
			return servicedto.APIKeyNote{}, err
		}
		return servicedto.APIKeyNote{APIAlias: normalizedAlias, Note: ""}, nil
	}
	saved, err := repository.UpsertAPIKeyNote(s.db, normalizedAlias, normalizedNote)
	if err != nil {
		return servicedto.APIKeyNote{}, err
	}
	return servicedto.APIKeyNote{APIAlias: saved.APIAlias, Note: saved.Note}, nil
}

func (s *usageService) DeleteAPIKeyNote(_ context.Context, apiAlias string) error {
	normalizedAlias, err := normalizeAPIKeyNoteAlias(apiAlias)
	if err != nil {
		return err
	}
	return repository.DeleteAPIKeyNote(s.db, normalizedAlias)
}

func (s *usageService) apiKeyNoteMap() (map[string]string, error) {
	notes, err := repository.ListAPIKeyNotes(s.db)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(notes))
	for _, note := range notes {
		if alias := strings.TrimSpace(note.APIAlias); alias != "" {
			result[alias] = strings.TrimSpace(note.Note)
		}
	}
	return result, nil
}

func normalizeAPIKeyNoteAlias(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "unknown" {
		return "", fmt.Errorf("api key alias is required")
	}
	if !strings.HasPrefix(trimmed, "redacted_api_") {
		return "", fmt.Errorf("invalid api key alias")
	}
	if len(trimmed) > 128 {
		return "", fmt.Errorf("api key alias is too long")
	}
	return trimmed, nil
}

func normalizeAPIKeyNoteText(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > maxAPIKeyNoteLength {
		return "", fmt.Errorf("note is too long; max %d characters", maxAPIKeyNoteLength)
	}
	return trimmed, nil
}
