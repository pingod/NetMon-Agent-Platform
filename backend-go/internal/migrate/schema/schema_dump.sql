-- [table] aars_config
CREATE TABLE aars_config (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        enabled INTEGER DEFAULT 1,
        min_severity TEXT DEFAULT 'medium',
        auto_execute_enabled INTEGER DEFAULT 1,
        approval_timeout_minutes INTEGER DEFAULT 30,
        max_concurrent INTEGER DEFAULT 5,
        ssh_timeout_sec INTEGER DEFAULT 30,
        verify_interval_sec INTEGER DEFAULT 30,
        notification_channels TEXT DEFAULT '["wecom","dingtalk","email"]',
        auto_execute_whitelist TEXT DEFAULT '["systemctl restart","logrotate","rm -rf /tmp/*"]',
        business_hours TEXT DEFAULT '{"start":"09:00","end":"18:00"}',
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] aars_response_logs
CREATE TABLE aars_response_logs (
        id TEXT PRIMARY KEY,
        alert_id TEXT NOT NULL,
        device_id TEXT,
        device_type TEXT,
        access_method TEXT,
        status TEXT NOT NULL DEFAULT 'identifying',
        probes_used TEXT,
        diagnosis_result TEXT,
        root_cause TEXT,
        remediation_plan TEXT,
        verification_result TEXT,
        execution_status TEXT,
        approval_status TEXT DEFAULT 'not_needed',
        notification_sent INTEGER DEFAULT 0,
        error_message TEXT,
        started_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime')),
        completed_at TEXT,
        FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE CASCADE
      );

-- [table] agent_executions
CREATE TABLE agent_executions (
        id TEXT PRIMARY KEY,
        agent_id TEXT NOT NULL,
        agent_name TEXT,
        input_text TEXT,
        output_text TEXT,
        status TEXT,
        error_message TEXT,
        execution_time_ms INTEGER,
        token_count INTEGER,
        metadata TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
      );

-- [table] agents
CREATE TABLE agents (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        avatar TEXT,
        role TEXT,
        system_prompt TEXT,
        model TEXT DEFAULT 'doubao-4o',
        temperature REAL DEFAULT 0.7,
        enabled INTEGER DEFAULT 1,
        is_preset INTEGER DEFAULT 0,
        category TEXT,
        tags TEXT,
        description TEXT,
        usage_count INTEGER DEFAULT 0,
        last_used_at DATETIME,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      , api_provider TEXT DEFAULT 'doubao', primary_model_id TEXT, fallback_model_id TEXT);

-- [table] ai_models
CREATE TABLE ai_models (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        provider_type TEXT NOT NULL,
        api_key TEXT,
        api_base TEXT,
        model_id TEXT NOT NULL,
        enabled INTEGER DEFAULT 1,
        sort_order INTEGER DEFAULT 0,
        is_default INTEGER DEFAULT 0,
        tags TEXT,
        last_test_status TEXT,
        last_test_time DATETIME,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] ai_remediations
CREATE TABLE ai_remediations (
      id TEXT PRIMARY KEY,
      title TEXT NOT NULL,
      description TEXT DEFAULT '',
      alert_id TEXT DEFAULT '',
      status TEXT DEFAULT 'pending',
      strategy TEXT DEFAULT '',
      result TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    , device_id TEXT, device_name TEXT, device_ip TEXT, task_id TEXT, workflow_id TEXT, diagnosis TEXT, remediation_commands TEXT, risk_level TEXT, execution_result TEXT, error_message TEXT);

-- [table] alert_auto_analysis
CREATE TABLE alert_auto_analysis (
        id TEXT PRIMARY KEY,
        alert_id TEXT NOT NULL,
        device_id TEXT NOT NULL,
        device_name TEXT NOT NULL,
        device_ip TEXT NOT NULL,
        device_type TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending',
        diagnosis TEXT,
        summary TEXT,
        raw_output TEXT,
        commands_executed TEXT,
        error_message TEXT,
        duration_ms INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE CASCADE
      );

-- [table] alert_configs
CREATE TABLE alert_configs (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        level TEXT NOT NULL,
        enabled INTEGER DEFAULT 1,
        channels TEXT NOT NULL,
        webhook_url TEXT,
        email_recipients TEXT,
        rate_limit_minutes INTEGER DEFAULT 5,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] alert_correlation_groups
CREATE TABLE alert_correlation_groups (
        id TEXT PRIMARY KEY,
        title TEXT NOT NULL,
        description TEXT,
        status TEXT NOT NULL DEFAULT 'open'
          CHECK (status IN ('open', 'resolved', 'closed')),
        root_alert_id TEXT,
        root_cause TEXT,
        alert_count INTEGER DEFAULT 0,
        device_ids TEXT DEFAULT '',
        severity TEXT NOT NULL DEFAULT 'medium'
          CHECK (severity IN ('critical', 'high', 'medium', 'low')),
        auto_detected INTEGER DEFAULT 0,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime')),
        resolved_at TEXT
      );

-- [table] alert_correlation_members
CREATE TABLE alert_correlation_members (
        id TEXT PRIMARY KEY,
        group_id TEXT NOT NULL REFERENCES alert_correlation_groups(id) ON DELETE CASCADE,
        alert_id TEXT NOT NULL,
        is_root INTEGER DEFAULT 0,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        UNIQUE(group_id, alert_id)
      );

-- [table] alert_device_associations
CREATE TABLE alert_device_associations (
      alert_id TEXT NOT NULL,
      device_type TEXT NOT NULL CHECK (device_type IN ('server','network_device')),
      device_id TEXT NOT NULL,
      match_method TEXT NOT NULL DEFAULT 'auto' CHECK (match_method IN ('exact_hostname','fuzzy_hostname','ip_address','title_keyword','manual')),
      confidence INTEGER DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 100),
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
      PRIMARY KEY (alert_id)
    );

-- [table] alert_device_match_log
CREATE TABLE alert_device_match_log (
      id TEXT PRIMARY KEY,
      alert_title TEXT,
      alert_hostname TEXT,
      match_method TEXT DEFAULT 'auto',
      matched INTEGER DEFAULT 0,
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] alert_noise_reduction
CREATE TABLE alert_noise_reduction (
        id TEXT PRIMARY KEY,
        alert_fingerprint TEXT NOT NULL UNIQUE,
        alert_source TEXT NOT NULL,
        alert_title TEXT NOT NULL,
        occurrence_count INTEGER DEFAULT 1,
        first_occurrence DATETIME NOT NULL,
        last_occurrence DATETIME NOT NULL,
        is_suppressed INTEGER DEFAULT 0,
        suppression_reason TEXT,
        suppression_until DATETIME
      );

-- [table] alert_notifications
CREATE TABLE alert_notifications (
        id TEXT PRIMARY KEY,
        config_id TEXT NOT NULL,
        level TEXT NOT NULL,
        title TEXT NOT NULL,
        message TEXT,
        metadata TEXT,
        channels TEXT NOT NULL,
        status TEXT DEFAULT 'pending',
        triggered_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] alert_processing_records
CREATE TABLE alert_processing_records (
        id TEXT PRIMARY KEY,
        alert_id TEXT NOT NULL,
        strategy TEXT NOT NULL,
        decision_reason TEXT,
        execution_id TEXT,
        task_id TEXT,
        aars_log_id TEXT,
        remediation_id TEXT,
        status TEXT NOT NULL DEFAULT 'pending',
        error_message TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE CASCADE
      );

-- [table] alert_provider_configs
CREATE TABLE alert_provider_configs (
        id TEXT PRIMARY KEY,
        provider_id TEXT NOT NULL,
        name TEXT NOT NULL,
        config TEXT,
        enabled INTEGER DEFAULT 1,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] alert_webhook_logs
CREATE TABLE alert_webhook_logs (
        id TEXT PRIMARY KEY,
        source TEXT NOT NULL,
        status TEXT NOT NULL,
        alert_count INTEGER DEFAULT 0,
        resolved_count INTEGER DEFAULT 0,
        error_message TEXT,
        request_body TEXT,
        ip_address TEXT,
        user_agent TEXT,
        processing_time_ms INTEGER,
        created_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] alert_workflow_mappings
CREATE TABLE alert_workflow_mappings (
        id TEXT PRIMARY KEY,
        alert_source TEXT,
        alert_severity TEXT,
        alert_title_pattern TEXT,
        workflow_id TEXT NOT NULL,
        enabled INTEGER DEFAULT 1,
        created_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] alerts
CREATE TABLE alerts (
        id TEXT PRIMARY KEY,
        source TEXT NOT NULL,
        severity TEXT NOT NULL,
        title TEXT NOT NULL,
        content TEXT,
        metadata TEXT,
        related_task_id TEXT,
        status TEXT DEFAULT 'new',
        alert_fingerprint TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] approval_requests
CREATE TABLE approval_requests (
        id TEXT PRIMARY KEY,
        task_id TEXT NOT NULL,
        node_id TEXT NOT NULL,
        node_label TEXT NOT NULL,
        description TEXT,
        status TEXT NOT NULL DEFAULT 'pending',
        requested_by TEXT,
        approved_by TEXT,
        approved_at DATETIME,
        reject_reason TEXT,
        timeout_at DATETIME,
        timeout_action TEXT DEFAULT 'reject',
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
      );

-- [table] audit_logs
CREATE TABLE audit_logs (
        id TEXT PRIMARY KEY,
        user_id TEXT,
        action TEXT NOT NULL,
        resource_type TEXT,
        resource_id TEXT,
        details TEXT,
        ip_address TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] auto_scale_history
CREATE TABLE auto_scale_history (
        id TEXT PRIMARY KEY, rule_id TEXT, rule_name TEXT,
        target_type TEXT, target_id TEXT,
        action TEXT, previous_count INTEGER, current_count INTEGER,
        metric_value REAL, result TEXT, reason TEXT,
        timestamp TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] auto_scale_rules
CREATE TABLE auto_scale_rules (
        id TEXT PRIMARY KEY, name TEXT NOT NULL,
        target_type TEXT NOT NULL, target_id TEXT NOT NULL, target_name TEXT,
        metric_type TEXT NOT NULL, threshold REAL NOT NULL, target_value REAL NOT NULL,
        min_instances INTEGER DEFAULT 1, max_instances INTEGER DEFAULT 10,
        scale_up_cooldown INTEGER DEFAULT 300, scale_down_cooldown INTEGER DEFAULT 600,
        enabled INTEGER DEFAULT 1, last_scale_time TEXT,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] automata_trust
CREATE TABLE automata_trust (
        operation_key TEXT PRIMARY KEY,
        approval_count INTEGER DEFAULT 0,
        rejection_count INTEGER DEFAULT 0,
        success_count INTEGER DEFAULT 0,
        failure_count INTEGER DEFAULT 0,
        success_rate REAL DEFAULT 0.5,
        last_updated TEXT
      );

-- [table] baseline_metrics
CREATE TABLE baseline_metrics (
        device_id TEXT NOT NULL,
        metric_name TEXT NOT NULL,
        sample_value REAL NOT NULL,
        sampled_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
        PRIMARY KEY (device_id, metric_name, sampled_at)
      );

-- [table] change_records
CREATE TABLE change_records (
        id TEXT PRIMARY KEY,
        server_id TEXT NOT NULL,
        change_type TEXT NOT NULL,
        description TEXT,
        changed_by TEXT,
        status TEXT DEFAULT 'completed',
        related_alert_id TEXT,
        is_root_cause INTEGER DEFAULT 0,
        metadata TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE,
        FOREIGN KEY (related_alert_id) REFERENCES alerts(id)
      );

-- [table] compliance_checks
CREATE TABLE compliance_checks (
        id TEXT PRIMARY KEY,
        server_id TEXT NOT NULL,
        check_name TEXT NOT NULL,
        check_results TEXT NOT NULL,
        status TEXT DEFAULT 'pending',
        started_at DATETIME,
        completed_at DATETIME,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
      );

-- [table] compose_projects
CREATE TABLE compose_projects (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL UNIQUE,
        description TEXT,
        compose_content TEXT NOT NULL,
        status TEXT DEFAULT 'stopped',
        service_count INTEGER DEFAULT 0,
        running_count INTEGER DEFAULT 0,
        working_dir TEXT,
        tags TEXT,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] config_backup_records
CREATE TABLE config_backup_records (
        id TEXT PRIMARY KEY,
        task_id TEXT,
        task_name TEXT,
        device_id TEXT NOT NULL,
        device_name TEXT NOT NULL,
        device_ip TEXT NOT NULL,
        device_vendor TEXT NOT NULL,
        template_id TEXT,
        template_name TEXT,
        status TEXT NOT NULL DEFAULT 'running',
        commands TEXT,
        output TEXT,
        output_size INTEGER DEFAULT 0,
        duration_ms INTEGER,
        error_message TEXT,
        started_at DATETIME DEFAULT (datetime('now','localtime')),
        completed_at DATETIME,
        FOREIGN KEY (task_id) REFERENCES config_backup_tasks(id),
        FOREIGN KEY (device_id) REFERENCES network_devices(id)
      );

-- [table] config_backup_tasks
CREATE TABLE config_backup_tasks (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        template_id TEXT NOT NULL,
        schedule_type TEXT NOT NULL DEFAULT 'manual',
        schedule_cron TEXT,
        device_ids TEXT NOT NULL DEFAULT '[]',
        enabled INTEGER DEFAULT 1,
        last_run_at DATETIME,
        last_run_status TEXT,
        next_run_at DATETIME,
        retention_days INTEGER DEFAULT 90,
        created_by TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (template_id) REFERENCES config_backup_templates(id)
      );

-- [table] config_backup_templates
CREATE TABLE config_backup_templates (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        vendor TEXT NOT NULL DEFAULT 'custom',
        commands TEXT NOT NULL,
        is_builtin INTEGER DEFAULT 0,
        created_by TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] config_template_history
CREATE TABLE config_template_history (
      id TEXT PRIMARY KEY,
      template_id TEXT NOT NULL,
      server_id TEXT NOT NULL,
      applied_by TEXT,
      variables_snapshot TEXT,
      backup_path TEXT,
      status TEXT NOT NULL,
      result TEXT,
      error_message TEXT,
      applied_at DATETIME DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (template_id) REFERENCES config_templates(id)
    );

-- [table] config_templates
CREATE TABLE config_templates (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      description TEXT DEFAULT '',
      type TEXT NOT NULL DEFAULT 'generic',
      category TEXT DEFAULT '',
      service_name TEXT DEFAULT '',
      content TEXT NOT NULL DEFAULT '',
      template_content TEXT DEFAULT '',
      variables TEXT DEFAULT '[]',
      os_type TEXT DEFAULT 'linux',
      target_type TEXT DEFAULT 'server',
      target_path TEXT DEFAULT '',
      tags TEXT DEFAULT '[]',
      version INTEGER DEFAULT 1,
      backup_before_apply INTEGER DEFAULT 1,
      restart_command TEXT DEFAULT '',
      validation_command TEXT DEFAULT '',
      is_system INTEGER DEFAULT 0,
      usage_count INTEGER DEFAULT 0,
      success_count INTEGER DEFAULT 0,
      created_by TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] container_images
CREATE TABLE container_images (
      id TEXT PRIMARY KEY,
      image_id TEXT NOT NULL,
      name TEXT NOT NULL,
      tag TEXT DEFAULT 'latest',
      size_bytes INTEGER DEFAULT 0,
      host TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] containers
CREATE TABLE containers (
      id TEXT PRIMARY KEY,
      container_id TEXT NOT NULL,
      name TEXT NOT NULL,
      image TEXT DEFAULT '',
      status TEXT DEFAULT 'unknown',
      host TEXT DEFAULT '',
      port_mappings TEXT DEFAULT '[]',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] copilot_conversations
CREATE TABLE copilot_conversations (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        messages TEXT NOT NULL,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] credentials
CREATE TABLE credentials (
        provider TEXT PRIMARY KEY,
        encrypted_value TEXT NOT NULL,
        key_version INTEGER DEFAULT 1,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] custom_dashboard_lists
CREATE TABLE custom_dashboard_lists (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      description TEXT DEFAULT '',
      device_ids TEXT NOT NULL DEFAULT '[]',
      columns TEXT NOT NULL DEFAULT '[]',
      refresh_interval INTEGER DEFAULT 60,
      show_on_dashboard INTEGER DEFAULT 1,
      display_order INTEGER DEFAULT 0,
      created_at TEXT,
      updated_at TEXT
    );

-- [table] custom_dashboard_metrics
CREATE TABLE custom_dashboard_metrics (
        id TEXT PRIMARY KEY,
        oid TEXT NOT NULL,
        custom_name TEXT NOT NULL,
        original_name TEXT,
        device_id TEXT NOT NULL,
        module_name TEXT,
        syntax TEXT,
        unit TEXT DEFAULT '',
        show_on_dashboard INTEGER NOT NULL DEFAULT 1,
        display_order INTEGER NOT NULL DEFAULT 0,
        last_value TEXT,
        last_sampled_at TEXT,
        snmp_version TEXT DEFAULT 'v2c',
        community TEXT DEFAULT 'public',
        chart_type TEXT DEFAULT 'auto',
        metric_name TEXT DEFAULT '',
        tags TEXT DEFAULT '',
        created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
        updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime')), value_mappings TEXT DEFAULT '', is_table INTEGER NOT NULL DEFAULT 0,
        FOREIGN KEY (device_id) REFERENCES network_devices(id)
      );

-- [table] database_connections
CREATE TABLE database_connections (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      type TEXT NOT NULL DEFAULT 'mysql',
      host TEXT NOT NULL,
      port INTEGER NOT NULL DEFAULT 3306,
      username TEXT NOT NULL DEFAULT '',
      password TEXT DEFAULT '',
      database_name TEXT DEFAULT '',
      ssl_enabled INTEGER DEFAULT 0,
      status TEXT DEFAULT 'disconnected',
      is_template INTEGER DEFAULT 0,
      tags TEXT DEFAULT '[]',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] databases
CREATE TABLE databases (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                db_type TEXT NOT NULL DEFAULT 'mysql',
                host TEXT NOT NULL,
                port INTEGER DEFAULT 3306,
                username TEXT NOT NULL,
                password TEXT NOT NULL,
                database TEXT NOT NULL,
                description TEXT,
                tags TEXT,
                enabled INTEGER DEFAULT 1,
                created_at DATETIME DEFAULT (datetime('now','localtime')),
                updated_at DATETIME DEFAULT (datetime('now','localtime'))
            , token TEXT, organization TEXT, bucket TEXT, retention_policy TEXT);

-- [table] dc_cables
CREATE TABLE dc_cables (
      id TEXT PRIMARY KEY,
      name TEXT DEFAULT '',
      cable_type TEXT DEFAULT 'cat6' CHECK(cable_type IN (
        'cat5','cat5e','cat6','cat6a','cat7','cat8',
        'fiber_om1','fiber_om2','fiber_om3','fiber_om4','fiber_os2',
        'coax','power','hdmi','sas','sata','other'
      )),
      cable_color TEXT DEFAULT '',
      length_m REAL DEFAULT NULL,
      status TEXT DEFAULT 'connected' CHECK(status IN ('connected','planned','decommissioned','fault')),
      -- A 端
      a_device_id TEXT NOT NULL,
      a_device_type TEXT NOT NULL CHECK(a_device_type IN ('server','network_device','vm_host','pdu','ups','power_feed','other')),
      a_port_name TEXT DEFAULT '',
      -- B 端
      b_device_id TEXT NOT NULL,
      b_device_type TEXT NOT NULL CHECK(b_device_type IN ('server','network_device','vm_host','pdu','ups','power_feed','other')),
      b_port_name TEXT DEFAULT '',
      description TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] dc_device_lifecycle
CREATE TABLE dc_device_lifecycle (
      id TEXT PRIMARY KEY,
      device_id TEXT NOT NULL,
      device_type TEXT NOT NULL,
      action TEXT NOT NULL,           -- 'mounted' | 'unmounted' | 'moved' | 'maintenance'
      from_rack_id TEXT,
      from_slot_start INTEGER,
      from_slot_end INTEGER,
      to_rack_id TEXT,
      to_slot_start INTEGER,
      to_slot_end INTEGER,
      performed_by TEXT DEFAULT 'system',
      notes TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] dc_pdus
CREATE TABLE dc_pdus (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      rack_id TEXT,
      type TEXT NOT NULL DEFAULT 'pdu', -- 'pdu' | 'ups' | 'ac' | 'other'
      status TEXT DEFAULT 'normal',
      model TEXT DEFAULT '',
      power_capacity_w REAL DEFAULT 4000,
      current_load_w REAL DEFAULT 0,
      input_voltage REAL DEFAULT 220,
      output_sockets INTEGER DEFAULT 8,
      ip_address TEXT DEFAULT '',
      snmp_community TEXT DEFAULT '',
      notes TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (rack_id) REFERENCES dc_racks(id) ON DELETE SET NULL
    );

-- [table] dc_power_feeds
CREATE TABLE dc_power_feeds (
      id TEXT PRIMARY KEY,
      power_panel_id TEXT NOT NULL,
      rack_id TEXT DEFAULT NULL,
      name TEXT NOT NULL,
      status TEXT DEFAULT 'active' CHECK(status IN ('active','planned','failed','decommissioned')),
      feed_type TEXT DEFAULT 'primary' CHECK(feed_type IN ('primary','redundant')),
      supply TEXT DEFAULT 'ac' CHECK(supply IN ('ac','dc')),
      voltage REAL DEFAULT 220,
      amperage REAL DEFAULT 16,
      max_utilization_pct REAL DEFAULT 80,
      current_load_w REAL DEFAULT 0,
      description TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (power_panel_id) REFERENCES dc_power_panels(id) ON DELETE RESTRICT,
      FOREIGN KEY (rack_id) REFERENCES dc_racks(id) ON DELETE SET NULL
    );

-- [table] dc_power_panels
CREATE TABLE dc_power_panels (
      id TEXT PRIMARY KEY,
      room_id TEXT NOT NULL,
      name TEXT NOT NULL,
      location_label TEXT DEFAULT '',
      panel_type TEXT DEFAULT 'rpp' CHECK(panel_type IN ('rpp','pdu','ups','generator','mains')),
      voltage REAL DEFAULT 220,
      amperage REAL DEFAULT 63,
      phase_count INTEGER DEFAULT 3 CHECK(phase_count IN (1, 3)),
      max_power_kw REAL GENERATED ALWAYS AS (voltage * amperage * phase_count / 1000.0) STORED,
      description TEXT DEFAULT '',
      sort_order INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (room_id) REFERENCES dc_rooms(id) ON DELETE CASCADE
    );

-- [table] dc_rack_slots
CREATE TABLE dc_rack_slots (
      id TEXT PRIMARY KEY,
      rack_id TEXT NOT NULL,           -- 所属机柜
      device_id TEXT NOT NULL,         -- 关联 devices.id / servers.id / network_devices.id
      device_type TEXT NOT NULL,       -- 'server' | 'network_device' | 'vm_host' | 'pdu' | 'ups' | 'other'
      start_u INTEGER NOT NULL,        -- 起始U位 (1-based)
      end_u INTEGER NOT NULL,          -- 结束U位
      position_face TEXT DEFAULT 'front', -- front/back
      notes TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime')), device_type_id TEXT REFERENCES device_types(id) ON DELETE SET NULL,
      FOREIGN KEY (rack_id) REFERENCES dc_racks(id) ON DELETE CASCADE
    );

-- [table] dc_racks
CREATE TABLE dc_racks (
      id TEXT PRIMARY KEY,
      room_id TEXT NOT NULL,           -- 所属机房
      name TEXT NOT NULL,              -- "A-01"
      label TEXT DEFAULT '',
      row_number INTEGER DEFAULT 1,   -- 第几排
      position_x REAL DEFAULT 0,      -- Three.js 场景X坐标
      position_z REAL DEFAULT 0,      -- Three.js 场景Z坐标
      total_u INTEGER DEFAULT 42,     -- 总U位数
      pdu_count INTEGER DEFAULT 2,    -- PDU数量
      max_power_w REAL DEFAULT 4000,  -- 最大功耗(W)
      status TEXT DEFAULT 'normal',    -- normal/warning/critical
      sort_order INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] dc_rooms
CREATE TABLE dc_rooms (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      label TEXT DEFAULT '',           -- 显示标签: "A区"
      description TEXT DEFAULT '',
      width_m REAL DEFAULT 20,         -- 物理宽度(米)
      depth_m REAL DEFAULT 15,         -- 物理进深(米)
      max_temperature REAL DEFAULT 28, -- 温度阈值
      min_temperature REAL DEFAULT 18,
      max_humidity REAL DEFAULT 70,
      min_humidity REAL DEFAULT 30,
      layout_config TEXT DEFAULT '{}',  -- Three.js 场景布局参数 JSON
      sort_order INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    , current_temperature REAL DEFAULT NULL, current_humidity REAL DEFAULT NULL, pue REAL DEFAULT 1.45, total_power_kw REAL DEFAULT 0);

-- [table] device_manufacturers
CREATE TABLE device_manufacturers (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL UNIQUE,
      slug TEXT NOT NULL UNIQUE,
      description TEXT DEFAULT '',
      logo_url TEXT DEFAULT '',
      sort_order INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] device_type_slot_definitions
CREATE TABLE device_type_slot_definitions (
      id TEXT PRIMARY KEY,
      device_type_id TEXT NOT NULL,
      slot_type TEXT NOT NULL CHECK(slot_type IN (
        'power_port','interface','console_port','console_server_port',
        'power_outlet','module_bay','device_bay','front_port','rear_port','pdu_outlet'
      )),
      slot_name TEXT NOT NULL,
      slot_label TEXT DEFAULT '',
      position_label TEXT DEFAULT '',
      u_position INTEGER DEFAULT 0,
      is_preferred INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (device_type_id) REFERENCES device_types(id) ON DELETE CASCADE
    );

-- [table] device_types
CREATE TABLE device_types (
      id TEXT PRIMARY KEY,
      manufacturer_id TEXT NOT NULL,
      model TEXT NOT NULL,
      slug TEXT NOT NULL,
      part_number TEXT DEFAULT '',
      u_height REAL DEFAULT 1 CHECK(u_height > 0),
      is_full_depth INTEGER DEFAULT 1,
      subdevice_role TEXT DEFAULT NULL CHECK(subdevice_role IS NULL OR subdevice_role IN ('parent','child')),
      airflow TEXT DEFAULT 'front-to-rear' CHECK(airflow IN ('front-to-rear','rear-to-front','left-to-right','passive')),
      weight_kg REAL DEFAULT NULL,
      max_power_w REAL DEFAULT NULL,
      front_image_url TEXT DEFAULT '',
      rear_image_url TEXT DEFAULT '',
      description TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (manufacturer_id) REFERENCES device_manufacturers(id) ON DELETE RESTRICT
    );

-- [table] docker_endpoints
CREATE TABLE docker_endpoints (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        host TEXT NOT NULL,
        port INTEGER DEFAULT 2375,
        protocol TEXT DEFAULT 'socket',
        tls_ca TEXT,
        tls_cert TEXT,
        tls_key TEXT,
        status TEXT DEFAULT 'inactive',
        error_message TEXT,
        containers_running INTEGER DEFAULT 0,
        containers_total INTEGER DEFAULT 0,
        images INTEGER DEFAULT 0,
        cpu_count INTEGER DEFAULT 0,
        memory_limit INTEGER DEFAULT 0,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] encryption_keys
CREATE TABLE encryption_keys (
        id TEXT PRIMARY KEY,
        key_type TEXT NOT NULL,
        key_value TEXT NOT NULL,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        active INTEGER DEFAULT 1
      );

-- [table] escalation_history
CREATE TABLE escalation_history (
        id TEXT PRIMARY KEY,
        alert_id TEXT NOT NULL,
        stage TEXT NOT NULL,
        entered_at TEXT NOT NULL,
        reason TEXT,
        notified INTEGER DEFAULT 0,
        resolved_at TEXT,
        FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE CASCADE
      );

-- [table] image_registries
CREATE TABLE image_registries (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        type TEXT NOT NULL,
        url TEXT NOT NULL,
        username TEXT,
        encrypted_password TEXT,
        encrypted_password_iv TEXT,
        status TEXT DEFAULT 'inactive',
        error_message TEXT,
        project_count INTEGER DEFAULT 0,
        repo_count INTEGER DEFAULT 0,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] influxdb_write_settings
CREATE TABLE influxdb_write_settings (
        id TEXT PRIMARY KEY,
        enabled INTEGER DEFAULT 0,
        db_connection_id TEXT,
        measurement_prefix TEXT DEFAULT 'snmp',
        write_mode TEXT DEFAULT 'async',
        batch_size INTEGER DEFAULT 100,
        flush_interval_ms INTEGER DEFAULT 5000,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] k8s_contexts
CREATE TABLE k8s_contexts (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        cluster_url TEXT,
        namespace TEXT DEFAULT 'default',
        auth_type TEXT DEFAULT 'kubeconfig',
        config TEXT,
        status TEXT DEFAULT 'inactive',
        node_count INTEGER DEFAULT 0,
        pod_count INTEGER DEFAULT 0,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] knowledge_base
CREATE TABLE knowledge_base (
        id TEXT PRIMARY KEY,
        title TEXT NOT NULL,
        category TEXT,
        content TEXT NOT NULL,
        tags TEXT,
        solutions TEXT,
        related_alerts TEXT,
        usage_count INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      , success_rating REAL DEFAULT 0.5, source TEXT DEFAULT 'manual', alert_id TEXT, workflow_id TEXT, task_id TEXT, server_id TEXT, duration_ms INTEGER);

-- [table] mib_modules
CREATE TABLE mib_modules (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        file_name TEXT NOT NULL,
        file_content TEXT,
        description TEXT,
        organization TEXT,
        contact_info TEXT,
        last_updated TEXT,
        oid_tree_json TEXT,
        object_count INTEGER DEFAULT 0,
        uploaded_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] mib_objects
CREATE TABLE mib_objects (
        id TEXT PRIMARY KEY,
        module_id TEXT NOT NULL,
        module_name TEXT NOT NULL,
        oid TEXT NOT NULL,
        name TEXT NOT NULL,
        syntax TEXT,
        access TEXT,
        status TEXT,
        description TEXT,
        parent_oid TEXT,
        table_name TEXT,
        index_columns TEXT,
        is_table INTEGER DEFAULT 0,
        is_scalar INTEGER DEFAULT 0,
        FOREIGN KEY (module_id) REFERENCES mib_modules(id)
      );

-- [table] network_config_backups
CREATE TABLE network_config_backups (
      id TEXT PRIMARY KEY,
      device_id TEXT NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
      config_md5 TEXT NOT NULL DEFAULT '',
      config_text TEXT,
      config_size INTEGER DEFAULT 0,
      status TEXT NOT NULL DEFAULT 'success' CHECK (status IN ('success','failed','partial')),
      error_message TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] network_devices
CREATE TABLE "network_devices" (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        ip_address TEXT NOT NULL UNIQUE,
        vendor TEXT NOT NULL,
        model TEXT,
        os_version TEXT,
        ssh_port INTEGER DEFAULT 22,
        ssh_key_id TEXT,
        username TEXT,
        password TEXT,
        enable_password TEXT,
        location TEXT,
        role TEXT,
        status TEXT DEFAULT 'online',
        last_inspection_at DATETIME,
        last_inspection_result TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')), device_type TEXT
        DEFAULT 'unknown' CHECK (device_type IN ('switch','router','firewall','loadbalancer','wlc','ap','gateway','unknown')), last_backup_at TEXT, device_role TEXT, snmp_enabled INTEGER NOT NULL DEFAULT 1, last_snmp_at TEXT, snmp_port INTEGER DEFAULT 161, snmp_credential_id TEXT REFERENCES snmp_credentials(id) ON DELETE SET NULL,
        FOREIGN KEY (ssh_key_id) REFERENCES ssh_keys(id) ON DELETE SET NULL
      );

-- [table] network_discovery_jobs
CREATE TABLE network_discovery_jobs (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        start_ip TEXT NOT NULL,
        end_ip TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'pending'
          CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
        progress INTEGER DEFAULT 0,
        total_hosts INTEGER DEFAULT 0,
        scanned_hosts INTEGER DEFAULT 0,
        found_devices INTEGER DEFAULT 0,
        credential_ids TEXT DEFAULT '[]',
        error_message TEXT,
        started_at TEXT,
        completed_at TEXT,
        created_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] network_discovery_results
CREATE TABLE network_discovery_results (
        id TEXT PRIMARY KEY,
        job_id TEXT NOT NULL,
        ip_address TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'offline'
          CHECK (status IN ('offline', 'online', 'snmp_ok', 'snmp_fail')),
        sys_name TEXT,
        sys_descr TEXT,
        sys_location TEXT,
        sys_object_id TEXT,
        snmp_version TEXT,
        community TEXT,
        interface_count INTEGER,
        vendor TEXT,
        model TEXT,
        response_time_ms INTEGER,
        created_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] network_external_devices
CREATE TABLE network_external_devices (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      discovered_from_device_id TEXT REFERENCES network_devices(id) ON DELETE SET NULL,
      platform TEXT,
      management_ip TEXT,
      last_seen_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] network_inspection_history
CREATE TABLE network_inspection_history (
        id TEXT PRIMARY KEY,
        device_id TEXT NOT NULL,
        inspection_type TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'running',
        commands_executed INTEGER DEFAULT 0,
        commands_failed INTEGER DEFAULT 0,
        results TEXT,
        summary TEXT,
        duration_ms INTEGER,
        created_at DATETIME DEFAULT (datetime('now','localtime')), device_type TEXT DEFAULT 'unknown',
        FOREIGN KEY (device_id) REFERENCES network_devices(id) ON DELETE CASCADE
      );

-- [table] network_ips
CREATE TABLE network_ips (
      id TEXT PRIMARY KEY,
      subnet_id TEXT NOT NULL,
      ip_address TEXT NOT NULL,
      status TEXT DEFAULT 'available' CHECK(status IN ('available','used','reserved')),
      device_id TEXT,
      device_name TEXT,
      mac_address TEXT,
      description TEXT,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime')),
      FOREIGN KEY (subnet_id) REFERENCES network_subnets(id) ON DELETE CASCADE
    );

-- [table] network_lldp_neighbors
CREATE TABLE network_lldp_neighbors (
      id TEXT PRIMARY KEY,
      device_id TEXT NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
      local_interface TEXT,
      remote_device_name TEXT,
      remote_interface TEXT,
      remote_platform TEXT,
      remote_mgmt_ip TEXT,
      protocol TEXT NOT NULL DEFAULT 'lldp' CHECK (protocol IN ('lldp','cdp')),
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] network_subnets
CREATE TABLE network_subnets (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      cidr TEXT NOT NULL,
      gateway TEXT,
      vlan_id INTEGER,
      network_type TEXT DEFAULT 'lan' CHECK(network_type IN ('lan','wan','dmz','mgmt','storage','other')),
      location TEXT,
      description TEXT,
      status TEXT DEFAULT 'active' CHECK(status IN ('active','reserved','deprecated')),
      total_ips INTEGER DEFAULT 0,
      used_ips INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] network_topology_links
CREATE TABLE network_topology_links (
      id TEXT PRIMARY KEY,
      deviceA_id TEXT NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
      deviceA_name TEXT,
      deviceA_interface TEXT,
      deviceB_id TEXT NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
      deviceB_name TEXT,
      deviceB_interface TEXT,
      status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','stale')),
      last_seen_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] notification_configs
CREATE TABLE notification_configs (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        webhook_enabled INTEGER DEFAULT 1,
        webhook_url TEXT,
        email_enabled INTEGER DEFAULT 0,
        email_config TEXT,
        wechat_enabled INTEGER DEFAULT 0,
        wechat_config TEXT,
        dingtalk_enabled INTEGER DEFAULT 0,
        dingtalk_config TEXT,
        alert_notification TEXT,
        task_notification TEXT,
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] notifications
CREATE TABLE notifications (
        id TEXT PRIMARY KEY,
        type TEXT NOT NULL,
        title TEXT NOT NULL,
        content TEXT,
        status TEXT DEFAULT 'unread',
        recipient TEXT,
        metadata TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime'))
      , related_alert_id TEXT, related_task_id TEXT, sent_at DATETIME, error_message TEXT);

-- [table] probe_execution_stats
CREATE TABLE probe_execution_stats (
        probe_id TEXT PRIMARY KEY,
        total_uses INTEGER DEFAULT 0,
        successful_diagnoses INTEGER DEFAULT 0,
        total_duration_ms INTEGER DEFAULT 0,
        last_used_at TEXT,
        device_id TEXT,
        alert_type TEXT
      );

-- [table] remediation_audits
CREATE TABLE remediation_audits (
        id TEXT PRIMARY KEY,
        rca_id TEXT NOT NULL,
        policy_id TEXT,
        server_id TEXT NOT NULL,
        risk_level TEXT NOT NULL,
        status TEXT DEFAULT 'pending',
        approved_by TEXT,
        approved_at TEXT,
        execution_log TEXT,
        result TEXT,
        is_rollback INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        completed_at TEXT,
        FOREIGN KEY (rca_id) REFERENCES root_cause_analyses(id),
        FOREIGN KEY (policy_id) REFERENCES remediation_policies(id),
        FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE,
        FOREIGN KEY (approved_by) REFERENCES users(username)
      );

-- [table] remediation_cooldowns
CREATE TABLE remediation_cooldowns (
        policy_id TEXT NOT NULL,
        alert_id TEXT NOT NULL,
        cooldown_until DATETIME NOT NULL,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        PRIMARY KEY (policy_id, alert_id),
        FOREIGN KEY (policy_id) REFERENCES remediation_policies(id) ON DELETE CASCADE,
        FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE CASCADE
      );

-- [table] remediation_executions
CREATE TABLE remediation_executions (
        id TEXT PRIMARY KEY,
        policy_id TEXT NOT NULL,
        alert_id TEXT NOT NULL,
        alert_snapshot TEXT,
        status TEXT NOT NULL DEFAULT 'pending',
        status_reason TEXT,
        approval_required INTEGER DEFAULT 0,
        approved_by TEXT,
        approved_at DATETIME,
        approval_comment TEXT,
        workflow_execution_id TEXT,
        started_at DATETIME,
        completed_at DATETIME,
        execution_result TEXT,
        verification_status TEXT,
        verification_result TEXT,
        verification_completed_at DATETIME,
        rollback_triggered INTEGER DEFAULT 0,
        rollback_execution_id TEXT,
        rollback_completed_at DATETIME,
        rollback_result TEXT,
        execution_duration_ms INTEGER,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (policy_id) REFERENCES remediation_policies(id),
        FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE CASCADE
      );

-- [table] remediation_history
CREATE TABLE remediation_history (
        id TEXT PRIMARY KEY,
        policy_id TEXT NOT NULL,
        alert_source TEXT,
        alert_severity TEXT,
        execution_status TEXT,
        root_cause TEXT,
        resolution TEXT,
        duration_ms INTEGER,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (policy_id) REFERENCES remediation_policies(id)
      );

-- [table] remediation_policies
CREATE TABLE remediation_policies (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        alert_source TEXT NOT NULL,
        alert_severity TEXT,
        alert_keywords TEXT,
        alert_tags TEXT,
        execution_mode TEXT NOT NULL DEFAULT 'approval',
        workflow_id TEXT,
        workflow_params TEXT,
        max_executions_per_hour INTEGER DEFAULT 5,
        cooldown_seconds INTEGER DEFAULT 300,
        require_confirmation TEXT,
        enable_verification INTEGER DEFAULT 1,
        verification_workflow_id TEXT,
        verification_params TEXT,
        verification_timeout_seconds INTEGER DEFAULT 120,
        enable_rollback INTEGER DEFAULT 1,
        rollback_workflow_id TEXT,
        rollback_on_failure INTEGER DEFAULT 1,
        enabled INTEGER DEFAULT 1,
        created_by TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] report_schedules
CREATE TABLE report_schedules (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        template_id TEXT NOT NULL,
        cron_expression TEXT NOT NULL,
        enabled INTEGER DEFAULT 1,
        recipients TEXT,
        format TEXT DEFAULT 'markdown',
        last_generated DATETIME,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (template_id) REFERENCES reports(id) ON DELETE CASCADE
      );

-- [table] reports
CREATE TABLE reports (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        type TEXT NOT NULL DEFAULT 'generated',
        content TEXT,
        format TEXT DEFAULT 'markdown',
        template_id TEXT,
        task_id TEXT,
        variables TEXT,
        metadata TEXT,
        is_preset INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] root_cause_analyses
CREATE TABLE root_cause_analyses (
        id TEXT PRIMARY KEY,
        alert_id TEXT,
        title TEXT NOT NULL,
        description TEXT,
        status TEXT DEFAULT 'pending',
        root_cause TEXT,
        symptoms TEXT,
        timeline TEXT,
        evidence TEXT,
        recommendations TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        completed_at DATETIME,
        FOREIGN KEY (alert_id) REFERENCES alerts(id)
      );

-- [table] scheduled_tasks
CREATE TABLE scheduled_tasks (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        workflow_id TEXT NOT NULL,
        schedule TEXT NOT NULL,
        enabled INTEGER DEFAULT 1,
        last_run DATETIME,
        next_run DATETIME,
        last_status TEXT DEFAULT 'unknown',
        context TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
      );

-- [table] scripts
CREATE TABLE scripts (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        category TEXT,
        language TEXT DEFAULT 'bash',
        content TEXT NOT NULL,
        tags TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] server_command_history
CREATE TABLE server_command_history (
        id TEXT PRIMARY KEY,
        server_id TEXT NOT NULL,
        command TEXT NOT NULL,
        stdout TEXT,
        stderr TEXT,
        success INTEGER DEFAULT 0,
        execution_time_ms INTEGER,
        executed_by TEXT,
        executed_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
      );

-- [table] server_group_mapping
CREATE TABLE server_group_mapping (
        server_id TEXT NOT NULL,
        group_id TEXT NOT NULL,
        PRIMARY KEY (server_id, group_id),
        FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE,
        FOREIGN KEY (group_id) REFERENCES server_groups(id) ON DELETE CASCADE
      );

-- [table] server_groups
CREATE TABLE server_groups (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        parent_id TEXT,
        sort_order INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (parent_id) REFERENCES server_groups(id) ON DELETE SET NULL
      );

-- [table] server_metrics
CREATE TABLE server_metrics (
        id TEXT PRIMARY KEY,
        server_id TEXT NOT NULL,
        cpu_usage REAL,
        memory_usage REAL,
        memory_total_gb REAL,
        memory_used_gb REAL,
        disk_usage REAL,
        disk_total_gb REAL,
        disk_used_gb REAL,
        network_in_mbps REAL,
        network_out_mbps REAL,
        load_1min REAL,
        load_5min REAL,
        load_15min REAL,
        uptime_seconds INTEGER,
        collected_at DATETIME,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
      );

-- [table] servers
CREATE TABLE servers (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        hostname TEXT NOT NULL,
        port INTEGER DEFAULT 22,
        username TEXT NOT NULL,
        password TEXT,
        private_key TEXT,
        use_ssh_key INTEGER DEFAULT 0,
        description TEXT,
        tags TEXT,
        enabled INTEGER DEFAULT 1,
        last_connected DATETIME,
        os TEXT,
        os_type TEXT DEFAULT 'linux',
        cpu_cores INTEGER,
        memory_gb REAL,
        disk_gb REAL,
        ip_address TEXT,
        private_ip TEXT,
        cloud_provider TEXT,
        cloud_instance_id TEXT,
        vnc_port INTEGER DEFAULT 5900,
        vnc_password TEXT,
        ssh_key_id TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        gpu_count INTEGER DEFAULT 0,
        gpu_model TEXT,
        os_detail TEXT,
        vendor TEXT DEFAULT 'other',
        model TEXT DEFAULT ''
);

-- [table] service_topologies
CREATE TABLE service_topologies (
        id TEXT PRIMARY KEY,
        source_server_id TEXT NOT NULL,
        target_server_id TEXT NOT NULL,
        dependency_type TEXT NOT NULL,
        protocol TEXT,
        port INTEGER,
        status TEXT DEFAULT 'active',
        last_verified_at TEXT,
        metadata TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (source_server_id) REFERENCES servers(id) ON DELETE CASCADE,
        FOREIGN KEY (target_server_id) REFERENCES servers(id) ON DELETE CASCADE
      );

-- [table] settings
CREATE TABLE settings (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        key TEXT UNIQUE NOT NULL,
        value TEXT,
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] snmp_collect_results
CREATE TABLE snmp_collect_results (
        id TEXT PRIMARY KEY,
        task_id TEXT NOT NULL,
        task_name TEXT,
        device_id TEXT NOT NULL,
        device_name TEXT,
        device_ip TEXT,
        oid TEXT NOT NULL,
        metric_name TEXT,
        metric_value TEXT,
        metric_type TEXT,
        error_message TEXT,
        duration_ms INTEGER,
        sampled_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] snmp_collect_tasks
CREATE TABLE snmp_collect_tasks (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        device_type TEXT DEFAULT 'all',
        device_ids TEXT NOT NULL DEFAULT '[]',
        oids TEXT NOT NULL DEFAULT '[]',
        schedule_type TEXT NOT NULL DEFAULT 'interval',
        interval_seconds INTEGER DEFAULT 300,
        cron_expression TEXT,
        timeout_ms INTEGER DEFAULT 10000,
        retries INTEGER DEFAULT 2,
        snmp_version TEXT DEFAULT 'v2c',
        community TEXT DEFAULT 'public',
        enabled INTEGER DEFAULT 1,
        retention_days INTEGER DEFAULT 90,
        last_run_at TEXT,
        last_run_status TEXT,
        next_run_at TEXT,
        created_by TEXT,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] snmp_credentials
CREATE TABLE snmp_credentials (
      id TEXT PRIMARY KEY,
      device_id TEXT REFERENCES network_devices(id) ON DELETE CASCADE,
      name TEXT NOT NULL DEFAULT 'default',
      community TEXT,
      snmp_version TEXT NOT NULL DEFAULT 'v2c' CHECK (snmp_version IN ('v1','v2c','v3')),
      snmp_port INTEGER NOT NULL DEFAULT 161,
      snmp_user TEXT,
      snmp_auth_protocol TEXT CHECK (snmp_auth_protocol IN ('MD5','SHA')),
      snmp_auth_key TEXT,
      snmp_priv_protocol TEXT CHECK (snmp_priv_protocol IN ('DES','AES')),
      snmp_priv_key TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
      updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    , host TEXT);

-- [table] snmp_custom_oid_profiles
CREATE TABLE snmp_custom_oid_profiles (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        vendor TEXT,
        description TEXT,
        oids_json TEXT NOT NULL DEFAULT '[]',
        poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
        enabled INTEGER NOT NULL DEFAULT 1,
        created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
        updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
      );

-- [table] snmp_custom_oid_results
CREATE TABLE snmp_custom_oid_results (
        id TEXT PRIMARY KEY,
        device_id TEXT NOT NULL,
        profile_id TEXT NOT NULL,
        metric_name TEXT NOT NULL,
        metric_value TEXT,
        oid TEXT NOT NULL,
        sampled_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
        FOREIGN KEY (device_id) REFERENCES network_devices(id),
        FOREIGN KEY (profile_id) REFERENCES snmp_custom_oid_profiles(id)
      );

-- [table] snmp_interface_metrics
CREATE TABLE snmp_interface_metrics (
        id TEXT PRIMARY KEY,
        device_id TEXT NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
        interface_name TEXT,
        if_index INTEGER NOT NULL,
        if_speed REAL,
        if_admin_status INTEGER,
        if_oper_status INTEGER,
        if_in_octets REAL,
        if_out_octets REAL,
        if_in_errors REAL,
        if_out_errors REAL,
        if_in_utilization REAL,
        if_out_utilization REAL,
        sampled_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
      , if_description TEXT);

-- [table] snmp_polling_snapshots
CREATE TABLE snmp_polling_snapshots (
          id INTEGER PRIMARY KEY AUTOINCREMENT,
          device_id TEXT NOT NULL,
          interface_index INTEGER NOT NULL,
          in_octets TEXT DEFAULT '0',
          out_octets TEXT DEFAULT '0',
          in_errors INTEGER DEFAULT 0,
          out_errors INTEGER DEFAULT 0,
          last_poll_at TEXT,
          UNIQUE(device_id, interface_index)
        );

-- [table] snmp_polling_tasks
CREATE TABLE snmp_polling_tasks (
      id TEXT PRIMARY KEY,
      device_id TEXT NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
      poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
      poll_items TEXT NOT NULL DEFAULT '["cpu","memory","interfaces"]',
      enabled INTEGER NOT NULL DEFAULT 1,
      last_poll_at TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] snmp_trap_events
CREATE TABLE snmp_trap_events (
      id TEXT PRIMARY KEY,
      source_ip TEXT NOT NULL,
      trap_type TEXT,
      enterprise_oid TEXT,
      agent_address TEXT,
      generic_type INTEGER DEFAULT 0,
      specific_type INTEGER DEFAULT 0,
      varbinds_json TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
    );

-- [table] ssh_keys
CREATE TABLE "ssh_keys" (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL UNIQUE,
        auth_type TEXT DEFAULT 'key',
        key_type TEXT,
        fingerprint TEXT,
        username TEXT,
        password TEXT,
        private_key TEXT,
        description TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] storage_volumes
CREATE TABLE storage_volumes (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      driver TEXT DEFAULT 'local',
      mount_point TEXT DEFAULT '',
      size_gb INTEGER DEFAULT 0,
      used_gb INTEGER DEFAULT 0,
      status TEXT DEFAULT 'available',
      host TEXT DEFAULT '',
      type TEXT DEFAULT 'docker',
      tags TEXT DEFAULT '[]',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] tasks
CREATE TABLE tasks (
        id TEXT PRIMARY KEY,
        workflow_id TEXT,
        name TEXT,
        status TEXT DEFAULT 'pending',
        start_time DATETIME,
        end_time DATETIME,
        current_node_id TEXT,
        node_results TEXT,
        logs TEXT,
        context TEXT,
        metrics TEXT,
        execution_order TEXT,
        report_id TEXT,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      , execution_variables TEXT, parallel_branches TEXT, loop_iterations TEXT, parent_task_id TEXT, execution_depth INTEGER DEFAULT 0);

-- [table] token_blacklist
CREATE TABLE token_blacklist (
        id TEXT PRIMARY KEY,
        token TEXT NOT NULL UNIQUE,
        user_id TEXT,
        reason TEXT,
        expires_at DATETIME NOT NULL,
        created_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [table] tool_links
CREATE TABLE tool_links (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        url TEXT NOT NULL,
        icon TEXT DEFAULT 'ExternalLink',
        category TEXT DEFAULT '未分类',
        description TEXT,
        sort_order INTEGER DEFAULT 0,
        is_external INTEGER DEFAULT 1,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      , image_icon TEXT);

-- [table] users
CREATE TABLE "users" (
        id TEXT PRIMARY KEY,
        username TEXT UNIQUE NOT NULL,
        password TEXT NOT NULL,
        email TEXT,
        role TEXT NOT NULL,
        enabled INTEGER DEFAULT 1,
        password_must_change INTEGER DEFAULT 0,
        failed_login_attempts INTEGER DEFAULT 0,
        locked_until DATETIME,
        last_failed_login DATETIME,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
      );

-- [table] virtual_machines
CREATE TABLE virtual_machines (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      host TEXT DEFAULT '',
      status TEXT DEFAULT 'unknown',
      os TEXT DEFAULT '',
      cpu_cores INTEGER DEFAULT 0,
      memory_mb INTEGER DEFAULT 0,
      disk_gb INTEGER DEFAULT 0,
      ip_address TEXT DEFAULT '',
      hypervisor TEXT DEFAULT '',
      agent_id TEXT DEFAULT '',
      server_id TEXT DEFAULT '',
      tags TEXT DEFAULT '[]',
      notes TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [table] vm_audit_logs
CREATE TABLE vm_audit_logs (
        id TEXT PRIMARY KEY,
        platform_id TEXT NOT NULL,
        vm_id TEXT,
        vm_name TEXT,
        operation TEXT NOT NULL,
        user_id TEXT,
        username TEXT,
        parameters TEXT,
        result TEXT,
        status TEXT NOT NULL,
        error_message TEXT,
        started_at TEXT NOT NULL,
        completed_at TEXT,
        FOREIGN KEY (platform_id) REFERENCES vm_platforms(id)
      );

-- [table] vm_migrations
CREATE TABLE vm_migrations (
        id TEXT PRIMARY KEY, vm_id TEXT NOT NULL, vm_name TEXT,
        source_host TEXT, target_host TEXT NOT NULL, platform_id TEXT NOT NULL,
        status TEXT DEFAULT 'pending', progress INTEGER DEFAULT 0,
        reason TEXT, error_message TEXT,
        started_at TEXT, completed_at TEXT,
        created_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] vm_platforms
CREATE TABLE vm_platforms (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        hypervisor_type TEXT NOT NULL,
        host TEXT NOT NULL,
        port INTEGER,
        username TEXT,
        encrypted_password TEXT,
        encrypted_password_iv TEXT,
        config TEXT,
        status TEXT NOT NULL DEFAULT 'inactive',
        last_connected TEXT,
        error_message TEXT,
        tags TEXT,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
      );

-- [table] vm_snapshot_policies
CREATE TABLE vm_snapshot_policies (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        platform_id TEXT NOT NULL,
        vm_id TEXT NOT NULL,
        cron_expression TEXT NOT NULL,
        retention INTEGER DEFAULT 7,
        snapshot_memory INTEGER DEFAULT 1,
        enabled INTEGER DEFAULT 1,
        last_run_at TEXT,
        next_run_at TEXT,
        created_at TEXT DEFAULT (datetime('now','localtime')),
        updated_at TEXT DEFAULT (datetime('now','localtime'))
      );

-- [table] workflow_execution_logs
CREATE TABLE workflow_execution_logs (
        id TEXT PRIMARY KEY,
        task_id TEXT NOT NULL,
        node_id TEXT NOT NULL,
        node_type TEXT NOT NULL,
        
        -- 循环迭代相关
        iteration_index INTEGER,           -- 循环迭代序号（从 0 开始）
        iteration_total INTEGER,           -- 循环总次数（如果已知）
        
        -- 并行分支相关
        branch_id TEXT,                     -- 并行分支 ID
        branch_total INTEGER,               -- 并行分支总数
        
        -- 变量快照（JSON 格式）
        input_variables TEXT,               -- 输入变量快照
        output_variables TEXT,              -- 输出变量快照
        
        -- 执行状态
        status TEXT NOT NULL,               -- pending / running / success / failed / skipped
        error_message TEXT,                 -- 错误信息
        
        -- 时间戳
        started_at TEXT,
        completed_at TEXT,
        duration_ms INTEGER,                -- 执行耗时（毫秒）
        
        -- 元数据
        metadata TEXT,                      -- 额外元数据（JSON 格式）
        
        -- 外键约束
        FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
      );

-- [table] workflow_variable_transfers
CREATE TABLE workflow_variable_transfers (
        id TEXT PRIMARY KEY,
        task_id TEXT NOT NULL,
        source_node_id TEXT,                -- 来源节点 ID（null 表示初始变量）
        target_node_id TEXT,                -- 目标节点 ID
        variable_name TEXT NOT NULL,         -- 变量名
        variable_value TEXT,                 -- 变量值（JSON 格式）
        transfer_type TEXT NOT NULL,         -- output_mapping / input_mapping / condition / loop
        created_at TEXT DEFAULT (datetime('now','localtime')),
        
        FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
      );

-- [table] workflows
CREATE TABLE workflows (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        nodes TEXT,
        edges TEXT,
        agent_configs TEXT,
        is_template INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT (datetime('now','localtime')),
        updated_at DATETIME DEFAULT (datetime('now','localtime'))
      );

-- [index] idx_aars_logs_alert
CREATE INDEX idx_aars_logs_alert ON aars_response_logs(alert_id);

-- [index] idx_aars_logs_status
CREATE INDEX idx_aars_logs_status ON aars_response_logs(status);

-- [index] idx_agent_executions_agent_created
CREATE INDEX idx_agent_executions_agent_created ON agent_executions(agent_id, created_at DESC);

-- [index] idx_agent_executions_agent_id
CREATE INDEX idx_agent_executions_agent_id ON agent_executions(agent_id);

-- [index] idx_agent_executions_created_at
CREATE INDEX idx_agent_executions_created_at ON agent_executions(created_at);

-- [index] idx_agent_executions_status
CREATE INDEX idx_agent_executions_status ON agent_executions(status);

-- [index] idx_agents_category
CREATE INDEX idx_agents_category ON agents(category);

-- [index] idx_agents_enabled
CREATE INDEX idx_agents_enabled ON agents(enabled);

-- [index] idx_agents_fallback_model
CREATE INDEX idx_agents_fallback_model ON agents(fallback_model_id);

-- [index] idx_agents_is_preset
CREATE INDEX idx_agents_is_preset ON agents(is_preset);

-- [index] idx_agents_primary_model
CREATE INDEX idx_agents_primary_model ON agents(primary_model_id);

-- [index] idx_agents_usage
CREATE INDEX idx_agents_usage ON agents(usage_count);

-- [index] idx_ai_models_default
CREATE INDEX idx_ai_models_default ON ai_models(is_default);

-- [index] idx_ai_models_enabled
CREATE INDEX idx_ai_models_enabled ON ai_models(enabled);

-- [index] idx_ai_models_provider
CREATE INDEX idx_ai_models_provider ON ai_models(provider_type);

-- [index] idx_ai_models_sort_order
CREATE INDEX idx_ai_models_sort_order ON ai_models(sort_order);

-- [index] idx_alert_configs_enabled
CREATE INDEX idx_alert_configs_enabled ON alert_configs(enabled);

-- [index] idx_alert_configs_level
CREATE INDEX idx_alert_configs_level ON alert_configs(level);

-- [index] idx_alert_mapping_enabled
CREATE INDEX idx_alert_mapping_enabled ON alert_workflow_mappings(enabled);

-- [index] idx_alert_notifications_config_id
CREATE INDEX idx_alert_notifications_config_id ON alert_notifications(config_id);

-- [index] idx_alert_notifications_level
CREATE INDEX idx_alert_notifications_level ON alert_notifications(level);

-- [index] idx_alert_notifications_triggered_at
CREATE INDEX idx_alert_notifications_triggered_at ON alert_notifications(triggered_at DESC);

-- [index] idx_alert_provider_configs_enabled
CREATE INDEX idx_alert_provider_configs_enabled ON alert_provider_configs(enabled);

-- [index] idx_alert_provider_configs_provider_id
CREATE INDEX idx_alert_provider_configs_provider_id ON alert_provider_configs(provider_id);

-- [index] idx_alerts_created_at
CREATE INDEX idx_alerts_created_at ON alerts(created_at);

-- [index] idx_alerts_fingerprint_unique
CREATE UNIQUE INDEX idx_alerts_fingerprint_unique ON alerts(alert_fingerprint) WHERE alert_fingerprint IS NOT NULL;

-- [index] idx_alerts_severity
CREATE INDEX idx_alerts_severity ON alerts(severity);

-- [index] idx_alerts_source_created
CREATE INDEX idx_alerts_source_created ON alerts(source, created_at DESC);

-- [index] idx_alerts_status
CREATE INDEX idx_alerts_status ON alerts(status);

-- [index] idx_alerts_status_created
CREATE INDEX idx_alerts_status_created ON alerts(status, created_at DESC);

-- [index] idx_alerts_task
CREATE INDEX idx_alerts_task ON alerts(related_task_id);

-- [index] idx_alerts_title
CREATE INDEX idx_alerts_title ON alerts(title);

-- [index] idx_approval_created
CREATE INDEX idx_approval_created ON approval_requests(created_at DESC);

-- [index] idx_approval_status
CREATE INDEX idx_approval_status ON approval_requests(status);

-- [index] idx_approval_task
CREATE INDEX idx_approval_task ON approval_requests(task_id);

-- [index] idx_audit_created_at
CREATE INDEX idx_audit_created_at ON audit_logs(created_at);

-- [index] idx_audit_rca
CREATE INDEX idx_audit_rca ON remediation_audits(rca_id);

-- [index] idx_audit_server
CREATE INDEX idx_audit_server ON remediation_audits(server_id);

-- [index] idx_audit_status
CREATE INDEX idx_audit_status ON remediation_audits(status);

-- [index] idx_audit_user
CREATE INDEX idx_audit_user ON audit_logs(user_id);

-- [index] idx_auto_analysis_alert
CREATE INDEX idx_auto_analysis_alert ON alert_auto_analysis(alert_id);

-- [index] idx_auto_analysis_status
CREATE INDEX idx_auto_analysis_status ON alert_auto_analysis(status);

-- [index] idx_backup_records_device
CREATE INDEX idx_backup_records_device ON config_backup_records(device_id);

-- [index] idx_backup_records_started
CREATE INDEX idx_backup_records_started ON config_backup_records(started_at);

-- [index] idx_backup_records_status
CREATE INDEX idx_backup_records_status ON config_backup_records(status);

-- [index] idx_backup_records_task
CREATE INDEX idx_backup_records_task ON config_backup_records(task_id);

-- [index] idx_backup_tasks_enabled
CREATE INDEX idx_backup_tasks_enabled ON config_backup_tasks(enabled);

-- [index] idx_backup_tasks_template
CREATE INDEX idx_backup_tasks_template ON config_backup_tasks(template_id);

-- [index] idx_backup_templates_vendor
CREATE INDEX idx_backup_templates_vendor ON config_backup_templates(vendor);

-- [index] idx_change_created
CREATE INDEX idx_change_created ON change_records(created_at DESC);

-- [index] idx_change_server
CREATE INDEX idx_change_server ON change_records(server_id);

-- [index] idx_change_status
CREATE INDEX idx_change_status ON change_records(status);

-- [index] idx_change_type
CREATE INDEX idx_change_type ON change_records(change_type);

-- [index] idx_cmd_history_executed_at
CREATE INDEX idx_cmd_history_executed_at ON server_command_history(executed_at);

-- [index] idx_cmd_history_server_id
CREATE INDEX idx_cmd_history_server_id ON server_command_history(server_id);

-- [index] idx_collect_results_device
CREATE INDEX idx_collect_results_device ON snmp_collect_results(device_id);

-- [index] idx_collect_results_sampled
CREATE INDEX idx_collect_results_sampled ON snmp_collect_results(sampled_at DESC);

-- [index] idx_collect_results_task
CREATE INDEX idx_collect_results_task ON snmp_collect_results(task_id);

-- [index] idx_collect_results_task_device
CREATE INDEX idx_collect_results_task_device ON snmp_collect_results(task_id, device_id);

-- [index] idx_compliance_created_at
CREATE INDEX idx_compliance_created_at ON compliance_checks(created_at);

-- [index] idx_compliance_server_id
CREATE INDEX idx_compliance_server_id ON compliance_checks(server_id);

-- [index] idx_compliance_status
CREATE INDEX idx_compliance_status ON compliance_checks(status);

-- [index] idx_config_backups_device
CREATE INDEX idx_config_backups_device ON network_config_backups(device_id, created_at DESC);

-- [index] idx_config_history_server
CREATE INDEX idx_config_history_server ON config_template_history(server_id);

-- [index] idx_config_history_status
CREATE INDEX idx_config_history_status ON config_template_history(status);

-- [index] idx_config_history_template
CREATE INDEX idx_config_history_template ON config_template_history(template_id);

-- [index] idx_config_templates_category
CREATE INDEX idx_config_templates_category ON config_templates(category);

-- [index] idx_config_templates_os
CREATE INDEX idx_config_templates_os ON config_templates(os_type);

-- [index] idx_config_templates_service
CREATE INDEX idx_config_templates_service ON config_templates(service_name);

-- [index] idx_config_templates_type
CREATE INDEX idx_config_templates_type ON config_templates(type);

-- [index] idx_copilot_user_id
CREATE INDEX idx_copilot_user_id ON copilot_conversations(user_id);

-- [index] idx_corr_group_status
CREATE INDEX idx_corr_group_status ON alert_correlation_groups(status, created_at DESC);

-- [index] idx_corr_member_alert
CREATE INDEX idx_corr_member_alert ON alert_correlation_members(alert_id);

-- [index] idx_corr_member_group
CREATE INDEX idx_corr_member_group ON alert_correlation_members(group_id);

-- [index] idx_custom_metrics_dashboard
CREATE INDEX idx_custom_metrics_dashboard ON custom_dashboard_metrics(show_on_dashboard, display_order);

-- [index] idx_custom_metrics_device
CREATE INDEX idx_custom_metrics_device ON custom_dashboard_metrics(device_id);

-- [index] idx_custom_oid_profiles_enabled
CREATE INDEX idx_custom_oid_profiles_enabled ON snmp_custom_oid_profiles(enabled);

-- [index] idx_custom_oid_profiles_vendor
CREATE INDEX idx_custom_oid_profiles_vendor ON snmp_custom_oid_profiles(vendor);

-- [index] idx_custom_oid_results_device
CREATE INDEX idx_custom_oid_results_device ON snmp_custom_oid_results(device_id);

-- [index] idx_custom_oid_results_profile
CREATE INDEX idx_custom_oid_results_profile ON snmp_custom_oid_results(profile_id);

-- [index] idx_custom_oid_results_sampled
CREATE INDEX idx_custom_oid_results_sampled ON snmp_custom_oid_results(sampled_at DESC);

-- [index] idx_databases_enabled
CREATE INDEX idx_databases_enabled ON databases(enabled);

-- [index] idx_databases_name
CREATE INDEX idx_databases_name ON databases(name);

-- [index] idx_databases_name_unique
CREATE UNIQUE INDEX idx_databases_name_unique ON databases(name);

-- [index] idx_dc_cables_a
CREATE INDEX idx_dc_cables_a ON dc_cables(a_device_id, a_device_type);

-- [index] idx_dc_cables_b
CREATE INDEX idx_dc_cables_b ON dc_cables(b_device_id, b_device_type);

-- [index] idx_dc_cables_status
CREATE INDEX idx_dc_cables_status ON dc_cables(status);

-- [index] idx_dc_lifecycle_device
CREATE INDEX idx_dc_lifecycle_device ON dc_device_lifecycle(device_id);

-- [index] idx_dc_lifecycle_rack
CREATE INDEX idx_dc_lifecycle_rack ON dc_device_lifecycle(to_rack_id);

-- [index] idx_dc_pdus_rack
CREATE INDEX idx_dc_pdus_rack ON dc_pdus(rack_id);

-- [index] idx_dc_power_feeds_panel
CREATE INDEX idx_dc_power_feeds_panel ON dc_power_feeds(power_panel_id);

-- [index] idx_dc_power_feeds_rack
CREATE INDEX idx_dc_power_feeds_rack ON dc_power_feeds(rack_id);

-- [index] idx_dc_power_panels_room
CREATE INDEX idx_dc_power_panels_room ON dc_power_panels(room_id);

-- [index] idx_dc_rack_slots_device
CREATE INDEX idx_dc_rack_slots_device ON dc_rack_slots(device_id);

-- [index] idx_dc_rack_slots_device_type
CREATE INDEX idx_dc_rack_slots_device_type ON dc_rack_slots(device_type_id);

-- [index] idx_dc_rack_slots_rack
CREATE INDEX idx_dc_rack_slots_rack ON dc_rack_slots(rack_id);

-- [index] idx_dc_racks_room
CREATE INDEX idx_dc_racks_room ON dc_racks(room_id);

-- [index] idx_device_types_manufacturer_model
CREATE UNIQUE INDEX idx_device_types_manufacturer_model
      ON device_types(manufacturer_id, model);

-- [index] idx_device_types_manufacturer_slug
CREATE UNIQUE INDEX idx_device_types_manufacturer_slug
      ON device_types(manufacturer_id, slug);

-- [index] idx_discovery_jobs_status
CREATE INDEX idx_discovery_jobs_status ON network_discovery_jobs(status);

-- [index] idx_discovery_results_ip
CREATE INDEX idx_discovery_results_ip ON network_discovery_results(ip_address);

-- [index] idx_discovery_results_job
CREATE INDEX idx_discovery_results_job ON network_discovery_results(job_id, status);

-- [index] idx_encryption_active
CREATE INDEX idx_encryption_active ON encryption_keys(active);

-- [index] idx_ext_device_name
CREATE UNIQUE INDEX idx_ext_device_name ON network_external_devices(name);

-- [index] idx_inspection_history_created
CREATE INDEX idx_inspection_history_created ON network_inspection_history(created_at DESC);

-- [index] idx_inspection_history_device
CREATE INDEX idx_inspection_history_device ON network_inspection_history(device_id);

-- [index] idx_inspection_history_status
CREATE INDEX idx_inspection_history_status ON network_inspection_history(status);

-- [index] idx_inspection_history_type
CREATE INDEX idx_inspection_history_type ON network_inspection_history(inspection_type);

-- [index] idx_kb_alert_id
CREATE INDEX idx_kb_alert_id ON knowledge_base(alert_id);

-- [index] idx_kb_category
CREATE INDEX idx_kb_category ON knowledge_base(category);

-- [index] idx_kb_source
CREATE INDEX idx_kb_source ON knowledge_base(source);

-- [index] idx_kb_usage
CREATE INDEX idx_kb_usage ON knowledge_base(usage_count);

-- [index] idx_kb_workflow_id
CREATE INDEX idx_kb_workflow_id ON knowledge_base(workflow_id);

-- [index] idx_lldp_neighbors_device
CREATE INDEX idx_lldp_neighbors_device ON network_lldp_neighbors(device_id);

-- [index] idx_lldp_neighbors_remote
CREATE INDEX idx_lldp_neighbors_remote ON network_lldp_neighbors(remote_device_name);

-- [index] idx_match_log_unmatched
CREATE INDEX idx_match_log_unmatched ON alert_device_match_log(matched, created_at DESC);

-- [index] idx_mib_objects_module_id
CREATE INDEX idx_mib_objects_module_id ON mib_objects(module_id);

-- [index] idx_mib_objects_module_name
CREATE INDEX idx_mib_objects_module_name ON mib_objects(module_name);

-- [index] idx_mib_objects_name
CREATE INDEX idx_mib_objects_name ON mib_objects(name);

-- [index] idx_mib_objects_oid
CREATE INDEX idx_mib_objects_oid ON mib_objects(oid);

-- [index] idx_network_devices_ip
CREATE INDEX idx_network_devices_ip ON network_devices(ip_address);

-- [index] idx_network_devices_ssh_key
CREATE INDEX idx_network_devices_ssh_key ON network_devices(ssh_key_id);

-- [index] idx_network_devices_status
CREATE INDEX idx_network_devices_status ON network_devices(status);

-- [index] idx_network_devices_vendor
CREATE INDEX idx_network_devices_vendor ON network_devices(vendor);

-- [index] idx_network_ips_subnet
CREATE INDEX idx_network_ips_subnet ON network_ips(subnet_id);

-- [index] idx_network_ips_unique
CREATE UNIQUE INDEX idx_network_ips_unique ON network_ips(subnet_id, ip_address);

-- [index] idx_noise_reduction_fingerprint
CREATE INDEX idx_noise_reduction_fingerprint ON alert_noise_reduction(alert_fingerprint);

-- [index] idx_noise_reduction_last_occurrence
CREATE INDEX idx_noise_reduction_last_occurrence ON alert_noise_reduction(last_occurrence DESC);

-- [index] idx_noise_reduction_suppressed
CREATE INDEX idx_noise_reduction_suppressed ON alert_noise_reduction(is_suppressed);

-- [index] idx_notifications_created_at
CREATE INDEX idx_notifications_created_at ON notifications(created_at);

-- [index] idx_notifications_status
CREATE INDEX idx_notifications_status ON notifications(status);

-- [index] idx_notifications_status_created
CREATE INDEX idx_notifications_status_created ON notifications(status, created_at DESC);

-- [index] idx_notifications_type
CREATE INDEX idx_notifications_type ON notifications(type);

-- [index] idx_proc_alert_id
CREATE INDEX idx_proc_alert_id ON alert_processing_records(alert_id);

-- [index] idx_proc_status
CREATE INDEX idx_proc_status ON alert_processing_records(status);

-- [index] idx_rca_alert_id
CREATE INDEX idx_rca_alert_id ON root_cause_analyses(alert_id);

-- [index] idx_rca_created
CREATE INDEX idx_rca_created ON root_cause_analyses(created_at);

-- [index] idx_rca_status
CREATE INDEX idx_rca_status ON root_cause_analyses(status);

-- [index] idx_remediation_cooldowns_until
CREATE INDEX idx_remediation_cooldowns_until ON remediation_cooldowns(cooldown_until);

-- [index] idx_remediation_exec_policy_status
CREATE INDEX idx_remediation_exec_policy_status ON remediation_executions(policy_id, status);

-- [index] idx_remediation_exec_workflow
CREATE INDEX idx_remediation_exec_workflow ON remediation_executions(workflow_execution_id);

-- [index] idx_remediation_executions_alert
CREATE INDEX idx_remediation_executions_alert ON remediation_executions(alert_id);

-- [index] idx_remediation_executions_created
CREATE INDEX idx_remediation_executions_created ON remediation_executions(created_at DESC);

-- [index] idx_remediation_executions_policy
CREATE INDEX idx_remediation_executions_policy ON remediation_executions(policy_id);

-- [index] idx_remediation_executions_status
CREATE INDEX idx_remediation_executions_status ON remediation_executions(status);

-- [index] idx_remediation_history_policy
CREATE INDEX idx_remediation_history_policy ON remediation_history(policy_id);

-- [index] idx_remediation_history_policy_status
CREATE INDEX idx_remediation_history_policy_status ON remediation_history(policy_id, execution_status);

-- [index] idx_remediation_history_status
CREATE INDEX idx_remediation_history_status ON remediation_history(execution_status);

-- [index] idx_remediation_policies_alert_source
CREATE INDEX idx_remediation_policies_alert_source ON remediation_policies(alert_source);

-- [index] idx_remediation_policies_enabled
CREATE INDEX idx_remediation_policies_enabled ON remediation_policies(enabled);

-- [index] idx_remediation_policies_execution_mode
CREATE INDEX idx_remediation_policies_execution_mode ON remediation_policies(execution_mode);

-- [index] idx_report_schedules_enabled
CREATE INDEX idx_report_schedules_enabled ON report_schedules(enabled);

-- [index] idx_report_schedules_template
CREATE INDEX idx_report_schedules_template ON report_schedules(template_id);

-- [index] idx_reports_created_at
CREATE INDEX idx_reports_created_at ON reports(created_at DESC);

-- [index] idx_reports_is_preset
CREATE INDEX idx_reports_is_preset ON reports(is_preset);

-- [index] idx_reports_task_id
CREATE INDEX idx_reports_task_id ON reports(task_id);

-- [index] idx_reports_template_id
CREATE INDEX idx_reports_template_id ON reports(template_id);

-- [index] idx_reports_type
CREATE INDEX idx_reports_type ON reports(type);

-- [index] idx_scheduled_enabled
CREATE INDEX idx_scheduled_enabled ON scheduled_tasks(enabled);

-- [index] idx_scripts_category
CREATE INDEX idx_scripts_category ON scripts(category);

-- [index] idx_scripts_name
CREATE INDEX idx_scripts_name ON scripts(name);

-- [index] idx_server_group_mapping_group
CREATE INDEX idx_server_group_mapping_group ON server_group_mapping(group_id);

-- [index] idx_server_group_mapping_server
CREATE INDEX idx_server_group_mapping_server ON server_group_mapping(server_id);

-- [index] idx_server_groups_parent
CREATE INDEX idx_server_groups_parent ON server_groups(parent_id);

-- [index] idx_server_metrics_collected
CREATE INDEX idx_server_metrics_collected ON server_metrics(collected_at DESC);

-- [index] idx_server_metrics_server
CREATE INDEX idx_server_metrics_server ON server_metrics(server_id);

-- [index] idx_server_metrics_server_collected
CREATE INDEX idx_server_metrics_server_collected ON server_metrics(server_id, collected_at DESC);

-- [index] idx_servers_cloud_instance
CREATE INDEX idx_servers_cloud_instance ON servers(cloud_provider, cloud_instance_id);

-- [index] idx_servers_cloud_provider
CREATE INDEX idx_servers_cloud_provider ON servers(cloud_provider);

-- [index] idx_servers_enabled
CREATE INDEX idx_servers_enabled ON servers(enabled);

-- [index] idx_servers_ip_unique
CREATE UNIQUE INDEX idx_servers_ip_unique ON servers(ip_address) WHERE ip_address IS NOT NULL;

-- [index] idx_servers_name
CREATE INDEX idx_servers_name ON servers(name);

-- [index] idx_settings_key
CREATE INDEX idx_settings_key ON settings(key);

-- [index] idx_slot_defs_type
CREATE INDEX idx_slot_defs_type ON device_type_slot_definitions(device_type_id);

-- [index] idx_snmp_cred_device
CREATE INDEX idx_snmp_cred_device ON snmp_credentials(device_id);

-- [index] idx_snmp_poll_device
CREATE INDEX idx_snmp_poll_device ON snmp_polling_tasks(device_id);

-- [index] idx_snmp_trap_ip
CREATE INDEX idx_snmp_trap_ip ON snmp_trap_events(source_ip);

-- [index] idx_snmp_trap_time
CREATE INDEX idx_snmp_trap_time ON snmp_trap_events(created_at DESC);

-- [index] idx_ssh_keys_auth_type
CREATE INDEX idx_ssh_keys_auth_type ON ssh_keys(auth_type);

-- [index] idx_ssh_keys_name
CREATE INDEX idx_ssh_keys_name ON ssh_keys(name);

-- [index] idx_tasks_created_at
CREATE INDEX idx_tasks_created_at ON tasks(created_at);

-- [index] idx_tasks_execution_depth
CREATE INDEX idx_tasks_execution_depth ON tasks(execution_depth);

-- [index] idx_tasks_parent_task
CREATE INDEX idx_tasks_parent_task ON tasks(parent_task_id);

-- [index] idx_tasks_report
CREATE INDEX idx_tasks_report ON tasks(report_id);

-- [index] idx_tasks_status
CREATE INDEX idx_tasks_status ON tasks(status);

-- [index] idx_tasks_status_created
CREATE INDEX idx_tasks_status_created ON tasks(status, created_at DESC);

-- [index] idx_tasks_workflow_status
CREATE INDEX idx_tasks_workflow_status ON tasks(workflow_id, status);

-- [index] idx_token_blacklist_expires
CREATE INDEX idx_token_blacklist_expires ON token_blacklist(expires_at);

-- [index] idx_token_blacklist_token
CREATE INDEX idx_token_blacklist_token ON token_blacklist(token);

-- [index] idx_topo_links_deviceA
CREATE INDEX idx_topo_links_deviceA ON network_topology_links(deviceA_id);

-- [index] idx_topo_links_deviceB
CREATE INDEX idx_topo_links_deviceB ON network_topology_links(deviceB_id);

-- [index] idx_topo_links_unique
CREATE UNIQUE INDEX idx_topo_links_unique
      ON network_topology_links(deviceA_id, deviceA_interface, deviceB_id, deviceB_interface);

-- [index] idx_topology_source
CREATE INDEX idx_topology_source ON service_topologies(source_server_id);

-- [index] idx_topology_status
CREATE INDEX idx_topology_status ON service_topologies(status);

-- [index] idx_topology_target
CREATE INDEX idx_topology_target ON service_topologies(target_server_id);

-- [index] idx_users_role
CREATE INDEX idx_users_role ON users(role);

-- [index] idx_variable_transfers_source
CREATE INDEX idx_variable_transfers_source ON workflow_variable_transfers(source_node_id);

-- [index] idx_variable_transfers_target
CREATE INDEX idx_variable_transfers_target ON workflow_variable_transfers(target_node_id);

-- [index] idx_variable_transfers_task
CREATE INDEX idx_variable_transfers_task ON workflow_variable_transfers(task_id);

-- [index] idx_vm_audit_platform
CREATE INDEX idx_vm_audit_platform ON vm_audit_logs(platform_id);

-- [index] idx_vm_audit_time
CREATE INDEX idx_vm_audit_time ON vm_audit_logs(started_at DESC);

-- [index] idx_vm_audit_vm
CREATE INDEX idx_vm_audit_vm ON vm_audit_logs(vm_id);

-- [index] idx_vm_platforms_status
CREATE INDEX idx_vm_platforms_status ON vm_platforms(status);

-- [index] idx_vm_platforms_type
CREATE INDEX idx_vm_platforms_type ON vm_platforms(hypervisor_type);

-- [index] idx_webhook_logs_created
CREATE INDEX idx_webhook_logs_created ON alert_webhook_logs(created_at);

-- [index] idx_webhook_logs_source
CREATE INDEX idx_webhook_logs_source ON alert_webhook_logs(source);

-- [index] idx_webhook_logs_source_created
CREATE INDEX idx_webhook_logs_source_created ON alert_webhook_logs(source, created_at DESC);

-- [index] idx_webhook_logs_status
CREATE INDEX idx_webhook_logs_status ON alert_webhook_logs(status);

-- [index] idx_workflow_logs_node
CREATE INDEX idx_workflow_logs_node ON workflow_execution_logs(node_id);

-- [index] idx_workflow_logs_started
CREATE INDEX idx_workflow_logs_started ON workflow_execution_logs(started_at DESC);

-- [index] idx_workflow_logs_status
CREATE INDEX idx_workflow_logs_status ON workflow_execution_logs(status);

-- [index] idx_workflow_logs_task
CREATE INDEX idx_workflow_logs_task ON workflow_execution_logs(task_id);

-- [index] idx_workflow_logs_task_iteration
CREATE INDEX idx_workflow_logs_task_iteration ON workflow_execution_logs(task_id, iteration_index);

-- [index] idx_workflow_logs_task_node
CREATE INDEX idx_workflow_logs_task_node ON workflow_execution_logs(task_id, node_id);

-- [index] idx_workflows_is_template
CREATE INDEX idx_workflows_is_template ON workflows(is_template);

-- [index] idx_workflows_name
CREATE INDEX idx_workflows_name ON workflows(name);

-- [index] idx_workflows_template_created
CREATE INDEX idx_workflows_template_created ON workflows(is_template DESC, created_at DESC);

-- [table] inspection_templates
CREATE TABLE inspection_templates (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      vendor TEXT DEFAULT 'other',
      category TEXT DEFAULT 'network',
      device_type TEXT DEFAULT 'unknown',
      os_type TEXT DEFAULT 'linux',
      description TEXT DEFAULT '',
      commands TEXT DEFAULT '[]',
      variables TEXT DEFAULT '[]',
      tags TEXT DEFAULT '[]',
      is_preset INTEGER DEFAULT 0,
      enabled INTEGER DEFAULT 1,
      usage_count INTEGER DEFAULT 0,
      created_by TEXT DEFAULT '',
      created_at TEXT DEFAULT (datetime('now','localtime')),
      updated_at TEXT DEFAULT (datetime('now','localtime'))
    );

-- [index] idx_inspection_templates_category
CREATE INDEX idx_inspection_templates_category ON inspection_templates(category);

-- [index] idx_inspection_templates_vendor
CREATE INDEX idx_inspection_templates_vendor ON inspection_templates(vendor);

-- [index] idx_inspection_templates_device_type
CREATE INDEX idx_inspection_templates_device_type ON inspection_templates(device_type);

-- [index] idx_inspection_templates_os_type
CREATE INDEX idx_inspection_templates_os_type ON inspection_templates(os_type);

