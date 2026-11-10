// Copilot 对话记录数据访问层
// 数据库表：copilot_conversations
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// CopilotMessage 消息实体
type CopilotMessage struct {
	Role      string `json:"role"`      // 'user' | 'assistant'
	Content   string `json:"content"`   // 消息内容
	Timestamp string `json:"timestamp"` // ISO 或时间字符串
}

// CopilotConversation 会话实体
type CopilotConversation struct {
	ID        string           `json:"id"`
	UserID    string           `json:"user_id"`
	Messages  []CopilotMessage `json:"messages"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

var CopilotConversations = &CopilotConversationRepo{}

type CopilotConversationRepo struct{}

func ensureCopilotConversationsSchema(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS copilot_conversations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			messages TEXT NOT NULL,
			created_at DATETIME DEFAULT (datetime('now','localtime')),
			updated_at DATETIME DEFAULT (datetime('now','localtime'))
		)
	`)
}

// List 列出用户的所有会话
func (r *CopilotConversationRepo) List(userID string) ([]*CopilotConversation, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	ensureCopilotConversationsSchema(db)

	query := `SELECT id, user_id, messages, created_at, updated_at FROM copilot_conversations`
	var rows *sql.Rows
	var err error
	if userID != "" {
		query += ` WHERE user_id = ? ORDER BY updated_at DESC`
		rows, err = db.Query(query, userID)
	} else {
		query += ` ORDER BY updated_at DESC`
		rows, err = db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*CopilotConversation
	for rows.Next() {
		var c CopilotConversation
		var msgsJSON string
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.UserID, &msgsJSON, &createdAt, &updatedAt); err != nil {
			continue
		}
		if createdAt.Valid {
			c.CreatedAt = createdAt.String
		}
		if updatedAt.Valid {
			c.UpdatedAt = updatedAt.String
		}
		if msgsJSON != "" {
			_ = json.Unmarshal([]byte(msgsJSON), &c.Messages)
		}
		if c.Messages == nil {
			c.Messages = []CopilotMessage{}
		}
		list = append(list, &c)
	}
	if list == nil {
		list = []*CopilotConversation{}
	}
	return list, nil
}

// GetByID 根据 ID 查询单个会话
func (r *CopilotConversationRepo) GetByID(id string) (*CopilotConversation, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	ensureCopilotConversationsSchema(db)

	var c CopilotConversation
	var msgsJSON string
	var createdAt, updatedAt sql.NullString
	err := db.QueryRow(`SELECT id, user_id, messages, created_at, updated_at FROM copilot_conversations WHERE id = ?`, id).
		Scan(&c.ID, &c.UserID, &msgsJSON, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if createdAt.Valid {
		c.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		c.UpdatedAt = updatedAt.String
	}
	if msgsJSON != "" {
		_ = json.Unmarshal([]byte(msgsJSON), &c.Messages)
	}
	if c.Messages == nil {
		c.Messages = []CopilotMessage{}
	}
	return &c, nil
}

// Create 创建新会话
func (r *CopilotConversationRepo) Create(userID string) (*CopilotConversation, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	ensureCopilotConversationsSchema(db)

	if userID == "" {
		userID = "admin"
	}
	id := uuid.New().String()
	now := time.Now().Format("2006-01-02 15:04:05")
	emptyMessages := "[]"

	_, err := db.Exec(`
		INSERT INTO copilot_conversations (id, user_id, messages, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, userID, emptyMessages, now, now,
	)
	if err != nil {
		return nil, err
	}

	return &CopilotConversation{
		ID:        id,
		UserID:    userID,
		Messages:  []CopilotMessage{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// AppendMessages 向会话批量追加消息并更新 updated_at
func (r *CopilotConversationRepo) AppendMessages(id string, newMsgs ...CopilotMessage) (*CopilotConversation, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	ensureCopilotConversationsSchema(db)

	conv, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		// 自动创建
		conv, err = r.Create("admin")
		if err != nil {
			return nil, err
		}
		id = conv.ID
	}

	conv.Messages = append(conv.Messages, newMsgs...)
	data, err := json.Marshal(conv.Messages)
	if err != nil {
		return nil, err
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.Exec(`UPDATE copilot_conversations SET messages = ?, updated_at = ? WHERE id = ?`,
		string(data), now, id)
	if err != nil {
		return nil, err
	}

	conv.UpdatedAt = now
	return conv, nil
}

// Delete 删除指定会话
func (r *CopilotConversationRepo) Delete(id string) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	ensureCopilotConversationsSchema(db)

	_, err := db.Exec(`DELETE FROM copilot_conversations WHERE id = ?`, id)
	return err
}
