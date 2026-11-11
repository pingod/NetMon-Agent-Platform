// 故障根因分析（/api/v1/root-cause-analysis）。
//
// 证据采集：告警本体 + 同设备 24h 相关告警 + 服务器最新指标 + 关联自愈执行记录。
// 根因推断：配置了大模型时走 LLM（ops.LLMHook 由 registry 注入，避免循环依赖），
// 否则使用关键词规则推断，结果明确标注「规则推断」，不冒充 AI 结论。
package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/services/audit"
)

// LLMHook 外部大模型调用入口，由 registry 注入 copilot.CallLLM；未配置模型时为 nil 或返回错误。
var LLMHook func(ctx context.Context, prompt string) (string, error)

type rcaRecord struct {
	ID              string   `json:"id"`
	AlertID         string   `json:"alert_id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	RootCause       string   `json:"root_cause"`
	Symptoms        []string `json:"symptoms"`
	Timeline        []rcaTimelineItem `json:"timeline"`
	Evidence        []string `json:"evidence"`
	Recommendations []string `json:"recommendations"`
	Confidence      float64  `json:"confidence"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	CompletedAt     string   `json:"completed_at"`
}

type rcaTimelineItem struct {
	Time  string `json:"time"`
	Event string `json:"event"`
}

const rcaSelectCols = `id, COALESCE(alert_id,''), title, COALESCE(description,''), COALESCE(status,'pending'),
	COALESCE(root_cause,''), COALESCE(symptoms,'[]'), COALESCE(timeline,'[]'), COALESCE(evidence,'[]'),
	COALESCE(recommendations,'[]'), COALESCE(confidence,0), COALESCE(created_at,''), COALESCE(updated_at,''), COALESCE(completed_at,'')`

func scanRca(scan func(dest ...interface{}) error) (*rcaRecord, error) {
	var r rcaRecord
	var symptoms, timeline, evidence, recs string
	if err := scan(&r.ID, &r.AlertID, &r.Title, &r.Description, &r.Status, &r.RootCause,
		&symptoms, &timeline, &evidence, &recs, &r.Confidence, &r.CreatedAt, &r.UpdatedAt, &r.CompletedAt); err != nil {
		return nil, err
	}
	r.Symptoms = []string{}
	r.Timeline = []rcaTimelineItem{}
	r.Evidence = []string{}
	r.Recommendations = []string{}
	_ = json.Unmarshal([]byte(symptoms), &r.Symptoms)
	_ = json.Unmarshal([]byte(timeline), &r.Timeline)
	_ = json.Unmarshal([]byte(evidence), &r.Evidence)
	_ = json.Unmarshal([]byte(recs), &r.Recommendations)
	return &r, nil
}

// RcaRoutes /api/v1/root-cause-analysis
func RcaRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", rcaList)
	r.Get("/stats", rcaStats)
	r.Get("/{id}", rcaDetail)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/", rcaCreate)
	r.With(write).Post("/{id}/analyze", rcaAnalyze)
	r.With(write).Post("/auto-analyze/{alertId}", rcaAutoAnalyze)
	r.With(middleware.RequireRole("admin")).Delete("/{id}", rcaDelete)
	return r
}

func rcaList(w http.ResponseWriter, r *http.Request) {
	rows, err := appdb.DB().Query(`SELECT ` + rcaSelectCols + ` FROM root_cause_analyses ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		httpx.Respond(w, []interface{}{})
		return
	}
	defer rows.Close()
	list := []rcaRecord{}
	for rows.Next() {
		if rec, err := scanRca(rows.Scan); err == nil {
			list = append(list, *rec)
		}
	}
	httpx.Respond(w, list)
}

func rcaDetail(w http.ResponseWriter, r *http.Request) {
	rec, err := scanRca(appdb.DB().QueryRow(`SELECT `+rcaSelectCols+` FROM root_cause_analyses WHERE id = ?`, chi.URLParam(r, "id")).Scan)
	if err != nil {
		httpx.RespondError(w, "分析记录不存在", http.StatusNotFound)
		return
	}
	httpx.Respond(w, rec)
}

func rcaStats(w http.ResponseWriter, r *http.Request) {
	db := appdb.DB()
	var todayCount, totalCompleted, autoRemediations int
	var avgConfidence sql.NullFloat64
	db.QueryRow(`SELECT COUNT(*) FROM root_cause_analyses WHERE date(created_at) = date('now','localtime')`).Scan(&todayCount)
	db.QueryRow(`SELECT COUNT(*), AVG(confidence) FROM root_cause_analyses WHERE status = 'completed'`).Scan(&totalCompleted, &avgConfidence)
	db.QueryRow(`SELECT COUNT(*) FROM remediation_executions WHERE alert_id IN (SELECT alert_id FROM root_cause_analyses WHERE alert_id IS NOT NULL AND alert_id != '')`).Scan(&autoRemediations)
	httpx.Respond(w, map[string]interface{}{
		"todayCount":       todayCount,
		"totalCompleted":   totalCompleted,
		"avgConfidence":    avgConfidence.Float64,
		"autoRemediations": autoRemediations,
		// 平台暂无告警误报标记能力，诚实返回 0
		"falsePositives": 0,
	})
}

type rcaCreateBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	AlertID     string `json:"alert_id"`
}

func rcaCreate(w http.ResponseWriter, r *http.Request) {
	var b rcaCreateBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
		return
	}
	b.Title = strings.TrimSpace(b.Title)
	if b.Title == "" {
		httpx.RespondError(w, "标题不能为空", http.StatusBadRequest)
		return
	}
	if len([]rune(b.Title)) > 200 {
		httpx.RespondError(w, "标题过长（最多 200 字）", http.StatusBadRequest)
		return
	}
	if b.AlertID != "" {
		var n int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM alerts WHERE id = ?`, b.AlertID).Scan(&n)
		if n == 0 {
			httpx.RespondError(w, "关联告警不存在", http.StatusBadRequest)
			return
		}
	}
	rec := insertRca(b)
	user := middleware.GetUser(r)
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "rca_create", ResourceType: "root_cause_analysis", ResourceID: rec.ID, Details: map[string]interface{}{"title": rec.Title, "alert_id": rec.AlertID}, IPAddress: middleware.ClientIP(r)})
	httpx.RespondStatus(w, http.StatusCreated, rec)
}

func insertRca(b rcaCreateBody) *rcaRecord {
	id := uuid.NewString()
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`INSERT INTO root_cause_analyses (id, alert_id, title, description, status, created_at, updated_at)
		VALUES (?, NULLIF(?,''), ?, ?, 'pending', ?, ?)`, id, b.AlertID, b.Title, b.Description, now, now)
	rec, _ := scanRca(appdb.DB().QueryRow(`SELECT ` + rcaSelectCols + ` FROM root_cause_analyses WHERE id = ?`, id).Scan)
	return rec
}

func rcaAnalyze(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var status string
	if appdb.DB().QueryRow(`SELECT status FROM root_cause_analyses WHERE id = ?`, id).Scan(&status) != nil {
		httpx.RespondError(w, "分析记录不存在", http.StatusNotFound)
		return
	}
	if status == "analyzing" {
		httpx.RespondError(w, "分析正在进行中", http.StatusConflict)
		return
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE root_cause_analyses SET status = 'analyzing', updated_at = ? WHERE id = ?`, now, id)
	go runRcaAnalysis(id)
	httpx.Respond(w, map[string]interface{}{"id": id, "status": "analyzing"})
}

// rcaAutoAnalyze 从告警一键创建并分析；同一告警已有未失败的分析时直接复用
func rcaAutoAnalyze(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "alertId")
	var title, content string
	if appdb.DB().QueryRow(`SELECT title, COALESCE(content,'') FROM alerts WHERE id = ?`, alertID).Scan(&title, &content) != nil {
		httpx.RespondError(w, "告警不存在", http.StatusNotFound)
		return
	}
	var existingID, existingStatus string
	if appdb.DB().QueryRow(`SELECT id, status FROM root_cause_analyses WHERE alert_id = ? AND status != 'failed' ORDER BY created_at DESC LIMIT 1`, alertID).Scan(&existingID, &existingStatus) == nil {
		httpx.Respond(w, map[string]interface{}{"id": existingID, "status": existingStatus, "reused": true})
		return
	}
	rec := insertRca(rcaCreateBody{Title: "告警根因分析：" + title, Description: content, AlertID: alertID})
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE root_cause_analyses SET status = 'analyzing', updated_at = ? WHERE id = ?`, now, rec.ID)
	go runRcaAnalysis(rec.ID)
	user := middleware.GetUser(r)
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "rca_auto_analyze", ResourceType: "root_cause_analysis", ResourceID: rec.ID, Details: map[string]interface{}{"alert_id": alertID}, IPAddress: middleware.ClientIP(r)})
	httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": rec.ID, "status": "analyzing"})
}

func rcaDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := appdb.DB().Exec(`DELETE FROM root_cause_analyses WHERE id = ?`, id)
	if err != nil {
		httpx.RespondError(w, "删除失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "分析记录不存在", http.StatusNotFound)
		return
	}
	user := middleware.GetUser(r)
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "rca_delete", ResourceType: "root_cause_analysis", ResourceID: id, IPAddress: middleware.ClientIP(r)})
	httpx.RespondMessage(w, "分析记录已删除")
}

// ── 分析执行 ──

type rcaAlertInfo struct {
	ID        string
	Source    string
	Severity  string
	Title     string
	Content   string
	Status    string
	DeviceID  string
	CreatedAt string
}

func runRcaAnalysis(id string) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf("RCA 分析 panic: %v", rec)
			markRcaFailed(id, fmt.Sprintf("分析异常: %v", rec))
		}
	}()
	rec, err := scanRca(appdb.DB().QueryRow(`SELECT ` + rcaSelectCols + ` FROM root_cause_analyses WHERE id = ?`, id).Scan)
	if err != nil {
		return
	}

	var alert *rcaAlertInfo
	if rec.AlertID != "" {
		var a rcaAlertInfo
		if appdb.DB().QueryRow(`SELECT id, COALESCE(source,''), COALESCE(severity,''), title, COALESCE(content,''), COALESCE(status,''), COALESCE(device_id,''), COALESCE(created_at,'') FROM alerts WHERE id = ?`, rec.AlertID).
			Scan(&a.ID, &a.Source, &a.Severity, &a.Title, &a.Content, &a.Status, &a.DeviceID, &a.CreatedAt) == nil {
			alert = &a
		}
	}

	symptoms := []string{}
	timeline := []rcaTimelineItem{}
	evidence := []string{}

	if alert != nil {
		symptoms = append(symptoms, fmt.Sprintf("%s（级别：%s，来源：%s）", alert.Title, alert.Severity, alert.Source))
		if alert.Content != "" {
			symptoms = append(symptoms, alert.Content)
		}
		timeline = append(timeline, rcaTimelineItem{Time: alert.CreatedAt, Event: "告警触发：" + alert.Title})
		evidence = append(evidence, fmt.Sprintf("告警当前状态：%s，触发时间：%s", alert.Status, alert.CreatedAt))
	} else if rec.Description != "" {
		symptoms = append(symptoms, rec.Description)
	}

	// 同设备（或同来源）24h 相关告警
	relatedCount := 0
	if alert != nil {
		q := `SELECT title, COALESCE(severity,''), COALESCE(created_at,'') FROM alerts
			WHERE id != ? AND created_at >= datetime(?, '-24 hours') AND created_at <= datetime(?, '+1 hour')`
		args := []interface{}{alert.ID, alert.CreatedAt, alert.CreatedAt}
		if alert.DeviceID != "" {
			q += ` AND device_id = ?`
			args = append(args, alert.DeviceID)
		} else {
			q += ` AND source = ?`
			args = append(args, alert.Source)
		}
		q += ` ORDER BY created_at DESC LIMIT 10`
		if rows, err := appdb.DB().Query(q, args...); err == nil {
			for rows.Next() {
				var t, sev, ts string
				if rows.Scan(&t, &sev, &ts) == nil {
					relatedCount++
					symptoms = append(symptoms, "相关告警："+t)
					timeline = append(timeline, rcaTimelineItem{Time: ts, Event: "相关告警：" + t})
				}
			}
			rows.Close()
		}
		if relatedCount > 0 {
			evidence = append(evidence, fmt.Sprintf("24 小时内同%s相关告警 %d 条", deviceOrSource(alert), relatedCount))
		} else {
			evidence = append(evidence, "24 小时内无同维度相关告警，倾向于孤立事件")
		}
	}

	// 服务器最新指标（按设备 ID / 名称 / IP 匹配 servers）
	if alert != nil {
		if m := latestServerMetrics(alert.DeviceID, alert.Title, alert.Content); m != "" {
			evidence = append(evidence, m)
		}
	}

	// 关联自愈执行
	if alert != nil {
		if rows, err := appdb.DB().Query(`SELECT id, status, COALESCE(status_reason,''), COALESCE(created_at,'') FROM remediation_executions WHERE alert_id = ? ORDER BY created_at DESC LIMIT 5`, alert.ID); err == nil {
			for rows.Next() {
				var eid, st, reason, ts string
				if rows.Scan(&eid, &st, &reason, &ts) == nil {
					evidence = append(evidence, fmt.Sprintf("自愈执行 %s：%s%s", eid[:8], st, reasonSuffix(reason)))
					timeline = append(timeline, rcaTimelineItem{Time: ts, Event: "自愈策略执行：" + st})
				}
			}
			rows.Close()
		}
	}

	if len(evidence) == 0 {
		evidence = append(evidence, "未关联告警，仅基于问题描述进行规则推断")
	}
	sort.SliceStable(timeline, func(i, j int) bool { return timeline[i].Time < timeline[j].Time })
	if len(symptoms) > 10 {
		symptoms = symptoms[:10]
	}
	if len(timeline) > 15 {
		timeline = timeline[:15]
	}

	rootCause, recommendations := inferRootCause(rec, alert, symptoms, evidence)

	// 置信度为证据覆盖度的启发式估算，非统计意义置信区间
	confidence := 0.3 + 0.08*float64(len(evidence))
	if confidence > 0.9 {
		confidence = 0.9
	}

	symJSON, _ := json.Marshal(symptoms)
	tlJSON, _ := json.Marshal(timeline)
	evJSON, _ := json.Marshal(evidence)
	rcJSON, _ := json.Marshal(recommendations)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = appdb.DB().Exec(`UPDATE root_cause_analyses SET status='completed', root_cause=?, symptoms=?, timeline=?, evidence=?, recommendations=?, confidence=?, completed_at=?, updated_at=? WHERE id=?`,
		rootCause, string(symJSON), string(tlJSON), string(evJSON), string(rcJSON), confidence, now, now, id)
	if err != nil {
		markRcaFailed(id, "保存分析结果失败: "+err.Error())
		return
	}
	logger.Infof("RCA %s completed (%d evidence items)", id, len(evidence))
}

func markRcaFailed(id, reason string) {
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE root_cause_analyses SET status='failed', root_cause=?, updated_at=? WHERE id=?`, "分析失败："+reason, now, id)
}

func deviceOrSource(a *rcaAlertInfo) string {
	if a.DeviceID != "" {
		return "设备"
	}
	return "来源"
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return "（" + reason + "）"
}

// latestServerMetrics 尝试把告警关联到纳管服务器并取最新指标
func latestServerMetrics(deviceID, title, content string) string {
	db := appdb.DB()
	var serverID, name string
	if deviceID != "" {
		if db.QueryRow(`SELECT id, hostname FROM servers WHERE id = ?`, deviceID).Scan(&serverID, &name) == nil {
			return formatServerMetrics(serverID, name)
		}
	}
	// 从标题/内容中匹配服务器名或 IP
	rows, err := db.Query(`SELECT id, hostname, COALESCE(ip_address,'') FROM servers LIMIT 200`)
	if err != nil {
		return ""
	}
	defer rows.Close()
	hay := strings.ToLower(title + " " + content)
	for rows.Next() {
		var sid, hostname, ip string
		if rows.Scan(&sid, &hostname, &ip) != nil {
			continue
		}
		if (hostname != "" && strings.Contains(hay, strings.ToLower(hostname))) || (ip != "" && strings.Contains(hay, ip)) {
			return formatServerMetrics(sid, hostname)
		}
	}
	return ""
}

func formatServerMetrics(serverID, name string) string {
	var cpu, mem, disk sql.NullFloat64
	var ts string
	err := appdb.DB().QueryRow(`SELECT cpu_usage, memory_usage, disk_usage, COALESCE(collected_at,'') FROM server_metrics WHERE server_id = ? ORDER BY collected_at DESC LIMIT 1`, serverID).
		Scan(&cpu, &mem, &disk, &ts)
	if err != nil {
		return ""
	}
	parts := []string{}
	if cpu.Valid {
		parts = append(parts, fmt.Sprintf("CPU %.1f%%", cpu.Float64))
	}
	if mem.Valid {
		parts = append(parts, fmt.Sprintf("内存 %.1f%%", mem.Float64))
	}
	if disk.Valid {
		parts = append(parts, fmt.Sprintf("磁盘 %.1f%%", disk.Float64))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("服务器 %s 最新指标：%s（采集于 %s）", name, strings.Join(parts, " / "), ts)
}

// inferRootCause 优先 LLM，失败/未配置回退规则推断
func inferRootCause(rec *rcaRecord, alert *rcaAlertInfo, symptoms, evidence []string) (string, []string) {
	if LLMHook != nil {
		prompt := buildRcaPrompt(rec, alert, symptoms, evidence)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if reply, err := LLMHook(ctx, prompt); err == nil && strings.TrimSpace(reply) != "" {
			if rc, recs, ok := parseRcaLLMReply(reply); ok {
				return rc, recs
			}
			// 非 JSON 回复也接受为根因文本
			return strings.TrimSpace(reply), heuristicRecommendations(rec, alert)
		}
	}
	return heuristicRootCause(rec, alert)
}

func buildRcaPrompt(rec *rcaRecord, alert *rcaAlertInfo, symptoms, evidence []string) string {
	var sb strings.Builder
	sb.WriteString("你是资深 SRE 故障根因分析专家。请基于以下真实现场证据推断最可能的根本原因，并给出可执行的修复建议。\n")
	sb.WriteString("只输出 JSON，格式：{\"root_cause\": \"...\", \"recommendations\": [\"...\", \"...\"]}，不要输出其他内容。\n\n")
	sb.WriteString("分析主题：" + rec.Title + "\n")
	if rec.Description != "" {
		sb.WriteString("问题描述：" + rec.Description + "\n")
	}
	if alert != nil {
		sb.WriteString(fmt.Sprintf("告警：%s（severity=%s, source=%s, device_id=%s）\n", alert.Title, alert.Severity, alert.Source, alert.DeviceID))
	}
	sb.WriteString("症状：\n")
	for _, s := range symptoms {
		sb.WriteString("- " + s + "\n")
	}
	sb.WriteString("证据：\n")
	for _, e := range evidence {
		sb.WriteString("- " + e + "\n")
	}
	return sb.String()
}

func parseRcaLLMReply(reply string) (string, []string, bool) {
	reply = strings.TrimSpace(reply)
	// 容忍 ```json 包裹
	if strings.HasPrefix(reply, "```") {
		if i := strings.Index(reply, "\n"); i >= 0 {
			reply = reply[i+1:]
		}
		reply = strings.TrimSuffix(strings.TrimSpace(reply), "```")
	}
	var out struct {
		RootCause       string   `json:"root_cause"`
		Recommendations []string `json:"recommendations"`
	}
	if err := json.Unmarshal([]byte(reply), &out); err != nil || strings.TrimSpace(out.RootCause) == "" {
		return "", nil, false
	}
	if len(out.Recommendations) == 0 {
		out.Recommendations = []string{"请结合现场证据人工复核"}
	}
	return out.RootCause, out.Recommendations, true
}

// heuristicRootCause 关键词规则推断，明确标注非 AI 结论
func heuristicRootCause(rec *rcaRecord, alert *rcaAlertInfo) (string, []string) {
	hay := strings.ToLower(rec.Title + " " + rec.Description)
	if alert != nil {
		hay += " " + strings.ToLower(alert.Title+" "+alert.Content)
	}
	switch {
	case strings.Contains(hay, "cpu"):
		return "【规则推断】CPU 资源耗尽：可能存在失控进程、流量突增或容量不足。建议结合 top/容器 stats 定位高耗进程。",
			[]string{"登录目标机执行 top -b -n1 | head -20 定位高 CPU 进程", "检查是否有突发流量或定时任务叠加", "持续高负载时扩容或限制异常进程资源（cgroup/容器 limit）"}
	case strings.Contains(hay, "memory") || strings.Contains(hay, "内存") || strings.Contains(hay, "oom"):
		return "【规则推断】内存不足：可能存在内存泄漏、缓存膨胀或实例规格偏小，存在 OOM 风险。",
			[]string{"执行 free -m 与 ps aux --sort=-%mem | head 定位内存大户", "检查应用近期发布是否引入泄漏", "必要时重启异常进程并升配内存"}
	case strings.Contains(hay, "disk") || strings.Contains(hay, "磁盘"):
		return "【规则推断】磁盘空间不足：日志/镜像/临时文件堆积最常见，可能导致写入失败与服务异常。",
			[]string{"执行 df -h 与 du -xh --max-depth=1 / 2>/dev/null | sort -rh | head 定位占用", "清理过期日志、悬空镜像（docker image prune）与临时文件", "配置日志轮转与磁盘水位告警"}
	case strings.Contains(hay, "netdown") || strings.Contains(hay, "网络") || strings.Contains(hay, "network") || strings.Contains(hay, "丢包"):
		return "【规则推断】网络连通性异常：可能为链路故障、网卡/驱动问题、ACL 变更或上游设备故障。",
			[]string{"从近到远分段 ping/traceroute 定位断点", "检查网卡状态（ip link）与最近的网络变更", "核对交换机端口状态与 ACL/安全组配置"}
	case strings.Contains(hay, "dbdown") || strings.Contains(hay, "数据库") || strings.Contains(hay, "mysql") || strings.Contains(hay, "postgres"):
		return "【规则推断】数据库服务不可用：可能为进程崩溃、连接数耗尽、磁盘满或主从切换异常。",
			[]string{"检查数据库进程与错误日志", "确认连接数是否打满（show status like 'Threads%'）", "检查磁盘空间与最近的主从/发布变更"}
	case strings.Contains(hay, "svcdown") || strings.Contains(hay, "服务") || strings.Contains(hay, "health"):
		return "【规则推断】服务健康检查失败：可能为进程退出、端口监听异常、依赖故障或发布引入的回归。",
			[]string{"检查服务进程与端口监听（ss -tlnp）", "查看服务最近日志与发布时间线", "验证下游依赖（数据库/缓存/消息队列）可用性"}
	default:
		return "【规则推断】现有证据不足以定位具体根因。已汇总相关告警、指标与执行记录，建议人工结合时间线进一步排查；配置大模型后可获得更深入的分析。",
			[]string{"按时间线核对告警前后的变更（发布/配置/扩缩容）", "查看目标设备/服务器的实时指标与日志", "在系统设置中配置 AI 模型后重新分析，获取 LLM 辅助推断"}
	}
}

func heuristicRecommendations(rec *rcaRecord, alert *rcaAlertInfo) []string {
	_, recs := heuristicRootCause(rec, alert)
	return recs
}