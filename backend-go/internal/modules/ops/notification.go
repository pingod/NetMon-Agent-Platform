package ops

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
)

// ============================================================
// 通知配置与记录模型
// ============================================================

type EmailConfig struct {
	SmtpHost    string `json:"smtp_host"`
	SmtpPort    int    `json:"smtp_port"`
	User        string `json:"user"`
	Password    string `json:"password,omitempty"`
	HasPassword bool   `json:"has_password"`
}

type WechatConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type DingtalkConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type AlertNotificationFilter struct {
	Critical bool `json:"critical"`
	Warning  bool `json:"warning"`
	Info     bool `json:"info"`
}

type TaskNotificationFilter struct {
	Success bool `json:"success"`
	Failed  bool `json:"failed"`
	Running bool `json:"running"`
}

type NotificationConfig struct {
	WebhookEnabled    bool                    `json:"webhook_enabled"`
	WebhookURL        string                  `json:"webhook_url"`
	EmailEnabled      bool                    `json:"email_enabled"`
	EmailConfig       EmailConfig             `json:"email_config"`
	WechatEnabled     bool                    `json:"wechat_enabled"`
	WechatConfig      WechatConfig            `json:"wechat_config"`
	DingtalkEnabled   bool                    `json:"dingtalk_enabled"`
	DingtalkConfig    DingtalkConfig          `json:"dingtalk_config"`
	AlertNotification AlertNotificationFilter `json:"alert_notification"`
	TaskNotification  TaskNotificationFilter  `json:"task_notification"`
}

type NotificationRecord struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Content        *string `json:"content"`
	Recipient      *string `json:"recipient"`
	Status         string  `json:"status"`
	RelatedAlertID *string `json:"related_alert_id"`
	RelatedTaskID  *string `json:"related_task_id"`
	SentAt         *string `json:"sent_at"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

// EnsureNotificationTables 初始化通知相关表结构
func EnsureNotificationTables() {
	db := appdb.DB()
	if db == nil {
		return
	}

	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			webhook_enabled INTEGER DEFAULT 1,
			webhook_url TEXT,
			email_enabled INTEGER DEFAULT 0,
			email_config TEXT,
			wechat_enabled INTEGER DEFAULT 0,
			wechat_config TEXT,
			dingtalk_enabled INTEGER DEFAULT 0,
			dingtalk_config TEXT,
			alert_notification TEXT,
			task_notification TEXT,
			updated_at TEXT
		)
	`)

	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT,
			status TEXT DEFAULT 'unread',
			recipient TEXT,
			metadata TEXT,
			related_alert_id TEXT,
			related_task_id TEXT,
			sent_at TEXT,
			error_message TEXT,
			created_at TEXT
		)
	`)

	// 检查是否有默认配置
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM notification_configs").Scan(&count)
	if err == nil && count == 0 {
		defaultEmail, _ := json.Marshal(EmailConfig{SmtpPort: 465})
		defaultWechat, _ := json.Marshal(WechatConfig{})
		defaultDingtalk, _ := json.Marshal(DingtalkConfig{})
		defaultAlert, _ := json.Marshal(AlertNotificationFilter{Critical: true, Warning: true, Info: false})
		defaultTask, _ := json.Marshal(TaskNotificationFilter{Success: true, Failed: true, Running: false})
		now := time.Now().Format("2006-01-02 15:04:05")

		_, _ = db.Exec(`
			INSERT INTO notification_configs (
				webhook_enabled, webhook_url, email_enabled, email_config,
				wechat_enabled, wechat_config, dingtalk_enabled, dingtalk_config,
				alert_notification, task_notification, updated_at
			) VALUES (0, '', 0, ?, 0, ?, 0, ?, ?, ?, ?)`,
			string(defaultEmail), string(defaultWechat), string(defaultDingtalk),
			string(defaultAlert), string(defaultTask), now,
		)
	}
}

// GetNotificationConfig 获取系统通知配置
func GetNotificationConfig() (*NotificationConfig, error) {
	EnsureNotificationTables()
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	var cfg NotificationConfig
	var (
		whEnabled, emEnabled, wxEnabled, dtEnabled int
		whUrl, emCfg, wxCfg, dtCfg, altCfg, tskCfg sql.NullString
	)

	err := db.QueryRow(`
		SELECT webhook_enabled, webhook_url, email_enabled, email_config,
		       wechat_enabled, wechat_config, dingtalk_enabled, dingtalk_config,
		       alert_notification, task_notification
		FROM notification_configs ORDER BY id ASC LIMIT 1
	`).Scan(&whEnabled, &whUrl, &emEnabled, &emCfg, &wxEnabled, &wxCfg, &dtEnabled, &dtCfg, &altCfg, &tskCfg)

	if err != nil {
		if err == sql.ErrNoRows {
			// 返回默认空配置
			return &NotificationConfig{
				AlertNotification: AlertNotificationFilter{Critical: true, Warning: true, Info: false},
				TaskNotification:  TaskNotificationFilter{Success: true, Failed: true, Running: false},
			}, nil
		}
		return nil, err
	}

	cfg.WebhookEnabled = whEnabled == 1
	cfg.WebhookURL = whUrl.String
	cfg.EmailEnabled = emEnabled == 1
	cfg.WechatEnabled = wxEnabled == 1
	cfg.DingtalkEnabled = dtEnabled == 1

	if emCfg.Valid && emCfg.String != "" {
		_ = json.Unmarshal([]byte(emCfg.String), &cfg.EmailConfig)
	}
	cfg.EmailConfig.HasPassword = cfg.EmailConfig.Password != ""
	if wxCfg.Valid && wxCfg.String != "" {
		_ = json.Unmarshal([]byte(wxCfg.String), &cfg.WechatConfig)
	}
	if dtCfg.Valid && dtCfg.String != "" {
		_ = json.Unmarshal([]byte(dtCfg.String), &cfg.DingtalkConfig)
	}
	if altCfg.Valid && altCfg.String != "" {
		_ = json.Unmarshal([]byte(altCfg.String), &cfg.AlertNotification)
	} else {
		cfg.AlertNotification = AlertNotificationFilter{Critical: true, Warning: true, Info: false}
	}
	if tskCfg.Valid && tskCfg.String != "" {
		_ = json.Unmarshal([]byte(tskCfg.String), &cfg.TaskNotification)
	} else {
		cfg.TaskNotification = TaskNotificationFilter{Success: true, Failed: true, Running: false}
	}

	return &cfg, nil
}

// SaveNotificationConfig 更新通知配置
func SaveNotificationConfig(cfg *NotificationConfig) error {
	EnsureNotificationTables()
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("db not initialized")
	}

	whEnabled := 0
	if cfg.WebhookEnabled {
		whEnabled = 1
	}
	emEnabled := 0
	if cfg.EmailEnabled {
		emEnabled = 1
	}
	wxEnabled := 0
	if cfg.WechatEnabled {
		wxEnabled = 1
	}
	dtEnabled := 0
	if cfg.DingtalkEnabled {
		dtEnabled = 1
	}

	emBytes, _ := json.Marshal(cfg.EmailConfig)
	wxBytes, _ := json.Marshal(cfg.WechatConfig)
	dtBytes, _ := json.Marshal(cfg.DingtalkConfig)
	altBytes, _ := json.Marshal(cfg.AlertNotification)
	tskBytes, _ := json.Marshal(cfg.TaskNotification)
	// 未提交新密码时保留已保存的密码，避免前端脱敏回显把密码清空
	if cfg.EmailConfig.Password == "" {
		if current, err := GetNotificationConfig(); err == nil && current != nil && current.EmailConfig.Password != "" {
			cfg.EmailConfig.Password = current.EmailConfig.Password
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")

	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM notification_configs").Scan(&count)
	if count == 0 {
		_, err := db.Exec(`
			INSERT INTO notification_configs (
				webhook_enabled, webhook_url, email_enabled, email_config,
				wechat_enabled, wechat_config, dingtalk_enabled, dingtalk_config,
				alert_notification, task_notification, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			whEnabled, cfg.WebhookURL, emEnabled, string(emBytes),
			wxEnabled, string(wxBytes), dtEnabled, string(dtBytes),
			string(altBytes), string(tskBytes), now,
		)
		return err
	}

	_, err := db.Exec(`
		UPDATE notification_configs SET
			webhook_enabled = ?, webhook_url = ?,
			email_enabled = ?, email_config = ?,
			wechat_enabled = ?, wechat_config = ?,
			dingtalk_enabled = ?, dingtalk_config = ?,
			alert_notification = ?, task_notification = ?,
			updated_at = ?
		WHERE id = (SELECT id FROM (SELECT id FROM notification_configs ORDER BY id ASC LIMIT 1) as t)`,
		whEnabled, cfg.WebhookURL, emEnabled, string(emBytes),
		wxEnabled, string(wxBytes), dtEnabled, string(dtBytes),
		string(altBytes), string(tskBytes), now,
	)
	return err
}

// ============================================================
// 发送与测试通知底层通信
// ============================================================

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// SendWebhookPost 发送通用 Webhook HTTP POST 请求
func SendWebhookPost(url string, payload interface{}) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("Webhook URL 不能为空")
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "NetMon-Alert-Webhook/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送 Webhook 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respSnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Webhook 目标返回错误状态码 %d: %s", resp.StatusCode, string(respSnippet))
	}
	return nil
}

// SendWechatRobot 发送企业微信机器人 Markdown 消息
func SendWechatRobot(webhookURL, title, markdownContent string) error {
	if strings.TrimSpace(webhookURL) == "" {
		return fmt.Errorf("企业微信 Webhook URL 不能为空")
	}

	msg := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": markdownContent,
		},
	}

	bodyBytes, _ := json.Marshal(msg)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送企业微信消息失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if result.ErrCode != 0 {
			return fmt.Errorf("企业微信 API 错误 (%d): %s", result.ErrCode, result.ErrMsg)
		}
	}
	return nil
}

// SendDingtalkRobot 发送钉钉机器人 Markdown 消息
func SendDingtalkRobot(webhookURL, title, markdownContent string) error {
	if strings.TrimSpace(webhookURL) == "" {
		return fmt.Errorf("钉钉 Webhook URL 不能为空")
	}

	msg := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  markdownContent,
		},
	}

	bodyBytes, _ := json.Marshal(msg)
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送钉钉消息失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if result.ErrCode != 0 {
			return fmt.Errorf("钉钉 API 错误 (%d): %s", result.ErrCode, result.ErrMsg)
		}
	}
	return nil
}

// sanitizeMailHeader 过滤 CR/LF，防止邮件头注入
func sanitizeMailHeader(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	return strings.ReplaceAll(v, "\n", "")
}

// validateMailAddress 校验收件人地址，禁止换行与逗号分隔的多地址注入
func validateMailAddress(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("收件人为空")
	}
	if strings.ContainsAny(addr, "\r\n,;") {
		return fmt.Errorf("收件人地址包含非法字符")
	}
	if _, err := mail.ParseAddress(addr); err != nil {
		return fmt.Errorf("收件人地址不合法: %w", err)
	}
	return nil
}

// SendSmtpEmail 发送 SMTP 邮件
func SendSmtpEmail(cfg EmailConfig, to, subject, body string) error {
	if cfg.SmtpHost == "" || cfg.User == "" {
		return fmt.Errorf("SMTP 配置不完整")
	}
	if to == "" {
		to = cfg.User
	}
	if err := validateMailAddress(to); err != nil {
		return err
	}
	port := cfg.SmtpPort
	if port <= 0 {
		port = 465
	}

	addr := net.JoinHostPort(cfg.SmtpHost, strconv.Itoa(port))
	auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.SmtpHost)

	// 主题与地址均过滤 CR/LF，主题按 RFC 2047 编码中文
	cleanSubject := mime.QEncoding.Encode("UTF-8", sanitizeMailHeader(subject))
	cleanTo := sanitizeMailHeader(to)
	cleanFrom := sanitizeMailHeader(cfg.User)
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		cleanFrom, cleanTo, cleanSubject, body)

	if port == 465 {
		// SSL 直连；证书校验默认开启，仅当显式设置 SMTP_INSECURE_SKIP_VERIFY=true 时跳过
		tlsconfig := &tls.Config{
			ServerName:         cfg.SmtpHost,
			InsecureSkipVerify: os.Getenv("SMTP_INSECURE_SKIP_VERIFY") == "true",
		}
		conn, err := tls.Dial("tcp", addr, tlsconfig)
		if err != nil {
			return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
		}
		c, err := smtp.NewClient(conn, cfg.SmtpHost)
		if err != nil {
			return fmt.Errorf("初始化 SMTP 客户端失败: %w", err)
		}
		defer c.Close()

		if err = c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
		if err = c.Mail(cleanFrom); err != nil {
			return err
		}
		if err = c.Rcpt(cleanTo); err != nil {
			return err
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		if _, err = w.Write([]byte(message)); err != nil {
			return err
		}
		if err = w.Close(); err != nil {
			return err
		}
		return c.Quit()
	}

	// 普通端口/STARTTLS
	return smtp.SendMail(addr, auth, cleanFrom, []string{cleanTo}, []byte(message))
}

// validateOutboundURL 校验外部通知地址：仅允许 http/https，禁止回环与私网地址（SSRF 防护）
func validateOutboundURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https 协议的地址")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("无法解析目标主机: %s", host)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("禁止访问回环/私网地址: %s", ip.String())
		}
	}
	return nil
}

// LogNotificationRecord 记录通知发送审计日志
func LogNotificationRecord(ntfType, title, content, recipient, status, alertID, taskID, errMsg string) {
	db := appdb.DB()
	if db == nil {
		return
	}
	id := "ntf-" + uuid.NewString()
	now := time.Now().Format("2006-01-02 15:04:05")

	_, _ = db.Exec(`
		INSERT INTO notifications (
			id, type, title, content, status, recipient,
			related_alert_id, related_task_id, sent_at, error_message, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, ntfType, title, content, status, recipient,
		alertID, taskID, now, errMsg, now,
	)
}

// ============================================================
// 告警通知主分发引擎
// ============================================================

type AlertDispatchInfo struct {
	AlertID    string
	Title      string
	Message    string
	Severity   string
	MetricName string
	Value      float64
	DeviceID   string
	DeviceName string
	DeviceIP   string
	Tags       map[string]string
	Status     string
}

// DispatchAlertNotification 根据系统配置自动分发告警至所有已启用的远程通道
func DispatchAlertNotification(info AlertDispatchInfo) {
	cfg, err := GetNotificationConfig()
	if err != nil || cfg == nil {
		logger.Warnf("GetNotificationConfig failed during alert dispatch: %v", err)
		return
	}

	// 1. 级别过滤判断
	sev := strings.ToLower(strings.TrimSpace(info.Severity))
	switch sev {
	case "critical", "p1":
		if !cfg.AlertNotification.Critical {
			logger.Infof("Alert %s severity [critical] notification is disabled in settings, skipping.", info.AlertID)
			return
		}
	case "warning", "p2":
		if !cfg.AlertNotification.Warning {
			logger.Infof("Alert %s severity [warning] notification is disabled in settings, skipping.", info.AlertID)
			return
		}
	case "info", "p3":
		if !cfg.AlertNotification.Info {
			logger.Infof("Alert %s severity [info] notification is disabled in settings, skipping.", info.AlertID)
			return
		}
	}

	timeStr := time.Now().Format("2006-01-02 15:04:05")
	sevLabel := "警告 (Warning)"
	sevColor := "#FFA500"
	if sev == "critical" || sev == "p1" {
		sevLabel = "严重 (Critical)"
		sevColor = "#FF0000"
	} else if sev == "info" || sev == "p3" {
		sevLabel = "提醒 (Info)"
		sevColor = "#008000"
	}

	devDisplay := info.DeviceName
	if devDisplay == "" {
		devDisplay = info.DeviceID
	}
	if info.DeviceIP != "" {
		devDisplay = fmt.Sprintf("%s (%s)", devDisplay, info.DeviceIP)
	}

	// 2. 分发至 Webhook
	if cfg.WebhookEnabled && strings.TrimSpace(cfg.WebhookURL) != "" {
		go func() {
			payload := map[string]interface{}{
				"event":       "alert",
				"alert_id":    info.AlertID,
				"title":       info.Title,
				"message":     info.Message,
				"severity":    info.Severity,
				"metric_name": info.MetricName,
				"value":       info.Value,
				"device_id":   info.DeviceID,
				"device_name": info.DeviceName,
				"device_ip":   info.DeviceIP,
				"tags":        info.Tags,
				"timestamp":   time.Now().Format(time.RFC3339),
				"status":      "firing",
			}
			err := SendWebhookPost(cfg.WebhookURL, payload)
			if err != nil {
				logger.Warnf("Failed to send alert webhook: %v", err)
				LogNotificationRecord("webhook", info.Title, info.Message, cfg.WebhookURL, "failed", info.AlertID, "", err.Error())
			} else {
				logger.Infof("Successfully sent alert webhook to %s", cfg.WebhookURL)
				LogNotificationRecord("webhook", info.Title, info.Message, cfg.WebhookURL, "sent", info.AlertID, "", "")
			}
		}()
	}

	// 3. 分发至企业微信机器人
	if cfg.WechatEnabled && strings.TrimSpace(cfg.WechatConfig.WebhookURL) != "" {
		go func() {
			wxContent := fmt.Sprintf("### 🚨 监控告警通知: %s\n> **告警级别**: <font color=\"%s\">%s</font>\n> **监控设备**: %s\n> **监控指标**: %s (当前值: `%.2f`)\n> **告警详情**: %s\n> **触发时间**: %s",
				info.Title, sevColor, sevLabel, devDisplay, info.MetricName, info.Value, info.Message, timeStr)
			err := SendWechatRobot(cfg.WechatConfig.WebhookURL, info.Title, wxContent)
			if err != nil {
				logger.Warnf("Failed to send WeChat robot notification: %v", err)
				LogNotificationRecord("wechat", info.Title, info.Message, cfg.WechatConfig.WebhookURL, "failed", info.AlertID, "", err.Error())
			} else {
				logger.Infof("Successfully sent WeChat robot alert notification")
				LogNotificationRecord("wechat", info.Title, info.Message, cfg.WechatConfig.WebhookURL, "sent", info.AlertID, "", "")
			}
		}()
	}

	// 4. 分发至钉钉机器人
	if cfg.DingtalkEnabled && strings.TrimSpace(cfg.DingtalkConfig.WebhookURL) != "" {
		go func() {
			dtContent := fmt.Sprintf("### 🚨 监控告警通知: %s\n\n- **告警级别**: %s\n- **监控设备**: %s\n- **监控指标**: %s (当前值: `%.2f`)\n- **告警详情**: %s\n- **触发时间**: %s",
				info.Title, sevLabel, devDisplay, info.MetricName, info.Value, info.Message, timeStr)
			err := SendDingtalkRobot(cfg.DingtalkConfig.WebhookURL, info.Title, dtContent)
			if err != nil {
				logger.Warnf("Failed to send DingTalk robot notification: %v", err)
				LogNotificationRecord("dingtalk", info.Title, info.Message, cfg.DingtalkConfig.WebhookURL, "failed", info.AlertID, "", err.Error())
			} else {
				logger.Infof("Successfully sent DingTalk robot alert notification")
				LogNotificationRecord("dingtalk", info.Title, info.Message, cfg.DingtalkConfig.WebhookURL, "sent", info.AlertID, "", "")
			}
		}()
	}

	// 5. 分发至邮件 (SMTP)
	if cfg.EmailEnabled && cfg.EmailConfig.SmtpHost != "" && cfg.EmailConfig.User != "" {
		go func() {
			subject := fmt.Sprintf("[NetMon 告警] %s - %s", sevLabel, info.Title)
			body := fmt.Sprintf(`
				<div style="font-family: Arial, sans-serif; max-width: 600px; padding: 20px; border: 1px solid #e0e0e0; border-radius: 8px;">
					<h2 style="color: %s; margin-top: 0;">🚨 监控告警通知</h2>
					<table style="width: 100%%; border-collapse: collapse; font-size: 14px;">
						<tr><td style="padding: 8px 0; color: #666; width: 90px;"><strong>告警名称:</strong></td><td>%s</td></tr>
						<tr><td style="padding: 8px 0; color: #666;"><strong>告警级别:</strong></td><td style="color: %s; font-weight: bold;">%s</td></tr>
						<tr><td style="padding: 8px 0; color: #666;"><strong>目标设备:</strong></td><td>%s</td></tr>
						<tr><td style="padding: 8px 0; color: #666;"><strong>指标名称:</strong></td><td>%s (当前值: %.2f)</td></tr>
						<tr><td style="padding: 8px 0; color: #666;"><strong>详细说明:</strong></td><td>%s</td></tr>
						<tr><td style="padding: 8px 0; color: #666;"><strong>触发时间:</strong></td><td>%s</td></tr>
					</table>
				</div>
			`, sevColor, info.Title, sevColor, sevLabel, devDisplay, info.MetricName, info.Value, info.Message, timeStr)

			to := cfg.EmailConfig.User
			err := SendSmtpEmail(cfg.EmailConfig, to, subject, body)
			if err != nil {
				logger.Warnf("Failed to send SMTP email notification: %v", err)
				LogNotificationRecord("email", info.Title, info.Message, to, "failed", info.AlertID, "", err.Error())
			} else {
				logger.Infof("Successfully sent SMTP email alert notification to %s", to)
				LogNotificationRecord("email", info.Title, info.Message, to, "sent", info.AlertID, "", "")
			}
		}()
	}
}

// ============================================================
// HTTP 路由注册
// ============================================================

// NotificationConfigRoutes 注册 /api/v1/notification-config 路由
func NotificationConfigRoutes() chi.Router {
	r := chi.NewRouter()

	// GET /api/v1/notification-config
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := GetNotificationConfig()
		if err != nil {
			httpx.RespondError(w, "获取通知配置失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// 永不回传 SMTP 明文口令，仅告知是否已配置
		cfg.EmailConfig.HasPassword = cfg.EmailConfig.Password != ""
		cfg.EmailConfig.Password = ""
		httpx.Respond(w, cfg)
	})

	// PUT /api/v1/notification-config
	r.With(middleware.RequireRole("admin")).Put("/", func(w http.ResponseWriter, r *http.Request) {
		var cfg NotificationConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			httpx.RespondError(w, "无效的配置参数: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := SaveNotificationConfig(&cfg); err != nil {
			httpx.RespondError(w, "保存通知配置失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "通知配置已更新"})
	})

	// POST /api/v1/notification-config/test/{channel}
	r.With(middleware.RequireRole("admin")).Post("/test/{channel}", handleTestNotificationChannel)
	// 兼容可能传入的前缀
	r.With(middleware.RequireRole("admin")).Post("/api/notification-config/test/{channel}", handleTestNotificationChannel)

	return r
}

func handleTestNotificationChannel(w http.ResponseWriter, r *http.Request) {
	channel := chi.URLParam(r, "channel")
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	switch strings.ToLower(channel) {
	case "webhook":
		targetUrl := ""
		if body != nil {
			if u, ok := body["webhook_url"].(string); ok {
				targetUrl = u
			}
		}
		if targetUrl == "" {
			cfg, _ := GetNotificationConfig()
			if cfg != nil {
				targetUrl = cfg.WebhookURL
			}
		}
		if targetUrl == "" {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": "请先填写 Webhook URL"})
			return
		}
		if err := validateOutboundURL(targetUrl); err != nil {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "目标地址不合法: "+err.Error())
			return
		}

		testPayload := map[string]interface{}{
			"event":     "test",
			"title":     "NetMon 监控 Webhook 测试",
			"message":   "这是一条来自 NetMon 自动化监控系统的测试 Webhook 消息，网络通信正常！",
			"timestamp": nowStr,
		}
		if err := SendWebhookPost(targetUrl, testPayload); err != nil {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "Webhook 测试请求发送成功"})

	case "wechat":
		targetUrl := ""
		if body != nil {
			if u, ok := body["webhook_url"].(string); ok {
				targetUrl = u
			}
		}
		if targetUrl == "" {
			cfg, _ := GetNotificationConfig()
			if cfg != nil {
				targetUrl = cfg.WechatConfig.WebhookURL
			}
		}
		if targetUrl == "" {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": "请先填写企业微信机器人 Webhook URL"})
			return
		}
		if err := validateOutboundURL(targetUrl); err != nil {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "目标地址不合法: "+err.Error())
			return
		}

		content := fmt.Sprintf("### 🔔 NetMon 监控系统测试消息\n> **通知类型**: 企业微信机器人连通性测试\n> **测试状态**: <font color=\"info\">通信正常</font>\n> **发送时间**: %s", nowStr)
		if err := SendWechatRobot(targetUrl, "NetMon 测试消息", content); err != nil {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "企业微信测试消息发送成功"})

	case "dingtalk":
		targetUrl := ""
		if body != nil {
			if u, ok := body["webhook_url"].(string); ok {
				targetUrl = u
			}
		}
		if targetUrl == "" {
			cfg, _ := GetNotificationConfig()
			if cfg != nil {
				targetUrl = cfg.DingtalkConfig.WebhookURL
			}
		}
		if targetUrl == "" {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": "请先填写钉钉机器人 Webhook URL"})
			return
		}
		if err := validateOutboundURL(targetUrl); err != nil {
			httpx.RespondFailStatus(w, http.StatusBadRequest, "目标地址不合法: "+err.Error())
			return
		}

		content := fmt.Sprintf("### 🔔 NetMon 监控系统测试消息\n\n- **通知类型**: 钉钉机器人连通性测试\n- **测试状态**: 通信正常\n- **发送时间**: %s", nowStr)
		if err := SendDingtalkRobot(targetUrl, "NetMon 测试消息", content); err != nil {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "钉钉测试消息发送成功"})

	case "email":
		var emailCfg EmailConfig
		to := ""
		if body != nil {
			if h, ok := body["smtp_host"].(string); ok {
				emailCfg.SmtpHost = h
			}
			if p, ok := body["smtp_port"].(float64); ok {
				emailCfg.SmtpPort = int(p)
			}
			if u, ok := body["user"].(string); ok {
				emailCfg.User = u
			}
			if pw, ok := body["password"].(string); ok {
				emailCfg.Password = pw
			}
			if t, ok := body["to"].(string); ok {
				to = t
			}
		}
		// 未提交新口令时沿用已保存的口令
		if emailCfg.Password == "" {
			if saved, _ := GetNotificationConfig(); saved != nil {
				emailCfg.Password = saved.EmailConfig.Password
			}
		}
		if emailCfg.SmtpHost == "" || emailCfg.User == "" {
			cfg, _ := GetNotificationConfig()
			if cfg != nil {
				emailCfg = cfg.EmailConfig
				if to == "" {
					to = cfg.EmailConfig.User
				}
			}
		}
		if emailCfg.SmtpHost == "" || emailCfg.User == "" {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": "请先完整填写 SMTP 配置"})
			return
		}

		subject := "NetMon 监控系统测试邮件"
		htmlBody := fmt.Sprintf("<div style='font-family: Arial, sans-serif; padding: 20px;'><h3 style='color: #2563eb;'>🔔 NetMon 监控系统测试邮件</h3><p>这是一条来自 NetMon 监控系统的测试邮件，表示 SMTP 邮件通知配置已生效！</p><p style='color: #888;'>发送时间: %s</p></div>", nowStr)

		if err := SendSmtpEmail(emailCfg, to, subject, htmlBody); err != nil {
			httpx.Respond(w, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "message": "测试邮件发送成功"})

	default:
		httpx.Respond(w, map[string]interface{}{"success": false, "error": "未知的通知渠道类型: " + channel})
	}
}

// NotificationRoutes 注册 /api/v1/notifications 路由
func NotificationRoutes() chi.Router {
	r := chi.NewRouter()

	// GET /api/v1/notifications
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		EnsureNotificationTables()
		db := appdb.DB()

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page <= 0 {
			page = 1
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		offset := (page - 1) * limit
		ntfType := r.URL.Query().Get("type")
		status := r.URL.Query().Get("status")

		query := "SELECT id, type, title, content, status, recipient, related_alert_id, related_task_id, sent_at, error_message, created_at FROM notifications WHERE 1=1"
		var args []interface{}

		if ntfType != "" {
			query += " AND type = ?"
			args = append(args, ntfType)
		}
		if status != "" {
			query += " AND status = ?"
			args = append(args, status)
		}

		// Count total
		countQuery := "SELECT COUNT(*) FROM (" + query + ") as t"
		var total int
		_ = db.QueryRow(countQuery, args...).Scan(&total)

		query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			httpx.Respond(w, map[string]interface{}{"notifications": []NotificationRecord{}, "total": 0})
			return
		}
		defer rows.Close()

		var list []NotificationRecord
		for rows.Next() {
			var rec NotificationRecord
			var content, recipient, alertID, taskID, sentAt, errMsg sql.NullString
			if err := rows.Scan(&rec.ID, &rec.Type, &rec.Title, &content, &rec.Status, &recipient, &alertID, &taskID, &sentAt, &errMsg, &rec.CreatedAt); err == nil {
				if content.Valid {
					rec.Content = &content.String
				}
				if recipient.Valid {
					rec.Recipient = &recipient.String
				}
				if alertID.Valid {
					rec.RelatedAlertID = &alertID.String
				}
				if taskID.Valid {
					rec.RelatedTaskID = &taskID.String
				}
				if sentAt.Valid {
					rec.SentAt = &sentAt.String
				}
				if errMsg.Valid {
					rec.ErrorMessage = &errMsg.String
				}
				list = append(list, rec)
			}
		}
		if list == nil {
			list = []NotificationRecord{}
		}

		httpx.Respond(w, map[string]interface{}{
			"notifications": list,
			"total":         total,
		})
	})

	// GET /api/v1/notifications/stats/summary
	r.Get("/stats/summary", func(w http.ResponseWriter, r *http.Request) {
		EnsureNotificationTables()
		db := appdb.DB()

		var pendingCount, todaySent int
		todayPrefix := time.Now().Format("2006-01-02")

		_ = db.QueryRow("SELECT COUNT(*) FROM notifications WHERE status = 'unread' OR status = 'pending'").Scan(&pendingCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM notifications WHERE (status = 'sent' OR status = 'read') AND created_at LIKE ?", todayPrefix+"%").Scan(&todaySent)

		rows, err := db.Query("SELECT type, COUNT(*) FROM notifications GROUP BY type")
		var typeStats []map[string]interface{}
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tName string
				var count int
				if rows.Scan(&tName, &count) == nil {
					typeStats = append(typeStats, map[string]interface{}{
						"type":  tName,
						"count": count,
					})
				}
			}
		}
		if typeStats == nil {
			typeStats = []map[string]interface{}{}
		}

		httpx.Respond(w, map[string]interface{}{
			"pendingCount": pendingCount,
			"todaySent":    todaySent,
			"typeStats":    typeStats,
		})
	})

	// PUT /api/v1/notifications/{id}/send — 真实投递一条待发送通知
	r.With(middleware.RequireRole("admin")).Put("/{id}/send", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		EnsureNotificationTables()
		db := appdb.DB()
		if db == nil {
			httpx.RespondError(w, "数据库不可用", http.StatusInternalServerError)
			return
		}

		var ntfType, title string
		var content, recipient sql.NullString
		err := db.QueryRow("SELECT type, title, content, recipient FROM notifications WHERE id = ?", id).
			Scan(&ntfType, &title, &content, &recipient)
		if err == sql.ErrNoRows {
			httpx.RespondFailStatus(w, http.StatusNotFound, "通知记录不存在")
			return
		}
		if err != nil {
			httpx.RespondError(w, "读取通知记录失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		cfg, cfgErr := GetNotificationConfig()
		if cfgErr != nil || cfg == nil {
			httpx.RespondFailStatus(w, http.StatusBadGateway, "读取通知配置失败")
			return
		}

		body := ""
		if content.Valid {
			body = content.String
		}

		var sendErr error
		switch strings.ToLower(ntfType) {
		case "webhook":
			target := recipient.String
			if target == "" {
				target = cfg.WebhookURL
			}
			if target == "" {
				sendErr = fmt.Errorf("未配置 Webhook 地址")
				break
			}
			sendErr = SendWebhookPost(target, map[string]interface{}{
				"event":     "notification",
				"title":     title,
				"message":   body,
				"timestamp": time.Now().Format(time.RFC3339),
			})
		case "wechat":
			target := recipient.String
			if target == "" {
				target = cfg.WechatConfig.WebhookURL
			}
			if target == "" {
				sendErr = fmt.Errorf("未配置企业微信机器人地址")
				break
			}
			sendErr = SendWechatRobot(target, title, body)
		case "dingtalk":
			target := recipient.String
			if target == "" {
				target = cfg.DingtalkConfig.WebhookURL
			}
			if target == "" {
				sendErr = fmt.Errorf("未配置钉钉机器人地址")
				break
			}
			sendErr = SendDingtalkRobot(target, title, body)
		case "email":
			target := recipient.String
			if target == "" {
				target = cfg.EmailConfig.User
			}
			sendErr = SendSmtpEmail(cfg.EmailConfig, target, title, body)
		default:
			httpx.RespondFailStatus(w, http.StatusNotImplemented, "该通知类型暂不支持重发: "+ntfType)
			return
		}

		now := time.Now().Format("2006-01-02 15:04:05")
		if sendErr != nil {
			_, _ = db.Exec("UPDATE notifications SET status = 'failed', error_message = ? WHERE id = ?", sendErr.Error(), id)
			httpx.RespondFailStatus(w, http.StatusBadGateway, "通知发送失败: "+sendErr.Error())
			return
		}
		res, err := db.Exec("UPDATE notifications SET status = 'sent', sent_at = ?, error_message = '' WHERE id = ?", now, id)
		if err != nil {
			httpx.RespondError(w, "更新通知状态失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			httpx.RespondFailStatus(w, http.StatusNotFound, "通知记录不存在")
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true, "status": "sent"})
	})

	// DELETE /api/v1/notifications/{id}
	r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, err := appdb.DB().Exec("DELETE FROM notifications WHERE id = ?", id)
		if err != nil {
			httpx.RespondError(w, "删除通知失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"success": true})
	})

	return r
}
