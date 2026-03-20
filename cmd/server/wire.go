//go:build wireinject
// +build wireinject

package main

import (
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"
	"caichip/internal/server"
	"caichip/internal/service"
	"caichip/pkg/platform/ickey"
	"caichip/pkg/platform/szlcsc"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireApp(*conf.Bootstrap, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		newSearchers,
		newApp,
	))
}

func newSearchers(c *conf.Bootstrap) []biz.PlatformSearcher {
	var list []biz.PlatformSearcher
	if c.Platform != nil {
		if c.Platform.Ickey != nil {
			crawlerPath := c.Platform.Ickey.CrawlerPath
			crawlerScript := c.Platform.Ickey.CrawlerScript
			if crawlerPath == "" {
				crawlerPath = "python"
			}
			if crawlerScript == "" {
				crawlerScript = "ickey_crawler.py"
			}
			list = append(list, ickey.NewClient(
				c.Platform.Ickey.SearchURL,
				c.Platform.Ickey.Timeout,
				crawlerPath,
				crawlerScript,
			))
		}
		if c.Platform.Szlcsc != nil {
			list = append(list, szlcsc.NewClient(
				c.Platform.Szlcsc.SearchURL,
				c.Platform.Szlcsc.Timeout,
			))
		}
	}
	if len(list) == 0 {
		list = append(list, ickey.NewClient("https://search.ickey.cn/", 15, "python", "ickey_crawler.py"))
		list = append(list, szlcsc.NewClient("https://www.szlcsc.com/", 15))
	}
	return list
}
