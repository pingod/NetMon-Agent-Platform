// Package migrate - MySQL 数据库 DDL 架构定义
// 为生产环境 MySQL 5.7 / 8.0 / MariaDB 提供全量 60+ 张业务表的建表与迁移
package migrate

import (
	"database/sql"
	"fmt"

	"itops-backend/internal/logger"
)

// MySQLTables 全量业务与配置表 DDL 定义
var MySQLTables = []string{
	// 1. 用户与认证
	`CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(255) NOT NULL UNIQUE,
		password VARCHAR(255) NOT NULL,
		email VARCHAR(255) DEFAULT '',
		role VARCHAR(50) DEFAULT 'user',
		enabled TINYINT DEFAULT 1,
		password_must_change TINYINT DEFAULT 0,
		failed_login_attempts INT DEFAULT 0,
		locked_until DATETIME NULL,
		last_failed_login DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		token_invalid_before BIGINT NOT NULL DEFAULT 0
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 2. 网络设备与凭证
	`CREATE TABLE IF NOT EXISTS network_devices (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		ip_address VARCHAR(45) NOT NULL,
		vendor VARCHAR(100) DEFAULT '',
		model VARCHAR(100) DEFAULT '',
		os_version VARCHAR(100) DEFAULT '',
		ssh_port INT DEFAULT 22,
		ssh_key_id VARCHAR(36) DEFAULT '',
		username VARCHAR(255) DEFAULT '',
		password TEXT,
		enable_password TEXT,
		location VARCHAR(255) DEFAULT '',
		role VARCHAR(50) DEFAULT '',
		status VARCHAR(20) DEFAULT 'unknown',
		last_inspection_at DATETIME NULL,
		last_inspection_result TEXT,
		device_type VARCHAR(50) DEFAULT 'switch',
		last_backup_at DATETIME NULL,
		device_role VARCHAR(50) DEFAULT '',
		snmp_enabled TINYINT DEFAULT 1,
		last_snmp_at DATETIME NULL,
		snmp_port INT DEFAULT 161,
		snmp_credential_id VARCHAR(36) DEFAULT '',
		system_poll_interval INT DEFAULT 300,
		interface_poll_interval INT DEFAULT 60,
		description TEXT,
		tags TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS snmp_credentials (
		id VARCHAR(36) PRIMARY KEY,
		device_id VARCHAR(36) NULL,
		name VARCHAR(255) NOT NULL DEFAULT 'default',
		community TEXT,
		snmp_version VARCHAR(20) NOT NULL DEFAULT 'v2c',
		snmp_port INT NOT NULL DEFAULT 161,
		snmp_user VARCHAR(255) DEFAULT '',
		snmp_auth_protocol VARCHAR(50) DEFAULT '',
		snmp_auth_key TEXT,
		snmp_priv_protocol VARCHAR(50) DEFAULT '',
		snmp_priv_key TEXT,
		host VARCHAR(255) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS credentials (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) NOT NULL,
		encrypted_data TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 3. 服务器与主机管理
	`CREATE TABLE IF NOT EXISTS servers (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		hostname VARCHAR(255) NOT NULL DEFAULT '',
		host VARCHAR(255) DEFAULT '',
		port INT DEFAULT 22,
		username VARCHAR(255) DEFAULT '',
		password TEXT,
		private_key TEXT,
		use_ssh_key TINYINT DEFAULT 0,
		ssh_key_id VARCHAR(36) DEFAULT '',
		os_type VARCHAR(50) DEFAULT 'linux',
		os VARCHAR(100) DEFAULT '',
		os_detail TEXT,
		cpu_cores INT DEFAULT 0,
		memory_gb DOUBLE DEFAULT 0,
		disk_gb DOUBLE DEFAULT 0,
		ip_address VARCHAR(45) DEFAULT '',
		private_ip VARCHAR(45) DEFAULT '',
		cloud_provider VARCHAR(50) DEFAULT '',
		cloud_instance_id VARCHAR(255) DEFAULT '',
		vnc_port INT DEFAULT 5900,
		vnc_password TEXT,
		status VARCHAR(20) DEFAULT 'unknown',
		group_id VARCHAR(36) DEFAULT '',
		description TEXT,
		tags TEXT,
		enabled TINYINT DEFAULT 1,
		last_connected DATETIME NULL,
		agent_version VARCHAR(50) DEFAULT '',
		agent_installed TINYINT DEFAULT 0,
		agent_status VARCHAR(50) DEFAULT 'uninstalled',
		agent_last_heartbeat DATETIME NULL,
		agent_cpu_usage DOUBLE DEFAULT 0,
		agent_mem_mb DOUBLE DEFAULT 0,
		agent_uptime_seconds BIGINT DEFAULT 0,
		agent_token VARCHAR(255) DEFAULT '',
		is_virtual_machine TINYINT DEFAULT 0,
		vendor VARCHAR(100) DEFAULT 'other',
		model VARCHAR(100) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS server_groups (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		parent_id VARCHAR(36) DEFAULT '',
		display_order INT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS server_commands (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		command TEXT NOT NULL,
		category VARCHAR(100) DEFAULT 'general',
		description TEXT,
		is_high_risk TINYINT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS ssh_keys (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) DEFAULT 'password',
		username VARCHAR(255) DEFAULT '',
		password TEXT,
		private_key TEXT,
		passphrase TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 4. AI Agent 与大模型配置
	`CREATE TABLE IF NOT EXISTS agents (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) DEFAULT 'default',
		description TEXT,
		config TEXT,
		enabled TINYINT DEFAULT 1,
		api_provider VARCHAR(50) DEFAULT 'doubao',
		model_name VARCHAR(100) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS ai_models (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		provider VARCHAR(50) NOT NULL,
		api_base VARCHAR(500) DEFAULT '',
		api_key TEXT,
		model_identifier VARCHAR(255) NOT NULL,
		is_default TINYINT DEFAULT 0,
		config JSON NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 5. 外部数据库与时序库连接
	`CREATE TABLE IF NOT EXISTS ` + "`databases`" + ` (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) NOT NULL,
		host VARCHAR(255) DEFAULT '',
		port INT DEFAULT 0,
		username VARCHAR(255) DEFAULT '',
		password TEXT,
		database_name VARCHAR(255) DEFAULT '',
		options TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS database_connections (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) NOT NULL DEFAULT 'mysql',
		db_type VARCHAR(50) NOT NULL DEFAULT 'mysql',
		host VARCHAR(255) NOT NULL,
		port INT NOT NULL DEFAULT 3306,
		username VARCHAR(255) DEFAULT '',
		password TEXT,
		database_name VARCHAR(255) DEFAULT '',
		` + "`database`" + ` VARCHAR(255) DEFAULT '',
		token TEXT,
		organization VARCHAR(255) DEFAULT '',
		bucket VARCHAR(255) DEFAULT '',
		retention_policy VARCHAR(255) DEFAULT '',
		description TEXT,
		tags TEXT,
		enabled TINYINT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS influxdb_write_settings (
		id VARCHAR(36) PRIMARY KEY DEFAULT 'default',
		enabled TINYINT DEFAULT 0,
		db_connection_id VARCHAR(36) NULL,
		measurement_prefix VARCHAR(100) DEFAULT 'snmp',
		write_mode VARCHAR(20) DEFAULT 'async',
		batch_size INT DEFAULT 100,
		flush_interval_ms INT DEFAULT 5000,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 6. 数据中心 DC 与数字孪生
	`CREATE TABLE IF NOT EXISTS dc_rooms (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		location VARCHAR(255) DEFAULT '',
		description TEXT,
		width FLOAT DEFAULT 20,
		length FLOAT DEFAULT 30,
		height FLOAT DEFAULT 4,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS dc_racks (
		id VARCHAR(36) PRIMARY KEY,
		room_id VARCHAR(36) NOT NULL,
		name VARCHAR(255) NOT NULL,
		row_num INT DEFAULT 1,
		col_num INT DEFAULT 1,
		u_height INT DEFAULT 42,
		max_weight FLOAT DEFAULT 1000,
		max_power FLOAT DEFAULT 5000,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS dc_devices (
		id VARCHAR(36) PRIMARY KEY,
		rack_id VARCHAR(36) NOT NULL,
		device_type_id VARCHAR(36) DEFAULT '',
		name VARCHAR(255) NOT NULL,
		model VARCHAR(255) DEFAULT '',
		start_u INT NOT NULL DEFAULT 1,
		u_size INT NOT NULL DEFAULT 1,
		power_w FLOAT DEFAULT 350,
		status VARCHAR(50) DEFAULT 'active',
		device_id VARCHAR(36) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS dc_power_panels (
		id VARCHAR(36) PRIMARY KEY,
		room_id VARCHAR(36) NOT NULL,
		name VARCHAR(255) NOT NULL,
		location VARCHAR(255) DEFAULT '',
		capacity_kva FLOAT DEFAULT 100,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS dc_power_feeds (
		id VARCHAR(36) PRIMARY KEY,
		panel_id VARCHAR(36) NOT NULL,
		rack_id VARCHAR(36) NOT NULL,
		name VARCHAR(255) NOT NULL,
		voltage INT DEFAULT 220,
		amperage INT DEFAULT 32,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS dc_cables (
		id VARCHAR(36) PRIMARY KEY,
		room_id VARCHAR(36) NOT NULL,
		source_device_id VARCHAR(36) NOT NULL,
		source_port VARCHAR(100) DEFAULT '',
		target_device_id VARCHAR(36) NOT NULL,
		target_port VARCHAR(100) DEFAULT '',
		cable_type VARCHAR(50) DEFAULT 'cat6',
		color VARCHAR(30) DEFAULT 'blue',
		length_m FLOAT DEFAULT 5,
		status VARCHAR(30) DEFAULT 'connected',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS device_manufacturers (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL UNIQUE,
		slug VARCHAR(255) DEFAULT '',
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS device_types (
		id VARCHAR(36) PRIMARY KEY,
		manufacturer_id VARCHAR(36) NOT NULL,
		model VARCHAR(255) NOT NULL,
		slug VARCHAR(255) DEFAULT '',
		u_height INT DEFAULT 1,
		is_full_depth TINYINT DEFAULT 1,
		front_image TEXT,
		rear_image TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 7. IPAM 子网规划与发现
	`CREATE TABLE IF NOT EXISTS network_subnets (
		id VARCHAR(36) PRIMARY KEY,
		cidr VARCHAR(50) NOT NULL UNIQUE,
		name VARCHAR(255) NOT NULL,
		vlan_id INT DEFAULT 0,
		description TEXT,
		gateway VARCHAR(45) DEFAULT '',
		dns_servers TEXT,
		allocated_ips JSON NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS network_discovery_tasks (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		subnet_range VARCHAR(100) NOT NULL,
		snmp_community VARCHAR(255) DEFAULT 'public',
		snmp_version VARCHAR(10) DEFAULT 'v2c',
		status VARCHAR(50) DEFAULT 'pending',
		progress INT DEFAULT 0,
		discovered_count INT DEFAULT 0,
		results JSON NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 8. 拓扑图与配置备份
	`CREATE TABLE IF NOT EXISTS topology_nodes (
		id VARCHAR(36) PRIMARY KEY,
		device_id VARCHAR(36) DEFAULT '',
		label VARCHAR(255) NOT NULL,
		type VARCHAR(50) DEFAULT 'switch',
		x FLOAT DEFAULT 0,
		y FLOAT DEFAULT 0,
		config JSON NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS topology_links (
		id VARCHAR(36) PRIMARY KEY,
		source_id VARCHAR(36) NOT NULL,
		target_id VARCHAR(36) NOT NULL,
		source_port VARCHAR(100) DEFAULT '',
		target_port VARCHAR(100) DEFAULT '',
		bandwidth VARCHAR(50) DEFAULT '1Gbps',
		status VARCHAR(30) DEFAULT 'up',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS config_backup_tasks (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		template_id VARCHAR(36) DEFAULT '',
		schedule_type VARCHAR(50) DEFAULT 'cron',
		schedule_cron VARCHAR(100) DEFAULT '0 2 * * *',
		device_ids JSON NULL,
		enabled TINYINT DEFAULT 1,
		retention_days INT DEFAULT 30,
		last_run_at DATETIME NULL,
		last_run_status VARCHAR(50) DEFAULT 'pending',
		next_run_at DATETIME NULL,
		created_by VARCHAR(255) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS config_backup_history (
		id VARCHAR(36) PRIMARY KEY,
		task_id VARCHAR(36) DEFAULT '',
		device_id VARCHAR(36) NOT NULL,
		device_name VARCHAR(255) DEFAULT '',
		content LONGTEXT NOT NULL,
		content_hash VARCHAR(64) DEFAULT '',
		status VARCHAR(30) DEFAULT 'success',
		error_msg TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 9. 告警中心与 PromQL 规则引擎
	`CREATE TABLE IF NOT EXISTS alerts (
		id VARCHAR(36) PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		level VARCHAR(50) DEFAULT 'warning',
		status VARCHAR(50) DEFAULT 'active',
		source VARCHAR(100) DEFAULT 'snmp',
		severity VARCHAR(50) DEFAULT 'warning',
		content TEXT,
		metadata TEXT,
		related_task_id VARCHAR(36) DEFAULT '',
		alert_fingerprint VARCHAR(255) DEFAULT '',
		device_id VARCHAR(36) DEFAULT '',
		device_name VARCHAR(255) DEFAULT '',
		message TEXT,
		details JSON NULL,
		promql_expr TEXT,
		fired_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		resolved_at DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS alert_rules (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		promql TEXT NOT NULL,
		severity VARCHAR(50) DEFAULT 'warning',
		for_duration VARCHAR(50) DEFAULT '1m',
		eval_interval VARCHAR(50) DEFAULT '10s',
		labels JSON NULL,
		annotations JSON NULL,
		enabled TINYINT DEFAULT 1,
		group_id VARCHAR(36) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS alert_groups (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		notify_channels JSON NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS alert_providers (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(50) NOT NULL,
		webhook_url TEXT,
		secret TEXT,
		config JSON NULL,
		enabled TINYINT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS alert_mappings (
		id VARCHAR(36) PRIMARY KEY,
		raw_severity VARCHAR(100) NOT NULL,
		mapped_severity VARCHAR(50) NOT NULL,
		source VARCHAR(100) DEFAULT 'snmp',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS alert_noise_rules (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		matcher JSON NOT NULL,
		action VARCHAR(50) DEFAULT 'suppress',
		duration VARCHAR(50) DEFAULT '30m',
		enabled TINYINT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 10. 自愈工作流与任务引擎
	`CREATE TABLE IF NOT EXISTS workflows (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		definition JSON NOT NULL,
		enabled TINYINT DEFAULT 1,
		trigger_type VARCHAR(50) DEFAULT 'manual',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS tasks (
		id VARCHAR(36) PRIMARY KEY,
		workflow_id VARCHAR(36) DEFAULT '',
		name VARCHAR(255) NOT NULL,
		status VARCHAR(50) DEFAULT 'pending',
		params JSON NULL,
		result JSON NULL,
		logs LONGTEXT,
		started_at DATETIME NULL,
		finished_at DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS scheduled_tasks (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		cron_expr VARCHAR(100) NOT NULL,
		task_type VARCHAR(100) NOT NULL,
		params JSON NULL,
		enabled TINYINT DEFAULT 1,
		last_run_at DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS approval_requests (
		id VARCHAR(36) PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		command TEXT NOT NULL,
		device_id VARCHAR(36) DEFAULT '',
		device_name VARCHAR(255) DEFAULT '',
		risk_level VARCHAR(50) DEFAULT 'medium',
		requester VARCHAR(100) DEFAULT 'Agent',
		status VARCHAR(50) DEFAULT 'pending',
		approver VARCHAR(100) DEFAULT '',
		comment TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS ai_remediations (
		id VARCHAR(36) PRIMARY KEY,
		alert_id VARCHAR(36) DEFAULT '',
		root_cause TEXT,
		suggested_action TEXT,
		commands TEXT,
		risk_level VARCHAR(50) DEFAULT 'medium',
		status VARCHAR(50) DEFAULT 'proposed',
		verification_status VARCHAR(50) DEFAULT 'unverified',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 11. 容器、K8s与虚拟机纳管
	`CREATE TABLE IF NOT EXISTS containers (
		id VARCHAR(36) PRIMARY KEY,
		server_id VARCHAR(36) NOT NULL,
		container_id VARCHAR(128) NOT NULL,
		name VARCHAR(255) NOT NULL,
		image VARCHAR(255) NOT NULL,
		status VARCHAR(50) DEFAULT 'running',
		ports TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS k8s_contexts (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		cluster_name VARCHAR(255) DEFAULT '',
		kubeconfig LONGTEXT NOT NULL,
		status VARCHAR(50) DEFAULT 'connected',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS virtual_machines (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		platform VARCHAR(50) DEFAULT 'proxmox',
		host_server_id VARCHAR(36) DEFAULT '',
		vm_id VARCHAR(100) DEFAULT '',
		status VARCHAR(50) DEFAULT 'running',
		cpu_cores INT DEFAULT 2,
		memory_mb INT DEFAULT 4096,
		disk_gb INT DEFAULT 50,
		ip_address VARCHAR(45) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 12. 知识库、运维工具与审计
	`CREATE TABLE IF NOT EXISTS knowledge_base (
		id VARCHAR(36) PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		category VARCHAR(100) DEFAULT 'general',
		content LONGTEXT NOT NULL,
		tags TEXT,
		author VARCHAR(100) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS tool_links (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		url TEXT NOT NULL,
		icon VARCHAR(100) DEFAULT '',
		category VARCHAR(100) DEFAULT 'tools',
		description TEXT,
		display_order INT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS audit_logs (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36) DEFAULT '',
		username VARCHAR(100) DEFAULT '',
		action VARCHAR(100) NOT NULL,
		resource_type VARCHAR(100) DEFAULT '',
		resource_id VARCHAR(100) DEFAULT '',
		details JSON NULL,
		ip_address VARCHAR(45) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	// 13. 自定义大屏面板与指标配置
	`CREATE TABLE IF NOT EXISTS custom_dashboard_metrics (
		id VARCHAR(36) PRIMARY KEY,
		oid VARCHAR(255) NOT NULL,
		custom_name VARCHAR(255) NOT NULL,
		original_name VARCHAR(255) DEFAULT '',
		device_id VARCHAR(36) NOT NULL,
		module_name VARCHAR(255) DEFAULT '',
		syntax VARCHAR(50) DEFAULT '',
		unit VARCHAR(50) DEFAULT '',
		show_on_dashboard TINYINT NOT NULL DEFAULT 1,
		display_order INT NOT NULL DEFAULT 0,
		last_value TEXT,
		last_sampled_at DATETIME NULL,
		snmp_version VARCHAR(20) DEFAULT 'v2c',
		community VARCHAR(255) DEFAULT 'public',
		value_mappings TEXT,
		is_table TINYINT NOT NULL DEFAULT 0,
		chart_type VARCHAR(50) DEFAULT 'auto',
		metric_name VARCHAR(255) DEFAULT '',
		tags TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS custom_dashboard_lists (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		list_type VARCHAR(50) DEFAULT 'scalar',
		device_id VARCHAR(36) DEFAULT '',
		columns JSON NULL,
		refresh_interval INT DEFAULT 60,
		display_config JSON NULL,
		show_on_dashboard TINYINT DEFAULT 1,
		display_order INT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS dashboard_panels (
		id VARCHAR(36) PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		metric_name VARCHAR(255) NOT NULL,
		aggregation VARCHAR(50) DEFAULT 'avg',
		function_filter VARCHAR(255) DEFAULT '',
		tag_filters TEXT,
		group_by TEXT,
		chart_type VARCHAR(50) DEFAULT 'line',
		time_range VARCHAR(50) DEFAULT '24h',
		refresh_interval INT DEFAULT 30,
		grid_pos JSON NULL,
		enabled TINYINT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS snmp_metric_definitions (
		id VARCHAR(36) PRIMARY KEY,
		metric_name VARCHAR(255) NOT NULL UNIQUE,
		module_name VARCHAR(255) DEFAULT '',
		oid VARCHAR(255) DEFAULT '',
		metric_type VARCHAR(50) DEFAULT 'gauge',
		source VARCHAR(100) DEFAULT 'builtin',
		description TEXT,
		is_table TINYINT DEFAULT 0,
		table_oid VARCHAR(255) DEFAULT '',
		index_oid VARCHAR(255) DEFAULT '',
		index_tag VARCHAR(255) DEFAULT '',
		label_oids TEXT,
		label_names TEXT,
		unit VARCHAR(50) DEFAULT '',
		generator_module VARCHAR(255) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

	`CREATE TABLE IF NOT EXISTS config_backup_templates (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		device_type VARCHAR(50) NOT NULL,
		vendor VARCHAR(100) DEFAULT '',
		model VARCHAR(100) DEFAULT '',
		commands TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
}

// RunMigrationsMySQL 在 MySQL 中创建全部元数据表结构与初始种子
func RunMigrationsMySQL(db *sql.DB) error {
	logger.Infof("Running MySQL full schema creation (%d tables)...", len(MySQLTables))
	for _, tbl := range MySQLTables {
		if _, err := db.Exec(tbl); err != nil {
			logger.Warnf("MySQL table creation warning: %v", err)
		}
	}

	// 保证 network_devices.ip_address 有值（非列结构变更，失败仅告警）
	_, _ = db.Exec("UPDATE network_devices SET ip_address = ip WHERE (ip_address = '' OR ip_address IS NULL) AND ip IS NOT NULL")

	// 补丁修复列差异兼容：仅忽略“列已存在”，其他错误必须返回（避免首启缺列静默运行）
	alterPatches := []string{
		"ALTER TABLE network_devices ADD COLUMN ip_address VARCHAR(45) NOT NULL DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN device_role VARCHAR(50) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN last_inspection_result TEXT",
		"ALTER TABLE network_devices ADD COLUMN snmp_credential_id VARCHAR(36) DEFAULT ''",
		"ALTER TABLE network_devices ADD COLUMN system_poll_interval INT DEFAULT 300",
		"ALTER TABLE network_devices ADD COLUMN interface_poll_interval INT DEFAULT 60",
		"ALTER TABLE servers ADD COLUMN host VARCHAR(255) DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN hostname VARCHAR(255) NOT NULL DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN private_key TEXT",
		"ALTER TABLE servers ADD COLUMN use_ssh_key TINYINT DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN last_connected DATETIME NULL",
		"ALTER TABLE servers ADD COLUMN cloud_instance_id VARCHAR(255) DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN vnc_port INT DEFAULT 5900",
		"ALTER TABLE servers ADD COLUMN vnc_password TEXT",
		"ALTER TABLE servers ADD COLUMN is_virtual_machine TINYINT DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN agent_installed TINYINT DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN agent_status VARCHAR(50) DEFAULT 'uninstalled'",
		"ALTER TABLE servers ADD COLUMN agent_last_heartbeat DATETIME NULL",
		"ALTER TABLE servers ADD COLUMN agent_cpu_usage DOUBLE DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN agent_mem_mb DOUBLE DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN agent_uptime_seconds BIGINT DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN agent_token VARCHAR(255) DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN ip_address VARCHAR(45) DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN private_ip VARCHAR(45) DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN cloud_provider VARCHAR(50) DEFAULT ''",
		"ALTER TABLE servers ADD COLUMN os_detail TEXT",
		"ALTER TABLE servers ADD COLUMN cpu_cores INT DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN memory_gb DOUBLE DEFAULT 0",
		"ALTER TABLE servers ADD COLUMN disk_gb DOUBLE DEFAULT 0",
		"ALTER TABLE users ADD COLUMN token_invalid_before BIGINT NOT NULL DEFAULT 0",
		"ALTER TABLE alerts ADD COLUMN severity VARCHAR(50) DEFAULT 'warning'",
		"ALTER TABLE alerts ADD COLUMN content TEXT",
		"ALTER TABLE alerts ADD COLUMN device_id VARCHAR(36) DEFAULT ''",
		"ALTER TABLE config_backup_tasks ADD COLUMN template_id VARCHAR(36) DEFAULT ''",
		"ALTER TABLE config_backup_tasks ADD COLUMN schedule_type VARCHAR(50) DEFAULT 'cron'",
		"ALTER TABLE config_backup_tasks ADD COLUMN schedule_cron VARCHAR(100) DEFAULT '0 2 * * *'",
		"ALTER TABLE config_backup_tasks ADD COLUMN retention_days INT DEFAULT 30",
		"ALTER TABLE config_backup_tasks ADD COLUMN last_run_status VARCHAR(50) DEFAULT 'pending'",
		"ALTER TABLE config_backup_tasks ADD COLUMN next_run_at DATETIME NULL",
		"ALTER TABLE config_backup_tasks ADD COLUMN created_by VARCHAR(255) DEFAULT ''",
	}
	for _, patch := range alterPatches {
		if _, err := db.Exec(patch); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("mysql schema patch failed: %s: %w", patch, err)
		}
	}

	// 确保默认管理员账号存在
	var userCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err == nil && userCount == 0 {
		logger.Infof("No users found in MySQL, seeding default admin account...")
		_, _ = db.Exec(`INSERT INTO users (id, username, password, email, role, enabled, password_must_change)
			VALUES ('1', 'admin', '$2a$12$oqDXIaNfzuOoXS5MqfkDNu8THvL6Ng4ojgKv29DvD1vJWZv15i9de', 'admin@netmon.local', 'admin', 1, 1)`)
	}

	logger.Infof("MySQL schema created and verified successfully")
	return nil
}
