// Package audit 审计日志服务，等价于 TS modules/infra/services/auditService.ts
package audit

import (
	"encoding/json"

	"github.com/google/uuid"

	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

// AuditInput 审计日志输入
type AuditInput struct {
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	Details      interface{} // 将被 JSON 序列化
	IPAddress    string
}

// CreateAuditLog 写入审计日志，返回日志 ID（等价 createAuditLog）
func CreateAuditLog(in AuditInput) string {
	id := uuid.NewString()

	var userID, resourceID, details, ip *string
	if in.UserID != "" {
		v := in.UserID
		userID = &v
	}
	if in.ResourceID != "" {
		v := in.ResourceID
		resourceID = &v
	}
	if in.IPAddress != "" {
		v := in.IPAddress
		ip = &v
	}
	if in.Details != nil {
		if b, err := json.Marshal(in.Details); err == nil {
			s := string(b)
			details = &s
		}
	}

	if err := repos.AuditLogs.Insert(repos.AuditLogInsertInput{
		ID:           id,
		UserID:       userID,
		Action:       in.Action,
		ResourceType: in.ResourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    ip,
	}); err != nil {
		logger.Errorf("Failed to create audit log: %v", err)
		return ""
	}
	return id
}
