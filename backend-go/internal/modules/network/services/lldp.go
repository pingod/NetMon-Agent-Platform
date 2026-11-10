// LLDP/CDP 邻居发现拓扑服务 — 等价于 TS lldpDiscoveryService.ts
// 用途：自动发现网络拓扑、构建链路图、告警影响范围定位
package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"itops-backend/internal/logger"
	"itops-backend/internal/modules/network/vendoradapter"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 类型定义
// ───────────────────────────────────────────────

// NeighborEntry 邻居条目
type NeighborEntry struct {
	LocalInterface  string `json:"localInterface"`
	RemoteDevice    string `json:"remoteDevice"`
	RemoteInterface string `json:"remoteInterface"`
	RemotePlatform  string `json:"remotePlatform,omitempty"`
	RemoteMgmtIP    string `json:"remoteMgmtIP,omitempty"`
	Protocol        string `json:"protocol"` // lldp | cdp
}

// ───────────────────────────────────────────────
// 服务单例
// ───────────────────────────────────────────────

var LldpDiscoveryServiceInstance = &LldpDiscoveryService{}

type LldpDiscoveryService struct{}

// DiscoverNeighbors 对单台设备执行 LLDP/CDP 邻居发现
func (s *LldpDiscoveryService) DiscoverNeighbors(deviceID string) ([]NeighborEntry, error) {
	device, err := NetworkDeviceServiceInstance.GetFullCredentials(deviceID)
	if err != nil {
		return nil, fmt.Errorf("设备不存在: %s", deviceID)
	}

	adapter := vendoradapter.CreateVendorAdapter(vendoradapter.VendorType(device.Vendor))
	neighborTpl, ok := adapter.GetCommand(vendoradapter.InspectNeighbor)
	if !ok {
		logger.Warnf("厂商 %s 没有邻居发现命令", device.Vendor)
		return []NeighborEntry{}, nil
	}

	// 通过 SSH exec 模式执行邻居发现命令（不需要分页处理）
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

	output, err := ConnectAndExec(sshDevice, neighborTpl.Command)
	if err != nil {
		logger.Errorf("LLDP 发现失败 %s: %v", device.Name, err)
		return []NeighborEntry{}, nil
	}

	// 解析邻居信息
	neighbors := parseNeighborOutput(device.Vendor, output)

	// 保存到数据库
	s.saveNeighborEntries(deviceID, neighbors)

	// 尝试建立双向拓扑关联
	s.buildTopologyLinks(deviceID, device.Name, neighbors)

	return neighbors, nil
}

// BatchDiscover 批量发现多台设备的 LLDP 邻居
func (s *LldpDiscoveryService) BatchDiscover(deviceIDs []string) map[string][]NeighborEntry {
	results := make(map[string][]NeighborEntry)
	for _, id := range deviceIDs {
		neighbors, _ := s.DiscoverNeighbors(id)
		results[id] = neighbors
	}
	return results
}

// DiscoverAll 发现所有在线设备的 LLDP 邻居
func (s *LldpDiscoveryService) DiscoverAll() map[string][]NeighborEntry {
	devices, err := repos.NetworkDevices.List()
	if err != nil {
		logger.Errorf("获取设备列表失败: %v", err)
		return map[string][]NeighborEntry{}
	}

	var onlineIDs []string
	for _, d := range devices {
		if d.Status == "online" || d.Status == "unknown" {
			onlineIDs = append(onlineIDs, d.ID)
		}
	}
	return s.BatchDiscover(onlineIDs)
}

// GetTopologyLinks 获取拓扑链路（可选按设备过滤）
func (s *LldpDiscoveryService) GetTopologyLinks(deviceID string) ([]repos.TopologyLink, error) {
	return repos.TopologyLinks.List(deviceID)
}

// GetAdjacentDevices 根据告警设备查找相邻设备
func (s *LldpDiscoveryService) GetAdjacentDevices(deviceID string) []string {
	links, err := repos.TopologyLinks.List(deviceID)
	if err != nil {
		return []string{}
	}

	adjSet := make(map[string]bool)
	for _, link := range links {
		if link.DeviceAID == deviceID {
			adjSet[link.DeviceBID] = true
		}
		if link.DeviceBID == deviceID {
			adjSet[link.DeviceAID] = true
		}
	}

	result := make([]string, 0, len(adjSet))
	for id := range adjSet {
		result = append(result, id)
	}
	return result
}

// GetImpactPath 获取完整的设备影响路径（BFS 遍历，最多 maxHops 跳）
func (s *LldpDiscoveryService) GetImpactPath(deviceID string, maxHops int) []string {
	if maxHops <= 0 {
		maxHops = 3
	}

	visited := make(map[string]bool)
	type queueItem struct {
		id   string
		hops int
	}
	queue := []queueItem{{id: deviceID, hops: 0}}
	visited[deviceID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.hops >= maxHops {
			continue
		}

		adj := s.GetAdjacentDevices(current.id)
		for _, neighborID := range adj {
			if !visited[neighborID] {
				visited[neighborID] = true
				queue = append(queue, queueItem{id: neighborID, hops: current.hops + 1})
			}
		}
	}

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	return result
}

// GetNeighbors 获取设备的 LLDP 邻居列表
func (s *LldpDiscoveryService) GetNeighbors(deviceID string) ([]repos.LldpNeighbor, error) {
	return repos.LldpNeighbors.ListByDeviceID(deviceID)
}

// GetExternalDevices 获取所有外部（未管理）设备
func (s *LldpDiscoveryService) GetExternalDevices() ([]repos.ExternalDevice, error) {
	return repos.ExternalDevices.List()
}

// ───────────────────────────────────────────────
// 私有方法
// ───────────────────────────────────────────────

// saveNeighborEntries 保存邻居条目到数据库
func (s *LldpDiscoveryService) saveNeighborEntries(deviceID string, neighbors []NeighborEntry) {
	// 先清除旧数据
	_ = repos.LldpNeighbors.ClearByDeviceID(deviceID)

	for _, n := range neighbors {
		_ = repos.LldpNeighbors.Insert(repos.LldpNeighborInsertInput{
			ID:               uuid.NewString(),
			DeviceID:         deviceID,
			LocalInterface:   n.LocalInterface,
			RemoteDeviceName: n.RemoteDevice,
			RemoteInterface:  n.RemoteInterface,
			RemotePlatform:   n.RemotePlatform,
			RemoteMgmtIP:     n.RemoteMgmtIP,
			Protocol:         n.Protocol,
		})
	}

	logger.Infof("保存 %d 条邻居记录 (设备: %s)", len(neighbors), deviceID)
}

// buildTopologyLinks 根据邻居信息建立拓扑链路
func (s *LldpDiscoveryService) buildTopologyLinks(deviceID, deviceName string, neighbors []NeighborEntry) {
	for _, n := range neighbors {
		// 查找远程设备是否已管理
		remoteID, remoteName, err := repos.NetworkDevices.GetByNameOrIP(n.RemoteDevice, n.RemoteMgmtIP)
		if err != nil {
			// 远程设备未管理，记录为外部设备（仅当不存在时插入）
			_, extErr := repos.ExternalDevices.GetByName(n.RemoteDevice)
			if extErr != nil {
				_ = repos.ExternalDevices.Insert(repos.ExternalDeviceInsertInput{
					ID:                     uuid.NewString(),
					Name:                   n.RemoteDevice,
					DiscoveredFromDeviceID: deviceID,
					Platform:               n.RemotePlatform,
					ManagementIP:           n.RemoteMgmtIP,
				})
			}
			continue
		}

		// 双向链路：检查是否已存在
		existingID, _ := repos.TopologyLinks.FindLink(deviceID, remoteID, n.LocalInterface, n.RemoteInterface)
		if existingID != "" {
			_ = repos.TopologyLinks.RefreshLink(existingID)
		} else {
			_ = repos.TopologyLinks.Create(repos.TopologyLinkCreateInput{
				ID:               uuid.NewString(),
				DeviceAID:        deviceID,
				DeviceAName:      deviceName,
				DeviceAInterface: n.LocalInterface,
				DeviceBID:        remoteID,
				DeviceBName:      remoteName,
				DeviceBInterface: n.RemoteInterface,
			})
			logger.Infof("新拓扑链路: %s:%s <-> %s:%s", deviceName, n.LocalInterface, remoteName, n.RemoteInterface)
		}
	}
}

// ───────────────────────────────────────────────
// 邻居输出解析（厂商特定）
// ───────────────────────────────────────────────

var multiSpaceRe = regexp.MustCompile(`\s{2,}`)

// parseNeighborOutput 解析邻居发现命令输出
func parseNeighborOutput(vendor, output string) []NeighborEntry {
	var neighbors []NeighborEntry
	lines := strings.Split(output, "\n")

	switch vendor {
	case "huawei", "h3c":
		neighbors = parseHuaweiH3cNeighbors(lines)
	case "cisco":
		neighbors = parseCiscoNeighbors(lines)
	case "fortinet":
		neighbors = parseFortinetNeighbors(lines)
	default:
		neighbors = parseGenericNeighbors(lines, output)
	}

	return neighbors
}

// parseHuaweiH3cNeighbors 解析华为/H3C LLDP 邻居输出
// display lldp neighbor brief 格式：
// LocalIntf   NeighborDev         NeighborIntf        Exptime
// GE0/0/1     SW-CORE-01           GE0/0/24             120s
func parseHuaweiH3cNeighbors(lines []string) []NeighborEntry {
	var neighbors []NeighborEntry
	inData := false
	for _, line := range lines {
		if strings.Contains(line, "LocalIntf") || strings.Contains(line, "NeighborDev") {
			inData = true
			continue
		}
		if !inData || strings.TrimSpace(line) == "" {
			continue
		}
		parts := multiSpaceRe.Split(strings.TrimSpace(line), -1)
		if len(parts) >= 3 {
			neighbors = append(neighbors, NeighborEntry{
				LocalInterface:  parts[0],
				RemoteDevice:    parts[1],
				RemoteInterface: parts[2],
				Protocol:        "lldp",
			})
		}
	}
	return neighbors
}

// parseCiscoNeighbors 解析 Cisco LLDP 邻居输出（detail 格式）
func parseCiscoNeighbors(lines []string) []NeighborEntry {
	var neighbors []NeighborEntry
	var cur NeighborEntry
	hasData := false

	flush := func() {
		if hasData && cur.LocalInterface != "" && cur.RemoteDevice != "" {
			neighbors = append(neighbors, cur)
		}
		cur = NeighborEntry{Protocol: "lldp"}
		hasData = false
	}

	cur = NeighborEntry{Protocol: "lldp"}

	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Local Intf:") {
			cur.LocalInterface = strings.TrimSpace(strings.TrimPrefix(t, "Local Intf:"))
			hasData = true
		} else if strings.HasPrefix(t, "Device ID:") {
			cur.RemoteDevice = strings.TrimSpace(strings.TrimPrefix(t, "Device ID:"))
			hasData = true
		} else if strings.HasPrefix(t, "Port id:") {
			cur.RemoteInterface = strings.TrimSpace(strings.TrimPrefix(t, "Port id:"))
		} else if strings.HasPrefix(t, "Platform:") {
			cur.RemotePlatform = strings.TrimSpace(strings.TrimPrefix(t, "Platform:"))
		} else if strings.HasPrefix(t, "Management Addresses:") || strings.HasPrefix(t, "Management Address:") {
			ip := strings.TrimSpace(strings.SplitN(t, ":", 2)[1])
			if ip != "" {
				cur.RemoteMgmtIP = ip
			}
		} else if t == "" {
			flush()
		}
	}
	flush() // 最后一条

	return neighbors
}

// parseFortinetNeighbors 解析 Fortinet LLDP 邻居输出
func parseFortinetNeighbors(lines []string) []NeighborEntry {
	var neighbors []NeighborEntry
	var cur NeighborEntry
	hasData := false

	flush := func() {
		if hasData && cur.LocalInterface != "" && cur.RemoteDevice != "" {
			neighbors = append(neighbors, cur)
		}
		cur = NeighborEntry{Protocol: "lldp"}
		hasData = false
	}

	cur = NeighborEntry{Protocol: "lldp"}

	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Port:") {
			cur.LocalInterface = strings.TrimSpace(strings.TrimPrefix(t, "Port:"))
			hasData = true
		} else if strings.HasPrefix(t, "Device:") {
			cur.RemoteDevice = strings.TrimSpace(strings.TrimPrefix(t, "Device:"))
			hasData = true
		} else if strings.HasPrefix(t, "Port desc:") {
			cur.RemoteInterface = strings.TrimSpace(strings.TrimPrefix(t, "Port desc:"))
		} else if strings.HasPrefix(t, "Platform:") {
			cur.RemotePlatform = strings.TrimSpace(strings.TrimPrefix(t, "Platform:"))
		} else if t == "" {
			flush()
		}
	}
	flush()

	return neighbors
}

// parseGenericNeighbors 通用解析（提取设备名和接口）
func parseGenericNeighbors(lines []string, output string) []NeighborEntry {
	var neighbors []NeighborEntry
	protocol := "lldp"
	if matched, _ := regexp.MatchString(`(?i)cdp`, output); matched {
		protocol = "cdp"
	}

	for _, line := range lines {
		if strings.Contains(line, "Local") || strings.Contains(line, "Neighbor") {
			continue
		}
		parts := multiSpaceRe.Split(strings.TrimSpace(line), -1)
		if len(parts) >= 3 {
			neighbors = append(neighbors, NeighborEntry{
				LocalInterface:  parts[0],
				RemoteDevice:    parts[1],
				RemoteInterface: parts[2],
				Protocol:        protocol,
			})
		}
	}
	return neighbors
}
