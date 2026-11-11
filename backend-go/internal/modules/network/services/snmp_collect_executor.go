// SNMP 采集任务执行器 + 周期调度器。
// 手动执行（POST /snmp-collect/tasks/{id}/execute）与 interval/cron 调度共用 ExecuteCollectTask。
package services

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
	"itops-backend/internal/utils"
)

const (
	collectDeviceConcurrency = 8                // 单任务内设备级并行上限
	collectDefaultTimeoutMs  = 5000             // 任务未配置超时时的默认值
	collectMaxTimeoutMs      = 30000            // 超时上限，避免手动执行长时间挂起
	collectMaxRetries        = 3                // 连接级重试上限
	collectSchedulerTick     = 15 * time.Second // 调度扫描间隔
	collectRetentionTick     = time.Hour        // 保留期清理间隔
)

type collectOidEntry struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
}

// ExecuteCollectTask 真实执行一次采集：逐设备并行采集任务 OID，结果批量入库并更新任务运行状态。
// 返回执行摘要。设备级错误（不存在/无 IP）也会落一条错误结果，保证执行可见。
func ExecuteCollectTask(task *repos.CollectTask) map[string]interface{} {
	started := time.Now()
	summary := map[string]interface{}{
		"task_id": task.ID, "task_name": task.Name,
		"devices": 0, "devices_ok": 0, "devices_failed": 0,
		"oids": 0, "results": 0, "status": "failed",
	}

	var deviceIDs []string
	if err := json.Unmarshal([]byte(task.DeviceIDs), &deviceIDs); err != nil || len(deviceIDs) == 0 {
		summary["error"] = "任务未配置有效的目标设备"
		repos.CollectTasks.UpdateRunResult(task.ID, "failed")
		return summary
	}
	var oidEntries []collectOidEntry
	if err := json.Unmarshal([]byte(task.OIDs), &oidEntries); err != nil || len(oidEntries) == 0 {
		summary["error"] = "任务未配置有效的采集 OID"
		repos.CollectTasks.UpdateRunResult(task.ID, "failed")
		return summary
	}

	// MIB 库中标记为表列的 OID 走 WALK，其余走标量 GET
	tableOIDs := loadTableOIDSet()
	entries := make([]CustomOidEntry, 0, len(oidEntries))
	for _, e := range oidEntries {
		oid := strings.TrimSpace(e.OID)
		if oid == "" {
			continue
		}
		name := strings.TrimSpace(e.Name)
		if name == "" {
			name = oid
		}
		isTable := 0
		if tableOIDs[strings.TrimPrefix(oid, ".")] {
			isTable = 1
		}
		entries = append(entries, CustomOidEntry{OID: oid, MetricName: name, IsTable: isTable})
	}
	if len(entries) == 0 {
		summary["error"] = "任务未配置有效的采集 OID"
		repos.CollectTasks.UpdateRunResult(task.ID, "failed")
		return summary
	}

	timeoutMs := task.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = collectDefaultTimeoutMs
	}
	if timeoutMs > collectMaxTimeoutMs {
		timeoutMs = collectMaxTimeoutMs
	}
	retries := task.Retries
	if retries < 0 {
		retries = 0
	}
	if retries > collectMaxRetries {
		retries = collectMaxRetries
	}
	version := SnmpVersion(strings.TrimSpace(task.SnmpVersion))
	if version == "" {
		version = SNMPv2c
	}
	taskCommunity := ""
	if task.Community != nil {
		taskCommunity = strings.TrimSpace(*task.Community)
	}

	var (
		mu          sync.Mutex
		wg          sync.WaitGroup
		allResults  []repos.CollectResult
		devOK, devFail int
	)
	sem := make(chan struct{}, collectDeviceConcurrency)

	for _, devID := range deviceIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(devID string) {
			defer wg.Done()
			defer func() { <-sem }()

			devStart := time.Now()
			name, ip, err := repos.NetworkDevices.GetNameAndIP(devID)
			if err != nil || ip == "" {
				mu.Lock()
				devFail++
				allResults = append(allResults, repos.CollectResult{
					TaskID: task.ID, TaskName: &task.Name, DeviceID: devID,
					OID: "-", MetricName: "-", MetricValue: "N/A",
					ErrorMessage: strPtr("设备不存在或未配置 IP"), DurationMs: 0,
				})
				mu.Unlock()
				return
			}

			cred := SnmpServiceInstance.GetCredential(devID)
			port, community := 161, "public"
			devVersion := version
			var user, authProto, authKey, privProto, privKey string
			if cred != nil {
				if cred.SnmpPort > 0 {
					port = cred.SnmpPort
				}
				if cred.Community != "" {
					community = cred.Community
				}
				user, authProto, authKey = cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey
				privProto, privKey = cred.SnmpPrivProtocol, cred.SnmpPrivKey
				if task.SnmpVersion == "" && cred.SnmpVersion != "" {
					devVersion = cred.SnmpVersion
				}
			}
			// 任务级 community 显式配置时优先（创建任务时默认写入 public）
			if taskCommunity != "" {
				community = taskCommunity
			}

			var collected []CustomOidResult
			for attempt := 0; attempt <= retries; attempt++ {
				collected = SnmpServiceInstance.CollectCustomOids(ip, port, devVersion, community,
					user, authProto, authKey, privProto, privKey, entries, timeoutMs)
				if !allCustomOidsFailed(collected) {
					break
				}
			}
			duration := int(time.Since(devStart).Milliseconds())

			rows := make([]repos.CollectResult, 0, len(collected))
			deviceOK := false
			for _, c := range collected {
				row := repos.CollectResult{
					TaskID: task.ID, TaskName: &task.Name,
					DeviceID: devID, DeviceName: &name, DeviceIP: &ip,
					OID: c.OID, MetricName: c.MetricName, MetricValue: c.MetricValue,
					DurationMs: duration,
				}
				if c.Error != "" {
					row.ErrorMessage = strPtr(c.Error)
				} else {
					deviceOK = true
				}
				rows = append(rows, row)
			}
			mu.Lock()
			if deviceOK {
				devOK++
			} else {
				devFail++
			}
			allResults = append(allResults, rows...)
			mu.Unlock()
		}(devID)
	}
	wg.Wait()

	if len(allResults) > 0 {
		if err := repos.CollectResults.InsertBatch(allResults); err != nil {
			logger.Errorf("collect task %s: 结果入库失败: %v", task.ID, err)
			summary["error"] = "结果入库失败: " + err.Error()
			repos.CollectTasks.UpdateRunResult(task.ID, "failed")
			return summary
		}
	}

	status := "success"
	if devOK == 0 {
		status = "failed"
	} else if devFail > 0 {
		status = "partial"
	}
	repos.CollectTasks.UpdateRunResult(task.ID, status)

	summary["devices"] = len(deviceIDs)
	summary["devices_ok"] = devOK
	summary["devices_failed"] = devFail
	summary["oids"] = len(entries)
	summary["results"] = len(allResults)
	summary["status"] = status
	summary["duration_ms"] = int(time.Since(started).Milliseconds())
	return summary
}

func allCustomOidsFailed(results []CustomOidResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		if r.Error == "" {
			return false
		}
	}
	return true
}

// loadTableOIDSet 读取 MIB 库中全部表列 OID（去前导点），用于决定 GET/WALK
func loadTableOIDSet() map[string]bool {
	out := map[string]bool{}
	rows, err := appdb.OidDB().Query(`SELECT oid FROM mib_objects WHERE is_table = 1`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var oid string
		if rows.Scan(&oid) == nil {
			out[strings.TrimPrefix(oid, ".")] = true
		}
	}
	return out
}

// ───────────────────────────────────────────────
// 周期调度器
// ───────────────────────────────────────────────

var collectScheduler struct {
	once    sync.Once
	stopCh  chan struct{}
	running sync.Map // taskID → struct{}，防止同一任务重叠执行
}

// StartCollectScheduler 启动采集任务调度（interval/cron）与保留期清理，幂等
func StartCollectScheduler() {
	collectScheduler.once.Do(func() {
		collectScheduler.stopCh = make(chan struct{})
		go collectSchedulerLoop()
		logger.Infof("SNMP collect scheduler started (tick %s)", collectSchedulerTick)
	})
}

// StopCollectScheduler 停止调度循环
func StopCollectScheduler() {
	if collectScheduler.stopCh != nil {
		close(collectScheduler.stopCh)
	}
}

func collectSchedulerLoop() {
	ticker := time.NewTicker(collectSchedulerTick)
	defer ticker.Stop()
	retentionTicker := time.NewTicker(collectRetentionTick)
	defer retentionTicker.Stop()
	for {
		select {
		case <-collectScheduler.stopCh:
			return
		case <-retentionTicker.C:
			cleanExpiredCollectResults()
		case <-ticker.C:
			scheduleDueCollectTasks()
		}
	}
}

func scheduleDueCollectTasks() {
	tasks, err := repos.CollectTasks.ListEnabled()
	if err != nil {
		return
	}
	now := time.Now()
	for i := range tasks {
		task := tasks[i]
		if task.ScheduleType != "interval" && task.ScheduleType != "cron" {
			continue
		}
		if _, busy := collectScheduler.running.LoadOrStore(task.ID, struct{}{}); busy {
			continue
		}
		due, next := collectTaskDue(&task, now)
		if next != "" {
			repos.CollectTasks.SetNextRun(task.ID, next)
		}
		if !due {
			collectScheduler.running.Delete(task.ID)
			continue
		}
		go func(t repos.CollectTask) {
			defer collectScheduler.running.Delete(t.ID)
			defer func() {
				if rec := recover(); rec != nil {
					logger.Errorf("collect task %s panic: %v", t.ID, rec)
					repos.CollectTasks.UpdateRunResult(t.ID, "failed")
				}
			}()
			summary := ExecuteCollectTask(&t)
			logger.Infof("collect task %s (%s) finished: %v", t.Name, t.ScheduleType, summary["status"])
		}(task)
	}
}

// collectTaskDue 判断任务是否到期，并返回下次触发时间（用于 UI 展示）
func collectTaskDue(task *repos.CollectTask, now time.Time) (bool, string) {
	lastRun := parseCollectTime(task.LastRunAt)
	switch task.ScheduleType {
	case "interval":
		interval := task.IntervalSeconds
		if interval < 10 {
			interval = 10 // 下限保护，避免打爆设备
		}
		if lastRun.IsZero() {
			return true, now.Add(time.Duration(interval) * time.Second).Format("2006-01-02 15:04:05")
		}
		next := lastRun.Add(time.Duration(interval) * time.Second)
		return !next.After(now), next.Format("2006-01-02 15:04:05")
	case "cron":
		if task.CronExpression == nil || strings.TrimSpace(*task.CronExpression) == "" {
			return false, ""
		}
		spec, err := utils.ParseCron(strings.TrimSpace(*task.CronExpression))
		if err != nil {
			logger.Warnf("collect task %s cron 表达式非法: %v", task.ID, err)
			return false, ""
		}
		base := lastRun
		if base.IsZero() {
			base = parseCollectTime(&task.CreatedAt)
			if base.IsZero() {
				base = now
			}
			// 从未运行：到点即跑（base 取创建时间，next 落在过去则立即执行）
		}
		next := spec.Next(base)
		if next.IsZero() {
			return false, ""
		}
		return !next.After(now), next.Format("2006-01-02 15:04:05")
	}
	return false, ""
}

// parseCollectTime 兼容 SQLite 的两种时间格式
func parseCollectTime(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, *s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func cleanExpiredCollectResults() {
	tasks, err := repos.CollectTasks.List()
	if err != nil {
		return
	}
	for _, t := range tasks {
		if t.RetentionDays <= 0 {
			continue
		}
		if n, err := repos.CollectResults.DeleteOlderThan(t.ID, t.RetentionDays); err == nil && n > 0 {
			logger.Infof("collect task %s: 清理过期采集结果 %d 条（保留 %d 天）", t.Name, n, t.RetentionDays)
		}
	}
}