package entities

import "testing"

func TestAllIncludesCoreModels(t *testing.T) {
	items := All()
	if len(items) != 5 {
		t.Fatalf("expected 5 models after adding api key notes, got %d", len(items))
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
}
