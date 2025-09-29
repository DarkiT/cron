package parser

import (
	"sync"
	"testing"
	"time"
)

// TestCacheHit 测试缓存命中功能
func TestCacheHit(t *testing.T) {
	// 清空全局缓存，确保测试环境干净
	parseCachesLock.Lock()
	parseCaches = make(map[ParseOption]*parserCache)
	parseCachesLock.Unlock()

	// 使用标准解析器
	p := standardParser

	// 解析相同的表达式多次
	expr := "*/5 * * * *"

	// 第一次解析，应该缓存未命中
	sched1, err := p.Parse(expr)
	if err != nil {
		t.Fatalf("解析表达式失败: %v", err)
	}

	// 第二次解析，应该缓存命中
	sched2, err := p.Parse(expr)
	if err != nil {
		t.Fatalf("解析表达式失败: %v", err)
	}

	// 检查两次返回的调度器是否相同
	if sched1 != sched2 {
		t.Errorf("缓存未命中: sched1(%p) != sched2(%p)", sched1, sched2)
	}

	// 验证缓存内容
	cache := getCacheForParser(p)
	cache.mu.RLock()
	cachedSched, exists := cache.cache[expr]
	cache.mu.RUnlock()

	if !exists {
		t.Errorf("表达式 %q 未被缓存", expr)
	}

	if cachedSched != sched1 {
		t.Errorf("缓存的调度器与返回的不同: cached(%p) != returned(%p)", cachedSched, sched1)
	}
}

// TestCacheLRU 测试LRU淘汰机制
func TestCacheLRU(t *testing.T) {
	// 清空全局缓存，确保测试环境干净
	parseCachesLock.Lock()
	parseCaches = make(map[ParseOption]*parserCache)
	parseCachesLock.Unlock()

	// 使用标准解析器
	p := standardParser

	// 创建一个自定义的小容量缓存进行测试
	const testCacheSize = 5

	// 保存原始的maxCacheSize值
	origMaxCacheSize := maxCacheSize

	// 修改为测试用的小容量
	maxCacheSize = testCacheSize

	// 测试结束后恢复原值
	defer func() {
		maxCacheSize = origMaxCacheSize
	}()

	// 创建超过缓存容量的表达式
	exprs := []string{
		"*/5 * * * *",
		"0 */2 * * *",
		"0 0 * * *",
		"0 0 1 * *",
		"0 0 1 1 *",
		"30 15 * * *", // 这个应该会导致第一个被淘汰
	}

	// 解析所有表达式
	for _, expr := range exprs {
		_, err := p.Parse(expr)
		if err != nil {
			t.Fatalf("解析表达式失败: %v", err)
		}
	}

	// 验证缓存内容
	cache := getCacheForParser(p)
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	// 检查缓存大小
	if len(cache.cache) > testCacheSize {
		t.Errorf("缓存大小超过限制: %d > %d", len(cache.cache), testCacheSize)
	}

	// 检查第一个表达式是否被淘汰
	if _, exists := cache.cache[exprs[0]]; exists {
		t.Errorf("LRU未正常工作: 表达式 %q 应该被淘汰", exprs[0])
	}

	// 检查最后一个表达式是否被缓存
	if _, exists := cache.cache[exprs[len(exprs)-1]]; !exists {
		t.Errorf("最新的表达式 %q 未被缓存", exprs[len(exprs)-1])
	}
}

// TestCacheConcurrency 测试并发安全性
func TestCacheConcurrency(t *testing.T) {
	// 清空全局缓存，确保测试环境干净
	parseCachesLock.Lock()
	parseCaches = make(map[ParseOption]*parserCache)
	parseCachesLock.Unlock()

	// 使用标准解析器
	p := standardParser

	// 创建一些不同的表达式
	exprs := []string{
		"*/5 * * * *",
		"0 */2 * * *",
		"0 0 * * *",
		"0 0 1 * *",
		"0 0 1 1 *",
		"30 15 * * *",
	}

	// 并发解析
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 每个协程解析所有表达式多次
			for j := 0; j < 10; j++ {
				for _, expr := range exprs {
					_, err := p.Parse(expr)
					if err != nil {
						t.Errorf("协程 %d 解析表达式 %q 失败: %v", id, expr, err)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// 验证所有表达式都被正确缓存
	cache := getCacheForParser(p)
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	for _, expr := range exprs {
		if _, exists := cache.cache[expr]; !exists {
			t.Errorf("表达式 %q 未被缓存", expr)
		}
	}
}

// TestCachePerformance 测试缓存性能提升
func TestCachePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过性能测试")
	}

	// 清空全局缓存，确保测试环境干净
	parseCachesLock.Lock()
	parseCaches = make(map[ParseOption]*parserCache)
	parseCachesLock.Unlock()

	// 使用标准解析器
	p := standardParser

	// 创建一个复杂的表达式
	expr := "*/5 */10 */15 */20 *"

	// 预热缓存
	_, err := p.Parse(expr)
	if err != nil {
		t.Fatalf("解析表达式失败: %v", err)
	}

	// 测试有缓存的解析性能
	start := time.Now()
	iterations := 10000
	for i := 0; i < iterations; i++ {
		_, err := p.Parse(expr)
		if err != nil {
			t.Fatalf("解析表达式失败: %v", err)
		}
	}

	cacheDuration := time.Since(start)

	// 记录性能数据
	t.Logf("使用缓存解析 %d 次耗时: %v (每次 %v)", iterations, cacheDuration, cacheDuration/time.Duration(iterations))

	// 这里我们不测试无缓存的情况，因为那需要修改代码
	// 但在实际开发中，可以临时禁用缓存进行对比测试
}
