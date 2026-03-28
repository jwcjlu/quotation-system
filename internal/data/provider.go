package data

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewData,
	NewOpenAIChat,
	NewAgentScriptAuthRepo,
	NewBOMSearchTaskRepo,
	NewBomSessionRepo,
	NewBomMergeDispatch,
	NewAgentScriptPackageRepo,
	NewDispatchTaskRepo,
	NewAgentRegistryRepo,
)
