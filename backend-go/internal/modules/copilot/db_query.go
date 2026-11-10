// Copilot 智能运维助手 — 数据库安全只读查询执行器
// 支持自然语言数据检索与安全只读 SQL 执行，双层隔离保护，敏感信息自动脱敏，Markdown 表格化直观呈现
package copilot

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

// isDatabaseQuery 识别是否为数据库数据查询意图
func isDatabaseQuery(rawQuery string) (bool, string, string) {
	trimmed := strings.TrimSpace(rawQuery)
	lower := strings.ToLower(trimmed)

	// 1. 直接输入 SQL 语句检测（SELECT 开头）
	sqlRegex := regexp.MustCompile(`(?i)^\s*select\s+.+\s+from\s+[\w` + "`" + `]+`)
	if sqlRegex.MatchString(trimmed) || (strings.HasPrefix(lower, "select ") && strings.Contains(lower, " from ")) {
		return true, trimmed, "custom_sql"
	}

	// 2. 包含带前缀的 SQL 执行意图（如 "查数据: select ..." 或 "执行sql: select ..."）
	if strings.Contains(lower, "select ") && (strings.Contains(lower, "sql") || strings.Contains(lower, "查数据") || strings.Contains(lower, "查询数据")) {
		idx := strings.Index(lower, "select ")
		if idx != -1 {
			return true, strings.TrimSpace(trimmed[idx:]), "custom_sql"
		}
	}

	// 3. 自然语言数据表结构与清单查询
	if strings.Contains(lower, "有哪些表") ||
		strings.Contains(lower, "数据库表") ||
		strings.Contains(lower, "查看表结构") ||
		strings.Contains(lower, "查看所有表") ||
		strings.Contains(lower, "数据表清单") ||
		strings.Contains(lower, "数据库有哪些数据") ||
		strings.Contains(lower, "查看数据库") ||
		strings.Contains(lower, "数据库结构") ||
		strings.Contains(lower, "数据库统计") {
		return true, "", "tables_list"
	}

	// 4. 自然语言核心业务数据表查询
	wantsData := strings.Contains(lower, "查数据") ||
		strings.Contains(lower, "查一下数据") ||
		strings.Contains(lower, "查询数据库") ||
		strings.Contains(lower, "查数据库") ||
		strings.Contains(lower, "数据库中") ||
		strings.Contains(lower, "数据库里") ||
		strings.Contains(lower, "访问数据库")

	if wantsData {
		if strings.Contains(lower, "告警") {
			return true, "SELECT id, source, severity, title, content, status, created_at FROM alerts ORDER BY created_at DESC LIMIT 10", "alerts"
		}
		if strings.Contains(lower, "设备") || strings.Contains(lower, "交换机") {
			return true, "SELECT id, name, ip_address, vendor, role, status, updated_at FROM network_devices ORDER BY updated_at DESC LIMIT 10", "devices"
		}
		if strings.Contains(lower, "主机") || strings.Contains(lower, "服务器") {
			return true, "SELECT id, name, hostname, os_type, agent_status, agent_cpu_usage, created_at FROM servers LIMIT 10", "servers"
		}
		if strings.Contains(lower, "备份") {
			return true, "SELECT id, device_name, device_ip, status, duration_ms, started_at FROM config_backup_records ORDER BY started_at DESC LIMIT 10", "backups"
		}
		if strings.Contains(lower, "巡检") {
			return true, "SELECT id, device_id, inspection_type, status, summary, created_at FROM network_inspection_history ORDER BY created_at DESC LIMIT 10", "inspections"
		}
		if strings.Contains(lower, "连接") || strings.Contains(lower, "数据库配置") {
			return true, "SELECT id, name, db_type, host, port, database, enabled FROM databases LIMIT 10", "db_conns"
		}
		if strings.Contains(lower, "子网") || strings.Contains(lower, "网络") {
			return true, "SELECT id, name, subnet, gateway, vlan_id, total_ips, used_ips FROM network_subnets LIMIT 10", "subnets"
		}
		if strings.Contains(lower, "日志") || strings.Contains(lower, "审计") {
			return true, "SELECT id, username, action, resource_type, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT 10", "audit"
		}
		if strings.Contains(lower, "用户") {
			return true, "SELECT id, username, role, email, enabled, created_at FROM users LIMIT 10", "users"
		}

		// 默认查核心网络设备表
		return true, "SELECT id, name, ip_address, vendor, role, status, last_inspection_result FROM network_devices LIMIT 10", "devices"
	}

	return false, "", ""
}

// validateReadOnlySQL 严格只读 SQL 安全审查
func validateReadOnlySQL(sqlQuery string) (bool, string) {
	trimmed := strings.TrimSpace(sqlQuery)
	lower := strings.ToLower(trimmed)

	// 必须以 SELECT / PRAGMA / EXPLAIN 开头
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "pragma") && !strings.HasPrefix(lower, "explain") {
		return false, "仅允许执行只读 SELECT 查询，严禁执行非读操作语句！"
	}

	// 拦截破坏性及写入性 SQL 关键字
	forbiddenWords := []string{
		"insert ", "update ", "delete ", "drop ", "alter ", "create ", "truncate ",
		"replace ", "grant ", "revoke ", "execute ", "exec ", "attach ", "detach ",
	}
	for _, word := range forbiddenWords {
		if strings.Contains(lower, word) {
			return false, fmt.Sprintf("安全审计拦截：检测到高危写入/破坏性指令 [%s]。NetMon 遵循只读最小权限原则，严禁变更数据！", strings.TrimSpace(word))
		}
	}

	// 拦截分号多语句拼接执行
	if strings.Count(trimmed, ";") > 1 || (strings.Contains(trimmed, ";") && !strings.HasSuffix(strings.TrimSpace(trimmed), ";")) {
		return false, "安全审计拦截：严禁使用分号拼接执行复合 SQL 语句！"
	}

	return true, ""
}

// ExecuteDatabaseQuery 执行数据库安全查询并生成 Markdown 表格呈现
func ExecuteDatabaseQuery(ctx context.Context, sqlStatement, queryType string) string {
	start := time.Now()

	// 特殊场景：查看系统全部数据表清单与行数统计
	if queryType == "tables_list" {
		return executeTablesListQuery()
	}

	// 1. 安全只读审查
	allowed, blockReason := validateReadOnlySQL(sqlStatement)
	if !allowed {
		logger.Warnf("SQL 审查拦截: %s (SQL: %s)", blockReason, sqlStatement)
		return GenerateSecurityBlockNotice(sqlStatement, blockReason)
	}

	// 2. 自动补充安全 LIMIT 保护（防止单次拉取海量数据爆内存）
	finalSQL := strings.TrimRight(strings.TrimSpace(sqlStatement), ";")
	lower := strings.ToLower(finalSQL)
	if !strings.Contains(lower, "limit ") {
		finalSQL += " LIMIT 20"
	}

	// 3. 执行查询
	db := appdb.DB()
	rows, err := db.QueryContext(ctx, finalSQL)
	if err != nil {
		return fmt.Sprintf("### ❌ 数据库查询失败\n\n> **执行 SQL**：`%s`\n\n**错误详情**：\n```\n%v\n```\n\n请核对表名与列名是否正确。您可发送「**查一下数据库里有哪些表**」查看有效表名。", finalSQL, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("❌ 解析数据列名失败: %v", err)
	}

	// 4. 动态读取所有结果行
	var resultRows [][]string
	for rows.Next() {
		colVals := make([]interface{}, len(cols))
		colValPtrs := make([]interface{}, len(cols))
		for i := range colVals {
			colValPtrs[i] = &colVals[i]
		}

		if err := rows.Scan(colValPtrs...); err != nil {
			continue
		}

		rowStr := make([]string, len(cols))
		for i, val := range colVals {
			colNameLower := strings.ToLower(cols[i])
			// 敏感凭据脱敏处理
			if strings.Contains(colNameLower, "password") ||
				strings.Contains(colNameLower, "auth_key") ||
				strings.Contains(colNameLower, "priv_key") ||
				strings.Contains(colNameLower, "secret") ||
				strings.Contains(colNameLower, "token") ||
				strings.Contains(colNameLower, "private_key") {
				rowStr[i] = "******"
				continue
			}

			if val == nil {
				rowStr[i] = "*NULL*"
			} else {
				switch v := val.(type) {
				case []byte:
					rowStr[i] = sanitizeTableText(string(v))
				default:
					rowStr[i] = sanitizeTableText(fmt.Sprintf("%v", v))
				}
			}
		}
		resultRows = append(resultRows, rowStr)
	}

	durationMs := time.Since(start).Milliseconds()

	// 5. 渲染 Markdown 数据表格
	var sb strings.Builder
	sb.WriteString("### 🗄️ NetMon 数据库安全只读查询结果\n\n")
	sb.WriteString("> **只读控制策略**：`SELECT 访问已放行` | `密码/密钥/凭证: 已自动掩码脱敏 (Masked)`\n\n")

	sb.WriteString("| 查询指标 | 运行参数 |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **执行 SQL** | `%s` |\n", finalSQL))
	sb.WriteString(fmt.Sprintf("| **数据源** | `app.db (SQLite 核心运维库)` |\n"))
	sb.WriteString(fmt.Sprintf("| **查询耗时** | `%d ms` |\n", durationMs))
	sb.WriteString(fmt.Sprintf("| **命中数据行数** | `%d 条` |\n\n", len(resultRows)))

	if len(resultRows) == 0 {
		sb.WriteString("ℹ️ *查询执行成功，但当前数据表中未检索到符合条件的记录。*\n\n")
	} else {
		sb.WriteString("#### 📋 检索结果明细表格\n\n")

		// 表头
		sb.WriteString("| 序号 | ")
		for _, col := range cols {
			sb.WriteString(col + " | ")
		}
		sb.WriteString("\n")

		// 分隔线
		sb.WriteString("| :---: | ")
		for range cols {
			sb.WriteString(":--- | ")
		}
		sb.WriteString("\n")

		// 行内容
		for idx, row := range resultRows {
			sb.WriteString(fmt.Sprintf("| %d | ", idx+1))
			for _, cell := range row {
				sb.WriteString(cell + " | ")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("### 🤖 数据解读与运维分析\n\n")
	sb.WriteString(generateDbInsight(queryType, len(resultRows), cols, resultRows))

	return sb.String()
}

// executeTablesListQuery 查询并输出数据库所有核心表清单
func executeTablesListQuery() string {
	start := time.Now()
	db := appdb.DB()

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name ASC`)
	if err != nil {
		return fmt.Sprintf("❌ 读取数据库表清单失败: %v", err)
	}
	defer rows.Close()

	type tableMeta struct {
		name     string
		category string
		rowCount int
		desc     string
	}

	categoryMap := map[string]string{
		"network_devices":            "网络设备核心资产台账表",
		"network_inspection_history": "网络设备深度巡检历史记录表",
		"snmp_interface_metrics":     "SNMP 物理接口流量与丢包指标库",
		"network_subnets":            "网络子网与 IP 地址规划表",
		"network_topology_links":     "LLDP 物理链路与网络拓扑结构表",
		"config_backups":             "设备配置备份历史版本库",
		"inspection_templates":       "网络巡检标准脚本与命令模板表",
		"servers":                    "服务器与计算节点主机资产表",
		"server_metrics":             "服务器 CPU/内存/磁盘性能时序指标表",
		"containers":                 "Docker/K8s 容器实例运行状态表",
		"alerts":                     "系统全域活跃与历史告警事件表",
		"alert_rules":                "PromQL / 阈值告警规则策略配置表",
		"databases":                  "企业外部运维数据库连接配置表",
		"audit_logs":                 "用户登录与关键操作安全审计日志表",
		"users":                      "系统多租户与运维权限用户表",
		"dc_racks":                   "数据中心机房机柜空间管理表",
		"dc_rooms":                   "物理机房基础设施信息表",
		"ssh_keys":                   "SSH 远程访问管理密钥库",
		"snmp_credentials":           "SNMP v2c/v3 采集安全凭证库",
	}

	var tables []tableMeta
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			var count int
			_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", name)).Scan(&count)

			desc, ok := categoryMap[name]
			if !ok {
				desc = "系统运维支撑元数据表"
			}
			cat := "基础组件"
			if strings.HasPrefix(name, "network_") || strings.HasPrefix(name, "snmp_") {
				cat = "网络运维"
			} else if strings.HasPrefix(name, "server") || strings.HasPrefix(name, "container") {
				cat = "服务器/容器"
			} else if strings.HasPrefix(name, "alert") {
				cat = "告警监控"
			} else if strings.HasPrefix(name, "dc_") {
				cat = "数据中心"
			}

			tables = append(tables, tableMeta{
				name:     name,
				category: cat,
				rowCount: count,
				desc:     desc,
			})
		}
	}

	durationMs := time.Since(start).Milliseconds()

	// 优先排序：已登记业务描述且有数据的表排在前面
	sort.SliceStable(tables, func(i, j int) bool {
		descI := tables[i].desc != "系统运维支撑元数据表"
		descJ := tables[j].desc != "系统运维支撑元数据表"
		if descI != descJ {
			return descI
		}
		return tables[i].rowCount > tables[j].rowCount
	})

	var sb strings.Builder
	sb.WriteString("### 🗄️ NetMon 系统核心数据库架构与表清单\n\n")
	sb.WriteString(fmt.Sprintf("> **统计概览**：已检索到系统数据表 **%d** 个，总耗时 `%d ms`。所有表均受安全只读引擎保护。\n\n", len(tables), durationMs))

	sb.WriteString("| 序号 | 业务模块 | 数据表名 (Table) | 当前记录数 | 业务作用与描述 |\n")
	sb.WriteString("| :---: | :---: | :--- | :---: | :--- |\n")

	maxDisplay := 30
	if len(tables) < maxDisplay {
		maxDisplay = len(tables)
	}

	for idx := 0; idx < maxDisplay; idx++ {
		t := tables[idx]
		sb.WriteString(fmt.Sprintf("| %d | %s | `%s` | **%d** | %s |\n",
			idx+1, t.category, t.name, t.rowCount, t.desc))
	}

	if len(tables) > maxDisplay {
		sb.WriteString(fmt.Sprintf("\n> ℹ️ *其余 %d 个系统内部基础组件表已自动折叠，您可随时通过直接输入 SQL 语句按需查询任意表。*\n", len(tables)-maxDisplay))
	}

	sb.WriteString("\n> 💡 **查询小提示**：您可直接对我输入指令进行精细查询，例如：\n")
	sb.WriteString("- 「**查一下数据库里的告警**」\n")
	sb.WriteString("- 「**查一下数据库里的网络设备**」\n")
	sb.WriteString("- 「**SELECT name, ip_address, status FROM network_devices**」")

	return sb.String()
}

// sanitizeTableText 替换表格文本中的换行符和竖线，防止破坏 Markdown 表格排版
func sanitizeTableText(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// generateDbInsight 生成数据库查询的 AI 提炼结论
func generateDbInsight(queryType string, count int, cols []string, rows [][]string) string {
	if count == 0 {
		return "检索完成，当前数据表中无匹配记录。系统运行平稳，无异常残留数据。"
	}

	switch queryType {
	case "devices":
		onlineCount := 0
		offlineCount := 0
		for _, r := range rows {
			rowStr := strings.Join(r, " ")
			if strings.Contains(strings.ToLower(rowStr), "online") {
				onlineCount++
			} else if strings.Contains(strings.ToLower(rowStr), "offline") {
				offlineCount++
			}
		}
		return fmt.Sprintf("✅ **资产分布洞察**：在本次查询命中的 %d 台网络设备中，**%d 台在线运行**，**%d 台处于离线脱网状态**。建议对离线设备发起一次巡检体检排查物理线路。",
			count, onlineCount, offlineCount)

	case "alerts":
		return fmt.Sprintf("⚡ **告警态势分析**：当前最新检索到 %d 条告警事件。建议重点关注「严重」级别的告警事件，并结合网络拓扑排查故障根因。", count)

	case "inspections":
		return fmt.Sprintf("🩺 **巡检归档分析**：检索到最近 %d 次巡检归档记录。系统已实现单机与批量体检的全自动化记录追溯。", count)

	case "backups":
		return fmt.Sprintf("💾 **备份基线分析**：检索到 %d 条网络设备配置备份版本记录，已为生产网络配置合规与防误操作提供基线存档。", count)

	default:
		return fmt.Sprintf("📊 **查询摘要**：SQL 执行成功，返回 %d 行数据，包含 `%s` 等 %d 个字段列。数据已按照只读安全标准完成清洗与渲染。",
			count, strings.Join(cols[:min(3, len(cols))], ", "), len(cols))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
