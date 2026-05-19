package service

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/redact"
	"cpa-usage-keeper/internal/repository"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func TestUsageServiceClearsUsageByRedactedAPIAliasAndRebuildsIdentityStats(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	ctx := context.Background()
	identity := entities.UsageIdentity{
		Name:         "Provider",
		AuthType:     entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "source-a",
		Type:         "openai",
		Provider:     "OpenAI",
	}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	events := []entities.UsageEvent{
		{EventKey: "evt-clear", APIGroupKey: "sk-clear-me", AuthType: "apikey", AuthIndex: "source-a", Timestamp: time.Unix(1, 0), InputTokens: 10, TotalTokens: 10},
		{EventKey: "evt-keep", APIGroupKey: "sk-keep-me", AuthType: "apikey", AuthIndex: "source-a", Timestamp: time.Unix(2, 0), InputTokens: 20, TotalTokens: 20},
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatalf("seed usage events: %v", err)
	}
	if err := repository.AggregateUsageIdentityStats(ctx, db, time.Unix(3, 0)); err != nil {
		t.Fatalf("aggregate stats: %v", err)
	}

	result, err := (&usageService{db: db}).ClearUsage(ctx, servicedto.ClearUsageInput{APIAlias: redact.APIAlias("sk-clear-me")})
	if err != nil {
		t.Fatalf("clear usage: %v", err)
	}
	if result.DeletedEvents != 1 {
		t.Fatalf("expected one deleted event, got %+v", result)
	}

	var remaining int64
	if err := db.Model(&entities.UsageEvent{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining events: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected one remaining event, got %d", remaining)
	}

	var got entities.UsageIdentity
	if err := db.First(&got, identity.ID).Error; err != nil {
		t.Fatalf("load identity: %v", err)
	}
	if got.TotalRequests != 1 || got.InputTokens != 20 || got.TotalTokens != 20 {
		t.Fatalf("expected rebuilt stats from kept event, got %+v", got)
	}
}
