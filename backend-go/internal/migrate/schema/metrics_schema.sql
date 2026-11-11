-- metrics.db 专用 schema
-- 包含所有设备指标采集相关的表
-- 与 app.db 中的元数据表分离，便于独立备份和清理

-- SNMP 采集任务定义
CREATE TABLE IF NOT EXISTS snmp_collect_tasks (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    task_name TEXT NOT NULL DEFAULT '',
    oid TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_type TEXT NOT NULL DEFAULT 'scalar',
    table_oid TEXT DEFAULT '',
    index_oid TEXT DEFAULT '',
    value_oid TEXT DEFAULT '',
    poll_interval INTEGER NOT NULL DEFAULT 300,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- SNMP 采集结果数据
CREATE TABLE IF NOT EXISTS snmp_collect_results (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    value TEXT NOT NULL,
    unit TEXT DEFAULT '',
    collected_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- 网络设备接口流量指标
CREATE TABLE IF NOT EXISTS snmp_interface_metrics (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    if_index INTEGER NOT NULL,
    if_descr TEXT DEFAULT '',
    if_alias TEXT DEFAULT '',
    if_type INTEGER DEFAULT 0,
    if_speed REAL DEFAULT 0,
    if_admin_status INTEGER DEFAULT 0,
    if_oper_status INTEGER DEFAULT 0,
    if_in_octets REAL DEFAULT 0,
    if_out_octets REAL DEFAULT 0,
    if_in_ucast_pkts REAL DEFAULT 0,
    if_out_ucast_pkts REAL DEFAULT 0,
    if_in_errors REAL DEFAULT 0,
    if_out_errors REAL DEFAULT 0,
    if_in_discards REAL DEFAULT 0,
    if_out_discards REAL DEFAULT 0,
    if_high_speed REAL DEFAULT 0,
    if_in_broadcast_pkts REAL DEFAULT 0,
    if_out_broadcast_pkts REAL DEFAULT 0,
    if_in_multicast_pkts REAL DEFAULT 0,
    if_out_multicast_pkts REAL DEFAULT 0,
    if_hc_in_octets REAL DEFAULT 0,
    if_hc_out_octets REAL DEFAULT 0,
    if_hc_in_ucast_pkts REAL DEFAULT 0,
    if_hc_out_ucast_pkts REAL DEFAULT 0,
    prev_in_octets REAL DEFAULT 0,
    prev_out_octets REAL DEFAULT 0,
    prev_timestamp DATETIME,
    prev_in_ucast_pkts REAL DEFAULT 0,
    prev_out_ucast_pkts REAL DEFAULT 0,
    prev_in_broadcast_pkts REAL DEFAULT 0,
    prev_out_broadcast_pkts REAL DEFAULT 0,
    prev_in_multicast_pkts REAL DEFAULT 0,
    prev_out_multicast_pkts REAL DEFAULT 0,
    prev_hc_in_octets REAL DEFAULT 0,
    prev_hc_out_octets REAL DEFAULT 0,
    throughput_in_bps REAL DEFAULT 0,
    throughput_out_bps REAL DEFAULT 0,
    throughput_in_pps REAL DEFAULT 0,
    throughput_out_pps REAL DEFAULT 0,
    utilization_in REAL DEFAULT 0,
    utilization_out REAL DEFAULT 0,
    collected_at DATETIME DEFAULT (datetime('now','localtime')),
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- 自定义 OID 采集模板
CREATE TABLE IF NOT EXISTS snmp_custom_oid_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    vendor TEXT DEFAULT '',
    oids TEXT DEFAULT '[]',
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- 自定义 OID 采集结果
CREATE TABLE IF NOT EXISTS snmp_custom_oid_results (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    oid TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    value TEXT NOT NULL,
    unit TEXT DEFAULT '',
    collected_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- SNMP Trap 事件记录
CREATE TABLE IF NOT EXISTS snmp_trap_events (
    id TEXT PRIMARY KEY,
    device_id TEXT,
    device_ip TEXT DEFAULT '',
    trap_oid TEXT DEFAULT '',
    trap_type INTEGER DEFAULT 0,
    varbinds TEXT DEFAULT '{}',
    severity TEXT DEFAULT 'info',
    received_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- 基线指标数据
CREATE TABLE IF NOT EXISTS baseline_metrics (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_type TEXT DEFAULT 'snmp',
    baseline_value REAL DEFAULT 0,
    current_value REAL DEFAULT 0,
    deviation REAL DEFAULT 0,
    threshold REAL DEFAULT 0,
    unit TEXT DEFAULT '',
    measured_at DATETIME DEFAULT (datetime('now','localtime')),
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- 注意：custom_dashboard_metrics / custom_dashboard_lists 的真实 schema 在 app.db
-- （schema_dump.sql，按设备存储的自定义指标/列表），此处曾存在同名但不同结构的
-- “大屏定义”表，无任何代码使用且会遮蔽路由，已移除。

-- 服务器性能指标
CREATE TABLE IF NOT EXISTS server_metrics (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL,
    cpu_usage REAL DEFAULT 0,
    memory_usage REAL DEFAULT 0,
    memory_total_gb REAL DEFAULT 0,
    memory_used_gb REAL DEFAULT 0,
    disk_usage REAL DEFAULT 0,
    disk_total_gb REAL DEFAULT 0,
    disk_used_gb REAL DEFAULT 0,
    network_in_mbps REAL DEFAULT 0,
    network_out_mbps REAL DEFAULT 0,
    load_1min REAL DEFAULT 0,
    load_5min REAL DEFAULT 0,
    load_15min REAL DEFAULT 0,
    uptime_seconds INTEGER DEFAULT 0,
    disk_read_kbps REAL DEFAULT 0,
    disk_write_kbps REAL DEFAULT 0,
    net_rx_kbps REAL DEFAULT 0,
    net_tx_kbps REAL DEFAULT 0,
    cpu_user REAL DEFAULT 0,
    cpu_system REAL DEFAULT 0,
    cpu_iowait REAL DEFAULT 0,
    memory_cached_gb REAL DEFAULT 0,
    memory_buffers_gb REAL DEFAULT 0,
    swap_used_gb REAL DEFAULT 0,
    raw_metrics TEXT DEFAULT '{}',
    collected_at DATETIME DEFAULT (datetime('now','localtime')),
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- MIB 模块定义
CREATE TABLE IF NOT EXISTS mib_modules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT DEFAULT '',
    description TEXT DEFAULT '',
    version TEXT DEFAULT '',
    organization TEXT DEFAULT '',
    oid_prefix TEXT DEFAULT '',
    file_path TEXT DEFAULT '',
    is_builtin INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- MIB 对象树
CREATE TABLE IF NOT EXISTS mib_objects (
    id TEXT PRIMARY KEY,
    module_id TEXT NOT NULL,
    oid TEXT NOT NULL,
    name TEXT NOT NULL,
    syntax TEXT DEFAULT '',
    access TEXT DEFAULT '',
    status TEXT DEFAULT '',
    description TEXT DEFAULT '',
    parent_oid TEXT DEFAULT '',
    table_oid TEXT DEFAULT '',
    is_table INTEGER DEFAULT 0,
    is_scalar INTEGER DEFAULT 0,
    is_index INTEGER DEFAULT 0,
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- SNMP 轮询任务配置
CREATE TABLE IF NOT EXISTS snmp_polling_tasks (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    task_name TEXT DEFAULT '',
    task_type TEXT DEFAULT 'status',
    interval_seconds INTEGER DEFAULT 300,
    enabled INTEGER DEFAULT 1,
    config TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- SNMP 轮询快照（上次采样值，用于增量计算）
CREATE TABLE IF NOT EXISTS snmp_polling_snapshots (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    oid TEXT DEFAULT '',
    value TEXT DEFAULT '',
    collected_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_snmp_collect_tasks_device ON snmp_collect_tasks(device_id);
CREATE INDEX IF NOT EXISTS idx_snmp_collect_results_task ON snmp_collect_results(task_id);
CREATE INDEX IF NOT EXISTS idx_snmp_collect_results_device ON snmp_collect_results(device_id);
CREATE INDEX IF NOT EXISTS idx_snmp_collect_results_time ON snmp_collect_results(collected_at);
CREATE INDEX IF NOT EXISTS idx_snmp_interface_metrics_device ON snmp_interface_metrics(device_id);
CREATE INDEX IF NOT EXISTS idx_snmp_interface_metrics_descr ON snmp_interface_metrics(if_descr);
CREATE INDEX IF NOT EXISTS idx_snmp_custom_oid_results_device ON snmp_custom_oid_results(device_id);
CREATE INDEX IF NOT EXISTS idx_snmp_trap_events_device ON snmp_trap_events(device_id);
CREATE INDEX IF NOT EXISTS idx_snmp_trap_events_time ON snmp_trap_events(received_at);
CREATE INDEX IF NOT EXISTS idx_baseline_metrics_device ON baseline_metrics(device_id);
CREATE INDEX IF NOT EXISTS idx_server_metrics_server ON server_metrics(server_id);
CREATE INDEX IF NOT EXISTS idx_server_metrics_time ON server_metrics(collected_at);
CREATE INDEX IF NOT EXISTS idx_snmp_polling_snapshots_device ON snmp_polling_snapshots(device_id);
CREATE INDEX IF NOT EXISTS idx_snmp_polling_tasks_device ON snmp_polling_tasks(device_id);

-- ============================================================
-- SNMP 指标命名体系（snmp_exporter 风格）
-- ============================================================

-- 指标定义：从 MIB OBJECT-TYPE / generator.yml 派生
-- metric_name 采用 snmp_exporter 风格（如 ifInOctets / huaweiCpuUsage）
-- 一条 definition 描述一个可被采集的指标：OID 模板 + 类型 + 标签规则
CREATE TABLE IF NOT EXISTS snmp_metric_definitions (
    id TEXT PRIMARY KEY,
    metric_name TEXT NOT NULL,         -- 规范名（snmp_exporter 风格）
    module_name TEXT DEFAULT '',       -- MIB 模块名（来源）
    oid TEXT NOT NULL,                 -- 标量 OID 或表列 OID 模板（含 {index} 占位）
    description TEXT DEFAULT '',
    metric_type TEXT DEFAULT 'gauge',  -- gauge | counter
    is_table INTEGER DEFAULT 0,        -- 是否为表列（需 WALK + 索引派生标签）
    index_oid TEXT DEFAULT '',         -- 索引列 OID（表型指标），用于派生 ifIndex 等标签
    index_tag TEXT DEFAULT '',         -- 索引标签名（如 ifIndex）
    label_oids TEXT DEFAULT '',        -- 附加标签列 OID（JSON 数组，如 [ifDescr OID]）
    label_names TEXT DEFAULT '',       -- 附加标签名（JSON 数组，如 ["ifDescr"]）
    unit TEXT DEFAULT '',
    source TEXT DEFAULT 'mib',         -- mib | generator
    generator_module TEXT DEFAULT '',  -- generator.yml 的 module 名
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_snmp_metric_def_name ON snmp_metric_definitions(metric_name);
CREATE INDEX IF NOT EXISTS idx_snmp_metric_def_module ON snmp_metric_definitions(module_name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_snmp_metric_def_unique ON snmp_metric_definitions(metric_name);

-- 通用时序样本：一条采集 = metric_name + {device, 标签...} = value
-- 类似 Prometheus 的一个 sample，支持任意 SNMP 指标的统一存储与 Grafana 式查询
CREATE TABLE IF NOT EXISTS snmp_metric_samples (
    id TEXT PRIMARY KEY,
    definition_id TEXT DEFAULT '',
    metric_name TEXT NOT NULL,
    device_id TEXT NOT NULL,
    tags_json TEXT DEFAULT '{}',        -- {"ifIndex":"10","ifDescr":"GigabitEthernet0/0/1"}
    value REAL DEFAULT 0,
    collected_at DATETIME DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_snmp_metric_samples_metric ON snmp_metric_samples(metric_name, collected_at);
CREATE INDEX IF NOT EXISTS idx_snmp_metric_samples_device ON snmp_metric_samples(device_id, collected_at);
CREATE INDEX IF NOT EXISTS idx_snmp_metric_samples_def ON snmp_metric_samples(definition_id, collected_at);

-- 大屏面板配置（Grafana 式）：metric_name + 聚合 + 标签过滤 + 图表类型
CREATE TABLE IF NOT EXISTS dashboard_panels (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    aggregation TEXT DEFAULT 'avg',     -- avg | max | min | sum | rate | last
    function_filter TEXT DEFAULT '',    -- 额外的 SQL/标签过滤条件（JSON）
    tag_filters TEXT DEFAULT '{}',      -- {"device_id":"xxx","ifDescr":"ge0/0/1"}
    group_by TEXT DEFAULT '',           -- 分组标签（JSON 数组），如 ["device_id"] 或 ["ifDescr"]
    chart_type TEXT DEFAULT 'line',    -- line | bar | area
    time_range TEXT DEFAULT '24h',      -- 1h | 6h | 24h | 7d
    refresh_interval INTEGER DEFAULT 30,
    grid_pos INTEGER DEFAULT 0,         -- 大屏布局位置
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_dashboard_panels_enabled ON dashboard_panels(enabled);

-- metrics.db 自身的迁移版本表
CREATE TABLE IF NOT EXISTS schema_migrations (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL UNIQUE,
    name TEXT NOT NULL,
    applied_at DATETIME DEFAULT (datetime('now','localtime')),
    success INTEGER NOT NULL DEFAULT 0,
    error_message TEXT
);
CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
CREATE INDEX IF NOT EXISTS idx_schema_migrations_success ON schema_migrations(success);
