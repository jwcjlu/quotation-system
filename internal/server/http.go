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
func NewHTTPServer(c *conf.Bootstrap, logger log.Logger, bomSvc *service.BomService, agentSvc *service.AgentService) *http.Server {
	addr := ":8000"
	timeout := 30 * time.Second
	if c != nil && c.Server != nil && c.Server.Http != nil {
		addr = c.Server.Http.Addr
		if c.Server.Http.Timeout > 0 {
			timeout = time.Duration(c.Server.Http.Timeout) * time.Second
		}
	}
	// Agent 长轮询可能接近 55s+，若配置偏短则抬升下限，避免心跳被服务端提前断开（见 docs）
	if timeout < 120*time.Second {
		if c != nil && c.Agent != nil && c.Agent.Enabled {
			timeout = 120 * time.Second
		}
	}
	opts := []http.ServerOption{
		http.Address(addr),
		http.Timeout(timeout),
	}
	if agentSvc != nil && agentSvc.Enabled() {
		opts = append(opts, http.Middleware(agentAPIKeyMiddleware(agentSvc)))
	}
	srv := http.NewServer(opts...)

	pb.RegisterBomServiceHTTPServer(srv, bomSvc)
	RegisterAgentHTTPServer(srv, agentSvc)

	return srv
}
