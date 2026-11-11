package utils

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, expr string) *CronSpec {
	t.Helper()
	c, err := ParseCron(expr)
	if err != nil {
		t.Fatalf("ParseCron(%q): %v", expr, err)
	}
	return c
}

func TestParseCronRejectsBadInput(t *testing.T) {
	for _, expr := range []string{"", "* * * *", "* * * * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8", "*/0 * * * *", "a * * * *", "5-1 * * * *"} {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) 应当报错", expr)
		}
	}
}

func TestCronNextEveryMinute(t *testing.T) {
	c := mustParse(t, "* * * * *")
	after := time.Date(2026, 11, 12, 10, 30, 45, 0, time.Local)
	got := c.Next(after)
	want := time.Date(2026, 11, 12, 10, 31, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCronNextDaily(t *testing.T) {
	c := mustParse(t, "30 2 * * *")
	after := time.Date(2026, 11, 12, 3, 0, 0, 0, time.Local)
	got := c.Next(after)
	want := time.Date(2026, 11, 13, 2, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// 边界：恰好等于触发时刻时应取下一次
	got = c.Next(want)
	want2 := time.Date(2026, 11, 14, 2, 30, 0, 0, time.Local)
	if !got.Equal(want2) {
		t.Fatalf("边界 got %v want %v", got, want2)
	}
}

func TestCronNextStepAndRange(t *testing.T) {
	c := mustParse(t, "*/15 9-17 * * 1-5")
	// 2026-11-12 是周四
	after := time.Date(2026, 11, 12, 8, 50, 0, 0, time.Local)
	got := c.Next(after)
	want := time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// 周五 17:45 之后 → 下周一 9:00
	after = time.Date(2026, 11, 13, 17, 45, 0, 0, time.Local)
	got = c.Next(after)
	want = time.Date(2026, 11, 16, 9, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("周末跳过 got %v want %v", got, want)
	}
}

func TestCronDomDowUnion(t *testing.T) {
	// 日与周同时受限：每月 1 号 OR 每周一
	c := mustParse(t, "0 0 1 * 1")
	after := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local) // 周一已过
	got := c.Next(after)
	want := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local) // 下周一
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCronSundaySeven(t *testing.T) {
	c := mustParse(t, "0 8 * * 7")
	after := time.Date(2026, 11, 12, 0, 0, 0, 0, time.Local) // 周四
	got := c.Next(after)
	want := time.Date(2026, 11, 15, 8, 0, 0, 0, time.Local) // 周日
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCronImpossibleDate(t *testing.T) {
	c := mustParse(t, "0 0 30 2 *")
	if got := c.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)); !got.IsZero() {
		t.Fatalf("2 月 30 日应无匹配，got %v", got)
	}
}

func TestCronLeapDay(t *testing.T) {
	c := mustParse(t, "0 0 29 2 *")
	after := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	got := c.Next(after)
	want := time.Date(2028, 2, 29, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}