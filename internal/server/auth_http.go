package server

import (
	"errors"
	"net/http"

	"caichip/internal/biz"
	"caichip/internal/service"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func RegisterAuthHTTPRoutes(s *khttp.Server, auth *service.AuthService) {
	if s == nil || auth == nil {
		return
	}
	r := s.Route("/")
	r.POST("/api/v1/auth/register", authRegister(auth))
	r.POST("/api/v1/auth/login", authLogin(auth))
	r.GET("/api/v1/auth/me", authMe(auth))
	r.POST("/api/v1/auth/logout", authLogout(auth))
}

func authRegister(auth *service.AuthService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Password    string `json:"password"`
		}
		if err := ctx.Bind(&in); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		user, err := auth.Register(ctx.Request().Context(), service.RegisterUserInput{
			Username: in.Username, DisplayName: in.DisplayName, Password: in.Password,
		})
		if err != nil {
			return mapAuthErr(ctx, err)
		}
		return ctx.Result(http.StatusOK, map[string]any{"user": user})
	}
}

func authLogin(auth *service.AuthService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := ctx.Bind(&in); err != nil {
			return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		}
		out, err := auth.Login(ctx.Request().Context(), in.Username, in.Password)
		if err != nil {
			return mapAuthErr(ctx, err)
		}
		return ctx.Result(http.StatusOK, out)
	}
}

func authMe(auth *service.AuthService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		user, err := auth.Me(ctx.Request().Context(), ctx.Request().Header.Get("Authorization"))
		if err != nil {
			return mapAuthErr(ctx, err)
		}
		return ctx.Result(http.StatusOK, map[string]any{"user": user})
	}
}

func authLogout(auth *service.AuthService) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		if err := auth.Logout(ctx.Request().Context(), ctx.Request().Header.Get("Authorization")); err != nil {
			return mapAuthErr(ctx, err)
		}
		return ctx.Result(http.StatusOK, map[string]any{})
	}
}

func mapAuthErr(ctx khttp.Context, err error) error {
	var br *service.BadRequestError
	switch {
	case errors.As(err, &br):
		return jsonErr(ctx, http.StatusBadRequest, "BAD_REQUEST", br.Message)
	case errors.Is(err, biz.ErrAuthUserExists):
		return jsonErr(ctx, http.StatusConflict, "USER_EXISTS", "username already exists")
	case errors.Is(err, service.ErrAuthInvalidCredentials):
		return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "invalid username or password")
	case errors.Is(err, service.ErrAuthUnauthorized):
		return jsonErr(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
	default:
		return jsonErr(ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}
