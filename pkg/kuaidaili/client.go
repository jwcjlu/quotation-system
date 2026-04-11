// Package kuaidaili 封装快代理私密代理 getdps（https://www.kuaidaili.com/doc/product/api/getdps/）。
// 主路径使用官方 SDK：github.com/kuaidaili/golang-sdk（见 https://github.com/kuaidaili/golang-sdk ）。
package kuaidaili

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	kdlauth "github.com/kuaidaili/golang-sdk/api-sdk/kdl/auth"
	kdlclient "github.com/kuaidaili/golang-sdk/api-sdk/kdl/client"
	kdlsign "github.com/kuaidaili/golang-sdk/api-sdk/kdl/signtype"
	"golang.org/x/time/rate"
)

// Proxy 单条代理（含可选账号密码，取决于 f_auth）。
type Proxy struct {
	Host     string
	Port     int
	User     string
	Password string
}

// Config 客户端配置。
type Config struct {
	// BaseURL 仅用于「仅 secret_token、无 secret_key」兼容路径；SDK 固定请求 dps.kdlapi.com。
	BaseURL  string
	SecretID string
	// SignType：hmacsha1（推荐）、token（与官方 SDK 一致，需 secret_key 以刷新令牌）、或 token+仅 secret_token 走兼容 HTTP。
	SignType    string
	SecretToken string
	SecretKey   string
	Num         int
	Area        string
	FAuth       int
	MaxQPS      float64
}

// Client 封装官方 kdl Client + 可选兼容路径 + 限流。
type Client struct {
	sdk    kdlclient.Client
	useSDK bool
	// 无 secret_key 时仅用静态 secret_token 调 getdps（非 SDK）。
	static  Config
	cfg     Config
	num     int
	sign    kdlsign.SignType
	kwargs  map[string]interface{}
	limiter *rate.Limiter
}

// NewClient 创建客户端。推荐 sign_type=hmacsha1 + secret_key；token 模式若走 SDK 需同时配置 secret_key。
func NewClient(cfg Config) (*Client, error) {
	sid := strings.TrimSpace(cfg.SecretID)
	if sid == "" {
		return nil, errors.New("kuaidaili: secret_id required")
	}
	st := strings.ToLower(strings.TrimSpace(cfg.SignType))
	if st == "" {
		// 与历史配置兼容：未写 sign_type 时默认 token（可仅配 secret_token 走静态兼容路径）。
		st = "token"
	}
	cfg.SignType = st
	if cfg.Num <= 0 {
		cfg.Num = 1
	}
	qps := cfg.MaxQPS
	if qps <= 0 {
		qps = 8
	}

	kwargs := map[string]interface{}{
		"format": "json",
	}
	if a := strings.TrimSpace(cfg.Area); a != "" {
		kwargs["area"] = a
	}
	if cfg.FAuth >= 1 {
		kwargs["f_auth"] = "1"
	}

	key := strings.TrimSpace(cfg.SecretKey)
	tok := strings.TrimSpace(cfg.SecretToken)

	switch st {
	case "hmacsha1":
		if key == "" {
			return nil, errors.New("kuaidaili: secret_key required for sign_type=hmacsha1")
		}
		return &Client{
			sdk: kdlclient.Client{
				Auth: kdlauth.Auth{SecretID: sid, SecretKey: key},
			},
			useSDK:  true,
			cfg:     cfg,
			num:     cfg.Num,
			sign:    kdlsign.HmacSha1,
			kwargs:  kwargs,
			limiter: rate.NewLimiter(rate.Limit(qps), int(qps)),
		}, nil
	case "token":
		if key != "" {
			return &Client{
				sdk: kdlclient.Client{
					Auth: kdlauth.Auth{SecretID: sid, SecretKey: key},
				},
				useSDK:  true,
				cfg:     cfg,
				num:     cfg.Num,
				sign:    kdlsign.Token,
				kwargs:  kwargs,
				limiter: rate.NewLimiter(rate.Limit(qps), int(qps)),
			}, nil
		}
		if tok == "" {
			return nil, errors.New("kuaidaili: sign_type=token requires secret_key (official SDK) or secret_token (static compat)")
		}
		// 仅静态令牌：官方 SDK 的 token 模式依赖 get_secret_token，此处走兼容 HTTP。
		sc := cfg
		if strings.TrimSpace(sc.BaseURL) == "" {
			sc.BaseURL = "https://dps.kdlapi.com/api/getdps"
		}
		return &Client{
			useSDK:  false,
			static:  sc,
			cfg:     cfg,
			num:     cfg.Num,
			kwargs:  kwargs,
			limiter: rate.NewLimiter(rate.Limit(qps), int(qps)),
		}, nil
	default:
		return nil, fmt.Errorf("kuaidaili: unsupported sign_type %q (use hmacsha1 or token)", st)
	}
}

// GetFirstProxy 拉取一条代理（num 来自配置）。
func (c *Client) GetFirstProxy(ctx context.Context) (*Proxy, error) {
	if c == nil || c.limiter == nil {
		return nil, errors.New("kuaidaili: nil client")
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	var list []string
	var err error
	if c.useSDK {
		list, err = c.sdk.GetDps(c.num, c.sign, c.kwargs)
	} else {
		list, err = getDpsStaticToken(ctx, c.static, c.num, c.kwargs)
	}
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errors.New("kuaidaili: empty proxy_list")
	}
	return parseProxyEntry(list[0], c.cfg.FAuth >= 1)
}

func parseProxyEntry(s string, withAuth bool) (*Proxy, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("kuaidaili: empty proxy entry")
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("kuaidaili: bad proxy format %q", s)
	}
	port, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("kuaidaili: bad port in %q", s)
	}
	p := &Proxy{Host: strings.TrimSpace(parts[0]), Port: port}
	if withAuth && len(parts) >= 4 {
		p.User = strings.TrimSpace(parts[2])
		p.Password = strings.TrimSpace(parts[3])
	}
	return p, nil
}
