// 平台数据库备份（/api/v1/backups）。
//
// 备份用 SQLite VACUUM INTO 在线生成一致性快照，不锁库、可直接下载/恢复。
// 恢复流程：校验完整性 → 自动备份当前库（安全网）→ 换文件 → syscall.Exec 原地重启。
// 仅支持 SQLite 模式；MySQL 模式下明确拒绝而不是假装成功。
package ops

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/middleware"
	"itops-backend/internal/paths"
	"itops-backend/internal/services/audit"
)

func platformBackupDir() string {
	return filepath.Join(filepath.Dir(paths.DatabasePath()), "backups")
}

// BackupRoutes /api/v1/backups
func BackupRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/history", backupHistory)
	r.Get("/download/{id}", backupDownload)
	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/create", backupCreate)
	r.With(write).Post("/upload", backupUpload)
	r.With(middleware.RequireRole("admin")).Post("/restore/{id}", backupRestore)
	r.With(middleware.RequireRole("admin")).Delete("/{id}", backupDelete)
	return r
}

type platformBackup struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Source    string `json:"source"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"createdAt"`
}

func backupHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := appdb.DB().Query(`SELECT id, filename, COALESCE(size_bytes,0), COALESCE(source,'manual'), COALESCE(created_by,''), COALESCE(created_at,'') FROM platform_backups ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		httpx.Respond(w, []interface{}{})
		return
	}
	defer rows.Close()
	list := []platformBackup{}
	for rows.Next() {
		var b platformBackup
		if rows.Scan(&b.ID, &b.Filename, &b.Size, &b.Source, &b.CreatedBy, &b.CreatedAt) == nil {
			list = append(list, b)
		}
	}
	httpx.Respond(w, list)
}

func backupCreate(w http.ResponseWriter, r *http.Request) {
	if appdb.IsMySQLMode() {
		httpx.RespondError(w, "当前为 MySQL 模式，请使用 mysqldump 等数据库原生工具备份", http.StatusBadRequest)
		return
	}
	rec, err := createPlatformBackup("manual", middleware.GetUser(r).Username)
	if err != nil {
		httpx.RespondError(w, "创建备份失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	user := middleware.GetUser(r)
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "platform_backup_create", ResourceType: "platform_backup", ResourceID: rec.ID, Details: map[string]interface{}{"filename": rec.Filename, "size": rec.Size}, IPAddress: middleware.ClientIP(r)})
	httpx.RespondStatus(w, http.StatusCreated, rec)
}

// createPlatformBackup VACUUM INTO 在线快照 + 登记
func createPlatformBackup(source, username string) (*platformBackup, error) {
	dir := platformBackupDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	filename := fmt.Sprintf("app-%s.db", time.Now().Format("20060102-150405"))
	dest := filepath.Join(dir, filename)
	// VACUUM INTO 目标已存在会报错；同名秒级冲突时加序号
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			break
		}
		filename = fmt.Sprintf("app-%s-%d.db", time.Now().Format("20060102-150405"), i)
		dest = filepath.Join(dir, filename)
	}
	if _, err := appdb.DB().Exec(`VACUUM INTO ?`, dest); err != nil {
		return nil, fmt.Errorf("VACUUM INTO: %w", err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := appdb.DB().Exec(`INSERT INTO platform_backups (id, filename, size_bytes, source, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, filename, fi.Size(), source, username, now); err != nil {
		os.Remove(dest)
		return nil, err
	}
	return &platformBackup{ID: id, Filename: filename, Size: fi.Size(), Source: source, CreatedBy: username, CreatedAt: now}, nil
}

func lookupBackup(id string) (*platformBackup, string, error) {
	var b platformBackup
	err := appdb.DB().QueryRow(`SELECT id, filename, COALESCE(size_bytes,0), COALESCE(source,''), COALESCE(created_by,''), COALESCE(created_at,'') FROM platform_backups WHERE id = ?`, id).
		Scan(&b.ID, &b.Filename, &b.Size, &b.Source, &b.CreatedBy, &b.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("备份记录不存在")
	}
	// 防目录穿越：只允许 backups 目录内的纯文件名
	if b.Filename != filepath.Base(b.Filename) || strings.ContainsAny(b.Filename, `/\`) {
		return nil, "", fmt.Errorf("非法的备份文件名")
	}
	p := filepath.Join(platformBackupDir(), b.Filename)
	if _, err := os.Stat(p); err != nil {
		return nil, "", fmt.Errorf("备份文件已丢失")
	}
	return &b, p, nil
}

func backupDownload(w http.ResponseWriter, r *http.Request) {
	b, p, err := lookupBackup(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, b.Filename))
	http.ServeFile(w, r, p)
}

const maxBackupUploadBytes = 512 << 20 // 512MB

func backupUpload(w http.ResponseWriter, r *http.Request) {
	if appdb.IsMySQLMode() {
		httpx.RespondError(w, "当前为 MySQL 模式，不支持上传 SQLite 备份", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.RespondError(w, "上传失败：文件过大或表单无效（上限 512MB）", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("backup")
	if err != nil {
		httpx.RespondError(w, "缺少备份文件字段 backup", http.StatusBadRequest)
		return
	}
	defer file.Close()

	dir := platformBackupDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		httpx.RespondError(w, "创建备份目录失败", http.StatusInternalServerError)
		return
	}
	// 上传文件统一改名落盘，绝不使用用户提供的路径；同秒冲突时加序号
	filename := fmt.Sprintf("upload-%s.db", time.Now().Format("20060102-150405"))
	dest := filepath.Join(dir, filename)
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			break
		}
		filename = fmt.Sprintf("upload-%s-%d.db", time.Now().Format("20060102-150405"), i)
		dest = filepath.Join(dir, filename)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		httpx.RespondError(w, "保存备份文件失败", http.StatusInternalServerError)
		return
	}
	written, err := io.Copy(out, file)
	out.Close()
	if err != nil {
		os.Remove(dest)
		httpx.RespondError(w, "写入备份文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := validateBackupFile(dest); err != nil {
		os.Remove(dest)
		httpx.RespondError(w, "备份文件校验失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	id := uuid.NewString()
	now := time.Now().Format("2006-01-02 15:04:05")
	user := middleware.GetUser(r)
	if _, err := appdb.DB().Exec(`INSERT INTO platform_backups (id, filename, size_bytes, source, created_by, created_at) VALUES (?, ?, ?, 'upload', ?, ?)`,
		id, filename, written, user.Username, now); err != nil {
		os.Remove(dest)
		httpx.RespondError(w, "登记备份失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "platform_backup_upload", ResourceType: "platform_backup", ResourceID: id, Details: map[string]interface{}{"filename": filename, "original_name": header.Filename, "size": written}, IPAddress: middleware.ClientIP(r)})
	httpx.RespondStatus(w, http.StatusCreated, platformBackup{ID: id, Filename: filename, Size: written, Source: "upload", CreatedBy: user.Username, CreatedAt: now})
}

// validateBackupFile 完整性检查 + 关键表存在性检查，防止把任意文件当数据库恢复
func validateBackupFile(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("无法读取（不是有效的 SQLite 数据库）")
	}
	if integrity != "ok" {
		return fmt.Errorf("完整性检查未通过: %s", integrity)
	}
	for _, table := range []string{"users", "servers", "alerts", "workflows"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n == 0 {
			return fmt.Errorf("缺少关键表 %s，不是本平台的数据库备份", table)
		}
	}
	return nil
}

func backupRestore(w http.ResponseWriter, r *http.Request) {
	if appdb.IsMySQLMode() {
		httpx.RespondError(w, "当前为 MySQL 模式，不支持 SQLite 备份恢复", http.StatusBadRequest)
		return
	}
	b, src, err := lookupBackup(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := validateBackupFile(src); err != nil {
		httpx.RespondError(w, "备份文件校验失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r)
	// 安全网：恢复前先把当前库备份一份
	safety, err := createPlatformBackup("pre-restore", user.Username)
	if err != nil {
		httpx.RespondError(w, "恢复前安全备份失败，已中止: "+err.Error(), http.StatusInternalServerError)
		return
	}
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "platform_backup_restore", ResourceType: "platform_backup", ResourceID: b.ID, Details: map[string]interface{}{"filename": b.Filename, "safety_backup": safety.Filename}, IPAddress: middleware.ClientIP(r)})

	dbPath := paths.DatabasePath()
	if err := swapDatabaseFile(src, dbPath); err != nil {
		httpx.RespondError(w, "替换数据库文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// VACUUM INTO 快照不含备份自身的登记行，安全备份又登记在旧库里：
	// 两者都要补登记进恢复后的新库，否则重启后历史里看不到、也无法删除
	for _, rec := range []*platformBackup{b, safety} {
		if err := registerBackupInRestoredDB(dbPath, rec); err != nil {
			logger.Warnf("备份 %s 登记进恢复后数据库失败（文件仍保留在 backups 目录）: %v", rec.Filename, err)
		}
	}
	httpx.RespondMessage(w, "备份已恢复，系统正在重启（恢复前的库已自动备份为 "+safety.Filename+"）")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	// 响应送达后原地重启：exec 替换当前进程，PID 不变，容器/裸机均适用
	time.AfterFunc(800*time.Millisecond, func() {
		self, err := os.Executable()
		if err != nil {
			logger.Errorf("恢复后重启失败（找不到自身路径）: %v", err)
			os.Exit(1)
		}
		logger.Infof("平台备份恢复完成，进程原地重启: %s", self)
		if err := syscall.Exec(self, os.Args, os.Environ()); err != nil {
			logger.Errorf("恢复后重启失败: %v", err)
			os.Exit(1)
		}
	})
}

// registerBackupInRestoredDB 把备份记录补登记进刚恢复的数据库文件（独立连接，主连接仍指向旧 inode）
func registerBackupInRestoredDB(dbPath string, b *platformBackup) error {
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	// 旧版本备份可能没有 platform_backups 表
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS platform_backups (
		id TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		size_bytes INTEGER DEFAULT 0,
		source TEXT DEFAULT 'manual',
		created_by TEXT DEFAULT '',
		created_at DATETIME DEFAULT (datetime('now','localtime'))
	)`); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO platform_backups (id, filename, size_bytes, source, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.Filename, b.Size, b.Source, b.CreatedBy, b.CreatedAt)
	return err
}

// swapDatabaseFile 用备份文件替换当前数据库，并清理 WAL/SHM 残留
func swapDatabaseFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".restore-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(dst + "-wal")
	os.Remove(dst + "-shm")
	return nil
}

func backupDelete(w http.ResponseWriter, r *http.Request) {
	b, p, err := lookupBackup(chi.URLParam(r, "id"))
	if err != nil {
		httpx.RespondError(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := os.Remove(p); err != nil {
		httpx.RespondError(w, "删除备份文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	appdb.DB().Exec(`DELETE FROM platform_backups WHERE id = ?`, b.ID)
	user := middleware.GetUser(r)
	audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "platform_backup_delete", ResourceType: "platform_backup", ResourceID: b.ID, Details: map[string]interface{}{"filename": b.Filename}, IPAddress: middleware.ClientIP(r)})
	httpx.RespondMessage(w, "备份已删除")
}