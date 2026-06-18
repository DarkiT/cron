# 更新日志

本文档记录了项目的所有重要变更。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 修复
- 🔧 **contextWatcher 泄漏修复** - Scheduler Stop 后 contextWatcher goroutine 未退出导致泄漏，引入 watcherStop 通道在停止时通知 watcher 退出
- 🔧 **RunNow 暂停检查** - RunNow 现在拒绝已暂停的任务
- 🔧 **ResumeAll 状态修复** - ResumeAll 正确清除暂停状态
- 🔧 **Dashboard 端口冲突修复** - 使用 net.Listen 获取实际地址避免端口冲突
- 🔧 **CORS 安全增强** - CORS 中间件不再默认返回 *，web 静态资源需 API Key 认证
- 🔧 **历史查询参数校验** - 历史查询参数严格校验（时间格式、范围、分页）
- 🔧 **EventChannelHook 阻塞修复** - 改为非阻塞 default 避免超时等待
- 🔧 **错误处理补全** - 各处 Start/Schedule/RegisterJob 调用增加错误处理
- 🔧 **生命周期测试补充** - 补充相关边界条件与生命周期测试

### 文档
-  精简项目文档与更新仓库信息

### 维护
- ⬆️ 升级 8 个 GitHub Actions 依赖（checkout v3→v6、setup-go v3→v6、action-gh-release v2→v3 等）

## [0.2.0] - 2025-07-23

### 新增
- 🔧 **MaxCatchUp 可配置** - `JobOptions.MaxCatchUp` 支持配置 MisfireCatchUp 策略的最大补跑次数（默认 5 次）
- 📊 **任务详情 API** - 新增 `GetTask(id)` 和 `GetAllTasks()` 方法，获取任务详细信息（包含 ID、调度表达式、配置、标签、下次执行时间、暂停/运行状态、创建时间）
- 🎛️ **批量任务控制** - 新增 `PauseAll()` 和 `ResumeAll()` 方法，支持一键暂停/恢复所有任务
- 🔐 **Dashboard 安全增强** - 新增 `WithAPIKey(apiKey)` 可选 API Key 认证和 `WithAllowedOrigins(origins)` 可配置 CORS 来源
- 📚 **Claude Code Skill** - 为 darkit/cron 项目创建专属技能文档，符合渐进式披露规范
  - SKILL.md (477行) - 核心技能文档
  - 5个引用文档 - API参考、配置、Cron语法、生产指南、故障排查
  - skill-rules.json - 自动触发规则配置
-  **GitHub Actions CI/CD** - 添加自动化构建、测试和发布流程
  - CI工作流：自动测试、race检测、golangci-lint检查
  - Release工作流：自动发布和changelog生成
-  **Dependabot 配置** - 自动依赖更新和安全漏洞检测
-  **代码文档自动生成** - 集成 codewiki 自动生成代码文档
- 🛡️ **Panic 保护增强** - 为历史记录器添加 panic 保护，防止异常导致程序崩溃
-  **运行时控制功能** - 新增 `RunNow(id)`、`Update(id, spec)`、`Pause(id)`、`Resume(id)` 方法，支持任务的动态管理
-  **Misfire 策略** - 支持 skip（跳过）、once（补跑一次）、catchup（追赶）三种策略，灵活处理错过的任务执行
-  **失败熔断机制** - 通过 `FailThreshold`、`FailWindow`、`PauseDuration` 配置自动熔断，防止持续失败任务占用资源
- ️ **任务标签支持** - 新增 `Labels` 字段，支持为任务添加元数据标签，Dashboard 支持标签过滤
- 🪝 **事件钩子系统** - 新增 `WithEventHook` 配置选项和 `Event` 类型，支持任务执行全生命周期监控
-  **事件钩子实现** - 提供 `NewEventLoggerHook` 和 `NewEventChannelHook` 两种默认实现
-  **优雅停止** - 新增 `StopGracefully(timeout)` 方法，支持等待在途异步任务完成
-  **实例化注册表** - 新增 `NewJobRegistry()` 构造函数和 `ScheduleFromRegistry` 方法，推荐使用实例化注册表
-  **历史清理脚本** - 提供 `scripts/cleanup_history.sh` 脚本，便于定期清理旧历史记录
- ️ 全面的 panic 恢复机制，包括 PanicHandler 接口和 SafeCall 函数
-  RecoveryJob 包装器，为现有 Job 添加异常捕获能力
-  WithPanicHandler 配置选项，支持自定义 panic 处理策略
-  panic-recovery 示例，展示异常恢复的完整使用方法
-  所有核心 API 内置 panic 保护，确保程序永不崩溃

### 变更
-  **历史存储格式** - 改用 JSONL 格式（`<task>/<date>.jsonl`），便于 tail/grep 等命令行工具分析
-  **Dashboard 增强** - 新增暂停状态显示、熔断状态可视化、标签过滤功能、运维操作 API
-  **注册表推荐** - 全局注册表（`RegisterJob`）保留仅用于兼容，推荐使用实例化注册表
-  **JobOptions 扩展** - 新增 `MisfirePolicy`、`FailThreshold`、`FailWindow`、`PauseDuration`、`Labels` 字段
-  完全移除所有可能导致程序崩溃的 Must* 系列方法（MustSchedule、MustRegisterJob 等）
-  重构 examples 目录结构，每个示例独立一个子目录
-  移动 jobs 包从根目录到 examples/jobs，统一示例代码管理
- ️ 所有示例代码更新为使用 Safe* 方法，展示最佳实践
-  更新所有 job 文件使用 RegisterJob 替代危险的 MustRegisterJob
-  清理过期的方案文档和总结文档

### 修复
- 🔧 **Context 泄露修复** - 修复调度器中的 Context 泄露风险，确保资源正确释放
- 🔧 **Monitor 并发安全** - 修复 Monitor 组件的数据竞争问题，增强并发安全性
- 🔧 **历史记录静默错误** - 修复历史记录器静默错误问题，改为日志记录
- 🔧 **Race Condition 修复** - 修复并发测试中的数据竞争问题
  - 修复 storage.go 中的并发读写问题
  - 修复 parser cache 中的数据竞争
  - 修复 monitor 中的统计数据竞争
  - 所有测试通过 `go test -race` 验证
-  修复调度数据竞争问题，提升解析健壮性
-  修复 benchmark 测试中未定义的 MustRegisterJob 引用问题
-  修复 examples 重构后的 import 路径问题
- ️ 移除所有可能造成程序意外终止的 panic 调用点

### 测试
- ✅ **测试用例增强** - 补充测试用例并添加 panic 保护
  - 新增 context_test.go - Context 相关测试
  - 新增 history/recorder_test.go - 历史记录器测试
  - 新增 execute_test.go - 任务执行测试（480行）
  - 新增 history/storage_test.go - 存储层测试（304行）
  - 新增 monitor_test.go - 监控统计测试（436行）
  - 新增 internal/parser/parser_test.go - 解析器测试增强（435行）
- 🏁 **Race Detector 验证** - 所有测试通过 `go test -race` 验证
- 📊 **测试覆盖率提升** - 核心模块覆盖率达到 80%+

### 文档
- 📚 **Claude Code Skill 创建** - 创建符合渐进式披露规范的专属技能文档
- 🔧 **配置文件完善** - 添加 golangci.yml (234行)和 .cnb/codewiki.yml 配置
-  补充运行时控制说明和 Dashboard 标签筛选文档
-  补充运维 API 示例和存储策略说明
-  推荐实例化注册表用法和熔断文案提示
-  提供历史清理脚本并完善 JSONL 保留说明

## [0.1.4] - 2025-07-23

### 新增
- 升级到 Go 1.23
- 任务执行统计与监控（Monitor）
- 智能调度器
- 性能基准测试

### 变更
- 改进 cron 表达式解析缓存机制
- 增强并发控制和安全性

### 修复
- 修复时区处理问题
- 解决并发执行时可能的死锁问题
- 修复长时间运行后的内存泄漏问题
- 改进任务执行时间计算的准确性

### 安全性
- 增强输入验证和参数检查
- 改进错误处理和异常捕获

## [1.2.1] - 2025-06-15

### 修复
-  修复在高并发情况下任务重复执行的问题
- ⏰ 修复夏令时切换时任务调度异常的问题
-  修复日志输出格式不一致的问题

### 安全性
-  修复潜在的竞态条件安全问题

## [1.2.0] - 2025-05-20

### 新增
- ⏰ 支持秒级cron表达式
-  添加基础的任务执行统计功能
-  支持自定义日志记录器
- ️ 添加任务执行超时配置

### 变更
-  改进错误信息的可读性
-  优化任务调度算法性能

### 修复
-  修复任务名称重复时的处理逻辑
- ⏰ 修复特殊cron表达式解析错误

## [1.1.0] - 2025-04-10

### 新增
-  支持异步任务执行
-  添加任务执行状态查询
- ️ 支持任务执行配置选项
-  完善API文档和使用示例

### 变更
-  改进任务调度器的稳定性
-  优化依赖包管理

### 修复
-  修复任务停止时的资源清理问题
- ⏰ 修复时区设置不生效的问题

## [1.0.0] - 2025-03-01

### 新增
-  首次发布
- ⏰ 基础cron表达式支持
-  任务注册和管理功能
-  调度器启动和停止功能
-  基础文档和示例

### 特性
-  支持标准cron表达式（分钟级别）
-  并发安全的任务管理
-  简洁的API设计
-  完整的单元测试覆盖

---

## 版本说明

### 版本号格式
版本号格式为 `主版本.次版本.修订版本`，例如 `2.1.0`

- **主版本**：不兼容的API变更
- **次版本**：向后兼容的功能新增
- **修订版本**：向后兼容的问题修复

### 变更类型说明

#### 新增 (Added)
新增的功能特性

#### 变更 (Changed)
现有功能的变更

#### 废弃 (Deprecated)
即将在未来版本中移除的功能

#### 移除 (Removed)
在此版本中移除的功能

#### 修复 (Fixed)
修复的Bug

#### 安全性 (Security)
安全相关的修复和改进

### 兼容性说明

- 次版本和修订版本保证向后兼容
- 主版本可能包含破坏性变更，变更日志中会明确标注

---

更多信息请参考：
- [问题反馈](https://github.com/darkit/cron/issues)