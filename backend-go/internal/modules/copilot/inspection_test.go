package copilot

import (
	"testing"

	"itops-backend/internal/modules/network/services"
)

func f64(v float64) *float64 { return &v }

func TestScoreDeviceNoSignal(t *testing.T) {
	if got := scoreDevice(deviceSignals{}); got != -1 {
		t.Fatalf("无信号应返回 -1（未评估），got %d", got)
	}
	// 仅有备份记录不构成评分信号
	if got := scoreDevice(deviceSignals{lastBackup: "2026-11-01 10:00:00"}); got != -1 {
		t.Fatalf("仅备份记录应视为未评估，got %d", got)
	}
}

func TestScoreDeviceSSH(t *testing.T) {
	cases := []struct {
		name string
		sig  deviceSignals
		want int
	}{
		{"全部成功", deviceSignals{sshOK: true, sshExecuted: 5}, 100},
		{"1 条失败扣 5", deviceSignals{sshOK: true, sshExecuted: 5, sshFailed: 1}, 95},
		{"失败扣分封顶 20", deviceSignals{sshOK: true, sshFailed: 10}, 80},
	}
	for _, c := range cases {
		if got := scoreDevice(c.sig); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestScoreDeviceHealth(t *testing.T) {
	cases := []struct {
		name string
		h    *services.DeviceHealth
		want int
	}{
		{"健康", &services.DeviceHealth{CpuUsage: f64(10), MemoryUsage: f64(30), InterfaceCount: 8, InterfacesUp: 8}, 100},
		{"CPU>90 扣 25", &services.DeviceHealth{CpuUsage: f64(95)}, 75},
		{"CPU>80 扣 15", &services.DeviceHealth{CpuUsage: f64(85)}, 85},
		{"内存>90 扣 25", &services.DeviceHealth{MemoryUsage: f64(92)}, 75},
		{"高温扣 10", &services.DeviceHealth{Temperature: f64(70)}, 90},
		{"高负载接口封顶 20", &services.DeviceHealth{InterfaceHighUtil: 9}, 80},
		{"down 多于 up 扣 5", &services.DeviceHealth{InterfaceCount: 10, InterfacesUp: 2, InterfacesDown: 8}, 95},
		{"组合不低于 0", &services.DeviceHealth{CpuUsage: f64(99), MemoryUsage: f64(99), Temperature: f64(90), InterfaceHighUtil: 20, InterfaceCount: 10, InterfacesUp: 0, InterfacesDown: 10}, 15},
	}
	for _, c := range cases {
		if got := scoreDevice(deviceSignals{health: c.h}); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestScoreDeviceAlerts(t *testing.T) {
	if got := scoreDevice(deviceSignals{activeAlerts: 1}); got != 92 {
		t.Errorf("1 条告警应扣 8: got %d", got)
	}
	if got := scoreDevice(deviceSignals{activeAlerts: 10}); got != 70 {
		t.Errorf("告警扣分封顶 30: got %d", got)
	}
	// SSH 成功 + 告警组合
	if got := scoreDevice(deviceSignals{sshOK: true, sshFailed: 2, activeAlerts: 2}); got != 74 {
		t.Errorf("组合扣分: got %d want 74", got)
	}
}

func TestScoreLabel(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{-1, "未评估"},
		{100, "优秀 🟢"},
		{90, "优秀 🟢"},
		{89, "良好 🟡"},
		{75, "良好 🟡"},
		{74, "一般 🟠"},
		{60, "一般 🟠"},
		{59, "异常 🔴"},
		{0, "异常 🔴"},
	}
	for _, c := range cases {
		if got := scoreLabel(c.score); got != c.want {
			t.Errorf("scoreLabel(%d) = %q want %q", c.score, got, c.want)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		cs   int64
		want string
	}{
		{0, "0 小时 0 分"},
		{360000, "1 小时 0 分"},           // 1h = 3600s = 360000cs
		{9000000, "1 天 1 小时 0 分"},     // 25h
		{540000, "1 小时 30 分"}, // 1.5h = 5400s = 540000cs
	}
	for _, c := range cases {
		if got := formatUptime(c.cs); got != c.want {
			t.Errorf("formatUptime(%d) = %q want %q", c.cs, got, c.want)
		}
	}
}