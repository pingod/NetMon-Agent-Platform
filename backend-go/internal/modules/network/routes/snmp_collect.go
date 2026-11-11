// SNMP 采集路由 — 31 个端点
// 等价于 TS routes/snmpCollectRoutes.ts
package routes

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// collectOidInput 兼容两种入参：纯字符串 OID 或 {oid, name} 对象（前端表单格式）
type collectOidInput struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
}

func (e *collectOidInput) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		e.OID = s
		return nil
	}
	var v struct {
		OID  string `json:"oid"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	e.OID, e.Name = v.OID, v.Name
	return nil
}

type collectTaskRequest struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	DeviceType      string            `json:"device_type"`
	DeviceIDs       []string          `json:"device_ids"`
	OIDs            []collectOidInput `json:"oids"`
	ScheduleType    string            `json:"schedule_type"`
	CronExpression  string            `json:"cron_expression"`
	IntervalSeconds int               `json:"interval_seconds"`
	SnmpVersion     string            `json:"snmp_version"`
	Community       string            `json:"community"`
	TimeoutMs       int               `json:"timeout_ms"`
	Retries         int               `json:"retries"`
	Enabled         *bool             `json:"enabled"`
	RetentionDays   int               `json:"retention_days"`
}

func (req *collectTaskRequest) toTask() repos.CollectTask {
	deviceIDsJSON, _ := json.Marshal(req.DeviceIDs)
	oidsJSON, _ := json.Marshal(req.OIDs)
	if req.SnmpVersion == "" {
		req.SnmpVersion = "v2c"
	}
	if req.Community == "" {
		req.Community = "public"
	}
	if req.ScheduleType == "" {
		req.ScheduleType = "manual"
	}
	return repos.CollectTask{
		Name:            req.Name,
		Description:     &req.Description,
		DeviceType:      &req.DeviceType,
		DeviceIDs:       string(deviceIDsJSON),
		OIDs:            string(oidsJSON),
		ScheduleType:    req.ScheduleType,
		CronExpression:  &req.CronExpression,
		IntervalSeconds: req.IntervalSeconds,
		SnmpVersion:     req.SnmpVersion,
		Community:       &req.Community,
		TimeoutMs:       req.TimeoutMs,
		Retries:         req.Retries,
		RetentionDays:   req.RetentionDays,
	}
}

// SnmpCollectRoutes SNMP 采集路由
func SnmpCollectRoutes() chi.Router {
	r := chi.NewRouter()

	// ============================================================
	// MIB 模块管理
	// ============================================================

	// GET / — 采集概览
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		var modCount, taskCount int
		appdb.DBForTable("mib_modules").QueryRow(`SELECT COUNT(*) FROM mib_modules`).Scan(&modCount)
		appdb.DBForTable("snmp_collect_tasks").QueryRow(`SELECT COUNT(*) FROM snmp_collect_tasks`).Scan(&taskCount)
		httpx.Respond(w, map[string]interface{}{
			"mib_modules_count": modCount,
			"tasks_count":       taskCount,
		})
	})

	// GET /mib-modules & GET /mib/modules — 获取 MIB 模块列表
	handleMibModules := func(w http.ResponseWriter, r *http.Request) {
		modules, err := repos.MibModules.List()
		if err != nil {
			httpx.RespondError(w, "获取MIB模块列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, modules)
	}
	r.Get("/mib-modules", handleMibModules)
	r.Get("/mib/modules", handleMibModules)

	// GET /custom-metrics & GET /metrics/all — 自定义指标列表
	handleCustomMetrics := func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DBForTable("custom_dashboard_metrics").Query(`SELECT id, custom_name, oid, device_id, unit, chart_type FROM custom_dashboard_metrics ORDER BY display_order ASC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, name, oid string
			var devID, unit, chartType sql.NullString
			if rows.Scan(&id, &name, &oid, &devID, &unit, &chartType) == nil {
				list = append(list, map[string]interface{}{
					"id":          id,
					"custom_name": name,
					"oid":         oid,
					"device_id":   devID.String,
					"unit":        unit.String,
					"chart_type":  chartType.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	}
	r.Get("/custom-metrics", handleCustomMetrics)
	r.Get("/metrics/all", handleCustomMetrics)

	// POST /mib-modules/upload — 上传并解析 MIB/Excel/ZIP 文件
	r.With(middleware.RequireRole("admin")).Post("/mib-modules/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			httpx.RespondError(w, "解析上传表单失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			files = r.MultipartForm.File["file"]
		}

		if len(files) == 0 {
			httpx.RespondError(w, "未接收到上传的文件", http.StatusBadRequest)
			return
		}

		resData, err := services.ProcessUploadedMibFiles(files)
		if err != nil {
			httpx.RespondError(w, "处理上传文件失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		httpx.Respond(w, resData)
	})

	// DELETE /mib-modules/:id — 删除 MIB 模块
	r.With(middleware.RequireRole("admin")).Delete("/mib-modules/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.MibModules.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除MIB模块失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "MIB模块已删除")
	})

	// POST /mib-modules/batch-delete — 批量删除 MIB 模块
	r.With(middleware.RequireRole("admin")).Post("/mib-modules/batch-delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.IDs) == 0 {
			httpx.RespondError(w, "请提供要删除的模块ID列表", http.StatusBadRequest)
			return
		}

		deleted := 0
		for _, id := range req.IDs {
			if err := repos.MibModules.Delete(id); err == nil {
				deleted++
			}
		}
		httpx.Respond(w, map[string]interface{}{"deleted": deleted, "total": len(req.IDs)})
	})

	// GET /mib-modules/:id/objects — 获取 MIB 模块的对象列表
	r.Get("/mib-modules/{id}/objects", func(w http.ResponseWriter, r *http.Request) {
		objects, err := repos.MibObjects.ListByModuleID(chi.URLParam(r, "id"))
		if err != nil {
			httpx.RespondError(w, "获取MIB对象列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, objects)
	})

	// POST /mib-modules/batch-objects — 批量获取多个 MIB 模块的对象列表
	r.Post("/mib-modules/batch-objects", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ModuleIDs []string `json:"module_ids"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.ModuleIDs) == 0 {
			httpx.Respond(w, []interface{}{})
			return
		}

		objects, err := repos.MibObjects.ListByModuleIDs(req.ModuleIDs)
		if err != nil {
			httpx.RespondError(w, "批量获取MIB对象列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, objects)
	})

	// GET /mib-objects/search — 搜索 MIB 对象
	r.Get("/mib-objects/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			httpx.Respond(w, []interface{}{})
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}

		objects, err := repos.MibObjects.Search(q, limit)
		if err != nil {
			httpx.RespondError(w, "搜索MIB对象失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, objects)
	})

	// GET /mib-objects/categories — 获取分类元数据
	r.Get("/mib-objects/categories", func(w http.ResponseWriter, r *http.Request) {
		// Go 版本暂无 mibClassifier，返回空
		httpx.Respond(w, []interface{}{})
	})

	// GET /mib-objects/classified — 获取已分类的 MIB 对象列表
	r.Get("/mib-objects/classified", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		moduleID := q.Get("module_id")
		searchQ := q.Get("q")
		category := q.Get("category")
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 500
		}

		var objects []repos.MibObject
		var err error

		if moduleID != "" {
			objects, err = repos.MibObjects.ListByModuleID(moduleID)
		} else if searchQ != "" {
			objects, err = repos.MibObjects.Search(searchQ, limit)
		} else {
			// 返回最多 2000 个对象
			objects, err = repos.MibObjects.Search("", 2000)
		}

		if err != nil {
			httpx.RespondError(w, "获取分类MIB对象列表失败", http.StatusInternalServerError)
			return
		}

		// 按分类筛选（简化：Go 版本暂不实现分类注入，直接返回原始列表）
		_ = category // TODO: classifyMibObjects
		httpx.Respond(w, objects)
	})

	// GET /mib-objects/hardware — 获取高频硬件指标对象 (CPU/Memory/Fan/Power/Optical)
	r.Get("/mib-objects/hardware", func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("category")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 1000 {
			limit = 500
		}

		cpuCond := `(
			LOWER(obj.name) LIKE '%cpu%' OR LOWER(obj.name) LIKE '%processor%' OR LOWER(obj.name) LIKE '%cpuload%' OR LOWER(obj.name) LIKE '%cpuratio%' OR LOWER(obj.name) LIKE '%cpuutil%' OR
			LOWER(obj.description) LIKE '%cpu%' OR obj.description LIKE '%处理器%' OR obj.description LIKE '%CPU%'
		) AND LOWER(obj.name) NOT LIKE 'lldp%'`

		memCond := `(
			LOWER(obj.name) LIKE '%memory%' OR LOWER(obj.name) LIKE '%memusage%' OR LOWER(obj.name) LIKE '%memutil%' OR LOWER(obj.name) LIKE '%memsize%' OR LOWER(obj.name) LIKE '%memfree%' OR LOWER(obj.name) LIKE '%memused%' OR LOWER(obj.name) LIKE '%memtotal%' OR LOWER(obj.name) LIKE '%memratio%' OR LOWER(obj.name) LIKE '%ram%' OR LOWER(obj.name) LIKE '%flash%' OR LOWER(obj.name) LIKE '%buffer%' OR LOWER(obj.name) LIKE '%zxanmem%' OR LOWER(obj.name) LIKE '%zxr10mem%' OR obj.description LIKE '%内存%' OR LOWER(obj.description) LIKE '%memory%'
		) AND LOWER(obj.name) NOT LIKE 'lldp%' AND LOWER(obj.name) NOT LIKE '%member%' AND LOWER(obj.name) NOT LIKE '%frame%' AND LOWER(obj.name) NOT LIKE '%element%'`

		fanCond := `(
			LOWER(obj.name) LIKE '%fan%' OR LOWER(obj.name) LIKE '%wind%' OR LOWER(obj.name) LIKE '%cooler%' OR LOWER(obj.name) LIKE '%shelffan%' OR LOWER(obj.name) LIKE '%rackfan%' OR LOWER(obj.name) LIKE '%envfan%' OR LOWER(obj.description) LIKE '%fan%' OR obj.description LIKE '%风扇%'
		) AND LOWER(obj.name) NOT LIKE 'lldp%'`

		powerCond := `(
			LOWER(obj.name) LIKE '%power%' OR LOWER(obj.name) LIKE '%psu%' OR LOWER(obj.name) LIKE '%voltage%' OR LOWER(obj.name) LIKE '%current%' OR LOWER(obj.name) LIKE '%powervoltage%' OR LOWER(obj.name) LIKE '%powerstatus%' OR LOWER(obj.description) LIKE '%power%' OR obj.description LIKE '%电源%' OR obj.description LIKE '%电压%' OR obj.description LIKE '%供电%'
		) AND LOWER(obj.name) NOT LIKE 'lldp%' AND LOWER(obj.name) NOT LIKE '%powerof2%'`

		tempCond := `(
			LOWER(obj.name) LIKE '%temp%' OR LOWER(obj.name) LIKE '%temperature%' OR LOWER(obj.name) LIKE '%thermal%' OR LOWER(obj.description) LIKE '%temperature%' OR obj.description LIKE '%温度%' OR obj.description LIKE '%摄氏度%'
		) AND LOWER(obj.name) NOT LIKE 'lldp%' AND LOWER(obj.description) NOT LIKE '%attempt%' AND LOWER(obj.description) NOT LIKE '%template%'`

		opticalCond := `(
			LOWER(obj.name) LIKE '%optical%' OR LOWER(obj.name) LIKE '%optic%' OR LOWER(obj.name) LIKE '%sfp%' OR LOWER(obj.name) LIKE '%qsfp%' OR LOWER(obj.name) LIKE '%transceiver%' OR LOWER(obj.name) LIKE '%txpower%' OR LOWER(obj.name) LIKE '%rxpower%' OR LOWER(obj.name) LIKE '%bias%' OR LOWER(obj.name) LIKE '%laser%' OR LOWER(obj.description) LIKE '%optical%' OR obj.description LIKE '%光模块%' OR obj.description LIKE '%光功率%' OR obj.description LIKE '%收光%' OR obj.description LIKE '%发光%'
		) AND LOWER(obj.name) NOT LIKE 'lldp%'`

		querySQL := `
			SELECT obj.id, obj.module_id, obj.module_name, obj.oid, obj.name, obj.syntax, obj.access, obj.status, obj.description, obj.parent_oid, obj.table_name, obj.index_columns, obj.is_table, obj.is_scalar
			FROM mib_objects obj
			WHERE (` + cpuCond + ` OR ` + memCond + ` OR ` + fanCond + ` OR ` + powerCond + ` OR ` + tempCond + ` OR ` + opticalCond + `)`

		if category != "" && category != "all" {
			switch category {
			case "cpu":
				querySQL += ` AND ` + cpuCond
			case "memory":
				querySQL += ` AND ` + memCond
			case "fan":
				querySQL += ` AND ` + fanCond
			case "power":
				querySQL += ` AND ` + powerCond
			case "temp", "temperature":
				querySQL += ` AND ` + tempCond
			case "optical":
				querySQL += ` AND ` + opticalCond
			}
		}

		querySQL += ` ORDER BY obj.oid ASC LIMIT ?`

		rows, err := appdb.OidDB().Query(querySQL, limit)
		if err != nil {
			httpx.RespondError(w, "查询硬件指标失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var objects []repos.MibObject
		for rows.Next() {
			var o repos.MibObject
			var syntax, access, status, desc, pOid, tName, idxCols sql.NullString
			if err := rows.Scan(&o.ID, &o.ModuleID, &o.ModuleName, &o.OID, &o.Name, &syntax, &access, &status, &desc, &pOid, &tName, &idxCols, &o.IsTable, &o.IsScalar); err == nil {
				o.Syntax = nullStrToPtr(syntax)
				o.Access = nullStrToPtr(access)
				o.Status = nullStrToPtr(status)
				o.Description = nullStrToPtr(desc)
				o.ParentOID = nullStrToPtr(pOid)
				o.TableName = nullStrToPtr(tName)
				o.IndexColumns = nullStrToPtr(idxCols)
				objects = append(objects, o)
			}
		}

		httpx.Respond(w, objects)
	})

	// ============================================================
	// 快速采集测试
	// ============================================================

	// POST /quick-test — 快速采集测试
	r.With(middleware.RequireRole("admin")).Post("/quick-test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceIDs        []string                 `json:"device_ids"`
			OIDs             []map[string]interface{} `json:"oids"`
			SnmpVersion      string                   `json:"snmp_version"`       // 可选覆盖
			Community        string                   `json:"community"`          // 可选覆盖
			SnmpUser         string                   `json:"snmp_user"`          // v3 用户名
			SnmpAuthProtocol string                   `json:"snmp_auth_protocol"` // v3 认证协议
			SnmpAuthKey      string                   `json:"snmp_auth_key"`      // v3 认证密码
			SnmpPrivProtocol string                   `json:"snmp_priv_protocol"` // v3 加密协议
			SnmpPrivKey      string                   `json:"snmp_priv_key"`      // v3 加密密码
			SnmpCredentialID string                   `json:"snmp_credential_id"` // 可选指定凭证ID
			TimeoutMs        int                      `json:"timeout_ms"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.DeviceIDs) == 0 {
			httpx.RespondError(w, "请选择至少一台设备", http.StatusBadRequest)
			return
		}
		if len(req.OIDs) == 0 {
			httpx.RespondError(w, "请提供至少一个 OID", http.StatusBadRequest)
			return
		}

		type deviceResult struct {
			DeviceID   string                   `json:"deviceId"`
			DeviceName string                   `json:"deviceName,omitempty"`
			DeviceIP   string                   `json:"deviceIp,omitempty"`
			Status     string                   `json:"status"`
			DurationMs int64                    `json:"durationMs"`
			Metrics    []map[string]interface{} `json:"metrics"`
			Error      string                   `json:"error,omitempty"`
		}
		var results []deviceResult

		// 若指定了全局凭证 ID，预先加载解密
		var explicitCred *services.SnmpCredential
		if req.SnmpCredentialID != "" {
			if row, err := repos.SnmpCredentials.GetByID(req.SnmpCredentialID); err == nil && row != nil {
				explicitCred = services.SnmpServiceInstance.GetCredential(row.ID)
			}
		}

		for _, deviceID := range req.DeviceIDs {
			device, err := repos.NetworkDevices.GetByID(deviceID)
			if err != nil || device == nil {
				results = append(results, deviceResult{
					DeviceID:   deviceID,
					DeviceName: deviceID,
					Status:     "failed",
					Error:      "设备不存在",
				})
				continue
			}

			// 1. 从设备关联凭证或显式指定凭证读取（自动解密），前端传的 community/version/v3 作为可选覆盖
			var (
				snmpPort                                           int
				snmpVersion                                        services.SnmpVersion
				community                                          string
				user, authProtocol, authKey, privProtocol, privKey string
			)

			cred := explicitCred
			if cred == nil {
				cred = services.SnmpServiceInstance.GetCredential(deviceID)
			}
			if cred != nil {
				snmpPort = cred.SnmpPort
				snmpVersion = cred.SnmpVersion
				community = cred.Community
				user = cred.SnmpUser
				authProtocol = cred.SnmpAuthProtocol
				authKey = cred.SnmpAuthKey
				privProtocol = cred.SnmpPrivProtocol
				privKey = cred.SnmpPrivKey
			}

			// 前端覆盖：当前端显式传了 community 或 snmp_version 或 v3 参数，优先使用
			if req.Community != "" {
				community = req.Community
			}
			if req.SnmpVersion != "" {
				snmpVersion = services.SnmpVersion(req.SnmpVersion)
			}
			if req.SnmpUser != "" {
				user = req.SnmpUser
			}
			if req.SnmpAuthProtocol != "" {
				authProtocol = req.SnmpAuthProtocol
			}
			if req.SnmpAuthKey != "" {
				authKey = req.SnmpAuthKey
			}
			if req.SnmpPrivProtocol != "" {
				privProtocol = req.SnmpPrivProtocol
			}
			if req.SnmpPrivKey != "" {
				privKey = req.SnmpPrivKey
			}

			// 回退默认值
			if snmpPort == 0 {
				snmpPort = 161
			}
			if snmpVersion == "" {
				snmpVersion = services.SNMPv2c
			}
			if community == "" {
				community = "public"
			}

			// 2. 构建 OID 列表，严格根据 is_table / syntax (COLUMNAR vs SCALAR) 进行区分
			var oidEntries []services.CustomOidEntry
			for _, o := range req.OIDs {
				oid, _ := o["oid"].(string)
				name, _ := o["metric_name"].(string)
				isTable := 0
				if v, ok := o["is_table"]; ok {
					switch iv := v.(type) {
					case float64:
						isTable = int(iv)
					case int:
						isTable = iv
					}
				}
				// 若前端未显式传递 is_table，从 OID 库实时查询 is_table / syntax 是否为 COLUMNAR
				if isTable == 0 && oid != "" {
					var dbIsTable int
					_ = appdb.OidDB().QueryRow(`
						SELECT CASE 
							WHEN is_table = 1 THEN 1 
							WHEN LOWER(syntax) LIKE '%column%' OR LOWER(syntax) LIKE '%table%' THEN 1 
							ELSE 0 
						END 
						FROM mib_objects WHERE oid = ? LIMIT 1`, oid).Scan(&dbIsTable)
					if dbIsTable == 1 {
						isTable = 1
					}
				}
				if oid != "" {
					oidEntries = append(oidEntries, services.CustomOidEntry{OID: oid, MetricName: name, IsTable: isTable})
				}
			}

			// 3. 执行采集
			startTime := time.Now()
			snmpResults := services.SnmpServiceInstance.CollectCustomOids(
				device.IPAddress, snmpPort, snmpVersion, community,
				user, authProtocol, authKey, privProtocol, privKey,
				oidEntries, req.TimeoutMs,
			)
			duration := time.Since(startTime).Milliseconds()

			var metrics []map[string]interface{}
			var deviceErr string
			hasSuccessVal := false
			for _, m := range snmpResults {
				entry := map[string]interface{}{
					"oid":          m.OID,
					"metric_name":  m.MetricName,
					"metric_value": m.MetricValue,
				}
				if m.Error != "" {
					entry["error"] = m.Error
					deviceErr = m.Error
				} else if m.MetricValue != "" && m.MetricValue != "N/A" {
					hasSuccessVal = true
				}
				metrics = append(metrics, entry)
			}

			status := "failed"
			if deviceErr == "" && (hasSuccessVal || len(snmpResults) > 0) {
				status = "success"
			}

			results = append(results, deviceResult{
				DeviceID:   deviceID,
				DeviceName: device.Name,
				DeviceIP:   device.IPAddress,
				Status:     status,
				DurationMs: duration,
				Metrics:    metrics,
				Error:      deviceErr,
			})
		}

		httpx.Respond(w, results)
	})

	// ============================================================
	// 采集任务管理
	// ============================================================

	// GET /tasks — 获取采集任务列表
	r.Get("/tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := repos.CollectTasks.List()
		if err != nil {
			httpx.RespondError(w, "获取采集任务列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, tasks)
	})

	// GET /tasks/:id — 获取单个采集任务
	r.Get("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		task, err := repos.CollectTasks.GetByID(chi.URLParam(r, "id"))
		if err != nil || task == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, task)
	})

	// POST /tasks — 创建采集任务
	r.With(middleware.RequireRole("admin")).Post("/tasks", func(w http.ResponseWriter, r *http.Request) {
		var req collectTaskRequest
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			httpx.RespondError(w, "任务名称不能为空", http.StatusBadRequest)
			return
		}

		task := req.toTask()
		task.ID = uuid.NewString()
		task.Enabled = 1
		if req.Enabled != nil && !*req.Enabled {
			task.Enabled = 0
		}

		if err := repos.CollectTasks.Create(task); err != nil {
			httpx.RespondError(w, "创建采集任务失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, task)
	})

	// PUT /tasks/:id — 更新采集任务
	r.With(middleware.RequireRole("admin")).Put("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		existing, err := repos.CollectTasks.GetByID(id)
		if err != nil || existing == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		var req collectTaskRequest
		if !parseJSON(w, r, &req) {
			return
		}
		task := req.toTask()
		task.ID = id
		task.Enabled = existing.Enabled
		if req.Enabled != nil {
			task.Enabled = 0
			if *req.Enabled {
				task.Enabled = 1
			}
		}
		if err := repos.CollectTasks.Update(id, task); err != nil {
			httpx.RespondError(w, "更新采集任务失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// DELETE /tasks/:id — 删除采集任务
	r.With(middleware.RequireRole("admin")).Delete("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.CollectTasks.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除采集任务失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "任务已删除")
	})

	// POST /tasks/:id/toggle — 切换任务启用状态
	r.With(middleware.RequireRole("admin")).Post("/tasks/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		task, err := repos.CollectTasks.GetByID(chi.URLParam(r, "id"))
		if err != nil || task == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}

		newEnabled := 1
		if task.Enabled == 1 {
			newEnabled = 0
		}
		task.Enabled = newEnabled
		if updateErr := repos.CollectTasks.Update(chi.URLParam(r, "id"), *task); updateErr != nil {
			httpx.RespondError(w, "切换采集任务状态失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, task)
	})

	// POST /tasks/:id/execute — 手动执行采集任务（真实采集并入库）
	r.With(middleware.RequireRole("admin")).Post("/tasks/{id}/execute", func(w http.ResponseWriter, r *http.Request) {
		task, err := repos.CollectTasks.GetByID(chi.URLParam(r, "id"))
		if err != nil || task == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		summary := services.ExecuteCollectTask(task)
		httpx.Respond(w, summary)
	})

	// ============================================================
	// 采集结果查询
	// ============================================================

	// GET /results — 获取采集结果
	r.Get("/results", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		if offset < 0 {
			offset = 0
		}

		// 仓库层直接支持 limit/offset，offset 不再被丢弃
		results, total, err := repos.CollectResults.ListByOffset(limit, offset, q.Get("task_id"), q.Get("device_id"))
		if err != nil {
			httpx.RespondError(w, "获取采集结果失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"results": results, "total": total, "limit": limit, "offset": offset})
	})

	// GET /results/latest — 获取设备最新采集数据
	r.Get("/results/latest", func(w http.ResponseWriter, r *http.Request) {
		// 简化：直接返回查询结果
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 50
		}

		results, _, err := repos.CollectResults.List(1, limit, q.Get("task_id"), q.Get("device_id"))
		if err != nil {
			httpx.RespondError(w, "获取最新采集数据失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, results)
	})

	// DELETE /results/:id — 删除采集结果
	r.With(middleware.RequireRole("admin")).Delete("/results/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.CollectResults.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除采集结果失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "记录已删除")
	})

	// ============================================================
	// 自定义监控面板指标
	// ============================================================

	// GET /dashboard-metrics — 获取面板指标列表
	r.Get("/dashboard-metrics", func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.URL.Query().Get("device_id")
		dashboardOnly := r.URL.Query().Get("dashboard_only")

		var metrics []repos.CustomDashboardMetric
		var err error

		if deviceID != "" {
			metrics, err = repos.DashboardMetrics.ListByDevice(deviceID)
		} else if dashboardOnly == "true" {
			metrics, err = repos.DashboardMetrics.ListForDashboard()
		} else {
			metrics, err = repos.DashboardMetrics.List()
		}

		if err != nil {
			httpx.RespondError(w, "获取面板指标失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, metrics)
	})

	// POST /dashboard-metrics — 创建面板指标
	r.Post("/dashboard-metrics", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OID           string `json:"oid"`
			CustomName    string `json:"custom_name"`
			OriginalName  string `json:"original_name"`
			DeviceID      string `json:"device_id"`
			ModuleName    string `json:"module_name"`
			Syntax        string `json:"syntax"`
			Unit          string `json:"unit"`
			SnmpVersion   string `json:"snmp_version"`
			Community     string `json:"community"`
			ValueMappings string `json:"value_mappings"`
			IsTable       int    `json:"is_table"`
			ChartType     string `json:"chart_type"`
			MetricName    string `json:"metric_name"`
			Tags          string `json:"tags"`
			LastValue     string `json:"last_value"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.OID == "" || req.CustomName == "" || req.DeviceID == "" {
			httpx.RespondError(w, "oid、custom_name 和 device_id 为必填", http.StatusBadRequest)
			return
		}

		metric, err := repos.DashboardMetrics.Create(repos.MetricCreateInput{
			OID:           req.OID,
			CustomName:    req.CustomName,
			OriginalName:  req.OriginalName,
			DeviceID:      req.DeviceID,
			ModuleName:    req.ModuleName,
			Syntax:        req.Syntax,
			Unit:          req.Unit,
			SnmpVersion:   req.SnmpVersion,
			Community:     req.Community,
			ValueMappings: req.ValueMappings,
			IsTable:       req.IsTable,
			ChartType:     req.ChartType,
			MetricName:    req.MetricName,
			Tags:          req.Tags,
			LastValue:     req.LastValue,
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, metric)
	})

	// POST /dashboard-metrics/batch — 批量创建面板指标
	r.Post("/dashboard-metrics/batch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Metrics []repos.MetricCreateInput `json:"metrics"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.Metrics) == 0 {
			httpx.RespondError(w, "metrics 数组不能为空", http.StatusBadRequest)
			return
		}
		for _, m := range req.Metrics {
			if m.OID == "" || m.CustomName == "" || m.DeviceID == "" {
				httpx.RespondError(w, "每条指标必须包含 oid、custom_name 和 device_id", http.StatusBadRequest)
				return
			}
		}

		results, err := repos.DashboardMetrics.BatchCreate(req.Metrics)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 针对复用的新设备同步/并行采样并即时更新 LastValue，保证大屏首次展示即有真实数据
		var wg sync.WaitGroup
		for i := range results {
			if results[i].LastValue == nil || *results[i].LastValue == "" {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					m := results[idx]
					device, err := repos.NetworkDevices.GetByID(m.DeviceID)
					if err != nil || device == nil {
						return
					}

					var (
						community                                          string
						version                                            string
						snmpPort                                           int
						user, authProtocol, authKey, privProtocol, privKey string
					)

					cred := services.SnmpServiceInstance.GetCredential(m.DeviceID)
					if cred != nil {
						community = cred.Community
						version = string(cred.SnmpVersion)
						snmpPort = cred.SnmpPort
						user = cred.SnmpUser
						authProtocol = cred.SnmpAuthProtocol
						authKey = cred.SnmpAuthKey
						privProtocol = cred.SnmpPrivProtocol
						privKey = cred.SnmpPrivKey
					}

					if community == "" && m.Community != "" {
						community = m.Community
					}
					if community == "" {
						community = "public"
					}
					if version == "" && m.SnmpVersion != "" {
						version = m.SnmpVersion
					}
					if version == "" {
						version = "v2c"
					}
					if snmpPort == 0 {
						snmpPort = 161
					}

					oidEntries := []services.CustomOidEntry{{OID: m.OID, MetricName: m.CustomName, IsTable: m.IsTable}}
					// 快采样 1500ms 超时
					resList := services.SnmpServiceInstance.CollectCustomOids(
						device.IPAddress, snmpPort, services.SnmpVersion(version), community,
						user, authProtocol, authKey, privProtocol, privKey, oidEntries, 1500)

					if len(resList) > 0 && resList[0].MetricValue != "" && resList[0].MetricValue != "N/A" {
						val := resList[0].MetricValue
						_ = repos.DashboardMetrics.UpdateValue(m.ID, val)
						results[idx].LastValue = &val
					}
				}(i)
			}
		}
		wg.Wait()

		httpx.Respond(w, results)
	})

	// PUT /dashboard-metrics/:id — 更新面板指标
	r.Put("/dashboard-metrics/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OID             *string `json:"oid"`
			CustomName      *string `json:"custom_name"`
			Unit            *string `json:"unit"`
			ShowOnDashboard *int    `json:"show_on_dashboard"`
			DisplayOrder    *int    `json:"display_order"`
			ValueMappings   *string `json:"value_mappings"`
			IsTable         *int    `json:"is_table"`
			ChartType       *string `json:"chart_type"`
			MetricName      *string `json:"metric_name"`
			Tags            *string `json:"tags"`
			LastValue       *string `json:"last_value"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		err := repos.DashboardMetrics.Update(chi.URLParam(r, "id"), repos.MetricUpdateInput{
			OID:             req.OID,
			CustomName:      req.CustomName,
			Unit:            req.Unit,
			ShowOnDashboard: req.ShowOnDashboard,
			DisplayOrder:    req.DisplayOrder,
			ValueMappings:   req.ValueMappings,
			IsTable:         req.IsTable,
			ChartType:       req.ChartType,
			MetricName:      req.MetricName,
			Tags:            req.Tags,
			LastValue:       req.LastValue,
		})
		if err != nil {
			httpx.RespondError(w, "更新面板指标失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// DELETE /dashboard-metrics/:id — 删除面板指标
	r.Delete("/dashboard-metrics/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.DashboardMetrics.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除面板指标失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// POST /dashboard-metrics/refresh — 并行刷新面板指标值
	r.Post("/dashboard-metrics/refresh", func(w http.ResponseWriter, r *http.Request) {
		metrics, err := repos.DashboardMetrics.ListForDashboard()
		if err != nil || len(metrics) == 0 {
			httpx.Respond(w, []interface{}{})
			return
		}

		var wg sync.WaitGroup
		for _, m := range metrics {
			wg.Add(1)
			go func(m repos.CustomDashboardMetric) {
				defer wg.Done()
				device, err := repos.NetworkDevices.GetByID(m.DeviceID)
				if err != nil || device == nil {
					return
				}

				// 从设备凭证读取连接参数
				var (
					community                                          string
					version                                            string
					snmpPort                                           int
					user, authProtocol, authKey, privProtocol, privKey string
				)

				cred := services.SnmpServiceInstance.GetCredential(m.DeviceID)
				if cred != nil {
					community = cred.Community
					version = string(cred.SnmpVersion)
					snmpPort = cred.SnmpPort
					user = cred.SnmpUser
					authProtocol = cred.SnmpAuthProtocol
					authKey = cred.SnmpAuthKey
					privProtocol = cred.SnmpPrivProtocol
					privKey = cred.SnmpPrivKey
				}

				// 指标自身存储的 community/version 作为备选
				if community == "" && m.Community != "" {
					community = m.Community
				}
				if community == "" {
					community = "public"
				}
				if version == "" && m.SnmpVersion != "" {
					version = m.SnmpVersion
				}
				if version == "" {
					version = "v2c"
				}
				if snmpPort == 0 {
					snmpPort = 161
				}

				isTable := 0
				if m.IsTable == 1 {
					isTable = 1
				}

				oidEntries := []services.CustomOidEntry{{OID: m.OID, MetricName: m.CustomName, IsTable: isTable}}
				// 设置 2000ms 短超时，离线设备快速失败不阻塞
				results := services.SnmpServiceInstance.CollectCustomOids(
					device.IPAddress, snmpPort, services.SnmpVersion(version), community,
					user, authProtocol, authKey, privProtocol, privKey, oidEntries, 2000)

				if len(results) > 0 && results[0].MetricValue != "" && results[0].MetricValue != "N/A" {
					_ = repos.DashboardMetrics.UpdateValue(m.ID, results[0].MetricValue)
				}
			}(m)
		}
		wg.Wait()

		refreshed, _ := repos.DashboardMetrics.ListForDashboard()
		httpx.Respond(w, refreshed)
	})

	// ============================================================
	// 自定义列表面板
	// ============================================================

	// GET /dashboard-lists — 获取所有列表面板
	r.Get("/dashboard-lists", func(w http.ResponseWriter, r *http.Request) {
		dashboardOnly := r.URL.Query().Get("dashboard_only")

		var lists []repos.CustomDashboardList
		var err error

		if dashboardOnly == "true" {
			lists, err = repos.DashboardLists.ListForDashboard()
		} else {
			lists, err = repos.DashboardLists.List()
		}

		if err != nil {
			httpx.RespondError(w, "获取列表面板失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, lists)
	})

	// POST /dashboard-lists — 创建列表面板
	r.With(middleware.RequireRole("admin")).Post("/dashboard-lists", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name            string             `json:"name"`
			Description     string             `json:"description"`
			DeviceIDs       []string           `json:"device_ids"`
			Columns         []repos.ListColumn `json:"columns"`
			RefreshInterval int                `json:"refresh_interval"`
			ShowOnDashboard int                `json:"show_on_dashboard"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || len(req.DeviceIDs) == 0 || len(req.Columns) == 0 {
			httpx.RespondError(w, "name、device_ids 和 columns 为必填", http.StatusBadRequest)
			return
		}

		list, err := repos.DashboardLists.Create(repos.ListCreateInput{
			Name:            req.Name,
			Description:     req.Description,
			DeviceIDs:       req.DeviceIDs,
			Columns:         req.Columns,
			RefreshInterval: req.RefreshInterval,
			ShowOnDashboard: req.ShowOnDashboard,
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, list)
	})

	// PUT /dashboard-lists/:id — 更新列表面板
	r.With(middleware.RequireRole("admin")).Put("/dashboard-lists/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name            *string             `json:"name"`
			Description     *string             `json:"description"`
			DeviceIDs       *[]string           `json:"device_ids"`
			Columns         *[]repos.ListColumn `json:"columns"`
			RefreshInterval *int                `json:"refresh_interval"`
			ShowOnDashboard *int                `json:"show_on_dashboard"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		err := repos.DashboardLists.Update(chi.URLParam(r, "id"), repos.ListUpdateInput{
			Name:            req.Name,
			Description:     req.Description,
			DeviceIDs:       req.DeviceIDs,
			Columns:         req.Columns,
			RefreshInterval: req.RefreshInterval,
			ShowOnDashboard: req.ShowOnDashboard,
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// DELETE /dashboard-lists/:id — 删除列表面板
	r.With(middleware.RequireRole("admin")).Delete("/dashboard-lists/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.DashboardLists.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, "删除列表面板失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// POST /dashboard-lists/:id/collect — 采集列表面板数据（真实 SNMP 采集）
	r.Post("/dashboard-lists/{id}/collect", func(w http.ResponseWriter, r *http.Request) {
		list, err := repos.DashboardLists.GetByID(chi.URLParam(r, "id"))
		if err != nil || list == nil {
			httpx.RespondError(w, "列表面板不存在", http.StatusNotFound)
			return
		}

		sampledAt := timeNowLocal()
		type cellData struct {
			Value   string `json:"value"`
			Display string `json:"display"`
		}
		type rowData struct {
			DeviceID   string               `json:"deviceId"`
			DeviceName string               `json:"deviceName"`
			DeviceIP   string               `json:"deviceIp"`
			RowIndex   string               `json:"rowIndex"`
			Cells      map[string]*cellData `json:"cells"`
		}

		var rows []rowData

		// 填充标签列单元格
		fillLabel := func(cells map[string]*cellData, col repos.ListColumn, dev *repos.NetworkDevice) {
			var val string
			switch col.Field {
			case "device_ip":
				val = dev.IPAddress
			case "device_name":
				val = dev.Name
			case "device_vendor":
				val = dev.Vendor
			case "device_model":
				if dev.Model != nil {
					val = *dev.Model
				}
			case "device_location":
				if dev.Location != nil {
					val = *dev.Location
				}
			default:
				val = "-"
			}
			if val == "" {
				val = "-"
			}
			cells[col.ID] = &cellData{Value: val, Display: val}
		}

		// 并行对关联设备采集
		type devResult struct {
			index int
			rows  []rowData
		}
		resCh := make(chan devResult, len(list.DeviceIDs))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 16) // 最多 16 并发设备采集

		for i, dID := range list.DeviceIDs {
			wg.Add(1)
			go func(idx int, deviceID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				device, err := repos.NetworkDevices.GetByID(deviceID)
				if err != nil || device == nil {
					return
				}

				var devRows []rowData

				// 获取 SNMP 凭证
				cred := services.SnmpServiceInstance.GetCredential(deviceID)
				if cred == nil {
					// 无凭证，仅填充标签列
					row := rowData{
						DeviceID:   deviceID,
						DeviceName: device.Name,
						DeviceIP:   device.IPAddress,
						RowIndex:   "_device",
						Cells:      make(map[string]*cellData),
					}
					for _, col := range list.Columns {
						if col.Type == "label" {
							fillLabel(row.Cells, col, device)
						}
					}
					devRows = append(devRows, row)
					resCh <- devResult{index: idx, rows: devRows}
					return
				}

				// 收集标量 OID 和表列 OID
				scalarOIDCols := map[string]*repos.ListColumn{}
				tableOIDCols := map[string]*repos.ListColumn{}
				for i := range list.Columns {
					col := &list.Columns[i]
					if col.Type != "metric" || col.OID == "" {
						continue
					}
					if col.IsTable == 1 {
						tableOIDCols[col.OID] = col
					} else {
						scalarOIDCols[col.OID] = col
					}
				}

				// 采集标量数据
				scalarResults := map[string]string{}
				if len(scalarOIDCols) > 0 {
					oids := make([]string, 0, len(scalarOIDCols))
					for oid := range scalarOIDCols {
						oids = append(oids, oid)
					}
					results := services.SnmpServiceInstance.SnmpGetMultiple(
						device.IPAddress, cred.SnmpPort, cred.SnmpVersion, cred.Community,
						cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
						cred.SnmpPrivProtocol, cred.SnmpPrivKey, oids)
					// 按 OID 回填（不依赖返回顺序，避免批量 GET 乱序导致指标错位）
					for _, res := range results {
						if res == nil {
							continue
						}
						scalarResults[res.OID] = res.Value
						scalarResults[strings.TrimPrefix(res.OID, ".")] = res.Value
					}
				}

				// 采集表列数据
				tableResults := map[string]map[string]string{}
				for oid := range tableOIDCols {
					results := services.SnmpServiceInstance.SnmpWalk(
						device.IPAddress, cred.SnmpPort, cred.SnmpVersion, cred.Community,
						cred.SnmpUser, cred.SnmpAuthProtocol, cred.SnmpAuthKey,
						cred.SnmpPrivProtocol, cred.SnmpPrivKey, oid, 0)
					idxMap := make(map[string]string)
					for _, r := range results {
						if r != nil {
							suffix := strings.TrimPrefix(r.OID, oid)
							suffix = strings.TrimPrefix(suffix, ".")
							idxMap[suffix] = r.Value
						}
					}
					tableResults[oid] = idxMap
				}

				// 如果有表列 OID，需要按索引合并行
				if len(tableOIDCols) > 0 {
					allIndices := map[string]bool{}
					for _, idxMap := range tableResults {
						for idx := range idxMap {
							allIndices[idx] = true
						}
					}
					if len(allIndices) == 0 {
						allIndices["0"] = true
					}

					for rowIdx := range allIndices {
						row := rowData{
							DeviceID:   deviceID,
							DeviceName: device.Name,
							DeviceIP:   device.IPAddress,
							RowIndex:   rowIdx,
							Cells:      make(map[string]*cellData),
						}
						for _, col := range list.Columns {
							if col.Type == "label" {
								fillLabel(row.Cells, col, device)
							} else if col.OID != "" {
								val := ""
								if col.IsTable == 1 {
									if idxMap, ok := tableResults[col.OID]; ok {
										val = idxMap[rowIdx]
									}
								} else {
									val = scalarResults[col.OID]
								}
								display := val
								if col.ValueMappings != nil {
									if mapped, ok := col.ValueMappings[val]; ok {
										display = mapped
									}
								}
								if val != "" && col.Unit != "" && display == val {
									display = val + " " + col.Unit
								}
								row.Cells[col.ID] = &cellData{Value: val, Display: display}
							}
						}
						devRows = append(devRows, row)
					}
				} else {
					// 仅标量数据，每设备一行
					row := rowData{
						DeviceID:   deviceID,
						DeviceName: device.Name,
						DeviceIP:   device.IPAddress,
						RowIndex:   "_device",
						Cells:      make(map[string]*cellData),
					}
					for _, col := range list.Columns {
						if col.Type == "label" {
							fillLabel(row.Cells, col, device)
						} else if col.OID != "" {
							val := scalarResults[col.OID]
							display := val
							if col.ValueMappings != nil {
								if mapped, ok := col.ValueMappings[val]; ok {
									display = mapped
								}
							}
							if val != "" && col.Unit != "" && display == val {
								display = val + " " + col.Unit
							}
							row.Cells[col.ID] = &cellData{Value: val, Display: display}
						}
					}
					devRows = append(devRows, row)
				}

				resCh <- devResult{index: idx, rows: devRows}
			}(i, dID)
		}
		wg.Wait()
		close(resCh)

		// 保持原始设备顺序组合行
		orderedDevRows := make([][]rowData, len(list.DeviceIDs))
		for res := range resCh {
			if res.index >= 0 && res.index < len(orderedDevRows) {
				orderedDevRows[res.index] = res.rows
			}
		}
		for _, devR := range orderedDevRows {
			rows = append(rows, devR...)
		}

		httpx.Respond(w, map[string]interface{}{
			"columns":   list.Columns,
			"rows":      rows,
			"sampledAt": sampledAt,
		})
	})

	return r
}

func nullStrToPtr(s sql.NullString) *string {
	if s.Valid {
		return &s.String
	}
	return nil
}
