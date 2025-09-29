# 自动注册示例

演示如何使用空导入（blank import）实现任务的自动注册功能。

## 功能特性

- 🎯 **空导入注册** - 使用`import _ "jobs"`自动注册所有任务  
- 📋 **任务发现** - 自动发现并列出所有已注册的任务
- ⚡ **一键调度** - 使用`ScheduleRegistered()`一次性调度所有注册任务
- 🏗️ **解耦设计** - 任务定义与调度逻辑完全分离
- 🔧 **统一配置** - 所有注册任务可应用相同的配置选项

## 目录结构

```
true-auto-register/
├── main.go          # 主程序，演示自动注册
└── ../jobs/         # 任务定义包
    ├── backup.go    # 自动备份任务
    ├── cleanup.go   # 自动清理任务
    └── monitor.go   # 系统监控任务
```

## 运行示例

```bash
cd examples/true-auto-register  
go run main.go
```

## 核心概念

### 1. 任务定义（jobs包）
```go
type BackupJob struct { /* ... */ }

func init() {
    // 包初始化时自动注册
    cron.MustRegisterJob(&BackupJob{})
}

func (j *BackupJob) Name() string { return "auto-backup" }
func (j *BackupJob) Schedule() string { return "*/3 * * * * *" }  
func (j *BackupJob) Run(ctx context.Context) error { /* ... */ }
```

### 2. 自动注册和调度
```go
import (
    "github.com/darkit/cron"
    _ "github.com/darkit/cron/examples/jobs" // 空导入触发注册
)

func main() {
    scheduler := cron.New()
    
    // 一键调度所有已注册的任务
    scheduler.ScheduleRegistered()
    scheduler.Start()
}
```

## 优势

- ✅ **零配置** - 任务包导入即可自动注册
- ✅ **模块化** - 任务定义与使用完全解耦  
- ✅ **可发现** - 自动发现所有可用任务
- ✅ **易扩展** - 新增任务无需修改主程序