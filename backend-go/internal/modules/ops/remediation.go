// 自愈中心路由 — /api/v1/remediation-policies, /api/v1/remediation-executions, /api/v1/remediation-audits
// 策略 CRUD 对齐 RemediationPolicyEditor；执行记录对齐 RemediationExecutions；
// audits 是执行记录的工作台视图（审批/手动执行/回滚/验证）
package ops

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/services/audit"
)

var remediationModes = map[string]bool{"auto": true, "approval": true, "suggestion": true}

// ── 策略 ──

type policyBody struct {
	Name                    string      `json:"name"`
	Description             string      `json:"description"`
	AlertSource             string      `json:"alert_source"`
	AlertSeverity           string      `json:"alert_severity"`
	AlertKeywords           interface{} `json:"alert_keywords"`
	AlertTags               interface{} `json:"alert_tags"`
	ExecutionMode           string      `json:"execution_mode"`
	WorkflowID              string      `json:"workflow_id"`
	WorkflowParams          interface{} `json:"workflow_params"`
	MaxExecutionsPerHour    *int        `json:"max_executions_per_hour"`
	CooldownSeconds         *int        `json:"cooldown_seconds"`
	EnableVerification      *bool       `json:"enable_verification"`
	VerificationWorkflowID  string      `json:"verification_workflow_id"`
	VerificationParams      interface{} `json:"verification_params"`
	VerificationTimeoutSecs *int        `json:"verification_timeout_seconds"`
	EnableRollback          *bool       `json:"enable_rollback"`
	RollbackWorkflowID      string      `json:"rollback_workflow_id"`
	RollbackOnFailure       *bool       `json:"rollback_on_failure"`
	Enabled                 *bool       `json:"enabled"`
}

func boolPtrInt(b *bool, def int) int {
	if b == nil {
		return def
	}
	if *b {
		return 1
	}
	return 0
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// normalizeStringArray 接受 JSON 数组字符串 / 数组 / 逗号分隔字符串 / null，返回存储用 JSON 字符串
func normalizeStringArray(v interface{}) (string, error) {
	if v == nil {
		return "", nil
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" || s == "null" {
			return "", nil
		}
		var arr []string
		if json.Unmarshal([]byte(s), &arr) == nil {
			out, _ := json.Marshal(arr)
			return string(out), nil
		}
		for _, part := range strings.Split(s, ",") {
			if p := strings.TrimSpace(part); p != "" {
				arr = append(arr, p)
			}
		}
		out, _ := json.Marshal(arr)
		return string(out), nil
	case []interface{}:
		var arr []string
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("数组元素必须是字符串")
			}
			if s = strings.TrimSpace(s); s != "" {
				arr = append(arr, s)
			}
		}
		out, _ := json.Marshal(arr)
		return string(out), nil
	default:
		return "", fmt.Errorf("必须是字符串数组或逗号分隔字符串")
	}
}

// normalizeParams 接受 JSON 对象字符串 / 对象 / null，返回存储用 JSON 字符串
func normalizeParams(v interface{}) (string, error) {
	if v == nil {
		return "{}", nil
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" || s == "null" {
			return "{}", nil
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(s), &m) != nil {
			return "", fmt.Errorf("参数必须是 JSON 对象")
		}
		out, _ := json.Marshal(m)
		return string(out), nil
	case map[string]interface{}:
		out, _ := json.Marshal(val)
		return string(out), nil
	default:
		return "", fmt.Errorf("参数必须是 JSON 对象")
	}
}

func (b *policyBody) validate() string {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len([]rune(b.Name)) > 100 {
		return "策略名称必填且不超过 100 字"
	}
	if len([]rune(b.Description)) > 500 {
		return "描述不超过 500 字"
	}
	b.AlertSource = strings.TrimSpace(b.AlertSource)
	if b.AlertSource == "" || len(b.AlertSource) > 50 {
		return "告警来源必填"
	}
	if len(b.AlertSeverity) > 20 {
		return "告警级别不合法"
	}
	if !remediationModes[b.ExecutionMode] {
		return "执行模式必须是 auto/approval/suggestion"
	}
	if b.WorkflowID != "" {
		var n int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM workflows WHERE id = ?`, b.WorkflowID).Scan(&n)
		if n == 0 {
			return "关联工作流不存在"
		}
	}
	wp, err := normalizeParams(b.WorkflowParams)
	if err != nil {
		return "工作流参数必须是 JSON 对象"
	}
	b.WorkflowParams = wp
	vp, err := normalizeParams(b.VerificationParams)
	if err != nil {
		return "验证参数必须是 JSON 对象"
	}
	b.VerificationParams = vp
	if b.EnableVerification != nil && *b.EnableVerification {
		if b.VerificationWorkflowID == "" {
			return "启用验证需选择验证工作流"
		}
		var n int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM workflows WHERE id = ?`, b.VerificationWorkflowID).Scan(&n)
		if n == 0 {
			return "验证工作流不存在"
		}
	}
	if b.EnableRollback != nil && *b.EnableRollback {
		if b.RollbackWorkflowID == "" {
			return "启用回滚需选择回滚工作流"
		}
		var n int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM workflows WHERE id = ?`, b.RollbackWorkflowID).Scan(&n)
		if n == 0 {
			return "回滚工作流不存在"
		}
	}
	if v := intOr(b.MaxExecutionsPerHour, 5); v < 1 || v > 1000 {
		return "每小时最大执行次数需在 1-1000 之间"
	}
	if v := intOr(b.CooldownSeconds, 300); v < 0 || v > 86400 {
		return "冷却时间需在 0-86400 秒之间"
	}
	if v := intOr(b.VerificationTimeoutSecs, 120); v < 1 || v > 3600 {
		return "验证超时需在 1-3600 秒之间"
	}
	return ""
}

func policyToMap(row scanFunc) (map[string]interface{}, error) {
	var id, name, alertSource, execMode, createdAt, updatedAt string
	var desc, alertSev, keywords, tags, workflowID, workflowParams, verWfID, verParams, rbWfID, createdBy sql.NullString
	var maxExec, cooldown, enableVer, verTimeout, enableRb, rbOnFail, enabled int
	if err := row(&id, &name, &desc, &alertSource, &alertSev, &keywords, &tags, &execMode,
		&workflowID, &workflowParams, &maxExec, &cooldown, &enableVer, &verWfID, &verParams,
		&verTimeout, &enableRb, &rbWfID, &rbOnFail, &enabled, &createdBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id": id, "name": name, "description": desc.String,
		"alert_source": alertSource, "alert_severity": alertSev.String,
		"alert_keywords": keywords.String, "alert_tags": tags.String,
		"execution_mode": execMode, "workflow_id": workflowID.String,
		"workflow_params": workflowParams.String,
		"max_executions_per_hour": maxExec, "cooldown_seconds": cooldown,
		"enable_verification": enableVer, "verification_workflow_id": verWfID.String,
		"verification_params": verParams.String, "verification_timeout_seconds": verTimeout,
		"enable_rollback": enableRb, "rollback_workflow_id": rbWfID.String,
		"rollback_on_failure": rbOnFail, "enabled": enabled,
		"created_by": createdBy.String, "created_at": createdAt, "updated_at": updatedAt,
	}, nil
}

type scanFunc func(dest ...interface{}) error

const policyAllCols = `id, name, description, alert_source, alert_severity, alert_keywords, alert_tags, execution_mode,
	workflow_id, workflow_params, max_executions_per_hour, cooldown_seconds, enable_verification, verification_workflow_id,
	verification_params, verification_timeout_seconds, enable_rollback, rollback_workflow_id, rollback_on_failure, enabled,
	created_by, created_at, updated_at`

// RemediationPolicyRoutes 自愈策略路由 (/api/v1/remediation-policies)
func RemediationPolicyRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 20
		}
		where := "1=1"
		var args []interface{}
		if e := q.Get("enabled"); e == "true" || e == "false" {
			where += " AND enabled = ?"
			if e == "true" {
				args = append(args, 1)
			} else {
				args = append(args, 0)
			}
		}
		var total int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM remediation_policies WHERE `+where, args...).Scan(&total)
		args = append(args, limit, (page-1)*limit)
		rows, err := appdb.DB().Query(`SELECT `+policyAllCols+` FROM remediation_policies WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, args...)
		if err != nil {
			httpx.Respond(w, map[string]interface{}{"policies": []interface{}{}, "total": 0})
			return
		}
		defer rows.Close()
		list := []map[string]interface{}{}
		for rows.Next() {
			if m, err := policyToMap(rows.Scan); err == nil {
				list = append(list, m)
			}
		}
		httpx.Respond(w, map[string]interface{}{"policies": list, "total": total})
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		m, err := policyToMap(appdb.DB().QueryRow(`SELECT `+policyAllCols+` FROM remediation_policies WHERE id = ?`, chi.URLParam(r, "id")).Scan)
		if err != nil {
			httpx.RespondError(w, "策略不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, m)
	})

	r.Get("/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days < 1 || days > 90 {
			days = 7
		}
		var total, success int
		var avgMs float64
		appdb.DB().QueryRow(`SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status='success' THEN 1 ELSE 0 END),0),
			COALESCE(AVG(CASE WHEN execution_duration_ms > 0 THEN execution_duration_ms END),0)
			FROM remediation_executions WHERE policy_id = ? AND created_at >= datetime('now','localtime',?)`,
			chi.URLParam(r, "id"), fmt.Sprintf("-%d days", days)).Scan(&total, &success, &avgMs)
		rate := 0.0
		if total > 0 {
			rate = float64(success) / float64(total) * 100
		}
		httpx.Respond(w, map[string]interface{}{
			"total_triggers": total, "success_rate": rate, "avg_duration_ms": avgMs,
		})
	})

	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var body policyBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&body); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		if msg := body.validate(); msg != "" {
			httpx.RespondError(w, msg, http.StatusBadRequest)
			return
		}
		keywords, err := normalizeStringArray(body.AlertKeywords)
		if err != nil {
			httpx.RespondError(w, "关键词格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
		tags, err := normalizeStringArray(body.AlertTags)
		if err != nil {
			httpx.RespondError(w, "标签格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")
		user := middleware.GetUser(r)
		_, err = appdb.DB().Exec(`
			INSERT INTO remediation_policies (id, name, description, alert_source, alert_severity, alert_keywords, alert_tags,
				execution_mode, workflow_id, workflow_params, max_executions_per_hour, cooldown_seconds,
				enable_verification, verification_workflow_id, verification_params, verification_timeout_seconds,
				enable_rollback, rollback_workflow_id, rollback_on_failure, enabled, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, body.Name, body.Description, body.AlertSource, body.AlertSeverity, keywords, tags,
			body.ExecutionMode, body.WorkflowID, body.WorkflowParams,
			intOr(body.MaxExecutionsPerHour, 5), intOr(body.CooldownSeconds, 300),
			boolPtrInt(body.EnableVerification, 0), body.VerificationWorkflowID, body.VerificationParams,
			intOr(body.VerificationTimeoutSecs, 120),
			boolPtrInt(body.EnableRollback, 0), body.RollbackWorkflowID, boolPtrInt(body.RollbackOnFailure, 1),
			boolPtrInt(body.Enabled, 1), user.ID, now, now)
		if err != nil {
			httpx.RespondError(w, "创建策略失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_policy_create", ResourceType: "remediation_policy", ResourceID: id, Details: map[string]interface{}{"name": body.Name}, IPAddress: middleware.ClientIP(r)})
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id, "name": body.Name})
	})

	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var n int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM remediation_policies WHERE id = ?`, id).Scan(&n)
		if n == 0 {
			httpx.RespondError(w, "策略不存在", http.StatusNotFound)
			return
		}
		var body policyBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&body); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		if msg := body.validate(); msg != "" {
			httpx.RespondError(w, msg, http.StatusBadRequest)
			return
		}
		keywords, err := normalizeStringArray(body.AlertKeywords)
		if err != nil {
			httpx.RespondError(w, "关键词格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
		tags, err := normalizeStringArray(body.AlertTags)
		if err != nil {
			httpx.RespondError(w, "标签格式错误: "+err.Error(), http.StatusBadRequest)
			return
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		_, err = appdb.DB().Exec(`
			UPDATE remediation_policies SET name=?, description=?, alert_source=?, alert_severity=?, alert_keywords=?, alert_tags=?,
				execution_mode=?, workflow_id=?, workflow_params=?, max_executions_per_hour=?, cooldown_seconds=?,
				enable_verification=?, verification_workflow_id=?, verification_params=?, verification_timeout_seconds=?,
				enable_rollback=?, rollback_workflow_id=?, rollback_on_failure=?, enabled=?, updated_at=?
			WHERE id=?`,
			body.Name, body.Description, body.AlertSource, body.AlertSeverity, keywords, tags,
			body.ExecutionMode, body.WorkflowID, body.WorkflowParams,
			intOr(body.MaxExecutionsPerHour, 5), intOr(body.CooldownSeconds, 300),
			boolPtrInt(body.EnableVerification, 0), body.VerificationWorkflowID, body.VerificationParams,
			intOr(body.VerificationTimeoutSecs, 120),
			boolPtrInt(body.EnableRollback, 0), body.RollbackWorkflowID, boolPtrInt(body.RollbackOnFailure, 1),
			boolPtrInt(body.Enabled, 1), now, id)
		if err != nil {
			httpx.RespondError(w, "更新策略失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_policy_update", ResourceType: "remediation_policy", ResourceID: id, Details: map[string]interface{}{"name": body.Name}, IPAddress: middleware.ClientIP(r)})
		httpx.Respond(w, map[string]interface{}{"id": id, "updated_at": now})
	})

	r.With(middleware.RequireRole("admin", "operator")).Patch("/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var enabled int
		if appdb.DB().QueryRow(`SELECT enabled FROM remediation_policies WHERE id = ?`, id).Scan(&enabled) != nil {
			httpx.RespondError(w, "策略不存在", http.StatusNotFound)
			return
		}
		newVal := 1
		if enabled == 1 {
			newVal = 0
		}
		appdb.DB().Exec(`UPDATE remediation_policies SET enabled = ?, updated_at = ? WHERE id = ?`, newVal, time.Now().Format("2006-01-02 15:04:05"), id)
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_policy_toggle", ResourceType: "remediation_policy", ResourceID: id, Details: map[string]interface{}{"enabled": newVal == 1}, IPAddress: middleware.ClientIP(r)})
		httpx.Respond(w, map[string]interface{}{"id": id, "enabled": newVal == 1})
	})

	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var active int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM remediation_executions WHERE policy_id = ? AND status IN ('waiting_approval','approved','running','verifying')`, id).Scan(&active)
		if active > 0 {
			httpx.RespondError(w, "该策略有未完成（待审批/执行中/验证中）的修复任务，无法删除", http.StatusConflict)
			return
		}
		// 历史执行记录随策略一并删除（外键约束）；操作本身留审计日志
		tx, err := appdb.DB().Begin()
		if err != nil {
			httpx.RespondError(w, "删除策略失败", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM remediation_executions WHERE policy_id = ?`, id); err != nil {
			httpx.RespondError(w, "删除策略执行记录失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := tx.Exec(`DELETE FROM remediation_policies WHERE id = ?`, id); err != nil {
			httpx.RespondError(w, "删除策略失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			httpx.RespondError(w, "删除策略失败", http.StatusInternalServerError)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_policy_delete", ResourceType: "remediation_policy", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		httpx.RespondMessage(w, "策略已删除")
	})

	return r
}

// ── 执行记录 ──

const executionListCols = `e.id, e.policy_id, e.alert_id, e.status, e.status_reason, e.execution_duration_ms,
	e.verification_status, e.started_at, e.completed_at, e.created_at,
	COALESCE(p.name,''), COALESCE(p.execution_mode,''),
	COALESCE(a.title, json_extract(e.alert_snapshot,'$.title'), ''),
	COALESCE(a.severity, json_extract(e.alert_snapshot,'$.severity'), '')`

const executionListFrom = `FROM remediation_executions e
	LEFT JOIN remediation_policies p ON e.policy_id = p.id
	LEFT JOIN alerts a ON e.alert_id = a.id`

func scanExecutionListItem(scan scanFunc) (map[string]interface{}, error) {
	var id, policyID, alertID, status, createdAt, policyName, execMode, alertTitle, alertSeverity string
	var statusReason, startedAt, completedAt, verStatus sql.NullString
	var durationMs sql.NullInt64
	if err := scan(&id, &policyID, &alertID, &status, &statusReason, &durationMs,
		&verStatus, &startedAt, &completedAt, &createdAt, &policyName, &execMode, &alertTitle, &alertSeverity); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id": id, "policy_id": policyID, "alert_id": alertID,
		"status": status, "status_reason": statusReason.String,
		"execution_duration_ms": durationMs.Int64, "verification_status": verStatus.String,
		"started_at": startedAt.String, "completed_at": completedAt.String, "created_at": createdAt,
		"policy_name": policyName, "execution_mode": execMode,
		"alert_title": alertTitle, "alert_severity": alertSeverity,
	}, nil
}

func listExecutions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	where := "1=1"
	var args []interface{}
	if s := q.Get("status"); s != "" && s != "all" {
		where += " AND e.status = ?"
		args = append(args, s)
	}
	if pid := q.Get("policy_id"); pid != "" {
		where += " AND e.policy_id = ?"
		args = append(args, pid)
	}
	if aid := q.Get("alert_id"); aid != "" {
		where += " AND e.alert_id = ?"
		args = append(args, aid)
	}
	var total int
	appdb.DB().QueryRow(`SELECT COUNT(*) `+executionListFrom+` WHERE `+where, args...).Scan(&total)
	args = append(args, limit, (page-1)*limit)
	rows, err := appdb.DB().Query(`SELECT `+executionListCols+` `+executionListFrom+` WHERE `+where+` ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		httpx.Respond(w, map[string]interface{}{"executions": []interface{}{}, "total": 0})
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		if m, err := scanExecutionListItem(rows.Scan); err == nil {
			list = append(list, m)
		}
	}
	httpx.Respond(w, map[string]interface{}{"executions": list, "total": total})
}

func getExecutionDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row := appdb.DB().QueryRow(`
		SELECT e.id, e.policy_id, e.alert_id, e.alert_snapshot, e.status, e.status_reason, e.approval_required,
			e.approved_by, e.approved_at, e.approval_comment, e.workflow_execution_id, e.started_at, e.completed_at,
			e.execution_result, e.verification_status, e.verification_result, e.verification_completed_at,
			e.rollback_triggered, e.rollback_execution_id, e.rollback_completed_at, e.rollback_result,
			e.execution_duration_ms, e.created_at, COALESCE(p.name,''), COALESCE(p.execution_mode,''),
			COALESCE(a.title, json_extract(e.alert_snapshot,'$.title'), ''),
			COALESCE(a.severity, json_extract(e.alert_snapshot,'$.severity'), '')
		FROM remediation_executions e
		LEFT JOIN remediation_policies p ON e.policy_id = p.id
		LEFT JOIN alerts a ON e.alert_id = a.id
		WHERE e.id = ?`, id)
	var execID, policyID, alertID, status, createdAt, policyName, execMode, alertTitle, alertSeverity string
	var snapshot, statusReason, approvedBy, approvedAt, approvalComment, wfExecID, startedAt, completedAt sql.NullString
	var execResult, verStatus, verResult, verCompletedAt, rbExecID, rbCompletedAt, rbResult sql.NullString
	var approvalRequired, rollbackTriggered int
	var durationMs sql.NullInt64
	if err := row.Scan(&execID, &policyID, &alertID, &snapshot, &status, &statusReason, &approvalRequired,
		&approvedBy, &approvedAt, &approvalComment, &wfExecID, &startedAt, &completedAt,
		&execResult, &verStatus, &verResult, &verCompletedAt,
		&rollbackTriggered, &rbExecID, &rbCompletedAt, &rbResult,
		&durationMs, &createdAt, &policyName, &execMode, &alertTitle, &alertSeverity); err != nil {
		httpx.RespondError(w, "执行记录不存在", http.StatusNotFound)
		return
	}
	httpx.Respond(w, map[string]interface{}{
		"id": execID, "policy_id": policyID, "alert_id": alertID,
		"alert_snapshot": parseJSONOrRaw(snapshot.String),
		"status": status, "status_reason": statusReason.String,
		"approval_required": approvalRequired,
		"approved_by": approvedBy.String, "approved_at": approvedAt.String, "approval_comment": approvalComment.String,
		"workflow_execution_id": wfExecID.String,
		"started_at": startedAt.String, "completed_at": completedAt.String,
		"execution_result": parseJSONOrRaw(execResult.String),
		"verification_status": verStatus.String,
		"verification_result": parseJSONOrRaw(verResult.String),
		"verification_completed_at": verCompletedAt.String,
		"rollback_triggered": rollbackTriggered,
		"rollback_execution_id": rbExecID.String,
		"rollback_completed_at": rbCompletedAt.String,
		"rollback_result": parseJSONOrRaw(rbResult.String),
		"execution_duration_ms": durationMs.Int64, "created_at": createdAt,
		"policy_name": policyName, "execution_mode": execMode,
		"alert_title": alertTitle, "alert_severity": alertSeverity,
		"risk_level": severityToRisk(alertSeverity),
	})
}

func parseJSONOrRaw(s string) interface{} {
	if s == "" {
		return nil
	}
	var v interface{}
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

func approveExecution(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&body)
	if body.Action == "" {
		body.Action = "approve"
	}
	if body.Action != "approve" && body.Action != "reject" {
		httpx.RespondError(w, "action 必须是 approve 或 reject", http.StatusBadRequest)
		return
	}
	var status string
	if appdb.DB().QueryRow(`SELECT status FROM remediation_executions WHERE id = ?`, id).Scan(&status) != nil {
		httpx.RespondError(w, "执行记录不存在", http.StatusNotFound)
		return
	}
	if status != "waiting_approval" {
		httpx.RespondError(w, "该记录当前状态不可审批", http.StatusConflict)
		return
	}
	user := middleware.GetUser(r)
	now := time.Now().Format("2006-01-02 15:04:05")
	if body.Action == "approve" {
		appdb.DB().Exec(`UPDATE remediation_executions SET status = 'approved', approved_by = ?, approved_at = ?, approval_comment = ? WHERE id = ?`,
			user.Username, now, body.Comment, id)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_approve", ResourceType: "remediation_execution", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		go dispatchExecution(id)
		httpx.RespondMessage(w, "已批准，开始执行")
	} else {
		appdb.DB().Exec(`UPDATE remediation_executions SET status = 'rejected', status_reason = '审批已拒绝', approved_by = ?, approved_at = ?, approval_comment = ?, completed_at = ? WHERE id = ?`,
			user.Username, now, body.Comment, now, id)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_reject", ResourceType: "remediation_execution", ResourceID: id, Details: map[string]interface{}{"comment": body.Comment}, IPAddress: middleware.ClientIP(r)})
		httpx.RespondMessage(w, "已拒绝")
	}
}

func retryExecution(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var status string
	if appdb.DB().QueryRow(`SELECT status FROM remediation_executions WHERE id = ?`, id).Scan(&status) != nil {
		httpx.RespondError(w, "执行记录不存在", http.StatusNotFound)
		return
	}
	switch status {
	case "failed", "rejected", "rolled_back", "skipped":
	default:
		httpx.RespondError(w, "只有失败/已拒绝/已回滚/已跳过的记录可以重试", http.StatusConflict)
		return
	}
	appdb.DB().Exec(`UPDATE remediation_executions SET status = 'approved', status_reason = '',
		started_at = NULL, completed_at = NULL, execution_result = NULL, execution_duration_ms = NULL,
		verification_status = NULL, verification_result = NULL, verification_completed_at = NULL, verification_task_id = NULL,
		rollback_triggered = 0, rollback_execution_id = NULL, rollback_completed_at = NULL, rollback_result = NULL,
		workflow_execution_id = NULL WHERE id = ?`, id)
	user := middleware.GetUser(r)
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_retry", ResourceType: "remediation_execution", ResourceID: id, IPAddress: middleware.ClientIP(r)})
	go dispatchExecution(id)
	httpx.RespondMessage(w, "已重新排队执行")
}

// RemediationExecutionRoutes 执行记录路由 (/api/v1/remediation-executions)
func RemediationExecutionRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", listExecutions)
	r.Get("/{id}", getExecutionDetail)
	r.With(middleware.RequireRole("admin")).Post("/{id}/approve", approveExecution)
	r.With(middleware.RequireRole("admin")).Post("/{id}/retry", retryExecution)
	return r
}

// ── 工作台（audits 视图） ──

func severityToRisk(sev string) string {
	switch strings.ToLower(sev) {
	case "disaster", "critical", "high":
		return "high"
	case "average", "medium", "warning":
		return "medium"
	default:
		return "low"
	}
}

// RemediationAuditRoutes 工作台路由 (/api/v1/remediation-audits)
func RemediationAuditRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 20
		}
		var total int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM remediation_executions`).Scan(&total)
		rows, err := appdb.DB().Query(`
			SELECT e.id, e.status, e.status_reason, e.created_at, e.policy_id, COALESCE(p.name,''),
				COALESCE(a.title, json_extract(e.alert_snapshot,'$.title'), ''),
				COALESCE(a.severity, json_extract(e.alert_snapshot,'$.severity'), ''),
				e.rollback_triggered, e.execution_duration_ms
			FROM remediation_executions e
			LEFT JOIN remediation_policies p ON e.policy_id = p.id
			LEFT JOIN alerts a ON e.alert_id = a.id
			ORDER BY e.created_at DESC LIMIT ? OFFSET ?`, limit, (page-1)*limit)
		if err != nil {
			httpx.Respond(w, map[string]interface{}{"audits": []interface{}{}, "total": 0})
			return
		}
		defer rows.Close()
		list := []map[string]interface{}{}
		for rows.Next() {
			var id, status, createdAt, policyID, policyName, alertTitle, alertSeverity string
			var statusReason sql.NullString
			var rollbackTriggered int
			var durationMs sql.NullInt64
			if rows.Scan(&id, &status, &statusReason, &createdAt, &policyID, &policyName, &alertTitle, &alertSeverity, &rollbackTriggered, &durationMs) != nil {
				continue
			}
			list = append(list, map[string]interface{}{
				"id": id, "status": status, "status_reason": statusReason.String, "created_at": createdAt,
				"policy_id": policyID, "policy_name": policyName,
				"alert_title": alertTitle, "risk_level": severityToRisk(alertSeverity),
				"is_rollback": rollbackTriggered == 1, "execution_duration_ms": durationMs.Int64,
			})
		}
		httpx.Respond(w, map[string]interface{}{"audits": list, "total": total})
	})

	r.Get("/{id}", getExecutionDetail)
	r.With(middleware.RequireRole("admin")).Post("/{id}/approve", approveExecution)

	// 手动执行：待审批/已批准 → 立即派发
	r.With(middleware.RequireRole("admin")).Post("/{id}/execute", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var status string
		if appdb.DB().QueryRow(`SELECT status FROM remediation_executions WHERE id = ?`, id).Scan(&status) != nil {
			httpx.RespondError(w, "执行记录不存在", http.StatusNotFound)
			return
		}
		if status != "waiting_approval" && status != "approved" {
			httpx.RespondError(w, "只有待审批或已批准的记录可以手动执行", http.StatusConflict)
			return
		}
		user := middleware.GetUser(r)
		now := time.Now().Format("2006-01-02 15:04:05")
		appdb.DB().Exec(`UPDATE remediation_executions SET status = 'approved', approved_by = ?, approved_at = ? WHERE id = ?`, user.Username, now, id)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_manual_execute", ResourceType: "remediation_execution", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		go dispatchExecution(id)
		httpx.RespondMessage(w, "已开始执行")
	})

	// 手动回滚
	r.With(middleware.RequireRole("admin")).Post("/{id}/rollback", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var status, policyID string
		var rbTaskID sql.NullString
		if appdb.DB().QueryRow(`SELECT status, policy_id, rollback_execution_id FROM remediation_executions WHERE id = ?`, id).
			Scan(&status, &policyID, &rbTaskID) != nil {
			httpx.RespondError(w, "执行记录不存在", http.StatusNotFound)
			return
		}
		if status != "success" && status != "failed" && status != "rolled_back" {
			httpx.RespondError(w, "只有已完成的记录可以回滚", http.StatusConflict)
			return
		}
		if rbTaskID.Valid && rbTaskID.String != "" {
			var rbStatus string
			if appdb.DB().QueryRow(`SELECT status FROM tasks WHERE id = ?`, rbTaskID.String).Scan(&rbStatus) == nil &&
				(rbStatus == "running" || rbStatus == "pending") {
				httpx.RespondError(w, "回滚任务正在执行中", http.StatusConflict)
				return
			}
		}
		policy := loadPolicyByID(policyID)
		if policy == nil || policy.RollbackWorkflowID == "" {
			httpx.RespondError(w, "策略未配置回滚工作流", http.StatusBadRequest)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_manual_rollback", ResourceType: "remediation_execution", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		startRollback(id, policy, "手动触发回滚")
		httpx.RespondMessage(w, "回滚任务已启动")
	})

	// 手动验证
	r.With(middleware.RequireRole("admin")).Post("/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var status, policyID string
		if appdb.DB().QueryRow(`SELECT status, policy_id FROM remediation_executions WHERE id = ?`, id).
			Scan(&status, &policyID) != nil {
			httpx.RespondError(w, "执行记录不存在", http.StatusNotFound)
			return
		}
		if status != "success" {
			httpx.RespondError(w, "只有执行成功的记录可以验证", http.StatusConflict)
			return
		}
		policy := loadPolicyByID(policyID)
		if policy == nil || policy.VerificationWorkflowID == "" {
			httpx.RespondError(w, "策略未配置验证工作流", http.StatusBadRequest)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "remediation_manual_verify", ResourceType: "remediation_execution", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		startVerification(id, policy)
		httpx.RespondMessage(w, "验证任务已启动")
	})

	return r
}