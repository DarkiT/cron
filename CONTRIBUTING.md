# 贡献指南

##  欢迎贡献

感谢您对本项目的关注！我们欢迎各种形式的贡献，包括但不限于：

-  报告Bug
-  提出新功能建议
-  改进文档
-  提交代码修复
-  性能优化
-  添加测试用例

##  贡献流程

### 1. 准备工作

#### 环境要求
- Go 1.23+
- Git
- 代码编辑器（推荐VS Code）

#### 开发环境设置
```bash
# 1. Fork项目到您的GitHub账户

# 2. 克隆您的Fork
git clone https://github.com/YOUR_USERNAME/PROJECT_NAME.git
cd PROJECT_NAME

# 3. 添加上游仓库
git remote add upstream https://github.com/ORIGINAL_OWNER/PROJECT_NAME.git

# 4. 安装依赖
go mod tidy

# 5. 运行测试确保环境正常
go test ./...
```

### 2. 开发流程

#### 创建功能分支
```bash
# 1. 同步最新代码
git checkout main
git pull upstream main

# 2. 创建功能分支
git checkout -b feature/your-feature-name
# 或者修复分支
git checkout -b fix/issue-number
```

#### 开发规范
- 遵循[技术选型和架构](docs/技术选型和架构.md)中的约定
- 使用中文编写注释和文档
- 确保代码通过所有测试
- 添加必要的测试用例
- 更新相关文档

#### 提交代码
```bash
# 1. 添加修改的文件
git add .

# 2. 提交代码（使用中文提交信息）
git commit -m "feat: 添加新功能描述"

# 3. 推送到您的Fork
git push origin feature/your-feature-name
```

### 3. 提交Pull Request

#### PR准备清单
- [ ] 代码已经过自测
- [ ] 所有测试用例通过
- [ ] 代码符合项目规范
- [ ] 添加了必要的测试
- [ ] 更新了相关文档
- [ ] 提交信息清晰明确

#### PR模板
```markdown
##  变更描述
[简要描述本次变更的内容和目的]

##  变更类型
- [ ] 新功能 (feature)
- [ ] 修复Bug (fix)
- [ ] 文档更新 (docs)
- [ ] 代码重构 (refactor)
- [ ] 性能优化 (perf)
- [ ] 测试相关 (test)
- [ ] 构建相关 (build)

##  测试
- [ ] 单元测试通过
- [ ] 集成测试通过
- [ ] 手动测试完成

##  相关Issue
关闭 #[issue编号]

##  截图（如适用）
[添加截图或GIF展示变更效果]

##  检查清单
- [ ] 代码遵循项目规范
- [ ] 添加了必要的测试
- [ ] 更新了相关文档
- [ ] 提交信息符合规范
```

##  Bug报告

### Bug报告模板
```markdown
##  Bug描述
[清晰简洁地描述遇到的问题]

##  重现步骤
1. [步骤1]
2. [步骤2]
3. [步骤3]

##  期望行为
[描述您期望发生的行为]

##  实际行为
[描述实际发生的行为]

## ️ 环境信息
- 操作系统: [例如 Ubuntu 20.04]
- Go版本: [例如 go1.23.0]
- 项目版本: [例如 v2.0.0]

##  额外信息
[添加任何其他有助于解决问题的信息]

##  日志和截图
[粘贴相关日志或添加截图]
```

### Bug报告指南
- 使用清晰的标题描述问题
- 提供详细的重现步骤
- 包含完整的错误信息和日志
- 说明期望的行为和实际行为
- 提供环境信息和版本号

##  功能建议

### 功能建议模板
```markdown
##  功能描述
[清晰简洁地描述建议的功能]

##  问题背景
[描述这个功能要解决的问题]

##  建议的解决方案
[详细描述您建议的实现方式]

##  替代方案
[描述您考虑过的其他解决方案]

##  附加信息
[添加任何其他相关信息，如截图、参考链接等]
```

### 功能建议指南
- 清楚地说明功能的价值和必要性
- 考虑功能的实现复杂度和维护成本
- 提供具体的使用场景和示例
- 考虑对现有功能的影响

##  代码规范

### 1. Go代码规范
```go
// 包文档使用中文
// Package example 提供了示例功能的实现。
//
// 主要特性：
//   - 功能1：描述
//   - 功能2：描述
package example

import (
    "context"
    "fmt"
    "time"
)

// 常量定义
const (
    DefaultTimeout = 30 * time.Second
    MaxRetries     = 3
)

// 错误定义
var (
    ErrInvalidInput = errors.New("输入参数无效")
    ErrTimeout      = errors.New("操作超时")
)

// Service 定义了服务接口。
//
// 所有方法都是并发安全的。
type Service interface {
    // Process 处理输入数据并返回结果。
    //
    // 参数：
    //   ctx - 上下文，用于取消操作
    //   input - 输入数据
    //
    // 返回值：
    //   *Result - 处理结果
    //   error - 错误信息
    Process(ctx context.Context, input *Input) (*Result, error)
}

// service 是Service接口的实现。
type service struct {
    timeout time.Duration
    logger  Logger
}

// NewService 创建新的服务实例。
func NewService(opts ...Option) Service {
    s := &service{
        timeout: DefaultTimeout,
    }
    
    for _, opt := range opts {
        opt(s)
    }
    
    return s
}

// Process 实现Service接口。
func (s *service) Process(ctx context.Context, input *Input) (*Result, error) {
    if input == nil {
        return nil, ErrInvalidInput
    }
    
    // 设置超时
    ctx, cancel := context.WithTimeout(ctx, s.timeout)
    defer cancel()
    
    // 处理逻辑
    result, err := s.processInternal(ctx, input)
    if err != nil {
        s.logger.Error("处理失败", "error", err)
        return nil, fmt.Errorf("处理输入失败: %w", err)
    }
    
    return result, nil
}
```

### 2. 测试代码规范
```go
func TestService_Process(t *testing.T) {
    tests := []struct {
        name    string
        input   *Input
        want    *Result
        wantErr bool
    }{
        {
            name: "正常情况",
            input: &Input{
                Data: "test data",
            },
            want: &Result{
                Status: "success",
            },
            wantErr: false,
        },
        {
            name:    "输入为空",
            input:   nil,
            want:    nil,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            s := NewService()
            got, err := s.Process(context.Background(), tt.input)
            
            if (err != nil) != tt.wantErr {
                t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Process() = %v, want %v", got, tt.want)
            }
        })
    }
}

func BenchmarkService_Process(b *testing.B) {
    s := NewService()
    input := &Input{Data: "benchmark data"}
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := s.Process(context.Background(), input)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### 3. 提交信息规范
```
<类型>: <简短描述>

<详细描述>

<相关Issue>
```

#### 提交类型
- `feat`: 新功能
- `fix`: 修复Bug
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 代码重构
- `perf`: 性能优化
- `test`: 测试相关
- `build`: 构建相关
- `ci`: CI/CD相关
- `chore`: 其他杂项

#### 示例
```
feat: 添加任务依赖功能

- 实现任务间的依赖关系管理
- 支持依赖超时和失败处理
- 添加循环依赖检测

关闭 #123
```

##  测试指南

### 1. 测试要求
- 新功能必须包含单元测试
- 测试覆盖率不低于80%
- 关键路径需要集成测试
- 性能敏感代码需要基准测试

### 2. 运行测试
```bash
# 运行所有测试
go test ./...

# 运行测试并显示覆盖率
go test -cover ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 运行基准测试
go test -bench=. -benchmem ./...

# 运行竞态检测
go test -race ./...
```

### 3. 测试最佳实践
- 使用表驱动测试
- 测试名称使用中文描述
- 测试数据要有代表性
- 使用Mock隔离外部依赖
- 测试要快速且稳定

##  文档贡献

### 1. 文档类型
- API文档：接口说明和使用示例
- 用户指南：功能介绍和使用教程
- 开发文档：架构设计和开发指南
- 故障排除：常见问题和解决方案

### 2. 文档规范
- 使用中文编写
- 结构清晰，层次分明
- 包含代码示例
- 及时更新，保持准确

### 3. 文档工具
- Markdown格式
- Mermaid图表
- 代码高亮
- 链接检查

## ️ 贡献者认可

### 贡献类型
-  Bug修复
-  新功能
-  文档改进
-  测试增强
-  性能优化
-  代码重构
-  安全改进

### 认可方式
- 贡献者列表
- 发布说明致谢
- 社区推荐
- 技术分享机会

##  获取帮助

### 沟通渠道
- **GitHub Issues**: 报告问题和功能建议
- **GitHub Discussions**: 技术讨论和问答
- **邮件**: [maintainer@example.com]
- **微信群**: [群二维码]

### 响应时间
- Bug报告：24小时内响应
- 功能建议：48小时内响应
- Pull Request：72小时内审查
- 一般问题：工作日内响应

##  贡献者协议

通过向本项目提交代码，您同意：

1. 您拥有所提交代码的版权
2. 您同意将代码以项目许可证发布
3. 您的贡献不侵犯第三方权利
4. 您同意遵守项目的行为准则

##  致谢

感谢所有为本项目做出贡献的开发者！您的每一个贡献都让项目变得更好。

### 主要贡献者
- [@contributor1](https://github.com/contributor1) - 核心功能开发
- [@contributor2](https://github.com/contributor2) - 文档完善
- [@contributor3](https://github.com/contributor3) - 测试增强

### 特别感谢
- 所有提交Issue的用户
- 参与讨论的社区成员
- 提供反馈的早期用户

---

再次感谢您的贡献！让我们一起构建更好的Go项目！ 
