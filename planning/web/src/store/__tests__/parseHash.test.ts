/**
 * store 层的纯函数 —— `parseHash`。
 *
 * ## 这个文件守护什么
 *
 * 深链是最容易被写坏又最难发现的东西：`#design` 能开，`#design?edit=1` 打不开，
 * 而后者只有「文档里给的那个链接」才会用到，日常点页签永远不会暴露。
 *
 * 线上真实缺陷（v0.4 实测）：`readInitialTab` 把整串 `design?edit=1` 当作
 * tab 名比对 → 落到 `floor`；同时 `DesignView` 从 `location.search` 读 `edit`
 * 而它在 SPA 里恒为空 → 深链完全失效，页面回到平面图且编辑态关闭。
 *
 * 所以这里锁三件事：
 * 1. `#tab?k=v` 的 `tab` 部分要正确识别（不能被 `?` 干扰）
 * 2. 参数要从 hash 的 query 里取（不能去 `location.search` 找）
 * 3. 非法 tab 名要落回 `floor` 而不是抛异常
 */
import { describe, expect, it } from 'vitest';

import { parseHash, TAB_KEYS } from '../appStore';

describe('parseHash', () => {
  it('识别裸 tab', () => {
    expect(parseHash('#design').tab).toBe('design');
    expect(parseHash('#floor').tab).toBe('floor');
  });

  it('带参数时 tab 部分不被 `?` 干扰', () => {
    // 这是线上那个 bug 的直接回归
    expect(parseHash('#design?edit=1').tab).toBe('design');
    expect(parseHash('#design?edit=1').params.edit).toBe('1');
  });

  it('多个参数', () => {
    const { tab, params } = parseHash('#cables?channel=2&rack=11');
    expect(tab).toBe('cables');
    expect(params.channel).toBe('2');
    expect(params.rack).toBe('11');
  });

  it('问号后为空不报错', () => {
    const { tab, params } = parseHash('#design?');
    expect(tab).toBe('design');
    expect(params).toEqual({});
  });

  it('非法 tab 落回 floor', () => {
    expect(parseHash('#没有这个页').tab).toBe('floor');
    expect(parseHash('').tab).toBe('floor');
    expect(parseHash('#').tab).toBe('floor');
  });

  it('大小写敏感（tab 是内部标识，不接受变体）', () => {
    expect(parseHash('#Design').tab).toBe('floor');
  });

  it('全部 tab 可解析', () => {
    // 与 `main.tsx` 的 hashchange 白名单同源：曾经那里只有 4 个，
    // 导致 design/projects/consumables 三个页签按浏览器后退键切不过去
    for (const key of TAB_KEYS) {
      expect(parseHash(`#${key}`).tab).toBe(key);
      expect(parseHash(`#${key}?x=1`).tab).toBe(key);
    }
  });

  it('TAB_KEYS 是十二个且不重复', () => {
    // 这条断言故意写死数量：新增页签时必须回来改这里。
    // 否则 `TAB_KEYS` 加了新 tab 而别处的白名单忘同步，
    // 就会出现「点页签能切、按浏览器后退键切不过去」的缺陷 —— 实测踩过。
    //
    // 12 = v0.1~v0.12 的十一页签 + v1.0 的 3D 机房。
    expect(TAB_KEYS).toHaveLength(12);
    expect(new Set(TAB_KEYS).size).toBe(12);
    expect(TAB_KEYS).toContain('design');
    expect(TAB_KEYS).toContain('projects');
    expect(TAB_KEYS).toContain('consumables');
    expect(TAB_KEYS).toContain('compare');
    expect(TAB_KEYS).toContain('ledger');
    expect(TAB_KEYS).toContain('models');
    expect(TAB_KEYS).toContain('labels');
    expect(TAB_KEYS).toContain('scene3d');
  });
});
