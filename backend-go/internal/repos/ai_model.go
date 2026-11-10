// AI Model 数据访问层 — 对应 TS aiModelRepository.ts
// 数据库表：ai_models
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// AIModel AI 模型实体
type AIModel struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ProviderType   string  `json:"provider_type"`
	APIKey         *string `json:"api_key,omitempty"`
	APIBase        *string `json:"api_base,omitempty"`
	ModelID        string  `json:"model_id"`
	Enabled        int     `json:"enabled"`
	SortOrder      int     `json:"sort_order"`
	IsDefault      int     `json:"is_default"`
	Tags           *string `json:"-"`
	LastTestStatus *string `json:"last_test_status,omitempty"`
	LastTestTime   *string `json:"last_test_time,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// MarshalJSON 自定义序列化：tags 从逗号分隔字符串转为 string[]
func (m AIModel) MarshalJSON() ([]byte, error) {
	var tags []string
	if m.Tags != nil && *m.Tags != "" {
		for _, t := range splitTags(*m.Tags) {
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	if tags == nil {
		tags = []string{}
	}

	aux := struct {
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		ProviderType   string   `json:"provider_type"`
		APIKey         *string  `json:"api_key,omitempty"`
		APIBase        *string  `json:"api_base,omitempty"`
		ModelID        string   `json:"model_id"`
		Enabled        int      `json:"enabled"`
		SortOrder      int      `json:"sort_order"`
		IsDefault      int      `json:"is_default"`
		Tags           []string `json:"tags"`
		LastTestStatus *string  `json:"last_test_status,omitempty"`
		LastTestTime   *string  `json:"last_test_time,omitempty"`
		CreatedAt      string   `json:"created_at"`
		UpdatedAt      string   `json:"updated_at"`
	}{
		ID:             m.ID,
		Name:           m.Name,
		ProviderType:   m.ProviderType,
		APIKey:         m.APIKey,
		APIBase:        m.APIBase,
		ModelID:        m.ModelID,
		Enabled:        m.Enabled,
		SortOrder:      m.SortOrder,
		IsDefault:      m.IsDefault,
		Tags:           tags,
		LastTestStatus: m.LastTestStatus,
		LastTestTime:   m.LastTestTime,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
	return json.Marshal(aux)
}

// CreateAIModelInput 创建模型请求输入
type CreateAIModelInput struct {
	Name         string   `json:"name"`
	ProviderType string   `json:"provider_type"`
	ModelID      string   `json:"model_id"`
	APIKey       *string  `json:"api_key"`
	APIBase      *string  `json:"api_base"`
	Tags         []string `json:"tags"`
}

// UpdateAIModelInput 更新模型请求输入
type UpdateAIModelInput struct {
	Name         *string   `json:"name"`
	ProviderType *string   `json:"provider_type"`
	ModelID      *string   `json:"model_id"`
	APIKey       *string   `json:"api_key"`
	APIBase      *string   `json:"api_base"`
	Enabled      *int      `json:"enabled"`
	IsDefault    *int      `json:"is_default"`
	Tags         *[]string `json:"tags"`
}

var AIModels = &AIModelRepo{}

type AIModelRepo struct{}

// List 列出所有 AI 模型
func (AIModelRepo) List() ([]AIModel, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT id, name, provider_type, api_key, api_base, model_id, enabled, sort_order, is_default, tags, last_test_status, last_test_time, created_at, updated_at
		FROM ai_models ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AIModel
	for rows.Next() {
		var m AIModel
		var apiKey, apiBase, tags, lastTestStatus, lastTestTime sql.NullString
		if err := rows.Scan(
			&m.ID, &m.Name, &m.ProviderType, &apiKey, &apiBase, &m.ModelID,
			&m.Enabled, &m.SortOrder, &m.IsDefault, &tags,
			&lastTestStatus, &lastTestTime, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			continue
		}
		m.APIKey = nullStrToPtr(apiKey)
		m.APIBase = nullStrToPtr(apiBase)
		m.Tags = nullStrToPtr(tags)
		m.LastTestStatus = nullStrToPtr(lastTestStatus)
		m.LastTestTime = nullStrToPtr(lastTestTime)
		items = append(items, m)
	}
	if items == nil {
		items = []AIModel{}
	}
	return items, nil
}

// GetByID 根据 ID 查询 AI 模型
func (AIModelRepo) GetByID(id string) (*AIModel, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var m AIModel
	var apiKey, apiBase, tags, lastTestStatus, lastTestTime sql.NullString
	err := db.QueryRow(`
		SELECT id, name, provider_type, api_key, api_base, model_id, enabled, sort_order, is_default, tags, last_test_status, last_test_time, created_at, updated_at
		FROM ai_models WHERE id = ?`, id).
		Scan(
			&m.ID, &m.Name, &m.ProviderType, &apiKey, &apiBase, &m.ModelID,
			&m.Enabled, &m.SortOrder, &m.IsDefault, &tags,
			&lastTestStatus, &lastTestTime, &m.CreatedAt, &m.UpdatedAt,
		)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	m.APIKey = nullStrToPtr(apiKey)
	m.APIBase = nullStrToPtr(apiBase)
	m.Tags = nullStrToPtr(tags)
	m.LastTestStatus = nullStrToPtr(lastTestStatus)
	m.LastTestTime = nullStrToPtr(lastTestTime)
	return &m, nil
}

// Create 创建新 AI 模型
func (AIModelRepo) Create(input CreateAIModelInput) (*AIModel, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	id := uuid.New().String()
	now := time.Now().Format("2006-01-02 15:04:05")

	var maxSort int
	_ = db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM ai_models").Scan(&maxSort)
	sortOrder := maxSort + 1

	tagsStr := ""
	if len(input.Tags) > 0 {
		tagsStr = strings.Join(input.Tags, ",")
	}

	_, err := db.Exec(`
		INSERT INTO ai_models (id, name, provider_type, api_key, api_base, model_id, enabled, sort_order, is_default, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, 0, ?, ?, ?)`,
		id, input.Name, input.ProviderType, input.APIKey, input.APIBase, input.ModelID, sortOrder, tagsStr, now, now,
	)
	if err != nil {
		return nil, err
	}

	return AIModels.GetByID(id)
}

// Update 更新 AI 模型
func (AIModelRepo) Update(id string, input UpdateAIModelInput) (*AIModel, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	existing, err := AIModels.GetByID(id)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("AI model not found")
	}

	if input.IsDefault != nil && *input.IsDefault == 1 {
		_, _ = db.Exec("UPDATE ai_models SET is_default = 0")
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}
	providerType := existing.ProviderType
	if input.ProviderType != nil {
		providerType = *input.ProviderType
	}
	modelID := existing.ModelID
	if input.ModelID != nil {
		modelID = *input.ModelID
	}
	apiKey := existing.APIKey
	if input.APIKey != nil {
		apiKey = input.APIKey
	}
	apiBase := existing.APIBase
	if input.APIBase != nil {
		apiBase = input.APIBase
	}
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	isDefault := existing.IsDefault
	if input.IsDefault != nil {
		isDefault = *input.IsDefault
	}

	tagsStr := ""
	if existing.Tags != nil {
		tagsStr = *existing.Tags
	}
	if input.Tags != nil {
		tagsStr = strings.Join(*input.Tags, ",")
	}

	_, err = db.Exec(`
		UPDATE ai_models SET name = ?, provider_type = ?, model_id = ?, api_key = ?, api_base = ?, enabled = ?, is_default = ?, tags = ?, updated_at = ?
		WHERE id = ?`,
		name, providerType, modelID, apiKey, apiBase, enabled, isDefault, tagsStr, now, id,
	)
	if err != nil {
		return nil, err
	}

	return AIModels.GetByID(id)
}

// Delete 删除 AI 模型
func (AIModelRepo) Delete(id string) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM ai_models WHERE id = ?", id)
	return err
}

// Reorder 重新排序 AI 模型
func (AIModelRepo) Reorder(modelIDs []string) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for idx, id := range modelIDs {
		_, err := tx.Exec("UPDATE ai_models SET sort_order = ? WHERE id = ?", idx+1, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateTestStatus 更新测试状态
func (AIModelRepo) UpdateTestStatus(id string, status string) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE ai_models SET last_test_status = ?, last_test_time = ? WHERE id = ?", status, now, id)
	return err
}
