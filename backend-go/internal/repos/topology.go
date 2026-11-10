// LLDP 邻居/外部设备/拓扑链路，等价于 TS networkDeviceRepository/lldp.ts
package repos

import (
	"database/sql"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// LLDP 邻居
// ───────────────────────────────────────────────

// LldpNeighbor LLDP 邻居记录
type LldpNeighbor struct {
	ID                string  `json:"id"`
	DeviceID          string  `json:"device_id"`
	LocalInterface    *string `json:"local_interface"`
	RemoteDeviceName  *string `json:"remote_device_name"`
	RemoteInterface   *string `json:"remote_interface"`
	RemotePlatform    *string `json:"remote_platform"`
	RemoteMgmtIP      *string `json:"remote_mgmt_ip"`
	Protocol          string  `json:"protocol"`
	CreatedAt         string  `json:"created_at"`
}

// LldpNeighborRepo LLDP 邻居仓库
type LldpNeighborRepo struct{}

var LldpNeighbors = &LldpNeighborRepo{}

// ClearByDeviceID 清除设备的所有 LLDP 邻居
func (r *LldpNeighborRepo) ClearByDeviceID(deviceID string) error {
	_, err := appdb.DB().Exec(`DELETE FROM network_lldp_neighbors WHERE device_id = ?`, deviceID)
	return err
}

// LldpNeighborInsertInput 插入邻居输入
type LldpNeighborInsertInput struct {
	ID               string
	DeviceID         string
	LocalInterface   string
	RemoteDeviceName string
	RemoteInterface  string
	RemotePlatform   string
	RemoteMgmtIP     string
	Protocol         string
}

// Insert 插入 LLDP 邻居
func (r *LldpNeighborRepo) Insert(input LldpNeighborInsertInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.Protocol == "" {
		input.Protocol = "lldp"
	}
	_, err := appdb.DB().Exec(`
		INSERT INTO network_lldp_neighbors (id, device_id, local_interface, remote_device_name, remote_interface, remote_platform, remote_mgmt_ip, protocol)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.DeviceID, sqlNullStr(input.LocalInterface), sqlNullStr(input.RemoteDeviceName),
		sqlNullStr(input.RemoteInterface), sqlNullStr(input.RemotePlatform), sqlNullStr(input.RemoteMgmtIP), input.Protocol)
	return err
}

// ListByDeviceID 列出设备的 LLDP 邻居
func (r *LldpNeighborRepo) ListByDeviceID(deviceID string) ([]LldpNeighbor, error) {
	rows, err := appdb.DB().Query(`SELECT id, device_id, local_interface, remote_device_name, remote_interface, remote_platform, remote_mgmt_ip, protocol, created_at FROM network_lldp_neighbors WHERE device_id = ?`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LldpNeighbor
	for rows.Next() {
		var n LldpNeighbor
		var localIf, remoteName, remoteIf, remotePlatform, remoteMgmtIP sql.NullString
		if err := rows.Scan(&n.ID, &n.DeviceID, &localIf, &remoteName, &remoteIf, &remotePlatform, &remoteMgmtIP, &n.Protocol, &n.CreatedAt); err != nil {
			continue
		}
		n.LocalInterface = ns(localIf)
		n.RemoteDeviceName = ns(remoteName)
		n.RemoteInterface = ns(remoteIf)
		n.RemotePlatform = ns(remotePlatform)
		n.RemoteMgmtIP = ns(remoteMgmtIP)
		out = append(out, n)
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 外部设备
// ───────────────────────────────────────────────

// ExternalDevice 外部设备记录
type ExternalDevice struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	DiscoveredFromDeviceID *string `json:"discovered_from_device_id"`
	Platform              *string `json:"platform"`
	ManagementIP          *string `json:"management_ip"`
	LastSeenAt            string  `json:"last_seen_at"`
}

// ExternalDeviceRepo 外部设备仓库
type ExternalDeviceRepo struct{}

var ExternalDevices = &ExternalDeviceRepo{}

// GetByName 按名称查找外部设备
func (r *ExternalDeviceRepo) GetByName(name string) (string, error) {
	var id string
	err := appdb.DB().QueryRow(`SELECT id FROM network_external_devices WHERE name = ?`, name).Scan(&id)
	return id, err
}

// ExternalDeviceInsertInput 插入外部设备输入
type ExternalDeviceInsertInput struct {
	ID                    string
	Name                  string
	DiscoveredFromDeviceID string
	Platform              string
	ManagementIP          string
}

// Insert 插入外部设备
func (r *ExternalDeviceRepo) Insert(input ExternalDeviceInsertInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		INSERT INTO network_external_devices (id, name, discovered_from_device_id, platform, management_ip, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, sqlNullStr(input.DiscoveredFromDeviceID), sqlNullStr(input.Platform), sqlNullStr(input.ManagementIP), now)
	return err
}

// List 列出所有外部设备
func (r *ExternalDeviceRepo) List() ([]ExternalDevice, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, discovered_from_device_id, platform, management_ip, last_seen_at FROM network_external_devices ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExternalDevice
	for rows.Next() {
		var d ExternalDevice
		var fromDeviceID, platform, mgmtIP sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &fromDeviceID, &platform, &mgmtIP, &d.LastSeenAt); err != nil {
			continue
		}
		d.DiscoveredFromDeviceID = ns(fromDeviceID)
		d.Platform = ns(platform)
		d.ManagementIP = ns(mgmtIP)
		out = append(out, d)
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 拓扑链路
// ───────────────────────────────────────────────

// TopologyLink 拓扑链路记录
type TopologyLink struct {
	ID               string  `json:"id"`
	DeviceAID        string  `json:"deviceA_id"`
	DeviceAName      *string `json:"deviceA_name"`
	DeviceAInterface *string `json:"deviceA_interface"`
	DeviceBID        string  `json:"deviceB_id"`
	DeviceBName      *string `json:"deviceB_name"`
	DeviceBInterface *string `json:"deviceB_interface"`
	Status           string  `json:"status"`
	LastSeenAt       string  `json:"last_seen_at"`
	CreatedAt        string  `json:"created_at"`
}

// TopologyLinkRepo 拓扑链路仓库
type TopologyLinkRepo struct{}

var TopologyLinks = &TopologyLinkRepo{}

// FindLink 查找拓扑链路（双向匹配）
func (r *TopologyLinkRepo) FindLink(deviceAID, deviceBID, deviceAInterface, deviceBInterface string) (string, error) {
	var id string
	err := appdb.DB().QueryRow(`
		SELECT id FROM network_topology_links
		WHERE (deviceA_id = ? AND deviceB_id = ? AND deviceA_interface = ? AND deviceB_interface = ?)
		   OR (deviceA_id = ? AND deviceB_id = ? AND deviceA_interface = ? AND deviceB_interface = ?)`,
		deviceAID, deviceBID, deviceAInterface, deviceBInterface,
		deviceBID, deviceAID, deviceBInterface, deviceAInterface).Scan(&id)
	return id, err
}

// RefreshLink 刷新链路状态
func (r *TopologyLinkRepo) RefreshLink(id string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`UPDATE network_topology_links SET status = 'active', last_seen_at = ? WHERE id = ?`, now, id)
	return err
}

// TopologyLinkCreateInput 创建链路输入
type TopologyLinkCreateInput struct {
	ID               string
	DeviceAID        string
	DeviceAName      string
	DeviceAInterface string
	DeviceBID        string
	DeviceBName      string
	DeviceBInterface string
}

// Create 创建拓扑链路
func (r *TopologyLinkRepo) Create(input TopologyLinkCreateInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	_, err := appdb.DB().Exec(`
		INSERT INTO network_topology_links (id, deviceA_id, deviceA_name, deviceA_interface, deviceB_id, deviceB_name, deviceB_interface, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active')`,
		input.ID, input.DeviceAID, sqlNullStr(input.DeviceAName), sqlNullStr(input.DeviceAInterface),
		input.DeviceBID, sqlNullStr(input.DeviceBName), sqlNullStr(input.DeviceBInterface))
	return err
}

// List 列出拓扑链路（可选按设备过滤）
func (r *TopologyLinkRepo) List(deviceID string) ([]TopologyLink, error) {
	var query string
	var args []interface{}
	if deviceID != "" {
		query = `SELECT id, deviceA_id, deviceA_name, deviceA_interface, deviceB_id, deviceB_name, deviceB_interface, status, last_seen_at, created_at FROM network_topology_links WHERE deviceA_id = ? OR deviceB_id = ? ORDER BY last_seen_at DESC`
		args = append(args, deviceID, deviceID)
	} else {
		query = `SELECT id, deviceA_id, deviceA_name, deviceA_interface, deviceB_id, deviceB_name, deviceB_interface, status, last_seen_at, created_at FROM network_topology_links WHERE status = 'active' ORDER BY last_seen_at DESC`
	}
	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopologyLink
	for rows.Next() {
		var l TopologyLink
		var deviceAName, deviceAInterface, deviceBName, deviceBInterface sql.NullString
		if err := rows.Scan(&l.ID, &l.DeviceAID, &deviceAName, &deviceAInterface, &l.DeviceBID, &deviceBName, &deviceBInterface, &l.Status, &l.LastSeenAt, &l.CreatedAt); err != nil {
			continue
		}
		l.DeviceAName = ns(deviceAName)
		l.DeviceAInterface = ns(deviceAInterface)
		l.DeviceBName = ns(deviceBName)
		l.DeviceBInterface = ns(deviceBInterface)
		out = append(out, l)
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 自定义网络拓扑图持久化存储
// ───────────────────────────────────────────────

// CustomNetworkTopology 自定义网络拓扑模型
type CustomNetworkTopology struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	BackgroundImage string `json:"background_image"`
	NodesJSON       string `json:"nodes_json"`
	LinksJSON       string `json:"links_json"`
	IsDefault       int    `json:"is_default"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type CustomTopologyRepo struct{}

var CustomTopologies = &CustomTopologyRepo{}

func (r *CustomTopologyRepo) EnsureTable() {
	_, _ = appdb.DB().Exec(`
		CREATE TABLE IF NOT EXISTS custom_network_topologies (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			background_image TEXT DEFAULT '',
			nodes_json TEXT DEFAULT '[]',
			links_json TEXT DEFAULT '[]',
			is_default INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now','localtime')),
			updated_at TEXT DEFAULT (datetime('now','localtime'))
		)`)
}

func (r *CustomTopologyRepo) List() ([]CustomNetworkTopology, error) {
	r.EnsureTable()
	rows, err := appdb.DB().Query(`
		SELECT id, name, description, background_image, nodes_json, links_json, is_default, created_at, updated_at
		FROM custom_network_topologies
		ORDER BY is_default DESC, updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CustomNetworkTopology
	for rows.Next() {
		var t CustomNetworkTopology
		var desc, bg, nodes, links, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &desc, &bg, &nodes, &links, &t.IsDefault, &createdAt, &updatedAt); err == nil {
			t.Description = desc.String
			t.BackgroundImage = bg.String
			t.NodesJSON = nodes.String
			t.LinksJSON = links.String
			t.CreatedAt = createdAt.String
			t.UpdatedAt = updatedAt.String
			list = append(list, t)
		}
	}
	if list == nil {
		list = []CustomNetworkTopology{}
	}
	return list, nil
}

func (r *CustomTopologyRepo) GetByID(id string) (*CustomNetworkTopology, error) {
	r.EnsureTable()
	var t CustomNetworkTopology
	var desc, bg, nodes, links, createdAt, updatedAt sql.NullString
	err := appdb.DB().QueryRow(`
		SELECT id, name, description, background_image, nodes_json, links_json, is_default, created_at, updated_at
		FROM custom_network_topologies
		WHERE id = ?`, id).Scan(&t.ID, &t.Name, &desc, &bg, &nodes, &links, &t.IsDefault, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Description = desc.String
	t.BackgroundImage = bg.String
	t.NodesJSON = nodes.String
	t.LinksJSON = links.String
	t.CreatedAt = createdAt.String
	t.UpdatedAt = updatedAt.String
	return &t, nil
}

func (r *CustomTopologyRepo) Create(t CustomNetworkTopology) (*CustomNetworkTopology, error) {
	r.EnsureTable()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.NodesJSON == "" {
		t.NodesJSON = "[]"
	}
	if t.LinksJSON == "" {
		t.LinksJSON = "[]"
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	t.CreatedAt = now
	t.UpdatedAt = now

	_, err := appdb.DB().Exec(`
		INSERT INTO custom_network_topologies (id, name, description, background_image, nodes_json, links_json, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Description, t.BackgroundImage, t.NodesJSON, t.LinksJSON, t.IsDefault, now, now)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *CustomTopologyRepo) Update(id string, t CustomNetworkTopology) error {
	r.EnsureTable()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := appdb.DB().Exec(`
		UPDATE custom_network_topologies SET
			name = COALESCE(NULLIF(?, ''), name),
			description = COALESCE(?, description),
			background_image = COALESCE(?, background_image),
			nodes_json = COALESCE(NULLIF(?, ''), nodes_json),
			links_json = COALESCE(NULLIF(?, ''), links_json),
			is_default = COALESCE(?, is_default),
			updated_at = ?
		WHERE id = ?`,
		t.Name, t.Description, t.BackgroundImage, t.NodesJSON, t.LinksJSON, t.IsDefault, now, id)
	return err
}

func (r *CustomTopologyRepo) Delete(id string) error {
	r.EnsureTable()
	_, err := appdb.DB().Exec(`DELETE FROM custom_network_topologies WHERE id = ?`, id)
	return err
}

