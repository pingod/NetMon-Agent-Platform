// 服务器数据访问层 — 对应 TS serverRepository.ts / serverGroupRepository.ts
// 数据库表：servers / server_groups / server_group_mapping / server_command_history / compliance_checks
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/crypto"
)

// Server 服务器实体（对应前端 Server 类型）
type Server struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Hostname           string           `json:"hostname"`
	Port               int              `json:"port"`
	Username           string           `json:"username"`
	Password           *string          `json:"password,omitempty"`    // 加密密文（列表时不返回）
	PrivateKey         *string          `json:"private_key,omitempty"` // 加密密文（列表时不返回）
	UseSSHKey          int              `json:"use_ssh_key"`
	Description        *string          `json:"description,omitempty"`
	Tags               *string          `json:"tags,omitempty"` // 逗号分隔存储（DB），序列化时转为数组
	Enabled            int              `json:"enabled"`
	LastConnected      *string          `json:"last_connected,omitempty"`
	OS                 *string          `json:"os,omitempty"`
	OSType             *string          `json:"os_type,omitempty"`
	CPUCores           *int             `json:"cpu_cores,omitempty"`
	MemoryGB           *float64         `json:"memory_gb,omitempty"`
	DiskGB             *float64         `json:"disk_gb,omitempty"`
	IPAddress          *string          `json:"ip_address,omitempty"`
	PrivateIP          *string          `json:"private_ip,omitempty"`
	CloudProvider      *string          `json:"cloud_provider,omitempty"`
	VncPort            *int             `json:"vnc_port,omitempty"`
	VncPassword        *string          `json:"vnc_password,omitempty"`
	SSHKeyID           *string          `json:"ssh_key_id,omitempty"`
	Vendor             *string          `json:"vendor,omitempty"`
	Model              *string          `json:"model,omitempty"`
	IsVirtualMachine   int              `json:"is_virtual_machine"`
	AgentInstalled     int              `json:"agent_installed"`
	AgentVersion       *string          `json:"agent_version,omitempty"`
	AgentStatus        *string          `json:"agent_status,omitempty"`
	AgentLastHeartbeat *string          `json:"agent_last_heartbeat,omitempty"`
	AgentCPUUsage      *float64         `json:"agent_cpu_usage,omitempty"`
	AgentMemMB         *float64         `json:"agent_mem_mb,omitempty"`
	AgentUptimeSeconds *int64           `json:"agent_uptime_seconds,omitempty"`
	Groups             []ServerGroupRef `json:"groups,omitempty"`
	CreatedAt          string           `json:"created_at"`
	UpdatedAt          string           `json:"updated_at"`
}

// MarshalJSON 自定义序列化：tags 逗号分隔字符串 → 数组
func (s Server) MarshalJSON() ([]byte, error) {
	type alias Server
	a := struct {
		alias
		Tags *[]string `json:"tags,omitempty"`
	}{alias: alias(s)}
	if s.Tags != nil && *s.Tags != "" {
		var tags []string
		for _, t := range strings.Split(*s.Tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		a.Tags = &tags
	}
	return json.Marshal(a)
}

// ServerGroupRef 服务器所属分组（轻量引用）
type ServerGroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ServerGroup 服务器分组
type ServerGroup struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   *string       `json:"description,omitempty"`
	ParentID      *string       `json:"parent_id,omitempty"`
	SortOrder     int           `json:"sort_order"`
	ServerCount   int           `json:"server_count,omitempty"`
	ChildrenCount int           `json:"children_count,omitempty"`
	Children      []ServerGroup `json:"children,omitempty"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
}

// CommandHistoryItem 命令历史
type CommandHistoryItem struct {
	ID              string `json:"id"`
	ServerID        string `json:"server_id"`
	Command         string `json:"command"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	Success         int    `json:"success"`
	ExecutionTimeMs int    `json:"execution_time_ms"`
	ExecutedBy      string `json:"executed_by"`
	ExecutedAt      string `json:"executed_at"`
}

// ComplianceCheck 合规检查记录
type ComplianceCheck struct {
	ID           string  `json:"id"`
	ServerID     string  `json:"server_id"`
	CheckName    string  `json:"check_name"`
	CheckResults string  `json:"check_results"`
	Status       string  `json:"status"`
	StartedAt    *string `json:"started_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

var Servers = &ServerRepo{}

type ServerRepo struct{}

// serverColumns 查询服务器列表的通用 SQL（含分组聚合）
const serverColumns = `
	SELECT s.id, s.name, s.hostname, s.port, s.username, s.use_ssh_key,
	       s.description, s.tags, s.enabled, s.last_connected, s.os, s.os_type,
	       s.cpu_cores, s.memory_gb, s.disk_gb, s.ip_address, s.private_ip,
	       s.cloud_provider, s.vnc_port, s.ssh_key_id, s.created_at, s.updated_at,
	       COALESCE(s.vendor, 'other'), COALESCE(s.model, ''),
	       COALESCE(s.is_virtual_machine, 0), COALESCE(s.agent_installed, 0),
	       COALESCE(s.agent_version, ''), COALESCE(s.agent_status, 'uninstalled'),
	       s.agent_last_heartbeat, COALESCE(s.agent_cpu_usage, 0),
	       COALESCE(s.agent_mem_mb, 0), COALESCE(s.agent_uptime_seconds, 0)
	FROM servers s`

type scannable interface {
	Scan(dest ...any) error
}

// scanServer 扫描一行服务器记录（不含敏感字段）
func scanServer(scanner scannable) (*Server, error) {
	var s Server
	var desc, tags, lastConn, os, osType, ip, privIP, cloud, sshKeyID, vendor, model sql.NullString
	var agentVer, agentStat, agentHeartbeat sql.NullString
	var vncPort, cpuCores, isVM, agentInst, agentUptime sql.NullInt64
	var memGB, diskGB, agentCPU, agentMem sql.NullFloat64
	err := scanner.Scan(&s.ID, &s.Name, &s.Hostname, &s.Port, &s.Username, &s.UseSSHKey,
		&desc, &tags, &s.Enabled, &lastConn, &os, &osType,
		&cpuCores, &memGB, &diskGB, &ip, &privIP, &cloud, &vncPort, &sshKeyID,
		&s.CreatedAt, &s.UpdatedAt, &vendor, &model,
		&isVM, &agentInst, &agentVer, &agentStat, &agentHeartbeat,
		&agentCPU, &agentMem, &agentUptime)
	if err != nil {
		return nil, err
	}
	s.Description = nullStrToPtr(desc)
	s.Tags = nullStrToPtr(tags)
	s.LastConnected = nullStrToPtr(lastConn)
	s.OS = nullStrToPtr(os)
	s.OSType = nullStrToPtr(osType)
	if cpuCores.Valid {
		v := int(cpuCores.Int64)
		s.CPUCores = &v
	}
	if memGB.Valid {
		s.MemoryGB = &memGB.Float64
	}
	if diskGB.Valid {
		s.DiskGB = &diskGB.Float64
	}
	s.IPAddress = nullStrToPtr(ip)
	s.PrivateIP = nullStrToPtr(privIP)
	s.CloudProvider = nullStrToPtr(cloud)
	if vncPort.Valid {
		v := int(vncPort.Int64)
		s.VncPort = &v
	}
	s.SSHKeyID = nullStrToPtr(sshKeyID)
	vStr := "other"
	if vendor.Valid && vendor.String != "" {
		vStr = vendor.String
	}
	s.Vendor = &vStr
	s.Model = nullStrToPtr(model)
	if isVM.Valid {
		s.IsVirtualMachine = int(isVM.Int64)
	}
	if agentInst.Valid {
		s.AgentInstalled = int(agentInst.Int64)
	}
	s.AgentVersion = nullStrToPtr(agentVer)
	s.AgentStatus = nullStrToPtr(agentStat)
	s.AgentLastHeartbeat = nullStrToPtr(agentHeartbeat)
	s.AgentCPUUsage = floatPtrFromNull(agentCPU)
	s.AgentMemMB = floatPtrFromNull(agentMem)
	s.AgentUptimeSeconds = intPtrFromNull(agentUptime)
	return &s, nil
}

// scanServerRow 扫描单行记录（QueryRow）
func scanServerRow(row *sql.Row) (*Server, error) {
	return scanServer(row)
}

// List 服务器列表（不含密码/私钥），附分组信息
func (ServerRepo) List() ([]Server, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(serverColumns + ` ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			continue
		}
		items = append(items, *s)
	}
	if items == nil {
		items = []Server{}
	}

	// 批量加载分组
	if len(items) > 0 {
		groupsByServer := Servers.loadGroupMappings()
		for i := range items {
			if gs, ok := groupsByServer[items[i].ID]; ok {
				items[i].Groups = gs
			} else {
				items[i].Groups = []ServerGroupRef{}
			}
		}
	}
	return items, nil
}

// loadGroupMappings 加载全部 服务器→分组 映射
func (ServerRepo) loadGroupMappings() map[string][]ServerGroupRef {
	db := appdb.DB()
	m := map[string][]ServerGroupRef{}
	rows, err := db.Query(`
		SELECT sgm.server_id, sg.id, sg.name
		FROM server_group_mapping sgm
		JOIN server_groups sg ON sg.id = sgm.group_id`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var serverID, gid, gname string
		if err := rows.Scan(&serverID, &gid, &gname); err != nil {
			continue
		}
		m[serverID] = append(m[serverID], ServerGroupRef{ID: gid, Name: gname})
	}
	return m
}

// GetByID 获取单台服务器（不含敏感字段），附分组
func (ServerRepo) GetByID(id string) (*Server, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	s, err := scanServerRow(db.QueryRow(serverColumns+` WHERE s.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	groups, _ := Servers.GetServerGroups(id)
	s.Groups = groups
	return s, nil
}

// GetCredentialFields 获取服务器的认证凭证（密码/私钥解密后）用于 SSH 连接
func (ServerRepo) GetCredentialFields(id string) (username string, password string, privateKey string, useSSHKey int, err error) {
	db := appdb.DB()
	if db == nil {
		return "", "", "", 0, fmt.Errorf("database not initialized")
	}
	var pwd, priv sql.NullString
	var useKey int
	err = db.QueryRow(`SELECT username, password, private_key, use_ssh_key FROM servers WHERE id = ?`, id).
		Scan(&username, &pwd, &priv, &useKey)
	if err == sql.ErrNoRows {
		return "", "", "", 0, fmt.Errorf("server not found")
	}
	if err != nil {
		return "", "", "", 0, err
	}
	if pwd.Valid && pwd.String != "" {
		if strings.HasPrefix(pwd.String, "2:") {
			dec, derr := crypto.Decrypt(pwd.String)
			if derr == nil {
				password = dec
			} else {
				return "", "", "", 0, fmt.Errorf("SSH 密码解密失败（服务端加密密钥已变更），请点击「编辑」重新输入密码保存")
			}
		} else {
			// 兼容明文密码（老数据）
			password = pwd.String
		}
	}
	if priv.Valid && priv.String != "" {
		if strings.HasPrefix(priv.String, "2:") {
			dec, derr := crypto.Decrypt(priv.String)
			if derr == nil {
				privateKey = dec
			} else {
				return "", "", "", 0, fmt.Errorf("SSH 私钥解密失败（服务端加密密钥已变更），请点击「编辑」重新上传私钥保存")
			}
		} else {
			privateKey = priv.String
		}
	}
	return username, password, privateKey, useKey, nil
}

// GetServerGroups 获取服务器所属分组
func (ServerRepo) GetServerGroups(serverID string) ([]ServerGroupRef, error) {
	db := appdb.DB()
	rows, err := db.Query(`
		SELECT sg.id, sg.name
		FROM server_group_mapping sgm
		JOIN server_groups sg ON sg.id = sgm.group_id
		WHERE sgm.server_id = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []ServerGroupRef
	for rows.Next() {
		var g ServerGroupRef
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			continue
		}
		groups = append(groups, g)
	}
	if groups == nil {
		groups = []ServerGroupRef{}
	}
	return groups, nil
}

// Create 创建服务器，返回新记录
func (ServerRepo) Create(req ServerCreateInput) (*Server, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	id := uuid.New().String()
	now := nowLocal()

	var pwd, priv interface{}
	if req.Password != "" {
		pwd = crypto.Encrypt(req.Password)
	}
	if req.PrivateKey != "" {
		priv = crypto.Encrypt(req.PrivateKey)
	}

	tagsStr := strings.Join(req.Tags, ",")
	vendorStr := strings.ToLower(strings.TrimSpace(req.Vendor))
	if vendorStr == "" {
		vendorStr = "other"
	}

	_, err := db.Exec(`
		INSERT INTO servers (id, name, hostname, port, username, password, private_key, use_ssh_key,
		                     description, tags, enabled, os_type, vnc_port, vnc_password, ssh_key_id,
		                     vendor, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Name, req.Hostname, req.Port, req.Username, pwd, priv, boolToInt(req.UseSSHKey),
		emptyToNull(req.Description), emptyToNull(tagsStr), 1, req.OSType,
		req.VncPort, emptyToNull(req.VncPassword), emptyToNull(req.SSHKeyID),
		vendorStr, emptyToNull(req.Model), now, now)
	if err != nil {
		return nil, err
	}

	// 关联分组
	if req.GroupID != "" {
		_, _ = db.Exec(`INSERT INTO server_group_mapping (server_id, group_id) VALUES (?, ?)`, id, req.GroupID)
	}
	return Servers.GetByID(id)
}

// Update 更新服务器，返回更新后记录
func (ServerRepo) Update(id string, req ServerUpdateInput) (*Server, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// 读取现有记录
	var current Server
	var pwd, priv, desc, tags, osType, vncPwd, sshKeyID, ip, privIP, cloud, vendor, model sql.NullString
	var vncPort sql.NullInt64
	var useKey, enabled int
	var port int
	err := db.QueryRow(`SELECT name, hostname, port, username, password, private_key, use_ssh_key,
		description, tags, enabled, os_type, vnc_port, vnc_password, ssh_key_id, ip_address, private_ip, cloud_provider,
		vendor, model
		FROM servers WHERE id = ?`, id).
		Scan(&current.Name, &current.Hostname, &port, &current.Username, &pwd, &priv, &useKey,
			&desc, &tags, &enabled, &osType, &vncPort, &vncPwd, &sshKeyID, &ip, &privIP, &cloud,
			&vendor, &model)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("server not found")
	}
	if err != nil {
		return nil, err
	}

	// 合并字段
	if req.Name != nil && *req.Name != "" {
		current.Name = *req.Name
	}
	if req.Hostname != nil && *req.Hostname != "" {
		current.Hostname = *req.Hostname
	}
	if req.Port != nil && *req.Port > 0 {
		port = *req.Port
	}
	if req.Username != nil && *req.Username != "" {
		current.Username = *req.Username
	}
	if req.Password != nil && *req.Password != "" {
		pwd = sql.NullString{String: crypto.Encrypt(*req.Password), Valid: true}
	}
	if req.PrivateKey != nil && *req.PrivateKey != "" {
		priv = sql.NullString{String: crypto.Encrypt(*req.PrivateKey), Valid: true}
	}
	if req.UseSSHKey != nil {
		useKey = boolToInt(*req.UseSSHKey)
	}
	if req.Description != nil {
		desc = sql.NullString{String: *req.Description, Valid: *req.Description != ""}
	}
	if req.Tags != nil {
		tagsStr := strings.Join(*req.Tags, ",")
		tags = sql.NullString{String: tagsStr, Valid: tagsStr != ""}
	}
	if req.OSType != nil && *req.OSType != "" {
		osType = sql.NullString{String: *req.OSType, Valid: true}
	}
	if req.VncPort != nil {
		vncPort = sql.NullInt64{Int64: int64(*req.VncPort), Valid: true}
	}
	if req.VncPassword != nil {
		vncPwd = sql.NullString{String: *req.VncPassword, Valid: *req.VncPassword != ""}
	}
	if req.SSHKeyID != nil {
		sshKeyID = sql.NullString{String: *req.SSHKeyID, Valid: *req.SSHKeyID != ""}
	}
	if req.IPAddress != nil {
		ip = sql.NullString{String: *req.IPAddress, Valid: *req.IPAddress != ""}
	}
	if req.PrivateIP != nil {
		privIP = sql.NullString{String: *req.PrivateIP, Valid: *req.PrivateIP != ""}
	}
	if req.CloudProvider != nil {
		cloud = sql.NullString{String: *req.CloudProvider, Valid: *req.CloudProvider != ""}
	}
	if req.Vendor != nil {
		vStr := strings.ToLower(strings.TrimSpace(*req.Vendor))
		if vStr == "" {
			vStr = "other"
		}
		vendor = sql.NullString{String: vStr, Valid: true}
	}
	if req.Model != nil {
		model = sql.NullString{String: *req.Model, Valid: *req.Model != ""}
	}

	now := nowLocal()
	_, err = db.Exec(`
		UPDATE servers SET name=?, hostname=?, port=?, username=?, password=?, private_key=?, use_ssh_key=?,
		                   description=?, tags=?, os_type=?, vnc_port=?, vnc_password=?, ssh_key_id=?,
		                   ip_address=?, private_ip=?, cloud_provider=?, vendor=?, model=?, updated_at=?
		WHERE id = ?`,
		current.Name, current.Hostname, port, current.Username, nullToAny(pwd), nullToAny(priv), useKey,
		nullToAny(desc), nullToAny(tags), nullToAny(osType), nullToAny(vncPort), nullToAny(vncPwd), nullToAny(sshKeyID),
		nullToAny(ip), nullToAny(privIP), nullToAny(cloud), nullToAny(vendor), nullToAny(model), now, id)
	if err != nil {
		return nil, err
	}

	// 更新分组关联
	if req.GroupID != nil {
		_, _ = db.Exec(`DELETE FROM server_group_mapping WHERE server_id = ?`, id)
		if *req.GroupID != "" {
			_, _ = db.Exec(`INSERT INTO server_group_mapping (server_id, group_id) VALUES (?, ?)`, id, *req.GroupID)
		}
	}

	// 更新采集信息（collect-info 回填）
	if req.OS != nil {
		_, _ = db.Exec(`UPDATE servers SET os=?, cpu_cores=?, memory_gb=?, disk_gb=?, os_detail=? WHERE id=?`,
			*req.OS, req.CPUCores, req.MemoryGB, req.DiskGB, req.OSDetail, id)
	}

	return Servers.GetByID(id)
}

// UpdateCollectedInfo 回填采集到的系统信息
func (ServerRepo) UpdateCollectedInfo(id string, os string, osDetail string, cpuCores int, memoryGB float64, diskGB float64, ipAddress string) error {
	db := appdb.DB()
	_, err := db.Exec(`
		UPDATE servers SET os=?, os_detail=?, cpu_cores=?, memory_gb=?, disk_gb=?, ip_address=?, updated_at=?
		WHERE id = ?`,
		emptyToNull(os), emptyToNull(osDetail), cpuCores, memoryGB, diskGB, emptyToNull(ipAddress), nowLocal(), id)
	return err
}

// UpdateLastConnected 更新最后连接时间
func (ServerRepo) UpdateLastConnected(id string) {
	db := appdb.DB()
	if db == nil {
		return
	}
	_, _ = db.Exec(`UPDATE servers SET last_connected=?, updated_at=? WHERE id=?`, nowLocal(), nowLocal(), id)
}

// Delete 删除服务器
func (ServerRepo) Delete(id string) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, _ = db.Exec(`DELETE FROM server_group_mapping WHERE server_id = ?`, id)
	_, _ = db.Exec(`DELETE FROM compliance_checks WHERE server_id = ?`, id)
	_, _ = db.Exec(`DELETE FROM server_commands WHERE server_id = ?`, id)
	_, err := db.Exec(`DELETE FROM servers WHERE id = ?`, id)
	return err
}

// BatchDelete 批量删除服务器
func (ServerRepo) BatchDelete(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	db := appdb.DB()
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	deletedCount := 0
	for _, id := range ids {
		_, _ = db.Exec(`DELETE FROM server_group_mapping WHERE server_id = ?`, id)
		_, _ = db.Exec(`DELETE FROM compliance_checks WHERE server_id = ?`, id)
		_, _ = db.Exec(`DELETE FROM server_commands WHERE server_id = ?`, id)
		res, err := db.Exec(`DELETE FROM servers WHERE id = ?`, id)
		if err == nil {
			if affected, _ := res.RowsAffected(); affected > 0 {
				deletedCount++
			}
		}
	}
	return deletedCount, nil
}

// Exists 检查服务器是否存在
func (ServerRepo) Exists(id string) (bool, error) {
	db := appdb.DB()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM servers WHERE id = ?", id).Scan(&count)
	return count > 0, err
}

// ExistsByHostname 检查主机名是否已存在（导入时去重）
func (ServerRepo) ExistsByHostname(hostname string) (bool, error) {
	db := appdb.DB()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM servers WHERE hostname = ?", strings.TrimSpace(hostname)).Scan(&count)
	return count > 0, err
}

// ListAllForCollect 列出所有启用服务器（批量采集用）
func (ServerRepo) ListAllForCollect() ([]Server, error) {
	return Servers.List()
}

// InsertCommandHistory 记录命令执行历史
func (ServerRepo) InsertCommandHistory(serverID, command, stdout, stderr string, success int, execMs int, executedBy string) error {
	db := appdb.DB()
	id := uuid.New().String()
	_, err := db.Exec(`
		INSERT INTO server_command_history (id, server_id, command, stdout, stderr, success, execution_time_ms, executed_by, executed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, serverID, command, stdout, stderr, success, execMs, executedBy, nowLocal())
	return err
}

// GetCommandHistory 查询命令历史
func (ServerRepo) GetCommandHistory(serverID string) ([]CommandHistoryItem, error) {
	db := appdb.DB()
	rows, err := db.Query(`
		SELECT id, server_id, command, stdout, stderr, success, execution_time_ms, executed_by, executed_at
		FROM server_command_history WHERE server_id = ? ORDER BY executed_at DESC LIMIT 100`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CommandHistoryItem
	for rows.Next() {
		var it CommandHistoryItem
		var stdout, stderr sql.NullString
		if err := rows.Scan(&it.ID, &it.ServerID, &it.Command, &stdout, &stderr, &it.Success, &it.ExecutionTimeMs, &it.ExecutedBy, &it.ExecutedAt); err != nil {
			continue
		}
		it.Stdout = stdout.String
		it.Stderr = stderr.String
		items = append(items, it)
	}
	if items == nil {
		items = []CommandHistoryItem{}
	}
	return items, nil
}

// InsertComplianceCheck 记录合规检查
func (ServerRepo) InsertComplianceCheck(serverID, checkName, checkResults, status string) error {
	db := appdb.DB()
	id := uuid.New().String()
	_, err := db.Exec(`
		INSERT INTO compliance_checks (id, server_id, check_name, check_results, status, started_at, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, serverID, checkName, checkResults, status, nowLocal(), nowLocal(), nowLocal())
	return err
}

// GetComplianceHistory 查询合规检查历史
func (ServerRepo) GetComplianceHistory(serverID string) ([]ComplianceCheck, error) {
	db := appdb.DB()
	rows, err := db.Query(`
		SELECT id, server_id, check_name, check_results, status, started_at, completed_at, created_at
		FROM compliance_checks WHERE server_id = ? ORDER BY created_at DESC LIMIT 50`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ComplianceCheck
	for rows.Next() {
		var it ComplianceCheck
		var started, completed sql.NullString
		if err := rows.Scan(&it.ID, &it.ServerID, &it.CheckName, &it.CheckResults, &it.Status, &started, &completed, &it.CreatedAt); err != nil {
			continue
		}
		it.StartedAt = nullStrToPtr(started)
		it.CompletedAt = nullStrToPtr(completed)
		items = append(items, it)
	}
	if items == nil {
		items = []ComplianceCheck{}
	}
	return items, nil
}

// InsertMetric 保存服务器性能指标
func (ServerRepo) InsertMetric(serverID string, metrics map[string]interface{}) error {
	db := appdb.DB()
	id := uuid.New().String()
	now := nowLocal()

	getF := func(k string) interface{} {
		if v, ok := metrics[k]; ok {
			return v
		}
		return nil
	}

	_, err := db.Exec(`
		INSERT INTO server_metrics (id, server_id, cpu_usage, memory_usage, memory_total_gb, memory_used_gb,
		                            disk_usage, disk_total_gb, disk_used_gb, network_in_mbps, network_out_mbps,
		                            load_1min, load_5min, load_15min, uptime_seconds, collected_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, serverID,
		getF("cpu_usage"), getF("memory_usage"), getF("memory_total_gb"), getF("memory_used_gb"),
		getF("disk_usage"), getF("disk_total_gb"), getF("disk_used_gb"),
		nil, nil,
		getF("load_1min"), getF("load_5min"), getF("load_15min"),
		getF("uptime_seconds"), now, now)
	return err
}

// ServerCreateInput 创建服务器请求
type ServerCreateInput struct {
	Name             string   `json:"name"`
	Hostname         string   `json:"hostname"`
	Port             int      `json:"port"`
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	PrivateKey       string   `json:"private_key"`
	UseSSHKey        bool     `json:"use_ssh_key"`
	Description      string   `json:"description"`
	Tags             []string `json:"tags"`
	OSType           string   `json:"os_type"`
	VncPort          int      `json:"vnc_port"`
	VncPassword      string   `json:"vnc_password"`
	SSHKeyID         string   `json:"ssh_key_id"`
	GroupID          string   `json:"group_id"`
	Vendor           string   `json:"vendor"`
	Model            string   `json:"model"`
	IsVirtualMachine int      `json:"is_virtual_machine"`
	IPAddress        string   `json:"ip_address"`
	PrivateIP        string   `json:"private_ip"`
	CloudProvider    string   `json:"cloud_provider"`
}

// ServerUpdateInput 更新服务器请求（指针区分"未传"和"清空"）
type ServerUpdateInput struct {
	Name             *string   `json:"name"`
	Hostname         *string   `json:"hostname"`
	Port             *int      `json:"port"`
	Username         *string   `json:"username"`
	Password         *string   `json:"password"`
	PrivateKey       *string   `json:"private_key"`
	UseSSHKey        *bool     `json:"use_ssh_key"`
	Description      *string   `json:"description"`
	Tags             *[]string `json:"tags"`
	OSType           *string   `json:"os_type"`
	VncPort          *int      `json:"vnc_port"`
	VncPassword      *string   `json:"vnc_password"`
	SSHKeyID         *string   `json:"ssh_key_id"`
	IPAddress        *string   `json:"ip_address"`
	PrivateIP        *string   `json:"private_ip"`
	CloudProvider    *string   `json:"cloud_provider"`
	GroupID          *string   `json:"group_id"`
	Vendor           *string   `json:"vendor"`
	Model            *string   `json:"model"`
	IsVirtualMachine *int      `json:"is_virtual_machine"`
	// 采集信息回填
	OS               *string   `json:"os"`
	CPUCores         *int      `json:"cpu_cores"`
	MemoryGB         *float64  `json:"memory_gb"`
	DiskGB           *float64  `json:"disk_gb"`
	OSDetail         *string   `json:"os_detail"`
}

// nullToAny 将 sql.NullX 转为 SQL 参数
func nullToAny(ns interface{}) interface{} {
	switch v := ns.(type) {
	case sql.NullString:
		if v.Valid {
			return v.String
		}
	case sql.NullInt64:
		if v.Valid {
			return v.Int64
		}
	case sql.NullFloat64:
		if v.Valid {
			return v.Float64
		}
	}
	return nil
}

// boolToInt bool 转 int（数据库 use_ssh_key 列为 INTEGER）
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// atoi 简单字符串转 int
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
