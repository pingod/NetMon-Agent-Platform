/**
 * 项目与版本（v0.4，持久化）。
 *
 * ## 这一页解决的是「改完存哪儿」
 *
 * v0.3 之前，接线意图只能存在 JSON 文件里：改一行要开编辑器、
 * 存了不知道存的是哪版、改错了没法回退。这一页把意图变成
 * **带版本号的项目**：每次保存是一个版本，能列历史、能比差异、能回退。
 *
 * ## 为什么不直接做「意图编辑器」而先做「版本管理」
 *
 * 编辑器改的是**内容**，版本管理管的是**内容的变更历史**。
 * 没有后者，编辑器做得再好也只是把 Excel 换成网页 ——
 * 用户照样不知道「上周那份和现在这份差在哪」。
 * 而且编辑器容易做，版本语义（何时建版本、回滚怎么表达）难，
 * 先把难的定下来，编辑器只是往 `saveProjectIntent` 上接一个表单。
 *
 * ## 版本语义（界面上必须让用户看得见）
 *
 * - 内容没变 → **不建新版本**（连点保存不该刷屏）
 * - 内容变了 → latest + 1
 * - 「留快照」→ 内容没变也建版本（里程碑：审图通过、交档）
 * - 回退 = 把旧内容存成**新版本号**，不删历史（能查到「谁改回去了」）
 */

import { useCallback, useEffect, useState } from 'react';
import type { JSX } from 'react';

import { api } from '../api/client';
import type {
  DiffPayload,
  ProjectSummary,
  VersionInfo,
} from '../api/client';

function fmtTime(iso: string): string {
  if (!iso) return '—';
  // 后端统一存 UTC（`...Z`），这里换成本地时间给人看
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** 一条意图记录的短摘要，与 CLI 的 `_brief` 保持同一套辨识字段。 */
function brief(item: Record<string, unknown>): string {
  const s = (k: string): string => (item[k] == null ? '' : String(item[k]));
  if (s('instance')) {
    const bits = [s('instance')];
    for (const k of ['model', 'channel', 'rack', 'u']) {
      if (s(k)) bits.push(`${k}=${s(k)}`);
    }
    return bits.join(' ');
  }
  if (s('nodeModel')) return `${s('nodeModel')} · ${s('nic')} → ${s('switchModel')}`;
  if (s('aDevice')) return `${s('aDevice')}:${s('aPort')} ↔ ${s('bDevice')}:${s('bPort')}`;
  if (s('switchModel')) {
    const n = Array.isArray(item['instances']) ? (item['instances'] as unknown[]).length : 0;
    return `通道${s('channel')} ${s('switchModel')} · ${s('nic')} · ${n} 台`;
  }
  if (s('channel')) return `通道${s('channel')} 柜${s('from')}..${s('to')} = ${s('model')}`;
  return JSON.stringify(item).slice(0, 90);
}

export function ProjectsView(): JSX.Element {
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [database, setDatabase] = useState<string>('');
  const [selected, setSelected] = useState<string>('');
  const [versions, setVersions] = useState<VersionInfo[]>([]);
  const [diff, setDiff] = useState<DiffPayload | null>(null);
  const [fromV, setFromV] = useState<number>(0);
  const [toV, setToV] = useState<number>(0);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const [newName, setNewName] = useState<string>('');

  const loadProjects = useCallback(async (): Promise<void> => {
    try {
      const data = await api.projects();
      setProjects(data.projects);
      setDatabase(data.database);
      setErr(null);
      // 只在「还没选过」时挑默认值。
      //
      // 依赖里不能放 `selected`：那样每点一个项目都会重新拉一遍列表，
      // 而列表内容根本没变 —— 白白多一次请求，列表还会闪一下。
      setSelected((prev) => (prev ? prev : (data.projects[0]?.name ?? '')));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  const loadVersions = useCallback(async (name: string): Promise<void> => {
    if (!name) {
      setVersions([]);
      return;
    }
    try {
      const data = await api.projectVersions(name);
      setVersions(data.versions);
      setDiff(null);
      setErr(null);
      // 默认比「上一版 vs 最新版」—— 用户最常问的就是这一句
      // 默认比「上一版 → 最新版」：用户问的是「这次改了什么」，
      // 所以 from 必须是旧的那个，方向反了会读成「把改动退回去了？」
      const [newest, second] = data.versions;
      if (newest && second) {
        setFromV(second.version);
        setToV(newest.version);
        // 打开页面就自动给出差异 —— 「上次改了什么」是最常问的问题，
        // 不该让用户再点一次按钮才发现要看哪一版
        setDiff(await api.projectDiff(name, second.version, newest.version));
      } else if (newest) {
        setFromV(0);
        setToV(newest.version);
        setDiff(null);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setVersions([]);
    }
  }, []);

  useEffect(() => {
    void loadVersions(selected);
  }, [selected, loadVersions]);

  const onCreate = async (): Promise<void> => {
    const name = newName.trim();
    if (!name) return;
    setBusy(true);
    try {
      await api.createProject(name);
      setNewName('');
      await loadProjects();
      setSelected(name);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const onCompare = async (): Promise<void> => {
    if (!selected || !fromV || !toV) return;
    setBusy(true);
    try {
      // versions 是新→旧；用户选的「从」应早于「到」，交换一下更符合直觉
      const lo = Math.min(fromV, toV);
      const hi = Math.max(fromV, toV);
      setDiff(await api.projectDiff(selected, lo, hi));
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setDiff(null);
    } finally {
      setBusy(false);
    }
  };

  const onLoadLatest = async (): Promise<void> => {
    if (!selected) return;
    try {
      const data = await api.projectIntent(selected);
      const blob = new Blob([JSON.stringify(data.doc, null, 2)], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${selected}-v${data.version}.json`;
      a.click();
      URL.revokeObjectURL(url);
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="projects-view">
      <div className="panel project-head">
        <div>
          <h2>项目与版本</h2>
          <p className="muted">
            每次保存是一个版本。内容没变不会刷版本号；要留里程碑用「留快照」。
            数据库：<code>{database || '—'}</code>
          </p>
        </div>
        <div className="project-new">
          <input
            value={newName}
            placeholder="新项目名（如 信息港二期）"
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onCreate();
            }}
          />
          <button type="button" onClick={() => void onCreate()} disabled={busy || !newName.trim()}>
            建项目
          </button>
        </div>
      </div>

      {err && (
        <div className="callout warn">
          <strong>出错了：</strong>
          {err}
        </div>
      )}

      <div className="project-split">
        <div className="panel">
          <h3>项目（{projects.length}）</h3>
          {projects.length === 0 && (
            <p className="muted">
              还没有项目。在右上角输入名字建一个，然后用 CLI{' '}
              <code>project-save --name &lt;名&gt; --intent &lt;json&gt;</code> 把意图存进来。
            </p>
          )}
          <ul className="project-list">
            {projects.map((p) => (
              <li
                key={p.id}
                className={p.name === selected ? 'active' : ''}
                onClick={() => setSelected(p.name)}
              >
                <div className="project-name">{p.name}</div>
                <div className="project-meta">
                  v{p.latestVersion} · 共 {p.versionCount} 版 · {fmtTime(p.updatedAt)}
                </div>
              </li>
            ))}
          </ul>
        </div>

        <div className="panel">
          <h3>
            版本历史{selected ? `（${selected}）` : ''}
          </h3>
          {selected && versions.length === 0 && (
            <p className="muted">这个项目还没有保存过意图。</p>
          )}
          {versions.length > 0 && (
            <>
              <table className="consume-table">
                <thead>
                  <tr>
                    <th>版本</th>
                    <th>保存于</th>
                    <th>意图行数</th>
                    <th>服务器</th>
                    <th>备注</th>
                  </tr>
                </thead>
                <tbody>
                  {versions.map((v) => (
                    <tr key={v.version}>
                      <td>v{v.version}</td>
                      <td>{fmtTime(v.createdAt)}</td>
                      <td>{v.summary.total}</td>
                      <td>{v.summary.serverCount}</td>
                      <td className="muted">{v.note || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <div className="project-actions">
                <label>
                  从
                  <select value={fromV} onChange={(e) => setFromV(Number(e.target.value))}>
                    {versions.map((v) => (
                      <option key={v.version} value={v.version}>
                        v{v.version}
                      </option>
                    ))}
                  </select>
                  到
                  <select value={toV} onChange={(e) => setToV(Number(e.target.value))}>
                    {versions.map((v) => (
                      <option key={v.version} value={v.version}>
                        v{v.version}
                      </option>
                    ))}
                  </select>
                </label>
                <button type="button" onClick={() => void onCompare()} disabled={busy}>
                  看差异
                </button>
                <button type="button" onClick={() => void onLoadLatest()} disabled={busy}>
                  下载最新意图 JSON
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {diff && (
        <div className="panel">
          <h3>
            差异：v{diff.fromVersion} → v{diff.toVersion}
            {diff.totalChanges === 0 ? '（内容完全相同）' : `（${diff.totalChanges} 处）`}
          </h3>
          {diff.totalChanges === 0 && (
            <p className="muted">
              两版内容一致。这通常是用「留快照」留下的里程碑版本。
            </p>
          )}
          {diff.sections
            .filter((s) => !s.isEmpty)
            .map((s) => (
              <div key={s.section} className="diff-section">
                <h4>
                  【{s.label}】{s.count} 处
                </h4>
                <ul>
                  {s.added.slice(0, 20).map((it, i) => (
                    <li key={`a${i}`} className="diff-added">
                      + {brief(it)}
                    </li>
                  ))}
                  {s.removed.slice(0, 20).map((it, i) => (
                    <li key={`r${i}`} className="diff-removed">
                      − {brief(it)}
                    </li>
                  ))}
                  {s.changed.slice(0, 20).map((pair, i) => (
                    <li key={`c${i}`} className="diff-changed">
                      ~ {brief(pair.before)} ⇒ {brief(pair.after)}
                    </li>
                  ))}
                </ul>
                {s.count > 60 && (
                  <p className="muted">另有 {s.count - 60} 条未展开。</p>
                )}
              </div>
            ))}
        </div>
      )}
    </div>
  );
}
