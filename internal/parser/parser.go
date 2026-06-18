package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Schedule describes a job's duty cycle.
type Schedule interface {
	// Next returns the next activation time, later than the given time.
	// Next is invoked initially, and then each time the job is run.
	Next(time.Time) time.Time
}

// Configuration options for creating a parser. Most options specify which
// fields should be included, while others enable features. If a field is not
// included the parser will assume a default value. These options do not change
// the order fields are parse in.

type ParseOption int

const (
	Second         ParseOption = 1 << iota // Seconds field, default 0
	SecondOptional                         // Optional seconds field, default 0
	Minute                                 // Minutes field, default 0
	Hour                                   // Hours field, default 0
	Dom                                    // Day of month field, default *
	Month                                  // Month field, default *
	Dow                                    // Day of week field, default *
	DowOptional                            // Optional day of week field, default *
	Descriptor                             // Allow descriptors such as @monthly, @weekly, etc.
)

var places = []ParseOption{
	Second,
	Minute,
	Hour,
	Dom,
	Month,
	Dow,
}

var defaults = []string{
	"0",
	"0",
	"0",
	"*",
	"*",
	"*",
}

// A custom Parser that can be configured.
type Parser struct {
	options ParseOption
}

// NewParser creates a Parser with custom options.
//
// Examples
//
//	// Standard parser without descriptors
//	specParser := NewParser(Minute | Hour | Dom | Month | Dow)
//	sched, err := specParser.Parse("0 0 15 */3 *")
//
//	// Same as above, just excludes time fields
//	subsParser := NewParser(Dom | Month | Dow)
//	sched, err := specParser.Parse("15 */3 *")
//
//	// Same as above, just makes Dow optional
//	subsParser, err := NewParser(Dom | Month | DowOptional)
//	sched, err := specParser.Parse("15 */3")
func NewParser(options ParseOption) (Parser, error) {
	optionals := 0
	if options&DowOptional > 0 {
		optionals++
	}
	if options&SecondOptional > 0 {
		optionals++
	}
	if optionals > 1 {
		return Parser{}, fmt.Errorf("multiple optionals may not be configured")
	}
	return Parser{options: options}, nil
}

// MustNewParser creates a Parser and panics on invalid options.
// Suitable for package-level defaults and tests that require fail-fast semantics.
func MustNewParser(options ParseOption) Parser {
	p, err := NewParser(options)
	if err != nil {
		panic(err)
	}
	return p
}

// Parse returns a new crontab schedule representing the given spec.
// It returns a descriptive error if the spec is not valid.
// It accepts crontab specs and features configured by NewParser.
func (p Parser) Parse(spec string) (Schedule, error) {
	// 使用缓存加速解析过程
	schedule, err := parseWithCache(p, spec)
	if err != nil {
		return nil, err
	}

	return schedule, nil
}

// normalizeFields takes a subset set of the time fields and returns the full set
// with defaults (zeroes) populated for unset fields.
//
// As part of performing this function, it also validates that the provided
// fields are compatible with the configured options.
func normalizeFields(fields []string, options ParseOption) ([]string, error) {
	// Validate optionals & add their field to options
	optionals := 0
	if options&SecondOptional > 0 {
		options |= Second
		optionals++
	}
	if options&DowOptional > 0 {
		options |= Dow
		optionals++
	}
	if optionals > 1 {
		return nil, fmt.Errorf("multiple optionals may not be configured")
	}

	// Figure out how many fields we need
	max := 0
	for _, place := range places {
		if options&place > 0 {
			max++
		}
	}
	min := max - optionals

	// Validate number of fields
	if count := len(fields); count < min || count > max {
		if min == max {
			return nil, fmt.Errorf("expected exactly %d fields, found %d: %s", min, count, fields)
		}
		return nil, fmt.Errorf("expected %d to %d fields, found %d: %s", min, max, count, fields)
	}

	// Populate the optional field if not provided
	if min < max && len(fields) == min {
		switch {
		case options&DowOptional > 0:
			fields = append(fields, defaults[5]) // TODO: improve access to default
		case options&SecondOptional > 0:
			fields = append([]string{defaults[0]}, fields...)
		default:
			return nil, fmt.Errorf("unknown optional field")
		}
	}

	// Populate all fields not part of options with their defaults
	n := 0
	expandedFields := make([]string, len(places))
	copy(expandedFields, defaults)
	for i, place := range places {
		if options&place > 0 {
			expandedFields[i] = fields[n]
			n++
		}
	}
	return expandedFields, nil
}

var standardParser = MustNewParser(
	Minute | Hour | Dom | Month | Dow | Descriptor,
)

// ParseStandard returns a new crontab schedule representing the given
// standardSpec (https://en.wikipedia.org/wiki/Cron). It requires 5 entries
// representing: minute, hour, day of month, month and day of week, in that
// order. It returns a descriptive error if the spec is not valid.
//
// It accepts
//   - Standard crontab specs, e.g. "* * * * ?"
//   - Descriptors, e.g. "@midnight", "@every 1h30m"
func ParseStandard(standardSpec string) (Schedule, error) {
	return standardParser.Parse(standardSpec)
}

// getField returns an Int with the bits set representing all of the times that
// the field represents or error parsing field value.  A "field" is a comma-separated
// list of "ranges".
func getField(field string, r bounds) (uint64, error) {
	var bits uint64
	ranges := strings.FieldsFunc(field, func(r rune) bool { return r == ',' })
	for _, expr := range ranges {
		bit, err := getRange(expr, r)
		if err != nil {
			return bits, err
		}
		bits |= bit
	}
	return bits, nil
}

// specialFieldInfo 保存特殊字段解析的结果
type specialFieldInfo struct {
	bits                   uint64
	lastDayOfMonth         bool
	lastWorkdayOfMonth     bool
	workdaysOfMonth        map[int]bool
	lastWeekDaysOfWeek     map[int]bool
	specificWeekDaysOfWeek map[int]bool
	isRestricted           bool // 是否受限（不是 *）
}

// getRange returns the bits indicated by the given expression:
//
//	number | number "-" number [ "/" number ]
//
// or error parsing range.
func getRange(expr string, r bounds) (uint64, error) {
	var (
		start, end, step uint
		rangeAndStep     = strings.Split(expr, "/")
		lowAndHigh       = strings.Split(rangeAndStep[0], "-")
		singleDigit      = len(lowAndHigh) == 1
		err              error
	)

	var extra uint64
	if lowAndHigh[0] == "*" || lowAndHigh[0] == "?" {
		start = r.min
		end = r.max
		extra = starBit
	} else {
		start, err = parseIntOrName(lowAndHigh[0], r.names)
		if err != nil {
			return 0, err
		}
		switch len(lowAndHigh) {
		case 1:
			end = start
		case 2:
			end, err = parseIntOrName(lowAndHigh[1], r.names)
			if err != nil {
				return 0, err
			}
		default:
			return 0, fmt.Errorf("too many hyphens: %s", expr)
		}
	}

	switch len(rangeAndStep) {
	case 1:
		step = 1
	case 2:
		step, err = mustParseInt(rangeAndStep[1])
		if err != nil {
			return 0, err
		}

		// Special handling: "N/step" means "N-max/step".
		if singleDigit {
			end = r.max
		}
		if step > 1 {
			extra = 0
		}
	default:
		return 0, fmt.Errorf("too many slashes: %s", expr)
	}

	if start < r.min {
		return 0, fmt.Errorf("beginning of range (%d) below minimum (%d): %s", start, r.min, expr)
	}

	// 特殊处理：周日可以用7表示，但内部仍使用0
	if r.max == dow.max && start == 7 {
		start = 0
	}
	if end > r.max {
		// 特殊处理：周日可以用7表示，但内部仍使用0
		if r.max == dow.max && end == 7 {
			end = 0
		} else {
			return 0, fmt.Errorf("end of range (%d) above maximum (%d): %s", end, r.max, expr)
		}
	}
	if start > end {
		return 0, fmt.Errorf("beginning of range (%d) beyond end of range (%d): %s", start, end, expr)
	}
	if step == 0 {
		return 0, fmt.Errorf("step of range should be a positive number: %s", expr)
	}

	return getBits(start, end, step) | extra, nil
}

// parseIntOrName returns the (possibly-named) integer contained in expr.
func parseIntOrName(expr string, names map[string]uint) (uint, error) {
	if names != nil {
		if namedInt, ok := names[strings.ToLower(expr)]; ok {
			return namedInt, nil
		}
	}

	// 特殊处理：如果是星期几字段（通过检查是否包含"sun"键），并且值为7，则转换为0（周日）
	if names != nil && expr == "7" {
		if _, hasSun := names["sun"]; hasSun {
			return 0, nil
		}
	}

	return mustParseInt(expr)
}

// mustParseInt parses the given expression as an int or returns an error.
func mustParseInt(expr string) (uint, error) {
	num, err := strconv.Atoi(expr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse int from %s: %s", expr, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("negative number (%d) not allowed: %s", num, expr)
	}

	return uint(num), nil
}

// getBits sets all bits in the range [min, max], modulo the given step size.
func getBits(min, max, step uint) uint64 {
	var bits uint64

	// If step is 1, use shifts.
	if step == 1 {
		return ^(math.MaxUint64 << (max + 1)) & (math.MaxUint64 << min)
	}

	// Else, use a simple loop.
	for i := min; i <= max; i += step {
		bits |= 1 << i
	}
	return bits
}

// all returns all bits within the given bounds.  (plus the star bit)
func all(r bounds) uint64 {
	return getBits(r.min, r.max, 1) | starBit
}

// getDomFieldSpecial 解析 Dom 字段的特殊语法（L/W/LW）
func getDomFieldSpecial(field string, r bounds) (*specialFieldInfo, error) {
	info := &specialFieldInfo{
		bits:            0,
		workdaysOfMonth: make(map[int]bool),
		isRestricted:    true,
	}

	// 检查是否是通配符
	if field == "*" || field == "?" {
		info.bits = all(r)
		info.isRestricted = false
		return info, nil
	}

	// 解析逗号分隔的多个表达式
	ranges := strings.FieldsFunc(field, func(r rune) bool { return r == ',' })
	for _, expr := range ranges {
		exprLower := strings.ToLower(strings.TrimSpace(expr))

		// L - 月末最后一天
		if exprLower == "l" {
			info.lastDayOfMonth = true
			continue
		}

		// LW - 月末最后一个工作日
		if exprLower == "lw" || exprLower == "wl" {
			info.lastWorkdayOfMonth = true
			continue
		}

		// 15W - 第15天最近的工作日
		if before, ok := strings.CutSuffix(exprLower, "w"); ok {
			dayStr := before
			day, err := mustParseInt(dayStr)
			if err != nil {
				return nil, fmt.Errorf("invalid workday syntax '%s': %s", expr, err)
			}
			if day < r.min || day > r.max {
				return nil, fmt.Errorf("workday %d out of range [%d-%d]", day, r.min, r.max)
			}
			info.workdaysOfMonth[int(day)] = true
			continue
		}

		// 标准语法：数字、范围、步长
		bit, err := getRange(expr, r)
		if err != nil {
			return nil, err
		}
		info.bits |= bit
	}

	return info, nil
}

// getDowFieldSpecial 解析 Dow 字段的特殊语法（L/#）
func getDowFieldSpecial(field string, r bounds) (*specialFieldInfo, error) {
	info := &specialFieldInfo{
		bits:                   0,
		lastWeekDaysOfWeek:     make(map[int]bool),
		specificWeekDaysOfWeek: make(map[int]bool),
		isRestricted:           true,
	}

	// 检查是否是通配符
	if field == "*" || field == "?" {
		info.bits = all(r)
		info.isRestricted = false
		return info, nil
	}

	// 解析逗号分隔的多个表达式
	ranges := strings.FieldsFunc(field, func(r rune) bool { return r == ',' })
	for _, expr := range ranges {
		exprLower := strings.ToLower(strings.TrimSpace(expr))

		// 5L - 每月最后一个星期五
		if before, ok := strings.CutSuffix(exprLower, "l"); ok {
			dowStr := before
			dow, err := parseIntOrName(dowStr, r.names)
			if err != nil {
				return nil, fmt.Errorf("invalid last weekday syntax '%s': %s", expr, err)
			}
			if dow < r.min || dow > r.max {
				return nil, fmt.Errorf("day-of-week %d out of range [%d-%d]", dow, r.min, r.max)
			}
			info.lastWeekDaysOfWeek[int(dow)] = true
			continue
		}

		// 5#3 - 每月第3个星期五
		if strings.Contains(exprLower, "#") {
			parts := strings.Split(exprLower, "#")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid specific weekday syntax '%s'", expr)
			}
			dow, err := parseIntOrName(parts[0], r.names)
			if err != nil {
				return nil, fmt.Errorf("invalid specific weekday syntax '%s': %s", expr, err)
			}
			week, err := mustParseInt(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid week number in '%s': %s", expr, err)
			}
			if dow < r.min || dow > r.max {
				return nil, fmt.Errorf("day-of-week %d out of range [%d-%d]", dow, r.min, r.max)
			}
			if week < 1 || week > 5 {
				return nil, fmt.Errorf("week number %d out of range [1-5]", week)
			}
			// 编码为 (week-1)*7 + dow，参考 supercronic 的实现
			info.specificWeekDaysOfWeek[int((week-1)*7+dow%7)] = true
			continue
		}

		// 标准语法：数字、范围、步长
		bit, err := getRange(expr, r)
		if err != nil {
			return nil, err
		}
		info.bits |= bit
	}

	return info, nil
}
