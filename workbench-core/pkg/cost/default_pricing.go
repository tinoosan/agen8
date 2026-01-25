package cost

func DefaultPricing() PricingFile {
	models := make(map[string]ModelPricing, len(modelInfos))
	for _, info := range modelInfos {
		if info.InputPerM == 0 && info.OutputPerM == 0 {
			continue
		}
		models[info.ID] = ModelPricing{
			InputPerM:  info.InputPerM,
			OutputPerM: info.OutputPerM,
		}
	}

	return PricingFile{
		Version:  "v1",
		Currency: "USD",
		Models:   models,
	}
}
