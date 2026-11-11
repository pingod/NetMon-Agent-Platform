// SSH 密钥/凭证路由 — 6 个端点
// 对应前端 SSHKeys.tsx 调用的 /api/v1/ssh-keys 路径
package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
	"itops-backend/internal/services/audit"
)

// SshKeyRoutes SSH 密钥管理路由
func SshKeyRoutes() chi.Router {
	r := chi.NewRouter()

	// GET / — 列出所有凭证（不含敏感字段）
	r.With(middleware.RequireRole("admin", "operator")).Get("/", func(w http.ResponseWriter, r *http.Request) {
		keys, err := repos.SshKeys.List()
		if err != nil {
			httpx.RespondError(w, "获取 SSH 凭证列表失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, keys)
	})

	// GET /{id} — 获取单个凭证元数据（不下发密码/私钥，查看明文走 /reveal）
	r.With(middleware.RequireRole("admin", "operator")).Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		key, err := repos.SshKeys.GetByID(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		if key == nil {
			httpx.RespondError(w, "SSH key not found", http.StatusNotFound)
			return
		}
		key.Password = nil
		key.PrivateKey = nil
		httpx.Respond(w, key)
	})

	// GET /{id}/reveal — 查看凭证明文（仅 admin，写审计日志）
	r.With(middleware.RequireRole("admin")).Get("/{id}/reveal", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		key, err := repos.SshKeys.GetByID(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		if key == nil {
			httpx.RespondError(w, "SSH key not found", http.StatusNotFound)
			return
		}

		result := map[string]interface{}{
			"id":        key.ID,
			"name":      key.Name,
			"auth_type": key.AuthType,
			"username":  key.Username,
		}
		if key.PrivateKey != nil && *key.PrivateKey != "" {
			plain, derr := crypto.Decrypt(*key.PrivateKey)
			if derr != nil {
				plain = *key.PrivateKey // 兼容历史明文数据
			}
			result["private_key"] = plain
		}
		if key.Password != nil && *key.Password != "" {
			plain, derr := crypto.Decrypt(*key.Password)
			if derr != nil {
				plain = *key.Password
			}
			result["password"] = plain
		}

		userID := ""
		if u := middleware.GetUser(r); u != nil {
			userID = u.ID
		}
		audit.CreateAuditLog(audit.AuditInput{
			UserID:       userID,
			Action:       "ssh_key_reveal",
			ResourceType: "ssh_key",
			ResourceID:   id,
			Details:      map[string]interface{}{"name": key.Name},
			IPAddress:    middleware.ClientIP(r),
		})
		httpx.Respond(w, result)
	})

	// POST / — 创建凭证
	r.With(middleware.RequireRole("admin")).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string  `json:"name"`
			AuthType    string  `json:"auth_type"`
			Username    *string `json:"username"`
			Password    *string `json:"password"`
			PrivateKey  *string `json:"private_key"`
			Description *string `json:"description"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		if req.Name == "" {
			httpx.RespondError(w, "密钥名称不能为空", http.StatusBadRequest)
			return
		}

		// 检查名称唯一性
		existing, _ := repos.SshKeys.FindByName(req.Name)
		if existing != nil {
			httpx.RespondError(w, "SSH key name already exists", http.StatusConflict)
			return
		}

		id := uuid.New().String()

		if req.AuthType == "key" {
			// SSH 密钥类型
			if req.PrivateKey == nil || *req.PrivateKey == "" {
				httpx.RespondError(w, "SSH密钥类型必须提供私钥", http.StatusBadRequest)
				return
			}

			privKey := *req.PrivateKey
			// 验证私钥格式
			if !strings.Contains(privKey, "BEGIN") || !strings.Contains(privKey, "PRIVATE KEY") || !strings.Contains(privKey, "END") {
				httpx.RespondError(w, "无效的 SSH 私钥格式，请确保粘贴的内容包含完整的私钥文本（从 BEGIN 到 END）", http.StatusBadRequest)
				return
			}

			// 提取密钥类型
			keyType := "unknown"
			if strings.Contains(privKey, "BEGIN OPENSSH PRIVATE KEY") {
				keyType = "openssh"
			} else if strings.Contains(privKey, "BEGIN RSA PRIVATE KEY") {
				keyType = "rsa"
			} else if strings.Contains(privKey, "BEGIN EC PRIVATE KEY") {
				keyType = "ec"
			} else if strings.Contains(privKey, "BEGIN DSA PRIVATE KEY") {
				keyType = "dsa"
			} else if strings.Contains(privKey, "BEGIN PRIVATE KEY") {
				keyType = "pkcs8"
			}

			// 生成指纹
			hash := sha256.Sum256([]byte(privKey))
			fingerprint := "SHA256:" + hex.EncodeToString(hash[:])[:43]

			// 加密私钥
			encryptedKey, encErr := crypto.Encrypt(privKey)
			if encErr != nil {
				httpx.RespondError(w, "加密私钥失败", http.StatusInternalServerError)
				return
			}

			desc := ""
			if req.Description != nil {
				desc = *req.Description
			}

			if err := repos.SshKeys.CreateKey(id, req.Name, keyType, fingerprint, encryptedKey, desc); err != nil {
				httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if req.AuthType == "password" {
			// 账号密码类型
			if req.Username == nil || *req.Username == "" || req.Password == nil || *req.Password == "" {
				httpx.RespondError(w, "用户名密码类型必须提供用户名和密码", http.StatusBadRequest)
				return
			}

			// 加密密码
			encryptedPwd, encErr := crypto.Encrypt(*req.Password)
			if encErr != nil {
				httpx.RespondError(w, "加密密码失败", http.StatusInternalServerError)
				return
			}

			desc := ""
			if req.Description != nil {
				desc = *req.Description
			}

			if err := repos.SshKeys.CreatePassword(id, req.Name, *req.Username, encryptedPwd, desc); err != nil {
				httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			httpx.RespondError(w, "auth_type 必须为 key 或 password", http.StatusBadRequest)
			return
		}

		httpx.Respond(w, map[string]string{"id": id})
	})

	// PUT /{id} — 更新凭证
	r.With(middleware.RequireRole("admin")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// 检查存在
		existing, err := repos.SshKeys.GetByID(id)
		if err != nil || existing == nil {
			httpx.RespondError(w, "SSH key not found", http.StatusNotFound)
			return
		}

		var req struct {
			Name        *string `json:"name"`
			AuthType    *string `json:"auth_type"`
			Username    *string `json:"username"`
			Password    *string `json:"password"`
			PrivateKey  *string `json:"private_key"`
			Description *string `json:"description"`
		}
		if !parseJSON(w, r, &req) {
			return
		}

		// 如果提供了新名称，检查唯一性
		if req.Name != nil && *req.Name != "" {
			dup, _ := repos.SshKeys.FindByNameExcludeID(*req.Name, id)
			if dup != nil {
				httpx.RespondError(w, "SSH key name already exists", http.StatusConflict)
				return
			}
		}

		update := *existing

		if req.Name != nil {
			update.Name = *req.Name
		}

		// 确定最终的 auth_type
		finalAuthType := ""
		if existing.AuthType != nil {
			finalAuthType = *existing.AuthType
		}
		if req.AuthType != nil {
			finalAuthType = *req.AuthType
		}

		if req.Username != nil {
			update.Username = req.Username
		}

		// 处理密码更新
		if req.Password != nil && *req.Password != "" && finalAuthType == "password" {
			encrypted, encErr := crypto.Encrypt(*req.Password)
			if encErr != nil {
				httpx.RespondError(w, "加密密码失败", http.StatusInternalServerError)
				return
			}
			update.Password = &encrypted
		}

		// 处理私钥更新
		if req.PrivateKey != nil && *req.PrivateKey != "" && finalAuthType == "key" {
			privKey := *req.PrivateKey
			if !strings.Contains(privKey, "BEGIN") || !strings.Contains(privKey, "PRIVATE KEY") || !strings.Contains(privKey, "END") {
				httpx.RespondError(w, "无效的 SSH 私钥格式", http.StatusBadRequest)
				return
			}

			keyType := "unknown"
			if strings.Contains(privKey, "BEGIN OPENSSH PRIVATE KEY") {
				keyType = "openssh"
			} else if strings.Contains(privKey, "BEGIN RSA PRIVATE KEY") {
				keyType = "rsa"
			} else if strings.Contains(privKey, "BEGIN EC PRIVATE KEY") {
				keyType = "ec"
			} else if strings.Contains(privKey, "BEGIN DSA PRIVATE KEY") {
				keyType = "dsa"
			} else if strings.Contains(privKey, "BEGIN PRIVATE KEY") {
				keyType = "pkcs8"
			}

			hash := sha256.Sum256([]byte(privKey))
			fp := "SHA256:" + hex.EncodeToString(hash[:])[:43]

			encrypted, encErr := crypto.Encrypt(privKey)
			if encErr != nil {
				httpx.RespondError(w, "加密私钥失败", http.StatusInternalServerError)
				return
			}
			update.PrivateKey = &encrypted
			update.KeyType = &keyType
			update.Fingerprint = &fp
		}

		if req.Description != nil {
			update.Description = req.Description
		}

		if err := repos.SshKeys.Update(id, &update); err != nil {
			httpx.RespondError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "updated")
	})

	// DELETE /{id} — 删除凭证
	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		exists, _ := repos.SshKeys.ExistsByID(id)
		if !exists {
			httpx.RespondError(w, "SSH key not found", http.StatusNotFound)
			return
		}

		// 检查使用情况
		usageCount, _ := repos.SshKeys.CountUsage(id)
		if usageCount > 0 {
			httpx.RespondError(w, fmt.Sprintf("该密钥正在被 %d 台设备使用，无法删除。请先解除关联后再删除。", usageCount), http.StatusConflict)
			return
		}

		if err := repos.SshKeys.Delete(id); err != nil {
			httpx.RespondError(w, "删除失败", http.StatusInternalServerError)
			return
		}
		httpx.RespondMessage(w, "deleted")
	})

	// GET /{id}/usage — 获取使用此密钥的服务器列表
	r.With(middleware.RequireRole("admin", "operator")).Get("/{id}/usage", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		servers, err := repos.SshKeys.ListServersByKey(id)
		if err != nil {
			httpx.RespondError(w, "查询失败", http.StatusInternalServerError)
			return
		}
		httpx.Respond(w, map[string]interface{}{
			"count":   len(servers),
			"servers": servers,
		})
	})

	return r
}
