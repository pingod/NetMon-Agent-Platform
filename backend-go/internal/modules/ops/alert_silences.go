// 告警静默规则（维护窗口）：在指定时间段内抑制匹配告警的产生。
// 作用于所有真实告警写入路径（规则评估器 / InfluxDB 写入），命中即跳过入库与分发。
package ops

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
	"itops-backend/internal/middleware"
	"itops-backend/internal/services/audit"
)

type alertSilence struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	MatchSource   string `json:"match_source"`
	MatchSeverity string `json:"match_severity"`
	MatchKeywords string `json:"match_keywords"`
	MatchDeviceID string `json:"match_device_id"`
	StartsAt      string `json:"starts_at"`
	EndsAt        string `json:"ends_at"`
	Enabled       bool   `json:"enabled"`
	CreatedBy     string `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// alertSilenceRequest 创建/更新入参；Enabled 为指针以区分「未传」与「显式 false」
type alertSilenceRequest struct {
	Name          string `json:"name"`
	MatchSource   string `json:"match_source"`
	MatchSeverity string `json:"match_severity"`
	MatchKeywords string `json:"match_keywords"`
	MatchDeviceID string `json:"match_device_id"`
	StartsAt      string `json:"starts_at"`
	EndsAt        string `json:"ends_at"`
	Enabled       *bool  `json:"enabled"`
}

const silenceSelectCols = `id, name, COALESCE(match_source,''), COALESCE(match_severity,''), COALESCE(match_keywords,''),
	COALESCE(match_device_id,''), starts_at, ends_at, enabled, COALESCE(created_by,''), COALESCE(created_at,''), COALESCE(updated_at,'')`

func scanSilence(scan func(dest ...interface{}) error) (*alertSilence, error) {
	var s alertSilence
	var enabled int
	if err := scan(&s.ID, &s.Name, &s.MatchSource, &s.MatchSeverity, &s.MatchKeywords,
		&s.MatchDeviceID, &s.StartsAt, &s.EndsAt, &enabled, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Enabled = enabled == 1
	return &s, nil
}

// IsAlertSilenced 判断一条即将产生的告警是否命中生效中的静默规则。
// 字段间 AND，关键词内部 OR（任一命中即可）；空字段不限制。
func IsAlertSilenced(source, severity, title, content, deviceID string) bool {
	now := time.Now().Format("2006-01-02 15:04:05")
	rows, err := appdb.DB().Query(`SELECT `+silenceSelectCols+` FROM alert_silences
		WHERE enabled = 1 AND starts_at <= ? AND ends_at >= ?`, now, now)
	if err != nil {
		return false
	}
	defer rows.Close()
	hay := strings.ToLower(title + " " + content)
	for rows.Next() {
		s, err := scanSilence(rows.Scan)
		if err != nil {
			continue
		}
		if s.MatchSource != "" && normalizeAlertSource(s.MatchSource) != normalizeAlertSource(source) {
			continue
		}
		if s.MatchSeverity != "" && !strings.EqualFold(s.MatchSeverity, severity) {
			continue
		}
		if s.MatchDeviceID != "" && s.MatchDeviceID != deviceID {
			continue
		}
		if s.MatchKeywords != "" {
			matched := false
			for _, kw := range strings.Split(s.MatchKeywords, ",") {
				kw = strings.TrimSpace(strings.ToLower(kw))
				if kw != "" && strings.Contains(hay, kw) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		return true
	}
	return false
}

// normalizeSilenceTime 接受 "2006-01-02 15:04[:05]" 或 datetime-local 的 "2006-01-02T15:04"。
func normalizeSilenceTime(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "T", " "))
	if len(s) == 16 {
		s += ":00"
	}
	return s
}

func validateSilenceTime(s string) bool {
	_, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	return err == nil
}

// AlertSilenceRoutes /api/v1/alert-silences
func AlertSilenceRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rows, err := appdb.DB().Query(`SELECT ` + silenceSelectCols + ` FROM alert_silences ORDER BY created_at DESC`)
		if err != nil {
			httpx.Respond(w, []interface{}{})
			return
		}
		defer rows.Close()
		list := []alertSilence{}
		for rows.Next() {
			if s, err := scanSilence(rows.Scan); err == nil {
				list = append(list, *s)
			}
		}
		httpx.Respond(w, list)
	})

	write := middleware.RequireRole("admin", "operator")
	r.With(write).Post("/", func(w http.ResponseWriter, r *http.Request) {
		var b alertSilenceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&b); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		b.Name = strings.TrimSpace(b.Name)
		b.StartsAt = normalizeSilenceTime(b.StartsAt)
		b.EndsAt = normalizeSilenceTime(b.EndsAt)
		if b.Name == "" {
			httpx.RespondError(w, "名称不能为空", http.StatusBadRequest)
			return
		}
		if !validateSilenceTime(b.StartsAt) || !validateSilenceTime(b.EndsAt) {
			httpx.RespondError(w, "时间格式非法（应为 YYYY-MM-DD HH:MM:SS）", http.StatusBadRequest)
			return
		}
		if b.EndsAt <= b.StartsAt {
			httpx.RespondError(w, "结束时间必须晚于开始时间", http.StatusBadRequest)
			return
		}
		if b.MatchSource == "" && b.MatchSeverity == "" && b.MatchKeywords == "" && b.MatchDeviceID == "" {
			httpx.RespondError(w, "至少指定一个匹配条件（来源/级别/关键词/设备），避免误静默全部告警", http.StatusBadRequest)
			return
		}
		user := middleware.GetUser(r)
		id := uuid.NewString()
		now := time.Now().Format("2006-01-02 15:04:05")
		enabled := 1
		if b.Enabled != nil && !*b.Enabled {
			enabled = 0
		}
		_, err := appdb.DB().Exec(`INSERT INTO alert_silences (id, name, match_source, match_severity, match_keywords, match_device_id, starts_at, ends_at, enabled, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, b.Name, strings.TrimSpace(b.MatchSource), strings.TrimSpace(b.MatchSeverity), strings.TrimSpace(b.MatchKeywords), strings.TrimSpace(b.MatchDeviceID),
			b.StartsAt, b.EndsAt, enabled, user.Username, now, now)
		if err != nil {
			httpx.RespondError(w, "创建静默规则失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "alert_silence_create", ResourceType: "alert_silence", ResourceID: id, Details: map[string]interface{}{"name": b.Name, "starts_at": b.StartsAt, "ends_at": b.EndsAt}, IPAddress: middleware.ClientIP(r)})
		httpx.RespondStatus(w, http.StatusCreated, map[string]interface{}{"id": id, "name": b.Name})
	})

	r.With(write).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var b alertSilenceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&b); err != nil {
			httpx.RespondError(w, "无效的请求", http.StatusBadRequest)
			return
		}
		b.Name = strings.TrimSpace(b.Name)
		if b.Name == "" {
			httpx.RespondError(w, "名称不能为空", http.StatusBadRequest)
			return
		}
		b.StartsAt = normalizeSilenceTime(b.StartsAt)
		b.EndsAt = normalizeSilenceTime(b.EndsAt)
		if !validateSilenceTime(b.StartsAt) || !validateSilenceTime(b.EndsAt) || b.EndsAt <= b.StartsAt {
			httpx.RespondError(w, "时间范围非法", http.StatusBadRequest)
			return
		}
		enabled := 1
		if b.Enabled == nil {
			// 未传 enabled 时保持当前状态，避免静默重启用被意外关闭的规则
			appdb.DB().QueryRow(`SELECT enabled FROM alert_silences WHERE id = ?`, id).Scan(&enabled)
		} else if !*b.Enabled {
			enabled = 0
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		res, err := appdb.DB().Exec(`UPDATE alert_silences SET name = ?, match_source = ?, match_severity = ?, match_keywords = ?, match_device_id = ?,
			starts_at = ?, ends_at = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			strings.TrimSpace(b.Name), strings.TrimSpace(b.MatchSource), strings.TrimSpace(b.MatchSeverity), strings.TrimSpace(b.MatchKeywords),
			strings.TrimSpace(b.MatchDeviceID), b.StartsAt, b.EndsAt, enabled, now, id)
		if err != nil {
			httpx.RespondError(w, "更新失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			httpx.RespondError(w, "静默规则不存在", http.StatusNotFound)
			return
		}
		httpx.RespondMessage(w, "静默规则已更新")
	})

	r.With(write).Post("/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		now := time.Now().Format("2006-01-02 15:04:05")
		res, err := appdb.DB().Exec(`UPDATE alert_silences SET enabled = 1 - enabled, updated_at = ? WHERE id = ?`, now, id)
		if err != nil {
			httpx.RespondError(w, "切换失败", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			httpx.RespondError(w, "静默规则不存在", http.StatusNotFound)
			return
		}
		var enabled int
		appdb.DB().QueryRow(`SELECT enabled FROM alert_silences WHERE id = ?`, id).Scan(&enabled)
		httpx.Respond(w, map[string]interface{}{"id": id, "enabled": enabled == 1})
	})

	r.With(middleware.RequireRole("admin")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		res, err := appdb.DB().Exec(`DELETE FROM alert_silences WHERE id = ?`, id)
		if err != nil {
			httpx.RespondError(w, "删除失败", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			httpx.RespondError(w, "静默规则不存在", http.StatusNotFound)
			return
		}
		user := middleware.GetUser(r)
		audit.CreateAuditLog(audit.AuditInput{UserID: user.ID, Action: "alert_silence_delete", ResourceType: "alert_silence", ResourceID: id, IPAddress: middleware.ClientIP(r)})
		httpx.RespondMessage(w, "静默规则已删除")
	})
	return r
}