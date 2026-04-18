# Cron

[![GoDoc](https://pkg.go.dev/github.com/darkit/cron?status.svg)](https://pkg.go.dev/github.com/darkit/cron)
[![Go Report Card](https://goreportcard.com/badge/github.com/darkit/cron)](https://goreportcard.com/report/github.com/darkit/cron)
[![CI](https://github.com/darkit/cron/workflows/Go/badge.svg)](https://github.com/darkit/cron/actions)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/darkit/cron/blob/master/LICENSE)

> 轻量级、安全可靠的 Go 语言计划任务调度库，零外部依赖。

## 特性

- **简洁 API** — `Schedule`、`ScheduleJob`、`ScheduleJobByName` 三种核心调度方式
- **安全保障** — 内置 panic 捕获与异常恢复，任务异常不影响调度器运行
- **并发安全** — 通过 `-race` 检测器验证，支持高并发场景
- **Cron 表达式** — 兼容标准 5/6 段表达式，支持 `@hourly`、`@every`、`L`/`W`/`#` 高级语法
- **时区支持** — `TZ=` / `CRON_TZ=` 前缀直接指定时区
- **上下文集成** — 所有任务函数支持 `context.Context`，与 `signal.NotifyContext` 无缝配合
- **智能重试** — 固定次数 / 无限 / 立即重试，可配间隔
- **运行时控制** — 动态添加、移除、暂停、恢复、立即触发、更新调度规则
- **Misfire 策略** — `skip` / `once` / `catchup` 三种错过执行的补偿策略
- **失败熔断** — 连续失败达到阈值自动暂停，防止资源浪费
- **事件钩子** — 任务生命周期回调，内置 Logger/Channel 钩子
- **历史记录** — 基于 JSONL 的持久化存储，支持查询、统计、清理
- **Sugar API** — `ScheduleOnceAt`、`ScheduleLimited` 等语义化便捷方法
- **任务注册** — 支持 `RegisteredJob` 接口的自动注册与批量调度
- **Web Dashboard** — 独立子包，提供可视化任务管理与 RESTful API

## 安装

```bash
go get github.com/darkit/cron
```

> Go 1.23+，零外部依赖。

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/darkit/cron"
)

func main() {
    c := cron.New()

    c.Schedule("hello", "*/5 * * * * *", func(ctx context.Context) {
        fmt.Println("每 5 秒执行一次")
    })

    c.Start()
    defer c.StopGracefully(5 * time.Second)

    time.Sleep(30 * time.Second)
}
```

## 用法

### 调度函数任务

```go
c := cron.New()
c.Schedule("task-id", "*/5 * * * * *", func(ctx context.Context) {
    fmt.Println("执行任务")
})
c.Start()
```

### 调度 Job 接口

```go
type BackupJob struct{}

func (j *BackupJob) Name() string { return "backup" }

func (j *BackupJob) Run(ctx context.Context) error {
    fmt.Println("执行备份")
    return nil
}

c.ScheduleJob("backup", "0 2 * * *", &BackupJob{})

// 或使用 Job.Name() 自动命名
c.ScheduleJobByName("0 2 * * *", &BackupJob{})
```

### 配置选项

```go
c.Schedule("task", "*/5 * * * * *", handler, cron.JobOptions{
    Timeout:       30 * time.Second,   // 超时时间
    MaxRetries:    3,                   // 最大重试次数（-1 无限，0 不重试）
    RetryInterval: 1 * time.Second,     // 重试间隔（0 立即重试）
    Async:         true,                // 异步执行
    MaxConcurrent: 3,                   // 最大并发数（0 不限）
    Labels:        map[string]string{"team": "ops"}, // 任务标签
})
```

### 生命周期管理

```go
// 绑定系统信号实现优雅关闭
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()

c := cron.New(cron.WithContext(ctx))
c.Start()

<-ctx.Done() // 收到信号时调度器自动停止
```

### 运行时控制

```go
c.RunNow("task-id")           // 立即触发
c.Update("task-id", "0 * * * *") // 更新调度规则
c.Pause("task-id")            // 暂停
c.Resume("task-id")           // 恢复
c.PauseAll()                  // 暂停全部
c.ResumeAll()                 // 恢复全部
c.Remove("task-id")           // 移除
```

### 优雅关闭

```go
// Stop / StopGracefully — 停止调度，不释放外部注入资源
c.StopGracefully(30 * time.Second)

// Close — 终局收口，关闭托管资源（如 history recorder），关闭后不可复用
defer c.Close()
```

### 事件钩子

```go
// 内置日志钩子
c := cron.New(cron.WithEventHook(cron.NewEventLoggerHook(logger)))

// 自定义钩子
hook := func(ev cron.Event) {
    if ev.Success {
        log.Infof("task %s done, duration=%s", ev.TaskID, ev.Duration)
    }
}
c := cron.New(cron.WithEventHook(hook))

// 推送到 channel
ch := make(chan cron.Event, 100)
c := cron.New(cron.WithEventHook(cron.NewEventChannelHook(ch)))
```

### Misfire 策略

```go
c.Schedule("task", "*/1 * * * * *", handler, cron.JobOptions{
    MisfirePolicy: cron.MisfireSkip,     // 跳过（默认）
    // MisfirePolicy: cron.MisfireRunOnce,  // 补跑一次
    // MisfirePolicy: cron.MisfireCatchUp,  // 追赶补跑（最多 5 次，可配 MaxCatchUp）
})
```

### 失败熔断

```go
c.Schedule("api-task", "@every 1m", handler, cron.JobOptions{
    FailThreshold: 5,                // 连续失败 5 次触发熔断
    FailWindow:    10 * time.Minute, // 10 分钟窗口内统计
    PauseDuration: 30 * time.Minute, // 自动暂停 30 分钟
})
```

### 历史记录

```go
import "github.com/darkit/cron/history"

storage, _ := history.NewFileStorage("./history")
recorder := history.NewHistoryRecorder(storage)
defer recorder.Close()

c := cron.New(cron.WithHistoryRecorder(recorder))

// 查询
records, _ := c.QueryHistory(history.RecordFilter{
    TaskID:      "my-task",
    FailedOnly:  true,
    StartTime:   &yesterday,
    EndTime:     &now,
    Limit:       10,
})

// 统计
count, _ := c.CountHistory(history.RecordFilter{TaskID: "my-task"})

// 清理（30 天前的记录）
deleted, _ := c.CleanupHistory(time.Now().Add(-30 * 24 * time.Hour))
```

### 任务注册

```go
// 推荐：实例化注册表
reg := cron.NewJobRegistry()
reg.SafeRegister(&BackupJob{})
reg.SafeRegister(&CleanupJob{})

c := cron.New()
c.ScheduleFromRegistry(reg)
c.Start()
```

### Sugar API — 一次性与有限次任务

```go
// 指定时间执行一次
c.ScheduleOnceAt("remind", runAt, func(ctx context.Context) { ... })

// 限制最多执行 N 次
c.ScheduleLimited("batch", "*/10 * * * * *", 5, func(ctx context.Context) { ... })

// 指定首次执行时间 + 限次
c.ScheduleLimitedFrom("batch", "*/10 * * * * *", startAt, 5, func(ctx context.Context) { ... })
```

### 自定义日志

```go
c := cron.New(cron.WithLogger(&cron.NoOpLogger{}))  // 禁用日志

// 自定义实现 Logger 接口
c := cron.New(cron.WithLogger(&MyLogger{}))
```

## Cron 表达式

### 标准语法

```go
// 6 段（秒级）
"*/30 * * * * *"     // 每 30 秒
"0 */5 * * * *"      // 每 5 分钟
"0 0 8 * * *"        // 每天 8:00

// 5 段（分钟级，自动在秒位补 0）
"*/5 * * * *"        // 每 5 分钟
"0 8 * * *"          // 每天 8:00
"0 0 1 * *"          // 每月 1 日
```

### 描述符

```go
"@hourly"            // 每小时
"@daily"             // 每天
"@weekly"            // 每周
"@monthly"           // 每月
"@yearly"            // 每年
"@every 5s"          // 每 5 秒
"@every 1h30m"       // 每 1 小时 30 分钟
```

### 高级语法

```go
"0 0 L * *"          // 每月最后一天
"0 0 15W * *"        // 每月 15 号最近的工作日
"0 0 * * 1#2"        // 每月第二个周一
"0 0 * * 1L"         // 每月最后一个周一
```

### 时区

```go
"TZ=America/New_York 0 0 * * *"         // 纽约时间每天 0 点
"CRON_TZ=Asia/Shanghai 0 30 9 * * *"    // 上海时间每天 9:30
```

## Web Dashboard

Dashboard 是独立子包（`github.com/darkit/cron/dashboard`），不增加主库依赖。

```go
import (
    "github.com/darkit/cron"
    "github.com/darkit/cron/dashboard"
)

c := cron.New()
c.Schedule("task-1", "@every 10s", handler)
c.Start()

srv := dashboard.NewServer(c, ":8080")
srv.Start()
defer srv.Stop()

// 访问 http://localhost:8080
```

### Dashboard API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/tasks` | 任务列表 |
| `GET` | `/api/tasks/{id}` | 任务详情 |
| `DELETE` | `/api/tasks/{id}` | 移除任务 |
| `POST` | `/api/tasks/{id}/run` | 立即触发 |
| `POST` | `/api/tasks/{id}/pause` | 暂停 |
| `POST` | `/api/tasks/{id}/resume` | 恢复 |
| `PATCH` | `/api/tasks/{id}/schedule` | 更新调度规则 |
| `GET` | `/api/stats` | 统计信息 |
| `GET` | `/api/history` | 历史记录（分页） |

```bash
# curl 示例
curl -X POST http://localhost:8080/api/tasks/{id}/run
curl -X POST http://localhost:8080/api/tasks/{id}/pause
curl -X POST http://localhost:8080/api/tasks/{id}/resume
curl -X PATCH -H 'Content-Type: application/json' \
  -d '{"schedule":"*/10 * * * * *"}' \
  http://localhost:8080/api/tasks/{id}/schedule
```

详细文档：[dashboard/README.md](./dashboard/README.md)

## 示例

| 目录 | 说明 |
|------|------|
| `examples/minimal/` | 基础调度（函数 + Job + ScheduleJobByName） |
| `examples/context-lifecycle/` | Context 生命周期管理 |
| `examples/retry/` | 重试机制 |
| `examples/history/` | 历史记录 |
| `examples/custom-logger/` | 自定义日志 |
| `examples/panic-recovery/` | Panic 恢复 |
| `examples/auto-register/` | 实例化 JobRegistry |
| `examples/true-auto-register/` | 全局注册 + 空导入 |
| `dashboard/examples/` | Web Dashboard |

```bash
go run examples/minimal/main.go
go run examples/retry/main.go
go run examples/history/main.go
go run dashboard/examples/main.go
```

## 完整 API

### 核心调度

```go
// 创建调度器
func New(opts ...Option) *Cron

// 添加任务
func (c *Cron) Schedule(id, schedule string, handler func(ctx context.Context), opts ...JobOptions) error
func (c *Cron) ScheduleJob(id, schedule string, job Job, opts ...JobOptions) error
func (c *Cron) ScheduleJobByName(schedule string, job Job, opts ...JobOptions) error

// Sugar API
func (c *Cron) ScheduleOnceAt(id string, runAt time.Time, handler func(ctx context.Context), opts ...JobOptions) error
func (c *Cron) ScheduleJobOnceAt(id string, runAt time.Time, job Job, opts ...JobOptions) error
func (c *Cron) ScheduleLimited(id, schedule string, maxRuns int, handler func(ctx context.Context), opts ...JobOptions) error
func (c *Cron) ScheduleLimitedFrom(id, schedule string, startAt time.Time, maxRuns int, handler func(ctx context.Context), opts ...JobOptions) error
func (c *Cron) ScheduleLimitedJob(id, schedule string, maxRuns int, job Job, opts ...JobOptions) error
func (c *Cron) ScheduleLimitedJobFrom(id, schedule string, startAt time.Time, maxRuns int, job Job, opts ...JobOptions) error
```

### 生命周期

```go
func (c *Cron) Start() error
func (c *Cron) Stop()
func (c *Cron) StopGracefully(timeout time.Duration)
func (c *Cron) Close() error
func (c *Cron) IsRunning() bool
```

### 运行时控制

```go
func (c *Cron) RunNow(id string) error
func (c *Cron) Update(id, schedule string, opts ...JobOptions) error
func (c *Cron) Pause(id string) error
func (c *Cron) Resume(id string) error
func (c *Cron) PauseAll()
func (c *Cron) ResumeAll()
func (c *Cron) Remove(id string) error
```

### 查询与监控

```go
func (c *Cron) List() []string
func (c *Cron) NextRun(id string) (time.Time, error)
func (c *Cron) GetTask(id string) (*TaskInfo, bool)
func (c *Cron) GetAllTasks() []*TaskInfo
func (c *Cron) GetStats(id string) (*Stats, bool)
func (c *Cron) GetAllStats() map[string]*Stats
```

### 构造选项

```go
func WithLogger(logger Logger) Option
func WithContext(ctx context.Context) Option
func WithEventHook(hook EventHook) Option
func WithPanicHandler(handler PanicHandler) Option
func WithHistoryRecorder(recorder history.Recorder) Option
```

### 接口

```go
type Job interface {
    Run(ctx context.Context) error
    Name() string
}

type RegisteredJob interface {
    Job
    Schedule() string
}

type Logger interface {
    Debugf(format string, args ...any)
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
}

type PanicHandler interface {
    HandlePanic(taskID string, panicValue interface{}, stack []byte)
}
```

## 贡献

欢迎提交 Issue 和 Pull Request！请参考 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解贡献指南。

## 许可证

[MIT License](./LICENSE)
