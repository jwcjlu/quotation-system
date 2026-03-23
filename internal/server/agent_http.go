package server

import (
	"errors"
	"net/http"

	v1 "caichip/api/agent/v1"
	"caichip/internal/biz"
	"caichip/internal/service"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterAgentHTTPServer 注册 Agent 相关路由（由 agent.proto + protoc-gen-go-http 生成）。
func RegisterAgentHTTPServer(s *khttp.Server, svc *service.AgentService) {
	if svc == nil || !svc.Enabled() {
		return
	}
	v1.RegisterAgentServiceHTTPServer(s, svc)
	if svc.DevEnqueueEnabled() {
		r := s.Route("/")
		r.POST("/api/v1/agent/dev/enqueue", agentDevEnqueue(svc))
	}
}

type devEnqueueReq struct {
	TaskID   string `json:"task_id"`
	ScriptID string `json:"script_id"`
	Version  string `json:"version"`
	Queue    string `json:"queue"`
}

func agentDevEnqueue(svc *service.AgentService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if !authAgent(ctx, svc) {
			return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing api key")
		}
		var req devEnqueueReq
		if err := ctx.Bind(&req); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		if req.ScriptID == "" || req.Version == "" {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", "script_id and version required")
		}
		svc.DevEnqueue(&biz.QueuedTask{
			TaskMessage: biz.TaskMessage{
				TaskID:   req.TaskID,
				ScriptID: req.ScriptID,
				Version:  req.Version,
			},
			Queue: req.Queue,
		})
		return ctx.Result(200, map[string]any{"enqueued": true})
	}
}

func authAgent(ctx khttp.Context, svc *service.AgentService) bool {
	r := ctx.Request()
	return svc.ValidateAPIKey(r.Header.Get("Authorization"), r.Header.Get("X-API-Key"))
}

func jsonErr(ctx khttp.Context, status int, code, msg string) error {
	b := &v1.ErrorBody{
		Error: &v1.ErrorDetail{
			Code:    code,
			Message: msg,
		},
	}
	return ctx.Result(status, b)
}

func mapSvcErr(ctx khttp.Context, err error) error {
	if err == nil {
		return nil
	}
	var br *service.BadRequestError
	if errors.As(err, &br) {
		return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", br.Message)
	}
	return jsonErr(ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
}
