# Cron 计划任务管理库

[![GoDoc](https://godoc.org/github.com/darkit/cron?status.svg)](https://pkg.go.dev/github.com/darkit/cron)
[![Go Report Card](https://goreportcard.com/badge/github.com/darkit/cron)](https://goreportcard.com/report/github.com/darkit/cron)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/darkit/cron/blob/master/LICENSE)

一个轻量级的 Go 语言计划任务调度管理库。

## 特性

- 支持标准的 crontab 表达式（5段/6段语法）
- 自动处理分钟级的5段语法（秒位补0）
- 支持同步/异步任务执行
- 内置 panic 捕获机制
- 支持动态添加/更新/删除任务
- 支持任务接口实现
- 完善的日志记录
- 优雅的停止机制

## 安装

```bash
go get github.com/darkit/cron
```

## 使用示例

### 基本用法

```go
package main

import (
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"
    "github.com/darkit/cron"
)

func main() {
    // 创建计划任务管理器
    scheduler := cron.New()

    // 添加一个简单任务
    err := scheduler.AddFunc("print-time", "*/5 * * * * *", func() {
        fmt.Printf("当前时间: %v\n", time.Now().Format("2006-01-02 15:04:05"))
    })
    if err != nil {
        log.Fatal(err)
    }

    // 启动调度器
    scheduler.Start()

    // 等待信号优雅退出
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    // 停止调度器
    scheduler.Stop()
}
```

### 高级用法

#### 1. 使用配置添加任务

```go
err := scheduler.AddJob(cron.JobConfig{
    Name:     "complex-job",
    Schedule: "*/10 * * * * *",  // 每10秒执行
    Async:    true,              // 异步执行
    TryCatch: true,             // 启用panic捕获
}, func() {
    // 任务逻辑
})
```

#### 2. 实现任务接口

```go
type MyJob struct {
    jobName string
}

func (j *MyJob) Name() string    { return j.jobName }
func (j *MyJob) Rule() string    { return "*/15 * * * * *" }
func (j *MyJob) Execute() {
    // 任务逻辑
}

// 添加任务
err := scheduler.AddJobInterface(&MyJob{jobName: "interface-job"})
```

#### 3. 设置 Panic 处理器

```go
type MyPanicHandler struct{}

func (h *MyPanicHandler) Handle(jobName string, err interface{}) {
    log.Printf("任务 [%s] 发生panic: %v", jobName, err)
}

scheduler := cron.New(cron.WithPanicHandler(&MyPanicHandler{}))
```

#### 4. 动态更新任务

```go
err := scheduler.UpdateJob(cron.JobConfig{
    Name:     "print-time",
    Schedule: "*/30 * * * * *",  // 更新为每30秒执行
}, func() {
    // 新的任务逻辑
})
```

#### 5. 获取任务下次执行时间

```go
nextTime, err := scheduler.NextRuntime("print-time")
if err != nil {
    log.Printf("获取执行时间失败: %v", err)
} else {
    fmt.Printf("下次执行时间: %v\n", nextTime.Format("2006-01-02 15:04:05"))
}
```

### 高级特性

#### 并发控制
```go
scheduler.AddJob(cron.JobConfig{
    Name:          "concurrent-job",
    Schedule:      "*/5 * * * * *",
    Async:         true,
    MaxConcurrent: 3,  // 限制最大并发数为3
}, func() {
    // 任务逻辑
})
```

#### 任务超时控制
```go
scheduler.AddJob(cron.JobConfig{
    Name:     "timeout-job",
    Schedule: "*/5 * * * * *",
    Timeout:  time.Second * 30,  // 设置30秒超时
}, func() {
    // 任务逻辑
})
```

### JobConfig 完整配置项
```go
type JobConfig struct {
    Name          string         // 任务名称
    Schedule      string         // 定时规则
    Async         bool           // 是否异步执行
    TryCatch      bool           // 是否进行异常捕获
    Timeout       time.Duration  // 任务超时时间
    MaxConcurrent int           // 最大并发数（仅异步任务有效）
}
```

### 最佳实践

1. 对于耗时任务：
   - 设置 `Async: true` 启用异步执行
   - 使用 `MaxConcurrent` 控制并发数
   - 设置合理的 `Timeout` 避免任务阻塞

2. 对于关键任务：
   - 启用 `TryCatch` 捕获异常
   - 使用 `NextRuntime` 监控执行计划

## Crontab 表达式

支持两种格式的 crontab 表达式：

### 6段语法（秒级crontab）
格式：`秒 分 时 日 月 周`
例如：
- `*/5 * * * * *` : 每5秒执行一次
- `0 */1 * * * *` : 每分钟执行一次
- `0 0 * * * *` : 每小时执行一次

### 5段语法（标准crontab）
格式：`分 时 日 月 周`
例如：
- `*/1 * * * *` : 每分钟执行一次（等同于 "0 */1 * * * *"）
- `0 * * * *` : 每小时执行一次（等同于 "0 0 * * * *"）
- `0 0 * * *` : 每天零点执行一次（等同于 "0 0 0 * * *"）

## API 文档

### 类型定义

```go
// 任务配置
type JobConfig struct {
    Name     string // 任务名称
    Schedule string // 定时规则
    Async    bool   // 是否异步执行
    TryCatch bool   // 是否进行异常捕获
}

// 任务接口
type CronJob interface {
    Name() string     // 返回任务名称
    Rule() string     // 返回cron表达式
    Execute()         // 返回执行函数
}

// Panic处理接口
type PanicHandler interface {
    Handle(jobName string, err interface{})
}
```

### 主要方法

```go
// 创建计划任务管理器
func New(opts ...Option) *Crontab

// 添加任务的多种方式
func (c *Crontab) AddJob(config JobConfig, fn func()) error
func (c *Crontab) AddFunc(name, spec string, fn func()) error
func (c *Crontab) AddJobInterface(jobs ...CronJob) error

// 更新任务
func (c *Crontab) UpdateJob(config JobConfig, fn func()) error

// 停止任务
func (c *Crontab) StopService(names ...string)
func (c *Crontab) StopServicePrefix(namePrefix string)

// 调度器控制
func (c *Crontab) Start()
func (c *Crontab) Stop()
func (c *Crontab) Reload()

// 任务信息
func (c *Crontab) NextRuntime(name string) (time.Time, error)
```

## 最佳实践

1. 任务命名建议使用有意义的名称，便于管理和调试
2. 长时间运行的任务建议使用 `Async: true`
3. 可能发生panic的任务建议使用 `TryCatch: true`
4. 任务执行时间不应超过调度间隔
5. 在生产环境中建议设置 panic 处理器
6. 使用 `StopServicePrefix` 可以批量管理相关任务
7. 使用 `NextRuntime` 可以预判任务执行时间
8. 对于不需要秒级精度的任务，推荐使用5段语法

## 许可证

MIT License