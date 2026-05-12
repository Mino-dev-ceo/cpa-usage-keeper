package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func SumAuthFileUsageTokens(ctx context.Context, db *gorm.DB, authIndex string, start, end time.Time) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("database is nil")
	}
	trimmedAuthIndex := strings.TrimSpace(authIndex)
	if trimmedAuthIndex == "" {
		return 0, fmt.Errorf("auth index is required")
	}
	var total int64
	if err := db.WithContext(ctx).
		Model(&entities.UsageEvent{}).
		Select("COALESCE(SUM(total_tokens), 0)").
		Where("auth_type = ? AND auth_index = ?", "oauth", trimmedAuthIndex).
		Where("timestamp >= ? AND timestamp < ?", start.UTC(), end.UTC()).
		Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("sum auth file usage tokens for %q: %w", trimmedAuthIndex, err)
	}
	return total, nil
}

func UpsertAccountGuardState(ctx context.Context, db *gorm.DB, state entities.AccountGuardState) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	state.AuthIndex = strings.TrimSpace(state.AuthIndex)
	state.Name = strings.TrimSpace(state.Name)
	state.Reason = strings.TrimSpace(state.Reason)
	if state.AuthIndex == "" {
		return fmt.Errorf("account guard auth index is required")
	}

	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "auth_index"}},
		DoUpdates: clause.Assignments(map[string]any{
			"name":              state.Name,
			"reason":            state.Reason,
			"guard_disabled":    state.GuardDisabled,
			"disabled_at":       state.DisabledAt,
			"reenable_at":       state.ReenableAt,
			"last_used_tokens":  state.LastUsedTokens,
			"last_limit_tokens": state.LastLimitTokens,
			"last_used_percent": state.LastUsedPercent,
			"updated_at":        time.Now().UTC(),
		}),
	}).Create(&state).Error
}

func ListActiveAccountGuardStates(ctx context.Context, db *gorm.DB) ([]entities.AccountGuardState, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	var states []entities.AccountGuardState
	if err := db.WithContext(ctx).
		Where("guard_disabled = ?", true).
		Order("updated_at desc, id desc").
		Find(&states).Error; err != nil {
		return nil, fmt.Errorf("list active account guard states: %w", err)
	}
	return states, nil
}

func MarkAccountGuardStateReenabled(ctx context.Context, db *gorm.DB, authIndex string, now time.Time) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	trimmedAuthIndex := strings.TrimSpace(authIndex)
	if trimmedAuthIndex == "" {
		return fmt.Errorf("account guard auth index is required")
	}
	updates := map[string]any{
		"guard_disabled": false,
		"updated_at":     now.UTC(),
	}
	if err := db.WithContext(ctx).
		Model(&entities.AccountGuardState{}).
		Where("auth_index = ?", trimmedAuthIndex).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("mark account guard state reenabled for %q: %w", trimmedAuthIndex, err)
	}
	return nil
}
