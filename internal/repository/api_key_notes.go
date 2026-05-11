package repository

import (
	"fmt"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ListAPIKeyNotes(db *gorm.DB) ([]entities.APIKeyNote, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}

	var notes []entities.APIKeyNote
	if err := db.Order("api_alias asc").Find(&notes).Error; err != nil {
		return nil, fmt.Errorf("list api key notes: %w", err)
	}
	return notes, nil
}

func UpsertAPIKeyNote(db *gorm.DB, apiAlias string, note string) (entities.APIKeyNote, error) {
	if db == nil {
		return entities.APIKeyNote{}, fmt.Errorf("database is nil")
	}

	record := entities.APIKeyNote{
		APIAlias: strings.TrimSpace(apiAlias),
		Note:     strings.TrimSpace(note),
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "api_alias"}},
		DoUpdates: clause.AssignmentColumns([]string{"note", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return entities.APIKeyNote{}, fmt.Errorf("upsert api key note: %w", err)
	}

	var saved entities.APIKeyNote
	if err := db.Where("api_alias = ?", record.APIAlias).First(&saved).Error; err != nil {
		return entities.APIKeyNote{}, fmt.Errorf("load api key note: %w", err)
	}
	return saved, nil
}

func DeleteAPIKeyNote(db *gorm.DB, apiAlias string) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if err := db.Where("api_alias = ?", strings.TrimSpace(apiAlias)).Delete(&entities.APIKeyNote{}).Error; err != nil {
		return fmt.Errorf("delete api key note: %w", err)
	}
	return nil
}
