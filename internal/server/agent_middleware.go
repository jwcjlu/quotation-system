package server

import (
	"context"
	"strings"

	"caichip/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// agentAPIKeyMiddleware 对 /api/v1/agent/* 校验 API Key（与协议 §1.2 一致）。
func agentAPIKeyMiddleware(svc *service.AgentService) middleware.Middleware {
	return func(h middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if svc == nil || !svc.Enabled() {
				return h(ctx, req)
			}
			r, ok := khttp.RequestFromServerContext(ctx)
			if !ok {
				return h(ctx, req)
			}
			if !strings.HasPrefix(r.URL.Path, "/api/v1/agent") {
				return h(ctx, req)
			}
			if !svc.ValidateAPIKey(r.Header.Get("Authorization"), r.Header.Get("X-API-Key")) {
				return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing api key")
			}
			return h(ctx, req)
		}
	}
}
