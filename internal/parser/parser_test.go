package parser

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

var secondParser = MustNewParser(Second | Minute | Hour | Dom | Month | DowOptional | Descriptor)

func TestRange(t *testing.T) {
	zero := uint64(0)
	ranges := []struct {
		expr     string
		min, max uint
		expected uint64
		err      string
	}{
		{"5", 0, 7, 1 << 5, ""},
		{"0", 0, 7, 1 << 0, ""},
		{"7", 0, 7, 1 << 7, ""},

		{"5-5", 0, 7, 1 << 5, ""},
		{"5-6", 0, 7, 1<<5 | 1<<6, ""},
		{"5-7", 0, 7, 1<<5 | 1<<6 | 1<<7, ""},

		{"5-6/2", 0, 7, 1 << 5, ""},
		{"5-7/2", 0, 7, 1<<5 | 1<<7, ""},
		{"5-7/1", 0, 7, 1<<5 | 1<<6 | 1<<7, ""},

		{"*", 1, 3, 1<<1 | 1<<2 | 1<<3 | starBit, ""},
		{"*/2", 1, 3, 1<<1 | 1<<3, ""},

		{"5--5", 0, 0, zero, "too many hyphens"},
		{"jan-x", 0, 0, zero, "failed to parse int from"},
		{"2-x", 1, 5, zero, "failed to parse int from"},
		{"*/-12", 0, 0, zero, "negative number"},
		{"*//2", 0, 0, zero, "too many slashes"},
		{"1", 3, 5, zero, "below minimum"},
		{"6", 3, 5, zero, "above maximum"},
		{"5-3", 3, 5, zero, "beyond end of range"},
		{"*/0", 0, 0, zero, "should be a positive number"},
	}

	for _, c := range ranges {
		actual, err := getRange(c.expr, bounds{c.min, c.max, nil})
		if len(c.err) != 0 && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s => expected %v, got %v", c.expr, c.err, err)
		}
		if len(c.err) == 0 && err != nil {
			t.Errorf("%s => unexpected error %v", c.expr, err)
		}
		if actual != c.expected {
			t.Errorf("%s => expected %d, got %d", c.expr, c.expected, actual)
		}
	}
}

func TestField(t *testing.T) {
	fields := []struct {
		expr     string
		min, max uint
		expected uint64
	}{
		{"5", 1, 7, 1 << 5},
		{"5,6", 1, 7, 1<<5 | 1<<6},
		{"5,6,7", 1, 7, 1<<5 | 1<<6 | 1<<7},
		{"1,5-7/2,3", 1, 7, 1<<1 | 1<<5 | 1<<7 | 1<<3},
	}

	for _, c := range fields {
		actual, _ := getField(c.expr, bounds{c.min, c.max, nil})
		if actual != c.expected {
			t.Errorf("%s => expected %d, got %d", c.expr, c.expected, actual)
		}
	}
}

func TestAll(t *testing.T) {
	allBits := []struct {
		r        bounds
		expected uint64
	}{
		{minutes, 0xfffffffffffffff}, // 0-59: 60 ones
		{hours, 0xffffff},            // 0-23: 24 ones
		{dom, 0xfffffffe},            // 1-31: 31 ones, 1 zero
		{months, 0x1ffe},             // 1-12: 12 ones, 1 zero
		{dow, 0x7f},                  // 0-6: 7 ones
	}

	for _, c := range allBits {
		actual := all(c.r) // all() adds the starBit, so compensate for that..
		if c.expected|starBit != actual {
			t.Errorf("%d-%d/%d => expected %b, got %b",
				c.r.min, c.r.max, 1, c.expected|starBit, actual)
		}
	}
}

func TestBits(t *testing.T) {
	bits := []struct {
		min, max, step uint
		expected       uint64
	}{
		{0, 0, 1, 0x1},
		{1, 1, 1, 0x2},
		{1, 5, 2, 0x2a}, // 101010
		{1, 4, 2, 0xa},  // 1010
	}

	for _, c := range bits {
		actual := getBits(c.min, c.max, c.step)
		if c.expected != actual {
			t.Errorf("%d-%d/%d => expected %b, got %b",
				c.min, c.max, c.step, c.expected, actual)
		}
	}
}

func TestParseScheduleErrors(t *testing.T) {
	tests := []struct{ expr, err string }{
		{"* 5 j * * *", "failed to parse int from"},
		{"@every Xm", "failed to parse duration"},
		{"@unrecognized", "unrecognized descriptor"},
		{"* * * *", "expected 5 to 6 fields"},
		{"", "empty spec string"},
	}
	for _, c := range tests {
		actual, err := secondParser.Parse(c.expr)
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s => expected %v, got %v", c.expr, c.err, err)
		}
		if actual != nil {
			t.Errorf("expected nil schedule on error, got %v", actual)
		}
	}
}

func TestParseSchedule(t *testing.T) {
	entries := []struct {
		parser   Parser
		expr     string
		expected Schedule
	}{
		{secondParser, "0 5 * * * *", every5min(time.Local, false)},
		{standardParser, "5 * * * *", every5min(time.Local, false)},
		{secondParser, "CRON_TZ=UTC  0 5 * * * *", every5min(time.UTC, true)},
		{standardParser, "CRON_TZ=UTC  5 * * * *", every5min(time.UTC, true)},
		{secondParser, "0 5 * * * *", every5min(time.Local, false)},
		{
			parser: secondParser,
			expr:   "* 5 * * * *",
			expected: &SpecSchedule{
				Second:                 all(seconds),
				Minute:                 1 << 5,
				Hour:                   all(hours),
				Dom:                    all(dom),
				Month:                  all(months),
				Dow:                    all(dow),
				Location:               time.Local,
				locationSet:            false,
				lastDayOfMonth:         false,
				lastWorkdayOfMonth:     false,
				workdaysOfMonth:        map[int]bool{},
				lastWeekDaysOfWeek:     map[int]bool{},
				specificWeekDaysOfWeek: map[int]bool{},
				daysOfMonthRestricted:  false,
				daysOfWeekRestricted:   false,
			},
		},
	}

	for _, c := range entries {
		actual, err := c.parser.Parse(c.expr)
		if err != nil {
			t.Errorf("%s => unexpected error %v", c.expr, err)
		}
		if !reflect.DeepEqual(actual, c.expected) {
			t.Errorf("%s => expected %b, got %b", c.expr, c.expected, actual)
		}
	}
}

func TestOptionalSecondSchedule(t *testing.T) {
	parser := MustNewParser(SecondOptional | Minute | Hour | Dom | Month | Dow | Descriptor)
	entries := []struct {
		expr     string
		expected Schedule
	}{
		{"0 5 * * * *", every5min(time.Local, false)},
		{"5 5 * * * *", every5min5s(time.Local, false)},
		{"5 * * * *", every5min(time.Local, false)},
	}

	for _, c := range entries {
		actual, err := parser.Parse(c.expr)
		if err != nil {
			t.Errorf("%s => unexpected error %v", c.expr, err)
		}
		if !reflect.DeepEqual(actual, c.expected) {
			t.Errorf("%s => expected %b, got %b", c.expr, c.expected, actual)
		}
	}
}

func TestNormalizeFields(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		options  ParseOption
		expected []string
	}{
		{
			"AllFields_NoOptional",
			[]string{"0", "5", "*", "*", "*", "*"},
			Second | Minute | Hour | Dom | Month | Dow | Descriptor,
			[]string{"0", "5", "*", "*", "*", "*"},
		},
		{
			"AllFields_SecondOptional_Provided",
			[]string{"0", "5", "*", "*", "*", "*"},
			SecondOptional | Minute | Hour | Dom | Month | Dow | Descriptor,
			[]string{"0", "5", "*", "*", "*", "*"},
		},
		{
			"AllFields_SecondOptional_NotProvided",
			[]string{"5", "*", "*", "*", "*"},
			SecondOptional | Minute | Hour | Dom | Month | Dow | Descriptor,
			[]string{"0", "5", "*", "*", "*", "*"},
		},
		{
			"SubsetFields_NoOptional",
			[]string{"5", "15", "*"},
			Hour | Dom | Month,
			[]string{"0", "0", "5", "15", "*", "*"},
		},
		{
			"SubsetFields_DowOptional_Provided",
			[]string{"5", "15", "*", "4"},
			Hour | Dom | Month | DowOptional,
			[]string{"0", "0", "5", "15", "*", "4"},
		},
		{
			"SubsetFields_DowOptional_NotProvided",
			[]string{"5", "15", "*"},
			Hour | Dom | Month | DowOptional,
			[]string{"0", "0", "5", "15", "*", "*"},
		},
		{
			"SubsetFields_SecondOptional_NotProvided",
			[]string{"5", "15", "*"},
			SecondOptional | Hour | Dom | Month,
			[]string{"0", "0", "5", "15", "*", "*"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := normalizeFields(test.input, test.options)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(actual, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, actual)
			}
		})
	}
}

func TestNormalizeFields_Errors(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		options ParseOption
		err     string
	}{
		{
			"TwoOptionals",
			[]string{"0", "5", "*", "*", "*", "*"},
			SecondOptional | Minute | Hour | Dom | Month | DowOptional,
			"",
		},
		{
			"TooManyFields",
			[]string{"0", "5", "*", "*"},
			SecondOptional | Minute | Hour,
			"",
		},
		{
			"NoFields",
			[]string{},
			SecondOptional | Minute | Hour,
			"",
		},
		{
			"TooFewFields",
			[]string{"*"},
			SecondOptional | Minute | Hour,
			"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := normalizeFields(test.input, test.options)
			if err == nil {
				t.Errorf("expected an error, got none. results: %v", actual)
			}
			if !strings.Contains(err.Error(), test.err) {
				t.Errorf("expected error %q, got %q", test.err, err.Error())
			}
		})
	}
}

func TestStandardSpecSchedule(t *testing.T) {
	entries := []struct {
		expr     string
		expected Schedule
		err      string
	}{
		{
			expr: "5 * * * *",
			expected: &SpecSchedule{
				Second:                 1 << seconds.min,
				Minute:                 1 << 5,
				Hour:                   all(hours),
				Dom:                    all(dom),
				Month:                  all(months),
				Dow:                    all(dow),
				Location:               time.Local,
				locationSet:            false,
				lastDayOfMonth:         false,
				lastWorkdayOfMonth:     false,
				workdaysOfMonth:        map[int]bool{},
				lastWeekDaysOfWeek:     map[int]bool{},
				specificWeekDaysOfWeek: map[int]bool{},
				daysOfMonthRestricted:  false,
				daysOfWeekRestricted:   false,
			},
		},

		{
			expr: "5 j * * *",
			err:  "failed to parse int from",
		},
		{
			expr: "* * * *",
			err:  "expected exactly 5 fields",
		},
	}

	for _, c := range entries {
		actual, err := ParseStandard(c.expr)
		if len(c.err) != 0 && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s => expected %v, got %v", c.expr, c.err, err)
		}
		if len(c.err) == 0 && err != nil {
			t.Errorf("%s => unexpected error %v", c.expr, err)
		}
		if !reflect.DeepEqual(actual, c.expected) {
			t.Errorf("%s => expected %b, got %b", c.expr, c.expected, actual)
		}
	}
}

func TestNoDescriptorParser(t *testing.T) {
	parser := MustNewParser(Minute | Hour)
	_, err := parser.Parse("@every 1m")
	if err == nil {
		t.Error("expected an error, got none")
	}
}

func TestNewParserRejectsMultipleOptionalFields(t *testing.T) {
	_, err := NewParser(SecondOptional | DowOptional | Minute | Hour | Dom | Month)
	if err == nil {
		t.Fatal("expected invalid parser options error")
	}
}

func every5min(loc *time.Location, explicit bool) *SpecSchedule {
	return &SpecSchedule{
		Second:                 1 << 0,
		Minute:                 1 << 5,
		Hour:                   all(hours),
		Dom:                    all(dom),
		Month:                  all(months),
		Dow:                    all(dow),
		Location:               loc,
		locationSet:            explicit,
		lastDayOfMonth:         false,
		lastWorkdayOfMonth:     false,
		workdaysOfMonth:        map[int]bool{},
		lastWeekDaysOfWeek:     map[int]bool{},
		specificWeekDaysOfWeek: map[int]bool{},
		daysOfMonthRestricted:  false,
		daysOfWeekRestricted:   false,
	}
}

func every5min5s(loc *time.Location, explicit bool) *SpecSchedule {
	return &SpecSchedule{
		Second:                 1 << 5,
		Minute:                 1 << 5,
		Hour:                   all(hours),
		Dom:                    all(dom),
		Month:                  all(months),
		Dow:                    all(dow),
		Location:               loc,
		locationSet:            explicit,
		lastDayOfMonth:         false,
		lastWorkdayOfMonth:     false,
		workdaysOfMonth:        map[int]bool{},
		lastWeekDaysOfWeek:     map[int]bool{},
		specificWeekDaysOfWeek: map[int]bool{},
		daysOfMonthRestricted:  false,
		daysOfWeekRestricted:   false,
	}
}

// TestSpecialDomSyntax 测试 L/W/LW 语法
func TestSpecialDomSyntax(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		checkFn  func(*testing.T, Schedule)
		wantErr  bool
		errorMsg string
	}{
		{
			name: "L - 月末最后一天",
			expr: "0 0 L * *",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.lastDayOfMonth {
					t.Error("expected lastDayOfMonth to be true")
				}
				if !spec.daysOfMonthRestricted {
					t.Error("expected daysOfMonthRestricted to be true")
				}
			},
		},
		{
			name: "LW - 月末最后一个工作日",
			expr: "0 0 LW * *",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.lastWorkdayOfMonth {
					t.Error("expected lastWorkdayOfMonth to be true")
				}
				if !spec.daysOfMonthRestricted {
					t.Error("expected daysOfMonthRestricted to be true")
				}
			},
		},
		{
			name: "15W - 第15天最近的工作日",
			expr: "0 0 15W * *",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.workdaysOfMonth[15] {
					t.Error("expected workdaysOfMonth[15] to be true")
				}
				if !spec.daysOfMonthRestricted {
					t.Error("expected daysOfMonthRestricted to be true")
				}
			},
		},
		{
			name: "1W,15W - 多个工作日",
			expr: "0 0 1W,15W * *",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.workdaysOfMonth[1] {
					t.Error("expected workdaysOfMonth[1] to be true")
				}
				if !spec.workdaysOfMonth[15] {
					t.Error("expected workdaysOfMonth[15] to be true")
				}
			},
		},
		{
			name: "L,15 - 混合标准和特殊语法",
			expr: "0 0 L,15 * *",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.lastDayOfMonth {
					t.Error("expected lastDayOfMonth to be true")
				}
				if spec.Dom&(1<<15) == 0 {
					t.Error("expected bit 15 to be set in Dom")
				}
			},
		},
		{
			name:     "0W - 无效的工作日（0不在范围内）",
			expr:     "0 0 0W * *",
			wantErr:  true,
			errorMsg: "out of range",
		},
		{
			name:     "32W - 无效的工作日（超出范围）",
			expr:     "0 0 32W * *",
			wantErr:  true,
			errorMsg: "out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseStandard(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, sched)
			}
		})
	}
}

// TestSpecialDowSyntax 测试 L/# 语法
func TestSpecialDowSyntax(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		checkFn  func(*testing.T, Schedule)
		wantErr  bool
		errorMsg string
	}{
		{
			name: "5L - 每月最后一个星期五",
			expr: "0 0 * * 5L",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.lastWeekDaysOfWeek[5] {
					t.Error("expected lastWeekDaysOfWeek[5] to be true")
				}
				if !spec.daysOfWeekRestricted {
					t.Error("expected daysOfWeekRestricted to be true")
				}
			},
		},
		{
			name: "FRIL - 使用名称的最后一个星期五",
			expr: "0 0 * * FRIL",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.lastWeekDaysOfWeek[5] {
					t.Error("expected lastWeekDaysOfWeek[5] to be true")
				}
			},
		},
		{
			name: "5#3 - 每月第3个星期五",
			expr: "0 0 * * 5#3",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				// 编码: (week-1)*7 + dow = (3-1)*7 + 5 = 19
				if !spec.specificWeekDaysOfWeek[19] {
					t.Errorf("expected specificWeekDaysOfWeek[19] to be true, got %v", spec.specificWeekDaysOfWeek)
				}
				if !spec.daysOfWeekRestricted {
					t.Error("expected daysOfWeekRestricted to be true")
				}
			},
		},
		{
			name: "1#1 - 每月第1个星期一",
			expr: "0 0 * * 1#1",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				// 编码: (1-1)*7 + 1 = 1
				if !spec.specificWeekDaysOfWeek[1] {
					t.Errorf("expected specificWeekDaysOfWeek[1] to be true, got %v", spec.specificWeekDaysOfWeek)
				}
			},
		},
		{
			name: "MON#2 - 使用名称的第2个星期一",
			expr: "0 0 * * MON#2",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				// 编码: (2-1)*7 + 1 = 8
				if !spec.specificWeekDaysOfWeek[8] {
					t.Errorf("expected specificWeekDaysOfWeek[8] to be true, got %v", spec.specificWeekDaysOfWeek)
				}
			},
		},
		{
			name: "1#1,5L - 混合特殊语法",
			expr: "0 0 * * 1#1,5L",
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.specificWeekDaysOfWeek[1] {
					t.Error("expected specificWeekDaysOfWeek[1] to be true")
				}
				if !spec.lastWeekDaysOfWeek[5] {
					t.Error("expected lastWeekDaysOfWeek[5] to be true")
				}
			},
		},
		{
			name:     "5#0 - 无效的周数（0）",
			expr:     "0 0 * * 5#0",
			wantErr:  true,
			errorMsg: "out of range",
		},
		{
			name:     "5#6 - 无效的周数（超出范围）",
			expr:     "0 0 * * 5#6",
			wantErr:  true,
			errorMsg: "out of range",
		},
		{
			name:    "7L - 无效的星期（7在dow字段会被转换为0，但这里测试边界）",
			expr:    "0 0 * * 7L",
			wantErr: false, // 7会被转换为0（周日）
			checkFn: func(t *testing.T, s Schedule) {
				spec := s.(*SpecSchedule)
				if !spec.lastWeekDaysOfWeek[0] {
					t.Error("expected lastWeekDaysOfWeek[0] to be true (7 should convert to 0)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseStandard(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, sched)
			}
		})
	}
}

// TestSpecialSyntaxNext 测试特殊语法的 Next() 方法
func TestSpecialSyntaxNext(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		from     time.Time
		expected time.Time
	}{
		{
			name:     "L - 2025年1月最后一天",
			expr:     "0 0 L * *",
			from:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "L - 2025年2月最后一天（28天）",
			expr:     "0 0 L * *",
			from:     time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "15W - 2025年3月15日是星期六，应该调整到14日（周五）",
			expr: "0 0 15W * *",
			from: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			// 2025年3月15日是星期六，最近的工作日是3月14日（周五）
			expected: time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "5#3 - 2025年1月第3个星期五",
			expr: "0 0 * * 5#3",
			from: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			// 2025年1月：3, 10, 17, 24, 31 是星期五，第3个是17日
			expected: time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "5L - 2025年1月最后一个星期五",
			expr: "0 0 * * 5L",
			from: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			// 2025年1月最后一个星期五是31日
			expected: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseStandard(tt.expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			next := sched.Next(tt.from)
			if !next.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, next)
			}
		})
	}
}
