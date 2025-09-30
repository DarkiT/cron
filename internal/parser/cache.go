package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 预定义错误
var (
	ErrInvalidSpec = ErrUnsupportedSpec // 兼容原有错误
)

// 缓存大小限制，避免内存无限增长
var maxCacheSize = 1000

// parserCache 提供了一个线程安全的表达式解析结果缓存
// 使用 LRU (最近最少使用) 算法管理缓存
type parserCache struct {
	cache       map[string]Schedule // 表达式到解析结果的映射
	parserType  Parser              // 解析器类型
	accessOrder []string            // 访问顺序，用于LRU淘汰
	mu          sync.RWMutex        // 读写锁，保护缓存
}

// 全局缓存实例，按解析器选项类型分别缓存
var (
	parseCaches     = make(map[ParseOption]*parserCache)
	parseCachesLock sync.RWMutex
)

// getCacheForParser 获取或创建特定解析器类型的缓存
func getCacheForParser(p Parser) *parserCache {
	parseCachesLock.RLock()
	cache, exists := parseCaches[p.options]
	parseCachesLock.RUnlock()

	if !exists {
		parseCachesLock.Lock()
		// 双重检查，避免竞态条件
		if cache, exists = parseCaches[p.options]; !exists {
			cache = &parserCache{
				cache:       make(map[string]Schedule),
				parserType:  p,
				accessOrder: make([]string, 0, maxCacheSize),
			}
			parseCaches[p.options] = cache
		}
		parseCachesLock.Unlock()
	}

	return cache
}

// parseWithCache 尝试从缓存中获取解析结果，如果不存在则解析并缓存
func parseWithCache(p Parser, spec string) (Schedule, error) {
	// 获取该解析器的缓存
	cache := getCacheForParser(p)

	// 尝试从缓存中读取
	cache.mu.RLock()
	if schedule, found := cache.cache[spec]; found {
		// 更新访问记录（需要升级为写锁）
		cache.mu.RUnlock()

		// 获取写锁并更新访问顺序
		cache.mu.Lock()
		// 再次检查，因为可能在获取写锁期间已被其他协程修改
		if _, stillExists := cache.cache[spec]; stillExists {
			// 将此项移到访问顺序的末尾（最新访问）
			cache.updateAccessOrder(spec)
		}
		cache.mu.Unlock()

		return schedule, nil
	}
	cache.mu.RUnlock()

	// 缓存未命中，解析表达式
	schedule, err := p.parseNoCache(spec)
	if err != nil {
		return nil, err
	}

	// 将结果添加到缓存
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// 检查缓存是否已满
	if len(cache.cache) >= maxCacheSize {
		// 移除最久未访问的项
		oldest := cache.accessOrder[0]
		delete(cache.cache, oldest)
		cache.accessOrder = cache.accessOrder[1:]
	}

	// 添加新项到缓存
	cache.cache[spec] = schedule
	cache.accessOrder = append(cache.accessOrder, spec)

	return schedule, nil
}

// updateAccessOrder 更新访问顺序，将指定项移到访问顺序的末尾
// 注意：调用前必须获取写锁
func (pc *parserCache) updateAccessOrder(spec string) {
	// 查找当前位置
	var pos int
	for i, s := range pc.accessOrder {
		if s == spec {
			pos = i
			break
		}
	}

	// 从访问顺序中移除
	pc.accessOrder = append(pc.accessOrder[:pos], pc.accessOrder[pos+1:]...)
	// 添加到末尾（最近访问）
	pc.accessOrder = append(pc.accessOrder, spec)
}

// 保留原始解析方法，用于缓存未命中时
func (p Parser) parseNoCache(spec string) (Schedule, error) {
	trimmed := strings.TrimSpace(spec)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty spec string")
	}

	loc := time.Local

	// 支持 TZ=/CRON_TZ= 前缀设置时区
	if strings.HasPrefix(trimmed, "TZ=") || strings.HasPrefix(trimmed, "CRON_TZ=") {
		var (
			prefix string
			rest   string
		)

		if fields := strings.Fields(trimmed); len(fields) >= 2 {
			prefix = fields[0]
			rest = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		} else {
			return nil, fmt.Errorf("invalid timezone spec: %s", spec)
		}

		if rest == "" {
			return nil, fmt.Errorf("missing cron expression after timezone prefix: %s", spec)
		}

		tzName := strings.TrimPrefix(prefix, "TZ=")
		tzName = strings.TrimPrefix(tzName, "CRON_TZ=")

		location, err := time.LoadLocation(tzName)
		if err != nil {
			return nil, fmt.Errorf("failed to load timezone %s: %w", tzName, err)
		}

		loc = location
		trimmed = rest
	}

	// 支持描述符语法（需要Descriptor选项）
	if strings.HasPrefix(trimmed, "@") {
		if p.options&Descriptor == 0 {
			return nil, fmt.Errorf("descriptor syntax not supported without Descriptor option: %s", trimmed)
		}
		return p.parseDescriptor(trimmed, loc)
	}

	// 使用通用的cron字段解析方法
	return p.parseCronFields(trimmed, loc)
}

// parseDescriptor 解析描述符语法，如 @every, @daily, @weekly, @monthly 等
func (p Parser) parseDescriptor(spec string, loc *time.Location) (Schedule, error) {
	var cronSpec string

	// 根据解析器选项确定字段格式
	if p.options&Second > 0 {
		// 6字段格式（包含秒）
		switch spec {
		case "@yearly", "@annually":
			cronSpec = "0 0 0 1 1 *"
		case "@monthly":
			cronSpec = "0 0 0 1 * *"
		case "@weekly":
			cronSpec = "0 0 0 * * 0"
		case "@daily", "@midnight":
			cronSpec = "0 0 0 * * *"
		case "@hourly":
			cronSpec = "0 0 * * * *"
		}
	} else {
		// 5字段格式（不包含秒）
		switch spec {
		case "@yearly", "@annually":
			cronSpec = "0 0 1 1 *"
		case "@monthly":
			cronSpec = "0 0 1 * *"
		case "@weekly":
			cronSpec = "0 0 * * 0"
		case "@daily", "@midnight":
			cronSpec = "0 0 * * *"
		case "@hourly":
			cronSpec = "0 * * * *"
		}
	}

	if cronSpec != "" {
		return p.parseCronFields(cronSpec, loc)
	}

	// 处理 @every 语法
	if strings.HasPrefix(spec, "@every ") {
		return p.parseEvery(spec[7:], loc)
	}

	return nil, fmt.Errorf("unrecognized descriptor: %s", spec)
}

// parseCronFields 解析标准的cron字段，不处理描述符语法
func (p Parser) parseCronFields(spec string, loc *time.Location) (Schedule, error) {
	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty spec string")
	}

	fields, err := normalizeFields(fields, p.options)
	if err != nil {
		return nil, err
	}

	var (
		second     uint64
		minute     uint64
		hour       uint64
		dayofmonth uint64
		month      uint64
		dayofweek  uint64
	)

	// 此时 fields 应该是已经由 normalizeFields 处理过的6个字段
	// 直接按照 places 顺序解析每个字段
	for idx, place := range places {
		if idx >= len(fields) {
			return nil, fmt.Errorf("field index out of range: %d", idx)
		}

		fieldSpec := fields[idx]
		var fieldValue uint64
		switch place {
		case Second:
			if fieldValue, err = getField(fieldSpec, seconds); err != nil {
				return nil, fmt.Errorf("failed to parse second field: %s", err)
			}
			second = fieldValue
		case Minute:
			if fieldValue, err = getField(fieldSpec, minutes); err != nil {
				return nil, fmt.Errorf("failed to parse minute field: %s", err)
			}
			minute = fieldValue
		case Hour:
			if fieldValue, err = getField(fieldSpec, hours); err != nil {
				return nil, fmt.Errorf("failed to parse hour field: %s", err)
			}
			hour = fieldValue
		case Dom:
			if fieldValue, err = getField(fieldSpec, dom); err != nil {
				return nil, fmt.Errorf("failed to parse day-of-month field: %s", err)
			}
			dayofmonth = fieldValue
		case Month:
			if fieldValue, err = getField(fieldSpec, months); err != nil {
				return nil, fmt.Errorf("failed to parse month field: %s", err)
			}
			month = fieldValue
		case Dow:
			if fieldValue, err = getField(fieldSpec, dow); err != nil {
				return nil, fmt.Errorf("failed to parse day-of-week field: %s", err)
			}
			dayofweek = fieldValue
		}
	}

	return &SpecSchedule{
		Second:   second,
		Minute:   minute,
		Hour:     hour,
		Dom:      dayofmonth,
		Month:    month,
		Dow:      dayofweek,
		Location: loc,
	}, nil
}

// parseEvery 解析 @every 语法，如 @every 1h30m
func (p Parser) parseEvery(spec string, loc *time.Location) (Schedule, error) {
	duration, err := parseDuration(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse duration %s: %s", spec, err)
	}

	if duration <= 0 {
		return nil, fmt.Errorf("non-positive duration %s", duration)
	}

	return &ConstantDelaySchedule{
		Delay:    duration,
		Location: loc,
	}, nil
}

// parseDuration 解析持续时间，支持更多格式
func parseDuration(spec string) (time.Duration, error) {
	// 首先尝试标准的时间格式
	if duration, err := time.ParseDuration(spec); err == nil {
		return duration, nil
	}

	// 支持数字+单位的格式，如 "5s", "30m", "1h"
	re := regexp.MustCompile(`^(\d+)([smhd])$`)
	matches := re.FindStringSubmatch(spec)
	if len(matches) == 3 {
		val, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, err
		}

		switch matches[2] {
		case "s":
			return time.Duration(val) * time.Second, nil
		case "m":
			return time.Duration(val) * time.Minute, nil
		case "h":
			return time.Duration(val) * time.Hour, nil
		case "d":
			return time.Duration(val) * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf("invalid duration format: %s", spec)
}

// ConstantDelaySchedule 表示固定延迟的调度
type ConstantDelaySchedule struct {
	Delay    time.Duration
	Location *time.Location
}

// Next 返回下一个执行时间
func (schedule *ConstantDelaySchedule) Next(t time.Time) time.Time {
	if schedule.Location != nil {
		t = t.In(schedule.Location)
	}
	return t.Add(schedule.Delay)
}
