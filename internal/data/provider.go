package data

import (
	"caichip/internal/biz"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewData,
	NewOpenAIChat,
	NewInprocKV,
	NewAgentScriptAuthRepo,
	NewCachedAgentScriptAuthRepo,
	wire.Bind(new(biz.AgentScriptAuthRepo), new(*CachedAgentScriptAuthRepo)),
	NewBOMSearchTaskRepo,
	NewBomSessionRepo,
	NewBomPlatformScriptRepo,
	NewCachedBomPlatformScriptRepo,
	wire.Bind(new(biz.BomPlatformScriptRepo), new(*CachedBomPlatformScriptRepo)),
	NewBomMergeProxyWaitRepo,
	NewKuaidailiClient,
	NewBomMergeDispatch,
	NewMergeProxyRetryWorker,
	NewBomFxRateRepoFromData,
	NewBomManufacturerAliasRepo,
	NewCachedBomManufacturerAliasRepo,
	wire.Bind(new(biz.BomManufacturerAliasRepo), new(*CachedBomManufacturerAliasRepo)),
	NewAgentScriptPackageRepo,
	NewDispatchTaskRepo,
	NewAgentRegistryRepo,
	NewCachedAgentRegistryRepo,
	wire.Bind(new(biz.AgentRegistryRepo), new(*CachedAgentRegistryRepo)),
	NewTableCacheRefresher,
	NewHSPolicyRepo,
	wire.Bind(new(biz.HSPolicyRepo), new(*HSPolicyRepo)),
	NewHSCaseRepo,
	wire.Bind(new(biz.HSCaseRepo), new(*HSCaseRepo)),
	NewHSReviewRepo,
	wire.Bind(new(biz.HSReviewRepo), new(*HSReviewRepo)),
)
