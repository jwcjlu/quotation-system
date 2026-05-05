package server

import (
	"context"
	"io"
	stdhttp "net/http"
	"path/filepath"
	"strings"

	v1 "caichip/api/bom/v1"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// HsResolveByModelHTTPStatus 设计 §10：同步完成 200；已转异步（accepted）202。
func HsResolveByModelHTTPStatus(reply *v1.HsResolveByModelReply) int {
	if reply != nil && reply.Accepted {
		return stdhttp.StatusAccepted
	}
	return stdhttp.StatusOK
}

// hsManualDatasheetUploader 可选：由 *service.HsResolveService 实现。
type hsManualDatasheetUploader interface {
	UploadHsManualDatasheet(context.Context, *v1.UploadHsManualDatasheetRequest) (*v1.UploadHsManualDatasheetReply, error)
}

// RegisterHsResolveServiceHTTPServer 覆盖生成代码中固定 200 的行为，使 ResolveByModel 符合 §10 状态码语义。
func RegisterHsResolveServiceHTTPServer(s *khttp.Server, srv v1.HsResolveServiceHTTPServer) {
	r := s.Route("/")
	r.POST("/api/hs/resolve/by-model", hsResolveByModelHTTPHandler(srv))
	r.POST("/api/hs/resolve/by-models:batch", hsResolveBatchByModelsHTTPHandler(srv))
	r.GET("/api/hs/resolve/task", hsResolveGetTaskHTTPHandler(srv))
	r.POST("/api/hs/resolve/confirm", hsResolveConfirmHTTPHandler(srv))
	r.GET("/api/hs/resolve/history", hsResolveHistoryHTTPHandler(srv))
	r.GET("/api/hs/resolve/pending-reviews", hsResolvePendingReviewsHTTPHandler(srv))
	if u, ok := srv.(hsManualDatasheetUploader); ok {
		r.POST("/api/hs/resolve/manual-datasheet/upload", hsManualUploadHTTPHandler(u))
	}
}

func hsManualUploadHTTPHandler(u hsManualDatasheetUploader) func(ctx khttp.Context) error {
	const maxMultipart = 32 << 20
	return func(ctx khttp.Context) error {
		req := ctx.Request()
		if err := req.ParseMultipartForm(maxMultipart); err != nil {
			return err
		}
		f, hdr, err := req.FormFile("file")
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		filename := strings.TrimSpace(hdr.Filename)
		if filename == "" {
			filename = "upload.pdf"
		}
		filename = filepath.Base(filename)
		body, err := io.ReadAll(io.LimitReader(f, maxMultipart+1))
		if err != nil {
			return err
		}
		if len(body) > maxMultipart {
			return kerrors.BadRequest("HS_RESOLVE_BAD_REQUEST", "file too large")
		}
		khttp.SetOperation(ctx, v1.HsResolveService_UploadHsManualDatasheet_FullMethodName)
		out, err := u.UploadHsManualDatasheet(req.Context(), &v1.UploadHsManualDatasheetRequest{
			File:     body,
			Filename: filename,
		})
		if err != nil {
			return err
		}
		return ctx.Result(stdhttp.StatusOK, out)
	}
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

func hsResolveBatchByModelsHTTPHandler(srv v1.HsResolveServiceHTTPServer) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in v1.HsBatchResolveByModelsRequest
		if err := ctx.Bind(&in); err != nil {
			return err
		}
		if err := ctx.BindQuery(&in); err != nil {
			return err
		}
		khttp.SetOperation(ctx, v1.HsResolveService_BatchResolveByModels_FullMethodName)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.BatchResolveByModels(ctx, req.(*v1.HsBatchResolveByModelsRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		reply := out.(*v1.HsBatchResolveByModelsReply)
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

func hsResolvePendingReviewsHTTPHandler(srv v1.HsResolveServiceHTTPServer) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in v1.HsPendingReviewsRequest
		if err := ctx.BindQuery(&in); err != nil {
			return err
		}
		khttp.SetOperation(ctx, v1.HsResolveService_ListPendingReviews_FullMethodName)
		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.ListPendingReviews(ctx, req.(*v1.HsPendingReviewsRequest))
		})
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}
		reply := out.(*v1.HsPendingReviewsReply)
		return ctx.Result(stdhttp.StatusOK, reply)
	}
}
