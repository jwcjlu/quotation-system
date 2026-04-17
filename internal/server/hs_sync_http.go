package server

import (
	"net/http"
	"strconv"
	"strings"

	"caichip/internal/biz"
	"caichip/internal/service"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func RegisterHsSyncRoutes(s *khttp.Server, syncSvc *service.HsSyncService) {
	if s == nil || syncSvc == nil {
		return
	}
	r := s.Route("/")
	r.POST("/api/hs/sync/run", hsSyncRun(syncSvc))
	r.GET("/api/hs/sync/jobs", hsSyncJobs(syncSvc))
	r.GET("/api/hs/sync/job_detail", hsSyncJobDetail(syncSvc))
	r.GET("/api/hs/items", hsItems(syncSvc))
	r.GET("/api/hs/items/{code_ts}", hsItemDetail(syncSvc))
}

func hsSyncRun(syncSvc *service.HsSyncService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in struct {
			Mode    string   `json:"mode"`
			CoreHS6 []string `json:"core_hs6"`
		}
		if err := ctx.Bind(&in); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		out, err := syncSvc.Run(ctx.Request().Context(), in.Mode, in.CoreHS6, "")
		if err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		return ctx.Result(http.StatusOK, out)
	}
}

func hsSyncJobs(syncSvc *service.HsSyncService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		page, _ := strconv.Atoi(ctx.Request().URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(ctx.Request().URL.Query().Get("page_size"))
		out := syncSvc.ListJobs(int32(page), int32(pageSize))
		return ctx.Result(http.StatusOK, out)
	}
}

func hsSyncJobDetail(syncSvc *service.HsSyncService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		id, _ := strconv.ParseInt(strings.TrimSpace(ctx.Request().URL.Query().Get("id")), 10, 64)
		if id <= 0 {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", "id 无效")
		}
		out, ok := syncSvc.JobDetail(id)
		if !ok {
			return jsonErr(ctx, http.StatusNotFound, "NOT_FOUND", "job 不存在")
		}
		return ctx.Result(http.StatusOK, out)
	}
}

func hsItems(syncSvc *service.HsSyncService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		page, _ := strconv.Atoi(ctx.Request().URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(ctx.Request().URL.Query().Get("page_size"))
		out, err := syncSvc.ListItems(ctx.Request().Context(), biz.HsItemListFilter{
			Page:          int32(page),
			PageSize:      int32(pageSize),
			CodeTS:        ctx.Request().URL.Query().Get("code_ts"),
			GName:         ctx.Request().URL.Query().Get("g_name"),
			SourceCoreHS6: ctx.Request().URL.Query().Get("source_core_hs6"),
		})
		if err != nil {
			return jsonErr(ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		}
		return ctx.Result(http.StatusOK, out)
	}
}

func hsItemDetail(syncSvc *service.HsSyncService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in struct {
			CodeTS string `json:"code_ts"`
		}
		if err := ctx.BindVars(&in); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		out, ok, err := syncSvc.ItemDetail(ctx.Request().Context(), in.CodeTS)
		if err != nil {
			return jsonErr(ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		}
		if !ok {
			return jsonErr(ctx, http.StatusNotFound, "NOT_FOUND", "item 不存在")
		}
		return ctx.Result(http.StatusOK, out)
	}
}
