/**
 * AG Grid 布线表（FR-4.x）。5127 行虚拟滚动 + 服务端分页。
 *
 * ## 为什么分页而不是一次传 5127 行
 *
 * 后端已经实现了服务端过滤，每页 200 行约 300KB。如果一次传全量
 * JSON（估计 8MB），用户点"只看通道 2"要等前端筛完，慢网络下会卡住。
 * 分页的代价是每次筛选一个 HTTP 请求 —— 本机部署下是 20ms 级。
 *
 * ## 为什么列定义从后端下发
 *
 * `CablesPayload.columns` 来自后端 `CABLE_GRID_COLUMNS`，
 * 表头文案与导出 Excel 的 25 列契约保��同源。把列定义写死在前端
 * 等于把契约复制一份，改一处忘一处。
 *
 * ## 无需 valueGetter：所有值都由后端算好
 *
 * Community 版没有 Calculated Columns（Enterprise 功能），但这里不需要
 * —— 长度链 N/O/P/Q/R/S/T 全部由 Python 域层算完下发，前端只做展示。
 * 这与"公式只在域层活一次"的铁律一致：前端不参与任何长度计算。
 */

import { AgGridReact } from 'ag-grid-react';
import type { ColDef, ICellRendererParams, RowClickedEvent } from 'ag-grid-community';
import { useCallback, useMemo, useRef } from 'react';
import type { JSX } from 'react';

import type { CableRow } from '../api/client';
import { PAGE_SIZE, useAppStore } from '../store/appStore';

/** 空值渲染。源表大量单元格为空（88% 的行没填 N/O），显示 — 比空白更清楚。 */
function Emptyable(params: ICellRendererParams<CableRow>): JSX.Element {
  const v = params.value;
  if (v === null || v === undefined || v === '') return <span className="cell-empty">—</span>;
  return <span>{String(v)}</span>;
}

/** 长度单元格：T 与 U 不一致时标点（实测 275/627 行存在人工调整）。 */
function LengthCell(params: ICellRendererParams<CableRow>): JSX.Element {
  const v = params.value;
  if (v === null || v === undefined || v === '') return <span className="cell-empty">—</span>;
  return <span>{String(v)}</span>;
}

/** 计算长度 T：与核定长度 U 不同则加印记，让"人工改过"一眼可见。 */
function TLadderCell(params: ICellRendererParams<CableRow>): JSX.Element {
  const row = params.data;
  if (!row) return <span className="cell-empty">—</span>;
  const t = typeof row.tLadder === 'number' ? row.tLadder : null;
  const u = typeof row.uApproved === 'number' ? row.uApproved : null;
  const drift = t !== null && u !== null && t !== u;
  const over = row.finalLength === null && t !== null;
  return (
    <span
      className={drift ? 'len-drifted' : undefined}
      title={drift ? `计算 ${t}，人工核定为 ${u}` : over ? '超出阶梯末档，需人工定档' : undefined}
    >
      {String(t ?? '—')}
      {drift && <i className="drift-dot" />}
    </span>
  );
}

/** 置信度：用户不需要理解 A/B/C 空间类，只需要知道该不该信这个值。 */
function ConfidenceCell(params: ICellRendererParams<CableRow>): JSX.Element | null {
  const row = params.data;
  if (!row) return null;
  const level = row.confidence;
  const map: Record<string, { text: string; cls: string; tip: string }> = {
    A: { text: '高', cls: 'conf-high', tip: '同排面对面 · 几何公式 93.1% 吻合' },
    B: { text: '极高', cls: 'conf-high', tip: '同通道跨侧 · 排端走线架公式 98.0% 吻合' },
    C: { text: '中', cls: 'conf-mid', tip: '跨通道 · 60.0% 吻合，建议人工复核' },
    HIGH: { text: '高', cls: 'conf-high', tip: '经验表样本充足且纯度高' },
    MEDIUM: { text: '中', cls: 'conf-mid', tip: '经验表命中但样本或纯度一般' },
    LOW: { text: '低', cls: 'conf-low', tip: '仅供参考，不进自动耗材汇总' },
    NA: { text: '无', cls: 'conf-low', tip: '无建议，必须人工填' },
  };
  const m = level ? map[level] : null;
  if (!m) return <span className="cell-empty">—</span>;
  return (
    <span className={`badge ${m.cls}`} title={m.tip}>
      {m.text}
    </span>
  );
}

/** 策略列：GEOMETRIC / EMPIRICAL / MANUAL / IMPORTED 的中文名。 */
const POLICY_LABEL: Record<string, string> = {
  GEOMETRIC: '公式',
  EMPIRICAL: '经验表',
  MANUAL: '人工',
  IMPORTED: '导入原值',
};

function PolicyCell(params: ICellRendererParams<CableRow>): JSX.Element {
  const v = params.value as string | null;
  if (!v) return <span className="cell-empty">—</span>;
  return <span className={`policy-tag policy-${v.toLowerCase()}`}>{POLICY_LABEL[v] ?? v}</span>;
}

export function CablesView(): JSX.Element {
  const cables = useAppStore((s) => s.cables);
  const filter = useAppStore((s) => s.filter);
  const cablePage = useAppStore((s) => s.cablePage);
  const loading = useAppStore((s) => s.loading);
  const selection = useAppStore((s) => s.selection);
  const setFilter = useAppStore((s) => s.setFilter);
  const setCablePage = useAppStore((s) => s.setCablePage);
  const selectDevice = useAppStore((s) => s.selectDevice);
  const gridRef = useRef<AgGridReact<CableRow> | null>(null);

  /**
   * 后端列定义 → AG Grid colDef。
   *
   * 后端已经用 CableRow 的字段名做 field（不是 contract25 的 snake_case），
   * 所以这里只需要按 field 挂对应的 cellRenderer。
   */
  const columnDefs = useMemo<ColDef<CableRow>[]>(() => {
    if (!cables) return [];
    return cables.columns.map((c) => {
      const def: ColDef<CableRow> = {
        headerName: c.header,
        colId: c.field,
        field: c.field as keyof CableRow & string,
        width: c.width,
        resizable: true,
        sortable: true,
      };
      // exactOptionalPropertyTypes 下不能显式赋 undefined，必须条件写入
      if (c.pinned === 'left') def.pinned = 'left';
      switch (c.field) {
        case 'localU':
        case 'peerU':
        case 'localModule':
        case 'peerModule':
        case 'nCross':
        case 'oGap':
        case 'pCount':
        case 'qInRack':
        case 'rInter':
        case 'sTotal':
        case 'uApproved':
          def.cellRenderer = Emptyable;
          break;
        case 'tLadder':
          def.cellRenderer = TLadderCell;
          break;
        case 'finalLength':
          def.cellRenderer = LengthCell;
          break;
        case 'confidence':
          def.cellRenderer = ConfidenceCell;
          break;
        case 'policy':
          def.cellRenderer = PolicyCell;
          break;
        default:
          break;
      }
      // 设备名列可点：点了就在平面图/U位图上定位它
      if (c.field === 'localDevice' || c.field === 'peerDevice') {
        def.cellClass = 'link-cell';
      }
      return def;
    });
  }, [cables]);

  const onRowClicked = useCallback(
    (e: RowClickedEvent<CableRow>) => {
      if (e.data) {
        void selectDevice(e.data.localChannel, e.data.localRackNo, e.data.localDeviceName);
      }
    },
    [selectDevice],
  );

  const exportCsv = useCallback(() => {
    gridRef.current?.api?.exportDataAsCsv({
      fileName: `布线表_第${cablePage + 1}页.csv`,
      columnSeparator: ',',
    });
  }, [cablePage]);

  if (!cables) {
    return <div className="view-placeholder">{loading ? '布线表加载中…' : '布线表未加载'}</div>;
  }

  const pageCount = Math.max(1, Math.ceil(cables.total / PAGE_SIZE));
  const filtered = Boolean(filter.channel || filter.rack || filter.cableType || filter.text);

  return (
    <div className="cables-view">
      <div className="cables-toolbar">
        <label>
          通道
          <select
            value={filter.channel ?? ''}
            onChange={(e) =>
              void setFilter({ channel: e.target.value ? Number(e.target.value) : null })
            }
          >
            <option value="">全部</option>
            {cables.filterOptions.channels.map((ch) => (
              <option key={ch} value={ch}>
                {ch}
              </option>
            ))}
          </select>
        </label>
        <label>
          线缆
          <select
            value={filter.cableType ?? ''}
            onChange={(e) => void setFilter({ cableType: e.target.value || null })}
          >
            <option value="">全部</option>
            {cables.filterOptions.cableTypes.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <label className="grow">
          搜索
          <input
            type="search"
            placeholder="设备名 / 端口 / 备注…"
            value={filter.text}
            onChange={(e) => void setFilter({ text: e.target.value })}
          />
        </label>
        {filtered && (
          <button
            type="button"
            onClick={() => void setFilter({ channel: null, rack: null, cableType: null, text: '' })}
          >
            清空筛选
          </button>
        )}
        <button type="button" onClick={exportCsv} title="导出当前页为 CSV（AG Grid Community 内置）">
          导出本页 CSV
        </button>
        <span className="toolbar-stat">
          共 <b>{cables.total}</b> 行
          {filtered && <span className="subtle">（已筛选）</span>}
        </span>
      </div>

      <div className="ag-theme-quartz cables-grid">
        <AgGridReact<CableRow>
          ref={gridRef}
          // v33+ 默认 Theming API 与 main.tsx 的 legacy CSS 主题二选一；
          // 项目自定义了 .ag-* 变量，显式走 legacy，消除 error #239。
          theme="legacy"
          rowData={cables.rows}
          columnDefs={columnDefs}
          getRowId={(p) => p.data.stableKey}
          rowHeight={28}
          headerHeight={32}
          enableCellTextSelection
          suppressScrollOnNewData
          onRowClicked={onRowClicked}
          getRowClass={(p) => {
            if (!selection.deviceName || !p.data) return undefined;
            const hit =
              p.data.localDeviceName === selection.deviceName ||
              p.data.peerDeviceName === selection.deviceName;
            return hit ? 'row-highlight' : undefined;
          }}
        />
      </div>

      <div className="pager">
        <button
          type="button"
          disabled={cablePage <= 0 || loading}
          onClick={() => void setCablePage(cablePage - 1)}
        >
          上一页
        </button>
        <span>
          第 {cablePage + 1} / {pageCount} 页 · 每页 {PAGE_SIZE} 行
        </span>
        <button
          type="button"
          disabled={cablePage + 1 >= pageCount || loading}
          onClick={() => void setCablePage(cablePage + 1)}
        >
          下一页
        </button>
      </div>
    </div>
  );
}
