// SNMP 凭证/Trap/接口指标/巡检历史，等价于 TS snmpRepository.ts
package repos

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/crypto"
)

// ───────────────────────────────────────────────
// SNMP 凭证
// ───────────────────────────────────────────────

// SnmpCredential SNMP 凭证记录
type SnmpCredential struct {
	ID               string  `json:"id"`
	DeviceID         *string `json:"device_id"`
	Name             string  `json:"name"`
	Community        *string `json:"community,omitempty"`
	SnmpVersion      string  `json:"snmp_version"`
	SnmpPort         int     `json:"snmp_port"`
	SnmpUser         *string `json:"snmp_user,omitempty"`
	SnmpAuthProtocol *string `json:"snmp_auth_protocol,omitempty"`
	SnmpAuthKey      *string `json:"snmp_auth_key,omitempty"`
	SnmpPrivProtocol *string `json:"snmp_priv_protocol,omitempty"`
	SnmpPrivKey      *string `json:"snmp_priv_key,omitempty"`
	Host             string  `json:"host"` // COALESCE(c.host, nd.ip_address)
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// SnmpCredentialListRow 列表展示行（不含敏感字段）
type SnmpCredentialListRow struct {
	ID               string  `json:"id"`
	DeviceID         *string `json:"device_id"`
	Name             string  `json:"name"`
	Community        *string `json:"community"`
	SnmpVersion      string  `json:"snmp_version"`
	SnmpPort         int     `json:"snmp_port"`
	SnmpUser         *string `json:"snmp_user"`
	SnmpAuthProtocol *string `json:"snmp_auth_protocol"`
	SnmpPrivProtocol *string `json:"snmp_priv_protocol"`
	Host             string  `json:"host"`
	CreatedAt        string  `json:"created_at"`
}

// SnmpCredentialCreateInput 创建凭证输入
type SnmpCredentialCreateInput struct {
	ID               string
	DeviceID         string
	Name             string
	Community        string
	SnmpVersion      string
	SnmpPort         int
	SnmpUser         string
	SnmpAuthProtocol string
	SnmpAuthKey      string
	SnmpPrivProtocol string
	SnmpPrivKey      string
}

// SnmpCredentialRepo SNMP 凭证仓库
type SnmpCredentialRepo struct{}

var SnmpCredentials = &SnmpCredentialRepo{}

func ensureSnmpCredentialsSchema(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS snmp_credentials (
			id VARCHAR(36) PRIMARY KEY,
			device_id VARCHAR(36) NULL,
			name VARCHAR(255) NOT NULL DEFAULT 'default',
			community TEXT,
			snmp_version VARCHAR(20) NOT NULL DEFAULT 'v2c',
			snmp_port INT NOT NULL DEFAULT 161,
			snmp_user VARCHAR(255) DEFAULT '',
			snmp_auth_protocol VARCHAR(50) DEFAULT '',
			snmp_auth_key TEXT,
			snmp_priv_protocol VARCHAR(50) DEFAULT '',
			snmp_priv_key TEXT,
			host VARCHAR(255) DEFAULT '',
			created_at VARCHAR(50),
			updated_at VARCHAR(50)
		)
	`)

	alters := []string{
		"ALTER TABLE snmp_credentials ADD COLUMN name VARCHAR(255) DEFAULT 'default'",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_version VARCHAR(20) DEFAULT 'v2c'",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_port INT DEFAULT 161",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_user VARCHAR(255) DEFAULT ''",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_auth_protocol VARCHAR(50) DEFAULT ''",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_auth_key TEXT",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_priv_protocol VARCHAR(50) DEFAULT ''",
		"ALTER TABLE snmp_credentials ADD COLUMN snmp_priv_key TEXT",
		"ALTER TABLE snmp_credentials ADD COLUMN host VARCHAR(255) DEFAULT ''",
		"ALTER TABLE snmp_credentials ADD COLUMN created_at VARCHAR(50)",
		"ALTER TABLE snmp_credentials ADD COLUMN updated_at VARCHAR(50)",
	}
	for _, a := range alters {
		_, _ = db.Exec(a)
	}
}

// List 列出所有 SNMP 凭证
func (r *SnmpCredentialRepo) List(deviceID string) ([]SnmpCredentialListRow, error) {
	db := appdb.DB()
	ensureSnmpCredentialsSchema(db)

	query := `
		SELECT c.id, c.device_id, c.name, c.snmp_version, c.snmp_port,
		       c.snmp_user, c.snmp_auth_protocol, c.snmp_priv_protocol,
		       COALESCE(c.host, nd.ip_address, '') AS host, c.created_at, c.community
		FROM snmp_credentials c
		LEFT JOIN network_devices nd ON c.device_id = nd.id`
	var args []interface{}
	if deviceID != "" {
		query += " WHERE c.device_id = ?"
		args = append(args, deviceID)
	}
	query += " ORDER BY c.created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SnmpCredentialListRow
	for rows.Next() {
		var c SnmpCredentialListRow
		var devID, snmpUser, authProto, privProto, community sql.NullString
		if err := rows.Scan(&c.ID, &devID, &c.Name, &c.SnmpVersion, &c.SnmpPort,
			&snmpUser, &authProto, &privProto, &c.Host, &c.CreatedAt, &community); err != nil {
			continue
		}
		c.DeviceID = ns(devID)
		c.SnmpUser = ns(snmpUser)
		c.SnmpAuthProtocol = ns(authProto)
		c.SnmpPrivProtocol = ns(privProto)
		if community.Valid && community.String != "" {
			commStr := community.String
			if strings.HasPrefix(commStr, "2:") {
				dec, err := crypto.Decrypt(commStr)
				if err == nil {
					c.Community = &dec
				} else {
					def := "public"
					c.Community = &def
				}
			} else {
				c.Community = &commStr
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// GetByID 获取单个凭证完整信息（含敏感字段）
func (r *SnmpCredentialRepo) GetByID(id string) (*SnmpCredential, error) {
	db := appdb.DB()
	ensureSnmpCredentialsSchema(db)

	query := `
		SELECT c.id, c.device_id, c.name, c.community, c.snmp_version, c.snmp_port,
		       c.snmp_user, c.snmp_auth_protocol, c.snmp_auth_key,
		       c.snmp_priv_protocol, c.snmp_priv_key,
		       COALESCE(c.host, nd.ip_address) AS host, c.created_at, c.updated_at
		FROM snmp_credentials c
		LEFT JOIN network_devices nd ON c.device_id = nd.id
		WHERE c.id = ?`
	row := db.QueryRow(query, id)
	return scanSnmpCredential(row)
}

// GetByDeviceID 通过设备 ID 获取凭证
func (r *SnmpCredentialRepo) GetByDeviceID(deviceID string) (*SnmpCredential, error) {
	db := appdb.DB()
	ensureSnmpCredentialsSchema(db)

	query := `
		SELECT c.id, c.device_id, c.name, c.community, c.snmp_version, c.snmp_port,
		       c.snmp_user, c.snmp_auth_protocol, c.snmp_auth_key,
		       c.snmp_priv_protocol, c.snmp_priv_key,
		       COALESCE(c.host, nd.ip_address) AS host, c.created_at, c.updated_at
		FROM snmp_credentials c
		LEFT JOIN network_devices nd ON c.device_id = nd.id
		WHERE c.device_id = ? LIMIT 1`
	row := db.QueryRow(query, deviceID)
	return scanSnmpCredential(row)
}

// GetDefault 获取全局默认凭证（device_id 为 NULL）
func (r *SnmpCredentialRepo) GetDefault() (*SnmpCredential, error) {
	db := appdb.DB()
	ensureSnmpCredentialsSchema(db)

	query := `
		SELECT c.id, c.device_id, c.name, c.community, c.snmp_version, c.snmp_port,
		       c.snmp_user, c.snmp_auth_protocol, c.snmp_auth_key,
		       c.snmp_priv_protocol, c.snmp_priv_key,
		       c.host, c.created_at, c.updated_at
		FROM snmp_credentials c
		WHERE c.device_id IS NULL ORDER BY c.created_at DESC LIMIT 1`
	row := db.QueryRow(query)
	return scanSnmpCredential(row)
}

func scanSnmpCredential(row *sql.Row) (*SnmpCredential, error) {
	var c SnmpCredential
	var deviceID, community, snmpUser, authProto, authKey, privProto, privKey, host sql.NullString
	err := row.Scan(&c.ID, &deviceID, &c.Name, &community, &c.SnmpVersion, &c.SnmpPort,
		&snmpUser, &authProto, &authKey, &privProto, &privKey, &host, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.DeviceID = ns(deviceID)
	if community.Valid && community.String != "" {
		commStr := community.String
		if strings.HasPrefix(commStr, "2:") {
			dec, err := crypto.Decrypt(commStr)
			if err == nil {
				c.Community = &dec
			} else {
				def := "public"
				c.Community = &def
			}
		} else {
			c.Community = &commStr
		}
	}
	c.SnmpUser = ns(snmpUser)
	c.SnmpAuthProtocol = ns(authProto)
	c.SnmpAuthKey = ns(authKey)
	c.SnmpPrivProtocol = ns(privProto)
	c.SnmpPrivKey = ns(privKey)
	c.Host = host.String
	return &c, nil
}

// Create 创建 SNMP 凭证
func (r *SnmpCredentialRepo) Create(input SnmpCredentialCreateInput) error {
	db := appdb.DB()
	ensureSnmpCredentialsSchema(db)

	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.SnmpVersion == "" {
		input.SnmpVersion = "2c"
	}
	if input.SnmpPort == 0 {
		input.SnmpPort = 161
	}
	now := time.Now().Format("2006-01-02 15:04:05")

	_, err := db.Exec(`
		INSERT INTO snmp_credentials (id, device_id, name, community, snmp_version, snmp_port,
			snmp_user, snmp_auth_protocol, snmp_auth_key, snmp_priv_protocol, snmp_priv_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, sqlNullStr(input.DeviceID), input.Name, sqlNullStr(input.Community),
		input.SnmpVersion, input.SnmpPort,
		sqlNullStr(input.SnmpUser), sqlNullStr(input.SnmpAuthProtocol), sqlNullStr(input.SnmpAuthKey),
		sqlNullStr(input.SnmpPrivProtocol), sqlNullStr(input.SnmpPrivKey), now, now)
	return err
}

// Update 更新 SNMP 凭证
func (r *SnmpCredentialRepo) Update(id string, input SnmpCredentialCreateInput) error {
	db := appdb.DB()
	ensureSnmpCredentialsSchema(db)

	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`
		UPDATE snmp_credentials SET name = ?, community = ?, snmp_version = ?, snmp_port = ?,
			snmp_user = ?, snmp_auth_protocol = ?, snmp_auth_key = ?, snmp_priv_protocol = ?, snmp_priv_key = ?,
			updated_at = ?
		WHERE id = ?`,
		input.Name, sqlNullStr(input.Community), input.SnmpVersion, input.SnmpPort,
		sqlNullStr(input.SnmpUser), sqlNullStr(input.SnmpAuthProtocol), sqlNullStr(input.SnmpAuthKey),
		sqlNullStr(input.SnmpPrivProtocol), sqlNullStr(input.SnmpPrivKey), now, id)
	return err
}

// Delete 删除 SNMP 凭证
func (r *SnmpCredentialRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM snmp_credentials WHERE id = ?`, id)
	return err
}

// ───────────────────────────────────────────────
// SNMP Trap 事件
// ───────────────────────────────────────────────

// SnmpTrapEvent Trap 事件记录
type SnmpTrapEvent struct {
	ID            string `json:"id"`
	SourceIP      string `json:"source_ip"`
	TrapType      string `json:"trap_type"`
	EnterpriseOID string `json:"enterprise_oid"`
	AgentAddress  string `json:"agent_address"`
	GenericType   int    `json:"generic_type"`
	SpecificType  int    `json:"specific_type"`
	VarbindsJSON  string `json:"varbinds_json"`
	CreatedAt     string `json:"created_at"`
}

// TrapEventsRepo Trap 事件仓库
type TrapEventsRepo struct{}

var SnmpTraps = &TrapEventsRepo{}

// List 列出 Trap 事件
func (r *TrapEventsRepo) List(limit int, sourceIP string) ([]SnmpTrapEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, source_ip, trap_type, enterprise_oid, agent_address, generic_type, specific_type, varbinds_json, created_at FROM snmp_trap_events`
	var args []interface{}
	if sourceIP != "" {
		query += " WHERE source_ip = ?"
		args = append(args, sourceIP)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SnmpTrapEvent
	for rows.Next() {
		var e SnmpTrapEvent
		if err := rows.Scan(&e.ID, &e.SourceIP, &e.TrapType, &e.EnterpriseOID, &e.AgentAddress,
			&e.GenericType, &e.SpecificType, &e.VarbindsJSON, &e.CreatedAt); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Insert 插入 Trap 事件
func (r *TrapEventsRepo) Insert(e SnmpTrapEvent) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO snmp_trap_events (id, source_ip, trap_type, enterprise_oid, agent_address, generic_type, specific_type, varbinds_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.SourceIP, e.TrapType, e.EnterpriseOID, e.AgentAddress,
		e.GenericType, e.SpecificType, e.VarbindsJSON, now)
	return err
}

// ───────────────────────────────────────────────
// 巡检历史
// ───────────────────────────────────────────────

// InspectionHistory 巡检历史记录
type InspectionHistory struct {
	ID               string  `json:"id"`
	DeviceID         string  `json:"device_id"`
	InspectionType   string  `json:"inspection_type"`
	Status           string  `json:"status"`
	CommandsExecuted int     `json:"commands_executed"`
	CommandsFailed   int     `json:"commands_failed"`
	Results          *string `json:"results"`
	Summary          *string `json:"summary"`
	DurationMs       int     `json:"duration_ms"`
	DeviceType       *string `json:"device_type"`
	CreatedAt        string  `json:"created_at"`
	DeviceName       *string `json:"device_name,omitempty"` // JOIN
}

// InspectionRepo 巡检历史仓库
type InspectionRepo struct{}

var Inspections = &InspectionRepo{}

// ListAll 列出所有巡检记录（带过滤）
func (r *InspectionRepo) ListAll(limit int, inspType, status string) ([]InspectionHistory, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	query := `SELECT h.id, h.device_id, h.inspection_type, h.status, h.commands_executed, h.commands_failed,
		h.results, h.summary, h.duration_ms, h.device_type, h.created_at, nd.name
		FROM network_inspection_history h
		LEFT JOIN network_devices nd ON h.device_id = nd.id WHERE 1=1`
	var args []interface{}
	if inspType != "" {
		query += " AND h.inspection_type = ?"
		args = append(args, inspType)
	}
	if status != "" {
		query += " AND h.status = ?"
		args = append(args, status)
	}
	query += " ORDER BY h.created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInspections(rows)
}

// ListByDeviceID 列出设备的巡检记录
func (r *InspectionRepo) ListByDeviceID(deviceID string, limit int) ([]InspectionHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := appdb.DB().Query(`
		SELECT h.id, h.device_id, h.inspection_type, h.status, h.commands_executed, h.commands_failed,
			h.results, h.summary, h.duration_ms, h.device_type, h.created_at, nd.name
		FROM network_inspection_history h
		LEFT JOIN network_devices nd ON h.device_id = nd.id
		WHERE h.device_id = ? ORDER BY h.created_at DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInspections(rows)
}

// GetByID 获取单条巡检记录
func (r *InspectionRepo) GetByID(id string) (*InspectionHistory, error) {
	row := appdb.DB().QueryRow(`
		SELECT h.id, h.device_id, h.inspection_type, h.status, h.commands_executed, h.commands_failed,
			h.results, h.summary, h.duration_ms, h.device_type, h.created_at, nd.name
		FROM network_inspection_history h
		LEFT JOIN network_devices nd ON h.device_id = nd.id
		WHERE h.id = ?`, id)
	var h InspectionHistory
	var results, summary, deviceType, deviceName sql.NullString
	err := row.Scan(&h.ID, &h.DeviceID, &h.InspectionType, &h.Status, &h.CommandsExecuted, &h.CommandsFailed,
		&results, &summary, &h.DurationMs, &deviceType, &h.CreatedAt, &deviceName)
	if err != nil {
		return nil, err
	}
	h.Results = ns(results)
	h.Summary = ns(summary)
	h.DeviceType = ns(deviceType)
	h.DeviceName = ns(deviceName)
	return &h, nil
}

// CreateRunning 创建一个 running 状态的巡检记录
func (r *InspectionRepo) CreateRunning(deviceID, inspType, deviceType string) (string, error) {
	id := uuid.NewString()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO network_inspection_history (id, device_id, inspection_type, status, commands_executed, commands_failed, device_type, created_at)
		VALUES (?, ?, ?, 'running', 0, 0, ?, ?)`,
		id, deviceID, inspType, sqlNullStr(deviceType), now)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateResult 更新巡检结果
func (r *InspectionRepo) UpdateResult(id, status string, commandsExecuted, commandsFailed int, results, summary string, durationMs int) error {
	_, err := appdb.DB().Exec(`
		UPDATE network_inspection_history SET status = ?, commands_executed = ?, commands_failed = ?, results = ?, summary = ?, duration_ms = ?
		WHERE id = ?`,
		status, commandsExecuted, commandsFailed, sqlNullStr(results), sqlNullStr(summary), durationMs, id)
	return err
}

func scanInspections(rows *sql.Rows) ([]InspectionHistory, error) {
	var out []InspectionHistory
	for rows.Next() {
		var h InspectionHistory
		var results, summary, deviceType, deviceName sql.NullString
		if err := rows.Scan(&h.ID, &h.DeviceID, &h.InspectionType, &h.Status, &h.CommandsExecuted, &h.CommandsFailed,
			&results, &summary, &h.DurationMs, &deviceType, &h.CreatedAt, &deviceName); err != nil {
			continue
		}
		h.Results = ns(results)
		h.Summary = ns(summary)
		h.DeviceType = ns(deviceType)
		h.DeviceName = ns(deviceName)
		out = append(out, h)
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 接口指标
// ───────────────────────────────────────────────

// InterfaceMetric SNMP 接口指标
type InterfaceMetric struct {
	ID            int64   `json:"id"`
	DeviceID      string  `json:"device_id"`
	InterfaceName string  `json:"interface_name"`
	IfDescription *string `json:"if_description"`
	IfIndex       int     `json:"if_index"`
	IfSpeed       int64   `json:"if_speed"`
	IfAdminStatus int     `json:"if_admin_status"`
	IfOperStatus  int     `json:"if_oper_status"`
	IfInOctets    int64   `json:"if_in_octets"`
	IfOutOctets   int64   `json:"if_out_octets"`
	IfInErrors    int64   `json:"if_in_errors"`
	IfOutErrors   int64   `json:"if_out_errors"`
	SampledAt     string  `json:"sampled_at"`
}

// InterfaceMetricsRepo 接口指标仓库
type InterfaceMetricsRepo struct{}

var SnmpInterfaceMetrics = &InterfaceMetricsRepo{}

// ListByDeviceID 列出设备接口指标
func (r *InterfaceMetricsRepo) ListByDeviceID(deviceID string, limit int) ([]InterfaceMetric, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := appdb.DB().Query(`
		SELECT id, device_id, interface_name, if_description, if_index, if_speed,
			if_admin_status, if_oper_status, if_in_octets, if_out_octets, if_in_errors, if_out_errors, sampled_at
		FROM snmp_interface_metrics WHERE device_id = ? ORDER BY sampled_at DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InterfaceMetric
	for rows.Next() {
		var m InterfaceMetric
		var ifDesc sql.NullString
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.InterfaceName, &ifDesc, &m.IfIndex, &m.IfSpeed,
			&m.IfAdminStatus, &m.IfOperStatus, &m.IfInOctets, &m.IfOutOctets, &m.IfInErrors, &m.IfOutErrors, &m.SampledAt); err != nil {
			continue
		}
		m.IfDescription = ns(ifDesc)
		out = append(out, m)
	}
	return out, nil
}

// GetLatestByDeviceID 获取设备每个接口最近一次的指标记录（用于速率计算）
func (r *InterfaceMetricsRepo) GetLatestByDeviceID(deviceID string) (map[int]InterfaceMetric, error) {
	rows, err := appdb.DB().Query(`
		SELECT m.id, m.device_id, m.interface_name, m.if_description, m.if_index, m.if_speed,
			m.if_admin_status, m.if_oper_status, m.if_in_octets, m.if_out_octets, m.if_in_errors, m.if_out_errors, m.sampled_at
		FROM snmp_interface_metrics m
		WHERE m.device_id = ?
		  AND m.sampled_at = (
		    SELECT MAX(m2.sampled_at) FROM snmp_interface_metrics m2
		    WHERE m2.device_id = m.device_id AND m2.if_index = m.if_index
		  )`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]InterfaceMetric)
	for rows.Next() {
		var m InterfaceMetric
		var ifDesc sql.NullString
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.InterfaceName, &ifDesc, &m.IfIndex, &m.IfSpeed,
			&m.IfAdminStatus, &m.IfOperStatus, &m.IfInOctets, &m.IfOutOctets, &m.IfInErrors, &m.IfOutErrors, &m.SampledAt); err != nil {
			continue
		}
		m.IfDescription = ns(ifDesc)
		result[m.IfIndex] = m
	}
	return result, nil
}

// InsertBatch 批量插入接口指标
func (r *InterfaceMetricsRepo) InsertBatch(deviceID string, metrics []InterfaceMetric) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Format("2006-01-02 15:04:05")
	stmt, err := tx.Prepare(`
		INSERT INTO snmp_interface_metrics (device_id, interface_name, if_description, if_index, if_speed,
			if_admin_status, if_oper_status, if_in_octets, if_out_octets, if_in_errors, if_out_errors, sampled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, m := range metrics {
		var ifDesc interface{}
		if m.IfDescription != nil {
			ifDesc = *m.IfDescription
		}
		_, err := stmt.Exec(deviceID, m.InterfaceName, ifDesc, m.IfIndex, m.IfSpeed,
			m.IfAdminStatus, m.IfOperStatus, m.IfInOctets, m.IfOutOctets, m.IfInErrors, m.IfOutErrors, now)
		if err != nil {
			return fmt.Errorf("insert interface metric: %w", err)
		}
	}
	return tx.Commit()
}

// ───────────────────────────────────────────────
// 自定义 OID 配置
// ───────────────────────────────────────────────

// CustomOidProfile 自定义 OID 配置
type CustomOidProfile struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Vendor              *string `json:"vendor"`
	Description         *string `json:"description"`
	OidsJSON            string  `json:"oids_json"`
	PollIntervalSeconds int     `json:"poll_interval_seconds"`
	Enabled             int     `json:"enabled"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

// CustomOidResult 自定义 OID 采集结果
type CustomOidResult struct {
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	ProfileID   string `json:"profile_id"`
	MetricName  string `json:"metric_name"`
	MetricValue string `json:"metric_value"`
	OID         string `json:"oid"`
	SampledAt   string `json:"sampled_at"`
}

// CustomOidRepo 自定义 OID 仓库
type CustomOidRepo struct{}

var SnmpCustomOids = &CustomOidRepo{}

// List 列出 OID 配置
func (r *CustomOidRepo) List(enabledOnly bool) ([]CustomOidProfile, error) {
	query := `SELECT id, name, vendor, description, oids_json, poll_interval_seconds, enabled, created_at, updated_at FROM snmp_custom_oid_profiles`
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY created_at DESC"
	rows, err := appdb.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomOidProfile
	for rows.Next() {
		var p CustomOidProfile
		var vendor, desc sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &vendor, &desc, &p.OidsJSON, &p.PollIntervalSeconds, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		p.Vendor = ns(vendor)
		p.Description = ns(desc)
		out = append(out, p)
	}
	return out, nil
}

// GetByID 获取单个配置
func (r *CustomOidRepo) GetByID(id string) (*CustomOidProfile, error) {
	row := appdb.DB().QueryRow(`SELECT id, name, vendor, description, oids_json, poll_interval_seconds, enabled, created_at, updated_at FROM snmp_custom_oid_profiles WHERE id = ?`, id)
	var p CustomOidProfile
	var vendor, desc sql.NullString
	err := row.Scan(&p.ID, &p.Name, &vendor, &desc, &p.OidsJSON, &p.PollIntervalSeconds, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Vendor = ns(vendor)
	p.Description = ns(desc)
	return &p, nil
}

// Create 创建配置
func (r *CustomOidRepo) Create(input CustomOidProfile) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.PollIntervalSeconds == 0 {
		input.PollIntervalSeconds = 300
	}
	if input.Enabled == 0 {
		input.Enabled = 1
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO snmp_custom_oid_profiles (id, name, vendor, description, oids_json, poll_interval_seconds, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, sqlNullStr(ptrStr(input.Vendor)), sqlNullStr(ptrStr(input.Description)),
		input.OidsJSON, input.PollIntervalSeconds, input.Enabled, now, now)
	return err
}

// Update 更新配置
func (r *CustomOidRepo) Update(id string, input CustomOidProfile) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		UPDATE snmp_custom_oid_profiles SET name = ?, vendor = ?, description = ?, oids_json = ?, poll_interval_seconds = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		input.Name, sqlNullStr(ptrStr(input.Vendor)), sqlNullStr(ptrStr(input.Description)),
		input.OidsJSON, input.PollIntervalSeconds, input.Enabled, now, id)
	return err
}

// Delete 删除配置
func (r *CustomOidRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM snmp_custom_oid_profiles WHERE id = ?`, id)
	return err
}

// InsertResults 插入采集结果
func (r *CustomOidRepo) InsertResults(deviceID, profileID string, results []CustomOidResult) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, res := range results {
		id := uuid.NewString()
		_, err := tx.Exec(`
			INSERT INTO snmp_custom_oid_results (id, device_id, profile_id, metric_name, metric_value, oid, sampled_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, deviceID, profileID, res.MetricName, res.MetricValue, res.OID, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListResults 查询采集结果
func (r *CustomOidRepo) ListResults(deviceID string, profileID string, limit int) ([]CustomOidResult, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT id, device_id, profile_id, metric_name, metric_value, oid, sampled_at FROM snmp_custom_oid_results WHERE device_id = ?`
	var args []interface{}
	args = append(args, deviceID)
	if profileID != "" {
		query += " AND profile_id = ?"
		args = append(args, profileID)
	}
	query += " ORDER BY sampled_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomOidResult
	for rows.Next() {
		var res CustomOidResult
		if err := rows.Scan(&res.ID, &res.DeviceID, &res.ProfileID, &res.MetricName, &res.MetricValue, &res.OID, &res.SampledAt); err != nil {
			continue
		}
		out = append(out, res)
	}
	return out, nil
}

// ptrStr 安全获取指针值
func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
