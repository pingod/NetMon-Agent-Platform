// 服务器分组数据访问层 — 对应 TS serverGroupRepository.ts
// 数据库表：server_groups / server_group_mapping
package repos

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

var ServerGroups = &ServerGroupRepo{}

type ServerGroupRepo struct{}

// Tree 构建分组树（含每层 server_count / children_count）
func (ServerGroupRepo) Tree() ([]ServerGroup, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT sg.id, sg.name, sg.description, sg.parent_id, sg.sort_order, sg.created_at, sg.updated_at,
		       (SELECT COUNT(*) FROM server_group_mapping sgm WHERE sgm.group_id = sg.id) as server_count,
		       (SELECT COUNT(*) FROM server_groups c WHERE c.parent_id = sg.id) as children_count
		FROM server_groups sg
		ORDER BY sg.sort_order ASC, sg.created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type flatGroup struct {
		ServerGroup
		ParentID *string
	}
	var all []flatGroup
	for rows.Next() {
		var g flatGroup
		var desc, parent sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &desc, &parent, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt,
			&g.ServerCount, &g.ChildrenCount); err != nil {
			continue
		}
		g.Description = nullStrToPtr(desc)
		g.ParentID = nullStrToPtr(parent)
		all = append(all, g)
	}

	// 构建树
	byID := map[string]*flatGroup{}
	for i := range all {
		byID[all[i].ID] = &all[i]
	}
	var roots []ServerGroup
	for i := range all {
		g := &all[i]
		child := ServerGroup{
			ID: g.ID, Name: g.Name, Description: g.Description,
			SortOrder: g.SortOrder, ServerCount: g.ServerCount,
			ChildrenCount: g.ChildrenCount,
			CreatedAt:     g.CreatedAt, UpdatedAt: g.UpdatedAt,
		}
		if g.ParentID != nil && *g.ParentID != "" {
			if parent, ok := byID[*g.ParentID]; ok {
				parent.Children = append(parent.Children, child)
				continue
			}
		}
		roots = append(roots, child)
	}
	if roots == nil {
		roots = []ServerGroup{}
	}
	return roots, nil
}

// Create 创建分组
func (ServerGroupRepo) Create(name, description, parentID string) (*ServerGroup, error) {
	db := appdb.DB()
	id := uuid.New().String()
	now := nowLocal()
	_, err := db.Exec(`
		INSERT INTO server_groups (id, name, description, parent_id, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, name, emptyToNull(description), emptyToNull(parentID), now, now)
	if err != nil {
		return nil, err
	}
	return ServerGroups.GetByID(id)
}

// Update 更新分组
func (ServerGroupRepo) Update(id, name string, description, parentID *string) (*ServerGroup, error) {
	db := appdb.DB()
	_, err := db.Exec(`
		UPDATE server_groups SET name=?, description=?, parent_id=?, updated_at=? WHERE id=?`,
		name, nullPtrToAny(description), nullPtrToAny(parentID), nowLocal(), id)
	if err != nil {
		return nil, err
	}
	return ServerGroups.GetByID(id)
}

// GetByID 获取单个分组
func (ServerGroupRepo) GetByID(id string) (*ServerGroup, error) {
	db := appdb.DB()
	var g ServerGroup
	var desc, parent sql.NullString
	err := db.QueryRow(`
		SELECT sg.id, sg.name, sg.description, sg.parent_id, sg.sort_order, sg.created_at, sg.updated_at,
		       (SELECT COUNT(*) FROM server_group_mapping sgm WHERE sgm.group_id = sg.id) as server_count
		FROM server_groups sg WHERE sg.id = ?`, id).
		Scan(&g.ID, &g.Name, &desc, &parent, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt, &g.ServerCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Description = nullStrToPtr(desc)
	g.ParentID = nullStrToPtr(parent)
	return &g, nil
}

// nullPtrToAny 将 *string 转为 SQL 参数（空字符串 → NULL）
func nullPtrToAny(p *string) interface{} {
	if p == nil {
		return nil
	}
	return emptyToNull(*p)
}
