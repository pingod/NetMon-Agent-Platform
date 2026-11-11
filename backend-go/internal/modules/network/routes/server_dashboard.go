// 服务器监控大屏路由 — Dashboard API handler
// 路由注册在 analytics.go 的 AnalyticsRoutes() 中
package routes

import (
	"database/sql"
	"net/http"
	"strconv"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/repos"
)

// handleDashboardFull 返回大屏全量数据 (stats + servers + tasks + alerts)
func handleDashboardFull(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()

	// 服务器统计
	var serverTotal, serverEnabled int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled=1 THEN 1 ELSE 0 END),0) FROM servers`).Scan(&serverTotal, &serverEnabled)

	// Agent 统计
	var agentTotal, agentEnabled int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled=1 THEN 1 ELSE 0 END),0) FROM agents`).Scan(&agentTotal, &agentEnabled)

	// 告警统计
	var alertTotal, alertActive, alertCritical, alertHigh int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status IN ('firing','acknowledged') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN severity='critical' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN severity='high' THEN 1 ELSE 0 END),0) FROM alerts`).Scan(&alertTotal, &alertActive, &alertCritical, &alertHigh)

	// 任务统计（tasks 表）
	var taskTotal, taskRunning, taskCompleted, taskFailed, taskPending int
	var taskSuccessRate float64
	db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&taskTotal)
	db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status='running' THEN 1 ELSE 0 END),0) FROM tasks`).Scan(&taskRunning)
	db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status='completed' THEN 1 ELSE 0 END),0) FROM tasks`).Scan(&taskCompleted)
	db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0) FROM tasks`).Scan(&taskFailed)
	db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END),0) FROM tasks`).Scan(&taskPending)
	if taskTotal > 0 {
		taskSuccessRate = float64(taskCompleted) / float64(taskTotal) * 100
	}

	// 工作流统计
	var workflowTotal, workflowTemplates int
	db.QueryRow(`SELECT COUNT(*) FROM workflows`).Scan(&workflowTotal)
	db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN is_template=1 THEN 1 ELSE 0 END),0) FROM workflows`).Scan(&workflowTemplates)

	// 知识库统计
	var knowledgeTotal int
	db.QueryRow(`SELECT COUNT(*) FROM knowledge_base`).Scan(&knowledgeTotal)

	// 最近告警
	alertRows, _ := db.Query(`SELECT id, title, severity, status, created_at FROM alerts ORDER BY created_at DESC LIMIT 10`)
	var alerts []map[string]interface{}
	if alertRows != nil {
		defer alertRows.Close()
		for alertRows.Next() {
			var id, title, severity, status, createdAt string
			if alertRows.Scan(&id, &title, &severity, &status, &createdAt) == nil {
				alerts = append(alerts, map[string]interface{}{
					"id": id, "title": title, "severity": severity,
					"status": status, "created_at": createdAt,
				})
			}
		}
	}
	if alerts == nil {
		alerts = []map[string]interface{}{}
	}

	// 最近任务
	taskRows, _ := db.Query(`SELECT id, name, status, created_at, workflow_id, execution_order, node_results, current_node_id FROM tasks ORDER BY created_at DESC LIMIT 10`)
	var tasks []map[string]interface{}
	if taskRows != nil {
		defer taskRows.Close()
		for taskRows.Next() {
			var id, name, status, createdAt string
			var workflowID, execOrder, nodeResults, currentNodeID sql.NullString
			if taskRows.Scan(&id, &name, &status, &createdAt, &workflowID, &execOrder, &nodeResults, &currentNodeID) == nil {
				t := map[string]interface{}{
					"id": id, "name": name, "status": status, "created_at": createdAt,
				}
				if workflowID.Valid {
					t["workflow_id"] = workflowID.String
				}
				if execOrder.Valid {
					t["execution_order"] = execOrder.String
				}
				if nodeResults.Valid {
					t["node_results"] = nodeResults.String
				}
				if currentNodeID.Valid {
					t["current_node_id"] = currentNodeID.String
				}
				tasks = append(tasks, t)
			}
		}
	}
	if tasks == nil {
		tasks = []map[string]interface{}{}
	}

	// 服务器列表
	servers, _ := repos.Servers.List()
	serverList := make([]map[string]interface{}, 0, len(servers))
	for _, s := range servers {
		serverList = append(serverList, map[string]interface{}{
			"id":       s.ID,
			"name":     s.Name,
			"hostname": s.Hostname,
			"enabled":  s.Enabled,
		})
	}

	result := map[string]interface{}{
		"stats": map[string]interface{}{
			"servers":   map[string]interface{}{"total": serverTotal, "enabled": serverEnabled},
			"agents":    map[string]interface{}{"total": agentTotal, "enabled": agentEnabled},
			"tasks":     map[string]interface{}{"total": taskTotal, "running": taskRunning, "completed": taskCompleted, "failed": taskFailed, "pending": taskPending, "successRate": taskSuccessRate},
			"alerts":    map[string]interface{}{"total": alertTotal, "active": alertActive, "critical": alertCritical, "high": alertHigh},
			"workflows": map[string]interface{}{"total": workflowTotal, "templates": workflowTemplates},
			"knowledge": map[string]interface{}{"total": knowledgeTotal},
		},
		"servers":      serverList,
		"recentTasks":  tasks,
		"recentAlerts": alerts,
	}

	httpx.Respond(w, result)
}

// handleServerMetrics 返回服务器性能指标（当前值+历史趋势）
func handleServerMetrics(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 {
			hours = v
		}
	}

	// 获取所有服务器最新指标
	allLatest, err := repos.ServerMetrics.GetAllLatest()
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "查询指标失败")
		return
	}

	// 构建服务器列表
	servers := make([]map[string]interface{}, 0, len(allLatest))
	for _, m := range allLatest {
		s := map[string]interface{}{
			"server_id":   m.ServerID,
			"server_name": m.ServerName,
		}
		if m.CPUUsage != nil {
			s["cpu_usage"] = *m.CPUUsage
		} else {
			s["cpu_usage"] = nil
		}
		if m.MemoryUsage != nil {
			s["memory_usage"] = *m.MemoryUsage
		} else {
			s["memory_usage"] = nil
		}
		if m.DiskUsage != nil {
			s["disk_usage"] = *m.DiskUsage
		} else {
			s["disk_usage"] = nil
		}
		if m.NetworkInMbps != nil {
			s["network_in_mbps"] = *m.NetworkInMbps
		} else {
			s["network_in_mbps"] = nil
		}
		if m.NetworkOutMbps != nil {
			s["network_out_mbps"] = *m.NetworkOutMbps
		} else {
			s["network_out_mbps"] = nil
		}
		if m.Load1min != nil {
			s["load_1min"] = *m.Load1min
		} else {
			s["load_1min"] = nil
		}
		if m.CollectedAt != "" {
			s["collected_at"] = m.CollectedAt
		} else {
			s["collected_at"] = nil
		}
		servers = append(servers, s)
	}

	hasRealData := len(allLatest) > 0

	// 获取趋势数据
	trends, _ := repos.ServerMetrics.GetTrendData(hours)

	// 构建历史数据 — 按指标类型分列
	cpuHistory := make([]map[string]interface{}, 0)
	memoryHistory := make([]map[string]interface{}, 0)
	networkHistory := make([]map[string]interface{}, 0)
	diskHistory := make([]map[string]interface{}, 0)

	for _, t := range trends {
		cpuHistory = append(cpuHistory, map[string]interface{}{
			"server_id": "avg", "value": t.CPUUsage, "timestamp": t.Time,
		})
		memoryHistory = append(memoryHistory, map[string]interface{}{
			"server_id": "avg", "value": t.MemoryUsage, "timestamp": t.Time,
		})
		networkHistory = append(networkHistory, map[string]interface{}{
			"server_id": "avg", "value": t.NetworkInMbps, "timestamp": t.Time,
		})
		diskHistory = append(diskHistory, map[string]interface{}{
			"server_id": "avg", "value": t.DiskUsage, "timestamp": t.Time,
		})
	}

	// 如果没有趋势数据，尝试从各服务器获取
	if len(trends) == 0 {
		for _, m := range allLatest {
			serverHistory, _ := repos.ServerMetrics.GetHistoryByServer(m.ServerID, hours)
			for _, h := range serverHistory {
				if h.CPUUsage != nil {
					cpuHistory = append(cpuHistory, map[string]interface{}{
						"server_id": h.ServerID, "value": *h.CPUUsage, "timestamp": h.CollectedAt,
					})
				}
				if h.MemoryUsage != nil {
					memoryHistory = append(memoryHistory, map[string]interface{}{
						"server_id": h.ServerID, "value": *h.MemoryUsage, "timestamp": h.CollectedAt,
					})
				}
				if h.NetworkInMbps != nil {
					networkHistory = append(networkHistory, map[string]interface{}{
						"server_id": h.ServerID, "value": *h.NetworkInMbps, "timestamp": h.CollectedAt,
					})
				}
				if h.DiskUsage != nil {
					diskHistory = append(diskHistory, map[string]interface{}{
						"server_id": h.ServerID, "value": *h.DiskUsage, "timestamp": h.CollectedAt,
					})
				}
			}
		}
	}

	result := map[string]interface{}{
		"servers":         servers,
		"has_real_data":   hasRealData,
		"cpu_history":     cpuHistory,
		"memory_history":  memoryHistory,
		"network_history": networkHistory,
		"disk_history":    diskHistory,
	}

	httpx.Respond(w, result)
}

// handleAlertTrends 返回告警趋势（24h）
func handleAlertTrends(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 {
			hours = v
		}
	}

	rows, err := db.Query(`
		SELECT strftime('%Y-%m-%dT%H:00:00', created_at) AS time_bucket,
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN severity='critical' THEN 1 ELSE 0 END),0) AS critical,
			COALESCE(SUM(CASE WHEN severity='high' THEN 1 ELSE 0 END),0) AS high,
			COALESCE(SUM(CASE WHEN severity='medium' THEN 1 ELSE 0 END),0) AS medium,
			COALESCE(SUM(CASE WHEN severity='low' THEN 1 ELSE 0 END),0) AS low
		FROM alerts
		WHERE created_at >= datetime('now','localtime','` + strconv.Itoa(-hours) + ` hours')
		GROUP BY time_bucket ORDER BY time_bucket ASC`)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "查询告警趋势失败")
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var bucket string
		var total, critical, high, medium, low int
		if rows.Scan(&bucket, &total, &critical, &high, &medium, &low) == nil {
			items = append(items, map[string]interface{}{
				"time_bucket": bucket, "total": total,
				"critical": critical, "high": high, "medium": medium, "low": low,
			})
		}
	}
	if items == nil {
		items = []map[string]interface{}{}
	}

	httpx.Respond(w, items)
}

// handleTaskTrends 返回任务趋势（24h）
func handleTaskTrends(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 {
			hours = v
		}
	}

	rows, err := db.Query(`
		SELECT strftime('%Y-%m-%dT%H:00:00', created_at) AS time_bucket,
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status='completed' THEN 1 ELSE 0 END),0) AS completed,
			COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0) AS failed,
			COALESCE(SUM(CASE WHEN status='running' THEN 1 ELSE 0 END),0) AS running
		FROM tasks
		WHERE created_at >= datetime('now','localtime','` + strconv.Itoa(-hours) + ` hours')
		GROUP BY time_bucket ORDER BY time_bucket ASC`)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "查询任务趋势失败")
		return
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var bucket string
		var total, completed, failed, running int
		if rows.Scan(&bucket, &total, &completed, &failed, &running) == nil {
			items = append(items, map[string]interface{}{
				"time_bucket": bucket, "total": total,
				"completed": completed, "failed": failed, "running": running,
			})
		}
	}
	if items == nil {
		items = []map[string]interface{}{}
	}

	httpx.Respond(w, items)
}

// handleAgentStats 返回 Agent 调用统计
func handleAgentStats(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()

	agents, err := repos.Agents.List()
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "查询 Agent 失败")
		return
	}

	agentStats := make([]map[string]interface{}, 0, len(agents))
	var totalExecutions, totalSuccess, todayExecutions int

	for _, a := range agents {
		// 从 agent_usage_stats 表获取统计
		var execCount, successCount, errorCount int
		db.QueryRow(`SELECT COALESCE(SUM(execution_count),0), COALESCE(SUM(success_count),0), COALESCE(SUM(error_count),0)
			FROM agent_usage_stats WHERE agent_id = ?`, a.ID).Scan(&execCount, &successCount, &errorCount)

		var successRate *float64
		if execCount > 0 {
			rate := float64(successCount) / float64(execCount) * 100
			successRate = &rate
		}

		avatar := ""
		if a.Avatar != nil {
			avatar = *a.Avatar
		}
		role := ""
		if a.Role != nil {
			role = *a.Role
		}

		agentStats = append(agentStats, map[string]interface{}{
			"id":               a.ID,
			"name":             a.Name,
			"avatar":           avatar,
			"role":             role,
			"enabled":          a.Enabled,
			"usage_count":      a.UsageCount,
			"total_executions": execCount,
			"success_count":    successCount,
			"error_count":      errorCount,
			"successRate":      successRate,
		})

		totalExecutions += execCount
		totalSuccess += successCount
	}

	// 今日统计
	db.QueryRow(`SELECT COALESCE(SUM(execution_count),0) FROM agent_usage_stats
		WHERE date(created_at) = date('now','localtime')`).Scan(&todayExecutions)

	overallSuccessRate := 0.0
	if totalExecutions > 0 {
		overallSuccessRate = float64(totalSuccess) / float64(totalExecutions) * 100
	}

	result := map[string]interface{}{
		"agents": agentStats,
		"overall": map[string]interface{}{
			"totalExecutions":    totalExecutions,
			"totalSuccess":       totalSuccess,
			"overallSuccessRate": overallSuccessRate,
			"todayExecutions":    todayExecutions,
		},
	}

	httpx.Respond(w, result)
}

// handleTaskDistribution 返回任务状态分布
func handleTaskDistribution(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()

	// 按状态分布
	rows, err := db.Query(`SELECT status, COUNT(*) AS cnt FROM tasks GROUP BY status ORDER BY cnt DESC`)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "查询任务分布失败")
		return
	}
	defer rows.Close()

	byStatus := make([]map[string]interface{}, 0)
	for rows.Next() {
		var status string
		var count int
		if rows.Scan(&status, &count) == nil {
			byStatus = append(byStatus, map[string]interface{}{
				"status": status, "count": count,
			})
		}
	}

	// 按工作流分布
	wfRows, _ := db.Query(`SELECT COALESCE(w.name,'Unknown') AS name, COUNT(*) AS cnt
		FROM tasks e LEFT JOIN workflows w ON e.workflow_id = w.id
		GROUP BY w.id ORDER BY cnt DESC LIMIT 10`)
	byWorkflow := make([]map[string]interface{}, 0)
	if wfRows != nil {
		defer wfRows.Close()
		for wfRows.Next() {
			var name string
			var count int
			if wfRows.Scan(&name, &count) == nil {
				byWorkflow = append(byWorkflow, map[string]interface{}{
					"name": name, "count": count,
				})
			}
		}
	}

	result := map[string]interface{}{
		"byStatus":   byStatus,
		"byWorkflow": byWorkflow,
	}

	httpx.Respond(w, result)
}

// handleRemediationStats 返回自动修复统计
func handleRemediationStats(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()

	// 自动修复策略统计
	var totalPolicies, enabledPolicies int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled=1 THEN 1 ELSE 0 END),0)
		FROM remediation_policies`).Scan(&totalPolicies, &enabledPolicies)

	// 今日执行统计
	var todayTotal, todaySuccess, todayFailed, todayRolledBack int
	var todayAvgDuration float64
	db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status='success' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='rolled_back' THEN 1 ELSE 0 END),0),
		COALESCE(AVG(CASE WHEN execution_duration_ms IS NOT NULL THEN CAST(execution_duration_ms AS REAL) END),0)
		FROM remediation_executions
		WHERE date(created_at) = date('now','localtime')`).Scan(&todayTotal, &todaySuccess, &todayFailed, &todayRolledBack, &todayAvgDuration)

	todaySuccessRate := 0.0
	if todayTotal > 0 {
		todaySuccessRate = float64(todaySuccess) / float64(todayTotal) * 100
	}

	// 待审批
	var waitingApproval int
	db.QueryRow(`SELECT COUNT(*) FROM remediation_executions WHERE status='waiting_approval'`).Scan(&waitingApproval)

	// 最近执行记录
	execRows, _ := db.Query(`SELECT re.id, re.status, re.status_reason, re.created_at,
		COALESCE(rp.name,''), COALESCE(rp.execution_mode,'manual'),
		COALESCE(a.title, json_extract(re.alert_snapshot,'$.title'), ''), COALESCE(a.severity,'')
		FROM remediation_executions re
		LEFT JOIN remediation_policies rp ON re.policy_id = rp.id
		LEFT JOIN alerts a ON re.alert_id = a.id
		ORDER BY re.created_at DESC LIMIT 10`)

	recentExecutions := make([]map[string]interface{}, 0)
	if execRows != nil {
		defer execRows.Close()
		for execRows.Next() {
			var id, status, createdAt, policyName, execMode, alertTitle, alertSeverity string
			var statusReason sql.NullString
			if execRows.Scan(&id, &status, &statusReason, &createdAt, &policyName, &execMode, &alertTitle, &alertSeverity) == nil {
				item := map[string]interface{}{
					"id": id, "status": status, "created_at": createdAt,
					"policy_name": policyName, "execution_mode": execMode,
					"alert_title": alertTitle, "alert_severity": alertSeverity,
				}
				if statusReason.Valid {
					item["status_reason"] = statusReason.String
				}
				recentExecutions = append(recentExecutions, item)
			}
		}
	}

	result := map[string]interface{}{
		"total_policies":   totalPolicies,
		"enabled_policies": enabledPolicies,
		"today": map[string]interface{}{
			"total":           todayTotal,
			"success":         todaySuccess,
			"failed":          todayFailed,
			"rolled_back":     todayRolledBack,
			"success_rate":    todaySuccessRate,
			"avg_duration_ms": todayAvgDuration,
		},
		"waiting_approval":  waitingApproval,
		"recent_executions": recentExecutions,
	}

	httpx.Respond(w, result)
}

// handleAlertSourceStats 按告警来源聚合状态与级别分布
func handleAlertSourceStats(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	rows, err := db.Query(`SELECT COALESCE(NULLIF(source,''),'unknown') AS src,
		COUNT(*) AS total,
		COALESCE(SUM(CASE WHEN status='new' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status NOT IN ('resolved','closed') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status IN ('resolved','closed') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN severity IN ('disaster','critical') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN severity='high' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN severity IN ('average','medium') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN severity IN ('warning','low','info') THEN 1 ELSE 0 END),0)
		FROM alerts GROUP BY src ORDER BY total DESC`)
	if err != nil {
		httpx.RespondFailStatus(w, http.StatusInternalServerError, "查询告警来源统计失败")
		return
	}
	defer rows.Close()

	stats := make([]map[string]interface{}, 0)
	for rows.Next() {
		var src string
		var total, newN, activeN, resolvedN, criticalN, highN, mediumN, lowN int
		if rows.Scan(&src, &total, &newN, &activeN, &resolvedN, &criticalN, &highN, &mediumN, &lowN) == nil {
			stats = append(stats, map[string]interface{}{
				"source": src, "total_alerts": total,
				"new_alerts": newN, "active_alerts": activeN, "resolved_alerts": resolvedN,
				"critical_count": criticalN, "high_count": highN, "medium_count": mediumN, "low_count": lowN,
			})
		}
	}
	httpx.Respond(w, map[string]interface{}{"source_stats": stats})
}

// handleSlaStats 返回 SLA 统计
func handleSlaStats(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()

	// MTTR: 平均解决时间（分钟）
	var mttrMinutes float64
	db.QueryRow(`SELECT COALESCE(AVG(
		(strftime('%s', updated_at) - strftime('%s', created_at)) / 60.0
	),0) FROM alerts WHERE status = 'resolved'`).Scan(&mttrMinutes)

	// 今日告警
	var totalAlertsToday, resolvedToday int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='resolved' THEN 1 ELSE 0 END),0)
		FROM alerts WHERE date(created_at) = date('now','localtime')`).Scan(&totalAlertsToday, &resolvedToday)

	// 可用性（基于有指标的服务器在线率）
	var uptimePct float64 = 99.9
	serverMetrics, _ := repos.ServerMetrics.GetAllLatest()
	if len(serverMetrics) > 0 {
		upCount := 0
		for _, m := range serverMetrics {
			if m.CPUUsage != nil {
				upCount++
			}
		}
		if len(serverMetrics) > 0 {
			uptimePct = float64(upCount) / float64(len(serverMetrics)) * 100
		}
	}

	// 平均响应时间（秒）
	var avgResponseSeconds float64
	db.QueryRow(`SELECT COALESCE(AVG(
		strftime('%s', updated_at) - strftime('%s', created_at)
	),0) FROM alerts WHERE status IN ('acknowledged','resolved')`).Scan(&avgResponseSeconds)

	// 告警解决率
	alertResolutionRate := 0.0
	if totalAlertsToday > 0 {
		alertResolutionRate = float64(resolvedToday) / float64(totalAlertsToday) * 100
	}

	result := map[string]interface{}{
		"mttr_minutes":          mttrMinutes,
		"uptime_percentage":     uptimePct,
		"avg_response_seconds":  avgResponseSeconds,
		"alert_resolution_rate": alertResolutionRate,
		"total_alerts_today":    totalAlertsToday,
		"resolved_today":        resolvedToday,
	}

	httpx.Respond(w, result)
}
