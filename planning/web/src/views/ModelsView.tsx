/**
 * 设备型号库页签（v0.7）。
 *
 * ## 这页回答的问题
 *
 * 「业务核心交换机 到底是 156 个口还是 108 个？」「我加的国产交换机
 * 为什么台账里显示端口未知？」—— 端口模板从代码里搬出来之后，
 * 用户自己就能答，不用等我们发版。
 *
 * ## 为什么「出处」必须显示
 *
 * 型号可能来自内置表、项目 `models.json`、或 Excel 导入。
 * 用户改了半天 Excel 发现数字没动，第一反应是程序坏了。
 * 把「builtin / json / excel」写在每一行旁边，他一眼就能看到
 * 「这个型号还是内置的，我的改动没生效」—— 只显示端口数做不到这件事，
 * 因为内置和覆盖后的数字看起来都合理。
 *
 * ## 为什么编辑用文本框而不是表格编辑器
 *
 * 型号库的结构是「型号 → 若干端口组」，组里有嵌套的 indices/speeds 数组。
 * 做成 AG Grid 那样的表格编辑器，光列定义就比这里所有代码加起来长，
 * 而用户实际操作是「从别处拷一段 JSON 过来改两个字」。
 * 直接给 JSON 文本框 + 校验反馈，是最短路径。
 */

import type { JSX } from 'react';
import { useCallback, useEffect, useMemo, useState } from 'react';

import type { ModelEntry, ModelsPayload } from '../api/client';
import { api } from '../api/client';

const CATEGORY_LABEL: Record<string, string> = {
  core: '核心',
  switch: '交换机',
  server: '服务器',
  storage: '存储',
  security: '安全',
  other: '其他',
};

const SOURCE_LABEL: Record<string, string> = {
  builtin: '内置',
  json: '项目库',
  excel: 'Excel',
};

/** 给标准 JSON 加要点：用户在 textarea 里手改，格式错也得能看出错在哪。 */
function pretty(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

export function ModelsView(): JSX.Element {
  const [data, setData] = useState<ModelsPayload | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [filter, setFilter] = useState('');
  const [sourceFilter, setSourceFilter] = useState('');
  const [draft, setDraft] = useState('');
  const [showEditor, setShowEditor] = useState(false);
  const [note, setNote] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const next = await api.models({ ports: true });
      setData(next);
      setErr(null);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openEditor = useCallback(async () => {
    try {
      const res = await api.modelsExport(data?.project ?? '');
      setDraft(pretty(res.doc));
      setShowEditor(true);
      setNote(null);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, [data?.project]);

  const submit = useCallback(async () => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(draft);
    } catch (e: unknown) {
      setNote(`JSON 格式有误：${e instanceof Error ? e.message : String(e)}`);
      return;
    }
    setBusy(true);
    try {
      const res = await api.modelsImport(data?.project ?? '', parsed);
      setData(res);
      setNote(res.diff.describe);
      setErr(null);
      setShowEditor(false);
    } catch (e: unknown) {
      setNote(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [draft, data?.project]);

  const rows = useMemo(() => {
    if (!data) return [];
    const kw = filter.trim();
    return data.models.filter((m) => {
      if (sourceFilter && m.source !== sourceFilter) return false;
      if (!kw) return true;
      return m.model.includes(kw) || m.category.includes(kw);
    });
  }, [data, filter, sourceFilter]);

  if (err) {
    return (
      <div className="models-view">
        <div className="error-box">读取型号库失败：{err}</div>
      </div>
    );
  }
  if (!data) {
    return <div className="models-view loading">正在读取型号库…</div>;
  }

  return (
    <div className="models-view">
      <section className="panel models-hero">
        {/* 必须显示读的是哪个项目：这页默认跟「库里最新的项目」走，
            而用户刚在别的页改的是另一个项目时，他会以为改动丢了。 */}
        <div className="models-project">
          <span className="models-label">当前项目</span>
          <span className="models-project-name">{data.project || '（未指定）'}</span>
        </div>
        <div className="models-stat">
          <span className="models-big">{data.total}</span>
          <span className="models-label">个型号</span>
        </div>
        <div className="models-stat">
          <span className="models-big">{data.totalPorts.toLocaleString('zh-CN')}</span>
          <span className="models-label">个端口</span>
        </div>
        <div className="models-stat">
          <span className="models-big">{data.bySource['builtin'] ?? 0}</span>
          <span className="models-label">内置</span>
        </div>
        <div className="models-stat">
          <span className="models-big">
            {(data.bySource['json'] ?? 0) + (data.bySource['excel'] ?? 0)}
          </span>
          <span className="models-label">本项目改过</span>
        </div>
        <div className="models-actions">
          <button type="button" className="btn-mini" onClick={() => void load()}>
            重新读取
          </button>
          <button type="button" className="btn-mini" onClick={() => void openEditor()}>
            导出并编辑
          </button>
        </div>
      </section>

      {data.warnings.length > 0 && (
        <section className="panel models-warn">
          <h3>加载时有 {data.warnings.length} 条提示</h3>
          <ul>
            {data.warnings.slice(0, 8).map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
          <p className="panel-note">
            坏条目只跳过、不阻断 —— 型号库错一条不该让整个项目打不开。
          </p>
        </section>
      )}

      {note && <div className="models-note">{note}</div>}

      {showEditor && (
        <section className="panel models-editor">
          <h3>编辑型号库</h3>
          <p className="panel-note">
            只列出本项目自己声明的型号 —— 内置型号不导出，否则再导入一次会把
            它们复制成用户数据，从此收不到内置表的更新。要删内置型号，
            加一条 <code>{'{"name": "型号名", "remove": true}'}</code>。
          </p>
          <textarea
            className="models-textarea"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            spellCheck={false}
          />
          <div className="models-editor-bar">
            <button type="button" className="btn" disabled={busy} onClick={() => void submit()}>
              {busy ? '导入中…' : '导入并生效'}
            </button>
            <button
              type="button"
              className="btn-mini"
              disabled={busy}
              onClick={() => setShowEditor(false)}
            >
              取消
            </button>
          </div>
        </section>
      )}

      <section className="panel">
        <div className="models-filter">
          <input
            className="models-search"
            placeholder="按型号名或类别筛选…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <div className="models-source-tabs">
            {['', 'builtin', 'json', 'excel'].map((k) => (
              <button
                key={k || 'all'}
                type="button"
                className={`btn-mini${sourceFilter === k ? ' active' : ''}`}
                onClick={() => setSourceFilter(k)}
              >
                {k === '' ? '全部' : SOURCE_LABEL[k]}
              </button>
            ))}
          </div>
          <span className="models-count">
            {rows.length} / {data.total} 行
          </span>
        </div>
        <table className="models-table">
          <thead>
            <tr>
              <th>型号</th>
              <th>类别</th>
              <th className="num">端口</th>
              <th>分组</th>
              <th>出处</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((m: ModelEntry) => (
              <tr key={m.model} className={m.source === 'builtin' ? '' : 'custom'}>
                <td className="model-name">{m.model}</td>
                <td>{CATEGORY_LABEL[m.category] ?? m.category}</td>
                <td className="num">{m.totalPorts}</td>
                <td className="model-groups">
                  {(m.bays ?? []).map((b) => `${b.group}×${b.count}`).join(' · ')}
                </td>
                <td>
                  <span className={`source-tag src-${m.source}`}>
                    {SOURCE_LABEL[m.source] ?? m.source}
                  </span>
                  <span className="source-detail">{m.sourceDetail}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length === 0 && <div className="empty-box">没有匹配的型号。</div>}
      </section>
    </div>
  );
}
