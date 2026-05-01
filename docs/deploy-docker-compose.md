# Docker Compose 部署（MySQL + Redis + Server + Web）

## 1. 前置条件

- 安装 Docker Desktop（含 `docker compose`）
- 在仓库根目录执行命令

## 2. 启动

```bash
docker compose up -d --build
```

首次启动会构建：

- 后端：`Dockerfile.server`
- 前端：`web/Dockerfile`

并启动四个服务：

- `mysql`（`3306`）
- `redis`（`6379`）
- `server`（HTTP `18080`，gRPC `19090`）
- `web`（`8080`，Nginx 反向代理 `/api/*` 到后端）

## 3. 访问

- 前端页面：[http://localhost:8080](http://localhost:8080)
- 后端 HTTP API：`http://localhost:18080`

## 4. 常用运维命令

```bash
# 查看服务状态
docker compose ps

# 查看后端日志
docker compose logs -f server

# 重启后端
docker compose restart server

# 停止并删除容器
docker compose down

# 停止并删除容器 + 数据卷（会清空 MySQL/Redis 数据）
docker compose down -v
```

## 5. 配置说明

- 后端容器默认读取：`configs/config.docker.yaml`
- 数据库 DSN 使用容器名：`mysql:3306`
- Redis 地址使用容器名：`redis:6379`

如需修改配置，可直接编辑 `configs/config.docker.yaml` 后重建：

```bash
docker compose up -d --build server
```

## 6. 使用 ACR 镜像（与 CI/CD 对齐）

`docker-compose.yml` 里 `server` / `web` 使用环境变量指定镜像名：

- `SERVER_IMAGE`：后端，默认未设置时为 `caichip-server:latest`（本机构建或本地 tag，**不是** ACR 全路径）。
- `WEB_IMAGE`：前端 Nginx 镜像，默认 `caichip-web:latest`。

CI 推送到阿里云 ACR 时（仓库名以实际为准，示例与流水线一致）：

- 后端：`${ACR_REGISTRY}/${ACR_REPOSITORY}:<git 完整 commit SHA>`，并额外打 `:latest`（与上述 SHA 指向同一 digest 时可二选一拉取）。
- 前端：同一仓库名下使用 **`web-` 前缀**，避免覆盖后端 tag：`:web-<git 完整 commit SHA>` 与 `:web-latest`。

GitHub Actions 的 **CD**（`.github/workflows/cd.yml`）在部署机上已设置：

- `SERVER_IMAGE=${ACR_REGISTRY}/${ACR_REPOSITORY}:${IMAGE_TAG}`（`IMAGE_TAG` 为本次部署的 commit SHA）
- `WEB_IMAGE=${ACR_REGISTRY}/${ACR_REPOSITORY}:web-${IMAGE_TAG}`

因此 **走 CD 的部署与 ACR 控制台里的 tag 是一一对应的**。

### 6.1 本机或 ECS 手动对齐 ACR

1. 登录 ACR（域名、用户名、密码以阿里云控制台为准）：

   ```bash
   echo "<ACR_PASSWORD>" | docker login --username "<ACR_USERNAME>" --password-stdin "<ACR_REGISTRY>"
   ```

2. 在仓库根目录创建或编辑 **`.env`**（与 `docker-compose.yml` 同级，`docker compose` 会自动读取），例如：

   ```bash
   SERVER_IMAGE=crpi-w9bl1lqh2u0svuwc.cn-guangzhou.personal.cr.aliyuncs.com/caichip/quotation-system:7b83859894b72dbb9177a82dca4681ba79d7d994
   WEB_IMAGE=crpi-w9bl1lqh2u0svuwc.cn-guangzhou.personal.cr.aliyuncs.com/caichip/quotation-system:web-7b83859894b72dbb9177a82dca4681ba79d7d994
   ```

   将 commit SHA 换成你要部署的版本（与 ACR 标签页一致；也可用 `:latest` / `:web-latest` 跟踪最新构建，生产环境更建议锁 SHA）。

3. 只拉镜像、不本地构建：

   ```bash
   docker compose pull server web
   docker compose up -d --no-build
   ```

### 6.2 与「第二节」本地构建的区别

- **第二节**：`docker compose up -d --build` 在本地用 `Dockerfile.server` 与 `web/Dockerfile` 构建，不依赖 ACR。
- **本节**：镜像已在 CI 中构建并推送到 ACR，运行环境只负责 `login` + `pull` + `up --no-build`，与线上流水线一致。
