# BOM 配单系统

基于 Go + Kratos 框架的 BOM 单货源搜索与询价系统。

## 项目结构

```
caichip/
├── api/bom/v1/           # Proto 定义与生成代码
├── cmd/server/           # 服务入口 (main, wire)
├── configs/              # 配置文件
├── internal/
│   ├── biz/              # 业务逻辑 (BOM, Search, Match)
│   ├── conf/             # 配置结构
│   ├── data/             # 数据层 (BOM Repo, Search Repo)
│   ├── server/           # HTTP/gRPC 服务
│   └── service/          # BOM 服务实现
├── pkg/
│   ├── parser/           # BOM Excel 解析器 (szlcsc, ickey, auto, custom)
│   └── platform/         # 平台抽象 (Ickey, SZLCSC 客户端)
└── third_party/          # Proto 依赖 (google/api)
```

## 解析模式

| 模式 | 说明 |
|------|------|
| szlcsc | 立创商城 BOM 模板 |
| ickey | 云汉芯城 BOM 模板 |
| auto | 自动识别表头 |
| custom | 自定义列映射 (column_mapping) |

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/bom/upload | 上传 BOM 文件 |
| POST | /api/v1/bom/search | 多平台搜索报价 |
| POST | /api/v1/bom/match | 自动配单 |
| GET | /api/v1/bom/template | 下载 BOM 模板 |
| GET | /api/v1/bom/{bom_id} | 获取 BOM 详情 |
| GET | /api/v1/bom/{bom_id}/match | 获取配单结果 |

## 运行

```bash
# 安装依赖
go mod download

# 生成 proto 代码 (需安装 protoc, protoc-gen-go, protoc-gen-go-grpc, protoc-gen-go-http)
protoc --proto_path=./api --proto_path=./third_party \
  --go_out=paths=source_relative:./api \
  --go-http_out=paths=source_relative:./api \
  --go-grpc_out=paths=source_relative:./api \
  api/bom/v1/bom.proto

# 运行服务
go run ./cmd/server/...
```

服务默认监听：
- HTTP: 0.0.0.0:8000
- gRPC: 0.0.0.0:9000

## 平台客户端

- **Ickey**: 云汉芯城 (当前为 stub，可后续接入 ickey_crawler.py)
- **SZLCSC**: 立创商城 (stub)

## 前端 (web/)

```bash
cd web && npm install && npm run dev
```

前端开发服务器 `http://localhost:5173`，API 通过 Vite 代理转发到后端。详见 [web/README.md](web/README.md)。
