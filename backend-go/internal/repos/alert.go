// 告警数据访问层 — 查询 alerts / alert_auto_analysis / alert_correlation_*
// / alert_noise_reduction / alert_workflow_mappings / alert_provider_configs 等表
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"itops-backend/internal/appdb"
)

// ── 告警核心 ──

type Alert struct {
	ID               string  `json:"id"`
	Source           string  `json:"source"`
	Severity         string  `json:"severity"`
	Title            string  `json:"title"`
	Content          *string `json:"content"`
	Metadata         *string `json:"metadata"`
	RelatedTaskID    *string `json:"related_task_id"`
	Status           string  `json:"status"`
	AlertFingerprint *string `json:"alert_fingerprint"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	// 连接字段（前端需要）
	SourceName string `json:"source_name,omitempty"`
}

type AlertFilter struct {
	Status   string
	Severity string
	Source   string
	Limit    int
	Offset   int
}

var Alerts = &AlertRepo{}

type AlertRepo struct{}

func (AlertRepo) List(f AlertFilter) ([]Alert, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	q := "SELECT id, source, severity, title, content, metadata, related_task_id, status, alert_fingerprint, created_at, updated_at FROM alerts WHERE 1=1"
	args := []interface{}{}

	if f.Status != "" {
		q += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Severity != "" {
		q += " AND severity = ?"
		args = append(args, f.Severity)
	}
	if f.Source != "" {
		q += " AND source = ?"
		args = append(args, f.Source)
	}

	q += " ORDER BY created_at DESC"

	if f.Limit <= 0 {
		f.Limit = 50
	}
	q += " LIMIT ?"
	args = append(args, f.Limit)

	if f.Offset > 0 {
		q += " OFFSET ?"
		args = append(args, f.Offset)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var content, metadata, relatedTaskID, fingerprint sql.NullString
		err := rows.Scan(&a.ID, &a.Source, &a.Severity, &a.Title, &content, &metadata, &relatedTaskID, &a.Status, &fingerprint, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			continue
		}
		a.Content = nullStrToPtr(content)
		a.Metadata = nullStrToPtr(metadata)
		a.RelatedTaskID = nullStrToPtr(relatedTaskID)
		a.AlertFingerprint = nullStrToPtr(fingerprint)
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func (AlertRepo) GetByID(id string) (*Alert, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var a Alert
	var content, metadata, relatedTaskID, fingerprint sql.NullString
	err := db.QueryRow("SELECT id, source, severity, title, content, metadata, related_task_id, status, alert_fingerprint, created_at, updated_at FROM alerts WHERE id = ?", id).
		Scan(&a.ID, &a.Source, &a.Severity, &a.Title, &content, &metadata, &relatedTaskID, &a.Status, &fingerprint, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Content = nullStrToPtr(content)
	a.Metadata = nullStrToPtr(metadata)
	a.RelatedTaskID = nullStrToPtr(relatedTaskID)
	a.AlertFingerprint = nullStrToPtr(fingerprint)
	return &a, nil
}

func (AlertRepo) UpdateStatus(id, status string) error {
	db := appdb.DB()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE alerts SET status = ?, updated_at = ? WHERE id = ?", status, now, id)
	return err
}

// ── 自动分析 ──

type AlertAutoAnalysis struct {
	ID               string  `json:"id"`
	AlertID          string  `json:"alert_id"`
	DeviceID         string  `json:"device_id"`
	DeviceName       string  `json:"device_name"`
	DeviceIP         string  `json:"device_ip"`
	DeviceType       string  `json:"device_type"`
	Status           string  `json:"status"`
	Diagnosis        *string `json:"diagnosis"`
	Summary          *string `json:"summary"`
	RawOutput        *string `json:"raw_output"`
	CommandsExecuted *string `json:"commands_executed"`
	ErrorMessage     *string `json:"error_message"`
	DurationMs       int     `json:"duration_ms"`
	CreatedAt        string  `json:"created_at"`
}

var AlertAutoAnalyses = &AlertAutoAnalysisRepo{}

type AlertAutoAnalysisRepo struct{}

func (AlertAutoAnalysisRepo) List(limit int, alertID string) ([]AlertAutoAnalysis, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	q := "SELECT id, alert_id, device_id, device_name, device_ip, device_type, status, diagnosis, summary, raw_output, commands_executed, error_message, duration_ms, created_at FROM alert_auto_analysis WHERE 1=1"
	args := []interface{}{}

	if alertID != "" {
		q += " AND alert_id = ?"
		args = append(args, alertID)
	}

	q += " ORDER BY created_at DESC"

	if limit <= 0 {
		limit = 50
	}
	q += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AlertAutoAnalysis
	for rows.Next() {
		var a AlertAutoAnalysis
		var diagnosis, summary, rawOutput, commandsExecuted, errMsg sql.NullString
		err := rows.Scan(&a.ID, &a.AlertID, &a.DeviceID, &a.DeviceName, &a.DeviceIP, &a.DeviceType, &a.Status, &diagnosis, &summary, &rawOutput, &commandsExecuted, &errMsg, &a.DurationMs, &a.CreatedAt)
		if err != nil {
			continue
		}
		a.Diagnosis = nullStrToPtr(diagnosis)
		a.Summary = nullStrToPtr(summary)
		a.RawOutput = nullStrToPtr(rawOutput)
		a.CommandsExecuted = nullStrToPtr(commandsExecuted)
		a.ErrorMessage = nullStrToPtr(errMsg)
		items = append(items, a)
	}
	return items, nil
}

// ── 告警关联 ──

type CorrelationGroup struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	Status       string  `json:"status"`
	RootAlertID  *string `json:"root_alert_id"`
	RootCause    *string `json:"root_cause"`
	AlertCount   int     `json:"alert_count"`
	DeviceIDs    *string `json:"device_ids"`
	Severity     string  `json:"severity"`
	AutoDetected bool    `json:"auto_detected"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ResolvedAt   *string `json:"resolved_at"`
}

type CorrelationStats struct {
	TotalGroups    int     `json:"total_groups"`
	OpenGroups     int     `json:"open_groups"`
	ResolvedGroups int     `json:"resolved_groups"`
	AutoDetected   int     `json:"auto_detected"`
	AvgGroupSize   float64 `json:"avg_group_size"`
}

var AlertCorrelations = &AlertCorrelationRepo{}

type AlertCorrelationRepo struct{}

func (AlertCorrelationRepo) ListGroups(status string, limit int) ([]CorrelationGroup, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	q := "SELECT id, title, description, status, root_alert_id, root_cause, alert_count, device_ids, severity, auto_detected, created_at, updated_at, resolved_at FROM alert_correlation_groups WHERE 1=1"
	args := []interface{}{}

	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}

	q += " ORDER BY created_at DESC"

	if limit <= 0 {
		limit = 50
	}
	q += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []CorrelationGroup
	for rows.Next() {
		var g CorrelationGroup
		var desc, rootAlertID, rootCause, deviceIDs, resolvedAt sql.NullString
		var autoDet int
		err := rows.Scan(&g.ID, &g.Title, &desc, &g.Status, &rootAlertID, &rootCause, &g.AlertCount, &deviceIDs, &g.Severity, &autoDet, &g.CreatedAt, &g.UpdatedAt, &resolvedAt)
		if err != nil {
			continue
		}
		g.Description = nullStrToPtr(desc)
		g.RootAlertID = nullStrToPtr(rootAlertID)
		g.RootCause = nullStrToPtr(rootCause)
		g.DeviceIDs = nullStrToPtr(deviceIDs)
		g.ResolvedAt = nullStrToPtr(resolvedAt)
		g.AutoDetected = autoDet == 1
		groups = append(groups, g)
	}
	return groups, nil
}

func (AlertCorrelationRepo) GetGroupByID(id string) (*CorrelationGroup, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var g CorrelationGroup
	var desc, rootAlertID, rootCause, deviceIDs, resolvedAt sql.NullString
	var autoDet int
	err := db.QueryRow("SELECT id, title, description, status, root_alert_id, root_cause, alert_count, device_ids, severity, auto_detected, created_at, updated_at, resolved_at FROM alert_correlation_groups WHERE id = ?", id).
		Scan(&g.ID, &g.Title, &desc, &g.Status, &rootAlertID, &rootCause, &g.AlertCount, &deviceIDs, &g.Severity, &autoDet, &g.CreatedAt, &g.UpdatedAt, &resolvedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Description = nullStrToPtr(desc)
	g.RootAlertID = nullStrToPtr(rootAlertID)
	g.RootCause = nullStrToPtr(rootCause)
	g.DeviceIDs = nullStrToPtr(deviceIDs)
	g.ResolvedAt = nullStrToPtr(resolvedAt)
	g.AutoDetected = autoDet == 1
	return &g, nil
}

func (AlertCorrelationRepo) DeleteGroup(id string) error {
	db := appdb.DB()
	_, err := db.Exec("DELETE FROM alert_correlation_groups WHERE id = ?", id)
	return err
}

func (AlertCorrelationRepo) ResolveGroup(id string) error {
	db := appdb.DB()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE alert_correlation_groups SET status = 'resolved', resolved_at = ?, updated_at = ? WHERE id = ?", now, now, id)
	return err
}

func (AlertCorrelationRepo) GetStats() (*CorrelationStats, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	s := &CorrelationStats{}
	db.QueryRow("SELECT COUNT(*) FROM alert_correlation_groups").Scan(&s.TotalGroups)
	db.QueryRow("SELECT COUNT(*) FROM alert_correlation_groups WHERE status = 'open'").Scan(&s.OpenGroups)
	db.QueryRow("SELECT COUNT(*) FROM alert_correlation_groups WHERE status = 'resolved'").Scan(&s.ResolvedGroups)
	db.QueryRow("SELECT COUNT(*) FROM alert_correlation_groups WHERE auto_detected = 1").Scan(&s.AutoDetected)

	var total int
	db.QueryRow("SELECT COALESCE(SUM(alert_count), 0) FROM alert_correlation_groups").Scan(&total)
	if s.TotalGroups > 0 {
		s.AvgGroupSize = float64(total) / float64(s.TotalGroups)
	}
	return s, nil
}

func (AlertCorrelationRepo) GetGroupMembers(groupID string) ([]map[string]interface{}, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query("SELECT id, group_id, alert_id, is_root, created_at FROM alert_correlation_members WHERE group_id = ?", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []map[string]interface{}
	for rows.Next() {
		var id, groupID, alertID, createdAt string
		var isRoot int
		if err := rows.Scan(&id, &groupID, &alertID, &isRoot, &createdAt); err != nil {
			continue
		}
		members = append(members, map[string]interface{}{
			"id":         id,
			"group_id":   groupID,
			"alert_id":   alertID,
			"is_root":    isRoot == 1,
			"created_at": createdAt,
		})
	}
	return members, nil
}

// ── 告警映射 (alert_workflow_mappings) ──

type AlertMapping struct {
	ID                string  `json:"id"`
	AlertSource       *string `json:"alert_source"`
	AlertSeverity     *string `json:"alert_severity"`
	AlertTitlePattern *string `json:"alert_title_pattern"`
	WorkflowID        string  `json:"workflow_id"`
	Enabled           int     `json:"enabled"`
	CreatedAt         string  `json:"created_at"`
	// join 字段
	WorkflowName *string `json:"workflow_name,omitempty"`
}

var AlertMappings = &AlertMappingRepo{}

type AlertMappingRepo struct{}

func (AlertMappingRepo) List() ([]AlertMapping, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query("SELECT id, alert_source, alert_severity, alert_title_pattern, workflow_id, enabled, created_at FROM alert_workflow_mappings ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AlertMapping
	for rows.Next() {
		var m AlertMapping
		var source, severity, titlePattern sql.NullString
		err := rows.Scan(&m.ID, &source, &severity, &titlePattern, &m.WorkflowID, &m.Enabled, &m.CreatedAt)
		if err != nil {
			continue
		}
		m.AlertSource = nullStrToPtr(source)
		m.AlertSeverity = nullStrToPtr(severity)
		m.AlertTitlePattern = nullStrToPtr(titlePattern)
		items = append(items, m)
	}
	return items, nil
}

func (AlertMappingRepo) Create(m AlertMapping) error {
	db := appdb.DB()
	_, err := db.Exec("INSERT INTO alert_workflow_mappings (id, alert_source, alert_severity, alert_title_pattern, workflow_id, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?, "+appdb.NowExpr()+")",
		m.ID, ptrToNullStr(m.AlertSource), ptrToNullStr(m.AlertSeverity), ptrToNullStr(m.AlertTitlePattern), m.WorkflowID, m.Enabled)
	return err
}

func (AlertMappingRepo) Update(id string, m AlertMapping) error {
	db := appdb.DB()
	_, err := db.Exec("UPDATE alert_workflow_mappings SET alert_source = ?, alert_severity = ?, alert_title_pattern = ?, workflow_id = ?, enabled = ? WHERE id = ?",
		ptrToNullStr(m.AlertSource), ptrToNullStr(m.AlertSeverity), ptrToNullStr(m.AlertTitlePattern), m.WorkflowID, m.Enabled, id)
	return err
}

func (AlertMappingRepo) Delete(id string) error {
	db := appdb.DB()
	_, err := db.Exec("DELETE FROM alert_workflow_mappings WHERE id = ?", id)
	return err
}

// ── 告警降噪 (alert_noise_reduction) ──

type NoiseReduction struct {
	ID                string  `json:"id"`
	AlertFingerprint  string  `json:"alert_fingerprint"`
	AlertSource       string  `json:"alert_source"`
	AlertTitle        string  `json:"alert_title"`
	OccurrenceCount   int     `json:"occurrence_count"`
	FirstOccurrence   string  `json:"first_occurrence"`
	LastOccurrence    string  `json:"last_occurrence"`
	IsSuppressed      int     `json:"is_suppressed"`
	SuppressionReason *string `json:"suppression_reason"`
	SuppressionUntil  *string `json:"suppression_until"`
}

var AlertNoise = &AlertNoiseRepo{}

type AlertNoiseRepo struct{}

func (AlertNoiseRepo) GetStats() (map[string]interface{}, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var total, suppressed, duplicateCount int
	db.QueryRow("SELECT COUNT(*) FROM alert_noise_reduction").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM alert_noise_reduction WHERE is_suppressed = 1").Scan(&suppressed)
	db.QueryRow("SELECT COALESCE(SUM(occurrence_count - 1), 0) FROM alert_noise_reduction").Scan(&duplicateCount)

	rate := 0.0
	if total > 0 {
		rate = float64(suppressed) / float64(total) * 100
	}

	return map[string]interface{}{
		"totalAlerts":        total,
		"suppressedAlerts":   suppressed,
		"duplicateCount":     duplicateCount,
		"noiseReductionRate": rate,
	}, nil
}

func (AlertNoiseRepo) ListSuppressed() ([]NoiseReduction, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query("SELECT id, alert_fingerprint, alert_source, alert_title, occurrence_count, first_occurrence, last_occurrence, is_suppressed, suppression_reason, suppression_until FROM alert_noise_reduction WHERE is_suppressed = 1 ORDER BY last_occurrence DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NoiseReduction
	for rows.Next() {
		var n NoiseReduction
		var reason, until sql.NullString
		err := rows.Scan(&n.ID, &n.AlertFingerprint, &n.AlertSource, &n.AlertTitle, &n.OccurrenceCount, &n.FirstOccurrence, &n.LastOccurrence, &n.IsSuppressed, &reason, &until)
		if err != nil {
			continue
		}
		n.SuppressionReason = nullStrToPtr(reason)
		n.SuppressionUntil = nullStrToPtr(until)
		items = append(items, n)
	}
	return items, nil
}

func (AlertNoiseRepo) Suppress(fingerprint, reason string, durationMinutes int) error {
	db := appdb.DB()
	minutes := durationMinutes
	if minutes <= 0 {
		minutes = 1440
	}
	untilStr := time.Now().Add(time.Duration(minutes) * time.Minute).Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE alert_noise_reduction SET is_suppressed = 1, suppression_reason = ?, suppression_until = ? WHERE alert_fingerprint = ?", reason, untilStr, fingerprint)
	return err
}

func (AlertNoiseRepo) Unsuppress(fingerprint string) error {
	db := appdb.DB()
	_, err := db.Exec("UPDATE alert_noise_reduction SET is_suppressed = 0, suppression_reason = NULL, suppression_until = NULL WHERE alert_fingerprint = ?", fingerprint)
	return err
}

func (AlertNoiseRepo) Cleanup(daysToKeep int) error {
	db := appdb.DB()
	if daysToKeep <= 0 {
		daysToKeep = 30
	}
	cutoff := time.Now().AddDate(0, 0, -daysToKeep).Format("2006-01-02 15:04:05")
	_, err := db.Exec("DELETE FROM alert_noise_reduction WHERE last_occurrence < ?", cutoff)
	return err
}

// ── 告警源配置 (alert_provider_configs) ──

type AlertProviderConfig struct {
	ID         string  `json:"id"`
	ProviderID string  `json:"provider_id"`
	Name       string  `json:"name"`
	Config     *string `json:"config"`
	Enabled    int     `json:"enabled"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

var AlertProviderConfigs = &AlertProviderConfigRepo{}

type AlertProviderConfigRepo struct{}

func (AlertProviderConfigRepo) List() ([]AlertProviderConfig, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query("SELECT id, provider_id, name, config, enabled, created_at, updated_at FROM alert_provider_configs ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AlertProviderConfig
	for rows.Next() {
		var c AlertProviderConfig
		var config sql.NullString
		err := rows.Scan(&c.ID, &c.ProviderID, &c.Name, &config, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		c.Config = nullStrToPtr(config)
		items = append(items, c)
	}
	return items, nil
}

func (AlertProviderConfigRepo) Create(c AlertProviderConfig) error {
	db := appdb.DB()
	configJSON := ptrToNullStr(c.Config)
	_, err := db.Exec("INSERT INTO alert_provider_configs (id, provider_id, name, config, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, "+appdb.NowExpr()+", "+appdb.NowExpr()+")",
		c.ID, c.ProviderID, c.Name, configJSON, c.Enabled)
	return err
}

func (AlertProviderConfigRepo) Update(id string, c AlertProviderConfig) error {
	db := appdb.DB()
	configJSON := ptrToNullStr(c.Config)
	_, err := db.Exec("UPDATE alert_provider_configs SET provider_id = ?, name = ?, config = ?, enabled = ?, updated_at = "+appdb.NowExpr()+" WHERE id = ?",
		c.ProviderID, c.Name, configJSON, c.Enabled, id)
	return err
}

func (AlertProviderConfigRepo) Delete(id string) error {
	db := appdb.DB()
	_, err := db.Exec("DELETE FROM alert_provider_configs WHERE id = ?", id)
	return err
}

// ── 自动响应日志 (aars_response_logs) ──

type AarsResponseLog struct {
	ID                 string  `json:"id"`
	AlertID            string  `json:"alert_id"`
	DeviceID           *string `json:"device_id"`
	DeviceType         *string `json:"device_type"`
	AccessMethod       *string `json:"access_method"`
	Status             string  `json:"status"`
	ProbesUsed         *string `json:"probes_used"`
	DiagnosisResult    *string `json:"diagnosis_result"`
	RootCause          *string `json:"root_cause"`
	RemediationPlan    *string `json:"remediation_plan"`
	VerificationResult *string `json:"verification_result"`
	ExecutionStatus    *string `json:"execution_status"`
	ApprovalStatus     string  `json:"approval_status"`
	ErrorMessage       *string `json:"error_message"`
	StartedAt          string  `json:"started_at"`
	CompletedAt        *string `json:"completed_at"`
}

var AarsResponseLogs = &AarsResponseLogRepo{}

type AarsResponseLogRepo struct{}

func (AarsResponseLogRepo) ListByAlert(alertID string) ([]AarsResponseLog, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT id, alert_id, device_id, device_type, access_method, status, probes_used, 
		diagnosis_result, root_cause, remediation_plan, verification_result, execution_status, 
		approval_status, error_message, started_at, completed_at 
		FROM aars_response_logs WHERE alert_id = ? ORDER BY started_at DESC`, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AarsResponseLog
	for rows.Next() {
		var l AarsResponseLog
		var deviceID, deviceType, accessMethod, probesUsed, diagResult, rootCause, remPlan, verResult, execStatus, errMsg, completedAt sql.NullString
		err := rows.Scan(&l.ID, &l.AlertID, &deviceID, &deviceType, &accessMethod, &l.Status, &probesUsed,
			&diagResult, &rootCause, &remPlan, &verResult, &execStatus,
			&l.ApprovalStatus, &errMsg, &l.StartedAt, &completedAt)
		if err != nil {
			continue
		}
		l.DeviceID = nullStrToPtr(deviceID)
		l.DeviceType = nullStrToPtr(deviceType)
		l.AccessMethod = nullStrToPtr(accessMethod)
		l.ProbesUsed = nullStrToPtr(probesUsed)
		l.DiagnosisResult = nullStrToPtr(diagResult)
		l.RootCause = nullStrToPtr(rootCause)
		l.RemediationPlan = nullStrToPtr(remPlan)
		l.VerificationResult = nullStrToPtr(verResult)
		l.ExecutionStatus = nullStrToPtr(execStatus)
		l.ErrorMessage = nullStrToPtr(errMsg)
		l.CompletedAt = nullStrToPtr(completedAt)
		items = append(items, l)
	}
	return items, nil
}

// ── 辅助函数 ──

func nullStrToPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

func ptrToNullStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// EnsureJSONMap 辅助：将可能为 nil 的 JSON 数据安全返回为空 map
func EnsureJSONMap(s *string) map[string]interface{} {
	if s == nil || *s == "" {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(*s), &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}
