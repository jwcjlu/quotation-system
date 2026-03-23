# api/agent/v1

- **agent.proto**：分布式采集 Agent 与 caichip 服务端之间的 HTTP/JSON 契约（与 `docs/分布式采集Agent-API协议.md` 对齐）。
- **生成代码**：在仓库根执行 `make api`（或 `Makefile` 中 `api` 目标），依赖 `protoc`、`protoc-gen-go`、`protoc-gen-go-http`、`protoc-gen-go-grpc`。
- **产物**：`agent.pb.go`、 `agent_http.pb.go`（Kratos HTTP 路由与 `AgentServiceHTTPClient`）、`agent_grpc.pb.go`（若后续走 gRPC 可复用）。
