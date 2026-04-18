# Cron调度库时序图

## 1. 任务创建和启动时序图

```mermaid
sequenceDiagram
    participant U as 用户应用
    participant C as Cron调度器
    participant JB as JobBuilder
    participant TM as TaskManager
    participant SE as SchedulerEngine
    participant CP as CronParser
    participant MO as Monitor
    
    Note over U,MO: 任务创建阶段
    U->>C: cron.New()
    C->>MO: 初始化监控器
    C->>SE: 初始化调度引擎
    
    U->>C: NewJob("task", "*/5 * * * * *")
    C->>JB: 创建JobBuilder
    JB-->>U: 返回JobBuilder实例
    
    U->>JB: Async().WithTimeout(30s)
    JB->>JB: 配置任务属性
    
    U->>JB: Do(taskFunc)
    JB->>C: AddJob(config, taskFunc)
    C->>TM: 添加任务到管理器
    TM->>CP: 解析cron表达式
    CP-->>TM: 返回Schedule对象
    TM->>MO: 注册任务监控
    TM-->>C: 任务添加成功
    C-->>U: 返回成功
    
    Note over U,MO: 调度器启动阶段
    U->>C: Start()
    C->>SE: 启动调度引擎
    SE->>SE: 创建任务运行器
    SE->>SE: 启动调度循环
    C-->>U: 启动成功
```

## 2. 任务执行时序图

```mermaid
sequenceDiagram
    participant SE as SchedulerEngine
    participant TR as TaskRunner
    participant EE as ExecutionEngine
    participant MO as Monitor
    participant DM as DependencyManager
    participant CE as ConditionEngine
    participant TF as TaskFunction
    participant LO as Logger
    
    Note over SE,LO: 任务调度执行流程
    
    loop 调度循环
        SE->>TR: 检查任务执行时间
        TR->>TR: 计算下次执行时间
        
        alt 到达执行时间
            TR->>MO: 记录任务开始执行
            MO->>MO: 设置运行状态
            
            alt 有依赖任务
                TR->>DM: 检查依赖任务状态
                DM-->>TR: 依赖检查结果
                
                alt 依赖未满足
                    TR->>LO: 记录跳过日志
                    TR->>MO: 记录跳过统计
                else 依赖满足
                    TR->>CE: 检查执行条件
                end
            else 无依赖
                TR->>CE: 检查执行条件
            end
            
            CE->>CE: 评估所有条件
            
            alt 条件满足
                CE->>EE: 执行任务
                
                alt 异步任务
                    EE->>EE: 创建goroutine
                    par 并行执行
                        EE->>TF: 调用任务函数
                        TF-->>EE: 任务执行完成
                    and 超时控制
                        EE->>EE: 监控执行时间
                        alt 超时
                            EE->>EE: 取消任务执行
                            EE->>LO: 记录超时日志
                        end
                    end
                else 同步任务
                    EE->>TF: 直接调用任务函数
                    TF-->>EE: 任务执行完成
                end
                
                EE->>MO: 记录执行结果
                EE->>DM: 通知任务完成
                
            else 条件不满足
                CE->>LO: 记录条件不满足
                CE->>MO: 记录跳过统计
            end
            
            MO->>MO: 更新任务统计
            MO->>MO: 重置运行状态
        end
    end
```

## 3. 智能调度器优化时序图

```mermaid
sequenceDiagram
    participant SS as SmartScheduler
    participant HC as HealthChecker
    participant OP as Optimizer
    participant MO as Monitor
    participant SE as SchedulerEngine
    participant LO as Logger
    
    Note over SS,LO: 智能调度优化流程
    
    SS->>HC: 启动健康监控
    
    loop 健康检查循环
        HC->>MO: 获取全局统计
        MO-->>HC: 返回统计数据
        
        HC->>HC: 分析系统健康状态
        
        alt 系统状态异常
            HC->>LO: 记录健康警告
            HC->>OP: 触发优化流程
            
            OP->>MO: 获取任务执行统计
            MO-->>OP: 返回详细统计
            
            OP->>OP: 分析任务执行模式
            OP->>OP: 计算优化策略
            
            alt 需要负载均衡
                OP->>SE: 调整任务调度策略
                SE->>SE: 重新分配任务时间
            end
            
            alt 需要并发控制
                OP->>SE: 调整并发限制
                SE->>SE: 更新并发配置
            end
            
            OP->>LO: 记录优化操作
            
        else 系统状态正常
            HC->>LO: 记录健康状态
        end
        
        HC->>HC: 等待下次检查间隔
    end
```

## 4. 任务依赖执行时序图

```mermaid
sequenceDiagram
    participant U as 用户应用
    participant C as Cron调度器
    participant DM as DependencyManager
    participant T1 as Task1(主任务)
    participant T2 as Task2(依赖任务)
    participant MO as Monitor
    
    Note over U,MO: 任务依赖执行流程
    
    U->>C: 添加主任务Task1
    C->>DM: 注册Task1
    
    U->>C: 添加依赖任务Task2
    Note right of U: Task2依赖Task1完成
    C->>DM: 注册Task2及其依赖关系
    
    U->>C: Start()
    
    Note over T1,MO: 主任务执行
    C->>T1: 触发Task1执行
    T1->>T1: 执行任务逻辑
    T1->>DM: 通知任务完成(成功)
    DM->>MO: 记录Task1完成状态
    
    Note over T2,MO: 依赖任务执行
    C->>T2: 触发Task2执行
    T2->>DM: 检查Task1是否完成
    DM-->>T2: Task1已完成且成功
    T2->>T2: 执行任务逻辑
    T2->>DM: 通知Task2完成
    DM->>MO: 记录Task2完成状态
    
    Note over U,MO: 依赖失败场景
    alt Task1执行失败
        T1->>DM: 通知任务完成(失败)
        DM->>MO: 记录Task1失败状态
        
        C->>T2: 触发Task2执行
        T2->>DM: 检查Task1是否完成
        DM-->>T2: Task1已完成但失败
        
        alt 配置了SkipOnDependencyFailure
            T2->>MO: 记录跳过执行
        else 未配置跳过
            T2->>T2: 继续执行任务
        end
    end
```

## 时序图说明

### 🔄 执行流程特点

1. **异步处理**: 支持异步任务执行，不阻塞调度器
2. **依赖管理**: 智能处理任务间的依赖关系
3. **条件检查**: 执行前进行条件验证
4. **监控统计**: 全程记录任务执行状态和统计信息
5. **智能优化**: 自动分析和优化调度策略

### ⚡ 性能优化点

- **并行执行**: 异步任务并行处理
- **缓存机制**: cron表达式解析结果缓存
- **智能调度**: 根据执行统计自动优化
- **资源控制**: 并发数限制和超时控制