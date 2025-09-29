# 示例任务包

这是一个演示如何创建可自动注册任务的示例包。

## 包含的任务

### 🗄️ BackupJob (backup.go)
- **Name**: `auto-backup`
- **调度**: 每3秒执行一次 (`*/3 * * * * *`)
- **功能**: 模拟自动备份操作

### 🧹 CleanupJob (cleanup.go) 
- **Name**: `auto-cleanup`
- **调度**: 每5秒执行一次 (`*/5 * * * * *`)
- **功能**: 模拟自动清理操作

### 📊 MonitorJob (monitor.go)
- **Name**: `auto-monitor` 
- **调度**: 每4秒执行一次 (`*/4 * * * * *`)
- **功能**: 模拟系统监控检查

## 使用方法

### 1. 空导入自动注册
```go
import _ "github.com/darkit/cron/examples/jobs"
```

### 2. 手动注册单个任务
```go
import "github.com/darkit/cron/examples/jobs"

// 需要导出任务类型或提供工厂函数
```

## 实现模式

每个任务都遵循相同的实现模式：

```go
type XxxJob struct { /* 任务状态 */ }

var xxxCounter = int64(0) // 全局计数器

func init() {
    // 包初始化时自动注册
    job := &XxxJob{ /* 初始化 */ }
    cron.MustRegisterJob(job)
}

// 实现RegisteredJob接口
func (j *XxxJob) Name() string       { return "unique-id" }
func (j *XxxJob) Schedule() string   { return "cron-expression" } 
func (j *XxxJob) Run(ctx context.Context) error { /* 执行逻辑 */ }
```

## 扩展指南

要添加新任务：

1. 创建新的.go文件
2. 定义实现`RegisteredJob`接口的结构体
3. 在`init()`函数中调用`cron.MustRegisterJob()`
4. 实现`Name()`, `Schedule()`, `Run()`方法

任务会在包导入时自动注册，无需修改其他代码！