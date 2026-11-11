/**
 * U 位图拖拽编辑用的「意图草稿」。
 *
 * ## 为什么草稿不放进 appStore
 *
 * `appStore` 里放的是**已导入的存量布线表**（5127 行，从 Excel 读来的只读视图）。
 * 拖拽改的是**意图**（949 行，人写给程序的），两者是不同的东西：
 * 一个是"现在是什么样"，一个是"应该是什么样"。混在一个 store 里，
 * 后续任何取 `rack.devices` 的地方都要判断"这读到的是哪个"，很难不出错。
 *
 * 所以这里用组件级 hook：谁需要拖拽谁挂一份草稿，卸载即丢弃。
 * 存不存由用户按「保存为版本」决定 —— 拖 5 次看效果不该留 5 个版本。
 *
 * ## 为什么可拖清单由后端给
 *
 * 判据是「这台设备在意图里有没有逐台声明」：130 台交换机 + 34 台特殊设备
 * 有独立 `u` 字段，663 台（79.5%）由布局段按槽位序列自动排布 ——
 * 后者拖了也没有字段可写，重新生成会跳回原位。前端若自己按
 * 「在不在 switches 段里」判断，两套规则迟早长歪。
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import type { ImpactPayload } from '../api/client';
import { api } from '../api/client';

/**
 * 可拖设备的键。
 *
 * ## 为什么必须带 `uStart`
 *
 * 布局段排布的 663 台设备**没有实例名** —— 台账里它们的 `displayName`
 * 就是型号（「计算节点A（ARM）」），同一机柜 8 台全一样。只用名字做键，
 * 拖动一台会让 8 台一起动；判断「这台能不能拖」也会把可拖的当成不可拖。
 *
 * 加上起始 U 位后 `(通道, 机柜, 显示名, 起始U)` 四元组在真实意图里唯一。
 */
export function editableKey(
  channel: number,
  rack: number,
  instance: string,
  uStart: number,
): string {
  return `${channel}\u0001${rack}\u0001${uStart}\u0001${instance}`;
}

/** 把 `editableKey` 拆回四元组 —— 批量拖动要把选中的键还原成 targets。 */
export function parseEditableKey(key: string): {
  channel: number;
  rack: number;
  uStart: number;
  instance: string;
} | null {
  const parts = key.split('\u0001');
  if (parts.length !== 4) return null;
  const channel = Number(parts[0]);
  const rack = Number(parts[1]);
  const uStart = Number(parts[2]);
  if (!Number.isFinite(channel) || !Number.isFinite(rack) || !Number.isFinite(uStart)) {
    return null;
  }
  return { channel, rack, uStart, instance: parts[3] ?? '' };
}

export interface MoveOutcome {
  ok: boolean;
  error: string | null;
  overlaps: string[];
  /**
   * 这次移动动到的行（v0.12）。
   *
   * 拖动会改掉用户**没选中**的行：换柜后这台在端口组里的位次变了，
   * 同组排在它后面的设备端口号整体前移（实测 91 行）。界面必须把它说出来，
   * 否则用户以为只动了一台，现场按旧表插线全插错。
   */
  impact: ImpactPayload | null;
}

export interface IntentDraft {
  project: string | null;
  /** 意图文档（`/api/projects/{name}/intent` 的 doc） */
  doc: Record<string, unknown> | null;
  /** 可拖设备键集合 */
  editable: Set<string>;
  loading: boolean;
  error: string | null;
  /** 有过成功移动但还没保存 */
  dirty: boolean;
  /** 最近一次移动的结果，用于界面提示 */
  lastMove: { instance: string; newU: number; overlaps: string[] } | null;
  move: (
    channel: number,
    rack: number,
    instance: string,
    newU: number,
    allowOverlap?: boolean,
    currentU?: number,
    targetRack?: number,
  ) => Promise<MoveOutcome>;
  /**
   * 一批设备**整体**平移 `deltaU`（可同时换机柜）。
   *
   * ## 为什么不循环调 `move`
   *
   * 8 台整体下移一格时每台的新位置恰好是下一台的旧位置，逐个调会让
   * 第 1 台看到第 2 台还站在原地 → 一台都动不了。后端 `move-batch`
   * 先把这一批从占用清单里整体摘掉再统一校验。
   */
  batchMove: (
    targets: {
      channel: number;
      rack: number;
      instance: string;
      currentU?: number;
    }[],
    deltaU: number,
    targetRack?: number,
  ) => Promise<MoveOutcome>;
  save: (note: string) => Promise<{ ok: boolean; message: string }>;
  reset: () => void;
  /**
   * 最近一次移动的影响面；没有移动过时为 null。
   *
   * 与 `lastMove` 分开：`lastMove` 只记「哪台挪到哪」（用来画方块），
   * `lastImpact` 是「这次改动波及了哪些行」（用来提示与决定要不要撤销）。
   */
  lastImpact: ImpactPayload | null;
  /**
   * **干跑预演**：松手之前问一句「会动到哪些行」，不改文档。
   *
   * ## 为什么这件事必须在松手之前
   *
   * 影响面横幅是松手后才出现的 —— 那时已经改了。用户看到「91 行端口号变了」
   * 想反悔，只能靠撤销。而撤销本身也是一次移动（又是 91 行），
   * 且他要凭记忆判断「原本是什么样」。
   *
   * 拖动过程中问一句，用户可以在**手指还没松开**时就决定继续或放弃。
   * 这不是性能优化，是知情同意。
   *
   * 失败时返回 null（不打扰用户）—— 预演是辅助信息，不该因为它
   * 报错就让拖动卡住。
   */
  preflight: (body: {
    channel: number;
    rack: number;
    instance: string;
    newU: number;
    currentU?: number;
    targetRack?: number;
  }) => Promise<ImpactPayload | null>;
  /**
   * 拖动过程中的预演结果（`deep=false`，只含端口与位置）。
   *
   * 与 `lastImpact` 分开：`lastImpact` 是**已经发生**的改动，
   * `preview` 是**还没发生**的。界面用不同颜色与措辞区分这两件事 ——
   * 混成一个字段，用户分不清「已经改了」和「松手会改」。
   */
  preview: ImpactPayload | null;
  /**
   * 清掉拖动中的预演。
   *
   * ## 为什么需要它（而不只是「移动成功后 setPreview(null)」）
   *
   * 预演是**拖动过程中**的状态。松手之后无论移动成功、失败、还是被
   * 用户取消（拖到空白处松手），预演都必须消失 —— 否则屏幕上会一直挂着
   * 一句「松手会改 21 行」，而那件事要么已经做完、要么根本没发生。
   *
   * 只在成功分支清是不够的：失败分支与取消分支都要清，而在 store 里
   * 各处散着写 `setPreview(null)` 一定会漏（第一版就漏了批量路径）。
   */
  clearPreview: () => void;
  /** 撤销最近一次移动，回到拖之前那一版文档。
   *
   * ## 为什么必须有撤销
   *
   * 影响面提示告诉用户「这次会改 91 行端口号」，但他点下去之前看不到结果。
   * 若拖完发现不对却只能自己拖回去，拖回去又是一次移动 —— 又是 91 行端口变化，
   * 而且这次连「原本是什么样」都得靠记忆。所以草稿里留一份上一版快照。
   *
   * 只留**一步**：更多步要的是版本库（「保存为版本」已经能做），
   * 在草稿里做多步撤销栈会让「现在到底改了什么」更难说清。
   */
  undo: () => boolean;
  /** 拖过但还没保存的设备的当前位置（布局段设备没有实例名，靠四元组定位）。 */
  movedPos: (
    channel: number,
    rack: number,
    name: string,
    startU: number,
  ) => DraftPos | undefined;
  /**
   * 这台设备是否已经被拖到**别的机柜**去了。
   *
   * 换柜拖拽后，原机柜的 payload 里它还在（payload 来自存量表，不是草稿），
   * 不滤掉就是「两个机柜里各有一台」。只靠 `movedPos` 返回 undefined
   * 表达不了这件事 —— undefined 的含义是「没动过，按原位置画」。
   */
  movedAway: (channel: number, rack: number, name: string, startU: number) => boolean;
  /**
   * 被拖进这个机柜、但存量表里还不存在的设备。
   *
   * U 位图画的是存量表推出来的位置，跨柜拖进来的那台在目标机柜的
   * payload 里根本没有 —— 不补这一层，用户跨柜拖完看到的是「东西消失了」。
   */
  movedInto: (channel: number, rack: number) => { name: string; pos: DraftPos }[];
}

/** 一次成功移动留下的痕迹。 */
export interface MovedEntry {
  channel: number;
  /** 拖之前所在机柜。 */
  fromRack: number;
  /** 拖之后所在机柜。同柜内调 U 位时与 `fromRack` 相同。 */
  toRack: number;
  name: string;
  /** 拖动前的起始 U 位。用它和「第一次拖的那台」对齐。 */
  origU: number;
  pos: DraftPos;
}

export function useIntentDraft(enabled: boolean): IntentDraft {
  const [project, setProject] = useState<string | null>(null);
  const [doc, setDoc] = useState<Record<string, unknown> | null>(null);
  const [editable, setEditable] = useState<Set<string>>(() => new Set());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [lastMove, setLastMove] = useState<IntentDraft['lastMove']>(null);
  const [moved, setMoved] = useState<MovedEntry[]>([]);
  const [lastImpact, setLastImpact] = useState<ImpactPayload | null>(null);
  const [preview, setPreview] = useState<ImpactPayload | null>(null);
  //: 预演的在途请求号。只认最后一次的响应 —— 拖动会连发几十个 pointermove，
  //: 先发的可能后到（网络乱序），不过滤会让提示在「91 行」与「0 行」之间乱跳。
  const preflightSeq = useRef(0);
  //: 移动前的文档快照（只留一步）。`null` = 没有可撤销的动作。
  const undoRef = useRef<{
    doc: Record<string, unknown>;
    moved: MovedEntry[];
    dirty: boolean;
  } | null>(null);
  const loadedRef = useRef(false);

  // 首次挂载时加载：项目 → 最新意图 → 可拖清单。
  // 三者并行不行：要先用项目名才能拉意图和清单。
  useEffect(() => {
    if (!enabled || loadedRef.current) return;
    loadedRef.current = true;
    let alive = true;
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        const projects = await api.projects();
        const first = projects.projects[0];
        if (!first) throw new Error('项目库里还没有项目，先在「项目与版本」页创建一个');
        const [intent, edit] = await Promise.all([
          api.projectIntent(first.name),
          api.intentEditable(),
        ]);
        if (!alive) return;
        setProject(first.name);
        setDoc(intent.doc as Record<string, unknown>);
        setEditable(
          new Set(
            edit.items.map((i) => editableKey(i.channel, i.rack, i.instance, i.uStart)),
          ),
        );
      } catch (exc) {
        if (alive) setError(exc instanceof Error ? exc.message : String(exc));
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [enabled]);

  const move = useCallback<IntentDraft['move']>(
    async (channel, rack, instance, newU, allowOverlap = false, currentU, targetRack) => {
      if (!doc) return { ok: false, error: '意图还没加载完成', overlaps: [], impact: null };
      const destRack = targetRack ?? rack;
      try {
        const res = await api.intentMove({
          doc,
          channel,
          rack,
          instance,
          newU,
          allowOverlap,
          ...(currentU === undefined ? {} : { currentU }),
          ...(targetRack === undefined ? {} : { targetRack }),
        });
        if (res.ok) {
          // 撤销快照必须在 `setDoc` **之前**存：之后 doc 就变成新版本了。
          undoRef.current = { doc, moved, dirty };
          // 后端返回的是新 doc（失败时原样回旧 doc，所以只在成功时替换）
          setDoc(res.doc as Record<string, unknown>);
          setDirty(true);
          setLastMove({ instance, newU, overlaps: res.overlaps });
          setLastImpact(res.impact ?? null);
          // 预演作废：实际结果已经出来了，再显示「松手会改…」就是重复且过期的
          setPreview(null);
          // 记下这台的新位置：U 位图画的是**存量表**推出来的位置，
          // 不叠这层，用户松手方块会弹回原位。
          const size = res.uSpan ?? 1;
          const landedRack = res.rackNo ?? destRack;
          setMoved((prev) => {
            const pos = { startU: newU, endU: newU + size - 1, sizeU: size };
            // 同一台第二次拖：按「名字 + 它现在的位置」找回上一条，原地更新，
            // 否则每次拖动都会多出一条，origU 也永远停在第 3U。
            //
            // `toRack === rack` 是必要条件：换柜拖过的设备已经不在原柜了，
            // 若只用名字 + U 位匹配，会把原柜里**另一台**同名设备（同柜 8 台
            // 计算节点全同名）当成它，导致拖 A 柜的第 3 台却改了 B 柜。
            const hit = prev.find(
              (e) =>
                e.channel === channel &&
                e.toRack === rack &&
                e.name === instance &&
                (e.origU === currentU || e.pos.startU === currentU),
            );
            if (hit) {
              return prev.map((e) =>
                e === hit ? { ...e, toRack: landedRack, pos } : e,
              );
            }
            return [
              ...prev,
              {
                channel,
                fromRack: rack,
                toRack: landedRack,
                name: instance,
                origU: currentU ?? -1,
                pos,
              },
            ];
          });
        }
        return {
          ok: res.ok,
          error: res.error,
          overlaps: res.overlaps,
          impact: res.impact ?? null,
        };
      } catch (exc) {
        return {
          ok: false,
          error: exc instanceof Error ? exc.message : String(exc),
          overlaps: [],
          impact: null,
        };
      }
    },
    [doc, moved, dirty],
  );

  const batchMove = useCallback<IntentDraft['batchMove']>(
    async (targets, deltaU, targetRack) => {
      if (!doc) return { ok: false, error: '意图还没加载完成', overlaps: [], impact: null };
      if (targets.length === 0) return { ok: true, error: null, overlaps: [], impact: null };
      try {
        const res = await api.intentMoveBatch({
          doc,
          targets,
          deltaU,
          ...(targetRack === undefined ? {} : { targetRack }),
        });
        if (res.ok) {
          undoRef.current = { doc, moved, dirty };
          setDoc(res.doc as Record<string, unknown>);
          setDirty(true);
          setLastImpact(res.impact ?? null);
          // 与单台路径同理：实际结果出来了，预演必须作废。
          // 漏了这句的症状是「松手后还挂着『松手会改 21 行』」——
          // 那句已经是过去时，却看着像还在等用户确认。
          setPreview(null);
          setLastMove({
            instance: `${res.moved.length} 台`,
            newU: deltaU,
            overlaps: res.overlaps,
          });
          // 后端 `moved` 只回 (instance, fromRack, fromU, toRack, toU) —— 没带
          // 通道。从 targets 里按「实例名 + 原机柜 + 原U位」反查，因为批量
          // 成员本来就只在同一通道（后端拒绝跨通道），也可以直接取 targets[0]。
          const channelOf = (name: string, fromRack: number): number => {
            const hit = targets.find(
              (t) => t.instance === name && t.rack === fromRack,
            );
            return hit ? hit.channel : 0;
          };
          setMoved((prev) => {
            let next = prev;
            for (const m of res.moved) {
              const channel = channelOf(m.instance, m.fromRack);
              // U 高度未知（布局段设备台账里没有）：沿用同一台之前的记录，
              // 没有就按 2U。方块真实高度以 payload 的 startU/endU 为准，
              // 这里只影响草稿层的提示。
              const prevEntry = next.find(
                (e) =>
                  e.channel === channel &&
                  e.name === m.instance &&
                  e.toRack === m.fromRack &&
                  (e.origU === m.fromU || e.pos.startU === m.fromU),
              );
              const size = prevEntry ? prevEntry.pos.sizeU : 2;
              const pos = { startU: m.toU, endU: m.toU + size - 1, sizeU: size };
              next = prevEntry
                ? next.map((e) =>
                    e === prevEntry ? { ...e, toRack: m.toRack, pos } : e,
                  )
                : [
                    ...next,
                    {
                      channel,
                      fromRack: m.fromRack,
                      toRack: m.toRack,
                      name: m.instance,
                      origU: m.fromU,
                      pos,
                    },
                  ];
            }
            return next;
          });
        }
        return {
          ok: res.ok,
          error: res.error,
          overlaps: res.overlaps,
          impact: res.impact ?? null,
        };
      } catch (exc) {
        return {
          ok: false,
          error: exc instanceof Error ? exc.message : String(exc),
          overlaps: [],
          impact: null,
        };
      }
    },
    [doc, moved, dirty],
  );

  const preflight = useCallback<IntentDraft['preflight']>(
    async (body) => {
      if (!doc) return null;
      const seq = ++preflightSeq.current;
      try {
        const res = await api.intentImpact({
          doc,
          move: body,
          // 拖动过程中不算线长（约 90ms → 55ms）。线长要等松手后
          // 真正的 move 响应才准 —— 那时才值得多等 40ms。
          deep: false,
        });
        // 迟到的响应直接丢掉：拖动已经有新的位置了，旧结论会误导。
        if (seq !== preflightSeq.current) return null;
        setPreview(res.ok ? res : null);
        return res;
      } catch {
        if (seq === preflightSeq.current) setPreview(null);
        return null;
      }
    },
    [doc],
  );

  const clearPreview = useCallback(() => {
    setPreview(null);
    // 让在途的预演请求作废：它们的响应回来时若还认这个序号，
    // 就会把已经清掉的提示又写回去（症状是松手后横幅自己冒出来）。
    preflightSeq.current += 1;
  }, []);

  const save = useCallback<IntentDraft['save']>(
    async (note) => {
      if (!project || !doc) return { ok: false, message: '没有可保存的内容' };
      try {
        const res = await api.saveProjectIntent(project, doc, note);
        setDirty(false);
        return {
          ok: true,
          message: res.isNew
            ? `已存为 v${res.version}（${res.summary.total} 行意图）`
            : `内容未变化，仍是 v${res.version}`,
        };
      } catch (exc) {
        return { ok: false, message: exc instanceof Error ? exc.message : String(exc) };
      }
    },
    [project, doc],
  );

  const reset = useCallback(() => {
    loadedRef.current = false;
    setDoc(null);
    setDirty(false);
    setLastMove(null);
    setLastImpact(null);
    setPreview(null);
    setError(null);
    setMoved([]);
    undoRef.current = null;
  }, []);

  const undo = useCallback(() => {
    const snap = undoRef.current;
    if (!snap) return false;
    setDoc(snap.doc);
    setMoved(snap.moved);
    setDirty(snap.dirty);
    // 撤销之后「最近一次影响面」就作废了 —— 留着会让横幅继续显示
    // 一个已经不存在的改动的后果。下一步撤销没有更多快照（只留一步）。
    setLastImpact(null);
    setLastMove(null);
    setPreview(null);
    undoRef.current = null;
    return true;
  }, []);

  const movedPos = useCallback<IntentDraft['movedPos']>(
    (channel, rack, name, startU) => {
      // 两种命中都算：`origU` 对得上（方块还画在旧位置）或
      // `pos.startU` 对得上（已经按草稿重画过、又拖了一次）。
      //
      // 必须 `toRack === rack`：换柜拖走后 `fromRack` 还指向原柜，
      // 若只按通道 + 名字匹配，原柜里那台会在**新机柜**的 U 位上画一遍，
      // 表现为「拖到别的柜，原柜里留下个幽灵，新柜里没出来」。
      for (const e of moved) {
        if (e.channel !== channel || e.toRack !== rack || e.name !== name) continue;
        if (e.origU === startU || e.pos.startU === startU) return e.pos;
      }
      return undefined;
    },
    [moved],
  );

  const movedAway = useCallback<IntentDraft['movedAway']>(
    (channel, rack, name, startU) => isMovedAway(moved, channel, rack, name, startU),
    [moved],
  );

  const movedInto = useCallback<IntentDraft['movedInto']>(
    (channel, rack) => movedIntoAt(moved, channel, rack),
    [moved],
  );

  return {
    project,
    doc,
    editable,
    loading,
    error,
    dirty,
    lastMove,
    move,
    batchMove,
    save,
    reset,
    lastImpact,
    preview,
    preflight,
    clearPreview,
    undo,
    movedPos,
    movedAway,
    movedInto,
  };
}

/** 草稿里某台的 U 位。 */
export interface DraftPos {
  startU: number;
  endU: number;
  sizeU: number;
}

/**
 * 这台是否已被拖到**别的机柜**（原柜不该再画它）。
 *
 * 单独抽成纯函数是为了可测：它错了的表现是「原柜残留一个方块」，
 * 而残留的方块看起来完全正常（位置就是拖动前的位置），
 * 只有把两个机柜并排看才发现同一台出现了两次。
 *
 * `e.origU === -1` 表示拖动时没传 `currentU`（调用方不知道它原来在第几 U），
 * 这种情况下只能按名字 + 机柜认定 —— 宁可多滤掉一台，也不要在两个机柜
 * 各画一个同样大小的方块让人以为装了两台。
 */
export function isMovedAway(
  moved: readonly MovedEntry[],
  channel: number,
  rack: number,
  name: string,
  startU: number,
): boolean {
  return moved.some(
    (e) =>
      e.channel === channel &&
      e.fromRack === rack &&
      e.toRack !== rack &&
      e.name === name &&
      (e.origU === startU || e.origU === -1),
  );
}

/**
 * 被拖进 `(channel, rack)` 的设备（这些在存量表的 payload 里没有）。
 *
 * 只收 `fromRack !== toRack` 的：同柜内调 U 位的记录由 `movedPos` 负责，
 * 两边都返回会让同一台设备在一个机柜里画两遍（一遍存量位置、一遍草稿位置）。
 */
export function movedIntoAt(
  moved: readonly MovedEntry[],
  channel: number,
  rack: number,
): { name: string; pos: DraftPos }[] {
  return moved
    .filter((e) => e.channel === channel && e.toRack === rack && e.fromRack !== rack)
    .map((e) => ({ name: e.name, pos: e.pos }));
}

/**
 * 从意图草稿里取出某机柜各设备的新 U 位 → `{实例名: 位置}`。
 *
 * ## 为什么需要这层映射
 *
 * U 位图画的是**存量布线表**推出来的位置（`/api/racks/{ch}/{seq}`），
 * 而拖拽改的是**意图**。两者在用户拖完、还没保存之前是不同的东西。
 * 不叠这一层，用户拖完松手，方块会弹回原位 —— 他会以为拖拽坏了。
 *
 * 键用实例名（`displayName`）：意图里 `switches`/`devices` 的 `instance`
 * 与 rack payload 的 `displayName` 是同一个值。
 */
export function draftPositions(
  doc: Record<string, unknown> | null,
  channel: number,
  rack: number,
): Map<string, DraftPos> {
  const out = new Map<string, DraftPos>();
  if (!doc) return out;
  for (const section of ['switches', 'devices'] as const) {
    const rows = doc[section];
    if (!Array.isArray(rows)) continue;
    for (const row of rows) {
      if (typeof row !== 'object' || row === null) continue;
      const r = row as Record<string, unknown>;
      if (r['channel'] !== channel || r['rack'] !== rack) continue;
      const inst = r['instance'];
      const u = r['u'];
      if (typeof inst !== 'string' || typeof u !== 'number') continue;
      const span = r['uSpan'];
      const size = typeof span === 'number' && span >= 1 ? span : 1;
      out.set(inst, { startU: u, endU: u + size - 1, sizeU: size });
    }
  }
  return out;
}
