/**
 * 3D 世界的坐标换算（纯函数，零 three 依赖，可单测）。
 *
 * ## 为什么这些函数不放进 3D 组件里
 *
 * 它们是**几何真值的投影**，不是渲染细节。放进组件就意味着：
 * ①没法单测（要起 WebGL 上下文）；②3D 和将来的别的后端会各写一份，
 * 然后位置对不上 —— 而这个项目已经踩过一次同类坑（2D 与导出的 SVG
 * 各写一份分类逻辑，页面红色、纸上黄色）。
 *
 * 所以这里只有数字进、数字出，不做任何请求、不 import three。
 *
 * ## 单位铁律
 *
 * 输入输出**全部是毫米**。three 的世界单位是米，所以只有 `mmToWorld`
 * 一个地方做 /1000。多一处换算就多一处差 1000 倍的机会（画面全白/全黑），
 * 而那种 bug 极难看出方向。
 *
 * ## 与后端的关系
 *
 * 后端 `application/services/scene.py` 给的是 mm；
 * 这里的常量必须与它逐字一致。改一边不改另一边 = 柜子穿模。
 * 有个测试直接比对两边（`scene.test.ts` 读 `constants` 字符串）。
 */

/** EIA-310 标准 1U = 1.75 inch = 44.45mm。硬标准，不是可调参数。 */
export const MM_PER_U = 44.45;

/** 柜体尺寸（mm）。柜宽 600 与布线表 R 列的 `0.6m/柜位` 同源。 */
export const CABINET_WIDTH_MM = 600;
export const CABINET_HEIGHT_MM = 2000;
export const CABINET_DEPTH_MM = 1200;

/** 通道净宽（mm） */
export const AISLE_WIDTH_MM = 1200;

/** 42U 有效空间 = 42 × 44.45 = 1866.9mm，剩下的 133.1mm 是上下框。 */
export const RACK_SPACE_MM = 42 * MM_PER_U;

/** 设备在柜内的底面抬高：42U 有效区在 2000mm 柜体内居中。 */
export const U_BASE_MM = (CABINET_HEIGHT_MM - RACK_SPACE_MM) / 2;

/** 毫米 → three 世界单位（1 单位 = 1 米）。**全项目唯一的 /1000。** */
export function mmToWorld(mm: number): number {
  return mm / 1000;
}

/**
 * U 位 → 柜内底面高度（mm，从柜底内表面算起）。
 *
 * ## 为什么 U1 在底部
 *
 * EIA-310 惯例：U1 在最下面，U42 在最上面。现场装设备是从下往上数。
 * 这条与 2D U 位图必须一致（那里也是 `y = (heightU - u) * unitH`，
 * 42U 画在最上面）—— 两处方向反了，用户会以为 3D 只是个示意图。
 *
 * ## 为什么是 `(u - 1)` 而不是 `u`
 *
 * U 位是 1-based（源表写 `3-4U`）。U1 占 [0, 44.45) 这一段。
 * 写成 `u * MM_PER_U` 会让所有设备整体高一个 U，底面贴不到柜底。
 */
export function uToHeightMm(u: number): number {
  return (u - 1) * MM_PER_U;
}

/** 一台设备在 3D 里的盒子尺寸（mm）。宽深由柜决定，高由 U 数决定。 */
export interface DeviceBoxMm {
  widthMm: number;
  heightMm: number;
  depthMm: number;
  /** 设备底面中心相对柜底面中心的高度（mm） */
  centerYMm: number;
}

/**
 * 设备盒子尺寸。
 *
 * ## 为什么三边都留一点缩进
 *
 * 相邻 U 的设备（3-4U 与 7-8U 之间还有 5-6U 的空隙，但 3-4U 与 5-6U 是紧邻的）
 * 若严格按 2 个 U 高画，两台设备的侧面会完全贴合，WebGL 里出现
 * z-fighting（闪烁的条纹）。留 1mm 缝即可，肉眼看不出但不再闪烁。
 */
export function deviceBoxMm(startU: number, sizeU: number): DeviceBoxMm {
  const heightMm = sizeU * MM_PER_U - 1;
  return {
    widthMm: CABINET_WIDTH_MM - 40,
    heightMm,
    depthMm: CABINET_DEPTH_MM - 120,
    // 底面抬高 + 自身半高
    centerYMm: U_BASE_MM + uToHeightMm(startU) + heightMm / 2,
  };
}

/**
 * 相机取景：让场景包围盒完整落在画面里所需的距离。
 *
 * ## 为什么不能只按「宽度 + 深度」算（第一版算错了）
 *
 * 第一版用 `hypot(width, depth)` 当画面需要覆盖的尺寸，除以 tan(fov/2)。
 * 对机房这种**扁而长**的场景（20400 × 2000 × 25200mm）会算出 49 米的距离 ——
 * 画面里机柜只剩中间一小块，四周大片空白。
 *
 * 错在两点：①俯视时「深度」大部分被压进屏幕纵轴，不是全部占画面；
 * ②斜视时投影范围随视角变化，不存在一个不依赖视角的固定答案。
 *
 * ## 正确做法：把包围盒 8 个角投影到相机自己的轴上
 *
 * 相机的 right / up 两个轴确定后，所有角点在那两个轴上的跨度就是
 * 「画面必须容纳的宽和高」。再按各自视场角求距离，取较大者。
 * 这对任何 theta/phi 都成立，换机房也不用调参数。
 *
 * 数学上等价于 three 里常见的 `fitCameraToObject`，但写成纯函数可单测 ——
 * 而这类「差一点点就看不全」的问题，肉眼极难发现，只能靠数值断言。
 */
export interface BoxMm {
  minX: number;
  maxX: number;
  minY: number;
  maxY: number;
  minZ: number;
  maxZ: number;
}

export function sceneBoxMm(
  rooms: { widthMm: number; depthMm: number; originX: number; originZ: number }[],
  cabinetHeightMm = CABINET_HEIGHT_MM,
): BoxMm {
  if (rooms.length === 0) {
    return { minX: -1000, maxX: 1000, minY: 0, maxY: cabinetHeightMm, minZ: -1000, maxZ: 1000 };
  }
  return {
    minX: Math.min(...rooms.map((r) => r.originX)),
    maxX: Math.max(...rooms.map((r) => r.originX + r.widthMm)),
    minY: 0,
    maxY: cabinetHeightMm,
    minZ: Math.min(...rooms.map((r) => r.originZ)),
    maxZ: Math.max(...rooms.map((r) => r.originZ + r.depthMm)),
  };
}

/** 相机在球坐标下的朝向（单位向量，世界坐标系，y 向上）。 */
function cameraBasis(theta: number, phi: number): {
  forward: [number, number, number];
  right: [number, number, number];
  up: [number, number, number];
} {
  // 从「目标点」看向相机的位置向量 = (sinφcosθ, cosφ, sinφsinθ)
  // forward 是它的反向（相机朝目标看）
  const dir: [number, number, number] = [
    Math.sin(phi) * Math.cos(theta),
    Math.cos(phi),
    Math.sin(phi) * Math.sin(theta),
  ];
  const forward: [number, number, number] = [-dir[0], -dir[1], -dir[2]];
  // right = forward × worldUp，worldUp = (0,1,0)
  const right: [number, number, number] = [
    forward[1] * 0 - forward[2] * 1,
    forward[2] * 0 - forward[0] * 0,
    forward[0] * 1 - forward[1] * 0,
  ];
  const rl = Math.hypot(right[0], right[1], right[2]) || 1;
  const rn: [number, number, number] = [right[0] / rl, right[1] / rl, right[2] / rl];
  // up = right × forward
  const up: [number, number, number] = [
    rn[1] * forward[2] - rn[2] * forward[1],
    rn[2] * forward[0] - rn[0] * forward[2],
    rn[0] * forward[1] - rn[1] * forward[0],
  ];
  return { forward, right: rn, up };
}

/**
 * 求让包围盒完整可见的相机距离（世界单位 = 米）。
 *
 * @param aspect 画布宽高比（>0）。同一场景在宽屏上要比竖屏离得近。
 * @param margin 留白系数。1.15 时四周约 7% 余量 —— 贴着边缘的柜子
 *               会正好卡在画面边界上，视觉上像被裁掉。
 */
export function fitDistanceForView(
  box: BoxMm,
  theta: number,
  phi: number,
  aspect: number,
  fovDeg = 45,
  margin = 1.15,
): number {
  const { right, up } = cameraBasis(theta, phi);
  const c: [number, number, number] = [
    (box.minX + box.maxX) / 2,
    (box.minY + box.maxY) / 2,
    (box.minZ + box.maxZ) / 2,
  ];

  let halfW = 0;
  let halfH = 0;
  for (const x of [box.minX, box.maxX]) {
    for (const y of [box.minY, box.maxY]) {
      for (const z of [box.minZ, box.maxZ]) {
        const d: [number, number, number] = [x - c[0], y - c[1], z - c[2]];
        // 投影到相机右轴 / 上轴。forward 分量不影响「是否在画面内」，
        // 只影响远近（这里取中心距离，等价于正视时的精确解）。
        const pr = d[0] * right[0] + d[1] * right[1] + d[2] * right[2];
        const pu = d[0] * up[0] + d[1] * up[1] + d[2] * up[2];
        halfW = Math.max(halfW, Math.abs(pr));
        halfH = Math.max(halfH, Math.abs(pu));
      }
    }
  }

  const vFov = (fovDeg * Math.PI) / 180;
  const hFov = 2 * Math.atan(Math.tan(vFov / 2) * Math.max(aspect, 0.1));
  // 需要同时满足「宽度装得下」与「高度装得下」，取更远的那个
  const dV = halfH / Math.tan(vFov / 2);
  const dH = halfW / Math.tan(hFov / 2);
  return Math.max(dV, dH, 1) * margin;
}

/** 相机参数（全部是世界单位：米）。 */
export interface CameraFit {
  distance: number;
  /** 注视点（场景包围盒中心，y 取柜高一半 —— 不是地板） */
  targetX: number;
  targetY: number;
  targetZ: number;
}

export function fitCamera(
  rooms: { widthMm: number; depthMm: number; originX: number; originZ: number }[],
  theta = Math.PI / 4,
  phi = Math.PI / 3.2,
  aspect = 16 / 9,
  fovDeg = 45,
): CameraFit {
  const box = sceneBoxMm(rooms);
  return {
    distance: mmToWorld(fitDistanceForView(box, theta, phi, aspect, fovDeg)),
    targetX: mmToWorld((box.minX + box.maxX) / 2),
    // 注视柜高一半而不是地板：看向地板时整个场景会挤在画面上半部，
    // 下方空白一片（实测第一版就是这样）。
    targetY: mmToWorld((box.minY + box.maxY) / 2),
    targetZ: mmToWorld((box.minZ + box.maxZ) / 2),
  };
}

/** 设备类别 → 颜色。取值来自 `styles.css` 的 `--dev-*`（与 2D 同一套配色）。 */
export const CATEGORY_COLOR: Record<string, string> = {
  core: '#7f5af0',
  switch: '#2e7dd1',
  server: '#2f8f6f',
  storage: '#b0821f',
  security: '#b04a5a',
  other: '#4a5568',
};

export function categoryColor(category: string): string {
  // `CATEGORY_COLOR` 的索引签名在 `noUncheckedIndexedAccess` 下返回
  // `string | undefined`，所以兜底要显式收窄，不能只靠 `??`。
  return CATEGORY_COLOR[category] ?? CATEGORY_COLOR.other ?? '#4a5568';
}
