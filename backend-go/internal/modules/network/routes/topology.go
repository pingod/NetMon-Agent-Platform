// 拓扑图路由 — 提供全局拓扑、自定义画布拓扑 CRUD、底图上传与实时接口状态与告警监控
package routes

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// TopologyRoutes 拓扑路由
func TopologyRoutes() chi.Router {
	r := chi.NewRouter()

	// ── GET / & GET /global — 获取全局拓扑数据 ──
	handleGlobalTopology := func(w http.ResponseWriter, r *http.Request) {
		devices, _ := repos.NetworkDevices.List()
		nodes := make([]map[string]interface{}, 0, len(devices))
		for _, d := range devices {
			nodeType := "switch"
			if d.Role != nil && *d.Role != "" {
				nodeType = *d.Role
			}
			nodes = append(nodes, map[string]interface{}{
				"id":           d.ID,
				"device_id":    d.ID,
				"name":         d.Name,
				"label":        d.Name,
				"ip":           d.IPAddress,
				"type":         nodeType,
				"vendor":       d.Vendor,
				"status":       d.Status,
				"snmp_enabled": d.SnmpEnabled,
			})
		}
		links, _ := repos.TopologyLinks.List("")
		edges := make([]map[string]interface{}, 0, len(links))
		for _, l := range links {
			edges = append(edges, map[string]interface{}{
				"id":               l.ID,
				"source":           l.DeviceAID,
				"target":           l.DeviceBID,
				"source_name":      l.DeviceAName,
				"source_interface": l.DeviceAInterface,
				"target_name":      l.DeviceBName,
				"target_interface": l.DeviceBInterface,
				"status":           l.Status,
			})
		}
		httpx.Respond(w, map[string]interface{}{
			"nodes": nodes,
			"edges": edges,
		})
	}
	r.Get("/", handleGlobalTopology)
	r.Get("/global", handleGlobalTopology)

	// ── 实时设备健康与接口 DOWN 状态汇总 ──
	r.Get("/live-status", func(w http.ResponseWriter, r *http.Request) {
		db := appdb.DB()

		type DeviceHealth struct {
			DeviceID            string                   `json:"device_id"`
			DeviceName          string                   `json:"device_name"`
			Status              string                   `json:"status"`
			TotalInterfaces     int                      `json:"total_interfaces"`
			UpInterfaces        int                      `json:"up_interfaces"`
			DownInterfacesCount int                      `json:"down_interfaces_count"`
			DownInterfaces      []string                 `json:"down_interfaces"`
			ActiveAlertCount    int                      `json:"active_alert_count"`
			MaxSeverity         string                   `json:"max_severity"`
			Alerts              []map[string]interface{} `json:"alerts"`
		}

		devices, err := repos.NetworkDevices.List()
		if err != nil {
			httpx.Respond(w, map[string]interface{}{"devices": map[string]DeviceHealth{}})
			return
		}

		healthMap := make(map[string]DeviceHealth)

		for _, dev := range devices {
			dh := DeviceHealth{
				DeviceID:       dev.ID,
				DeviceName:     dev.Name,
				Status:         dev.Status,
				DownInterfaces: make([]string, 0),
				Alerts:         make([]map[string]interface{}, 0),
			}

			// 查询该设备最新的每个接口指标（指标库）
			rows, qerr := appdb.DBForTable("snmp_interface_metrics").Query(`
				SELECT interface_name, if_oper_status, if_admin_status
				FROM snmp_interface_metrics
				WHERE device_id = ?
				ORDER BY sampled_at DESC, if_index ASC
				LIMIT 100`, dev.ID)
			if qerr == nil {
				seenIfs := make(map[string]bool)
				for rows.Next() {
					var ifName string
					var operStatus, adminStatus int
					if rows.Scan(&ifName, &operStatus, &adminStatus) == nil {
						if seenIfs[ifName] {
							continue
						}
						seenIfs[ifName] = true
						dh.TotalInterfaces++
						if operStatus == 1 || operStatus == 0 { // 1: up
							dh.UpInterfaces++
						} else {
							// 2: down, 3: testing, etc.
							dh.DownInterfacesCount++
							dh.DownInterfaces = append(dh.DownInterfaces, ifName)
						}
					}
				}
				rows.Close()
			}

			// 查询该设备的活跃告警（alerts.device_id 为显式关联列，状态词表统一为 firing/acknowledged/resolved）
			alertRows, aerr := db.Query(`
				SELECT id, title, severity, status, created_at
				FROM alerts
				WHERE (device_id = ? OR metadata LIKE ?) AND status IN ('firing', 'acknowledged')
				ORDER BY created_at DESC LIMIT 5`, dev.ID, "%"+dev.IPAddress+"%")
			if aerr == nil {
				for alertRows.Next() {
					var aID, aTitle, aSev, aStatus, aCreated string
					if alertRows.Scan(&aID, &aTitle, &aSev, &aStatus, &aCreated) == nil {
						dh.ActiveAlertCount++
						if dh.MaxSeverity == "" || aSev == "critical" || (aSev == "high" && dh.MaxSeverity != "critical") {
							dh.MaxSeverity = aSev
						}
						dh.Alerts = append(dh.Alerts, map[string]interface{}{
							"id":         aID,
							"title":      aTitle,
							"severity":   aSev,
							"status":     aStatus,
							"created_at": aCreated,
						})
					}
				}
				alertRows.Close()
			}

			// 如果有接口 down 且没有更高等级告警，标记至少为 warning
			if dh.DownInterfacesCount > 0 && dh.MaxSeverity == "" {
				dh.MaxSeverity = "warning"
			}

			healthMap[dev.ID] = dh
		}

		httpx.Respond(w, map[string]interface{}{
			"devices": healthMap,
		})
	})

	// ── 自定义画布拓扑 CRUD ──
	r.Get("/custom", func(w http.ResponseWriter, r *http.Request) {
		list, err := repos.CustomTopologies.List()
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		httpx.Respond(w, list)
	})

	r.Get("/custom/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		t, err := repos.CustomTopologies.GetByID(id)
		if err != nil || t == nil {
			httpx.RespondError(w, "拓扑不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, t)
	})

	r.With(middleware.RequireRole("admin")).Post("/custom", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name            string      `json:"name"`
			Description     string      `json:"description"`
			BackgroundImage string      `json:"background_image"`
			Nodes           interface{} `json:"nodes"`
			Links           interface{} `json:"links"`
			IsDefault       int         `json:"is_default"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			req.Name = "未命名网络拓扑"
		}
		nodesJSON, _ := json.Marshal(req.Nodes)
		linksJSON, _ := json.Marshal(req.Links)

		t, err := repos.CustomTopologies.Create(repos.CustomNetworkTopology{
			Name:            req.Name,
			Description:     req.Description,
			BackgroundImage: req.BackgroundImage,
			NodesJSON:       string(nodesJSON),
			LinksJSON:       string(linksJSON),
			IsDefault:       req.IsDefault,
		})
		if err != nil {
			httpx.RespondError(w, "创建拓扑失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, t)
	})

	r.With(middleware.RequireRole("admin")).Put("/custom/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Name            string      `json:"name"`
			Description     string      `json:"description"`
			BackgroundImage string      `json:"background_image"`
			Nodes           interface{} `json:"nodes"`
			Links           interface{} `json:"links"`
			IsDefault       int         `json:"is_default"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		nodesJSON, _ := json.Marshal(req.Nodes)
		linksJSON, _ := json.Marshal(req.Links)

		err := repos.CustomTopologies.Update(id, repos.CustomNetworkTopology{
			Name:            req.Name,
			Description:     req.Description,
			BackgroundImage: req.BackgroundImage,
			NodesJSON:       string(nodesJSON),
			LinksJSON:       string(linksJSON),
			IsDefault:       req.IsDefault,
		})
		if err != nil {
			httpx.RespondError(w, "更新拓扑失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"id": id, "success": true})
	})

	r.With(middleware.RequireRole("admin")).Delete("/custom/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_ = repos.CustomTopologies.Delete(id)
		httpx.RespondMessage(w, "拓扑已删除")
	})

	// ── 上传拓扑底图 (Base64 或 Multipart) ──
	r.With(middleware.RequireRole("admin")).Post("/upload-bg", func(w http.ResponseWriter, r *http.Request) {
		// 检查是否为 multipart 上传
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(10 << 20); err == nil {
				files := r.MultipartForm.File["file"]
				if len(files) > 0 {
					f, ferr := files[0].Open()
					if ferr == nil {
						defer f.Close()
						bytes, rerr := io.ReadAll(f)
						if rerr == nil {
							mimeType := http.DetectContentType(bytes)
							dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(bytes)
							httpx.Respond(w, map[string]string{
								"url":      dataURL,
								"filename": files[0].Filename,
							})
							return
						}
					}
				}
			}
		}

		// JSON Base64 格式
		var req struct {
			ImageBase64 string `json:"image_base64"`
			Filename    string `json:"filename"`
		}
		if parseJSON(w, r, &req) && req.ImageBase64 != "" {
			httpx.Respond(w, map[string]string{
				"url":      req.ImageBase64,
				"filename": req.Filename,
			})
			return
		}

		httpx.RespondError(w, "未接收到有效的图片文件", http.StatusBadRequest)
	})

	// ── 拓扑依赖兼容端点 ──
	r.Get("/dependency", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []interface{}{})
	})
	r.With(middleware.RequireRole("admin")).Post("/dependency", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, map[string]string{"message": "success"})
	})
	r.With(middleware.RequireRole("admin")).Delete("/dependency/{id}", func(w http.ResponseWriter, r *http.Request) {
		httpx.RespondMessage(w, "deleted")
	})
	r.Get("/server/{id}", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, map[string]interface{}{"nodes": []interface{}{}, "edges": []interface{}{}})
	})
	r.Get("/affected/{alertId}", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []interface{}{})
	})

	return r
}
