// 设备状态探测服务 — 定期 ICMP ping 探测所有设备并更新在线/离线状态
package services

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

// DeviceStatusProber 设备状态探测器
type DeviceStatusProber struct {
	mu       sync.Mutex
	stopCh   chan struct{}
	running  bool
	interval time.Duration // 探测间隔
}

// DeviceStatusProberInstance 全局单例
var DeviceStatusProberInstance = &DeviceStatusProber{
	interval: 60 * time.Second, // 默认60秒探测一次
}

// Start 启动定时探测
func (p *DeviceStatusProber) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	go p.loop()
	logger.Infof("设备状态探测服务已启动，间隔 %s", p.interval)
}

// Shutdown 停止探测
func (p *DeviceStatusProber) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running && p.stopCh != nil {
		close(p.stopCh)
		p.running = false
	}
}

// SetInterval 设置探测间隔
func (p *DeviceStatusProber) SetInterval(d time.Duration) {
	p.mu.Lock()
	p.interval = d
	p.mu.Unlock()
}

func (p *DeviceStatusProber) loop() {
	// 首次立即执行
	p.probeAll()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			logger.Infof("设备状态探测服务已停止")
			return
		case <-ticker.C:
			p.probeAll()
		}
	}
}

// ProbeNow 手动触发立即 Ping 探测所有设备，并返回在线/离线统计结果
func (p *DeviceStatusProber) ProbeNow() (total int, online int, offline int) {
	return p.probeAll()
}

func (p *DeviceStatusProber) probeAll() (total int, online int, offline int) {
	db := appdb.DB()

	// 获取所有设备 ID、IP、当前状态、SNMP 开启标志
	rows, err := db.Query(`SELECT id, ip_address, status, COALESCE(snmp_enabled, 0) FROM network_devices`)
	if err != nil {
		logger.Errorf("探测设备状态失败: %v", err)
		return 0, 0, 0
	}
	defer rows.Close()

	type dev struct {
		id          string
		ip          string
		status      string
		snmpEnabled int
	}
	var devices []dev
	for rows.Next() {
		var d dev
		if rows.Scan(&d.id, &d.ip, &d.status, &d.snmpEnabled) == nil {
			devices = append(devices, d)
		}
	}

	if len(devices) == 0 {
		return 0, 0, 0
	}

	logger.Debugf("开始探测 %d 台设备综合在线状态(Ping + SNMP/SSH 协议验证)...", len(devices))

	var wg sync.WaitGroup
	var mu sync.Mutex

	// 并发上限：避免设备数量大时无限创建 goroutine
	sem := make(chan struct{}, 16)

	for _, d := range devices {
		wg.Add(1)
		go func(device dev) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			reachable := probeDeviceIsOnline(device.id, device.ip, device.snmpEnabled)
			newStatus := "offline"
			if reachable {
				newStatus = "online"
			}

			mu.Lock()
			if reachable {
				online++
			} else {
				offline++
			}
			mu.Unlock()

			nowStr := time.Now().Format("2006-01-02 15:04:05")
			db.Exec(`UPDATE network_devices SET status = ?, updated_at = ? WHERE id = ?`, newStatus, nowStr, device.id)
			if newStatus != device.status {
				logger.Infof("设备 %s (%s) 状态变更: %s -> %s", device.id, device.ip, device.status, newStatus)
			}
		}(d)
	}
	wg.Wait()

	total = len(devices)
	logger.Debugf("探测完成: 总数 %d, %d 在线, %d 离线", total, online, offline)
	return total, online, offline
}

// ProbeSingleDevice 探测单台设备并更新其在线状态
func (p *DeviceStatusProber) ProbeSingleDevice(deviceID string) {
	db := appdb.DB()
	var ip, status string
	var snmpEnabled int
	err := db.QueryRow(`SELECT ip_address, status, COALESCE(snmp_enabled, 0) FROM network_devices WHERE id = ?`, deviceID).Scan(&ip, &status, &snmpEnabled)
	if err != nil {
		return
	}
	reachable := probeDeviceIsOnline(deviceID, ip, snmpEnabled)
	newStatus := "offline"
	if reachable {
		newStatus = "online"
	}
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	db.Exec(`UPDATE network_devices SET status = ?, updated_at = ? WHERE id = ?`, newStatus, nowStr, deviceID)
	if newStatus != status {
		logger.Infof("设备 %s (%s) 状态变更: %s -> %s", deviceID, ip, status, newStatus)
	}
}

// metricFreshnessWindow 指标/心跳新鲜度窗口：只有在该窗口内仍持续产生真实采集数据的设备才视为"新鲜"
const metricFreshnessWindow = 5 * time.Minute

// hasRecentMetrics 检查设备最近是否仍有真实采集数据（按时间阈值判定）
// 注意：不能再以"历史上产生过指标"作为在线依据，否则设备断连后会永远显示在线。
func hasRecentMetrics(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	threshold := time.Now().Add(-metricFreshnessWindow).Format("2006-01-02 15:04:05")

	var ifCount int
	if err := appdb.DBForTable("snmp_interface_metrics").QueryRow(
		`SELECT COUNT(*) FROM snmp_interface_metrics WHERE device_id = ? AND sampled_at >= ?`,
		deviceID, threshold).Scan(&ifCount); err == nil && ifCount > 0 {
		return true
	}

	var sampleCount int
	if err := appdb.DBForTable("snmp_metric_samples").QueryRow(
		`SELECT COUNT(*) FROM snmp_metric_samples WHERE device_id = ? AND created_at >= ?`,
		deviceID, threshold).Scan(&sampleCount); err == nil && sampleCount > 0 {
		return true
	}
	return false
}

// getWindowsPhysicalSourceIP 获取 Windows 物理网卡的本地有效 IPv4，用于在 Ping 时绑定物理接口绕过 TUN/TAP/代理虚拟网卡劫持
func getWindowsPhysicalSourceIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "meta") ||
			strings.Contains(nameLower, "tun") ||
			strings.Contains(nameLower, "tap") ||
			strings.Contains(nameLower, "wireguard") ||
			strings.Contains(nameLower, "virtual") ||
			strings.Contains(nameLower, "vmnet") ||
			strings.Contains(nameLower, "vbox") ||
			strings.Contains(nameLower, "loopback") ||
			strings.Contains(nameLower, "npcap") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			ipStr := ip4.String()
			if strings.HasPrefix(ipStr, "169.254.") || strings.HasPrefix(ipStr, "198.18.") || strings.HasPrefix(ipStr, "127.") {
				continue
			}
			return ipStr
		}
	}
	return ""
}

// probeDeviceIsOnline 综合判定设备在线状态（SNMP 协议 / SSH 管理端口 / 真实物理 Ping 三位一体）
func probeDeviceIsOnline(deviceID, ip string, snmpEnabled int) bool {
	if ip == "" {
		return false
	}

	// 1. 【用户核心规则】只要 SNMP 协议可达，就绝对判定为在线！
	if snmpEnabled == 1 {
		cred := SnmpServiceInstance.GetCredential(deviceID)
		if cred == nil {
			cred = SnmpServiceInstance.GetDefaultCredential()
		}
		if cred != nil {
			port := cred.SnmpPort
			if port == 0 {
				port = 161
			}
			version := SnmpVersion(cred.SnmpVersion)
			if version == "" {
				version = SNMPv2c
			}
			community := cred.Community
			if community == "" && version != SNMPv3 {
				community = "public"
			}
			if SnmpServiceInstance.TestCredentialConnection(ip, port, version, community,
				cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
				cred.SnmpPrivProtocol, cred.SnmpPrivKey) {
				return true
			}
		} else {
			// 尝试使用默认 public/v2c 探测
			if SnmpServiceInstance.TestCredentialConnection(ip, 161, SNMPv2c, "public", "", "", "", "", "") {
				return true
			}
		}
	}

	// 2. SSH 管理端口连通性校验
	sshRes := NetworkDeviceServiceInstance.TestConnection(deviceID)
	if sshRes != nil && sshRes.Success {
		return true
	}

	// 3. ICMP Ping 连通性测试（强制绑定物理网卡，避开 TUN 虚拟网卡伪造 TTL 回复）
	if pingHost(ip) {
		return true
	}

	// 4. 仅当设备在最近采集窗口内仍有真实指标上报时保活展示；超过阈值未上报则判定离线
	if hasRecentMetrics(deviceID) {
		return true
	}

	return false
}

// pingHost 执行系统 ping 命令，返回是否可达
func pingHost(ip string) bool {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		physicalIP := getWindowsPhysicalSourceIP()
		if physicalIP != "" {
			cmd = exec.Command("ping", "-S", physicalIP, "-n", "1", "-w", "1500", ip)
		} else {
			cmd = exec.Command("ping", "-n", "1", "-w", "1500", ip)
		}
	} else {
		cmd = exec.Command("ping", "-c", "1", "-W", "2", ip)
	}

	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return isPingSuccess(string(output))
}

// isPingSuccess 检测 ping 输出是否表示成功（中英文兼容，先排除错误信息）
func isPingSuccess(output string) bool {
	s := strings.ToLower(output)

	// 1. 明确的丢包与不可达错误特征排查（Windows/Linux 中英文）
	if strings.Contains(s, "无法访问目标主机") ||
		strings.Contains(s, "destination host unreachable") ||
		strings.Contains(s, "请求超时") ||
		strings.Contains(s, "request timed out") ||
		strings.Contains(s, "100% 丢失") ||
		strings.Contains(s, "100% loss") ||
		strings.Contains(s, "0 received") ||
		strings.Contains(s, "0 packets received") ||
		strings.Contains(s, "已接收 = 0") ||
		strings.Contains(s, "transmit failed") ||
		strings.Contains(s, "general failure") ||
		strings.Contains(s, "传输失败") {
		return false
	}

	// 2. 成功的标志：必须包含实际的接收指示，绝不能单独判断 "来自"（因为报错信息也包含"来自"）
	if strings.Contains(s, "ttl=") {
		return true
	}
	if strings.Contains(s, "time=") || strings.Contains(s, "time<") || strings.Contains(s, "时间=") || strings.Contains(s, "时间<") {
		return true
	}
	if strings.Contains(s, "1 received") || strings.Contains(s, "1 packets received") || strings.Contains(s, "bytes from") {
		return true
	}
	if strings.Contains(s, "已接收 = 1") || strings.Contains(s, "received = 1") {
		return true
	}
	return false
}

// init 在包初始化时根据环境变量设置探测间隔
func init() {
	if v := os.Getenv("DEVICE_PROBE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			DeviceStatusProberInstance.interval = d
		}
	}
}
