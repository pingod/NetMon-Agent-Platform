/**
 * 意图编辑器 —— 六段表格的页面内编辑（FR-3.5）。
 *
 * ## 为什么这个组件不自己算任何东西
 *
 * 单元格类型转换（`"3"` → `3`）、必填校验、跨段引用检查全部走后端。
 * 前端也实现一套的话，两套规则迟早不一致 —— 而现在不一致的后果是
 * 「界面说没问题，保存时 422」。这一条在 v0.2 的比对基准 bug 里验证过：
 * 三个 bug 全部源于「两个地方各算一次同样的东西」。
 *
 * 所以这里的每一格编辑都是一次 `POST /api/intent/row`，
 * 返回的是**后端改完的完整文档 + 后端重算的校验结果**。前端只负责摆出来。
 *
 * ## 为什么 Edit 与 Save 分开
 *
 * 编辑走 `/api/intent/row`（不落库），保存走 `PUT /api/projects/{name}/intent`
 * （建版本）。这样用户能「改十次存一次」，而不是「改十次留十个版本」。
 * 版本历史是给人看的审计记录，不该被编辑过程中的中间态污染。
 *
 * ## 为什么表格不用 AG Grid
 *
 * 这六段每段列不同、行数最多 697 行、且需要行内错误标记。
 * AG Grid 的虚拟滚动与列冻结在这里是过量能力（697 行 DOM 直接渲染约 8ms），
 * 而它带来的 columnDef 动态改造 + cellRenderer 注册成本反而更高。
 * 用原生 `<table>` + 受控 `<input>`，行内错误用 `<tr>` 的 className 表达，
 * 一眼能看完 —— 布线表那 5127 行才需要 AG Grid。
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { JSX } from 'react';

import { parseHash } from '../store/appStore';
import { fillBand, inSel, normSel, parseSelParam, selCount } from '../store/selMath';
import type { Sel } from '../store/selMath';
import type {
  IntentCheck,
  IntentCopyResult,
  IntentFillResult,
  IntentIssue,
  IntentRowResult,
} from '../api/client';
import { api, type IntentPasteResult } from '../api/client';

/** 六段的列定义。`key` 必须与后端 `intentcheck.COLUMN_TYPES` 一致。 */
interface ColumnDef {
  key: string;
  label: string;
  width?: string;
  hint?: string;
}

interface SectionDef {
  key: string;
  label: string;
  desc: string;
  columns: ColumnDef[];
}

const SECTIONS: SectionDef[] = [
  {
    key: 'racks',
    label: '布局',
    desc: '一个机柜段一行：某通道的某几台机柜装哪种服务器。生成器按这里的段去算「哪台交换机服务哪些节点」。',
    columns: [
      { key: 'channel', label: '通道', width: '64px' },
      { key: 'from', label: '起始柜', width: '72px' },
      { key: 'to', label: '结束柜', width: '72px' },
      { key: 'model', label: '服务器型号' },
    ],
  },
  {
    key: 'switches',
    label: '交换机',
    desc: '一台交换机一行：实例名 + 型号 + 装在哪。实例名是交换机组与显式连线引用它的唯一凭据，不能重名。',
    columns: [
      { key: 'instance', label: '实例名' },
      { key: 'model', label: '型号' },
      { key: 'channel', label: '通道', width: '64px' },
      { key: 'rack', label: '机柜', width: '64px' },
      { key: 'u', label: 'U位', width: '64px' },
      { key: 'uSpan', label: '占U', width: '56px', hint: '默认 1；3-12U 这种跨 10U 的填 10' },
    ],
  },
  {
    key: 'devices',
    label: '特殊设备',
    desc: '有名字的非交换机设备（防火墙、负载均衡、网关…）。它们不进交换机组，只出现在显式连线里。',
    columns: [
      { key: 'instance', label: '实例名' },
      { key: 'model', label: '型号' },
      { key: 'channel', label: '通道', width: '64px' },
      { key: 'rack', label: '机柜', width: '64px' },
      { key: 'u', label: 'U位', width: '64px' },
      { key: 'uSpan', label: '占U', width: '56px' },
    ],
  },
  {
    key: 'routes',
    label: '路由',
    desc: '一条规则一行：「某种服务器的某个网口，接某种型号的交换机」。这是把 550 台服务器接进 26 条交换机组的前提。',
    columns: [
      { key: 'nodeModel', label: '服务器型号' },
      { key: 'nic', label: '网口', width: '96px', hint: '如 万兆1 / 千兆1 / IPMI / 25G1' },
      { key: 'switchModel', label: '接哪种交换机' },
    ],
  },
  {
    key: 'chains',
    label: '交换机组',
    desc: '一条链一行：同型号的若干台交换机，按顺序以某个网口下联一批服务器。顺序即接线顺序（第一个装满再装第二个）。',
    columns: [
      { key: 'channel', label: '通道', width: '64px' },
      { key: 'switchModel', label: '交换机型号' },
      { key: 'nic', label: '网口', width: '96px' },
      { key: 'instances', label: '交换机实例（分号分隔）', hint: '顺序即接线顺序' },
      { key: 'capacity', label: '每台端口数', width: '96px', hint: '留空用默认 44' },
      { key: 'order', label: '排序轴', width: '104px', hint: 'rack_major（默认）/ slot_major' },
    ],
  },
  {
    key: 'explicitLinks',
    label: '显式连线',
    desc: '规则推不出来的那一根一根写：交换机互联、特殊机型接线。实测 697 行，占全量 13.6%。',
    columns: [
      { key: 'a', label: 'A 端（设备 / 端口）' },
      { key: 'b', label: 'B 端（设备 / 端口）' },
      { key: 'cableType', label: '线缆类型', width: '120px', hint: '留空由端口速率推导' },
    ],
  },
];

const SEVERITY_ORDER: Record<string, number> = { error: 0, warn: 1, info: 2 };

function severityOf(issue: IntentIssue): string {
  return issue.severity;
}

/** 一行的最坏严重度 —— 用来给 `<tr>` 上色。 */
function worstOf(issues: IntentIssue[]): string | null {
  if (issues.length === 0) return null;
  const sorted = [...issues].sort((a, b) => severityRank(a) - severityRank(b));
  const first = sorted[0];
  return first ? first.severity : null;
}

/** `SEVERITY_ORDER[x]` 在 `noUncheckedIndexedAccess` 下是 `number | undefined`，
 *  用函数包一层才能既保持表驱动又让类型收窄。 */
function severityRank(issue: IntentIssue): number {
  return SEVERITY_ORDER[issue.severity] ?? 9;
}

function CellInput({
  value,
  onCommit,
  disabled,
}: {
  value: unknown;
  onCommit: (next: string) => void;
  disabled: boolean;
}): JSX.Element {
  const text = value === null || value === undefined ? '' : String(value);
  const [draft, setDraft] = useState(text);

  // 文档被后端改过（例如整体重载）时要跟上；否则输入框会一直显示旧值。
  // 只在外部值与内部草稿不一致且草稿未被编辑过时才同步 —— 简化做法：
  // 用 key 让 React 重建输入框。见父组件的 `key={...}`。
  return (
    <input
      className="cell-input"
      value={draft}
      disabled={disabled}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => {
        if (draft !== text) onCommit(draft);
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.currentTarget.blur();
        } else if (e.key === 'Escape') {
          setDraft(text);
          e.currentTarget.blur();
        }
      }}
    />
  );
}

/** 显式连线的端点：`["设备名", "端口名"]` → 两个小输入框。 */
function EndpointInput({
  pair,
  onCommit,
  disabled,
}: {
  pair: unknown;
  onCommit: (next: unknown) => void;
  disabled: boolean;
}): JSX.Element {
  const arr = Array.isArray(pair) ? pair : ['', ''];
  const dev = String(arr[0] ?? '');
  const port = String(arr[1] ?? '');
  const [d, setD] = useState(dev);
  const [p, setP] = useState(port);
  return (
    <span className="endpoint-pair">
      <input
        className="cell-input endpoint-dev"
        value={d}
        disabled={disabled}
        placeholder="设备名"
        onChange={(e) => setD(e.target.value)}
        onBlur={() => {
          if (d !== dev) onCommit([d, p]);
        }}
      />
      <input
        className="cell-input endpoint-port"
        value={p}
        disabled={disabled}
        placeholder="端口"
        onChange={(e) => setP(e.target.value)}
        onBlur={() => {
          if (p !== port) onCommit([d, p]);
        }}
      />
    </span>
  );
}

/** 交换机组实例列表：数组 → 分号分隔的单个输入框。 */
function InstancesInput({
  list,
  onCommit,
  disabled,
}: {
  list: unknown;
  onCommit: (next: unknown) => void;
  disabled: boolean;
}): JSX.Element {
  const arr = Array.isArray(list) ? list.map(String) : [];
  const text = arr.join('; ');
  const [draft, setDraft] = useState(text);
  return (
    <input
      className="cell-input"
      value={draft}
      disabled={disabled}
      placeholder="实例1; 实例2; 实例3"
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => {
        if (draft === text) return;
        const next = draft
          .split(/[;；,，]/)
          .map((s) => s.trim())
          .filter(Boolean);
        onCommit(next);
      }}
    />
  );
}

export interface IntentEditorProps {
  /** 当前草稿文档（由父组件持有）。 */
  doc: Record<string, unknown>;
  /** 校验结果（由父组件每次改完更新）。 */
  check: IntentCheck | null;
  /** 每次成功改一格后回调：把后端返回的新文档 + 新校验交给父组件。 */
  onChanged: (result: IntentRowResult) => void;
  /** 保存为一个新版本。 */
  onSave: (note: string) => Promise<void> | void;
  /** 是否正在保存。 */
  saving?: boolean;
  /** 只读（例如浏览历史版本时）。 */
  readOnly?: boolean;
}

export function IntentEditor({
  doc,
  check,
  onChanged,
  onSave,
  saving = false,
  readOnly = false,
}: IntentEditorProps): JSX.Element {
  const [section, setSection] = useState<string>('switches');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  // ── 粘贴（v0.4 #3 Excel 化交互层）──
  //
  // `pastedRef` 而不是 state：`paste` 事件里读到的文本必须立刻拿到，
  // 而 state 更新是异步的、`onPaste` 里读 `state` 拿到的是旧值。
  const pastedRef = useRef('');
  const [pasteAt, setPasteAt] = useState<number | null>(null);
  const [pasteNote, setPasteNote] = useState<string | null>(null);

  // ── 选区 / 复制出 / 填充柄（v0.4 #3 的三件）──
  //
  // 选区状态放这里而不是子组件：复制、填充、高亮三处都要读它，
  // 提到组件外就得靠回调层层传，且三处容易算出不一致的范围。
  // 支持 `#design?edit=1&sel=1-3-1-3` 深链预置选区。
  //
  // 为什么需要：选区、填充柄、复制按钮只有在「有选区」时才渲染，
  // 而 headless 截图没法真的用鼠标拖 —— 没有这条路，界面的这部分
  // 就永远只能靠读代码判断对不对。与 `?edit=1` 出自同一考虑。
  const [sel, setSel] = useState<Sel | null>(() => {
    if (typeof window === 'undefined') return null;
    return parseSelParam(parseHash(window.location.hash).params.sel);
  });
  // 起点单独存 ref：拖动过程中 `sel` 每次都在变，若拿 `sel` 当锚点，
  // 反向拖动时锚点会跟着漂，选出来的矩形会缩成一个点。
  const anchorRef = useRef<{ r: number; c: number } | null>(null);
  const draggingRef = useRef(false);
  // 填充柄拖到的行号。null = 没在拖。
  const [fillTo, setFillTo] = useState<number | null>(null);
  const fillingRef = useRef(false);

  // `SECTIONS` 非空且 `?? SECTIONS[0]` 兜底，但 `noUncheckedIndexedAccess`
  // 下 `SECTIONS[0]` 仍是 `SectionDef | undefined`。用 `!` 会掩盖真实问题，
  // 这里的做法是把兜底也写成确定值。
  const current: SectionDef =
    SECTIONS.find((s) => s.key === section) ??
    ({ key: 'switches', label: '交换机', desc: '', columns: [] } satisfies SectionDef);
  const rows = useMemo(() => {
    const raw = doc[section];
    return Array.isArray(raw) ? (raw as Record<string, unknown>[]) : [];
  }, [doc, section]);

  const issuesByRow = useMemo(() => {
    const map = new Map<number, IntentIssue[]>();
    if (!check) return map;
    for (const issue of check.issues) {
      if (issue.section !== section || issue.rowNo === null) continue;
      const list = map.get(issue.rowNo) ?? [];
      list.push(issue);
      map.set(issue.rowNo, list);
    }
    return map;
  }, [check, section]);

  const sectionIssues = useMemo(() => {
    if (!check) return [];
    return check.issues.filter((x) => x.section === section);
  }, [check, section]);

  const run = useCallback(
    async (fn: () => Promise<IntentRowResult>) => {
      setBusy(true);
      setErr(null);
      try {
        const result = await fn();
        if (!result.ok && result.error) setErr(result.error);
        onChanged(result);
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      } finally {
        setBusy(false);
      }
    },
    [onChanged],
  );

  const commitCell = useCallback(
    (rowNo: number, column: string, value: unknown) => {
      void run(() =>
        api.intentRow({ doc, section, op: 'cell', rowNo, column, value }),
      );
    },
    [doc, section, run],
  );

  const addRow = useCallback(() => {
    void run(() => api.intentRow({ doc, section, op: 'append' }));
  }, [doc, section, run]);

  const removeRow = useCallback(
    (rowNo: number) => {
      void run(() => api.intentRow({ doc, section, op: 'delete', rowNo }));
    },
    [doc, section, run],
  );

  /**
   * 从剪贴板粘贴一块数据。
   *
   * ## 为什么整块交给服务端解析
   *
   * Excel 的剪贴板格式有一堆边角：`\r\n`、引号包住含换行的单元格、
   * 末尾多一个换行、短行缺列。这些规则如果前后端各写一份，
   * 迟早出现「界面上看着粘对了、保存时错位」。所以前端只把
   * `text/plain` 原样送上去，切分与类型转换都在后端。
   *
   * ## 为什么提示要与「写了几行」分开显示
   *
   * `written=8, notes=["第 3 行 u 需要整数"]` 的真实含义是
   * 「8 行都写了，但其中 1 格没接受」。只显示「成功 8 行」会让用户
   * 以为全对了；只显示错误又让他以为整块没写进去。
   */
  const pasteBlock = useCallback(
    (startRow: number | null) => {
      const text = pastedRef.current;
      pastedRef.current = '';
      if (!text) return;
      setBusy(true);
      setErr(null);
      setPasteNote(null);
      api
        .intentPaste({
          doc,
          section,
          text,
          ...(startRow === null ? {} : { startRow }),
        })
        .then((result: IntentPasteResult) => {
          if (!result.ok && result.error) setErr(result.error);
          const bits: string[] = [];
          if (result.written > 0) bits.push(`写入 ${result.written} 行`);
          if (result.notes.length) bits.push(...result.notes);
          setPasteNote(bits.join(' · ') || null);
          onChanged({ ok: result.ok, error: result.error, rowNo: 0, doc: result.doc, check: result.check });
        })
        .catch((e: unknown) => setErr(e instanceof Error ? e.message : String(e)))
        .finally(() => setBusy(false));
    },
    [doc, section, onChanged],
  );

  /**
   * 复制选区到系统剪贴板（v0.4 #3 的「复制出」）。
   *
   * ## 为什么文本由后端生成
   *
   * Excel 的 TSV 转义规则（`\r\n` 连行、`\t` 分列、含制表符或换行的
   * 单元格要用双引号包住并把内部引号翻倍）只该有一份实现。前端拼一份、
   * 后端拼一份，迟早出现「复制出去粘回 Excel 少一列」。后端那份与
   * `parse_clipboard` 是对称的一对，往返恒等有单测守着。
   *
   * ## 为什么走 navigator.clipboard 而不是 document.execCommand
   *
   * `execCommand('copy')` 已废弃，且需要先选中真实 DOM 文本 ——
   * 表格里是一个个受控 input，选中它们会把用户的编辑状态打乱。
   * `clipboard.writeText` 需要 HTTPS 或 localhost；本工具就是单机
   * localhost 部署（`scripts/run.sh`），不满足时下面会给出明确提示
   * 而不是静默失败。
   */
  const copySelection = useCallback(() => {
    if (!sel) return;
    setErr(null);
    setPasteNote(null);
    api
      .intentCopy({
        doc,
        section,
        rowStart: sel.r0 + 1,
        rowEnd: sel.r1 + 1,
        colStart: sel.c0 + 1,
        colEnd: sel.c1 + 1,
      })
      .then(async (result: IntentCopyResult) => {
        if (!navigator.clipboard) {
          setErr('这个浏览器不允许脚本写剪贴板（需要 https 或 localhost）。');
          return;
        }
        try {
          await navigator.clipboard.writeText(result.text);
          setPasteNote(`已复制 ${result.range}，共 ${result.cellCount} 格`);
        } catch (e: unknown) {
          setErr(`写剪贴板被拒：${e instanceof Error ? e.message : String(e)}`);
        }
      })
      .catch((e: unknown) => setErr(e instanceof Error ? e.message : String(e)));
  }, [sel, doc, section]);

  /**
   * 把选区往下填到 `targetRow`（v0.4 #3 的「填充柄」）。
   *
   * 规律推算在后端：数字等差、中文/英文序列（周一…周日、MON…SUN）、
   * 否则照抄。放后端同样是为了只有一份实现 —— 且它要能单测。
   *
   * 前端不做「先本地算出结果再发」的乐观更新：填充会改多行，
   * 若规律算得不一样，界面会先闪一个错的结果再被覆盖，
   * 用户看到的是「闪了一下就变了」，反而更不放心。
   */
  const fillSelection = useCallback(
    (targetRow: number) => {
      if (!sel) return;
      setBusy(true);
      setErr(null);
      setPasteNote(null);
      api
        .intentFill({
          doc,
          section,
          rowStart: sel.r0 + 1,
          rowEnd: sel.r1 + 1,
          colStart: sel.c0 + 1,
          colEnd: sel.c1 + 1,
          targetRow: targetRow + 1,
        })
        .then((result: IntentFillResult) => {
          if (!result.ok && result.error) setErr(result.error);
          const bits: string[] = [];
          if (result.written > 0) bits.push(`${result.pattern} · 填了 ${result.written} 行`);
          if (result.notes.length) bits.push(...result.notes);
          setPasteNote(bits.join(' · ') || null);
          setSel({
            r0: sel.r0,
            r1: targetRow,
            c0: sel.c0,
            c1: sel.c1,
          });
          onChanged({
            ok: result.ok,
            error: result.error,
            rowNo: 0,
            doc: result.doc,
            check: result.check,
          });
        })
        .catch((e: unknown) => setErr(e instanceof Error ? e.message : String(e)))
        .finally(() => {
          setBusy(false);
          setFillTo(null);
        });
    },
    [sel, doc, section, onChanged],
  );

  /**
   * 选区跟随表格切换清空。
   *
   * 不清的后果：在「交换机」里选了第 3~5 行，切到「布局」只有 28 行，
   * 填充柄还挂在一个不存在的行号上，点下去报「越界」——用户以为自己
   * 选错了，其实是上一个页签的残留。
   */
  // 只在**切换段**时清空，挂载时不清。
  //
  // 直接写 `useEffect(..., [section])` 会在挂载后立刻执行一次，把深链
  // 预置的选区（`?sel=1-4-1-4`）当场清掉 —— 这个坑真的踩到了。
  // 用 ref 记住上一次的段名，首次渲染时 `prev === section`，跳过。
  const prevSectionRef = useRef(section);
  useEffect(() => {
    if (prevSectionRef.current === section) return;
    prevSectionRef.current = section;
    setSel(null);
    setFillTo(null);
    anchorRef.current = null;
  }, [section]);

  const disabled = readOnly || busy || saving;

  return (
    <section className="panel intent-editor">
      <header className="editor-head">
        <h3>
          意图编辑器
          <span className="subtle">
            （改格子即时校验，保存在「版本」里；编辑中的改动不会产生版本）
          </span>
        </h3>
        <div className="editor-actions">
          <input
            className="note-input"
            value={note}
            placeholder="这次保存的说明（如：ch9 加一台交换机）"
            onChange={(e) => setNote(e.target.value)}
            disabled={readOnly || saving}
          />
          <button
            className="btn-primary"
            disabled={readOnly || saving || (check ? !check.ok : true)}
            onClick={() => void onSave(note)}
            title={check && !check.ok ? `还有 ${check.errorCount} 个错误未修正` : undefined}
          >
            {saving ? '保存中…' : '保存为新版本'}
          </button>
        </div>
      </header>

      {check && (
        <div className={`check-bar ${check.ok ? 'ok' : 'bad'}`}>
          {check.ok ? (
            <span>
              ✅ 可生成 · 警告 {check.warnCount} · 提示 {check.infoCount}
            </span>
          ) : (
            <span>
              ❌ 还有 <b>{check.errorCount}</b> 个错误必须修正 · 警告 {check.warnCount} ·
              提示 {check.infoCount}
            </span>
          )}
        </div>
      )}

      {err && <div className="editor-error">改不动：{err}</div>}

      <nav className="section-tabs">
        {SECTIONS.map((s) => {
          const n = Array.isArray(doc[s.key]) ? (doc[s.key] as unknown[]).length : 0;
          const bad = check
            ? check.issues.filter((x) => x.section === s.key && x.severity === 'error').length
            : 0;
          return (
            <button
              key={s.key}
              className={`section-tab ${s.key === section ? 'active' : ''}`}
              onClick={() => setSection(s.key)}
            >
              {s.label}
              <span className="tab-count">{n}</span>
              {bad > 0 && <span className="tab-bad">{bad}</span>}
            </button>
          );
        })}
      </nav>

      <p className="section-desc">
        {current.desc}
        {sel && (
          <span className="sel-hint">
            已选 {sel.r1 - sel.r0 + 1} × {sel.c1 - sel.c0 + 1}（第 {sel.r0 + 1}~
            {sel.r1 + 1} 行 · 第 {sel.c0 + 1}~{sel.c1 + 1} 列）
            <button className="btn-tiny" onClick={copySelection}>
              复制
            </button>
            <button className="btn-tiny" onClick={() => setSel(null)}>
              取消
            </button>
            <span className="sel-tip">拖右下角的小方块可填充</span>
          </span>
        )}
        <span className="paste-hint">
          {sel !== null ? null : pasteAt === null ? (
            <>
              在 Excel 里框选一块，<b>Ctrl+V</b> 粘进来（追加到末尾）；点左侧行号可指定起点。
            </>
          ) : (
            <>
              粘贴起点：第 <b>{pasteAt + 1}</b> 行（从 Excel 复制后 Ctrl+V；再点一次行号取消）
            </>
          )}
        </span>
      </p>

      {pasteNote && (
        <div className="paste-note">
          {pasteNote}
          <button className="btn-tiny" onClick={() => setPasteNote(null)}>
            知道了
          </button>
        </div>
      )}

      <div
        className="editor-table-wrap"
        onMouseUp={() => {
          // 两种情况在这里收尾：拖选结束、填充柄松手。
          // 判定顺序重要 —— 填充松手时 `draggingRef` 本来就没被置上
          // （柄上的 mousedown 已 stopPropagation），所以不会误清选区。
          if (fillingRef.current) {
            fillingRef.current = false;
            if (fillTo !== null && sel !== null && fillTo > sel.r1) {
              fillSelection(fillTo);
            } else {
              setFillTo(null);
            }
            return;
          }
          draggingRef.current = false;
          anchorRef.current = null;
        }}
        onMouseLeave={() => {
          // 拖到表格外面松手时 `mouseup` 收不到，不兜底的话选区会一直
          // 粘在「拖拽中」状态：之后鼠标随便划过一格都会改动选区。
          if (!draggingRef.current && !fillingRef.current) return;
          if (fillingRef.current) {
            fillingRef.current = false;
            setFillTo(null);
          }
          draggingRef.current = false;
          anchorRef.current = null;
        }}
      >
        <table
          className="editor-table"
          onKeyDown={(e) => {
            // Ctrl/Cmd+C：选区跨多格时接管，单格放行给浏览器。
            //
            // 跨多格必须接管：焦点在某个 input 里，浏览器只会复制那个
            // input 里被选中的文字，而在 Excel 习惯里「选了 4×6 就复制 24 格」。
            // 单格放行：用户双击一个长设备名只想复制这串文字，
            // 此时抢过来会复制到整个格子的值（含不可见前后缀），反而更糟。
            if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 'c') return;
            if (!sel) return;
            if (selCount(sel) < 2) return;
            e.preventDefault();
            copySelection();
          }}
          onPaste={(e) => {
            // 从 Excel 粘贴：只认真实的二维块（含制表符或换行）。
            // 单格粘贴走浏览器默认行为（input 自己的 value 变更），
            // 否则用户在格子里粘一个长设备名会被当成「一行一列」重写整行。
            const text = e.clipboardData.getData('text/plain');
            if (!text) return;
            const hasStructure = text.includes('\t') || text.trimEnd().includes('\n');
            if (!hasStructure) return;
            e.preventDefault();
            pastedRef.current = text;
            pasteBlock(pasteAt);
            setPasteAt(null);
          }}
        >
          <thead>
            <tr>
              <th className="col-no">#</th>
              {current.columns.map((c) => (
                <th key={c.key} style={c.width ? { width: c.width } : undefined}>
                  {c.label}
                  {c.hint && <span className="col-hint">{c.hint}</span>}
                </th>
              ))}
              <th className="col-act" />
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => {
              const cellIssues = issuesByRow.get(i) ?? [];
              const worst = worstOf(cellIssues);
              return (
                <tr
                  key={`${section}-${i}-${worst ?? 'ok'}`}
                  className={worst ? `row-${worst}` : undefined}
                  title={cellIssues.map((x) => x.message).join('\n') || undefined}
                  onMouseEnter={() => {
                    // 在填充柄上按下后，鼠标经过哪一行就预览填到哪一行。
                    // 用 row 级事件而不是每个 td：填的是整块区域，
                    // 用户不太可能精确地把鼠标停在某个格子上。
                    if (fillingRef.current && sel) setFillTo(Math.max(i, sel.r1));
                  }}
                >
                  <td
                    className={`col-no ${pasteAt === i ? 'paste-at' : ''}`}
                    title="点这里设为粘贴原点"
                    onClick={() => setPasteAt(pasteAt === i ? null : i)}
                  >
                    {i + 1}
                  </td>
                  {current.columns.map((c, ci) => {
                    const fieldIssues = cellIssues.filter(
                      (x) => x.field === c.key || x.field.startsWith(`${c.key}[`),
                    );
                    const fieldWorst = worstOf(fieldIssues);
                    const selected = inSel(sel, i, ci);
                    const band = fillBand(sel, fillTo);
                    const inFill = inSel(band, i, ci);
                    const isCorner = sel !== null && i === sel.r1 && ci === sel.c1;
                    const cls = [
                      fieldWorst ? `cell-${fieldWorst}` : '',
                      selected ? 'cell-sel' : '',
                      inFill ? 'cell-fill' : '',
                    ]
                      .filter(Boolean)
                      .join(' ');
                    return (
                      <td
                        key={c.key}
                        className={cls || undefined}
                        onMouseDown={(e) => {
                          // 落点在 input 上时仍然要能开始选区 —— 但不要抢掉
                          // 光标定位：只在左键按下且没按修饰键时记录锚点，
                          // 点击本身照常传给 input。
                          if (e.button !== 0 || disabled) return;
                          anchorRef.current = { r: i, c: ci };
                          draggingRef.current = true;
                          setSel(normSel({ r: i, c: ci }, { r: i, c: ci }));
                          setFillTo(null);
                        }}
                        onMouseEnter={() => {
                          if (!draggingRef.current || !anchorRef.current) return;
                          setSel(normSel(anchorRef.current, { r: i, c: ci }));
                        }}
                      >
                        {c.key === 'a' || c.key === 'b' ? (
                          <EndpointInput
                            pair={row[c.key]}
                            disabled={disabled}
                            onCommit={(next) => commitCell(i, c.key, next)}
                          />
                        ) : c.key === 'instances' ? (
                          <InstancesInput
                            list={row[c.key]}
                            disabled={disabled}
                            onCommit={(next) => commitCell(i, c.key, next)}
                          />
                        ) : (
                          <CellInput
                            value={row[c.key]}
                            disabled={disabled}
                            onCommit={(next) => commitCell(i, c.key, next)}
                          />
                        )}
                        {isCorner && !disabled && (
                          // 填充柄。用 `onMouseDown` + `stopPropagation`：
                          // 不拦的话按在柄上会先把选区重置成一个点，
                          // 后面拖出来的范围就从那一格开始算，填充源凭空少了几列。
                          <span
                            className="fill-handle"
                            title="往下拖，按规律填充"
                            onMouseDown={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              fillingRef.current = true;
                              setFillTo(sel.r1);
                            }}
                            onMouseEnter={undefined}
                          />
                        )}
                      </td>
                    );
                  })}
                  <td className="col-act">
                    <button
                      className="btn-danger btn-tiny"
                      disabled={disabled}
                      onClick={() => removeRow(i)}
                      title="删除这一行"
                    >
                      删
                    </button>
                  </td>
                </tr>
              );
            })}
            {rows.length === 0 && (
              <tr>
                <td className="empty-row" colSpan={current.columns.length + 2}>
                  这一段还没有内容。点下面「加一行」开始。
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="editor-foot">
        <button className="btn-secondary" onClick={addRow} disabled={disabled}>
          + 在「{current.label}」末尾加一行
        </button>
        <span className="subtle">
          共 {rows.length} 行 · 这一段 {sectionIssues.length} 条问题
        </span>
      </div>

      {sectionIssues.length > 0 && (
        <ul className="issue-list">
          {sectionIssues.slice(0, 50).map((x, i) => (
            <li key={`${x.code}-${x.rowNo}-${i}`} className={`issue-${severityOf(x)}`}>
              {x.rowNo !== null && <b>第 {x.rowNo + 1} 行</b>}
              <span className="issue-field">{x.field}</span>
              {x.message}
            </li>
          ))}
          {sectionIssues.length > 50 && (
            <li className="issue-more">还有 {sectionIssues.length - 50} 条，改完上面再看。</li>
          )}
        </ul>
      )}
    </section>
  );
}
