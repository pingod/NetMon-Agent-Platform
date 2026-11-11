package copilot

import (
	"reflect"
	"testing"
)

func TestReferencedTables(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"plain", "SELECT * FROM users", []string{"users"}},
		{"quoted no space", `SELECT * FROM"users"`, []string{"users"}},
		{"bracket no space", "SELECT * FROM[users]", []string{"users"}},
		{"backtick no space", "SELECT * FROM `users`", []string{"users"}},
		{"quoted then where", `SELECT * FROM"users"WHERE id=1`, []string{"users"}},
		{"schema prefix", "SELECT * FROM main.users", []string{"users"}},
		{"schema quoted", `SELECT * FROM "main"."users"`, []string{"users"}},
		{"comma join", "SELECT * FROM alerts, users", []string{"alerts", "users"}},
		{"comma join no space", "SELECT * FROM alerts,users", []string{"alerts", "users"}},
		{"explicit join", "SELECT * FROM alerts a JOIN users u ON a.id=u.id", []string{"alerts", "users"}},
		{"left join quoted", `SELECT * FROM alerts a LEFT JOIN"users"u ON a.id=u.id`, []string{"alerts", "users"}},
		{"subquery", "SELECT * FROM (SELECT * FROM users)", []string{"users"}},
		{"string literal not table", "SELECT * FROM alerts WHERE note='from users'", []string{"alerts"}},
		{"quoted column not table", `SELECT "from" FROM alerts`, []string{"alerts"}},
		{"no from", "SELECT 1", nil},
		{"dedup", "SELECT * FROM users JOIN users ON 1=1", []string{"users"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := referencedTables(c.sql)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("referencedTables(%q) = %v, want %v", c.sql, got, c.want)
			}
		})
	}
}

func TestValidateReadOnlySQLSensitiveBypass(t *testing.T) {
	blocked := []string{
		`SELECT * FROM users`,
		`SELECT * FROM"users"`,
		`SELECT * FROM[users]`,
		"SELECT * FROM `users`",
		`SELECT * FROM"users"WHERE id=1`,
		`SELECT * FROM main.users`,
		`SELECT * FROM alerts, users`,
		`SELECT * FROM alerts a JOIN users u ON a.id=u.id`,
		`SELECT * FROM (SELECT * FROM users)`,
		`SELECT password FROM ai_models`,
		`SELECT * FROM snmp_credentials`,
	}
	for _, sql := range blocked {
		if ok, _ := validateReadOnlySQL(sql); ok {
			t.Errorf("expected block, got allow: %s", sql)
		}
	}
	allowed := []string{
		`SELECT * FROM alerts`,
		`SELECT * FROM alerts WHERE note='from users'`,
		`SELECT count(*) FROM metric_samples`,
	}
	for _, sql := range allowed {
		if ok, reason := validateReadOnlySQL(sql); !ok {
			t.Errorf("expected allow, got block (%s): %s", reason, sql)
		}
	}
}