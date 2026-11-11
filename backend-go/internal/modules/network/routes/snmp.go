// SNMP 路由 — 27 个端点
// 等价于 TS routes/snmpRoutes.ts
package routes

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// SnmpRoutes SNMP 路由
func SnmpRoutes() chi.Router {
	r := chi.NewRouter()

	// ================================================================
	// SNMP 凭证管理
	// ================================================================

	// GET / & GET /status — SNMP 状态概览
	handleSnmpStatus := func(w http.ResponseWriter, r *http.Request) {
		var credCount, taskCount int
		appdb.DB().QueryRow(`SELECT COUNT(*) FROM snmp_credentials`).Scan(&credCount)
		appdb.DBForTable("snmp_polling_tasks").QueryRow(`SELECT COUNT(*) FROM snmp_polling_tasks`).Scan(&taskCount)
		httpx.Respond(w, map[string]interface{}{
			"status":            "healthy",
			"credentials_count": credCount,
			"tasks_count":       taskCount,
		})
	}
	r.Get("/", handleSnmpStatus)
	r.Get("/status", handleSnmpStatus)

	// GET /credentials — 获取 SNMP 凭证列表（含 community，仅 admin/operator）
	r.With(middleware.RequireRole("admin", "operator")).Get("/credentials", func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.URL.Query().Get("deviceId")
		rows, err := repos.SnmpCredentials.List(deviceID)
		if err != nil {
			httpx.RespondError(w, "获取 SNMP 凭证失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, rows)
	})

	// POST /credentials — 保存 SNMP 凭证
	r.With(middleware.RequireRole("admin")).Post("/credentials", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceID         string `json:"device_id"`
			Name             string `json:"name"`
			Community        string `json:"community"`
			SnmpVersion      string `json:"snmp_version"`
			SnmpPort         int    `json:"snmp_port"`
			SnmpUser         string `json:"snmp_user"`
			SnmpAuthProtocol string `json:"snmp_auth_protocol"`
			SnmpAuthKey      string `json:"snmp_auth_key"`
			SnmpPrivProtocol string `json:"snmp_priv_protocol"`
			SnmpPrivKey      string `json:"snmp_priv_key"`
			Host             string `json:"host"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		if req.SnmpVersion == "" {
			req.SnmpVersion = "v2c"
		}
		if req.SnmpPort == 0 {
			req.SnmpPort = 161
		}
		if req.Name == "" {
			req.Name = "default"
		}

		// SNMP 只读串直接明文存储与展示
		communityVal := req.Community
		if communityVal == "" {
			communityVal = "public"
		}
		var authKeyEnc, privKeyEnc string
		if req.SnmpAuthKey != "" {
			enc, encErr := crypto.Encrypt(req.SnmpAuthKey)
			if encErr != nil {
				httpx.RespondError(w, "加密 SNMP 认证密钥失败", http.StatusInternalServerError)
				return
			}
			authKeyEnc = enc
		}
		if req.SnmpPrivKey != "" {
			enc, encErr := crypto.Encrypt(req.SnmpPrivKey)
			if encErr != nil {
				httpx.RespondError(w, "加密 SNMP 加密密钥失败", http.StatusInternalServerError)
				return
			}
			privKeyEnc = enc
		}

		id := uuid.New().String()
		err := repos.SnmpCredentials.Create(repos.SnmpCredentialCreateInput{
			ID:               id,
			DeviceID:         req.DeviceID,
			Name:             req.Name,
			Community:        communityVal,
			SnmpVersion:      req.SnmpVersion,
			SnmpPort:         req.SnmpPort,
			SnmpUser:         req.SnmpUser,
			SnmpAuthProtocol: req.SnmpAuthProtocol,
			SnmpAuthKey:      authKeyEnc,
			SnmpPrivProtocol: req.SnmpPrivProtocol,
			SnmpPrivKey:      privKeyEnc,
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]string{"id": id})
	})

	// DELETE /credentials/:id — 删除 SNMP 凭证
	r.With(middleware.RequireRole("admin")).Delete("/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := repos.SnmpCredentials.Delete(id); err != nil {
			httpx.RespondError(w, "删除 SNMP 凭证失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// PUT /credentials/:id — 更新 SNMP 凭证
	r.With(middleware.RequireRole("admin")).Put("/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Name             string `json:"name"`
			Community        string `json:"community"`
			SnmpVersion      string `json:"snmp_version"`
			SnmpPort         int    `json:"snmp_port"`
			SnmpUser         string `json:"snmp_user"`
			SnmpAuthProtocol string `json:"snmp_auth_protocol"`
			SnmpAuthKey      string `json:"snmp_auth_key"`
			SnmpPrivProtocol string `json:"snmp_priv_protocol"`
			SnmpPrivKey      string `json:"snmp_priv_key"`
			Host             string `json:"host"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		communityVal := req.Community
		var authKeyEnc, privKeyEnc string
		if req.SnmpAuthKey != "" {
			enc, encErr := crypto.Encrypt(req.SnmpAuthKey)
			if encErr != nil {
				httpx.RespondError(w, "加密 SNMP 认证密钥失败", http.StatusInternalServerError)
				return
			}
			authKeyEnc = enc
		}
		if req.SnmpPrivKey != "" {
			enc, encErr := crypto.Encrypt(req.SnmpPrivKey)
			if encErr != nil {
				httpx.RespondError(w, "加密 SNMP 加密密钥失败", http.StatusInternalServerError)
				return
			}
			privKeyEnc = enc
		}

		err := repos.SnmpCredentials.Update(id, repos.SnmpCredentialCreateInput{
			Name:             req.Name,
			Community:        communityVal,
			SnmpVersion:      req.SnmpVersion,
			SnmpPort:         req.SnmpPort,
			SnmpUser:         req.SnmpUser,
			SnmpAuthProtocol: req.SnmpAuthProtocol,
			SnmpAuthKey:      authKeyEnc,
			SnmpPrivProtocol: req.SnmpPrivProtocol,
			SnmpPrivKey:      privKeyEnc,
		})
		if err != nil {
			httpx.RespondError(w, "更新 SNMP 凭证失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// ================================================================
	// SNMP 操作
	// ================================================================

	// POST /test — 测试 SNMP 连通性
	handleTestConnection := func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host             string `json:"host"`
			Port             int    `json:"port"`
			Version          string `json:"version"`
			SnmpVersion      string `json:"snmp_version"`
			Community        string `json:"community"`
			User             string `json:"user"`
			SnmpUser         string `json:"snmp_user"`
			AuthProtocol     string `json:"auth_protocol"`
			SnmpAuthProtocol string `json:"snmp_auth_protocol"`
			AuthKey          string `json:"auth_key"`
			SnmpAuthKey      string `json:"snmp_auth_key"`
			PrivProtocol     string `json:"priv_protocol"`
			SnmpPrivProtocol string `json:"snmp_priv_protocol"`
			PrivKey          string `json:"priv_key"`
			SnmpPrivKey      string `json:"snmp_priv_key"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Port == 0 {
			req.Port = 161
		}
		version := req.Version
		if version == "" {
			version = req.SnmpVersion
		}
		if version == "" {
			version = "v2c"
		}
		community := req.Community
		if community == "" {
			community = "public"
		}
		user := req.User
		if user == "" {
			user = req.SnmpUser
		}
		authProto := req.AuthProtocol
		if authProto == "" {
			authProto = req.SnmpAuthProtocol
		}
		authKey := req.AuthKey
		if authKey == "" {
			authKey = req.SnmpAuthKey
		}
		privProto := req.PrivProtocol
		if privProto == "" {
			privProto = req.SnmpPrivProtocol
		}
		privKey := req.PrivKey
		if privKey == "" {
			privKey = req.SnmpPrivKey
		}
		if !checkSnmpTarget(w, req.Host, req.Port) {
			return
		}

		ok := services.SnmpServiceInstance.TestCredentialConnection(
			req.Host, req.Port, services.SnmpVersion(version),
			community, user, authProto, authKey, privProto, privKey,
		)
		code := 0
		msg := "SNMP 探测成功，设备响应正常"
		if !ok {
			code = -1
			msg = "SNMP 探测超时或无响应（请检查设备IP、UDP 161端口放通或只读串口令）"
		}
		httpx.Respond(w, map[string]interface{}{
			"code":      code,
			"success":   ok,
			"reachable": ok,
			"message":   msg,
			"data": map[string]bool{
				"reachable": ok,
			},
		})
	}

	// POST /test & POST /test-connection — 测试 SNMP 连通性
	r.With(middleware.RequireRole("admin", "operator")).Post("/test", handleTestConnection)
	r.With(middleware.RequireRole("admin", "operator")).Post("/test-connection", handleTestConnection)

	// POST /credentials/:id/test — 测试已存储凭证连通性
	r.With(middleware.RequireRole("admin", "operator")).Post("/credentials/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		cred, err := repos.SnmpCredentials.GetByID(id)
		if err != nil || cred == nil {
			httpx.RespondError(w, "凭证不存在", http.StatusNotFound)
			return
		}

		var req struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		}
		_ = parseJSON(w, r, &req)

		host := req.Host
		if host == "" {
			host = cred.Host
		}
		if host == "" {
			httpx.RespondError(w, "无法确定设备 IP，请在设备表单中填写 IP 地址", http.StatusBadRequest)
			return
		}

		community := "public"
		if cred.Community != nil && *cred.Community != "" {
			commStr := *cred.Community
			if strings.HasPrefix(commStr, "2:") {
				if dec, err := crypto.Decrypt(commStr); err == nil && dec != "" {
					community = dec
				}
			} else {
				community = commStr
			}
		}
		port := req.Port
		if port == 0 {
			port = cred.SnmpPort
		}
		if port == 0 {
			port = 161
		}

		user := ""
		if cred.SnmpUser != nil {
			user = *cred.SnmpUser
		}
		authProto := ""
		if cred.SnmpAuthProtocol != nil {
			authProto = *cred.SnmpAuthProtocol
		}
		authKey := ""
		if cred.SnmpAuthKey != nil {
			authKey = *cred.SnmpAuthKey
			if strings.HasPrefix(authKey, "2:") {
				if dec, err := crypto.Decrypt(authKey); err == nil {
					authKey = dec
				}
			}
		}
		privProto := ""
		if cred.SnmpPrivProtocol != nil {
			privProto = *cred.SnmpPrivProtocol
		}
		privKey := ""
		if cred.SnmpPrivKey != nil {
			privKey = *cred.SnmpPrivKey
			if strings.HasPrefix(privKey, "2:") {
				if dec, err := crypto.Decrypt(privKey); err == nil {
					privKey = dec
				}
			}
		}

		ok := services.SnmpServiceInstance.TestCredentialConnection(
			host, port, services.SnmpVersion(cred.SnmpVersion),
			community, user, authProto, authKey, privProto, privKey,
		)
		code := 0
		msg := "SNMP 探测成功，设备响应正常"
		if !ok {
			code = -1
			msg = "SNMP 探测超时或无响应（请检查设备IP、UDP 161端口放通或只读串口令）"
		}
		httpx.Respond(w, map[string]interface{}{
			"code":      code,
			"success":   ok,
			"reachable": ok,
			"message":   msg,
			"data": map[string]bool{
				"reachable": ok,
			},
		})
	})

	// POST /get — SNMP GET
	r.With(middleware.RequireRole("admin", "operator")).Post("/get", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host      string `json:"host"`
			Port      int    `json:"port"`
			Version   string `json:"version"`
			Community string `json:"community"`
			OID       string `json:"oid"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Community == "" {
			req.Community = "public"
		}
		if req.OID == "" {
			req.OID = services.SystemOIDs.SysName
		}
		if !checkSnmpTarget(w, req.Host, req.Port) {
			return
		}
		if !validOID(req.OID) {
			httpx.RespondError(w, "无效的 OID 格式", http.StatusBadRequest)
			return
		}

		result := services.SnmpServiceInstance.SnmpGet(req.Host, req.Port, services.SnmpVersion(req.Version), req.Community, "", "", "", "", "", req.OID)
		httpx.Respond(w, result)
	})

	// POST /walk — SNMP WALK
	r.With(middleware.RequireRole("admin", "operator")).Post("/walk", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host      string `json:"host"`
			Port      int    `json:"port"`
			Version   string `json:"version"`
			Community string `json:"community"`
			OID       string `json:"oid"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Community == "" {
			req.Community = "public"
		}
		if req.OID == "" {
			req.OID = services.IfMibOIDs.IfTable
		}
		if !checkSnmpTarget(w, req.Host, req.Port) {
			return
		}
		if !validOID(req.OID) {
			httpx.RespondError(w, "无效的 OID 格式", http.StatusBadRequest)
			return
		}

		results := services.SnmpServiceInstance.SnmpWalk(req.Host, req.Port, services.SnmpVersion(req.Version), req.Community, "", "", "", "", "", req.OID, 0)
		httpx.Respond(w, results)
	})

	// POST /system-info — 获取系统信息
	r.With(middleware.RequireRole("admin", "operator")).Post("/system-info", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host      string `json:"host"`
			Port      int    `json:"port"`
			Version   string `json:"version"`
			Community string `json:"community"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Community == "" {
			req.Community = "public"
		}
		if !checkSnmpTarget(w, req.Host, req.Port) {
			return
		}

		info := services.SnmpServiceInstance.GetSystemInfoSimple(req.Host, req.Port, services.SnmpVersion(req.Version), req.Community)
		httpx.Respond(w, info)
	})

	// POST /interfaces — 获取接口列表
	r.With(middleware.RequireRole("admin", "operator")).Post("/interfaces", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host      string `json:"host"`
			Port      int    `json:"port"`
			Version   string `json:"version"`
			Community string `json:"community"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Community == "" {
			req.Community = "public"
		}
		if !checkSnmpTarget(w, req.Host, req.Port) {
			return
		}

		ifs := services.SnmpServiceInstance.GetInterfacesSimple(req.Host, req.Port, services.SnmpVersion(req.Version), req.Community)
		httpx.Respond(w, ifs)
	})

	// GET /health/:deviceId — 设备健康检查
	r.Get("/health/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")

		_, ip, err := repos.NetworkDevices.GetNameAndIP(deviceID)
		if err != nil || ip == "" {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}

		// 获取凭证
		cred := services.SnmpServiceInstance.GetCredential(deviceID)
		if cred == nil {
			cred = services.SnmpServiceInstance.GetDefaultCredential()
		}
		if cred == nil {
			httpx.RespondError(w, "No SNMP credential configured", http.StatusBadRequest)
			return
		}

		// 获取设备IP
		_, ip, _ = repos.NetworkDevices.GetNameAndIP(deviceID)
		port := cred.SnmpPort
		if port == 0 {
			port = 161
		}

		health := services.SnmpServiceInstance.HealthCheck(deviceID, ip, port)
		if health == nil {
			httpx.RespondError(w, "No SNMP credential or device not found", http.StatusNotFound)
			return
		}
		httpx.Respond(w, health)
	})

	// POST /health-batch — 批量设备健康检查（Goroutine 并行多线程并发采集）
	r.With(middleware.RequireRole("admin", "operator")).Post("/health-batch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceIDs []string `json:"deviceIds"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.DeviceIDs) == 0 {
			httpx.RespondError(w, "deviceIds array required", http.StatusBadRequest)
			return
		}

		results := make(map[string]interface{})
		var mu sync.Mutex
		sem := make(chan struct{}, 20) // 最多 20 并发
		var wg sync.WaitGroup

		for _, id := range req.DeviceIDs {
			wg.Add(1)
			go func(deviceID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				_, ip, _ := repos.NetworkDevices.GetNameAndIP(deviceID)
				cred := services.SnmpServiceInstance.GetCredential(deviceID)
				if cred == nil {
					cred = services.SnmpServiceInstance.GetDefaultCredential()
				}
				if cred == nil || ip == "" {
					mu.Lock()
					results[deviceID] = nil
					mu.Unlock()
					return
				}
				port := cred.SnmpPort
				if port == 0 {
					port = 161
				}
				healthRes := services.SnmpServiceInstance.HealthCheck(deviceID, ip, port)

				mu.Lock()
				results[deviceID] = healthRes
				mu.Unlock()
			}(id)
		}
		wg.Wait()
		httpx.Respond(w, results)
	})

	// POST /discover — SNMP 自动发现
	r.With(middleware.RequireRole("admin", "operator")).Post("/discover", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Subnet    string `json:"subnet"`
			Community string `json:"community"`
			Version   string `json:"version"`
			Port      int    `json:"port"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Community == "" {
			req.Community = "public"
		}
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if !validSnmpPort(req.Port) {
			httpx.RespondError(w, "无效的端口（1-65535）", http.StatusBadRequest)
			return
		}
		// 仅允许 CIDR 网段；服务层只扫 /24 及以上，路由层提前给出明确报错
		_, ipNet, err := net.ParseCIDR(req.Subnet)
		if err != nil {
			httpx.RespondError(w, "无效的网段（CIDR 格式，如 192.168.1.0/24）", http.StatusBadRequest)
			return
		}
		ones, bits := ipNet.Mask.Size()
		if bits-ones > 8 {
			httpx.RespondError(w, "网段过大（最大支持 /24，即 254 个地址）", http.StatusBadRequest)
			return
		}

		devices := services.SnmpServiceInstance.DiscoverDevices(req.Subnet, req.Community, services.SnmpVersion(req.Version), req.Port)
		httpx.Respond(w, devices)
	})

	// ================================================================
	// SNMP Trap 管理
	// ================================================================

	// GET /traps — Trap 历史
	r.Get("/traps", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		sourceIP := r.URL.Query().Get("sourceIp")

		traps, err := repos.SnmpTraps.List(limit, sourceIP)
		if err != nil {
			httpx.RespondError(w, "获取 SNMP Trap 记录失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, traps)
	})

	// POST /traps/test — 生成测试 Trap 记录
	r.With(middleware.RequireRole("admin")).Post("/traps/test", func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		varbinds := `[{"oid":"1.3.6.1.2.1.1.5.0","value":"iKuai","type":4},{"oid":"1.3.6.1.2.1.1.3.0","value":"123456","type":67},{"oid":"1.3.6.1.4.1.9.9.43.1.1.1.0","value":"linkDown","type":4},{"oid":"1.3.6.1.6.3.1.1.4.1.0","value":"1.3.6.1.6.3.1.1.5.3","type":6}]`

		err := repos.SnmpTraps.Insert(repos.SnmpTrapEvent{
			ID:            id,
			SourceIP:      "192.168.60.1",
			TrapType:      "coldStart",
			EnterpriseOID: "1.3.6.1.4.1",
			AgentAddress:  "192.168.60.1",
			GenericType:   0,
			SpecificType:  0,
			VarbindsJSON:  varbinds,
			CreatedAt:     timeNowLocal(),
		})
		if err != nil {
			httpx.RespondError(w, "生成测试 Trap 失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]string{"id": id, "message": "测试 Trap 已生成"})
	})

	// POST /trap/start — 启动 Trap 监听（真实 UDP 监听器，配置持久化）
	r.With(middleware.RequireRole("admin")).Post("/trap/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Port int `json:"port"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&req)
		}
		if err := services.SnmpTrapListenerInstance.Start(req.Port); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusConflict)
			return
		}
		httpx.Respond(w, services.SnmpTrapListenerInstance.Status())
	})

	// POST /trap/stop — 停止 Trap 监听
	r.With(middleware.RequireRole("admin")).Post("/trap/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := services.SnmpTrapListenerInstance.Stop(); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusConflict)
			return
		}
		httpx.Respond(w, services.SnmpTrapListenerInstance.Status())
	})

	// GET /trap/status — Trap 监听器状态
	r.Get("/trap/status", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, services.SnmpTrapListenerInstance.Status())
	})

	// ================================================================
	// 使用设备 ID 执行 SNMP
	// ================================================================

	// GET /device/:deviceId/system-info
	r.Get("/device/{deviceId}/system-info", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		_, ip, err := repos.NetworkDevices.GetNameAndIP(deviceID)
		if err != nil || ip == "" {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}

		cred := services.SnmpServiceInstance.GetCredential(deviceID)
		if cred == nil {
			cred = services.SnmpServiceInstance.GetDefaultCredential()
		}
		if cred == nil {
			httpx.RespondError(w, "No SNMP credential configured", http.StatusBadRequest)
			return
		}

		community := cred.Community
		if community == "" {
			community = "public"
		}
		port := cred.SnmpPort
		if port == 0 {
			port = 161
		}

		info := services.SnmpServiceInstance.GetSystemInfoSimple(ip, port, services.SnmpVersion(cred.SnmpVersion), community)
		httpx.Respond(w, info)
	})

	// GET /device/:deviceId/interfaces
	r.Get("/device/{deviceId}/interfaces", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		_, ip, err := repos.NetworkDevices.GetNameAndIP(deviceID)
		if err != nil || ip == "" {
			httpx.RespondError(w, "Device not found", http.StatusNotFound)
			return
		}

		cred := services.SnmpServiceInstance.GetCredential(deviceID)
		if cred == nil {
			cred = services.SnmpServiceInstance.GetDefaultCredential()
		}
		if cred == nil {
			httpx.RespondError(w, "No SNMP credential configured", http.StatusBadRequest)
			return
		}

		community := cred.Community
		if community == "" {
			community = "public"
		}
		port := cred.SnmpPort
		if port == 0 {
			port = 161
		}

		ifs := services.SnmpServiceInstance.GetInterfacesSimple(ip, port, services.SnmpVersion(cred.SnmpVersion), community)
		httpx.Respond(w, ifs)
	})

	// POST /poll-interfaces — 接口指标采集
	r.With(middleware.RequireRole("admin", "operator")).Post("/poll-interfaces", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Host      string `json:"host"`
			Port      int    `json:"port"`
			Version   string `json:"version"`
			Community string `json:"community"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Port == 0 {
			req.Port = 161
		}
		if req.Version == "" {
			req.Version = "v2c"
		}
		if req.Community == "" {
			req.Community = "public"
		}
		if !checkSnmpTarget(w, req.Host, req.Port) {
			return
		}

		ifs := services.SnmpServiceInstance.GetInterfacesSimple(req.Host, req.Port, services.SnmpVersion(req.Version), req.Community)

		// 精简返回值只保留关键指标
		type metricItem struct {
			Index      int    `json:"index"`
			Name       string `json:"name"`
			OperStatus string `json:"operStatus"`
			Speed      int64  `json:"speed"`
			InOctets   uint64 `json:"inOctets"`
			OutOctets  uint64 `json:"outOctets"`
			InErrors   int64  `json:"inErrors"`
			OutErrors  int64  `json:"outErrors"`
		}
		var metrics []metricItem
		for _, i := range ifs {
			if i.OperStatus == "up" && i.Speed > 0 {
				metrics = append(metrics, metricItem{
					Index:      i.Index,
					Name:       i.Name,
					OperStatus: i.OperStatus,
					Speed:      i.Speed,
					InOctets:   i.InOctets,
					OutOctets:  i.OutOctets,
					InErrors:   i.InErrors,
					OutErrors:  i.OutErrors,
				})
			}
		}
		httpx.Respond(w, metrics)
	})

	// ================================================================
	// 自定义 OID 采集配置管理
	// ================================================================

	// GET /custom-oids — 列出自定义 OID 配置
	r.Get("/custom-oids", func(w http.ResponseWriter, r *http.Request) {
		profiles, err := repos.SnmpCustomOids.List(false)
		if err != nil {
			httpx.RespondError(w, "获取自定义 OID 配置失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, profiles)
	})

	// GET /custom-oids/:id — 获取单个自定义 OID 配置
	r.Get("/custom-oids/{id}", func(w http.ResponseWriter, r *http.Request) {
		profile, err := repos.SnmpCustomOids.GetByID(chi.URLParam(r, "id"))
		if err != nil || profile == nil {
			httpx.RespondError(w, "配置不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, profile)
	})

	// POST /custom-oids — 创建自定义 OID 配置
	r.With(middleware.RequireRole("admin")).Post("/custom-oids", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name                string                   `json:"name"`
			Vendor              string                   `json:"vendor"`
			Description         string                   `json:"description"`
			OIDs                []map[string]interface{} `json:"oids"`
			PollIntervalSeconds int                      `json:"poll_interval_seconds"`
			Enabled             bool                     `json:"enabled"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || len(req.OIDs) == 0 {
			httpx.RespondError(w, "name 和 oids 数组为必填项", http.StatusBadRequest)
			return
		}

		oidsJSON, _ := json.Marshal(req.OIDs)
		enabled := 1
		if !req.Enabled {
			enabled = 0
		}
		if req.PollIntervalSeconds == 0 {
			req.PollIntervalSeconds = 300
		}

		profile := repos.CustomOidProfile{
			ID:                  uuid.New().String(),
			Name:                req.Name,
			Vendor:              nsStr(req.Vendor),
			Description:         nsStr(req.Description),
			OidsJSON:            string(oidsJSON),
			PollIntervalSeconds: req.PollIntervalSeconds,
			Enabled:             enabled,
		}
		if err := repos.SnmpCustomOids.Create(profile); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]string{"id": profile.ID})
	})

	// PUT /custom-oids/:id — 更新自定义 OID 配置
	r.With(middleware.RequireRole("admin")).Put("/custom-oids/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		existing, err := repos.SnmpCustomOids.GetByID(id)
		if err != nil || existing == nil {
			httpx.RespondError(w, "配置不存在", http.StatusNotFound)
			return
		}

		var req struct {
			Name                string                   `json:"name"`
			Vendor              string                   `json:"vendor"`
			Description         string                   `json:"description"`
			OIDs                []map[string]interface{} `json:"oids"`
			PollIntervalSeconds int                      `json:"poll_interval_seconds"`
			Enabled             *bool                    `json:"enabled"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		update := *existing
		if req.Name != "" {
			update.Name = req.Name
		}
		if req.Vendor != "" {
			update.Vendor = nsStr(req.Vendor)
		}
		if req.Description != "" {
			update.Description = nsStr(req.Description)
		}
		if len(req.OIDs) > 0 {
			oidsJSON, _ := json.Marshal(req.OIDs)
			update.OidsJSON = string(oidsJSON)
		}
		if req.PollIntervalSeconds > 0 {
			update.PollIntervalSeconds = req.PollIntervalSeconds
		}
		if req.Enabled != nil {
			if *req.Enabled {
				update.Enabled = 1
			} else {
				update.Enabled = 0
			}
		}

		if err := repos.SnmpCustomOids.Update(id, update); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// DELETE /custom-oids/:id — 删除自定义 OID 配置
	r.With(middleware.RequireRole("admin")).Delete("/custom-oids/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.SnmpCustomOids.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除自定义 OID 配置失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// GET /custom-oids/results/:deviceId — 查询设备自定义 OID 采集结果
	r.Get("/custom-oids/results/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		profileID := r.URL.Query().Get("profileId")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}

		results, err := repos.SnmpCustomOids.ListResults(deviceID, profileID, limit)
		if err != nil {
			httpx.RespondError(w, "获取自定义 OID 结果失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, results)
	})

	// POST /custom-oids/:id/collect/:deviceId — 手动触发采集
	r.With(middleware.RequireRole("admin")).Post("/custom-oids/{id}/collect/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		profileID := chi.URLParam(r, "id")
		deviceID := chi.URLParam(r, "deviceId")

		profile, err := repos.SnmpCustomOids.GetByID(profileID)
		if err != nil || profile == nil {
			httpx.RespondError(w, "配置不存在", http.StatusNotFound)
			return
		}

		_, ip, derr := repos.NetworkDevices.GetNameAndIP(deviceID)
		if derr != nil || ip == "" {
			httpx.RespondError(w, "设备不存在", http.StatusNotFound)
			return
		}

		cred := services.SnmpServiceInstance.GetCredential(deviceID)
		if cred == nil {
			cred = services.SnmpServiceInstance.GetDefaultCredential()
		}
		if cred == nil {
			httpx.RespondError(w, "未配置 SNMP 凭证", http.StatusBadRequest)
			return
		}

		community := cred.Community
		if community == "" {
			community = "public"
		}
		port := cred.SnmpPort
		if port == 0 {
			port = 161
		}

		// 解析 OID 列表并读取 is_table
		var oidEntries []services.CustomOidEntry
		_ = json.Unmarshal([]byte(profile.OidsJSON), &oidEntries)

		results := services.SnmpServiceInstance.CollectCustomOids(ip, port, services.SnmpVersion(cred.SnmpVersion), community, "", "", "", "", "", oidEntries, 0)

		// 保存结果
		var repoResults []repos.CustomOidResult
		for _, r := range results {
			repoResults = append(repoResults, repos.CustomOidResult{
				DeviceID:    deviceID,
				ProfileID:   profileID,
				MetricName:  r.MetricName,
				MetricValue: r.MetricValue,
				OID:         r.OID,
				SampledAt:   timeNowLocal(),
			})
		}
		_ = repos.SnmpCustomOids.InsertResults(deviceID, profileID, repoResults)

		httpx.Respond(w, results)
	})

	return r
}

// nsStr 辅助：空字符串返回 nil 指针，否则返回指向字符串的指针
func nsStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
