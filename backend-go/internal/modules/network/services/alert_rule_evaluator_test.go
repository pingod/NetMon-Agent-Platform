package services

import (
	"testing"
	"time"

	"itops-backend/internal/repos"
)

func TestIsSampleFresh(t *testing.T) {
	now := time.Now()
	nowTs := now.UnixMilli()

	cases := []struct {
		name string
		ts   string
		want bool
	}{
		{"刚采集", now.Format("2006-01-02 15:04:05"), true},
		{"窗口边界内", now.Add(-4*time.Minute - 59*time.Second).Format("2006-01-02 15:04:05"), true},
		{"超过窗口", now.Add(-6 * time.Minute).Format("2006-01-02 15:04:05"), false},
		{"远古样本", "2020-01-01 00:00:00", false},
		{"无法解析", "not-a-time", false},
		{"空串", "", false},
		{"未来样本", now.Add(1 * time.Minute).Format("2006-01-02 15:04:05"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isSampleFresh(c.ts, nowTs); got != c.want {
				t.Errorf("isSampleFresh(%q) = %v, want %v", c.ts, got, c.want)
			}
		})
	}
}

func TestCheckThresholdCondition(t *testing.T) {
	cases := []struct {
		val       float64
		op        string
		threshold float64
		want      bool
	}{
		{90, ">", 80, true},
		{80, ">", 80, false},
		{80, ">=", 80, true},
		{79.9, ">=", 80, false},
		{10, "<", 20, true},
		{20, "<", 20, false},
		{20, "<=", 20, true},
		{20, "==", 20, true},
		{20, "==", 21, false},
		{20, "!=", 21, true},
		{20, "!=", 20, false},
		{90, "unknown-op", 80, true}, // 未知运算符回退为 >
		{70, "unknown-op", 80, false},
	}
	for _, c := range cases {
		if got := CheckThresholdCondition(c.val, c.op, c.threshold); got != c.want {
			t.Errorf("CheckThresholdCondition(%v %q %v) = %v, want %v", c.val, c.op, c.threshold, got, c.want)
		}
	}
}

func TestMatchTagFilters(t *testing.T) {
	tags := map[string]string{"ifName": "GigabitEthernet0/0/1", "env": "prod"}

	cases := []struct {
		name    string
		filters []repos.TagFilterItem
		want    bool
	}{
		{"无过滤器", nil, true},
		{"等值匹配", []repos.TagFilterItem{{Key: "env", Op: "=", Value: "prod"}}, true},
		{"等值不匹配", []repos.TagFilterItem{{Key: "env", Op: "=", Value: "dev"}}, false},
		{"不等匹配", []repos.TagFilterItem{{Key: "env", Op: "!=", Value: "dev"}}, true},
		{"不等不匹配", []repos.TagFilterItem{{Key: "env", Op: "!=", Value: "prod"}}, false},
		{"正则匹配", []repos.TagFilterItem{{Key: "ifName", Op: "=~", Value: "^GigabitEthernet"}}, true},
		{"正则不匹配", []repos.TagFilterItem{{Key: "ifName", Op: "=~", Value: "^Ten-Gig"}}, false},
		{"负正则匹配", []repos.TagFilterItem{{Key: "ifName", Op: "!~", Value: "^Ten-Gig"}}, true},
		{"负正则不匹配", []repos.TagFilterItem{{Key: "ifName", Op: "!~", Value: "^Gigabit"}}, false},
		{"缺失键等值", []repos.TagFilterItem{{Key: "missing", Op: "=", Value: "x"}}, false},
		{"缺失键不等", []repos.TagFilterItem{{Key: "missing", Op: "!=", Value: "x"}}, true},
		{"空键跳过", []repos.TagFilterItem{{Key: "", Op: "=", Value: "x"}}, true},
		{"多条件与", []repos.TagFilterItem{
			{Key: "env", Op: "=", Value: "prod"},
			{Key: "ifName", Op: "=~", Value: "0/0/1$"},
		}, true},
		{"多条件一错全错", []repos.TagFilterItem{
			{Key: "env", Op: "=", Value: "prod"},
			{Key: "ifName", Op: "=", Value: "other"},
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MatchTagFilters(tags, c.filters); got != c.want {
				t.Errorf("MatchTagFilters(%v) = %v, want %v", c.filters, got, c.want)
			}
		})
	}
}