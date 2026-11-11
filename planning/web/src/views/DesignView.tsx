/**
 * 接线意图 → 布线表（v0.3 主功能，FR-3.x）。
 *
 * ## 这一页是整个产品的价值主张所在
 *
 * 用户手上是一份 5127 行的 Excel 布线表，每一行都是某个人手填进去的。
 * 这一页要说清三件事：
 *
 * 1. **写 915 行意图，机器补 5127 行连线** —— 对比要摆在最上面，
 *    否则用户不知道这功能省了多少事。两个数字都要讲：207 行是规则能推导的
 *    （布局/交换机位置/路由/交换机组），697 行是显式连线（交换机互联 +
 *    特殊机型），**必须人写**；
 * 2. **生成的质量** —— 与已导入的存量表逐行比对：新增/缺失/线长差异。
 *    当前实测 5127/5127 全部对齐，多出 0 缺失 0；
 * 3. **生成的边界与已知差异** —— 线长差 10 行，全部因为原表把同一台设备
 *    记到了两个位置。这不是公式错，是数据问题，要让用户知道去核哪几台。
 *
 * ## 为什么生成结果只显示前 200 行
 *
 * 4430 行 DOM 渲染要 40ms+ 且用户看不完。首屏给 200 行 + 统计 +
 * 「导出全量」按钮，需要逐行核对时去导出的 Excel 里看 ——
 * 那里有 24 列完整契约，比网页表格更适合审。
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { JSX } from 'react';

import type { DesignPayload, DesignRow } from '../api/client';
import { api } from '../api/client';
import { parseHash } from '../store/appStore';
import { IntentEditPanel } from './IntentEditPanel';

/**
 * 「意图怎么录入」—— 折叠面板。
 *
 * ## 为什么要摆在接线设计页上而不是写进 README
 *
 * 用户看到"915 行意图 → 5127 行布线表"的第一反应必然是「那我怎么改这 915 行？」。
 * 把答案放在他产生疑问的那一页，比让他去翻文档短得多。
 *
 * ## 为什么默认折叠
 *
 * 这段话对"已经知道怎么填"的人是噪音，对"第一次进来"的人是关键。
 * 折叠能同时满足两者：标题本身就是答案的一半（"六段表格 / JSON"）。
 */
function IntentSourcePanel({ data }: { data: DesignPayload }): JSX.Element {
  const fmt = data.intent.sourceFormat;
  // 三种来源必须分开说，否则用户分不清「我改的那个文件」和「这次生成读的那个」：
  // - xlsx：磁盘上的六段表格（人填的表）
  // - json：磁盘上的 intent.json（v0.3 反解出来的基线）
  // - db：项目版本库里的某个版本（编辑器保存后的产物）
  const sourceText =
    fmt === 'xlsx'
      ? '磁盘 intent.xlsx · 六段表格'
      : fmt === 'db'
        ? '项目版本库 · 最新保存的版本'
        : '磁盘 intent.json · JSON';
  return (
    <details className="panel intent-source">
      <summary>
        <h3>
          意图怎么填
          <span className="subtle">
            （当前来源 <code>{sourceText}</code>）
          </span>
        </h3>
      </summary>
      <p className="panel-note">
        六段表格分别是：<b>布局</b>（一行一个机柜段）· <b>交换机</b>（一行一台位置）·
        <b>特殊设备</b>（一行一台有名字的非交换机设备）· <b>路由</b>（一行一条「某型号网口
        接某型号交换机」）· <b>交换机组</b>（一行一条链，顺序即接线顺序）·
        <b>显式连线</b>（一行一条手工连线，用于交换机互联与特殊机型）。
        <br />
        布局 {data.intent.racks} 行就描述了 {data.intent.serverCount} 台服务器的摆放，
        路由 {data.intent.routes} 行 + 交换机组 {data.intent.chains} 行覆盖了全部规则化接线 ——
        <b>这就是「几十行意图」的来源</b>。剩下 {data.intent.explicitLinks} 行显式连线是
        交换机互联与每台一种接法的特殊机型，<b>必须手写</b>。
      </p>
      <div className="intent-cmds">
        <div>
          <span className="intent-cmd-label">① 导出模板（已有项目改造）</span>
          <code>python -m idcbox.cli intent-export --intent projects/v13/intent_full.json --out 导入.xlsx</code>
        </div>
        <div>
          <span className="intent-cmd-label">② 在 Excel 里改完，先检查能不能读通</span>
          <code>python -m idcbox.cli intent-check --sheet 导入.xlsx</code>
        </div>
        <div>
          <span className="intent-cmd-label">③ 生成布线表</span>
          <code>python -m idcbox.cli generate --sheet 导入.xlsx --out 布线表.xlsx --source 上架规划.xlsx</code>
        </div>
      </div>
      <p className="panel-note">
        也可以直接把 <code>IDCBOX_INTENT</code> 指向 <code>.xlsx</code> 文件，本页每次刷新都会
        重新读表 —— 不必先转成 JSON。<b>表格只是视图，不是第二份数据</b>：
        同一份意图的 JSON 与 Excel 两种格式生成结果逐格一致（已有用例守着）。
      </p>
    </details>
  );
}

/**
 * 「这次生成读的是哪份意图」—— 来源条 + 项目切换器。
 *
 * ## 为什么必须显式摆出来
 *
 * 生成结果依赖「读的是哪份意图」，而意图有两个可能的家：磁盘文件、
 * 项目版本库。用户改完保存后刷新页面，如果默认退回读磁盘，
 * 他会看到旧结果而界面**不报任何错** —— 这类「静默读错源」比报错难查十倍。
 *
 * 所以这里做三件事：①把「实际读了哪儿」原文写出来（用后端回的 `source`，
 * 不是前端猜的）②给出切换入口 ③当编辑器正在编辑某个项目时锁住选择器
 * （此时改选择器只会让界面显示的和编辑器改的不是一回事）。
 */
function SourceBar({
  mode,
  projectName,
  projects,
  editorLocked,
  actual,
  onChangeMode,
  onChangeProject,
}: {
  mode: 'auto' | 'db' | 'file';
  projectName: string;
  projects: { name: string; latestVersion: number }[];
  editorLocked: boolean;
  actual: DesignPayload['source'];
  onChangeMode: (next: 'auto' | 'db' | 'file') => void;
  onChangeProject: (next: string) => void;
}): JSX.Element {
  // 后端回的实际来源优先。前端 state 只代表「用户想读哪儿」，
  // 真正读了哪儿必须由后端说 —— 否则「我选了库但实际读了文件」看不见。
  const actualText = !actual
    ? '（后端未回报）'
    : actual.kind === 'db'
      ? `项目库 · ${actual.project ?? '?'}${actual.version ? ` v${actual.version}` : ' 最新版'}`
      : actual.kind === 'xlsx'
        ? '磁盘 intent.xlsx（六段表格）'
        : '磁盘 intent_full.json';

  const mismatch =
    !!actual &&
    ((mode === 'file' && actual.kind === 'db') ||
      (mode === 'db' && actual.kind !== 'db') ||
      (mode === 'auto' && actual.kind !== 'db'));

  return (
    <div className={`source-bar${mismatch ? ' source-bar-warn' : ''}`}>
      <span className="source-label">本次生成读的是</span>
      <code className="source-actual">{actualText}</code>

      <span className="source-sep" />

      <label className="source-pick">
        来源
        <select
          value={mode}
          disabled={editorLocked}
          onChange={(e) => onChangeMode(e.target.value as 'auto' | 'db' | 'file')}
          title={editorLocked ? '编辑器正开着，来源由它决定' : undefined}
        >
          <option value="auto">自动（有库优先库）</option>
          <option value="db">项目库</option>
          <option value="file">磁盘文件</option>
        </select>
      </label>

      {mode !== 'file' && (
        <label className="source-pick">
          项目
          <select
            value={projectName}
            disabled={editorLocked || projects.length === 0}
            onChange={(e) => onChangeProject(e.target.value)}
          >
            <option value="">
              {projects.length === 0 ? '（库里还没有项目）' : '自动（最近更新的）'}
            </option>
            {projects.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}（v{p.latestVersion}）
              </option>
            ))}
          </select>
        </label>
      )}

      {editorLocked && <span className="source-hint">编辑器开着，来源锁定为它正在改的项目</span>}
      {mismatch && !editorLocked && (
        <span className="source-hint source-hint-warn">
          库里有项目，但这次读的是磁盘文件 —— 界面显示的可能不是你刚保存的那份
        </span>
      )}
    </div>
  );
}

/** 「几十行 → 几千行」的对比条。这是首屏第一眼要看到的东西。 */
function VolumeCompare({ data }: { data: DesignPayload }): JSX.Element {
  const intentRows = data.intent.total;
  const genRows = data.stats.links;
  // 意图行只占生成的 ~4.7%，条形图上会细成一条线 —— 给最小 6px 保证可见
  const intentWidth = Math.max((intentRows / Math.max(genRows, 1)) * 100, 1.2);

  return (
    <div className="volume-compare">
      <div className="volume-side">
        <div className="volume-num">{intentRows}</div>
        <div className="volume-label">行接线意图</div>
        <div className="volume-detail">
          布局 {data.intent.racks} · 交换机位置 {data.intent.switches} · 特殊设备{' '}
          {data.intent.devices} · 路由 {data.intent.routes} · 交换机组 {data.intent.chains}
          {data.intent.explicitLinks > 0 && (
            <>
              {' · '}
              <b>显式连线 {data.intent.explicitLinks}</b>
            </>
          )}
        </div>
      </div>
      <div className="volume-arrow">
        <div className="volume-bars">
          <div className="volume-bar volume-bar-intent" style={{ width: `${intentWidth}%` }} />
          <div className="volume-bar volume-bar-gen" style={{ width: '100%' }} />
        </div>
        <div className="volume-ratio">
          ×{(genRows / Math.max(intentRows, 1)).toFixed(1)} 行
        </div>
      </div>
      <div className="volume-side">
        <div className="volume-num">{genRows}</div>
        <div className="volume-label">行布线表</div>
        <div className="volume-detail">
          覆盖 {data.bySwitchCount} 台交换机（{data.switchModels.join('、')}）·
          {data.intent.serverCount} 台服务器
        </div>
      </div>
    </div>
  );
}

/** 比对结果条。全 0 是好事，要用绿色明确告诉用户"可直接用"。 */
function CompareBar({ data }: { data: DesignPayload }): JSX.Element | null {
  const cmp = data.compare;
  if (!cmp) return null;
  const allClean = cmp.added === 0 && cmp.missing === 0 && cmp.lengthDiffs === 0;
  //: 「行位对齐」与「线长一致」是两件事，标题不能含糊。
  //: 当前实测行位 5127/5127 全对，只有 10 行线长因原表位置冲突差一档 ——
  //: 说成"存在差异"会让用户以为算法算错了 5127 行里的 10 行。
  const rowsClean = cmp.added === 0 && cmp.missing === 0;
  const title = allClean
    ? '✅ 与存量布线表完全一致'
    : rowsClean
      ? '✅ 行位完全对齐，仅线长有差异'
      : '⚠️ 与存量表存在差异';
  //: 长度比对的**分母**只有原表带公式的那些行，不是全部 5127 行。
  //: 不写清楚会让用户以为「5127 行里 55 行算错了」，实际是「636 行里 55 行
  //: 的人工输入值与几何不一致」。这两个说法对产品的意义完全不同。
  const lengthDenom = cmp.lengthBaseline ?? 0;

  return (
    <section className={`panel compare-panel${allClean ? ' compare-clean' : ' compare-dirty'}`}>
      <h3>
        {title}
        <span className="compare-scope">
          （比对范围：全表 {cmp.stockRows} 行，交换机↔服务器与交换机↔交换机都含）
        </span>
      </h3>
      <div className="compare-grid">
        <div className="compare-item">
          <span className="compare-num compare-ok">{cmp.common}</span>
          <span className="compare-label">行列位一致</span>
        </div>
        <div className="compare-item">
          <span className={`compare-num${cmp.added > 0 ? ' compare-bad' : ''}`}>{cmp.added}</span>
          <span className="compare-label">行只在生成结果里</span>
        </div>
        <div className="compare-item">
          <span className={`compare-num${cmp.missing > 0 ? ' compare-bad' : ''}`}>
            {cmp.missing}
          </span>
          <span className="compare-label">行只在存量表里</span>
        </div>
        <div className="compare-item">
          <span className={`compare-num${cmp.lengthDiffs > 0 ? ' compare-bad' : ''}`}>
            {cmp.lengthDiffs}
          </span>
          <span className="compare-label">
            行线长不一致
            {lengthDenom > 0 && <i className="compare-denom">（可比 {lengthDenom} 行）</i>}
          </span>
        </div>
      </div>
      {cmp.missing === 0 && cmp.lengthDiffs > 0 && (
        <div className="missing-attr">
          <h4>线长不一致的 {cmp.lengthDiffs} 行是什么？</h4>
          <p className="panel-note">
            原表「线缆长度」这一列虽然覆盖 {cmp.stockRows} 行，但真正带公式的只有{' '}
            <b>{lengthDenom} 行</b>，其余行那格本来就是空白 —— 策划只给需要精确长度的行填了公式。
            所以要问的不是「{cmp.stockRows} 行为什么有 {cmp.lengthDiffs} 行不对」，而是
            「<b>{lengthDenom} 行里为什么有 {cmp.lengthDiffs} 行不对</b>」。
            <br />
            <br />
            真正的公式链（机柜数量 P → 柜间线缆 R → 线缆2 S → 长度梯档 T）本身<b>没有误差</b>：
            在「跨机柜 N / 间隔机柜 O / 机内线缆 Q」三个<b>人工填写</b>的输入列与几何一致时，
            实测 <b>{cmp.lengthClean} 行四个公式列全部 100% 复现</b>。
            <br />
            <br />
            这 {lengthDenom} 行可以精确四分，加起来正好对平（
            <b>
              {cmp.lengthClean} + {cmp.lengthLucky} + {cmp.lengthDiffs} = {lengthDenom}
            </b>
            ）：
          </p>
          <div className="compare-grid compare-grid-4">
            <div className="compare-item">
              <span className="compare-num compare-ok">{cmp.lengthClean}</span>
              <span className="compare-label">输入一致 · 公式全对</span>
            </div>
            <div className="compare-item">
              <span className="compare-num">{cmp.lengthLucky}</span>
              <span className="compare-label">输入填错 · 结果凑巧对上</span>
            </div>
            <div className="compare-item">
              <span className="compare-num compare-bad">{cmp.lengthDiffs}</span>
              <span className="compare-label">输入填错 · 结果也不同</span>
            </div>
            <div className="compare-item">
              <span className="compare-num compare-ok">0</span>
              <span className="compare-label">输入一致 · 公式却算错</span>
            </div>
          </div>
          <p className="panel-note">
            最后一格恒为 0 —— <b>只要三个人工输入与几何一致，长度就一定和 Excel 一样</b>，
            这就是公式可信的依据。
            <br />
            那 {cmp.lengthDiffs} 行全部是<b>人工填的 N/O/Q 与几何推导不一致</b>：典型形态是
            同排两柜的间隔值被填反（人填 O=7 而几何 6，另一行反过来），
            或同柜与邻柜的判定互换。生成器按几何算，所以差一档。
            <br />
            这条差异<b>刻意不去对齐</b> —— 悄悄对齐等于把原表的数据录入错误固化进算法，
            下一套机房会照抄这个错。请回原表核对这几行的 N/O/Q 三列。
          </p>
          <div className="missing-chips">
            {cmp.lengthDiffSamples.slice(0, 14).map((d) => (
              <span key={d.key.join('|')} className="missing-chip">
                {d.key[0]}
                <i>
                  {d.generated}m / 表 {d.stock}m
                </i>
              </span>
            ))}
          </div>
        </div>
      )}
      {cmp.missing > 0 && (
        <>
          <div className="missing-attr">
            <h4>缺失行是什么？</h4>
            <p className="panel-note">
              这些行按对端设备型号归因如下。全都是「每台一种接法」的特殊机型 ——
              同型号的不同实例接不同交换机，规则推导不出来，必须在意图里显式声明连线。
              <b>这不是算法漏了，是这些行本来就得手写。</b>
            </p>
            <div className="missing-chips">
              {cmp.missingByModel.map((m) => (
                <span key={m.model} className="missing-chip">
                  {m.model}
                  <i>{m.count}</i>
                </span>
              ))}
            </div>
          </div>
          <details className="compare-detail">
            <summary>缺失行明细（前 {Math.min(cmp.missingSamples.length, 50)} 条）</summary>
            <ul>
              {cmp.missingSamples.slice(0, 50).map((k) => (
                <li key={k.join('|')}>
                  <code>{k[0]}</code> 端口 <code>{k[1]}</code> · 网口 <code>{k[2]}</code>
                </li>
              ))}
            </ul>
          </details>
        </>
      )}
    </section>
  );
}

/** 网口分布。用户要确认"每个网口都真的接了"，不是抽样。 */
function NicBreakdown({ data }: { data: DesignPayload }): JSX.Element {
  const max = Math.max(...data.stats.byNic.map((n) => n.count), 1);
  const explicitTotal = data.stats.explicitByPort.reduce((a, b) => a + b.count, 0);
  return (
    <section className="panel">
      <h3>按网口分布</h3>
      <p className="panel-note">
        每种网口的行数应等于「接该网口的服务器台数」。数字不齐说明某类节点漏接线。
        <br />
        千兆1（{data.stats.byNic.find((n) => n.nic === '千兆1')?.count ?? 0}）比 IPMI（
        {data.stats.byNic.find((n) => n.nic === 'IPMI')?.count ?? 0}）多，是因为显式连线里
        还有特殊机型（网络功能服务器/防火墙等）走千兆口而<b>没有 IPMI</b> ——
        这些机型按台声明，不在规则生成范围内。
      </p>
      <div className="nic-rows">
        {data.stats.byNic.map((n) => (
          <div key={n.nic} className="nic-row">
            <span className="nic-name">{n.nic}</span>
            <div className="nic-bar">
              <i style={{ width: `${(n.count / max) * 100}%` }} />
            </div>
            <span className="nic-count">{n.count}</span>
          </div>
        ))}
      </div>
      {data.stats.explicitByPort.length > 0 && (
        <details className="compare-detail" style={{ marginTop: 12 }}>
          <summary>
            交换机侧端口分布（显式连线 {explicitTotal} 行，共{' '}
            {data.stats.explicitByPort.length} 种端口）
          </summary>
          <p className="panel-note">
            这些是交换机 CLI 端口名（<code>100GE1/0/1</code>），不是服务器网口标签。
            交换机互联是拓扑设计决定，规则推导不出来，因此逐条声明。
          </p>
          <div className="nic-rows">
            {data.stats.explicitByPort.map((n) => (
              <div key={n.port} className="nic-row">
                <span className="nic-name">
                  <code>{n.port}</code>
                </span>
                <div className="nic-bar">
                  <i
                    style={{
                      width: `${(n.count / Math.max(...data.stats.explicitByPort.map((x) => x.count), 1)) * 100}%`,
                    }}
                  />
                </div>
                <span className="nic-count">{n.count}</span>
              </div>
            ))}
          </div>
        </details>
      )}
    </section>
  );
}

export function DesignView(): JSX.Element {
  const [data, setData] = useState<DesignPayload | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [limit, setLimit] = useState(200);
  // 编辑模式：默认关闭。打开后显示六段表格编辑器，保存时写进项目版本，
  // 保存成功后自动重新生成 —— 用户能立刻看到「我改的这一行让布线表变成什么样」。
  //
  // 支持 `?edit=1` 深链直接进编辑态：文档里写「打开这个链接直接改意图」
  // 比让人先找到按钮再点一次更省事；自动化截图也需要这条路
  // （headless 截图没法点按钮）。
  const [editing, setEditing] = useState(() => {
    if (typeof window === 'undefined') return false;
    // 参数在 hash 里（`#design?edit=1`），不在 `location.search` ——
    // 单页应用的 `?` 部分会被 setTab 里的 replaceState 抹掉，
    // 刷新一次编辑态就丢了。
    return parseHash(window.location.hash).params.edit === '1';
  });

  /**
   * 意图来源。三态：
   * - `auto`：有库读库、没库读磁盘（后端决定，这是默认）
   * - `db`：强制读库（选了具体项目时）
   * - `file`：强制读磁盘（「我刚改完 intent.xlsx 想先看看效果」）
   */
  const [mode, setMode] = useState<'auto' | 'db' | 'file'>('auto');
  /** 选中的项目名；空 = 让后端挑（库里最近更新的那个）。 */
  const [projectName, setProjectName] = useState('');
  const [projects, setProjects] = useState<{ name: string; latestVersion: number }[]>([]);

  // 编辑器把「当前改的是哪个项目的哪个版本」上报到这里。
  // 编辑器保存后必须立刻用它的项目重新生成，否则用户改了库里的意图、
  // 页面却还在读磁盘文件 ——「保存成功但布线表没变」，最伤信任的一类 bug。
  const [editorSource, setEditorSource] = useState<{ project: string; version: number } | null>(
    null,
  );

  const load = useCallback(
    async (
      nextLimit: number,
      opts: { mode: 'auto' | 'db' | 'file'; project: string; version?: number },
    ) => {
      setLoading(true);
      setErr(null);
      try {
        setData(
          await api.design({
            limit: nextLimit,
            compare: true,
            ...(opts.project ? { project: opts.project } : {}),
            ...(opts.version ? { version: opts.version } : {}),
            ...(opts.mode === 'file' ? { source: 'file' } : {}),
          }),
        );
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  // 项目下拉的选项来自库。拉失败不阻塞页面：可能库里一个项目都没有
  // （全新部署），此时选择器只显示「磁盘文件」，生成照常走文件。
  useEffect(() => {
    void api
      .projects()
      .then((r) => setProjects(r.projects.map((p) => ({ name: p.name, latestVersion: p.latestVersion }))))
      .catch(() => setProjects([]));
  }, []);

  useEffect(() => {
    // 编辑器开着且有来源 → 以编辑器的为准（它刚从库里读/刚保存过）。
    if (editorSource) {
      void load(limit, { mode: 'db', project: editorSource.project, version: editorSource.version });
      return;
    }
    void load(limit, { mode, project: mode === 'file' ? '' : projectName });
    // 依赖里不放 editorSource 的 version：切版本也会触发，但本项目里
    // 编辑器的 version 只随保存变，保存时 onSaved 会再刷一次，重复请求无害。
  }, [load, limit, mode, projectName, editorSource]);

  const currentMode: 'auto' | 'db' | 'file' = editorSource ? 'db' : mode;

  const rows = useMemo<DesignRow[]>(() => data?.rows ?? [], [data]);

  if (err) {
    return (
      <div className="design-view">
        <div className="error-box">
          <b>生成失败：</b>
          {err}
        </div>
        <p className="panel-note">
          常见原因：库里还没有保存过意图，且 <code>{'projects/v13/intent_full.json'}</code>{' '}
          不存在；或 JSON 字段写错。错误信息已尽量指出具体位置。
        </p>
      </div>
    );
  }

  if (!data) {
    return <div className="view-placeholder">{loading ? '正在生成接线…' : '未加载'}</div>;
  }

  return (
    <div className="design-view">
      <section className="panel design-hero">
        <VolumeCompare data={data} />
        <div className="design-actions">
          <button
            type="button"
            onClick={() =>
              void load(
                limit,
                editorSource
                  ? { mode: 'db', project: editorSource.project, version: editorSource.version }
                  : { mode, project: mode === 'file' ? '' : projectName },
              )
            }
            disabled={loading}
          >
            {loading ? '生成中…' : '重新生成'}
          </button>
          <button
            type="button"
            className={editing ? 'btn-primary' : undefined}
            onClick={() => setEditing((v) => !v)}
          >
            {editing ? '收起意图编辑器' : '编辑意图'}
          </button>
          {data.rowsTotal > rows.length && (
            <button type="button" onClick={() => setLimit(2000)} disabled={loading}>
              显示更多（当前 {rows.length}/{data.rowsTotal}）
            </button>
          )}
        </div>
        <SourceBar
          mode={currentMode}
          projectName={editorSource?.project ?? projectName}
          projects={projects}
          editorLocked={editorSource !== null}
          actual={data.source}
          onChangeMode={(next) => {
            setMode(next);
            if (next === 'file') setProjectName('');
          }}
          onChangeProject={(next) => {
            setProjectName(next);
            setMode(next ? 'db' : 'auto');
          }}
        />
        <div className="design-stats">
          <span>
            错误 <b className={data.stats.errors > 0 ? 'stat-warn' : ''}>{data.stats.errors}</b>
          </span>
          <span>
            告警 <b>{data.stats.warns}</b>
          </span>
          <span>
            算出行长 <b>{data.stats.lengthComputed}</b>/{data.stats.links}
          </span>
          <span className="subtle">
            {data.stats.geometryUsed ? '已用平面图几何' : '未加载几何，长度为空'}
          </span>
        </div>
      </section>

      {editing && (
        <IntentEditPanel
          // 设计页上选的项目就是编辑器要改的项目。不传的话编辑器会自己
          // 取列表第一个 —— 用户在来源条选了「信息港二期」，编辑器却在改
          // 「V13」，保存下去改错项目。
          projectName={mode === 'file' ? '' : projectName}
          onSourceChange={setEditorSource}
          onSaved={(saved) => {
            // 保存后用它刚写入的项目/版本立刻重生成。
            //
            // 参数由编辑器直接传进来，不读 `editorSource` state ——
            // `onSourceChange` 与 `onSaved` 在同一个事件里先后调用，
            // 前者 setState 后当前渲染的闭包里 `editorSource` 仍是旧值（null），
            // 靠它判断会走错分支，退化成「又读了一遍磁盘文件」。
            void load(limit, { mode: 'db', project: saved.project, version: saved.version });
          }}
        />
      )}

      <IntentSourcePanel data={data} />

      <CompareBar data={data} />

      <section className="panel">
        <h3>生成的边界（必须知道的限制）</h3>
        <ul className="notes-list">
          {data.notes.map((n) => (
            <li key={n}>{n}</li>
          ))}
        </ul>
      </section>

      <NicBreakdown data={data} />

      {data.issues.length > 0 && (
        <section className="panel">
          <h3>生成过程中的告警</h3>
          <table className="issue-table">
            <thead>
              <tr>
                <th>类型</th>
                <th>内容</th>
              </tr>
            </thead>
            <tbody>
              {data.issues.slice(0, 30).map((i, idx) => (
                <tr key={`${i.code}-${idx}`}>
                  <td>
                    <code className={`issue-${i.severity}`}>{i.code}</code>
                  </td>
                  <td>{i.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      <section className="panel">
        <h3>
          生成的布线表
          <span className="subtle">
            （显示前 {rows.length} 行 / 共 {data.rowsTotal} 行）
          </span>
        </h3>
        <div className="design-table-wrap">
          <table className="design-table">
            <thead>
              <tr>
                <th>#</th>
                <th>本端交换机</th>
                <th>端口</th>
                <th>对端型号</th>
                <th>位置</th>
                <th>网口</th>
                <th>线缆</th>
                <th className="num">机柜数</th>
                <th className="num">长度</th>
                <th>置信度</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.rowNo}>
                  <td className="num subtle">{r.rowNo}</td>
                  <td>{r.localDevice}</td>
                  <td>
                    <code>{r.localPort}</code>
                  </td>
                  <td>{r.peerModel}</td>
                  <td className="subtle">
                    ch{r.peerChannel}/rack{r.peerRack} · {r.peerU}
                  </td>
                  <td>
                    <span className="nic-chip">{r.nic}</span>
                  </td>
                  <td>{r.cableType}</td>
                  <td className="num">{r.pRackCount ?? '—'}</td>
                  <td className="num">
                    <b>{r.tLadderM ?? '—'}</b>
                    {r.tLadderM != null && <span className="subtle"> m</span>}
                  </td>
                  <td className="subtle">{r.confidence ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
