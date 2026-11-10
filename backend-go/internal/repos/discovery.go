// 设备发现 — 扫描任务和结果，等价于 TS networkDeviceRepository/discovery.ts
package repos

import (
	"database/sql"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// 发现任务
// ───────────────────────────────────────────────

// DiscoveryJob 发现任务记录
type DiscoveryJob struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	StartIP       string  `json:"start_ip"`
	EndIP         string  `json:"end_ip"`
	Status        string  `json:"status"`
	Progress      int     `json:"progress"`
	TotalHosts    int     `json:"total_hosts"`
	ScannedHosts  int     `json:"scanned_hosts"`
	FoundDevices  int     `json:"found_devices"`
	CredentialIDs string  `json:"credential_ids"`
	ErrorMessage  *string `json:"error_message"`
	StartedAt     *string `json:"started_at"`
	CompletedAt   *string `json:"completed_at"`
	CreatedAt     string  `json:"created_at"`
}

// DiscoveryJobRepo 发现任务仓库
type DiscoveryJobRepo struct{}

var DiscoveryJobs = &DiscoveryJobRepo{}

// DiscoveryJobCreateInput 创建任务输入
type DiscoveryJobCreateInput struct {
	ID            string
	Name          string
	StartIP       string
	EndIP         string
	Status        string
	Progress      int
	TotalHosts    int
	ScannedHosts  int
	FoundDevices  int
	CredentialIDs string
}

// Create 创建发现任务
func (r *DiscoveryJobRepo) Create(input DiscoveryJobCreateInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO network_discovery_jobs (id, name, start_ip, end_ip, status, progress, total_hosts, scanned_hosts, found_devices, credential_ids, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, input.StartIP, input.EndIP, input.Status, input.Progress,
		input.TotalHosts, input.ScannedHosts, input.FoundDevices, input.CredentialIDs, now)
	return err
}

// GetByID 获取单个任务
func (r *DiscoveryJobRepo) GetByID(id string) (*DiscoveryJob, error) {
	return scanDiscoveryJob(appdb.DB().QueryRow(`SELECT id, name, start_ip, end_ip, status, progress, total_hosts, scanned_hosts, found_devices, credential_ids, error_message, started_at, completed_at, created_at FROM network_discovery_jobs WHERE id = ?`, id))
}

// List 列出所有任务
func (r *DiscoveryJobRepo) List() ([]DiscoveryJob, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, start_ip, end_ip, status, progress, total_hosts, scanned_hosts, found_devices, credential_ids, error_message, started_at, completed_at, created_at FROM network_discovery_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiscoveryJob
	for rows.Next() {
		j, err := scanDiscoveryJob(rows)
		if err != nil {
			continue
		}
		out = append(out, *j)
	}
	return out, nil
}

// UpdateStatus 更新任务状态
func (r *DiscoveryJobRepo) UpdateStatus(id, status string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_discovery_jobs SET status = ?, started_at = ? WHERE id = ?`, status, now, id)
	return err
}

// UpdateProgress 更新任务进度
func (r *DiscoveryJobRepo) UpdateProgress(id string, progress, scanned, found int) error {
	_, err := appdb.DB().Exec(`UPDATE network_discovery_jobs SET progress = ?, scanned_hosts = ?, found_devices = ? WHERE id = ?`, progress, scanned, found, id)
	return err
}

// Finish 完成任务
func (r *DiscoveryJobRepo) Finish(id, status string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_discovery_jobs SET status = ?, progress = 100, completed_at = ? WHERE id = ?`, status, now, id)
	return err
}

// Cancel 取消正在运行的任务
func (r *DiscoveryJobRepo) Cancel(id string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_discovery_jobs SET status = ?, completed_at = ? WHERE id = ? AND status = ?`, "cancelled", now, id, "running")
	return err
}

// Delete 删除任务（级联删除结果）
func (r *DiscoveryJobRepo) Delete(id string) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _ = tx.Exec(`DELETE FROM network_discovery_results WHERE job_id = ?`, id)
	_, err = tx.Exec(`DELETE FROM network_discovery_jobs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func scanDiscoveryJob(scanner interface{ Scan(...interface{}) error }) (*DiscoveryJob, error) {
	var j DiscoveryJob
	var errMsg, startedAt, completedAt sql.NullString
	err := scanner.Scan(&j.ID, &j.Name, &j.StartIP, &j.EndIP, &j.Status, &j.Progress,
		&j.TotalHosts, &j.ScannedHosts, &j.FoundDevices, &j.CredentialIDs,
		&errMsg, &startedAt, &completedAt, &j.CreatedAt)
	if err != nil {
		return nil, err
	}
	j.ErrorMessage = ns(errMsg)
	j.StartedAt = ns(startedAt)
	j.CompletedAt = ns(completedAt)
	return &j, nil
}

// ───────────────────────────────────────────────
// 发现结果
// ───────────────────────────────────────────────

// DiscoveryResult 发现结果记录
type DiscoveryResult struct {
	ID             string  `json:"id"`
	JobID          string  `json:"job_id"`
	IPAddress      string  `json:"ip_address"`
	Status         string  `json:"status"`
	SysName        *string `json:"sys_name"`
	SysDescr       *string `json:"sys_descr"`
	SysLocation    *string `json:"sys_location"`
	SysObjectID    *string `json:"sys_object_id"`
	SnmpVersion    *string `json:"snmp_version"`
	Community      *string `json:"community"`
	InterfaceCount *int    `json:"interface_count"`
	Vendor         *string `json:"vendor"`
	Model          *string `json:"model"`
	ResponseTimeMs *int    `json:"response_time_ms"`
	CreatedAt      string  `json:"created_at"`
}

// DiscoveryResultRepo 发现结果仓库
type DiscoveryResultRepo struct{}

var DiscoveryResults = &DiscoveryResultRepo{}

// DiscoveryResultListOptions 结果列表选项
type DiscoveryResultListOptions struct {
	JobID  string
	Status string
	Limit  int
	Offset int
}

// List 列出发现结果（分页+过滤）
func (r *DiscoveryResultRepo) List(opts DiscoveryResultListOptions) ([]DiscoveryResult, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	query := `SELECT id, job_id, ip_address, status, sys_name, sys_descr, sys_location, sys_object_id, snmp_version, community, interface_count, vendor, model, response_time_ms, created_at FROM network_discovery_results WHERE 1=1`
	var args []interface{}
	if opts.JobID != "" {
		query += " AND job_id = ?"
		args = append(args, opts.JobID)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}

	// 计算总数
	countQuery := `SELECT COUNT(*) FROM network_discovery_results WHERE 1=1`
	if opts.JobID != "" {
		countQuery += " AND job_id = ?"
	}
	if opts.Status != "" {
		countQuery += " AND status = ?"
	}
	var countArgs []interface{}
	if opts.JobID != "" {
		countArgs = append(countArgs, opts.JobID)
	}
	if opts.Status != "" {
		countArgs = append(countArgs, opts.Status)
	}
	var total int
	_ = appdb.DB().QueryRow(countQuery, countArgs...).Scan(&total)

	query += " ORDER BY status ASC, response_time_ms ASC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanDiscoveryResults(rows, total)
}

// GetByID 获取单个结果
func (r *DiscoveryResultRepo) GetByID(id string) (*DiscoveryResult, error) {
	row := appdb.DB().QueryRow(`SELECT id, job_id, ip_address, status, sys_name, sys_descr, sys_location, sys_object_id, snmp_version, community, interface_count, vendor, model, response_time_ms, created_at FROM network_discovery_results WHERE id = ?`, id)
	var res DiscoveryResult
	var sysName, sysDescr, sysLocation, sysObjectID, snmpVersion, community, vendor, model sql.NullString
	var interfaceCount, responseTimeMs sql.NullInt64
	err := row.Scan(&res.ID, &res.JobID, &res.IPAddress, &res.Status, &sysName, &sysDescr, &sysLocation, &sysObjectID,
		&snmpVersion, &community, &interfaceCount, &vendor, &model, &responseTimeMs, &res.CreatedAt)
	if err != nil {
		return nil, err
	}
	res.SysName = ns(sysName)
	res.SysDescr = ns(sysDescr)
	res.SysLocation = ns(sysLocation)
	res.SysObjectID = ns(sysObjectID)
	res.SnmpVersion = ns(snmpVersion)
	res.Community = ns(community)
	res.Vendor = ns(vendor)
	res.Model = ns(model)
	if interfaceCount.Valid {
		v := int(interfaceCount.Int64)
		res.InterfaceCount = &v
	}
	if responseTimeMs.Valid {
		v := int(responseTimeMs.Int64)
		res.ResponseTimeMs = &v
	}
	return &res, nil
}

// DeleteByJobID 删除任务关联的结果
func (r *DiscoveryResultRepo) DeleteByJobID(jobID string) error {
	_, err := appdb.DB().Exec(`DELETE FROM network_discovery_results WHERE job_id = ?`, jobID)
	return err
}

// InsertOffline 插入离线结果
func (r *DiscoveryResultRepo) InsertOffline(id, jobID, ip string, responseTimeMs int) error {
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO network_discovery_results (id, job_id, ip_address, status, response_time_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, jobID, ip, "offline", responseTimeMs, now)
	return err
}

// DiscoveryResultOnlineInput 在线结果输入
type DiscoveryResultOnlineInput struct {
	ID             string
	JobID          string
	IPAddress      string
	Status         string
	SysName        string
	SysDescr       string
	SysLocation    string
	SysObjectID    string
	SnmpVersion    string
	Community      string
	InterfaceCount int
	Vendor         string
	Model          string
	ResponseTimeMs int
}

// InsertOnline 插入在线结果
func (r *DiscoveryResultRepo) InsertOnline(input DiscoveryResultOnlineInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	var interfaceCountVal interface{}
	if input.InterfaceCount > 0 {
		interfaceCountVal = input.InterfaceCount
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO network_discovery_results (id, job_id, ip_address, status, sys_name, sys_descr, sys_location, sys_object_id,
			snmp_version, community, interface_count, vendor, model, response_time_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.JobID, input.IPAddress, input.Status,
		sqlNullStr(input.SysName), sqlNullStr(input.SysDescr), sqlNullStr(input.SysLocation), sqlNullStr(input.SysObjectID),
		sqlNullStr(input.SnmpVersion), sqlNullStr(input.Community), interfaceCountVal,
		sqlNullStr(input.Vendor), sqlNullStr(input.Model), input.ResponseTimeMs, now)
	return err
}

func scanDiscoveryResults(rows *sql.Rows, total int) ([]DiscoveryResult, int, error) {
	out := make([]DiscoveryResult, 0)
	for rows.Next() {
		var r DiscoveryResult
		var sysName, sysDescr, sysLocation, sysObjectID, snmpVersion, community, vendor, model sql.NullString
		var interfaceCount, responseTimeMs sql.NullInt64
		if err := rows.Scan(&r.ID, &r.JobID, &r.IPAddress, &r.Status, &sysName, &sysDescr, &sysLocation, &sysObjectID,
			&snmpVersion, &community, &interfaceCount, &vendor, &model, &responseTimeMs, &r.CreatedAt); err != nil {
			continue
		}
		r.SysName = ns(sysName)
		r.SysDescr = ns(sysDescr)
		r.SysLocation = ns(sysLocation)
		r.SysObjectID = ns(sysObjectID)
		r.SnmpVersion = ns(snmpVersion)
		r.Community = ns(community)
		r.Vendor = ns(vendor)
		r.Model = ns(model)
		if interfaceCount.Valid {
			v := int(interfaceCount.Int64)
			r.InterfaceCount = &v
		}
		if responseTimeMs.Valid {
			v := int(responseTimeMs.Int64)
			r.ResponseTimeMs = &v
		}
		out = append(out, r)
	}
	return out, total, nil
}
