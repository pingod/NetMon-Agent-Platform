// 自愈引擎 — 告警 → 策略匹配 → 审批门 → 工作流执行 → 验证 → 回滚
// 匹配入口：告警评估器即时钩子 + 周期兜底扫描（覆盖所有告警写入路径，含重启前遗留）
// 状态机：waiting_approval/approved → running → verifying → success/failed → rolled_back
package ops

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

type remediationPolicy struct {
	ID                      string
	Name                    string
	AlertSource             string
	AlertSeverity           string
	AlertKeywords           []string
	AlertTags               []string
	ExecutionMode           string
	WorkflowID              string
	WorkflowParams          string
	MaxExecutionsPerHour    int
	CooldownSeconds         int
	EnableVerification      bool
	VerificationWorkflowID  string
	VerificationParams      string
	VerificationTimeoutSecs int
	EnableRollback          bool
	RollbackWorkflowID      string
	RollbackOnFailure       bool
}

var remediationDispatchMu sync.Mutex

// StartRemediationEngine 启动引擎：注入任务完成回调 + 周期对账
func StartRemediationEngine() {
	OnTaskFinished = func(taskID string) {
		go finalizeExecutionByTask(taskID)
	}
	// 重启孤儿任务：running 任务没有执行 goroutine，诚实标记失败而不是悄悄重跑副作用节点
	// waiting_approval 保留：审批后仍可恢复执行
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE tasks SET status = 'failed', end_time = ?, updated_at = ? WHERE status IN ('running', 'pending')`, now, now)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			reconcileExecutions()
			expireApprovals()
		}
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			sweepRecentAlerts()
		}
	}()
	logger.Infof("Remediation engine started")
}

// HandleAlertForRemediation 告警产生后的即时匹配入口（异步，不阻塞告警评估）
func HandleAlertForRemediation(info AlertDispatchInfo) {
	go matchAlertToPolicies(info)
}

func normalizeAlertSource(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "promql_alert" {
		return "prometheus"
	}
	return s
}

func matchAlertToPolicies(info AlertDispatchInfo) {
	for _, p := range loadEnabledPolicies() {
		if !policyMatchesAlert(p, info) {
			continue
		}
		if executionExists(p.ID, info.AlertID) {
			continue
		}
		if reason := policyThrottleReason(p); reason != "" {
			logger.Infof("remediation policy %s throttled for alert %s: %s", p.Name, info.AlertID, reason)
			continue
		}
		createExecutionForAlert(p, info)
	}
}

const policySelectCols = `id, name, alert_source, COALESCE(alert_severity,''), COALESCE(alert_keywords,''), COALESCE(alert_tags,''),
	execution_mode, COALESCE(workflow_id,''), COALESCE(workflow_params,'{}'),
	COALESCE(max_executions_per_hour,5), COALESCE(cooldown_seconds,300),
	COALESCE(enable_verification,0), COALESCE(verification_workflow_id,''), COALESCE(verification_params,'{}'),
	COALESCE(verification_timeout_seconds,120), COALESCE(enable_rollback,0), COALESCE(rollback_workflow_id,''),
	COALESCE(rollback_on_failure,1)`

func scanPolicy(scan func(dest ...interface{}) error) (*remediationPolicy, error) {
	var p remediationPolicy
	var keywords, tags string
	var enableVer, enableRb, rbOnFail int
	if err := scan(&p.ID, &p.Name, &p.AlertSource, &p.AlertSeverity, &keywords, &tags,
		&p.ExecutionMode, &p.WorkflowID, &p.WorkflowParams,
		&p.MaxExecutionsPerHour, &p.CooldownSeconds,
		&enableVer, &p.VerificationWorkflowID, &p.VerificationParams,
		&p.VerificationTimeoutSecs, &enableRb, &p.RollbackWorkflowID, &rbOnFail); err != nil {
		return nil, err
	}
	p.AlertKeywords = parseStringArray(keywords)
	p.AlertTags = parseStringArray(tags)
	p.EnableVerification = enableVer == 1
	p.EnableRollback = enableRb == 1
	p.RollbackOnFailure = rbOnFail == 1
	return &p, nil
}

func loadEnabledPolicies() []remediationPolicy {
	rows, err := appdb.DB().Query(`SELECT ` + policySelectCols + ` FROM remediation_policies WHERE enabled = 1`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var list []remediationPolicy
	for rows.Next() {
		if p, err := scanPolicy(rows.Scan); err == nil {
			list = append(list, *p)
		}
	}
	return list
}

func loadPolicyByID(id string) *remediationPolicy {
	p, err := scanPolicy(appdb.DB().QueryRow(`SELECT `+policySelectCols+` FROM remediation_policies WHERE id = ?`, id).Scan)
	if err != nil {
		return nil
	}
	return p
}

func parseStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var arr []string
	if json.Unmarshal([]byte(raw), &arr) == nil {
		return arr
	}
	// 兼容逗号分隔的裸字符串
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func policyMatchesAlert(p remediationPolicy, info AlertDispatchInfo) bool {
	if normalizeAlertSource(p.AlertSource) != normalizeAlertSource(info.Source) {
		return false
	}
	if p.AlertSeverity != "" && !strings.EqualFold(p.AlertSeverity, info.Severity) {
		return false
	}
	if len(p.AlertKeywords) > 0 {
		hay := strings.ToLower(info.Title + " " + info.Message)
		matched := false
		for _, kw := range p.AlertKeywords {
			if kw != "" && strings.Contains(hay, strings.ToLower(kw)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(p.AlertTags) > 0 {
		matched := false
		for _, want := range p.AlertTags {
			want = strings.ToLower(want)
			for k, v := range info.Tags {
				if strings.Contains(strings.ToLower(k), want) || strings.Contains(strings.ToLower(v), want) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func executionExists(policyID, alertID string) bool {
	var n int
	appdb.DB().QueryRow(`SELECT COUNT(*) FROM remediation_executions WHERE policy_id = ? AND alert_id = ?`, policyID, alertID).Scan(&n)
	return n > 0
}

func policyThrottleReason(p remediationPolicy) string {
	db := appdb.DB()
	var hourCount int
	db.QueryRow(`SELECT COUNT(*) FROM remediation_executions
		WHERE policy_id = ? AND status != 'skipped' AND created_at >= datetime('now','localtime','-1 hour')`, p.ID).Scan(&hourCount)
	if p.MaxExecutionsPerHour > 0 && hourCount >= p.MaxExecutionsPerHour {
		return fmt.Sprintf("每小时执行次数已达上限 %d", p.MaxExecutionsPerHour)
	}
	if p.CooldownSeconds > 0 {
		var lastCreated sql.NullString
		db.QueryRow(`SELECT created_at FROM remediation_executions WHERE policy_id = ? AND status != 'skipped' ORDER BY created_at DESC LIMIT 1`, p.ID).Scan(&lastCreated)
		if lastCreated.Valid && lastCreated.String != "" {
			if t, err := appdb.ParseDBTime(lastCreated.String); err == nil {
				if time.Since(t) < time.Duration(p.CooldownSeconds)*time.Second {
					return fmt.Sprintf("冷却中（%ds）", p.CooldownSeconds)
				}
			}
		}
	}
	return ""
}

func createExecutionForAlert(p remediationPolicy, info AlertDispatchInfo) {
	snapshot, _ := json.Marshal(map[string]interface{}{
		"title": info.Title, "message": info.Message, "severity": info.Severity,
		"source": info.Source, "device_id": info.DeviceID,
		"device_name": info.DeviceName, "device_ip": info.DeviceIP,
		"metric_name": info.MetricName, "value": info.Value, "tags": info.Tags,
	})
	status := "waiting_approval"
	reason := ""
	approvalRequired := 1
	switch p.ExecutionMode {
	case "auto":
		status = "approved"
		approvalRequired = 0
	case "suggestion":
		status = "skipped"
		reason = "仅建议模式：已记录匹配，不自动执行"
		approvalRequired = 0
	}
	id := uuid.NewString()
	_, err := appdb.DB().Exec(`
		INSERT INTO remediation_executions (id, policy_id, alert_id, alert_snapshot, status, status_reason, approval_required, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.ID, info.AlertID, string(snapshot), status, reason, approvalRequired, time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		logger.Errorf("创建自愈执行记录失败: %v", err)
		return
	}
	logger.Infof("Remediation execution %s created (policy=%s alert=%s mode=%s)", id, p.Name, info.AlertID, p.ExecutionMode)
	if status == "approved" {
		go dispatchExecution(id)
	}
}

// dispatchExecution 把已批准的执行记录推入工作流执行
func dispatchExecution(execID string) {
	remediationDispatchMu.Lock()
	defer remediationDispatchMu.Unlock()

	db := appdb.DB()
	var policyID, status string
	if db.QueryRow(`SELECT policy_id, status FROM remediation_executions WHERE id = ?`, execID).
		Scan(&policyID, &status) != nil {
		return
	}
	if status != "approved" {
		return
	}
	policy := loadPolicyByID(policyID)
	if policy == nil {
		markExecutionFailed(execID, "策略不存在")
		return
	}
	if policy.WorkflowID == "" {
		markExecutionFailed(execID, "策略未关联工作流")
		return
	}

	taskID, err := createRemediationTask(execID, policy, "自愈-"+policy.Name, policy.WorkflowID, policy.WorkflowParams)
	if err != nil {
		markExecutionFailed(execID, "创建执行任务失败: "+err.Error())
		return
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	db.Exec(`UPDATE remediation_executions SET status = 'running', started_at = ?, workflow_execution_id = ?, status_reason = '' WHERE id = ?`, now, taskID, execID)
	StartTaskExecution(taskID)
}

func markExecutionFailed(execID, reason string) {
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE remediation_executions SET status = 'failed', status_reason = ?, completed_at = ? WHERE id = ?`, reason, now, execID)
}

// finalizeExecutionByTask 任务终态 → 推进执行记录（主流程/验证/回滚三分支）
func finalizeExecutionByTask(taskID string) {
	remediationDispatchMu.Lock()
	defer remediationDispatchMu.Unlock()

	db := appdb.DB()
	var execID, policyID, status string
	var rollbackTaskID, verificationTaskID sql.NullString
	err := db.QueryRow(`SELECT id, policy_id, status, rollback_execution_id, verification_task_id FROM remediation_executions
		WHERE workflow_execution_id = ? OR rollback_execution_id = ? OR verification_task_id = ?`, taskID, taskID, taskID).
		Scan(&execID, &policyID, &status, &rollbackTaskID, &verificationTaskID)
	if err != nil {
		return
	}
	var taskStatus string
	var nodeResults sql.NullString
	var startTime, endTime sql.NullString
	if db.QueryRow(`SELECT status, node_results, start_time, end_time FROM tasks WHERE id = ?`, taskID).
		Scan(&taskStatus, &nodeResults, &startTime, &endTime) != nil {
		return
	}
	if taskStatus != "completed" && taskStatus != "failed" && taskStatus != "cancelled" {
		return
	}
	policy := loadPolicyByID(policyID)
	if policy == nil {
		markExecutionFailed(execID, "策略不存在")
		return
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	taskOK := taskStatus == "completed"

	// 回滚任务终态
	if rollbackTaskID.Valid && rollbackTaskID.String == taskID {
		if taskOK {
			db.Exec(`UPDATE remediation_executions SET status = 'rolled_back', rollback_result = ?, rollback_completed_at = ?, completed_at = ?, status_reason = '修复未达预期，已回滚' WHERE id = ?`, nodeResults.String, now, now, execID)
		} else {
			db.Exec(`UPDATE remediation_executions SET status = 'failed', rollback_result = ?, rollback_completed_at = ?, completed_at = ?, status_reason = '修复失败且回滚未成功，需人工介入' WHERE id = ?`, nodeResults.String, now, now, execID)
		}
		return
	}

	// 验证任务终态
	if verificationTaskID.Valid && verificationTaskID.String == taskID {
		vStatus := "failed"
		if taskOK {
			vStatus = "passed"
		}
		db.Exec(`UPDATE remediation_executions SET verification_status = ?, verification_result = ?, verification_completed_at = ? WHERE id = ?`,
			vStatus, nodeResults.String, now, execID)
		if taskOK {
			db.Exec(`UPDATE remediation_executions SET status = 'success', completed_at = ?, status_reason = '' WHERE id = ?`, now, execID)
		} else if policy.EnableRollback && policy.RollbackWorkflowID != "" {
			startRollback(execID, policy, "验证未通过，触发自动回滚")
		} else {
			db.Exec(`UPDATE remediation_executions SET status = 'failed', completed_at = ?, status_reason = '修复后验证未通过' WHERE id = ?`, now, execID)
		}
		return
	}

	// 主流程终态
	if status != "running" {
		return
	}
	durationMs := 0
	if startTime.Valid && endTime.Valid {
		if s, e1 := appdb.ParseDBTime(startTime.String); e1 == nil {
			if e, e2 := appdb.ParseDBTime(endTime.String); e2 == nil {
				durationMs = int(e.Sub(s).Milliseconds())
			}
		}
	}
	db.Exec(`UPDATE remediation_executions SET execution_result = ?, execution_duration_ms = ? WHERE id = ?`, nodeResults.String, durationMs, execID)

	if taskOK {
		if policy.EnableVerification && policy.VerificationWorkflowID != "" {
			startVerification(execID, policy)
		} else {
			db.Exec(`UPDATE remediation_executions SET status = 'success', completed_at = ?, status_reason = '' WHERE id = ?`, now, execID)
		}
		return
	}
	reason := "修复工作流执行失败"
	if taskStatus == "cancelled" {
		reason = "修复任务被取消"
	}
	if policy.EnableRollback && policy.RollbackOnFailure && policy.RollbackWorkflowID != "" {
		startRollback(execID, policy, reason+"，触发自动回滚")
	} else {
		db.Exec(`UPDATE remediation_executions SET status = 'failed', completed_at = ?, status_reason = ? WHERE id = ?`, now, reason, execID)
	}
}

func startVerification(execID string, p *remediationPolicy) {
	taskID, err := createRemediationTask(execID, p, "验证-"+p.Name, p.VerificationWorkflowID, p.VerificationParams)
	if err != nil {
		markExecutionFailed(execID, "验证任务创建失败: "+err.Error())
		return
	}
	appdb.DB().Exec(`UPDATE remediation_executions SET status = 'verifying', verification_task_id = ? WHERE id = ?`, taskID, execID)
	StartTaskExecution(taskID)
}

func startRollback(execID string, p *remediationPolicy, reason string) {
	taskID, err := createRemediationTask(execID, p, "回滚-"+p.Name, p.RollbackWorkflowID, "{}")
	if err != nil {
		markExecutionFailed(execID, reason+"；回滚任务创建失败: "+err.Error())
		return
	}
	appdb.DB().Exec(`UPDATE remediation_executions SET rollback_triggered = 1, rollback_execution_id = ?, status_reason = ? WHERE id = ?`, taskID, reason, execID)
	StartTaskExecution(taskID)
}

func createRemediationTask(execID string, p *remediationPolicy, name, workflowID, paramsJSON string) (string, error) {
	var snapshotStr sql.NullString
	appdb.DB().QueryRow(`SELECT alert_snapshot FROM remediation_executions WHERE id = ?`, execID).Scan(&snapshotStr)
	alertCtx := map[string]interface{}{}
	if snapshotStr.Valid && snapshotStr.String != "" {
		_ = json.Unmarshal([]byte(snapshotStr.String), &alertCtx)
	}
	params := map[string]interface{}{}
	if paramsJSON != "" {
		_ = json.Unmarshal([]byte(paramsJSON), &params)
	}
	ctxJSON, _ := json.Marshal(map[string]interface{}{"params": params, "alert": alertCtx})
	taskID := uuid.NewString()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`INSERT INTO tasks (id, name, workflow_id, status, start_time, context, created_at, updated_at)
		VALUES (?, ?, ?, 'running', ?, ?, ?, ?)`, taskID, name, workflowID, now, string(ctxJSON), now, now)
	return taskID, err
}

// reconcileExecutions 对账：批准未派发 / 回调丢失 / 任务失踪
func reconcileExecutions() {
	db := appdb.DB()
	rows, err := db.Query(`SELECT id, status, COALESCE(workflow_execution_id,''), COALESCE(rollback_execution_id,''), COALESCE(verification_task_id,'')
		FROM remediation_executions WHERE status IN ('approved','running','verifying')`)
	if err != nil {
		return
	}
	type execRow struct{ id, status, wfTask, rbTask, vTask string }
	var list []execRow
	for rows.Next() {
		var e execRow
		if rows.Scan(&e.id, &e.status, &e.wfTask, &e.rbTask, &e.vTask) == nil {
			list = append(list, e)
		}
	}
	rows.Close()
	for _, e := range list {
		switch {
		case e.status == "approved" && e.wfTask == "":
			go dispatchExecution(e.id)
		case e.status == "running" && e.wfTask != "":
			reconcileTaskLink(e.id, e.wfTask)
		case e.status == "verifying" && e.vTask != "":
			reconcileTaskLink(e.id, e.vTask)
		}
		if e.rbTask != "" {
			reconcileTaskLink(e.id, e.rbTask)
		}
	}
}

func reconcileTaskLink(execID, taskID string) {
	var taskStatus string
	err := appdb.DB().QueryRow(`SELECT status FROM tasks WHERE id = ?`, taskID).Scan(&taskStatus)
	if err != nil {
		markExecutionFailed(execID, "关联任务记录丢失")
		return
	}
	if taskStatus == "completed" || taskStatus == "failed" || taskStatus == "cancelled" {
		finalizeExecutionByTask(taskID)
	}
}

// expireApprovals 工作流审批节点超时处理
func expireApprovals() {
	db := appdb.DB()
	rows, err := db.Query(`SELECT id, task_id, timeout_action FROM approval_requests
		WHERE status = 'pending' AND timeout_at IS NOT NULL AND timeout_at != '' AND timeout_at <= datetime('now','localtime')`)
	if err != nil {
		return
	}
	type apRow struct{ id, taskID, action string }
	var list []apRow
	for rows.Next() {
		var a apRow
		if rows.Scan(&a.id, &a.taskID, &a.action) == nil {
			list = append(list, a)
		}
	}
	rows.Close()
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, a := range list {
		if a.action == "wait" {
			continue
		}
		db.Exec(`UPDATE approval_requests SET status = 'rejected', reject_reason = '审批超时自动拒绝', approved_at = ?, updated_at = ? WHERE id = ?`, now, now, a.id)
		failTask(a.taskID, "审批节点超时自动拒绝")
	}
}

// sweepRecentAlerts 兜底扫描：近 10 分钟新告警（覆盖 influxdb/自定义等所有写入路径）
func sweepRecentAlerts() {
	rows, err := appdb.DB().Query(`SELECT id, source, severity, title, COALESCE(content,''), COALESCE(metadata,'{}'), COALESCE(device_id,'')
		FROM alerts WHERE created_at >= datetime('now','localtime','-10 minutes') AND status IN ('new','firing')`)
	if err != nil {
		return
	}
	defer rows.Close()
	var infos []AlertDispatchInfo
	for rows.Next() {
		var id, source, severity, title, content, metaStr, deviceID string
		if rows.Scan(&id, &source, &severity, &title, &content, &metaStr, &deviceID) != nil {
			continue
		}
		meta := map[string]interface{}{}
		_ = json.Unmarshal([]byte(metaStr), &meta)
		info := AlertDispatchInfo{
			AlertID: id, Source: source, Title: title, Message: content, Severity: severity,
			DeviceID: deviceID, Status: "firing",
		}
		if v, ok := meta["device_name"].(string); ok {
			info.DeviceName = v
		}
		if v, ok := meta["device_ip"].(string); ok {
			info.DeviceIP = v
		}
		if v, ok := meta["metric_name"].(string); ok {
			info.MetricName = v
		}
		if tags, ok := meta["tags"].(map[string]interface{}); ok {
			info.Tags = map[string]string{}
			for k, v := range tags {
				info.Tags[k] = wfStr(v)
			}
		}
		infos = append(infos, info)
	}
	for _, info := range infos {
		matchAlertToPolicies(info)
	}
}