// 服务器管理路由 — 对应 TS serverManagementRoutes.ts
// 挂载路径：/api/v1/server-management
// 提供：采集主机信息、采集性能指标、批量采集、批量导入
package routes

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// ServerManagementRoutes 服务器管理路由
func ServerManagementRoutes() chi.Router {
	r := chi.NewRouter()

	// POST /{id}/collect-info — 采集单台服务器主机信息
	r.With(middleware.RequireRole("admin")).Post("/{id}/collect-info", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}
		info, err := collectServerInfo(id)
		if err != nil {
			httpx.RespondError(w, "采集失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		httpx.Respond(w, info)
	})

	// POST /collect-all — 批量采集所有主机信息
	r.With(middleware.RequireRole("admin")).Post("/collect-all", func(w http.ResponseWriter, r *http.Request) {
		list, err := repos.Servers.ListAllForCollect()
		if err != nil {
			httpx.RespondError(w, "获取服务器列表失败", http.StatusInternalServerError)
			return
		}
		result := collectAllInfo(list)
		httpx.Respond(w, result)
	})

	// POST /{id}/collect-metrics — 采集单台服务器性能指标
	r.With(middleware.RequireRole("admin")).Post("/{id}/collect-metrics", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}
		metrics, err := collectServerMetrics(id)
		if err != nil {
			httpx.RespondError(w, "采集失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		httpx.Respond(w, metrics)
	})

	// POST /collect-all-metrics — 批量采集所有性能指标
	r.With(middleware.RequireRole("admin")).Post("/collect-all-metrics", func(w http.ResponseWriter, r *http.Request) {
		list, err := repos.Servers.ListAllForCollect()
		if err != nil {
			httpx.RespondError(w, "获取服务器列表失败", http.StatusInternalServerError)
			return
		}
		result := collectAllMetrics(list)
		httpx.Respond(w, result)
	})

	// POST /import — 批量导入（JSON 版）
	r.With(middleware.RequireRole("admin")).Post("/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Servers        []importServerItem `json:"servers"`
			TestConnection bool               `json:"test_connection"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.Servers) == 0 {
			httpx.RespondError(w, "没有要导入的服务器", http.StatusBadRequest)
			return
		}

		var result struct {
			Success int                `json:"success"`
			Failed  int                `json:"failed"`
			Skipped int                `json:"skipped"`
			Details []importDetailItem `json:"details"`
			Errors  []string           `json:"errors"`
		}
		result.Details = []importDetailItem{}
		result.Errors = []string{}

		for _, item := range req.Servers {
			detail := importDetailItem{Name: item.Name}
			if item.Hostname != "" {
				detail.Hostname = item.Hostname
			}

			// 去重：hostname 已存在则跳过
			exists, _ := repos.Servers.ExistsByHostname(item.Hostname)
			if exists {
				result.Skipped++
				detail.Status = "duplicate"
				detail.Error = "主机名已存在"
				result.Details = append(result.Details, detail)
				continue
			}

			port := item.Port
			if port == 0 {
				port = 22
			}
			useKey := false
			if item.UseSSHKey != nil && *item.UseSSHKey == 1 {
				useKey = true
			}
			var tags []string
			if item.Tags != nil {
				tags = *item.Tags
			}

			createReq := repos.ServerCreateInput{
				Name:        item.Name,
				Hostname:    item.Hostname,
				Port:        port,
				Username:    item.Username,
				Password:    item.Password,
				PrivateKey:  item.PrivateKey,
				UseSSHKey:   useKey,
				Description: item.Description,
				Tags:        tags,
				GroupID:     item.GroupID,
				OSType:      strings.ToLower(strings.TrimSpace(item.OSType)),
			}
			if createReq.OSType != "windows" && createReq.OSType != "linux" {
				createReq.OSType = "linux"
			}

			s, err := repos.Servers.Create(createReq)
			if err != nil {
				result.Failed++
				detail.Status = "error"
				detail.Error = err.Error()
				result.Details = append(result.Details, detail)
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Name, err))
				continue
			}

			// 连接测试（可选）— 异步执行，不阻塞导入主流程
			if req.TestConnection {
				go func(srvID string) {
					cfg, cfgErr := buildServerSSHConfig(srvID)
					if cfgErr != nil {
						return
					}
					if client, dialErr := services.SSHDial(cfg); dialErr == nil {
						client.Close()
						repos.Servers.UpdateLastConnected(srvID)
					}
				}(s.ID)
			}

			result.Success++
			detail.Status = "success"
			result.Details = append(result.Details, detail)
		}

		httpx.Respond(w, result)
	})

	return r
}

// ───────────────────────────────────────────────
// 采集实现
// ───────────────────────────────────────────────

// importServerItem 导入条目
type importServerItem struct {
	Name        string    `json:"name"`
	Hostname    string    `json:"hostname"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"password"`
	PrivateKey  string    `json:"private_key"`
	UseSSHKey   *int      `json:"use_ssh_key"`
	Description string    `json:"description"`
	Tags        *[]string `json:"tags"`
	GroupID     string    `json:"group_id"`
	OSType      string    `json:"os_type"`
}

// importDetailItem 导入详情
type importDetailItem struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// execSimple 执行命令并返回 stdout（忽略错误）
func execSimple(client *services.SSHClient, cmd string, timeout time.Duration) string {
	stdout, _, _ := client.ExecCommand(cmd, timeout)
	return stdout
}

// collectServerInfo 采集单台服务器信息并回填
func collectServerInfo(id string) (map[string]interface{}, error) {
	cfg, err := buildServerSSHConfig(id)
	if err != nil {
		return nil, err
	}
	client, err := services.SSHDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	defer client.Close()
	repos.Servers.UpdateLastConnected(id)

	info := map[string]interface{}{}

	// 系统信息（Linux）
	osName := execSimple(client, "cat /etc/os-release 2>/dev/null | grep -E '^(PRETTY_NAME|NAME)=' | head -2", 15*time.Second)
	cpuCores := execSimple(client, "nproc 2>/dev/null || echo 0", 15*time.Second)
	memTotal := execSimple(client, "free -m 2>/dev/null | awk '/Mem:/{print $2}'", 15*time.Second)
	diskTotal := execSimple(client, "df -h / 2>/dev/null | awk 'NR==2{print $2}'", 15*time.Second)
	kernel := execSimple(client, "uname -r 2>/dev/null", 15*time.Second)
	ipAddr := execSimple(client, "hostname -I 2>/dev/null | awk '{print $1}'", 15*time.Second)

	osNameStr := parseFirstLine(osName)
	kernelStr := parseFirstLine(kernel)
	osDetail := strings.TrimSpace(kernelStr)
	if osDetail == "" {
		osDetail = strings.TrimSpace(osNameStr)
	}

	cpuN := parseNumber(cpuCores)
	memGB := parseFloatMB(memTotal) / 1024.0
	diskGB := parseDiskSizeGB(diskTotal)
	ipStr := parseFirstLine(ipAddr)

	// Windows 分支
	if cfg.OSTypeHintWindows() {
		osName = execSimple(client, "wmic os get Caption /value 2>nul | findstr Caption", 15*time.Second)
		cpuCores = execSimple(client, "echo %NUMBER_OF_PROCESSORS%", 15*time.Second)
		osNameStr = parseFirstLine(osName)
		cpuN = parseNumber(cpuCores)
	}

	_ = repos.Servers.UpdateCollectedInfo(id, osNameStr, osDetail, cpuN, memGB, diskGB, ipStr)

	info["os"] = osNameStr
	info["os_detail"] = osDetail
	info["cpu_cores"] = cpuN
	info["memory_gb"] = memGB
	info["disk_gb"] = diskGB
	info["ip_address"] = ipStr

	return info, nil
}

// collectServerMetrics 采集单台服务器性能指标
func collectServerMetrics(id string) (map[string]interface{}, error) {
	cfg, err := buildServerSSHConfig(id)
	if err != nil {
		return nil, err
	}
	client, err := services.SSHDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	defer client.Close()

	metrics := map[string]interface{}{}
	// CPU 使用率（取 1s 采样）
	cpuUsage := execSimple(client, "top -bn2 2>/dev/null | grep 'Cpu(s)' | tail -1 | awk '{print 100-$8}'", 30*time.Second)
	metrics["cpu_usage"] = parseFloat(cpuUsage)

	// 内存
	memLine := execSimple(client, "free -m 2>/dev/null | awk '/Mem:/{print $2, $3, $7}'", 20*time.Second)
	memParts := strings.Fields(memLine)
	if len(memParts) >= 2 {
		total := parseFloat(memParts[0])
		used := parseFloat(memParts[1])
		metrics["memory_total_gb"] = total / 1024.0
		metrics["memory_used_gb"] = used / 1024.0
		if total > 0 {
			metrics["memory_usage"] = round2(used / total * 100)
		}
	}

	// 磁盘
	diskLine := execSimple(client, "df / 2>/dev/null | awk 'NR==2{print $2, $3, $5}'", 20*time.Second)
	diskParts := strings.Fields(diskLine)
	if len(diskParts) >= 3 {
		totalKB := parseFloat(diskParts[0])
		usedKB := parseFloat(diskParts[1])
		metrics["disk_total_gb"] = round2(totalKB / 1024.0 / 1024.0)
		metrics["disk_used_gb"] = round2(usedKB / 1024.0 / 1024.0)
		metrics["disk_usage"] = parseFloat(strings.ReplaceAll(diskParts[2], "%", ""))
	}

	// 负载
	loadLine := execSimple(client, "cat /proc/loadavg 2>/dev/null | awk '{print $1, $2, $3}'", 15*time.Second)
	loadParts := strings.Fields(loadLine)
	if len(loadParts) >= 3 {
		metrics["load_1min"] = parseFloat(loadParts[0])
		metrics["load_5min"] = parseFloat(loadParts[1])
		metrics["load_15min"] = parseFloat(loadParts[2])
	}

	// 运行时间
	uptime := execSimple(client, "cat /proc/uptime 2>/dev/null | awk '{print int($1)}'", 15*time.Second)
	metrics["uptime_seconds"] = int(parseFloat(uptime))

	// 保存到 server_metrics 表
	repos.Servers.InsertMetric(id, metrics)

	return metrics, nil
}

// collectAllInfo 批量采集主机信息（并发）
func collectAllInfo(list []repos.Server) map[string]interface{} {
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	failed := 0
	errors := []string{}

	for _, s := range list {
		wg.Add(1)
		go func(server repos.Server) {
			defer wg.Done()
			_, err := collectServerInfo(server.ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("%s: %v", server.Name, err))
			} else {
				success++
			}
		}(s)
	}
	wg.Wait()

	if errors == nil {
		errors = []string{}
	}
	return map[string]interface{}{
		"success": success,
		"failed":  failed,
		"errors":  errors,
		"total":   len(list),
	}
}

// collectAllMetrics 批量采集性能指标（并发）
func collectAllMetrics(list []repos.Server) map[string]interface{} {
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	failed := 0
	errors := []string{}

	for _, s := range list {
		wg.Add(1)
		go func(server repos.Server) {
			defer wg.Done()
			_, err := collectServerMetrics(server.ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("%s: %v", server.Name, err))
			} else {
				success++
			}
		}(s)
	}
	wg.Wait()

	if errors == nil {
		errors = []string{}
	}
	return map[string]interface{}{
		"success": success,
		"failed":  failed,
		"errors":  errors,
		"total":   len(list),
	}
}

// ───────────────────────────────────────────────
// 解析工具
// ───────────────────────────────────────────────

var numRegex = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// parseFirstLine 取第一行有效内容
func parseFirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// parseNumber 解析整数（取第一个数字）
func parseNumber(s string) int {
	m := numRegex.FindString(s)
	n, _ := strconv.Atoi(m)
	return n
}

// parseFloat 解析浮点数（取第一个数字）
func parseFloat(s string) float64 {
	m := numRegex.FindString(s)
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

// parseFloatMB 解析 MB 数值
func parseFloatMB(s string) float64 {
	m := numRegex.FindString(s)
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

// parseDiskSizeGB 解析 df 输出的磁盘大小（如 100G / 1.5T / 500M）
func parseDiskSizeGB(s string) float64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	val := fields[0]
	m := numRegex.FindString(val)
	f, _ := strconv.ParseFloat(m, 64)
	if strings.Contains(val, "T") || strings.Contains(val, "t") {
		return f * 1024
	}
	if strings.Contains(val, "M") || strings.Contains(val, "m") {
		return f / 1024
	}
	return f // G 或无单位视为 GB
}

// round2 保留两位小数
func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
