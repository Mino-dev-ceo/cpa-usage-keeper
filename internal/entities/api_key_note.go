package entities

import "time"

// APIKeyNote stores an operator-friendly label for a redacted inbound API key.
type APIKeyNote struct {
	ID        uint      `gorm:"primaryKey"`
	APIAlias  string    `gorm:"column:api_alias;uniqueIndex;not null"`
	Note      string    `gorm:"column:note;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}
