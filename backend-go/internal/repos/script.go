package repos

import (
	"database/sql"
	"encoding/json"
	"strings"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// Script 仓库（脚本中心 /api/v1/scripts）
// ───────────────────────────────────────────────

// ScriptParameter 脚本参数定义
type ScriptParameter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ScriptRecord 脚本完整记录（parameters 以 JSON 存储，读出时解析为数组）
type ScriptRecord struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Content     string            `json:"content"`
	Parameters  []ScriptParameter `json:"parameters"`
	Category    string            `json:"category"`
	Version     int               `json:"version"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// ScriptInput 创建/更新输入
type ScriptInput struct {
	Name        string
	Description string
	Type        string
	Content     string
	Parameters  []ScriptParameter
	Category    string
}

type ScriptRepo struct{}

var Scripts = &ScriptRepo{}

const scriptColumns = `id, name, description, type, content, parameters, category, version, created_at, updated_at`

func scanScript(row interface{ Scan(...interface{}) error }) (*ScriptRecord, error) {
	var s ScriptRecord
	var desc, params, category sql.NullString
	if err := row.Scan(&s.ID, &s.Name, &desc, &s.Type, &s.Content, &params, &category, &s.Version, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Description = nsStr(desc)
	s.Category = nsStr(category)
	s.Parameters = []ScriptParameter{}
	if params.Valid && strings.TrimSpace(params.String) != "" {
		_ = json.Unmarshal([]byte(params.String), &s.Parameters)
	}
	if s.Parameters == nil {
		s.Parameters = []ScriptParameter{}
	}
	return &s, nil
}

// List 按名称/描述模糊搜索 + 分类过滤
func (r *ScriptRepo) List(search, category string) ([]ScriptRecord, error) {
	q := `SELECT ` + scriptColumns + ` FROM scripts`
	var conds []string
	var vals []interface{}
	if search != "" {
		conds = append(conds, "(name LIKE ? OR description LIKE ?)")
		vals = append(vals, "%"+search+"%", "%"+search+"%")
	}
	if category != "" {
		conds = append(conds, "category = ?")
		vals = append(vals, category)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY updated_at DESC"
	rows, err := appdb.DB().Query(q, vals...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScriptRecord{}
	for rows.Next() {
		s, err := scanScript(rows)
		if err != nil {
			continue
		}
		out = append(out, *s)
	}
	return out, nil
}

// Categories 全部非空分类（去重排序）
func (r *ScriptRepo) Categories() ([]string, error) {
	rows, err := appdb.DB().Query(`SELECT DISTINCT category FROM scripts WHERE category IS NOT NULL AND category != '' ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *ScriptRepo) GetByID(id string) (*ScriptRecord, error) {
	return scanScript(appdb.DB().QueryRow(`SELECT `+scriptColumns+` FROM scripts WHERE id = ?`, id))
}

func (r *ScriptRepo) Create(id string, in ScriptInput) error {
	params, _ := json.Marshal(in.Parameters)
	now := nowLocal()
	_, err := appdb.DB().Exec(`INSERT INTO scripts (id, name, description, type, content, parameters, category, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, in.Name, in.Description, in.Type, in.Content, string(params), in.Category, now, now)
	return err
}

// Update 更新内容并递增版本号
func (r *ScriptRepo) Update(id string, in ScriptInput) (int, error) {
	params, _ := json.Marshal(in.Parameters)
	res, err := appdb.DB().Exec(`UPDATE scripts SET name = ?, description = ?, type = ?, content = ?, parameters = ?, category = ?,
		version = version + 1, updated_at = ? WHERE id = ?`,
		in.Name, in.Description, in.Type, in.Content, string(params), in.Category, nowLocal(), id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *ScriptRepo) Delete(id string) (bool, error) {
	res, err := appdb.DB().Exec(`DELETE FROM scripts WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}