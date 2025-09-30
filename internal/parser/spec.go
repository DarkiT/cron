package parser

import "time"

// SpecSchedule specifies a duty cycle (to the second granularity), based on a
// traditional crontab specification. It is computed initially and stored as bit sets.
type SpecSchedule struct {
	Second, Minute, Hour, Dom, Month, Dow uint64

	// Override location for this schedule.
	Location *time.Location
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
	// 简化实现：调整到下一秒，然后查找匹配的时间
	t = t.Add(1*time.Second - time.Duration(t.Nanosecond())*time.Nanosecond)

	if s.Location != nil {
		t = t.In(s.Location)
	}

	// 限制搜索范围，避免无限循环
	yearLimit := t.Year() + 4

WRAP:
	for t.Year() < yearLimit {
		// 检查月份
		for 1<<uint(t.Month())&s.Month == 0 {
			// 如果月份不匹配，跳到下个月的第一天
			if t.Month() == time.December {
				t = time.Date(t.Year()+1, time.January, 1, 0, 0, 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			}
			if t.Year() >= yearLimit {
				return time.Time{}
			}
		}

		// 检查日期
		for !dayMatches(s, t) {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())

			if t.Day() == 1 {
				// 已经进入新月份，重新检查月份
				goto WRAP
			}
			if t.Year() >= yearLimit {
				return time.Time{}
			}
		}

		// 检查小时
		for 1<<uint(t.Hour())&s.Hour == 0 {
			t = t.Add(1 * time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())

			if t.Hour() == 0 {
				// 已经进入新的一天，重新检查日期
				goto WRAP
			}
			if t.Year() >= yearLimit {
				return time.Time{}
			}
		}

		// 检查分钟
		for 1<<uint(t.Minute())&s.Minute == 0 {
			t = t.Add(1 * time.Minute)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())

			if t.Minute() == 0 {
				// 已经进入新的小时，重新检查小时
				goto WRAP
			}
			if t.Year() >= yearLimit {
				return time.Time{}
			}
		}

		// 检查秒
		for 1<<uint(t.Second())&s.Second == 0 {
			t = t.Add(1 * time.Second)

			if t.Second() == 0 {
				// 已经进入新的分钟，重新检查分钟
				goto WRAP
			}
			if t.Year() >= yearLimit {
				return time.Time{}
			}
		}

		return t
	}

	return time.Time{}
}

// dayMatches returns true if the schedule's day-of-week and day-of-month
// restrictions are satisfied by the given time.
func dayMatches(s *SpecSchedule, t time.Time) bool {
	var (
		domMatch bool = 1<<uint(t.Day())&s.Dom > 0
		dowMatch bool = 1<<uint(t.Weekday())&s.Dow > 0
	)
	if s.Dom&starBit > 0 || s.Dow&starBit > 0 {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}
