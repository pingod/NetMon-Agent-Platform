// InfluxDB 写入服务 — 移植自 TS influxdbWriteService.ts
// 职责：管理 influxdb_write_settings 配置 + 将 SNMP 采集结果写入 InfluxDB
// 支持 InfluxDB 1.x (/write)、2.x/3.x (/api/v2/write) + token 认证
package services

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"itops-backend/internal/appdb"
	"itops-backend/internal/config"
	"itops-backend/internal/crypto"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 类型
// ───────────────────────────────────────────────

// InfluxdbWriteSettings InfluxDB 写入配置
type InfluxdbWriteSettings struct {
	ID                string  `json:"id"`
	Enabled           int     `json:"enabled"`
	DBConnectionID    *string `json:"db_connection_id"`
	MeasurementPrefix string  `json:"measurement_prefix"`
	WriteMode         string  `json:"write_mode"`
	BatchSize         int     `json:"batch_size"`
	FlushIntervalMs   int     `json:"flush_interval_ms"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

// SnmpCollectResultRow SNMP 采集结果行
type SnmpCollectResultRow struct {
	TaskID      string `json:"task_id"`
	TaskName    string `json:"task_name"`
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	DeviceIP    string `json:"device_ip"`
	OID         string `json:"oid"`
	MetricName  string `json:"metric_name"`
	MetricValue string `json:"metric_value"`
	MetricType  string `json:"metric_type"`
	SampledAt   string `json:"sampled_at"`
}

type InfluxV2OrgItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type InfluxV2BucketItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	OrgID string `json:"orgID"`
	Type  string `json:"type"`
}

// ───────────────────────────────────────────────
// 服务实现
// ───────────────────────────────────────────────

var (
	influxdbPendingBuffer []SnmpCollectResultRow
	influxdbFlushTimer    *time.Timer
	influxdbMu            sync.Mutex
)

// EnsureInfluxdbSettings 确保 settings 表存在且至少有一条记录，并支持环境变量自动注入
func EnsureInfluxdbSettings() {
	db := appdb.DB()
	if db == nil {
		return
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS influxdb_write_settings (
		id VARCHAR(64) PRIMARY KEY,
		enabled INTEGER DEFAULT 0,
		db_connection_id VARCHAR(64),
		measurement_prefix VARCHAR(64) DEFAULT 'snmp',
		write_mode VARCHAR(32) DEFAULT 'async',
		batch_size INTEGER DEFAULT 100,
		flush_interval_ms INTEGER DEFAULT 5000,
		created_at VARCHAR(64),
		updated_at VARCHAR(64)
	)`
	if _, err := db.Exec(createTableSQL); err != nil {
		logger.Warnf("Failed to ensure influxdb_write_settings table: %v", err)
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM influxdb_write_settings").Scan(&count)
	if err == nil && count == 0 {
		now := time.Now().Format("2006-01-02 15:04:05")
		_, err = db.Exec(`INSERT INTO influxdb_write_settings (id, enabled, db_connection_id, measurement_prefix, write_mode, batch_size, flush_interval_ms, created_at, updated_at)
			VALUES ('default', 0, NULL, 'snmp', 'async', 100, 5000, ?, ?)`, now, now)
		if err != nil {
			logger.Warnf("Failed to insert default influxdb_write_settings: %v", err)
		}
	}

	// 若环境变量开启了 INFLUXDB_ENABLED，自动同步至配置中心
	if config.Env != nil && config.Env.InfluxdbEnabled {
		connID := "env-influxdb"
		dbType := "influxdb-v2"
		if config.Env.InfluxdbVersion == "v1" {
			dbType = "influxdb-v1"
		} else if config.Env.InfluxdbVersion == "v3" {
			dbType = "influxdb-v3"
		}

		encToken := crypto.Encrypt(config.Env.InfluxdbToken)
		encPass := crypto.Encrypt(config.Env.InfluxdbPassword)

		_ = repos.DbConnections.Insert(&repos.DbConnection{
			ID:           connID,
			Name:         "环境变量 InfluxDB",
			DBType:       dbType,
			Host:         config.Env.InfluxdbHost,
			Port:         config.Env.InfluxdbPort,
			Username:     config.Env.InfluxdbUsername,
			Password:     encPass,
			Database:     config.Env.InfluxdbDatabase,
			Token:        &encToken,
			Organization: &config.Env.InfluxdbOrg,
			Bucket:       &config.Env.InfluxdbBucket,
			Enabled:      1,
		})

		prefix := config.Env.InfluxdbMeasurementPrefix
		if prefix == "" {
			prefix = "snmp"
		}
		mode := config.Env.InfluxdbWriteMode
		if mode == "" {
			mode = "async"
		}
		batchSize := config.Env.InfluxdbBatchSize
		if batchSize <= 0 {
			batchSize = 100
		}
		flushMs := config.Env.InfluxdbFlushIntervalMs
		if flushMs <= 0 {
			flushMs = 3000
		}
		now := time.Now().Format("2006-01-02 15:04:05")

		_, _ = db.Exec(`UPDATE influxdb_write_settings SET enabled = 1, db_connection_id = ?, measurement_prefix = ?, write_mode = ?, batch_size = ?, flush_interval_ms = ?, updated_at = ? WHERE id = 'default'`,
			connID, prefix, mode, batchSize, flushMs, now)
	}
}

// GetInfluxdbSettings 获取当前配置
func GetInfluxdbSettings() *InfluxdbWriteSettings {
	EnsureInfluxdbSettings()
	db := appdb.DB()
	if db == nil {
		return nil
	}

	var s InfluxdbWriteSettings
	var dbConnID, createdAt, updatedAt sql.NullString
	row := db.QueryRow(`SELECT id, enabled, db_connection_id, measurement_prefix, write_mode, batch_size, flush_interval_ms, created_at, updated_at
		FROM influxdb_write_settings WHERE id = 'default' LIMIT 1`)
	err := row.Scan(&s.ID, &s.Enabled, &dbConnID, &s.MeasurementPrefix, &s.WriteMode, &s.BatchSize, &s.FlushIntervalMs, &createdAt, &updatedAt)
	if err != nil {
		// 回退尝试不带 WHERE
		rowFallback := db.QueryRow(`SELECT id, enabled, db_connection_id, measurement_prefix, write_mode, batch_size, flush_interval_ms, created_at, updated_at
			FROM influxdb_write_settings LIMIT 1`)
		if errFallback := rowFallback.Scan(&s.ID, &s.Enabled, &dbConnID, &s.MeasurementPrefix, &s.WriteMode, &s.BatchSize, &s.FlushIntervalMs, &createdAt, &updatedAt); errFallback != nil {
			logger.Warnf("GetInfluxdbSettings query error: %v", errFallback)
			return &InfluxdbWriteSettings{
				ID:                "default",
				Enabled:           0,
				MeasurementPrefix: "snmp",
				WriteMode:         "async",
				BatchSize:         100,
				FlushIntervalMs:   5000,
			}
		}
	}
	s.DBConnectionID = nullStrToPtr(dbConnID)
	if createdAt.Valid {
		s.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		s.UpdatedAt = updatedAt.String
	}
	return &s
}

// UpdateInfluxdbSettings 更新配置
func UpdateInfluxdbSettings(fields map[string]interface{}) *InfluxdbWriteSettings {
	EnsureInfluxdbSettings()
	db := appdb.DB()
	if db == nil {
		return nil
	}

	sets := []string{}
	params := []interface{}{}

	if v, ok := fields["enabled"]; ok {
		sets = append(sets, "enabled = ?")
		params = append(params, v)
	}
	if v, ok := fields["db_connection_id"]; ok {
		sets = append(sets, "db_connection_id = ?")
		if s, ok := v.(string); ok && s == "" {
			params = append(params, nil)
		} else {
			params = append(params, v)
		}
	}
	if v, ok := fields["measurement_prefix"]; ok {
		sets = append(sets, "measurement_prefix = ?")
		params = append(params, v)
	}
	if v, ok := fields["write_mode"]; ok {
		sets = append(sets, "write_mode = ?")
		params = append(params, v)
	}
	if v, ok := fields["batch_size"]; ok {
		sets = append(sets, "batch_size = ?")
		params = append(params, v)
	}
	if v, ok := fields["flush_interval_ms"]; ok {
		sets = append(sets, "flush_interval_ms = ?")
		params = append(params, v)
	}

	if len(sets) > 0 {
		now := time.Now().Format("2006-01-02 15:04:05")
		sets = append(sets, "updated_at = ?")
		params = append(params, now)
		query := fmt.Sprintf("UPDATE influxdb_write_settings SET %s WHERE id = 'default'", strings.Join(sets, ", "))
		res, err := db.Exec(query, params...)
		if err != nil {
			logger.Errorf("Failed to update influxdb_write_settings: %v", err)
		} else if rows, _ := res.RowsAffected(); rows == 0 {
			// 如果没有更新到任何行，尝试没有 WHERE 的更新或重新确保插入
			queryAll := fmt.Sprintf("UPDATE influxdb_write_settings SET %s", strings.Join(sets, ", "))
			_, _ = db.Exec(queryAll, params...)
		}
	}

	return GetInfluxdbSettings()
}

// IsInfluxdbWriteEnabled 是否启用了 InfluxDB 写入
func IsInfluxdbWriteEnabled() bool {
	s := GetInfluxdbSettings()
	return s != nil && s.Enabled == 1 && s.DBConnectionID != nil && *s.DBConnectionID != ""
}

// WriteSnmpResultsToInfluxdb 将采集结果写入 InfluxDB（异步缓冲或直接写入）
func WriteSnmpResultsToInfluxdb(results []SnmpCollectResultRow) {
	if !IsInfluxdbWriteEnabled() || len(results) == 0 {
		return
	}

	settings := GetInfluxdbSettings()

	if settings.WriteMode == "sync" {
		flushToInfluxdb(results)
	} else {
		// 异步：先入缓冲区
		influxdbMu.Lock()
		influxdbPendingBuffer = append(influxdbPendingBuffer, results...)

		if len(influxdbPendingBuffer) >= settings.BatchSize {
			batch := influxdbPendingBuffer
			influxdbPendingBuffer = nil
			if influxdbFlushTimer != nil {
				influxdbFlushTimer.Stop()
				influxdbFlushTimer = nil
			}
			influxdbMu.Unlock()
			flushToInfluxdb(batch)
		} else {
			if influxdbFlushTimer == nil {
				influxdbFlushTimer = time.AfterFunc(time.Duration(settings.FlushIntervalMs)*time.Millisecond, func() {
					FlushInfluxdbBuffer()
				})
			}
			influxdbMu.Unlock()
		}
	}
}

// FlushInfluxdbBuffer 刷新缓冲区
func FlushInfluxdbBuffer() {
	influxdbMu.Lock()
	if len(influxdbPendingBuffer) == 0 {
		influxdbMu.Unlock()
		return
	}
	batch := influxdbPendingBuffer
	influxdbPendingBuffer = nil
	if influxdbFlushTimer != nil {
		influxdbFlushTimer.Stop()
		influxdbFlushTimer = nil
	}
	influxdbMu.Unlock()

	flushToInfluxdb(batch)
}

// flushToInfluxdb 实际写入 InfluxDB
func flushToInfluxdb(results []SnmpCollectResultRow) {
	settings := GetInfluxdbSettings()
	if settings == nil || settings.DBConnectionID == nil || *settings.DBConnectionID == "" {
		return
	}

	conn, err := repos.DbConnections.GetByID(*settings.DBConnectionID)
	if err != nil || conn == nil {
		logger.Warnf("InfluxDB 写入失败：数据库连接不存在 (id=%s)", *settings.DBConnectionID)
		return
	}

	// 构建 Line Protocol 数据
	var lines []string
	for _, r := range results {
		measurement := fmt.Sprintf("%s_%s", settings.MeasurementPrefix, escapeMeasurement(r.MetricName))
		if r.MetricName == "" {
			measurement = fmt.Sprintf("%s_unknown", settings.MeasurementPrefix)
		}
		tags := fmt.Sprintf("device=%s,ip=%s,task=%s,oid=%s",
			escapeTag(r.DeviceName), escapeTag(r.DeviceIP), escapeTag(r.TaskName), escapeTag(r.OID))
		value := "value=0"
		if r.MetricValue != "" {
			// 尝试转为数字
			value = fmt.Sprintf("value=%s", r.MetricValue)
		}
		ts := parseTimeNanos(r.SampledAt)
		lines = append(lines, fmt.Sprintf("%s,%s %s %d", measurement, tags, value, ts))
	}

	body := strings.Join(lines, "\n")

	// 解密凭证
	decryptedPassword := ""
	if conn.Password != "" {
		decrypted, err := crypto.Decrypt(conn.Password)
		if err == nil {
			decryptedPassword = decrypted
		} else {
			decryptedPassword = conn.Password
		}
	}

	baseURL := fmt.Sprintf("http://%s:%d", conn.Host, conn.Port)

	var writeErr error
	dbType := strings.ToLower(strings.TrimSpace(conn.DBType))
	switch {
	case dbType == "influxdb-v1" || dbType == "influxdb1" || dbType == "influx-v1":
		writeErr = writeInfluxdbV1(baseURL, conn.Database, conn.Username, decryptedPassword, body, ptrStr(conn.RetentionPolicy))
	case dbType == "influxdb-v2" || dbType == "influxdb-v3" || dbType == "influxdb" || strings.Contains(dbType, "influx"):
		writeErr = writeInfluxdbV2WithConn(conn, body)
	default:
		logger.Warnf("InfluxDB 写入失败：不支持的类型 %s", conn.DBType)
		return
	}

	if writeErr != nil {
		logger.Errorf("InfluxDB 写入失败: %v", writeErr)
	} else {
		logger.Infof("InfluxDB 写入成功: %d 条记录 (%s)", len(results), conn.DBType)
	}
}

// writeInfluxdbV1 InfluxDB 1.x: POST /write
func writeInfluxdbV1(baseURL, database, username, password, body, rp string) error {
	url := fmt.Sprintf("%s/write?db=%s&u=%s&p=%s", baseURL,
		escapeURL(database), escapeURL(username), escapeURL(password))
	if rp != "" {
		url += "&rp=" + escapeURL(rp)
	}

	resp, err := http.Post(url, "text/plain", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("InfluxDB 1.x write request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("InfluxDB 1.x write failed (%d): %s", resp.StatusCode, string(text))
	}
	return nil
}

// performV2Write 尝试发送一次 InfluxDB v2 write 请求
func performV2Write(baseURL, token, orgParam, orgIDParam, bucket, body string) (int, string, error) {
	url := baseURL + "/api/v2/write?precision=ns"
	if orgParam != "" {
		url += "&org=" + escapeURL(orgParam)
	} else if orgIDParam != "" {
		url += "&orgID=" + escapeURL(orgIDParam)
	}
	if bucket != "" {
		url += "&bucket=" + escapeURL(bucket)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("InfluxDB 2.x 请求网络异常: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	respStr := string(bodyBytes)

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return resp.StatusCode, respStr, nil
	}
	return resp.StatusCode, respStr, fmt.Errorf("InfluxDB 2.x write failed (%d): %s", resp.StatusCode, respStr)
}

// autoDiscoverV2OrgsAndBuckets 自动从 InfluxDB 2.x 获取当前 Token 可见的所有组织和用户存储桶
func autoDiscoverV2OrgsAndBuckets(baseURL, token string) ([]InfluxV2OrgItem, []InfluxV2BucketItem) {
	client := &http.Client{Timeout: 8 * time.Second}

	var orgs []InfluxV2OrgItem
	reqOrg, err := http.NewRequest("GET", baseURL+"/api/v2/orgs", nil)
	if err == nil {
		reqOrg.Header.Set("Authorization", "Token "+token)
		if respOrg, err := client.Do(reqOrg); err == nil {
			defer respOrg.Body.Close()
			if respOrg.StatusCode == http.StatusOK {
				var orgResp struct {
					Orgs []InfluxV2OrgItem `json:"orgs"`
				}
				_ = json.NewDecoder(respOrg.Body).Decode(&orgResp)
				orgs = orgResp.Orgs
			}
		}
	}

	var buckets []InfluxV2BucketItem
	reqBucket, err := http.NewRequest("GET", baseURL+"/api/v2/buckets", nil)
	if err == nil {
		reqBucket.Header.Set("Authorization", "Token "+token)
		if respBucket, err := client.Do(reqBucket); err == nil {
			defer respBucket.Body.Close()
			if respBucket.StatusCode == http.StatusOK {
				var bucketResp struct {
					Buckets []InfluxV2BucketItem `json:"buckets"`
				}
				if err := json.NewDecoder(respBucket.Body).Decode(&bucketResp); err == nil {
					for _, b := range bucketResp.Buckets {
						// 过滤内部系统 bucket（如 _monitoring, _tasks 等）
						if !strings.HasPrefix(b.Name, "_") {
							buckets = append(buckets, b)
						}
					}
				}
			}
		}
	}

	return orgs, buckets
}

// autoCreateV2Bucket 尝试自动创建目标 bucket
func autoCreateV2Bucket(baseURL, token, orgID, bucketName string) bool {
	if orgID == "" || bucketName == "" {
		return false
	}
	payload := map[string]interface{}{
		"orgID": orgID,
		"name":  bucketName,
		"retentionRules": []map[string]interface{}{
			{
				"type":         "expire",
				"everySeconds": 2592000, // 30 天
			},
		},
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", baseURL+"/api/v2/buckets", bytes.NewReader(data))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK
}

// writeInfluxdbV2WithConn 智能写入 InfluxDB 2.x/3.x，支持缺失参数自动探测、自愈匹配与自动建 Bucket
func writeInfluxdbV2WithConn(conn *repos.DbConnection, body string) error {
	if conn == nil {
		return fmt.Errorf("数据库连接不存在")
	}

	baseURL := fmt.Sprintf("http://%s:%d", conn.Host, conn.Port)

	// 解密 token
	token := ""
	if conn.Token != nil && *conn.Token != "" {
		if d, err := crypto.Decrypt(*conn.Token); err == nil {
			token = d
		} else {
			token = *conn.Token
		}
	}
	// 若 Token 为空，回退尝试密码字段（某些用户会将 Token 填在 Password 字段）
	if token == "" && conn.Password != "" {
		if d, err := crypto.Decrypt(conn.Password); err == nil {
			token = d
		} else {
			token = conn.Password
		}
	}
	if token == "" {
		return fmt.Errorf("InfluxDB 2.x 需要配置 API Token，请在「数据库管理」中编辑该连接并填写 Token")
	}

	org := ptrStr(conn.Organization)
	if org == "" && conn.Username != "" {
		org = conn.Username
	}
	bucket := ptrStr(conn.Bucket)
	if bucket == "" && conn.Database != "" {
		bucket = conn.Database
	}

	// 1. 如果已配置了 org 和 bucket，首先尝试标准写入
	if org != "" && bucket != "" {
		status, _, err := performV2Write(baseURL, token, org, "", bucket, body)
		if err == nil {
			return nil
		}
		// 若因 orgID 传错，尝试以 orgID 写入
		if status == http.StatusNotFound {
			_, _, errID := performV2Write(baseURL, token, "", org, bucket, body)
			if errID == nil {
				return nil
			}
		}
	}

	// 2. 若缺少参数或返回 404，启动智能自愈探测
	discoveredOrgs, discoveredBuckets := autoDiscoverV2OrgsAndBuckets(baseURL, token)

	var targetOrg InfluxV2OrgItem
	if len(discoveredOrgs) > 0 {
		matched := false
		for _, o := range discoveredOrgs {
			if strings.EqualFold(o.Name, org) || o.ID == org {
				targetOrg = o
				matched = true
				break
			}
		}
		if !matched {
			targetOrg = discoveredOrgs[0]
		}
	}

	var targetBucket InfluxV2BucketItem
	if len(discoveredBuckets) > 0 {
		matched := false
		for _, b := range discoveredBuckets {
			if strings.EqualFold(b.Name, bucket) || b.ID == bucket || strings.EqualFold(b.Name, conn.Database) {
				targetBucket = b
				matched = true
				break
			}
		}
		if !matched {
			targetBucket = discoveredBuckets[0]
		}
	}

	// 如果找到了组织但没有可用的 Bucket，尝试自动创建默认的 snmp / 配置的 bucket
	if targetOrg.ID != "" && targetBucket.Name == "" {
		wantBucket := bucket
		if wantBucket == "" {
			wantBucket = conn.Database
		}
		if wantBucket == "" {
			wantBucket = "snmp"
		}
		if autoCreateV2Bucket(baseURL, token, targetOrg.ID, wantBucket) {
			targetBucket = InfluxV2BucketItem{
				Name:  wantBucket,
				OrgID: targetOrg.ID,
			}
		}
	}

	// 3. 采用探测到的最优组织与 Bucket 执行写入
	if targetOrg.Name != "" && targetBucket.Name != "" {
		// 尝试按 Org Name 写入
		_, _, errName := performV2Write(baseURL, token, targetOrg.Name, "", targetBucket.Name, body)
		if errName == nil {
			// 成功！自愈同步写回数据库连接配置
			if conn.Organization == nil || *conn.Organization == "" || conn.Bucket == nil || *conn.Bucket == "" {
				conn.Organization = &targetOrg.Name
				conn.Bucket = &targetBucket.Name
				_ = repos.DbConnections.Update(conn.ID, conn)
			}
			return nil
		}
		// 尝试按 Org ID 写入
		_, _, errID := performV2Write(baseURL, token, "", targetOrg.ID, targetBucket.Name, body)
		if errID == nil {
			if conn.Organization == nil || *conn.Organization == "" || conn.Bucket == nil || *conn.Bucket == "" {
				conn.Organization = &targetOrg.Name
				conn.Bucket = &targetBucket.Name
				_ = repos.DbConnections.Update(conn.ID, conn)
			}
			return nil
		}
	}

	// 4. 若最终仍无法写入，输出精准友好的诊断提示
	var orgList []string
	for _, o := range discoveredOrgs {
		orgList = append(orgList, fmt.Sprintf("%s(%s)", o.Name, o.ID))
	}
	var bucketList []string
	for _, b := range discoveredBuckets {
		bucketList = append(bucketList, b.Name)
	}

	return fmt.Errorf("InfluxDB 2.x 写入失败 (未找到匹配的组织或 Bucket)。当前配置 [组织: %q, Bucket: %q]，服务端探测到的可用组织: %v，可用 Bucket: %v。请在「数据库管理」中编辑该连接并正确填写 Organization 和 Bucket",
		org, bucket, orgList, bucketList)
}

// ───────────────────────────────────────────────
// 连接测试与测试写入
// ───────────────────────────────────────────────

// TestWriteToInfluxdb 测试向指定的 InfluxDB 写入一条数据
func TestWriteToInfluxdb(conn *repos.DbConnection, prefix string) error {
	if conn == nil {
		return fmt.Errorf("数据库连接为空")
	}
	if prefix == "" {
		prefix = "snmp"
	}

	decryptedPassword := ""
	if conn.Password != "" {
		decrypted, err := crypto.Decrypt(conn.Password)
		if err == nil {
			decryptedPassword = decrypted
		} else {
			decryptedPassword = conn.Password
		}
	}

	body := fmt.Sprintf("%s_test_probe,host=localhost,status=ok value=1.0 %d", prefix, time.Now().UnixNano())
	baseURL := fmt.Sprintf("http://%s:%d", conn.Host, conn.Port)

	dbType := strings.ToLower(strings.TrimSpace(conn.DBType))
	switch {
	case dbType == "influxdb-v1" || dbType == "influxdb1" || dbType == "influx-v1":
		return writeInfluxdbV1(baseURL, conn.Database, conn.Username, decryptedPassword, body, ptrStr(conn.RetentionPolicy))
	case dbType == "influxdb-v2" || dbType == "influxdb-v3" || dbType == "influxdb" || strings.Contains(dbType, "influx"):
		return writeInfluxdbV2WithConn(conn, body)
	default:
		return fmt.Errorf("不支持的 InfluxDB 类型: %s", conn.DBType)
	}
}

// TestInfluxdbConnection 测试 InfluxDB 连接
// 支持 v1 (user/pass)、v2 (token+org+bucket)、v3 (health check)
func TestInfluxdbConnection(dbType, host string, port int, username, password, database, token, organization, bucket string) (bool, string, string) {
	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	client := &http.Client{Timeout: 10 * time.Second}

	normType := strings.ToLower(strings.TrimSpace(dbType))
	switch {
	case normType == "influxdb-v1" || normType == "influxdb1" || normType == "influx-v1":
		// InfluxDB 1.x: GET /query?q=SHOW+DATABASES
		url := fmt.Sprintf("%s/query?q=SHOW+DATABASES&u=%s&p=%s", baseURL,
			escapeURL(username), escapeURL(password))
		resp, err := client.Get(url)
		if err != nil {
			return false, "InfluxDB 1.x 连接测试失败", err.Error()
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true, "InfluxDB 1.x 连接成功", ""
		}
		text, _ := io.ReadAll(resp.Body)
		return false, "InfluxDB 1.x 连接失败", string(text)

	case normType == "influxdb-v3" || normType == "influx3":
		// InfluxDB 3.x: GET /health
		url := baseURL + "/health"
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return false, "InfluxDB 3.x 连接测试失败", err.Error()
		}
		effectiveToken := token
		if effectiveToken == "" {
			effectiveToken = password
		}
		if effectiveToken != "" {
			req.Header.Set("Authorization", "Bearer "+effectiveToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			return false, "InfluxDB 3.x 连接测试失败", err.Error()
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true, "InfluxDB 3.x 连接成功", ""
		}
		text, _ := io.ReadAll(resp.Body)
		return false, "InfluxDB 3.x 连接失败", string(text)

	case normType == "influxdb-v2" || normType == "influxdb" || normType == "influx2" || strings.Contains(normType, "influx"):
		effectiveToken := token
		if effectiveToken == "" {
			effectiveToken = password
		}
		if effectiveToken == "" {
			return false, "InfluxDB 2.x 需要 API Token 认证，请填写 Token", ""
		}

		// 智能探测可用组织与存储桶
		orgs, buckets := autoDiscoverV2OrgsAndBuckets(baseURL, effectiveToken)
		if len(orgs) > 0 || len(buckets) > 0 {
			var orgNames []string
			for _, o := range orgs {
				orgNames = append(orgNames, o.Name)
			}
			var bNames []string
			for _, b := range buckets {
				bNames = append(bNames, b.Name)
			}
			msg := fmt.Sprintf("InfluxDB 2.x 连接成功！(检测到组织: %s, 可用 Bucket: %s)",
				strings.Join(orgNames, "/"), strings.Join(bNames, "/"))
			return true, msg, ""
		}

		// 回退尝试 GET /api/v2/buckets
		url := baseURL + "/api/v2/buckets"
		if organization != "" {
			url += "?org=" + escapeURL(organization)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return false, "InfluxDB 2.x 连接测试失败", err.Error()
		}
		req.Header.Set("Authorization", "Token "+effectiveToken)
		resp, err := client.Do(req)
		if err != nil {
			return false, "InfluxDB 2.x 连接测试失败", err.Error()
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true, "InfluxDB 2.x 连接成功", ""
		}
		text, _ := io.ReadAll(resp.Body)
		return false, "InfluxDB 2.x 连接失败", string(text)

	default:
		return false, fmt.Sprintf("不支持的 InfluxDB 类型: %s", dbType), ""
	}
}

// ───────────────────────────────────────────────
// 工具函数
// ───────────────────────────────────────────────

// escapeTag 转义 Line Protocol tag 值中的特殊字符
func escapeTag(s string) string {
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "=", "\\=")
	return s
}

// escapeMeasurement 转义 measurement 名称
func escapeMeasurement(s string) string {
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	return s
}

// escapeURL URL 编码（简单实现，仅处理空格和特殊字符）
func escapeURL(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == '~' {
			sb.WriteRune(r)
		} else if r == ' ' {
			sb.WriteString("+")
		} else {
			sb.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return sb.String()
}

// parseTimeNanos 将 ISO 时间字符串转为纳秒时间戳
func parseTimeNanos(timeStr string) int64 {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		// 尝试其他格式
		t, err = time.Parse("2006-01-02T15:04:05.000Z", timeStr)
		if err != nil {
			return time.Now().UnixNano()
		}
	}
	return t.UnixNano()
}

// nullStrToPtr sql.NullString → *string
func nullStrToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// ptrStr 安全解引用 *string，nil 返回空字符串
func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
