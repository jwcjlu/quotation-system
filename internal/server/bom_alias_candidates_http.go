package server

import (
	"net/http"

	"caichip/internal/service"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func RegisterBomAliasCandidateRoutes(s *khttp.Server, bom *service.BomService) {
	if s == nil || bom == nil {
		return
	}
	r := s.Route("/")
	r.GET("/api/v1/bom-sessions/{session_id}/manufacturer-alias-candidates", bomManufacturerAliasCandidates(bom))
}

func bomManufacturerAliasCandidates(bom *service.BomService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in struct {
			SessionID string `json:"session_id"`
		}
		if err := ctx.BindVars(&in); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		out, err := bom.ListManufacturerAliasCandidates(ctx.Request().Context(), in.SessionID)
		if err != nil {
			return err
		}
		return ctx.Result(http.StatusOK, out)
	}
}
