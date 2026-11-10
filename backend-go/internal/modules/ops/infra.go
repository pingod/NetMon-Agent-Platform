package ops

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

// ContainerRoutes 容器管理路由 (/api/v1/containers)
func ContainerRoutes() chi.Router {
	return FullContainerRoutes()
}

// KubernetesRoutes K8s管理路由 (/api/v1/kubernetes)
func KubernetesRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/contexts", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, name, cluster_url, namespace, auth_type, status, node_count, pod_count, created_at FROM k8s_contexts ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, name string
			var clusterURL, namespace, authType, status, createdAt sql.NullString
			var nodeCount, podCount int
			if rows.Scan(&id, &name, &clusterURL, &namespace, &authType, &status, &nodeCount, &podCount, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":          id,
					"name":        name,
					"cluster_url": clusterURL.String,
					"namespace":   namespace.String,
					"auth_type":   authType.String,
					"status":      status.String,
					"node_count":  nodeCount,
					"pod_count":   podCount,
					"created_at":  createdAt.String,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.Get("/nodes", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []map[string]interface{}{})
	})
	r.Get("/pods", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []map[string]interface{}{})
	})
	r.Get("/deployments", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []map[string]interface{}{})
	})
	r.Get("/services", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, []map[string]interface{}{})
	})

	return r
}

// VirtualMachineRoutes 虚拟机路由 (/api/v1/virtual-machines)
func VirtualMachineRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, name, host, status, os, cpu_cores, memory_mb, disk_gb, ip_address, hypervisor, created_at FROM virtual_machines ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, name, host, status, os, ipAddr, hypervisor, createdAt string
			var cpuCores, memMB, diskGB int
			if rows.Scan(&id, &name, &host, &status, &os, &cpuCores, &memMB, &diskGB, &ipAddr, &hypervisor, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":          id,
					"name":        name,
					"host":        host,
					"status":      status,
					"os":          os,
					"cpu_cores":   cpuCores,
					"memory_mb":   memMB,
					"disk_gb":     diskGB,
					"ip_address":  ipAddr,
					"hypervisor":  hypervisor,
					"created_at":  createdAt,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		var total, running, stopped int
		appdb.DB().QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='running' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status='stopped' THEN 1 ELSE 0 END),0) FROM virtual_machines`).
			Scan(&total, &running, &stopped)
		httpx.Respond(w, map[string]interface{}{
			"total":   total,
			"running": running,
			"stopped": stopped,
		})
	})

	r.Get("/platforms", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT id, name, hypervisor_type, host, port, username, status, created_at FROM vm_platforms ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		var list []map[string]interface{}
		for rows.Next() {
			var id, name, hType, host, username, status, createdAt string
			var port int
			if rows.Scan(&id, &name, &hType, &host, &port, &username, &status, &createdAt) == nil {
				list = append(list, map[string]interface{}{
					"id":              id,
					"name":            name,
					"hypervisor_type": hType,
					"host":            host,
					"port":            port,
					"username":        username,
					"status":          status,
					"created_at":      createdAt,
				})
			}
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		httpx.Respond(w, list)
	})

	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string `json:"name"`
			IPAddress  string `json:"ip_address"`
			OS         string `json:"os"`
			CPUCores   int    `json:"cpu_cores"`
			MemoryMB   int    `json:"memory_mb"`
			DiskGB     int    `json:"disk_gb"`
			Hypervisor string `json:"hypervisor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err := appdb.DB().Exec(`
			INSERT INTO virtual_machines (id, name, ip_address, os, cpu_cores, memory_mb, disk_gb, hypervisor, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'running', ?, ?)`,
			id, req.Name, req.IPAddress, req.OS, req.CPUCores, req.MemoryMB, req.DiskGB, req.Hypervisor, now, now)
		if err != nil {
			httpx.RespondError(w, "创建虚拟机失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id, "name": req.Name})
	})

	return r
}
