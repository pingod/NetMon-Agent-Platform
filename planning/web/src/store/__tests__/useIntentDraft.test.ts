/**
 * U 位拖拽编辑的前端纯逻辑 —— `draftPositions` / `editableKey`。
 *
 * ## 这组用例守护什么
 *
 * 拖拽本身的鼠标交互（pointer 捕获、落点换算）没法在 jsdom 里测，
 * 但那部分出错用户当场能看见（方块粘在鼠标上）。真正危险的是**静默**的错：
 *
 * 1. `draftPositions` 把改过的 U 位叠到图上 —— 错了表现为「拖完松手弹回原位」，
 *    用户以为拖拽坏了，实际是他拖成功了但视图没跟上。
 * 2. `editableKey` 的键拼接 —— 错了表现为「某些设备明明可拖却拖不动」，
 *    而且只在个别实例名上复现（名字里有分隔符时）。
 */

import { describe, expect, it } from 'vitest';

import type { MovedEntry } from '../useIntentDraft';
import {
  draftPositions,
  editableKey,
  isMovedAway,
  movedIntoAt,
  parseEditableKey,
} from '../useIntentDraft';

describe('editableKey', () => {
  it('区分不同通道的同名设备', () => {
    // `-1` 后缀跨通道重复：ch2 与 ch8 都有「计算接入交换机(业务)-1」
    expect(editableKey(2, 1, 'A-1', 3)).not.toBe(editableKey(8, 1, 'A-1', 3));
  });

  it('区分同通道不同机柜的同名设备', () => {
    expect(editableKey(2, 1, 'A-1', 3)).not.toBe(editableKey(2, 2, 'A-1', 3));
  });

  it('★ 区分同机柜同名的 8 台设备（布局段排布的服务器）', () => {
    // 这 663 台在台账里没有实例名，`displayName` 就是型号 ——
    // 同一机柜 8 台的显示名完全一样，只有 U 位不同。
    // 不带 uStart 的话：拖动一台会让 8 台一起变位置，
    // 而「能不能拖」的判断会把可拖的当成不可拖。
    const same = '计算节点A（ARM）';
    expect(editableKey(2, 1, same, 3)).not.toBe(editableKey(2, 1, same, 7));
    expect(editableKey(2, 1, same, 3)).not.toBe(editableKey(2, 1, same, 31));
  });

  it('名字里含常见分隔符也不串键', () => {
    // 实例名里真实存在 `（）` 与 `-`；若用 `|` 拼接且设备名含 `|` 会串键
    const a = editableKey(1, 1, 'a|b', 3);
    const b = editableKey(1, 1, 'a', 3);
    expect(a).not.toBe(b);
    expect(a).not.toContain('a|b\u0001');
  });

  it('相同四键稳定', () => {
    expect(editableKey(3, 7, 'X', 5)).toBe(editableKey(3, 7, 'X', 5));
  });
});

describe('draftPositions', () => {
  const doc = {
    switches: [
      { instance: 'SW-1', channel: 2, rack: 5, u: 40 },
      { instance: 'SW-2', channel: 2, rack: 5, u: 37, uSpan: 2 },
      { instance: 'SW-3', channel: 2, rack: 6, u: 10 },
    ],
    devices: [{ instance: 'dev-1', channel: 2, rack: 5, u: 3, uSpan: 4 }],
  };

  it('返回该机柜全部设备', () => {
    const pos = draftPositions(doc, 2, 5);
    expect(pos.get('SW-1')).toEqual({ startU: 40, endU: 40, sizeU: 1 });
    expect(pos.get('dev-1')).toEqual({ startU: 3, endU: 6, sizeU: 4 });
  });

  it('uSpan 缺省按 1U', () => {
    expect(draftPositions(doc, 2, 5).get('SW-1')?.sizeU).toBe(1);
  });

  it('其他机柜不混入', () => {
    expect(draftPositions(doc, 2, 5).has('SW-3')).toBe(false);
  });

  it('跨段合并 —— 用户看不出 switches 与 devices 的区别', () => {
    const pos = draftPositions(doc, 2, 5);
    expect(pos.has('SW-1')).toBe(true);
    expect(pos.has('dev-1')).toBe(true);
  });

  it('doc 为 null 安全', () => {
    expect(draftPositions(null, 2, 5).size).toBe(0);
  });

  it('半行（缺 u）跳过而不是抛异常', () => {
    const bad = { switches: [{ instance: 'X', channel: 2, rack: 5 }] };
    expect(draftPositions(bad, 2, 5).size).toBe(0);
  });

  it('非数组段安全', () => {
    expect(draftPositions({ switches: 'not-a-list' }, 2, 5).size).toBe(0);
  });
});

describe('isMovedAway / movedIntoAt（跨机柜拖动）', () => {
  const moved: MovedEntry[] = [
    // 同柜内调 U 位：不该被当成「拖走了」
    { channel: 2, fromRack: 5, toRack: 5, name: 'SW-1', origU: 40, pos: { startU: 30, endU: 31, sizeU: 2 } },
    // 跨柜：从 5 柜拖到 7 柜
    { channel: 2, fromRack: 5, toRack: 7, name: 'SW-2', origU: 37, pos: { startU: 37, endU: 38, sizeU: 2 } },
    // 跨柜但不知道原 U 位（调用方没传 currentU）
    { channel: 2, fromRack: 9, toRack: 7, name: 'SW-3', origU: -1, pos: { startU: 10, endU: 11, sizeU: 2 } },
    // 别的通道，不该互相影响
    { channel: 8, fromRack: 5, toRack: 7, name: 'SW-9', origU: 3, pos: { startU: 3, endU: 4, sizeU: 2 } },
  ];

  it('★ 跨柜拖走的设备在原柜不再出现', () => {
    // 不滤掉的表现：原柜那台还画在原位 —— 看起来完全正常，
    // 只有把两个机柜并排看才发现同一台出现两次。
    expect(isMovedAway(moved, 2, 5, 'SW-2', 37)).toBe(true);
  });

  it('同柜内调 U 位不算「拖走」', () => {
    expect(isMovedAway(moved, 2, 5, 'SW-1', 40)).toBe(false);
  });

  it('★ 原 U 位未知（origU=-1）也按名字 + 机柜认定', () => {
    // 宁可多滤掉一台，也不要在两个机柜各画一个同样大小的方块
    // 让人以为装了两台。
    expect(isMovedAway(moved, 2, 9, 'SW-3', 3)).toBe(true);
    expect(isMovedAway(moved, 2, 9, 'SW-3', 99)).toBe(true);
  });

  it('别的通道同名设备不算拖走', () => {
    expect(isMovedAway(moved, 8, 5, 'SW-2', 37)).toBe(false);
  });

  it('★ 跨柜拖进的设备出现在目标机柜', () => {
    const into = movedIntoAt(moved, 2, 7);
    expect(into.map((x) => x.name).sort()).toEqual(['SW-2', 'SW-3']);
    expect(into.find((x) => x.name === 'SW-2')?.pos.startU).toBe(37);
  });

  it('★ 同柜内调 U 位不进 movedInto（否则一台在同一柜画两遍）', () => {
    const into = movedIntoAt(moved, 2, 5);
    expect(into).toEqual([]);
  });

  it('目标机柜在别的通道时不会串', () => {
    expect(movedIntoAt(moved, 2, 7).some((x) => x.name === 'SW-9')).toBe(false);
    expect(movedIntoAt(moved, 8, 7).map((x) => x.name)).toEqual(['SW-9']);
  });
});

describe('parseEditableKey（批量拖动的 targets 还原）', () => {
  it('四元组能原样拆回', () => {
    const k = editableKey(2, 5, '计算节点A（ARM）', 3);
    expect(parseEditableKey(k)).toEqual({
      channel: 2,
      rack: 5,
      uStart: 3,
      instance: '计算节点A（ARM）',
    });
  });

  it('★ 同名 8 台靠 uStart 区分，拆回来不能丢', () => {
    // 批量拖动必须把每一台的 currentU 传给后端 —— 少了它后端只能
    // 「按型号+U位找第一台」，8 台同名会让整组落到同一台身上。
    const a = parseEditableKey(editableKey(2, 1, '计算节点A（ARM）', 3));
    const b = parseEditableKey(editableKey(2, 1, '计算节点A（ARM）', 7));
    expect(a?.instance).toBe(b?.instance);
    expect(a?.uStart).not.toBe(b?.uStart);
  });

  it('格式不对返回 null 而不是抛错', () => {
    // 走的是「前端把一堆字符串 map 成 targets」的路径，
    // 抛错会让整批拖动失败，而坏键只该被跳过。
    expect(parseEditableKey('garbage')).toBeNull();
    expect(parseEditableKey('1\u00012\u00013')).toBeNull();
    expect(parseEditableKey('a\u00012\u00013\u0001x')).toBeNull();
  });
});
