package entities

import "time"

const (
	AccountGuardReasonQuotaThreshold = "quota_threshold"
)

// AccountGuardState records auth files disabled by the local guard so cleanup
// jobs can distinguish quota protection from truly unavailable accounts.
type AccountGuardState struct {
	ID              uint   `gorm:"primaryKey"`
	AuthIndex       string `gorm:"uniqueIndex:uniq_account_guard_states_auth_index"`
	Name            string
	Reason          string
	GuardDisabled   bool `gorm:"index:idx_account_guard_states_guard_disabled"`
	DisabledAt      *time.Time
	ReenableAt      *time.Time
	LastUsedTokens  int64
	LastLimitTokens int64
	LastUsedPercent float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
