package main

import (
	"context"

	"caichip/internal/conf"
	"caichip/internal/data"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func newApp(bc *conf.Bootstrap, logger log.Logger, hs *http.Server, refresher *data.TableCacheRefresher, mergeRetry *data.MergeProxyRetryWorker, manualJanitor *data.HsManualDatasheetJanitor) *kratos.App {
	addr := ":8000"
	if bc != nil && bc.Server != nil && bc.Server.Http != nil {
		addr = bc.Server.Http.Addr
	}
	opts := []kratos.Option{
		kratos.ID(addr),
		kratos.Name("bom-match"),
		kratos.Version("1.0"),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs),
	}
	if refresher != nil {
		opts = append(opts,
			kratos.BeforeStart(func(context.Context) error {
				refresher.Start()
				return nil
			}),
			kratos.BeforeStop(func(context.Context) error {
				refresher.Stop()
				return nil
			}),
		)
	}
	if mergeRetry != nil {
		opts = append(opts,
			kratos.BeforeStart(func(context.Context) error {
				mergeRetry.Start()
				return nil
			}),
			kratos.BeforeStop(func(context.Context) error {
				mergeRetry.Stop()
				return nil
			}),
		)
	}
	if manualJanitor != nil {
		opts = append(opts,
			kratos.BeforeStart(func(context.Context) error {
				manualJanitor.Start()
				return nil
			}),
			kratos.BeforeStop(func(context.Context) error {
				manualJanitor.Stop()
				return nil
			}),
		)
	}
	return kratos.New(opts...)
}
