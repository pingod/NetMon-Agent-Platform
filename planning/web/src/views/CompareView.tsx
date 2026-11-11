/**
 * 跨版本对比页签（v0.4 #6）。
 *
 * ## 这个页面回答什么问题
 *
 * 用户手里有 11 个 Excel：`V3 / V8 / V9 / V11 / V12 / V13`。
 * 「V12 到 V13 到底改了什么」是真实提问，而 Excel 自己查不出来。
 *
 * ## 三条实测决定的界面设计
 *
 * 1. **下拉里默认不出现「连线表(不含长度)」在前排**
 *    实测这份表从 V8 到 V13 五个星期一个格没动（5110 行逐行相同）。
 *    如果排在当前布线表旁边，用户第一次就选它 → 得到「完全一致」→
 *    以为功能坏了。所以按后端给的 priority 排序：当前布线表 0、无长度那版 3、存档 9。
 *
 * 2. **必须显示「用了哪层比对键、为什么」**
 *    V12 → V13 用 strict 键会得到「4944 条只在右边」，而真相是 V13
 *    把实例名列从 14% 补到 100%。所以键层级由后端按填充率选，
 *    前端把它和理由一起摆在结果最上方。
 *
 * 3. **填充率变化单独一块，不混进「修改了 N 行」**
 *    补一列名字按行比是 4400 条"修改"，但语义是"补录"。
 *    这一块的数字比差异条数更重要 —— 它解释了为什么差异条数那么少。
 *
 * ## 为什么不做成「本项目版本对比」
 *
 * 已有 `ProjectsView`（第七页签）比的是数据库里的意图版本（949 行）。
 * 这里比的是磁盘上的 Excel 交付物（5127 行），两者是不同的东西：
 * 前者是"我打算怎么做"，后者是"我承诺了什么"。
 *
 * ## 两种对比对象：磁盘 Excel / 库里的项目
 *
 * 除了「V12 → V13」，用户还会问「一期 vs 二期」「403 机房 vs 404 机房」——
 * 那是两个**项目**而不是同一份规划的两版。两者都走同一套 diff
 * （`stockdiff` 只认表格，不关心表从哪来），差别只在数据源。
 * 所以顶部加一个模式切换，而不是再做第九个页签。
 */
import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactElement } from 'react';

import { api } from '../api/client';
import type { ProjectSummary, StockComparePayload, StockFileInfo } from '../api/client';

/** 一个下拉选项的展示文字。文件名太长，只留版本号 + sheet 名。 */
function fileLabel(f: StockFileInfo): string {
  // 文件名形如 `V13(内部)甘肃信息港-设备上架规划-20260714.xlsx`
  const ver = /^(V\d+)/.exec(f.name)?.[1] ?? f.name;
  return `${ver} · ${f.sheet}`;
}

function pct(x: number): string {
  return `${(x * 100).toFixed(1)}%`;
}

/** 比对键层级的人类说明。 */
const KEY_MODE_TEXT: Record<string, string> = {
  strict: '按实例名精确比对',
  loose: '对端用型号+位置比对',
  position: '按纯位置比对',
};

/** 对比对象：磁盘上的 Excel 交付物，还是库里的项目。 */
type CompareMode = 'files' | 'projects';

export function CompareView(): ReactElement {
  const [mode, setMode] = useState<CompareMode>('files');
  const [files, setFiles] = useState<StockFileInfo[]>([]);
  const [leftKey, setLeftKey] = useState('');
  const [rightKey, setRightKey] = useState('');
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [leftProj, setLeftProj] = useState('');
  const [rightProj, setRightProj] = useState('');
  const [result, setResult] = useState<StockComparePayload | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const keyOf = (f: StockFileInfo): string => `${f.path}::${f.sheet}`;

  useEffect(() => {
    void (async () => {
      try {
        const data = await api.stockFiles();
        setFiles(data.files);
        // 默认选「当前版」与「上一版」：两者都是 priority 0，按版本号倒序排在前两位
        const cur = data.files.filter((f) => f.priority === 0);
        if (cur.length >= 2) {
          setRightKey(keyOf(cur[0]!));
          setLeftKey(keyOf(cur[1]!));
        } else if (cur.length === 1) {
          setRightKey(keyOf(cur[0]!));
        }
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      }
    })();
  }, []);

  // 项目下拉：与文件下拉并行拉。失败不影响文件模式 —— 库里没项目时
  // 那半个页面用不上，不该让整个页签报错。
  useEffect(() => {
    void api
      .projects()
      .then((r) => {
        setProjects(r.projects);
        // 有至少两个项目才预选：只有一个时「对比」没有意义
        if (r.projects.length >= 2) {
          setRightProj(r.projects[0]!.name);
          setLeftProj(r.projects[1]!.name);
        } else if (r.projects.length === 1) {
          setRightProj(r.projects[0]!.name);
        }
      })
      .catch(() => setProjects([]));
  }, []);

  const find = useCallback(
    (key: string): StockFileInfo | undefined => files.find((f) => keyOf(f) === key),
    [files],
  );

  const run = useCallback(async () => {
    setBusy(true);
    setErr(null);
    try {
      if (mode === 'files') {
        const left = find(leftKey);
        const right = find(rightKey);
        if (!left || !right) {
          setBusy(false);
          return;
        }
        setResult(
          await api.compareStocks({
            leftPath: left.path,
            leftSheet: left.sheet,
            rightPath: right.path,
            rightSheet: right.sheet,
            limit: 30,
          }),
        );
      } else {
        if (!leftProj || !rightProj) {
          setBusy(false);
          return;
        }
        setResult(await api.compareProjects(leftProj, rightProj, 30));
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setResult(null);
    } finally {
      setBusy(false);
    }
  }, [mode, find, leftKey, rightKey, leftProj, rightProj]);

  // 切模式时清掉上一次的结果：留着会把「V12→V13 的 4886 条」显示成
  // 「一期 vs 二期的结果」，而用户刚点完切换，会以为新结果已经出来了。
  useEffect(() => {
    setResult(null);
    setErr(null);
  }, [mode]);

  // 两侧都选好就自动跑一次：这个页面只有一个动作，让用户再点一下没有意义
  useEffect(() => {
    const ready =
      mode === 'files' ? Boolean(leftKey && rightKey) : Boolean(leftProj && rightProj);
    if (ready && !result && !busy) void run();
  }, [mode, leftKey, rightKey, leftProj, rightProj, result, busy, run]);

  const options = useMemo(
    () =>
      files.map((f) => (
        <option key={keyOf(f)} value={keyOf(f)}>
          {fileLabel(f)}
          {f.priority >= 3 ? '（无长度列）' : ''}
          {f.priority >= 9 ? '（存档）' : ''}
          {` · ${f.rows} 行`}
        </option>
      )),
    [files],
  );

  return (
    <section className="compare-view">
      <div className="compare-mode">
        <button
          type="button"
          className={mode === 'files' ? 'active' : undefined}
          onClick={() => setMode('files')}
        >
          比磁盘 Excel
        </button>
        <button
          type="button"
          className={mode === 'projects' ? 'active' : undefined}
          onClick={() => setMode('projects')}
        >
          比项目（最新版）
        </button>
        <span className="compare-mode-hint">
          {mode === 'files'
            ? '仓库「信息港部署上架」目录里的 Excel 交付物'
            : '项目版本库里各项目的最新版（比的是「一期 vs 二期」这类）'}
        </span>
      </div>

      <div className="compare-head">
        <div className="picker">
          <label htmlFor="cmp-left">左侧（旧）</label>
          {mode === 'files' ? (
            <select id="cmp-left" value={leftKey} onChange={(e) => setLeftKey(e.target.value)}>
              <option value="">— 选择 —</option>
              {options}
            </select>
          ) : (
            <select id="cmp-left" value={leftProj} onChange={(e) => setLeftProj(e.target.value)}>
              <option value="">— 选择 —</option>
              {projects.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name}（v{p.latestVersion}）
                </option>
              ))}
            </select>
          )}
        </div>
        <span className="arrow">→</span>
        <div className="picker">
          <label htmlFor="cmp-right">右侧（新）</label>
          {mode === 'files' ? (
            <select id="cmp-right" value={rightKey} onChange={(e) => setRightKey(e.target.value)}>
              <option value="">— 选择 —</option>
              {options}
            </select>
          ) : (
            <select id="cmp-right" value={rightProj} onChange={(e) => setRightProj(e.target.value)}>
              <option value="">— 选择 —</option>
              {projects.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name}（v{p.latestVersion}）
                </option>
              ))}
            </select>
          )}
        </div>
        <button
          type="button"
          className="btn-primary"
          disabled={busy || (mode === 'files' ? !leftKey || !rightKey : !leftProj || !rightProj)}
          onClick={() => void run()}
        >
          {busy ? '对比中…' : '重新对比'}
        </button>
      </div>

      {mode === 'projects' && projects.length < 2 && (
        <div className="callout info">
          库里只有 {projects.length} 个项目，跨项目对比至少要两个。先在
          「项目与版本」页建第二个项目并保存一次意图。
        </div>
      )}

      {err && <div className="compare-error">{err}</div>}

      {result && (
        <>
          <div className={`compare-key ${result.keyMode}`}>
            <strong>{KEY_MODE_TEXT[result.keyMode] ?? result.keyMode}</strong>
            <span className="key-reason">{result.keyReason}</span>
            <span className="key-counts">
              {result.leftRows} 行 → {result.rightRows} 行
            </span>
          </div>

          {result.fillChanges.length > 0 && (
            <div className="panel fill-panel">
              <h3>列填充率变化</h3>
              <p className="fill-hint">
                这一块比下面的差异条数更重要：补一列名字按行比会算成几千条「修改」，
                实际语义是「补录」。它解释了为什么差异条数那么少。
              </p>
              <table className="fill-table">
                <thead>
                  <tr>
                    <th>列</th>
                    <th>左</th>
                    <th>右</th>
                  </tr>
                </thead>
                <tbody>
                  {result.fillChanges.map((c) => (
                    <tr key={c.field}>
                      <td>{c.label}</td>
                      <td className="num">{pct(c.before)}</td>
                      <td className="num">
                        {pct(c.after)}
                        <span className={c.after > c.before ? 'up' : 'down'}>
                          {c.after > c.before ? ' ↑' : ' ↓'}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* 设备级聚合必须排在行级统计**前面**。
              因为行级数字（V12→V13 是 4886 新增 / 4890 删除）会让人以为
              整个规划重做了一遍，而设备级是「56 台挪了 U 位 + 2 台新增 +
              54 台改了端口名」—— 后者才是施工队能读懂的说法。
              先给结论，再给明细。 */}
          {result.byDevice && !result.byDevice.isEmpty && (
            <div className="panel device-panel">
              <h3>设备级变化</h3>
              <p className="device-hint">
                按「一台设备在一个机柜里的一个 U 位」聚合。同一台交换机的 44 根线
                挂在同一个 U 位上，逐行比会算成 44 条新增 + 44 条删除；
                设备级只报「这台设备挪了 1U，44 个端口未变」。
              </p>
              {result.byDevice.note && <p className="device-note">{result.byDevice.note}</p>}
              <div className="device-counts">
                <DeviceStat n={result.byDevice.counts.moved} label="挪了 U 位" tone="moved" />
                <DeviceStat n={result.byDevice.counts.changed} label="端口有变" tone="chg" />
                <DeviceStat n={result.byDevice.counts.added} label="新增设备" tone="add" />
                <DeviceStat n={result.byDevice.counts.removed} label="移除设备" tone="del" />
                <span className="baseline-note">
                  {result.byDevice.counts.leftDevices} 台 → {result.byDevice.counts.rightDevices} 台
                </span>
              </div>
              <div className="device-grid">
                {result.byDevice.moved.length > 0 && (
                  <div className="device-block moved">
                    <h4>挪了 U 位 {result.byDevice.counts.moved} 台</h4>
                    <ul>
                      {result.byDevice.moved.map((d, i) => (
                        <li key={`m${i}`}>{d}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {result.byDevice.changed.length > 0 && (
                  <div className="device-block chg">
                    <h4>端口有变 {result.byDevice.counts.changed} 台</h4>
                    <ul>
                      {result.byDevice.changed.map((d, i) => (
                        <li key={`c${i}`}>{d}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {result.byDevice.added.length > 0 && (
                  <div className="device-block add">
                    <h4>新增设备 {result.byDevice.counts.added} 台</h4>
                    <ul>
                      {result.byDevice.added.map((d, i) => (
                        <li key={`da${i}`}>{d}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {result.byDevice.removed.length > 0 && (
                  <div className="device-block del">
                    <h4>移除设备 {result.byDevice.counts.removed} 台</h4>
                    <ul>
                      {result.byDevice.removed.map((d, i) => (
                        <li key={`dr${i}`}>{d}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            </div>
          )}

          <div className="compare-stats">
            <Stat n={result.addedCount} label="新增行" tone="add" />
            <Stat n={result.removedCount} label="删除行" tone="del" />
            <Stat n={result.changedCount} label="修改行" tone="chg" />
            <Stat n={result.lengthChangeCount} label="线长变化" tone="len" />
            {result.lengthBaseline !== undefined && (
              <span className="baseline-note">
                线长分母 {result.lengthBaseline} 行（两侧都有长度值的行）
              </span>
            )}
          </div>

          {result.isEmpty ? (
            <div className="empty-box">
              两份表逐行完全一致。
              {result.notes.find((n) => n.includes('完全一致')) && (
                <div className="empty-hint">{result.notes.find((n) => n.includes('完全一致'))}</div>
              )}
            </div>
          ) : (
            <div className="diff-grid">
              {result.added.length > 0 && (
                <div className="panel diff-block added">
                  <h3>新增 {result.addedCount} 行</h3>
                  <ul>
                    {result.added.map((r, i) => (
                      <li key={`a${i}`}>{r}</li>
                    ))}
                  </ul>
                  {result.addedCount > result.added.length && (
                    <p className="more">另有 {result.addedCount - result.added.length} 行</p>
                  )}
                </div>
              )}
              {result.removed.length > 0 && (
                <div className="panel diff-block removed">
                  <h3>删除 {result.removedCount} 行</h3>
                  <ul>
                    {result.removed.map((r, i) => (
                      <li key={`r${i}`}>{r}</li>
                    ))}
                  </ul>
                  {result.removedCount > result.removed.length && (
                    <p className="more">另有 {result.removedCount - result.removed.length} 行</p>
                  )}
                </div>
              )}
              {result.changed.length > 0 && (
                <div className="panel diff-block changed">
                  <h3>修改 {result.changedCount} 行</h3>
                  <ul>
                    {result.changed.map((c, i) => (
                      <li key={`c${i}`}>
                        <code>{c.row}</code>
                        <ul className="fields">
                          {c.fields.map((f) => (
                            <li key={f.field}>
                              {f.label}：<span className="before">{f.before || '—'}</span> →{' '}
                              <span className="after">{f.after || '—'}</span>
                            </li>
                          ))}
                        </ul>
                        {c.fieldCount > c.fields.length && (
                          <span className="more">另有 {c.fieldCount - c.fields.length} 个字段</span>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {result.lengthChanges.length > 0 && (
                <div className="panel diff-block length">
                  <h3>线长变化 {result.lengthChangeCount} 行</h3>
                  <ul>
                    {result.lengthChanges.map((l, i) => (
                      <li key={`l${i}`}>
                        <code>{l.row}</code> {l.before} 米 → {l.after} 米
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}

          {result.notes.length > 0 && (
            <div className="panel note-panel">
              <h3>说明</h3>
              <ul>
                {result.notes.map((n, i) => (
                  <li key={i}>{n}</li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  );
}

function Stat({ n, label, tone }: { n: number; label: string; tone: string }): ReactElement {
  return (
    <div className={`stat ${tone} ${n === 0 ? 'zero' : ''}`}>
      <span className="stat-n">{n}</span>
      <span className="stat-label">{label}</span>
    </div>
  );
}

function DeviceStat({ n, label, tone }: { n: number; label: string; tone: string }): ReactElement {
  return (
    <div className={`stat dev ${tone} ${n === 0 ? 'zero' : ''}`}>
      <span className="stat-n">{n}</span>
      <span className="stat-label">{label}</span>
      <span className="stat-unit">台</span>
    </div>
  );
}
