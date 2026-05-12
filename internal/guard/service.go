package guard

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/authfiles"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

type CPAClient interface {
	FetchAuthFiles(context.Context) (*response.AuthFilesResult, error)
	PatchAuthFileStatus(context.Context, string, bool) (*response.AuthFileStatusResult, error)
	DeleteAuthFiles(context.Context, []string) (*response.AuthFilesDeleteResult, error)
}

type QuotaChecker interface {
	Check(context.Context, quota.CheckRequest) (quota.CheckResponse, error)
}

type Config struct {
	Enabled                    bool
	Interval                   time.Duration
	UsageThreshold             float64
	WeeklyTokenLimit           int64
	ProviderQuotaEnabled       bool
	DryRun                     bool
	AutoReenable               bool
	ResetWeekday               time.Weekday
	ResetHour                  int
	RemoveBannedEnabled        bool
	RemoveBannedDryRun         bool
	RemoveBannedStatusMessages []string
}

type Service struct {
	db           *gorm.DB
	client       CPAClient
	quotaChecker QuotaChecker
	config       Config
	now          func() time.Time
}

type RunResult struct {
	WindowStart     time.Time     `json:"window_start"`
	WindowEnd       time.Time     `json:"window_end"`
	Checked         int           `json:"checked"`
	ThresholdHits   int           `json:"threshold_hits"`
	Disabled        int           `json:"disabled"`
	DryRunDisabled  int           `json:"dry_run_disabled"`
	AlreadyDisabled int           `json:"already_disabled"`
	Reenabled       int           `json:"reenabled"`
	DryRunReenabled int           `json:"dry_run_reenabled"`
	Cleanup         CleanupResult `json:"cleanup"`
}

type CleanupResult struct {
	DryRun     bool               `json:"dry_run"`
	Candidates []CleanupCandidate `json:"candidates"`
	Deleted    []string           `json:"deleted"`
	Failed     []DeleteFailure    `json:"failed"`
}

type CleanupCandidate struct {
	AuthIndex     string `json:"auth_index"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Provider      string `json:"provider"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	Reason        string `json:"reason"`
}

type DeleteFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

func NewService(db *gorm.DB, client CPAClient, config Config) *Service {
	return NewServiceWithQuotaChecker(db, client, nil, config)
}

func NewServiceWithQuotaChecker(db *gorm.DB, client CPAClient, quotaChecker QuotaChecker, config Config) *Service {
	return &Service{
		db:           db,
		client:       client,
		quotaChecker: quotaChecker,
		config:       normalizeConfig(config),
		now:          time.Now,
	}
}

func (s *Service) RunOnce(ctx context.Context) (RunResult, error) {
	if err := s.validate(); err != nil {
		return RunResult{}, err
	}

	now := s.now()
	windowStart, windowEnd := WeekWindow(now, s.config.ResetWeekday, s.config.ResetHour)
	result := RunResult{WindowStart: windowStart, WindowEnd: windowEnd}
	var errs []error

	if s.config.Enabled {
		enforceResult, err := s.enforceUsageThreshold(ctx, now, windowStart, windowEnd)
		result.Checked = enforceResult.Checked
		result.ThresholdHits = enforceResult.ThresholdHits
		result.Disabled = enforceResult.Disabled
		result.DryRunDisabled = enforceResult.DryRunDisabled
		result.AlreadyDisabled = enforceResult.AlreadyDisabled
		result.Reenabled = enforceResult.Reenabled
		result.DryRunReenabled = enforceResult.DryRunReenabled
		if err != nil {
			errs = append(errs, err)
		}
	}

	if s.config.RemoveBannedEnabled {
		cleanupResult, err := s.CleanupBanned(ctx, s.config.RemoveBannedDryRun)
		result.Cleanup = cleanupResult
		if err != nil {
			errs = append(errs, err)
		}
	}

	return result, errors.Join(errs...)
}

func (s *Service) CleanupBanned(ctx context.Context, dryRun bool) (CleanupResult, error) {
	if err := s.validate(); err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{DryRun: dryRun}

	authFiles, err := s.client.FetchAuthFiles(ctx)
	if err != nil {
		return result, fmt.Errorf("fetch auth files for banned cleanup: %w", err)
	}
	guardStates, err := repository.ListActiveAccountGuardStates(ctx, s.db)
	if err != nil {
		return result, err
	}
	guardDisabled := guardDisabledLookup(guardStates)

	names := make([]string, 0)
	for _, file := range authFiles.Payload.Files {
		if isGuardDisabledAuthFile(file, guardDisabled) {
			continue
		}
		candidate, ok := bannedCleanupCandidate(file, s.config.RemoveBannedStatusMessages)
		if !ok {
			continue
		}
		result.Candidates = append(result.Candidates, candidate)
		if candidate.Name != "" {
			names = append(names, candidate.Name)
		}
	}
	if dryRun || len(names) == 0 {
		return result, nil
	}

	deleteResult, err := s.client.DeleteAuthFiles(ctx, names)
	if err != nil {
		return result, fmt.Errorf("delete banned auth files: %w", err)
	}
	result.Deleted = append(result.Deleted, deleteResult.Payload.Files...)
	if len(result.Deleted) == 0 && deleteResult.Payload.Deleted > 0 {
		result.Deleted = names[:min(deleteResult.Payload.Deleted, len(names))]
	}
	for _, failure := range deleteResult.Payload.Failed {
		result.Failed = append(result.Failed, DeleteFailure{Name: failure.Name, Error: failure.Error})
	}
	return result, nil
}

type enforceResult struct {
	Checked         int
	ThresholdHits   int
	Disabled        int
	DryRunDisabled  int
	AlreadyDisabled int
	Reenabled       int
	DryRunReenabled int
}

type thresholdDecision struct {
	Hit         bool
	UsedTokens  int64
	LimitTokens int64
	UsedPercent float64
}

func (s *Service) enforceUsageThreshold(ctx context.Context, now, windowStart, windowEnd time.Time) (enforceResult, error) {
	var result enforceResult

	authFiles, err := s.client.FetchAuthFiles(ctx)
	if err != nil {
		return result, fmt.Errorf("fetch auth files for account guard: %w", err)
	}
	filesByAuthIndex := authFilesByAuthIndex(authFiles.Payload.Files)

	activeStates, err := repository.ListActiveAccountGuardStates(ctx, s.db)
	if err != nil {
		return result, err
	}
	activeGuardStates := accountGuardStatesByAuthIndex(activeStates)
	if s.config.AutoReenable {
		reenabled, dryRunReenabled, err := s.reenableExpiredGuardStates(ctx, now, filesByAuthIndex, activeGuardStates)
		result.Reenabled = reenabled
		result.DryRunReenabled = dryRunReenabled
		if err != nil {
			return result, err
		}
	}

	identities, err := repository.ListActiveUsageIdentities(ctx, s.db)
	if err != nil {
		return result, err
	}

	var errs []error
	for _, identity := range identities {
		if identity.AuthType != entities.UsageIdentityAuthTypeAuthFile || strings.TrimSpace(identity.Identity) == "" {
			continue
		}
		file, ok := filesByAuthIndex[identity.Identity]
		if !ok {
			continue
		}
		result.Checked++
		decision, err := s.accountThresholdDecision(ctx, identity, windowStart, windowEnd)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !decision.Hit {
			continue
		}
		result.ThresholdHits++
		if _, ok := activeGuardStates[identity.Identity]; ok {
			result.AlreadyDisabled++
			if !s.config.DryRun {
				errs = append(errs, s.recordGuardDisabled(ctx, file, now, windowEnd, decision))
			}
			continue
		}
		if file.Disabled || strings.EqualFold(strings.TrimSpace(file.Status), "disabled") {
			result.AlreadyDisabled++
			continue
		}
		if s.config.DryRun {
			result.DryRunDisabled++
			continue
		}
		patchName := firstNonEmpty(file.Name, file.ID, file.AuthIndex)
		if _, err := s.client.PatchAuthFileStatus(ctx, patchName, true); err != nil {
			errs = append(errs, fmt.Errorf("disable auth file %q: %w", patchName, err))
			continue
		}
		if err := s.recordGuardDisabled(ctx, file, now, windowEnd, decision); err != nil {
			errs = append(errs, err)
			continue
		}
		activeGuardStates[file.AuthIndex] = entities.AccountGuardState{AuthIndex: file.AuthIndex, Name: file.Name, GuardDisabled: true}
		result.Disabled++
	}

	return result, errors.Join(errs...)
}

func (s *Service) accountThresholdDecision(ctx context.Context, identity entities.UsageIdentity, windowStart, windowEnd time.Time) (thresholdDecision, error) {
	if s.config.ProviderQuotaEnabled && s.quotaChecker != nil {
		usedPercent, ok, err := s.providerWeeklyUsedPercent(ctx, identity.Identity)
		if err != nil && s.config.WeeklyTokenLimit <= 0 {
			return thresholdDecision{}, err
		}
		if ok {
			return thresholdDecision{
				Hit:         usedPercent+epsilon >= s.config.UsageThreshold,
				UsedPercent: usedPercent,
			}, nil
		}
	}

	if s.config.WeeklyTokenLimit <= 0 {
		return thresholdDecision{}, nil
	}
	usedTokens, err := repository.SumAuthFileUsageTokens(ctx, s.db, identity.Identity, windowStart, windowEnd)
	if err != nil {
		return thresholdDecision{}, err
	}
	usedPercent := float64(usedTokens) / float64(s.config.WeeklyTokenLimit)
	return thresholdDecision{
		Hit:         usedPercent+epsilon >= s.config.UsageThreshold,
		UsedTokens:  usedTokens,
		LimitTokens: s.config.WeeklyTokenLimit,
		UsedPercent: usedPercent,
	}, nil
}

func (s *Service) providerWeeklyUsedPercent(ctx context.Context, authIndex string) (float64, bool, error) {
	response, err := s.quotaChecker.Check(ctx, quota.CheckRequest{AuthIndex: authIndex})
	if err != nil {
		return 0, false, fmt.Errorf("check provider quota for %q: %w", authIndex, err)
	}
	maxUsedPercent := 0.0
	found := false
	for _, row := range response.Quota {
		if !isWeeklyQuotaRow(row) {
			continue
		}
		if usedPercent, ok := quotaRowUsedRatio(row); ok {
			maxUsedPercent = math.Max(maxUsedPercent, usedPercent)
			found = true
		}
	}
	return maxUsedPercent, found, nil
}

func isWeeklyQuotaRow(row quota.QuotaRow) bool {
	if row.Window != nil && row.Window.Seconds != nil && *row.Window.Seconds == 604800 {
		return true
	}
	haystack := normalizeStatusText(strings.Join([]string{row.Key, row.Label, row.Scope, row.Metric}, " "))
	return strings.Contains(haystack, "weekly") ||
		strings.Contains(haystack, "seven_day") ||
		strings.Contains(haystack, "7d") ||
		strings.Contains(haystack, "secondary_window")
}

func quotaRowUsedRatio(row quota.QuotaRow) (float64, bool) {
	if row.UsedPercent != nil {
		return normalizeQuotaPercent(*row.UsedPercent), true
	}
	if row.RemainingFraction != nil {
		return clampRatio(1 - *row.RemainingFraction), true
	}
	if row.Used != nil && row.Limit != nil && *row.Limit > 0 {
		return clampRatio(*row.Used / *row.Limit), true
	}
	return 0, false
}

func normalizeQuotaPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value > 1 {
		return clampRatio(value / 100)
	}
	return clampRatio(value)
}

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func (s *Service) reenableExpiredGuardStates(ctx context.Context, now time.Time, filesByAuthIndex map[string]authfiles.AuthFile, activeStates map[string]entities.AccountGuardState) (int, int, error) {
	var reenabled int
	var dryRunReenabled int
	var errs []error

	for authIndex, state := range activeStates {
		if state.ReenableAt == nil || now.Before(*state.ReenableAt) {
			continue
		}
		file, ok := filesByAuthIndex[authIndex]
		if !ok {
			continue
		}
		if s.config.DryRun {
			dryRunReenabled++
			continue
		}
		if _, err := s.client.PatchAuthFileStatus(ctx, firstNonEmpty(file.Name, state.Name, authIndex), false); err != nil {
			errs = append(errs, fmt.Errorf("reenable auth file %q: %w", firstNonEmpty(file.Name, state.Name, authIndex), err))
			continue
		}
		if err := repository.MarkAccountGuardStateReenabled(ctx, s.db, authIndex, now); err != nil {
			errs = append(errs, err)
			continue
		}
		delete(activeStates, authIndex)
		reenabled++
	}

	return reenabled, dryRunReenabled, errors.Join(errs...)
}

func (s *Service) recordGuardDisabled(ctx context.Context, file authfiles.AuthFile, now, reenableAt time.Time, decision thresholdDecision) error {
	disabledAt := now.UTC()
	reenable := reenableAt.UTC()
	state := entities.AccountGuardState{
		AuthIndex:       strings.TrimSpace(file.AuthIndex),
		Name:            strings.TrimSpace(firstNonEmpty(file.Name, file.ID, file.AuthIndex)),
		Reason:          entities.AccountGuardReasonQuotaThreshold,
		GuardDisabled:   true,
		DisabledAt:      &disabledAt,
		ReenableAt:      &reenable,
		LastUsedTokens:  decision.UsedTokens,
		LastLimitTokens: decision.LimitTokens,
		LastUsedPercent: roundPercent(decision.UsedPercent),
	}
	if err := repository.UpsertAccountGuardState(ctx, s.db, state); err != nil {
		return fmt.Errorf("record account guard state for %q: %w", file.AuthIndex, err)
	}
	return nil
}

func (s *Service) validate() error {
	if s == nil {
		return fmt.Errorf("account guard service is nil")
	}
	if s.db == nil {
		return fmt.Errorf("account guard database is nil")
	}
	if s.client == nil {
		return fmt.Errorf("account guard cpa client is nil")
	}
	if s.now == nil {
		s.now = time.Now
	}
	return nil
}

func WeekWindow(now time.Time, resetWeekday time.Weekday, resetHour int) (time.Time, time.Time) {
	if resetHour < 0 {
		resetHour = 0
	}
	if resetHour > 23 {
		resetHour = 23
	}
	localNow := now.In(time.Local)
	daysSinceReset := (int(localNow.Weekday()) - int(resetWeekday) + 7) % 7
	candidate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), resetHour, 0, 0, 0, time.Local).
		AddDate(0, 0, -daysSinceReset)
	if localNow.Before(candidate) {
		candidate = candidate.AddDate(0, 0, -7)
	}
	return candidate.UTC(), candidate.AddDate(0, 0, 7).UTC()
}

const epsilon = 1e-9

func normalizeConfig(config Config) Config {
	if config.Interval <= 0 {
		config.Interval = 5 * time.Minute
	}
	if config.UsageThreshold <= 0 {
		config.UsageThreshold = 0.8
	}
	if config.UsageThreshold > 1 {
		config.UsageThreshold = 1
	}
	if config.ResetHour < 0 {
		config.ResetHour = 0
	}
	if config.ResetHour > 23 {
		config.ResetHour = 23
	}
	if len(config.RemoveBannedStatusMessages) == 0 {
		config.RemoveBannedStatusMessages = []string{"unauthorized", "payment_required", "not_found"}
	}
	return config
}

func authFilesByAuthIndex(files []authfiles.AuthFile) map[string]authfiles.AuthFile {
	byAuthIndex := make(map[string]authfiles.AuthFile, len(files))
	for _, file := range files {
		authIndex := strings.TrimSpace(file.AuthIndex)
		if authIndex == "" {
			continue
		}
		byAuthIndex[authIndex] = file
	}
	return byAuthIndex
}

func accountGuardStatesByAuthIndex(states []entities.AccountGuardState) map[string]entities.AccountGuardState {
	byAuthIndex := make(map[string]entities.AccountGuardState, len(states))
	for _, state := range states {
		if authIndex := strings.TrimSpace(state.AuthIndex); authIndex != "" {
			byAuthIndex[authIndex] = state
		}
	}
	return byAuthIndex
}

func guardDisabledLookup(states []entities.AccountGuardState) map[string]entities.AccountGuardState {
	return accountGuardStatesByAuthIndex(states)
}

func isGuardDisabledAuthFile(file authfiles.AuthFile, states map[string]entities.AccountGuardState) bool {
	if _, ok := states[strings.TrimSpace(file.AuthIndex)]; ok {
		return true
	}
	name := strings.TrimSpace(file.Name)
	if name == "" {
		return false
	}
	for _, state := range states {
		if strings.EqualFold(strings.TrimSpace(state.Name), name) {
			return true
		}
	}
	return false
}

func bannedCleanupCandidate(file authfiles.AuthFile, messages []string) (CleanupCandidate, bool) {
	name := strings.TrimSpace(firstNonEmpty(file.Name, file.ID))
	if name == "" || file.RuntimeOnly {
		return CleanupCandidate{}, false
	}
	status := normalizeStatusText(file.Status)
	statusMessage := normalizeStatusText(file.StatusMessage)
	if file.Disabled || status == "disabled" {
		return CleanupCandidate{}, false
	}
	if !file.Unavailable && status != "error" {
		return CleanupCandidate{}, false
	}
	if statusMessage == "" || strings.Contains(statusMessage, "quota") {
		return CleanupCandidate{}, false
	}
	for _, message := range messages {
		needle := normalizeStatusText(message)
		if needle == "" {
			continue
		}
		if strings.Contains(statusMessage, needle) {
			return CleanupCandidate{
				AuthIndex:     strings.TrimSpace(file.AuthIndex),
				Name:          name,
				Type:          strings.TrimSpace(file.Type),
				Provider:      strings.TrimSpace(file.Provider),
				Status:        strings.TrimSpace(file.Status),
				StatusMessage: strings.TrimSpace(file.StatusMessage),
				Reason:        needle,
			}, true
		}
	}
	return CleanupCandidate{}, false
}

func normalizeStatusText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func roundPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*10000) / 10000
}
