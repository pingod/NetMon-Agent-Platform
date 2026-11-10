package services

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

var (
	pingTimeRegex = regexp.MustCompile(`(?:time|时间)[<=](\d+)ms`)
)

// ParseIPRange 将起始 IP 和结束 IP 解析为 IP 列表
func ParseIPRange(startStr, endStr string) ([]string, error) {
	startIP := net.ParseIP(strings.TrimSpace(startStr)).To4()
	if startIP == nil {
		return nil, fmt.Errorf("无效的起始 IPv4 地址: %s", startStr)
	}
	endIP := net.ParseIP(strings.TrimSpace(endStr)).To4()
	if endIP == nil {
		return nil, fmt.Errorf("无效的结束 IPv4 地址: %s", endStr)
	}

	startInt := binary.BigEndian.Uint32(startIP)
	endInt := binary.BigEndian.Uint32(endIP)

	if startInt > endInt {
		startInt, endInt = endInt, startInt
	}

	count := endInt - startInt + 1
	if count > 4096 {
		return nil, fmt.Errorf("扫描范围过大 (最多支持 4096 个 IP 地址，当前为 %d 个)", count)
	}

	ips := make([]string, 0, count)
	for i := startInt; i <= endInt; i++ {
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, i)
		ips = append(ips, ip.String())
	}
	return ips, nil
}

// ProbeResult 探测结果
type ProbeResult struct {
	IPAddress      string
	IsOnline       bool
	Status         string // "online", "snmp_ok", "offline"
	ResponseTimeMs int
	SysName        string
	SysDescr       string
	Vendor         string
	Model          string
	InterfaceCount int
}

// ProbeHost 多方式混合探测单个主机（ICMP Ping + TCP 端口探测 + SNMP 探测）
func ProbeHost(ip string) ProbeResult {
	res := ProbeResult{
		IPAddress: ip,
		Status:    "offline",
	}

	// 1. ICMP Ping 探测
	start := time.Now()
	pingOk, pingMs := executePing(ip, 1200*time.Millisecond)
	if pingOk {
		res.IsOnline = true
		res.Status = "online"
		res.ResponseTimeMs = pingMs
	}

	// 2. TCP 端口快速探活（当 Ping 被防火墙禁用但端口开放时）
	if !res.IsOnline {
		tcpOk, tcpMs := executeTCPProbe(ip, 500*time.Millisecond)
		if tcpOk {
			res.IsOnline = true
			res.Status = "online"
			res.ResponseTimeMs = tcpMs
		}
	}

	if !res.IsOnline {
		res.ResponseTimeMs = int(time.Since(start).Milliseconds())
		if res.ResponseTimeMs > 1000 {
			res.ResponseTimeMs = 1000
		}
		return res
	}

	// 3. SNMP 探活（如果在线，尝试检测 SNMP 并识别厂商和型号）
	snmpOk, sysName, sysDescr, vendor, model, ifCount := probeSNMPQuick(ip)
	if snmpOk {
		res.Status = "snmp_ok"
		res.SysName = sysName
		res.SysDescr = sysDescr
		res.Vendor = vendor
		res.Model = model
		res.InterfaceCount = ifCount
	}

	return res
}

// executePing 执行操作系统原生的 Ping 命令
func executePing(ip string, timeout time.Duration) (bool, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", strconv.Itoa(int(timeout.Milliseconds())), ip)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip)
	}

	start := time.Now()
	out, err := cmd.CombinedOutput()
	durationMs := int(time.Since(start).Milliseconds())

	if err == nil {
		outStr := strings.ToLower(string(out))
		// 校验 Windows 和 Linux 的 Ping 响应标志
		if strings.Contains(outStr, "ttl=") ||
			strings.Contains(outStr, "time=") ||
			strings.Contains(outStr, "ʱ<") ||
			strings.Contains(outStr, "ʱ=") ||
			strings.Contains(outStr, "bytes from") ||
			strings.Contains(outStr, "1 received") ||
			strings.Contains(outStr, "已接收 = 1") ||
			strings.Contains(outStr, "ms") {
			// 尝试提取精确延迟
			matches := pingTimeRegex.FindStringSubmatch(string(out))
			if len(matches) > 1 {
				if ms, err := strconv.Atoi(matches[1]); err == nil {
					return true, ms
				}
			}
			if durationMs < 1 {
				durationMs = 1
			}
			return true, durationMs
		}
	}
	return false, 0
}

// executeTCPProbe 快速扫描常用管理端口，防止 ICMP 禁 ping 漏判
func executeTCPProbe(ip string, timeout time.Duration) (bool, int) {
	commonPorts := []int{80, 443, 22, 8080, 3389, 23, 161, 445, 8443}
	for _, port := range commonPorts {
		target := net.JoinHostPort(ip, strconv.Itoa(port))
		start := time.Now()
		conn, err := net.DialTimeout("tcp", target, timeout)
		if err == nil {
			dur := int(time.Since(start).Milliseconds())
			_ = conn.Close()
			if dur < 1 {
				dur = 1
			}
			return true, dur
		}
	}
	return false, 0
}

// probeSNMPQuick 尝试连接 SNMP 快速获取设备信息
func probeSNMPQuick(ip string) (bool, string, string, string, string, int) {
	communities := []string{"public", "private", "snmp", "monitor"}
	for _, comm := range communities {
		snmp := SnmpServiceInstance.CreateSession(ip, 161, SNMPv2c, comm, "", "", "", "", "")
		snmp.Timeout = 600 * time.Millisecond
		snmp.Retries = 0
		if err := snmp.Connect(); err != nil {
			continue
		}

		result, err := snmp.Get([]string{".1.3.6.1.2.1.1.1.0", ".1.3.6.1.2.1.1.5.0"})
		_ = snmp.Conn.Close()
		if err == nil && len(result.Variables) > 0 {
			var sysDescr, sysName string
			for _, v := range result.Variables {
				if strings.HasSuffix(v.Name, "1.1.0") {
					sysDescr = fmt.Sprintf("%v", v.Value)
				} else if strings.HasSuffix(v.Name, "1.5.0") {
					sysName = fmt.Sprintf("%v", v.Value)
				}
			}
			vendor := detectVendorFromDescr(sysDescr)
			return true, sysName, sysDescr, vendor, "", 0
		}
	}
	return false, "", "", "", "", 0
}

// detectVendorFromDescr 从 sysDescr 推断设备厂商
func detectVendorFromDescr(descr string) string {
	d := strings.ToLower(descr)
	if strings.Contains(d, "cisco") {
		return "cisco"
	}
	if strings.Contains(d, "huawei") || strings.Contains(d, "vrp") {
		return "huawei"
	}
	if strings.Contains(d, "h3c") || strings.Contains(d, "comware") {
		return "h3c"
	}
	if strings.Contains(d, "ruijie") {
		return "ruijie"
	}
	if strings.Contains(d, "juniper") || strings.Contains(d, "junos") {
		return "juniper"
	}
	if strings.Contains(d, "fortinet") || strings.Contains(d, "fortios") {
		return "fortinet"
	}
	if strings.Contains(d, "linux") {
		return "linux"
	}
	if strings.Contains(d, "windows") {
		return "windows"
	}
	return "generic"
}

// RunDiscoveryJob 异步执行网络发现扫描任务
func RunDiscoveryJob(jobID, startIP, endIP string) {
	ips, err := ParseIPRange(startIP, endIP)
	if err != nil {
		logger.Errorf("Discovery job %s failed to parse IP range: %v", jobID, err)
		errMsg := err.Error()
		nowStr := time.Now().Format("2006-01-02 15:04:05")
		_, _ = appdb.DB().Exec(`UPDATE network_discovery_jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`, errMsg, nowStr, jobID)
		return
	}

	totalHosts := len(ips)
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	_, _ = appdb.DB().Exec(`UPDATE network_discovery_jobs SET status = 'running', total_hosts = ?, progress = 0, scanned_hosts = 0, found_devices = 0, started_at = ? WHERE id = ?`, totalHosts, nowStr, jobID)

	logger.Infof("Starting discovery job %s: range %s - %s (%d hosts)", jobID, startIP, endIP, totalHosts)

	// 并发工作池（最大 30 并发）
	concurrency := 30
	if concurrency > totalHosts {
		concurrency = totalHosts
	}
	if concurrency < 1 {
		concurrency = 1
	}

	ipChan := make(chan string, totalHosts)
	for _, ip := range ips {
		ipChan <- ip
	}
	close(ipChan)

	var scannedCount int64
	var foundCount int64
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range ipChan {
				// 检查任务是否被用户取消
				var currentStatus string
				err := appdb.DB().QueryRow(`SELECT status FROM network_discovery_jobs WHERE id = ?`, jobID).Scan(&currentStatus)
				if err == nil && currentStatus == "cancelled" {
					return
				}

				res := ProbeHost(ip)
				currentScanned := atomic.AddInt64(&scannedCount, 1)

				if res.IsOnline {
					atomic.AddInt64(&foundCount, 1)
					_ = repos.DiscoveryResults.InsertOnline(repos.DiscoveryResultOnlineInput{
						ID:             uuid.NewString(),
						JobID:          jobID,
						IPAddress:      res.IPAddress,
						Status:         res.Status,
						SysName:        res.SysName,
						SysDescr:       res.SysDescr,
						Vendor:         res.Vendor,
						Model:          res.Model,
						InterfaceCount: res.InterfaceCount,
						ResponseTimeMs: res.ResponseTimeMs,
					})
				} else {
					_ = repos.DiscoveryResults.InsertOffline(uuid.NewString(), jobID, res.IPAddress, res.ResponseTimeMs)
				}

				// 每扫描 5 台或全部扫完时更新进度
				if currentScanned%5 == 0 || int(currentScanned) == totalHosts {
					progress := int(currentScanned * 100 / int64(totalHosts))
					if progress > 99 && int(currentScanned) < totalHosts {
						progress = 99
					}
					_ = repos.DiscoveryJobs.UpdateProgress(jobID, progress, int(currentScanned), int(atomic.LoadInt64(&foundCount)))
				}
			}
		}()
	}

	wg.Wait()

	// 再次检查状态，避免覆盖取消状态
	var finalStatus string
	err = appdb.DB().QueryRow(`SELECT status FROM network_discovery_jobs WHERE id = ?`, jobID).Scan(&finalStatus)
	if err == nil && finalStatus != "cancelled" {
		finalFound := int(atomic.LoadInt64(&foundCount))
		finalScanned := int(atomic.LoadInt64(&scannedCount))
		nowStr := time.Now().Format("2006-01-02 15:04:05")
		_, _ = appdb.DB().Exec(`UPDATE network_discovery_jobs SET status = 'completed', progress = 100, scanned_hosts = ?, found_devices = ?, completed_at = ? WHERE id = ?`, finalScanned, finalFound, nowStr, jobID)
		logger.Infof("Discovery job %s completed: scanned %d, found %d online", jobID, finalScanned, finalFound)
	}
}
