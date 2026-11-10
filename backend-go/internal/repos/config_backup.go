// 配置备份 — 模板/任务/记录，等价于 TS configBackupRepository.ts
package repos

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// 配置备份模板
// ───────────────────────────────────────────────

// ConfigBackupTemplate 配置备份模板
type ConfigBackupTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Vendor      string   `json:"vendor"`
	Commands    []string `json:"commands"`
	IsBuiltin   int      `json:"is_builtin"`
	CreatedBy   *string  `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// TemplateCreateInput 模板创建输入
type TemplateCreateInput struct {
	ID          string
	Name        string
	Description string
	Vendor      string
	Commands    []string
	CreatedBy   string
}

// TemplateUpdateInput 模板更新输入
type TemplateUpdateInput struct {
	Name        *string
	Description *string
	Commands    *[]string
	Vendor      *string
}

// ConfigBackupTemplateRepo 模板仓库
type ConfigBackupTemplateRepo struct{}

var ConfigBackupTemplates = &ConfigBackupTemplateRepo{}

func parseTemplateRow(id, name string, desc sql.NullString, vendor string, commandsJSON string, isBuiltin int, createdBy sql.NullString, createdAt, updatedAt string) *ConfigBackupTemplate {
	var cmds []string
	_ = json.Unmarshal([]byte(commandsJSON), &cmds)
	return &ConfigBackupTemplate{
		ID:          id,
		Name:        name,
		Description: ns(desc),
		Vendor:      vendor,
		Commands:    cmds,
		IsBuiltin:   isBuiltin,
		CreatedBy:   ns(createdBy),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

// List 列出所有模板
func (r *ConfigBackupTemplateRepo) List() ([]*ConfigBackupTemplate, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, description, vendor, commands, is_builtin, created_by, created_at, updated_at FROM config_backup_templates ORDER BY is_builtin DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConfigBackupTemplate
	for rows.Next() {
		var id, name, vendor, commandsJSON, createdAt, updatedAt string
		var desc, createdBy sql.NullString
		var isBuiltin int
		if err := rows.Scan(&id, &name, &desc, &vendor, &commandsJSON, &isBuiltin, &createdBy, &createdAt, &updatedAt); err != nil {
			continue
		}
		out = append(out, parseTemplateRow(id, name, desc, vendor, commandsJSON, isBuiltin, createdBy, createdAt, updatedAt))
	}
	return out, nil
}

// ListByVendor 按厂商列出模板
func (r *ConfigBackupTemplateRepo) ListByVendor(vendor string) ([]*ConfigBackupTemplate, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, description, vendor, commands, is_builtin, created_by, created_at, updated_at FROM config_backup_templates WHERE vendor = ? OR vendor = 'custom' ORDER BY is_builtin DESC, name ASC`, vendor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConfigBackupTemplate
	for rows.Next() {
		var id, name, v, commandsJSON, createdAt, updatedAt string
		var desc, createdBy sql.NullString
		var isBuiltin int
		if err := rows.Scan(&id, &name, &desc, &v, &commandsJSON, &isBuiltin, &createdBy, &createdAt, &updatedAt); err != nil {
			continue
		}
		out = append(out, parseTemplateRow(id, name, desc, v, commandsJSON, isBuiltin, createdBy, createdAt, updatedAt))
	}
	return out, nil
}

// GetByID 获取单个模板
func (r *ConfigBackupTemplateRepo) GetByID(id string) (*ConfigBackupTemplate, error) {
	var tid, name, vendor, commandsJSON, createdAt, updatedAt string
	var desc, createdBy sql.NullString
	var isBuiltin int
	err := appdb.DB().QueryRow(`SELECT id, name, description, vendor, commands, is_builtin, created_by, created_at, updated_at FROM config_backup_templates WHERE id = ?`, id).
		Scan(&tid, &name, &desc, &vendor, &commandsJSON, &isBuiltin, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return parseTemplateRow(tid, name, desc, vendor, commandsJSON, isBuiltin, createdBy, createdAt, updatedAt), nil
}

// Create 创建模板
func (r *ConfigBackupTemplateRepo) Create(input TemplateCreateInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	cmdsJSON, _ := json.Marshal(input.Commands)
	_, err := appdb.DB().Exec(`
		INSERT INTO config_backup_templates (id, name, description, vendor, commands, is_builtin, created_by)
		VALUES (?, ?, ?, ?, ?, 0, ?)`,
		input.ID, input.Name, sqlNullStr(input.Description), input.Vendor, string(cmdsJSON), sqlNullStr(input.CreatedBy))
	return err
}

// Update 更新模板
func (r *ConfigBackupTemplateRepo) Update(id string, input TemplateUpdateInput) error {
	var sets []string
	var args []interface{}
	if input.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *input.Name)
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, sqlNullStr(*input.Description))
	}
	if input.Commands != nil {
		cmdsJSON, _ := json.Marshal(*input.Commands)
		sets = append(sets, "commands = ?")
		args = append(args, string(cmdsJSON))
	}
	if input.Vendor != nil {
		sets = append(sets, "vendor = ?")
		args = append(args, *input.Vendor)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	sets = append(sets, "updated_at = ?")
	args = append(args, now, id)
	_, err := appdb.DB().Exec("UPDATE config_backup_templates SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// Delete 删除模板
func (r *ConfigBackupTemplateRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM config_backup_templates WHERE id = ?`, id)
	return err
}

// ───────────────────────────────────────────────
// 配置备份任务
// ───────────────────────────────────────────────

// ConfigBackupTask 配置备份任务
type ConfigBackupTask struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	TemplateID    string   `json:"template_id"`
	ScheduleType  string   `json:"schedule_type"`
	ScheduleCron  *string  `json:"schedule_cron"`
	DeviceIDs     []string `json:"device_ids"`
	Enabled       int      `json:"enabled"`
	LastRunAt     *string  `json:"last_run_at"`
	LastRunStatus *string  `json:"last_run_status"`
	NextRunAt     *string  `json:"next_run_at"`
	RetentionDays int      `json:"retention_days"`
	CreatedBy     *string  `json:"created_by"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	// JOIN 字段
	TemplateName *string `json:"template_name,omitempty"`
	DeviceCount  int     `json:"device_count,omitempty"`
}

// TaskCreateInput 任务创建输入
type TaskCreateInput struct {
	ID            string
	Name          string
	TemplateID    string
	ScheduleType  string
	ScheduleCron  string
	DeviceIDs     []string
	Enabled       bool
	RetentionDays int
	CreatedBy     string
}

// TaskUpdateInput 任务更新输入
type TaskUpdateInput struct {
	Name          *string
	TemplateID    *string
	ScheduleType  *string
	ScheduleCron  *string
	DeviceIDs     *[]string
	Enabled       *bool
	RetentionDays *int
}

// ConfigBackupTaskRepo 任务仓库
type ConfigBackupTaskRepo struct{}

var ConfigBackupTasks = &ConfigBackupTaskRepo{}

func scanTaskRow(scanner interface{ Scan(...interface{}) error }) (*ConfigBackupTask, error) {
	var t ConfigBackupTask
	var scheduleCron, lastRunAt, lastRunStatus, nextRunAt, createdBy, templateName sql.NullString
	var deviceIDsJSON string
	err := scanner.Scan(&t.ID, &t.Name, &t.TemplateID, &t.ScheduleType, &scheduleCron, &deviceIDsJSON,
		&t.Enabled, &lastRunAt, &lastRunStatus, &nextRunAt, &t.RetentionDays, &createdBy, &t.CreatedAt, &t.UpdatedAt, &templateName)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(deviceIDsJSON), &t.DeviceIDs)
	if t.DeviceIDs == nil {
		t.DeviceIDs = []string{}
	}
	t.ScheduleCron = ns(scheduleCron)
	t.LastRunAt = ns(lastRunAt)
	t.LastRunStatus = ns(lastRunStatus)
	t.NextRunAt = ns(nextRunAt)
	t.CreatedBy = ns(createdBy)
	t.TemplateName = ns(templateName)
	t.DeviceCount = len(t.DeviceIDs)
	return &t, nil
}

// List 列出所有任务
func (r *ConfigBackupTaskRepo) List() ([]*ConfigBackupTask, error) {
	rows, err := appdb.DB().Query(`
		SELECT t.id, t.name, t.template_id, t.schedule_type, t.schedule_cron, t.device_ids,
			t.enabled, t.last_run_at, t.last_run_status, t.next_run_at, t.retention_days,
			t.created_by, t.created_at, t.updated_at, tp.name AS template_name
		FROM config_backup_tasks t
		LEFT JOIN config_backup_templates tp ON t.template_id = tp.id
		ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConfigBackupTask
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// ListEnabled 列出启用的任务
func (r *ConfigBackupTaskRepo) ListEnabled() ([]*ConfigBackupTask, error) {
	rows, err := appdb.DB().Query(`
		SELECT t.id, t.name, t.template_id, t.schedule_type, t.schedule_cron, t.device_ids,
			t.enabled, t.last_run_at, t.last_run_status, t.next_run_at, t.retention_days,
			t.created_by, t.created_at, t.updated_at, tp.name AS template_name
		FROM config_backup_tasks t
		LEFT JOIN config_backup_templates tp ON t.template_id = tp.id
		WHERE t.enabled = 1
		ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConfigBackupTask
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// GetByID 获取单个任务
func (r *ConfigBackupTaskRepo) GetByID(id string) (*ConfigBackupTask, error) {
	row := appdb.DB().QueryRow(`
		SELECT t.id, t.name, t.template_id, t.schedule_type, t.schedule_cron, t.device_ids,
			t.enabled, t.last_run_at, t.last_run_status, t.next_run_at, t.retention_days,
			t.created_by, t.created_at, t.updated_at, tp.name AS template_name
		FROM config_backup_tasks t
		LEFT JOIN config_backup_templates tp ON t.template_id = tp.id
		WHERE t.id = ?`, id)
	return scanTaskRow(row)
}

// Create 创建任务
func (r *ConfigBackupTaskRepo) Create(input TaskCreateInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.RetentionDays == 0 {
		input.RetentionDays = 90
	}
	enabledVal := 0
	if input.Enabled {
		enabledVal = 1
	}
	deviceIDsJSON, _ := json.Marshal(input.DeviceIDs)
	nextRun := computeNextRun(input.ScheduleType, input.ScheduleCron)
	_, err := appdb.DB().Exec(`
		INSERT INTO config_backup_tasks (id, name, template_id, schedule_type, schedule_cron, device_ids, enabled, retention_days, next_run_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, input.TemplateID, input.ScheduleType,
		sqlNullStr(input.ScheduleCron), string(deviceIDsJSON),
		enabledVal, input.RetentionDays,
		sqlNullStr(nextRun), sqlNullStr(input.CreatedBy))
	return err
}

// Update 更新任务
func (r *ConfigBackupTaskRepo) Update(id string, input TaskUpdateInput) error {
	var sets []string
	var args []interface{}
	if input.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *input.Name)
	}
	if input.TemplateID != nil {
		sets = append(sets, "template_id = ?")
		args = append(args, *input.TemplateID)
	}
	if input.ScheduleType != nil {
		sets = append(sets, "schedule_type = ?")
		args = append(args, *input.ScheduleType)
	}
	if input.ScheduleCron != nil {
		sets = append(sets, "schedule_cron = ?")
		args = append(args, sqlNullStr(*input.ScheduleCron))
	}
	if input.DeviceIDs != nil {
		deviceIDsJSON, _ := json.Marshal(*input.DeviceIDs)
		sets = append(sets, "device_ids = ?")
		args = append(args, string(deviceIDsJSON))
	}
	if input.Enabled != nil {
		enabledVal := 0
		if *input.Enabled {
			enabledVal = 1
		}
		sets = append(sets, "enabled = ?")
		args = append(args, enabledVal)
	}
	if input.RetentionDays != nil {
		sets = append(sets, "retention_days = ?")
		args = append(args, *input.RetentionDays)
	}
	if len(sets) == 0 {
		return nil
	}
	// 重新计算 next_run_at
	if input.ScheduleType != nil || input.ScheduleCron != nil {
		current, _ := r.GetByID(id)
		if current != nil {
			scheduleType := current.ScheduleType
			if input.ScheduleType != nil {
				scheduleType = *input.ScheduleType
			}
			scheduleCron := ""
			if current.ScheduleCron != nil {
				scheduleCron = *current.ScheduleCron
			}
			if input.ScheduleCron != nil {
				scheduleCron = *input.ScheduleCron
			}
			nextRun := computeNextRun(scheduleType, scheduleCron)
			if nextRun != "" {
				sets = append(sets, "next_run_at = ?")
				args = append(args, nextRun)
			}
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	sets = append(sets, "updated_at = ?")
	args = append(args, now, id)
	_, err := appdb.DB().Exec("UPDATE config_backup_tasks SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// UpdateRunResult 更新任务运行结果
func (r *ConfigBackupTaskRepo) UpdateRunResult(id, status string) error {
	current, _ := r.GetByID(id)
	nextRun := ""
	if current != nil {
		cron := ""
		if current.ScheduleCron != nil {
			cron = *current.ScheduleCron
		}
		nextRun = computeNextRun(current.ScheduleType, cron)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE config_backup_tasks SET last_run_at = ?, last_run_status = ?, next_run_at = ? WHERE id = ?`,
		now, status, sqlNullStr(nextRun), id)
	return err
}

// Delete 删除任务
func (r *ConfigBackupTaskRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM config_backup_tasks WHERE id = ?`, id)
	return err
}

// ───────────────────────────────────────────────
// 配置备份记录
// ───────────────────────────────────────────────

// ConfigBackupRecord 配置备份记录
type ConfigBackupRecord struct {
	ID           string  `json:"id"`
	TaskID       *string `json:"task_id"`
	TaskName     *string `json:"task_name"`
	DeviceID     string  `json:"device_id"`
	DeviceName   string  `json:"device_name"`
	DeviceIP     string  `json:"device_ip"`
	DeviceVendor string  `json:"device_vendor"`
	TemplateID   *string `json:"template_id"`
	TemplateName *string `json:"template_name"`
	Status       string  `json:"status"`
	Commands     *string `json:"commands"`
	Output       *string `json:"output"`
	OutputSize   int     `json:"output_size"`
	DurationMs   *int    `json:"duration_ms"`
	ErrorMessage *string `json:"error_message"`
	StartedAt    string  `json:"started_at"`
	CompletedAt  *string `json:"completed_at"`
}

// RecordListOptions 记录列表选项
type RecordListOptions struct {
	TaskID   string
	DeviceID string
	Status   string
	Limit    int
	Offset   int
}

// ConfigBackupRecordRepo 记录仓库
type ConfigBackupRecordRepo struct{}

var ConfigBackupRecords = &ConfigBackupRecordRepo{}

// List 列出备份记录
func (r *ConfigBackupRecordRepo) List(opts RecordListOptions) ([]*ConfigBackupRecord, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	query := `SELECT id, task_id, task_name, device_id, device_name, device_ip, device_vendor, template_id, template_name, status, commands, output, output_size, duration_ms, error_message, started_at, completed_at FROM config_backup_records WHERE 1=1`
	var args []interface{}
	if opts.TaskID != "" {
		query += " AND task_id = ?"
		args = append(args, opts.TaskID)
	}
	if opts.DeviceID != "" {
		query += " AND device_id = ?"
		args = append(args, opts.DeviceID)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	query += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBackupRecords(rows)
}

// Count 统计记录数
func (r *ConfigBackupRecordRepo) Count(opts RecordListOptions) (int, error) {
	query := `SELECT COUNT(*) FROM config_backup_records WHERE 1=1`
	var args []interface{}
	if opts.TaskID != "" {
		query += " AND task_id = ?"
		args = append(args, opts.TaskID)
	}
	if opts.DeviceID != "" {
		query += " AND device_id = ?"
		args = append(args, opts.DeviceID)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	var c int
	err := appdb.DB().QueryRow(query, args...).Scan(&c)
	return c, err
}

// GetByID 获取单条记录
func (r *ConfigBackupRecordRepo) GetByID(id string) (*ConfigBackupRecord, error) {
	row := appdb.DB().QueryRow(`SELECT id, task_id, task_name, device_id, device_name, device_ip, device_vendor, template_id, template_name, status, commands, output, output_size, duration_ms, error_message, started_at, completed_at FROM config_backup_records WHERE id = ?`, id)
	var rec ConfigBackupRecord
	var taskID, taskName, templateID, templateName, commands, output, errMsg, completedAt sql.NullString
	var durationMs sql.NullInt64
	err := row.Scan(&rec.ID, &taskID, &taskName, &rec.DeviceID, &rec.DeviceName, &rec.DeviceIP, &rec.DeviceVendor,
		&templateID, &templateName, &rec.Status, &commands, &output, &rec.OutputSize, &durationMs, &errMsg, &rec.StartedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	rec.TaskID = ns(taskID)
	rec.TaskName = ns(taskName)
	rec.TemplateID = ns(templateID)
	rec.TemplateName = ns(templateName)
	rec.Commands = ns(commands)
	rec.Output = ns(output)
	rec.ErrorMessage = ns(errMsg)
	rec.CompletedAt = ns(completedAt)
	if durationMs.Valid {
		d := int(durationMs.Int64)
		rec.DurationMs = &d
	}
	return &rec, nil
}

// CreateRecordInput 创建记录输入
type CreateRecordInput struct {
	ID           string
	TaskID       string
	TaskName     string
	DeviceID     string
	DeviceName   string
	DeviceIP     string
	DeviceVendor string
	TemplateID   string
	TemplateName string
	Status       string
	Commands     string
	Output       string
	OutputSize   int
	DurationMs   int
	ErrorMessage string
}

// Create 创建备份记录
func (r *ConfigBackupRecordRepo) Create(input CreateRecordInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	var durationVal interface{}
	if input.DurationMs > 0 {
		durationVal = input.DurationMs
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO config_backup_records (id, task_id, task_name, device_id, device_name, device_ip, device_vendor, template_id, template_name, status, commands, output, output_size, duration_ms, error_message, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, sqlNullStr(input.TaskID), sqlNullStr(input.TaskName),
		input.DeviceID, input.DeviceName, input.DeviceIP, input.DeviceVendor,
		sqlNullStr(input.TemplateID), sqlNullStr(input.TemplateName),
		input.Status, sqlNullStr(input.Commands), sqlNullStr(input.Output),
		input.OutputSize, durationVal, sqlNullStr(input.ErrorMessage), now)
	return err
}

// RecordResultInput 结果更新输入
type RecordResultInput struct {
	Status       string
	Output       string
	OutputSize   int
	DurationMs   int
	ErrorMessage string
}

// UpdateResult 更新记录结果
func (r *ConfigBackupRecordRepo) UpdateResult(id string, result RecordResultInput) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	var sets []string
	var args []interface{}
	sets = append(sets, "status = ?")
	args = append(args, result.Status)
	sets = append(sets, "completed_at = ?")
	args = append(args, now)
	if result.Output != "" {
		sets = append(sets, "output = ?")
		args = append(args, result.Output)
	}
	if result.OutputSize > 0 {
		sets = append(sets, "output_size = ?")
		args = append(args, result.OutputSize)
	}
	if result.DurationMs > 0 {
		sets = append(sets, "duration_ms = ?")
		args = append(args, result.DurationMs)
	}
	if result.ErrorMessage != "" {
		sets = append(sets, "error_message = ?")
		args = append(args, result.ErrorMessage)
	}
	args = append(args, id)
	_, err := appdb.DB().Exec("UPDATE config_backup_records SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// DeleteOlderThan 删除过期记录
func (r *ConfigBackupRecordRepo) DeleteOlderThan(taskID, beforeDate string) (int, error) {
	res, err := appdb.DB().Exec(`DELETE FROM config_backup_records WHERE task_id = ? AND started_at < ?`, taskID, beforeDate)
	if err != nil {
		return 0, err
	}
	c, _ := res.RowsAffected()
	return int(c), nil
}

// Delete 删除单条记录
func (r *ConfigBackupRecordRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM config_backup_records WHERE id = ?`, id)
	return err
}

// DeleteByTaskID 删除任务的全部记录
func (r *ConfigBackupRecordRepo) DeleteByTaskID(taskID string) (int, error) {
	res, err := appdb.DB().Exec(`DELETE FROM config_backup_records WHERE task_id = ?`, taskID)
	if err != nil {
		return 0, err
	}
	c, _ := res.RowsAffected()
	return int(c), nil
}

// GetLatestForDevice 获取设备最新成功备份
func (r *ConfigBackupRecordRepo) GetLatestForDevice(deviceID string) (*ConfigBackupRecord, error) {
	row := appdb.DB().QueryRow(`SELECT id, task_id, task_name, device_id, device_name, device_ip, device_vendor, template_id, template_name, status, commands, output, output_size, duration_ms, error_message, started_at, completed_at FROM config_backup_records WHERE device_id = ? AND status = 'success' ORDER BY started_at DESC LIMIT 1`, deviceID)
	var rec ConfigBackupRecord
	var taskID, taskName, templateID, templateName, commands, output, errMsg, completedAt sql.NullString
	var durationMs sql.NullInt64
	err := row.Scan(&rec.ID, &taskID, &taskName, &rec.DeviceID, &rec.DeviceName, &rec.DeviceIP, &rec.DeviceVendor,
		&templateID, &templateName, &rec.Status, &commands, &output, &rec.OutputSize, &durationMs, &errMsg, &rec.StartedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	rec.TaskID = ns(taskID)
	rec.TaskName = ns(taskName)
	rec.TemplateID = ns(templateID)
	rec.TemplateName = ns(templateName)
	rec.Commands = ns(commands)
	rec.Output = ns(output)
	rec.ErrorMessage = ns(errMsg)
	rec.CompletedAt = ns(completedAt)
	if durationMs.Valid {
		d := int(durationMs.Int64)
		rec.DurationMs = &d
	}
	return &rec, nil
}

func scanBackupRecords(rows *sql.Rows) ([]*ConfigBackupRecord, error) {
	var out []*ConfigBackupRecord
	for rows.Next() {
		var rec ConfigBackupRecord
		var taskID, taskName, templateID, templateName, commands, output, errMsg, completedAt sql.NullString
		var durationMs sql.NullInt64
		if err := rows.Scan(&rec.ID, &taskID, &taskName, &rec.DeviceID, &rec.DeviceName, &rec.DeviceIP, &rec.DeviceVendor,
			&templateID, &templateName, &rec.Status, &commands, &output, &rec.OutputSize, &durationMs, &errMsg, &rec.StartedAt, &completedAt); err != nil {
			continue
		}
		rec.TaskID = ns(taskID)
		rec.TaskName = ns(taskName)
		rec.TemplateID = ns(templateID)
		rec.TemplateName = ns(templateName)
		rec.Commands = ns(commands)
		rec.Output = ns(output)
		rec.ErrorMessage = ns(errMsg)
		rec.CompletedAt = ns(completedAt)
		if durationMs.Valid {
			d := int(durationMs.Int64)
			rec.DurationMs = &d
		}
		out = append(out, &rec)
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 辅助函数
// ───────────────────────────────────────────────

// computeNextRun 计算下次运行时间（与 TS 逻辑一致）
func computeNextRun(scheduleType, scheduleCron string) string {
	switch scheduleType {
	case "manual":
		return ""
	case "daily":
		return nowLocalPlusDays(1, 3, 0, 0)
	case "weekly":
		return nowLocalNextMonday(3, 0, 0)
	case "monthly":
		return nowLocalNextMonthFirst(3, 0, 0)
	case "cron":
		if scheduleCron == "" {
			return ""
		}
		// placeholder: +1 day
		return nowLocalPlusDays(1, 0, 0, 0)
	default:
		return ""
	}
}

// nowLocalPlusDays 返回 N 天后的指定时间
func nowLocalPlusDays(days, hour, min, sec int) string {
	t := time.Now().AddDate(0, 0, days)
	t = time.Date(t.Year(), t.Month(), t.Day(), hour, min, sec, 0, time.Local)
	return t.Format("2006-01-02 15:04:05")
}

// nowLocalNextMonday 返回下周一的指定时间
func nowLocalNextMonday(hour, min, sec int) string {
	t := time.Now()
	daysUntilMonday := (int(time.Monday) - int(t.Weekday()) + 7) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	t = t.AddDate(0, 0, daysUntilMonday)
	t = time.Date(t.Year(), t.Month(), t.Day(), hour, min, sec, 0, time.Local)
	return t.Format("2006-01-02 15:04:05")
}

// nowLocalNextMonthFirst 返回下月1号的指定时间
func nowLocalNextMonthFirst(hour, min, sec int) string {
	t := time.Now()
	t = time.Date(t.Year(), t.Month()+1, 1, hour, min, sec, 0, time.Local)
	return t.Format("2006-01-02 15:04:05")
}
