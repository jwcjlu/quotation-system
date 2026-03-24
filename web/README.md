# BOM 配单系统 - 前端

基于 React + Vite + Tailwind CSS 的 BOM 货源搜索与询价前端页面。

## 功能

- **经典上传**：仅调用 `/api/v1/bom/upload`，内存 BOM + 搜索配单（与旧版一致）
- **货源会话**：`POST /api/v1/bom-sessions` → `POST /api/v1/bom/upload`（body 带 `session_id`）→ **会话看板**轮询 `readiness` / `lines`，勾选平台 `PUT .../platforms`（与 [接口清单](../docs/BOM货源搜索-接口清单.md) 流程对齐；清单中 multipart 专用上传路径以后端实现为准）
- **导出**：`GET /api/v1/bom-sessions/{session_id}/export?format=xlsx|csv`（会话看板内「导出 Excel / CSV」）
- **配单历史**：`GET /api/v1/bom-match-history` 分页列表、`GET /api/v1/bom-match-history/{id}` 快照详情（导航「配单历史」）；会话 UUID 在 **匹配单** 配单成功后会写入数据库快照
- **模板下载**：下载标准 BOM 模板
- **配单结果**：展示匹配结果，支持按状态筛选、显示更多（展开全部平台报价）

## 开发

```bash
# 安装依赖
npm install

# 启动开发服务器（需先启动后端 go run ./cmd/server/...）
npm run dev
```

开发服务器默认 `http://localhost:5173`，API 请求通过 Vite 代理转发到 `http://127.0.0.1:18080`。

## 构建

```bash
npm run build
```

产物输出到 `dist/`，可部署到任意静态服务器。

## 目录结构

```
web/
├── src/
│   ├── api/
│   │   ├── index.ts       # 统一导出
│   │   ├── http.ts        # fetchJson、错误解析
│   │   ├── types.ts       # 类型与平台枚举 PLATFORM_IDS
│   │   ├── bomLegacy.ts   # /api/v1/bom/*
│   │   ├── bomSession.ts  # /api/v1/bom-sessions/*（含 export）
│   │   └── bomHistory.ts  # /api/v1/bom-match-history
│   ├── App.tsx
│   ├── main.tsx
│   ├── style.css
│   └── pages/
│       ├── UploadPage.tsx           # 经典 / 货源会话 上传（flow 区分）
│       ├── SourcingSessionPage.tsx  # 会话看板（轮询 + 行表 + 平台 + 导出）
│       ├── MatchHistoryPage.tsx     # 配单历史列表与快照
│       └── MatchResultPage.tsx
├── index.html
├── vite.config.ts
└── package.json
```
