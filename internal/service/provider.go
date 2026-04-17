package service

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewAgentService,
	NewScriptPackageAdmin,
	NewBomService,
	NewAgentAdminService,
	NewDefaultHsResolveService,
	NewHsMetaService,
	NewHsSyncService,
)
