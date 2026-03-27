package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"caichip/internal/data"
	"caichip/internal/service"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func authScriptAdmin(ctx khttp.Context, admin *service.ScriptPackageAdmin) bool {
	if admin == nil {
		return false
	}
	r := ctx.Request()
	return admin.ValidateAdminKey(r.Header.Get("Authorization"), r.Header.Get("X-API-Key"))
}

// RegisterScriptPackageAdminRoutes 管理端上传/发布/查询（需 script_admin.api_keys）。
func RegisterScriptPackageAdminRoutes(s *khttp.Server, admin *service.ScriptPackageAdmin) {
	if s == nil || admin == nil || !admin.Enabled() {
		return
	}
	r := s.Route("/")
	r.POST("/api/v1/admin/agent-scripts/packages", adminUploadPackage(admin))
	r.POST("/api/v1/admin/agent-scripts/packages/{id}/publish", adminPublishPackage(admin))
	r.GET("/api/v1/admin/agent-scripts/current", adminCurrentPackage(admin))
	r.GET("/api/v1/admin/agent-scripts/packages", adminListPackages(admin))
}

func adminUploadPackage(admin *service.ScriptPackageAdmin) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if !authScriptAdmin(ctx, admin) {
			return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing admin api key")
		}
		id, dp, sha, err := admin.UploadPackage(ctx.Request().Context(), ctx.Request())
		if err != nil {
			if errors.Is(err, data.ErrScriptStoreUnavailable) {
				return jsonErr(ctx, http.StatusServiceUnavailable, "INTERNAL", err.Error())
			}
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		return ctx.Result(200, map[string]any{
			"package_id":    id,
			"download_path": dp,
			"sha256":        sha,
		})
	}
}

func adminPublishPackage(admin *service.ScriptPackageAdmin) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if !authScriptAdmin(ctx, admin) {
			return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing admin api key")
		}
		var in struct {
			// Kratos BindVars 走 form 解码，默认读 json 标签（与 gorilla/mux 的 vars key 一致即可）
			ID string `json:"id"`
		}
		if err := ctx.BindVars(&in); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		pkgID, err := strconv.ParseInt(strings.TrimSpace(in.ID), 10, 64)
		if err != nil || pkgID <= 0 {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", "invalid id")
		}
		if err := admin.PublishPackage(ctx.Request().Context(), pkgID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return jsonErr(ctx, http.StatusNotFound, "NOT_FOUND", err.Error())
			}
			return mapSvcErr(ctx, err)
		}
		return ctx.Result(200, map[string]any{"published": true, "id": pkgID})
	}
}

func adminCurrentPackage(admin *service.ScriptPackageAdmin) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if !authScriptAdmin(ctx, admin) {
			return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing admin api key")
		}
		sid := strings.TrimSpace(ctx.Request().URL.Query().Get("script_id"))
		if sid == "" {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", "script_id required")
		}
		p, err := admin.GetCurrentPublished(ctx.Request().Context(), sid)
		if err != nil {
			if errors.Is(err, data.ErrScriptStoreUnavailable) {
				return jsonErr(ctx, http.StatusServiceUnavailable, "INTERNAL", err.Error())
			}
			return mapSvcErr(ctx, err)
		}
		if p == nil {
			return jsonErr(ctx, http.StatusNotFound, "NOT_FOUND", "no published package")
		}
		base := strings.TrimRight(admin.URLPrefixForPublicPath(), "/")
		pubPath := base + "/" + strings.TrimLeft(strings.TrimSpace(p.StorageRelPath), "/")
		return ctx.Result(200, map[string]any{
			"id":               p.ID,
			"script_id":        p.ScriptID,
			"version":          p.Version,
			"sha256":           p.SHA256,
			"storage_rel_path": p.StorageRelPath,
			"filename":         p.Filename,
			"entry_file":       p.EntryFile,
			"status":           p.Status,
			"public_path":      pubPath,
		})
	}
}

func adminListPackages(admin *service.ScriptPackageAdmin) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if !authScriptAdmin(ctx, admin) {
			return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing admin api key")
		}
		off, _ := strconv.Atoi(ctx.Request().URL.Query().Get("offset"))
		lim, _ := strconv.Atoi(ctx.Request().URL.Query().Get("limit"))
		list, err := admin.ListPackages(ctx.Request().Context(), off, lim)
		if err != nil {
			if errors.Is(err, data.ErrScriptStoreUnavailable) {
				return jsonErr(ctx, http.StatusServiceUnavailable, "INTERNAL", err.Error())
			}
			return mapSvcErr(ctx, err)
		}
		out := make([]map[string]any, 0, len(list))
		for _, p := range list {
			if p == nil {
				continue
			}
			out = append(out, map[string]any{
				"id": p.ID, "script_id": p.ScriptID,
				"version": p.Version, "sha256": p.SHA256, "status": p.Status,
				"storage_rel_path": p.StorageRelPath, "filename": p.Filename,
				"entry_file": p.EntryFile,
			})
		}
		return ctx.Result(200, map[string]any{"packages": out})
	}
}
