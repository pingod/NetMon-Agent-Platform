// 标准 5 段 cron（分 时 日 月 周）解析与下次触发时间计算。
// 支持 *、*/n、a、a-b、a-b/n、逗号列表；周 0-7（0 与 7 均为周日）。
// 日/周同时受限时按 cron 惯例取并集（OR）。
package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CronSpec struct {
	minute, hour, dom, month, dow map[int]bool
	domRestricted, dowRestricted  bool
}

func parseCronField(field string, min, max int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("空字段段")
		}
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s <= 0 {
				return nil, fmt.Errorf("非法步长: %s", part)
			}
			step = s
			part = part[:i]
		}
		lo, hi := min, max
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			bounds := strings.SplitN(part, "-", 2)
			a, err1 := strconv.Atoi(bounds[0])
			b, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || a > b {
				return nil, fmt.Errorf("非法范围: %s", part)
			}
			lo, hi = a, b
		default:
			v, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("非法数值: %s", part)
			}
			lo, hi = v, v
			if step > 1 {
				hi = max // "a/n" 表示从 a 到最大值按步长
			}
		}
		if lo < min || hi > max {
			return nil, fmt.Errorf("数值越界 [%d,%d]: %s", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	return out, nil
}

// ParseCron 解析 5 段 cron 表达式
func ParseCron(expr string) (*CronSpec, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron 表达式须为 5 段（分 时 日 月 周），实际 %d 段", len(fields))
	}
	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("分钟字段: %w", err)
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("小时字段: %w", err)
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("日期字段: %w", err)
	}
	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("月份字段: %w", err)
	}
	dow, err := parseCronField(fields[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("星期字段: %w", err)
	}
	if dow[7] {
		dow[0] = true
		delete(dow, 7)
	}
	return &CronSpec{
		minute: minute, hour: hour, dom: dom, month: month, dow: dow,
		domRestricted: fields[2] != "*",
		dowRestricted: fields[4] != "*",
	}, nil
}

func (c *CronSpec) dayMatches(t time.Time) bool {
	if !c.month[int(t.Month())] {
		return false
	}
	domOK := c.dom[t.Day()]
	dowOK := c.dow[int(t.Weekday())]
	switch {
	case c.domRestricted && c.dowRestricted:
		return domOK || dowOK
	case c.domRestricted:
		return domOK
	case c.dowRestricted:
		return dowOK
	default:
		return true
	}
}

// Next 返回严格晚于 after 的下一个触发时间（本地时区，秒级归零）。
// 无匹配（如 2 月 30 日）时返回零值。
func (c *CronSpec) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	// 从 after 当天起逐日扫描，最多 4 年（覆盖 2/29）
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	for i := 0; i < 366*4; i++ {
		if c.dayMatches(day) {
			for h := 0; h < 24; h++ {
				if !c.hour[h] {
					continue
				}
				for m := 0; m < 60; m++ {
					if !c.minute[m] {
						continue
					}
					cand := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
					if cand.After(after) {
						return cand
					}
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return time.Time{}
}