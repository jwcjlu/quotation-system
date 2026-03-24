# caichip Agent

与 [分布式采集Agent需求与架构](../docs/分布式采集Agent需求与架构.md)、[API 协议](../docs/分布式采集Agent-API协议.md) 对齐的 **轻量 Agent 进程**。

本仓库提供 **两种实现，二选一**：

| 实现 | 入口 | 说明 |
|------|------|------|
| **Python** | `python -m agent` | 见下文 |
| **Go** | 在仓库根执行 `go build -o agent ./cmd/agent` 后运行 `./agent`（Windows 为 `agent.exe`） | 协议类型来自 **`api/agent/v1/agent.proto`**（`make api` 生成）；与服务的 HTTP 调用使用 **Kratos** + 生成的 **`AgentServiceHTTPClient`**。任务仍由 **Python** 执行 `main.py`/`run.py`。启动时会检查 Python/pip；默认可自动安装（见下表） |

## 环境要求（必须先有 Python）

- **本 Agent 用 Python 编写**，主机上 **必须已安装 Python**（建议 **3.10+**，与需求文档 §4.1.1 中解释器下限一致），**不会**随 Agent 自动安装系统级 Python。
- 用 **`python -m agent`** 启动时，任务里的 `main.py` / `run.py` 也由 **同一解释器**（`sys.executable`）执行；若业务脚本需要独立 venv，需在部署时自行创建并把解释器路径配进环境（当前实现未做 per-script venv）。
- **依赖**：仅需 `requests`（见 `agent/requirements.txt`）。

```bash
pip install -r agent/requirements.txt
```

说明：需求文档里「Agent 自举安装 Python」针对的是 **未来/完整版 Agent 安装包** 的自检能力；**本仓库当前 Python Agent** 假定运维/镜像已预装 Python。

## 能力（当前版本）

- 任务心跳：`POST /api/v1/agent/task/heartbeat`（长轮询）
- 脚本安装心跳：`POST /api/v1/agent/script-sync/heartbeat`（脚本包分发以 **`script_id`** 为维，无 `platform_id`；见协议 §3）
- 任务结果：`POST /api/v1/agent/task/result`
- 扫描本地 `AGENT_DATA_DIR/<script_id>/<version>/version.txt` 上报 `installed_scripts`
- 执行：本机 `script_id/version/` 下 `main.py` 或 `run.py`（或 `entry_file`），注入 `CAICHIP_TASK_PARAMS`
- 超时：默认 300s，终止进程树（Windows `taskkill /T`，POSIX 进程组）
- **同一 `script_id` 串行**（不同 `script_id` 可并行，受 `AGENT_MAX_PARALLEL` 限制）

未实现：ZIP 下载、venv/pip、`attempt` 递增等（见服务端实现说明）。

## 运行

在仓库根目录（使 `agent` 包可导入）：

```bash
pip install -r agent/requirements.txt

set CAICHIP_API_KEY=你的密钥
set CAICHIP_BASE_URL=http://127.0.0.1:18080
set AGENT_DATA_DIR=D:\workspace\caichip\agent_data
python -m agent
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `CAICHIP_API_KEY` | **必填** |
| `CAICHIP_BASE_URL` | 默认 `http://127.0.0.1:18080` |
| `AGENT_ID` | 可选，默认本机 UUID 派生 |
| `AGENT_DATA_DIR` | 脚本包根目录，默认 `./agent_data` |
| `AGENT_QUEUE` | 默认 `default` |
| `AGENT_TAGS` | 逗号分隔 |
| `AGENT_LONG_POLL_SEC` | 任务心跳长轮询秒数，默认 `10` |
| `AGENT_HTTP_TIMEOUT_SEC` | HTTP 客户端超时，须 **大于** 长轮询，默认 `120` |
| `AGENT_SCRIPT_SYNC_SEC` | 脚本安装心跳周期，默认 `600` |
| `AGENT_MAX_PARALLEL` | 不同脚本并行数，默认 `4` |
| `CAICHIP_PYTHON` | 指定解释器路径（Go/Python Agent 均可用） |
| `CAICHIP_SKIP_PYTHON_CHECK` | **仅 Go Agent**：设为 `1` 或 `true` 时跳过启动前 Python/pip 检查（仅当你确定环境已就绪） |
| `CAICHIP_AUTO_INSTALL_PYTHON` | **仅 Go Agent**：**默认开启**。缺少 Python/pip 时尝试自动安装（Windows：`winget`，失败则下载 python.org 官方安装包静默安装；Linux：需 **root** + `apt`；macOS：`brew`）。设为 `0` / `false` / `off` 可关闭 |

## 本地测试包布局

```text
agent_data/
  demo/
    1.0.0/
      version.txt   # 单行 1.0.0 或 v1.0.0
      main.py
```

配合服务端 `dev/enqueue` 入队 `script_id=demo, version=1.0.0` 即可拉取执行。
