# BOM 配单系统 - 前端

基于 React + Vite + Tailwind CSS 的 BOM 货源搜索与询价前端页面。

## 功能

- **BOM 上传**：拖拽或选择 Excel 文件上传，支持解析模式选择（立创/云汉/通用/自定义）
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
│   ├── api.ts          # API 客户端
│   ├── App.tsx         # 主应用
│   ├── main.tsx        # 入口
│   ├── style.css       # 全局样式
│   └── pages/
│       ├── UploadPage.tsx      # BOM 上传页
│       └── MatchResultPage.tsx # 配单结果页
├── index.html
├── vite.config.ts
└── package.json
```
