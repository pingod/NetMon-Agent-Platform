// 配置备份服务 — 模板/任务/记录管理 + 定时调度 + SSH 备份执行
// 等价于 TS configBackupService.ts
package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/logger"
	"itops-backend/internal/modules/network/vendoradapter"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 类型定义
// ───────────────────────────────────────────────

// BackupDeviceResult 备份结果
type BackupDeviceResult struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	DeviceIP   string `json:"deviceIp"`
	Status     string `json:"status"` // success | failed
	Output     string `json:"output,omitempty"`
	OutputSize int64  `json:"outputSize,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ───────────────────────────────────────────────
// 服务单例
// ───────────────────────────────────────────────

var ConfigBackupServiceInstance = &ConfigBackupService{}

type ConfigBackupService struct {
	mu            sync.Mutex
	scheduledJobs map[string]*time.Timer
}

// Init 初始化定时任务
func (s *ConfigBackupService) Init() {
	s.scheduledJobs = make(map[string]*time.Timer)
	tasks, err := repos.ConfigBackupTasks.ListEnabled()
	if err != nil {
		logger.Errorf("加载备份任务失败: %v", err)
		return
	}
	for _, task := range tasks {
		if task.ScheduleType != "manual" {
			s.scheduleTask(task)
		}
	}
	logger.Infof("配置备份服务启动: 已注册 %d 个定时任务", len(s.scheduledJobs))
}

// Shutdown 关闭定时任务
func (s *ConfigBackupService) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.scheduledJobs {
		job.Stop()
		logger.Debugf("取消定时备份任务: %s", id)
	}
	s.scheduledJobs = make(map[string]*time.Timer)
	logger.Infof("配置备份服务已关闭")
}

// ───────────────────────────────────────────────
// 模板管理
// ───────────────────────────────────────────────

// ListTemplates 列出所有备份模板
func (s *ConfigBackupService) ListTemplates() ([]*repos.ConfigBackupTemplate, error) {
	return repos.ConfigBackupTemplates.List()
}

// GetTemplate 获取单个模板
func (s *ConfigBackupService) GetTemplate(id string) (*repos.ConfigBackupTemplate, error) {
	return repos.ConfigBackupTemplates.GetByID(id)
}

// CreateTemplate 创建模板
func (s *ConfigBackupService) CreateTemplate(input repos.TemplateCreateInput) (*repos.ConfigBackupTemplate, error) {
	id := uuid.New().String()
	input.ID = id
	if err := repos.ConfigBackupTemplates.Create(input); err != nil {
		return nil, err
	}
	return repos.ConfigBackupTemplates.GetByID(id)
}

// UpdateTemplate 更新模板
func (s *ConfigBackupService) UpdateTemplate(id string, fields repos.TemplateUpdateInput) (*repos.ConfigBackupTemplate, error) {
	if err := repos.ConfigBackupTemplates.Update(id, fields); err != nil {
		return nil, err
	}
	return repos.ConfigBackupTemplates.GetByID(id)
}

// DeleteTemplate 删除模板
func (s *ConfigBackupService) DeleteTemplate(id string) error {
	// 检查是否被任务引用
	tasks, _ := repos.ConfigBackupTasks.List()
	for _, t := range tasks {
		if t.TemplateID == id {
			return fmt.Errorf("该模板正在被备份任务使用，无法删除")
		}
	}
	return repos.ConfigBackupTemplates.Delete(id)
}

// ───────────────────────────────────────────────
// 任务管理
// ───────────────────────────────────────────────

// ListTasks 列出所有备份任务
func (s *ConfigBackupService) ListTasks() ([]*repos.ConfigBackupTask, error) {
	return repos.ConfigBackupTasks.List()
}

// GetTask 获取单个任务
func (s *ConfigBackupService) GetTask(id string) (*repos.ConfigBackupTask, error) {
	return repos.ConfigBackupTasks.GetByID(id)
}

// CreateTask 创建任务
func (s *ConfigBackupService) CreateTask(input repos.TaskCreateInput) (*repos.ConfigBackupTask, error) {
	// 验证模板存在
	if input.TemplateID != "" {
		_, err := repos.ConfigBackupTemplates.GetByID(input.TemplateID)
		if err != nil {
			return nil, fmt.Errorf("模板不存在")
		}
	}

	// 验证设备存在
	for _, deviceID := range input.DeviceIDs {
		_, err := repos.NetworkDevices.GetByID(deviceID)
		if err != nil {
			return nil, fmt.Errorf("设备不存在: %s", deviceID)
		}
	}

	id := uuid.New().String()
	input.ID = id
	if err := repos.ConfigBackupTasks.Create(input); err != nil {
		return nil, err
	}

	task, _ := repos.ConfigBackupTasks.GetByID(id)
	// 注册调度
	if task != nil && task.ScheduleType != "manual" && task.Enabled == 1 {
		s.scheduleTask(task)
	}
	return task, nil
}

// UpdateTask 更新任务
func (s *ConfigBackupService) UpdateTask(id string, fields repos.TaskUpdateInput) (*repos.ConfigBackupTask, error) {
	if err := repos.ConfigBackupTasks.Update(id, fields); err != nil {
		return nil, err
	}
	task, _ := repos.ConfigBackupTasks.GetByID(id)

	// 重新注册调度
	s.unscheduleTask(id)
	if task != nil && task.ScheduleType != "manual" && task.Enabled == 1 {
		s.scheduleTask(task)
	}
	return task, nil
}

// DeleteTask 删除任务
func (s *ConfigBackupService) DeleteTask(id string) error {
	s.unscheduleTask(id)
	_, _ = repos.ConfigBackupRecords.DeleteByTaskID(id)
	return repos.ConfigBackupTasks.Delete(id)
}

// ToggleTask 切换任务启用状态
func (s *ConfigBackupService) ToggleTask(id string) (*repos.ConfigBackupTask, error) {
	task, err := repos.ConfigBackupTasks.GetByID(id)
	if err != nil || task == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	newEnabled := task.Enabled != 1
	return s.UpdateTask(id, repos.TaskUpdateInput{Enabled: &newEnabled})
}

// ───────────────────────────────────────────────
// 备份执行
// ───────────────────────────────────────────────

// BackupProgress 备份进度信息
type BackupProgress struct {
	TaskID    string `json:"task_id"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Success   int    `json:"success"`
	Failed    int    `json:"failed"`
	Running   int    `json:"running"`
	IsRunning bool   `json:"is_running"`
}

// backupProgressMap 记录每个任务的实时进度
var (
	backupProgressMu  sync.Mutex
	backupProgressMap = make(map[string]*BackupProgress)
)

// GetBackupProgress 获取备份执行进度（返回副本，避免调用方读到并发修改中的结构体）
func GetBackupProgress(taskID string) *BackupProgress {
	backupProgressMu.Lock()
	defer backupProgressMu.Unlock()
	if p, ok := backupProgressMap[taskID]; ok {
		snapshot := *p
		return &snapshot
	}
	return &BackupProgress{TaskID: taskID, IsRunning: false}
}

// ExecuteTask 手动执行备份任务（异步）
func (s *ConfigBackupService) ExecuteTask(taskID string) (*BackupProgress, error) {
	task, err := repos.ConfigBackupTasks.GetByID(taskID)
	if err != nil || task == nil {
		return nil, fmt.Errorf("任务不存在")
	}

	var template *repos.ConfigBackupTemplate
	if task.TemplateID != "" {
		template, err = repos.ConfigBackupTemplates.GetByID(task.TemplateID)
		if err != nil || template == nil {
			return nil, fmt.Errorf("模板不存在")
		}
	}

	deviceIDs := task.DeviceIDs
	if len(deviceIDs) == 0 {
		return nil, fmt.Errorf("任务没有关联设备")
	}

	commands := []string{}
	if template != nil {
		commands = template.Commands
	}
	// 如果模板没有配置指令，尝试使用通用默认备份命令
	// 在 backupDevice 中会根据设备厂商自动选择正确的默认命令
	if len(commands) == 0 {
		logger.Infof("备份任务 %s: 模板无指令，将使用厂商默认备份命令", task.Name)
	}

	// 初始化进度
	progress := &BackupProgress{
		TaskID:    taskID,
		Total:     len(deviceIDs),
		IsRunning: true,
	}
	backupProgressMu.Lock()
	backupProgressMap[taskID] = progress
	backupProgressMu.Unlock()

	// 异步并发执行
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("备份任务 goroutine panic: %v", r)
				backupProgressMu.Lock()
				progress.IsRunning = false
				backupProgressMu.Unlock()
			}
		}()

		workerCount := 5
		if len(deviceIDs) < workerCount {
			workerCount = len(deviceIDs)
		}

		type devTask struct {
			index int
			id    string
		}
		taskCh := make(chan devTask, len(deviceIDs))
		for i, dID := range deviceIDs {
			taskCh <- devTask{index: i, id: dID}
		}
		close(taskCh)

		var wg sync.WaitGroup
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range taskCh {
					logger.Infof("备份任务 %s: 开始备份设备 (%d/%d) %s", task.Name, item.index+1, len(deviceIDs), item.id)

					resultCh := make(chan BackupDeviceResult, 1)
					go func(dID string) {
						defer func() {
							if r := recover(); r != nil {
								logger.Errorf("backupDevice panic: %v", r)
								resultCh <- BackupDeviceResult{DeviceID: dID, Status: "failed", Error: fmt.Sprintf("内部错误: %v", r)}
							}
						}()
						resultCh <- s.backupDevice(dID, commands, task, template)
					}(item.id)

					var result BackupDeviceResult
					select {
					case result = <-resultCh:
					case <-time.After(240 * time.Second):
						result = BackupDeviceResult{
							DeviceID: item.id,
							Status:   "failed",
							Error:    "备份超时(240s)，exec+shell两种模式均超时",
						}
						logger.Warnf("备份任务 %s: 设备 %s 备份超时", task.Name, item.id)
					}

					backupProgressMu.Lock()
					progress.Completed++
					if result.Status == "success" {
						progress.Success++
					} else {
						progress.Failed++
					}
					backupProgressMu.Unlock()
					logger.Infof("备份任务 %s: 设备 %s 完成, 状态=%s", task.Name, result.DeviceName, result.Status)
				}
			}()
		}
		wg.Wait()

		// 更新任务最后执行状态
		backupProgressMu.Lock()
		allSuccess := progress.Success == progress.Total
		anySuccess := progress.Success > 0
		taskStatus := "failed"
		if allSuccess {
			taskStatus = "success"
		} else if anySuccess {
			taskStatus = "partial"
		}
		progress.IsRunning = false
		backupProgressMu.Unlock()
		_ = repos.ConfigBackupTasks.UpdateRunResult(taskID, taskStatus)

		// 清理过期记录
		if task.RetentionDays > 0 {
			beforeDate := time.Now().AddDate(0, 0, -task.RetentionDays).Format("2006-01-02 15:04:05")
			deleted, _ := repos.ConfigBackupRecords.DeleteOlderThan(taskID, beforeDate)
			if deleted > 0 {
				logger.Infof("清理过期备份记录: %d 条 (任务: %s)", deleted, task.Name)
			}
		}

		logger.Infof("备份任务 %s 执行完成: 成功=%d, 失败=%d", task.Name, progress.Success, progress.Failed)
	}()

	return progress, nil
}

// BackupDeviceManual 手动备份单个设备
// templateID 为空时自动使用厂商默认备份命令
func (s *ConfigBackupService) BackupDeviceManual(deviceID string, templateID string) (*BackupDeviceResult, error) {
	var template *repos.ConfigBackupTemplate
	var commands []string

	if templateID != "" {
		tpl, err := repos.ConfigBackupTemplates.GetByID(templateID)
		if err != nil || tpl == nil {
			return nil, fmt.Errorf("模板不存在")
		}
		template = tpl
		commands = tpl.Commands
	}

	// 模板指令为空时使用厂商默认备份命令
	if len(commands) == 0 {
		// 获取设备厂商
		device, err := repos.NetworkDevices.GetByID(deviceID)
		if err != nil || device == nil {
			return nil, fmt.Errorf("设备不存在")
		}
		commands = vendoradapter.DefaultBackupCommands(vendoradapter.VendorType(device.Vendor))
		logger.Infof("BackupDeviceManual: 使用厂商 %s 默认备份命令: %v", device.Vendor, commands)
	}

	result := s.backupDevice(deviceID, commands, nil, template)
	return &result, nil
}

// BatchBackup 批量手动备份（异步并发）
// templateID 为空时自动使用各设备厂商的默认备份命令
func (s *ConfigBackupService) BatchBackup(deviceIDs []string, templateID string) (*BackupProgress, error) {
	var template *repos.ConfigBackupTemplate
	var commands []string

	if templateID != "" {
		tpl, err := repos.ConfigBackupTemplates.GetByID(templateID)
		if err != nil || tpl == nil {
			return nil, fmt.Errorf("模板不存在")
		}
		template = tpl
		commands = tpl.Commands
	}

	// 模板指令为空时，批量备份中使用厂商默认命令
	if len(commands) == 0 {
		logger.Infof("BatchBackup: 模板无指令，将使用各设备厂商默认备份命令")
	}

	// 用一个虚拟的 batch ID 作为进度追踪
	batchID := "batch-" + uuid.New().String()[:8]
	progress := &BackupProgress{
		TaskID:    batchID,
		Total:     len(deviceIDs),
		IsRunning: true,
	}
	backupProgressMu.Lock()
	backupProgressMap[batchID] = progress
	backupProgressMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("批量备份 goroutine panic: %v", r)
				backupProgressMu.Lock()
				progress.IsRunning = false
				backupProgressMu.Unlock()
			}
		}()

		workerCount := 5
		if len(deviceIDs) < workerCount {
			workerCount = len(deviceIDs)
		}

		type devTask struct {
			index int
			id    string
		}
		taskCh := make(chan devTask, len(deviceIDs))
		for i, dID := range deviceIDs {
			taskCh <- devTask{index: i, id: dID}
		}
		close(taskCh)

		var wg sync.WaitGroup
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range taskCh {
					logger.Infof("批量备份: 开始备份设备 (%d/%d) %s", item.index+1, len(deviceIDs), item.id)

					resultCh := make(chan BackupDeviceResult, 1)
					go func(dID string) {
						defer func() {
							if r := recover(); r != nil {
								logger.Errorf("backupDevice panic: %v", r)
								resultCh <- BackupDeviceResult{DeviceID: dID, Status: "failed", Error: fmt.Sprintf("内部错误: %v", r)}
							}
						}()
						resultCh <- s.backupDevice(dID, template.Commands, nil, template)
					}(item.id)

					var result BackupDeviceResult
					select {
					case result = <-resultCh:
					case <-time.After(240 * time.Second):
						result = BackupDeviceResult{
							DeviceID: item.id,
							Status:   "failed",
							Error:    "备份超时(240s)，exec+shell两种模式均超时",
						}
						logger.Warnf("批量备份: 设备 %s 备份超时", item.id)
					}

					backupProgressMu.Lock()
					progress.Completed++
					if result.Status == "success" {
						progress.Success++
					} else {
						progress.Failed++
					}
					backupProgressMu.Unlock()
					logger.Infof("批量备份: 设备 %s 完成, 状态=%s", result.DeviceName, result.Status)
				}
			}()
		}
		wg.Wait()

		backupProgressMu.Lock()
		progress.IsRunning = false
		backupProgressMu.Unlock()
		logger.Infof("批量备份执行完成: 成功=%d, 失败=%d", progress.Success, progress.Failed)
	}()

	return progress, nil
}

// backupDevice 备份单个设备 — 核心执行逻辑
func (s *ConfigBackupService) backupDevice(deviceID string, commands []string, task *repos.ConfigBackupTask, template *repos.ConfigBackupTemplate) BackupDeviceResult {
	start := time.Now()
	recordID := uuid.New().String()

	logger.Infof("backupDevice: 获取设备凭据 %s", deviceID)
	device, err := NetworkDeviceServiceInstance.GetFullCredentials(deviceID)
	if err != nil || device == nil {
		logger.Errorf("backupDevice: 获取设备 %s 凭据失败: %v", deviceID, err)
		return BackupDeviceResult{
			DeviceID:   deviceID,
			DeviceName: "未知",
			Status:     "failed",
			Error:      "设备不存在",
		}
	}
	logger.Infof("backupDevice: 设备 %s (%s), 准备SSH连接", device.Name, device.IPAddress)

	// 如果命令列表为空，使用厂商默认备份命令
	if len(commands) == 0 && device.Vendor != "" {
		commands = vendoradapter.DefaultBackupCommands(vendoradapter.VendorType(device.Vendor))
		logger.Infof("backupDevice: %s 使用厂商 %s 默认备份命令: %v", device.Name, device.Vendor, commands)
	}

	// 创建备份记录
	taskID := ""
	taskName := ""
	if task != nil {
		taskID = task.ID
		taskName = task.Name
	}
	templateID := ""
	templateName := ""
	if template != nil {
		templateID = template.ID
		templateName = template.Name
	}
	commandsJSON, _ := json.Marshal(commands)
	_ = repos.ConfigBackupRecords.Create(repos.CreateRecordInput{
		ID:           recordID,
		TaskID:       taskID,
		TaskName:     taskName,
		DeviceID:     deviceID,
		DeviceName:   device.Name,
		DeviceIP:     device.IPAddress,
		DeviceVendor: device.Vendor,
		TemplateID:   templateID,
		TemplateName: templateName,
		Status:       "running",
		Commands:     string(commandsJSON),
	})

	// SSH 配置
	sshDevice := SSHDeviceConfig{
		Vendor:         device.Vendor,
		Host:           device.IPAddress,
		Port:           device.SSHPort,
		Username:       device.Username,
		Password:       device.Password,
		EnablePassword: device.EnablePassword,
		PrivateKey:     device.PrivateKey,
		AuthType:       device.AuthType,
	}

	// 策略：先 shell 模式（支持分页禁用和提示符检测），失败再 fallback 到 exec 模式
	// shell 模式发送两种厂商的分页禁用命令，避免厂商标记错误

	// 尝试1: Shell 模式（60s超时，支持分页禁用和enable模式）
	logger.Infof("backupDevice: %s 尝试 shell 模式备份", device.Name)
	output, err := ConnectAndRunShellWithTimeout(sshDevice, commands, 60*time.Second)
	shellOutput := output

	if err != nil {
		logger.Warnf("backupDevice: %s shell 模式失败(%v)，尝试 exec 模式", device.Name, err)
		// 尝试2: exec 模式 fallback
		execOutput, execErr := ConnectAndExecCommands(sshDevice, commands)
		if execErr == nil && len(strings.TrimSpace(execOutput)) > 0 {
			output = execOutput
			err = nil
			logger.Infof("backupDevice: %s exec 模式成功", device.Name)
		} else {
			// 两种模式都失败，保留有内容的输出
			logger.Warnf("backupDevice: %s exec 模式也失败(%v)", device.Name, execErr)
			if len(shellOutput) > len(execOutput) {
				output = shellOutput
			} else if len(execOutput) > 0 {
				output = execOutput
			}
			if execErr != nil {
				err = fmt.Errorf("shell模式: %v; exec模式: %v", err, execErr)
			}
		}
	}

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		errMsg := err.Error()
		// 失败时也保存部分输出（便于排查）
		recordUpdate := repos.RecordResultInput{
			Status:       "failed",
			DurationMs:   int(durationMs),
			ErrorMessage: errMsg,
		}
		if len(output) > 0 {
			recordUpdate.Output = output
			recordUpdate.OutputSize = int(len(output))
		}
		_ = repos.ConfigBackupRecords.UpdateResult(recordID, recordUpdate)
		logger.Errorf("配置备份失败: %s (%s): %s", device.Name, device.IPAddress, errMsg)
		return BackupDeviceResult{
			DeviceID:   deviceID,
			DeviceName: device.Name,
			DeviceIP:   device.IPAddress,
			Status:     "failed",
			Error:      errMsg,
			DurationMs: durationMs,
		}
	}

	outputSize := int64(len(output))
	_ = repos.ConfigBackupRecords.UpdateResult(recordID, repos.RecordResultInput{
		Status:     "success",
		Output:     output,
		OutputSize: int(outputSize),
		DurationMs: int(durationMs),
	})
	logger.Infof("配置备份成功: %s (%s), %d bytes, %dms", device.Name, device.IPAddress, outputSize, durationMs)

	return BackupDeviceResult{
		DeviceID:   deviceID,
		DeviceName: device.Name,
		DeviceIP:   device.IPAddress,
		Status:     "success",
		Output:     output,
		OutputSize: outputSize,
		DurationMs: durationMs,
	}
}

// ───────────────────────────────────────────────
// 记录查询
// ───────────────────────────────────────────────

// ListRecords 列出备份记录
func (s *ConfigBackupService) ListRecords(taskID, deviceID, status string, limit, offset int) ([]*repos.ConfigBackupRecord, error) {
	return repos.ConfigBackupRecords.List(repos.RecordListOptions{
		TaskID:   taskID,
		DeviceID: deviceID,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
	})
}

// GetRecord 获取单条记录
func (s *ConfigBackupService) GetRecord(id string) (*repos.ConfigBackupRecord, error) {
	return repos.ConfigBackupRecords.GetByID(id)
}

// DeleteRecord 删除记录
func (s *ConfigBackupService) DeleteRecord(id string) error {
	return repos.ConfigBackupRecords.Delete(id)
}

// GetRecordCount 获取记录数
func (s *ConfigBackupService) GetRecordCount(taskID, deviceID, status string) (int, error) {
	return repos.ConfigBackupRecords.Count(repos.RecordListOptions{
		TaskID:   taskID,
		DeviceID: deviceID,
		Status:   status,
	})
}

// ───────────────────────────────────────────────
// 调度管理
// ───────────────────────────────────────────────

// scheduleTask 注册定时任务
func (s *ConfigBackupService) scheduleTask(task *repos.ConfigBackupTask) {
	s.unscheduleTask(task.ID)

	var duration time.Duration
	switch task.ScheduleType {
	case "daily":
		// 每天 3:00 执行
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		duration = next.Sub(now)
	case "weekly":
		// 每周一 3:00 执行
		now := time.Now()
		daysUntilMonday := (1 - int(now.Weekday()) + 7) % 7
		if daysUntilMonday == 0 && now.Hour() >= 3 {
			daysUntilMonday = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 3, 0, 0, 0, now.Location())
		duration = next.Sub(now)
	case "monthly":
		// 每月 1 号 3:00 执行
		now := time.Now()
		next := time.Date(now.Year(), now.Month()+1, 1, 3, 0, 0, 0, now.Location())
		duration = next.Sub(now)
	case "cron":
		if task.ScheduleCron == nil || strings.TrimSpace(*task.ScheduleCron) == "" {
			logger.Warnf("定时备份任务 %s 是 cron 类型但没有 cron 表达式，跳过调度", task.ID)
			return
		}
		d, err := nextCronDuration(*task.ScheduleCron, time.Now())
		if err != nil {
			// 无法解析的表达式不能静默按 24h 执行，记录告警并跳过
			logger.Warnf("定时备份任务 %s 的 cron 表达式 %q 无法解析: %v，跳过调度", task.ID, *task.ScheduleCron, err)
			return
		}
		duration = d
	default:
		return
	}
	s.mu.Lock()
	timer := time.AfterFunc(duration, func() {
		logger.Infof("定时备份任务触发: %s (%s)", task.Name, task.ID)
		_, err := s.ExecuteTask(task.ID)
		if err != nil {
			logger.Errorf("定时备份任务执行失败: %s - %v", task.Name, err)
		}
		// 重新调度下一个周期
		updatedTask, _ := repos.ConfigBackupTasks.GetByID(task.ID)
		if updatedTask != nil && updatedTask.Enabled == 1 && updatedTask.ScheduleType != "manual" {
			s.scheduleTask(updatedTask)
		}
	})
	s.scheduledJobs[task.ID] = timer
	s.mu.Unlock()

	logger.Infof("注册定时备份任务: %s (%s), 下次执行: %s", task.Name, task.ScheduleType, duration.String())
}

// unscheduleTask 取消定时任务
func (s *ConfigBackupService) unscheduleTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timer, ok := s.scheduledJobs[taskID]; ok {
		timer.Stop()
		delete(s.scheduledJobs, taskID)
	}
}

// nextCronDuration 计算 cron 表达式距下一次执行的时间。
// 支持 @every <duration>、@hourly/@daily/@weekly 以及 "*/N * * * *" / "M * * * *" 这类分钟级表达式；
// 其它无法可靠解析的表达式返回错误，由调用方记录告警并跳过（不再默认 24h）。
func nextCronDuration(expr string, now time.Time) (time.Duration, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, fmt.Errorf("empty cron expression")
	}

	if strings.HasPrefix(expr, "@") {
		switch {
		case strings.HasPrefix(expr, "@every "):
			d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(expr, "@every ")))
			if err != nil || d <= 0 {
				return 0, fmt.Errorf("invalid @every duration: %q", expr)
			}
			return d, nil
		case expr == "@hourly":
			return now.Truncate(time.Hour).Add(time.Hour).Sub(now), nil
		case expr == "@daily" || expr == "@midnight":
			next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
			return next.Sub(now), nil
		case expr == "@weekly":
			daysUntilMonday := (1 - int(now.Weekday()) + 7) % 7
			if daysUntilMonday == 0 {
				daysUntilMonday = 7
			}
			next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 0, 0, 0, 0, now.Location())
			return next.Sub(now), nil
		default:
			return 0, fmt.Errorf("unsupported cron macro: %q", expr)
		}
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return 0, fmt.Errorf("unsupported cron expression (expect 5 fields): %q", expr)
	}
	for _, f := range fields[1:] {
		if f != "*" {
			return 0, fmt.Errorf("unsupported cron expression (only minute-level supported): %q", expr)
		}
	}

	minuteField := fields[0]
	if strings.HasPrefix(minuteField, "*/") {
		step, err := strconv.Atoi(strings.TrimPrefix(minuteField, "*/"))
		if err != nil || step <= 0 || step > 60 {
			return 0, fmt.Errorf("invalid minute step in cron expression: %q", expr)
		}
		next := now.Truncate(time.Minute).Add(time.Duration(step) * time.Minute)
		return next.Sub(now), nil
	}
	fixedMin, err := strconv.Atoi(minuteField)
	if err != nil || fixedMin < 0 || fixedMin > 59 {
		return 0, fmt.Errorf("invalid minute field in cron expression: %q", expr)
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), fixedMin, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(time.Hour)
	}
	return next.Sub(now), nil
}

// commandsToString 工具函数
func commandsToString(commands []string) string {
	return strings.Join(commands, "\n")
}
