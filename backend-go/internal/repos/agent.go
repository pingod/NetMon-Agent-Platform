// AI Agent 数据访问层 — 对应 TS agentRepository.ts
// 数据库表：agents
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"itops-backend/internal/appdb"
)

// splitTags 将逗号分隔字符串拆分为切片
func splitTags(s string) []string {
	s = strings.TrimSpace(s)
	// 处理 JSON 格式的空数组字符串
	if s == "[]" || s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// Agent AI Agent 实体（列表视图）
type Agent struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Avatar      *string     `json:"avatar,omitempty"`
	Role        *string     `json:"role,omitempty"`
	Enabled     int         `json:"enabled"`
	IsPreset    int         `json:"is_preset"`
	Category    *string     `json:"category,omitempty"`
	Tags        *string     `json:"-"` // 内部字段，逗号分隔字符串；通过 MarshalJSON 输出为数组
	Description *string     `json:"description,omitempty"`
	UsageCount  int         `json:"usage_count"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`
}

// MarshalJSON 自定义序列化：tags 从逗号分隔字符串转为 string[]
func (a Agent) MarshalJSON() ([]byte, error) {
	type Alias Agent
	var tags []string
	if a.Tags != nil && *a.Tags != "" {
		for _, t := range splitTags(*a.Tags) {
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	if tags == nil {
		tags = []string{}
	}

	// 避免递归调用 — 手动序列化
	type Alias2 Agent
	aux2 := &struct {
		ID          string      `json:"id"`
		Name        string      `json:"name"`
		Avatar      *string     `json:"avatar,omitempty"`
		Role        *string     `json:"role,omitempty"`
		Enabled     int         `json:"enabled"`
		IsPreset    int         `json:"is_preset"`
		Category    *string     `json:"category,omitempty"`
		Tags        []string    `json:"tags"`
		Description *string     `json:"description,omitempty"`
		UsageCount  int         `json:"usage_count"`
		CreatedAt   string      `json:"created_at"`
		UpdatedAt   string      `json:"updated_at"`
	}{
		ID:          a.ID,
		Name:        a.Name,
		Avatar:      a.Avatar,
		Role:        a.Role,
		Enabled:     a.Enabled,
		IsPreset:    a.IsPreset,
		Category:    a.Category,
		Tags:        tags,
		Description: a.Description,
		UsageCount:  a.UsageCount,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
	return json.Marshal(aux2)
}

var Agents = &AgentRepo{}

type AgentRepo struct{}

// List 列出所有 Agent
func (AgentRepo) List() ([]Agent, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`
		SELECT id, name, avatar, role, enabled, is_preset, category, tags, description, usage_count, created_at, updated_at
		FROM agents ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Agent
	for rows.Next() {
		var a Agent
		var avatar, role, category, tags, desc sql.NullString
		if err := rows.Scan(&a.ID, &a.Name, &avatar, &role, &a.Enabled, &a.IsPreset,
			&category, &tags, &desc, &a.UsageCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		a.Avatar = nullStrToPtr(avatar)
		a.Role = nullStrToPtr(role)
		a.Category = nullStrToPtr(category)
		a.Tags = nullStrToPtr(tags)
		a.Description = nullStrToPtr(desc)
		items = append(items, a)
	}
	if items == nil {
		items = []Agent{}
	}
	return items, nil
}

// GetEnabled 获取已启用的第一个 Agent（AI 生成命令用）
func (AgentRepo) GetEnabled() (*Agent, error) {
	db := appdb.DB()
	var a Agent
	var avatar, role, category, tags, desc sql.NullString
	err := db.QueryRow(`
		SELECT id, name, avatar, role, enabled, is_preset, category, tags, description, usage_count, created_at, updated_at
		FROM agents WHERE enabled = 1 ORDER BY created_at ASC LIMIT 1`).
		Scan(&a.ID, &a.Name, &avatar, &role, &a.Enabled, &a.IsPreset,
			&category, &tags, &desc, &a.UsageCount, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Avatar = nullStrToPtr(avatar)
	a.Role = nullStrToPtr(role)
	a.Category = nullStrToPtr(category)
	a.Tags = nullStrToPtr(tags)
	a.Description = nullStrToPtr(desc)
	return &a, nil
}

// IncrementUsage 增加使用次数
func (AgentRepo) IncrementUsage(id string) {
	db := appdb.DB()
	if db == nil {
		return
	}
	_, _ = db.Exec(`UPDATE agents SET usage_count = usage_count + 1, last_used_at = ?, updated_at = ? WHERE id = ?`,
		nowLocal(), nowLocal(), id)
}
