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
		wire.Bind(new(biz.BOMSearchTaskEnsurer), new(*data.BOMSearchTaskRepo)),
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		newApp,
	))
}
