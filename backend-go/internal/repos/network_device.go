// 网络设备核心 CRUD，等价于 TS networkDeviceRepository/core.ts
package repos

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// 网络设备
// ───────────────────────────────────────────────

// NetworkDevice network_devices 表完整记录
type NetworkDevice struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	IPAddress             string  `json:"ip_address"`
	Vendor                string  `json:"vendor"`
	Model                 *string `json:"model"`
	OsVersion             *string `json:"os_version"`
	SSHPort               int     `json:"ssh_port"`
	SSHKeyID              *string `json:"ssh_key_id"`
	Username              *string `json:"username"`
	Password              *string `json:"password,omitempty"`
	EnablePassword        *string `json:"enable_password,omitempty"`
	Location              *string `json:"location"`
	Role                  *string `json:"role"`
	Status                string  `json:"status"`
	LastInspectionAt      *string `json:"last_inspection_at"`
	LastInspectionResult  *string `json:"last_inspection_result"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
	DeviceType            *string `json:"device_type"`
	LastBackupAt          *string `json:"last_backup_at"`
	DeviceRole            *string `json:"device_role"`
	SnmpEnabled           int     `json:"snmp_enabled"`
	LastSnmpAt            *string `json:"last_snmp_at"`
	SnmpPort              int     `json:"snmp_port"`
	SnmpCredentialID      *string `json:"snmp_credential_id"`
	SystemPollInterval    int     `json:"system_poll_interval"`
	InterfacePollInterval int     `json:"interface_poll_interval"`
	SnmpCredentialName    *string `json:"snmp_credential_name,omitempty"` // JOIN 字段
}

// NetworkDeviceCreateInput 创建设备输入
type NetworkDeviceCreateInput struct {
	ID                    string
	Name                  string
	IPAddress             string
	Vendor                string
	Model                 string
	OsVersion             string
	SSHPort               int
	SSHKeyID              string
	Username              string
	Password              string
	EnablePassword        string
	Location              string
	Role                  string
	Status                string
	SnmpEnabled           int
	SnmpCredentialID      string
	SnmpPort              int
	SystemPollInterval    int
	InterfacePollInterval int
}

// NetworkDeviceUpdateInput 动态更新字段
type NetworkDeviceUpdateInput struct {
	Name                  *string
	IPAddress             *string
	Vendor                *string
	Model                 *string
	OsVersion             *string
	SSHPort               *int
	SSHKeyID              *string
	Username              *string
	Password              *string
	EnablePassword        *string
	Location              *string
	Role                  *string
	Status                *string
	DeviceType            *string
	DeviceRole            *string
	SnmpEnabled           *int
	SnmpCredentialID      *string
	SnmpPort              *int
	SystemPollInterval    *int
	InterfacePollInterval *int
}

// NetworkDeviceRepo 网络设备数据访问
type NetworkDeviceRepo struct{}

var NetworkDevices = &NetworkDeviceRepo{}

func ensureNetworkDevicesSchema(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS network_devices (
			id VARCHAR(36) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			ip_address VARCHAR(45) NOT NULL,
			vendor VARCHAR(100) DEFAULT '',
			model VARCHAR(100) DEFAULT '',
			os_version VARCHAR(100) DEFAULT '',
			ssh_port INT DEFAULT 22,
			ssh_key_id VARCHAR(36) DEFAULT '',
			username VARCHAR(255) DEFAULT '',
			password TEXT,
			enable_password TEXT,
			location VARCHAR(255) DEFAULT '',
			role VARCHAR(50) DEFAULT '',
			status VARCHAR(20) DEFAULT 'online',
			last_inspection_at VARCHAR(50) NULL,
			last_inspection_result TEXT,
			device_type VARCHAR(50) DEFAULT 'switch',
			last_backup_at VARCHAR(50) NULL,
			device_role VARCHAR(50) DEFAULT '',
			snmp_enabled INT DEFAULT 1,
			last_snmp_at VARCHAR(50) NULL,
			snmp_port INT DEFAULT 161,
			snmp_credential_id VARCHAR(36) DEFAULT '',
			system_poll_interval INT DEFAULT 300,
			interface_poll_interval INT DEFAULT 60,
			description TEXT,
			tags TEXT,
			created_at VARCHAR(50),
			updated_at VARCHAR(50)
		)
	`)

	alters := []string{
		"ALTER TABLE network_devices ADD COLUMN vendor VARCHAR(100) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN model VARCHAR(100) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN os_version VARCHAR(100) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN ssh_port INT DEFAULT 22",
		"ALTER TABLE network_devices ADD COLUMN ssh_key_id VARCHAR(36) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN username VARCHAR(255) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN password TEXT",
		"ALTER TABLE network_devices ADD COLUMN enable_password TEXT",
		"ALTER TABLE network_devices ADD COLUMN location VARCHAR(255) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN role VARCHAR(50) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN status VARCHAR(20) DEFAULT 'online'",
		"ALTER TABLE network_devices ADD COLUMN last_inspection_at VARCHAR(50) NULL",
		"ALTER TABLE network_devices ADD COLUMN last_inspection_result TEXT",
		"ALTER TABLE network_devices ADD COLUMN device_type VARCHAR(50) DEFAULT 'switch'",
		"ALTER TABLE network_devices ADD COLUMN last_backup_at VARCHAR(50) NULL",
		"ALTER TABLE network_devices ADD COLUMN device_role VARCHAR(50) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN snmp_enabled INT DEFAULT 1",
		"ALTER TABLE network_devices ADD COLUMN last_snmp_at VARCHAR(50) NULL",
		"ALTER TABLE network_devices ADD COLUMN snmp_port INT DEFAULT 161",
		"ALTER TABLE network_devices ADD COLUMN snmp_credential_id VARCHAR(36) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN system_poll_interval INT DEFAULT 300",
		"ALTER TABLE network_devices ADD COLUMN interface_poll_interval INT DEFAULT 60",
		"ALTER TABLE network_devices ADD COLUMN description TEXT",
		"ALTER TABLE network_devices ADD COLUMN tags TEXT",
		"ALTER TABLE network_devices ADD COLUMN created_at VARCHAR(50)",
		"ALTER TABLE network_devices ADD COLUMN updated_at VARCHAR(50)",
	}
	for _, a := range alters {
		_, _ = db.Exec(a)
	}
}

// scanDevice 从行扫描完整设备记录（带 JOIN 的 snmp_credential_name）
func scanDeviceWithCred(rows *sql.Rows) (*NetworkDevice, error) {
	var d NetworkDevice
	var model, osVer, sshKeyID, username, password, enablePw, location, role sql.NullString
	var lastInspect, lastInspectResult, lastBackup, lastSnmp, deviceType, deviceRole sql.NullString
	var snmpCredID, snmpCredName sql.NullString
	var sysPoll, ifPoll sql.NullInt64

	if err := rows.Scan(
		&d.ID, &d.Name, &d.IPAddress, &d.Vendor,
		&model, &osVer, &d.SSHPort, &sshKeyID, &username, &password, &enablePw,
		&location, &role, &d.Status, &lastInspect, &lastInspectResult,
		&d.CreatedAt, &d.UpdatedAt, &deviceType, &lastBackup, &deviceRole,
		&d.SnmpEnabled, &lastSnmp, &d.SnmpPort, &snmpCredID, &sysPoll, &ifPoll, &snmpCredName,
	); err != nil {
		return nil, err
	}

	d.Model = nullStr(model)
	d.OsVersion = nullStr(osVer)
	d.SSHKeyID = nullStr(sshKeyID)
	d.Username = nullStr(username)
	d.Password = nullStr(password)
	d.EnablePassword = nullStr(enablePw)
	d.Location = nullStr(location)
	d.Role = nullStr(role)
	d.LastInspectionAt = nullStr(lastInspect)
	d.LastInspectionResult = nullStr(lastInspectResult)
	d.LastBackupAt = nullStr(lastBackup)
	d.LastSnmpAt = nullStr(lastSnmp)
	d.DeviceType = nullStr(deviceType)
	d.DeviceRole = nullStr(deviceRole)
	d.SnmpCredentialID = nullStr(snmpCredID)
	d.SnmpCredentialName = nullStr(snmpCredName)

	if sysPoll.Valid && sysPoll.Int64 > 0 {
		d.SystemPollInterval = int(sysPoll.Int64)
	} else {
		d.SystemPollInterval = 300
	}
	if ifPoll.Valid && ifPoll.Int64 > 0 {
		d.InterfacePollInterval = int(ifPoll.Int64)
	} else {
		d.InterfacePollInterval = 60
	}

	return &d, nil
}

// nullStr 将 sql.NullString 转为 *string
func nullStr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// ns2 非空字符串转指针
func ns2(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

const deviceSelectFields = `
	nd.id, nd.name, nd.ip_address, nd.vendor, nd.model, nd.os_version, nd.ssh_port,
	nd.ssh_key_id, nd.username, nd.password, nd.enable_password, nd.location, nd.role,
	nd.status, nd.last_inspection_at, nd.last_inspection_result, nd.created_at, nd.updated_at,
	nd.device_type, nd.last_backup_at, nd.device_role, nd.snmp_enabled, nd.last_snmp_at,
	nd.snmp_port, nd.snmp_credential_id, COALESCE(nd.system_poll_interval, 300), COALESCE(nd.interface_poll_interval, 60),
	sc.name AS snmp_credential_name`

// List 获取所有网络设备（含 SNMP 凭证名）
func (r *NetworkDeviceRepo) List() ([]*NetworkDevice, error) {
	db := appdb.DB()
	ensureNetworkDevicesSchema(db)

	rows, err := db.Query(`
		SELECT ` + deviceSelectFields + `
		FROM network_devices nd
		LEFT JOIN snmp_credentials sc ON nd.snmp_credential_id = sc.id
		ORDER BY nd.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*NetworkDevice
	for rows.Next() {
		d, err := scanDeviceWithCred(rows)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// ListSafe 获取所有网络设备并清除凭据字段（供 API 列表返回，避免密文外泄）
func (r *NetworkDeviceRepo) ListSafe() ([]*NetworkDevice, error) {
	devices, err := r.List()
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d == nil {
			continue
		}
		d.Password = nil
		d.EnablePassword = nil
	}
	return devices, nil
}

// ListSnmpEnabledBasic 列出 SNMP 启用的设备（仅 id/name/ip）
func (r *NetworkDeviceRepo) ListSnmpEnabledBasic() ([]struct {
	ID, Name, IPAddress string
}, error) {
	db := appdb.DB()
	ensureNetworkDevicesSchema(db)

	rows, err := db.Query(`SELECT id, name, ip_address FROM network_devices WHERE snmp_enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID, Name, IPAddress string
	}
	for rows.Next() {
		var item struct {
			ID, Name, IPAddress string
		}
		if err := rows.Scan(&item.ID, &item.Name, &item.IPAddress); err != nil {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// GetByID 获取单个设备
func (r *NetworkDeviceRepo) GetByID(id string) (*NetworkDevice, error) {
	db := appdb.DB()
	ensureNetworkDevicesSchema(db)

	rows, err := db.Query(`
		SELECT `+deviceSelectFields+`
		FROM network_devices nd
		LEFT JOIN snmp_credentials sc ON nd.snmp_credential_id = sc.id
		WHERE nd.id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanDeviceWithCred(rows)
	}
	return nil, nil
}

// GetByIDBasic 获取设备基础信息（仅 id/name/ip）
func (r *NetworkDeviceRepo) GetByIDBasic(id string) (string, string, string, error) {
	var name, ip string
	err := appdb.DB().QueryRow(`SELECT id, name, ip_address FROM network_devices WHERE id = ?`, id).Scan(&id, &name, &ip)
	return id, name, ip, err
}

// GetNameAndIP 获取设备名称和 IP
func (r *NetworkDeviceRepo) GetNameAndIP(id string) (string, string, error) {
	var name, ip string
	err := appdb.DB().QueryRow(`SELECT name, ip_address FROM network_devices WHERE id = ?`, id).Scan(&name, &ip)
	return name, ip, err
}

// GetVendor 获取设备厂商
func (r *NetworkDeviceRepo) GetVendor(id string) (string, error) {
	var vendor string
	err := appdb.DB().QueryRow(`SELECT vendor FROM network_devices WHERE id = ?`, id).Scan(&vendor)
	return vendor, err
}

// GetByIP 通过 IP 查找设备
func (r *NetworkDeviceRepo) GetByIP(ip string) (string, string, error) {
	var id, name string
	err := appdb.DB().QueryRow(`SELECT id, name FROM network_devices WHERE ip_address = ?`, ip).Scan(&id, &name)
	return id, name, err
}

// GetByNameOrIP 通过名称或 IP 查找设备（LLDP 拓扑关联用）
func (r *NetworkDeviceRepo) GetByNameOrIP(name, ip string) (string, string, error) {
	if ip != "" {
		var id, devName string
		err := appdb.DB().QueryRow(`SELECT id, name FROM network_devices WHERE ip_address = ?`, ip).Scan(&id, &devName)
		if err == nil {
			return id, devName, nil
		}
	}
	if name != "" {
		var id, devName string
		err := appdb.DB().QueryRow(`SELECT id, name FROM network_devices WHERE name = ?`, name).Scan(&id, &devName)
		if err == nil {
			return id, devName, nil
		}
	}
	return "", "", sql.ErrNoRows
}

// FullCredentials 设备完整 SSH 凭证
type FullCredentials struct {
	ID             string
	Name           string
	IPAddress      string
	Vendor         string
	SSHPort        int
	Username       string
	Password       string
	EnablePassword string
}

// GetFullCredentials 获取设备完整 SSH 凭证（密码已加密）
func (r *NetworkDeviceRepo) GetFullCredentials(id string) (*FullCredentials, error) {
	row := appdb.DB().QueryRow(`
		SELECT id, name, ip_address, vendor, ssh_port, username, password, enable_password
		FROM network_devices WHERE id = ?`, id)
	var c FullCredentials
	var username, password, enablePw sql.NullString
	if err := row.Scan(&c.ID, &c.Name, &c.IPAddress, &c.Vendor, &c.SSHPort, &username, &password, &enablePw); err != nil {
		return nil, err
	}
	c.Username = username.String
	c.Password = password.String
	c.EnablePassword = enablePw.String
	return &c, nil
}

// GetTestConnectionFields 获取测试连接字段
func (r *NetworkDeviceRepo) GetTestConnectionFields(id string) (string, string, int, string, string, error) {
	var name, ip string
	var port int
	var username, password sql.NullString
	err := appdb.DB().QueryRow(`SELECT id, name, ip_address, ssh_port, username, password FROM network_devices WHERE id = ?`, id).
		Scan(&id, &name, &ip, &port, &username, &password)
	return name, ip, port, username.String, password.String, err
}

// CountAll 设备总数
func (r *NetworkDeviceRepo) CountAll() int {
	var c int
	_ = appdb.DB().QueryRow(`SELECT COUNT(*) FROM network_devices`).Scan(&c)
	return c
}

// Create 创建网络设备
func (r *NetworkDeviceRepo) Create(input NetworkDeviceCreateInput) error {
	db := appdb.DB()
	ensureNetworkDevicesSchema(db)

	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Status == "" {
		input.Status = "online"
	}
	if input.SSHPort == 0 {
		input.SSHPort = 22
	}
	if input.SnmpPort == 0 {
		input.SnmpPort = 161
	}
	if input.SnmpEnabled == 0 {
		input.SnmpEnabled = 1
	}
	now := time.Now().Format("2006-01-02 15:04:05")

	_, err := db.Exec(`
		INSERT INTO network_devices (id, name, ip_address, vendor, model, os_version, ssh_port, ssh_key_id,
			username, password, enable_password, location, role, status, snmp_enabled, snmp_credential_id, snmp_port,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, input.IPAddress, input.Vendor,
		sqlNullStr(input.Model), sqlNullStr(input.OsVersion), input.SSHPort,
		sqlNullStr(input.SSHKeyID), sqlNullStr(input.Username), sqlNullStr(input.Password), sqlNullStr(input.EnablePassword),
		sqlNullStr(input.Location), sqlNullStr(input.Role), input.Status,
		input.SnmpEnabled, sqlNullStr(input.SnmpCredentialID), input.SnmpPort,
		now, now)
	if err != nil {
		return err
	}
	return nil
}

// Update 更新设备（部分更新）
func (r *NetworkDeviceRepo) Update(id string, input NetworkDeviceUpdateInput) error {
	db := appdb.DB()
	ensureNetworkDevicesSchema(db)

	var sets []string
	var args []interface{}

	if input.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *input.Name)
	}
	if input.IPAddress != nil {
		sets = append(sets, "ip_address = ?")
		args = append(args, *input.IPAddress)
	}
	if input.Vendor != nil {
		sets = append(sets, "vendor = ?")
		args = append(args, *input.Vendor)
	}
	if input.Model != nil {
		sets = append(sets, "model = ?")
		args = append(args, sqlNullStr(*input.Model))
	}
	if input.OsVersion != nil {
		sets = append(sets, "os_version = ?")
		args = append(args, sqlNullStr(*input.OsVersion))
	}
	if input.SSHPort != nil {
		sets = append(sets, "ssh_port = ?")
		args = append(args, *input.SSHPort)
	}
	if input.SSHKeyID != nil {
		sets = append(sets, "ssh_key_id = ?")
		args = append(args, sqlNullStr(*input.SSHKeyID))
	}
	if input.Username != nil {
		sets = append(sets, "username = ?")
		args = append(args, sqlNullStr(*input.Username))
	}
	if input.Password != nil {
		sets = append(sets, "password = ?")
		args = append(args, sqlNullStr(*input.Password))
	}
	if input.EnablePassword != nil {
		sets = append(sets, "enable_password = ?")
		args = append(args, sqlNullStr(*input.EnablePassword))
	}
	if input.Location != nil {
		sets = append(sets, "location = ?")
		args = append(args, sqlNullStr(*input.Location))
	}
	if input.Role != nil {
		sets = append(sets, "role = ?")
		args = append(args, sqlNullStr(*input.Role))
	}
	if input.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
	}
	if input.DeviceType != nil {
		sets = append(sets, "device_type = ?")
		args = append(args, sqlNullStr(*input.DeviceType))
	}
	if input.DeviceRole != nil {
		sets = append(sets, "device_role = ?")
		args = append(args, sqlNullStr(*input.DeviceRole))
	}
	if input.SnmpEnabled != nil {
		sets = append(sets, "snmp_enabled = ?")
		args = append(args, *input.SnmpEnabled)
	}
	if input.SnmpCredentialID != nil {
		sets = append(sets, "snmp_credential_id = ?")
		args = append(args, sqlNullStr(*input.SnmpCredentialID))
	}
	if input.SnmpPort != nil {
		sets = append(sets, "snmp_port = ?")
		args = append(args, *input.SnmpPort)
	}

	if len(sets) == 0 {
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	sets = append(sets, "updated_at = ?")
	args = append(args, now, id)

	query := "UPDATE network_devices SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	_, err := db.Exec(query, args...)
	return err
}

// UpdateInspectionResult 更新设备巡检结果
func (r *NetworkDeviceRepo) UpdateInspectionResult(id, summary string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_devices SET last_inspection_at = ?, last_inspection_result = ?, updated_at = ? WHERE id = ?`, now, summary, now, id)
	return err
}

// UpdateLastBackupAt 更新设备最后备份时间
func (r *NetworkDeviceRepo) UpdateLastBackupAt(id string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_devices SET last_backup_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}

// UpdateLastSnmpAt 更新设备最后 SNMP 时间
func (r *NetworkDeviceRepo) UpdateLastSnmpAt(id string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_devices SET last_snmp_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}

// 设备删除时清理的关联表（app.db 内，均含 device_id 列）
// 注意：remediation_executions 只关联 alert_id/policy_id，属于告警处置审计，
// 与 alerts 一样保留不删。
var deviceCleanupAppTables = []string{
	"config_backup_records",
	"alert_device_associations",
	"alert_auto_analysis",
	"snmp_credentials",
	"network_config_backups",
	"network_inspection_history",
	"network_lldp_neighbors",
	"probe_execution_stats",
	"ai_remediations",
	"aars_response_logs",
	"dc_device_lifecycle",
}

// 指标类关联表（metrics.db 内；app.db 可能残留拆分前的旧副本，一并清理）
var deviceCleanupMetricsTables = []string{
	"snmp_custom_oid_results",
	"custom_dashboard_metrics",
	"snmp_polling_snapshots",
	"snmp_interface_metrics",
	"snmp_collect_results",
	"baseline_metrics",
	"snmp_polling_tasks",
	"snmp_metric_samples",
}

// execDeviceCleanup 在两个库上执行清理；表不存在视为无需清理
func execDeviceCleanup(db *sql.DB, query, id string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(query, id)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

func cleanupDeviceMetrics(id string) error {
	for _, t := range deviceCleanupMetricsTables {
		q := `DELETE FROM ` + t + ` WHERE device_id = ?`
		if err := execDeviceCleanup(appdb.DBForTable(t), q, id); err != nil {
			return fmt.Errorf("清理 %s 失败: %w", t, err)
		}
		if appdb.DBForTable(t) != appdb.DB() {
			if err := execDeviceCleanup(appdb.DB(), q, id); err != nil {
				return fmt.Errorf("清理 %s 失败: %w", t, err)
			}
		}
	}
	return nil
}

// cleanupDeviceAppTx 在 app.db 事务内清理关联数据并删除设备本体，返回是否真实删除了设备行
func cleanupDeviceAppTx(tx *sql.Tx, id string) (bool, error) {
	for _, t := range deviceCleanupAppTables {
		if _, err := tx.Exec(`DELETE FROM `+t+` WHERE device_id = ?`, id); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return false, fmt.Errorf("清理 %s 失败: %w", t, err)
		}
	}
	// 释放机架 U 位（槽位行随设备删除）
	if _, err := tx.Exec(`DELETE FROM dc_rack_slots WHERE device_id = ? AND device_type = 'network_device'`, id); err != nil && !strings.Contains(err.Error(), "no such table") {
		return false, fmt.Errorf("释放机架槽位失败: %w", err)
	}
	// 释放占用的 IP（保留 reserved 预留）
	if _, err := tx.Exec(`UPDATE network_ips SET device_id = NULL, device_name = NULL, status = 'available', updated_at = datetime('now','localtime') WHERE device_id = ? AND status = 'used'`, id); err != nil && !strings.Contains(err.Error(), "no such table") {
		return false, fmt.Errorf("释放 IP 地址失败: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM network_devices WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// Delete 删除设备（含关联数据清理，app.db 侧事务化）
func (r *NetworkDeviceRepo) Delete(id string) error {
	if err := cleanupDeviceMetrics(id); err != nil {
		return err
	}
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := cleanupDeviceAppTx(tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// BatchDelete 批量删除设备
func (r *NetworkDeviceRepo) BatchDelete(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	for _, id := range ids {
		if err := cleanupDeviceMetrics(id); err != nil {
			return 0, err
		}
	}
	tx, err := appdb.DB().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	deletedCount := 0
	for _, id := range ids {
		deleted, err := cleanupDeviceAppTx(tx, id)
		if err != nil {
			return 0, err
		}
		if deleted {
			deletedCount++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deletedCount, nil
}

// CreateFromDiscovery 从发现结果创建设备
func (r *NetworkDeviceRepo) CreateFromDiscovery(name, ip, vendor, model, username string, sshPort int, status, osVersion string) error {
	id := uuid.NewString()
	if sshPort == 0 {
		sshPort = 22
	}
	if status == "" {
		status = "online"
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO network_devices (id, name, ip_address, vendor, model, username, ssh_port, status, os_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, ip, vendor, sqlNullStr(model), sqlNullStr(username), sshPort, status, sqlNullStr(osVersion), now, now)
	return err
}

// UpdatePollInterval 单设备更新采集间隔
func (r *NetworkDeviceRepo) UpdatePollInterval(id string, systemInterval, interfaceInterval int) error {
	if systemInterval <= 0 {
		systemInterval = 300
	}
	if interfaceInterval <= 0 {
		interfaceInterval = 60
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		UPDATE network_devices 
		SET system_poll_interval = ?, interface_poll_interval = ?, updated_at = ?
		WHERE id = ?`, systemInterval, interfaceInterval, now, id)
	return err
}

// BatchUpdatePollIntervals 批量按品牌/设备列表更新采集间隔
func (r *NetworkDeviceRepo) BatchUpdatePollIntervals(vendor string, deviceIDs []string, systemInterval, interfaceInterval int) (int64, error) {
	if systemInterval <= 0 {
		systemInterval = 300
	}
	if interfaceInterval <= 0 {
		interfaceInterval = 60
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	var res sql.Result
	var err error
	if len(deviceIDs) > 0 {
		query := `UPDATE network_devices SET system_poll_interval = ?, interface_poll_interval = ?, updated_at = ? WHERE id IN (`
		args := []interface{}{systemInterval, interfaceInterval, now}
		for i, id := range deviceIDs {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, id)
		}
		query += ")"
		res, err = appdb.DB().Exec(query, args...)
	} else if vendor != "" && vendor != "all" {
		res, err = appdb.DB().Exec(`
			UPDATE network_devices 
			SET system_poll_interval = ?, interface_poll_interval = ?, updated_at = ?
			WHERE LOWER(vendor) = LOWER(?)`, systemInterval, interfaceInterval, now, vendor)
	} else {
		res, err = appdb.DB().Exec(`
			UPDATE network_devices 
			SET system_poll_interval = ?, interface_poll_interval = ?, updated_at = ?`, systemInterval, interfaceInterval, now)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ExistsByIP 检查 IP 是否已存在
func (r *NetworkDeviceRepo) ExistsByIP(ip string) bool {
	var c int
	_ = appdb.DB().QueryRow(`SELECT COUNT(*) FROM network_devices WHERE ip_address = ?`, ip).Scan(&c)
	return c > 0
}

// sqlNullStr 将字符串转为 sql.NullString
func sqlNullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
