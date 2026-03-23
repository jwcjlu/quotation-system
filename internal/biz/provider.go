package biz

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewBOMUseCase,
	NewSearchUseCase,
	NewMatchUseCase,
	NewAgentHub,
)
