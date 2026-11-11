/**
 * 意图编辑器的选区数学（v0.4 #3 区域选择 / 填充柄共用）。
 *
 * ## 为什么单独成模块
 *
 * 这段逻辑只有几行，但它是「拖动时选出来的区域」与「真正被复制/填充的
 * 区域」之间唯一的换算点。放在组件里只能靠手点验证，而拖动方向一变
 * （右下→左上）就出错，且错法很隐蔽：选出来的框看着是对的，
 * 实际范围缩成一格或镜像到另一边。抽出来才测得动。
 */

/** 一块矩形选区，行列都是 **0-based 且含端点**。 */
export interface Sel {
  r0: number;
  r1: number;
  c0: number;
  c1: number;
}

/** 由拖动锚点与当前落点算出规范化的矩形（左上小、右下大）。 */
export function normSel(
  a: { r: number; c: number },
  b: { r: number; c: number },
): Sel {
  return {
    r0: Math.min(a.r, b.r),
    r1: Math.max(a.r, b.r),
    c0: Math.min(a.c, b.c),
    c1: Math.max(a.c, b.c),
  };
}

/** 选区内的格数。 */
export function selCount(s: Sel): number {
  return (s.r1 - s.r0 + 1) * (s.c1 - s.c0 + 1);
}

/** 某个格子是否落在选区内。 */
export function inSel(s: Sel | null, r: number, c: number): boolean {
  if (!s) return false;
  return r >= s.r0 && r <= s.r1 && c >= s.c0 && c <= s.c1;
}

/**
 * 填充预览范围：从选区末行往下到 `fillTo`、列与选区相同。
 *
 * 只在 `fillTo > s.r1` 时返回，否则 null —— 往上拖不该填充，
 * 那会覆盖用户已有数据且没有明确语义（Excel 里往上拖会覆盖旧值）。
 */
export function fillBand(s: Sel | null, fillTo: number | null): Sel | null {
  if (!s || fillTo === null || fillTo <= s.r1) return null;
  return { r0: s.r1 + 1, r1: fillTo, c0: s.c0, c1: s.c1 };
}

/**
 * 解析深链里的选区参数 `?sel=1-4-1-4`（1-based，与界面显示一致）。
 *
 * 坏值返回 null 而不是抛错：深链是手改 URL 进来的，写错了就当作没选，
 * 让人看到界面而不是一片白屏。
 */
export function parseSelParam(raw: string | undefined | null): Sel | null {
  if (!raw) return null;
  const parts = raw.split('-').map((x) => Number.parseInt(x, 10));
  if (parts.length !== 4 || parts.some((x) => Number.isNaN(x))) return null;
  const [r0, r1, c0, c1] = parts as [number, number, number, number];
  if (r0 < 1 || r1 < 1 || c0 < 1 || c1 < 1) return null;
  return normSel({ r: r0 - 1, c: c0 - 1 }, { r: r1 - 1, c: c1 - 1 });
}
