# BOM 配单系统 - 前端

基于 React + Vite + Tailwind CSS 的 BOM 货源搜索与询价前端页面。

与当前后端 `go run ./cmd/server/...` 联调：默认通过 Vite 代理访问 `http://127.0.0.1:18080` 的 BOM 会话 API。

## 功能

- **BOM 会话**：`POST /api/v1/bom-sessions`（可选 `readiness_mode`: `lenient` | `strict`）→ `POST /api/v1/bom/upload`（body 带 `session_id`）→ **会话看板**维护行、勾选平台 `PUT .../platforms`、PATCH 单据信息（含就绪策略与联系方式）
- **导出**：`GET /api/v1/bom-sessions/{session_id}/export?format=xlsx|csv`（会话看板内「导出 Excel / CSV」）
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
│   │   └── agentScripts.ts
│   ├── App.tsx
│   ├── main.tsx
│   ├── style.css
│   └── pages/
│       ├── UploadPage.tsx           # 新建会话并上传 BOM
│       ├── BomSessionListPage.tsx   # 会话列表 + 弹框上传 / 看板
│       ├── SourcingSessionPage.tsx  # 会话看板（行表 + 平台 + 导出）
│       └── MatchResultPage.tsx
├── index.html
├── vite.config.ts
└── package.json
```
