package entities

import "testing"

func TestAllIncludesCoreModels(t *testing.T) {
	items := All()
	if len(items) != 6 {
		t.Fatalf("expected 6 models after adding account guard states, got %d", len(items))
	}
	if _, ok := items[0].(*UsageEvent); !ok {
		t.Fatalf("expected UsageEvent to be first registered model, got %T", items[0])
	}
	if _, ok := items[1].(*RedisUsageInbox); !ok {
		t.Fatalf("expected RedisUsageInbox to be registered, got %T", items[1])
	}
	if _, ok := items[4].(*APIKeyNote); !ok {
		t.Fatalf("expected APIKeyNote to be registered, got %T", items[4])
	}
	if _, ok := items[5].(*AccountGuardState); !ok {
		t.Fatalf("expected AccountGuardState to be registered, got %T", items[5])
	}
}
