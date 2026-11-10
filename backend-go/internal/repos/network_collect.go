// MIB 模块/对象 + 采集任务/结果，等价于 TS snmpCollectRepository.ts
package repos

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// MIB 模块
// ───────────────────────────────────────────────

// MibModule MIB 模块记录
type MibModule struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	FileName     *string `json:"file_name"`
	FileContent  *string `json:"file_content,omitempty"`
	Description  *string `json:"description"`
	Organization *string `json:"organization"`
	ContactInfo  *string `json:"contact_info"`
	LastUpdated  *string `json:"last_updated"`
	OidTreeJSON  *string `json:"oid_tree_json,omitempty"`
	ObjectCount  int     `json:"object_count"`
	UploadedAt   string  `json:"uploaded_at"`
}

// MibObject MIB 对象记录
type MibObject struct {
	ID           string  `json:"id"`
	ModuleID     string  `json:"module_id"`
	ModuleName   string  `json:"module_name"`
	OID          string  `json:"oid"`
	Name         string  `json:"name"`
	Syntax       *string `json:"syntax"`
	Access       *string `json:"access"`
	Status       *string `json:"status"`
	Description  *string `json:"description"`
	ParentOID    *string `json:"parent_oid"`
	TableName    *string `json:"table_name"`
	IndexColumns *string `json:"index_columns"`
	IsTable      int     `json:"is_table"`
	IsScalar     int     `json:"is_scalar"`
	// 以下字段为 JOIN 或查询结果
	Category    *string `json:"category,omitempty"`
	Subcategory *string `json:"subcategory,omitempty"`
	Label       *string `json:"label,omitempty"`
	Color       *string `json:"color,omitempty"`
}

// MibModuleRepo MIB 模块仓库
type MibModuleRepo struct{}

var MibModules = &MibModuleRepo{}

func (r *MibModuleRepo) List() ([]MibModule, error) {
	rows, err := appdb.OidDB().Query(`SELECT id, name, file_name, description, organization, contact_info, last_updated, object_count, uploaded_at FROM mib_modules ORDER BY uploaded_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MibModule
	for rows.Next() {
		var m MibModule
		var fileName, desc, org, contact, lastUpdated sql.NullString
		if err := rows.Scan(&m.ID, &m.Name, &fileName, &desc, &org, &contact, &lastUpdated, &m.ObjectCount, &m.UploadedAt); err != nil {
			continue
		}
		m.FileName = ns(fileName)
		m.Description = ns(desc)
		m.Organization = ns(org)
		m.ContactInfo = ns(contact)
		m.LastUpdated = ns(lastUpdated)
		out = append(out, m)
	}
	return out, nil
}

func (r *MibModuleRepo) GetByID(id string) (*MibModule, error) {
	row := appdb.OidDB().QueryRow(`SELECT id, name, file_name, file_content, description, organization, contact_info, last_updated, oid_tree_json, object_count, uploaded_at FROM mib_modules WHERE id = ?`, id)
	var m MibModule
	var fileName, fileContent, desc, org, contact, lastUpdated, oidTree sql.NullString
	err := row.Scan(&m.ID, &m.Name, &fileName, &fileContent, &desc, &org, &contact, &lastUpdated, &oidTree, &m.ObjectCount, &m.UploadedAt)
	if err != nil {
		return nil, err
	}
	m.FileName = ns(fileName)
	m.FileContent = ns(fileContent)
	m.Description = ns(desc)
	m.Organization = ns(org)
	m.ContactInfo = ns(contact)
	m.LastUpdated = ns(lastUpdated)
	m.OidTreeJSON = ns(oidTree)
	return &m, nil
}

func (r *MibModuleRepo) Create(m MibModule) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.OidDB().Exec(`
		INSERT INTO mib_modules (id, name, file_name, file_content, description, organization, contact_info, last_updated, oid_tree_json, object_count, uploaded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Name, sqlNullStr(ptrStr(m.FileName)), sqlNullStr(ptrStr(m.FileContent)),
		sqlNullStr(ptrStr(m.Description)), sqlNullStr(ptrStr(m.Organization)), sqlNullStr(ptrStr(m.ContactInfo)),
		sqlNullStr(ptrStr(m.LastUpdated)), sqlNullStr(ptrStr(m.OidTreeJSON)), m.ObjectCount, now)
	return err
}

func (r *MibModuleRepo) UpdateObjectCount(id string, count int) error {
	_, err := appdb.OidDB().Exec(`UPDATE mib_modules SET object_count = ? WHERE id = ?`, count, id)
	return err
}

func (r *MibModuleRepo) Delete(id string) error {
	tx, err := appdb.OidDB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM mib_objects WHERE module_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM mib_modules WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ───────────────────────────────────────────────
// MIB 对象
// ───────────────────────────────────────────────

type MibObjectRepo struct{}

var MibObjects = &MibObjectRepo{}

func (r *MibObjectRepo) ListByModuleID(moduleID string) ([]MibObject, error) {
	rows, err := appdb.OidDB().Query(`SELECT id, module_id, module_name, oid, name, syntax, access, status, description, parent_oid, table_name, index_columns, is_table, is_scalar FROM mib_objects WHERE module_id = ? ORDER BY oid ASC`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMibObjects(rows)
}

func (r *MibObjectRepo) ListByModuleIDs(moduleIDs []string) ([]MibObject, error) {
	if len(moduleIDs) == 0 {
		return nil, nil
	}
	placeholders := ""
	args := make([]interface{}, len(moduleIDs))
	for i, id := range moduleIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT id, module_id, module_name, oid, name, syntax, access, status, description, parent_oid, table_name, index_columns, is_table, is_scalar FROM mib_objects WHERE module_id IN (%s) ORDER BY oid ASC`, placeholders)
	rows, err := appdb.OidDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMibObjects(rows)
}

func (r *MibObjectRepo) Search(query string, limit int) ([]MibObject, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	pattern := "%" + query + "%"
	rows, err := appdb.OidDB().Query(`
		SELECT obj.id, obj.module_id, obj.module_name, obj.oid, obj.name, obj.syntax, obj.access, obj.status, obj.description, obj.parent_oid, obj.table_name, obj.index_columns, obj.is_table, obj.is_scalar
		FROM mib_objects obj
		LEFT JOIN mib_modules mod ON obj.module_id = mod.id
		WHERE obj.name LIKE ? OR obj.oid LIKE ? OR obj.description LIKE ? OR obj.module_name LIKE ? OR mod.name LIKE ? OR mod.organization LIKE ?
		ORDER BY obj.oid ASC LIMIT ?`,
		pattern, pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMibObjects(rows)
}

func (r *MibObjectRepo) CreateBatch(objects []MibObject) error {
	tx, err := appdb.OidDB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO mib_objects (id, module_id, module_name, oid, name, syntax, access, status, description, parent_oid, table_name, index_columns, is_table, is_scalar)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, o := range objects {
		if o.ID == "" {
			o.ID = uuid.NewString()
		}
		_, err := stmt.Exec(o.ID, o.ModuleID, o.ModuleName, o.OID, o.Name,
			sqlNullStr(ptrStr(o.Syntax)), sqlNullStr(ptrStr(o.Access)), sqlNullStr(ptrStr(o.Status)),
			sqlNullStr(ptrStr(o.Description)), sqlNullStr(ptrStr(o.ParentOID)),
			sqlNullStr(ptrStr(o.TableName)), sqlNullStr(ptrStr(o.IndexColumns)),
			o.IsTable, o.IsScalar)
		if err != nil {
			return fmt.Errorf("batch insert mib object: %w", err)
		}
	}
	return tx.Commit()
}

func (r *MibObjectRepo) DeleteByModuleID(moduleID string) error {
	_, err := appdb.OidDB().Exec(`DELETE FROM mib_objects WHERE module_id = ?`, moduleID)
	return err
}

func scanMibObjects(rows *sql.Rows) ([]MibObject, error) {
	var out []MibObject
	for rows.Next() {
		var o MibObject
		var syntax, access, status, desc, parentOID, tableName, indexCols sql.NullString
		if err := rows.Scan(&o.ID, &o.ModuleID, &o.ModuleName, &o.OID, &o.Name,
			&syntax, &access, &status, &desc, &parentOID, &tableName, &indexCols, &o.IsTable, &o.IsScalar); err != nil {
			continue
		}
		o.Syntax = ns(syntax)
		o.Access = ns(access)
		o.Status = ns(status)
		o.Description = ns(desc)
		o.ParentOID = ns(parentOID)
		o.TableName = ns(tableName)
		o.IndexColumns = ns(indexCols)
		out = append(out, o)
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 采集任务
// ───────────────────────────────────────────────

// CollectTask SNMP 采集任务
type CollectTask struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	DeviceType      *string `json:"device_type"`
	DeviceIDs       string  `json:"device_ids"` // JSON array
	OIDs            string  `json:"oids"`       // JSON array
	ScheduleType    string  `json:"schedule_type"`
	IntervalSeconds int     `json:"interval_seconds"`
	CronExpression  *string `json:"cron_expression"`
	TimeoutMs       int     `json:"timeout_ms"`
	Retries         int     `json:"retries"`
	SnmpVersion     string  `json:"snmp_version"`
	Community       *string `json:"community,omitempty"`
	Enabled         int     `json:"enabled"`
	RetentionDays   int     `json:"retention_days"`
	LastRunAt       *string `json:"last_run_at"`
	LastRunStatus   *string `json:"last_run_status"`
	NextRunAt       *string `json:"next_run_at"`
	CreatedBy       *string `json:"created_by"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// CollectResult SNMP 采集结果
type CollectResult struct {
	ID           string  `json:"id"`
	TaskID       string  `json:"task_id"`
	TaskName     *string `json:"task_name"`
	DeviceID     string  `json:"device_id"`
	DeviceName   *string `json:"device_name"`
	DeviceIP     *string `json:"device_ip"`
	OID          string  `json:"oid"`
	MetricName   string  `json:"metric_name"`
	MetricValue  string  `json:"metric_value"`
	MetricType   *string `json:"metric_type"`
	ErrorMessage *string `json:"error_message"`
	DurationMs   int     `json:"duration_ms"`
	SampledAt    string  `json:"sampled_at"`
}

// CollectTaskRepo 采集任务仓库
type CollectTaskRepo struct{}

var CollectTasks = &CollectTaskRepo{}

func (r *CollectTaskRepo) List() ([]CollectTask, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, description, device_type, device_ids, oids, schedule_type, interval_seconds, cron_expression, timeout_ms, retries, snmp_version, community, enabled, retention_days, last_run_at, last_run_status, next_run_at, created_by, created_at, updated_at FROM snmp_collect_tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCollectTasks(rows)
}

func (r *CollectTaskRepo) GetByID(id string) (*CollectTask, error) {
	row := appdb.DB().QueryRow(`SELECT id, name, description, device_type, device_ids, oids, schedule_type, interval_seconds, cron_expression, timeout_ms, retries, snmp_version, community, enabled, retention_days, last_run_at, last_run_status, next_run_at, created_by, created_at, updated_at FROM snmp_collect_tasks WHERE id = ?`, id)
	return scanCollectTask(row)
}

func (r *CollectTaskRepo) ListEnabled() ([]CollectTask, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, description, device_type, device_ids, oids, schedule_type, interval_seconds, cron_expression, timeout_ms, retries, snmp_version, community, enabled, retention_days, last_run_at, last_run_status, next_run_at, created_by, created_at, updated_at FROM snmp_collect_tasks WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCollectTasks(rows)
}

func (r *CollectTaskRepo) Create(t CollectTask) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO snmp_collect_tasks (id, name, description, device_type, device_ids, oids, schedule_type, interval_seconds, cron_expression, timeout_ms, retries, snmp_version, community, enabled, retention_days, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, sqlNullStr(ptrStr(t.Description)), sqlNullStr(ptrStr(t.DeviceType)),
		t.DeviceIDs, t.OIDs, t.ScheduleType, t.IntervalSeconds, sqlNullStr(ptrStr(t.CronExpression)),
		t.TimeoutMs, t.Retries, t.SnmpVersion, sqlNullStr(ptrStr(t.Community)),
		t.Enabled, t.RetentionDays, sqlNullStr(ptrStr(t.CreatedBy)), now, now)
	return err
}

func (r *CollectTaskRepo) Update(id string, t CollectTask) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		UPDATE snmp_collect_tasks SET name = ?, description = ?, device_type = ?, device_ids = ?, oids = ?, schedule_type = ?, interval_seconds = ?, cron_expression = ?, timeout_ms = ?, retries = ?, snmp_version = ?, community = ?, enabled = ?, retention_days = ?, updated_at = ?
		WHERE id = ?`,
		t.Name, sqlNullStr(ptrStr(t.Description)), sqlNullStr(ptrStr(t.DeviceType)),
		t.DeviceIDs, t.OIDs, t.ScheduleType, t.IntervalSeconds, sqlNullStr(ptrStr(t.CronExpression)),
		t.TimeoutMs, t.Retries, t.SnmpVersion, sqlNullStr(ptrStr(t.Community)),
		t.Enabled, t.RetentionDays, now, id)
	return err
}

func (r *CollectTaskRepo) UpdateRunResult(id, status string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE snmp_collect_tasks SET last_run_at = ?, last_run_status = ? WHERE id = ?`, now, status, id)
	return err
}

func (r *CollectTaskRepo) Delete(id string) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM snmp_collect_results WHERE task_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM snmp_collect_tasks WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func scanCollectTasks(rows *sql.Rows) ([]CollectTask, error) {
	var out []CollectTask
	for rows.Next() {
		var t CollectTask
		if err := scanCollectTaskRows(rows, &t); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func scanCollectTask(row *sql.Row) (*CollectTask, error) {
	var t CollectTask
	var desc, devType, cron, community, lastRun, lastStatus, nextRun, createdBy sql.NullString
	err := row.Scan(&t.ID, &t.Name, &desc, &devType, &t.DeviceIDs, &t.OIDs, &t.ScheduleType, &t.IntervalSeconds,
		&cron, &t.TimeoutMs, &t.Retries, &t.SnmpVersion, &community, &t.Enabled, &t.RetentionDays,
		&lastRun, &lastStatus, &nextRun, &createdBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.Description = ns(desc)
	t.DeviceType = ns(devType)
	t.CronExpression = ns(cron)
	t.Community = ns(community)
	t.LastRunAt = ns(lastRun)
	t.LastRunStatus = ns(lastStatus)
	t.NextRunAt = ns(nextRun)
	t.CreatedBy = ns(createdBy)
	return &t, nil
}

func scanCollectTaskRows(rows *sql.Rows, t *CollectTask) error {
	var desc, devType, cron, community, lastRun, lastStatus, nextRun, createdBy sql.NullString
	err := rows.Scan(&t.ID, &t.Name, &desc, &devType, &t.DeviceIDs, &t.OIDs, &t.ScheduleType, &t.IntervalSeconds,
		&cron, &t.TimeoutMs, &t.Retries, &t.SnmpVersion, &community, &t.Enabled, &t.RetentionDays,
		&lastRun, &lastStatus, &nextRun, &createdBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return err
	}
	t.Description = ns(desc)
	t.DeviceType = ns(devType)
	t.CronExpression = ns(cron)
	t.Community = ns(community)
	t.LastRunAt = ns(lastRun)
	t.LastRunStatus = ns(lastStatus)
	t.NextRunAt = ns(nextRun)
	t.CreatedBy = ns(createdBy)
	return nil
}

// ───────────────────────────────────────────────
// 采集结果
// ───────────────────────────────────────────────

type CollectResultRepo struct{}

var CollectResults = &CollectResultRepo{}

func (r *CollectResultRepo) List(page, pageSize int, taskID, deviceID string) ([]CollectResult, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	return r.ListByOffset(pageSize, (page-1)*pageSize, taskID, deviceID)
}

// ListByOffset 按 limit/offset 查询采集结果（调用方可直接使用 offset 分页）
func (r *CollectResultRepo) ListByOffset(pageSize, offset int, taskID, deviceID string) ([]CollectResult, int, error) {
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := `SELECT r.id, r.task_id, t.name, r.device_id, nd.name, nd.ip_address, r.oid, r.metric_name, r.metric_value, r.metric_type, r.error_message, r.duration_ms, r.sampled_at
		FROM snmp_collect_results r
		LEFT JOIN snmp_collect_tasks t ON r.task_id = t.id
		LEFT JOIN network_devices nd ON r.device_id = nd.id WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM snmp_collect_results r WHERE 1=1`
	var args []interface{}
	if taskID != "" {
		query += " AND r.task_id = ?"
		countQuery += " AND r.task_id = ?"
		args = append(args, taskID)
	}
	if deviceID != "" {
		query += " AND r.device_id = ?"
		countQuery += " AND r.device_id = ?"
		args = append(args, deviceID)
	}
	query += " ORDER BY r.sampled_at DESC LIMIT ? OFFSET ?"

	queryArgs := append(args, pageSize, offset)
	rows, err := appdb.DB().Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []CollectResult
	for rows.Next() {
		var r CollectResult
		var taskName, devName, devIP, metricType, errMsg sql.NullString
		if err := rows.Scan(&r.ID, &r.TaskID, &taskName, &r.DeviceID, &devName, &devIP, &r.OID, &r.MetricName, &r.MetricValue, &metricType, &errMsg, &r.DurationMs, &r.SampledAt); err != nil {
			continue
		}
		r.TaskName = ns(taskName)
		r.DeviceName = ns(devName)
		r.DeviceIP = ns(devIP)
		r.MetricType = ns(metricType)
		r.ErrorMessage = ns(errMsg)
		out = append(out, r)
	}

	var total int
	_ = appdb.DB().QueryRow(countQuery, args...).Scan(&total)
	return out, total, nil
}

func (r *CollectResultRepo) InsertBatch(results []CollectResult) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Format("2006-01-02 15:04:05")
	stmt, err := tx.Prepare(`
		INSERT INTO snmp_collect_results (id, task_id, task_name, device_id, device_name, device_ip, oid, metric_name, metric_value, metric_type, error_message, duration_ms, sampled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, res := range results {
		id := uuid.NewString()
		_, err := stmt.Exec(id, res.TaskID, sqlNullStr(ptrStr(res.TaskName)), res.DeviceID,
			sqlNullStr(ptrStr(res.DeviceName)), sqlNullStr(ptrStr(res.DeviceIP)),
			res.OID, res.MetricName, res.MetricValue, res.MetricType,
			sqlNullStr(ptrStr(res.ErrorMessage)), res.DurationMs, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *CollectResultRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM snmp_collect_results WHERE id = ?`, id)
	return err
}
