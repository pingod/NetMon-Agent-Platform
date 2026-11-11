import { describe, expect, it } from 'vitest';

import { fillBand, inSel, normSel, parseSelParam, selCount } from '../selMath';

/**
 * 选区数学的用例。
 *
 * ## 为什么这几行值得单独测
 *
 * 选区是「用户拖出来的框」到「真正被复制/填充的区域」之间唯一的换算点。
 * 它的错法都很隐蔽：**选出来的框看着是对的**，但实际范围缩成一格、
 * 镜像到另一侧、或差一行 —— 用户在界面上看不出问题，直到复制出去
 * 少了两列、或填充覆盖了自己刚填的数据。
 *
 * 而它又必须靠手点才能验证（拖动事件），所以把数学抽出来是唯一的
 * 可测路径。
 */
describe('normSel', () => {
  it('正着拖（左上 → 右下）', () => {
    expect(normSel({ r: 0, c: 0 }, { r: 2, c: 3 })).toEqual({
      r0: 0,
      r1: 2,
      c0: 0,
      c1: 3,
    });
  });

  it('反着拖（右下 → 左上）得到同一个框', () => {
    // 反拖是最容易出错的：若直接用 anchor 当 r0，会得到 r0 > r1 的
    // 空区间，界面上表现为「框住了但什么都没选中」。
    expect(normSel({ r: 2, c: 3 }, { r: 0, c: 0 })).toEqual(
      normSel({ r: 0, c: 0 }, { r: 2, c: 3 }),
    );
  });

  it('斜着拖（右上 → 左下）', () => {
    expect(normSel({ r: 0, c: 3 }, { r: 2, c: 0 })).toEqual({
      r0: 0,
      r1: 2,
      c0: 0,
      c1: 3,
    });
  });

  it('单格', () => {
    expect(normSel({ r: 5, c: 2 }, { r: 5, c: 2 })).toEqual({
      r0: 5,
      r1: 5,
      c0: 2,
      c1: 2,
    });
  });
});

describe('selCount', () => {
  it('含端点的格数', () => {
    expect(selCount({ r0: 0, r1: 0, c0: 0, c1: 0 })).toBe(1);
    expect(selCount({ r0: 0, r1: 3, c0: 0, c1: 3 })).toBe(16);
    expect(selCount({ r0: 2, r1: 4, c0: 1, c1: 2 })).toBe(6);
  });
});

describe('inSel', () => {
  const s = { r0: 2, r1: 4, c0: 1, c1: 3 };

  it('四角都在内', () => {
    expect(inSel(s, 2, 1)).toBe(true);
    expect(inSel(s, 2, 3)).toBe(true);
    expect(inSel(s, 4, 1)).toBe(true);
    expect(inSel(s, 4, 3)).toBe(true);
  });

  it('边界外一格不在内', () => {
    expect(inSel(s, 1, 1)).toBe(false);
    expect(inSel(s, 5, 1)).toBe(false);
    expect(inSel(s, 2, 0)).toBe(false);
    expect(inSel(s, 2, 4)).toBe(false);
  });

  it('null 选区永远不在内', () => {
    expect(inSel(null, 0, 0)).toBe(false);
  });
});

describe('fillBand', () => {
  const s = { r0: 0, r1: 1, c0: 2, c1: 3 };

  it('往下拖给出「末行之后」的带', () => {
    // r0 必须是 s.r1 + 1 而不是 s.r0：从源区第一行开始画预览，
    // 视觉上等于告诉用户「源区也要被覆盖」。
    expect(fillBand(s, 5)).toEqual({ r0: 2, r1: 5, c0: 2, c1: 3 });
  });

  it('拖到末行本身不给带', () => {
    expect(fillBand(s, 1)).toBeNull();
  });

  it('往上拖不给带', () => {
    // 往上拖在 Excel 里会覆盖已有值，这里直接不支持：
    // 覆盖用户数据的操作不该靠一次误拖就发生。
    expect(fillBand(s, 0)).toBeNull();
  });

  it('没有选区时安全返回 null', () => {
    expect(fillBand(null, 5)).toBeNull();
  });

  it('没有 fillTo 时安全返回 null', () => {
    expect(fillBand(s, null)).toBeNull();
  });
});

describe('parseSelParam', () => {
  it('解析 1-based 深链参数', () => {
    expect(parseSelParam('1-4-1-4')).toEqual({ r0: 0, r1: 3, c0: 0, c1: 3 });
  });

  it('反序参数也被规范化', () => {
    expect(parseSelParam('4-1-4-1')).toEqual({ r0: 0, r1: 3, c0: 0, c1: 3 });
  });

  it('空参数返回 null', () => {
    expect(parseSelParam('')).toBeNull();
    expect(parseSelParam(null)).toBeNull();
    expect(parseSelParam(undefined)).toBeNull();
  });

  it('段数不对返回 null', () => {
    expect(parseSelParam('1-4-1')).toBeNull();
    expect(parseSelParam('1-4-1-4-9')).toBeNull();
  });

  it('非数字返回 null', () => {
    expect(parseSelParam('a-b-c-d')).toBeNull();
    expect(parseSelParam('1-4-1-x')).toBeNull();
  });

  it('0 或负数返回 null（深链是 1-based）', () => {
    // 不返回 null 的后果：`r0: -1` 会让 `<td>` 的判定全部错位，
    // 表现为「整张表都被框住了」。
    expect(parseSelParam('0-4-1-4')).toBeNull();
    expect(parseSelParam('-1-4-1-4')).toBeNull();
  });
});
