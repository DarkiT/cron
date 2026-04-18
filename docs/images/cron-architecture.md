# Cron调度库架构图

```mermaid
graph TB
    subgraph "用户层 User Layer"
        UA[用户应用 User App]
        API[API调用 API Calls]
    end
    
    subgraph "接口层 Interface Layer"
        QA[Quick API<br/>快速接口]
        CA[Chain API<br/>链式接口]
        BA[Basic API<br/>基础接口]
        IA[Interface API<br/>接口模式]
    end
    
    subgraph "核心层 Core Layer"
        CR[Cron调度器<br/>Cron Scheduler]
        JB[任务构建器<br/>Job Builder]
        SC[智能调度器<br/>Smart Scheduler]
    end
    
    subgraph "功能层 Feature Layer"
        subgraph "任务管理 Task Management"
            TM[任务管理器<br/>Task Manager]
            TC[任务配置<br/>Task Config]
            TI[任务接口<br/>Task Interface]
        end
        
        subgraph "执行引擎 Execution Engine"
            SE[调度引擎<br/>Scheduler Engine]
            TR[任务运行器<br/>Task Runner]
            EE[执行引擎<br/>Execution Engine]
        end
        
        subgraph "增强功能 Enhanced Features"
            CE[条件执行<br/>Condition Execution]
            TD[任务依赖<br/>Task Dependency]
            DM[依赖管理<br/>Dependency Manager]
        end
        
        subgraph "监控统计 Monitoring"
            MO[任务监控<br/>Task Monitor]
            ST[统计收集<br/>Statistics]
            HC[健康检查<br/>Health Check]
        end
    end
    
    subgraph "解析层 Parser Layer"
        CP[Cron解析器<br/>Cron Parser]
        CC[缓存管理<br/>Cache Manager]
        SS[调度规范<br/>Schedule Spec]
    end
    
    subgraph "基础设施层 Infrastructure Layer"
        LO[日志系统<br/>Logger]
        PH[异常处理<br/>Panic Handler]
        CT[上下文管理<br/>Context]
        GO[Go 1.23特性<br/>Go 1.23 Features]
    end
    
    %% 用户层到接口层
    UA --> API
    API --> QA
    API --> CA
    API --> BA
    API --> IA
    
    %% 接口层到核心层
    QA --> JB
    CA --> JB
    BA --> CR
    IA --> CR
    JB --> CR
    CR --> SC
    
    %% 核心层到功能层
    CR --> TM
    CR --> SE
    SC --> MO
    SC --> HC
    
    %% 功能层内部连接
    TM --> TC
    TM --> TI
    SE --> TR
    TR --> EE
    CE --> EE
    TD --> DM
    DM --> EE
    MO --> ST
    
    %% 功能层到解析层
    TM --> CP
    SE --> CP
    CP --> CC
    CP --> SS
    
    %% 基础设施层支持
    EE --> LO
    EE --> PH
    EE --> CT
    TR --> GO
    MO --> GO
    
    %% 样式定义
    classDef userLayer fill:#e1f5fe
    classDef interfaceLayer fill:#f3e5f5
    classDef coreLayer fill:#e8f5e8
    classDef featureLayer fill:#fff3e0
    classDef parserLayer fill:#fce4ec
    classDef infraLayer fill:#f1f8e9
    
    class UA,API userLayer
    class QA,CA,BA,IA interfaceLayer
    class CR,JB,SC coreLayer
    class TM,TC,TI,SE,TR,EE,CE,TD,DM,MO,ST,HC featureLayer
    class CP,CC,SS parserLayer
    class LO,PH,CT,GO infraLayer
```

## 架构说明

### 🏗️ 分层架构设计

#### 1. 用户层 (User Layer)
- **用户应用**: 最终用户的业务应用
- **API调用**: 通过各种API接口调用调度功能

#### 2. 接口层 (Interface Layer)
- **Quick API**: 快速创建简单任务的便捷接口
- **Chain API**: 链式调用接口，支持流畅的配置
- **Basic API**: 传统的基础接口，保持向后兼容
- **Interface API**: 支持接口模式的任务定义

#### 3. 核心层 (Core Layer)
- **Cron调度器**: 核心调度逻辑
- **任务构建器**: 支持链式调用的任务构建
- **智能调度器**: 具备自适应优化能力的高级调度器

#### 4. 功能层 (Feature Layer)
- **任务管理**: 任务的增删改查和配置管理
- **执行引擎**: 任务的实际执行和调度
- **增强功能**: 条件执行、任务依赖等高级特性
- **监控统计**: 实时监控和统计分析

#### 5. 解析层 (Parser Layer)
- **Cron解析器**: 解析各种cron表达式格式
- **缓存管理**: 解析结果缓存，提升性能
- **调度规范**: 标准化的调度规范定义

#### 6. 基础设施层 (Infrastructure Layer)
- **日志系统**: 统一的日志记录
- **异常处理**: 全局异常捕获和处理
- **上下文管理**: Context支持和管理
- **Go 1.23特性**: 充分利用Go 1.23新特性

### 🔄 数据流向

1. **用户请求** → 接口层 → 核心层
2. **任务配置** → 功能层 → 解析层
3. **任务执行** → 执行引擎 → 基础设施层
4. **监控数据** → 监控统计 → 用户反馈

### 🎯 设计优势

- **分层清晰**: 职责分离，易于维护
- **高内聚低耦合**: 模块间依赖关系清晰
- **可扩展性**: 新功能可以轻松集成
- **向后兼容**: 保持API稳定性
- **性能优化**: 缓存和Go 1.23特性优化