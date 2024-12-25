// Package crontab 提供了一个简单而强大的定时任务管理库
//
// 基本用法:
//
//	scheduler := crontab.New()
//
//	// 添加一个简单的定时任务
//	scheduler.AddFunc("print-time", "*/5 * * * * *", func() {
//	    fmt.Println("当前时间:", time.Now())
//	})
//
//	// 启动调度器
//	scheduler.Start()
//
// 使用配置选项:
//
//	scheduler := crontab.New(
//	    crontab.WithPanicHandler(&MyPanicHandler{}),
//	)
//
// 实现 CronJob 接口:
//
//	type MyJob struct {
//	    name string
//	}
//
//	func (j *MyJob) Name() string    { return j.name }
//	func (j *MyJob) Rule() string    { return "*/5 * * * * *" }
//	func (j *MyJob) Execute() func() { return func() { ... } }
//
//	scheduler.AddJobInterface(&MyJob{name: "my-job"})
//
// Cron 表达式格式:
//
//	秒 分 时 日 月 周
//
//	*/5 * * * * *    // 每5秒执行一次
//	0 */1 * * * *    // 每分钟执行一次
//	0 0 * * * *      // 每小时执行一次
//	0 0 0 * * *      // 每天零点执行一次
//
// 异步任务:
//
//	scheduler.AddJob(crontab.JobConfig{
//	    Name:     "async-job",
//	    Schedule: "*/5 * * * * *",
//	    Async:    true,
//	}, myJob)
//
// 异常捕获:
//
//	type MyPanicHandler struct{}
//
//	func (h *MyPanicHandler) Handle(jobName string, err interface{}) {
//	    log.Printf("任务 [%s] 发生异常: %v", jobName, err)
//	}
//
//	scheduler.AddJob(crontab.JobConfig{
//	    Name:     "safe-job",
//	    Schedule: "*/5 * * * * *",
//	    TryCatch: true,
//	}, myJob)
//
// 动态管理任务:
//
//	// 更新任务
//	scheduler.UpdateJob(crontab.JobConfig{
//	    Name:     "my-job",
//	    Schedule: "*/10 * * * * *",
//	}, newJob)
//
//	// 停止任务
//	scheduler.StopService("my-job")
//
//	// 停止前缀匹配的任务
//	scheduler.StopServicePrefix("test-")
//
//	// 获取下次执行时间
//	nextTime, err := scheduler.NextRuntime("my-job")
//
// 并发控制示例:
//
//	scheduler.AddJob(crontab.JobConfig{
//	    Name:          "concurrent-job",
//	    Schedule:      "*/5 * * * * *",
//	    Async:         true,
//	    MaxConcurrent: 3,
//	    Timeout:       time.Second * 30,
//	}, myJob)
//
// 优雅退出:
//
//	// 停止所有任务
//	scheduler.Stop()
//
//	// 等待所有任务完成
//	scheduler.Wait()
package cron
