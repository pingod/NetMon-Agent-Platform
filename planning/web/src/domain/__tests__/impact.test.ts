/**
 * 影响面文案的守卫。
 *
 * ## 为什么这段文案值得单测
 *
 * 它出现在用户松手后的第一眼，写错的表现不是报错而是**说了假话**：
 *
 * 1. 说「线长没变」而其实**没算**（`lengthChecked=false`）——
 *    用户以为安全，现场拿到的是错的长度；
 * 2. 漏掉端口重编号 —— 用户以为只动了一台，实际 91 行端口变了；
 * 3. 移动失败时还说「删除了 N 行」—— 失败的动作配最坏的消息。
 *
 * 这三种都不会崩溃，只会让人做出错误判断。所以逻辑抽成纯函数在这里测，
 * 而不是散在 JSX 的三元表达式里。
 */

import { describe, expect, it } from 'vitest';

import type { ImpactPayload } from '../../api/client';
import {
  impactDetail,
  impactHeadline,
  impactSeverity,
  previewHeadline,
} from '../impact';

function payload(over: Partial<ImpactPayload> = {}): ImpactPayload {
  return {
    ok: true,
    error: null,
    hasHiddenImpact: false,
    counts: {
      added: 0,
      removed: 0,
      portChanges: 0,
      positionChanges: 0,
      lengthChanges: 0,
      affectedSwitches: 0,
      total: 0,
    },
    lengthChecked: true,
    affectedSwitches: [],
    affectedRacks: [],
    portSamples: [],
    lengthSamples: [],
    addedSamples: [],
    removedSamples: [],
    ...over,
  };
}

describe('impactHeadline', () => {
  it('没有影响时不出声', () => {
    expect(impactHeadline(null)).toBeNull();
    expect(impactHeadline(payload())).toBeNull();
  });

  it('只挪自己（无隐藏影响）时不出声', () => {
    // 用户拖了一台、只动了自己 —— 一句「已移动」就够，不该刷横幅。
    // 提示滥用会让 91 行那次也没人看。
    const p = payload({
      counts: { ...payload().counts, positionChanges: 1, total: 1 },
    });
    expect(impactHeadline(p)).toBeNull();
  });

  it('★ 端口重编号必须说出来，并点明「标签要重打」', () => {
    const p = payload({
      hasHiddenImpact: true,
      counts: {
        ...payload().counts,
        portChanges: 91,
        affectedSwitches: 7,
        total: 91,
      },
    });
    const text = impactHeadline(p);
    expect(text).toContain('91 行端口号变了');
    expect(text).toContain('7 台交换机');
    expect(text).toContain('标签要重打');
  });

  it('★ lengthChecked=false 时说「未校验」，绝不说「没变」', () => {
    // 这是最容易写错的一句。源文件缺失时线长根本没算，
    // 若说成「线长没变」用户会以为安全。
    const p = payload({
      hasHiddenImpact: true,
      lengthChecked: false,
      counts: { ...payload().counts, positionChanges: 7, total: 7 },
    });
    const text = impactHeadline(p);
    expect(text).toContain('线长未校验');
    expect(text).not.toContain('线长没变');
    expect(text).not.toContain('线长变了');
  });

  it('lengthChecked=true 且确实变了才说「线长变了」', () => {
    const p = payload({
      hasHiddenImpact: true,
      lengthChecked: true,
      counts: { ...payload().counts, lengthChanges: 6, positionChanges: 7, total: 7 },
    });
    expect(impactHeadline(p)).toContain('6 行线长变了');
  });

  it('lengthChecked=true 且没变时，不提线长', () => {
    const p = payload({
      hasHiddenImpact: true,
      lengthChecked: true,
      counts: { ...payload().counts, portChanges: 56, total: 56 },
    });
    const text = impactHeadline(p);
    expect(text).toContain('56 行端口号变了');
    expect(text).not.toContain('线长');
  });

  it('增删要说明「接线拓扑变了」', () => {
    const p = payload({
      hasHiddenImpact: true,
      counts: { ...payload().counts, added: 3, removed: 2, total: 5 },
    });
    const text = impactHeadline(p);
    expect(text).toContain('新增 3 行');
    expect(text).toContain('删除 2 行');
    expect(text).toContain('接线拓扑变了');
  });

  it('失败的移动不出横幅', () => {
    expect(impactHeadline(payload({ ok: false, hasHiddenImpact: true }))).toBeNull();
  });
});

describe('impactDetail', () => {
  it('空影响时无明细', () => {
    expect(impactDetail(null)).toEqual([]);
    expect(impactDetail(payload())).toEqual([]);
  });

  it('失败时无明细（原因由 error 单独说）', () => {
    expect(impactDetail(payload({ ok: false }))).toEqual([]);
  });

  it('列出端口的前后对照', () => {
    const p = payload({
      hasHiddenImpact: true,
      portSamples: [
        { switch: '计算接入交换机(业务)-1', node: 'N02-01-S1', nic: '万兆1', before: '10GE1/0/2', after: '10GE1/0/1' },
      ],
    });
    const lines = impactDetail(p);
    expect(lines.some((l) => l.includes('10GE1/0/2') && l.includes('10GE1/0/1'))).toBe(true);
  });

  it('线长明细带正负号', () => {
    const p = payload({
      hasHiddenImpact: true,
      lengthSamples: [
        { switch: 'sw', node: 'n', nic: '万兆1', before: 7, after: 10, delta: 3 },
        { switch: 'sw', node: 'n2', nic: '万兆3', before: 15, after: 5, delta: -10 },
      ],
    });
    const lines = impactDetail(p);
    expect(lines.some((l) => l.includes('+3'))).toBe(true);
    expect(lines.some((l) => l.includes('-10'))).toBe(true);
  });

  it('交换机清单超过 limit 时给总数', () => {
    const p = payload({
      hasHiddenImpact: true,
      affectedSwitches: ['a', 'b', 'c', 'd', 'e'],
    });
    const lines = impactDetail(p, 2);
    const swLine = lines.find((l) => l.includes('端口被动过的交换机'));
    expect(swLine).toContain('a、b');
    expect(swLine).toContain('5 台');
  });
});

describe('impactSeverity', () => {
  it('无影响 / 失败 → none', () => {
    expect(impactSeverity(null)).toBe('none');
    expect(impactSeverity(payload())).toBe('none');
    expect(impactSeverity(payload({ ok: false, hasHiddenImpact: true }))).toBe('none');
  });

  it('只有端口/线长变化 → warn（设计对，重打标签）', () => {
    const p = payload({
      hasHiddenImpact: true,
      counts: { ...payload().counts, portChanges: 91, total: 91 },
    });
    expect(impactSeverity(p)).toBe('warn');
  });

  it('★ 增删连线 → danger（拓扑变了，要重新看设计）', () => {
    // 「标签要重打」和「设计错了」是两件事，颜色必须能分开。
    const p = payload({
      hasHiddenImpact: true,
      counts: { ...payload().counts, added: 1, total: 1 },
    });
    expect(impactSeverity(p)).toBe('danger');
  });
});

describe('previewHeadline', () => {
  it('无影响 / 失败时不出声', () => {
    expect(previewHeadline(null)).toBeNull();
    expect(previewHeadline(payload())).toBeNull();
    expect(previewHeadline(payload({ ok: false, hasHiddenImpact: true }))).toBeNull();
  });

  it('★ 措辞是「松手会」而不是「这次改动影响」', () => {
    // 同一批数字，两句话的意思完全不同：
    //   前者是预测、还能拖到别处；后者是既成事实、只能撤销。
    // 混用会让用户错过唯一还能反悔的窗口。
    const p = payload({
      hasHiddenImpact: true,
      lengthChecked: false,
      counts: { ...payload().counts, portChanges: 91, affectedSwitches: 7, total: 91 },
    });
    const text = previewHeadline(p);
    expect(text).toContain('松手会');
    expect(text).toContain('91 行端口号会变');
    expect(text).not.toContain('这次改动影响');
  });

  it('★ 预演恒为 deep=false，所以只能说「线长未校验」', () => {
    const p = payload({
      hasHiddenImpact: true,
      lengthChecked: false,
      counts: { ...payload().counts, positionChanges: 7, total: 7 },
    });
    const text = previewHeadline(p);
    expect(text).toContain('线长未校验');
    expect(text).not.toContain('线长不变');
  });

  it('拓扑会变时明确说是接线变化', () => {
    const p = payload({
      hasHiddenImpact: true,
      counts: { ...payload().counts, added: 2, removed: 1, total: 3 },
    });
    expect(previewHeadline(p)).toContain('接线会变');
  });

  it('提示可以撤销（让用户知道松手不是不可挽回）', () => {
    const p = payload({
      hasHiddenImpact: true,
      counts: { ...payload().counts, portChanges: 5, affectedSwitches: 1, total: 5 },
    });
    expect(previewHeadline(p)).toContain('可撤销');
  });
});
