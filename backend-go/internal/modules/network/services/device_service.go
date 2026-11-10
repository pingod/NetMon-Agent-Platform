// 网络设备服务 — CRUD、连接测试、巡检历史
// 等价于 TS networkDeviceService.ts
package services

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/crypto"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 类型定义
// ───────────────────────────────────────────────

// CreateDeviceRequest 创建设备请求
type CreateDeviceRequest struct {
	Name             string `json:"name"`
	IPAddress        string `json:"ip_address"`
	Vendor           string `json:"vendor"`
	Model            string `json:"model"`
	OsVersion        string `json:"os_version"`
	SSHPort          int    `json:"ssh_port"`
	SSHKeyID         string `json:"ssh_key_id"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	EnablePassword   string `json:"enable_password"`
	Location         string `json:"location"`
	Role             string `json:"role"`
	SnmpEnabled      int    `json:"snmp_enabled"`
	SnmpCredentialID string `json:"snmp_credential_id"`
	SnmpPort         int    `json:"snmp_port"`
	SnmpCommunity    string `json:"snmp_community"`
	SnmpVersion      string `json:"snmp_version"`
}

// UpdateDeviceRequest 更新设备请求
type UpdateDeviceRequest struct {
	Name             *string `json:"name,omitempty"`
	IPAddress        *string `json:"ip_address,omitempty"`
	Vendor           *string `json:"vendor,omitempty"`
	Model            *string `json:"model,omitempty"`
	OsVersion        *string `json:"os_version,omitempty"`
	SSHPort          *int    `json:"ssh_port,omitempty"`
	SSHKeyID         *string `json:"ssh_key_id,omitempty"`
	Username         *string `json:"username,omitempty"`
	Password         *string `json:"password,omitempty"`
	EnablePassword   *string `json:"enable_password,omitempty"`
	Location         *string `json:"location,omitempty"`
	Role             *string `json:"role,omitempty"`
	SnmpEnabled      *int    `json:"snmp_enabled,omitempty"`
	SnmpCredentialID *string `json:"snmp_credential_id,omitempty"`
	SnmpPort         *int    `json:"snmp_port,omitempty"`
}

// TestConnectionResult 连接测试结果
type TestConnectionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Latency int64  `json:"latency,omitempty"`
}

// BatchImportResult 批量导入结果
type BatchImportResult struct {
	Success []BatchImportItem `json:"success"`
	Failed  []BatchImportError `json:"failed"`
	Skipped []BatchImportSkip  `json:"skipped"`
}

type BatchImportItem struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
}

type BatchImportError struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Error     string `json:"error"`
}

type BatchImportSkip struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Reason    string `json:"reason"`
}

// ───────────────────────────────────────────────
// 服务单例
// ───────────────────────────────────────────────

var NetworkDeviceServiceInstance = &NetworkDeviceService{}

type NetworkDeviceService struct{}

// GetAllDevices 获取所有设备（清除敏感字段）
func (s *NetworkDeviceService) GetAllDevices() ([]*repos.NetworkDevice, error) {
	return repos.NetworkDevices.List()
}

// GetDeviceByID 获取单个设备
func (s *NetworkDeviceService) GetDeviceByID(id string) (*repos.NetworkDevice, error) {
	return repos.NetworkDevices.GetByID(id)
}

// CreateDevice 创建设备
func (s *NetworkDeviceService) CreateDevice(req CreateDeviceRequest) (*repos.NetworkDevice, error) {
	id := uuid.New().String()

	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	snmpEnabled := req.SnmpEnabled
	if snmpEnabled == 0 {
		snmpEnabled = 1 // 默认启用
	}
	snmpPort := req.SnmpPort
	if snmpPort == 0 {
		snmpPort = 161
	}

	encryptedPassword := crypto.Encrypt(req.Password)
	encryptedEnablePassword := ""
	if req.EnablePassword != "" {
		encryptedEnablePassword = crypto.Encrypt(req.EnablePassword)
	}

	input := repos.NetworkDeviceCreateInput{
		ID:               id,
		Name:             req.Name,
		IPAddress:        req.IPAddress,
		Vendor:           req.Vendor,
		Model:            req.Model,
		OsVersion:        req.OsVersion,
		SSHPort:          sshPort,
		SSHKeyID:         req.SSHKeyID,
		Username:         req.Username,
		Password:         encryptedPassword,
		EnablePassword:   encryptedEnablePassword,
		Location:         req.Location,
		Role:             req.Role,
		Status:           "offline",
		SnmpEnabled:      snmpEnabled,
		SnmpCredentialID: req.SnmpCredentialID,
		SnmpPort:         snmpPort,
	}

	if err := repos.NetworkDevices.Create(input); err != nil {
		return nil, fmt.Errorf("创建设备失败: %w", err)
	}

	// 异步触发一次即时状态探测：如果是真实可达设备则快速上线，伪造不存在设备则保持 offline
	go func() {
		DeviceStatusProberInstance.ProbeSingleDevice(id)
	}()

	// 如果提供了 SNMP 只读串，自动创建 SNMP 凭证并关联
	if req.SnmpCommunity != "" && snmpEnabled == 1 {
		snmpVersion := req.SnmpVersion
		if snmpVersion == "" {
			snmpVersion = "v2c"
		}
		credID := uuid.New().String()
		communityVal := req.SnmpCommunity
		credInput := repos.SnmpCredentialCreateInput{
			ID:          credID,
			DeviceID:    id,
			Name:        "auto-import",
			Community:   communityVal,
			SnmpVersion: snmpVersion,
			SnmpPort:    snmpPort,
		}
		if err := repos.SnmpCredentials.Create(credInput); err != nil {
			logger.Warnf("自动创建 SNMP 凭证失败 (device=%s): %v", id, err)
		} else {
			// 更新设备的 snmp_credential_id
			_ = repos.NetworkDevices.Update(id, repos.NetworkDeviceUpdateInput{
				SnmpCredentialID: &credID,
			})
		}
	}

	logger.Infof("Network device created: %s (%s)", req.Name, req.IPAddress)
	return repos.NetworkDevices.GetByID(id)
}

// BatchCreateDevices 批量创建设备
func (s *NetworkDeviceService) BatchCreateDevices(devices []CreateDeviceRequest) *BatchImportResult {
	result := &BatchImportResult{
		Success: []BatchImportItem{},
		Failed:  []BatchImportError{},
		Skipped: []BatchImportSkip{},
	}

	// 查询已有 IP
	existingDevices, _ := repos.NetworkDevices.List()
	existingIPs := make(map[string]bool)
	for _, d := range existingDevices {
		existingIPs[d.IPAddress] = true
	}

	for _, data := range devices {
		if existingIPs[data.IPAddress] {
			result.Skipped = append(result.Skipped, BatchImportSkip{
				Name:      data.Name,
				IPAddress: data.IPAddress,
				Reason:    "IP地址已存在",
			})
			continue
		}

		_, err := s.CreateDevice(data)
		if err != nil {
			result.Failed = append(result.Failed, BatchImportError{
				Name:      data.Name,
				IPAddress: data.IPAddress,
				Error:     err.Error(),
			})
		} else {
			existingIPs[data.IPAddress] = true
			result.Success = append(result.Success, BatchImportItem{
				Name:      data.Name,
				IPAddress: data.IPAddress,
			})
		}
	}

	logger.Infof("批量导入网络设备: 成功 %d, 跳过 %d, 失败 %d",
		len(result.Success), len(result.Skipped), len(result.Failed))
	return result
}

// UpdateDevice 更新设备
func (s *NetworkDeviceService) UpdateDevice(id string, req UpdateDeviceRequest) (*repos.NetworkDevice, error) {
	existing, err := repos.NetworkDevices.GetByID(id)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("设备不存在: %s", id)
	}

	input := repos.NetworkDeviceUpdateInput{}
	if req.Name != nil {
		input.Name = req.Name
	}
	if req.IPAddress != nil && *req.IPAddress != "" {
		if *req.IPAddress != existing.IPAddress {
			devices, _ := repos.NetworkDevices.List()
			for _, d := range devices {
				if d.ID != id && d.IPAddress == *req.IPAddress {
					return nil, fmt.Errorf("IP地址 %s 已被设备 %s 占用", *req.IPAddress, d.Name)
				}
			}
		}
		input.IPAddress = req.IPAddress
	}
	if req.Vendor != nil {
		input.Vendor = req.Vendor
	}
	if req.Model != nil {
		input.Model = req.Model
	}
	if req.OsVersion != nil {
		input.OsVersion = req.OsVersion
	}
	if req.SSHPort != nil {
		input.SSHPort = req.SSHPort
	}
	if req.SSHKeyID != nil {
		input.SSHKeyID = req.SSHKeyID
	}
	if req.Username != nil {
		input.Username = req.Username
	}
	if req.Password != nil && *req.Password != "" {
		enc := crypto.Encrypt(*req.Password)
		input.Password = &enc
	}
	if req.EnablePassword != nil {
		if *req.EnablePassword != "" {
			enc := crypto.Encrypt(*req.EnablePassword)
			input.EnablePassword = &enc
		} else {
			empty := ""
			input.EnablePassword = &empty
		}
	}
	if req.Location != nil {
		input.Location = req.Location
	}
	if req.Role != nil {
		input.Role = req.Role
	}
	if req.SnmpEnabled != nil {
		v := 0
		if *req.SnmpEnabled != 0 {
			v = 1
		}
		input.SnmpEnabled = &v
	}
	if req.SnmpCredentialID != nil {
		input.SnmpCredentialID = req.SnmpCredentialID
	}
	if req.SnmpPort != nil {
		input.SnmpPort = req.SnmpPort
	}

	if err := repos.NetworkDevices.Update(id, input); err != nil {
		return nil, fmt.Errorf("更新设备失败: %w", err)
	}

	if req.IPAddress != nil && *req.IPAddress != existing.IPAddress {
		go func() {
			DeviceStatusProberInstance.ProbeSingleDevice(id)
		}()
	}

	logger.Infof("Network device updated: %s", id)
	return repos.NetworkDevices.GetByID(id)
}

// DeleteDevice 删除设备
func (s *NetworkDeviceService) DeleteDevice(id string) bool {
	if err := repos.NetworkDevices.Delete(id); err != nil {
		logger.Errorf("Delete device %s failed: %v", id, err)
		return false
	}
	logger.Infof("Network device deleted: %s", id)
	return true
}

// BatchDeleteDevices 批量删除设备
func (s *NetworkDeviceService) BatchDeleteDevices(ids []string) int {
	count, err := repos.NetworkDevices.BatchDelete(ids)
	if err != nil {
		logger.Errorf("Batch delete devices failed: %v", err)
	}
	logger.Infof("Batch deleted %d network devices", count)
	return count
}

// TestConnection 测试设备 SSH 连接
func (s *NetworkDeviceService) TestConnection(deviceID string) *TestConnectionResult {
	device, err := repos.NetworkDevices.GetByID(deviceID)
	if err != nil || device == nil {
		return &TestConnectionResult{Success: false, Message: "Device not found"}
	}

	// 解密密码
	password := ""
	if device.Password != nil && *device.Password != "" {
		password, _ = crypto.Decrypt(*device.Password)
	}

	username := ""
	if device.Username != nil {
		username = *device.Username
	}

	privateKey := ""
	authType := ""

	// 如果设备关联了 ssh_key_id，从中获取用户名、密码或密钥
	if device.SSHKeyID != nil && *device.SSHKeyID != "" {
		cred, err := repos.SshKeys.GetByID(*device.SSHKeyID)
		if err == nil && cred != nil {
			if cred.Username != nil && *cred.Username != "" {
				username = *cred.Username
			}
			if cred.AuthType != nil && *cred.AuthType != "" {
				authType = *cred.AuthType
			}
			if cred.Password != nil && *cred.Password != "" {
				decrypted, err := crypto.Decrypt(*cred.Password)
				if err == nil {
					password = decrypted
				}
			}
			if cred.PrivateKey != nil && *cred.PrivateKey != "" {
				decrypted, err := crypto.Decrypt(*cred.PrivateKey)
				if err == nil {
					privateKey = decrypted
				}
			}
		}
	}

	sshPort := device.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	// 使用 config 方式测试连接，支持密钥和密码认证
	sshDevice := SSHDeviceConfig{
		Host:       device.IPAddress,
		Port:       sshPort,
		Username:   username,
		Password:   password,
		PrivateKey: privateKey,
		AuthType:   authType,
	}

	latency, err := TestSSHConnectionWithConfig(sshDevice)
	if err != nil {
		return &TestConnectionResult{
			Success: false,
			Message: err.Error(),
			Latency: latency,
		}
	}

	return &TestConnectionResult{
		Success: true,
		Message: "Connection successful",
		Latency: latency,
	}
}

// TestTemporaryConnection 临时连接测试（明文凭证，不经过数据库）
// 支持 password 和 key 两种认证方式
func (s *NetworkDeviceService) TestTemporaryConnection(host string, port int, username, password string) *TestConnectionResult {
	return s.TestTemporaryConnectionWithKey(host, port, username, password, "")
}

// TestTemporaryConnectionWithConfig 临时连接测试（支持完整 SSHDeviceConfig）
func (s *NetworkDeviceService) TestTemporaryConnectionWithConfig(sshDevice SSHDeviceConfig) *TestConnectionResult {
	start := time.Now()
	if sshDevice.Port == 0 {
		sshDevice.Port = 22
	}

	latency, err := TestSSHConnectionWithConfig(sshDevice)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &TestConnectionResult{
			Success: false,
			Message: err.Error(),
			Latency: elapsed,
		}
	}

	return &TestConnectionResult{
		Success: true,
		Message: "Connection successful",
		Latency: latency,
	}
}

// TestTemporaryConnectionWithKey 临时连接测试（支持密钥认证）
func (s *NetworkDeviceService) TestTemporaryConnectionWithKey(host string, port int, username, password, privateKey string) *TestConnectionResult {
	return s.TestTemporaryConnectionWithConfig(SSHDeviceConfig{
		Host:       host,
		Port:       port,
		Username:   username,
		Password:   password,
		PrivateKey: privateKey,
	})
}

// GetInspectionHistory 获取设备巡检历史
func (s *NetworkDeviceService) GetInspectionHistory(deviceID string, limit int) ([]repos.InspectionHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	return repos.Inspections.ListByDeviceID(deviceID, limit)
}

// GetAllInspections 获取所有巡检记录（支持类型和状态过滤）
func (s *NetworkDeviceService) GetAllInspections(limit int, inspectionType, status string) ([]repos.InspectionHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	return repos.Inspections.ListAll(limit, inspectionType, status)
}

// GetInspectionDetail 获取巡检详情
func (s *NetworkDeviceService) GetInspectionDetail(inspectionID string) (*repos.InspectionHistory, error) {
	return repos.Inspections.GetByID(inspectionID)
}

	// GetFullCredentials 获取设备完整凭证（含解密后密码和密钥），供巡检/备份服务使用
func (s *NetworkDeviceService) GetFullCredentials(deviceID string) (*DeviceInfo, error) {
	device, err := repos.NetworkDevices.GetByID(deviceID)
	if err != nil || device == nil {
		return nil, fmt.Errorf("设备不存在: %s", deviceID)
	}

	password := ""
	if device.Password != nil && *device.Password != "" {
		dec, err := crypto.Decrypt(*device.Password)
		if err == nil && dec != "" {
			password = dec
		} else {
			// 兼容未加密的明文密码
			password = *device.Password
		}
	}

	enablePassword := ""
	if device.EnablePassword != nil && *device.EnablePassword != "" {
		dec, err := crypto.Decrypt(*device.EnablePassword)
		if err == nil && dec != "" {
			enablePassword = dec
		} else {
			enablePassword = *device.EnablePassword
		}
	}

	username := ""
	if device.Username != nil {
		username = *device.Username
	}

	privateKey := ""
	authType := ""

	// 如果设备关联了 ssh_key_id，从中获取用户名、密码或密钥
	if device.SSHKeyID != nil && *device.SSHKeyID != "" {
		cred, err := repos.SshKeys.GetByID(*device.SSHKeyID)
		if err == nil && cred != nil {
			if cred.Username != nil && *cred.Username != "" {
				username = *cred.Username
			}
			if cred.AuthType != nil && *cred.AuthType != "" {
				authType = *cred.AuthType
			}
			// 密码类型凭证：解密密码
			if cred.Password != nil && *cred.Password != "" {
				decrypted, err := crypto.Decrypt(*cred.Password)
				if err == nil && decrypted != "" {
					password = decrypted
				} else {
					password = *cred.Password
				}
			}
			// 密钥类型凭证：解密私钥
			if cred.PrivateKey != nil && *cred.PrivateKey != "" {
				decrypted, err := crypto.Decrypt(*cred.PrivateKey)
				if err == nil && decrypted != "" {
					privateKey = decrypted
				} else {
					privateKey = *cred.PrivateKey
				}
			}
		}
	}

	sshPort := device.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	return &DeviceInfo{
		ID:             device.ID,
		Name:           device.Name,
		IPAddress:      device.IPAddress,
		Vendor:         device.Vendor,
		SSHPort:        sshPort,
		Username:       username,
		Password:       password,
		EnablePassword: enablePassword,
		PrivateKey:     privateKey,
		AuthType:       authType,
	}, nil
}

// DeviceInfo 设备连接信息（解密后，供巡检/备份使用）
type DeviceInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	Vendor         string `json:"vendor"`
	SSHPort        int    `json:"ssh_port"`
	Username       string `json:"username"`
	Password       string `json:"-"`
	EnablePassword string `json:"-"`
	PrivateKey     string `json:"-"`   // PEM 格式私钥（已解密）
	AuthType       string `json:"-"`   // "password" 或 "key"
}
