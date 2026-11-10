// 设备接口摘要 — 等价于 TS networkDeviceRoutes.ts /:id/interface-summary
package routes

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"

	"itops-backend/internal/appdb"
	"itops-backend/internal/httpx"
)

// handleInterfaceSummary 返回单台设备的接口摘要（接口列表+吞吐率）
func handleInterfaceSummary(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	db := appdb.DB()

	// 查询该设备最近的接口指标（if_speed/if_*_octets/if_*_errors 均为 REAL 类型，需用 NullFloat64 扫描）
	rows, err := db.Query(`
		SELECT if_index, interface_name, if_description, if_speed,
		       if_oper_status, if_admin_status,
		       if_in_octets, if_out_octets, if_in_errors, if_out_errors,
		       if_in_utilization, if_out_utilization,
		       sampled_at
		FROM snmp_interface_metrics
		WHERE device_id = ?
		ORDER BY sampled_at DESC, if_index ASC
		LIMIT 100`, deviceID)
	if err != nil {
		httpx.RespondError(w, "查询接口指标失败", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type IfMetric struct {
		Index          int      `json:"index"`
		Name           string   `json:"name"`
		Description    *string  `json:"description"`
		Speed          *float64 `json:"speed"`
		OperStatus     int      `json:"operStatus"`
		OperStatusText string   `json:"operStatusText"`
		AdminStatus    int      `json:"adminStatus"`
		InOctets       *float64 `json:"inOctets"`
		OutOctets      *float64 `json:"outOctets"`
		InErrors       *float64 `json:"inErrors"`
		OutErrors      *float64 `json:"outErrors"`
		InUtilization  *float64 `json:"if_in_utilization"`
		OutUtilization *float64 `json:"if_out_utilization"`
	}

	latestByIndex := make(map[int]IfMetric)
	var lastSampledAt *string

	for rows.Next() {
		var idx int
		var name string
		var desc sql.NullString
		var speed sql.NullFloat64
		var operStatus, adminStatus int
		var inOct, outOct, inErr, outErr, inUtil, outUtil sql.NullFloat64
		var sampled string

		if err := rows.Scan(&idx, &name, &desc, &speed, &operStatus, &adminStatus,
			&inOct, &outOct, &inErr, &outErr, &inUtil, &outUtil, &sampled); err != nil {
			continue
		}

		if lastSampledAt == nil {
			lastSampledAt = &sampled
		}

		if _, exists := latestByIndex[idx]; !exists {
			m := IfMetric{
				Index:          idx,
				Name:           name,
				OperStatus:     operStatus,
				OperStatusText: operStatusText(operStatus),
				AdminStatus:    adminStatus,
			}
			if desc.Valid {
				m.Description = &desc.String
			}
			if speed.Valid {
				m.Speed = &speed.Float64
			}
			if inOct.Valid {
				m.InOctets = &inOct.Float64
			}
			if outOct.Valid {
				m.OutOctets = &outOct.Float64
			}
			if inErr.Valid {
				m.InErrors = &inErr.Float64
			}
			if outErr.Valid {
				m.OutErrors = &outErr.Float64
			}
			if inUtil.Valid {
				m.InUtilization = &inUtil.Float64
			}
			if outUtil.Valid {
				m.OutUtilization = &outUtil.Float64
			}
			latestByIndex[idx] = m
		}
	}

	interfaces := make([]IfMetric, 0, len(latestByIndex))
	for _, m := range latestByIndex {
		interfaces = append(interfaces, m)
	}

	totalUp := 0
	totalDown := 0
	for _, m := range interfaces {
		if m.OperStatus == 1 {
			totalUp++
		} else {
			totalDown++
		}
	}

	// 计算吞吐率（最近两次采样差值）
	type Throughput struct {
		InMbps    float64 `json:"inMbps"`
		OutMbps   float64 `json:"outMbps"`
		TotalMbps float64 `json:"totalMbps"`
	}
	var throughput *Throughput

	tpRows, err := db.Query(`
		WITH Ranked AS (
			SELECT 
				device_id, if_index, if_speed, if_in_octets, if_out_octets, sampled_at,
				ROW_NUMBER() OVER (PARTITION BY device_id, if_index ORDER BY sampled_at DESC) as rn
			FROM snmp_interface_metrics
			WHERE device_id = ?
		)
		SELECT 
			COALESCE(cur.if_speed, 0),
			(cur.if_in_octets - prev.if_in_octets),
			(cur.if_out_octets - prev.if_out_octets),
			(julianday(cur.sampled_at) - julianday(prev.sampled_at)) * 86400
		FROM Ranked cur
		JOIN Ranked prev 
			ON cur.device_id = prev.device_id 
			AND cur.if_index = prev.if_index 
			AND prev.rn = 2
		WHERE cur.rn = 1
		  AND cur.if_in_octets >= prev.if_in_octets
		  AND cur.if_out_octets >= prev.if_out_octets
		  AND (julianday(cur.sampled_at) - julianday(prev.sampled_at)) * 86400 >= 1
	`, deviceID)

	if err == nil {
		defer tpRows.Close()
		var devInMbps, devOutMbps float64
		hasData := false
		for tpRows.Next() {
			var ifSpeed, deltaIn, deltaOut, sec float64
			if err := tpRows.Scan(&ifSpeed, &deltaIn, &deltaOut, &sec); err == nil && sec >= 1 {
				rateIn := (deltaIn * 8) / (sec * 1_000_000)
				rateOut := (deltaOut * 8) / (sec * 1_000_000)
				if ifSpeed > 0 {
					maxSpeedMbps := (ifSpeed / 1_000_000) * 1.5
					if rateIn > maxSpeedMbps {
						rateIn = 0
					}
					if rateOut > maxSpeedMbps {
						rateOut = 0
					}
				} else {
					if rateIn > 100_000 {
						rateIn = 0
					}
					if rateOut > 100_000 {
						rateOut = 0
					}
				}
				devInMbps += rateIn
				devOutMbps += rateOut
				hasData = true
			}
		}
		if hasData {
			throughput = &Throughput{
				InMbps:    roundTo2(devInMbps),
				OutMbps:   roundTo2(devOutMbps),
				TotalMbps: roundTo2(devInMbps + devOutMbps),
			}
		}
	}

	httpx.Respond(w, map[string]interface{}{
		"interfaces":    interfaces,
		"totalUp":       totalUp,
		"totalDown":     totalDown,
		"lastSampledAt": lastSampledAt,
		"throughput":    throughput,
	})
}

// handleInterfaceStatsSummary 返回网络设备接口统计概览（对应大屏顶部6个卡片：吞吐量、总流量、丢包错包率）
func handleInterfaceStatsSummary(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	db := appdb.DB()

	var whereClause string
	var args []interface{}
	if deviceID != "" && deviceID != "all" {
		whereClause = "WHERE device_id = ?"
		args = append(args, deviceID)
	}

	// 1. 查询选定范围内最近接口测得的流量字节与错包统计
	var totalInOctets, totalOutOctets, totalInErrors, totalOutErrors sql.NullFloat64
	querySummary := `
		SELECT 
			SUM(if_in_octets),
			SUM(if_out_octets),
			SUM(if_in_errors),
			SUM(if_out_errors)
		FROM (
			SELECT device_id, if_index, if_in_octets, if_out_octets, if_in_errors, if_out_errors,
			       ROW_NUMBER() OVER (PARTITION BY device_id, if_index ORDER BY sampled_at DESC) as rn
			FROM snmp_interface_metrics
			` + whereClause + `
		) WHERE rn = 1`
	_ = db.QueryRow(querySummary, args...).Scan(&totalInOctets, &totalOutOctets, &totalInErrors, &totalOutErrors)

	// 2. 查询选定范围内的实时吞吐速率 (Mbps) — 按设备及接口维度独立计算差值与耗时，防止跨设备时间戳错位与计数器溢出
	var tpWhere string
	var tpArgs []interface{}
	if deviceID != "" && deviceID != "all" {
		tpWhere = "WHERE cur.device_id = ?"
		tpArgs = append(tpArgs, deviceID)
	}

	tpRows, err := db.Query(`
		WITH Ranked AS (
			SELECT 
				device_id, if_index, if_speed, if_in_octets, if_out_octets, sampled_at,
				ROW_NUMBER() OVER (PARTITION BY device_id, if_index ORDER BY sampled_at DESC) as rn
			FROM snmp_interface_metrics
		)
		SELECT 
			COALESCE(cur.if_speed, 0),
			(cur.if_in_octets - prev.if_in_octets),
			(cur.if_out_octets - prev.if_out_octets),
			(julianday(cur.sampled_at) - julianday(prev.sampled_at)) * 86400
		FROM Ranked cur
		JOIN Ranked prev 
			ON cur.device_id = prev.device_id 
			AND cur.if_index = prev.if_index 
			AND prev.rn = 2
		`+tpWhere+`
		WHERE cur.rn = 1
		  AND cur.if_in_octets >= prev.if_in_octets
		  AND cur.if_out_octets >= prev.if_out_octets
		  AND (julianday(cur.sampled_at) - julianday(prev.sampled_at)) * 86400 >= 1
	`, tpArgs...)

	inMbps := 0.0
	outMbps := 0.0
	if err == nil {
		defer tpRows.Close()
		for tpRows.Next() {
			var ifSpeed, deltaIn, deltaOut, sec float64
			if err := tpRows.Scan(&ifSpeed, &deltaIn, &deltaOut, &sec); err == nil && sec >= 1 {
				rateIn := (deltaIn * 8) / (sec * 1_000_000)
				rateOut := (deltaOut * 8) / (sec * 1_000_000)

				// 物理接口速率合理性保护（防止 SNMP 计数器溢出或设备重启跳变）
				if ifSpeed > 0 {
					maxSpeedMbps := (ifSpeed / 1_000_000) * 1.5
					if rateIn > maxSpeedMbps {
						rateIn = 0
					}
					if rateOut > maxSpeedMbps {
						rateOut = 0
					}
				} else {
					if rateIn > 100_000 {
						rateIn = 0
					}
					if rateOut > 100_000 {
						rateOut = 0
					}
				}

				inMbps += rateIn
				outMbps += rateOut
			}
		}
	}
	inMbps = roundTo2(inMbps)
	outMbps = roundTo2(outMbps)

	// 转化为真实 GB 流量 (未采集到数据或无设备时严格返回 0)
	inBytes := 0.0
	if totalInOctets.Valid {
		inBytes = totalInOctets.Float64
	}
	outBytes := 0.0
	if totalOutOctets.Valid {
		outBytes = totalOutOctets.Float64
	}

	inGb := roundTo2(inBytes / (1024 * 1024 * 1024))
	outGb := roundTo2(outBytes / (1024 * 1024 * 1024))

	// 丢包率 & 错包率 (基于真实接口错误包统计)
	errorRate := 0.00
	if totalInOctets.Valid && totalInOctets.Float64 > 0 && totalInErrors.Valid && totalInErrors.Float64 > 0 {
		errorRate = roundTo2((totalInErrors.Float64 / totalInOctets.Float64) * 100)
	}

	httpx.Respond(w, map[string]interface{}{
		"inMbps":           inMbps,
		"inMbpsTrend":      0.0,
		"outMbps":          outMbps,
		"outMbpsTrend":     0.0,
		"totalInVolumeGb":  inGb,
		"totalOutVolumeGb": outGb,
		"packetLossRate":   0.00,
		"packetErrorRate":  errorRate,
		"deviceId":         deviceID,
	})
}

// operStatusText 将数值状态转为可读文本
func operStatusText(status int) string {
	if status == 1 {
		return "up"
	}
	return "down"
}

func roundTo2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
