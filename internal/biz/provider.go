package biz

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewBOMUseCase,
	NewBOMSessionUseCase,
	NewMatchUseCase,
	NewAgentHub,
)
