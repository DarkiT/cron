# Cron Dashboard - Web 管理界面

Cron Dashboard 是一个独立的 Web 管理界面子包，需要单独引入，不影响主库依赖，为 [darkit/cron](https://github.com/darkit/cron) 提供可视化的任务管理和监控功能。

##  功能特性

### 核心功能

- **任务列表** - 实时查看所有任务的状态和统计信息
- **执行历史** - 查询和分析任务执行历史记录
- **统计分析** - 系统级统计数据和成功率分析
- **任务管理** - 支持移除任务等管理操作
- **实时刷新** - 每 5 秒自动刷新数据

### 技术特点

- **独立子包** - 独立的 `go.mod`，可选择性集成
- **轻量级前端** - 使用 TailwindCSS + Alpine.js + HTMX，无需构建步骤
- **RESTful API** - 标准的 HTTP API 接口
- **嵌入式资源** - 前端文件通过 embed 打包到二进制
- **CORS 支持** - 支持跨域请求

##  安装

Dashboard 是一个独立的子包，需要单独引入：

```bash
go get github.com/darkit/cron/dashboard
```

##  快速开始

### 基本使用

```go
package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/darkit/cron"
	"github.com/darkit/cron/dashboard"
	"github.com/darkit/cron/history"
)

func main() {
	// 1. 创建历史记录存储（可选）
	storage, _ := history.NewFileStorage("./history")
	defer storage.Close()

	recorder := history.NewHistoryRecorder(storage)
	defer recorder.Close()

	// 2. 创建调度器
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()

	c := cron.New(
		cron.WithHistoryRecorder(recorder),
		cron.WithContext(ctx),
	)

	// 3. 添加任务
	c.Schedule("task-1", "@every 10s", func(ctx context.Context) {
		// 任务逻辑
	})

	c.Start()
	defer c.Stop()

// 4. 启动 Dashboard（可选）
dashboardServer := dashboard.NewServer(c, ":8080")
if err := dashboardServer.Start(); err != nil {
    panic(err)
}
defer dashboardServer.Stop()

	// 访问 http://localhost:8080
	<-ctx.Done()
}
```

### 自定义端口

```go
// 使用自定义端口
dashboardServer := dashboard.NewServer(c, ":9000")

// 使用默认端口（:8080）
dashboardServer := dashboard.NewServer(c, "")
```

##  API 文档

Dashboard 提供以下 RESTful API 端点：

> 兼容说明：`POST /api/tasks/{id}/unfuse` 仍可用，但它只是历史兼容别名，行为等同于 `POST /api/tasks/{id}/resume`，新接入应统一使用 `/resume`。
>
> 机读契约：完整 OpenAPI 草案见 `dashboard/openapi.yaml`。

### 任务管理

#### GET /api/tasks

获取所有任务列表。

**响应示例：**

```json
[
  {
    "id": "task-1",
    "schedule": "@every 10s",
    "nextRun": "2025-10-30T18:00:10Z",
    "isRunning": false,
    "runCount": 120,
    "successCount": 118,
    "failCount": 2,
    "retryCount": 5,
    "lastRunTime": "2025-10-30T18:00:00Z",
    "lastRunStatus": "success",
    "lastError": "",
    "descriptions": {}
  }
]
```

#### GET /api/tasks/{id}

获取单个任务详情。

**参数：**
- `id` - 任务 ID

**响应：** 同上单个任务对象

#### DELETE /api/tasks/{id}

移除指定任务。

**参数：**
- `id` - 任务 ID

**响应：**

```json
{
  "message": "Task removed successfully"
}
```

#### POST /api/tasks/{id}/run

立即触发指定任务一次。

**响应：**

```json
{
  "message": "Task triggered"
}
```

#### POST /api/tasks/{id}/pause

暂停指定任务的后续调度。

**响应：**

```json
{
  "message": "Task paused"
}
```

#### POST /api/tasks/{id}/resume

恢复指定任务的调度。这是恢复任务的 canonical API。

**响应：**

```json
{
  "message": "Task resumed"
}
```

#### POST /api/tasks/{id}/unfuse

兼容旧版“解除熔断”入口，本质等价于 `POST /api/tasks/{id}/resume`。
仅为兼容旧客户端而保留，新接入请统一使用 `/resume`。

**响应：**

```json
{
  "message": "Task unfused (deprecated alias; compatibility alias of /resume)"
}
```

#### PATCH /api/tasks/{id}/schedule

更新指定任务的调度表达式。

**请求体：**

```json
{
  "schedule": "*/10 * * * * *"
}
```

**响应：**

```json
{
  "message": "Schedule updated"
}
```

### 错误码、认证与 CORS 边界

- `400` - 参数错误、非法 JSON、空 `schedule`、或任务状态/ID 不合法
- `401` - 当启用 `WithAPIKey(...)` 且未提供或提供了错误 API Key
- `403` - 当配置 `WithAllowedOrigins(...)` 后，请求 `Origin` 不在 allowlist 内；响应为 JSON `ErrorResponse`，消息为 `Origin not allowed`
- `404` - `DELETE /api/tasks/{id}` 删除不存在任务时返回

当启用 `WithAPIKey(...)` 时，认证适用于所有 `/api/*` 接口，包括读接口与写接口。支持三种携带方式：

- Header：`X-API-Key`
- Query：`api_key`
- Header：`Authorization: Bearer <token>`

当配置 allowlist 且请求没有 `Origin` 头时，server-to-server 请求会直接放行，但不会附加 CORS 响应头。

### 统计信息

#### GET /api/stats

获取系统级统计信息。

**响应示例：**

```json
{
  "totalTasks": 5,
  "runningTasks": 2,
  "totalRuns": 1250,
  "successRuns": 1200,
  "failedRuns": 50,
  "totalRetries": 25,
  "successRate": 96.0,
  "avgDuration": "N/A",
  "totalDuration": "N/A",
  "historyRecords": 1250
}
```

### 历史记录

#### GET /api/history

查询任务执行历史记录，支持多种过滤条件和分页。

**查询参数：**

| 参数 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `taskId` | string | 按任务 ID 筛选 | - |
| `successOnly` | boolean | 仅成功记录 | false |
| `failedOnly` | boolean | 仅失败记录 | false |
| `startTime` | string | 开始时间（RFC3339 格式） | - |
| `endTime` | string | 结束时间（RFC3339 格式） | - |
| `limit` | int | 每页记录数 | 50 |
| `offset` | int | 偏移量 | 0 |

**响应示例：**

```json
{
  "records": [
    {
      "id": "task-1_1730304000123",
      "taskID": "task-1",
      "startTime": "2025-10-30T18:00:00Z",
      "endTime": "2025-10-30T18:00:01Z",
      "duration": 1000000000,
      "success": true,
      "retryCount": 0,
      "error": ""
    }
  ],
  "total": 1250,
  "page": 1,
  "pageSize": 50,
  "totalPages": 25
}
```

**查询示例：**

```bash
# 查询特定任务的历史
curl "http://localhost:8080/api/history?taskId=task-1&limit=10"

# 查询最近 1 小时的失败记录
curl "http://localhost:8080/api/history?failedOnly=true&startTime=2025-10-30T17:00:00Z"

# 分页查询
curl "http://localhost:8080/api/history?limit=20&offset=40"
```

##  Web 界面

Dashboard 提供了简洁直观的 Web 界面，包含三个主要标签页：

### 1. 任务列表

- 显示所有任务的实时状态
- 任务 ID、状态、下次执行时间
- 运行次数、成功/失败统计
- 支持移除任务操作

### 2. 执行历史

- 查看任务执行历史记录
- 支持按任务 ID 和状态筛选
- 显示开始时间、耗时、状态、重试次数
- 分页浏览历史记录

### 3. 统计分析

- 系统级统计数据
- 总运行次数、成功/失败次数
- 成功率分析
- 重试次数统计

##  集成示例

### 完整示例

查看 `dashboard/examples/main.go` 获取完整的集成示例：

```bash
go run dashboard/examples/main.go
```

然后在浏览器中访问 http://localhost:8080

### 与历史记录集成

Dashboard 自动集成历史记录功能（如果启用）：

```go
// 启用历史记录
storage, _ := history.NewFileStorage("./history")
recorder := history.NewHistoryRecorder(storage)

c := cron.New(cron.WithHistoryRecorder(recorder))

// Dashboard 会自动显示历史记录
dashboardServer := dashboard.NewServer(c, ":8080")
```

### 与上下文集成

支持优雅关闭：

```go
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
defer cancel()

c := cron.New(cron.WithContext(ctx))
dashboardServer := dashboard.NewServer(c, ":8080")

dashboardServer.Start()
defer dashboardServer.Stop()

<-ctx.Done() // 等待中断信号
```

##  数据刷新

Dashboard 使用以下刷新策略：

- **自动刷新** - 每 5 秒自动刷新当前标签页数据
- **手动刷新** - 每个标签页都有刷新按钮
- **实时统计** - 顶部统计卡片始终显示最新数据

## ️ 安全考虑

### 内置安全功能

Dashboard 提供了内置的安全配置选项：

#### API Key 认证

```go
// 启用 API Key 认证
dashboardServer := dashboard.NewServer(c, ":8080",
    dashboard.WithAPIKey("your-secret-api-key"),
)

// 客户端调用时需要携带 API Key
// 方式1: Header
curl -H "X-API-Key: your-secret-api-key" http://localhost:8080/api/tasks

// 方式2: Query 参数
curl "http://localhost:8080/api/tasks?api_key=your-secret-api-key"

// 方式3: Bearer Token
curl -H "Authorization: Bearer your-secret-api-key" http://localhost:8080/api/tasks
```

#### CORS 来源限制

```go
// 限制允许的 CORS 来源
dashboardServer := dashboard.NewServer(c, ":8080",
    dashboard.WithAllowedOrigins([]string{
        "https://admin.example.com",
        "https://dashboard.example.com",
    }),
)

// 空切片表示允许所有来源（默认行为）
dashboardServer := dashboard.NewServer(c, ":8080",
    dashboard.WithAllowedOrigins([]string{}),
)
```

#### 组合使用

```go
// 同时启用 API Key 和 CORS 限制
dashboardServer := dashboard.NewServer(c, ":8080",
    dashboard.WithAPIKey("your-secret-api-key"),
    dashboard.WithAllowedOrigins([]string{"https://admin.example.com"}),
)
```

### 生产环境建议

1. **访问控制** - 使用反向代理添加认证
2. **网络隔离** - 仅在内网访问或使用 VPN
3. **HTTPS** - 通过反向代理启用 HTTPS
4. **防火墙** - 限制 Dashboard 端口访问

### Nginx 反向代理示例

```nginx
server {
    listen 443 ssl;
    server_name dashboard.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # 基本认证
    auth_basic "Cron Dashboard";
    auth_basic_user_file /etc/nginx/.htpasswd;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

##  测试

运行 Dashboard 单元测试：

```bash
cd dashboard
go test -v
```

测试覆盖：

- Handler API 端点测试
- Server 启动/停止测试
- CORS 中间件测试
- 历史记录集成测试

##  许可证

Dashboard 随主项目一起发布，遵循 MIT 许可证。

##  贡献

欢迎提交 Issue 和 Pull Request！

---

**简洁、直观、易用** - 这就是 Cron Dashboard 的设计理念！
