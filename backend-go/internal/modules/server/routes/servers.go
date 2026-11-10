// 服务器路由 — 对应 TS serverRoutes.ts
// 挂载路径：/api/v1/servers
package routes

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/modules/network/services"
	"itops-backend/internal/repos"
)

// ServerRoutes 服务器模块路由
func ServerRoutes() chi.Router {
	r := chi.NewRouter()

	// ── Agent 采集与性能时序接口 ──
	r.Mount("/agent", AgentCollectorRoutes())
	r.Get("/{id}/metrics/history", handleGetServerMetricsHistory)
	r.Post("/{id}/agent/install", handleSingleInstallAgent)
	r.Post("/agent/batch-install", handleBatchInstallAgent)

	// GET / — 服务器列表
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		list, err := repos.Servers.List()
		if err != nil {
			httpx.RespondError(w, "获取服务器列表失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, list)
	})

	// POST / — 创建服务器
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req repos.ServerCreateInput
		if !parseJSON(w, r, &req) {
			return
		}
		if req.Name == "" || req.Hostname == "" || req.Username == "" {
			httpx.RespondError(w, "名称、主机名、用户名不能为空", http.StatusBadRequest)
			return
		}
		if req.Port == 0 {
			req.Port = 22
		}
		if req.OSType == "" {
			req.OSType = "linux"
		}
		s, err := repos.Servers.Create(req)
		if err != nil {
			httpx.RespondError(w, "创建服务器失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondStatus(w, http.StatusCreated, s)
	})

	// PUT /{id} — 更新服务器
	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}

		var raw struct {
			Name          *string   `json:"name"`
			Hostname      *string   `json:"hostname"`
			Port          *int      `json:"port"`
			Username      *string   `json:"username"`
			Password      *string   `json:"password"`
			PrivateKey    *string   `json:"private_key"`
			UseSSHKey     *bool     `json:"use_ssh_key"`
			Description   *string   `json:"description"`
			Tags          *[]string `json:"tags"`
			OSType        *string   `json:"os_type"`
			VncPort       *int      `json:"vnc_port"`
			VncPassword   *string   `json:"vnc_password"`
			SSHKeyID      *string   `json:"ssh_key_id"`
			IPAddress     *string   `json:"ip_address"`
			PrivateIP     *string   `json:"private_ip"`
			CloudProvider *string   `json:"cloud_provider"`
			GroupID       *string   `json:"group_id"`
			Vendor        *string   `json:"vendor"`
			Model         *string   `json:"model"`
		}
		if !parseJSON(w, r, &raw) {
			return
		}

		req := repos.ServerUpdateInput{
			Name: raw.Name, Hostname: raw.Hostname, Port: raw.Port, Username: raw.Username,
			Password: raw.Password, PrivateKey: raw.PrivateKey, Description: raw.Description,
			OSType: raw.OSType, VncPort: raw.VncPort, VncPassword: raw.VncPassword,
			SSHKeyID: raw.SSHKeyID, IPAddress: raw.IPAddress, PrivateIP: raw.PrivateIP,
			CloudProvider: raw.CloudProvider, GroupID: raw.GroupID,
			UseSSHKey: raw.UseSSHKey, Vendor: raw.Vendor, Model: raw.Model,
		}
		if raw.Tags != nil {
			req.Tags = raw.Tags
		}

		s, err := repos.Servers.Update(id, req)
		if err != nil {
			httpx.RespondError(w, "更新服务器失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, s)
	})

	// DELETE /{id} — 删除服务器
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}
		if err := repos.Servers.Delete(id); err != nil {
			httpx.RespondError(w, "删除失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// GET /{id}/command-history — 命令历史
	r.Get("/{id}/command-history", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}
		items, err := repos.Servers.GetCommandHistory(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, items)
	})

	// GET /{id}/command-history/export — 导出命令历史（JSON）
	r.Get("/{id}/command-history/export", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		items, err := repos.Servers.GetCommandHistory(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		exportJSON(w, items, "command-history")
	})

	// GET /{id}/compliance-history — 合规检查历史
	r.Get("/{id}/compliance-history", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		exists, _ := repos.Servers.Exists(id)
		if !exists {
			httpx.RespondError(w, "服务器不存在", http.StatusNotFound)
			return
		}
		items, err := repos.Servers.GetComplianceHistory(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, items)
	})

	// GET /{id}/compliance-history/export — 导出合规历史（JSON）
	r.Get("/{id}/compliance-history/export", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		items, err := repos.Servers.GetComplianceHistory(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		exportJSON(w, items, "compliance-history")
	})

	// POST /batch-inspect & POST /batch-compliance — 批量服务器合规巡检
	r.With(middleware.RequireRole("admin")).Post("/batch-inspect", handleBatchServerInspection)
	r.With(middleware.RequireRole("admin")).Post("/batch-compliance", handleBatchServerInspection)

	// POST /batch-delete — 批量删除服务器
	r.With(middleware.RequireRole("admin")).Post("/batch-delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ServerIDs      []string `json:"server_ids"`
			ServerIDsCamel []string `json:"serverIds"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		targetIDs := req.ServerIDs
		if len(targetIDs) == 0 {
			targetIDs = req.ServerIDsCamel
		}
		if len(targetIDs) == 0 {
			httpx.RespondError(w, "server_ids is required", http.StatusBadRequest)
			return
		}
		count, err := repos.Servers.BatchDelete(targetIDs)
		if err != nil {
			httpx.RespondError(w, "批量删除失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{
			"success":       true,
			"deleted_count": count,
			"message":       fmt.Sprintf("已成功批量删除 %d 台服务器", count),
		})
	})

	// POST /test-connection — 临时连接测试（表单填写时直接测试）
	r.With(middleware.RequireRole("admin")).Post("/test-connection", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ServerID     string `json:"server_id"`
			Host         string `json:"host"`
			Hostname     string `json:"hostname"`
			IPAddress    string `json:"ip_address"`
			Port         int    `json:"port"`
			Username     string `json:"username"`
			Password     string `json:"password"`
			PrivateKey   string `json:"private_key"`
			UseSSHKey    bool   `json:"use_ssh_key"`
			SSHKeyID     string `json:"ssh_key_id"`
			CredentialID string `json:"credential_id"`
			OSType       string `json:"os_type"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		targetHost := req.Hostname
		if targetHost == "" {
			targetHost = req.Host
		}
		if targetHost == "" {
			targetHost = req.IPAddress
		}

		// 若提供了已有 server_id 且未输入密码/主机名，自动回退使用已保存的凭据
		if req.ServerID != "" {
			if existing, err := repos.Servers.GetByID(req.ServerID); err == nil && existing != nil {
				if targetHost == "" {
					if existing.Hostname != "" {
						targetHost = existing.Hostname
					} else if existing.IPAddress != nil {
						targetHost = *existing.IPAddress
					}
				}
				if req.Port == 0 && existing.Port != 0 {
					req.Port = existing.Port
				}
				if req.Username == "" && existing.Username != "" {
					req.Username = existing.Username
				}
				if req.Password == "" && existing.Password != nil {
					if dec, err := crypto.Decrypt(*existing.Password); err == nil {
						req.Password = dec
					} else {
						req.Password = *existing.Password
					}
				}
				if req.SSHKeyID == "" && existing.SSHKeyID != nil {
					req.SSHKeyID = *existing.SSHKeyID
				}
				if req.OSType == "" && existing.OSType != nil {
					req.OSType = *existing.OSType
				}
			}
		}

		if targetHost == "" {
			httpx.RespondError(w, "主机名或 IP 地址不能为空", http.StatusBadRequest)
			return
		}

		testPort := req.Port
		if testPort == 0 {
			testPort = 22
		}

		testUsername := req.Username
		testPassword := req.Password
		testPrivateKey := req.PrivateKey

		// 如果指定了凭证 ID (SSH Key)
		keyID := req.SSHKeyID
		if keyID == "" {
			keyID = req.CredentialID
		}
		if keyID != "" {
			cred, err := repos.SshKeys.GetByID(keyID)
			if err == nil && cred != nil {
				if cred.Username != nil && *cred.Username != "" && testUsername == "" {
					testUsername = *cred.Username
				}
				if cred.Password != nil && *cred.Password != "" {
					if dec, err := crypto.Decrypt(*cred.Password); err == nil {
						testPassword = dec
					} else {
						testPassword = *cred.Password
					}
				}
				if cred.PrivateKey != nil && *cred.PrivateKey != "" {
					if dec, err := crypto.Decrypt(*cred.PrivateKey); err == nil {
						testPrivateKey = dec
					} else {
						testPrivateKey = *cred.PrivateKey
					}
				}
			}
		}

		if testUsername == "" {
			if req.OSType == "windows" {
				testUsername = "Administrator"
			} else {
				testUsername = "root"
			}
		}

		sshDevice := services.SSHDeviceConfig{
			Host:       targetHost,
			Port:       testPort,
			Username:   testUsername,
			Password:   testPassword,
			PrivateKey: testPrivateKey,
			OSType:     req.OSType,
		}

		result := services.NetworkDeviceServiceInstance.TestTemporaryConnectionWithConfig(sshDevice)
		httpx.Respond(w, result)
	})

	// POST /{id}/test-connection — 单台服务器连接测试
	r.With(middleware.RequireRole("admin")).Post("/{id}/test-connection", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sshCfg, err := buildServerSSHConfig(id)
		if err != nil {
			httpx.RespondError(w, "无法获取服务器凭据: "+err.Error(), http.StatusBadRequest)
			return
		}
		result := services.NetworkDeviceServiceInstance.TestTemporaryConnectionWithConfig(sshCfg)
		httpx.Respond(w, result)
	})

	return r
}

// exportJSON 导出 JSON 文件（下载）
func exportJSON(w http.ResponseWriter, data interface{}, baseName string) {
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		httpx.RespondError(w, "导出失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename="+baseName+".json")
	w.Write(buf)
}

// buildServerSSHConfig 构建服务器 SSH 连接配置（含 ssh_key 凭证解析）
func buildServerSSHConfig(serverID string) (services.SSHDeviceConfig, error) {
	username, password, privateKey, useSSHKey, err := repos.Servers.GetCredentialFields(serverID)
	if err != nil {
		return services.SSHDeviceConfig{}, err
	}

	// 如果服务器没有直接密码/私钥，尝试通过 ssh_key_id 关联凭证
	if password == "" && privateKey == "" {
		s, err := repos.Servers.GetByID(serverID)
		if err == nil && s != nil && s.SSHKeyID != nil && *s.SSHKeyID != "" {
			key, kerr := repos.SshKeys.GetByID(*s.SSHKeyID)
			if kerr == nil && key != nil {
				if key.Username != nil && *key.Username != "" {
					username = *key.Username
				}
				// 解密凭证中的密码/私钥
				if key.AuthType != nil && *key.AuthType == "password" {
					if key.Password != nil && *key.Password != "" {
						if dec, derr := decryptLegacy(*key.Password); derr == nil {
							password = dec
						}
					}
				} else {
					if key.PrivateKey != nil && *key.PrivateKey != "" {
						if dec, derr := decryptLegacy(*key.PrivateKey); derr == nil {
							privateKey = dec
						}
					}
				}
			}
		}
	}

	cfg := services.SSHDeviceConfig{
		Host:       hostFromServer(serverID),
		Port:       portFromServer(serverID),
		Username:   username,
		Password:   password,
		PrivateKey: privateKey,
	}
	if useSSHKey == 1 {
		cfg.AuthType = "key"
	} else {
		cfg.AuthType = "password"
	}
	// 设置 os_type 用于采集分支判断
	if s, err := repos.Servers.GetByID(serverID); err == nil && s != nil && s.OSType != nil {
		cfg.OSType = *s.OSType
	}
	return cfg, nil
}

// decryptLegacy 解密（用于 ssh_keys 表中的密文）
func decryptLegacy(enc string) (string, error) {
	return crypto.Decrypt(enc)
}

// hostFromServer 获取服务器主机名/IP
func hostFromServer(serverID string) string {
	s, err := repos.Servers.GetByID(serverID)
	if err != nil || s == nil {
		return ""
	}
	// 优先使用用户配置的 Hostname（通常为纳管公网 IP、外网域名或指定连接地址）
	if s.Hostname != "" {
		return s.Hostname
	}
	if s.IPAddress != nil && *s.IPAddress != "" {
		return *s.IPAddress
	}
	return ""
}

// portFromServer 获取服务器 SSH 端口
func portFromServer(serverID string) int {
	s, err := repos.Servers.GetByID(serverID)
	if err != nil || s == nil {
		return 22
	}
	if s.Port > 0 {
		return s.Port
	}
	return 22
}

// newID 生成 UUID
func newID() string {
	return uuid.New().String()
}
