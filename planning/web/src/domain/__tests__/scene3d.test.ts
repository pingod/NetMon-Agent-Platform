/**
 * 3D 坐标换算的守卫（`domain/scene3d.ts`）。
 *
 * ## 为什么这些必须单测
 *
 * 3D 的问题**几乎都是"差一点"**：柜子穿模 100mm、相机远一倍、设备高一个 U。
 * 这些在屏幕上不一定看得出（机房那么大，多 100mm 谁知道），
 * 但一旦打印或换个机房就对不上。数值断言是唯一能稳定抓住它们的方式。
 *
 * 尤其是「相机取景」—— 第一版算错时画面里机柜只占中间一小块，
 * 那看起来"也挺正常"，直到跟真机房对比才发现视角完全不对。
 */

import { describe, expect, it } from 'vitest';

import {
  AISLE_WIDTH_MM,
  CABINET_DEPTH_MM,
  CABINET_HEIGHT_MM,
  CABINET_WIDTH_MM,
  MM_PER_U,
  RACK_SPACE_MM,
  U_BASE_MM,
  categoryColor,
  deviceBoxMm,
  fitCamera,
  fitDistanceForView,
  mmToWorld,
  sceneBoxMm,
  uToHeightMm,
} from '../scene3d';

describe('常量（EIA-310 与布线表同源）', () => {
  it('1U = 44.45mm（EIA-310 标准 1.75 inch）', () => {
    expect(MM_PER_U).toBe(44.45);
  });

  it('42U 有效空间 = 42 × 44.45 = 1866.9mm', () => {
    expect(RACK_SPACE_MM).toBeCloseTo(1866.9, 4);
  });

  it('42U 有效空间能塞进 2000mm 柜体内，且有余量', () => {
    // 这条守的是「柜高不能小于 42U」—— 写错的话最上面那台设备会捅出柜顶
    expect(RACK_SPACE_MM).toBeLessThan(CABINET_HEIGHT_MM);
  });

  it('柜宽 600 与布线表 R 列的 0.6m/柜位 同源', () => {
    // 不是巧合：O 公式的 `0.6 米/柜位` 就是柜宽。改了这里等于改线长公式。
    expect(CABINET_WIDTH_MM).toBe(600);
  });

  it('柜深 1200 + 通道宽 1200 = 一个通道占 2400mm', () => {
    expect(CABINET_DEPTH_MM + AISLE_WIDTH_MM).toBe(2400);
  });
});

describe('mmToWorld', () => {
  it('1 米 = 1 世界单位（three 的惯例）', () => {
    expect(mmToWorld(1000)).toBe(1);
    expect(mmToWorld(44.45)).toBeCloseTo(0.04445, 8);
  });

  it('★ 全项目只有这一处 /1000', () => {
    // 若在别处再除一次，画面会缩到 1/1000 —— 看起来像"什么都没渲染"，
    // 而那种 bug 极难看出方向。这条断言锁定换算口径本身。
    expect(mmToWorld(0)).toBe(0);
    expect(mmToWorld(2000)).toBe(2);
  });
});

describe('uToHeightMm', () => {
  it('U1 在底部（底面高度 0）', () => {
    expect(uToHeightMm(1)).toBe(0);
  });

  it('U2 高一个 U', () => {
    expect(uToHeightMm(2)).toBeCloseTo(MM_PER_U, 6);
  });

  it('U42 的高度 = 41U', () => {
    expect(uToHeightMm(42)).toBeCloseTo(41 * MM_PER_U, 6);
  });

  it('★ U1 + 42U 高 = 1866.9mm，不超出柜内空间', () => {
    // 从 U1 起占满 42U 的设备，顶面应正好在有效空间顶端
    const top = uToHeightMm(1) + 42 * MM_PER_U;
    expect(top).toBeCloseTo(RACK_SPACE_MM, 4);
  });
});

describe('deviceBoxMm', () => {
  it('2U 设备高 2×44.45 - 1（留 1mm 缝防 z-fighting）', () => {
    expect(deviceBoxMm(3, 2).heightMm).toBeCloseTo(2 * MM_PER_U - 1, 4);
  });

  it('★ 设备中心 = 柜内底面抬高 + U 起点高度 + 半高', () => {
    const box = deviceBoxMm(3, 2);
    const expected = U_BASE_MM + uToHeightMm(3) + box.heightMm / 2;
    expect(box.centerYMm).toBeCloseTo(expected, 4);
  });

  it('设备比柜窄、比柜浅（不会从柜壁穿出来）', () => {
    const box = deviceBoxMm(3, 2);
    expect(box.widthMm).toBeLessThan(CABINET_WIDTH_MM);
    expect(box.depthMm).toBeLessThan(CABINET_DEPTH_MM);
  });

  it('★ 最上面一台设备不捅出柜顶', () => {
    // U41 起占 2U = U41-U42，顶面应仍在柜体内
    const box = deviceBoxMm(41, 2);
    const topMm = box.centerYMm + box.heightMm / 2;
    expect(topMm).toBeLessThanOrEqual(CABINET_HEIGHT_MM);
  });

  it('U 越大中心越高（单调性）', () => {
    expect(deviceBoxMm(20, 2).centerYMm).toBeGreaterThan(deviceBoxMm(3, 2).centerYMm);
  });
});

describe('sceneBoxMm', () => {
  it('两个房间并排 → 宽度覆盖两者', () => {
    const box = sceneBoxMm([
      { widthMm: 9600, depthMm: 25200, originX: 0, originZ: 0 },
      { widthMm: 9600, depthMm: 25200, originX: 10800, originZ: 0 },
    ]);
    expect(box.minX).toBe(0);
    expect(box.maxX).toBe(20400);
    expect(box.maxZ).toBe(25200);
  });

  it('高度取柜高（0 到 2000）', () => {
    const box = sceneBoxMm([{ widthMm: 600, depthMm: 600, originX: 0, originZ: 0 }]);
    expect(box.minY).toBe(0);
    expect(box.maxY).toBe(CABINET_HEIGHT_MM);
  });

  it('空场景也给一个有限的盒子（不能返回 Infinity）', () => {
    const box = sceneBoxMm([]);
    expect(Number.isFinite(box.minX)).toBe(true);
    expect(box.maxX).toBeGreaterThan(box.minX);
  });
});

describe('fitDistanceForView', () => {
  const box = sceneBoxMm([
    { widthMm: 9600, depthMm: 25200, originX: 0, originZ: 0 },
    { widthMm: 9600, depthMm: 25200, originX: 10800, originZ: 0 },
  ]);

  it('★ 距离必须是有限正数（第一版算出 49 米，机柜只占中间一小块）', () => {
    const d = fitDistanceForView(box, Math.PI / 4, Math.PI / 3.2, 16 / 9);
    expect(Number.isFinite(d)).toBe(true);
    expect(d).toBeGreaterThan(0);
    // 20400×25200mm 的场景，45° 视场下 20~40 米是合理区间。
    // 远小于此说明算错（会把场景画得极小），远大于此说明看不全。
    expect(d).toBeGreaterThan(10_000);
    expect(d).toBeLessThan(60_000);
  });

  it('画布越窄（aspect 越小）需要越远', () => {
    const wide = fitDistanceForView(box, Math.PI / 4, Math.PI / 3.2, 2.0);
    const narrow = fitDistanceForView(box, Math.PI / 4, Math.PI / 3.2, 0.8);
    expect(narrow).toBeGreaterThan(wide);
  });

  it('俯视角变化会改变所需距离（所以不能存绝对距离）', () => {
    const shallow = fitDistanceForView(box, Math.PI / 4, Math.PI / 2.2, 16 / 9);
    const top = fitDistanceForView(box, Math.PI / 4, 0.2, 16 / 9);
    expect(Math.abs(shallow - top)).toBeGreaterThan(100);
  });

  it('margin 越大距离越远', () => {
    const tight = fitDistanceForView(box, Math.PI / 4, Math.PI / 3.2, 16 / 9, 45, 1.0);
    const loose = fitDistanceForView(box, Math.PI / 4, Math.PI / 3.2, 16 / 9, 45, 1.5);
    expect(loose).toBeGreaterThan(tight);
  });
});

describe('fitCamera', () => {
  const rooms = [
    { widthMm: 9600, depthMm: 25200, originX: 0, originZ: 0 },
    { widthMm: 9600, depthMm: 25200, originX: 10800, originZ: 0 },
  ];

  it('注视点是场景中心（世界单位）', () => {
    const fit = fitCamera(rooms);
    expect(fit.targetX).toBeCloseTo(10.2, 2); // (0 + 20400)/2 mm = 10.2 m
    expect(fit.targetZ).toBeCloseTo(12.6, 2);
  });

  it('★ 注视高度是柜高一半，不是地板', () => {
    // 看向地板会让整个场景挤在画面上半部、下方一大片空白（实测第一版）
    const fit = fitCamera(rooms);
    expect(fit.targetY).toBeCloseTo(mmToWorld(CABINET_HEIGHT_MM / 2), 4);
    expect(fit.targetY).toBeGreaterThan(0);
  });

  it('空场景返回一个能用的默认值', () => {
    const fit = fitCamera([]);
    expect(fit.distance).toBeGreaterThan(0);
    expect(Number.isFinite(fit.targetX)).toBe(true);
  });
});

describe('categoryColor', () => {
  it('六类都有颜色', () => {
    for (const c of ['core', 'switch', 'server', 'storage', 'security', 'other']) {
      expect(categoryColor(c)).toMatch(/^#[0-9a-f]{6}$/i);
    }
  });

  it('★ 未知类别兜底为 other 色，不是 undefined', () => {
    // 后端加了新类别而前端没跟上时，不能画出黑块（材质拿到 undefined 会崩）
    expect(categoryColor('全新的类别')).toBe(categoryColor('other'));
  });

  it('核心与接入的颜色必须不同', () => {
    // 混色的话现场分不清哪台是核心交换机
    expect(categoryColor('core')).not.toBe(categoryColor('switch'));
  });
});
