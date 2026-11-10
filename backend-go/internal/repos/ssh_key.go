// SSH 密钥/凭证数据访问层 — 对应 TS sshKeys.ts
// 数据库表：ssh_keys（v005 后支持 auth_type='password'）
package repos

import (
	"database/sql"
	"fmt"
	"time"

	"itops-backend/internal/appdb"
)

// SshKey SSH 密钥/凭证
type SshKey struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	AuthType    *string `json:"auth_type"`
	KeyType     *string `json:"key_type"`
	Fingerprint *string `json:"fingerprint"`
	Username    *string `json:"username"`
	Password    *string `json:"password"`     // 加密密文，列表时为空
	PrivateKey  *string `json:"private_key"`  // 加密密文，列表时为空
	Description *string `json:"description"`
	UsageCount  int     `json:"usage_count,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

var SshKeys = &SshKeyRepo{}

type SshKeyRepo struct{}

// List 列出所有凭证（不含敏感字段），带 usage_count
func (SshKeyRepo) List() ([]SshKey, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT sk.id, sk.name, sk.auth_type, sk.key_type, sk.fingerprint,
		       sk.username, sk.description, sk.created_at, sk.updated_at,
		       COUNT(DISTINCT s.id) + COUNT(DISTINCT nd.id) as usage_count
		FROM ssh_keys sk
		LEFT JOIN servers s ON s.ssh_key_id = sk.id
		LEFT JOIN network_devices nd ON nd.ssh_key_id = sk.id
		GROUP BY sk.id
		ORDER BY sk.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SshKey
	for rows.Next() {
		var k SshKey
		var authType, keyType, fingerprint, username, desc sql.NullString
		err := rows.Scan(&k.ID, &k.Name, &authType, &keyType, &fingerprint,
			&username, &desc, &k.CreatedAt, &k.UpdatedAt, &k.UsageCount)
		if err != nil {
			continue
		}
		k.AuthType = nullStrToPtr(authType)
		k.KeyType = nullStrToPtr(keyType)
		k.Fingerprint = nullStrToPtr(fingerprint)
		k.Username = nullStrToPtr(username)
		k.Description = nullStrToPtr(desc)
		items = append(items, k)
	}
	return items, nil
}

// GetByID 获取单个凭证（含加密的敏感字段）
func (SshKeyRepo) GetByID(id string) (*SshKey, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var k SshKey
	var authType, keyType, fingerprint, username, pwd, privKey, desc sql.NullString
	err := db.QueryRow(`
		SELECT id, name, auth_type, key_type, fingerprint, username, password,
		       private_key, description, created_at, updated_at
		FROM ssh_keys WHERE id = ?`, id).
		Scan(&k.ID, &k.Name, &authType, &keyType, &fingerprint, &username, &pwd,
			&privKey, &desc, &k.CreatedAt, &k.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	k.AuthType = nullStrToPtr(authType)
	k.KeyType = nullStrToPtr(keyType)
	k.Fingerprint = nullStrToPtr(fingerprint)
	k.Username = nullStrToPtr(username)
	k.Password = nullStrToPtr(pwd)
	k.PrivateKey = nullStrToPtr(privKey)
	k.Description = nullStrToPtr(desc)
	return &k, nil
}

// FindByName 按名称查找
func (SshKeyRepo) FindByName(name string) (*SshKey, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var k SshKey
	var authType, keyType, fingerprint, username, desc sql.NullString
	err := db.QueryRow(`
		SELECT id, name, auth_type, key_type, fingerprint, username, description, created_at, updated_at
		FROM ssh_keys WHERE name = ?`, name).
		Scan(&k.ID, &k.Name, &authType, &keyType, &fingerprint, &username, &desc, &k.CreatedAt, &k.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	k.AuthType = nullStrToPtr(authType)
	k.KeyType = nullStrToPtr(keyType)
	k.Fingerprint = nullStrToPtr(fingerprint)
	k.Username = nullStrToPtr(username)
	k.Description = nullStrToPtr(desc)
	return &k, nil
}

// CreateKey 创建 SSH 密钥类型凭证
func (SshKeyRepo) CreateKey(id, name, keyType, fingerprint, encryptedPrivateKey, description string) error {
	db := appdb.DB()
	_, err := db.Exec(`
		INSERT INTO ssh_keys (id, name, auth_type, key_type, fingerprint, private_key, description)
		VALUES (?, ?, 'key', ?, ?, ?, ?)`,
		id, name, keyType, fingerprint, encryptedPrivateKey, emptyToNull(description))
	return err
}

// CreatePassword 创建账号密码类型凭证
func (SshKeyRepo) CreatePassword(id, name, username, encryptedPassword, description string) error {
	db := appdb.DB()
	_, err := db.Exec(`
		INSERT INTO ssh_keys (id, name, auth_type, key_type, username, password, description)
		VALUES (?, ?, 'password', 'password', ?, ?, ?)`,
		id, name, username, encryptedPassword, emptyToNull(description))
	return err
}

// Update 更新凭证
func (SshKeyRepo) Update(id string, k *SshKey) error {
	db := appdb.DB()
	// COALESCE 非敏感字段，CASE WHEN 敏感字段（null表示保留原值）
	_, err := db.Exec(`
		UPDATE ssh_keys SET
			name        = COALESCE(?, name),
			auth_type   = COALESCE(?, auth_type),
			key_type    = COALESCE(?, key_type),
			fingerprint = COALESCE(?, fingerprint),
			username    = COALESCE(?, username),
			password    = CASE WHEN ? IS NOT NULL THEN ? ELSE password END,
			private_key = CASE WHEN ? IS NOT NULL THEN ? ELSE private_key END,
			description = COALESCE(?, description),
			updated_at  = ?
		WHERE id = ?`,
		emptyToNull(k.Name),
		emptyToNull(strPtr(k.AuthType)),
		emptyToNull(strPtr(k.KeyType)),
		emptyToNull(strPtr(k.Fingerprint)),
		emptyToNull(strPtr(k.Username)),
		emptyToNull(strPtr(k.Password)), emptyToNull(strPtr(k.Password)),
		emptyToNull(strPtr(k.PrivateKey)), emptyToNull(strPtr(k.PrivateKey)),
		emptyToNull(strPtr(k.Description)),
		time.Now().Format("2006-01-02 15:04:05"),
		id)
	return err
}

// Delete 删除凭证
func (SshKeyRepo) Delete(id string) error {
	db := appdb.DB()
	_, err := db.Exec("DELETE FROM ssh_keys WHERE id = ?", id)
	return err
}

// CountUsage 统计使用此凭证的服务器 + 网络设备数量
func (SshKeyRepo) CountUsage(id string) (int, error) {
	db := appdb.DB()
	var serverCount, deviceCount int
	db.QueryRow("SELECT COUNT(*) FROM servers WHERE ssh_key_id = ?", id).Scan(&serverCount)
	db.QueryRow("SELECT COUNT(*) FROM network_devices WHERE ssh_key_id = ?", id).Scan(&deviceCount)
	return serverCount + deviceCount, nil
}

// ListServersByKey 列出使用此密钥的服务器
func (SshKeyRepo) ListServersByKey(id string) ([]map[string]interface{}, error) {
	db := appdb.DB()
	rows, err := db.Query("SELECT id, name, hostname FROM servers WHERE ssh_key_id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id, name, hostname string
		if err := rows.Scan(&id, &name, &hostname); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":       id,
			"name":     name,
			"hostname": hostname,
		})
	}
	return items, nil
}

// ExistsByID 检查是否存在
func (SshKeyRepo) ExistsByID(id string) (bool, error) {
	db := appdb.DB()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM ssh_keys WHERE id = ?", id).Scan(&count)
	return count > 0, err
}

// FindByNameExcludeID 按名称查找（排除指定ID）
func (SshKeyRepo) FindByNameExcludeID(name, excludeID string) (*SshKey, error) {
	db := appdb.DB()
	var k SshKey
	err := db.QueryRow("SELECT id, name FROM ssh_keys WHERE name = ? AND id != ?", name, excludeID).Scan(&k.ID, &k.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// strPtr 辅助：*string → string（空指针返回空字符串）
func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// emptyToNull 辅助：空字符串转为 NULL 用于 SQL 参数
func emptyToNull(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
