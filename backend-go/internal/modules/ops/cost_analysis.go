// 成本分析（/api/v1/cost-analysis）。
//
// 全部为「按纳管规格 × 固定费率」的估算，不是云厂商账单：
// 容器取实时 docker stats（CPU/内存实际占用），VM 取 virtual_machines 台账规格。
// 费率常量与前端 CostAnalysis.tsx 保持一致。
package ops

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
)

const (
	rateContainerCPU = 0.05   // ¥/核/小时
	rateContainerMem = 0.005  // ¥/MB/小时
	rateVMCPU        = 0.10   // ¥/核/小时
	rateVMMem        = 0.01   // ¥/MB/小时
	rateVMDisk       = 0.0002 // ¥/GB/小时
	hoursPerMonth    = 24 * 30
)

// CostAnalysisRoutes /api/v1/cost-analysis
func CostAnalysisRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/containers", costContainers)
	r.Get("/vms", costVMs)
	r.Get("/recommendations", costRecommendations)
	r.Get("/summary", costSummary)
	return r
}

type containerCost struct {
	Name            string  `json:"name"`
	Host            string  `json:"host"`
	CPUCores        float64 `json:"cpuCores"`
	MemoryMB        float64 `json:"memoryMB"`
	HourlyRate      float64 `json:"hourlyRate"`
	DailyEstimate   float64 `json:"dailyEstimate"`
	MonthlyEstimate float64 `json:"monthlyEstimate"`
}

type vmCost struct {
	Name            string  `json:"name"`
	Platform        string  `json:"platform"`
	CPUCores        float64 `json:"cpuCores"`
	MemoryGB        float64 `json:"memoryGB"`
	DiskGB          float64 `json:"diskGB"`
	HourlyRate      float64 `json:"hourlyRate"`
	MonthlyEstimate float64 `json:"monthlyEstimate"`
}

type costRecommendation struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	MonthlySavings float64 `json:"monthlySavings"`
	Resource      string  `json:"resource"`
}

// collectContainerCosts 实时 docker stats；无可用 Docker 宿主机时返回空（不编造数据）
func collectContainerCosts() []containerCost {
	list := []containerCost{}
	client, host, err := getDockerSSHClient("")
	if err != nil {
		return list
	}
	defer client.Close()
	out, _, err := client.ExecCommand(`docker stats --no-stream --format '{{json .}}'`, 30*time.Second)
	if err != nil {
		return list
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s struct {
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemUsage string `json:"MemUsage"`
		}
		if json.Unmarshal([]byte(line), &s) != nil || s.Name == "" {
			continue
		}
		cpuPct := parsePerc(s.CPUPerc)
		memMB := parseMemUsageMB(s.MemUsage)
		cores := cpuPct / 100
		hourly := cores*rateContainerCPU + memMB*rateContainerMem
		list = append(list, containerCost{
			Name: s.Name, Host: host, CPUCores: cores, MemoryMB: memMB,
			HourlyRate: hourly, DailyEstimate: hourly * 24, MonthlyEstimate: hourly * hoursPerMonth,
		})
	}
	return list
}

func collectVMCosts() []vmCost {
	list := []vmCost{}
	rows, err := appdb.DB().Query(`SELECT name, COALESCE(hypervisor,''), COALESCE(cpu_cores,0), COALESCE(memory_mb,0), COALESCE(disk_gb,0) FROM virtual_machines ORDER BY name`)
	if err != nil {
		return list
	}
	defer rows.Close()
	for rows.Next() {
		var name, platform string
		var cores float64
		var memMB, diskGB float64
		if rows.Scan(&name, &platform, &cores, &memMB, &diskGB) != nil {
			continue
		}
		if platform == "" {
			platform = "未知平台"
		}
		hourly := cores*rateVMCPU + memMB*rateVMMem + diskGB*rateVMDisk
		list = append(list, vmCost{
			Name: name, Platform: platform, CPUCores: cores, MemoryGB: memMB / 1024, DiskGB: diskGB,
			HourlyRate: hourly, MonthlyEstimate: hourly * hoursPerMonth,
		})
	}
	return list
}

func costContainers(w http.ResponseWriter, r *http.Request) {
	httpx.Respond(w, collectContainerCosts())
}

func costVMs(w http.ResponseWriter, r *http.Request) {
	httpx.Respond(w, collectVMCosts())
}

func costRecommendations(w http.ResponseWriter, r *http.Request) {
	httpx.Respond(w, buildCostRecommendations(collectContainerCosts(), collectVMCosts()))
}

func costSummary(w http.ResponseWriter, r *http.Request) {
	containers := collectContainerCosts()
	vms := collectVMCosts()
	var cm, vm, idle float64
	for _, c := range containers {
		cm += c.MonthlyEstimate
		if c.CPUCores < 0.05 {
			idle += c.MonthlyEstimate
		}
	}
	for _, v := range vms {
		vm += v.MonthlyEstimate
	}
	httpx.Respond(w, map[string]interface{}{
		"containerMonthly": cm,
		"vmMonthly":        vm,
		"totalMonthly":     cm + vm,
		"idleWaste":        idle,
	})
}

func buildCostRecommendations(containers []containerCost, vms []vmCost) []costRecommendation {
	recs := []costRecommendation{}
	// 闲置容器：CPU 占用不足 5% 核
	for i, c := range containers {
		if c.CPUCores >= 0.05 || c.MonthlyEstimate <= 0 {
			continue
		}
		recs = append(recs, costRecommendation{
			ID: fmt.Sprintf("idle-%d", i), Type: "idle",
			Title:       fmt.Sprintf("容器 %s 疑似闲置", c.Name),
			Description: fmt.Sprintf("当前 CPU 占用仅 %.2f 核（估算月成本 ¥%.2f）。如确认无业务流量，可停止或合并该容器。", c.CPUCores, c.MonthlyEstimate),
			MonthlySavings: c.MonthlyEstimate, Resource: c.Name,
		})
		if len(recs) >= 10 {
			break
		}
	}
	// 降配：VM 关联服务器且有真实低负载指标时才建议（无指标不臆造利用率）
	for i, v := range vms {
		cpu, mem, ok := vmRealUtilization(v.Name)
		if !ok || cpu >= 20 || mem >= 30 || v.CPUCores < 4 {
			continue
		}
		savings := v.MonthlyEstimate * 0.4
		recs = append(recs, costRecommendation{
			ID: fmt.Sprintf("downsize-%d", i), Type: "downsize",
			Title:       fmt.Sprintf("VM %s 可考虑降配", v.Name),
			Description: fmt.Sprintf("最近采集 CPU %.1f%% / 内存 %.1f%%，规格 %d 核明显过剩。降配一档预计节省约 40%%（估算）。", cpu, mem, int(v.CPUCores)),
			MonthlySavings: savings, Resource: v.Name,
		})
	}
	// 预留实例：VM 规模达到一定量级时提示包年包月折扣（估算）
	var vmTotal float64
	for _, v := range vms {
		vmTotal += v.MonthlyEstimate
	}
	if len(vms) >= 3 && vmTotal > 100 {
		recs = append(recs, costRecommendation{
			ID: "reserved-summary", Type: "reserved",
			Title:       "长期运行 VM 可评估包年包月/预留实例",
			Description: fmt.Sprintf("当前纳管 %d 台 VM，按量估算月成本 ¥%.2f。主流云平台包年包月通常可优惠 30%%~50%%，建议按实际账单评估。", len(vms), vmTotal),
			MonthlySavings: vmTotal * 0.3, Resource: "全部 VM",
		})
	}
	return recs
}

// vmRealUtilization 通过 VM 名匹配纳管服务器的最新真实指标
func vmRealUtilization(vmName string) (cpu, mem float64, ok bool) {
	var serverID string
	err := appdb.DB().QueryRow(`SELECT id FROM servers WHERE hostname = ? OR name = ? LIMIT 1`, vmName, vmName).Scan(&serverID)
	if err != nil {
		return 0, 0, false
	}
	var cpuV, memV sql.NullFloat64
	if appdb.DB().QueryRow(`SELECT cpu_usage, memory_usage FROM server_metrics WHERE server_id = ? ORDER BY collected_at DESC LIMIT 1`, serverID).
		Scan(&cpuV, &memV) != nil || !cpuV.Valid || !memV.Valid {
		return 0, 0, false
	}
	return cpuV.Float64, memV.Float64, true
}

func parsePerc(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseMemUsageMB 解析 docker stats 的 "123.4MiB / 1.9GiB" 已用部分为 MB
func parseMemUsageMB(s string) float64 {
	used := strings.TrimSpace(strings.SplitN(s, "/", 2)[0])
	units := []struct {
		suffix string
		mult   float64
	}{{"GiB", 1024}, {"MiB", 1}, {"KiB", 1.0 / 1024}, {"TiB", 1024 * 1024}, {"GB", 1000}, {"MB", 1}, {"KB", 1.0 / 1000}, {"B", 1.0 / (1024 * 1024)}}
	for _, u := range units {
		if strings.HasSuffix(used, u.suffix) {
			v, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(used, u.suffix)), 64)
			return v * u.mult
		}
	}
	return 0
}