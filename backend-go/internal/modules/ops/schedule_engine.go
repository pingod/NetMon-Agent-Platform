// 定时调度引擎：工作流定时任务（scheduled_tasks）与定时报告（report_schedules）。
// 两者此前只有 CRUD 没有执行器；本引擎按 cron 表达式到期触发，
// 工作流创建真实任务走 StartTaskExecution，报告按模板生成并可邮件投递接收人。
package ops

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
	"itops-backend/internal/utils"

	"github.com/google/uuid"
)

const scheduleEngineTick = 15 * time.Second

var scheduleEngineOnce sync.Once

// StartScheduleEngine 启动定时调度循环，幂等
func StartScheduleEngine() {
	scheduleEngineOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(scheduleEngineTick)
			defer ticker.Stop()
			for range ticker.C {
				runDueWorkflowSchedules()
				runDueReportSchedules()
			}
		}()
		logger.Infof("Schedule engine started (workflow schedules + report schedules, tick %s)", scheduleEngineTick)
	})
}

// scheduleBaseTime 计算 cron 基准时间：上次运行时间，从未运行取创建时间
func scheduleBaseTime(lastRun, createdAt string, now time.Time) time.Time {
	if t := parseScheduleTime(lastRun); !t.IsZero() {
		return t
	}
	if t := parseScheduleTime(createdAt); !t.IsZero() {
		return t
	}
	return now
}

func parseScheduleTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ───────────────────────────────────────────────
// 工作流定时任务
// ───────────────────────────────────────────────

func runDueWorkflowSchedules() {
	db := appdb.DB()
	rows, err := db.Query(`SELECT id, name, workflow_id, cron_expression, COALESCE(last_run_at,''), COALESCE(created_at,'')
		FROM scheduled_tasks WHERE enabled = 1`)
	if err != nil {
		return
	}
	type schedRow struct{ id, name, workflowID, cron, lastRun, createdAt string }
	var list []schedRow
	for rows.Next() {
		var s schedRow
		if rows.Scan(&s.id, &s.name, &s.workflowID, &s.cron, &s.lastRun, &s.createdAt) == nil {
			list = append(list, s)
		}
	}
	rows.Close()

	now := time.Now()
	for _, s := range list {
		spec, err := utils.ParseCron(strings.TrimSpace(s.cron))
		if err != nil {
			logger.Warnf("scheduled task %s cron 表达式非法: %v", s.id, err)
			continue
		}
		next := spec.Next(scheduleBaseTime(s.lastRun, s.createdAt, now))
		if next.IsZero() {
			continue
		}
		nowStr := now.Format("2006-01-02 15:04:05")
		if next.After(now) {
			db.Exec(`UPDATE scheduled_tasks SET next_run_at = ? WHERE id = ?`, next.Format("2006-01-02 15:04:05"), s.id)
			continue
		}

		var wfName string
		if db.QueryRow(`SELECT name FROM workflows WHERE id = ?`, s.workflowID).Scan(&wfName) != nil {
			logger.Warnf("scheduled task %s: 工作流 %s 不存在，跳过", s.id, s.workflowID)
			db.Exec(`UPDATE scheduled_tasks SET last_run_at = ?, next_run_at = ? WHERE id = ?`,
				nowStr, spec.Next(now).Format("2006-01-02 15:04:05"), s.id)
			continue
		}

		taskID := uuid.NewString()
		taskName := fmt.Sprintf("%s-%s", s.name, now.Format("0102-1504"))
		ctxJSON, _ := json.Marshal(map[string]interface{}{"params": map[string]interface{}{}, "schedule_id": s.id})
		if _, err := db.Exec(`INSERT INTO tasks (id, name, workflow_id, status, start_time, context, created_at, updated_at)
			VALUES (?, ?, ?, 'running', ?, ?, ?, ?)`, taskID, taskName, s.workflowID, nowStr, string(ctxJSON), nowStr, nowStr); err != nil {
			logger.Errorf("scheduled task %s: 创建任务失败: %v", s.id, err)
			continue
		}
		db.Exec(`UPDATE scheduled_tasks SET last_run_at = ?, next_run_at = ? WHERE id = ?`,
			nowStr, spec.Next(now).Format("2006-01-02 15:04:05"), s.id)
		StartTaskExecution(taskID)
		logger.Infof("scheduled task %s (%s) 触发工作流 %s → task %s", s.name, s.cron, wfName, taskID)
	}
}

// ───────────────────────────────────────────────
// 定时报告
// ───────────────────────────────────────────────

func runDueReportSchedules() {
	db := appdb.DB()
	rows, err := db.Query(`SELECT id, name, template_id, cron_expression, COALESCE(recipients,'[]'), COALESCE(last_generated,''), COALESCE(created_at,'')
		FROM report_schedules WHERE enabled = 1`)
	if err != nil {
		return
	}
	type schedRow struct {
		id, name, templateID, cron, recipients, lastGen, createdAt string
	}
	var list []schedRow
	for rows.Next() {
		var s schedRow
		if rows.Scan(&s.id, &s.name, &s.templateID, &s.cron, &s.recipients, &s.lastGen, &s.createdAt) == nil {
			list = append(list, s)
		}
	}
	rows.Close()

	now := time.Now()
	for _, s := range list {
		spec, err := utils.ParseCron(strings.TrimSpace(s.cron))
		if err != nil {
			logger.Warnf("report schedule %s cron 表达式非法: %v", s.id, err)
			continue
		}
		next := spec.Next(scheduleBaseTime(s.lastGen, s.createdAt, now))
		if next.IsZero() || next.After(now) {
			continue
		}
		generateScheduledReport(s.id, s.name, s.templateID, s.recipients)
		db.Exec(`UPDATE report_schedules SET last_generated = ? WHERE id = ?`, now.Format("2006-01-02 15:04:05"), s.id)
	}
}

// generateScheduledReport 按模板生成一份报告；配置了 SMTP 且存在接收人时邮件投递
func generateScheduledReport(scheduleID, name, templateID, recipientsJSON string) {
	db := appdb.DB()
	var tplName, tplContent string
	if err := db.QueryRow(`SELECT name, content FROM reports WHERE id = ? AND type = 'template'`, templateID).Scan(&tplName, &tplContent); err != nil {
		logger.Warnf("report schedule %s: 模板 %s 不存在，跳过", scheduleID, templateID)
		return
	}

	now := time.Now()
	vars := map[string]interface{}{
		"date":     now.Format("2006-01-02"),
		"datetime": now.Format("2006-01-02 15:04"),
		"schedule": name,
	}
	content := tplContent
	for k, v := range vars {
		content = strings.ReplaceAll(content, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}

	reportID := uuid.NewString()
	reportName := fmt.Sprintf("%s - %s", tplName, now.Format("2006-01-02 15:04"))
	varsJSON, _ := json.Marshal(vars)
	if _, err := db.Exec(`INSERT INTO reports (id, name, type, content, format, template_id, variables, created_at, updated_at)
		VALUES (?, ?, 'inspection', ?, 'markdown', ?, ?, datetime('now','localtime'), datetime('now','localtime'))`,
		reportID, reportName, content, templateID, string(varsJSON)); err != nil {
		logger.Errorf("report schedule %s: 生成报告失败: %v", scheduleID, err)
		return
	}
	logger.Infof("report schedule %s 生成报告 %s (%s)", name, reportName, reportID)

	var recipients []string
	_ = json.Unmarshal([]byte(recipientsJSON), &recipients)
	if len(recipients) == 0 {
		return
	}
	cfg, err := GetNotificationConfig()
	if err != nil || cfg == nil || !cfg.EmailEnabled || cfg.EmailConfig.SmtpHost == "" {
		logger.Infof("report schedule %s: 未启用邮件通道，报告 %s 仅存档", name, reportID)
		return
	}
	body := fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:720px;padding:16px;">
		<h2>%s</h2><p style="color:#666;">由定时报告「%s」于 %s 自动生成</p>
		<pre style="white-space:pre-wrap;background:#f6f8fa;padding:16px;border-radius:8px;font-size:13px;">%s</pre></div>`,
		html.EscapeString(reportName), html.EscapeString(name), html.EscapeString(now.Format("2006-01-02 15:04")), html.EscapeString(content))
	for _, to := range recipients {
		to = strings.TrimSpace(to)
		if to == "" {
			continue
		}
		if err := SendSmtpEmail(cfg.EmailConfig, to, "[NetMon 定时报告] "+reportName, body); err != nil {
			logger.Warnf("report schedule %s: 投递 %s 失败: %v", name, to, err)
			LogNotificationRecord("email", reportName, "定时报告投递", to, "failed", "", "", err.Error())
		} else {
			LogNotificationRecord("email", reportName, "定时报告投递", to, "sent", "", "", "")
		}
	}
}