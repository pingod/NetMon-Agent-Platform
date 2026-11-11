// SNMP Trap 真实监听器：gosnmp.TrapListener 接收 v1/v2c Trap 与 Inform，落库 snmp_trap_events。
// 开关与端口持久化在 snmp_trap_config（单行），进程重启后按配置自动恢复监听。
package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"itops-backend/internal/appdb"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

const trapDefaultPort = 162

type SnmpTrapListenerService struct {
	mu        sync.Mutex
	listener  *gosnmp.TrapListener
	running   bool
	port      int
	startedAt time.Time
	received  atomic.Uint64
	lastError string
}

var SnmpTrapListenerInstance = &SnmpTrapListenerService{}

func (s *SnmpTrapListenerService) ensureConfigTable() {
	if db := appdb.DB(); db != nil {
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS snmp_trap_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			enabled INTEGER NOT NULL DEFAULT 0,
			port INTEGER NOT NULL DEFAULT 162,
			updated_at TEXT
		)`)
	}
}

func (s *SnmpTrapListenerService) loadConfig() (enabled bool, port int) {
	s.ensureConfigTable()
	port = trapDefaultPort
	var en int
	if err := appdb.DB().QueryRow(`SELECT enabled, port FROM snmp_trap_config WHERE id = 1`).Scan(&en, &port); err != nil {
		return false, trapDefaultPort
	}
	if port <= 0 || port > 65535 {
		port = trapDefaultPort
	}
	return en == 1, port
}

func (s *SnmpTrapListenerService) saveConfig(enabled bool, port int) {
	s.ensureConfigTable()
	en := 0
	if enabled {
		en = 1
	}
	_, _ = appdb.DB().Exec(`INSERT INTO snmp_trap_config (id, enabled, port, updated_at) VALUES (1, ?, ?, datetime('now','localtime'))
		ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, port = excluded.port, updated_at = excluded.updated_at`, en, port)
}

// Start 启动监听。port<=0 时使用已保存端口（默认 162）。
func (s *SnmpTrapListenerService) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("Trap 监听已在运行（端口 %d）", s.port)
	}
	if port <= 0 {
		_, port = s.loadConfig()
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("端口非法: %d", port)
	}

	tl := gosnmp.NewTrapListener()
	tl.Params = &gosnmp.GoSNMP{Logger: gosnmp.NewLogger(log.New(io.Discard, "", 0))}
	tl.OnNewTrap = s.handleTrap

	errCh := make(chan error, 1)
	go func() {
		err := tl.Listen(fmt.Sprintf("0.0.0.0:%d", port))
		errCh <- err
		s.mu.Lock()
		if s.listener == tl {
			s.running = false
			if err != nil {
				s.lastError = err.Error()
			}
		}
		s.mu.Unlock()
	}()

	select {
	case <-tl.Listening():
	case err := <-errCh:
		if err == nil {
			err = fmt.Errorf("监听意外退出")
		}
		return fmt.Errorf("启动 Trap 监听失败: %w", err)
	case <-time.After(3 * time.Second):
		tl.Close()
		return fmt.Errorf("启动 Trap 监听超时（端口 %d）", port)
	}

	s.listener = tl
	s.running = true
	s.port = port
	s.startedAt = time.Now()
	s.lastError = ""
	s.saveConfig(true, port)
	logger.Infof("SNMP Trap listener started on 0.0.0.0:%d", port)
	return nil
}

// Stop 停止监听并记住「关闭」状态，重启后不再自动恢复。
func (s *SnmpTrapListenerService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.listener == nil {
		return fmt.Errorf("Trap 监听未在运行")
	}
	s.listener.Close()
	s.listener = nil
	s.running = false
	s.saveConfig(false, s.port)
	logger.Infof("SNMP Trap listener stopped")
	return nil
}

// Restore 进程启动时按持久化配置恢复监听；失败仅记录日志，不阻塞主服务。
func (s *SnmpTrapListenerService) Restore() {
	if enabled, port := s.loadConfig(); enabled {
		if err := s.Start(port); err != nil {
			logger.Warnf("SNMP Trap listener restore failed: %v", err)
		}
	}
}

// Status 返回监听器运行状态。
func (s *SnmpTrapListenerService) Status() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := map[string]interface{}{
		"running":   s.running,
		"port":      s.port,
		"received":  s.received.Load(),
		"last_error": s.lastError,
	}
	if !s.running {
		_, savedPort := s.loadConfig()
		st["port"] = savedPort
	}
	if s.running {
		st["started_at"] = s.startedAt.Format("2006-01-02 15:04:05")
	}
	return st
}

var trapGenericNames = map[int]string{
	0: "coldStart", 1: "warmStart", 2: "linkDown", 3: "linkUp",
	4: "authenticationFailure", 5: "egpNeighborLoss", 6: "enterpriseSpecific",
}

var trapOIDNames = map[string]string{
	"1.3.6.1.6.3.1.1.5.1": "coldStart",
	"1.3.6.1.6.3.1.1.5.2": "warmStart",
	"1.3.6.1.6.3.1.1.5.3": "linkDown",
	"1.3.6.1.6.3.1.1.5.4": "linkUp",
	"1.3.6.1.6.3.1.1.5.5": "authenticationFailure",
	"1.3.6.1.6.3.1.1.5.6": "egpNeighborLoss",
}

func (s *SnmpTrapListenerService) handleTrap(pkt *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	s.received.Add(1)
	sourceIP := ""
	if addr != nil {
		sourceIP = addr.IP.String()
	}

	trapType := "unknown"
	enterprise := strings.TrimPrefix(pkt.Enterprise, ".")
	agentAddr := pkt.AgentAddress
	generic, specific := pkt.GenericTrap, pkt.SpecificTrap

	if pkt.Version == gosnmp.Version1 {
		if name, ok := trapGenericNames[generic]; ok {
			trapType = name
		}
		if agentAddr == "" {
			agentAddr = sourceIP
		}
	} else {
		// v2c/v3：snmpTrapOID.0 标识陷阱类型
		for _, v := range pkt.Variables {
			if strings.TrimPrefix(v.Name, ".") == "1.3.6.1.6.3.1.1.4.1.0" {
				enterprise = strings.TrimPrefix(trapPDUString(v), ".")
				if name, ok := trapOIDNames[enterprise]; ok {
					trapType = name
				} else if enterprise != "" {
					trapType = "enterpriseSpecific"
				}
				break
			}
		}
		agentAddr = sourceIP
	}

	type varbind struct {
		OID   string      `json:"oid"`
		Value interface{} `json:"value"`
		Type  int         `json:"type"`
	}
	binds := make([]varbind, 0, len(pkt.Variables))
	for _, v := range pkt.Variables {
		binds = append(binds, varbind{OID: strings.TrimPrefix(v.Name, "."), Value: trapPDUValue(v), Type: int(v.Type)})
	}
	bindsJSON := "[]"
	if b, err := json.Marshal(binds); err == nil {
		bindsJSON = string(b)
	}

	if err := repos.SnmpTraps.Insert(repos.SnmpTrapEvent{
		ID:            uuid.NewString(),
		SourceIP:      sourceIP,
		TrapType:      trapType,
		EnterpriseOID: enterprise,
		AgentAddress:  agentAddr,
		GenericType:   generic,
		SpecificType:  specific,
		VarbindsJSON:  bindsJSON,
	}); err != nil {
		logger.Warnf("SNMP Trap 落库失败（来源 %s）: %v", sourceIP, err)
	}
}

func trapPDUString(v gosnmp.SnmpPDU) string {
	switch val := v.Value.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func trapPDUValue(v gosnmp.SnmpPDU) interface{} {
	if b, ok := v.Value.([]byte); ok {
		return string(b)
	}
	return v.Value
}