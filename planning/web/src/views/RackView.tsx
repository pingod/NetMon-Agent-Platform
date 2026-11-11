/**
 * 单机柜 U 位图（FR-6.3）+ 拖拽调 U 位（v0.4 #5）。
 *
 * ## 性能关键：U 刻度必须用 pattern，不能逐 U 画线
 *
 * 42U × 102 个机柜 = 4284 条 `<line>`。看似不多，但每换一个机柜就
 * 重渲染 42 个节点，切来切去会累积。用 `<pattern>` 定义"每 U 一条横线"
 * 的图案，整个机柜底图只要 **1 个 `<rect>`**，节点数从 45 降到 3。
 *
 * 文件里还画了 U 号文字（42 个 `<text>`），这个没法 pattern 化，
 * 但只在机柜切换时变，不是 hover 级高频操作，可以接受。
 *
 * ## 为什么从柜顶往下画
 *
 * 真实机柜 U1 在**底部**。用户看这张图是为了"照着装机器"，
 * 所以视觉上方必须是 40U/42U。坐标系用 `y = (heightU - u) * unitH`。
 *
 * ## 拖拽为什么只放开 164 台
 *
 * 判据是「这台设备在意图里有没有逐台声明」：133 台交换机 + 31 台特殊设备
 * 有独立 `u` 字段；663 台（79.5%）由布局段按槽位序列自动排布。
 *
 * 布局段那批拖动时不再改布局段，而是往 `devices` 段写一条覆盖记录
 * （`instance` 是槽位 id，`expand_placements` 认它）—— 只挪一台，
 * 同段另外几十台不受影响。只剩 7 台源数据位置自身的矛盾的设备真正拖不动，
 * 它们压暗，点一下说明「要先去接线设计里补一条声明」。
 *
 * ## ★ 「改 U 位是安全的」这句话在 v0.12 被证伪了
 *
 * 早期判断是：同机柜两端 `path_units = 0`，所以跳线长度档位不变，
 * 拖动只影响占位冲突与越界。
 *
 * 这个推理只看到了**线长**，漏了**端口号**。端口分配规则 C2 是
 * `端口号 = 组内位次 + base`，而组内位次按 `(机柜升序, U 位升序)` 排 ——
 * 换柜或换到不同槽位就换位次，**同组排在它后面的设备端口号整体前移**。
 *
 * 实测（V13 真数据）：
 *
 * | 动作 | 端口重编号 | 线长变化 |
 * |---|---|---|
 * | 柜内 3U → 5U（同槽位） | 0 | 0 |
 * | 柜内 3U → 35U | 0 | 0 |
 * | 柜内 3U → 39U（落到第 10 槽） | **56 行 / 7 台交换机** | 6 |
 * | 跨柜 机柜1 → 机柜5 | **91 行 / 7 台交换机** | 6 |
 *
 * 所以「同柜内挪 = 安全」也是错觉：换个 U 就可能换位次。
 * 现在每次移动后端都返回 `impact`，界面在松手后立刻说出来，
 * 并提供一步撤销（`draft.undo()`）—— 用户看到 91 行会想撤，
 * 没有撤销的话他只能拖回去，而拖回去**又是一次** 91 行端口变化。
 *
 * ## 为什么用 pointer 事件而不是 HTML5 drag
 *
 * HTML5 drag-and-drop 在 SVG 元素上行为不一致（各浏览器对 `<g>` 的
 * draggable 支持差别大），且拖影是一整块 DOM 截图，位置与 U 格对不上。
 * pointer 事件自己算「鼠标落在第几 U」，落点即所见。
 */

import { useCallback, useMemo, useRef, useState } from 'react';
import type { JSX, PointerEvent as ReactPointerEvent } from 'react';

import {
  impactDetail,
  impactHeadline,
  impactSeverity,
  previewHeadline,
} from '../domain/impact';
import { useAppStore } from '../store/appStore';
import { useDragStore } from '../store/dragStore';
import {
  editableKey,
  parseEditableKey,
  useIntentDraft,
} from '../store/useIntentDraft';

/** 每 U 的像素高度。42U × 18px ≈ 756px，一屏能看全。 */
const UNIT_H = 18;
const LABEL_W = 34;
const RACK_W = 260;

/** 设备类型配色。按型号关键字分组——用户一眼区分服务器/交换机/安全设备。 */
function deviceColor(model: string): string {
  // 判定顺序必须与后端 `domain/exporting/svg.py::categorize()` 完全一致 ——
  // 屏幕上红色、纸上黄色，现场会以为拿错了图。
  if (model.includes('核心')) return 'var(--dev-core)';
  if (model.includes('接入') || model.includes('交换机')) return 'var(--dev-switch)';
  // 安全设备排在「服务器」之前：服务器密码机 / 签名服务器 名字里有「服务器」，
  // 但它们是画成红色的安全设备，不是计算节点。
  if (
    model.includes('防火墙') ||
    model.includes('负载') ||
    model.includes('密码') ||
    model.includes('签名') ||
    model.includes('加密') ||
    model.includes('密评')
  )
    return 'var(--dev-security)';
  if (model.includes('存储')) return 'var(--dev-storage)';
  if (model.includes('服务器') || model.includes('节点')) return 'var(--dev-server)';
  return 'var(--dev-other)';
}

export function RackView(): JSX.Element {
  const rack = useAppStore((s) => s.rack);
  const selection = useAppStore((s) => s.selection);
  const selectDevice = useAppStore((s) => s.selectDevice);
  const selectRack = useAppStore((s) => s.selectRack);

  // 深链 `?edit=1` 直接进入编辑模式 —— headless 截图没法点按钮，
  // 自动化验证也需要这条路（与 DesignView 的 `?edit=1` 同构）。
  const [editMode, setEditMode] = useState(
    () => new URLSearchParams(window.location.search).get('edit') === '1',
  );
  const draft = useIntentDraft(editMode);
  const [dragging, setDragging] = useState<string | null>(null);
  const [ghostU, setGhostU] = useState<number | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  /**
   * 多选中的设备键（Shift 点击加入/移出）。
   *
   * ## 为什么不框选
   *
   * 框选与拖拽手势冲突：按住左键拉一个矩形，和按住左键拖一台设备，
   * 前 100ms 完全一样，只能靠「拉起来的距离」区分 —— 用户想选 3 台
   * 结果动了其中一台。Shift 点击没有这个歧义，代价是多点几下。
   *
   * ## 为什么只有 1 台被选中时也走批量
   *
   * 拖动任意一台**被选中的**设备时整组一起动，包括只有 1 台时 ——
   * 两条代码路径（单台/批量）在「有没有多选」上分叉，比按数量分叉
   * 少一处状态不一致。单台的批量等价于它自己。
   */
  /**
   * 影响面文案。
   *
   * ## 为什么这里要 useMemo 而不是直接调
   *
   * 拖动过程中每次 pointermove 都会重渲染这个组件（幽灵方块要跟手）。
   * 影响面文案要遍历 samples 拼字符串，不该跟着 60fps 一起跑。
   *
   * 依赖只有 `draft.lastImpact` —— 它每次移动成功才换一个对象引用，
   * 拖动过程中的高频重渲染不会触发重算。
   */
  const impactBanner = useMemo(() => impactHeadline(draft.lastImpact), [draft.lastImpact]);
  const impactLines = useMemo(() => impactDetail(draft.lastImpact), [draft.lastImpact]);
  /** 拖动中的预演提示（**还没发生**，措辞与横幅不同）。 */
  const previewText = useMemo(() => previewHeadline(draft.preview), [draft.preview]);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const svgRef = useRef<SVGSVGElement | null>(null);
  const beginDrag = useDragStore((s) => s.begin);
  const endDrag = useDragStore((s) => s.end);

  const heightU = rack?.rackHeightU ?? 42;

  const conflictSet = useMemo(() => {
    const set = new Set<string>();
    for (const c of rack?.conflicts ?? []) {
      set.add(c.a);
      set.add(c.b);
    }
    return set;
  }, [rack]);

  /** 鼠标 y 坐标 → U 号（柜顶 42 往下）。 */
  const uAtPointer = useCallback(
    (clientY: number): number | null => {
      const svg = svgRef.current;
      if (!svg) return null;
      const box = svg.getBoundingClientRect();
      const y = clientY - box.top;
      if (y < 0 || y > box.height) return null;
      const row = Math.floor(y / UNIT_H);
      return Math.max(1, Math.min(heightU, heightU - row));
    },
    [heightU],
  );

  /**
   * 光标下面是哪个机柜（用于跨机柜拖动的落点判定）。
   *
   * ## 为什么不能用 pointerup 事件的目标元素
   *
   * 起点调了 `setPointerCapture`，捕获期间**所有** pointer 事件都被起点
   * 那个 `<g>` 吃掉 —— 平面图根本收不到 `pointerup`，靠事件冒泡拿目标
   * 会永远得到「还在原地」。`document.elementFromPoint` 是一次独立的
   * 命中测试，不受捕获影响，返回光标下真实那个元素。
   *
   * 依赖平面图每个机柜 `<rect>` 上的 `data-channel` / `data-rack`
   * （`FloorView` 已有，两处必须一起改，否则跨柜拖拽静默失效）。
   */
  const rackAtPointer = useCallback(
    (clientX: number, clientY: number): { channel: number; rack: number } | null => {
      const el = document.elementFromPoint(clientX, clientY);
      if (!el) return null;
      const holder = el.closest('[data-rack]');
      if (!holder) return null;
      const ch = Number(holder.getAttribute('data-channel'));
      const rk = Number(holder.getAttribute('data-rack'));
      if (!Number.isFinite(ch) || !Number.isFinite(rk)) return null;
      return { channel: ch, rack: rk };
    },
    [],
  );

  /** 拖动经过的落点机柜（仅用于界面提示，最终成败由后端校验）。 */
  const [dropRack, setDropRack] = useState<{ channel: number; rack: number } | null>(null);

  /**
   * 拖动过程中的**干跑节流器**。
   *
   * ## 为什么要节流，且阈值是 220ms
   *
   * 一次真实拖动会连发几十个 `pointermove`。每个都请求一次干跑，
   * 后端 `deep=false` 约 55~90ms，请求会排队堆积，最后回调的
   * 可能是几百毫秒前的位置 —— 提示就变成「过期的恐吓」。
   *
   * 220ms 是实测折中：比后端耗时宽一倍（不排队），又快到用户
   * 停手之前就能看到结论。
   *
   * ## 为什么只对「位置真的变了」发请求
   *
   * 鼠标在同 U 位内微动、或者落点机柜没变时，请求的答案是同一个。
   * 不比对就发，等于把一次拖动放大成几十次全量生成。
   */
  /**
   * 拖动中那条**预演节流状态**。
   *
   * ## 为什么用 ref 而不是 state
   *
   * 它只在拖动过程中被读、被写，从不驱动渲染。放 state 会让每次
   * `pointermove` 都重渲染整个 U 位图（42 个 U 号 + 全部方块）。
   */
  const preflightRef = useRef<{ at: number; key: string }>({ at: 0, key: '' });

  /**
   * ★ 正在拖的设备键 —— 给 `onPointerUp` 用 ref 读，不用 state。
   *
   * ## 这个 ref 修的是一个「拖动忽然完全不生效」的急 bug
   *
   * 预演每次回来都会 `setPreview(...)` 触发重渲染。而 `onPointerUp` 的
   * 闭包捕获的是**那一次渲染时**的 `dragging` state。若重渲染发生在
   * 松手前的最后一刻，闭包里的 `dragging` 还是旧值：
   *
   * ```tsx
   * onPointerUp={(e) => { if (dragging !== d.deviceKey) return; ... }}
   * //                     ↑ 旧闭包里可能是 null 或上一台的键 → 直接 return
   * ```
   *
   * 表现是**松手什么都没发生**：不发请求、不报错、notice 空着 ——
   * 极难排查（我一开始以为是 CDP 脚本的问题，查了三轮）。
   *
   * ref 的值永远是最新的，不受闭包快照影响。
   */
  const draggingRef = useRef<string | null>(null);
  const maybePreflight = useCallback(
    (d: { displayName: string; startU: number }, u: number | null, over: { channel: number; rack: number } | null) => {
      if (!draft.doc || u === null || !rack) return;
      // 拖动期间不做跨通道预演：那条路必被拒，`finishDrag` 已经会说清楚，
      // 再弹一个「会改 X 行」的预演等于给一个不可能的动作用户看。
      if (over !== null && over.channel !== rack.channelNo) return;
      const targetRack = over !== null && over.rack !== rack.seqNo ? over.rack : undefined;
      const key = `${u}|${targetRack ?? ''}`;
      const now = Date.now();
      if (key === preflightRef.current.key && now - preflightRef.current.at < 1200) return;
      if (now - preflightRef.current.at < 220) return;
      preflightRef.current = { at: now, key };
      void draft.preflight({
        channel: rack.channelNo,
        rack: rack.seqNo,
        instance: d.displayName,
        newU: u,
        currentU: d.startU,
        ...(targetRack === undefined ? {} : { targetRack }),
      });
    },
    [draft, rack],
  );

  /**
   * 松手落地。分三种情况：批量、跨机柜、同柜调 U 位。
   *
   * ## 批量为什么放在最前面
   *
   * 拖动一台**被选中的**设备时整组一起动（含只选中 1 台）。批量路径
   * 与单台路径的落点语义不同：批量是「整体平移 deltaU」，所以落点要
   * 先换算成 delta（`落点U - 起点U`），而不是直接给每台一个新 U 位。
   */
  const finishDrag = useCallback(
    async (
      deviceKey: string,
      dragChannel: number,
      dragRack: number,
      deviceName: string,
      sizeU: number,
      clientX: number,
      clientY: number,
      startU: number,
    ) => {
      if (!rack) return;
      draggingRef.current = null;
      setDragging(null);
      setGhostU(null);
      setDropRack(null);
      endDrag();
      // ★ 松手就清预演，**不看这次移动成不成功**。
      // 预演是「还没发生」的话；松手之后它要么已经发生、要么根本没发生，
      // 两种情况下继续挂着一句「松手会改 21 行」都是在说胡话。
      // 只在成功分支清会漏掉失败与取消（第一版就是这么漏的）。
      draft.clearPreview();
      preflightRef.current = { at: 0, key: '' };

      const drop = rackAtPointer(clientX, clientY);
      const target = uAtPointer(clientY);

      // ---- 批量：拖动的是多选中的一台 ----
      const draggingSelected = selected.has(deviceKey);
      const group = draggingSelected ? [...selected] : [];
      if (draggingSelected && group.length > 0) {
        const crossingGroup =
          drop !== null && drop.rack !== dragRack && drop.channel === dragChannel;
        if (drop !== null && drop.channel !== dragChannel) {
          setNotice(
            `不能拖到通道${drop.channel} —— 换通道等于换一组 ToR 来服务它，那是重新规划。` +
              '请在「接线设计」里改布局段。',
          );
          return;
        }
        // 跨柜：整组落到目标柜并保持各自 U 位（delta=0）；
        // 同柜：delta 由落点与拖动那台的原 U 位算出来。
        const deltaU = crossingGroup
          ? 0
          : target === null
            ? 0
            : Math.max(
                1 - startU,
                Math.min(heightU - startU - sizeU + 1, target - sizeU + 1 - startU),
              );
        if (!crossingGroup && deltaU === 0) return;

        const targets = group
          .map((key) => parseEditableKey(key))
          .filter((x): x is NonNullable<typeof x> => x !== null)
          .map((x) => ({
            channel: x.channel,
            rack: x.rack,
            instance: x.instance,
            currentU: x.uStart,
          }));
        if (targets.length === 0) return;
        const res = await draft.batchMove(
          targets,
          deltaU,
          crossingGroup && drop !== null ? drop.rack : undefined,
        );
        setNotice(
          res.ok
            ? crossingGroup && drop !== null
              ? `已把 ${targets.length} 台移到通道${drop.channel} 机柜${drop.rack}（未保存）`
              : `已把 ${targets.length} 台整体平移 ${deltaU > 0 ? '+' : ''}${deltaU}U（未保存）`
            : (res.error ?? '批量移动失败'),
        );
        if (res.ok && crossingGroup && drop !== null) {
          await selectRack(drop.channel, drop.rack);
        }
        if (res.ok) setSelected(new Set());
        return;
      }

      const crossing =
        drop !== null &&
        drop.rack !== rack.seqNo &&
        drop.channel === rack.channelNo;

      // 跨机柜：落点是平面图上一个格子，那里没有「第几 U」，
      // 所以保持设备当前的 U 位 —— 这是用户唯一可预期的落点。
      // 若那位已被占，后端会拒绝并说清是谁占着，用户再点进目标机柜微调。
      if (crossing) {
        const res = await draft.move(
          rack.channelNo,
          rack.seqNo,
          deviceName,
          startU,
          false,
          startU,
          drop.rack,
        );
        setNotice(
          res.ok
            ? `已移到通道${drop.channel} 机柜${drop.rack} 的 ${startU}U（未保存）`
            : (res.error ?? '跨机柜移动失败'),
        );
        // 成功后跳到目标机柜：不跳的话用户眼前这个柜里那台已经「消失」
        // （被 movedAway 滤掉），而新柜里它长什么样完全看不到 ——
        // 感觉像拖丢了。跳过去才形成「拖走 → 落在那边」的闭环。
        if (res.ok) await selectRack(drop.channel, drop.rack);
        return;
      }

      // 落在别的通道的机柜上：后端也只允许同通道换柜，这里先给出人话，
      // 别让用户等到一个「通道2 里没有机柜7 的排布声明」才明白。
      if (drop !== null && drop.channel !== rack.channelNo) {
        setNotice(
          `不能拖到通道${drop.channel} —— 换通道等于换一组 ToR 来服务它，那是重新规划。` +
            '请在「接线设计」里改布局段。',
        );
        return;
      }

      if (target === null) return;
      // 落点按「鼠标所在那一格是设备底边」对齐：方块跟着鼠标走，
      // 鼠标位置对应设备的底边（U 号小的那头在下）。
      const newU = Math.max(1, Math.min(heightU - sizeU + 1, target - sizeU + 1));
      const res = await draft.move(
        rack.channelNo,
        rack.seqNo,
        deviceName,
        newU,
        false,
        startU,
      );
      if (res.ok) {
        setNotice(
          res.overlaps.length > 0
            ? `已移到 ${newU}U，但与 ${res.overlaps.join('、')} 重叠`
            : `已移到 ${newU}U（未保存）`,
        );
      } else {
        setNotice(res.error ?? '移动失败');
      }
    },
    [draft, endDrag, heightU, rack, rackAtPointer, selectRack, selected, uAtPointer],
  );

  /**
   * `finishDrag` 的 ref 镜像 —— `onPointerUp` 一律走它，不走闭包直调。
   *
   * ## 为什么必须这样（这一条修的是个很隐蔽的急 bug）
   *
   * `finishDrag` 是 `useCallback`，依赖里含 `draft`。预演每次请求回来都会
   * `setPreview(...)` → `draft` 是新对象 → `finishDrag` 重建 → 重新绑定
   * `onPointerUp`。而浏览器里**已经发生**的那次 pointerdown 之后，
   * 松手时的 `pointerup` 用的仍是**按下时那一次渲染**的 handler：
   * 它闭包里的 `draft` 是旧的，`applyCell`/`move` 都指向旧状态。
   *
   * 症状极其迷惑：**松手什么都不发生** —— 不发请求、不报错、notice 空着。
   * 我先后怀疑过 CDP 脚本、怀疑过 `uAtPointer` 越界、怀疑过 pointer capture，
   * 查了三轮才定位到是闭包快照。`ref` 永远指向最新那份，不受此影响。
   */
  const finishDragRef = useRef(finishDrag);
  finishDragRef.current = finishDrag;

  if (!rack) {
    return <div className="view-placeholder">从平面图选择一个机柜</div>;
  }

  const bodyH = heightU * UNIT_H;

  return (
    <div className="rack-view">
      <header className="rack-header">
        <div>
          <h3>
            通道 {rack.channelNo} / 机柜 {rack.seqNo}
          </h3>
          <p className="subtle">
            {rack.roomCode} 房间 · {rack.side === 'N' ? '北侧' : '南侧'} · 占地列 {rack.col} / 行{' '}
            {rack.row}
          </p>
        </div>
        <div className="rack-stats">
          <span>
            <b>{rack.devices.length}</b> 台设备
          </span>
          <span>
            最高 <b>{rack.maxUsedU}U</b>
          </span>
          <span>
            连线 <b>{rack.cables.total}</b>
          </span>
          {rack.conflicts.length > 0 && (
            <span className="stat-warn">
              <b>{rack.conflicts.length}</b> 处 U 位重叠
            </span>
          )}
          <button
            type="button"
            className={`btn-mini${editMode ? ' active' : ''}`}
            onClick={() => {
              setEditMode((v) => !v);
              setNotice(null);
            }}
            title="编辑模式下可直接拖动设备改 U 位；改的是意图，未保存不影响存量表"
          >
            {editMode ? '退出编辑' : '编辑 U 位'}
          </button>
          <a
            className="rack-export"
            href={`/api/draw/racks.html?channel=${rack.channelNo}&rack=${rack.seqNo}`}
            target="_blank"
            rel="noreferrer"
            title="导出本柜 U 位图（白底、A3 横向、可直接打印）"
          >
            导出本柜
          </a>
        </div>
      </header>

      {editMode && (
        <div className={`draft-bar${draft.dirty ? ' dirty' : ''}`}>
          {draft.loading && <span>正在加载意图…</span>}
          {draft.error && <span className="draft-error">意图加载失败：{draft.error}</span>}
          {!draft.loading && !draft.error && (
            <>
              <span>
                可拖 <b>{draft.editable.size}</b> 台
                <span className="subtle">
                  （交换机与特殊设备直接改 U 位；布局段排布的会记一条覆盖记录）
                </span>
              </span>
              {draft.project && <span className="subtle">项目 {draft.project}</span>}
              {draft.dirty && <span className="draft-dirty">有未保存的改动</span>}
              <button
                type="button"
                className="btn-mini"
                disabled={!draft.dirty}
                onClick={() => {
                  void draft.save('U 位图拖拽调整').then((r) => setNotice(r.message));
                }}
              >
                保存为版本
              </button>
            </>
          )}
        </div>
      )}

      {notice && <div className="draft-notice">{notice}</div>}

      {/*
        拖动中的预演（**还没发生**）。

        ## 为什么它必须在松手之前出现

        影响面横幅是松手后才有的 —— 那时已经改了。用户看到「91 行端口号变了」
        想反悔只能撤销，而撤销本身也是一次移动（又是 91 行）。

        预演让他在**手指还没松开**时就知道后果，可以直接拖到别处。

        ## 为什么文案是「松手会」而不是「这次改动影响」

        同一批数字，两句话的意思完全不同：前者是预测、还能改；后者是既成事实、
        只能撤销。混用会让用户错过唯一还能反悔的窗口。
      */}
      {previewText && (
        <div className="draft-notice impact-preview">
          <div className="impact-head">{previewText}</div>
        </div>
      )}

      {/*
        影响面横幅（v0.12）。

        ## 为什么它在 notice 下面而不是弹窗

        拖动是连续动作，弹窗会把「再拖一次」变成「先关掉弹窗」。
        横幅能同时容纳「已移到哪」和「还动了什么」，也留着撤销按钮。

        ## 为什么只显示三行明细

        跨柜拖一台会改 91 行，全列出来没人看。只给最要紧的三条
        （哪台交换机、哪个端口改名、哪根线变长），完整数据在
        「接线设计」页的布线表里能看到 —— 那才是用户核对的地方。
      */}
      {impactBanner && (
        <div className={`draft-notice impact-${impactSeverity(draft.lastImpact)}`}>
          <div className="impact-head">{impactBanner}</div>
          {impactLines.length > 0 && (
            <ul className="impact-lines">
              {impactLines.map((line, i) => (
                <li key={i}>{line}</li>
              ))}
            </ul>
          )}
          <div className="impact-actions">
            <button
              type="button"
              className="btn-mini"
              onClick={() => {
                if (draft.undo()) setNotice('已撤销上一次拖动（回到拖动前的意图）');
              }}
            >
              撤销这次拖动
            </button>
            <span className="subtle">
              改动仍在草稿里，点「保存为版本」才会落库
            </span>
          </div>
        </div>
      )}

      {/*
        多选状态条。没有它，用户 Shift 点了几下完全看不出发生了什么 ——
        只有方块边缘一圈虚线，很容易以为是 hover 效果。
        「清空」按钮必须有：选错了 20 台却只能一台台 Shift 点回去，没人受得了。
      */}
      {editMode && selected.size > 0 && (
        <div className="draft-notice group-bar">
          已选 <b>{selected.size}</b> 台 —— 拖动其中任意一台，整组一起平移同样的格数
          <button
            type="button"
            className="btn-mini"
            onClick={() => setSelected(new Set())}
          >
            清空选择
          </button>
        </div>
      )}

      {/*
        拖动时提示光标下的机柜。跨机柜拖动时鬼影线会跑到 U 位图外，
        用户看到的是平面图上的高亮 —— 这一行是文字兜底，也让「拖到了
        别的通道」这种会被拒绝的情况在松手前就看得见。
      */}
      {dropRack !== null && dropRack.channel !== rack.channelNo && (
        <div className="draft-notice warn">
          光标在通道{dropRack.channel} 机柜{dropRack.rack} —— 不能跨通道（换通道等于重组 ToR）
        </div>
      )}
      {dropRack !== null &&
        dropRack.channel === rack.channelNo &&
        dropRack.rack !== rack.seqNo && (
          <div className="draft-notice">
            松手将移到通道{dropRack.channel} 机柜{dropRack.rack}（保持当前 U 位）
          </div>
        )}

      <div className="rack-body-wrap">
        <svg
          ref={svgRef}
          width={LABEL_W + RACK_W + 20}
          height={bodyH + 8}
          role="img"
          aria-label={`机柜 U 位图 通道${rack.channelNo}机柜${rack.seqNo}`}
        >
          <defs>
            {/* 一条横线 = 1U。整张底图靠这个 pattern 铺满，只占 1 个节点 */}
            <pattern
              id="u-grid"
              width={RACK_W}
              height={UNIT_H}
              patternUnits="userSpaceOnUse"
            >
              <line
                x1={0}
                y1={UNIT_H}
                x2={RACK_W}
                y2={UNIT_H}
                stroke="var(--u-line)"
                strokeWidth={1}
              />
            </pattern>
          </defs>

          {/* 机柜外框 + U 网格 */}
          <rect
            x={LABEL_W}
            y={0}
            width={RACK_W}
            height={bodyH}
            className="rack-frame"
            fill="url(#u-grid)"
          />

          {/* U 号：从柜顶 42 往下标。每 5U 加粗，方便数格 */}
          {Array.from({ length: heightU }, (_, i) => {
            const u = heightU - i;
            const y = i * UNIT_H + UNIT_H - 4;
            const major = u % 5 === 0;
            return (
              <text
                key={u}
                x={LABEL_W - 6}
                y={y}
                textAnchor="end"
                className={`u-label${major ? ' major' : ''}`}
              >
                {u}
              </text>
            );
          })}

          {/* 拖拽落点提示线 */}
          {ghostU !== null && (
            <line
              x1={LABEL_W}
              y1={(heightU - ghostU + 1) * UNIT_H}
              x2={LABEL_W + RACK_W}
              y2={(heightU - ghostU + 1) * UNIT_H}
              className="drop-guide"
            />
          )}

          {rack.devices
            .filter(
              (d) =>
                !draft.movedAway(rack.channelNo, rack.seqNo, d.displayName, d.startU),
            )
            .map((d) => {
            const pos = draft.movedPos(rack.channelNo, rack.seqNo, d.displayName, d.startU);
            const startU = pos?.startU ?? d.startU;
            const endU = pos?.endU ?? d.endU;
            const sizeU = pos?.sizeU ?? d.sizeU;
            const topY = (heightU - endU) * UNIT_H;
            const h = sizeU * UNIT_H - 2;
            const isSelected = selection.deviceName === d.displayName;
            const hasConflict = conflictSet.has(d.model);
            const canDrag = draft.editable.has(
              editableKey(rack.channelNo, rack.seqNo, d.displayName, d.startU),
            );
            // 键必须用 `deviceKey`（含 U 位）：同机柜 8 台计算节点的
            // `displayName` 完全相同，用名字会让拖一台时 8 台一起高亮。
            const isDragging = dragging === d.deviceKey;
            return (
              <g
                key={d.deviceKey}
                className={`device-block${editMode ? (canDrag ? ' draggable' : ' locked') : ''}${
                  isDragging ? ' dragging' : ''
                }${editMode && selected.has(d.deviceKey) ? ' group-selected' : ''}`}
                onClick={(event) => {
                  // Shift 点击 = 加入/移出多选组。普通点击照旧「选中这台
                  // 看它的连线」—— 两个动作在编辑模式下不会互相干扰，
                  // 因为多选只在编辑模式有意义。
                  if (editMode && canDrag && event.shiftKey) {
                    event.stopPropagation();
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (next.has(d.deviceKey)) next.delete(d.deviceKey);
                      else next.add(d.deviceKey);
                      return next;
                    });
                    return;
                  }
                  void selectDevice(rack.channelNo, rack.seqNo, d.displayName);
                }}
                onPointerDown={(event: ReactPointerEvent<SVGGElement>) => {
                  if (!editMode) return;
                  if (!canDrag) {
                    setNotice(
                      `${d.displayName} 在意图里找不到对应声明（源数据自身位置矛盾的那几台），` +
                        '没法确定该改哪一行。请先在「接线设计」里补一条设备声明。',
                    );
                    return;
                  }
                  event.preventDefault();
                  // 指针捕获：拖快一点鼠标就会离开这个方块。不捕获的话
                  // `pointerup` 落在别处，拖拽永远结束不了 —— 方块粘在鼠标上。
                  event.currentTarget.setPointerCapture(event.pointerId);
                  setDragging(d.deviceKey);
                  draggingRef.current = d.deviceKey;
                  setGhostU(uAtPointer(event.clientY));
                  // 告诉平面图「我在拖这台」—— 它据此把同通道的其他机柜
                  // 标成可用落点。跨视图只能靠共享 store。
                  beginDrag({
                    channel: rack.channelNo,
                    rack: rack.seqNo,
                    name: d.displayName,
                    startU: d.startU,
                    sizeU,
                  });
                }}
                onPointerMove={(event: ReactPointerEvent<SVGGElement>) => {
                  if (draggingRef.current !== d.deviceKey) return;
                  const u = uAtPointer(event.clientY);
                  setGhostU(u);
                  // 光标已经移出 U 位图时，鬼影线会消失，此时靠平面图
                  // 的 `drag-over` 高亮告诉用户能落哪。
                  const over = rackAtPointer(event.clientX, event.clientY);
                  setDropRack(over);
                  maybePreflight(d, u, over);
                }}
                onPointerCancel={() => {
                  // 系统手势/焦点切换会打断拖拽：清干净，不留半拖状态。
                  // 这条路径不经过 `finishDrag`，所以预演要单独清 ——
                  // 漏了的症状是「拖到一半被系统打断，『松手会改…』一直挂着」。
                  if (draggingRef.current !== d.deviceKey) return;
                  draggingRef.current = null;
                  setDragging(null);
                  setGhostU(null);
                  setDropRack(null);
                  endDrag();
                  draft.clearPreview();
                  preflightRef.current = { at: 0, key: '' };
                }}
                onPointerUp={(event: ReactPointerEvent<SVGGElement>) => {
                  // 用 ref 而不是 `dragging` state —— 见 `draggingRef` 的说明。
                  if (draggingRef.current !== d.deviceKey) return;
                  draggingRef.current = null;
                  event.currentTarget.releasePointerCapture(event.pointerId);
                  void finishDragRef.current(
                    d.deviceKey,
                    rack.channelNo,
                    rack.seqNo,
                    d.displayName,
                    sizeU,
                    event.clientX,
                    event.clientY,
                    startU,
                  );
                }}
              >
                <rect
                  x={LABEL_W + 3}
                  y={topY + 1}
                  width={RACK_W - 6}
                  height={h}
                  rx={2}
                  className={`device-rect${isSelected ? ' selected' : ''}${
                    hasConflict ? ' conflict' : ''
                  }`}
                  style={{ fill: deviceColor(d.model) }}
                >
                  <title>
                    {`${d.model}｜${d.displayName}｜${startU}-${endU}U（${sizeU}U）｜${d.portCount} 口` +
                      (editMode
                        ? canDrag
                          ? '｜可拖动'
                          : '｜意图里无声明，先补一条才能拖'
                        : '')}
                  </title>
                </rect>
                {sizeU >= 2 && (
                  <text
                    x={LABEL_W + 10}
                    y={topY + h / 2 + 4}
                    className="device-label"
                    pointerEvents="none"
                  >
                    {d.model.length > 12 ? `${d.model.slice(0, 12)}…` : d.model}
                    <tspan className="device-u">
                      {' '}
                      {startU}-{endU}U
                    </tspan>
                  </text>
                )}
              </g>
            );
          })}

          {/*
            跨机柜拖进来的设备：存量表里这台还在原柜，目标机柜的 payload
            里根本没有它。不补这一层，用户跨柜拖完看到的是「东西消失了」。
            只画位置和名字 —— 型号要重新查账，而草稿里正好没有。
          */}
          {draft.movedInto(rack.channelNo, rack.seqNo).map((m) => {
            const topY = (heightU - m.pos.endU) * UNIT_H;
            const h = m.pos.sizeU * UNIT_H - 2;
            return (
              <g key={`into-${m.name}-${m.pos.startU}`} className="device-block dragging">
                <rect
                  x={LABEL_W + 3}
                  y={topY + 1}
                  width={RACK_W - 6}
                  height={h}
                  rx={2}
                  className="device-rect moved-in"
                >
                  <title>{`${m.name}｜从其他机柜拖入 ${m.pos.startU}-${m.pos.endU}U（未保存）`}</title>
                </rect>
                {m.pos.sizeU >= 2 && (
                  <text
                    x={LABEL_W + 10}
                    y={topY + h / 2 + 4}
                    className="device-label"
                    pointerEvents="none"
                  >
                    {m.name.length > 12 ? `${m.name.slice(0, 12)}…` : m.name}
                    <tspan className="device-u">
                      {' '}
                      {m.pos.startU}-{m.pos.endU}U
                    </tspan>
                  </text>
                )}
              </g>
            );
          })}
        </svg>

        {rack.devices.length === 0 && (
          <div className="rack-empty-hint">该机柜还没有设备（平面图上有位置，规划中未使用）</div>
        )}
      </div>

      {rack.conflicts.length > 0 && (
        <div className="conflict-panel">
          <h4>U 位重叠（需人工裁决）</h4>
          <ul>
            {rack.conflicts.map((c, i) => (
              <li key={i}>
                <code>{c.a}</code> [{c.uRangeA.join('-')}U] 与 <code>{c.b}</code> [
                {c.uRangeB.join('-')}U]
                {c.isNameVariant && <span className="tag">名称变体（同一台设备两种写法）</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
