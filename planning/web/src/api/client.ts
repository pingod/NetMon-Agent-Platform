/**
 * API 客户端。**唯一允许 fetch 的地方**（视图层只消费这里返回的类型）。
 *
 * ## 为什么手写类型而不是 codegen
 *
 * 后端只有 10 个只读端点，字段稳定。引入 openapi-typescript 会带来
 * 一条"改了后端要记得重跑生成"的流程负担，而实测这套接口 v0.2~v0.3
 * 不会大改。等写接口上来（30+ 端点）再上 codegen 更划算。
 *
 * 因此这里的接口类型是**手写的契约**，任何字段改名必须同时改这两处
 * —— 由 `pnpm typecheck` 兜住（前端会报"属性不存在"）。
 */

// 独立运行时为 /api；嵌入 NetMon 构建时由 VITE_API_BASE 指到 /api/v1/planning。
const BASE = import.meta.env.VITE_API_BASE ?? '/api';

/**
 * 嵌入 NetMon 后，规划引擎挂在认证代理之后。
 * 与主应用共用 localStorage 中的 JWT（同源 iframe 可直接读取）。
 */
function authHeaders(extra?: Record<string, string>): Record<string, string> {
  const headers: Record<string, string> = { ...extra };
  const token = localStorage.getItem('token');
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

/** 会话失效时回到 NetMon 登录页（iframe 内要操作顶层窗口）。 */
function redirectIfUnauthorized(status: number): void {
  if (status !== 401) return;
  localStorage.removeItem('token');
  localStorage.removeItem('user');
  try {
    window.top?.location.replace('/login');
  } catch {
    window.location.replace('/login');
  }
}

export interface RowStats {
  total: number;
  effective: number;
  empty: number;
  skipped: number;
}

export interface MetaPayload {
  projectLabel: string;
  sourceFile: string;
  rows: RowStats;
  geometry: { rackPositions: number; cmaxByRoom: Record<string, number>; rooms: string[] };
  devices: { normalized: number; raw: number };
  cables: { computed: number; errorRows: number; warnRows: number; overLimitRows: number };
  ready: boolean;
}

export interface RackSlot {
  rackNo: number;
  col: number;
  row: number;
  side: 'N' | 'S';
  deviceCount: number;
  usedU: number;
  tools: string[];
}

export interface ChannelEntry {
  channelNo: number;
  north: RackSlot[];
  south: RackSlot[];
}

export interface RoomEntry {
  roomCode: string;
  cmax: number | null;
  channels: ChannelEntry[];
  cols: [number, number];
  rows: [number, number];
}

export interface FloorPayload {
  rackHeightU: number;
  rooms: RoomEntry[];
  totals: { racks: number; racksWithDevices: number; devices: number };
}

export interface RackDevice {
  deviceKey: string;
  model: string;
  displayName: string;
  startU: number;
  endU: number;
  sizeU: number;
  portCount: number;
  ports: string[];
  sourceRows: number[];
}

export interface RackConflict {
  a: string;
  b: string;
  uRangeA: [number, number];
  uRangeB: [number, number];
  isNameVariant: boolean;
}

export interface RackPayload {
  channelNo: number;
  seqNo: number;
  roomCode: string;
  side: 'N' | 'S';
  col: number;
  row: number;
  rackHeightU: number;
  maxUsedU: number;
  devices: RackDevice[];
  conflicts: RackConflict[];
  cables: { asLocal: number; asPeer: number; total: number };
}

export interface CableColumn {
  field: string;
  header: string;
  width: number;
  pinned?: string;
}

export interface CableRow {
  rowNo: number;
  stableKey: string;
  localDevice: string;
  localDeviceName: string;
  localPort: string;
  localRack: string;
  localChannel: number;
  localRackNo: number;
  localU: unknown;
  peerDevice: string;
  peerDeviceName: string;
  peerPort: string;
  peerRack: string;
  peerChannel: number;
  peerRackNo: number;
  peerU: unknown;
  cableType: string;
  nCross: unknown;
  oGap: unknown;
  pCount: unknown;
  qInRack: unknown;
  rInter: unknown;
  sTotal: unknown;
  tLadder: unknown;
  uApproved: unknown;
  finalLength: number | null;
  policy: string | null;
  confidence: string | null;
  oClass: string | null;
  localModule: unknown;
  peerModule: unknown;
  remark: string;
}

export interface CablesPayload {
  offset: number;
  limit: number;
  total: number;
  rows: CableRow[];
  columns: CableColumn[];
  filterOptions: { channels: number[]; cableTypes: string[] };
}

export interface HealthFinding {
  ruleCode: string;
  severity: 'blocker' | 'warn' | 'info';
  title: string;
  detail: string;
  sheet: string;
  rowNo: number | null;
  column: string | null;
  group: string | null;
  action: string | null;
}

export interface HealthReportPayload {
  projectLabel: string;
  score: number;
  totalRows: number;
  counts: { blocker: number; warn: number; info: number };
  scoreBreakdown: { dimension: string; score: number; weight: number }[];
  rules: {
    code: string;
    severity: string;
    title: string;
    count: number;
    affectedRows: number;
  }[];
  findings: HealthFinding[];
  facts: Record<string, unknown>;
}

export interface PolicyPayload {
  policies: { policy: string; count: number; participatesInDeviation: boolean }[];
  confidence: { level: string; count: number }[];
  oClasses: { class: string; count: number }[];
  /** 「假如是新建项目」口径：纯几何/经验表能替用户算多少行。 */
  freshProject: {
    geometric: number;
    empirical: number;
    manual: number;
    total: number;
    geometricPct: number;
    manualPct: number;
  };
}

/** 耗材与模块需求清单（采购用）。 */
export interface ConsumableCableRow {
  type: string;
  length: number;
  demand: number;
  /**
   * 建议采购量 = 需求 × purchaseRatio 向上取整。
   * **不是最终采购量** —— 实测人工核定倍数是 1.12~2.31，界面必须允许逐行改。
   */
  suggestedPurchase: number;
}

export interface ConsumableModuleRow {
  module: string;
  local: number;
  peer: number;
  total: number;
}

export interface ConsumablesPayload {
  /** 批次过滤条件；`null` 表示全项目口径。 */
  phase: string | null;
  rowsTotal: number;
  cables: {
    total: number;
    byType: Record<string, number>;
    ladder: number[];
    matrix: Record<string, Record<string, number>>;
    rows: ConsumableCableRow[];
    purchaseRatio: number;
  };
  modules: {
    rows: ConsumableModuleRow[];
    coverageRows: number;
    totalRows: number;
    coverageRatio: number;
    /**
     * 覆盖率披露文案，**必须原样显示**。
     * 按覆盖率外推会少买模块导致现场停工，所以这句不能省略也不能改写。
     */
    disclosure: string;
  };
  notes: string[];
}

/** 接线意图 → 布线表的生成结果（v0.3 主功能）。 */
export interface DesignIntentVolume {
  racks: number;
  switches: number;
  /** 特殊设备（网络功能服务器/防火墙/GSO…）逐台声明，与 switches 分开计数 */
  devices: number;
  routes: number;
  chains: number;
  explicitLinks: number;
  /** 意图总行数 —— UI 上要与 generatedRows 并排展示，形成"几十行 → 几千行"的对比。 */
  total: number;
  serverCount: number;
  /** 意图来源格式：'json' | 'xlsx'。用户改完 Excel 忘记转 JSON 时靠它发现问题 */
  sourceFormat: string;
}

export interface DesignRow {
  rowNo: number;
  localModel: string;
  localDevice: string;
  localChannel: number;
  localRack: number;
  localU: string;
  localPort: string;
  peerModel: string;
  peerDevice: string;
  peerChannel: number;
  peerRack: number;
  peerU: string;
  peerPort: string;
  cableType: string;
  nic: string;
  peerId: string;
  nCrossRack?: number;
  oGapRack?: number;
  pRackCount?: number;
  qInRackM?: number;
  rInterRackM?: number;
  sTotalM?: number;
  tLadderM?: number;
  uApprovedM?: number | null;
  confidence?: string;
  oClass?: string | null;
}

export interface DesignIssue {
  code: string;
  severity: string;
  message: string;
  field: string | null;
}

export interface DesignCompare {
  generatedRows: number;
  stockRows: number;
  added: number;
  missing: number;
  common: number;
  /** 长度比对的**分母**：存量表「线缆长度」列有数值的行数（实测 636）。
   *  原表 5127 行里只有这些行带公式，其余那格本就空白。
   *  不传它，界面只能拿 5127 当分母，看起来像「98.9% 准确」。 */
  lengthBaseline: number;
  /** 人工 N/O/Q 与几何一致、且公式链完全复现的行数（实测 578）。
   *  这是「公式没有误差」的唯一证据 —— 不传它，前端只能拿 baseline−diffs 硬算，
   *  算出来是 581（错），因为还有几行人工值凑巧抵消。 */
  lengthClean: number;
  /** 人工输入填错但错误恰好抵消、T 反而对上的行数（实测 3）。
   *  三个数加起来必须等于 lengthBaseline：578 + 3 + 55 = 636，用户能自己验算。 */
  lengthLucky: number;
  lengthDiffs: number;
  addedSamples: string[][];
  missingSamples: string[][];
  lengthDiffSamples: { key: string[]; generated: number; stock: number }[];
  /** 缺失行按对端型号归因 —— 让用户分清"算法漏了"和"本来就得手写"。 */
  missingByModel: { model: string; count: number }[];
}

export interface DesignPayload {
  project: string;
  site: string;
  intent: DesignIntentVolume;
  stats: {
    links: number;
    errors: number;
    warns: number;
    byNic: { nic: string; count: number }[];
    /**
     * 显式连线按「对端端口名」的分布（`100GE1/0/1` 这类）。
     *
     * 不能与 `byNic` 合并：`byNic` 是服务器网口标签（`千兆1`/`IPMI`），
     * 这里是交换机 CLI 端口名。混在一起会让「千兆1 694 / IPMI 684」
     * 看起来像漏了 10 行接线，实际是两套命名。
     */
    explicitByPort: { port: string; count: number }[];
    lengthComputed: number;
    lengthError: number;
    geometryUsed: boolean;
  };
  notes: string[];
  columns: { id: string; label: string }[];
  rows: DesignRow[];
  rowsTotal: number;
  byModel: { model: string; count: number }[];
  bySwitchCount: number;
  switchModels: string[];
  issues: DesignIssue[];
  compare?: DesignCompare;
  /**
   * 这次生成到底读了哪儿。
   *
   * `intent.sourceFormat` 只说「库 or 文件」，但库里可能有多个项目 ——
   * 用户改完 A 项目却看到 B 项目的结果，会以为保存没生效。所以把
   * 具体项目名与版本号也带回来，界面上直接写明。
   */
  source?: DesignSource;
}

export interface DesignSource {
  /** `db` 项目库 / `json` 或 `xlsx` 磁盘文件 */
  kind: string;
  project: string | null;
  version: number | null;
}

export interface RackListItem {
  channelNo: number;
  seqNo: number;
  deviceCount: number;
  label: string;
}

export interface PortUtilization {
  byModel: {
    model: string;
    deviceCount: number;
    usedPorts: number;
    avgPortsPerDevice: number;
    topPorts: { port: string; count: number }[];
    /** 端口模板给出的标称端口数；型号没模板时为 null（不是 0）。 */
    totalPorts: number | null;
    freePorts: number | null;
    /** 已用 / 标称。分母未知时为 null —— 界面必须显示「—」而不是 100%。 */
    ratio: number | null;
    hasSpec: boolean;
    noSpecDevices: number;
  }[];
  summary: {
    models: number;
    modelsWithSpec: number;
    usedPorts: number;
    totalPorts: number;
    freePorts: number;
    ratio: number;
  };
  noSpecModels: string[];
  unknownPortDevices: string[];
  /**
   * 同一实例名在源表里被记成两个型号的那些设备。
   * 看到「未知端口」时先查这里：命中说明是源表型号记错，不是端口模板问题。
   */
  modelConflicts: string[];
  note: string;
}

/** 台账里的一台设备（不含端口明细）。 */
export interface LedgerDevice {
  key: string;
  label: string;
  model: string;
  category: string;
  channel: number;
  rack: number;
  uStart: number;
  uSpan: number;
  used: number;
  total: number;
  free: number;
  hasSpec: boolean;
  /** 无模板时为 null。 */
  ratio: number | null;
  byRole: Record<string, number>;
  unknownPorts: string[];
  /** 单台详情才有。 */
  portList?: LedgerPort[];
  freeBays?: [string, number, number][];
}

export interface LedgerPort {
  port: string;
  bay: [string, number, number] | null;
  role: string;
  speed: number | null;
  peer: string;
  peerPort: string;
  cable: string;
  used: boolean;
}

export interface LedgerPayload {
  project: string;
  summary: {
    devices: number;
    used: number;
    capacity: number;
    free: number;
    ratio: number;
    noSpecDevices: number;
    overUsedDevices: number;
    unknownPortDevices: number;
  };
  byModel: {
    model: string;
    category: string;
    devices: number;
    used: number;
    capacity: number;
    free: number;
    noSpec: number;
    ratio: number | null;
    minUsed: number;
    maxUsed: number;
  }[];
  byRack: {
    channel: number;
    rack: number;
    devices: number;
    used: number;
    capacity: number;
    free: number;
    ratio: number | null;
  }[];
  devicesTotal: number;
  devices?: LedgerDevice[];
  attention?: { kind: string; severity: string; device?: string; detail: string }[];
}

class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function getJson<T>(path: string, params?: Record<string, unknown>): Promise<T> {
  const url = new URL(`${BASE}${path}`, window.location.origin);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, String(v));
    }
  }
  const res = await fetch(url.toString(), {
    headers: authHeaders({ Accept: 'application/json' }),
  });
  if (!res.ok) {
    let detail = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { detail?: string; error?: string };
      if (body.detail) detail = body.detail;
      else if (body.error) detail = body.error;
    } catch {
      /* 响应不是 JSON，保留 HTTP 状态码 */
    }
    redirectIfUnauthorized(res.status);
    throw new ApiError(detail, res.status);
  }
  return (await res.json()) as T;
}

/**
 * POST / PUT 用。与 getJson 分开是为了让「写」这个动作在调用处显眼 ——
 * 一个只读页面上出现写请求通常是 bug。
 */
async function sendJson<T>(
  method: 'POST' | 'PUT',
  path: string,
  body: unknown,
): Promise<T> {
  const url = new URL(`${BASE}${path}`, window.location.origin);
  const res = await fetch(url.toString(), {
    method,
    headers: authHeaders({ 'Content-Type': 'application/json', Accept: 'application/json' }),
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    let detail = `HTTP ${res.status}`;
    try {
      const payload = (await res.json()) as { detail?: unknown; error?: unknown };
      if (typeof payload.detail === 'string') detail = payload.detail;
      else if (typeof payload.error === 'string') detail = payload.error;
    } catch {
      /* 响应不是 JSON，保留 HTTP 状态码 */
    }
    redirectIfUnauthorized(res.status);
    throw new ApiError(detail, res.status);
  }
  return (await res.json()) as T;
}

// ---------------------------------------------------------------------------
// 项目持久化（v0.4）
// ---------------------------------------------------------------------------

export interface ProjectSummary {
  id: string;
  name: string;
  siteKey: string;
  status: string;
  latestVersion: number;
  versionCount: number;
  updatedAt: string;
}

export interface ProjectsPayload {
  projects: ProjectSummary[];
  total: number;
  database: string;
}

export interface IntentSummary {
  racks: number;
  switches: number;
  devices: number;
  routes: number;
  chains: number;
  explicitLinks: number;
  total: number;
  serverCount: number;
}

export interface VersionInfo {
  version: number;
  createdAt: string;
  createdBy: string;
  note: string;
  summary: IntentSummary;
}

export interface VersionsPayload {
  project: string;
  total: number;
  versions: VersionInfo[];
}

export interface SaveResult {
  project: string;
  version: number;
  isNew: boolean;
  summary: IntentSummary;
}

export interface DiffSection {
  section: string;
  label: string;
  added: Record<string, unknown>[];
  removed: Record<string, unknown>[];
  changed: { before: Record<string, unknown>; after: Record<string, unknown> }[];
  count: number;
  isEmpty: boolean;
}

export interface DiffPayload {
  project: string;
  fromVersion: number;
  toVersion: number;
  sections: DiffSection[];
  totalChanges: number;
}

// ---- 意图编辑与校验 ----

/** 一条校验问题。`rowNo` 是**段内** 0-based 行号，正好是表格行索引。 */
export interface IntentIssue {
  section: string;
  sectionLabel: string;
  rowNo: number | null;
  field: string;
  severity: 'error' | 'warn' | 'info';
  code: string;
  message: string;
}

export interface IntentCheck {
  ok: boolean;
  errorCount: number;
  warnCount: number;
  infoCount: number;
  sections: Record<string, number>;
  issues: IntentIssue[];
}

/** `/api/intent/validate` 的返回：业务校验 + 结构校验都跑了。 */
export interface IntentValidateResult extends IntentCheck {
  structuralError: string | null;
  generatable: boolean;
}

/** `/api/intent/row` 的返回：改完的文档 + 立刻重算的校验结果。 */
export interface IntentRowResult {
  ok: boolean;
  error: string | null;
  rowNo: number;
  doc: Record<string, unknown>;
  check: IntentCheck;
}

/** 粘贴结果。`written` 与 `notes` 分开：写了几行是事实，提示是解释。 */
export interface IntentPasteResult {
  ok: boolean;
  error: string | null;
  written: number;
  notes: string[];
  doc: Record<string, unknown>;
  check: IntentCheck;
}

export interface LabelRow {
  text: string;
  position: string;
  device: string;
  port: string;
}

export interface LabelIssueRow {
  code: string;
  message: string;
  label: string;
  severity: string;
}

export interface LabelsPayload {
  project: string;
  total: number;
  rows: LabelRow[];
  issues: LabelIssueRow[];
  countsByCode: Record<string, number>;
  describe: string;
}

export const api = {
  meta: () => getJson<MetaPayload>('/api/meta'),
  floor: () => getJson<FloorPayload>('/api/floor'),
  /**
   * 3D 场景（FR-7.1/7.2）。一次拿全房间 + 机柜 + 834 台设备。
   *
   * 为什么不复用 `/api/floor` + `/api/racks`：3D 要为每个柜算一个实例矩阵，
   * 复用的话启动就要 102 个请求才能画出第一帧。
   */
  scene: () => getJson<ScenePayload>('/api/scene'),
  racks: () => getJson<{ racks: RackListItem[]; total: number }>('/api/racks'),
  rack: (channel: number, seq: number) => getJson<RackPayload>(`/api/racks/${channel}/${seq}`),
  cables: (params: {
    offset?: number;
    limit?: number;
    channel?: number | null;
    rack?: number | null;
    cableType?: string | null;
    text?: string | null;
  }) => getJson<CablesPayload>('/api/cables', params),
  deviceCables: (name: string) =>
    getJson<{ deviceName: string; total: number; rows: CableRow[]; truncated: boolean }>(
      `/api/devices/${encodeURIComponent(name)}/cables`,
    ),
  healthReport: () => getJson<HealthReportPayload>('/api/health-report'),
  policies: () => getJson<PolicyPayload>('/api/policies'),
  consumables: (params?: { phase?: string }) =>
    getJson<ConsumablesPayload>('/api/consumables', params),
  portUtilization: () => getJson<PortUtilization>('/api/ports/utilization'),
  ledger: (params?: {
    devices?: boolean;
    limit?: number;
    device?: string;
    project?: string;
  }) => getJson<LedgerPayload>('/api/ledger', params),
  ledgerDevice: (name: string, project?: string) =>
    getJson<{ project: string; device: LedgerDevice }>('/api/ledger', {
      device: name,
      project,
    }),
  labels: (params?: { limit?: number; numberMissing?: boolean }) =>
    getJson<LabelsPayload>('/api/labels', params),
  labelsTextUrl: (params?: { limit?: number; numberMissing?: boolean }) => {
    const q = new URLSearchParams({ format: 'text' });
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.numberMissing) q.set('numberMissing', 'true');
    return `/api/labels?${q.toString()}`;
  },
  models: (params?: { project?: string; ports?: boolean }) =>
    getJson<ModelsPayload>('/api/models', params),
  modelsImport: (project: string, doc: unknown) =>
    sendJson<ModelImportResult>('POST', '/api/models/import', { project, doc }),
  modelsExport: (project: string, all = false) =>
    getJson<ModelExportResult>('/api/models/export', { project, all }),
  design: (params?: {
    limit?: number;
    compare?: boolean;
    project?: string;
    version?: number;
    /** `db` 强制读库 / `file` 强制读磁盘；空=自动（有库优先库） */
    source?: string;
  }) => getJson<DesignPayload>('/api/design', params),

  // ---- 项目持久化 ----
  projects: () => getJson<ProjectsPayload>('/api/projects'),
  createProject: (name: string, siteKey = '') =>
    sendJson<{ id: string; name: string; latestVersion: number }>('POST', '/api/projects', {
      name,
      siteKey,
    }),
  projectVersions: (name: string) =>
    getJson<VersionsPayload>(`/api/projects/${encodeURIComponent(name)}/versions`),
  projectIntent: (name: string, version?: number) =>
    getJson<{ project: string; version: number; doc: unknown }>(
      `/api/projects/${encodeURIComponent(name)}/intent`,
      version ? { version } : undefined,
    ),
  saveProjectIntent: (name: string, doc: unknown, note = '') =>
    sendJson<SaveResult>('PUT', `/api/projects/${encodeURIComponent(name)}/intent`, {
      doc,
      note,
    }),
  projectDiff: (name: string, fromVersion: number, toVersion: number) =>
    getJson<DiffPayload>(`/api/projects/${encodeURIComponent(name)}/diff`, {
      from: fromVersion,
      to: toVersion,
    }),

  // ---- 意图编辑（本地草稿，不落库）----
  intentCheck: (doc: unknown) => sendJson<IntentCheck>('POST', '/api/intent/check', { doc }),
  intentValidate: (doc: unknown) =>
    sendJson<IntentValidateResult>('POST', '/api/intent/validate', { doc }),
  intentRow: (body: {
    doc: unknown;
    section: string;
    op: 'cell' | 'append' | 'delete';
    rowNo?: number;
    column?: string;
    value?: unknown;
    template?: Record<string, unknown>;
  }) => sendJson<IntentRowResult>('POST', '/api/intent/row', body),
  intentPaste: (body: {
    doc: unknown;
    section: string;
    text: string;
    startRow?: number;
    mode?: 'overwrite' | 'insert';
  }) => sendJson<IntentPasteResult>('POST', '/api/intent/paste', body),

  // ---- Excel 化交互层：复制出 / 填充柄 ----
  // 这两件都必须回服务端算，不能在前端拼：
  //   复制出去的 TSV 由 `format_for_clipboard` 转义（Excel 那套引号规则），
  //   填充的等差/序列推算由 `plan_fill` 决定。
  // 前端再实现一份 → 两套规则迟早不一致，表现为「复制回来丢了一列」
  // 或「拖 5 格填出 4 格」，用户会以为是自己用错了。
  intentCopy: (body: {
    doc: unknown;
    section: string;
    rowStart: number;
    rowEnd: number;
    colStart: number;
    colEnd: number;
  }) => sendJson<IntentCopyResult>('POST', '/api/intent/copy', body),
  intentFill: (body: {
    doc: unknown;
    section: string;
    rowStart: number;
    rowEnd: number;
    colStart: number;
    colEnd: number;
    targetRow: number;
  }) => sendJson<IntentFillResult>('POST', '/api/intent/fill', body),

  // ---- U 位拖拽编辑 ----
  // 哪些设备能拖由后端算：判据是「意图里有没有逐台声明」。
  // 663 台（79.5%）是布局段自动排布的，拖了也没有字段可写 ——
  // 前端自己按「在不在 switches 段」猜，两套规则迟早长歪。
  intentEditable: () => getJson<IntentEditableResult>('/api/intent/editable'),

  /**
   * **干跑**：这次拖动会动到哪些行。不落库、不改 doc。
   *
   * ## 为什么要在拖的时候就能问
   *
   * 拖动会改掉用户没选中的行 —— 换柜后这台在端口组里的位次变了，
   * 同组排在它后面的设备端口号整体前移。实测把通道2 机柜1 的一台挪到
   * 5 号柜，**91 行端口号变了**。等松手才告诉他已经晚了：现场按旧表
   * 插线的人不在电脑前，改完再撤销又要重算一遍。
   *
   * `deep=false` 跳过线长重算（约 90ms → 55ms），拖动过程中用；
   * 松手前最后一次用 `deep=true` 拿完整结论。
   */
  intentImpact: (body: {
    doc: unknown;
    move?: {
      channel: number;
      rack: number;
      instance: string;
      newU: number;
      currentU?: number;
      targetRack?: number;
      allowOverlap?: boolean;
    };
    targets?: {
      channel: number;
      rack: number;
      instance: string;
      currentU?: number;
    }[];
    deltaU?: number;
    targetRack?: number;
    allowOverlap?: boolean;
    deep?: boolean;
  }) => sendJson<ImpactPayload>('POST', '/api/intent/impact', body),
  intentMove: (body: {
    doc: unknown;
    channel: number;
    rack: number;
    instance: string;
    newU: number;
    allowOverlap?: boolean;
    /** 拖布局段设备时必需：那 663 台没有实例名，靠「型号 + 当前位置」定位。 */
    currentU?: number;
    /**
     * 跨机柜拖动的目的机柜。不传 = 原地调 U 位。
     *
     * 只允许同通道内换柜：换通道等于换一组 ToR 来服务它，那是重新规划不是拖动。
     * 这条约束由后端把关（平面图上的落点高亮只是提示，不构成保证）。
     */
    targetRack?: number;
  }) => sendJson<IntentMoveResult>('POST', '/api/intent/move', body),

  /**
   * 一批设备**整体**平移 `deltaU` 个 U（可选同时换机柜）。
   *
   * ## 为什么不能在前端循环调 `intentMove`
   *
   * 8 台整体下移一格时，每台的新位置恰好是下一台的旧位置。逐个调的话
   * 第 1 台看到第 2 台还站在那里 → 一台都动不了。批量端点先把这一批
   * 从占用清单里整体摘掉再统一校验。
   *
   * `deltaU` 是相对量而非各自的新 U 位：现场动作是「这一摞往下挪 8 格」，
   * 给每台单独指定新位置要用户手算保持间距，算错就是设备叠一起。
   */
  intentMoveBatch: (body: {
    doc: unknown;
    targets: {
      channel: number;
      rack: number;
      instance: string;
      currentU?: number;
    }[];
    deltaU: number;
    targetRack?: number;
    allowOverlap?: boolean;
  }) => sendJson<IntentMoveBatchResult>('POST', '/api/intent/move-batch', body),

  // ---- 存量表跨版本对比 ----
  // 后端按表头特征认 sheet、按填充率选比对键层级 —— 这两件都不能放前端猜：
  // 六版 Excel 的 sheet 名各不相同，而键层级选错会把「补名字」报成「换了几千条线」。
  stockFiles: () => getJson<StockFilesPayload>('/api/compare/stock-files'),
  compareStocks: (params: {
    leftPath: string;
    leftSheet: string;
    rightPath: string;
    rightSheet: string;
    limit?: number;
  }) => getJson<StockComparePayload>('/api/compare/stocks', params),
  compareProjects: (left: string, right: string, limit = 20) =>
    getJson<StockComparePayload>('/api/compare/projects', { left, right, limit }),
};

export { ApiError };

export interface IntentEditableItem {
  channel: number;
  rack: number;
  /** 逐台声明过的设备是实例名；布局段排布的是**型号**（663 台都没有实例名）。 */
  instance: string;
  /** 现在占的起始 U 位。布局段设备靠它区分同柜同型号的 8 台。 */
  uStart: number;
  /** `switches` / `devices` = 有独立字段可改；`layout` = 拖动会写覆盖记录。 */
  section: string;
}

export interface IntentEditableResult {
  items: IntentEditableItem[];
  total: number;
  bySection: Record<string, number>;
}

export interface IntentCopyResult {
  ok: boolean;
  /** 已按 Excel 规则转义好的 TSV（`\r\n` 连行、`\t` 分列）。 */
  text: string;
  cellCount: number;
  /** 人话描述，如「第 1~3 行 × 第 1~6 列」，直接显示给用户核对。 */
  range: string;
}

export interface IntentFillResult {
  ok: boolean;
  error: string | null;
  /** 实际处理的行数（不是成功格数：一行里某列失败仍算处理过）。 */
  written: number;
  /** 识别出的规律，如「递增 -1」「序列（周一…）」「照抄」。 */
  pattern: string;
  notes: string[];
  doc: Record<string, unknown>;
  check: IntentCheck;
}

export interface IntentMoveBatchResult {
  ok: boolean;
  error: string | null;
  /** 实际搬动的每一台。`fromU`/`toU` 让前端能精确更新草稿位置。 */
  moved: {
    instance: string;
    fromRack: number;
    fromU: number;
    toRack: number;
    toU: number;
  }[];
  overlaps: string[];
  doc: Record<string, unknown>;
  check: IntentCheck;
  /** 这次批量移动动到了哪些行（v0.12）。失败时 counts 全 0。 */
  impact: ImpactPayload;
}

/** 一次拖动的完整影响面（`/api/intent/impact` 与 move 响应的 `impact`）。 */
export interface ImpactPayload {
  ok: boolean;
  error: string | null;
  /**
   * 除了「设备自己挪了」之外还动了别的行。
   *
   * 这是界面**必须弹提示**的判据：只挪自己时一句「已移动」就够；
   * 一旦动了别人的行，用户必须在保存前知道。
   */
  hasHiddenImpact: boolean;
  counts: {
    added: number;
    removed: number;
    /** 端口号变了（同一根线换了口）—— 标签要重打 */
    portChanges: number;
    /** 物理位置变了（机柜或 U 位）—— 影响线长 */
    positionChanges: number;
    lengthChanges: number;
    affectedSwitches: number;
    total: number;
  };
  /**
   * 线长到底算没算。
   *
   * `false` 时 `counts.lengthChanges === 0` 的含义是「**没算**」而不是「没变」。
   * 界面不能把这两句话混成一句：前者要提示用户补源文件，后者会让他以为安全。
   */
  lengthChecked: boolean;
  affectedSwitches: string[];
  affectedRacks: { channel: number; rack: number }[];
  portSamples: {
    switch: string;
    node: string;
    nic: string;
    before: string | null;
    after: string | null;
  }[];
  lengthSamples: {
    switch: string;
    node: string;
    nic: string;
    before: number;
    after: number;
    delta: number;
  }[];
  addedSamples: { switch: string; node: string; port: string | null; peerPort: string }[];
  removedSamples: { switch: string; node: string; port: string | null; peerPort: string }[];
}

export interface IntentMoveResult {
  ok: boolean;
  error: string | null;
  section: string | null;
  rowNo: number | null;
  overlaps: string[];
  /** 这台设备占几 U（前端要画正确高度，布局段设备台账里没有 U 高度）。 */
  uSpan: number;
  /**
   * 移动后所在的机柜号。跨机柜拖动时与请求里的 `rack` 不同。
   *
   * 前端靠它判断「要不要把左右两张图都重拉」：只改 U 位时同一张图刷新即可，
   * 换了柜则平面图上要换高亮、U 位图要换整张图。
   */
  rackNo: number | null;
  doc: unknown;
  check: IntentCheck;
  /** 这次移动动到了哪些行（v0.12）。失败时 counts 全 0。 */
  impact: ImpactPayload;
}

// ---- 存量表跨版本对比 ----

/** 3D 场景数据（`/api/scene`）。所有长度**都是毫米**，不是像素也不是世界单位。 */
export interface ScenePayload {
  /** 恒为 `"mm"`。前端据此断言，防止有人把 mm 当米（差 1000 倍，画面全白）。 */
  units: 'mm';
  constants: {
    mmPerU: number;
    cabinetWidth: number;
    cabinetHeight: number;
    cabinetDepth: number;
    aisleWidth: number;
    rackHeightU: number;
  };
  rooms: {
    roomCode: string;
    widthMm: number;
    depthMm: number;
    channels: number;
    cabinets: number;
    originX: number;
    originZ: number;
  }[];
  racks: {
    channel: number;
    rack: number;
    x: number;
    y: number;
    z: number;
    rotationY: number;
    deviceCount: number;
    usedU: number;
  }[];
  devices: {
    key: string;
    channel: number;
    rack: number;
    model: string;
    displayName: string;
    startU: number;
    endU: number;
    sizeU: number;
    /** 由后端 `categorize()` 判定。3D 颜色按它上色，前端不重复判定。 */
    category: string;
  }[];
  totals: {
    rooms: number;
    racks: number;
    racksWithDevices: number;
    devices: number;
  };
}

/** 一份可参与对比的存量表（一个 Excel 文件里的一个 sheet）。 */
export interface StockFileInfo {
  path: string;
  name: string;
  sheet: string;
  rows: number;
  cols: number;
  priority: number;
  label: string;
}

export interface StockFilesPayload {
  files: StockFileInfo[];
  total: number;
  directory: string;
}

/** 某一列在两侧的填充率变化（≥5 个百分点才报）。 */
export interface StockFillChange {
  field: string;
  label: string;
  before: number;
  after: number;
}

export interface StockFieldChange {
  field: string;
  label: string;
  before: string;
  after: string;
}

export interface StockChangedRow {
  row: string;
  fields: StockFieldChange[];
  fieldCount: number;
}

export interface StockLengthChange {
  row: string;
  before: number;
  after: number;
}

/** 设备级差异的计数。与下面的样本数组是**不同的键** ——
 *  第一版后端把计数和样本用同一个键，后者覆盖前者，前端拿到的是字符串数组。 */
export interface StockDeviceCounts {
  moved: number;
  added: number;
  removed: number;
  changed: number;
  leftDevices: number;
  rightDevices: number;
}

/** 设备级聚合差异：把「4886 条新增」翻译成「56 台设备挪了 U 位」。
 *
 *  逐行 diff 只能回答「哪个格子变了」；用户真正问的是「哪几台设备干了什么」。
 *  一柜 44 根线挂在同一个交换机 U 位上，报成 44 条新增 + 44 条删除是错的抽象层级。
 */
export interface StockDeviceDiff {
  counts: StockDeviceCounts;
  totalChanges: number;
  isEmpty: boolean;
  moved: string[];
  added: string[];
  removed: string[];
  changed: string[];
  /** 解释行级与设备级数字差距的一句话。空串表示不适用（无行级差异时）。 */
  note: string;
}

export interface StockComparePayload {
  left: string;
  right: string;
  /** strict / loose / position —— 后端自动选的比对键层级。 */
  keyMode: string;
  /** 为什么用这一层。必须显示给用户，否则「怎么判定的」无从追问。 */
  keyReason: string;
  leftRows: number;
  rightRows: number;
  totalChanges: number;
  isEmpty: boolean;
  addedCount: number;
  removedCount: number;
  changedCount: number;
  lengthChangeCount: number;
  added: string[];
  removed: string[];
  changed: StockChangedRow[];
  lengthChanges: StockLengthChange[];
  fillChanges: StockFillChange[];
  /** 长度变化的真实分母：两侧都有长度值的行数。 */
  lengthBaseline?: number;
  /** 设备级聚合。缺省时不渲染该块（向后兼容没有该字段的旧后端）。 */
  byDevice?: StockDeviceDiff;
  notes: string[];
}

// ---------------------------------------------------------------------------
// 设备型号库（v0.7）
// ---------------------------------------------------------------------------

/** 一个端口组：一组同规格的物理口。 */
export interface ModelBay {
  group: string;
  count: number;
  nominalMbps: number;
  speeds: number[];
  role: string;
  label?: string;
}

export interface ModelEntry {
  model: string;
  category: string;
  totalPorts: number;
  /** builtin / json / excel —— 这个型号的规格是谁提供的。 */
  source: string;
  sourceDetail: string;
  sourceText: string;
  /** 仅 `ports=true` 时返回。 */
  bays?: ModelBay[];
}

export interface ModelDiff {
  added: string[];
  removed: string[];
  changed: string[];
  isEmpty: boolean;
  describe: string;
}

export interface ModelsPayload {
  project: string;
  models: ModelEntry[];
  total: number;
  totalPorts: number;
  bySource: Record<string, number>;
  warnings: string[];
}

export interface ModelImportResult extends ModelsPayload {
  diff: ModelDiff;
}

export interface ModelExportResult {
  project: string;
  doc: { schema: number; models: unknown[] };
}
