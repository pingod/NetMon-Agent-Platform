// 自动伸缩（/api/v1/auto-scale）。
// 容器目标：按 Compose 项目/服务的实时 docker stats 指标，通过 docker compose --scale 真实扩缩容。
// VM 目标依赖虚拟化平台 API（当前未接入），创建规则时明确拒绝并说明。
package ops

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
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

type scaleRuleBody struct {
	Name             string  `json:"name"`
	TargetType       string  `json:"targetType"`
	TargetID         string  `json:"targetId"`
	TargetName       string  `json:"targetName"`
	MetricType       string  `json:"metricType"`
	Threshold        float64 `json:"threshold"`
	TargetValue      float64 `json:"targetValue"`
	MinInstances     int     `json:"minInstances"`
	MaxInstances     int     `json:"maxInstances"`
	ScaleUpCooldown  int     `json:"scaleUpCooldown"`
	ScaleDownCooldown int    `json:"scaleDownCooldown"`
	Enabled          *bool   `json:"enabled"`
}

func (b *scaleRuleBody) validate() string {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len([]rune(b.Name)) > 100 {
		return "规则名称必填且不超过 100 字"
	}
	if b.TargetType != "container" {
		return "当前仅支持容器目标：VM 伸缩需要虚拟化平台 API，尚未接入"
	}
	if b.MetricType != "cpu" && b.MetricType != "memory" {
		return "指标类型仅支持 cpu/memory（pod_count 依赖 K8s 实时数据，尚未接入）"
	}
	b.TargetName = strings.TrimSpace(b.TargetName)
	if b.TargetName == "" || !strings.Contains(b.TargetName, "/") {
		return "目标名称格式为「Compose 项目名/服务名」，如 web/api"
	}
	if b.Threshold < 1 || b.Threshold > 100 || b.TargetValue < 1 || b.TargetValue > 100 {
		return "阈值与目标值需在 1-100 之间"
	}
	if b.MinInstances < 1 || b.MaxInstances < b.MinInstances || b.MaxInstances > 100 {
		return "实例数范围不合法（1 ≤ 最小 ≤ 最大 ≤ 100）"
	}
	if b.ScaleUpCooldown < 0 || b.ScaleUpCooldown > 3600 || b.ScaleDownCooldown < 0 || b.ScaleDownCooldown > 3600 {
		return "冷却时间需在 0-3600 秒之间"
	}
	return ""
}

// resolveScaleTarget 按项目名定位 compose 项目并校验服务存在
func resolveScaleTarget(targetName string) (projectID string, errMsg string) {
	parts := strings.SplitN(targetName, "/", 2)
	projName, svcName := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	var content string
	err := appdb.DB().QueryRow(`SELECT id, COALESCE(compose_content,'') FROM compose_projects WHERE name = ?`, projName).Scan(&projectID, &content)
	if err != nil {
		return "", "Compose 项目不存在: " + projName
	}
	services, err := parseComposeServices(content)
	if err != nil {
		return "", "Compose YAML 解析失败: " + err.Error()
	}
	for _, s := range services {
		if s.Name == svcName {
			return projectID, ""
		}
	}
	return "", "项目中不存在服务: " + svcName
}

func scaleRuleToMap(id, name, targetType, targetID, targetName, metricType string, threshold, targetValue float64,
	minInst, maxInst, upCD, downCD, enabled int, createdAt, lastScale string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "name": name, "targetType": targetType, "targetId": targetID, "targetName": targetName,
		"metricType": metricType, "threshold": threshold, "targetValue": targetValue,
		"minInstances": minInst, "maxInstances": maxInst,
		"scaleUpCooldown": upCD, "scaleDownCooldown": downCD,
		"enabled": enabled == 1, "createdAt": createdAt, "lastScaleTime": lastScale,
	}
}

const scaleRuleCols = `id, name, target_type, target_id, COALESCE(target_name,''), metric_type, threshold, target_value,
	COALESCE(min_instances,1), COALESCE(max_instances,10), COALESCE(scale_up_cooldown,300), COALESCE(scale_down_cooldown,600),
	COALESCE(enabled,1), COALESCE(created_at,''), COALESCE(last_scale_time,'')`

func scanScaleRule(scan func(dest ...interface{}) error) (map[string]interface{}, error) {
	var id, name, tType, tID, tName, mType, createdAt, lastScale string
	var threshold, targetValue float64
	var minI, maxI, upCD, downCD, enabled int
	if err := scan(&id, &name, &tType, &tID, &tName, &mType, &threshold, &targetValue, &minI, &maxI, &upCD, &downCD, &enabled, &createdAt, &lastScale); err != nil {
		return nil, err
	}
	return scaleRuleToMap(id, name, tType, tID, tName, mType, threshold, targetValue, minI, maxI, upCD, downCD, enabled, createdAt, lastScale), nil
}

// AutoScaleRoutes /api/v1/auto-scale
func AutoScaleRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/rules", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT ` + scaleRuleCols + ` FROM auto_scale_rules ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		list := []map[string]interface{}{}
		for rows.Next() {
			if m, err := scanScaleRule(rows.Scan); err == nil {
				list = append(list, m)
			}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin")).Post("/rules", func(w http.ResponseWriter, r *http.Request) {
		var body scaleRuleBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&body); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		if msg := body.validate(); msg != "" {
			httpx.RespondError(w, msg, http.StatusBadRequest)
			return
		}
		projectID, errMsg := resolveScaleTarget(body.TargetName)
		if errMsg != "" {
			httpx.RespondError(w, errMsg, http.StatusBadRequest)
			return
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")
		enabled := 1
		if body.Enabled != nil && !*body.Enabled {
			enabled = 0
		}
		_, err := appdb.DB().Exec(`INSERT INTO auto_scale_rules
			(id, name, target_type, target_id, target_name, metric_type, threshold, target_value,
			 min_instances, max_instances, scale_up_cooldown, scale_down_cooldown, enabled, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, body.Name, body.TargetType, projectID, body.TargetName, body.MetricType, body.Threshold, body.TargetValue,
			body.MinInstances, body.MaxInstances, body.ScaleUpCooldown, body.ScaleDownCooldown, enabled, now, now)
		if err != nil {
			httpx.RespondError(w, "创建规则失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "autoscale_rule_create", ResourceType: "auto_scale_rule", ResourceID: id, Details: map[string]interface{}{"name": body.Name}, IPAddress: middleware.ClientIP(r)})
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id})
	})

	r.With(middleware.RequireRole("admin")).Put("/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body scaleRuleBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024)).Decode(&body); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		if msg := body.validate(); msg != "" {
			httpx.RespondError(w, msg, http.StatusBadRequest)
			return
		}
		projectID, errMsg := resolveScaleTarget(body.TargetName)
		if errMsg != "" {
			httpx.RespondError(w, errMsg, http.StatusBadRequest)
			return
		}
		enabled := 1
		if body.Enabled != nil && !*body.Enabled {
			enabled = 0
		}
		res, err := appdb.DB().Exec(`UPDATE auto_scale_rules SET
			name=?, target_type=?, target_id=?, target_name=?, metric_type=?, threshold=?, target_value=?,
			min_instances=?, max_instances=?, scale_up_cooldown=?, scale_down_cooldown=?, enabled=?, updated_at=?
			WHERE id=?`,
			body.Name, body.TargetType, projectID, body.TargetName, body.MetricType, body.Threshold, body.TargetValue,
			body.MinInstances, body.MaxInstances, body.ScaleUpCooldown, body.ScaleDownCooldown, enabled,
			time.Now().Format("2006-01-02 15:04:05"), id)
		if err != nil || affected(res) == 0 {
			httpx.RespondError(w, "规则不存在", http.StatusNotFound)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "autoscale_rule_update", ResourceType: "auto_scale_rule", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		httpx.RespondMessage(w, "规则已更新")
	})

	r.With(middleware.RequireRole("admin")).Delete("/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		res, err := appdb.DB().Exec(`DELETE FROM auto_scale_rules WHERE id = ?`, id)
		if err != nil || affected(res) == 0 {
			httpx.RespondError(w, "规则不存在", http.StatusNotFound)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "autoscale_rule_delete", ResourceType: "auto_scale_rule", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		httpx.RespondMessage(w, "规则已删除")
	})

	// 历史：前端读取 res.data.data 与 res.data.total（顶层 envelope）
	r.Get("/history", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(q.Get("pageSize"))
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}
		where := "WHERE 1=1"
		args := []interface{}{}
		for _, f := range []struct{ param, col string }{{"startTime", "timestamp >= ?"}, {"endTime", "timestamp <= ?"}} {
			if v := q.Get(f.param); v != "" {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					where += " AND " + f.col
					args = append(args, t.Local().Format("2006-01-02 15:04:05"))
				}
			}
		}
		var total int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM auto_scale_history `+where, args...).Scan(&total)
		rows, err := appdb.DB().Query(`SELECT id, COALESCE(rule_name,''), COALESCE(target_id,''), action,
			COALESCE(previous_count,0), COALESCE(current_count,0), COALESCE(metric_value,0), result, COALESCE(reason,''), timestamp
			FROM auto_scale_history `+where+` ORDER BY timestamp DESC LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
		if err != nil {
			httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{"success": true, "data": []interface{}{}, "total": 0})
			return
		}
		defer rows.Close()
		list := []map[string]interface{}{}
		for rows.Next() {
			var id, ruleName, target, action, result, reason, ts string
			var prev, cur int
			var metric float64
			if rows.Scan(&id, &ruleName, &target, &action, &prev, &cur, &metric, &result, &reason, &ts) == nil {
				list = append(list, map[string]interface{}{
					"id": id, "time": ts, "ruleName": ruleName, "target": target, "action": action,
					"beforeCount": prev, "afterCount": cur, "metricValue": metric, "result": result, "reason": reason,
				})
			}
		}
		httpx.RespondRaw(w, http.StatusOK, map[string]interface{}{"success": true, "data": list, "total": total})
	})

	r.Get("/summary", func(w http.ResponseWriter, r *http.Request) {
		var activeRules, todayUp, todayDown int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM auto_scale_rules WHERE enabled=1`).Scan(&activeRules)
		appdb.DB().QueryRow(`SELECT
			COALESCE(SUM(CASE WHEN action='scale_up' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN action='scale_down' THEN 1 ELSE 0 END),0)
			FROM auto_scale_history WHERE result='success' AND date(timestamp)=date('now','localtime')`).Scan(&todayUp, &todayDown)
		// 管理实例总数：每条启用规则最近一次伸缩后的实例数（无记录按下限计）
		var managed int
		rows, err := appdb.DB().Query(`SELECT r.id, COALESCE(r.min_instances,1),
			(SELECT h.current_count FROM auto_scale_history h WHERE h.rule_id=r.id ORDER BY h.timestamp DESC LIMIT 1)
			FROM auto_scale_rules r WHERE r.enabled=1`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var rid string
				var minI int
				var lastCount *int
				if rows.Scan(&rid, &minI, &lastCount) == nil {
					if lastCount != nil {
						managed += *lastCount
					} else {
						managed += minI
					}
				}
			}
		}
		httpx.Respond(w, map[string]interface{}{
			"activeRules": activeRules, "todayScaleUp": todayUp, "todayScaleDown": todayDown, "totalManagedInstances": managed,
		})
	})

	return r
}

// ── 伸缩引擎 ──

// StartAutoScaleEngine 每 30s 评估启用规则，真实执行 docker compose 扩缩容
func StartAutoScaleEngine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			evaluateScaleRules()
		}
	}()
	logger.Infof("Auto-scale engine started")
}

type scaleRuleRow struct {
	ID, Name, TargetID, TargetName, MetricType string
	Threshold, TargetValue                     float64
	MinInstances, MaxInstances                 int
	ScaleUpCooldown, ScaleDownCooldown         int
}

func evaluateScaleRules() {
	rows, err := appdb.DB().Query(`SELECT id, name, target_id, COALESCE(target_name,''), metric_type, threshold, target_value,
		COALESCE(min_instances,1), COALESCE(max_instances,10), COALESCE(scale_up_cooldown,300), COALESCE(scale_down_cooldown,600)
		FROM auto_scale_rules WHERE enabled=1`)
	if err != nil {
		return
	}
	var rules []scaleRuleRow
	for rows.Next() {
		var r scaleRuleRow
		if rows.Scan(&r.ID, &r.Name, &r.TargetID, &r.TargetName, &r.MetricType, &r.Threshold, &r.TargetValue,
			&r.MinInstances, &r.MaxInstances, &r.ScaleUpCooldown, &r.ScaleDownCooldown) == nil {
			rules = append(rules, r)
		}
	}
	rows.Close()
	if len(rules) == 0 {
		return
	}

	client, _, err := getDockerSSHClient("")
	if err != nil {
		for _, r := range rules {
			recordScaleFailure(r, 0, "未配置可用 Docker 宿主机，无法采集指标与执行伸缩")
		}
		return
	}
	defer client.Close()

	for _, r := range rules {
		evaluateOneScaleRule(client, r)
	}
}

// recordScaleFailure 失败也遵守冷却，避免无宿主机时每 30s 刷一条失败历史
func recordScaleFailure(r scaleRuleRow, count int, reason string) {
	if scaleCooldownActive(r.ID, "scale_up", r.ScaleUpCooldown) {
		return
	}
	insertScaleHistory(r, "scale_up", count, count, 0, "failed", reason)
}

func scaleCooldownActive(ruleID, action string, cooldown int) bool {
	if cooldown <= 0 {
		return false
	}
	var last string
	err := appdb.DB().QueryRow(`SELECT timestamp FROM auto_scale_history WHERE rule_id=? AND action=? ORDER BY timestamp DESC LIMIT 1`, ruleID, action).Scan(&last)
	if err != nil || last == "" {
		return false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", last, time.Local)
	if err != nil {
		return false
	}
	return time.Since(t) < time.Duration(cooldown)*time.Second
}

func insertScaleHistory(r scaleRuleRow, action string, prev, cur int, metric float64, result, reason string) {
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`INSERT INTO auto_scale_history
		(id, rule_id, rule_name, target_type, target_id, action, previous_count, current_count, metric_value, result, reason, timestamp)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), r.ID, r.Name, "container", r.TargetName, action, prev, cur, metric, result, reason, now)
	appdb.DB().Exec(`UPDATE auto_scale_rules SET last_scale_time=? WHERE id=?`, now, r.ID)
}

func evaluateOneScaleRule(client dockerCommandClient, r scaleRuleRow) {
	parts := strings.SplitN(r.TargetName, "/", 2)
	if len(parts) != 2 {
		return
	}
	projName, svcName := parts[0], parts[1]

	idsOut, _, err := client.ExecCommand(fmt.Sprintf(
		"docker ps -q --filter label=com.docker.compose.project=%s --filter label=com.docker.compose.service=%s",
		shellQuote(projName), shellQuote(svcName)), 20*time.Second)
	if err != nil {
		recordScaleFailure(r, 0, "查询容器失败: "+err.Error())
		return
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(idsOut), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ids = append(ids, line)
		}
	}
	current := len(ids)
	if current == 0 {
		return // 服务未部署，不擅自创建
	}

	statsOut, _, err := client.ExecCommand("docker stats --no-stream --format '{{json .}}' "+strings.Join(ids, " "), 30*time.Second)
	if err != nil {
		recordScaleFailure(r, current, "采集指标失败: "+err.Error())
		return
	}
	var sum float64
	var n int
	for _, line := range strings.Split(strings.TrimSpace(statsOut), "\n") {
		var s struct {
			CPUPerc string `json:"CPUPerc"`
			MemPerc string `json:"MemPerc"`
		}
		if json.Unmarshal([]byte(line), &s) != nil {
			continue
		}
		raw := s.CPUPerc
		if r.MetricType == "memory" {
			raw = s.MemPerc
		}
		if v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(raw), "%"), 64); err == nil {
			sum += v
			n++
		}
	}
	if n == 0 {
		return
	}
	metric := sum / float64(n)

	desired := int(math.Ceil(float64(current) * metric / r.TargetValue))
	if desired < r.MinInstances {
		desired = r.MinInstances
	}
	if desired > r.MaxInstances {
		desired = r.MaxInstances
	}

	action := ""
	switch {
	case metric >= r.Threshold && desired > current:
		action = "scale_up"
	case metric < r.TargetValue && desired < current:
		action = "scale_down"
	default:
		return
	}
	cooldown := r.ScaleUpCooldown
	if action == "scale_down" {
		cooldown = r.ScaleDownCooldown
	}
	if scaleCooldownActive(r.ID, action, cooldown) {
		return
	}

	var yamlContent string
	if appdb.DB().QueryRow(`SELECT COALESCE(compose_content,'') FROM compose_projects WHERE id=?`, r.TargetID).Scan(&yamlContent) != nil || yamlContent == "" {
		insertScaleHistory(r, action, current, current, metric, "failed", "Compose 项目配置缺失")
		return
	}
	if _, _, err := client.ExecCommand("docker compose version >/dev/null 2>&1", 10*time.Second); err != nil {
		insertScaleHistory(r, action, current, current, metric, "failed", "Docker 环境未安装 compose 插件")
		return
	}
	cmd := fmt.Sprintf("docker compose -p %s -f - up -d --scale %s=%d", shellQuote(projName), shellQuote(svcName), desired)
	stdout, stderr, err := client.ExecCommandInput(cmd, yamlContent, 180*time.Second)
	if err != nil {
		insertScaleHistory(r, action, current, current, metric, "failed",
			fmt.Sprintf("伸缩执行失败: %v %s %s", err, strings.TrimSpace(stderr), strings.TrimSpace(stdout)))
		return
	}
	reason := fmt.Sprintf("%s 指标 %.1f%%（阈值 %.0f%% / 目标 %.0f%%），实例 %d → %d",
		r.MetricType, metric, r.Threshold, r.TargetValue, current, desired)
	insertScaleHistory(r, action, current, desired, metric, "success", reason)
	logger.Infof("Auto-scale %s: %s %d→%d (metric=%.1f%%)", r.Name, action, current, desired, metric)
}