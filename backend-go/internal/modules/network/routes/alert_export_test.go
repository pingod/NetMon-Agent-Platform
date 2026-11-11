package routes

import "testing"

func TestCsvSafeCell(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"正常标题", "正常标题"},
		{"=1+1", "'=1+1"},
		{"+cmd", "'+cmd"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\t=evil", "'\t=evil"},
		{"\r=evil", "'\r=evil"},
		{"a=b", "a=b"},
		{"2026-11-12 08:00:00", "2026-11-12 08:00:00"},
	}
	for _, c := range cases {
		if got := csvSafeCell(c.in); got != c.want {
			t.Errorf("csvSafeCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAlertLabel(t *testing.T) {
	if got := alertLabel(alertSeverityLabels, "critical"); got != "严重" {
		t.Errorf("critical -> %q", got)
	}
	if got := alertLabel(alertStatusLabels, "firing"); got != "触发中" {
		t.Errorf("firing -> %q", got)
	}
	if got := alertLabel(alertStatusLabels, "unknown"); got != "unknown" {
		t.Errorf("unknown 应原样返回, got %q", got)
	}
}