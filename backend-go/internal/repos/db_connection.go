// 数据库连接数据访问层 — 对应 TS dbConnectionRepository.ts
// 表：databases（MySQL/PostgreSQL/Oracle + InfluxDB v1/v2/v3）
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"itops-backend/internal/appdb"
)

// DbConnection 数据库连接
type DbConnection struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	DBType          string  `json:"db_type"`
	Host            string  `json:"host"`
	Port            int     `json:"port"`
	Username        string  `json:"username"`
	Password        string  `json:"password"` // 加密密文，列表时为空
	Database        string  `json:"database"`
	Description     *string `json:"description"`
	Tags            *string `json:"-"` // 内部字段，通过 MarshalJSON 输出为数组
	Enabled         int     `json:"enabled"`
	Token           *string `json:"token"` // 加密密文
	Organization    *string `json:"organization"`
	Bucket          *string `json:"bucket"`
	RetentionPolicy *string `json:"retention_policy"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// MarshalJSON 自定义序列化：tags 从逗号分隔字符串转为 string[]
func (d DbConnection) MarshalJSON() ([]byte, error) {
	var tags []string
	if d.Tags != nil && *d.Tags != "" {
		for _, t := range splitTags(*d.Tags) {
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	if tags == nil {
		tags = []string{}
	}

	type Alias struct {
		ID              string   `json:"id"`
		Name            string   `json:"name"`
		DBType          string   `json:"db_type"`
		Host            string   `json:"host"`
		Port            int      `json:"port"`
		Username        string   `json:"username"`
		Password        string   `json:"password"`
		Database        string   `json:"database"`
		Description     *string  `json:"description"`
		Tags            []string `json:"tags"`
		Enabled         int      `json:"enabled"`
		Token           *string  `json:"token"`
		Organization    *string  `json:"organization"`
		Bucket          *string  `json:"bucket"`
		RetentionPolicy *string  `json:"retention_policy"`
		CreatedAt       string   `json:"created_at"`
		UpdatedAt       string   `json:"updated_at"`
	}
	return json.Marshal(Alias{
		ID: d.ID, Name: d.Name, DBType: d.DBType, Host: d.Host, Port: d.Port,
		Username: d.Username, Password: d.Password, Database: d.Database,
		Description: d.Description, Tags: tags, Enabled: d.Enabled,
		Token: d.Token, Organization: d.Organization, Bucket: d.Bucket,
		RetentionPolicy: d.RetentionPolicy, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	})
}

var DbConnections = &DbConnectionRepo{}

type DbConnectionRepo struct{}

func ensureDatabasesSchema(db *sql.DB) {
	if db == nil {
		return
	}
	// 1. 保证表结构存在 (跨 SQLite 与 MySQL 兼容)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS ` + "`databases`" + ` (
			id VARCHAR(36) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			db_type VARCHAR(50) NOT NULL DEFAULT 'mysql',
			host VARCHAR(255) NOT NULL,
			port INT NOT NULL DEFAULT 3306,
			username VARCHAR(255) DEFAULT '',
			password TEXT,
			` + "`database`" + ` VARCHAR(255) DEFAULT '',
			description TEXT,
			tags TEXT,
			enabled INT DEFAULT 1,
			token TEXT,
			organization VARCHAR(255) DEFAULT '',
			bucket VARCHAR(255) DEFAULT '',
			retention_policy VARCHAR(255) DEFAULT '',
			created_at VARCHAR(50),
			updated_at VARCHAR(50)
		)
	`)

	// 2. 自愈添加可能缺失的字段
	columns := []string{
		"ALTER TABLE `databases` ADD COLUMN db_type VARCHAR(50) DEFAULT 'mysql'",
		"ALTER TABLE `databases` ADD COLUMN `database` VARCHAR(255) DEFAULT ''",
		"ALTER TABLE `databases` ADD COLUMN description TEXT",
		"ALTER TABLE `databases` ADD COLUMN tags TEXT",
		"ALTER TABLE `databases` ADD COLUMN enabled INT DEFAULT 1",
		"ALTER TABLE `databases` ADD COLUMN token TEXT",
		"ALTER TABLE `databases` ADD COLUMN organization VARCHAR(255) DEFAULT ''",
		"ALTER TABLE `databases` ADD COLUMN bucket VARCHAR(255) DEFAULT ''",
		"ALTER TABLE `databases` ADD COLUMN retention_policy VARCHAR(255) DEFAULT ''",
		"ALTER TABLE `databases` ADD COLUMN created_at VARCHAR(50)",
		"ALTER TABLE `databases` ADD COLUMN updated_at VARCHAR(50)",
	}
	for _, alter := range columns {
		_, _ = db.Exec(alter)
	}
}

// ListAll 列出所有连接（含加密字段，调用方需自行清空敏感字段）
func (DbConnectionRepo) ListAll() ([]DbConnection, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	ensureDatabasesSchema(db)

	rows, err := db.Query(`
		SELECT id, name, COALESCE(db_type, 'mysql'), host, port, username, password, COALESCE(` + "`database`" + `, ''),
		       description, tags, COALESCE(enabled, 1), token, organization, bucket, retention_policy,
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM ` + "`databases`" + ` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []DbConnection
	for rows.Next() {
		var c DbConnection
		var desc, tags, token, org, bucket, rp sql.NullString
		err := rows.Scan(&c.ID, &c.Name, &c.DBType, &c.Host, &c.Port,
			&c.Username, &c.Password, &c.Database,
			&desc, &tags, &c.Enabled, &token, &org, &bucket, &rp,
			&c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		c.Description = nullStrToPtr(desc)
		c.Tags = nullStrToPtr(tags)
		c.Token = nullStrToPtr(token)
		c.Organization = nullStrToPtr(org)
		c.Bucket = nullStrToPtr(bucket)
		c.RetentionPolicy = nullStrToPtr(rp)
		items = append(items, c)
	}
	return items, nil
}

// GetByID 获取单个连接（含加密字段）
func (DbConnectionRepo) GetByID(id string) (*DbConnection, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	ensureDatabasesSchema(db)

	var c DbConnection
	var desc, tags, token, org, bucket, rp sql.NullString
	err := db.QueryRow(`
		SELECT id, name, COALESCE(db_type, 'mysql'), host, port, username, password, COALESCE(` + "`database`" + `, ''),
		       description, tags, COALESCE(enabled, 1), token, organization, bucket, retention_policy,
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM ` + "`databases`" + ` WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.DBType, &c.Host, &c.Port,
			&c.Username, &c.Password, &c.Database,
			&desc, &tags, &c.Enabled, &token, &org, &bucket, &rp,
			&c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Description = nullStrToPtr(desc)
	c.Tags = nullStrToPtr(tags)
	c.Token = nullStrToPtr(token)
	c.Organization = nullStrToPtr(org)
	c.Bucket = nullStrToPtr(bucket)
	c.RetentionPolicy = nullStrToPtr(rp)
	return &c, nil
}

// Insert 创建数据库连接
func (DbConnectionRepo) Insert(c *DbConnection) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	ensureDatabasesSchema(db)

	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`
		INSERT INTO ` + "`databases`" + ` (
			id, name, db_type, host, port, username, password, ` + "`database`" + `,
			description, tags, enabled, token, organization, bucket, retention_policy,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.DBType, c.Host, c.Port, c.Username, c.Password, c.Database,
		emptyToNull(strPtr(c.Description)), emptyToNull(strPtr(c.Tags)), c.Enabled,
		emptyToNull(strPtr(c.Token)), emptyToNull(strPtr(c.Organization)),
		emptyToNull(strPtr(c.Bucket)), emptyToNull(strPtr(c.RetentionPolicy)),
		now, now)
	return err
}

// Update 更新数据库连接
func (DbConnectionRepo) Update(id string, c *DbConnection) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	ensureDatabasesSchema(db)

	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec(`
		UPDATE ` + "`databases`" + ` SET
			name = ?, db_type = ?, host = ?, port = ?, username = ?, password = ?,
			` + "`database`" + ` = ?, description = ?, tags = ?, enabled = ?,
			token = ?, organization = ?, bucket = ?, retention_policy = ?,
			updated_at = ?
		WHERE id = ?`,
		c.Name, c.DBType, c.Host, c.Port, c.Username, c.Password,
		c.Database, emptyToNull(strPtr(c.Description)), emptyToNull(strPtr(c.Tags)), c.Enabled,
		emptyToNull(strPtr(c.Token)), emptyToNull(strPtr(c.Organization)),
		emptyToNull(strPtr(c.Bucket)), emptyToNull(strPtr(c.RetentionPolicy)),
		now, id)
	return err
}

// Delete 删除数据库连接
func (DbConnectionRepo) Delete(id string) error {
	db := appdb.DB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	ensureDatabasesSchema(db)
	_, err := db.Exec("DELETE FROM `databases` WHERE id = ?", id)
	return err
}
