// 网络设备巡检服务 — SSH 巡检 + 厂商适配器 + 结果解析
// 等价于 TS networkInspectionService.ts
package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/logger"
	"itops-backend/internal/modules/inspection"
	"itops-backend/internal/modules/network/vendoradapter"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 类型定义
// ───────────────────────────────────────────────

// InspectionResult 巡检结果
type InspectionResult struct {
	InspectionID     string         `json:"inspectionId"`
	DeviceID         string         `json:"deviceId"`
	InspectionType   string         `json:"inspectionType"` // standard | custom | full
	Status           string         `json:"status"`         // success | partial | failed
	Results          []ParsedResult `json:"results"`
	CommandsExecuted int            `json:"commandsExecuted"`
	CommandsFailed   int            `json:"commandsFailed"`
	DurationMs       int64          `json:"durationMs"`
	Summary          string         `json:"summary"`
	TemplateID       string         `json:"templateId,omitempty"`
	TemplateName     string         `json:"templateName,omitempty"`
}

// ───────────────────────────────────────────────
// 服务单例
// ───────────────────────────────────────────────

var NetworkInspectionServiceInstance = &NetworkInspectionService{}

type NetworkInspectionService struct{}

// InspectDevice 执行设备巡检
// inspectionType: standard | custom | full
// customTypes: 自定义巡检类型列表（inspectionType 为 custom 或 full 时使用）
func (s *NetworkInspectionService) InspectDevice(deviceID string, inspectionType string, customTypes []string) (*InspectionResult, error) {
	return s.InspectDeviceWithTemplate(deviceID, inspectionType, customTypes, "", nil)
}

// InspectDeviceWithTemplate 支持传入 templateID 与指定 commandNames 执行模版巡检（严格执行同品牌模版校验）
func (s *NetworkInspectionService) InspectDeviceWithTemplate(deviceID string, inspectionType string, customTypes []string, templateID string, selectedCommandNames []string) (*InspectionResult, error) {
	start := time.Now()
	inspectionID := uuid.New().String()

	if inspectionType == "" {
		inspectionType = "standard"
	}

	device, err := NetworkDeviceServiceInstance.GetFullCredentials(deviceID)
	if err != nil {
		return nil, fmt.Errorf("设备不存在: %s", deviceID)
	}

	// 创建巡检记录（running 状态）— repo 返回自己的 ID，使用 repo 的 ID 覆盖
	repoID, createErr := repos.Inspections.CreateRunning(deviceID, inspectionType, "")
	if createErr == nil && repoID != "" {
		inspectionID = repoID
	}

	var results []ParsedResult
	var commandsExecuted, commandsFailed int
	var tplName string

	// 1. 如果指定了模版 ID，则优先按模版执行
	if templateID != "" {
		tpl, tplErr := inspection.GetTemplateDetail(templateID)
		if tplErr != nil {
			return nil, fmt.Errorf("获取巡检模版失败: %w", tplErr)
		}
		tplName = tpl.Name

		// 品牌一致性严格校验：巡检的设备一定使用同个品牌的巡检模板
		devVendor := strings.ToLower(strings.TrimSpace(device.Vendor))
		tplVendor := strings.ToLower(strings.TrimSpace(tpl.Vendor))
		if tplVendor != "other" && devVendor != "" && tplVendor != "" && devVendor != tplVendor {
			return nil, fmt.Errorf("品牌不匹配：设备品牌为【%s】，无法使用【%s】的巡检模版（%s）", device.Vendor, tpl.Vendor, tpl.Name)
		}

		// 根据模版命令构建 CommandTemplate 列表
		var selectedMap map[string]bool
		if len(selectedCommandNames) > 0 {
			selectedMap = make(map[string]bool, len(selectedCommandNames))
			for _, name := range selectedCommandNames {
				selectedMap[strings.TrimSpace(name)] = true
			}
		}

		var cmdTemplates []vendoradapter.CommandTemplate
		for _, cmd := range tpl.Commands {
			if selectedMap != nil && !selectedMap[cmd.Name] && !selectedMap[cmd.Command] {
				continue
			}
			cmdTemplates = append(cmdTemplates, vendoradapter.CommandTemplate{
				Type:             vendoradapter.InspectionType(cmd.Type),
				Name:             cmd.Name,
				Command:          cmd.Command,
				FallbackCommands: cmd.Fallbacks,
				Description:      cmd.Description,
			})
		}

		if len(cmdTemplates) > 0 {
			templateResults := s.executeCommands(device, cmdTemplates)
			results = append(results, templateResults...)
			commandsExecuted += len(templateResults)
			for _, r := range templateResults {
				if r.Status == "error" {
					commandsFailed++
				}
			}
		}

		// 累加模版使用次数
		inspection.IncrementTemplateUsage(templateID)
	} else {
		// 2. 无模版 ID 时，按标准/自定义类型巡检
		if inspectionType == "standard" || inspectionType == "full" {
			standardResults := s.executeStandardInspection(device)
			results = append(results, standardResults...)
			commandsExecuted += len(standardResults)
			for _, r := range standardResults {
				if r.Status == "error" {
					commandsFailed++
				}
			}
		}

		if inspectionType == "custom" && len(customTypes) == 0 {
			customTypes = []string{"cpu", "memory", "interface", "routes", "log", "environment", "power", "fan"}
		}

		if (inspectionType == "custom" || inspectionType == "full") && len(customTypes) > 0 {
			customResults := s.executeCustomInspection(device, customTypes)
			results = append(results, customResults...)
			commandsExecuted += len(customResults)
			for _, r := range customResults {
				if r.Status == "error" {
					commandsFailed++
				}
			}
		}
	}

	durationMs := time.Since(start).Milliseconds()
	status := "success"
	if commandsFailed > 0 {
		if commandsFailed >= commandsExecuted/2 {
			status = "failed"
		} else {
			status = "partial"
		}
	}
	summary := s.generateSummary(results)

	// 更新巡检记录
	resultsJSON, _ := json.Marshal(results)
	_ = repos.Inspections.UpdateResult(inspectionID, status, commandsExecuted, commandsFailed, string(resultsJSON), summary, int(durationMs))

	// 更新设备最后巡检结果
	_ = repos.NetworkDevices.UpdateInspectionResult(deviceID, summary)

	return &InspectionResult{
		InspectionID:     inspectionID,
		DeviceID:         deviceID,
		InspectionType:   inspectionType,
		Status:           status,
		Results:          results,
		CommandsExecuted: commandsExecuted,
		CommandsFailed:   commandsFailed,
		DurationMs:       durationMs,
		Summary:          summary,
		TemplateID:       templateID,
		TemplateName:     tplName,
	}, nil
}

// BatchInspect 批量巡检（并行多线程并发执行）
func (s *NetworkInspectionService) BatchInspect(deviceIDs []string, inspectionType string, customTypes []string) []*InspectionResult {
	return s.BatchInspectWithTemplate(deviceIDs, inspectionType, customTypes, "", nil)
}

// BatchInspectWithTemplate 批量巡检支持指定模版
func (s *NetworkInspectionService) BatchInspectWithTemplate(deviceIDs []string, inspectionType string, customTypes []string, templateID string, selectedCommandNames []string, concurrency ...int) []*InspectionResult {
	if len(deviceIDs) == 0 {
		return []*InspectionResult{}
	}

	results := make([]*InspectionResult, len(deviceIDs))
	workerCount := 5
	if len(concurrency) > 0 && concurrency[0] > 0 {
		workerCount = concurrency[0]
	}
	if len(deviceIDs) < workerCount {
		workerCount = len(deviceIDs)
	}

	type devTask struct {
		index int
		id    string
	}
	taskCh := make(chan devTask, len(deviceIDs))
	for i, id := range deviceIDs {
		taskCh <- devTask{index: i, id: id}
	}
	close(taskCh)

	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				res, err := s.InspectDeviceWithTemplate(task.id, inspectionType, customTypes, templateID, selectedCommandNames)
				if err != nil {
					logger.Errorf("Batch inspection failed for device %s: %v", task.id, err)
					continue
				}
				results[task.index] = res
			}
		}()
	}
	wg.Wait()

	var finalResults []*InspectionResult
	for _, r := range results {
		if r != nil {
			finalResults = append(finalResults, r)
		}
	}
	return finalResults
}


// ───────────────────────────────────────────────
// 巡检执行
// ───────────────────────────────────────────────

// executeStandardInspection 执行标准巡检（全部标准巡检类型）
func (s *NetworkInspectionService) executeStandardInspection(device *DeviceInfo) []ParsedResult {
	adapter := vendoradapter.CreateVendorAdapter(vendoradapter.VendorType(device.Vendor))
	commands := adapter.GetCommands(nil, vendoradapter.DeviceUnknown) // nil = all types
	return s.executeCommands(device, commands)
}

// executeCustomInspection 执行自定义巡检
func (s *NetworkInspectionService) executeCustomInspection(device *DeviceInfo, types []string) []ParsedResult {
	adapter := vendoradapter.CreateVendorAdapter(vendoradapter.VendorType(device.Vendor))
	inspTypes := make([]vendoradapter.InspectionType, len(types))
	for i, t := range types {
		inspTypes[i] = vendoradapter.InspectionType(t)
	}
	commands := adapter.GetCommands(inspTypes, vendoradapter.DeviceUnknown)
	return s.executeCommands(device, commands)
}

// executeCommands 通过 SSH Shell 执行命令并解析结果
func (s *NetworkInspectionService) executeCommands(device *DeviceInfo, commands []vendoradapter.CommandTemplate) []ParsedResult {
	var results []ParsedResult

	if len(commands) == 0 {
		return results
	}

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

	// 提取命令字符串列表
	cmdStrs := make([]string, len(commands))
	for i, c := range commands {
		cmdStrs[i] = c.Command
	}

	// 通过 Shell 执行所有命令
	shellOutput, err := ConnectAndRunShell(sshDevice, cmdStrs)

	for _, cmd := range commands {
		if err != nil {
			results = append(results, ParsedResult{
				Type:      string(cmd.Type),
				Success:   false,
				Status:    "error",
				Details:   fmt.Sprintf("命令执行失败: %s - %s", cmd.Command, truncateStr(err.Error(), 100)),
				RawOutput: "",
				Timestamp: nowISO(),
			})
			continue
		}

		// 从组合输出中提取单条命令的输出
		cmdOutput := ExtractCommandOutput(shellOutput, cmd.Command)
		if cmdOutput != "" {
			parser := GetParser(device.Vendor, string(cmd.Type))
			parsed := parser(cmdOutput)
			results = append(results, parsed)
		} else {
			logger.Warnf("无法从 shell 输出中提取命令: %s", cmd.Command)
			results = append(results, ParsedResult{
				Type:      string(cmd.Type),
				Success:   false,
				Status:    "error",
				Details:   fmt.Sprintf("%s: 无法解析命令输出", cmd.Name),
				RawOutput: "",
				Timestamp: nowISO(),
			})
		}
	}

	return results
}

// ───────────────────────────────────────────────
// 辅助方法
// ───────────────────────────────────────────────

// generateSummary 生成巡检摘要
func (s *NetworkInspectionService) generateSummary(results []ParsedResult) string {
	normal := 0
	warning := 0
	critical := 0
	errorCount := 0

	for _, r := range results {
		switch r.Status {
		case "normal":
			normal++
		case "warning":
			warning++
		case "critical":
			critical++
		case "error":
			errorCount++
		}
	}

	if critical > 0 {
		return fmt.Sprintf("发现 %d 个严重问题，%d 个警告，需要立即处理", critical, warning)
	}
	if warning > 0 {
		return fmt.Sprintf("发现 %d 个警告项，建议关注", warning)
	}
	if errorCount > 0 {
		return fmt.Sprintf("%d 个命令执行失败，请检查设备连接", errorCount)
	}
	return fmt.Sprintf("巡检完成，%d 项全部正常", normal)
}

// truncateStr 截断字符串到指定长度
func truncateStr(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// AnalyzeCommandOutput 分析命令输出（简化版，不依赖 AI 服务）
// 原始 TS 版本通过 LLM 分析输出，Go 版本暂时返回原始输出的摘要
func AnalyzeCommandOutput(vendor, command, output string) string {
	// 使用结果解析器进行分析
	parser := GetParser(vendor, "version")
	result := parser(output)
	if result.Details != "" {
		return result.Details
	}
	if len(output) > 300 {
		return output[:300]
	}
	return output
}

// GenerateCommands 生成巡检命令（简化版，回退到厂商标准命令）
// 原始 TS 版本通过 LLM 生成命令，Go 版本暂使用标准命令
func GenerateCommands(vendor string, description string) []vendoradapter.CommandTemplate {
	adapter := vendoradapter.CreateVendorAdapter(vendoradapter.VendorType(vendor))
	allCommands := adapter.GetCommands(nil, vendoradapter.DeviceUnknown)
	if len(allCommands) > 5 {
		return allCommands[:5]
	}
	return allCommands
}

// CustomInspectionRequest 自定义巡检请求
type CustomInspectionRequest struct {
	DeviceID    string   `json:"deviceId"`
	Description string   `json:"description"`
	Types       []string `json:"inspectionType"`
}
