// Package demoseed 示例演示数据自动生成服务
// 当系统全新安装或数据较少时，自动补充插入预设的仿真网络设备、指标、巡检、告警、服务器与网段数据
package demoseed

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"itops-backend/internal/logger"
)

// EnsureDemoData 检查并为系统填充示例演示数据（幂等）
func EnsureDemoData(db *sql.DB, metricsDb *sql.DB) error {
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM network_devices").Scan(&count)
	if count >= 8 {
		logger.Infof("网络设备数量已充足 (%d 台)，无需补充示例数据", count)
		return nil
	}

	logger.Infof("🌱 正在初始化/补充示例演示数据 (现有设备: %d 台)...", count)

	// 1. 插入示例网络设备
	type demoDev struct {
		id, name, ip, vendor, model, role, location, status, deviceType string
		snmpEnabled                                                     int
	}

	devices := []demoDev{
		{"dev-hw-core-01", "HW-Core-SW-01", "192.168.1.1", "huawei", "CloudEngine 12800", "core", "中心机房 Rack-A01", "online", "switch", 1},
		{"dev-h3c-core-02", "H3C-Core-SW-02", "192.168.1.2", "h3c", "S12500X-AF", "core", "中心机房 Rack-A02", "online", "switch", 1},
		{"dev-cisco-gw-01", "Cisco-GW-RT-01", "192.168.1.254", "cisco", "ISR4451-X", "gateway", "中心机房 Rack-B01", "online", "router", 1},
		{"dev-hw-agg-03", "HW-Agg-SW-03", "192.168.2.1", "huawei", "S5735-S48T4X", "aggregation", "楼宇机房 Rack-C01", "online", "switch", 1},
		{"dev-h3c-acc-04", "H3C-Access-SW-04", "192.168.2.2", "h3c", "S5130S-52P-PWR", "access", "楼宇机房 Rack-C02", "online", "switch", 1},
		{"dev-ruijie-ac-01", "Ruijie-AC-01", "192.168.1.100", "ruijie", "WS6816", "wireless_ac", "无线控室 Rack-D01", "online", "wlc", 1},
		{"dev-zte-rt-01", "ZTE-Core-Router", "192.168.1.250", "zte", "ZXR10 M6000", "router", "出口机房 Rack-E01", "online", "router", 1},
		{"dev-cisco-fw-01", "Cisco-FW-01", "192.168.1.253", "cisco", "ASA5585-X", "firewall", "安全机房 Rack-F01", "offline", "firewall", 1},
	}

	for _, d := range devices {
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM network_devices WHERE name = ? OR ip_address = ?", d.name, d.ip).Scan(&exists)
		if exists == 0 {
			_, err := db.Exec(`
				INSERT INTO network_devices (
					id, name, ip_address, vendor, model, os_version, ssh_port,
					location, role, status, snmp_enabled, snmp_port, device_type, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 'v8.20', 22, ?, ?, ?, ?, 161, ?, datetime('now','localtime'), datetime('now','localtime'))
			`, d.id, d.name, d.ip, d.vendor, d.model, d.location, d.role, d.status, d.snmpEnabled, d.deviceType)
			if err != nil {
				logger.Warnf("插入演示设备失败 (%s): %v", d.name, err)
			}
		}
	}
	logger.Infof("✅ 示例网络设备就绪")

	// 2. 插入 SNMP 接口指标数据
	now := time.Now().Format("2006-01-02 15:04:05")
	targetMetricsDb := metricsDb
	if targetMetricsDb == nil {
		targetMetricsDb = db
	}

	interfaceConfigs := []struct {
		devID     string
		ifIndex   int
		descr     string
		oper      int
		speed     float64
		inOctets  float64
		outOctets float64
		inBps     float64
		outBps    float64
	}{
		{"dev-hw-core-01", 1, "Ten-GigabitEthernet0/0/1", 1, 10000000000, 524288000, 314572800, 85400000, 42100000},
		{"dev-hw-core-01", 2, "Ten-GigabitEthernet0/0/2", 1, 10000000000, 419430400, 209715200, 45200000, 21000000},
		{"dev-hw-core-01", 3, "GigabitEthernet0/0/1", 1, 1000000000, 104857600, 52428800, 12500000, 8400000},
		{"dev-hw-core-01", 4, "GigabitEthernet0/0/2", 2, 1000000000, 0, 0, 0, 0},
		{"dev-h3c-core-02", 1, "FortyGigE1/0/1", 1, 40000000000, 1073741824, 838860800, 124000000, 98000000},
		{"dev-h3c-core-02", 2, "Ten-GigabitEthernet1/0/1", 1, 10000000000, 629145600, 419430400, 68000000, 35000000},
		{"dev-h3c-core-02", 3, "GigabitEthernet1/0/1", 1, 1000000000, 209715200, 104857600, 18000000, 9200000},
		{"dev-cisco-gw-01", 1, "GigabitEthernet0/0/0", 1, 1000000000, 314572800, 157286400, 32000000, 16000000},
		{"dev-cisco-gw-01", 2, "GigabitEthernet0/0/1", 1, 1000000000, 157286400, 104857600, 14000000, 8000000},
		{"dev-hw-agg-03", 1, "GigabitEthernet0/0/1", 1, 1000000000, 419430400, 209715200, 28000000, 14000000},
		{"dev-h3c-acc-04", 1, "GigabitEthernet1/0/1", 1, 1000000000, 157286400, 52428800, 12000000, 5000000},
		{"dev-ruijie-ac-01", 1, "GigabitEthernet0/1", 1, 1000000000, 262144000, 131072000, 22000000, 11000000},
		{"dev-zte-rt-01", 1, "GE0/0/0", 1, 1000000000, 524288000, 262144000, 42000000, 24000000},
	}

	for _, ic := range interfaceConfigs {
		var exists int
		targetMetricsDb.QueryRow("SELECT COUNT(*) FROM snmp_interface_metrics WHERE device_id = ? AND if_index = ?", ic.devID, ic.ifIndex).Scan(&exists)
		if exists == 0 {
			targetMetricsDb.Exec(`
				INSERT INTO snmp_interface_metrics (
					id, device_id, if_index, if_descr, if_oper_status, if_admin_status, if_speed,
					if_in_octets, if_out_octets, throughput_in_bps, throughput_out_bps,
					if_in_errors, if_out_errors, sampled_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)
			`, uuid.New().String(), ic.devID, ic.ifIndex, ic.descr, ic.oper, ic.speed, ic.inOctets, ic.outOctets, ic.inBps, ic.outBps, now, now, now)
		}
	}
	logger.Infof("✅ 示例 SNMP 接口指标数据就绪")

	// 3. 插入示例巡检记录
	inspections := []struct {
		devID  string
		itype  string
		status string
		dur    int64
		sum    string
	}{
		{"dev-hw-core-01", "routine", "passed", 1250, "核心交换机定期例行巡检完成，CPU利用率18%，内存利用率34%，风暴控制正常"},
		{"dev-h3c-core-02", "deep", "passed", 2300, "深度全量接口巡检完成，所有40G上行端口无错包及丢包记录"},
		{"dev-cisco-gw-01", "security", "passed", 1800, "边界路由器ACL配置及BGP对等体状态巡检正常"},
		{"dev-hw-agg-03", "routine", "passed", 950, "汇聚交换机温度及风扇状态正常"},
		{"dev-cisco-fw-01", "routine", "warning", 500, "防火墙心跳离线报警，备用链路已接管"},
	}

	for _, ins := range inspections {
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM network_inspection_history WHERE device_id = ?", ins.devID).Scan(&exists)
		if exists == 0 {
			db.Exec(`
				INSERT INTO network_inspection_history (
					id, device_id, inspection_type, status, duration_ms, summary, created_at
				) VALUES (?, ?, ?, ?, ?, ?, datetime('now','localtime'))
			`, uuid.New().String(), ins.devID, ins.itype, ins.status, ins.dur, ins.sum)
		}
	}

	// 4. 插入示例告警记录
	alerts := []struct {
		title    string
		severity string
		message  string
		status   string
		devID    string
	}{
		{"Cisco-FW-01 防火墙设备离线", "critical", "安全机房边界防火墙心跳包丢失，SNMP 与 SSH 响应超时", "open", "dev-cisco-fw-01"},
		{"HW-Core-SW-01 上行接口流量陡增", "warning", "Ten-GigabitEthernet0/0/1 入方向带利用率持续5分钟超过75%", "acknowledged", "dev-hw-core-01"},
		{"H3C-Access-SW-04 接入端口链路 Down", "warning", "接入交换机4号端口链路断开，影响办公网段设备连通", "open", "dev-h3c-acc-04"},
		{"Cisco-GW-RT-01 CPU 温度触发预警", "info", "网关路由器芯片环境温度达到 45°C (阀值 50°C)", "resolved", "dev-cisco-gw-01"},
	}

	for i, alt := range alerts {
		var exists int
		alertID := fmt.Sprintf("ALT-20260813-%03d", i+1)
		db.QueryRow("SELECT COUNT(*) FROM alerts WHERE alert_id = ?", alertID).Scan(&exists)
		if exists == 0 {
			db.Exec(`
				INSERT INTO alerts (
					id, alert_id, title, severity, message, source, status, device_id, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 'network', ?, ?, datetime('now','localtime'), datetime('now','localtime'))
			`, alertID, alertID, alt.title, alt.severity, alt.message, alt.status, alt.devID)
		}
	}

	// 5. 插入示例服务器及指标数据
	servers := []struct {
		id, name, ip, os, status string
		cpu, mem, disk           float64
	}{
		{"svr-app-01", "SVR-APP-01", "192.168.1.50", "Ubuntu 22.04 LTS", "online", 24.5, 42.1, 38.0},
		{"svr-db-01", "SVR-DB-MASTER", "192.168.1.60", "CentOS 7.9", "online", 48.2, 76.5, 62.4},
		{"svr-k8s-01", "SVR-K8S-NODE1", "192.168.1.70", "Debian 12", "online", 61.0, 58.3, 45.1},
	}

	for _, s := range servers {
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM servers WHERE name = ?", s.name).Scan(&exists)
		if exists == 0 {
			db.Exec(`
				INSERT INTO servers (
					id, name, ip_address, status, os_name, cpu_usage, memory_usage, disk_usage, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now','localtime'), datetime('now','localtime'))
			`, s.id, s.name, s.ip, s.status, s.os, s.cpu, s.mem, s.disk)

			targetMetricsDb.Exec(`
				INSERT INTO server_metrics (
					id, server_id, cpu_usage, memory_usage, memory_total_gb, memory_used_gb,
					disk_usage, disk_total_gb, disk_used_gb, network_in_mbps, network_out_mbps, collected_at
				) VALUES (?, ?, ?, ?, 64.0, ?, ?, 500.0, ?, 12.5, 8.4, datetime('now','localtime'))
			`, uuid.New().String(), s.id, s.cpu, s.mem, s.mem*0.64, s.disk, s.disk*5.0)
		}
	}

	// 6. 插入示例子网数据
	subnets := []struct {
		name, cidr string
		vlan       int
		used       int
	}{
		{"Core-Mgmt-Net", "192.168.1.0/24", 10, 48},
		{"Office-Access-Net", "192.168.2.0/24", 20, 182},
		{"DC-Server-Net", "10.0.1.0/24", 100, 64},
	}

	for _, sub := range subnets {
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM network_subnets WHERE cidr = ?", sub.cidr).Scan(&exists)
		if exists == 0 {
			db.Exec(`
				INSERT INTO network_subnets (
					id, name, cidr, vlan_id, gateway, total_ips, used_ips, description, created_at, updated_at
				) VALUES (?, ?, ?, ?, '192.168.1.254', 254, ?, '示例网络子网', datetime('now','localtime'), datetime('now','localtime'))
			`, uuid.New().String(), sub.name, sub.cidr, sub.vlan, sub.used)
		}
	}

	logger.Infof("🎉 示例演示数据已完美就绪！")
	return nil
}
