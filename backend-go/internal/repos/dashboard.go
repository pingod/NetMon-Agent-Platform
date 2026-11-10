// 自定义大屏指标和列表，等价于 TS customDashboardMetricRepository.ts + customDashboardListRepository.ts
package repos

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"itops-backend/internal/appdb"
)

// ───────────────────────────────────────────────
// 自定义大屏指标
// ───────────────────────────────────────────────

// CustomDashboardMetric 自定义大屏标量指标
type CustomDashboardMetric struct {
	ID              string  `json:"id"`
	OID             string  `json:"oid"`
	CustomName      string  `json:"custom_name"`
	OriginalName    *string `json:"original_name"`
	DeviceID        string  `json:"device_id"`
	ModuleName      *string `json:"module_name"`
	Syntax          *string `json:"syntax"`
	Unit            string  `json:"unit"`
	ShowOnDashboard int     `json:"show_on_dashboard"`
	DisplayOrder    int     `json:"display_order"`
	LastValue       *string `json:"last_value"`
	LastSampledAt   *string `json:"last_sampled_at"`
	SnmpVersion     string  `json:"snmp_version"`
	Community       string  `json:"community"`
	ValueMappings   *string `json:"value_mappings,omitempty"`
	IsTable         int     `json:"is_table"`
	ChartType       string  `json:"chart_type"`
	MetricName      *string `json:"metric_name,omitempty"`
	Tags            *string `json:"tags,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// MetricCreateInput 指标创建输入
type MetricCreateInput struct {
	OID           string `json:"oid"`
	CustomName    string `json:"custom_name"`
	OriginalName  string `json:"original_name"`
	DeviceID      string `json:"device_id"`
	ModuleName    string `json:"module_name"`
	Syntax        string `json:"syntax"`
	Unit          string `json:"unit"`
	SnmpVersion   string `json:"snmp_version"`
	Community     string `json:"community"`
	ValueMappings string `json:"value_mappings"`
	IsTable       int    `json:"is_table"`
	ChartType     string `json:"chart_type"`
	MetricName    string `json:"metric_name"`
	Tags          string `json:"tags"`
	LastValue     string `json:"last_value"`
}

// MetricUpdateInput 指标更新输入
type MetricUpdateInput struct {
	OID             *string `json:"oid"`
	CustomName      *string `json:"custom_name"`
	Unit            *string `json:"unit"`
	ShowOnDashboard *int    `json:"show_on_dashboard"`
	DisplayOrder    *int    `json:"display_order"`
	ValueMappings   *string `json:"value_mappings"`
	IsTable         *int    `json:"is_table"`
	ChartType       *string `json:"chart_type"`
	MetricName      *string `json:"metric_name"`
	Tags            *string `json:"tags"`
	LastValue       *string `json:"last_value"`
}

// CustomDashboardMetricRepo 指标仓库
type CustomDashboardMetricRepo struct{}

var DashboardMetrics = &CustomDashboardMetricRepo{}

const selectMetricFields = `SELECT id, oid, custom_name, original_name, device_id, module_name, syntax, unit, show_on_dashboard, display_order, last_value, last_sampled_at, snmp_version, community, value_mappings, is_table, COALESCE(chart_type, 'auto'), COALESCE(metric_name, ''), COALESCE(tags, ''), created_at, updated_at FROM custom_dashboard_metrics`

func scanMetric(scanner interface{ Scan(...interface{}) error }) (*CustomDashboardMetric, error) {
	var m CustomDashboardMetric
	var originalName, moduleName, syntax, unit, lastValue, lastSampledAt, snmpVersion, community, valueMappings, chartType, metricName, tags sql.NullString
	err := scanner.Scan(&m.ID, &m.OID, &m.CustomName, &originalName, &m.DeviceID, &moduleName, &syntax,
		&unit, &m.ShowOnDashboard, &m.DisplayOrder, &lastValue, &lastSampledAt,
		&snmpVersion, &community, &valueMappings, &m.IsTable, &chartType, &metricName, &tags, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	m.OriginalName = ns(originalName)
	m.ModuleName = ns(moduleName)
	m.Syntax = ns(syntax)
	m.Unit = unit.String
	m.LastValue = ns(lastValue)
	m.LastSampledAt = ns(lastSampledAt)
	if snmpVersion.Valid && snmpVersion.String != "" {
		m.SnmpVersion = snmpVersion.String
	} else {
		m.SnmpVersion = "v2c"
	}
	if community.Valid && community.String != "" {
		m.Community = community.String
	} else {
		m.Community = "public"
	}
	m.ValueMappings = ns(valueMappings)
	if chartType.Valid && chartType.String != "" {
		m.ChartType = chartType.String
	} else {
		m.ChartType = "auto"
	}
	m.MetricName = ns(metricName)
	m.Tags = ns(tags)
	return &m, nil
}

// List 列出所有指标
func (r *CustomDashboardMetricRepo) List() ([]CustomDashboardMetric, error) {
	rows, err := appdb.DB().Query(selectMetricFields + ` ORDER BY display_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDashboardMetric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			continue
		}
		out = append(out, *m)
	}
	return out, nil
}

// ListByDevice 按设备列出指标
func (r *CustomDashboardMetricRepo) ListByDevice(deviceID string) ([]CustomDashboardMetric, error) {
	rows, err := appdb.DB().Query(selectMetricFields + ` WHERE device_id = ? ORDER BY display_order ASC, created_at ASC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDashboardMetric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			continue
		}
		out = append(out, *m)
	}
	return out, nil
}

// ListForDashboard 列出大屏可见指标
func (r *CustomDashboardMetricRepo) ListForDashboard() ([]CustomDashboardMetric, error) {
	rows, err := appdb.DB().Query(selectMetricFields + ` WHERE show_on_dashboard = 1 ORDER BY display_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDashboardMetric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			continue
		}
		out = append(out, *m)
	}
	return out, nil
}

// GetByID 获取单个指标
func (r *CustomDashboardMetricRepo) GetByID(id string) (*CustomDashboardMetric, error) {
	row := appdb.DB().QueryRow(selectMetricFields+` WHERE id = ?`, id)
	return scanMetric(row)
}

// Create 创建或更新指标（按 device_id + oid 自动去重，已存在则更新，不重复新增方格）
func (r *CustomDashboardMetricRepo) Create(input MetricCreateInput) (*CustomDashboardMetric, error) {
	now := nowLocal()
	if input.SnmpVersion == "" {
		input.SnmpVersion = "v2c"
	}
	if input.Community == "" {
		input.Community = "public"
	}
	if input.ChartType == "" {
		input.ChartType = "auto"
	}

	// 检查该设备是否已包含此 OID 的指标方格（兼容带点/不带点格式）
	cleanOID := strings.TrimPrefix(input.OID, ".")
	var existingID string
	err := appdb.DB().QueryRow(`SELECT id FROM custom_dashboard_metrics WHERE device_id = ? AND (oid = ? OR oid = ? OR REPLACE(oid, '.', '') = ?)`, input.DeviceID, input.OID, "."+cleanOID, cleanOID).Scan(&existingID)
	if err == nil && existingID != "" {
		// 已存在，更新现有方格的自定义名称、单位、值映射规则、只读串与新采值，不生成重复方格
		var updateSets []string
		var updateArgs []interface{}

		if input.CustomName != "" {
			updateSets = append(updateSets, "custom_name = ?")
			updateArgs = append(updateArgs, input.CustomName)
		}
		if input.OriginalName != "" {
			updateSets = append(updateSets, "original_name = ?")
			updateArgs = append(updateArgs, input.OriginalName)
		}
		updateSets = append(updateSets, "unit = ?")
		updateArgs = append(updateArgs, input.Unit)

		updateSets = append(updateSets, "value_mappings = ?")
		updateArgs = append(updateArgs, input.ValueMappings)

		updateSets = append(updateSets, "snmp_version = ?")
		updateArgs = append(updateArgs, input.SnmpVersion)

		updateSets = append(updateSets, "community = ?")
		updateArgs = append(updateArgs, input.Community)

		if input.ChartType != "" {
			updateSets = append(updateSets, "chart_type = ?")
			updateArgs = append(updateArgs, input.ChartType)
		}
		if input.MetricName != "" {
			updateSets = append(updateSets, "metric_name = ?")
			updateArgs = append(updateArgs, input.MetricName)
		}
		if input.Tags != "" {
			updateSets = append(updateSets, "tags = ?")
			updateArgs = append(updateArgs, input.Tags)
		}

		updateSets = append(updateSets, "show_on_dashboard = 1")

		if input.LastValue != "" {
			updateSets = append(updateSets, "last_value = ?", "last_sampled_at = ?")
			updateArgs = append(updateArgs, input.LastValue, now)
		}

		updateSets = append(updateSets, "updated_at = ?")
		updateArgs = append(updateArgs, now)

		updateArgs = append(updateArgs, existingID)

		query := "UPDATE custom_dashboard_metrics SET " + strings.Join(updateSets, ", ") + " WHERE id = ?"
		_, _ = appdb.DB().Exec(query, updateArgs...)

		return r.GetByID(existingID)
	}

	// 不存在，插入新指标方格
	id := uuid.NewString()
	var maxOrder sql.NullInt64
	_ = appdb.DB().QueryRow(`SELECT MAX(display_order) FROM custom_dashboard_metrics`).Scan(&maxOrder)
	order := 0
	if maxOrder.Valid {
		order = int(maxOrder.Int64) + 1
	}

	var lastValArg interface{} = nil
	var lastSampArg interface{} = nil
	if input.LastValue != "" {
		lastValArg = input.LastValue
		lastSampArg = now
	}

	_, err = appdb.DB().Exec(`
		INSERT INTO custom_dashboard_metrics (id, oid, custom_name, original_name, device_id, module_name, syntax, unit, value_mappings, show_on_dashboard, display_order, snmp_version, community, is_table, chart_type, metric_name, tags, last_value, last_sampled_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.OID, input.CustomName, sqlNullStr(input.OriginalName),
		input.DeviceID, sqlNullStr(input.ModuleName), sqlNullStr(input.Syntax), sqlNullStr(input.Unit),
		sqlNullStr(input.ValueMappings), order, input.SnmpVersion, input.Community, input.IsTable, input.ChartType,
		sqlNullStr(input.MetricName), sqlNullStr(input.Tags), lastValArg, lastSampArg, now, now)
	if err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

// Update 更新指标
func (r *CustomDashboardMetricRepo) Update(id string, input MetricUpdateInput) error {
	var sets []string
	var args []interface{}
	if input.OID != nil {
		sets = append(sets, "oid = ?")
		args = append(args, *input.OID)
	}
	if input.CustomName != nil {
		sets = append(sets, "custom_name = ?")
		args = append(args, *input.CustomName)
	}
	if input.Unit != nil {
		sets = append(sets, "unit = ?")
		args = append(args, *input.Unit)
	}
	if input.ShowOnDashboard != nil {
		sets = append(sets, "show_on_dashboard = ?")
		args = append(args, *input.ShowOnDashboard)
	}
	if input.DisplayOrder != nil {
		sets = append(sets, "display_order = ?")
		args = append(args, *input.DisplayOrder)
	}
	if input.ValueMappings != nil {
		sets = append(sets, "value_mappings = ?")
		args = append(args, *input.ValueMappings)
	}
	if input.IsTable != nil {
		sets = append(sets, "is_table = ?")
		args = append(args, *input.IsTable)
	}
	if input.ChartType != nil {
		sets = append(sets, "chart_type = ?")
		args = append(args, *input.ChartType)
	}
	if input.MetricName != nil {
		sets = append(sets, "metric_name = ?")
		args = append(args, *input.MetricName)
	}
	if input.Tags != nil {
		sets = append(sets, "tags = ?")
		args = append(args, *input.Tags)
	}
	if input.LastValue != nil {
		sets = append(sets, "last_value = ?")
		args = append(args, *input.LastValue)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, nowLocal())
	args = append(args, id)
	_, err := appdb.DB().Exec("UPDATE custom_dashboard_metrics SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// UpdateValue 更新采样值
func (r *CustomDashboardMetricRepo) UpdateValue(id, value string) error {
	now := nowLocal()
	_, err := appdb.DB().Exec(`UPDATE custom_dashboard_metrics SET last_value = ?, last_sampled_at = ?, updated_at = ? WHERE id = ?`, value, now, now, id)
	return err
}

// Delete 删除指标
func (r *CustomDashboardMetricRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM custom_dashboard_metrics WHERE id = ?`, id)
	return err
}

// BatchCreate 批量创建指标
func (r *CustomDashboardMetricRepo) BatchCreate(items []MetricCreateInput) ([]CustomDashboardMetric, error) {
	var out []CustomDashboardMetric
	for _, item := range items {
		m, err := r.Create(item)
		if err != nil {
			return out, err
		}
		if m != nil {
			out = append(out, *m)
		}
	}
	return out, nil
}

// ───────────────────────────────────────────────
// 自定义大屏列表
// ───────────────────────────────────────────────

// ListColumn 列定义
type ListColumn struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`          // label | metric
	Label         string                 `json:"label"`
	Field         string                 `json:"field,omitempty"`
	OID           string                 `json:"oid,omitempty"`
	IsTable       int                    `json:"is_table,omitempty"`
	Unit          string                 `json:"unit,omitempty"`
	Color         string                 `json:"color,omitempty"`
	BgColor       string                 `json:"bgColor,omitempty"`
	Align         string                 `json:"align,omitempty"`
	Link          bool                   `json:"link,omitempty"`
	ValueMappings map[string]string      `json:"valueMappings,omitempty"`
}

// CustomDashboardList 自定义大屏列表
type CustomDashboardList struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	DeviceIDs       []string     `json:"device_ids"`
	Columns         []ListColumn `json:"columns"`
	RefreshInterval int          `json:"refresh_interval"`
	ShowOnDashboard int          `json:"show_on_dashboard"`
	DisplayOrder    int          `json:"display_order"`
	CreatedAt       string       `json:"created_at"`
	UpdatedAt       string       `json:"updated_at"`
}

// ListCreateInput 列表创建输入
type ListCreateInput struct {
	Name            string
	Description     string
	DeviceIDs       []string
	Columns         []ListColumn
	RefreshInterval int
	ShowOnDashboard int
}

// ListUpdateInput 列表更新输入
type ListUpdateInput struct {
	Name            *string
	Description     *string
	DeviceIDs       *[]string
	Columns         *[]ListColumn
	RefreshInterval *int
	ShowOnDashboard *int
	DisplayOrder    *int
}

// CustomDashboardListRepo 列表仓库
type CustomDashboardListRepo struct{}

var DashboardLists = &CustomDashboardListRepo{}

func parseListRow(id, name, description, deviceIDsJSON, columnsJSON string, refreshInterval, showOnDashboard, displayOrder int, createdAt, updatedAt string) *CustomDashboardList {
	var deviceIDs []string
	_ = json.Unmarshal([]byte(deviceIDsJSON), &deviceIDs)
	if deviceIDs == nil {
		deviceIDs = []string{}
	}
	var columns []ListColumn
	_ = json.Unmarshal([]byte(columnsJSON), &columns)
	if columns == nil {
		columns = []ListColumn{}
	}
	return &CustomDashboardList{
		ID:              id,
		Name:            name,
		Description:     description,
		DeviceIDs:       deviceIDs,
		Columns:         columns,
		RefreshInterval: refreshInterval,
		ShowOnDashboard: showOnDashboard,
		DisplayOrder:    displayOrder,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

func scanList(scanner interface{ Scan(...interface{}) error }) (*CustomDashboardList, error) {
	var id, name, description, deviceIDsJSON, columnsJSON, createdAt, updatedAt string
	var refreshInterval, showOnDashboard, displayOrder int
	err := scanner.Scan(&id, &name, &description, &deviceIDsJSON, &columnsJSON, &refreshInterval, &showOnDashboard, &displayOrder, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return parseListRow(id, name, description, deviceIDsJSON, columnsJSON, refreshInterval, showOnDashboard, displayOrder, createdAt, updatedAt), nil
}

// List 列出所有列表
func (r *CustomDashboardListRepo) List() ([]CustomDashboardList, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, description, device_ids, columns, refresh_interval, show_on_dashboard, display_order, created_at, updated_at FROM custom_dashboard_lists ORDER BY display_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDashboardList
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			continue
		}
		out = append(out, *l)
	}
	return out, nil
}

// ListForDashboard 列出大屏可见列表
func (r *CustomDashboardListRepo) ListForDashboard() ([]CustomDashboardList, error) {
	rows, err := appdb.DB().Query(`SELECT id, name, description, device_ids, columns, refresh_interval, show_on_dashboard, display_order, created_at, updated_at FROM custom_dashboard_lists WHERE show_on_dashboard = 1 ORDER BY display_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDashboardList
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			continue
		}
		out = append(out, *l)
	}
	return out, nil
}

// GetByID 获取单个列表
func (r *CustomDashboardListRepo) GetByID(id string) (*CustomDashboardList, error) {
	row := appdb.DB().QueryRow(`SELECT id, name, description, device_ids, columns, refresh_interval, show_on_dashboard, display_order, created_at, updated_at FROM custom_dashboard_lists WHERE id = ?`, id)
	return scanList(row)
}

// Create 创建列表
func (r *CustomDashboardListRepo) Create(input ListCreateInput) (*CustomDashboardList, error) {
	id := uuid.NewString()
	now := nowLocal()
	if input.RefreshInterval == 0 {
		input.RefreshInterval = 60
	}
	if input.ShowOnDashboard == 0 {
		input.ShowOnDashboard = 1
	}
	deviceIDsJSON, _ := json.Marshal(input.DeviceIDs)
	if string(deviceIDsJSON) == "null" {
		deviceIDsJSON = []byte("[]")
	}
	columnsJSON, _ := json.Marshal(input.Columns)
	if string(columnsJSON) == "null" {
		columnsJSON = []byte("[]")
	}
	// 获取最大 display_order
	var maxOrder sql.NullInt64
	_ = appdb.DB().QueryRow(`SELECT MAX(display_order) FROM custom_dashboard_lists`).Scan(&maxOrder)
	order := 0
	if maxOrder.Valid {
		order = int(maxOrder.Int64) + 1
	}
	_, err := appdb.DB().Exec(`
		INSERT INTO custom_dashboard_lists (id, name, description, device_ids, columns, refresh_interval, show_on_dashboard, display_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.Description, string(deviceIDsJSON), string(columnsJSON),
		input.RefreshInterval, input.ShowOnDashboard, order, now, now)
	if err != nil {
		return nil, err
	}
	return scanList(appdb.DB().QueryRow(`SELECT id, name, description, device_ids, columns, refresh_interval, show_on_dashboard, display_order, created_at, updated_at FROM custom_dashboard_lists WHERE id = ?`, id))
}

// Update 更新列表
func (r *CustomDashboardListRepo) Update(id string, input ListUpdateInput) error {
	var sets []string
	var args []interface{}
	if input.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *input.Name)
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *input.Description)
	}
	if input.DeviceIDs != nil {
		deviceIDsJSON, _ := json.Marshal(*input.DeviceIDs)
		sets = append(sets, "device_ids = ?")
		args = append(args, string(deviceIDsJSON))
	}
	if input.Columns != nil {
		columnsJSON, _ := json.Marshal(*input.Columns)
		sets = append(sets, "columns = ?")
		args = append(args, string(columnsJSON))
	}
	if input.RefreshInterval != nil {
		sets = append(sets, "refresh_interval = ?")
		args = append(args, *input.RefreshInterval)
	}
	if input.ShowOnDashboard != nil {
		sets = append(sets, "show_on_dashboard = ?")
		args = append(args, *input.ShowOnDashboard)
	}
	if input.DisplayOrder != nil {
		sets = append(sets, "display_order = ?")
		args = append(args, *input.DisplayOrder)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, nowLocal())
	args = append(args, id)
	_, err := appdb.DB().Exec("UPDATE custom_dashboard_lists SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// Delete 删除列表
func (r *CustomDashboardListRepo) Delete(id string) error {
	_, err := appdb.DB().Exec(`DELETE FROM custom_dashboard_lists WHERE id = ?`, id)
	return err
}
