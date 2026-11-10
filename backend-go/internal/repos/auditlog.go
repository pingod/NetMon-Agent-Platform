package repos

import (
	"database/sql"
	"time"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// AuditLog 仓库（等价 auditLogRepository.ts）
// ───────────────────────────────────────────────

// AuditLogRecord audit_logs 表记录
type AuditLogRecord struct {
	ID           string  `json:"id"`
	UserID       *string `json:"user_id"`
	Action       string  `json:"action"`
	ResourceType string  `json:"resource_type"`
	ResourceID   *string `json:"resource_id"`
	Details      *string `json:"details"`
	IPAddress    *string `json:"ip_address"`
	CreatedAt    string  `json:"created_at"`
}

// AuditLogInsertInput 插入输入
type AuditLogInsertInput struct {
	ID           string
	UserID       *string
	Action       string
	ResourceType string
	ResourceID   *string
	Details      *string
	IPAddress    *string
}

// AuditLogListFilters 列表过滤
type AuditLogListFilters struct {
	Action       string
	ResourceType string
	UserID       string
	StartDate    string
	EndDate      string
	Limit        int
	Offset       int
}

type AuditLogRepo struct{}

var AuditLogs = &AuditLogRepo{}

func (r *AuditLogRepo) Insert(in AuditLogInsertInput) error {
	_, err := appdb.DB().Exec(`INSERT INTO audit_logs (id, user_id, action, resource_type, resource_id, details, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.ID, in.UserID, in.Action, in.ResourceType, in.ResourceID, in.Details, in.IPAddress)
	return err
}

func (r *AuditLogRepo) scan(rows *sql.Rows) []AuditLogRecord {
	var out []AuditLogRecord
	for rows.Next() {
		var a AuditLogRecord
		var uid, rid, det, ip sql.NullString
		if err := rows.Scan(&a.ID, &uid, &a.Action, &a.ResourceType, &rid, &det, &ip, &a.CreatedAt); err != nil {
			continue
		}
		a.UserID = ns(uid)
		a.ResourceID = ns(rid)
		a.Details = ns(det)
		a.IPAddress = ns(ip)
		out = append(out, a)
	}
	return out
}

func (r *AuditLogRepo) buildWhere(f AuditLogListFilters) (string, []interface{}) {
	q := " WHERE 1=1"
	var p []interface{}
	if f.Action != "" {
		q += " AND action = ?"
		p = append(p, f.Action)
	}
	if f.ResourceType != "" {
		q += " AND resource_type = ?"
		p = append(p, f.ResourceType)
	}
	if f.UserID != "" {
		q += " AND user_id = ?"
		p = append(p, f.UserID)
	}
	if f.StartDate != "" {
		q += " AND created_at >= ?"
		p = append(p, f.StartDate)
	}
	if f.EndDate != "" {
		q += " AND created_at <= ?"
		p = append(p, f.EndDate)
	}
	return q, p
}

func (r *AuditLogRepo) List(f AuditLogListFilters) ([]AuditLogRecord, error) {
	where, p := r.buildWhere(f)
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := appdb.DB().Query(`SELECT id, user_id, action, resource_type, resource_id, details, ip_address, created_at FROM audit_logs`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(p, limit, f.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scan(rows), nil
}

func (r *AuditLogRepo) Count(f AuditLogListFilters) int {
	where, p := r.buildWhere(f)
	var total int
	_ = appdb.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs`+where, p...).Scan(&total)
	return total
}

// ───────────────────────────────────────────────
// TokenBlacklist 仓库（等价 tokenBlacklistRepository.ts）
// ───────────────────────────────────────────────

type TokenBlacklistRepo struct{}

var TokenBlacklists = &TokenBlacklistRepo{}

func (r *TokenBlacklistRepo) Add(id, token string, userID, reason *string, expiresAt string) error {
	_, err := appdb.DB().Exec(`INSERT OR IGNORE INTO token_blacklist (id, token, user_id, reason, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, token, userID, reason, expiresAt)
	return err
}

// IsBlacklisted 判断 token 是否在黑名单且未过期
func (r *TokenBlacklistRepo) IsBlacklisted(token string) bool {
	var n int
	now := time.Now().Format("2006-01-02 15:04:05")
	err := appdb.DB().QueryRow(`SELECT COUNT(*) FROM token_blacklist WHERE token = ? AND expires_at > ?`, token, now).Scan(&n)
	return err == nil && n > 0
}

// CleanExpired 清理过期 token，返回删除数量
func (r *TokenBlacklistRepo) CleanExpired() int {
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := appdb.DB().Exec(`DELETE FROM token_blacklist WHERE expires_at <= ?`, now)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}
