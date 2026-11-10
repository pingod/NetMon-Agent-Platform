// SNMP 指标命名体系 + 通用时序样本 + 面板配置 数据访问层
package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"itops-backend/internal/appdb"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────
// 指标定义 (snmp_metric_definitions) — snmp_exporter 风格
// ─────────────────────────────────────────────────────────────

// SnmpMetricDefinition 指标定义：命名规则 + 标签派生规则
type SnmpMetricDefinition struct {
	ID               string `json:"id"`
	MetricName       string `json:"metric_name"`
	ModuleName       string `json:"module_name"`
	OID              string `json:"oid"`          // 标量 OID 或表列 OID 模板（含 {index}）
	Description      string `json:"description"`
	MetricType       string `json:"metric_type"`  // gauge | counter
	IsTable          int    `json:"is_table"`
	IndexOID         string `json:"index_oid"`    // 索引列 OID（表型）
	IndexTag         string `json:"index_tag"`    // 索引标签名（如 ifIndex）
	LabelOIDs        string `json:"label_oids"`   // 附加标签列 OID（JSON 数组）
	LabelNames       string `json:"label_names"`  // 附加标签名（JSON 数组）
	Unit             string `json:"unit"`
	Source           string `json:"source"`        // mib | generator
	GeneratorModule  string `json:"generator_module"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

var SnmpMetricDefs = &SnmpMetricDefinitionRepo{}

type SnmpMetricDefinitionRepo struct{}

// UpsertByMetricName 按 metric_name 唯一插入或更新（避免重复定义）
func (SnmpMetricDefinitionRepo) Upsert(def *SnmpMetricDefinition) error {
	db := appdb.MetricsDB()
	if db == nil {
		return fmt.Errorf("metrics database not initialized")
	}
	if def.ID == "" {
		def.ID = uuid.New().String()
	}
	now := nowLocal()
	_, err := db.Exec(`
		INSERT INTO snmp_metric_definitions
			(id, metric_name, module_name, oid, description, metric_type, is_table,
			 index_oid, index_tag, label_oids, label_names, unit, source, generator_module, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(metric_name) DO UPDATE SET
			module_name=excluded.module_name, oid=excluded.oid, description=excluded.description,
			metric_type=excluded.metric_type, is_table=excluded.is_table, index_oid=excluded.index_oid,
			index_tag=excluded.index_tag, label_oids=excluded.label_oids, label_names=excluded.label_names,
			unit=excluded.unit, source=excluded.source, generator_module=excluded.generator_module,
			updated_at=excluded.updated_at`,
		def.ID, def.MetricName, def.ModuleName, def.OID, def.Description,
		def.MetricType, def.IsTable, def.IndexOID, def.IndexTag,
		def.LabelOIDs, def.LabelNames, def.Unit, def.Source, def.GeneratorModule,
		now, now)
	return err
}

// List 列出所有指标定义（支持按 module 过滤）
func (SnmpMetricDefinitionRepo) List(moduleName string) ([]SnmpMetricDefinition, error) {
	db := appdb.MetricsDB()
	if db == nil {
		return nil, fmt.Errorf("metrics database not initialized")
	}
	var rows *sql.Rows
	var err error
	if moduleName != "" {
		rows, err = db.Query(`SELECT id, metric_name, module_name, oid, description, metric_type,
			is_table, index_oid, index_tag, label_oids, label_names, unit, source, generator_module, created_at, updated_at
			FROM snmp_metric_definitions WHERE module_name = ? ORDER BY metric_name`, moduleName)
	} else {
		rows, err = db.Query(`SELECT id, metric_name, module_name, oid, description, metric_type,
			is_table, index_oid, index_tag, label_oids, label_names, unit, source, generator_module, created_at, updated_at
			FROM snmp_metric_definitions ORDER BY metric_name`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SnmpMetricDefinition
	for rows.Next() {
		var d SnmpMetricDefinition
		var moduleName, oid, desc, mtype, idxOid, idxTag, lblOids, lblNames, unit, src, genMod, created, updated sql.NullString
		var isTable sql.NullInt64
		if err := rows.Scan(&d.ID, &d.MetricName, &moduleName, &oid, &desc, &mtype,
			&isTable, &idxOid, &idxTag, &lblOids, &lblNames, &unit, &src, &genMod, &created, &updated); err != nil {
			continue
		}
		d.ModuleName = nsStr(moduleName)
		d.OID = nsStr(oid)
		d.Description = nsStr(desc)
		d.MetricType = nsStr(mtype)
		if isTable.Valid {
			d.IsTable = int(isTable.Int64)
		}
		d.IndexOID = nsStr(idxOid)
		d.IndexTag = nsStr(idxTag)
		d.LabelOIDs = nsStr(lblOids)
		d.LabelNames = nsStr(lblNames)
		d.Unit = nsStr(unit)
		d.Source = nsStr(src)
		d.GeneratorModule = nsStr(genMod)
		d.CreatedAt = nsStr(created)
		d.UpdatedAt = nsStr(updated)
		items = append(items, d)
	}
	if items == nil {
		items = []SnmpMetricDefinition{}
	}
	return items, nil
}

// Search 搜索指标定义（支持按 module 过滤、q 模糊搜索、limit 分页限制）
func (SnmpMetricDefinitionRepo) Search(moduleName, q string, limit int) ([]SnmpMetricDefinition, error) {
	db := appdb.MetricsDB()
	if db == nil {
		return nil, fmt.Errorf("metrics database not initialized")
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	query := `SELECT id, metric_name, module_name, oid, description, metric_type,
		is_table, index_oid, index_tag, label_oids, label_names, unit, source, generator_module, created_at, updated_at
		FROM snmp_metric_definitions WHERE 1=1`
	var args []interface{}
	if moduleName != "" {
		query += " AND module_name = ?"
		args = append(args, moduleName)
	}
	if q != "" {
		pattern := "%" + q + "%"
		query += " AND (metric_name LIKE ? OR description LIKE ? OR oid LIKE ? OR module_name LIKE ?)"
		args = append(args, pattern, pattern, pattern, pattern)
	}
	query += " ORDER BY metric_name ASC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SnmpMetricDefinition
	for rows.Next() {
		var d SnmpMetricDefinition
		var moduleName, oid, desc, mtype, idxOid, idxTag, lblOids, lblNames, unit, src, genMod, created, updated sql.NullString
		var isTable sql.NullInt64
		if err := rows.Scan(&d.ID, &d.MetricName, &moduleName, &oid, &desc, &mtype,
			&isTable, &idxOid, &idxTag, &lblOids, &lblNames, &unit, &src, &genMod, &created, &updated); err != nil {
			continue
		}
		d.ModuleName = nsStr(moduleName)
		d.OID = nsStr(oid)
		d.Description = nsStr(desc)
		d.MetricType = nsStr(mtype)
		d.IsTable = int(isTable.Int64)
		d.IndexOID = nsStr(idxOid)
		d.IndexTag = nsStr(idxTag)
		d.LabelOIDs = nsStr(lblOids)
		d.LabelNames = nsStr(lblNames)
		d.Unit = nsStr(unit)
		d.Source = nsStr(src)
		d.GeneratorModule = nsStr(genMod)
		d.CreatedAt = nsStr(created)
		d.UpdatedAt = nsStr(updated)
		items = append(items, d)
	}
	if items == nil {
		items = []SnmpMetricDefinition{}
	}
	return items, nil
}

// GetByMetricName 按 metric_name 获取定义
func (SnmpMetricDefinitionRepo) GetByMetricName(name string) (*SnmpMetricDefinition, error) {
	db := appdb.MetricsDB()
	if db == nil {
		return nil, fmt.Errorf("metrics database not initialized")
	}
	var d SnmpMetricDefinition
	var moduleName, oid, desc, mtype, idxOid, idxTag, lblOids, lblNames, unit, src, genMod, created, updated sql.NullString
	var isTable sql.NullInt64
	err := db.QueryRow(`SELECT id, metric_name, module_name, oid, description, metric_type,
		is_table, index_oid, index_tag, label_oids, label_names, unit, source, generator_module, created_at, updated_at
		FROM snmp_metric_definitions WHERE metric_name = ?`, name).
		Scan(&d.ID, &d.MetricName, &moduleName, &oid, &desc, &mtype,
			&isTable, &idxOid, &idxTag, &lblOids, &lblNames, &unit, &src, &genMod, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.ModuleName = nsStr(moduleName)
	d.OID = nsStr(oid)
	d.Description = nsStr(desc)
	d.MetricType = nsStr(mtype)
	if isTable.Valid {
		d.IsTable = int(isTable.Int64)
	}
	d.IndexOID = nsStr(idxOid)
	d.IndexTag = nsStr(idxTag)
	d.LabelOIDs = nsStr(lblOids)
	d.LabelNames = nsStr(lblNames)
	d.Unit = nsStr(unit)
	d.Source = nsStr(src)
	d.GeneratorModule = nsStr(genMod)
	d.CreatedAt = nsStr(created)
	d.UpdatedAt = nsStr(updated)
	return &d, nil
}

// ─────────────────────────────────────────────────────────────
// 通用时序样本 (snmp_metric_samples)
// ─────────────────────────────────────────────────────────────

// SnmpMetricSample 一条采集样本
type SnmpMetricSample struct {
	ID           string  `json:"id"`
	DefinitionID string  `json:"definition_id"`
	MetricName   string  `json:"metric_name"`
	DeviceID     string  `json:"device_id"`
	TagsJSON     string  `json:"tags_json"` // {"ifIndex":"10","ifDescr":"..."}
	Value        float64 `json:"value"`
	CollectedAt  string  `json:"collected_at"`
}

var SnmpMetricSamples = &SnmpMetricSampleRepo{}

type SnmpMetricSampleRepo struct{}

// InsertBatch 批量写入样本
func (SnmpMetricSampleRepo) InsertBatch(samples []SnmpMetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	db := appdb.MetricsDB()
	if db == nil {
		return fmt.Errorf("metrics database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO snmp_metric_samples
		(id, definition_id, metric_name, device_id, tags_json, value, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range samples {
		id := s.ID
		if id == "" {
			id = uuid.New().String()
		}
		tags := s.TagsJSON
		if tags == "" {
			tags = "{}"
		}
		if _, err := stmt.Exec(id, s.DefinitionID, s.MetricName, s.DeviceID, tags, s.Value, s.CollectedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MetricQueryOptions 面板查询选项
type MetricQueryOptions struct {
	MetricName   string
	DeviceID     string
	TagFilters   map[string]string
	GroupBy      []string
	Aggregation  string // avg | max | min | sum | last
	Hours        int
	Limit        int
}

// QueryAggregated 按面板配置聚合查询（Grafana 式）
// 返回时间序列点 + 分组标签
func (SnmpMetricSampleRepo) QueryAggregated(opt MetricQueryOptions) ([]MetricSeries, error) {
	db := appdb.MetricsDB()
	if db == nil {
		return nil, fmt.Errorf("metrics database not initialized")
	}

	hours := opt.Hours
	if hours <= 0 {
		hours = 24
	}

	agg := opt.Aggregation
	if agg == "" {
		agg = "avg"
	}

	// 构建 WHERE 条件
	where := "WHERE metric_name = ? AND collected_at >= datetime('now','localtime', ?||' hours')"
	args := []interface{}{opt.MetricName, fmt.Sprintf("-%d", hours)}

	if opt.DeviceID != "" {
		where += " AND device_id = ?"
		args = append(args, opt.DeviceID)
	}

	// tag 过滤：由于 tags_json 是 JSON 文本，用 LIKE 做前缀匹配（简单可靠）
	for k, v := range opt.TagFilters {
		if v == "" {
			continue
		}
		where += " AND tags_json LIKE ?"
		args = append(args, fmt.Sprintf("%%\"%s\":\"%s\"%%", k, v))
	}

	// 分组字段：优先用 group_by 指定的标签；否则按 device_id 分组
	groupBy := opt.GroupBy
	if len(groupBy) == 0 {
		groupBy = []string{"device_id"}
	}

	// 简化实现：按 device_id 分组 + 时间分桶（1 分钟），标签过滤已通过 WHERE 收缩
	groupClause := "GROUP BY device_id, strftime('%Y-%m-%d %H:%M:00', collected_at)"

	funcExpr := map[string]string{
		"avg":  "AVG(value)",
		"max":  "MAX(value)",
		"min":  "MIN(value)",
		"sum":  "SUM(value)",
		"last": "AVG(value)",
	}[agg]
	if funcExpr == "" {
		funcExpr = "AVG(value)"
	}

	query := fmt.Sprintf(`
		SELECT device_id, strftime('%%Y-%%m-%%d %%H:%%M:00', collected_at) AS bucket, %s AS val
		FROM snmp_metric_samples
		%s
		%s
		ORDER BY bucket ASC`, funcExpr, where, groupClause)

	rows, err := db.Query(query, args...)
	seriesMap := map[string]*MetricSeries{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var deviceID, bucket string
			var val float64
			if err := rows.Scan(&deviceID, &bucket, &val); err != nil {
				continue
			}
			key := deviceID
			s, ok := seriesMap[key]
			if !ok {
				s = &MetricSeries{DeviceID: deviceID, Points: []MetricPoint{}}
				seriesMap[key] = s
			}
			s.Points = append(s.Points, MetricPoint{Timestamp: bucket, Value: val})
		}
	}

	// 若在指定窗口内未落入数据，自动扩大范围至最近 7 天，避免大屏显示空白
	if len(seriesMap) == 0 {
		argsFallback := []interface{}{opt.MetricName}
		whereFallback := "WHERE metric_name = ? AND collected_at >= datetime('now','localtime', '-168 hours')"
		if opt.DeviceID != "" {
			whereFallback += " AND device_id = ?"
			argsFallback = append(argsFallback, opt.DeviceID)
		}
		for k, v := range opt.TagFilters {
			if v != "" {
				whereFallback += " AND tags_json LIKE ?"
				argsFallback = append(argsFallback, fmt.Sprintf("%%\"%s\":\"%s\"%%", k, v))
			}
		}
		queryFB := fmt.Sprintf(`
			SELECT device_id, strftime('%%Y-%%m-%%d %%H:%%M:00', collected_at) AS bucket, %s AS val
			FROM snmp_metric_samples
			%s
			%s
			ORDER BY bucket ASC`, funcExpr, whereFallback, groupClause)
		rowsFB, errFB := db.Query(queryFB, argsFallback...)
		if errFB == nil {
			defer rowsFB.Close()
			for rowsFB.Next() {
				var deviceID, bucket string
				var val float64
				if err := rowsFB.Scan(&deviceID, &bucket, &val); err != nil {
					continue
				}
				key := deviceID
				s, ok := seriesMap[key]
				if !ok {
					s = &MetricSeries{DeviceID: deviceID, Points: []MetricPoint{}}
					seriesMap[key] = s
				}
				s.Points = append(s.Points, MetricPoint{Timestamp: bucket, Value: val})
			}
		}
	}

	result := make([]MetricSeries, 0, len(seriesMap))
	for _, s := range seriesMap {
		result = append(result, *s)
	}
	if result == nil {
		result = []MetricSeries{}
	}
	return result, nil
}

// MetricSeries 一个时间序列（一个设备/一个标签组合）
type MetricSeries struct {
	DeviceID string             `json:"device_id"`
	Tags     map[string]string  `json:"tags,omitempty"`
	Points   []MetricPoint      `json:"points"`
}

// MetricPoint 时间点
type MetricPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// ─────────────────────────────────────────────────────────────
// 大屏面板配置 (dashboard_panels)
// ─────────────────────────────────────────────────────────────

var DashboardPanels = &DashboardPanelRepo{}

type DashboardPanelRepo struct{}

// DashboardPanel 大屏面板配置
type DashboardPanel struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	MetricName      string `json:"metric_name"`
	Aggregation     string `json:"aggregation"`
	FunctionFilter  string `json:"function_filter"`
	TagFilters      string `json:"tag_filters"`  // JSON
	GroupBy         string `json:"group_by"`     // JSON 数组
	ChartType       string `json:"chart_type"`
	TimeRange       string `json:"time_range"`
	RefreshInterval int    `json:"refresh_interval"`
	GridPos         int    `json:"grid_pos"`
	Enabled         int    `json:"enabled"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// List 列出所有面板
func (DashboardPanelRepo) List() ([]DashboardPanel, error) {
	db := appdb.MetricsDB()
	if db == nil {
		return nil, fmt.Errorf("metrics database not initialized")
	}
	rows, err := db.Query(`SELECT id, title, metric_name, aggregation, function_filter,
		tag_filters, group_by, chart_type, time_range, refresh_interval, grid_pos, enabled, created_at, updated_at
		FROM dashboard_panels ORDER BY grid_pos ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []DashboardPanel
	for rows.Next() {
		var p DashboardPanel
		var title, metric, agg, fnFilter, tagFilters, groupBy, chartType, timeRange, created, updated sql.NullString
		var refresh, gridPos, enabled sql.NullInt64
		if err := rows.Scan(&p.ID, &title, &metric, &agg, &fnFilter, &tagFilters, &groupBy,
			&chartType, &timeRange, &refresh, &gridPos, &enabled, &created, &updated); err != nil {
			continue
		}
		p.Title = nsStr(title)
		p.MetricName = nsStr(metric)
		p.Aggregation = nsStr(agg)
		p.FunctionFilter = nsStr(fnFilter)
		p.TagFilters = nsStr(tagFilters)
		p.GroupBy = nsStr(groupBy)
		p.ChartType = nsStr(chartType)
		p.TimeRange = nsStr(timeRange)
		if refresh.Valid {
			p.RefreshInterval = int(refresh.Int64)
		}
		if gridPos.Valid {
			p.GridPos = int(gridPos.Int64)
		}
		if enabled.Valid {
			p.Enabled = int(enabled.Int64)
		}
		p.CreatedAt = nsStr(created)
		p.UpdatedAt = nsStr(updated)
		items = append(items, p)
	}
	if items == nil {
		items = []DashboardPanel{}
	}
	return items, nil
}

// Create 创建面板
func (DashboardPanelRepo) Create(p *DashboardPanel) error {
	db := appdb.MetricsDB()
	if db == nil {
		return fmt.Errorf("metrics database not initialized")
	}
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := nowLocal()
	_, err := db.Exec(`INSERT INTO dashboard_panels
		(id, title, metric_name, aggregation, function_filter, tag_filters, group_by, chart_type, time_range, refresh_interval, grid_pos, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Title, p.MetricName, p.Aggregation, p.FunctionFilter, p.TagFilters, p.GroupBy,
		p.ChartType, p.TimeRange, p.RefreshInterval, p.GridPos, p.Enabled, now, now)
	return err
}

// Update 更新面板
func (DashboardPanelRepo) Update(id string, p *DashboardPanel) error {
	db := appdb.MetricsDB()
	if db == nil {
		return fmt.Errorf("metrics database not initialized")
	}
	now := nowLocal()
	_, err := db.Exec(`UPDATE dashboard_panels SET
		title=?, metric_name=?, aggregation=?, function_filter=?, tag_filters=?, group_by=?,
		chart_type=?, time_range=?, refresh_interval=?, grid_pos=?, enabled=?, updated_at=?
		WHERE id=?`,
		p.Title, p.MetricName, p.Aggregation, p.FunctionFilter, p.TagFilters, p.GroupBy,
		p.ChartType, p.TimeRange, p.RefreshInterval, p.GridPos, p.Enabled, now, id)
	return err
}

// Delete 删除面板
func (DashboardPanelRepo) Delete(id string) error {
	db := appdb.MetricsDB()
	if db == nil {
		return fmt.Errorf("metrics database not initialized")
	}
	_, err := db.Exec(`DELETE FROM dashboard_panels WHERE id=?`, id)
	return err
}

// ─────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────

// ParseTagFilters 解析 tag_filters JSON 字符串为 map
func ParseTagFilters(jsonStr string) map[string]string {
	result := map[string]string{}
	if jsonStr == "" {
		return result
	}
	_ = json.Unmarshal([]byte(jsonStr), &result)
	return result
}

// ParseStringArray 解析 JSON 字符串数组
func ParseStringArray(jsonStr string) []string {
	var result []string
	if jsonStr == "" {
		return result
	}
	_ = json.Unmarshal([]byte(jsonStr), &result)
	return result
}
