// 网络高级功能路由 — 11 个端点
// 等价于 TS routes/networkAdvancedRoutes.ts
package routes

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// NetworkAdvancedRoutes 网络高级功能路由
func NetworkAdvancedRoutes() chi.Router {
	r := chi.NewRouter()

	// ================================================================
	// 配置备份 API
	// ================================================================

	// POST /backup/:deviceId — 备份指定设备
	r.With(middleware.RequireRole("admin")).Post("/backup/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		result, err := services.ConfigBackupServiceInstance.BackupDeviceManual(deviceID, "")
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, result)
	})

	// POST /backup-all — 批量备份所有在线设备
	r.With(middleware.RequireRole("admin")).Post("/backup-all", func(w http.ResponseWriter, r *http.Request) {
		// 获取所有设备，逐个备份
		devices, err := repos.NetworkDevices.List()
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var deviceIDs []string
		for _, d := range devices {
			if d.Status == "online" {
				deviceIDs = append(deviceIDs, d.ID)
			}
		}

		var results []services.BackupDeviceResult
		for _, id := range deviceIDs {
			result, berr := services.ConfigBackupServiceInstance.BackupDeviceManual(id, "")
			if berr != nil {
				results = append(results, services.BackupDeviceResult{
					DeviceID: id,
					Status:   "error",
					Error:    berr.Error(),
				})
			} else {
				results = append(results, *result)
			}
		}
		httpx.Respond(w, results)
	})

	// GET /backup-history/:deviceId — 获取设备备份历史
	r.Get("/backup-history/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 30
		}
		deviceID := chi.URLParam(r, "deviceId")
		records, err := services.ConfigBackupServiceInstance.ListRecords("", deviceID, "", limit, 0)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, records)
	})

	// GET /backup-content/:backupId — 读取备份内容
	r.Get("/backup-content/{backupId}", func(w http.ResponseWriter, r *http.Request) {
		record, err := services.ConfigBackupServiceInstance.GetRecord(chi.URLParam(r, "backupId"))
		if err != nil || record == nil {
			httpx.RespondError(w, "Backup not found", http.StatusNotFound)
			return
		}
		httpx.Respond(w, record)
	})

	// GET /backup-diff/:backupIdA/:backupIdB — 对比两个备份版本
	r.Get("/backup-diff/{backupIdA}/{backupIdB}", func(w http.ResponseWriter, r *http.Request) {
		recordA, errA := services.ConfigBackupServiceInstance.GetRecord(chi.URLParam(r, "backupIdA"))
		recordB, errB := services.ConfigBackupServiceInstance.GetRecord(chi.URLParam(r, "backupIdB"))
		if errA != nil || recordA == nil || errB != nil || recordB == nil {
			httpx.RespondError(w, "Backup not found", http.StatusNotFound)
			return
		}
		// 简单对比：返回两个记录的输出
		httpx.Respond(w, map[string]interface{}{
			"backupA": recordA,
			"backupB": recordB,
		})
	})

	// POST /check-change/:deviceId — 检查配置变更
	r.With(middleware.RequireRole("admin")).Post("/check-change/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		// 获取最近两次备份对比
		records, err := services.ConfigBackupServiceInstance.ListRecords("", deviceID, "success", 2, 0)
		if err != nil || len(records) < 2 {
			httpx.Respond(w, map[string]interface{}{"changed": false, "message": "not enough backup history"})
			return
		}
		httpx.Respond(w, map[string]interface{}{
			"changed":   true,
			"previous":  records[1],
			"current":   records[0],
		})
	})

	// ================================================================
	// LLDP 邻居发现 API
	// ================================================================

	// POST /lldp/:deviceId — 发现指定设备邻居
	r.With(middleware.RequireRole("admin")).Post("/lldp/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		neighbors, err := services.LldpDiscoveryServiceInstance.DiscoverNeighbors(chi.URLParam(r, "deviceId"))
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, neighbors)
	})

	// POST /lldp-batch — 批量发现邻居
	r.With(middleware.RequireRole("admin")).Post("/lldp-batch", func(w http.ResponseWriter, r *http.Request) {
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
		result := services.LldpDiscoveryServiceInstance.BatchDiscover(req.DeviceIDs)
		httpx.Respond(w, result)
	})

	// POST /lldp-all — 发现所有在线设备邻居
	r.With(middleware.RequireRole("admin")).Post("/lldp-all", func(w http.ResponseWriter, r *http.Request) {
		result := services.LldpDiscoveryServiceInstance.DiscoverAll()
		httpx.Respond(w, result)
	})

	// GET /topology — 获取拓扑链路
	r.Get("/topology", func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.URL.Query().Get("deviceId")
		links, err := services.LldpDiscoveryServiceInstance.GetTopologyLinks(deviceID)
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, links)
	})

	// GET /impact-path/:deviceId — 获取设备影响路径
	r.Get("/impact-path/{deviceId}", func(w http.ResponseWriter, r *http.Request) {
		maxHops, _ := strconv.Atoi(r.URL.Query().Get("maxHops"))
		if maxHops <= 0 {
			maxHops = 3
		}
		path := services.LldpDiscoveryServiceInstance.GetImpactPath(chi.URLParam(r, "deviceId"), maxHops)
		httpx.Respond(w, path)
	})

	return r
}
