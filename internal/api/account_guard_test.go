package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/guard"
)

func TestAccountGuardCleanupDefaultsToDryRun(t *testing.T) {
	provider := &accountGuardProviderStub{result: guard.CleanupResult{
		DryRun: true,
		Candidates: []guard.CleanupCandidate{{
			AuthIndex:     "auth-1",
			Name:          "banned.json",
			Status:        "error",
			StatusMessage: "unauthorized",
			Reason:        "unauthorized",
		}},
	}}
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{AccountGuard: provider})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account-guard/cleanup-banned", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(provider.dryRuns) != 1 || !provider.dryRuns[0] {
		t.Fatalf("expected dry_run=true call, got %+v", provider.dryRuns)
	}
	var body guard.CleanupResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.DryRun || len(body.Candidates) != 1 || body.Candidates[0].Name != "banned.json" {
		t.Fatalf("unexpected response body: %+v", body)
	}
}

func TestAccountGuardCleanupCanExecuteDelete(t *testing.T) {
	provider := &accountGuardProviderStub{result: guard.CleanupResult{DryRun: false, Deleted: []string{"banned.json"}}}
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{AccountGuard: provider})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account-guard/cleanup-banned", strings.NewReader(`{"dry_run":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(provider.dryRuns) != 1 || provider.dryRuns[0] {
		t.Fatalf("expected dry_run=false call, got %+v", provider.dryRuns)
	}
}

type accountGuardProviderStub struct {
	dryRuns []bool
	result  guard.CleanupResult
	err     error
}

func (s *accountGuardProviderStub) CleanupBanned(_ context.Context, dryRun bool) (guard.CleanupResult, error) {
	s.dryRuns = append(s.dryRuns, dryRun)
	if s.err != nil {
		return guard.CleanupResult{}, s.err
	}
	s.result.DryRun = dryRun
	return s.result, nil
}
