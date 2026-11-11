// 告警 CSV 导出 — 复用列表筛选条件，带 BOM 与公式注入防护
package routes

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"itops-backend/internal/httpx"
	"itops-backend/internal/logger"
	"itops-backend/internal/repos"
)

var alertSeverityLabels = map[string]string{
	"critical": "严重",
	"high":     "高",
	"medium":   "中",
	"low":      "低",
	"warning":  "警告",
	"info":     "信息",
}

var alertStatusLabels = map[string]string{
	"new":          "新告警",
	"firing":       "触发中",
	"acknowledged": "已确认",
	"resolved":     "已解决",
}

// csvSafeCell 防止 Excel/WPS 将单元格内容当作公式执行（= + - @ 及制表/回车开头）
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

func alertLabel(m map[string]string, v string) string {
	if label, ok := m[v]; ok {
		return label
	}
	return v
}

func handleExportAlerts(w http.ResponseWriter, r *http.Request) {
	f := repos.AlertFilter{
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
		Source:   r.URL.Query().Get("source"),
		Limit:    10000,
	}
	alerts, err := repos.Alerts.List(f)
	if err != nil {
		httpx.RespondError(w, "导出告警失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=alerts-%s.csv", time.Now().Format("20060102-150405")))
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.Write([]string{"ID", "来源", "级别", "标题", "内容", "状态", "创建时间", "更新时间"})
	for _, a := range alerts {
		content := ""
		if a.Content != nil {
			content = *a.Content
		}
		row := []string{
			a.ID,
			a.Source,
			alertLabel(alertSeverityLabels, a.Severity),
			a.Title,
			content,
			alertLabel(alertStatusLabels, a.Status),
			a.CreatedAt,
			a.UpdatedAt,
		}
		for i, cell := range row {
			row[i] = csvSafeCell(cell)
		}
		cw.Write(row)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		logger.Warnf("告警 CSV 导出写入中断: %v", err)
	}
}