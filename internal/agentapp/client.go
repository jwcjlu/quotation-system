package agentapp

import (
	"context"
	"net/http"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	v1 "caichip/api/agent/v1"
)

// LeaseConflictError HTTP 409 租约冲突（上报结果被拒）。
type LeaseConflictError struct {
	Detail string
}

func (e *LeaseConflictError) Error() string {
	if e == nil {
		return "lease conflict"
	}
	return "lease conflict: " + e.Detail
}

// Client 基于 Kratos transport/http + protoc 生成的 AgentServiceHTTPClient。
type Client struct {
	cli    v1.AgentServiceHTTPClient
	apiKey string
}

// NewClient 使用 Kratos HTTP Client 与 api/agent/v1 生成代码。
func NewClient(ctx context.Context, baseURL, apiKey string, timeout time.Duration) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	kc, err := khttp.NewClient(ctx,
		khttp.WithEndpoint(base),
		khttp.WithTimeout(timeout),
		khttp.WithUserAgent("caichip-agent/kratos-http"),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		cli:    v1.NewAgentServiceHTTPClient(kc),
		apiKey: apiKey,
	}, nil
}

func (c *Client) authOpts() []khttp.CallOption {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+c.apiKey)
	return []khttp.CallOption{khttp.Header(&h)}
}

// TaskHeartbeat POST /api/v1/agent/task/heartbeat
func (c *Client) TaskHeartbeat(ctx context.Context, req *v1.TaskHeartbeatRequest) (*v1.TaskHeartbeatReply, error) {
	return c.cli.TaskHeartbeat(ctx, req, c.authOpts()...)
}

// ScriptSyncHeartbeat POST /api/v1/agent/script-sync/heartbeat
func (c *Client) ScriptSyncHeartbeat(ctx context.Context, req *v1.ScriptSyncHeartbeatRequest) (*v1.ScriptSyncHeartbeatReply, error) {
	return c.cli.ScriptSyncHeartbeat(ctx, req, c.authOpts()...)
}

// TaskResult POST /api/v1/agent/task/result
func (c *Client) TaskResult(ctx context.Context, req *v1.TaskResultRequest) error {
	_, err := c.cli.TaskResult(ctx, req, c.authOpts()...)
	if err == nil {
		return nil
	}
	if se := kerrors.FromError(err); se != nil && se.Code == 409 {
		msg := se.Message
		if msg == "" {
			msg = err.Error()
		}
		return &LeaseConflictError{Detail: msg}
	}
	return err
}
