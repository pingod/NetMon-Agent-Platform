// 服务器指标查询层 — server_metrics 表读操作
package repos

import (
	"database/sql"
	"fmt"

	"itops-backend/internal/appdb"
)

// ServerMetric 服务器性能指标记录
type ServerMetric struct {
	ID             string   `json:"id"`
	ServerID       string   `json:"server_id"`
	CPUUsage       *float64 `json:"cpu_usage"`
	MemoryUsage    *float64 `json:"memory_usage"`
	MemoryTotalGB  *float64 `json:"memory_total_gb"`
	MemoryUsedGB   *float64 `json:"memory_used_gb"`
	DiskUsage      *float64 `json:"disk_usage"`
	DiskTotalGB    *float64 `json:"disk_total_gb"`
	DiskUsedGB     *float64 `json:"disk_used_gb"`
	NetworkInMbps  *float64 `json:"network_in_mbps"`
	NetworkOutMbps *float64 `json:"network_out_mbps"`
	Load1min       *float64 `json:"load_1min"`
	Load5min       *float64 `json:"load_5min"`
	Load15min      *float64 `json:"load_15min"`
	UptimeSeconds  *int64   `json:"uptime_seconds"`
	CollectedAt    string   `json:"collected_at"`
}

// ServerMetricWithInfo 带服务器名称的指标
type ServerMetricWithInfo struct {
	ServerMetric
	ServerName  string  `json:"server_name"`
	Hostname    string  `json:"hostname"`
	OSType      string  `json:"os_type"`
}

var ServerMetrics = &ServerMetricRepo{}

type ServerMetricRepo struct{}

// GetAllLatest 获取所有服务器的最新指标
func (ServerMetricRepo) GetAllLatest() ([]ServerMetricWithInfo, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT sm.id, sm.server_id, sm.cpu_usage, sm.memory_usage,
		       sm.memory_total_gb, sm.memory_used_gb,
		       sm.disk_usage, sm.disk_total_gb, sm.disk_used_gb,
		       sm.network_in_mbps, sm.network_out_mbps,
		       sm.load_1min, sm.load_5min, sm.load_15min, sm.uptime_seconds,
		       sm.collected_at,
		       COALESCE(s.name, ''), COALESCE(s.hostname, ''), COALESCE(s.os_type, 'linux')
		FROM server_metrics sm
		INNER JOIN (
			SELECT server_id, MAX(collected_at) AS max_time
			FROM server_metrics
			GROUP BY server_id
		) latest ON sm.server_id = latest.server_id AND sm.collected_at = latest.max_time
		LEFT JOIN servers s ON sm.server_id = s.id
		ORDER BY sm.collected_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ServerMetricWithInfo
	for rows.Next() {
		var m ServerMetricWithInfo
		var cpuU, memU, memT, memUsd, dU, dT, dUsd, netIn, netOut, l1, l5, l15 sql.NullFloat64
		var uptime sql.NullInt64
		if err := rows.Scan(&m.ID, &m.ServerID, &cpuU, &memU,
			&memT, &memUsd, &dU, &dT, &dUsd,
			&netIn, &netOut, &l1, &l5, &l15, &uptime,
			&m.CollectedAt, &m.ServerName, &m.Hostname, &m.OSType); err != nil {
			continue
		}
		m.CPUUsage = floatPtrFromNull(cpuU)
		m.MemoryUsage = floatPtrFromNull(memU)
		m.MemoryTotalGB = floatPtrFromNull(memT)
		m.MemoryUsedGB = floatPtrFromNull(memUsd)
		m.DiskUsage = floatPtrFromNull(dU)
		m.DiskTotalGB = floatPtrFromNull(dT)
		m.DiskUsedGB = floatPtrFromNull(dUsd)
		m.NetworkInMbps = floatPtrFromNull(netIn)
		m.NetworkOutMbps = floatPtrFromNull(netOut)
		m.Load1min = floatPtrFromNull(l1)
		m.Load5min = floatPtrFromNull(l5)
		m.Load15min = floatPtrFromNull(l15)
		m.UptimeSeconds = intPtrFromNull(uptime)
		items = append(items, m)
	}
	if items == nil {
		items = []ServerMetricWithInfo{}
	}
	return items, nil
}

// GetHistoryByServer 获取指定服务器的指标历史（最近 N 小时）
func (ServerMetricRepo) GetHistoryByServer(serverID string, hours int) ([]ServerMetric, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT id, server_id, cpu_usage, memory_usage,
		       memory_total_gb, memory_used_gb,
		       disk_usage, disk_total_gb, disk_used_gb,
		       network_in_mbps, network_out_mbps,
		       load_1min, load_5min, load_15min, uptime_seconds,
		       collected_at
		FROM server_metrics
		WHERE server_id = ? AND collected_at >= datetime('now', 'localtime', ?||' hours')
		ORDER BY collected_at ASC`, serverID, fmt.Sprintf("-%d", hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ServerMetric
	for rows.Next() {
		var m ServerMetric
		var cpuU, memU, memT, memUsd, dU, dT, dUsd, netIn, netOut, l1, l5, l15 sql.NullFloat64
		var uptime sql.NullInt64
		if err := rows.Scan(&m.ID, &m.ServerID, &cpuU, &memU,
			&memT, &memUsd, &dU, &dT, &dUsd,
			&netIn, &netOut, &l1, &l5, &l15, &uptime,
			&m.CollectedAt); err != nil {
			continue
		}
		m.CPUUsage = floatPtrFromNull(cpuU)
		m.MemoryUsage = floatPtrFromNull(memU)
		m.MemoryTotalGB = floatPtrFromNull(memT)
		m.MemoryUsedGB = floatPtrFromNull(memUsd)
		m.DiskUsage = floatPtrFromNull(dU)
		m.DiskTotalGB = floatPtrFromNull(dT)
		m.DiskUsedGB = floatPtrFromNull(dUsd)
		m.NetworkInMbps = floatPtrFromNull(netIn)
		m.NetworkOutMbps = floatPtrFromNull(netOut)
		m.Load1min = floatPtrFromNull(l1)
		m.Load5min = floatPtrFromNull(l5)
		m.Load15min = floatPtrFromNull(l15)
		m.UptimeSeconds = intPtrFromNull(uptime)
		items = append(items, m)
	}
	if items == nil {
		items = []ServerMetric{}
	}
	return items, nil
}

// GetAggregated 获取所有服务器的聚合指标（平均值）
func (ServerMetricRepo) GetAggregated() (*AggregatedMetrics, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	agg := &AggregatedMetrics{}
	// 取最新一轮采集的聚合值
	err := db.QueryRow(`
		SELECT
			COALESCE(AVG(cpu_usage), 0),
			COALESCE(AVG(memory_usage), 0),
			COALESCE(AVG(disk_usage), 0),
			COALESCE(AVG(network_in_mbps), 0),
			COALESCE(AVG(network_out_mbps), 0),
			COUNT(*)
		FROM server_metrics sm
		INNER JOIN (
			SELECT server_id, MAX(collected_at) AS max_time
			FROM server_metrics
			GROUP BY server_id
		) latest ON sm.server_id = latest.server_id AND sm.collected_at = latest.max_time
	`).Scan(&agg.CPUUsage, &agg.MemoryUsage, &agg.DiskUsage,
		&agg.NetworkInMbps, &agg.NetworkOutMbps, &agg.ServerCount)
	if err != nil {
		return nil, err
	}
	return agg, nil
}

// AggregatedMetrics 聚合指标
type AggregatedMetrics struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    float64 `json:"memory_usage"`
	DiskUsage      float64 `json:"disk_usage"`
	NetworkInMbps  float64 `json:"network_in_mbps"`
	NetworkOutMbps float64 `json:"network_out_mbps"`
	ServerCount    int     `json:"server_count"`
}

// GetTrendData 获取聚合趋势数据（按小时分组，最近24小时）
func (ServerMetricRepo) GetTrendData(hours int) ([]TrendPoint, error) {
	db := appdb.DB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT
			strftime('%Y-%m-%d %H:00', collected_at) AS hour,
			AVG(cpu_usage) AS avg_cpu,
			AVG(memory_usage) AS avg_memory,
			AVG(disk_usage) AS avg_disk,
			AVG(COALESCE(network_in_mbps, 0)) AS avg_net_in,
			AVG(COALESCE(network_out_mbps, 0)) AS avg_net_out
		FROM server_metrics
		WHERE collected_at >= datetime('now', 'localtime', ?||' hours')
		GROUP BY hour
		ORDER BY hour ASC`, fmt.Sprintf("-%d", hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TrendPoint
	for rows.Next() {
		var t TrendPoint
		if err := rows.Scan(&t.Time, &t.CPUUsage, &t.MemoryUsage, &t.DiskUsage,
			&t.NetworkInMbps, &t.NetworkOutMbps); err != nil {
			continue
		}
		items = append(items, t)
	}
	if items == nil {
		items = []TrendPoint{}
	}
	return items, nil
}

// TrendPoint 趋势数据点
type TrendPoint struct {
	Time           string  `json:"time"`
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    float64 `json:"memory_usage"`
	DiskUsage      float64 `json:"disk_usage"`
	NetworkInMbps  float64 `json:"network_in_mbps"`
	NetworkOutMbps float64 `json:"network_out_mbps"`
}

// 辅助函数
func floatPtrFromNull(n sql.NullFloat64) *float64 {
	if n.Valid {
		return &n.Float64
	}
	return nil
}

func intPtrFromNull(n sql.NullInt64) *int64 {
	if n.Valid {
		return &n.Int64
	}
	return nil
}
