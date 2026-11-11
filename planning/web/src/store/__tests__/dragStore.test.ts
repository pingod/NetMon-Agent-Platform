/**
 * 平面图落点高亮 —— `rackDropClass`。
 *
 * ## 这组用例守护什么
 *
 * 它决定「用户松手前看到什么」。错了不会报错，只会让人做出错误判断：
 * 同通道别的柜没高亮 → 他以为不能跨柜拖；别的通道也高亮了 → 他拖过去
 * 才被后端拒绝，白拖一次。
 */

import { describe, expect, it } from 'vitest';

import { rackDropClass } from '../dragStore';
import type { DragDevice } from '../dragStore';

const dev: DragDevice = { channel: 2, rack: 5, name: '计算节点A（ARM）', startU: 3, sizeU: 2 };

describe('rackDropClass', () => {
  it('没在拖的时候一格都不标', () => {
    expect(rackDropClass(null, 2, 5)).toBe('');
    expect(rackDropClass(null, 8, 7)).toBe('');
  });

  it('★ 同通道的其他机柜是合法落点', () => {
    expect(rackDropClass(dev, 2, 7)).toBe(' drop-target');
    expect(rackDropClass(dev, 2, 1)).toBe(' drop-target');
  });

  it('★ 起点那个柜单独标', () => {
    // 不标的话用户以为松手会把它移到「这个柜」（其实就是原地），
    // 或者以为这个柜也能当落点。
    expect(rackDropClass(dev, 2, 5)).toBe(' drop-origin');
  });

  it('★ 别的通道标成不可落（但不隐藏）', () => {
    // 后端只允许同通道换柜（换通道等于换一组 ToR 服务它）。
    // 隐藏会让平面图在拖动时突然少一半格子，用户以为丢东西了。
    expect(rackDropClass(dev, 8, 5)).toBe(' drop-invalid');
    expect(rackDropClass(dev, 3, 5)).toBe(' drop-invalid');
  });

  it('同机柜号不同通道 → 不是起点是非法', () => {
    // ch2 rack5 与 ch8 rack5 是两个不同的物理机柜
    expect(rackDropClass(dev, 8, 5)).toBe(' drop-invalid');
    expect(rackDropClass(dev, 2, 5)).toBe(' drop-origin');
  });
});
