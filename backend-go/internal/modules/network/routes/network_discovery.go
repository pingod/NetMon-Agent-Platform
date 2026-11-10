// 网络发现路由 — 7 个端点
// 等价于 TS routes/networkDiscoveryRoutes.ts
package routes

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// NetworkDiscoveryRoutes 网络发现路由
func NetworkDiscoveryRoutes() chi.Router {
	r := chi.NewRouter()

	// ====================== 扫描任务管理 ======================

	// GET /jobs, GET /tasks, GET / — 获取所有扫描任务
	handleListJobs := func(w http.ResponseWriter, r *http.Request) {
		jobs, err := repos.DiscoveryJobs.List()
		if err != nil {
			httpx.RespondError(w, "获取扫描任务列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, jobs)
	}
	r.Get("/", handleListJobs)
	r.Get("/jobs", handleListJobs)
	r.Get("/tasks", handleListJobs)

	// GET /jobs/:id — 获取单个任务详情
	r.Get("/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, err := repos.DiscoveryJobs.GetByID(chi.URLParam(r, "id"))
		if err != nil || job == nil {
			httpx.RespondError(w, "任务不存在", http.StatusNotFound)
			return
		}
		httpx.Respond(w, job)
	})

	// POST /jobs — 创建并启动扫描任务
	r.With(middleware.RequireRole("admin")).Post("/jobs", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name          string   `json:"name"`
			StartIP       string   `json:"start_ip"`
			EndIP         string   `json:"end_ip"`
			CredentialIDs []string `json:"credential_ids"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || req.StartIP == "" || req.EndIP == "" {
			httpx.RespondError(w, "name, start_ip, end_ip are required", http.StatusBadRequest)
			return
		}

		id := uuid.New().String()
		credIDsJSON, _ := json.Marshal(req.CredentialIDs)

		err := repos.DiscoveryJobs.Create(repos.DiscoveryJobCreateInput{
			ID:            id,
			Name:          req.Name,
			StartIP:       req.StartIP,
			EndIP:         req.EndIP,
			Status:        "pending",
			CredentialIDs: string(credIDsJSON),
		})
		if err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 异步启动真实并发 Ping / TCP / SNMP 混合扫描任务
		go services.RunDiscoveryJob(id, req.StartIP, req.EndIP)

		job, _ := repos.DiscoveryJobs.GetByID(id)
		httpx.Respond(w, job)
	})

	// POST /jobs/:id/cancel — 取消扫描任务
	r.With(middleware.RequireRole("admin")).Post("/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.DiscoveryJobs.Cancel(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "任务已取消")
	})

	// DELETE /jobs/:id — 删除扫描任务
	r.With(middleware.RequireRole("admin")).Delete("/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := repos.DiscoveryJobs.Delete(chi.URLParam(r, "id")); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "任务已删除")
	})

	// ====================== 扫描结果管理 ======================

	// GET /results — 获取扫描结果
	r.Get("/results", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		offset, _ := strconv.Atoi(q.Get("offset"))

		results, total, err := repos.DiscoveryResults.List(repos.DiscoveryResultListOptions{
			JobID:  q.Get("jobId"),
			Status: q.Get("status"),
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			httpx.RespondError(w, "获取扫描结果失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{"results": results, "total": total})
	})

	// POST /import — 将发现结果导入设备库
	r.With(middleware.RequireRole("admin")).Post("/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ResultIDs   []string `json:"result_ids"`
			SSHUsername string   `json:"ssh_username"`
			SSHPassword string   `json:"ssh_password"`
			SSHPort     int      `json:"ssh_port"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if len(req.ResultIDs) == 0 {
			httpx.RespondError(w, "result_ids array is required", http.StatusBadRequest)
			return
		}

		imported := 0
		failed := 0
		for _, rid := range req.ResultIDs {
			result, err := repos.DiscoveryResults.GetByID(rid)
			if err != nil || result == nil {
				failed++
				continue
			}
			port := req.SSHPort
			if port == 0 {
				port = 22
			}
			vendor := ""
			if result.Vendor != nil {
				vendor = *result.Vendor
			}
			model := ""
			if result.Model != nil {
				model = *result.Model
			}
			name := result.IPAddress
			if result.SysName != nil && *result.SysName != "" {
				name = *result.SysName
			}

			err = repos.NetworkDevices.CreateFromDiscovery(name, result.IPAddress, vendor, model, req.SSHUsername, port, "online", "")
			if err != nil {
				failed++
			} else {
				imported++
			}
		}

		httpx.Respond(w, map[string]interface{}{
			"imported": imported,
			"failed":   failed,
		})
	})

	return r
}
