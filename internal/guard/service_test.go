package guard

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/authfiles"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunOnceDisablesAuthFileAtWeeklyThreshold(t *testing.T) {
	db := openGuardTestDB(t)
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&entities.UsageIdentity{
		Name:         "Codex",
		AuthType:     entities.UsageIdentityAuthTypeAuthFile,
		AuthTypeName: "oauth",
		Identity:     "auth-1",
		Type:         "codex",
		Provider:     "Codex",
	}).Error; err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if err := db.Create(&entities.UsageEvent{
		EventKey:    "event-1",
		AuthType:    "oauth",
		AuthIndex:   "auth-1",
		Timestamp:   now.Add(-time.Hour),
		TotalTokens: 850,
	}).Error; err != nil {
		t.Fatalf("create usage event: %v", err)
	}
	client := &guardCPAStub{files: []authfiles.AuthFile{{
		ID:        "auth-1",
		AuthIndex: "auth-1",
		Name:      "codex.json",
		Type:      "codex",
		Provider:  "Codex",
		Status:    "active",
	}}}
	service := NewService(db, client, Config{
		Enabled:          true,
		UsageThreshold:   0.8,
		WeeklyTokenLimit: 1000,
		DryRun:           false,
		AutoReenable:     true,
		ResetWeekday:     time.Monday,
	})
	service.now = func() time.Time { return now }

	result, err := service.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Checked != 1 || result.ThresholdHits != 1 || result.Disabled != 1 {
		t.Fatalf("unexpected guard result: %+v", result)
	}
	if len(client.patches) != 1 || client.patches[0].name != "codex.json" || !client.patches[0].disabled {
		t.Fatalf("unexpected patches: %+v", client.patches)
	}
	var state entities.AccountGuardState
	if err := db.Where("auth_index = ?", "auth-1").First(&state).Error; err != nil {
		t.Fatalf("load guard state: %v", err)
	}
	if !state.GuardDisabled || state.Reason != entities.AccountGuardReasonQuotaThreshold || state.LastUsedTokens != 850 || state.LastLimitTokens != 1000 {
		t.Fatalf("unexpected guard state: %+v", state)
	}
}

func TestRunOnceUsesProviderWeeklyQuotaWhenTokenLimitIsUnknown(t *testing.T) {
	db := openGuardTestDB(t)
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&entities.UsageIdentity{
		Name:         "Codex",
		AuthType:     entities.UsageIdentityAuthTypeAuthFile,
		AuthTypeName: "oauth",
		Identity:     "auth-1",
		Type:         "codex",
		Provider:     "Codex",
	}).Error; err != nil {
		t.Fatalf("create identity: %v", err)
	}
	client := &guardCPAStub{files: []authfiles.AuthFile{{
		ID:        "auth-1",
		AuthIndex: "auth-1",
		Name:      "codex.json",
		Status:    "active",
	}}}
	quotaChecker := &guardQuotaStub{responses: map[string]quota.CheckResponse{
		"auth-1": {
			ID: "auth-1",
			Quota: []quota.QuotaRow{{
				Key:         "rate_limit.secondary_window",
				Label:       "Weekly",
				UsedPercent: floatPtr(82),
				Window:      &quota.QuotaWindow{Seconds: int64Ptr(604800)},
			}},
		},
	}}
	service := NewServiceWithQuotaChecker(db, client, quotaChecker, Config{
		Enabled:              true,
		ProviderQuotaEnabled: true,
		UsageThreshold:       0.8,
		WeeklyTokenLimit:     0,
		DryRun:               false,
		AutoReenable:         true,
		ResetWeekday:         time.Monday,
	})
	service.now = func() time.Time { return now }

	result, err := service.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Checked != 1 || result.ThresholdHits != 1 || result.Disabled != 1 {
		t.Fatalf("unexpected guard result: %+v", result)
	}
	var state entities.AccountGuardState
	if err := db.Where("auth_index = ?", "auth-1").First(&state).Error; err != nil {
		t.Fatalf("load guard state: %v", err)
	}
	if state.LastLimitTokens != 0 || state.LastUsedTokens != 0 || state.LastUsedPercent != 0.82 {
		t.Fatalf("expected provider quota percent to be recorded without token limit, got %+v", state)
	}
}

func TestRunOnceReenablesExpiredGuardState(t *testing.T) {
	db := openGuardTestDB(t)
	reenableAt := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	disabledAt := reenableAt.Add(-24 * time.Hour)
	if err := db.Create(&entities.AccountGuardState{
		AuthIndex:     "auth-1",
		Name:          "codex.json",
		Reason:        entities.AccountGuardReasonQuotaThreshold,
		GuardDisabled: true,
		DisabledAt:    &disabledAt,
		ReenableAt:    &reenableAt,
	}).Error; err != nil {
		t.Fatalf("create guard state: %v", err)
	}
	client := &guardCPAStub{files: []authfiles.AuthFile{{
		ID:        "auth-1",
		AuthIndex: "auth-1",
		Name:      "codex.json",
		Status:    "disabled",
		Disabled:  true,
	}}}
	service := NewService(db, client, Config{
		Enabled:          true,
		WeeklyTokenLimit: 1000,
		DryRun:           false,
		AutoReenable:     true,
		ResetWeekday:     time.Monday,
	})
	service.now = func() time.Time { return reenableAt.Add(time.Minute) }

	result, err := service.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Reenabled != 1 {
		t.Fatalf("expected one reenabled account, got %+v", result)
	}
	if len(client.patches) != 1 || client.patches[0].disabled {
		t.Fatalf("unexpected patches: %+v", client.patches)
	}
	var state entities.AccountGuardState
	if err := db.Where("auth_index = ?", "auth-1").First(&state).Error; err != nil {
		t.Fatalf("load guard state: %v", err)
	}
	if state.GuardDisabled {
		t.Fatalf("expected guard state to be inactive, got %+v", state)
	}
}

func TestCleanupBannedSkipsGuardDisabledAccounts(t *testing.T) {
	db := openGuardTestDB(t)
	if err := db.Create(&entities.AccountGuardState{
		AuthIndex:     "guard-auth",
		Name:          "guard.json",
		Reason:        entities.AccountGuardReasonQuotaThreshold,
		GuardDisabled: true,
	}).Error; err != nil {
		t.Fatalf("create guard state: %v", err)
	}
	client := &guardCPAStub{files: []authfiles.AuthFile{
		{AuthIndex: "guard-auth", Name: "guard.json", Status: "error", StatusMessage: "unauthorized", Unavailable: true},
		{AuthIndex: "banned-auth", Name: "banned.json", Status: "error", StatusMessage: "unauthorized", Unavailable: true},
		{AuthIndex: "quota-auth", Name: "quota.json", Status: "error", StatusMessage: "quota exhausted", Unavailable: true},
	}}
	service := NewService(db, client, Config{
		RemoveBannedStatusMessages: []string{"unauthorized", "payment_required", "not_found"},
	})

	result, err := service.CleanupBanned(context.Background(), false)
	if err != nil {
		t.Fatalf("CleanupBanned returned error: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Name != "banned.json" {
		t.Fatalf("unexpected cleanup candidates: %+v", result.Candidates)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "banned.json" {
		t.Fatalf("unexpected deleted files: %+v", client.deleted)
	}
}

func TestCleanupBannedDryRunDoesNotDelete(t *testing.T) {
	db := openGuardTestDB(t)
	client := &guardCPAStub{files: []authfiles.AuthFile{{
		AuthIndex:     "banned-auth",
		Name:          "banned.json",
		Status:        "error",
		StatusMessage: "payment_required",
		Unavailable:   true,
	}}}
	service := NewService(db, client, Config{})

	result, err := service.CleanupBanned(context.Background(), true)
	if err != nil {
		t.Fatalf("CleanupBanned returned error: %v", err)
	}
	if !result.DryRun || len(result.Candidates) != 1 {
		t.Fatalf("unexpected dry run result: %+v", result)
	}
	if len(client.deleted) != 0 {
		t.Fatalf("expected dry run not to delete, got %+v", client.deleted)
	}
}

func TestWeekWindowUsesConfiguredLocalReset(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.FixedZone("UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocal })

	start, end := WeekWindow(time.Date(2026, 5, 12, 1, 0, 0, 0, time.Local), time.Monday, 8)
	expectedStart := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	if !start.Equal(expectedStart) || !end.Equal(expectedEnd) {
		t.Fatalf("expected %s-%s, got %s-%s", expectedStart, expectedEnd, start, end)
	}
}

type patchCall struct {
	name     string
	disabled bool
}

type guardCPAStub struct {
	files     []authfiles.AuthFile
	patches   []patchCall
	deleted   []string
	fetchErr  error
	patchErr  error
	deleteErr error
}

type guardQuotaStub struct {
	responses map[string]quota.CheckResponse
	err       error
}

func (s *guardQuotaStub) Check(_ context.Context, request quota.CheckRequest) (quota.CheckResponse, error) {
	if s.err != nil {
		return quota.CheckResponse{}, s.err
	}
	if response, ok := s.responses[request.AuthIndex]; ok {
		return response, nil
	}
	return quota.CheckResponse{ID: request.AuthIndex}, nil
}

func floatPtr(value float64) *float64 {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func (s *guardCPAStub) FetchAuthFiles(context.Context) (*response.AuthFilesResult, error) {
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	return &response.AuthFilesResult{Payload: authfiles.AuthFilesResponse{Files: s.files}}, nil
}

func (s *guardCPAStub) PatchAuthFileStatus(_ context.Context, name string, disabled bool) (*response.AuthFileStatusResult, error) {
	if s.patchErr != nil {
		return nil, s.patchErr
	}
	s.patches = append(s.patches, patchCall{name: name, disabled: disabled})
	return &response.AuthFileStatusResult{Payload: authfiles.AuthFileStatusResponse{Status: "ok", Disabled: disabled}}, nil
}

func (s *guardCPAStub) DeleteAuthFiles(_ context.Context, names []string) (*response.AuthFilesDeleteResult, error) {
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	s.deleted = append(s.deleted, names...)
	return &response.AuthFilesDeleteResult{Payload: authfiles.AuthFilesDeleteResponse{Status: "ok", Deleted: len(names), Files: names}}, nil
}

func openGuardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "guard.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(entities.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("load sql db: %v", err)
		}
		_ = sqlDB.Close()
	})
	return db
}
