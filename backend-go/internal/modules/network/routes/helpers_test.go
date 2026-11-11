package routes

import "testing"

func TestValidSnmpHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"192.168.1.1", true},
		{"::1", true},
		{"fe80::1", true},
		{"switch-01.example.com", true},
		{"localhost", true},
		{"", false},
		{"http://192.168.1.1", false},
		{"192.168.1.1/path", false},
		{"192.168.1.1:161", false},
		{"user@host", false},
		{"host name", false},
		{"host\nname", false},
		{"-bad.example.com", true}, // 连字符合法性交由 DNS 解析层处理
		{"bad_label.example.com", false},
		{"a..b", false},
	}
	for _, c := range cases {
		if got := validSnmpHost(c.host); got != c.want {
			t.Errorf("validSnmpHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestValidSnmpPort(t *testing.T) {
	cases := []struct {
		port int
		want bool
	}{
		{161, true}, {1, true}, {65535, true},
		{0, false}, {-1, false}, {65536, false},
	}
	for _, c := range cases {
		if got := validSnmpPort(c.port); got != c.want {
			t.Errorf("validSnmpPort(%d) = %v, want %v", c.port, got, c.want)
		}
	}
}

func TestValidOID(t *testing.T) {
	cases := []struct {
		oid  string
		want bool
	}{
		{"1.3.6.1.2.1.1.1.0", true},
		{".1.3.6.1.2.1.1.1.0", true},
		{"1.3.6", true},
		{"", false},
		{".", false},
		{"1.3..6", false},
		{"1.3.6.", false},
		{"sysName", false},
		{"1.3.6.1;rm", false},
	}
	for _, c := range cases {
		if got := validOID(c.oid); got != c.want {
			t.Errorf("validOID(%q) = %v, want %v", c.oid, got, c.want)
		}
	}
}