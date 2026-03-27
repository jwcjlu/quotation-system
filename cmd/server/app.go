package main

import (
	"caichip/internal/conf"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func newApp(bc *conf.Bootstrap, logger log.Logger, hs *http.Server) *kratos.App {
	addr := ":8000"
	if bc != nil && bc.Server != nil && bc.Server.Http != nil {
		addr = bc.Server.Http.Addr
	}
	return kratos.New(
		kratos.ID(addr),
		kratos.Name("bom-match"),
		kratos.Version("1.0"),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs),
	)
}
