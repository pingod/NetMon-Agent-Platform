// Package repos 数据访问层，等价于 TS repositories/
package repos

import (
	"database/sql"
	"strings"
	"time"

	"itops-backend/internal/appdb"
)

// nowLocal 返回与 TS datetime('now','localtime') 一致的本地时间字符串
func nowLocal() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// ns 将 sql.NullString 转为 *string（NULL→nil）
func ns(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}

// nsStr 将 sql.NullString 转为 string（NULL→""）
func nsStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// ───────────────────────────────────────────────
// User 仓库（等价 userRepository.ts）
// ───────────────────────────────────────────────

// UserRecord users 表完整记录
type UserRecord struct {
	ID                 string  `json:"id"`
	Username           string  `json:"username"`
	Password           string  `json:"password,omitempty"`
	Email              *string `json:"email"`
	Role               string  `json:"role"`
	Enabled            int     `json:"enabled"`
	PasswordMustChange int     `json:"password_must_change"`
	FailedLoginAttempt int     `json:"failed_login_attempts"`
	LockedUntil        *string `json:"locked_until"`
	LastFailedLogin    *string `json:"last_failed_login"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

// UserListItem 管理员列表展示字段（不含密码）
type UserListItem struct {
	ID                 string  `json:"id"`
	Username           string  `json:"username"`
	Email              *string `json:"email"`
	Role               string  `json:"role"`
	Enabled            int     `json:"enabled"`
	FailedLoginAttempt int     `json:"failed_login_attempts"`
	LockedUntil        *string `json:"locked_until"`
	CreatedAt          string  `json:"created_at"`
}

// UserAuthRecord 认证所需字段
type UserAuthRecord struct {
	ID                 string
	Username           string
	Password           string
	Role               string
	Email              *string
	Enabled            int
	PasswordMustChange int
}

// UserCacheFields 中间件/WebSocket 缓存字段
type UserCacheFields struct {
	ID                 string
	Username           string
	Email              *string
	Role               string
	Enabled            int
	PasswordMustChange int
}

// UserLockoutStatus 登录锁定状态
type UserLockoutStatus struct {
	ID                 string
	FailedLoginAttempt int
	LockedUntil        *string
}

// UserProfile /me 端点档案
type UserProfile struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	Email     *string `json:"email"`
	Role      string  `json:"role"`
	Enabled   int     `json:"enabled"`
	CreatedAt string  `json:"created_at"`
}

// UserRepo 用户数据访问
type UserRepo struct{}

var Users = &UserRepo{}

func (r *UserRepo) List() ([]UserListItem, error) {
	rows, err := appdb.DB().Query(`SELECT id, username, email, role, enabled, failed_login_attempts, locked_until, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserListItem
	for rows.Next() {
		var u UserListItem
		var email, locked sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &email, &u.Role, &u.Enabled, &u.FailedLoginAttempt, &locked, &u.CreatedAt); err != nil {
			continue
		}
		u.Email = ns(email)
		u.LockedUntil = ns(locked)
		out = append(out, u)
	}
	return out, nil
}

// userFullColumns 显式列清单（避免 users 表新增列后 SELECT * 与扫描字段错位）
const userFullColumns = `id, username, password, email, role, enabled, password_must_change, failed_login_attempts, locked_until, last_failed_login, created_at, updated_at`

func (r *UserRepo) GetByID(id string) (*UserRecord, error) {
	return r.scanFull(appdb.DB().QueryRow(`SELECT `+userFullColumns+` FROM users WHERE id = ?`, id))
}

func (r *UserRepo) GetByUsername(username string) (*UserRecord, error) {
	return r.scanFull(appdb.DB().QueryRow(`SELECT `+userFullColumns+` FROM users WHERE username = ?`, username))
}

// GetForAuth 等价 getForAuth（登录验证用）
func (r *UserRepo) GetForAuth(username string) (*UserAuthRecord, error) {
	row := appdb.DB().QueryRow(`SELECT id, username, password, role, email, enabled, password_must_change FROM users WHERE username = ?`, username)
	var u UserAuthRecord
	var email sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Role, &email, &u.Enabled, &u.PasswordMustChange); err != nil {
		return nil, err
	}
	u.Email = ns(email)
	return &u, nil
}

func (r *UserRepo) scanFull(row *sql.Row) (*UserRecord, error) {
	var u UserRecord
	var email, locked, lastFailed sql.NullString
	err := row.Scan(&u.ID, &u.Username, &u.Password, &email, &u.Role, &u.Enabled,
		&u.PasswordMustChange, &u.FailedLoginAttempt, &locked, &lastFailed, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.Email = ns(email)
	u.LockedUntil = ns(locked)
	u.LastFailedLogin = ns(lastFailed)
	return &u, nil
}

// GetByIDSafe 管理员视图（不含密码）
func (r *UserRepo) GetByIDSafe(id string) (*UserListItem, error) {
	row := appdb.DB().QueryRow(`SELECT id, username, email, role, enabled, failed_login_attempts, locked_until, created_at FROM users WHERE id = ?`, id)
	var u UserListItem
	var email, locked sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &email, &u.Role, &u.Enabled, &u.FailedLoginAttempt, &locked, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.Email = ns(email)
	u.LockedUntil = ns(locked)
	return &u, nil
}

func (r *UserRepo) GetUsername(id string) (string, bool) {
	var username string
	err := appdb.DB().QueryRow(`SELECT username FROM users WHERE id = ?`, id).Scan(&username)
	return username, err == nil
}

func (r *UserRepo) GetIDByUsername(username string) (string, bool) {
	var id string
	err := appdb.DB().QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	return id, err == nil
}

func (r *UserRepo) ExistsByUsername(username string) bool {
	_, ok := r.GetIDByUsername(username)
	return ok
}

func (r *UserRepo) GetProfile(id string) (*UserProfile, error) {
	row := appdb.DB().QueryRow(`SELECT id, username, email, role, enabled, created_at FROM users WHERE id = ?`, id)
	var u UserProfile
	var email sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &email, &u.Role, &u.Enabled, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.Email = ns(email)
	return &u, nil
}

func (r *UserRepo) GetCachedFields(id string) (*UserCacheFields, error) {
	row := appdb.DB().QueryRow(`SELECT id, username, email, role, enabled, password_must_change FROM users WHERE id = ?`, id)
	var u UserCacheFields
	var email sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &email, &u.Role, &u.Enabled, &u.PasswordMustChange); err != nil {
		return nil, err
	}
	u.Email = ns(email)
	return &u, nil
}

// GetForWebSocket WebSocket 认证字段
func (r *UserRepo) GetForWebSocket(id string) (*UserCacheFields, error) {
	return r.GetCachedFields(id)
}

func (r *UserRepo) GetLockoutStatus(username string) (*UserLockoutStatus, error) {
	row := appdb.DB().QueryRow(`SELECT id, failed_login_attempts, locked_until FROM users WHERE username = ?`, username)
	var u UserLockoutStatus
	var locked sql.NullString
	if err := row.Scan(&u.ID, &u.FailedLoginAttempt, &locked); err != nil {
		return nil, err
	}
	u.LockedUntil = ns(locked)
	return &u, nil
}

func (r *UserRepo) GetFailedAttempts(username string) (*UserLockoutStatus, error) {
	return r.GetLockoutStatus(username)
}

func (r *UserRepo) CountAll() int {
	var c int
	_ = appdb.DB().QueryRow(`SELECT COUNT(*) FROM users`).Scan(&c)
	return c
}

// CountEnabledAdmins 启用状态的管理员数量（最后管理员保护用）
func (r *UserRepo) CountEnabledAdmins() int {
	var c int
	_ = appdb.DB().QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1`).Scan(&c)
	return c
}

// UserCreateInput 创建用户输入
type UserCreateInput struct {
	ID             string
	Username       string
	HashedPassword string
	Email          *string
	Role           string
	Enabled        int
}

func (r *UserRepo) Create(in UserCreateInput) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := appdb.DB().Exec(`INSERT INTO users (id, username, password, email, role, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ID, in.Username, in.HashedPassword, in.Email, in.Role, in.Enabled, now, now)
	return err
}

// CreateDefaultAdmin 创建默认管理员（含 password_must_change）
func (r *UserRepo) CreateDefaultAdmin(id, username, hashedPassword, email, role string, enabled, passwordMustChange int) error {
	_, err := appdb.DB().Exec(`INSERT INTO users (id, username, password, email, role, enabled, password_must_change) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, username, hashedPassword, email, role, enabled, passwordMustChange)
	return err
}

// UserUpdateInput 动态更新字段
type UserUpdateInput struct {
	Username *string
	Email    **string // 二级指针区分"未提供"与"置空"
	Role     *string
	Enabled  *int
	Password *string
}

func (r *UserRepo) Update(id string, f UserUpdateInput) (int, error) {
	var set []string
	var vals []interface{}
	if f.Username != nil {
		set = append(set, "username = ?")
		vals = append(vals, *f.Username)
	}
	if f.Email != nil {
		set = append(set, "email = ?")
		if *f.Email == nil {
			vals = append(vals, nil)
		} else {
			vals = append(vals, **f.Email)
		}
	}
	if f.Role != nil {
		set = append(set, "role = ?")
		vals = append(vals, *f.Role)
	}
	if f.Enabled != nil {
		set = append(set, "enabled = ?")
		vals = append(vals, *f.Enabled)
	}
	if f.Password != nil {
		set = append(set, "password = ?")
		vals = append(vals, *f.Password)
	}
	if len(set) == 0 {
		return 0, nil
	}
	set = append(set, "updated_at = ?")
	vals = append(vals, time.Now().UTC().Format(time.RFC3339))
	vals = append(vals, id)
	res, err := appdb.DB().Exec(`UPDATE users SET `+strings.Join(set, ", ")+` WHERE id = ?`, vals...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *UserRepo) UpdatePassword(id, hashedPassword string) (int, error) {
	now := nowLocal()
	res, err := appdb.DB().Exec(`UPDATE users SET password = ?, password_must_change = 0, updated_at = ? WHERE id = ?`, hashedPassword, now, id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *UserRepo) TouchLogin(id string) {
	now := nowLocal()
	_, _ = appdb.DB().Exec(`UPDATE users SET updated_at = ? WHERE id = ?`, now, id)
}

func (r *UserRepo) Unlock(id string) {
	now := nowLocal()
	_, _ = appdb.DB().Exec(`UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = ? WHERE id = ?`, now, id)
}

func (r *UserRepo) ClearLockout(id string) {
	_, _ = appdb.DB().Exec(`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = ?`, id)
}

func (r *UserRepo) RecordFailedLoginWithLock(id string, attempts int, lockedUntilIso, nowIso string) {
	now := nowLocal()
	_, _ = appdb.DB().Exec(`UPDATE users SET failed_login_attempts = ?, locked_until = ?, last_failed_login = ?, updated_at = ? WHERE id = ?`,
		attempts, lockedUntilIso, nowIso, now, id)
}

func (r *UserRepo) RecordFailedLogin(id string, attempts int, nowIso string) {
	now := nowLocal()
	_, _ = appdb.DB().Exec(`UPDATE users SET failed_login_attempts = ?, last_failed_login = ?, updated_at = ? WHERE id = ?`,
		attempts, nowIso, now, id)
}

// FailedLoginState 原子递增后的登录失败状态
type FailedLoginState struct {
	ID                 string
	FailedLoginAttempt int
	LockedUntil        *string
}

// IncrementFailedLogin 以单条原子 UPDATE 递增失败次数，并在达到阈值时写入锁定时间。
// 并发登录失败不会因读-改-写竞争而丢失计数。
func (r *UserRepo) IncrementFailedLogin(username string, threshold int, lockedUntilIso, nowIso string) (*FailedLoginState, error) {
	now := nowLocal()
	_, err := appdb.DB().Exec(`UPDATE users SET
		failed_login_attempts = failed_login_attempts + 1,
		locked_until = CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END,
		last_failed_login = ?,
		updated_at = ?
		WHERE username = ?`, threshold, lockedUntilIso, nowIso, now, username)
	if err != nil {
		return nil, err
	}

	row := appdb.DB().QueryRow(`SELECT id, failed_login_attempts, locked_until FROM users WHERE username = ?`, username)
	var st FailedLoginState
	var locked sql.NullString
	if err := row.Scan(&st.ID, &st.FailedLoginAttempt, &locked); err != nil {
		return nil, err
	}
	st.LockedUntil = ns(locked)
	return &st, nil
}

// GetTokenInvalidBefore 返回用户 token 失效时间戳（unix 秒）；列缺失或出错时返回 0
func (r *UserRepo) GetTokenInvalidBefore(id string) int64 {
	var v sql.NullInt64
	if err := appdb.DB().QueryRow(`SELECT token_invalid_before FROM users WHERE id = ?`, id).Scan(&v); err != nil {
		return 0
	}
	if v.Valid {
		return v.Int64
	}
	return 0
}

// UpdatePasswordAndRevokeTokens 更新密码并让该用户此前签发的所有 token 失效
func (r *UserRepo) UpdatePasswordAndRevokeTokens(id, hashedPassword string, invalidBefore int64) (int, error) {
	res, err := appdb.DB().Exec(`UPDATE users SET password = ?, password_must_change = 0, token_invalid_before = ?, updated_at = ? WHERE id = ?`,
		hashedPassword, invalidBefore, nowLocal(), id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *UserRepo) Delete(id string) (bool, error) {
	res, err := appdb.DB().Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
