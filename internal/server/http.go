package server

import (
	"strings"
	"time"

	v1admin "caichip/api/admin/v1"
	v1bom "caichip/api/bom/v1"
	"caichip/internal/conf"
	"caichip/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer 创建 HTTP 服务（Agent、脚本包、BOM 会话、Agent 运维 API）。
func NewHTTPServer(c *conf.Bootstrap, logger log.Logger, agentSvc *service.AgentService, scriptAdmin *service.ScriptPackageAdmin, bomSvc *service.BomService, agentAdmin *service.AgentAdminService, hsResolve *service.HsResolveService, hsMeta *service.HsMetaService, hsSync *service.HsSyncService) *http.Server {
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

	RegisterAgentHTTPServer(srv, agentSvc)
	RegisterScriptPackageAdminRoutes(srv, scriptAdmin)
	if bomSvc != nil {
		v1bom.RegisterBomServiceHTTPServer(srv, bomSvc)
	}
	if agentAdmin != nil && agentAdmin.Enabled() {
		v1admin.RegisterAgentAdminServiceHTTPServer(srv, agentAdmin)
	}
	if hsResolve != nil {
		RegisterHsResolveServiceHTTPServer(srv, hsResolve)
	}
	if hsMeta != nil {
		v1bom.RegisterHsMetaServiceHTTPServer(srv, hsMeta)
	}
	RegisterHsSyncRoutes(srv, hsSync)

	if c != nil && c.ScriptStore != nil && c.ScriptStore.Enabled && strings.TrimSpace(c.ScriptStore.Root) != "" {
		pref := strings.TrimSpace(c.ScriptStore.UrlPrefix)
		if pref == "" {
			pref = "/static/agent-scripts"
		}
		if !strings.HasPrefix(pref, "/") {
			pref = "/" + pref
		}
		srv.HandlePrefix(pref, scriptStoreFileHandler(c.ScriptStore.Root, pref))
	}

	return srv
}
