package server

import (
	"time"

	pb "caichip/api/bom/v1"
	"caichip/internal/conf"
	"caichip/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer 创建 HTTP 服务
func NewHTTPServer(c *conf.Bootstrap, logger log.Logger, bomSvc *service.BomService) *http.Server {
	addr := ":8000"
	timeout := 30 * time.Second
	if c != nil && c.Server != nil && c.Server.Http != nil {
		addr = c.Server.Http.Addr
		if c.Server.Http.Timeout > 0 {
			timeout = time.Duration(c.Server.Http.Timeout) * time.Second
		}
	}
	srv := http.NewServer(
		http.Address(addr),
		http.Timeout(timeout),
	)

	pb.RegisterBomServiceHTTPServer(srv, bomSvc)

	return srv
}
