package dto

import "cpa-usage-keeper/internal/entities"

// UpdatePricingInput 是更新定价的服务层输入。
type UpdatePricingInput struct {
	Model                string
	PromptPricePer1M     float64
	CompletionPricePer1M float64
	CachePricePer1M      float64
}

type ApplyOfficialPricingInput struct {
	Multiplier float64
}

type ApplyOfficialPricingResult struct {
	Pricing       []entities.ModelPriceSetting
	SkippedModels []string
	Multiplier    float64
	Source        string
}
