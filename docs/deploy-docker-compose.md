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
