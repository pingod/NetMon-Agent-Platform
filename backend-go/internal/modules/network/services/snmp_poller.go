// SNMP 轮询服务 — 支持单设备/按模块（系统模块、接口模块）独立自定义轮询间隔
package services

import (
	"database/sql"
	"os"
	"sync"
	"time"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
)

// SnmpPoller SNMP 轮询服务
type SnmpPoller struct {
	mu                sync.Mutex
	stopCh            chan struct{}
	running           bool
	tickerInterval    time.Duration
	lastSystemPoll    map[string]time.Time
	lastInterfacePoll map[string]time.Time
}

// SnmpPollerInstance 全局单例
var SnmpPollerInstance = &SnmpPoller{
	tickerInterval:    10 * time.Second, // 10秒检查一次到达周期的设备
	lastSystemPoll:    make(map[string]time.Time),
	lastInterfacePoll: make(map[string]time.Time),
}

// Start 启动定时轮询
func (p *SnmpPoller) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	go p.loop()
	logger.Infof("SNMP 分模块独立轮询调度器已启动（精准调度粒度 %s）", p.tickerInterval)
}

// Shutdown 停止轮询
func (p *SnmpPoller) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running && p.stopCh != nil {
		close(p.stopCh)
		p.running = false
	}
}

// SetInterval 设置检查粒度
func (p *SnmpPoller) SetInterval(d time.Duration) {
	p.mu.Lock()
	p.tickerInterval = d
	p.mu.Unlock()
}

func (p *SnmpPoller) loop() {
	// 首次延迟 15 秒执行
	select {
	case <-p.stopCh:
		return
	case <-time.After(15 * time.Second):
	}

	p.pollDueDevices()

	ticker := time.NewTicker(p.tickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			logger.Infof("SNMP 轮询服务已停止")
			return
		case <-ticker.C:
			p.pollDueDevices()
		}
	}
}

type devPollConfig struct {
	id                string
	name              string
	systemInterval    int
	interfaceInterval int
}

// pollDueDevices 检查到达系统模块或接口模块采间隔的设备并执行轮询
func (p *SnmpPoller) pollDueDevices() {
	db := appdb.DB()

	rows, err := db.Query(`
		SELECT id, name, COALESCE(system_poll_interval, 300), COALESCE(interface_poll_interval, 60) 
		FROM network_devices 
		WHERE snmp_enabled = 1`)
	if err != nil {
		logger.Errorf("SNMP 轮询查询设备列表失败: %v", err)
		return
	}
	defer rows.Close()

	var devices []devPollConfig
	for rows.Next() {
		var d devPollConfig
		var sys, iface sql.NullInt64
		if err := rows.Scan(&d.id, &d.name, &sys, &iface); err == nil {
			d.systemInterval = int(sys.Int64)
			d.interfaceInterval = int(iface.Int64)
			if d.systemInterval <= 0 {
				d.systemInterval = 300
			}
			if d.interfaceInterval <= 0 {
				d.interfaceInterval = 60
			}
			devices = append(devices, d)
		}
	}

	if len(devices) == 0 {
		return
	}

	now := time.Now()
	var dueSystem []devPollConfig
	var dueInterface []devPollConfig

	p.mu.Lock()
	for _, d := range devices {
		lastSys, sysExist := p.lastSystemPoll[d.id]
		if !sysExist || now.Sub(lastSys) >= time.Duration(d.systemInterval)*time.Second {
			dueSystem = append(dueSystem, d)
			p.lastSystemPoll[d.id] = now
		}

		lastIface, ifaceExist := p.lastInterfacePoll[d.id]
		if !ifaceExist || now.Sub(lastIface) >= time.Duration(d.interfaceInterval)*time.Second {
			dueInterface = append(dueInterface, d)
			p.lastInterfacePoll[d.id] = now
		}
	}
	p.mu.Unlock()

	// 需要被轮询的合集设备
	toPollMap := make(map[string]devPollConfig)
	for _, d := range dueInterface {
		toPollMap[d.id] = d
	}
	for _, d := range dueSystem {
		toPollMap[d.id] = d
	}

	if len(toPollMap) == 0 {
		return
	}

	logger.Debugf("开始 SNMP 调度轮询 %d 台到达采集间隔的设备...", len(toPollMap))

	concurrency := 16
	if len(toPollMap) < concurrency {
		concurrency = len(toPollMap)
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, d := range toPollMap {
		wg.Add(1)
		go func(device devPollConfig) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := SnmpServiceInstance.InspectDevice(device.id)
			if result != nil && result.Reachable {
				logger.Debugf("SNMP 轮询成功: %s (%s) - %d 接口, %d UP, %d DOWN, %dms",
					device.name, device.id, result.InterfaceCount, result.UpCount, result.DownCount, result.PollDurationMs)
			}
		}(d)
	}
	wg.Wait()
}

func init() {
	if v := os.Getenv("SNMP_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			SnmpPollerInstance.tickerInterval = d
		}
	}
}
