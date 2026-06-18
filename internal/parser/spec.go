package parser

import "slices"

import "time"

// SpecSchedule specifies a duty cycle (to the second granularity), based on a
// traditional crontab specification. It is computed initially and stored as bit sets.
type SpecSchedule struct {
	Second, Minute, Hour, Dom, Month, Dow uint64

	// Override location for this schedule（仅在显式指定 TZ 时使用）
	Location *time.Location

	// locationSet 表示是否显式指定了时区
	locationSet bool

	// L/W/# 语法支持（扩展字段）
	lastDayOfMonth         bool         // 月末最后一天（L）
	lastWorkdayOfMonth     bool         // 月末最后一个工作日（LW）
	workdaysOfMonth        map[int]bool // 特定日期的工作日（如 15W）
	lastWeekDaysOfWeek     map[int]bool // 每月最后一个星期X（如 5L = 最后一个星期五）
	specificWeekDaysOfWeek map[int]bool // 每月第N个星期X（如 5#3 = 第3个星期五）
	daysOfMonthRestricted  bool         // Dom 是否受限（不是 *）
	daysOfWeekRestricted   bool         // Dow 是否受限（不是 *）
}

// bounds provides a range of acceptable values (plus a map of name to value).
type bounds struct {
	min, max uint
	names    map[string]uint
}

// The bounds for each field.
var (
	seconds = bounds{0, 59, nil}
	minutes = bounds{0, 59, nil}
	hours   = bounds{0, 23, nil}
	dom     = bounds{1, 31, nil}
	months  = bounds{1, 12, map[string]uint{
		"jan": 1,
		"feb": 2,
		"mar": 3,
		"apr": 4,
		"may": 5,
		"jun": 6,
		"jul": 7,
		"aug": 8,
		"sep": 9,
		"oct": 10,
		"nov": 11,
		"dec": 12,
	}}
	dow = bounds{0, 6, map[string]uint{
		"sun": 0,
		"mon": 1,
		"tue": 2,
		"wed": 3,
		"thu": 4,
		"fri": 5,
		"sat": 6,
	}}
)

const (
	// Set the top bit if a star was included in the expression.
	starBit = 1 << 63
)

// Next returns the next time this schedule is activated, greater than the given
// time. If no time can be found to satisfy the schedule, return the zero time.
func (s *SpecSchedule) Next(t time.Time) time.Time {
	origLocation := t.Location()

	loc := s.Location
	if s.locationSet {
		if loc == nil {
			loc = time.Local
		}
	} else {
		if loc == nil || loc == time.Local {
			loc = t.Location()
		}
	}

	if s.locationSet && loc != nil {
		t = t.In(loc)
	}

	t = t.Add(1*time.Second - time.Duration(t.Nanosecond())*time.Nanosecond)

	added := false
	yearLimit := t.Year() + 5

WRAP:
	if t.Year() > yearLimit {
		return time.Time{}
	}

	for 1<<uint(t.Month())&s.Month == 0 {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 1, 0)
		if t.Month() == time.January {
			goto WRAP
		}
	}

	for !dayMatches(s, t) {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 0, 1)
		if t.Hour() != 0 {
			if t.Hour() > 12 {
				t = t.Add(time.Duration(24-t.Hour()) * time.Hour)
			} else {
				t = t.Add(time.Duration(-t.Hour()) * time.Hour)
			}
		}
		if t.Day() == 1 {
			goto WRAP
		}
	}

	for 1<<uint(t.Hour())&s.Hour == 0 {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
		}
		t = t.Add(1 * time.Hour)

		if t.Hour() == 0 {
			goto WRAP
		}
	}

	for 1<<uint(t.Minute())&s.Minute == 0 {
		if !added {
			added = true
			t = t.Truncate(time.Minute)
		}
		t = t.Add(1 * time.Minute)

		if t.Minute() == 0 {
			goto WRAP
		}
	}

	for 1<<uint(t.Second())&s.Second == 0 {
		if !added {
			added = true
			t = t.Truncate(time.Second)
		}
		t = t.Add(1 * time.Second)

		if t.Second() == 0 {
			goto WRAP
		}
	}

	return t.In(origLocation)
}

// dayMatches returns true if the schedule's day-of-week and day-of-month
// restrictions are satisfied by the given time.
func dayMatches(s *SpecSchedule, t time.Time) bool {
	// 如果使用了 L/W/# 语法，需要动态计算实际日期
	if s.hasSpecialDaySyntax() {
		actualDays := s.calculateActualDaysOfMonth(t.Year(), int(t.Month()))
		return slices.Contains(actualDays, t.Day())
	}

	// 标准的位图匹配逻辑（保持向后兼容）
	var (
		domMatch = 1<<uint(t.Day())&s.Dom > 0
		dowMatch = 1<<uint(t.Weekday())&s.Dow > 0
	)
	if s.Dom&starBit > 0 || s.Dow&starBit > 0 {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}

// hasSpecialDaySyntax 检查是否使用了 L/W/# 等特殊语法
func (s *SpecSchedule) hasSpecialDaySyntax() bool {
	return s.lastDayOfMonth || s.lastWorkdayOfMonth ||
		len(s.workdaysOfMonth) > 0 ||
		len(s.lastWeekDaysOfWeek) > 0 ||
		len(s.specificWeekDaysOfWeek) > 0
}

// calculateActualDaysOfMonth 根据 L/W/# 语法动态计算给定年月的实际日期列表
// 参考 supercronic/cronexpr 的实现
func (s *SpecSchedule) calculateActualDaysOfMonth(year, month int) []int {
	actualDaysOfMonthMap := make(map[int]bool)
	firstDayOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastDayOfMonth := firstDayOfMonth.AddDate(0, 1, -1)

	// 按照 crontab 规范：
	// "如果 day-of-month 和 day-of-week 都受限（不是 *），命令在任一字段匹配时运行"
	// 如果两个字段都不受限，则所有日期都匹配
	if !s.daysOfMonthRestricted && !s.daysOfWeekRestricted {
		days := make([]int, lastDayOfMonth.Day())
		for i := 1; i <= lastDayOfMonth.Day(); i++ {
			days[i-1] = i
		}
		return days
	}

	// 处理 day-of-month 字段（Dom）
	if s.daysOfMonthRestricted {
		// L - 月末最后一天
		if s.lastDayOfMonth {
			actualDaysOfMonthMap[lastDayOfMonth.Day()] = true
		}

		// LW - 月末最后一个工作日
		if s.lastWorkdayOfMonth {
			actualDaysOfMonthMap[workdayOfMonth(lastDayOfMonth, lastDayOfMonth)] = true
		}

		// 标准日期（位图）
		for day := 1; day <= lastDayOfMonth.Day(); day++ {
			if 1<<uint(day)&s.Dom > 0 {
				actualDaysOfMonthMap[day] = true
			}
		}

		// 15W - 特定日期的最近工作日
		for day := range s.workdaysOfMonth {
			if day <= lastDayOfMonth.Day() {
				targetDay := firstDayOfMonth.AddDate(0, 0, day-1)
				actualDaysOfMonthMap[workdayOfMonth(targetDay, lastDayOfMonth)] = true
			}
		}
	}

	// 处理 day-of-week 字段（Dow）
	if s.daysOfWeekRestricted {
		// 计算第一天是星期几的偏移量
		offset := 7 - int(firstDayOfMonth.Weekday())

		// 标准的星期几（位图）
		// 对于每个匹配的星期几，计算该月所有这样的日期
		for dow := 0; dow <= 6; dow++ {
			if 1<<uint(dow)&s.Dow > 0 {
				// 计算该星期几在这个月的所有日期
				// 第一次出现：1 + (offset + dow) % 7
				firstOccurrence := 1 + (offset+dow)%7
				for day := firstOccurrence; day <= lastDayOfMonth.Day(); day += 7 {
					actualDaysOfMonthMap[day] = true
				}
			}
		}

		// 5#3 - 每月第N个星期X
		// 编码格式：(week-1)*7 + dow
		for encoded := range s.specificWeekDaysOfWeek {
			week := encoded / 7
			dow := encoded % 7
			// 计算目标日期：1 + 7*week + (offset + dow) % 7
			day := 1 + 7*week + (offset+dow)%7
			if day <= lastDayOfMonth.Day() {
				actualDaysOfMonthMap[day] = true
			}
		}

		// 5L - 每月最后一个星期X
		// 从月末往前推一周，然后计算偏移
		lastWeekOrigin := firstDayOfMonth.AddDate(0, 1, -7)
		lastWeekOffset := 7 - int(lastWeekOrigin.Weekday())
		for dow := range s.lastWeekDaysOfWeek {
			day := lastWeekOrigin.Day() + (lastWeekOffset+dow)%7
			if day <= lastDayOfMonth.Day() {
				actualDaysOfMonthMap[day] = true
			}
		}
	}

	// 将 map 转换为排序的切片
	days := make([]int, 0, len(actualDaysOfMonthMap))
	for day := range actualDaysOfMonthMap {
		days = append(days, day)
	}
	// 简单排序（冒泡排序，对于小数组足够高效）
	for i := 0; i < len(days); i++ {
		for j := i + 1; j < len(days); j++ {
			if days[i] > days[j] {
				days[i], days[j] = days[j], days[i]
			}
		}
	}
	return days
}

// workdayOfMonth 计算给定日期最近的工作日（周一至周五）
// 如果目标日是周六，返回周五（如果不是1号）或周一（如果是1号）
// 如果目标日是周日，返回周一（如果不是月末）或周五（如果是月末）
// 参考 supercronic/cronexpr 的实现
func workdayOfMonth(targetDom, lastDom time.Time) int {
	dom := targetDom.Day()
	dow := targetDom.Weekday()
	switch dow {
	case time.Saturday:
		// 周六：如果不是1号，返回周五；否则返回下周一
		if dom > 1 {
			dom -= 1
		} else {
			dom += 2
		}
	case time.Sunday:
		// 周日：如果不是月末，返回周一；否则返回上周五
		if dom < lastDom.Day() {
			dom += 1
		} else {
			dom -= 2
		}
	}
	return dom
}
