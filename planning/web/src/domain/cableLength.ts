/**
 * 前端公式镜像的唯一来源。
 *
 * 对应 Python 侧 `backend/idcbox/domain/cable_length/formulas.py`。
 *
 * ## 为什么允许存在第二份实现
 *
 * 用户在浏览器里改一个 U 位，必须**立刻**看到 5113 行的长度刷新，不能等 HTTP 往返。
 * 所以前端必须能自己算。但"两份实现"是长期腐化的头号来源，因此：
 *
 * 1. 只允许这一份镜像，且**不得有任何业务判断**（不做策略选择、不做告警分级，
 *    那些留在后端）。前端只算「给定两个机柜 + 线缆类型 → N/O/P/Q/R/S/T」。
 * 2. 必须通过 `shared/golden/length_vectors.json` 的双向校验
 *    （`vitest` 读同一份 JSON，`pytest` 也读它）。
 * 3. **`0.72` 绝不出现**：`0.6 * 1.2` 是业务系数，写积会让换机房变成改代码。
 *
 * ## 与 Python 侧的差异（有意为之）
 *
 * - 不做 `Issue` 收集（前端没有告警基础设施，只算值）
 * - CMAX 缺失、未知线缆类型等降级路径返回 `null` 而非默认值 ——
 *   让调用方能区分"算出来是 null"和"算出来是 0"
 */

/** O 的物理含义常量：走线架在排端，跨侧/跨通道必须走到排端再折返 */
export const CROSS_AISLE_OFFSET = 1;

/** 机柜在平面图上的位置。`row` 决定通道与侧别，`col` 决定排内位置。 */
export interface RackPos {
  readonly roomCode: string;
  readonly channelNo: number;
  readonly seqNo: number;
  readonly side: 'N' | 'S';
  readonly col: number;
  readonly row: number;
}

export type OClass = 'A' | 'B' | 'C';

/** 长度规则。与 Python `LengthRule` 同构。 */
export interface LengthRule {
  readonly rackSpacingM: number;
  readonly slackRatio: number;
  readonly nRowCoef: number;
  readonly nRowExtra: number;
  readonly inRackLenByType: Readonly<Record<string, number>>;
  readonly cmaxByRoom: Readonly<Record<string, number>>;
  readonly crossAisleOverrides: Readonly<Record<string, number>>;
  readonly defaultUnknownQ: number;
}

/** 阶梯规则（与 Python `Rung` 同构）。`op` 只有 le/eq/lt 三种。 */
export interface Rung {
  readonly seq: number;
  readonly op: 'le' | 'eq' | 'lt';
  readonly limit: number;
  readonly out: number;
  readonly label: string;
}

export const DEFAULT_RULE: LengthRule = {
  rackSpacingM: 0.6,
  slackRatio: 1.2,
  nRowCoef: 4,
  nRowExtra: 4,
  inRackLenByType: { 'RJ45': 7, 'LC-LC': 5, 'MPO': 5 },
  cmaxByRoom: { '403': 38, '404': 18 },
  crossAisleOverrides: {},
  defaultUnknownQ: 5,
};

/**
 * IFS 逐字复刻的阶梯表。**顺序即语义，不得重排。**
 *
 * 注意它不是单调阶梯：`S<=6→7` 后紧跟 `S==7→7` 与 `S<=8.5→10`，
 * 留下 `(6,7)` 开区间 —— `S=6.44` 会落到 10 而不是 7。
 * 这是源 Excel 的真实行为（49 行受影响），已由后端体检报告披露。
 */
export const IFS_COMPAT: readonly Rung[] = [
  { seq: 1, op: 'le', limit: 5.0, out: 5, label: 'le_5' },
  { seq: 2, op: 'le', limit: 6.0, out: 7, label: 'le_6' },
  { seq: 3, op: 'eq', limit: 7.0, out: 7, label: 'eq_7' },
  { seq: 4, op: 'le', limit: 8.5, out: 10, label: 'le_8.5' },
  { seq: 5, op: 'le', limit: 13.0, out: 15, label: 'le_13' },
  { seq: 6, op: 'le', limit: 17.5, out: 20, label: 'le_17.5' },
  { seq: 7, op: 'le', limit: 22.0, out: 25, label: 'le_22' },
  { seq: 8, op: 'le', limit: 27.0, out: 30, label: 'le_27' },
  { seq: 9, op: 'le', limit: 32.0, out: 35, label: 'le_32' },
  { seq: 10, op: 'le', limit: 37.0, out: 40, label: 'le_37' },
];

/** 修正版：只把 `eq 7` 放宽为 `le 7`，补掉 (6,7) 缝隙。仅供体检对比，不作生产默认。 */
export const CONTINUOUS: readonly Rung[] = [
  { seq: 1, op: 'le', limit: 5.0, out: 5, label: 'le_5' },
  { seq: 2, op: 'le', limit: 7.0, out: 7, label: 'le_7' },
  { seq: 3, op: 'le', limit: 8.5, out: 10, label: 'le_8.5' },
  { seq: 4, op: 'le', limit: 13.0, out: 15, label: 'le_13' },
  { seq: 5, op: 'le', limit: 17.5, out: 20, label: 'le_17.5' },
  { seq: 6, op: 'le', limit: 22.0, out: 25, label: 'le_22' },
  { seq: 7, op: 'le', limit: 27.0, out: 30, label: 'le_27' },
  { seq: 8, op: 'le', limit: 32.0, out: 35, label: 'le_32' },
  { seq: 9, op: 'le', limit: 37.0, out: 40, label: 'le_37' },
];

/** 展示档位 9 档（含 35）；采购档位 8 档（35→40 合并）。两者不是同一个东西。 */
export const LADDER_TIERS: readonly number[] = [5, 7, 10, 15, 20, 25, 30, 35, 40];
export const PURCHASE_TIERS: readonly number[] = [5, 7, 10, 15, 20, 25, 30, 40];

/** 把双精度结果收敛到 Excel 的 15 位有效数字口径。 */
export function excel15(x: number): number {
  if (!Number.isFinite(x) || x === 0) return x;
  return parseFloat(x.toPrecision(15));
}

/** 判定空间关系类别。与 Python `classify_o` 逐字一致。 */
export function classifyO(a: RackPos, b: RackPos): OClass {
  if (a.row === b.row) return 'A';
  if (a.channelNo === b.channelNo) return 'B';
  return 'C';
}

/** N（跨机柜）= 0 if Δrow==0 else coef*Δrow + extra。 */
export function nCrossRack(a: RackPos, b: RackPos, rule: LengthRule = DEFAULT_RULE): number {
  const d = Math.abs(a.row - b.row);
  return d === 0 ? 0 : rule.nRowCoef * d + rule.nRowExtra;
}

/**
 * O（间隔机柜）。A 类横向直穿，B/C 类排端折返。
 *
 * 返回 `null` 表示算不出来（CMAX 缺失），**不是 0** ——
 * 让调用方能把"缺几何"与"真的相邻"区分开。
 */
export function oGapRack(
  a: RackPos,
  b: RackPos,
  rule: LengthRule = DEFAULT_RULE,
  oClass?: OClass,
): number | null {
  const cls = oClass ?? classifyO(a, b);
  if (cls === 'A') return Math.abs(a.col - b.col);

  if (cls === 'C') {
    const key = `${a.channelNo}-${b.channelNo}`;
    const override = rule.crossAisleOverrides[key];
    if (override !== undefined) return override;
  }

  const cmaxA = rule.cmaxByRoom[a.roomCode];
  const cmaxB = rule.cmaxByRoom[b.roomCode];
  if (cmaxA === undefined || cmaxB === undefined) return null;
  return cmaxA - a.col + (cmaxB - b.col) + CROSS_AISLE_OFFSET;
}

/** P = N + O。 */
export function pRackCount(n: number, o: number): number {
  return n + o;
}

/** Q（机内线缆）：按线缆类型查表。未知类型返回 `defaultUnknownQ`。 */
export function qInRack(
  cableType: string,
  rule: LengthRule = DEFAULT_RULE,
  override?: number,
): number {
  if (override !== undefined) return override;
  return rule.inRackLenByType[cableType] ?? rule.defaultUnknownQ;
}

/** R = P × 机柜间距 × 余量系数，走 excel15。**不是 P × 0.72。** */
export function rInterRack(p: number, rule: LengthRule = DEFAULT_RULE): number {
  return excel15(p * rule.rackSpacingM * rule.slackRatio);
}

/** S = R + Q，走 excel15。 */
export function sTotal(r: number, q: number): number {
  return excel15(r + q);
}

/**
 * T = 阶梯规则表逐条匹配，首条命中即返回。
 *
 * **禁止用 `ceil` / 数组上取整替代。** 档位法在 636 行样本上只能复现 424 行
 * （66.67%），而逐条匹配是 636/636。
 *
 * 返回 `null` 表示 S 超出末档 37，交人工定档。
 */
export function ladderRound(s: number, rungs: readonly Rung[] = IFS_COMPAT): number | null {
  for (const r of rungs) {
    if (r.op === 'le' ? s <= r.limit : r.op === 'eq' ? s === r.limit : s < r.limit) {
      return r.out;
    }
  }
  return null;
}

/** 完整公式链结果。字段名与后端 `LengthResult` 对应。 */
export interface LengthChain {
  readonly oClass: OClass;
  readonly n: number;
  readonly o: number | null;
  readonly p: number | null;
  readonly q: number | null;
  readonly r: number | null;
  readonly s: number | null;
  readonly t: number | null;
  readonly overLimit: boolean;
}

/**
 * 一次算完整条链。缺几何时后续逐级为 `null`，**绝不填 0**。
 */
export function computeLengthChain(
  a: RackPos,
  b: RackPos,
  cableType: string,
  rule: LengthRule = DEFAULT_RULE,
  rungs: readonly Rung[] = IFS_COMPAT,
): LengthChain {
  const oClass = classifyO(a, b);
  const n = nCrossRack(a, b, rule);
  const o = oGapRack(a, b, rule, oClass);
  if (o === null) {
    return { oClass, n, o: null, p: null, q: null, r: null, s: null, t: null, overLimit: false };
  }
  const p = pRackCount(n, o);
  const q = qInRack(cableType, rule);
  const r = rInterRack(p, rule);
  const s = sTotal(r, q);
  const t = ladderRound(s, rungs);
  return { oClass, n, o, p, q, r, s, t, overLimit: t === null };
}

/** 经验表键：路径距离（列坐标单位）。A 类横向直穿，B/C 类排端折返。 */
export function pathUnits(a: RackPos, b: RackPos, rule: LengthRule = DEFAULT_RULE): number {
  const cls = classifyO(a, b);
  if (cls === 'A') return Math.abs(a.col - b.col);
  const cmaxA = rule.cmaxByRoom[a.roomCode];
  const cmaxB = rule.cmaxByRoom[b.roomCode];
  if (cmaxA === undefined || cmaxB === undefined) return Math.abs(a.col - b.col);
  return cmaxA - a.col + (cmaxB - b.col) + CROSS_AISLE_OFFSET;
}

/** 采购档位：35 → 40（35 不单独采购）。 */
export function toPurchaseTier(lengthM: number): number {
  for (const tier of PURCHASE_TIERS) {
    if (lengthM <= tier) return tier;
  }
  return PURCHASE_TIERS[PURCHASE_TIERS.length - 1] as number;
}
