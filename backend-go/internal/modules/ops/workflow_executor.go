// 工作流执行器 — DAG 顺序执行 provider / approval / agent 节点
// 节点结果与日志持久化到 tasks.node_results / tasks.logs，前端轮询可见
// SSH 能力通过 SSHExecHook 由 registry 注入（避免 ops → server/routes 循环依赖）
package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	"golang.org/x/crypto/ssh"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

// SSHExecHook 由 registry 注入 srvroutes.ExecOnServer（按已注册服务器凭证执行命令）
var SSHExecHook func(serverID, command string, timeout time.Duration) (string, string, error)

// AgentRunHook 由 registry 注入 copilot.ProcessMessage（copilot 依赖 network/services，不能直接 import）
var AgentRunHook func(ctx context.Context, prompt string) string

// OnTaskFinished 任务终态回调（自愈引擎用于推进执行记录），由引擎启动时注入
var OnTaskFinished func(taskID string)

const (
	wfNodeTimeout   = 120 * time.Second
	wfHTTPTimeout   = 30 * time.Second
	wfAgentTimeout  = 60 * time.Second
	wfMaxOutputSize = 64 * 1024
)

// shellQuoteParam / pythonQuoteParam 与脚本中心一致的转义语义
func shellQuoteParam(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func pythonQuoteParam(s string) string {
	return strconv.Quote(s)
}

var wfTemplate = regexp.MustCompile(`\{\{\s*(params|alert)\.([a-zA-Z0-9_]+)\s*\}\}`)

type wfNode struct {
	ID   string                 `json:"id"`
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type wfEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type wfNodeResult struct {
	Status     string `json:"status"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type wfLogEntry struct {
	Ts      string `json:"ts"`
	Level   string `json:"level"`
	NodeID  string `json:"node_id,omitempty"`
	Message string `json:"message"`
}

// StartTaskExecution 异步执行任务（新建或审批/暂停后恢复）
func StartTaskExecution(taskID string) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Errorf("workflow executor panic (task %s): %v", taskID, rec)
				failTask(taskID, fmt.Sprintf("执行器内部错误: %v", rec))
			}
		}()
		runTask(taskID)
	}()
}

func runTask(taskID string) {
	db := appdb.DB()
	var workflowID, status string
	var contextStr sql.NullString
	if err := db.QueryRow(`SELECT workflow_id, status, context FROM tasks WHERE id = ?`, taskID).
		Scan(&workflowID, &status, &contextStr); err != nil {
		return
	}
	if status != "running" && status != "pending" {
		return
	}

	var nodesJSON, edgesJSON sql.NullString
	if err := db.QueryRow(`SELECT nodes, edges FROM workflows WHERE id = ?`, workflowID).
		Scan(&nodesJSON, &edgesJSON); err != nil {
		failTask(taskID, "关联工作流不存在")
		return
	}
	var nodes []wfNode
	var edges []wfEdge
	_ = json.Unmarshal([]byte(nodesJSON.String), &nodes)
	_ = json.Unmarshal([]byte(edgesJSON.String), &edges)
	if len(nodes) == 0 {
		failTask(taskID, "工作流没有任何节点")
		return
	}

	order, err := wfTopoOrder(nodes, edges)
	if err != nil {
		failTask(taskID, err.Error())
		return
	}

	taskCtx := map[string]interface{}{}
	if contextStr.Valid && contextStr.String != "" {
		_ = json.Unmarshal([]byte(contextStr.String), &taskCtx)
	}

	results := loadNodeResults(taskID)
	logs := loadTaskLogs(taskID)
	appendLog := func(level, nodeID, msg string) {
		logs = append(logs, wfLogEntry{Ts: time.Now().Format("2006-01-02 15:04:05"), Level: level, NodeID: nodeID, Message: msg})
		persistTaskProgress(taskID, "", results, logs)
	}
	appendLog("info", "", "任务开始执行")

	byID := map[string]wfNode{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	for _, nodeID := range order {
		if r, ok := results[nodeID]; ok && (r.Status == "completed" || r.Status == "skipped") {
			continue
		}
		// 节点边界检查取消/暂停
		var curStatus string
		if db.QueryRow(`SELECT status FROM tasks WHERE id = ?`, taskID).Scan(&curStatus) != nil {
			return
		}
		if curStatus == "cancelled" || curStatus == "paused" || curStatus == "failed" {
			return
		}

		node := byID[nodeID]
		label := wfStr(node.Data["label"])
		if label == "" {
			label = nodeID
		}
		results[nodeID] = wfNodeResult{Status: "running", StartedAt: time.Now().Format("2006-01-02 15:04:05")}
		persistTaskProgress(taskID, nodeID, results, logs)
		appendLog("info", nodeID, fmt.Sprintf("开始节点 [%s] (%s)", label, node.Type))

		var output string
		var execErr error
		switch node.Type {
		case "provider":
			output, execErr = executeProviderNode(node, taskCtx, taskID)
		case "approval":
			handled, approved := handleApprovalNode(taskID, node, &logs)
			if handled {
				if !approved {
					results[nodeID] = wfNodeResult{Status: "failed", Error: "审批被拒绝", FinishedAt: time.Now().Format("2006-01-02 15:04:05")}
					persistTaskProgress(taskID, nodeID, results, logs)
					finishTask(taskID, "failed")
					return
				}
				results[nodeID] = wfNodeResult{Status: "completed", Output: "审批已通过", FinishedAt: time.Now().Format("2006-01-02 15:04:05")}
				persistTaskProgress(taskID, nodeID, results, logs)
				continue
			}
			// 已创建审批请求，任务挂起等待
			persistTaskProgress(taskID, nodeID, results, logs)
			return
		case "agent":
			output, execErr = executeAgentNode(node, taskCtx)
		default:
			execErr = fmt.Errorf("未知节点类型: %s", node.Type)
		}

		now := time.Now().Format("2006-01-02 15:04:05")
		if execErr != nil {
			results[nodeID] = wfNodeResult{Status: "failed", Error: execErr.Error(), Output: capOutput(output), StartedAt: results[nodeID].StartedAt, FinishedAt: now}
			appendLog("error", nodeID, fmt.Sprintf("节点 [%s] 失败: %v", label, execErr))
			persistTaskProgress(taskID, nodeID, results, logs)
			finishTask(taskID, "failed")
			return
		}
		results[nodeID] = wfNodeResult{Status: "completed", Output: capOutput(output), StartedAt: results[nodeID].StartedAt, FinishedAt: now}
		appendLog("info", nodeID, fmt.Sprintf("节点 [%s] 完成", label))
		persistTaskProgress(taskID, nodeID, results, logs)
	}

	appendLog("info", "", "任务执行完成")
	finishTask(taskID, "completed")
}

// handleApprovalNode 返回 (handled, approved)。handled=false 表示已挂起等待审批
func handleApprovalNode(taskID string, node wfNode, logs *[]wfLogEntry) (bool, bool) {
	db := appdb.DB()
	label := wfStr(node.Data["label"])
	if label == "" {
		label = "审批节点"
	}
	var existingID, existingStatus string
	err := db.QueryRow(`SELECT id, status FROM approval_requests WHERE task_id = ? AND node_id = ? ORDER BY created_at DESC LIMIT 1`, taskID, node.ID).
		Scan(&existingID, &existingStatus)
	if err == nil {
		switch existingStatus {
		case "approved":
			return true, true
		case "rejected":
			return true, false
		default:
			db.Exec(`UPDATE tasks SET status = 'waiting_approval', current_node_id = ? WHERE id = ?`, node.ID, taskID)
			return false, false
		}
	}

	cfg, _ := node.Data["approvalConfig"].(map[string]interface{})
	desc := wfStr(cfg["description"])
	timeoutSec := 3600
	if v, ok := cfg["timeout"].(float64); ok && v >= 0 {
		timeoutSec = int(v)
	}
	timeoutAction := wfStr(cfg["timeoutAction"])
	if timeoutAction == "" {
		timeoutAction = "reject"
	}
	now := time.Now()
	var timeoutAt interface{}
	if timeoutSec > 0 {
		timeoutAt = now.Add(time.Duration(timeoutSec) * time.Second).Format("2006-01-02 15:04:05")
	}
	_, err = db.Exec(`
		INSERT INTO approval_requests (id, task_id, node_id, node_label, description, status, requested_by, timeout_at, timeout_action, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'pending', 'workflow', ?, ?, ?, ?)`,
		uuid.NewString(), taskID, node.ID, label, desc, timeoutAt, timeoutAction,
		now.Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
	if err != nil {
		logger.Errorf("创建审批请求失败: %v", err)
	}
	*logs = append(*logs, wfLogEntry{Ts: now.Format("2006-01-02 15:04:05"), Level: "info", NodeID: node.ID, Message: "等待人工审批: " + label})
	db.Exec(`UPDATE tasks SET status = 'waiting_approval', current_node_id = ? WHERE id = ?`, node.ID, taskID)
	return false, false
}

func executeProviderNode(node wfNode, taskCtx map[string]interface{}, taskID string) (string, error) {
	providerID := wfStr(node.Data["providerId"])
	cfg, _ := node.Data["config"].(map[string]interface{})
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	// 命令类节点的模板替换做 shell 转义，防止告警内容注入平台主机
	shellCfg := substituteConfig(cfg, taskCtx, shellQuoteParam)
	rawCfg := substituteConfig(cfg, taskCtx, nil)

	switch providerID {
	case "bash":
		command := wfStr(shellCfg["command"])
		if command == "" {
			return "", fmt.Errorf("bash 节点缺少 command 配置")
		}
		ctx, cancel := context.WithTimeout(context.Background(), wfNodeTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, "bash", "-c", command).CombinedOutput()
		output := capOutput(string(out))
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("命令执行超时（%v）", wfNodeTimeout)
		}
		if err != nil {
			return output, fmt.Errorf("命令退出错误: %v", err)
		}
		return output, nil

	case "python":
		script := wfStr(substituteConfig(cfg, taskCtx, pythonQuoteParam)["script"])
		if script == "" {
			return "", fmt.Errorf("python 节点缺少 script 配置")
		}
		ctx, cancel := context.WithTimeout(context.Background(), wfNodeTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, "python3", "-c", script).CombinedOutput()
		output := capOutput(string(out))
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("脚本执行超时（%v）", wfNodeTimeout)
		}
		if err != nil {
			return output, fmt.Errorf("脚本退出错误: %v", err)
		}
		return output, nil

	case "ssh":
		command := wfStr(shellCfg["command"])
		if command == "" {
			return "", fmt.Errorf("ssh 节点缺少 command 配置")
		}
		serverID := wfStr(rawCfg["server_id"])
		if serverID == "" {
			if params, ok := taskCtx["params"].(map[string]interface{}); ok {
				serverID = wfStr(params["server_id"])
			}
		}
		if serverID != "" {
			if SSHExecHook == nil {
				return "", fmt.Errorf("SSH 执行能力未初始化")
			}
			stdout, stderr, err := SSHExecHook(serverID, command, wfNodeTimeout)
			output := capOutput(stdout + stderr)
			if err != nil {
				return output, err
			}
			return output, nil
		}
		host := wfStr(rawCfg["host"])
		if host == "" {
			return "", fmt.Errorf("ssh 节点需要 server_id（已注册服务器）或 host 配置")
		}
		return execSSHDirect(rawCfg, command)

	case "http":
		return executeHTTPNode(rawCfg)

	case "snmp":
		return executeSNMPNode(rawCfg)

	case "notify":
		return executeNotifyNode(rawCfg, taskID)

	default:
		return "", fmt.Errorf("不支持的 provider: %s", providerID)
	}
}

func execSSHDirect(cfg map[string]interface{}, command string) (string, error) {
	host := wfStr(cfg["host"])
	port := wfStr(cfg["port"])
	if port == "" {
		port = "22"
	}
	username := wfStr(cfg["username"])
	password := wfStr(cfg["password"])
	privateKey := wfStr(cfg["private_key"])
	if username == "" {
		return "", fmt.Errorf("ssh 直连模式缺少 username")
	}
	var auths []ssh.AuthMethod
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return "", fmt.Errorf("私钥解析失败: %v", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if password != "" {
		auths = append(auths, ssh.Password(password))
	}
	if len(auths) == 0 {
		return "", fmt.Errorf("ssh 直连模式需要 password 或 private_key")
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(host, port), &ssh.ClientConfig{
		User:            username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 运维内网场景与服务器命令模块一致
		Timeout:         15 * time.Second,
	})
	if err != nil {
		return "", fmt.Errorf("SSH 连接失败: %v", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH 会话创建失败: %v", err)
	}
	defer session.Close()

	done := make(chan struct{})
	var out []byte
	var execErr error
	go func() {
		out, execErr = session.CombinedOutput(command)
		close(done)
	}()
	select {
	case <-done:
		return capOutput(string(out)), execErr
	case <-time.After(wfNodeTimeout):
		client.Close()
		return capOutput(string(out)), fmt.Errorf("SSH 命令执行超时（%v）", wfNodeTimeout)
	}
}

func executeHTTPNode(cfg map[string]interface{}) (string, error) {
	url := wfStr(cfg["url"])
	if url == "" {
		return "", fmt.Errorf("http 节点缺少 url 配置")
	}
	method := strings.ToUpper(wfStr(cfg["method"]))
	if method == "" {
		method = "POST"
	}
	body := wfStr(cfg["body"])
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("请求构造失败: %v", err)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if headers, ok := cfg["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, wfStr(v))
		}
	}
	client := &http.Client{Timeout: wfHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	output := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(respBody))
	if resp.StatusCode >= 400 {
		return output, fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}
	return output, nil
}

func executeSNMPNode(cfg map[string]interface{}) (string, error) {
	host := wfStr(cfg["host"])
	oid := wfStr(cfg["oid"])
	if host == "" || oid == "" {
		return "", fmt.Errorf("snmp 节点需要 host 与 oid 配置")
	}
	community := wfStr(cfg["community"])
	if community == "" {
		community = "public"
	}
	port := 161
	if p, err := strconv.Atoi(wfStr(cfg["port"])); err == nil && p > 0 {
		port = p
	}
	params := &gosnmp.GoSNMP{
		Target:    host,
		Port:      uint16(port),
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   10 * time.Second,
		Retries:   1,
	}
	if err := params.Connect(); err != nil {
		return "", fmt.Errorf("SNMP 连接失败: %v", err)
	}
	defer params.Conn.Close()
	result, err := params.Get([]string{oid})
	if err != nil {
		return "", fmt.Errorf("SNMP GET 失败: %v", err)
	}
	if len(result.Variables) == 0 {
		return "", fmt.Errorf("SNMP 无返回变量")
	}
	var sb strings.Builder
	for _, pdu := range result.Variables {
		val := pdu.Value
		if b, ok := val.([]byte); ok {
			val = string(b)
		}
		sb.WriteString(fmt.Sprintf("%s = %v\n", pdu.Name, val))
	}
	return sb.String(), nil
}

func executeNotifyNode(cfg map[string]interface{}, taskID string) (string, error) {
	channel := wfStr(cfg["channel"])
	if channel == "" {
		channel = "webhook"
	}
	message := wfStr(cfg["message"])
	if message == "" {
		return "", fmt.Errorf("notify 节点缺少 message 配置")
	}
	title := wfStr(cfg["title"])
	if title == "" {
		title = "工作流通知"
	}
	webhookURL := wfStr(cfg["webhook_url"])
	if webhookURL == "" {
		if ncfg, err := GetNotificationConfig(); err == nil && ncfg != nil {
			switch channel {
			case "wecom":
				webhookURL = ncfg.WechatConfig.WebhookURL
			case "dingtalk":
				webhookURL = ncfg.DingtalkConfig.WebhookURL
			default:
				webhookURL = ncfg.WebhookURL
			}
		}
	}
	if webhookURL == "" {
		return "", fmt.Errorf("未配置 %s 的 webhook 地址（节点配置或系统通知配置）", channel)
	}
	var err error
	switch channel {
	case "wecom":
		err = SendWechatRobot(webhookURL, title, message)
	case "dingtalk":
		err = SendDingtalkRobot(webhookURL, title, message)
	default:
		err = SendWebhookPost(webhookURL, map[string]interface{}{"title": title, "message": message, "task_id": taskID})
	}
	status := "sent"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	LogNotificationRecord(channel, title, message, webhookURL, status, "", taskID, errMsg)
	if err != nil {
		return "", fmt.Errorf("通知发送失败: %v", err)
	}
	return "通知已发送 (" + channel + ")", nil
}

func executeAgentNode(node wfNode, taskCtx map[string]interface{}) (string, error) {
	prompt := wfStr(node.Data["prompt"])
	if prompt == "" {
		prompt = wfStr(node.Data["label"])
	}
	if prompt == "" {
		return "", fmt.Errorf("agent 节点缺少 prompt")
	}
	prompt = substituteTemplates(prompt, taskCtx, nil)
	ctx, cancel := context.WithTimeout(context.Background(), wfAgentTimeout)
	defer cancel()
	if AgentRunHook == nil {
		return "", fmt.Errorf("agent 执行器未初始化")
	}
	reply := AgentRunHook(ctx, prompt)
	return capOutput(reply), nil
}

// ── 模板与配置工具 ──

func substituteConfig(cfg map[string]interface{}, taskCtx map[string]interface{}, quote func(string) string) map[string]interface{} {
	out := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		if s, ok := v.(string); ok {
			out[k] = substituteTemplates(s, taskCtx, quote)
		} else {
			out[k] = v
		}
	}
	return out
}

func substituteTemplates(s string, taskCtx map[string]interface{}, quote func(string) string) string {
	return wfTemplate.ReplaceAllStringFunc(s, func(m string) string {
		parts := wfTemplate.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		scope, ok := taskCtx[parts[1]].(map[string]interface{})
		if !ok {
			return m
		}
		v, ok := scope[parts[2]]
		if !ok {
			return m
		}
		val := wfStr(v)
		if quote != nil {
			return quote(val)
		}
		return val
	})
}

func wfStr(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func capOutput(s string) string {
	if len(s) > wfMaxOutputSize {
		return s[:wfMaxOutputSize] + "\n...[输出截断]"
	}
	return s
}

// ── DAG 与持久化 ──

func wfTopoOrder(nodes []wfNode, edges []wfEdge) ([]string, error) {
	indegree := map[string]int{}
	adj := map[string][]string{}
	valid := map[string]bool{}
	for _, n := range nodes {
		indegree[n.ID] = 0
		valid[n.ID] = true
	}
	for _, e := range edges {
		if !valid[e.Source] || !valid[e.Target] {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
		indegree[e.Target]++
	}
	var queue []string
	for _, n := range nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	var order []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, next := range adj[cur] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, fmt.Errorf("工作流存在环形依赖，无法执行")
	}
	return order, nil
}

func loadNodeResults(taskID string) map[string]wfNodeResult {
	results := map[string]wfNodeResult{}
	var raw sql.NullString
	if appdb.DB().QueryRow(`SELECT node_results FROM tasks WHERE id = ?`, taskID).Scan(&raw) == nil && raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &results)
	}
	return results
}

func loadTaskLogs(taskID string) []wfLogEntry {
	var logs []wfLogEntry
	var raw sql.NullString
	if appdb.DB().QueryRow(`SELECT logs FROM tasks WHERE id = ?`, taskID).Scan(&raw) == nil && raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &logs)
	}
	return logs
}

func persistTaskProgress(taskID, currentNodeID string, results map[string]wfNodeResult, logs []wfLogEntry) {
	resultsJSON, _ := json.Marshal(results)
	logsJSON, _ := json.Marshal(logs)
	if currentNodeID != "" {
		appdb.DB().Exec(`UPDATE tasks SET current_node_id = ?, node_results = ?, logs = ?, updated_at = ? WHERE id = ?`,
			currentNodeID, string(resultsJSON), string(logsJSON), time.Now().Format("2006-01-02 15:04:05"), taskID)
	} else {
		appdb.DB().Exec(`UPDATE tasks SET node_results = ?, logs = ?, updated_at = ? WHERE id = ?`,
			string(resultsJSON), string(logsJSON), time.Now().Format("2006-01-02 15:04:05"), taskID)
	}
}

func finishTask(taskID, status string) {
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE tasks SET status = ?, end_time = ?, current_node_id = '', updated_at = ? WHERE id = ?`, status, now, now, taskID)
	if OnTaskFinished != nil {
		OnTaskFinished(taskID)
	}
}

func failTask(taskID, reason string) {
	logs := loadTaskLogs(taskID)
	logs = append(logs, wfLogEntry{Ts: time.Now().Format("2006-01-02 15:04:05"), Level: "error", Message: reason})
	logsJSON, _ := json.Marshal(logs)
	now := time.Now().Format("2006-01-02 15:04:05")
	appdb.DB().Exec(`UPDATE tasks SET status = 'failed', end_time = ?, logs = ?, updated_at = ? WHERE id = ?`, now, string(logsJSON), now, taskID)
	if OnTaskFinished != nil {
		OnTaskFinished(taskID)
	}
}