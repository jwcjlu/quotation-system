# BOM 单货源搜索与询价系统 - 架构设计文档

基于 Go + Kratos 框架的技术架构、详细流程与接口协议定义。

---

## 1. 技术栈

| 组件 | 技术选型 |
|------|----------|
| 语言 | Go 1.21+ |
| 框架 | Kratos v2 |
| API 定义 | Protobuf + gRPC-Gateway (REST) |
| 配置 | YAML + Kratos config |
| 依赖注入 | Wire |
| 数据存储 | 待定（MySQL/PostgreSQL + Redis） |

---

## 2. 系统架构

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                               Client (Web / App)                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          API Gateway (HTTP/gRPC)                             │
│                    Kratos Server (internal/server)                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                    ┌───────────────────┼───────────────────┐
                    ▼                   ▼                   ▼
┌───────────────────────┐  ┌───────────────────────┐  ┌───────────────────────┐
│   BOM Service        │  │   Search Service      │  │   Match Service       │
│   (internal/service)  │  │   (internal/service)  │  │   (internal/service)  │
└───────────────────────┘  └───────────────────────┘  └───────────────────────┘
            │                           │                           │
            ▼                           ▼                           ▼
┌───────────────────────┐  ┌───────────────────────┐  ┌───────────────────────┐
│   BOM UseCase (biz)   │  │   Search UseCase (biz) │  │   Match UseCase (biz)  │
│   - ParseBOM         │  │   - MultiPlatformSearch│  │   - AutoMatch          │
│   - SaveBOM          │  │   - AggregateQuotes    │  │   - ApplyStrategy      │
└───────────────────────┘  └───────────────────────┘  └───────────────────────┘
            │                           │                           │
            └───────────────────────────┼───────────────────────────┘
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Data Layer (internal/data)                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ BOM Repo    │  │ Search Repo │  │ IckeyClient │  │ SZLCSCClient        │ │
│  │ (持久化)     │  │ (缓存)       │  │ ICGOOClient │  │ (平台爬虫/API)       │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Kratos 项目结构

```
bom-match/
├── api/                          # API 定义
│   └── bom/v1/
│       ├── bom.proto             # BOM 相关接口
│       ├── bom.pb.go
│       ├── bom_http.pb.go
│       └── bom_errors.pb.go
├── cmd/
│   └── server/
│       ├── main.go
│       └── wire.go
├── configs/
│   └── config.yaml
├── internal/
│   ├── biz/                      # 业务逻辑
│   │   ├── bom.go                # BOM 解析、存储
│   │   ├── search.go             # 多平台搜索编排
│   │   ├── match.go               # 自动配单
│   │   └── model.go               # 领域模型
│   ├── data/                     # 数据访问
│   │   ├── bom_repo.go
│   │   ├── search_repo.go
│   │   ├── ickey_client.go       # 云汉芯城
│   │   ├── szlcsc_client.go      # 立创商城
│   │   └── icgoo_client.go      # ICGOO
│   ├── server/
│   │   ├── grpc.go
│   │   └── http.go
│   ├── service/                  # 服务实现
│   │   └── bom_service.go
│   └── conf/
│       └── conf.go
├── pkg/
│   ├── parser/                   # BOM 解析器
│   │   └── excel_parser.go
│   └── platform/                 # 平台抽象
│       ├── interface.go          # 统一搜索接口
│       └── ...
└── go.mod
```

### 2.3 分层职责

| 层级 | 职责 |
|------|------|
| **Service** | 接收请求、参数校验、调用 Biz、返回响应 |
| **Biz** | 业务编排、领域逻辑、事务边界 |
| **Data** | 数据持久化、外部平台调用、缓存 |
| **pkg/parser** | BOM Excel 解析，无业务依赖 |
| **pkg/platform** | 各平台爬虫/API 客户端，实现统一接口 |

---

## 3. 详细流程设计

### 3.1 BOM 导入与解析流程

```
┌─────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Client  │     │ BOM Service │     │ BOM UseCase │     │ BOM Repo    │
└────┬────┘     └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
     │                 │                    │                    │
     │ UploadBOM       │                    │                    │
     │ (multipart)     │                    │                    │
     │────────────────>│                    │                    │
     │                 │ ParseAndSave       │                    │
     │                 │───────────────────>│                    │
     │                 │                    │ ParseExcel         │
     │                 │                    │ (pkg/parser)       │
     │                 │                    │──┐                 │
     │                 │                    │  │ 解析型号/厂牌/   │
     │                 │                    │<─┘ 封装/数量       │
     │                 │                    │                    │
     │                 │                    │ SaveBOM            │
     │                 │                    │──────────────────>│
     │                 │                    │                    │ 持久化
     │                 │                    │<──────────────────│
     │                 │<───────────────────│                    │
     │                 │                    │                    │
     │ UploadBOMReply  │                    │                    │
     │<────────────────│                    │                    │
     │ (bom_id, items) │                    │                    │
```

**流程说明：**
1. 客户端上传 Excel 文件（multipart/form-data），可携带 `parse_mode`（szlcsc | ickey | auto | custom）及自定义模式的 `column_mapping`
2. Service 接收文件，调用 Biz.ParseAndSave
3. Biz 根据 parse_mode 调用 pkg/parser 解析 Excel，提取型号、厂牌、封装、数量
4. Biz 调用 BOM Repo 持久化解析结果，生成 bom_id
5. 返回 bom_id 和解析后的物料列表

### 3.2 多平台搜索流程

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────────────────────────┐
│ Search      │     │ Search      │     │ Platform Clients (并行)               │
│ Service     │     │ UseCase     │     │ Ickey | SZLCSC | ICGOO               │
└──────┬──────┘     └──────┬──────┘     └──────────────────┬──────────────────┘
       │                   │                                │
       │ SearchQuotes      │                                │
       │ (bom_id)          │                                │
       │──────────────────>│                                │
       │                   │ GetBOMItems(bom_id)            │
       │                   │──┐                             │
       │                   │  │ 获取物料列表                 │
       │                   │<─┘                             │
       │                   │                                │
       │                   │ for each item:                 │
       │                   │   SearchIckey(model)  ─────────>│ Ickey 搜索
       │                   │   SearchSZLCSC(model)─────────>│ 立创搜索
       │                   │   SearchICGOO(model)─────────>│ ICGOO 搜索
       │                   │   Aggregate(quotes)            │
       │                   │<───────────────────────────────│
       │                   │                                │
       │                   │ SaveSearchResult               │
       │                   │ (缓存/持久化)                   │
       │                   │                                │
       │<──────────────────│                                │
       │ SearchQuotesReply  │                                │
       │ (items + quotes)   │                                │
```

**流程说明：**
1. 客户端传入 bom_id，触发多平台搜索
2. Biz 根据 bom_id 获取物料列表
3. 对每个物料型号，并行调用各平台客户端搜索
4. 汇总各平台报价，按物料维度聚合
5. 可选：缓存搜索结果，返回给客户端

### 3.2.1 匹配与选型规则（需求 2.3 / 2.4）

各平台搜索后返回多条记录，系统需按以下规则筛选并选出推荐记录：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 输入：BOM 物料 item (model, manufacturer, package, quantity)                │
│       各平台报价列表 quotes[] (每平台多条)                                     │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Step 1: 完全匹配筛选                                                         │
│   对每条平台报价 q，满足以下三项则保留：                                       │
│   - q.MatchedModel == item.Model     （型号一致）                             │
│   - q.Manufacturer == item.Manufacturer （厂牌一致）                          │
│   - q.Package == item.Package        （封装一致，需平台返回 Package 字段）     │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Step 2: 价格最低选型                                                         │
│   在 Step 1 筛选后的记录中，按 UnitPrice 升序，取第一条                        │
│   若无完全匹配记录，则标记为「待确认」或「无法匹配」                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 输出：推荐报价 best（或 nil），all_quotes（各平台全部报价供详情展示）          │
└─────────────────────────────────────────────────────────────────────────────┘
```

**实现位置：** `internal/biz/match.go` 的 `selectBest` 前增加 `filterFullyMatched(quotes, item)`，仅对完全匹配记录执行选型。

**数据要求：** 平台报价结构 `Quote` 需包含 `Package` 字段，以便封装匹配。`pkg/platform` 与 `internal/biz` 的 Quote 需同步增加。

### 3.3 配单结果展示（与需求 2.5 对应）

- **默认展示**：每行显示系统推荐的最优匹配（推荐型号、来源平台、库存、货期、单价、小计）
- **显示更多**：每行「推荐最优型号」支持展开，展示该物料在各平台搜索到的全部报价
- **查看详情**：每行操作列增加「查看详情」按钮，点击后弹出/展开详情面板，展示该物料在**各平台搜索到的全部报价记录**（含未选中的）

**查看详情数据流：**

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ 前端表格行   │     │ GetMatchResult  │     │ MatchItem       │
│ 点击「查看详情」│     │ API            │     │ .all_quotes     │
└──────┬──────┘     └────────┬────────┘     └────────┬────────┘
       │                     │                       │
       │ 已有 bom_id         │                       │
       │ 与 match 结果       │                       │
       │────────────────────>│                       │
       │                     │ 返回 MatchItem[]      │
       │                     │ 每项含 all_quotes     │
       │<────────────────────│<──────────────────────│
       │                     │                       │
       │ 弹窗/抽屉展示        │                       │
       │ all_quotes 按平台分组│                       │
       │ 展示：平台、型号、   │                       │
       │ 厂牌、封装、库存、   │                       │
       │ 货期、价格梯度、单价 │                       │
       │ 高亮当前选中记录     │                       │
       │                     │                       │
```

**数据来源**：`GetMatchResult` 返回的 `MatchItem` 已包含 `all_quotes`（各平台全部报价），前端无需额外请求，直接按平台分组展示。

### 3.4 自动配单流程

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Match       │     │ Match       │     │ Search Repo │
│ Service     │     │ UseCase     │     │ (报价数据)   │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       │ AutoMatch         │                   │
       │ (bom_id, strategy)│                   │
       │──────────────────>│                   │
       │                   │ GetQuotesByBOM    │
       │                   │──────────────────>│
       │                   │<──────────────────│
       │                   │                   │
       │                   │ for each item:    │
       │                   │   ApplyStrategy   │
       │                   │   (price/stock/   │
       │                   │    leadtime)      │
       │                   │   SelectBest      │
       │                   │   -> 推荐型号     │
       │                   │   -> 来源平台     │
       │                   │   -> 单价/小计    │
       │                   │                   │
       │                   │ SaveMatchResult   │
       │                   │                   │
       │<──────────────────│                   │
       │ MatchResult       │                   │
```

**流程说明：**
1. 客户端传入 bom_id 和配单策略（价格优先/库存优先/货期优先）
2. Biz 获取该 BOM 的多平台报价数据
3. 对每个物料，先按**匹配与选型规则**（3.2.1）筛选：仅保留型号、封装、厂牌均匹配的记录
4. 在完全匹配记录中，按策略（价格/库存/货期）选出最优
5. 生成配单结果（推荐型号、来源平台、单价、小计），并将该物料的**全部平台报价**（含未选中的）填入 `all_quotes`，供前端「显示更多」与「查看详情」展示
6. 持久化配单结果，返回给客户端

---

## 4. 接口协议定义

### 4.1 Proto 文件结构

```
api/bom/v1/
├── bom.proto          # 主服务定义
├── message.proto      # 通用消息（可选拆分）
└── error_reason.proto # 错误码
```

### 4.2 BOM 服务 Proto 定义

```protobuf
syntax = "proto3";

package api.bom.v1;

import "google/api/annotations.proto";

option go_package = "bom-match/api/bom/v1;v1";

// BOM 配单服务
service BomService {
  // 上传并解析 BOM
  rpc UploadBOM(UploadBOMRequest) returns (UploadBOMReply) {
    option (google.api.http) = {
      post: "/api/v1/bom/upload"
      body: "*"
    };
  }

  // 多平台搜索报价
  rpc SearchQuotes(SearchQuotesRequest) returns (SearchQuotesReply) {
    option (google.api.http) = {
      post: "/api/v1/bom/search"
      body: "*"
    };
  }

  // 自动配单
  rpc AutoMatch(AutoMatchRequest) returns (AutoMatchReply) {
    option (google.api.http) = {
      post: "/api/v1/bom/match"
      body: "*"
    };
  }

  // 获取 BOM 详情（含解析结果）
  rpc GetBOM(GetBOMRequest) returns (GetBOMReply) {
    option (google.api.http) = {
      get: "/api/v1/bom/{bom_id}"
    };
  }

  // 获取配单结果
  rpc GetMatchResult(GetMatchResultRequest) returns (GetMatchResultReply) {
    option (google.api.http) = {
      get: "/api/v1/bom/{bom_id}/match"
    };
  }

  // 下载 BOM 模板
  rpc DownloadTemplate(DownloadTemplateRequest) returns (DownloadTemplateReply) {
    option (google.api.http) = {
      get: "/api/v1/bom/template"
    };
  }
}

// ========== UploadBOM ==========
message UploadBOMRequest {
  bytes file = 1;           // Excel 文件内容
  string filename = 2;     // 文件名
  string parse_mode = 3;   // 解析模式：szlcsc | ickey | auto | custom
  map<string, string> column_mapping = 4;  // 自定义模式时：{"model":"A","manufacturer":"B",...}
}

message UploadBOMReply {
  string bom_id = 1;        // BOM 唯一标识
  repeated ParsedItem items = 2;  // 解析后的物料列表
  int32 total = 3;          // 物料数量
}

message ParsedItem {
  int32 index = 1;          // 序号
  string raw = 2;           // 原始需求文本
  string model = 3;         // 型号
  string manufacturer = 4;  // 厂牌
  string package = 5;       // 封装
  int32 quantity = 6;       // 需求量
  string params = 7;        // 其他参数
}

// ========== SearchQuotes ==========
message SearchQuotesRequest {
  string bom_id = 1;
  repeated string platforms = 2;  // 可选：指定平台，空则全平台
}

message SearchQuotesReply {
  repeated ItemQuotes item_quotes = 1;
}

message ItemQuotes {
  string model = 1;         // 物料型号
  int32 quantity = 2;       // 需求量
  repeated PlatformQuote quotes = 3;  // 各平台报价
}

message PlatformQuote {
  string platform = 1;      // 立创/ICGOO/云汉
  string matched_model = 2; // 匹配到的型号
  string manufacturer = 3;  // 厂牌
  string package = 4;       // 封装（用于型号/封装/厂牌匹配筛选）
  string description = 5;   // 描述
  int64 stock = 6;          // 库存
  string lead_time = 7;     // 货期
  int32 moq = 8;           // 起订量
  int32 increment = 9;      // 增量
  string price_tiers = 10;  // 价格梯度
  string hk_price = 11;     // 中国香港交货
  string mainland_price = 12; // 内地交货(含增值税)
  double unit_price = 13;   // 单价(含税)
  double subtotal = 14;     // 小计
}

// ========== AutoMatch ==========
message AutoMatchRequest {
  string bom_id = 1;
  string strategy = 2;      // price_first | stock_first | leadtime_first | comprehensive
}

message AutoMatchReply {
  repeated MatchItem items = 1;
  double total_amount = 2;  // 合计金额
}

message MatchItem {
  int32 index = 1;
  string model = 2;         // 需求型号
  int32 quantity = 3;      // 需求量
  string matched_model = 4; // 推荐型号
  string manufacturer = 5;
  string platform = 6;     // 来源平台
  string lead_time = 7;
  int64 stock = 8;
  double unit_price = 9;
  double subtotal = 10;
  string match_status = 11; // exact | pending | no_match
  repeated PlatformQuote all_quotes = 12;  // 该物料在各平台的全部报价，供「显示更多」「查看详情」展示
}

// ========== GetBOM ==========
message GetBOMRequest {
  string bom_id = 1;
}

message GetBOMReply {
  string bom_id = 1;
  string created_at = 2;
  repeated ParsedItem items = 3;
}

// ========== GetMatchResult ==========
message GetMatchResultRequest {
  string bom_id = 1;
}

message GetMatchResultReply {
  repeated MatchItem items = 1;
  double total_amount = 2;
}

// ========== DownloadTemplate ==========
message DownloadTemplateRequest {}

message DownloadTemplateReply {
  bytes file = 1;
  string filename = 2;
}
```

### 4.3 平台搜索接口（内部抽象）

```go
// pkg/platform/interface.go

package platform

// Quote 平台报价统一结构
type Quote struct {
    Platform       string  // ickey | szlcsc | icgoo
    MatchedModel   string
    Manufacturer   string
    Package        string  // 封装，用于型号/封装/厂牌匹配筛选
    Description    string
    Stock          int64
    LeadTime       string
    MOQ            int32
    Increment      int32
    PriceTiers     string
    HKPrice        string
    MainlandPrice  string
    UnitPrice      float64
}

// Searcher 平台搜索接口
type Searcher interface {
    Name() string
    Search(model string, quantity int) ([]*Quote, error)
}
```

### 4.4 HTTP 接口汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/bom/upload | 上传 BOM 文件，解析并保存 |
| POST | /api/v1/bom/search | 多平台搜索报价 |
| POST | /api/v1/bom/match | 自动配单 |
| GET | /api/v1/bom/{bom_id} | 获取 BOM 详情 |
| GET | /api/v1/bom/{bom_id}/match | 获取配单结果 |
| GET | /api/v1/bom/template | 下载 BOM 模板 |

### 4.5 错误码定义

```protobuf
// api/bom/v1/error_reason.proto

enum ErrorReason {
  BOM_UNSPECIFIED = 0;
  BOM_PARSE_FAILED = 1;      // 解析失败
  BOM_NOT_FOUND = 2;         // BOM 不存在
  BOM_SEARCH_FAILED = 3;      // 搜索失败
  BOM_PLATFORM_UNAVAILABLE = 4; // 平台不可用
}
```

---

## 5. 配置定义

```yaml
# configs/config.yaml

server:
  http:
    addr: 0.0.0.0:8000
    timeout: 30s
  grpc:
    addr: 0.0.0.0:9000
    timeout: 10s

data:
  database:
    driver: mysql
    dsn: user:pass@tcp(127.0.0.1:3306)/bom_match?charset=utf8mb4
  redis:
    addr: 127.0.0.1:6379
    read_timeout: 0.2s
    write_timeout: 0.2s

platform:
  ickey:
    search_url: "https://search.ickey.cn/"
    timeout: 15
  szlcsc:
    search_url: "https://www.szlcsc.com/"
    timeout: 15
  icgoo:
    search_url: "https://www.icgoo.net/"
    timeout: 15
```

---

## 6. 扩展说明

1. **异步任务**：BOM 搜索与配单可改为异步，通过 Job 队列 + 轮询/WebSocket 返回结果
2. **平台扩展**：新增平台时实现 `Searcher` 接口并注册到 Search UseCase
3. **爬虫迁移**：现有 Python ickey_crawler 可通过子进程调用或重写为 Go 实现

---

## 7. 开发任务清单（基于需求 v1.2）

| 序号 | 任务 | 模块 | 说明 |
|------|------|------|------|
| 1 | 匹配与选型规则 | biz/match.go | 增加 `filterFullyMatched`，选型前按型号/封装/厂牌筛选 |
| 2 | Quote 增加 Package | pkg/platform, biz/model, proto | 平台报价与领域模型增加 Package 字段 |
| 3 | 查看详情前端 | web/ | 操作列增加「查看详情」按钮，弹窗展示 all_quotes 按平台分组 |
| 4 | 平台数据映射 | pkg/platform/* | 各平台爬虫/API 返回数据映射 Package 到 Quote |

---

*文档版本：v1.2*  
*同步需求文档 v1.2：型号/封装/厂牌匹配与价格最低选型规则；查看详情按钮及多平台报价展示；匹配算法详细流程；开发任务清单*
