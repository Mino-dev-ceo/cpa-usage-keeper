package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func createAccountGuardStatesMigration(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&entities.AccountGuardState{}); err != nil {
		return fmt.Errorf("create account guard states table: %w", err)
	}
	return nil
}
