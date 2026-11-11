-- IDCBoxLine 数据库 schema v1（SQLite 方言）
--
-- 契约来源：`docs/architecture/02-系统架构设计.md` §3.3。
-- **DDL 即契约**：域层不得绕过仓储直接 SQL；任何表改动必须同步架构文档 + 本文件 + 版本号。
--
-- ## 与架构 §3.3 的差异（两处，均有意为之）
--
-- 1. **新增 `schema_version`**：不引 alembic（离线环境装包不可靠），
--    改用「幂等 DDL + 版本表」做迁移。每次启动读 `schema_version`，
--    比当前版本低就按顺序执行 `MIGRATIONS` 里的脚本。
-- 2. **新增 `intent_document`**：架构 §3.3 的 34 张表里**没有意图存储**。
--    架构的设计是「意图分解进 `device_instance` / `switch_service_range` /
--    `device_net_plane` / `cable`」——那是最终形态，但代价是每次保存意图
--    都要做一次全量分解与反向映射。当初分解本身还依赖尚未实现的端口分配流程，
--    所以 v0.4 先把意图作为**带版本号的文档**落库（哈希去重 + 可回溯），
--    保留「用户改了什么」的全部信息；分解为规范模型列为后续工作。
--
--    **v0.6 已完成分解**（`domain/ports/canon.py` + `db/canon_store.py`，
--    命令 `idcbox decompose`）。两份现在并存且分工明确：
--    - `intent_document`：人写的原始意图，逐版本留历史、可对比、可回退。
--      编辑器的工作对象 —— 改一格要立刻看到校验结果，不该等重算 2 万行。
--    - 规范表（`device_instance` / `port` / `cable` / `switch_service_range`）：
--      摊平后的台账，供查询统计（"全项目还剩多少个 25GE 口"）。
--      由 `idcbox decompose` 从文档全量重建。
--
--    仍然不做「每次保存自动分解」：分解要跑完整生成流程（5127 行连线 +
--    端口分配），保存一次多几秒，而用户保存时想看的是「存住了没」。
--
-- ## 为什么是 34 张表而不是 5 张
--
-- 前几张表（project / length_rule / rack / device_instance / cable）就够跑通
-- 主流程了。但剩下的表对应的是**已经用真实数据实测出来的问题**：
-- `device_conflict`（13 对 U 位重叠）、`name_alias`（两套命名体系）、
-- `lookup_entry`（模块查找表 570/5113 行）、`bom_sheet.coverage_json`（禁止外推）、
-- `switch_service_range.base_port_no`（base 挂关系不挂交换机）。
-- 少建一张表，对应那个结论就没地方落，只能硬编码进代码里。
-- 所以这里一次建全，避免后面反复改 schema。

PRAGMA foreign_keys = ON;

-- ============ 元信息 ============
CREATE TABLE IF NOT EXISTS schema_version (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL,
  note       TEXT
);

-- ============ 0. 项目 / 意图文档 / 公式版本 / 线长规则 ============
CREATE TABLE IF NOT EXISTS project (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  site_key   TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'draft'
             CHECK (status IN ('draft','baselined','archived')),
  source_path TEXT,
  note       TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_name ON project(name);

CREATE TABLE IF NOT EXISTS intent_document (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  version     INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  doc_json    TEXT NOT NULL,
  summary_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL,
  created_by  TEXT NOT NULL DEFAULT 'local',
  note        TEXT,
  UNIQUE (project_id, version)
);
CREATE INDEX IF NOT EXISTS idx_intent_project ON intent_document(project_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_intent_hash ON intent_document(project_id, content_hash);

CREATE TABLE IF NOT EXISTS formula_version (
  id          TEXT PRIMARY KEY,
  semver      TEXT NOT NULL,
  content_hash TEXT NOT NULL UNIQUE,
  rule_json   TEXT NOT NULL,
  ladder_mode TEXT NOT NULL DEFAULT 'ifs_compat'
              CHECK (ladder_mode IN ('ifs_compat','continuous')),
  status      TEXT NOT NULL DEFAULT 'active'
              CHECK (status IN ('draft','active','frozen','retired')),
  note        TEXT,
  created_at  TEXT NOT NULL,
  created_by  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS length_rule (
  id          TEXT PRIMARY KEY,
  project_id  TEXT REFERENCES project(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  rack_spacing_m REAL NOT NULL DEFAULT 0.6,
  slack_ratio REAL NOT NULL DEFAULT 1.2,
  n_base      INTEGER NOT NULL DEFAULT 4,
  n_extra     INTEGER NOT NULL DEFAULT 4,
  cmax_403    INTEGER NOT NULL DEFAULT 38,
  cmax_404    INTEGER NOT NULL DEFAULT 18,
  o_cross_aisle_mode TEXT NOT NULL DEFAULT 'rack_end'
              CHECK (o_cross_aisle_mode IN ('rack_end','manual_only')),
  allow_auto_q_override INTEGER NOT NULL DEFAULT 1,
  is_active   INTEGER NOT NULL DEFAULT 1,
  UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS length_rule_param (
  rule_id    TEXT NOT NULL REFERENCES length_rule(id) ON DELETE CASCADE,
  key        TEXT NOT NULL,
  value_json TEXT NOT NULL,
  PRIMARY KEY (rule_id, key)
);

CREATE TABLE IF NOT EXISTS ladder_rung (
  rule_id     TEXT NOT NULL REFERENCES length_rule(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  op          TEXT NOT NULL CHECK (op IN ('le','eq','lt')),
  limit_value REAL NOT NULL,
  out_length  INTEGER NOT NULL,
  label       TEXT,
  PRIMARY KEY (rule_id, seq)
);

CREATE TABLE IF NOT EXISTS len_empirical_rule (
  id          TEXT PRIMARY KEY,
  project_id  TEXT REFERENCES project(id) ON DELETE CASCADE,
  formula_version_id TEXT NOT NULL REFERENCES formula_version(id),
  space_class TEXT NOT NULL CHECK (space_class IN ('A','B','C')),
  cable_type  TEXT NOT NULL,
  path_units  INTEGER NOT NULL,
  length_m    INTEGER NOT NULL,
  sample_n    INTEGER NOT NULL,
  purity      REAL NOT NULL,
  confidence  TEXT GENERATED ALWAYS AS (
    CASE WHEN purity >= 0.95 AND sample_n >= 10 THEN 'HIGH'
         WHEN purity >= 0.85 THEN 'MEDIUM' ELSE 'LOW' END) VIRTUAL,
  UNIQUE (project_id, formula_version_id, space_class, cable_type, path_units)
);

-- ============ 1. 空间与机柜 ============
CREATE TABLE IF NOT EXISTS room (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  code       TEXT NOT NULL,
  col_min    INTEGER NOT NULL,
  col_max    INTEGER NOT NULL,
  power_kw_per_rack REAL NOT NULL DEFAULT 4.0,
  declared_rack_count INTEGER,
  UNIQUE (project_id, code)
);

CREATE TABLE IF NOT EXISTS aisle (
  id         TEXT PRIMARY KEY,
  room_id    TEXT NOT NULL REFERENCES room(id) ON DELETE CASCADE,
  channel_no INTEGER NOT NULL,
  name       TEXT,
  plan_row_n INTEGER NOT NULL,
  plan_row_s INTEGER NOT NULL,
  UNIQUE (room_id, channel_no)
);

CREATE TABLE IF NOT EXISTS rack (
  id         TEXT PRIMARY KEY,
  room_id    TEXT NOT NULL REFERENCES room(id),
  aisle_id   TEXT NOT NULL REFERENCES aisle(id) ON DELETE CASCADE,
  seq_no     INTEGER NOT NULL,
  side       TEXT NOT NULL CHECK (side IN ('N','S')),
  display_name TEXT,
  alias_code TEXT,
  col_index  INTEGER NOT NULL,
  row_index  INTEGER NOT NULL,
  slot_index INTEGER,
  rack_units INTEGER NOT NULL DEFAULT 42,
  width_mm   INTEGER NOT NULL DEFAULT 600,
  power_budget_kw REAL NOT NULL DEFAULT 4.0,
  status     TEXT NOT NULL DEFAULT 'planned'
             CHECK (status IN ('planned','installed','reserved','empty')),
  UNIQUE (aisle_id, side, seq_no)
);
CREATE INDEX IF NOT EXISTS idx_rack_geom ON rack(room_id, col_index, row_index);

CREATE TABLE IF NOT EXISTS grid_cell (
  id         TEXT PRIMARY KEY,
  room_id    TEXT NOT NULL REFERENCES room(id) ON DELETE CASCADE,
  col_index  INTEGER NOT NULL,
  row_index  INTEGER NOT NULL,
  kind       TEXT NOT NULL
             CHECK (kind IN ('rack','aircon','pillar','duct','walkway','header','empty')),
  raw_text   TEXT,
  rack_id    TEXT REFERENCES rack(id),
  UNIQUE (room_id, col_index, row_index)
);

CREATE TABLE IF NOT EXISTS phase (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  code       TEXT NOT NULL,
  name       TEXT NOT NULL,
  seq        INTEGER NOT NULL,
  planned_at TEXT,
  is_baseline INTEGER NOT NULL DEFAULT 0,
  note       TEXT,
  UNIQUE (project_id, code)
);

-- ============ 2. 型号 / 端口模板 / 设备实例 ============
CREATE TABLE IF NOT EXISTS device_model (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  name_normalized TEXT NOT NULL,
  vendor     TEXT,
  category   TEXT NOT NULL
             CHECK (category IN ('switch','server','security','balancer','storage','other')),
  role_hint  TEXT,
  package_code TEXT,
  u_height   REAL NOT NULL DEFAULT 1,
  width_ratio REAL NOT NULL DEFAULT 1.0,
  spec_text  TEXT,
  UNIQUE (project_id, name_normalized)
);

CREATE TABLE IF NOT EXISTS device_model_port (
  id         TEXT PRIMARY KEY,
  model_id   TEXT NOT NULL REFERENCES device_model(id) ON DELETE CASCADE,
  seq        INTEGER NOT NULL,
  physical_name TEXT NOT NULL,
  display_name TEXT,
  speed_mbps INTEGER NOT NULL,
  connector  TEXT NOT NULL
             CHECK (connector IN ('RJ45','LC','MPO','SFP28','QSFP28','OTHER')),
  port_type  TEXT NOT NULL CHECK (port_type IN ('electrical','optical')),
  panel_slot INTEGER,
  panel_row  INTEGER,
  panel_col  INTEGER,
  parent_index INTEGER,
  lane_index INTEGER,
  capacity_divisor INTEGER NOT NULL DEFAULT 1,
  UNIQUE (model_id, physical_name)
);

CREATE TABLE IF NOT EXISTS device_instance (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  model_id   TEXT NOT NULL REFERENCES device_model(id),
  rack_id    TEXT NOT NULL REFERENCES rack(id),
  u_start    INTEGER NOT NULL CHECK (u_start BETWEEN 1 AND 48),
  u_end      INTEGER NOT NULL CHECK (u_end >= u_start),
  -- ★ 可空：824 个位置里有 631 个没有实例名，绝不能当唯一键
  name       TEXT,
  name_normalized TEXT,
  role       TEXT,
  role_enum  TEXT,
  phase_id   TEXT REFERENCES phase(id),
  status     TEXT NOT NULL DEFAULT 'planned'
             CHECK (status IN ('planned','installed','moved','removed')),
  is_half_width INTEGER NOT NULL DEFAULT 0,
  side_offset TEXT,
  power_w    INTEGER,
  serial_no  TEXT,
  mgmt_ip    TEXT,
  first_seen_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
-- ★ 不加 U 位唯一约束：源数据有 13 对重叠，导入必须成功 → 落 device_conflict
CREATE INDEX IF NOT EXISTS idx_dev_rack_u ON device_instance(rack_id, u_start, u_end);
CREATE INDEX IF NOT EXISTS idx_dev_name ON device_instance(project_id, name_normalized);

CREATE TABLE IF NOT EXISTS device_conflict (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL
             CHECK (kind IN ('U_OVERLAP','MODEL_CONFLICT','NAME_CONFLICT',
                             'PORT_CONFLICT','RATE_MISMATCH')),
  rack_id    TEXT REFERENCES rack(id),
  u_start    INTEGER,
  u_end      INTEGER,
  detail_json TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT 'import',
  severity   TEXT NOT NULL DEFAULT 'warn' CHECK (severity IN ('warn','error')),
  resolution TEXT NOT NULL DEFAULT 'open'
             CHECK (resolution IN ('open','accepted','resolved','ignored')),
  resolved_by TEXT,
  resolved_at TEXT,
  resolve_note TEXT,
  import_job_id TEXT
);
CREATE INDEX IF NOT EXISTS idx_devconflict_open ON device_conflict(project_id, resolution);

CREATE TABLE IF NOT EXISTS port (
  id         TEXT PRIMARY KEY,
  device_id  TEXT NOT NULL REFERENCES device_instance(id) ON DELETE CASCADE,
  physical_name TEXT NOT NULL,
  display_name TEXT,
  speed_mbps INTEGER NOT NULL,
  connector  TEXT NOT NULL,
  port_type  TEXT NOT NULL CHECK (port_type IN ('electrical','optical')),
  breakout_parent_id TEXT REFERENCES port(id),
  lane_index INTEGER,
  capacity_divisor INTEGER NOT NULL DEFAULT 1,
  logical_network_id TEXT,
  -- ★ 一等枚举：决定候选池 / 校验 / 2D 配色
  port_role  TEXT NOT NULL DEFAULT 'unknown'
             CHECK (port_role IN ('downlink','uplink','peer_link','stack','mgmt','spare','unknown')),
  status     TEXT NOT NULL DEFAULT 'free'
             CHECK (status IN ('free','occupied','reserved','fault','unknown')),
  panel_slot INTEGER,
  panel_row  INTEGER,
  panel_col  INTEGER,
  -- ★ 端口硬唯一（C1 规则的前置条件）
  UNIQUE (device_id, physical_name)
);
CREATE INDEX IF NOT EXISTS idx_port_dev_speed ON port(device_id, speed_mbps, status);

-- ============ 3. 连线与长度快照 ============
CREATE TABLE IF NOT EXISTS cable (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  stable_key TEXT NOT NULL,
  occurrence INTEGER NOT NULL DEFAULT 0,
  a_port_id  TEXT NOT NULL REFERENCES port(id),
  z_port_id  TEXT REFERENCES port(id),
  cable_type TEXT NOT NULL DEFAULT 'UNKNOWN'
             CHECK (cable_type IN ('LC-LC','RJ45','MPO','DAC','AOC','UNKNOWN')),
  cable_type_source TEXT NOT NULL DEFAULT 'computed'
             CHECK (cable_type_source IN ('computed','manual_override','imported','default_rule')),
  polarity   TEXT DEFAULT 'B' CHECK (polarity IN ('A','B','C')),
  n_cross_rack INTEGER,
  o_gap_rack INTEGER,
  p_rack_count INTEGER,
  q_in_rack_m REAL,
  r_inter_rack_m REAL,
  s_total_m  REAL,
  t_ladder_m INTEGER,
  u_approved_m INTEGER,
  u_overridden INTEGER NOT NULL DEFAULT 0,
  -- ★ 四策略 + 行级溯源：87.6% 存量行绕过公式链
  length_policy TEXT NOT NULL DEFAULT 'GEOMETRIC'
             CHECK (length_policy IN ('GEOMETRIC','EMPIRICAL','MANUAL','IMPORTED')),
  suggest_len_m INTEGER,
  confidence TEXT CHECK (confidence IN ('HIGH','MEDIUM','LOW')),
  sample_n   INTEGER,
  approve_reason TEXT,
  approve_reason_code TEXT,
  approved_by TEXT,
  approved_at TEXT,
  length_source TEXT NOT NULL DEFAULT 'none'
             CHECK (length_source IN ('computed','empirical','manual_override',
                                      'imported','historical','none','pending')),
  formula_version_id TEXT REFERENCES formula_version(id),
  a_module   TEXT,
  z_module   TEXT,
  a_module_qty INTEGER DEFAULT 1,
  z_module_qty INTEGER DEFAULT 1,
  phase_id   TEXT REFERENCES phase(id),
  status     TEXT NOT NULL DEFAULT 'planned'
             CHECK (status IN ('planned','installed','cutover','removed')),
  remark     TEXT,
  raw_row_json TEXT,
  source_row_no INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (project_id, stable_key, occurrence)
);
CREATE INDEX IF NOT EXISTS idx_cable_z ON cable(z_port_id);
CREATE INDEX IF NOT EXISTS idx_cable_type ON cable(project_id, cable_type, u_approved_m);
CREATE INDEX IF NOT EXISTS idx_cable_phase ON cable(project_id, phase_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cable_a_active
  ON cable(a_port_id) WHERE status <> 'removed';

CREATE TABLE IF NOT EXISTS length_snapshot (
  id         TEXT PRIMARY KEY,
  cable_id   TEXT NOT NULL REFERENCES cable(id) ON DELETE CASCADE,
  formula_version_id TEXT NOT NULL REFERENCES formula_version(id),
  n_cross_rack INTEGER,
  o_gap_rack INTEGER,
  p_rack_count INTEGER,
  q_in_rack_m REAL,
  r_inter_rack_m REAL,
  s_total_m  REAL,
  t_ladder_m INTEGER,
  u_approved_m INTEGER,
  policy     TEXT NOT NULL DEFAULT 'GEOMETRIC',
  suggest_len_m INTEGER,
  sample_n   INTEGER,
  path_units INTEGER,
  confidence TEXT NOT NULL DEFAULT 'A' CHECK (confidence IN ('A','B','C','NA')),
  o_class    TEXT CHECK (o_class IN ('A','B','C')),
  over_limit INTEGER NOT NULL DEFAULT 0,
  inputs_json TEXT NOT NULL,
  computed_at TEXT NOT NULL,
  reason     TEXT
);
CREATE INDEX IF NOT EXISTS idx_snap_cable ON length_snapshot(cable_id, computed_at DESC);

-- ============ 4. 别名与通用经验查找表 ============
CREATE TABLE IF NOT EXISTS name_alias (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  scope      TEXT NOT NULL CHECK (scope IN ('model','instance','cross_system')),
  raw_name   TEXT NOT NULL,
  normalized TEXT NOT NULL,
  canonical_kind TEXT NOT NULL CHECK (canonical_kind IN ('model','instance','role')),
  canonical_id TEXT,
  canonical_name TEXT,
  source_sheet TEXT,
  confidence REAL NOT NULL DEFAULT 1.0,
  confirmed  INTEGER NOT NULL DEFAULT 0,
  note       TEXT,
  UNIQUE (project_id, scope, normalized, canonical_kind, canonical_name)
);

CREATE TABLE IF NOT EXISTS lookup_entry (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL
             CHECK (kind IN ('module_by_port','u_override_reason','cable_type_alias')),
  key_json   TEXT NOT NULL,
  value      TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 1.0,
  -- ★ 必须可区分来源，否则无法审计
  source     TEXT NOT NULL CHECK (source IN ('import','rule','manual')),
  hit_count  INTEGER NOT NULL DEFAULT 0,
  confirmed_by TEXT,
  confirmed_at TEXT,
  UNIQUE (project_id, kind, key_json, value)
);

-- ============ 5. 导入 ============
CREATE TABLE IF NOT EXISTS import_template (
  id         TEXT PRIMARY KEY,
  project_id TEXT REFERENCES project(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  version    INTEGER NOT NULL DEFAULT 1,
  builtin    INTEGER NOT NULL DEFAULT 0,
  spec_json  TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS import_job (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  template_id TEXT REFERENCES import_template(id),
  file_name  TEXT NOT NULL,
  file_hash  TEXT NOT NULL,
  file_path  TEXT NOT NULL,
  state      TEXT NOT NULL DEFAULT 'staged'
             CHECK (state IN ('staged','previewed','committed','aborted')),
  stats_json TEXT,
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL,
  committed_at TEXT
);

CREATE TABLE IF NOT EXISTS import_issue (
  id         TEXT PRIMARY KEY,
  job_id     TEXT NOT NULL REFERENCES import_job(id) ON DELETE CASCADE,
  sheet_name TEXT,
  row_no     INTEGER,
  col_name   TEXT,
  code       TEXT NOT NULL,
  severity   TEXT NOT NULL CHECK (severity IN ('info','warn','error')),
  raw_value  TEXT,
  normalized_value TEXT,
  message    TEXT,
  isolated   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS import_diff (
  id         TEXT PRIMARY KEY,
  job_id     TEXT NOT NULL REFERENCES import_job(id) ON DELETE CASCADE,
  cable_id   TEXT,
  incoming_key TEXT NOT NULL,
  change     TEXT NOT NULL
             CHECK (change IN ('UNCHANGED','UPDATED','ADDED','REMOVED','MOVED',
                               'DUPLICATE_IN_IMPORT')),
  fields_json TEXT,
  decision   TEXT NOT NULL DEFAULT 'pending'
             CHECK (decision IN ('pending','accept','reject','inherit')),
  decided_by TEXT,
  decided_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_diff_job ON import_diff(job_id, change);

-- ============ 6. 校验 / 审计 / 耗材 ============
CREATE TABLE IF NOT EXISTS validation_run (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  trigger    TEXT NOT NULL,
  rule_set_version TEXT NOT NULL,
  strict     INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  warn_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS validation_finding (
  id         TEXT PRIMARY KEY,
  run_id     TEXT NOT NULL REFERENCES validation_run(id) ON DELETE CASCADE,
  rule_code  TEXT NOT NULL,
  severity   TEXT NOT NULL CHECK (severity IN ('P','w')),
  entity_kind TEXT NOT NULL,
  entity_id  TEXT,
  location   TEXT,
  message    TEXT NOT NULL,
  suggestion_json TEXT,
  blocking   INTEGER NOT NULL DEFAULT 0,
  acked_by   TEXT,
  acked_at   TEXT,
  ack_note   TEXT
);
CREATE INDEX IF NOT EXISTS idx_finding_run ON validation_finding(run_id, severity);

CREATE TABLE IF NOT EXISTS audit_log (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  actor_id   TEXT NOT NULL,
  action     TEXT NOT NULL,
  entity_kind TEXT NOT NULL,
  entity_id  TEXT,
  before_json TEXT,
  after_json TEXT,
  formula_version_id TEXT,
  ip         TEXT,
  ua         TEXT,
  at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_log(project_id, entity_kind, entity_id, at);

CREATE TABLE IF NOT EXISTS bom_sheet (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  phase_id   TEXT REFERENCES phase(id),
  kind       TEXT NOT NULL CHECK (kind IN ('cable','module','mixed')),
  scope_note TEXT,
  redundancy_ratio REAL NOT NULL DEFAULT 1.3,
  -- ★ 覆盖率必填，禁止静默外推
  coverage_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS bom_item (
  id         TEXT PRIMARY KEY,
  sheet_id   TEXT NOT NULL REFERENCES bom_sheet(id) ON DELETE CASCADE,
  item_kind  TEXT NOT NULL CHECK (item_kind IN ('cable','module')),
  item_name  TEXT NOT NULL,
  length_m   INTEGER,
  demand_qty INTEGER NOT NULL,
  purchase_qty INTEGER NOT NULL,
  covered_cables INTEGER,
  note       TEXT,
  data_completeness TEXT NOT NULL DEFAULT 'complete'
             CHECK (data_completeness IN ('complete','partial','unknown'))
);

-- ============ 7. 端口自动分配 ============
CREATE TABLE IF NOT EXISTS switch_service_range (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  switch_device_id TEXT NOT NULL REFERENCES device_instance(id) ON DELETE CASCADE,
  node_model_id TEXT NOT NULL REFERENCES device_model(id),
  -- ★ base 挂关系上，不挂交换机上（否则只能表达 1:1）
  base_port_no INTEGER NOT NULL,
  sort_rule  TEXT NOT NULL DEFAULT 'rack_then_u'
             CHECK (sort_rule IN ('rack_then_u','u_then_rack','manual')),
  port_role  TEXT NOT NULL DEFAULT 'downlink',
  node_count_expected INTEGER,
  phase_id   TEXT REFERENCES phase(id),
  note       TEXT,
  UNIQUE (project_id, switch_device_id, node_model_id, base_port_no)
);

CREATE TABLE IF NOT EXISTS device_net_plane (
  id         TEXT PRIMARY KEY,
  device_model_id TEXT NOT NULL REFERENCES device_model(id) ON DELETE CASCADE,
  nic_name   TEXT NOT NULL,
  port_seq   INTEGER NOT NULL,
  local_port_name TEXT NOT NULL,
  peer_model_id TEXT REFERENCES device_model(id),
  net_plane  INTEGER,
  redundancy_group TEXT,
  UNIQUE (device_model_id, local_port_name)
);

CREATE TABLE IF NOT EXISTS port_assign_policy (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  strategy   TEXT NOT NULL DEFAULT 'deterministic'
             CHECK (strategy IN ('deterministic','greedy_backtrack','min_cost_flow')),
  repair_threshold REAL NOT NULL DEFAULT 0.05,
  prefer_consecutive INTEGER NOT NULL DEFAULT 1,
  reserve_rules_json TEXT,
  slot_pitch_u INTEGER NOT NULL DEFAULT 4,
  slot_base_u INTEGER NOT NULL DEFAULT 3,
  slot_count INTEGER NOT NULL DEFAULT 8
);

-- ============ 8. P1 桥架（Level 2 路径累加预留，当前无数据） ============
CREATE TABLE IF NOT EXISTS tray (
  id         TEXT PRIMARY KEY,
  room_id    TEXT NOT NULL REFERENCES room(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL CHECK (kind IN ('main','branch','vertical')),
  capacity_ratio REAL NOT NULL DEFAULT 0.4,
  level      INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS tray_segment (
  id         TEXT PRIMARY KEY,
  tray_id    TEXT NOT NULL REFERENCES tray(id) ON DELETE CASCADE,
  from_node  TEXT NOT NULL,
  to_node    TEXT NOT NULL,
  length_mm  INTEGER NOT NULL,
  penalty_kind TEXT NOT NULL DEFAULT 'none'
);
