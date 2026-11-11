// VM 快照策略（/api/v1/snapshot-policies）。
// 前端容器模块的「快照策略」页此前 404；这里补 CRUD，字段与页面保持 camelCase。
package ops

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
)

type SnapshotPolicy struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PlatformID     string `json:"platformId"`
	VMID           string `json:"vmId"`
	CronExpression string `json:"cronExpression"`
	Retention      int    `json:"retention"`
	SnapshotMemory bool   `json:"snapshotMemory"`
	Enabled        bool   `json:"enabled"`
	LastRunAt      string `json:"lastRunAt"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type snapshotPolicyInput struct {
	Name           string `json:"name"`
	PlatformID     string `json:"platformId"`
	VMID           string `json:"vmId"`
	CronExpression string `json:"cronExpression"`
	Retention      int    `json:"retention"`
	SnapshotMemory bool   `json:"snapshotMemory"`
	Enabled        bool   `json:"enabled"`
}

func SnapshotPolicyRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", snapshotPolicyList)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/", snapshotPolicyCreate)
	r.With(write).Put("/{id}", snapshotPolicyUpdate)
	r.With(write).Delete("/{id}", snapshotPolicyDelete)
	return r
}

const snapshotPolicyQuery = `SELECT id, name, COALESCE(platform_id,''), COALESCE(vm_id,''),
	COALESCE(cron_expression,''), COALESCE(retention,7), COALESCE(snapshot_memory,1), COALESCE(enabled,1),
	COALESCE(last_run_at,''), COALESCE(created_at,''), COALESCE(updated_at,'') FROM vm_snapshot_policies`

func scanSnapshotPolicy(row interface{ Scan(...any) error }) (SnapshotPolicy, error) {
	var p SnapshotPolicy
	var mem, enabled int
	err := row.Scan(&p.ID, &p.Name, &p.PlatformID, &p.VMID, &p.CronExpression, &p.Retention, &mem, &enabled,
		&p.LastRunAt, &p.CreatedAt, &p.UpdatedAt)
	p.SnapshotMemory = mem == 1
	p.Enabled = enabled == 1
	return p, err
}

func snapshotPolicyList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	db := appdb.DB()
	where, args := "", []any{}
	if q := strings.TrimSpace(r.URL.Query().Get("search")); q != "" {
		where = " WHERE name LIKE ?"
		args = append(args, "%"+q+"%")
	}
	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM vm_snapshot_policies`+where, args...).Scan(&total)
	rows, err := db.Query(snapshotPolicyQuery+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		httpx.RespondError(w, "读取快照策略失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []SnapshotPolicy{}
	for rows.Next() {
		if p, err := scanSnapshotPolicy(rows); err == nil {
			items = append(items, p)
		}
	}
	httpx.RespondRaw(w, http.StatusOK, map[string]any{"success": true, "data": items, "total": total})
}

func snapshotPolicyCreate(w http.ResponseWriter, r *http.Request) {
	var in snapshotPolicyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		httpx.RespondError(w, "策略名称不能为空", http.StatusBadRequest)
		return
	}
	if in.CronExpression == "" {
		in.CronExpression = "0 2 * * *"
	}
	if in.Retention <= 0 {
		in.Retention = 7
	}
	id := uuid.NewString()
	_, err := appdb.DB().Exec(`INSERT INTO vm_snapshot_policies
		(id,name,platform_id,vm_id,cron_expression,retention,snapshot_memory,enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,datetime('now','localtime'),datetime('now','localtime'))`,
		id, in.Name, in.PlatformID, in.VMID, in.CronExpression, in.Retention, boolInt(in.SnapshotMemory), boolInt(in.Enabled))
	if err != nil {
		httpx.RespondError(w, "创建快照策略失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	p, _ := scanSnapshotPolicy(appdb.DB().QueryRow(snapshotPolicyQuery+` WHERE id = ?`, id))
	httpx.Respond(w, p)
}

func snapshotPolicyUpdate(w http.ResponseWriter, r *http.Request) {
	var in snapshotPolicyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RespondError(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(in.Name) == "" {
		httpx.RespondError(w, "策略名称不能为空", http.StatusBadRequest)
		return
	}
	if in.CronExpression == "" {
		in.CronExpression = "0 2 * * *"
	}
	if in.Retention <= 0 {
		in.Retention = 7
	}
	res, err := appdb.DB().Exec(`UPDATE vm_snapshot_policies SET name=?, platform_id=?, vm_id=?, cron_expression=?,
		retention=?, snapshot_memory=?, enabled=?, updated_at=datetime('now','localtime') WHERE id=?`,
		in.Name, in.PlatformID, in.VMID, in.CronExpression, in.Retention, boolInt(in.SnapshotMemory), boolInt(in.Enabled), id)
	if err != nil {
		httpx.RespondError(w, "更新快照策略失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "快照策略不存在", http.StatusNotFound)
		return
	}
	p, _ := scanSnapshotPolicy(appdb.DB().QueryRow(snapshotPolicyQuery+` WHERE id = ?`, id))
	httpx.Respond(w, p)
}

func snapshotPolicyDelete(w http.ResponseWriter, r *http.Request) {
	res, err := appdb.DB().Exec(`DELETE FROM vm_snapshot_policies WHERE id = ?`, chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, "删除快照策略失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RespondError(w, "快照策略不存在", http.StatusNotFound)
		return
	}
	httpx.RespondMessage(w, "快照策略已删除")
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
