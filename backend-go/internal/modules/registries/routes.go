// Package registries 镜像仓库管理（/api/v1/registries）。
//
// 前端镜像仓库页此前调用该前缀一直 404。这里补齐 CRUD、连通性测试与镜像列举；
// 密码用平台 crypto 加密后落库，接口永不回显。
package registries

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/crypto"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

type Registry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Status    string `json:"status"`
	Error     string `json:"error_message"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type registryInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Routes /api/v1/registries
func Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", list)
	r.Get("/{id}/images", images)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/", create)
	r.With(write).Put("/{id}", update)
	r.With(write).Delete("/{id}", remove)
	r.With(write).Post("/{id}/test", test)
	return r
}

const query = `SELECT id, name, type, url, COALESCE(username,''), COALESCE(status,'inactive'),
	COALESCE(error_message,''), COALESCE(created_at,''), COALESCE(updated_at,'') FROM image_registries`

func scan(row interface{ Scan(...any) error }) (Registry, error) {
	var r Registry
	err := row.Scan(&r.ID, &r.Name, &r.Type, &r.URL, &r.Username, &r.Status, &r.Error, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func list(w http.ResponseWriter, r *http.Request) {
	rows, err := appdb.DB().Query(query + ` ORDER BY created_at DESC`)
	if err != nil {
		httpx.RespondError(w, "读取镜像仓库失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []Registry{}
	for rows.Next() {
		if reg, err := scan(rows); err == nil {
			items = append(items, reg)
		}
	}
	httpx.Respond(w, items)
}

func create(w http.ResponseWriter, r *http.Request) {
	var in registryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.URL) == "" {
		httpx.RespondError(w, "名称和地址不能为空", http.StatusBadRequest)
		return
	}
	if in.Type == "" {
		in.Type = "generic"
	}
	enc, err := crypto.Encrypt(in.Password)
	if err != nil {
		httpx.RespondError(w, "密码加密失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	id := uuid.NewString()
	_, err = appdb.DB().Exec(`INSERT INTO image_registries
		(id,name,type,url,username,encrypted_password,encrypted_password_iv,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?, 'inactive', datetime('now','localtime'), datetime('now','localtime'))`,
		id, in.Name, in.Type, strings.TrimRight(in.URL, "/"), in.Username, enc, "")
	if err != nil {
		httpx.RespondError(w, "创建镜像仓库失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	reg, _ := scan(appdb.DB().QueryRow(query+` WHERE id = ?`, id))
	httpx.Respond(w, reg)
}

func update(w http.ResponseWriter, r *http.Request) {
	var in registryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.URL) == "" {
		httpx.RespondError(w, "名称和地址不能为空", http.StatusBadRequest)
		return
	}
	if in.Password != "" {
		enc, err := crypto.Encrypt(in.Password)
		if err != nil {
			httpx.RespondError(w, "密码加密失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = appdb.DB().Exec(`UPDATE image_registries SET name=?, type=?, url=?, username=?, encrypted_password=?,
			updated_at=datetime('now','localtime') WHERE id=?`,
			in.Name, in.Type, strings.TrimRight(in.URL, "/"), in.Username, enc, id)
		if err != nil {
			httpx.RespondError(w, "更新镜像仓库失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		_, err := appdb.DB().Exec(`UPDATE image_registries SET name=?, type=?, url=?, username=?,
			updated_at=datetime('now','localtime') WHERE id=?`,
			in.Name, in.Type, strings.TrimRight(in.URL, "/"), in.Username, id)
		if err != nil {
			httpx.RespondError(w, "更新镜像仓库失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	reg, _ := scan(appdb.DB().QueryRow(query+` WHERE id = ?`, id))
	httpx.Respond(w, reg)
}

func remove(w http.ResponseWriter, r *http.Request) {
	res, err := appdb.DB().Exec(`DELETE FROM image_registries WHERE id = ?`, chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "删除镜像仓库失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "镜像仓库不存在", http.StatusNotFound)
		return
	}
	httpx.RespondMessage(w, "镜像仓库已删除")
}

type credential struct {
	URL      string
	Username string
	Password string
}

func loadCredential(id string) (credential, error) {
	var c credential
	var enc sql.NullString
	err := appdb.DB().QueryRow(`SELECT url, COALESCE(username,''), COALESCE(encrypted_password,'') FROM image_registries WHERE id = ?`, id).
		Scan(&c.URL, &c.Username, &enc)
	if err != nil {
		return c, err
	}
	if enc.String != "" {
		if pwd, err := crypto.Decrypt(enc.String); err == nil {
			c.Password = pwd
		}
	}
	return c, nil
}

func test(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := loadCredential(id)
	if err != nil {
		httpx.RespondError(w, "镜像仓库不存在", http.StatusNotFound)
		return
	}
	status, msg := probe(c)
	_, _ = appdb.DB().Exec(`UPDATE image_registries SET status=?, error_message=?, updated_at=datetime('now','localtime') WHERE id=?`,
		status, msg, id)
	httpx.Respond(w, map[string]any{"status": status, "message": msg})
}

func probe(c credential) (string, string) {
	u, err := url.Parse(strings.TrimRight(c.URL, "/") + "/v2/")
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "error", "地址格式无效，需以 http:// 或 https:// 开头"
	}
	client := newClient()
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "error", "连接失败: " + err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	// 401 说明服务可达但需要认证；这已经是有效仓库。
	if resp.StatusCode < 500 {
		return "active", fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return "error", fmt.Sprintf("仓库返回 HTTP %d", resp.StatusCode)
}

func newClient() *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			// 企业内网仓库常用自签证书；连不上比证书校验失败更难排查。
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

// images 尽力列镜像：Harbor 走 projects API，通用 registry 走 /v2/_catalog。
// 任何失败都返回空数组而不是 500，页面至少能打开。
func images(w http.ResponseWriter, r *http.Request) {
	c, err := loadCredential(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "镜像仓库不存在", http.StatusNotFound)
		return
	}
	var typ string
	_ = appdb.DB().QueryRow(`SELECT type FROM image_registries WHERE id = ?`, chi.URLParam(r, "id")).Scan(&typ)
	out := []map[string]any{}
	client := newClient()
	base := strings.TrimRight(c.URL, "/")
	if typ == "harbor" {
		var projects []struct {
			Name string `json:"name"`
		}
		if getJSON(client, base+"/api/v2.0/projects?page_size=100", c, &projects) == nil {
			for _, p := range projects {
				out = append(out, map[string]any{"project": p.Name, "repository": p.Name, "tag": "", "size": ""})
			}
		}
	} else {
		var catalog struct {
			Repositories []string `json:"repositories"`
		}
		if getJSON(client, base+"/v2/_catalog", c, &catalog) == nil {
			for _, repo := range catalog.Repositories {
				out = append(out, map[string]any{"project": "", "repository": repo, "tag": "", "size": ""})
			}
		}
	}
	httpx.Respond(w, out)
}

func getJSON(client *http.Client, u string, c credential, out any) error {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
