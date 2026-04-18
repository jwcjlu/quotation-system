package server

import (
	"context"
	stdhttp "net/http"

	v1 "caichip/api/bom/v1"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// HsResolveByModelHTTPStatus 设计 §10：同步完成 200；已转异步（accepted）202。
func HsResolveByModelHTTPStatus(reply *v1.HsResolveByModelReply) int {
	if reply != nil && reply.Accepted {
		return stdhttp.StatusAccepted
	}
	return stdhttp.StatusOK
}

// RegisterHsResolveServiceHTTPServer 覆盖生成代码中固定 200 的行为，使 ResolveByModel 符合 §10 状态码语义。
func RegisterHsResolveServiceHTTPServer(s *khttp.Server, srv v1.HsResolveServiceHTTPServer) {
	r := s.Route("/")
	r.POST("/api/hs/resolve/by-model", hsResolveByModelHTTPHandler(srv))
	r.GET("/api/hs/resolve/task", hsResolveGetTaskHTTPHandler(srv))
	r.POST("/api/hs/resolve/confirm", hsResolveConfirmHTTPHandler(srv))
	r.GET("/api/hs/resolve/history", hsResolveHistoryHTTPHandler(srv))
}

func hsResolveByModelHTTPHandler(srv v1.HsResolveServiceHTTPServer) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in v1.HsResolveByModelRequest
		if err := ctx.Bind(&in); err != nil {
			return err
		}
		if err := ctx.BindQuery(&in); err != nil {
			return err
		}
		khttp.SetOperation(ctx, v1.OperationHsResolveServiceResolveByModel)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.ResolveByModel(ctx, req.(*v1.HsResolveByModelRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		reply := out.(*v1.HsResolveByModelReply)
		return ctx.Result(HsResolveByModelHTTPStatus(reply), reply)
	}
}

func hsResolveGetTaskHTTPHandler(srv v1.HsResolveServiceHTTPServer) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in v1.HsResolveTaskRequest
		if err := ctx.BindQuery(&in); err != nil {
			return err
		}
		khttp.SetOperation(ctx, v1.OperationHsResolveServiceGetResolveTask)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.GetResolveTask(ctx, req.(*v1.HsResolveTaskRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		reply := out.(*v1.HsResolveTaskReply)
		return ctx.Result(stdhttp.StatusOK, reply)
	}
}

func hsResolveConfirmHTTPHandler(srv v1.HsResolveServiceHTTPServer) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in v1.HsResolveConfirmRequest
		if err := ctx.Bind(&in); err != nil {
			return err
		}
		if err := ctx.BindQuery(&in); err != nil {
			return err
		}
		khttp.SetOperation(ctx, v1.OperationHsResolveServiceConfirmResolve)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.ConfirmResolve(ctx, req.(*v1.HsResolveConfirmRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		reply := out.(*v1.HsResolveConfirmReply)
		return ctx.Result(stdhttp.StatusOK, reply)
	}
}

func hsResolveHistoryHTTPHandler(srv v1.HsResolveServiceHTTPServer) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in v1.HsResolveHistoryRequest
		if err := ctx.BindQuery(&in); err != nil {
			return err
		}
		khttp.SetOperation(ctx, v1.OperationHsResolveServiceGetResolveHistory)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.GetResolveHistory(ctx, req.(*v1.HsResolveHistoryRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		reply := out.(*v1.HsResolveHistoryReply)
		return ctx.Result(stdhttp.StatusOK, reply)
	}
}
