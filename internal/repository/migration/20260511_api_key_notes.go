package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func createAPIKeyNotesMigration(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&entities.APIKeyNote{}); err != nil {
		return fmt.Errorf("create api key notes table: %w", err)
	}
	return nil
}
