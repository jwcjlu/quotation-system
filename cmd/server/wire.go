//go:build wireinject
// +build wireinject

package main

import (
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"
	"caichip/internal/server"
	"caichip/internal/service"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireApp(*conf.Bootstrap, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		wire.Bind(new(biz.DispatchTaskRepo), new(*data.DispatchTaskRepo)),
		wire.Bind(new(biz.AgentRegistryRepo), new(*data.AgentRegistryRepo)),
		wire.Bind(new(biz.BOMSearchTaskRepo), new(*data.BOMSearchTaskRepo)),
		wire.Bind(new(biz.BOMSessionRepo), new(*data.BomSessionRepo)),
		wire.Bind(new(biz.MergeDispatchExecutor), new(*data.BomMergeDispatch)),
		wire.Bind(new(biz.AgentScriptPublishedLister), new(*data.AgentScriptPackageRepo)),
		wire.Bind(new(biz.AgentScriptAuthRepo), new(*data.AgentScriptAuthRepo)),
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		newApp,
	))
}
