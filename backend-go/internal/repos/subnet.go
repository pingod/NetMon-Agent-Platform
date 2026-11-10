// 子网/IP 地址管理，等价于 TS networkSubnetRepository.ts
package repos

import (
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// 子网
// ───────────────────────────────────────────────

// NetworkSubnet 子网记录
type NetworkSubnet struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	CIDR        string  `json:"cidr"`
	Gateway     *string `json:"gateway"`
	VlanID      *int    `json:"vlan_id"`
	NetworkType *string `json:"network_type"`
	Location    *string `json:"location"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	TotalIPs    int     `json:"total_ips"`
	UsedIPs     int     `json:"used_ips"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// SubnetCreateInput 子网创建输入
type SubnetCreateInput struct {
	ID          string
	Name        string
	CIDR        string
	Gateway     string
	VlanID      int
	NetworkType string
	Location    string
	Description string
	Status      string
	TotalIPs    int
}

// SubnetUpdateInput 子网更新输入
type SubnetUpdateInput struct {
	Name        *string
	Gateway     *string
	VlanID      *int
	NetworkType *string
	Location    *string
	Description *string
	Status      *string
}

// NetworkSubnetRepo 子网仓库
type NetworkSubnetRepo struct{}

var NetworkSubnets = &NetworkSubnetRepo{}

// List 列出所有子网（含已用 IP 数）
func (r *NetworkSubnetRepo) List() ([]NetworkSubnet, error) {
	rows, err := appdb.DB().Query(`
		SELECT s.id, s.name, s.cidr, s.gateway, s.vlan_id, s.network_type, s.location, s.description, s.status, s.total_ips,
			(SELECT COUNT(*) FROM network_ips WHERE subnet_id = s.id AND status != 'available') as used_ips,
			s.created_at, s.updated_at
		FROM network_subnets s ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NetworkSubnet
	for rows.Next() {
		s, err := scanSubnet(rows)
		if err != nil {
			continue
		}
		out = append(out, *s)
	}
	return out, nil
}

// GetByID 获取单个子网
func (r *NetworkSubnetRepo) GetByID(id string) (*NetworkSubnet, error) {
	row := appdb.DB().QueryRow(`
		SELECT s.id, s.name, s.cidr, s.gateway, s.vlan_id, s.network_type, s.location, s.description, s.status, s.total_ips,
			(SELECT COUNT(*) FROM network_ips WHERE subnet_id = s.id AND status != 'available') as used_ips,
			s.created_at, s.updated_at
		FROM network_subnets s WHERE s.id = ?`, id)
	return scanSubnet(row)
}

func scanSubnet(scanner interface{ Scan(...interface{}) error }) (*NetworkSubnet, error) {
	var s NetworkSubnet
	var gateway, networkType, location, description, status sql.NullString
	var vlanID sql.NullInt64
	err := scanner.Scan(&s.ID, &s.Name, &s.CIDR, &gateway, &vlanID, &networkType, &location, &description, &status, &s.TotalIPs, &s.UsedIPs, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Gateway = ns(gateway)
	s.NetworkType = ns(networkType)
	s.Location = ns(location)
	s.Description = ns(description)
	s.Status = ns(status)
	if vlanID.Valid {
		v := int(vlanID.Int64)
		s.VlanID = &v
	}
	return &s, nil
}

// Create 创建子网
func (r *NetworkSubnetRepo) Create(input SubnetCreateInput) error {
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	if input.NetworkType == "" {
		input.NetworkType = "lan"
	}
	if input.Status == "" {
		input.Status = "active"
	}
	var vlanVal interface{}
	if input.VlanID > 0 {
		vlanVal = input.VlanID
	}
	_, err := appdb.DB().Exec(`
		INSERT INTO network_subnets (id, name, cidr, gateway, vlan_id, network_type, location, description, status, total_ips)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, input.CIDR, sqlNullStr(input.Gateway), vlanVal,
		input.NetworkType, sqlNullStr(input.Location), sqlNullStr(input.Description),
		input.Status, input.TotalIPs)
	return err
}

// Update 更新子网（COALESCE 部分更新）
func (r *NetworkSubnetRepo) Update(id string, input SubnetUpdateInput) error {
	var sets []string
	var args []interface{}
	if input.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *input.Name)
	}
	if input.Gateway != nil {
		sets = append(sets, "gateway = ?")
		args = append(args, sqlNullStr(*input.Gateway))
	}
	if input.VlanID != nil {
		sets = append(sets, "vlan_id = ?")
		if *input.VlanID > 0 {
			args = append(args, *input.VlanID)
		} else {
			args = append(args, nil)
		}
	}
	if input.NetworkType != nil {
		sets = append(sets, "network_type = ?")
		args = append(args, *input.NetworkType)
	}
	if input.Location != nil {
		sets = append(sets, "location = ?")
		args = append(args, sqlNullStr(*input.Location))
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, sqlNullStr(*input.Description))
	}
	if input.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
	}
	if len(sets) == 0 {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	sets = append(sets, "updated_at = ?")
	args = append(args, now, id)
	_, err := appdb.DB().Exec("UPDATE network_subnets SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// Delete 删除子网
func (r *NetworkSubnetRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM network_subnets WHERE id = ?`, id)
	return err
}

// ───────────────────────────────────────────────
// IP 地址
// ───────────────────────────────────────────────

// NetworkIP IP 地址记录
type NetworkIP struct {
	ID          string  `json:"id"`
	SubnetID    string  `json:"subnet_id"`
	IPAddress   string  `json:"ip_address"`
	Status      string  `json:"status"`
	DeviceID    *string `json:"device_id"`
	DeviceName  *string `json:"device_name"`
	MacAddress  *string `json:"mac_address"`
	Description *string `json:"description"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// IPListFilters IP 列表过滤选项
type IPListFilters struct {
	SubnetID string
	Status   string
	Search   string
	Limit    int
	Offset   int
}

// IPUpdateInput IP 更新输入
type IPUpdateInput struct {
	Status      *string
	DeviceName  *string
	MacAddress  *string
	Description *string
}

// IPBatchUpdateInput IP 批量更新输入
type IPBatchUpdateInput struct {
	Status      string
	DeviceName  string
	Description string
}

// NetworkIPRepo IP 地址仓库
type NetworkIPRepo struct{}

var NetworkIPs = &NetworkIPRepo{}

// List 列出子网下的 IP（带过滤+分页）
func (r *NetworkIPRepo) List(filters IPListFilters) ([]NetworkIP, error) {
	if filters.Limit <= 0 {
		filters.Limit = 100
	}
	query := `SELECT id, subnet_id, ip_address, status, device_id, device_name, mac_address, description, created_at, updated_at FROM network_ips WHERE subnet_id = ?`
	var args []interface{}
	args = append(args, filters.SubnetID)
	if filters.Status != "" && filters.Status != "all" {
		query += " AND status = ?"
		args = append(args, filters.Status)
	}
	if filters.Search != "" {
		query += " AND (ip_address LIKE ? OR device_name LIKE ? OR mac_address LIKE ?)"
		s := "%" + filters.Search + "%"
		args = append(args, s, s, s)
	}
	query += " ORDER BY ip_address ASC LIMIT ? OFFSET ?"
	args = append(args, filters.Limit, filters.Offset)

	rows, err := appdb.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIPs(rows)
}

// Count 统计 IP 数量（带过滤）
func (r *NetworkIPRepo) Count(filters IPListFilters) (int, error) {
	query := `SELECT COUNT(*) FROM network_ips WHERE subnet_id = ?`
	var args []interface{}
	args = append(args, filters.SubnetID)
	if filters.Status != "" && filters.Status != "all" {
		query += " AND status = ?"
		args = append(args, filters.Status)
	}
	if filters.Search != "" {
		query += " AND (ip_address LIKE ? OR device_name LIKE ? OR mac_address LIKE ?)"
		s := "%" + filters.Search + "%"
		args = append(args, s, s, s)
	}
	var c int
	err := appdb.DB().QueryRow(query, args...).Scan(&c)
	return c, err
}

// IPStatusStat 按状态分组统计
type IPStatusStat struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// StatsByStatus 按状态分组统计 IP 数量
func (r *NetworkIPRepo) StatsByStatus(subnetID string) ([]IPStatusStat, error) {
	rows, err := appdb.DB().Query(`SELECT status, COUNT(*) as count FROM network_ips WHERE subnet_id = ? GROUP BY status`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPStatusStat
	for rows.Next() {
		var s IPStatusStat
		if err := rows.Scan(&s.Status, &s.Count); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// BulkInsertAvailable 批量插入可用 IP
func (r *NetworkIPRepo) BulkInsertAvailable(subnetID string, ips []string) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	insertSQL := `INSERT OR IGNORE INTO network_ips (id, subnet_id, ip_address, status) VALUES (?, ?, ?, 'available')`
	if appdb.IsMySQLMode() {
		insertSQL = `INSERT IGNORE INTO network_ips (id, subnet_id, ip_address, status) VALUES (?, ?, ?, 'available')`
	}
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, ip := range ips {
		_, err := stmt.Exec(uuid.NewString(), subnetID, ip)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Update 更新单个 IP（需匹配 id 和 subnet_id）
func (r *NetworkIPRepo) Update(ipID, subnetID string, input IPUpdateInput) error {
	var sets []string
	var args []interface{}
	if input.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
	}
	if input.DeviceName != nil {
		sets = append(sets, "device_name = ?")
		args = append(args, sqlNullStr(*input.DeviceName))
	}
	if input.MacAddress != nil {
		sets = append(sets, "mac_address = ?")
		args = append(args, sqlNullStr(*input.MacAddress))
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, sqlNullStr(*input.Description))
	}
	if len(sets) == 0 {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	sets = append(sets, "updated_at = ?")
	args = append(args, now, ipID, subnetID)
	_, err := appdb.DB().Exec("UPDATE network_ips SET "+strings.Join(sets, ", ")+" WHERE id = ? AND subnet_id = ?", args...)
	return err
}

// BatchUpdate 批量更新 IP
func (r *NetworkIPRepo) BatchUpdate(ipIDs []string, subnetID string, input IPBatchUpdateInput) error {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, ipID := range ipIDs {
		_, err := tx.Exec(`
			UPDATE network_ips SET status = ?, device_name = ?, description = COALESCE(?, description), updated_at = ?
			WHERE id = ? AND subnet_id = ?`,
			input.Status, sqlNullStr(input.DeviceName), sqlNullStr(input.Description), now, ipID, subnetID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IPImportItem Excel 导入的单条 IP 记录
type IPImportItem struct {
	IPAddress   string
	Status      string
	DeviceName  string
	MacAddress  string
	Description string
}

// ImportByIP 按 IP 地址更新（不存在则插入），返回 (新增数, 更新数)
func (r *NetworkIPRepo) ImportByIP(subnetID string, items []IPImportItem) (int, int, error) {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	inserted, updated := 0, 0
	now := time.Now().Format("2006-01-02 15:04:05")

	for _, item := range items {
		if strings.TrimSpace(item.IPAddress) == "" {
			continue
		}
		ipAddr := strings.TrimSpace(item.IPAddress)
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "available"
		}
		// 状态别名归一化
		switch status {
		case "可用", "空闲", "available", "free":
			status = "available"
		case "已用", "使用中", "占用", "used", "in_use":
			status = "used"
		case "预留", "保留", "reserved", "reserve":
			status = "reserved"
		}

		var existingID string
		err := tx.QueryRow(`SELECT id FROM network_ips WHERE subnet_id = ? AND ip_address = ?`, subnetID, ipAddr).Scan(&existingID)
		if err == sql.ErrNoRows {
			// 插入新记录
			_, err = tx.Exec(`
				INSERT INTO network_ips (id, subnet_id, ip_address, status, device_name, mac_address, description, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), subnetID, ipAddr, status,
				sqlNullStr(strings.TrimSpace(item.DeviceName)),
				sqlNullStr(strings.TrimSpace(item.MacAddress)),
				sqlNullStr(strings.TrimSpace(item.Description)), now, now)
			if err != nil {
				return 0, 0, err
			}
			inserted++
		} else if err != nil {
			return 0, 0, err
		} else {
			// 更新已有记录
			_, err = tx.Exec(`
				UPDATE network_ips SET
					status = CASE WHEN ? != '' THEN ? ELSE status END,
					device_name = CASE WHEN ? != '' THEN ? ELSE device_name END,
					mac_address = CASE WHEN ? != '' THEN ? ELSE mac_address END,
					description = CASE WHEN ? != '' THEN ? ELSE description END,
					updated_at = ?
				WHERE id = ?`,
				status, status,
				strings.TrimSpace(item.DeviceName), strings.TrimSpace(item.DeviceName),
				strings.TrimSpace(item.MacAddress), strings.TrimSpace(item.MacAddress),
				strings.TrimSpace(item.Description), strings.TrimSpace(item.Description),
				now,
				existingID)
			if err != nil {
				return 0, 0, err
			}
			updated++
		}
	}
	return inserted, updated, tx.Commit()
}

// ImportIPsInput 批量导入 IP 输入
type ImportIPsInput struct {
	DeviceName  string
	MacAddress  string
	Description string
	Status      string
}

// ImportIPs 批量导入/更新 IP（由 IPAM 批量导入调用）
func (r *NetworkIPRepo) ImportIPs(subnetID string, items map[string]ImportIPsInput) (int, int, error) {
	tx, err := appdb.DB().Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	inserted := 0
	updated := 0
	now := time.Now().Format("2006-01-02 15:04:05")

	for ipAddr, item := range items {
		status := item.Status
		if status == "" {
			status = "allocated"
		}

		var existingID string
		err := tx.QueryRow(`SELECT id FROM network_ips WHERE subnet_id = ? AND ip_address = ?`, subnetID, ipAddr).Scan(&existingID)
		if err == sql.ErrNoRows {
			// 插入新记录
			_, err = tx.Exec(`
				INSERT INTO network_ips (id, subnet_id, ip_address, status, device_name, mac_address, description, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), subnetID, ipAddr, status,
				sqlNullStr(strings.TrimSpace(item.DeviceName)),
				sqlNullStr(strings.TrimSpace(item.MacAddress)),
				sqlNullStr(strings.TrimSpace(item.Description)), now, now)
			if err != nil {
				return 0, 0, err
			}
			inserted++
		} else if err != nil {
			return 0, 0, err
		} else {
			// 更新已有记录
			_, err = tx.Exec(`
				UPDATE network_ips SET
					status = CASE WHEN ? != '' THEN ? ELSE status END,
					device_name = CASE WHEN ? != '' THEN ? ELSE device_name END,
					mac_address = CASE WHEN ? != '' THEN ? ELSE mac_address END,
					description = CASE WHEN ? != '' THEN ? ELSE description END,
					updated_at = ?
				WHERE id = ?`,
				status, status,
				strings.TrimSpace(item.DeviceName), strings.TrimSpace(item.DeviceName),
				strings.TrimSpace(item.MacAddress), strings.TrimSpace(item.MacAddress),
				strings.TrimSpace(item.Description), strings.TrimSpace(item.Description),
				now,
				existingID)
			if err != nil {
				return 0, 0, err
			}
			updated++
		}
	}
	return inserted, updated, tx.Commit()
}

func scanIPs(rows *sql.Rows) ([]NetworkIP, error) {
	var out []NetworkIP
	for rows.Next() {
		var ip NetworkIP
		var deviceID, deviceName, macAddress, description sql.NullString
		if err := rows.Scan(&ip.ID, &ip.SubnetID, &ip.IPAddress, &ip.Status, &deviceID, &deviceName, &macAddress, &description, &ip.CreatedAt, &ip.UpdatedAt); err != nil {
			continue
		}
		ip.DeviceID = ns(deviceID)
		ip.DeviceName = ns(deviceName)
		ip.MacAddress = ns(macAddress)
		ip.Description = ns(description)
		out = append(out, ip)
	}
	return out, nil
}
