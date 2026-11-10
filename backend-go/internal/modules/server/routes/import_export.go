// 服务器导入导出路由 — 对应 TS importExportRoutes.ts
// 挂载路径：/api/v1/import-export
// 提供：CSV 模板下载、CSV 批量导入、导出
package routes

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/repos"
)

// importExportColumnMap CSV 列名映射（中文/英文 → 标准字段）
var importExportColumnMap = map[string]string{
	"名称": "name", "服务器名称": "name", "name": "name", "主机名": "hostname", "hostname": "hostname",
	"IP地址": "hostname", "ip": "hostname", "ip地址": "hostname", "IP": "hostname",
	"SSH端口": "port", "端口": "port", "port": "port",
	"用户名": "username", "username": "username", "登录用户": "username",
	"密码": "password", "password": "password", "登录密码": "password",
	"私钥": "private_key", "private_key": "private_key", "SSH私钥": "private_key",
	"使用SSH密钥": "use_ssh_key", "use_ssh_key": "use_ssh_key",
	"描述": "description", "description": "description", "备注": "description",
	"标签": "tags", "tags": "tags", "tag": "tags",
	"系统类型": "os_type", "os_type": "os_type", "操作系统": "os_type",
	"分组": "group_name", "group": "group_name", "group_name": "group_name",
}

// ImportExportRoutes 导入导出路由
func ImportExportRoutes() chi.Router {
	r := chi.NewRouter()

	// GET /template/servers — 下载 CSV 导入模板
	r.Get("/template/servers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=servers-import-template.csv")

		// UTF-8 BOM 以便 Excel 正确识别中文
		w.Write([]byte{0xEF, 0xBB, 0xBF})

		cw := csv.NewWriter(w)
		// 表头
		headers := []string{"名称", "主机名", "SSH端口", "用户名", "密码", "私钥", "使用SSH密钥", "描述", "标签", "系统类型", "分组"}
		cw.Write(headers)
		// 示例数据（3 行）
		examples := [][]string{
			{"web-server-01", "192.168.1.10", "22", "root", "P@ssw0rd", "", "否", "Web 服务器", "web,prod", "linux", ""},
			{"db-server-01", "192.168.1.20", "22", "admin", "", "-----BEGIN OPENSSH PRIVATE KEY-----", "是", "数据库服务器", "db,prod", "linux", ""},
			{"win-server-01", "192.168.1.30", "22", "administrator", "Admin@123", "", "否", "Windows 服务器", "win,test", "windows", ""},
		}
		for _, row := range examples {
			cw.Write(row)
		}
		cw.Flush()
	})

	// POST /servers/import — CSV 批量导入
	r.With(middleware.RequireRole("admin")).Post("/servers/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CSVContent string `json:"csvContent"`
		}
		if !parseJSON(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.CSVContent) == "" {
			httpx.RespondError(w, "CSV 内容为空", http.StatusBadRequest)
			return
		}

		// 解析 CSV
		reader := csv.NewReader(strings.NewReader(req.CSVContent))
		reader.FieldsPerRecord = -1 // 允许变长行

		headerRow, err := reader.Read()
		if err != nil {
			httpx.RespondError(w, "CSV 解析失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		// 映射表头
		headers := make([]string, len(headerRow))
		for i, h := range headerRow {
			h = strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))
			if mapped, ok := importExportColumnMap[h]; ok {
				headers[i] = mapped
			} else {
				headers[i] = strings.ToLower(h)
			}
		}

		// 解析数据行
		var rows []map[string]string
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			m := make(map[string]string)
			for colIdx, cellVal := range record {
				if colIdx < len(headers) {
					m[headers[colIdx]] = strings.TrimSpace(cellVal)
				}
			}
			rows = append(rows, m)
		}

		result := importServerRows(rows)
		httpx.Respond(w, result)
	})

	// GET /servers/export — 导出服务器（CSV/JSON）
	r.Get("/servers/export", func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "csv"
		}

		list, err := repos.Servers.List()
		if err != nil {
			httpx.RespondError(w, "获取服务器列表失败", http.StatusInternalServerError)
			return
		}

		if format == "json" {
			buf, _ := json.MarshalIndent(list, "", "  ")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", "attachment; filename=servers-export.json")
			w.Write(buf)
			return
		}

		// CSV 导出
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=servers-export.csv")
		w.Write([]byte{0xEF, 0xBB, 0xBF})
		cw := csv.NewWriter(w)
		cw.Write([]string{"名称", "主机名", "SSH端口", "用户名", "系统类型", "描述", "标签", "状态"})
		for _, s := range list {
			tags := ""
			if s.Tags != nil {
				tags = *s.Tags
			}
			desc := ""
			if s.Description != nil {
				desc = *s.Description
			}
			osType := "linux"
			if s.OSType != nil {
				osType = *s.OSType
			}
			status := "启用"
			if s.Enabled == 0 {
				status = "禁用"
			}
			cw.Write([]string{s.Name, s.Hostname, fmt.Sprintf("%d", s.Port), s.Username, osType, desc, tags, status})
		}
		cw.Flush()
	})

	return r
}

// importServerRows 批量导入服务器行数据
func importServerRows(rows []map[string]string) map[string]interface{} {
	result := map[string]interface{}{
		"imported": 0,
		"failed":   0,
		"errors":   []string{},
	}
	errors := []string{}

	if len(rows) == 0 {
		result["errors"] = errors
		return result
	}

	imported := 0
	failed := 0

	for i, row := range rows {
		name := strings.TrimSpace(row["name"])
		hostname := strings.TrimSpace(row["hostname"])

		// 名称空时用主机名
		if name == "" {
			name = hostname
		}
		if name == "" || hostname == "" {
			failed++
			errors = append(errors, fmt.Sprintf("第 %d 行: 名称或主机名不能为空", i+2))
			continue
		}

		// 去重
		exists, _ := repos.Servers.ExistsByHostname(hostname)
		if exists {
			failed++
			errors = append(errors, fmt.Sprintf("第 %d 行: 主机名 %q 已存在", i+2, hostname))
			continue
		}

		port := 22
		if v := strings.TrimSpace(row["port"]); v != "" {
			if n, err := parseInt(v); err == nil && n > 0 {
				port = n
			}
		}

		username := strings.TrimSpace(row["username"])
		if username == "" {
			username = "root"
		}

		useKey := false
		switch strings.ToLower(strings.TrimSpace(row["use_ssh_key"])) {
		case "是", "true", "1", "yes", "y":
			useKey = true
		}

		osType := strings.ToLower(strings.TrimSpace(row["os_type"]))
		if osType != "windows" && osType != "linux" {
			osType = "linux"
		}

		// 查找分组
		groupID := ""
		if gname := strings.TrimSpace(row["group_name"]); gname != "" {
			groupID = findGroupByName(gname)
		}

		req := repos.ServerCreateInput{
			Name:        name,
			Hostname:    hostname,
			Port:        port,
			Username:    username,
			Password:    strings.TrimSpace(row["password"]),
			PrivateKey:  strings.TrimSpace(row["private_key"]),
			UseSSHKey:   useKey,
			Description: strings.TrimSpace(row["description"]),
			Tags:        parseTags(strings.TrimSpace(row["tags"])),
			OSType:      osType,
			GroupID:     groupID,
		}

		if _, err := repos.Servers.Create(req); err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("第 %d 行: 导入失败 %v", i+2, err))
			continue
		}
		imported++
	}

	result["imported"] = imported
	result["failed"] = failed
	if errors == nil {
		errors = []string{}
	}
	result["errors"] = errors
	return result
}

// findGroupByName 按名称查找分组 ID（找不到返回空）
func findGroupByName(name string) string {
	tree, err := repos.ServerGroups.Tree()
	if err != nil {
		return ""
	}
	var found string
	var walk func(groups []repos.ServerGroup)
	walk = func(groups []repos.ServerGroup) {
		for _, g := range groups {
			if g.Name == name {
				found = g.ID
				return
			}
			walk(g.Children)
		}
	}
	walk(tree)
	return found
}
