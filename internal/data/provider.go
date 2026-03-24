package data

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewData,
	NewBOMRepo,
	NewSearchRepo,
	NewBOMSessionRepo,
	NewBOMSearchTaskRepo,
	NewBOMMatchHistoryRepo,
	NewAgentScriptPackageRepo,
	NewDispatchTaskRepo,
	NewAgentRegistryRepo,
)
