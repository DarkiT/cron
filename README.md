# Cron 计划任务调度库

[![GoDoc](https://godoc.org/github.com/darkit/cron?status.svg)](https://pkg.go.dev/github.com/darkit/cron)
[![Go Report Card](https://goreportcard.com/badge/github.com/darkit/cron)](https://goreportcard.com/report/github.com/darkit/cron)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/darkit/cron/blob/master/LICENSE)

一个轻量级、安全可靠的 Go 语言计划任务调度库。

## 🚀 特性

- **🎯 简洁的API设计** - 提供Schedule、SafeSchedule等直观易用的方法
- **🛡️ 完善的安全保障** - 内置panic捕获和异常恢复机制，确保程序稳定运行
- **📝 灵活的日志系统** - 支持自定义Logger接口，默认使用log/slog结构化日志
- **🔀 上下文支持** - 所有任务函数支持context.Context进行取消和超时控制
- **📊 基础监控功能** - 提供任务执行统计和状态查询
- **🎭 多种调度方式** - 支持函数调度和Job接口调度
- **🔄 运行时管理** - 支持动态添加、删除任务
- **🔧 基础配置选项** - 支持异步执行、超时控制、并发限制
- **🌟 Cron表达式支持** - 兼容标准5段和6段cron表达式，支持@hourly、@daily、@every等描述符
- **🤖 自动注册功能** - 支持Job的自动注册和批量调度

## 📦 安装

```bash
go get github.com/darkit/cron
```

## 🎯 快速开始

### 最简单的使用方式

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/darkit/cron"
)

func main() {
    // 创建调度器
    c := cron.New()
    
    // 添加任务 - 安全可靠
    c.Schedule("hello", "*/5 * * * * *", func(ctx context.Context) {
        fmt.Println("每5秒执行一次")
    })
    
    // 启动调度器
    c.Start()
    defer c.Stop()
    
    // 等待一段时间观察执行
    time.Sleep(30 * time.Second)
}
```

### 内置安全保障的任务调度

```go
func main() {
    c := cron.New()
    
    // 所有任务都自动具备panic保护，无需特殊API
    c.Schedule("safe-task", "*/3 * * * * *", func(ctx context.Context) {
        fmt.Println("安全任务执行")
    })
    
    // 即使任务内部panic也不会影响调度器
    c.Schedule("risky-task", "*/10 * * * * *", func(ctx context.Context) {
        if someCondition {
            panic("任务遇到问题") // 这不会让程序崩溃
        }
        fmt.Println("正常执行")
    })
    
    c.Start()
    defer c.Stop()
    
    time.Sleep(time.Minute)
}
```

## 🛡️ 安全保障机制

### 永不崩溃的设计

```go
func main() {
    c := cron.New()
    
    // 即使任务panic，程序也不会崩溃（内置保护）
    c.Schedule("panic-task", "*/5 * * * * *", func(ctx context.Context) {
        panic("这个panic不会让程序崩溃！")
    })
    
    c.Start()
    // 程序会持续运行，不受任务异常影响
}
```

### 自定义异常处理

```go
// 自定义panic处理器
type MyPanicHandler struct{}

func (h *MyPanicHandler) HandlePanic(taskID string, panicValue interface{}, stack []byte) {
    log.Printf("⚠️ 任务 %s 异常: %v", taskID, panicValue)
    // 发送告警、记录日志等
}

// 使用自定义处理器
c := cron.New(cron.WithPanicHandler(&MyPanicHandler{}))
```

## 🔗 生命周期管理 WithContext

### 优雅关闭与信号集成

```go
func main() {
    // 创建响应系统信号的上下文
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()
    
    // 调度器绑定到应用生命周期
    scheduler := cron.New(cron.WithContext(ctx))
    
    scheduler.Schedule("background-task", "*/10 * * * * *", func(taskCtx context.Context) {
        // 任务支持优雅取消
        select {
        case <-time.After(5 * time.Second):
            fmt.Println("任务正常完成")
        case <-taskCtx.Done():
            fmt.Println("任务收到取消信号，优雅退出")
            return
        }
    })
    
    scheduler.Start()
    
    // 收到 Ctrl+C 信号时，调度器自动停止
    <-ctx.Done()
    fmt.Println("应用优雅关闭")
}
```

### 微服务架构中的应用

```go
// Web服务器与调度器生命周期绑定
func startServer() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()
    
    // 调度器与服务器共享生命周期
    taskScheduler := cron.New(cron.WithContext(ctx))
    taskScheduler.Schedule("health-check", "@every 30s", healthCheckTask)
    taskScheduler.Start()
    
    server := &http.Server{Addr: ":8080"}
    go server.ListenAndServe()
    
    // 等待停止信号
    <-ctx.Done()
    
    // 调度器自动停止，无需手动管理
    server.Shutdown(context.Background())
}
```

### 测试场景中的精确控制

```go
func TestTaskExecution(t *testing.T) {
    // 5秒后自动停止调度器
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    scheduler := cron.New(cron.WithContext(ctx))
    
    var executionCount int64
    scheduler.Schedule("test-task", "*/1 * * * * *", func(taskCtx context.Context) {
        atomic.AddInt64(&executionCount, 1)
    })
    
    scheduler.Start()
    <-ctx.Done() // 自动停止
    
    // 验证执行次数
    assert.Equal(t, int64(5), executionCount)
}
```

### 级联取消场景

```go
// 父服务控制子调度器
func parentService(parentCtx context.Context) {
    // 子上下文继承父级取消信号
    childCtx, cancel := context.WithCancel(parentCtx) 
    defer cancel()
    
    // 调度器响应父级服务的生命周期
    scheduler := cron.New(cron.WithContext(childCtx))
    
    // 父服务停止时，调度器自动停止
}
```

## 📝 自定义日志系统

### 使用自定义Logger

```go
// 自定义日志实现
type MyLogger struct{}

func (l *MyLogger) Debugf(format string, args ...any) { 
    log.Printf("[DEBUG] "+format, args...) 
}
func (l *MyLogger) Infof(format string, args ...any) { 
    log.Printf("[INFO] "+format, args...) 
}
func (l *MyLogger) Warnf(format string, args ...any) { 
    log.Printf("[WARN] "+format, args...) 
}
func (l *MyLogger) Errorf(format string, args ...any) { 
    log.Printf("[ERROR] "+format, args...) 
}

// 创建使用自定义logger的调度器
c := cron.New(cron.WithLogger(&MyLogger{}))
```

### 默认日志实现

```go
// 默认使用log/slog，输出结构化日志：
// time=2025-08-10T18:30:54.360+08:00 level=INFO msg="Starting cron scheduler"

// 禁用日志输出
c := cron.New(cron.WithLogger(&cron.NoOpLogger{}))
```

## 🎭 Job接口支持

### 实现Job接口

```go
type BackupJob struct {
    name string
    path string
}

func (j *BackupJob) Name() string {
    return j.name
}

func (j *BackupJob) Run(ctx context.Context) error {
    fmt.Printf("执行备份任务: %s，路径: %s\n", j.name, j.path)
    
    // 支持上下文取消
    select {
    case <-time.After(2 * time.Second):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func main() {
    c := cron.New()
    job := &BackupJob{name: "daily-backup", path: "/data"}
    
    // 方式1：传统API - 手动指定任务ID
    c.ScheduleJob("backup", "0 2 * * *", job)
    
    // 方式2：新API - 自动使用Job的Name()方法 ✨
    c.ScheduleJobByName("0 2 * * *", job)
    
    c.Start()
    defer c.Stop()
}
```

### 🌟 工厂模式 - 更简洁优雅的API

```go
// 工厂函数
func NewBackupJob(path string) *BackupJob {
    return &BackupJob{
        name: "backup-job",
        path: path,
    }
}

func NewCleanupJob(target string) *CleanupJob {
    return &CleanupJob{
        name: "cleanup-job", 
        target: target,
    }
}

func main() {
    c := cron.New()
    
    // 🎯 极简优雅的调度方式
    c.ScheduleJobByName("0 2 * * *", NewBackupJob("/data"))     // 每天2点备份
    c.ScheduleJobByName("0 3 * * *", NewCleanupJob("temp"))     // 每天3点清理
    c.ScheduleJobByName("*/30 * * * * *", NewMonitorJob())      // 每30秒监控
    
    c.Start()
    defer c.Stop()
}
```

### Job配置选项

```go
// 使用JobOptions配置任务
c.ScheduleJob("heavy-task", "*/5 * * * * *", job, cron.JobOptions{
    Async:         true,              // 异步执行
    Timeout:       30 * time.Second,  // 超时时间
    MaxConcurrent: 3,                 // 最大并发数
})
```

## 🤖 自动注册功能

### RegisteredJob接口

```go
type CleanupJob struct{}

func (j *CleanupJob) Name() string { return "cleanup" }
func (j *CleanupJob) Schedule() string { return "0 3 * * *" }
func (j *CleanupJob) Run(ctx context.Context) error {
    fmt.Println("执行清理任务")
    return nil
}

func init() {
    // 在包初始化时自动注册
    cron.SafeRegisterJob(&CleanupJob{})
}

func main() {
    c := cron.New()
    
    // 批量调度所有已注册的任务
    c.ScheduleRegistered()
    
    c.Start()
    defer c.Stop()
}
```

## 📊 监控和统计

```go
func main() {
    c := cron.New()
    c.Schedule("monitored", "*/10 * * * * *", func(ctx context.Context) {
        fmt.Println("监控任务执行")
    })
    
    c.Start()
    defer c.Stop()
    
    // 获取任务统计
    if stats, ok := c.GetStats("monitored"); ok {
        fmt.Printf("运行次数: %d, 成功次数: %d\n", 
            stats.RunCount, stats.SuccessCount)
    }
    
    // 获取所有任务统计
    allStats := c.GetAllStats()
    for id, stats := range allStats {
        fmt.Printf("任务 %s: %+v\n", id, stats)
    }
}
```

## 🔄 运行时管理

```go
func main() {
    c := cron.New()
    
    // 添加任务
    c.Schedule("task1", "*/5 * * * * *", handler1)
    c.Schedule("task2", "*/10 * * * * *", handler2)
    
    c.Start()
    
    // 列出所有任务
    tasks := c.List()
    fmt.Printf("当前任务: %v\n", tasks)
    
    // 获取下次执行时间
    if nextRun, err := c.NextRun("task1"); err == nil {
        fmt.Printf("task1下次执行: %v\n", nextRun)
    }
    
    // 移除任务
    c.Remove("task2")
    
    c.Stop()
}
```

## 🌟 Cron表达式支持

### 标准表达式

```go
// 6段表达式（秒级）
"*/30 * * * * *"      // 每30秒
"0 */5 * * * *"       // 每5分钟
"0 0 8 * * *"         // 每天8点

// 5段表达式（分钟级）
"*/5 * * * *"         // 每5分钟
"0 8 * * *"           // 每天8点
"0 0 1 * *"           // 每月1日
```

### 描述符语法

```go
// 预定义描述符
"@hourly"             // 每小时
"@daily"              // 每天
"@weekly"             // 每周
"@monthly"            // 每月
"@yearly"             // 每年

// @every语法
"@every 5s"           // 每5秒
"@every 1m"           // 每1分钟
"@every 2h"           // 每2小时
```

## 🎪 完整示例

### 运行示例

```bash
# 最简示例
go run examples/minimal/main.go

# 自动注册演示
go run examples/true-auto-register/main.go

# 异常恢复演示
go run examples/panic-recovery/main.go

# 自定义Logger演示
go run examples/custom-logger/main.go
```

### 示例说明

| 示例目录 | 功能演示 | 适用场景 |
|---------|---------|---------|
| `examples/minimal/` | 最基本的使用方式 | 快速上手，了解基本API |
| `examples/true-auto-register/` | 自动注册机制 | 学习Job自动注册和批量调度 |
| `examples/panic-recovery/` | 异常恢复演示 | 了解安全保障机制 |
| `examples/custom-logger/` | 自定义Logger | 学习日志系统定制 |
| `examples/jobs/` | 可复用的任务集合 | 参考任务实现模式 |

## 📚 完整API文档

### 核心方法

#### 🌟 核心API（简洁优雅）

```go
// ⭐ 主力推荐：最简洁优雅的API
func (c *Cron) ScheduleJobByName(schedule string, job Job, opts ...JobOptions) error

// ⭐ 显式指定任务名时使用
func (c *Cron) ScheduleJob(id, schedule string, job Job, opts ...JobOptions) error

// ⭐ 函数式调度（简单场景）
func (c *Cron) Schedule(id, schedule string, handler func(ctx context.Context)) error

// 批量调度
func (c *Cron) ScheduleRegistered(opts ...JobOptions) error

// 生命周期管理
func (c *Cron) Start() error
func (c *Cron) Stop()
func (c *Cron) IsRunning() bool

// 任务管理
func (c *Cron) Remove(id string) error
func (c *Cron) List() []string
func (c *Cron) NextRun(id string) (time.Time, error)

// 监控统计
func (c *Cron) GetStats(id string) (*Stats, bool)
func (c *Cron) GetAllStats() map[string]*Stats
```

### 配置选项

```go
// 创建选项
func New(opts ...Option) *Cron
func WithLogger(logger Logger) Option
func WithPanicHandler(handler PanicHandler) Option
func WithContext(ctx context.Context) Option

// 任务配置
type JobOptions struct {
    Timeout       time.Duration // 任务超时时间
    Async         bool          // 是否异步执行
    MaxConcurrent int           // 最大并发数
}
```

### 接口定义

```go
// 基本Job接口
type Job interface {
    Run(ctx context.Context) error
    Name() string                  // 🌟 新增：返回任务名称，用于ScheduleJobByName()
}

// 可注册Job接口
type RegisteredJob interface {
    Job
    Name() string       // 返回任务名称
    Schedule() string // cron调度表达式
}

// 💡 最佳实践：统一使用Name()方法，简化API设计
// 示例实现：
func (j *MyJob) Name() string { return "my-job" }

// 日志接口
type Logger interface {
    Debugf(format string, args ...any)
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
}

// 异常处理接口
type PanicHandler interface {
    HandlePanic(taskID string, panicValue interface{}, stack []byte)
}
```

## 🎯 最佳实践

### 1. ⭐ API方法选择指南

```go
// 🥇 首选：ScheduleJobByName - 最简洁优雅
c.ScheduleJobByName("0 2 * * *", NewBackupJob("/data"))
c.ScheduleJobByName("*/30 * * * * *", NewMonitorJob())

// 🥈 备选：ScheduleJob - 需要显式控制任务名时
c.ScheduleJob("custom-name", "0 3 * * *", job)

// 🥉 简单场景：Schedule - 函数式调度
c.Schedule("simple-task", "*/5 * * * * *", func(ctx context.Context) {
    fmt.Println("简单任务执行")
})

// ✨ 现在只有3个核心方法，简洁明了！
```

### 2. 利用上下文进行优雅取消

```go
c.Schedule("graceful-task", "*/30 * * * * *", func(ctx context.Context) {
    for i := 0; i < 100; i++ {
        select {
        case <-ctx.Done():
            return // 优雅退出
        default:
            processItem(i)
        }
    }
})
```

### 3. 异步处理耗时任务

```go
// 对于耗时任务使用异步执行
c.ScheduleJob("heavy-task", "*/5 * * * * *", heavyJob, cron.JobOptions{
    Async:   true,
    Timeout: 2 * time.Minute,
})
```

### 4. 监控任务执行状态

```go
// 定期检查任务状态
go func() {
    ticker := time.NewTicker(time.Minute)
    for range ticker.C {
        stats := c.GetAllStats()
        for id, stat := range stats {
            if stat.IsRunning {
                log.Printf("任务 %s 正在执行中", id)
            }
        }
    }
}()
```

## ❓ 常见问题

### Q: 如何确保任务不会因为panic而影响其他任务？

**A: 所有任务都内置panic保护：**

```go
// 所有核心API都自动具备panic保护，任务panic不会影响调度器
c.Schedule("task", "*/5 * * * * *", func(ctx context.Context) {
    // 即使这里panic，其他任务仍然正常执行
    panic("任务异常")
})
```

### Q: 如何自定义日志输出格式？

**A: 实现Logger接口：**

```go
type CustomLogger struct{}
func (l *CustomLogger) Infof(format string, args ...any) {
    // 自定义输出格式
    log.Printf("[CRON] "+format, args...)
}
// 其他方法...

c := cron.New(cron.WithLogger(&CustomLogger{}))
```

### Q: 如何处理长时间运行的任务？

**A: 使用异步执行和超时控制：**

```go
c.ScheduleJob("long-task", "0 2 * * *", job, cron.JobOptions{
    Async:   true,              // 异步执行，不阻塞调度器
    Timeout: 30 * time.Minute,  // 设置超时时间
})
```

### Q: 如何在程序启动时自动加载预定义任务？

**A: 使用RegisteredJob和自动注册：**

```go
// 在init函数中注册
func init() {
    cron.SafeRegisterJob(&MyJob{})
}

// 在main函数中批量调度
func main() {
    c := cron.New()
    c.ScheduleRegistered()
    c.Start()
}
```

## 🔧 故障排除

### 常见错误

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `task xxx already exists` | 任务ID重复 | 使用唯一的任务ID |
| `invalid cron spec` | cron表达式错误 | 检查表达式格式 |
| `handler cannot be nil` | 处理函数为空 | 确保传入有效函数 |
| `scheduler is already running` | 重复启动调度器 | 检查调度器状态 |

### 调试建议

```go
// 启用详细日志
logger := cron.NewDefaultLogger()
c := cron.New(cron.WithLogger(logger))

// 监控任务状态
stats := c.GetAllStats()
for id, stat := range stats {
    fmt.Printf("任务 %s: 运行%d次, 成功%d次\n", 
        id, stat.RunCount, stat.SuccessCount)
}
```

## 📄 许可证

MIT License - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！请参考 [CONTRIBUTING.md](CONTRIBUTING.md) 了解贡献指南。

---

**简洁、安全、易用** - 这就是 Cron 调度库的设计理念！