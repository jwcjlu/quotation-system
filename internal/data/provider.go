package data

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewData,
	NewBOMSearchTaskRepo,
	NewBomSessionRepo,
	NewBomMergeDispatch,
	NewAgentScriptPackageRepo,
	NewDispatchTaskRepo,
	NewAgentRegistryRepo,
)
