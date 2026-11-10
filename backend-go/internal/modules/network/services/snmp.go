// SNMP 服务层 — 设备凭证管理、SNMP GET/WALK、系统信息、接口采集、健康检查、自定义 OID 采集
// 等价于 TS snmpService.ts + snmpCollector.ts + snmpParser.ts + snmpDiscovery.ts
package services

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"itops-backend/internal/crypto"
	"itops-backend/internal/logger"
	dbservices "itops-backend/internal/modules/database/services"
	"itops-backend/internal/repos"
)

// ───────────────────────────────────────────────
// 常量
// ───────────────────────────────────────────────

const (
	snmpSessionTimeout  = 30 * time.Second // session watchdog 超时
	snmpGetBatchSize    = 50               // 标量 OID 批量 GET 每批最大数量
	snmpWalkConcurrency = 10               // 表列 OID 并行 WALK 最大并发数
	snmpHealthTimeout   = 15 * time.Second // 健康检查整体超时
	snmpTempTimeout     = 3 * time.Second  // 温度采集超时
	snmpDefaultTimeout  = 10 * time.Second // 单次 SNMP 操作超时
	snmpDefaultRetries  = 2                // SNMP 重试次数
)

// ───────────────────────────────────────────────
// 辅助函数
// ───────────────────────────────────────────────

// getIfIndex 从 OID 末尾提取接口索引数字
func getIfIndex(oid string) int {
	re := regexp.MustCompile(`\.(\d+)$`)
	m := re.FindStringSubmatch(oid)
	if m == nil {
		return -1
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return idx
}

// ───────────────────────────────────────────────
// SNMP 服务
// ───────────────────────────────────────────────

// SnmpService SNMP 服务单例
var SnmpServiceInstance = &SnmpService{}

// SnmpService SNMP 服务
type SnmpService struct{}

// GetCredential 获取设备 SNMP 凭证（自动解密敏感字段）
// 优先使用设备 snmp_credential_id 指定的凭证，其次查 device_id 关联，最后回退到全局默认
func (s *SnmpService) GetCredential(deviceID string) *SnmpCredential {
	// 1. 查设备记录获取 snmp_credential_id
	device, err := repos.NetworkDevices.GetByID(deviceID)
	if err == nil && device != nil && device.SnmpCredentialID != nil && *device.SnmpCredentialID != "" {
		row, err := repos.SnmpCredentials.GetByID(*device.SnmpCredentialID)
		if err == nil && row != nil {
			return repoCredentialToService(row)
		}
	}
	// 2. 尝试通过 device_id 关联查询
	row, err := repos.SnmpCredentials.GetByDeviceID(deviceID)
	if err == nil && row != nil {
		return repoCredentialToService(row)
	}
	// 3. 回退到全局默认凭证
	return s.GetDefaultCredential()
}

// GetDefaultCredential 获取全局默认凭证
func (s *SnmpService) GetDefaultCredential() *SnmpCredential {
	row, err := repos.SnmpCredentials.GetDefault()
	if err != nil || row == nil {
		return nil
	}
	return repoCredentialToService(row)
}

// repoCredentialToService 将 repo 层凭证（加密）转为 service 层凭证（解密）
func repoCredentialToService(row *repos.SnmpCredential) *SnmpCredential {
	c := &SnmpCredential{
		ID:          row.ID,
		Name:        row.Name,
		SnmpVersion: SnmpVersion(row.SnmpVersion),
		SnmpPort:    row.SnmpPort,
	}
	if row.DeviceID != nil {
		c.DeviceID = *row.DeviceID
	}
	if row.Community != nil && *row.Community != "" {
		commStr := *row.Community
		if strings.HasPrefix(commStr, "2:") {
			if dec, err := crypto.Decrypt(commStr); err == nil {
				c.Community = dec
			} else {
				c.Community = "public"
			}
		} else {
			c.Community = commStr
		}
	}
	if row.SnmpUser != nil {
		c.SnmpUser = *row.SnmpUser
	}
	if row.SnmpAuthProtocol != nil {
		c.SnmpAuthProtocol = *row.SnmpAuthProtocol
	}
	if row.SnmpAuthKey != nil && *row.SnmpAuthKey != "" {
		if dec, err := crypto.Decrypt(*row.SnmpAuthKey); err == nil {
			c.SnmpAuthKey = dec
		} else {
			c.SnmpAuthKey = *row.SnmpAuthKey
		}
	}
	if row.SnmpPrivProtocol != nil {
		c.SnmpPrivProtocol = *row.SnmpPrivProtocol
	}
	if row.SnmpPrivKey != nil && *row.SnmpPrivKey != "" {
		if dec, err := crypto.Decrypt(*row.SnmpPrivKey); err == nil {
			c.SnmpPrivKey = dec
		} else {
			c.SnmpPrivKey = *row.SnmpPrivKey
		}
	}
	if c.SnmpPort == 0 {
		c.SnmpPort = 161
	}
	return c
}

// CreateSession 创建 gosnmp 会话参数（不连接）
func (s *SnmpService) CreateSession(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string) *gosnmp.GoSNMP {

	if port == 0 {
		port = 161
	}
	params := &gosnmp.GoSNMP{
		Target:  host,
		Port:    uint16(port),
		Timeout: snmpDefaultTimeout,
		Retries: snmpDefaultRetries,
		MaxOids: snmpGetBatchSize,
	}

	switch version {
	case SNMPv1:
		params.Version = gosnmp.Version1
		params.Community = community
	case SNMPv3:
		params.Version = gosnmp.Version3
		params.SecurityModel = gosnmp.UserSecurityModel
		secParams := &gosnmp.UsmSecurityParameters{
			UserName: user,
		}
		// 认证协议支持: MD5, SHA(SHA1), SHA224, SHA256, SHA384, SHA512
		cleanAuth := strings.ToUpper(strings.TrimSpace(authProtocol))
		switch cleanAuth {
		case "MD5":
			secParams.AuthenticationProtocol = gosnmp.MD5
		case "SHA", "SHA1":
			secParams.AuthenticationProtocol = gosnmp.SHA
		case "SHA224":
			secParams.AuthenticationProtocol = gosnmp.SHA224
		case "SHA256":
			secParams.AuthenticationProtocol = gosnmp.SHA256
		case "SHA384":
			secParams.AuthenticationProtocol = gosnmp.SHA384
		case "SHA512":
			secParams.AuthenticationProtocol = gosnmp.SHA512
		default:
			secParams.AuthenticationProtocol = gosnmp.NoAuth
		}
		if authKey != "" {
			secParams.AuthenticationPassphrase = authKey
		}
		// 加密协议支持: DES, AES(AES128), AES192, AES256, AES192C, AES256C
		cleanPriv := strings.ToUpper(strings.TrimSpace(privProtocol))
		switch cleanPriv {
		case "DES":
			secParams.PrivacyProtocol = gosnmp.DES
		case "AES", "AES128":
			secParams.PrivacyProtocol = gosnmp.AES
		case "AES192":
			secParams.PrivacyProtocol = gosnmp.AES192
		case "AES256":
			secParams.PrivacyProtocol = gosnmp.AES256
		case "AES192C":
			secParams.PrivacyProtocol = gosnmp.AES192C
		case "AES256C":
			secParams.PrivacyProtocol = gosnmp.AES256C
		default:
			secParams.PrivacyProtocol = gosnmp.NoPriv
		}
		if privKey != "" {
			secParams.PrivacyPassphrase = privKey
		}
		// 消息安全级别标志: AuthPriv, AuthNoPriv, NoAuthNoPriv
		hasAuth := secParams.AuthenticationProtocol != gosnmp.NoAuth && secParams.AuthenticationPassphrase != ""
		hasPriv := secParams.PrivacyProtocol != gosnmp.NoPriv && secParams.PrivacyPassphrase != ""
		if hasAuth && hasPriv {
			params.MsgFlags = gosnmp.AuthPriv
		} else if hasAuth {
			params.MsgFlags = gosnmp.AuthNoPriv
		} else {
			params.MsgFlags = gosnmp.NoAuthNoPriv
		}
		params.SecurityParameters = secParams
	default: // v2c
		params.Version = gosnmp.Version2c
		params.Community = community
	}

	return params
}

// createAndConnect 创建会话并连接
func (s *SnmpService) createAndConnect(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string) (*gosnmp.GoSNMP, error) {
	params := s.CreateSession(host, port, version, community, user, authProtocol, authKey, privProtocol, privKey)
	err := params.Connect()
	if err != nil {
		return nil, err
	}
	return params, nil
}

// ───────────────────────────────────────────────
// SNMP GET
// ───────────────────────────────────────────────

// SnmpGet 单 OID GET
func (s *SnmpService) SnmpGet(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string, oid string) *SnmpResult {

	if oid == "" {
		oid = SystemOIDs.SysName
	}

	resultCh := make(chan *SnmpResult, 1)
	go func() {
		session, err := s.createAndConnect(host, port, version, community,
			user, authProtocol, authKey, privProtocol, privKey)
		if err != nil {
			resultCh <- nil
			return
		}
		defer session.Conn.Close()

		resp, err := session.Get([]string{oid})
		if err != nil || len(resp.Variables) == 0 {
			resultCh <- nil
			return
		}

		pdu := resp.Variables[0]
		if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
			resultCh <- nil
			return
		}

		resultCh <- &SnmpResult{
			OID:      pdu.Name,
			Value:    NormalizeSnmpValue(int(pdu.Type), pdu.Value),
			Type:     int(pdu.Type),
			TypeName: SnmpTypeName(int(pdu.Type)),
		}
	}()

	select {
	case r := <-resultCh:
		return r
	case <-time.After(snmpSessionTimeout):
		return nil
	}
}

// SnmpGetMultiple 批量 GET
func (s *SnmpService) SnmpGetMultiple(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string, oids []string) []*SnmpResult {

	resultCh := make(chan []*SnmpResult, 1)
	go func() {
		session, err := s.createAndConnect(host, port, version, community,
			user, authProtocol, authKey, privProtocol, privKey)
		if err != nil {
			resultCh <- nil
			return
		}
		defer session.Conn.Close()

		var output []*SnmpResult
		// 分批 GET，每批 snmpGetBatchSize 个
		for i := 0; i < len(oids); i += snmpGetBatchSize {
			end := i + snmpGetBatchSize
			if end > len(oids) {
				end = len(oids)
			}
			batch := oids[i:end]

			resp, err := session.Get(batch)
			if err != nil {
				continue
			}
			// 按 OID 建映射，再按请求顺序回填，避免响应乱序导致指标错位
			valueMap := make(map[string]*SnmpResult, len(resp.Variables))
			for _, pdu := range resp.Variables {
				if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
					continue
				}
				res := &SnmpResult{
					OID:      pdu.Name,
					Value:    NormalizeSnmpValue(int(pdu.Type), pdu.Value),
					Type:     int(pdu.Type),
					TypeName: SnmpTypeName(int(pdu.Type)),
				}
				name := strings.TrimPrefix(pdu.Name, ".")
				valueMap[name] = res
				valueMap["."+name] = res
			}
			for _, oid := range batch {
				cleanOid := strings.TrimPrefix(oid, ".")
				res := valueMap[cleanOid]
				if res == nil {
					res = valueMap["."+cleanOid]
				}
				if res == nil {
					res = valueMap[cleanOid+".0"]
				}
				if res == nil {
					continue
				}
				output = append(output, &SnmpResult{
					OID:      oid,
					Value:    res.Value,
					Type:     res.Type,
					TypeName: res.TypeName,
				})
			}
		}
		resultCh <- output
	}()

	select {
	case r := <-resultCh:
		if r == nil {
			return []*SnmpResult{}
		}
		return r
	case <-time.After(snmpSessionTimeout):
		return []*SnmpResult{}
	}
}

// ───────────────────────────────────────────────
// SNMP WALK
// ───────────────────────────────────────────────

// SnmpWalk subtree walk，返回所有匹配的 PDU（受 snmpSessionTimeout 约束）
func (s *SnmpService) SnmpWalk(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string, oid string, maxRepetitions int) []*SnmpResult {

	ctx, cancel := context.WithTimeout(context.Background(), snmpSessionTimeout)
	defer cancel()
	return s.SnmpWalkCtx(ctx, host, port, version, community,
		user, authProtocol, authKey, privProtocol, privKey, oid, maxRepetitions)
}

// SnmpWalkCtx 带 context 的 WALK：ctx 取消时关闭连接以中断阻塞的 walk，避免协程泄漏
func (s *SnmpService) SnmpWalkCtx(ctx context.Context, host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string, oid string, maxRepetitions int) []*SnmpResult {

	if maxRepetitions <= 0 {
		maxRepetitions = 25
	}
	if ctx.Err() != nil {
		return []*SnmpResult{}
	}

	resultCh := make(chan []*SnmpResult, 1)
	go func() {
		session, err := s.createAndConnect(host, port, version, community,
			user, authProtocol, authKey, privProtocol, privKey)
		if err != nil {
			resultCh <- nil
			return
		}
		defer session.Conn.Close()

		walkDone := make(chan struct{})
		defer close(walkDone)
		go func() {
			select {
			case <-ctx.Done():
				session.Conn.Close()
			case <-walkDone:
			}
		}()

		var output []*SnmpResult
		walkFn := func(pdu gosnmp.SnmpPDU) error {
			if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
				return nil
			}
			output = append(output, &SnmpResult{
				OID:      pdu.Name,
				Value:    NormalizeSnmpValue(int(pdu.Type), pdu.Value),
				Type:     int(pdu.Type),
				TypeName: SnmpTypeName(int(pdu.Type)),
			})
			return nil
		}

		// v2c/v3 用 BulkWalk，v1 用 Walk
		if version == SNMPv1 {
			err = session.Walk(oid, walkFn)
		} else {
			err = session.BulkWalk(oid, walkFn)
		}
		if err != nil {
			resultCh <- nil
			return
		}
		resultCh <- output
	}()

	select {
	case r := <-resultCh:
		if r == nil {
			return []*SnmpResult{}
		}
		return r
	case <-ctx.Done():
		return []*SnmpResult{}
	}
}

// ───────────────────────────────────────────────
// 系统信息
// ───────────────────────────────────────────────

// GetSystemInfo 获取设备系统基本信息
func (s *SnmpService) GetSystemInfo(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string) *SystemInfo {

	oids := []string{
		SystemOIDs.SysName, SystemOIDs.SysDescr, SystemOIDs.SysUptime,
		SystemOIDs.SysLocation, SystemOIDs.SysContact,
	}
	results := s.SnmpGetMultiple(host, port, version, community,
		user, authProtocol, authKey, privProtocol, privKey, oids)

	if len(results) == 0 {
		return nil
	}

	find := func(targetOID string) string {
		for _, r := range results {
			if r.OID == targetOID {
				return r.Value
			}
		}
		return ""
	}

	name := find(SystemOIDs.SysName)
	descr := find(SystemOIDs.SysDescr)
	uptimeStr := find(SystemOIDs.SysUptime)
	uptime, _ := strconv.ParseInt(uptimeStr, 10, 64)
	loc := find(SystemOIDs.SysLocation)
	contact := find(SystemOIDs.SysContact)

	// 若所有系统字段均为空且没有有效运行时间，说明未能从设备获取有效 SNMP 响应
	if name == "" && descr == "" && uptime == 0 && loc == "" && contact == "" {
		return nil
	}

	return &SystemInfo{
		SysName:     name,
		SysDescr:    descr,
		SysUptime:   uptime,
		SysLocation: loc,
		SysContact:  contact,
	}
}

// GetSystemInfoSimple 简化调用（仅 host/port/version/community）
func (s *SnmpService) GetSystemInfoSimple(host string, port int, version SnmpVersion, community string) *SystemInfo {
	return s.GetSystemInfo(host, port, version, community, "", "", "", "", "")
}

// walkWithContext 使用已有会话执行 WALK（由调用者管理连接生命周期）
func (s *SnmpService) walkWithContext(session *gosnmp.GoSNMP, oid string, walkFn gosnmp.WalkFunc) error {
	// 判断 SNMP 版本：v1 用 Walk，v2c/v3 用 BulkWalk
	if session.Version == gosnmp.Version1 {
		return session.Walk(oid, walkFn)
	}
	return session.BulkWalk(oid, walkFn)
}

// ───────────────────────────────────────────────
// 接口采集
// ───────────────────────────────────────────────

// GetInterfaces 获取设备全部接口信息（复用单个 SNMP 会话顺序 WALK，避免为每个 OID 重复建连）
func (s *SnmpService) GetInterfaces(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string) []InterfaceInfo {

	ctx, cancel := context.WithTimeout(context.Background(), snmpSessionTimeout)
	defer cancel()
	return s.GetInterfacesCtx(ctx, host, port, version, community,
		user, authProtocol, authKey, privProtocol, privKey)
}

// GetInterfacesCtx 带 context 的接口采集：共享一个会话顺序 WALK，ctx 取消时立即中断
func (s *SnmpService) GetInterfacesCtx(ctx context.Context, host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string) []InterfaceInfo {

	walkOIDs := []string{
		IfMibOIDs.IfName, IfMibOIDs.IfDescr, IfMibOIDs.IfType, IfMibOIDs.IfSpeed,
		IfMibOIDs.IfMtu, IfMibOIDs.IfPhysAddress, IfMibOIDs.IfAdminStatus, IfMibOIDs.IfOperStatus,
		IfMibOIDs.IfAlias, IfMibOIDs.IfHCInOctets, IfMibOIDs.IfHCOutOctets,
		IfMibOIDs.IfInErrors, IfMibOIDs.IfOutErrors, IfMibOIDs.IfHighSpeed,
		IfMibOIDs.IfInOctets, IfMibOIDs.IfOutOctets,
		IfMibOIDs.IfInUcastPkts, IfMibOIDs.IfOutUcastPkts, IfMibOIDs.IfLastChange,
	}

	results := make([][]*SnmpResult, len(walkOIDs))

	session, err := s.createAndConnect(host, port, version, community,
		user, authProtocol, authKey, privProtocol, privKey)
	if err != nil {
		return nil
	}
	defer session.Conn.Close()

	walkDone := make(chan struct{})
	defer close(walkDone)
	go func() {
		select {
		case <-ctx.Done():
			session.Conn.Close()
		case <-walkDone:
		}
	}()

	for i, targetOID := range walkOIDs {
		if ctx.Err() != nil {
			break
		}
		var output []*SnmpResult
		walkFn := func(pdu gosnmp.SnmpPDU) error {
			if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
				return nil
			}
			output = append(output, &SnmpResult{
				OID:      pdu.Name,
				Value:    NormalizeSnmpValue(int(pdu.Type), pdu.Value),
				Type:     int(pdu.Type),
				TypeName: SnmpTypeName(int(pdu.Type)),
			})
			return nil
		}
		if err := s.walkWithContext(session, targetOID, walkFn); err != nil {
			// 单个维度失败（设备未实现该列）不影响其余维度
			continue
		}
		results[i] = output
	}

	// 构建 index → 值 映射
	makeMap := func(arr []*SnmpResult) map[int]string {
		m := make(map[int]string)
		for _, r := range arr {
			idx := getIfIndex(r.OID)
			if idx >= 0 {
				m[idx] = r.Value
			}
		}
		return m
	}

	nameMap := makeMap(results[0])
	descrMap := makeMap(results[1])
	typeMap := makeMap(results[2])
	speedMap := makeMap(results[3])
	mtuMap := makeMap(results[4])
	macMap := makeMap(results[5])
	adminMap := makeMap(results[6])
	operMap := makeMap(results[7])
	aliasMap := makeMap(results[8])
	hcInOctetsMap := makeMap(results[9])
	hcOutOctetsMap := makeMap(results[10])
	inErrMap := makeMap(results[11])
	outErrMap := makeMap(results[12])
	highSpeedMap := makeMap(results[13])
	inOctets32Map := makeMap(results[14])
	outOctets32Map := makeMap(results[15])
	inUcastPktsMap := makeMap(results[16])
	outUcastPktsMap := makeMap(results[17])
	lastChangeMap := makeMap(results[18])

	// 合并所有 index（以 ifName 和 ifDescr 为主）
	allIndexes := make(map[int]bool)
	for idx := range nameMap {
		allIndexes[idx] = true
	}
	for idx := range descrMap {
		allIndexes[idx] = true
	}

	var interfaces []InterfaceInfo
	for idx := range allIndexes {
		speed, _ := strconv.ParseInt(speedMap[idx], 10, 64)
		highSpeed, _ := strconv.ParseInt(highSpeedMap[idx], 10, 64)
		effectiveSpeed := speed
		if effectiveSpeed <= 0 && highSpeed > 0 {
			effectiveSpeed = highSpeed * 1_000_000 // highSpeed → bps
		}

		// 优先使用 HC octets，回退到 32 位 octets
		inOctetsStr := hcInOctetsMap[idx]
		if inOctetsStr == "" {
			inOctetsStr = inOctets32Map[idx]
		}
		outOctetsStr := hcOutOctetsMap[idx]
		if outOctetsStr == "" {
			outOctetsStr = outOctets32Map[idx]
		}
		inOctets, _ := strconv.ParseUint(inOctetsStr, 10, 64)
		outOctets, _ := strconv.ParseUint(outOctetsStr, 10, 64)

		mtu, _ := strconv.Atoi(mtuMap[idx])
		if mtu == 0 {
			mtu = 1500
		}
		ifType, _ := strconv.Atoi(typeMap[idx])

		adminStatus := "down"
		if adminMap[idx] == "1" {
			adminStatus = "up"
		}
		operStatus := "down"
		if operMap[idx] == "1" {
			operStatus = "up"
		}

		name := nameMap[idx]
		if name == "" {
			name = fmt.Sprintf("if%d", idx)
		}

		interfaces = append(interfaces, InterfaceInfo{
			Index:        idx,
			Name:         name,
			Descr:        descrMap[idx],
			Type:         ifType,
			TypeName:     GetIfTypeName(ifType),
			Speed:        effectiveSpeed,
			MTU:          mtu,
			MAC:          FormatMAC(macMap[idx]),
			AdminStatus:  adminStatus,
			OperStatus:   operStatus,
			Alias:        aliasMap[idx],
			InOctets:     inOctets,
			OutOctets:    outOctets,
			InErrors:     parseInt64Safe(inErrMap[idx]),
			OutErrors:    parseInt64Safe(outErrMap[idx]),
			InUcastPkts:  parseInt64Safe(inUcastPktsMap[idx]),
			OutUcastPkts: parseInt64Safe(outUcastPktsMap[idx]),
			LastChange:   parseInt64Safe(lastChangeMap[idx]),
		})
	}

	return interfaces
}

// GetInterfacesSimple 简化调用
func (s *SnmpService) GetInterfacesSimple(host string, port int, version SnmpVersion, community string) []InterfaceInfo {
	return s.GetInterfaces(host, port, version, community, "", "", "", "", "")
}

// ───────────────────────────────────────────────
// 健康检查
// ───────────────────────────────────────────────

// HealthCheck 设备健康检查（系统信息 + 接口 + 厂商指标，整体超时 15s）
func (s *SnmpService) HealthCheck(deviceID string, host string, port int) *DeviceHealth {
	credential := s.GetCredential(deviceID)
	if credential == nil {
		credential = s.GetDefaultCredential()
	}
	if credential == nil {
		return nil
	}

	device, err := repos.NetworkDevices.GetByID(deviceID)
	if err != nil || device == nil {
		return nil
	}

	targetHost := host
	if targetHost == "" {
		targetHost = device.IPAddress
	}
	targetPort := port
	if targetPort == 0 {
		targetPort = credential.SnmpPort
	}
	community := credential.Community
	if community == "" {
		community = "public"
	}

	ctx, cancel := context.WithTimeout(context.Background(), snmpHealthTimeout)
	defer cancel()

	resultCh := make(chan *DeviceHealth, 1)
	go func() {
		resultCh <- s.doHealthCheck(ctx, targetHost, targetPort, credential, device.Vendor)
	}()

	select {
	case r := <-resultCh:
		return r
	case <-ctx.Done():
		return nil
	}
}

// doHealthCheck 实际执行健康检查；所有 SNMP walk 都受 ctx 约束，超时后立即中断
func (s *SnmpService) doHealthCheck(ctx context.Context, host string, port int, cred *SnmpCredential, vendor string) *DeviceHealth {
	// 并行获取系统信息和接口
	var sysInfo *SystemInfo
	var interfaces []InterfaceInfo
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		sysInfo = s.GetSystemInfo(host, port, cred.SnmpVersion, cred.Community,
			cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
			cred.SnmpPrivProtocol, cred.SnmpPrivKey)
	}()
	go func() {
		defer wg.Done()
		interfaces = s.GetInterfacesCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
			cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
			cred.SnmpPrivProtocol, cred.SnmpPrivKey)
	}()
	wg.Wait()

	// 获取厂商特定指标
	var cpuUsage, memoryUsage, temperature *float64
	var powerStatus, fanStatus *string
	var opticRxPower, opticTxPower, opticTemp *float64

	if vendorOIDs, ok := VendorOIDs[vendor]; ok {
		// CPU 和内存
		var vendorOIDList []string
		if vendorOIDs.CPUUsage != "" {
			vendorOIDList = append(vendorOIDList, vendorOIDs.CPUUsage)
		}
		if vendorOIDs.CPU5sec != "" {
			vendorOIDList = append(vendorOIDList, vendorOIDs.CPU5sec)
		}
		if vendorOIDs.MemoryUsage != "" {
			vendorOIDList = append(vendorOIDList, vendorOIDs.MemoryUsage)
		}

		if len(vendorOIDList) > 0 {
			vendorMetrics := s.SnmpGetMultiple(host, port, cred.SnmpVersion, cred.Community,
				cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
				cred.SnmpPrivProtocol, cred.SnmpPrivKey, vendorOIDList)
			for _, m := range vendorMetrics {
				val, _ := strconv.ParseFloat(m.Value, 64)
				if m.OID == vendorOIDs.CPUUsage || m.OID == vendorOIDs.CPU5sec {
					cpuUsage = &val
				}
				if m.OID == vendorOIDs.MemoryUsage {
					memoryUsage = &val
				}
			}
		}

		// 温度（限时 3 秒）
		if len(vendorOIDs.Temperature) > 0 {
			tempCh := make(chan *float64, 1)
			go func() {
				for _, tempOID := range vendorOIDs.Temperature {
					temps := s.SnmpWalkCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
						cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
						cred.SnmpPrivProtocol, cred.SnmpPrivKey, tempOID, 10)
					var maxVal float64
					found := false
					for _, t := range temps {
						v, _ := strconv.ParseFloat(t.Value, 64)
						if v > 0 {
							if v > maxVal {
								maxVal = v
							}
							found = true
						}
					}
					if found {
						tempCh <- &maxVal
						return
					}
				}
				tempCh <- nil
			}()

			select {
			case t := <-tempCh:
				temperature = t
			case <-time.After(snmpTempTimeout):
				temperature = nil
			}
		}

		// 电源状态（WALK，3秒超时）
		if vendorOIDs.PowerStatus != "" {
			powerCh := make(chan *string, 1)
			go func() {
				results := s.SnmpWalkCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
					cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
					cred.SnmpPrivProtocol, cred.SnmpPrivKey, vendorOIDs.PowerStatus, 20)
				if len(results) == 0 {
					powerCh <- nil
					return
				}
				// 汇总：状态值 1=正常, 其他=异常
				normalCount := 0
				abnormalCount := 0
				for _, r := range results {
					v, _ := strconv.ParseFloat(r.Value, 64)
					if v == 1 || v == 2 {
						normalCount++
					} else {
						abnormalCount++
					}
				}
				status := fmt.Sprintf("正常 %d / 异常 %d", normalCount, abnormalCount)
				if abnormalCount > 0 {
					status = fmt.Sprintf("⚠️ %d 个电源异常 (正常 %d)", abnormalCount, normalCount)
				}
				powerCh <- &status
			}()
			select {
			case ps := <-powerCh:
				powerStatus = ps
			case <-time.After(snmpTempTimeout):
			}
		}

		// 风扇状态（WALK，3秒超时）
		if vendorOIDs.FanStatus != "" {
			fanCh := make(chan *string, 1)
			go func() {
				results := s.SnmpWalkCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
					cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
					cred.SnmpPrivProtocol, cred.SnmpPrivKey, vendorOIDs.FanStatus, 20)
				if len(results) == 0 {
					fanCh <- nil
					return
				}
				normalCount := 0
				abnormalCount := 0
				for _, r := range results {
					v, _ := strconv.ParseFloat(r.Value, 64)
					if v == 1 || v == 2 {
						normalCount++
					} else {
						abnormalCount++
					}
				}
				status := fmt.Sprintf("正常 %d / 异常 %d", normalCount, abnormalCount)
				if abnormalCount > 0 {
					status = fmt.Sprintf("⚠️ %d 个风扇异常 (正常 %d)", abnormalCount, normalCount)
				}
				fanCh <- &status
			}()
			select {
			case fs := <-fanCh:
				fanStatus = fs
			case <-time.After(snmpTempTimeout):
			}
		}

		// 光模块指标（WALK，取第一个有效值，3秒超时）
		if vendorOIDs.OpticRxPower != "" || vendorOIDs.OpticTxPower != "" || vendorOIDs.OpticTemperature != "" {
			opticWG := sync.WaitGroup{}
			opticDone := make(chan struct{})
			go func() {
				if vendorOIDs.OpticRxPower != "" {
					opticWG.Add(1)
					go func() {
						defer opticWG.Done()
						results := s.SnmpWalkCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
							cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
							cred.SnmpPrivProtocol, cred.SnmpPrivKey, vendorOIDs.OpticRxPower, 20)
						for _, r := range results {
							v, _ := strconv.ParseFloat(r.Value, 64)
							// 光功率值通常在 -30 ~ 5 dBm 范围
							if v != 0 && v > -100 && v < 100 {
								opticRxPower = &v
								break
							}
						}
					}()
				}
				if vendorOIDs.OpticTxPower != "" {
					opticWG.Add(1)
					go func() {
						defer opticWG.Done()
						results := s.SnmpWalkCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
							cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
							cred.SnmpPrivProtocol, cred.SnmpPrivKey, vendorOIDs.OpticTxPower, 20)
						for _, r := range results {
							v, _ := strconv.ParseFloat(r.Value, 64)
							if v != 0 && v > -100 && v < 100 {
								opticTxPower = &v
								break
							}
						}
					}()
				}
				if vendorOIDs.OpticTemperature != "" {
					opticWG.Add(1)
					go func() {
						defer opticWG.Done()
						results := s.SnmpWalkCtx(ctx, host, port, cred.SnmpVersion, cred.Community,
							cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
							cred.SnmpPrivProtocol, cred.SnmpPrivKey, vendorOIDs.OpticTemperature, 20)
						for _, r := range results {
							v, _ := strconv.ParseFloat(r.Value, 64)
							if v != 0 && v > -50 && v < 200 {
								opticTemp = &v
								break
							}
						}
					}()
				}
				opticWG.Wait()
				close(opticDone)
			}()
			select {
			case <-opticDone:
			case <-time.After(snmpTempTimeout):
			}
		}
	}

	// 统计接口
	upIfs := 0
	highUtilIfs := 0
	for _, iface := range interfaces {
		if iface.OperStatus == "up" {
			upIfs++
		}
		if iface.InUtilization > 80 || iface.OutUtilization > 80 {
			highUtilIfs++
		}
	}

	health := &DeviceHealth{
		InterfaceCount:    len(interfaces),
		InterfacesUp:      upIfs,
		InterfacesDown:    len(interfaces) - upIfs,
		InterfaceHighUtil: highUtilIfs,
		CpuUsage:          cpuUsage,
		MemoryUsage:       memoryUsage,
		Temperature:       temperature,
		PowerStatus:       powerStatus,
		FanStatus:         fanStatus,
		OpticRxPower:      opticRxPower,
		OpticTxPower:      opticTxPower,
		OpticTemperature:  opticTemp,
	}

	if sysInfo != nil {
		health.SysName = sysInfo.SysName
		health.SysDescr = sysInfo.SysDescr
		health.SysUptime = sysInfo.SysUptime
		health.SysLocation = sysInfo.SysLocation
		health.SysContact = sysInfo.SysContact
	}

	return health
}

// ───────────────────────────────────────────────
// SNMP 巡检（inspect-snmp 端点）
// ───────────────────────────────────────────────

// InspectDevice 对单台设备执行 SNMP 巡检
// 返回系统信息 + 接口状态 + 速率/利用率 + 告警，同时持久化接口指标到 snmp_interface_metrics
func (s *SnmpService) InspectDevice(deviceID string) *SnmpInspectionResult {
	startTime := time.Now()

	// 获取设备信息
	device, err := repos.NetworkDevices.GetByID(deviceID)
	if err != nil || device == nil {
		return nil
	}
	if device.SnmpEnabled != 1 {
		return nil
	}

	// 获取 SNMP 凭证（自动解密）
	cred := s.GetCredential(deviceID)
	if cred == nil {
		cred = s.GetDefaultCredential()
	}
	if cred == nil {
		// 无凭证时尝试使用默认 public
		cred = &SnmpCredential{
			SnmpVersion: SNMPv2c,
			SnmpPort:    161,
			Community:   "public",
		}
	}

	host := device.IPAddress
	port := device.SnmpPort
	if port == 0 {
		port = cred.SnmpPort
	}
	if port == 0 {
		port = 161
	}
	community := cred.Community
	if community == "" {
		community = "public"
	}

	// 并行获取系统信息和接口列表
	var sysInfo *SystemInfo
	var interfaces []InterfaceInfo
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sysInfo = s.GetSystemInfo(host, port, cred.SnmpVersion, community,
			cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
			cred.SnmpPrivProtocol, cred.SnmpPrivKey)
	}()
	go func() {
		defer wg.Done()
		interfaces = s.GetInterfaces(host, port, cred.SnmpVersion, community,
			cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
			cred.SnmpPrivProtocol, cred.SnmpPrivKey)
	}()
	wg.Wait()

	// 设备完全不可达
	if sysInfo == nil && len(interfaces) == 0 {
		return &SnmpInspectionResult{
			Reachable:      false,
			Alerts:         []string{"❌ SNMP 连接失败，设备无法响应"},
			Interfaces:     []SnmpInterfaceMetric{},
			PollDurationMs: time.Since(startTime).Milliseconds(),
		}
	}

	// 获取上一轮快照用于速率计算
	prevMetrics, _ := repos.SnmpInterfaceMetrics.GetLatestByDeviceID(deviceID)

	// 构建巡检结果
	var metrics []SnmpInterfaceMetric
	var alerts []string
	upCount := 0
	downCount := 0
	var dbMetrics []repos.InterfaceMetric

	for _, iface := range interfaces {
		// 告警：管理 UP 但运行 DOWN
		if iface.AdminStatus == "up" && iface.OperStatus == "down" {
			alerts = append(alerts, fmt.Sprintf("⚠️ %s - 管理状态 UP 但运行状态 DOWN", iface.Name))
		}

		// 从上一轮快照计算速率和利用率
		var inBps, outBps int64
		var inUtil, outUtil float64
		if prev, ok := prevMetrics[iface.Index]; ok {
			prevTime, _ := time.ParseInLocation("2006-01-02 15:04:05", prev.SampledAt, time.Local)
			intervalSec := time.Since(prevTime).Seconds()
			if intervalSec < 30 {
				intervalSec = 30
			}

			if iface.InOctets >= uint64(prev.IfInOctets) {
				deltaIn := int64(iface.InOctets) - prev.IfInOctets
				inBps = deltaIn * 8 / int64(intervalSec)
			}
			if iface.OutOctets >= uint64(prev.IfOutOctets) {
				deltaOut := int64(iface.OutOctets) - prev.IfOutOctets
				outBps = deltaOut * 8 / int64(intervalSec)
			}

			if iface.Speed > 0 {
				inUtil = float64(inBps) / float64(iface.Speed) * 100
				if inUtil > 100 {
					inUtil = 100
				}
				outUtil = float64(outBps) / float64(iface.Speed) * 100
				if outUtil > 100 {
					outUtil = 100
				}
			}

			// 错误包激增告警
			if iface.AdminStatus == "up" && iface.OperStatus == "up" {
				errDelta := iface.InErrors - prev.IfInErrors
				if errDelta > 100 {
					alerts = append(alerts, fmt.Sprintf("⚠️ %s - 入方向错误包激增（+%d）", iface.Name, errDelta))
				}
			}
		}

		// 四舍五入到 2 位小数
		inUtil = float64(int(inUtil*100)) / 100
		outUtil = float64(int(outUtil*100)) / 100

		if iface.OperStatus == "up" {
			upCount++
		} else {
			downCount++
		}

		metrics = append(metrics, SnmpInterfaceMetric{
			Index:          iface.Index,
			Name:           iface.Name,
			OperStatus:     iface.OperStatus,
			AdminStatus:    iface.AdminStatus,
			Speed:          iface.Speed,
			MTU:            iface.MTU,
			MAC:            iface.MAC,
			InBps:          inBps,
			OutBps:         outBps,
			InUtilization:  inUtil,
			OutUtilization: outUtil,
			InErrors:       iface.InErrors,
			OutErrors:      iface.OutErrors,
		})

		// 构建 DB 持久化数据
		adminStatusInt := 2
		if iface.AdminStatus == "up" {
			adminStatusInt = 1
		}
		operStatusInt := 2
		if iface.OperStatus == "up" {
			operStatusInt = 1
		}
		dbMetrics = append(dbMetrics, repos.InterfaceMetric{
			InterfaceName: iface.Name,
			IfIndex:       iface.Index,
			IfSpeed:       iface.Speed,
			IfAdminStatus: adminStatusInt,
			IfOperStatus:  operStatusInt,
			IfInOctets:    int64(iface.InOctets),
			IfOutOctets:   int64(iface.OutOctets),
			IfInErrors:    iface.InErrors,
			IfOutErrors:   iface.OutErrors,
		})
	}

	// 若开启了 InfluxDB 时序库写入，即时推送轮询到的接口与设备指标至 InfluxDB；并跳过 SQLite 写入以防 SQLite 膨胀
	if dbservices.IsInfluxdbWriteEnabled() {
		var influxRows []dbservices.SnmpCollectResultRow
		nowStr := time.Now().Format("2006-01-02 15:04:05")
		for _, iface := range interfaces {
			ifIndex := fmt.Sprintf("%d", iface.Index)
			ifName := iface.Name
			// 出向字节数
			influxRows = append(influxRows, dbservices.SnmpCollectResultRow{
				TaskName:    "snmp_interface_poll",
				DeviceID:    deviceID,
				DeviceName:  device.Name,
				DeviceIP:    device.IPAddress,
				OID:         fmt.Sprintf("1.3.6.1.2.1.31.1.1.1.10.%s", ifIndex),
				MetricName:  fmt.Sprintf("ifHCOutOctets_%s", ifName),
				MetricValue: fmt.Sprintf("%d", iface.OutOctets),
				MetricType:  "counter",
				SampledAt:   nowStr,
			})
			// 入向字节数
			influxRows = append(influxRows, dbservices.SnmpCollectResultRow{
				TaskName:    "snmp_interface_poll",
				DeviceID:    deviceID,
				DeviceName:  device.Name,
				DeviceIP:    device.IPAddress,
				OID:         fmt.Sprintf("1.3.6.1.2.1.31.1.1.1.6.%s", ifIndex),
				MetricName:  fmt.Sprintf("ifHCInOctets_%s", ifName),
				MetricValue: fmt.Sprintf("%d", iface.InOctets),
				MetricType:  "counter",
				SampledAt:   nowStr,
			})
			// 运行状态 (1=UP, 2=DOWN)
			operStatusVal := "2"
			if iface.OperStatus == "up" {
				operStatusVal = "1"
			}
			influxRows = append(influxRows, dbservices.SnmpCollectResultRow{
				TaskName:    "snmp_interface_poll",
				DeviceID:    deviceID,
				DeviceName:  device.Name,
				DeviceIP:    device.IPAddress,
				OID:         fmt.Sprintf("1.3.6.1.2.1.2.2.1.8.%s", ifIndex),
				MetricName:  fmt.Sprintf("ifOperStatus_%s", ifName),
				MetricValue: operStatusVal,
				MetricType:  "gauge",
				SampledAt:   nowStr,
			})
		}
		if len(influxRows) > 0 {
			dbservices.WriteSnmpResultsToInfluxdb(influxRows)
		}
	} else {
		// 未开启 InfluxDB 时，写入本地 SQLite 存储
		if len(dbMetrics) > 0 {
			if err := repos.SnmpInterfaceMetrics.InsertBatch(deviceID, dbMetrics); err != nil {
				logger.Warnf("Failed to save interface metrics for device %s: %v", deviceID, err)
			}
		}
		// 写入命名指标样本（snmp_exporter 风格，供首页大屏/面板编辑器统一消费）
		WriteSnmpMetricSamplesFromInspect(deviceID, sysInfo, interfaces)
	}

	// 更新设备最后 SNMP 时间
	if err := repos.NetworkDevices.UpdateLastSnmpAt(deviceID); err != nil {
		logger.Warnf("Failed to update last_snmp_at for device %s: %v", deviceID, err)
	}

	// 系统信息
	sysName := "unknown"
	sysDescr := ""
	var sysUptime int64
	if sysInfo != nil {
		if sysInfo.SysName != "" {
			sysName = sysInfo.SysName
		}
		sysDescr = sysInfo.SysDescr
		sysUptime = sysInfo.SysUptime
	}

	if metrics == nil {
		metrics = []SnmpInterfaceMetric{}
	}
	if alerts == nil {
		alerts = []string{}
	}

	return &SnmpInspectionResult{
		Reachable:      true,
		SysName:        sysName,
		SysDescr:       sysDescr,
		SysUptime:      sysUptime,
		Interfaces:     metrics,
		InterfaceCount: len(metrics),
		UpCount:        upCount,
		DownCount:      downCount,
		Alerts:         alerts,
		PollDurationMs: time.Since(startTime).Milliseconds(),
	}
}

// ───────────────────────────────────────────────
// 连接测试
// TestConnection 测试 SNMP 连通性
func (s *SnmpService) TestConnection(host string, port int, version SnmpVersion, community string) bool {
	return s.TestCredentialConnection(host, port, version, community, "", "", "", "", "")
}

// TestCredentialConnection 测试带完整凭据的 SNMP 连通性（快速探测，单次 2 秒超时，1 次重试，批量发送探测包）
func (s *SnmpService) TestCredentialConnection(host string, port int, version SnmpVersion, community, user, authProtocol, authKey, privProtocol, privKey string) bool {
	if port == 0 {
		port = 161
	}
	if version == "" {
		version = SNMPv2c
	}
	if community == "" && version != SNMPv3 {
		community = "public"
	}
	if host == "" {
		return false
	}

	testOIDs := []string{
		SystemOIDs.SysUptime, // .1.3.6.1.2.1.1.3.0
		SystemOIDs.SysName,   // .1.3.6.1.2.1.1.5.0
		SystemOIDs.SysDescr,  // .1.3.6.1.2.1.1.1.0
	}

	resultCh := make(chan bool, 1)
	go func() {
		session := s.CreateSession(host, port, version, community, user, authProtocol, authKey, privProtocol, privKey)
		session.Timeout = 2 * time.Second
		session.Retries = 1
		if err := session.Connect(); err != nil {
			resultCh <- false
			return
		}
		defer session.Conn.Close()

		resp, err := session.Get(testOIDs)
		if err != nil || len(resp.Variables) == 0 {
			resultCh <- false
			return
		}

		for _, pdu := range resp.Variables {
			if pdu.Type != gosnmp.NoSuchObject && pdu.Type != gosnmp.NoSuchInstance && pdu.Type != gosnmp.Null {
				val := NormalizeSnmpValue(int(pdu.Type), pdu.Value)
				if val != "" && val != "N/A" {
					resultCh <- true
					return
				}
			}
		}
		resultCh <- false
	}()

	select {
	case ok := <-resultCh:
		return ok
	case <-time.After(3500 * time.Millisecond):
		return false
	}
}

// ───────────────────────────────────────────────
// 自定义 OID 采集
// ───────────────────────────────────────────────

// CustomOidEntry 自定义 OID 采集条目
type CustomOidEntry struct {
	OID        string `json:"oid"`
	MetricName string `json:"metric_name"`
	IsTable    int    `json:"is_table,omitempty"`
}

// CustomOidResult 自定义 OID 采集结果
type CustomOidResult struct {
	MetricName  string `json:"metric_name"`
	MetricValue string `json:"metric_value"`
	OID         string `json:"oid"`
	Error       string `json:"error,omitempty"` // 连接失败/认证错误等
}

// CollectCustomOids 采集自定义 OID（标量 GET + 表列 WALK）
func (s *SnmpService) CollectCustomOids(host string, port int, version SnmpVersion, community string,
	user, authProtocol, authKey, privProtocol, privKey string, oidEntries []CustomOidEntry, timeoutMs int) []CustomOidResult {

	if len(oidEntries) == 0 {
		return []CustomOidResult{}
	}

	var output []CustomOidResult

	// 创建会话测试连通性
	session := s.CreateSession(host, port, version, community, user, authProtocol, authKey, privProtocol, privKey)
	if timeoutMs > 0 {
		session.Timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	if err := session.Connect(); err != nil {
		// 连接失败：所有 OID 返回错误信息
		errMsg := fmt.Sprintf("连接失败: %v", err)
		var results []CustomOidResult
		for _, e := range oidEntries {
			results = append(results, CustomOidResult{
				MetricName:  e.MetricName,
				MetricValue: "N/A",
				OID:         e.OID,
				Error:       errMsg,
			})
		}
		return results
	}
	defer session.Conn.Close()

	// 分组：标量 vs 表列
	var scalarEntries, tableEntries []CustomOidEntry
	for _, e := range oidEntries {
		if e.IsTable == 1 {
			tableEntries = append(tableEntries, e)
		} else {
			scalarEntries = append(scalarEntries, e)
		}
	}

	// 标量 OID：使用已有会话分批 GET
	if len(scalarEntries) > 0 {
		oidMap := make(map[string]string)
		for i := 0; i < len(scalarEntries); i += snmpGetBatchSize {
			end := i + snmpGetBatchSize
			if end > len(scalarEntries) {
				end = len(scalarEntries)
			}
			batch := scalarEntries[i:end]
			oids := make([]string, len(batch))
			for j, e := range batch {
				oids[j] = e.OID
			}
			resp, err := session.Get(oids)
			if err != nil {
				// GET 批次失败，记录错误
				for _, e := range batch {
					output = append(output, CustomOidResult{
						MetricName:  e.MetricName,
						MetricValue: "N/A",
						OID:         e.OID,
						Error:       fmt.Sprintf("SNMP GET 失败: %v", err),
					})
				}
				continue
			}
			for _, pdu := range resp.Variables {
				if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
					continue
				}
				normVal := NormalizeSnmpValue(int(pdu.Type), pdu.Value)
				cleanName := strings.TrimPrefix(pdu.Name, ".")
				oidMap[pdu.Name] = normVal
				oidMap[cleanName] = normVal
				oidMap["."+cleanName] = normVal
			}
		}
		for _, entry := range scalarEntries {
			cleanOid := strings.TrimPrefix(entry.OID, ".")
			val := oidMap[entry.OID]
			if val == "" {
				val = oidMap[cleanOid]
			}
			if val == "" {
				val = oidMap["."+cleanOid]
			}
			// 若无返回且无 .0 尾缀，尝试补 .0 采集标量
			if val == "" && !strings.HasSuffix(entry.OID, ".0") {
				dotZeroOid := entry.OID + ".0"
				if respZero, errZero := session.Get([]string{dotZeroOid}); errZero == nil {
					for _, pdu := range respZero.Variables {
						if pdu.Type != gosnmp.NoSuchObject && pdu.Type != gosnmp.NoSuchInstance {
							val = NormalizeSnmpValue(int(pdu.Type), pdu.Value)
							break
						}
					}
				}
			}

			if val == "" {
				val = "N/A"
			}
			output = append(output, CustomOidResult{
				MetricName:  entry.MetricName,
				MetricValue: val,
				OID:         entry.OID,
			})
		}
	}

	// 表列 OID：分批并行 WALK（需要各自独立会话，因为 WALK 需要独占连接）
	for i := 0; i < len(tableEntries); i += snmpWalkConcurrency {
		end := i + snmpWalkConcurrency
		if end > len(tableEntries) {
			end = len(tableEntries)
		}
		batch := tableEntries[i:end]

		type walkResult struct {
			entry  CustomOidEntry
			walked []*SnmpResult
			err    error
		}
		walkResults := make([]walkResult, len(batch))
		var wg sync.WaitGroup
		for j, entry := range batch {
			wg.Add(1)
			go func(idx int, e CustomOidEntry) {
				defer wg.Done()
				// WALK 需要独立会话
				ws := s.CreateSession(host, port, version, community, user, authProtocol, authKey, privProtocol, privKey)
				if timeoutMs > 0 {
					ws.Timeout = time.Duration(timeoutMs) * time.Millisecond
				}
				if connErr := ws.Connect(); connErr != nil {
					walkResults[idx] = walkResult{entry: e, err: connErr}
					return
				}
				defer ws.Conn.Close()

				var walked []*SnmpResult
				walkFn := func(pdu gosnmp.SnmpPDU) error {
					if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
						return nil
					}
					walked = append(walked, &SnmpResult{
						OID:      pdu.Name,
						Value:    NormalizeSnmpValue(int(pdu.Type), pdu.Value),
						Type:     int(pdu.Type),
						TypeName: SnmpTypeName(int(pdu.Type)),
					})
					return nil
				}
				err := s.walkWithContext(ws, e.OID, walkFn)
				walkResults[idx] = walkResult{entry: e, walked: walked, err: err}
			}(j, entry)
		}
		wg.Wait()

		for _, wr := range walkResults {
			if wr.err != nil {
				output = append(output, CustomOidResult{
					MetricName:  wr.entry.MetricName,
					MetricValue: "N/A",
					OID:         wr.entry.OID,
					Error:       fmt.Sprintf("WALK 失败: %v", wr.err),
				})
			} else if len(wr.walked) == 0 {
				output = append(output, CustomOidResult{
					MetricName:  wr.entry.MetricName,
					MetricValue: "N/A",
					OID:         wr.entry.OID,
				})
			} else {
				// 合并为 "index=value" 格式
				var parts []string
				cleanBase := strings.TrimPrefix(wr.entry.OID, ".")
				for _, r := range wr.walked {
					cleanOid := strings.TrimPrefix(r.OID, ".")
					idx := strings.TrimPrefix(cleanOid, cleanBase+".")
					parts = append(parts, fmt.Sprintf("%s=%s", idx, r.Value))
				}
				output = append(output, CustomOidResult{
					MetricName:  wr.entry.MetricName,
					MetricValue: strings.Join(parts, "; "),
					OID:         wr.entry.OID,
				})
			}
		}
	}

	return output
}

// ───────────────────────────────────────────────
// 设备发现（网段扫描）
// ───────────────────────────────────────────────

// DiscoveredDevice 发现的设备
type DiscoveredDevice struct {
	IP       string `json:"ip"`
	SysName  string `json:"sysName"`
	SysDescr string `json:"sysDescr"`
}

// DiscoverDevices 从网段自动发现 SNMP 设备（并行多线程并发扫描，仅支持 /24 及以上子网）
func (s *SnmpService) DiscoverDevices(subnet string, community string, version SnmpVersion, port int) []DiscoveredDevice {
	results := []DiscoveredDevice{}
	if subnet == "" || port == 0 {
		port = 161
	}
	if community == "" {
		community = "public"
	}

	// 解析网段 CIDR
	parts := strings.SplitN(subnet, "/", 2)
	if len(parts) != 2 {
		return results
	}
	ipParts := strings.Split(parts[0], ".")
	if len(ipParts) != 4 {
		return results
	}
	prefixLen, err := strconv.Atoi(parts[1])
	if err != nil || prefixLen < 24 {
		return results // 只扫描 /24 以上
	}

	hostCount := 1 << (32 - prefixLen)
	hostCount -= 2 // 去掉网络地址和广播地址
	if hostCount > 254 {
		return results
	}

	// 计算 IP 范围
	base := make([]int, 4)
	for i, p := range ipParts {
		base[i], _ = strconv.Atoi(p)
	}

	type ipTask struct {
		ip string
	}
	ipTasks := make(chan ipTask, hostCount)
	for i := 1; i <= hostCount; i++ {
		ipTasks <- ipTask{ip: fmt.Sprintf("%d.%d.%d.%d", base[0], base[1], base[2], base[3]+i)}
	}
	close(ipTasks)

	workerCount := 32
	if hostCount < workerCount {
		workerCount = hostCount
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range ipTasks {
				sysInfo := s.GetSystemInfoSimple(task.ip, port, version, community)
				if sysInfo != nil && sysInfo.SysName != "" {
					mu.Lock()
					results = append(results, DiscoveredDevice{
						IP:       task.ip,
						SysName:  sysInfo.SysName,
						SysDescr: sysInfo.SysDescr,
					})
					mu.Unlock()
					logger.Infof("SNMP discovered: %s (%s)", task.ip, sysInfo.SysName)
				}
			}
		}()
	}
	wg.Wait()

	return results
}

// ───────────────────────────────────────────────
// 辅助函数
// ───────────────────────────────────────────────

// parseInt64Safe 安全解析 int64
func parseInt64Safe(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
