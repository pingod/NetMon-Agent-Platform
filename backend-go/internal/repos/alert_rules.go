package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

type TagFilterItem struct {
	Key   string `json:"key"`
	Op    string `json:"op"` // "=", "!=", "=~", "!~"
	Value string `json:"value"`
}

type AlertRuleRecord struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Severity       string          `json:"severity"` // "critical", "warning", "info"
	MetricName     string          `json:"metricName"`
	PromQL         string          `json:"promql"`
	TagsFilter     []TagFilterItem `json:"tagsFilter"`
	TagsFilterJSON string          `json:"tagsFilterJson,omitempty"`
	Operator       string          `json:"operator"` // ">", ">=", "<", "<=", "==", "!="
	Threshold      float64         `json:"threshold"`
	ForDurationSec int             `json:"forDurationSec"`
	GroupByTags    string          `json:"groupByTags"`
	Enabled        bool            `json:"enabled"`
	NotifyRemote   bool            `json:"notifyRemote"`
	CooldownMs     int64           `json:"cooldownMs"`
	LastTriggered  int64           `json:"lastTriggered,omitempty"`
	Template       string          `json:"template"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

type AlertRuleRepo interface {
	List() ([]AlertRuleRecord, error)
	GetByID(id string) (*AlertRuleRecord, error)
	Create(r AlertRuleRecord) error
	Update(id string, r AlertRuleRecord) error
	Delete(id string) error
	Toggle(id string, enabled bool) error
	UpdateLastTriggered(id string, ts int64) error
}

type alertRuleSQLiteRepo struct{}

var AlertRules AlertRuleRepo = &alertRuleSQLiteRepo{}

func ensureAlertRulesSchema(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS alert_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			severity TEXT NOT NULL DEFAULT 'warning',
			metric_name TEXT NOT NULL,
			promql TEXT,
			tags_filter_json TEXT,
			operator TEXT NOT NULL DEFAULT '>',
			threshold REAL NOT NULL DEFAULT 0,
			for_duration_sec INTEGER NOT NULL DEFAULT 0,
			group_by_tags TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			notify_remote INTEGER NOT NULL DEFAULT 1,
			cooldown_ms INTEGER NOT NULL DEFAULT 600000,
			last_triggered_at INTEGER DEFAULT 0,
			template TEXT,
			created_at TEXT,
			updated_at TEXT
		)
	`)
	// 针对已有表尝试增加 notify_remote 字段
	_, _ = db.Exec(`ALTER TABLE alert_rules ADD COLUMN notify_remote INTEGER NOT NULL DEFAULT 1`)
}

func (repo *alertRuleSQLiteRepo) List() ([]AlertRuleRecord, error) {
	db := appdb.DB()
	ensureAlertRulesSchema(db)

	rows, err := db.Query(`
		SELECT id, name, description, severity, metric_name, promql, tags_filter_json,
		       operator, threshold, for_duration_sec, group_by_tags, enabled,
		       cooldown_ms, last_triggered_at, template, created_at, updated_at,
		       COALESCE(notify_remote, 1)
		FROM alert_rules
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()

	var rules []AlertRuleRecord
	for rows.Next() {
		var r AlertRuleRecord
		var desc, promql, tagsJSON, groupBy, tmpl, cAt, uAt sql.NullString
		var enabledInt, notifyRemoteInt int
		if err := rows.Scan(
			&r.ID, &r.Name, &desc, &r.Severity, &r.MetricName, &promql, &tagsJSON,
			&r.Operator, &r.Threshold, &r.ForDurationSec, &groupBy, &enabledInt,
			&r.CooldownMs, &r.LastTriggered, &tmpl, &cAt, &uAt,
			&notifyRemoteInt,
		); err != nil {
			continue
		}
		r.Description = desc.String
		r.PromQL = promql.String
		r.GroupByTags = groupBy.String
		r.Template = tmpl.String
		r.CreatedAt = cAt.String
		r.UpdatedAt = uAt.String
		r.Enabled = enabledInt == 1
		r.NotifyRemote = notifyRemoteInt == 1

		if tagsJSON.Valid && tagsJSON.String != "" {
			_ = json.Unmarshal([]byte(tagsJSON.String), &r.TagsFilter)
		} else {
			r.TagsFilter = []TagFilterItem{}
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (repo *alertRuleSQLiteRepo) GetByID(id string) (*AlertRuleRecord, error) {
	db := appdb.DB()
	ensureAlertRulesSchema(db)

	var r AlertRuleRecord
	var desc, promql, tagsJSON, groupBy, tmpl, cAt, uAt sql.NullString
	var enabledInt, notifyRemoteInt int

	err := db.QueryRow(`
		SELECT id, name, description, severity, metric_name, promql, tags_filter_json,
		       operator, threshold, for_duration_sec, group_by_tags, enabled,
		       cooldown_ms, last_triggered_at, template, created_at, updated_at,
		       COALESCE(notify_remote, 1)
		FROM alert_rules WHERE id = ?`, id).Scan(
		&r.ID, &r.Name, &desc, &r.Severity, &r.MetricName, &promql, &tagsJSON,
		&r.Operator, &r.Threshold, &r.ForDurationSec, &groupBy, &enabledInt,
		&r.CooldownMs, &r.LastTriggered, &tmpl, &cAt, &uAt,
		&notifyRemoteInt,
	)
	if err != nil {
		return nil, err
	}
	r.Description = desc.String
	r.PromQL = promql.String
	r.GroupByTags = groupBy.String
	r.Template = tmpl.String
	r.CreatedAt = cAt.String
	r.UpdatedAt = uAt.String
	r.Enabled = enabledInt == 1
	r.NotifyRemote = notifyRemoteInt == 1

	if tagsJSON.Valid && tagsJSON.String != "" {
		_ = json.Unmarshal([]byte(tagsJSON.String), &r.TagsFilter)
	} else {
		r.TagsFilter = []TagFilterItem{}
	}
	return &r, nil
}

func (repo *alertRuleSQLiteRepo) Create(r AlertRuleRecord) error {
	db := appdb.DB()
	ensureAlertRulesSchema(db)

	tagsBytes, _ := json.Marshal(r.TagsFilter)
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	notifyRemoteInt := 0
	if r.NotifyRemote {
		notifyRemoteInt = 1
	}
	nowStr := time.Now().Format("2006-01-02 15:04:05")

	_, err := db.Exec(`
		INSERT INTO alert_rules (
			id, name, description, severity, metric_name, promql, tags_filter_json,
			operator, threshold, for_duration_sec, group_by_tags, enabled,
			cooldown_ms, last_triggered_at, template, created_at, updated_at, notify_remote
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Description, r.Severity, r.MetricName, r.PromQL, string(tagsBytes),
		r.Operator, r.Threshold, r.ForDurationSec, r.GroupByTags, enabledInt, r.CooldownMs,
		r.LastTriggered, r.Template, nowStr, nowStr, notifyRemoteInt,
	)
	if err != nil {
		logger.Errorf("Create alert rule failed: %v", err)
	}
	return err
}

func (repo *alertRuleSQLiteRepo) Update(id string, r AlertRuleRecord) error {
	db := appdb.DB()
	ensureAlertRulesSchema(db)

	tagsBytes, _ := json.Marshal(r.TagsFilter)
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	notifyRemoteInt := 0
	if r.NotifyRemote {
		notifyRemoteInt = 1
	}
	nowStr := time.Now().Format("2006-01-02 15:04:05")

	_, err := db.Exec(`
		UPDATE alert_rules
		SET name = ?, description = ?, severity = ?, metric_name = ?, promql = ?, tags_filter_json = ?,
		    operator = ?, threshold = ?, for_duration_sec = ?, group_by_tags = ?, enabled = ?,
		    cooldown_ms = ?, template = ?, updated_at = ?, notify_remote = ?
		WHERE id = ?`,
		r.Name, r.Description, r.Severity, r.MetricName, r.PromQL, string(tagsBytes),
		r.Operator, r.Threshold, r.ForDurationSec, r.GroupByTags, enabledInt, r.CooldownMs,
		r.Template, nowStr, notifyRemoteInt, id,
	)
	return err
}

func (repo *alertRuleSQLiteRepo) Delete(id string) error {
	db := appdb.DB()
	_, err := db.Exec(`DELETE FROM alert_rules WHERE id = ?`, id)
	return err
}

func (repo *alertRuleSQLiteRepo) Toggle(id string, enabled bool) error {
	db := appdb.DB()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`UPDATE alert_rules SET enabled = ?, updated_at = ? WHERE id = ?`, enabledInt, nowStr, id)
	return err
}

func (repo *alertRuleSQLiteRepo) UpdateLastTriggered(id string, ts int64) error {
	db := appdb.DB()
	_, err := db.Exec(`UPDATE alert_rules SET last_triggered_at = ? WHERE id = ?`, ts, id)
	return err
}
