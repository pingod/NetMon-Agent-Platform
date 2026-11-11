/**
 * 3D 机房场景（FR-7.1~7.4、7.6）。
 *
 * ## ★ 一条不可违反的渲染规矩：禁止 `devices.map(d => <mesh/>)`
 *
 * 834 台设备 = 834 个 Mesh = **834 个 draw call**，帧率会掉到 20 以下。
 * `docs/research/02-visualization-stack.md` §3.2 算过账：
 *
 * | 物件 | 数量 | 渲染方式 | draw call |
 * |---|---|---|---|
 * | 机柜 | 362 | 1 个 InstancedMesh | 1 |
 * | 设备（按尺寸分组） | 834 | 每组 1 个 InstancedMesh | ~4 |
 * | 地板 / 房间 | 2 | 普通 Mesh | ~2 |
 * | **合计** | | | **≈ 8** |
 *
 * 8 个 draw call 对任何 GPU 都是空载。所以 AC-7「200 柜 + 3000 设备 ≥30fps」
 * **不是性能挑战，是架构挑战** —— 只要不用「一设备一 Mesh」的写法，
 * 30fps 是保底。
 *
 * ## 为什么不用 r3f 的 `<Instances>` / `<Instance>` 声明式包装
 *
 * 那套 API 每加一个 `<Instance>` 都是一个 React 组件实例。834 个设备就是
 * 834 个组件挂载 —— React 的 reconciliation 成本比 draw call 更早成为瓶颈
 * （首帧要 200ms+）。这里直接用 `useMemo` 算矩阵、`setMatrixAt` 写进
 * 一个 InstancedMesh，React 只渲染**一个**组件。
 *
 * ## 为什么不引 drei / OrbitControls 包
 *
 * 只需要「拖动旋转 + 滚轮缩放」两件事，自己写 30 行 pointer 事件就够。
 * drei 会带进一堆依赖（troika-text、meshline…），而这是要离线交付的项目 ——
 * 每多一个包，绿色包里就多几 MB 和一个版本冲突点。
 *
 * ## 坐标：后端给 mm，这里只做一次 /1000
 *
 * three 的世界单位惯例是米。`mmToWorld()` 是唯一换算点（`domain/scene3d.ts`）。
 * 若在组件里再除一次，画面会缩到 1/1000，看起来像「什么都没渲染」。
 */

import { useEffect, useRef, useState } from 'react';
import type { JSX } from 'react';
import * as THREE from 'three';

import type { ScenePayload } from '../api/client';
import {
  CABINET_DEPTH_MM,
  CABINET_HEIGHT_MM,
  CABINET_WIDTH_MM,
  categoryColor,
  deviceBoxMm,
  fitCamera,
  mmToWorld,
} from '../domain/scene3d';

export interface Scene3DProps {
  scene: ScenePayload;
  /** 点机柜 → 跳去 2D 那个柜（FR-7.3 的联动） */
  onPickRack: (channel: number, rack: number) => void;
  /** 当前选中的机柜（2D 选的，3D 跟着高亮） */
  selectedRack: { channel: number; rack: number } | null;
}

/** 设备按尺寸分组的键：同尺寸才能共用一个 InstancedMesh（几何体相同）。 */
function sizeKey(sizeU: number): string {
  return `${sizeU}U`;
}

export function Scene3D({ scene, onPickRack, selectedRack }: Scene3DProps): JSX.Element {
  const mountRef = useRef<HTMLDivElement | null>(null);
  const [error, setError] = useState<string | null>(null);

  /**
   * 相机球坐标。拖动改 theta/phi、滚轮改 radiusScale。
   *
   * ## 为什么存「缩放倍率」而不是绝对距离
   *
   * `fitCamera()` 算出的距离依赖**视角与画布宽高比**。若把绝对距离存进 ref，
   * 窗口一缩放、或用户转个角度，距离就过期了 —— 表现是「转一下就看不全」。
   *
   * 存倍率（默认 1 = 恰好装下），每次渲染用「当前 theta/phi/aspect 算出的
   * 基准距离 × 倍率」，取景永远正确。滚轮改的只是这个倍率。
   */
  const camRef = useRef<{
    theta: number;
    phi: number;
    radiusScale: number;
  } | null>(null);
  if (camRef.current === null) {
    camRef.current = { theta: Math.PI / 4, phi: Math.PI / 3.2, radiusScale: 1 };
  }

  /** 选中的机柜键，供拾取时比对 */
  const selectedKey = selectedRack ? `${selectedRack.channel}-${selectedRack.rack}` : null;


  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;

    let renderer: THREE.WebGLRenderer;
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false });
    } catch (exc) {
      // 无 GPU / 禁用 WebGL 的环境（CI、远程桌面、虚拟机）会走到这里。
      // 3D 是增强功能，不该让整页挂掉 —— 给出人话并让用户回 2D。
      setError(
        `这台机器的浏览器没能启用 WebGL（${exc instanceof Error ? exc.message : String(exc)}）。` +
          '3D 需要硬件加速，请改用「平面与机柜」页，或检查浏览器设置里是否禁用了硬件加速。',
      );
      return;
    }

    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.setSize(mount.clientWidth, mount.clientHeight);
    renderer.setClearColor(0x0f1319, 1);
    mount.appendChild(renderer.domElement);

    const sceneObj = new THREE.Scene();

    const camera = new THREE.PerspectiveCamera(
      45,
      mount.clientWidth / Math.max(1, mount.clientHeight),
      0.05,
      500,
    );

    // ---- 相机：按当前视角 + 画布宽高比求一个恰好装下场景的距离 ----
    const cam = camRef.current!;
    let fit = fitCamera(
      scene.rooms,
      cam.theta,
      cam.phi,
      mount.clientWidth / Math.max(1, mount.clientHeight),
    );

    // ---- 灯光：环境光 + 一盏方向光，够看清盒子六个面 ----
    // 不用阴影（362 个实例投射阴影会翻倍 draw call），改用「面朝向带来的
    // 明暗差」表达立体感 —— 这是 3D 里最省的立体感做法。
    sceneObj.add(new THREE.AmbientLight(0xffffff, 1.6));
    const dir = new THREE.DirectionalLight(0xffffff, 1.6);
    dir.position.set(1, 2, 1.5);
    sceneObj.add(dir);
    const dir2 = new THREE.DirectionalLight(0x88aaff, 0.5);
    dir2.position.set(-1, 1, -1.5);
    sceneObj.add(dir2);

    // ---- 地板：每个房间一块，深色区分房间边界 ----
    //
    // `side: DoubleSide` 是必需的：`PlaneGeometry` 的正面朝 +z，
    // 从相机（在 +y 上方斜看）看过去时**看到的是背面**，单面材质会直接剔掉 ——
    // 表现是「地板完全不出现，场景飘在黑色虚空里」（实测第一版就是这样）。
    // 用 `MeshBasicMaterial` 而不是 Lambert：地板不需要立体感，
    // 而 Basic 不参与光照，就不存在「光照方向不对导致一片死黑」的可能。
    const floorMat = new THREE.MeshBasicMaterial({
      color: 0x1e2734,
      side: THREE.DoubleSide,
    });
    for (const room of scene.rooms) {
      const geo = new THREE.PlaneGeometry(
        mmToWorld(room.widthMm),
        mmToWorld(room.depthMm),
      );
      const mesh = new THREE.Mesh(geo, floorMat);
      mesh.rotation.x = -Math.PI / 2;
      mesh.position.set(
        mmToWorld(room.originX + room.widthMm / 2),
        0,
        mmToWorld(room.originZ + room.depthMm / 2),
      );
      sceneObj.add(mesh);
    }

    // ---- 机柜：不透明底盒 + 顶面亮条 ----
    //
    // ## 柜体怎么画（试过三种，只有第三种对）
    //
    // ① **半透明实体**（opacity 0.22~0.35）：362 个透明盒子互相叠加，
    //    WebGL 对同尺寸物体的透明排序不稳定，颜色越积越暗 ——
    //    实测截图是一整片黑雾，后排的柜把前排糊住。
    // ② **半透明实体 + `EdgesGeometry` 线框**：线框在 `InstancedMesh` 里
    //    是 `LineSegments` 几何，SwiftShader/ANGLE 下渲染成放射状尖刺。
    //    这条路在软件渲染环境不可用，而**客户现场很可能就是没有独显的
    //    跳板机** —— 不能赌它有硬件加速。
    // ③ **背面渲染的不透明盒 + 顶面亮条**（采用）：
    //    底盒用 `MeshBasicMaterial` + `side: BackSide` —— **只画内壁**。
    //    这是 3D 里做「看得进去的柜子」的经典做法：
    //    正面被剔除，视线直接穿到柜内设备上，但四个侧壁与底面的内表面
    //    仍然参与深度测试，于是每个柜看起来像一个「敞开的抽屉」。
    //    既没有透明排序问题（材质仍是不透明），又能看见柜里的设备。
    //
    //    为什么不用 `DoubleSide`：那样正面会挡住设备，等于回到①的问题。
    //    为什么用 `MeshBasicMaterial`：Lambert 会因光照方向导致某些侧面全黑，
    //    而柜体只需表达"边界在哪"，不需要光影。
    const cabGeo = new THREE.BoxGeometry(
      mmToWorld(CABINET_WIDTH_MM),
      mmToWorld(CABINET_HEIGHT_MM),
      mmToWorld(CABINET_DEPTH_MM),
    );
    const cabMat = new THREE.MeshBasicMaterial({
      color: 0xffffff,
      // 只画内壁 —— 见上方 ③ 的说明
      side: THREE.BackSide,
    });
    const cabinetMesh = new THREE.InstancedMesh(cabGeo, cabMat, scene.racks.length);
    cabinetMesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);

    // 顶面亮条：宽/深 90%、厚 20mm，贴在柜顶
    const capGeo = new THREE.BoxGeometry(
      mmToWorld(CABINET_WIDTH_MM * 0.9),
      mmToWorld(20),
      mmToWorld(CABINET_DEPTH_MM * 0.9),
    );
    // 顶面亮条用 FrontSide：柜体是 BackSide（只画内壁），但亮条要
    // 从**外面**看得见 —— 跟着 BackSide 会翻到柜底，柜顶一片空。
    const capMat = new THREE.MeshBasicMaterial({ color: 0xffffff });
    const capMesh = new THREE.InstancedMesh(capGeo, capMat, scene.racks.length);
    capMesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);

    const m4 = new THREE.Matrix4();
    const q = new THREE.Quaternion();
    const yAxis = new THREE.Vector3(0, 1, 0);
    const scaleV = new THREE.Vector3(1, 1, 1);
    const posV = new THREE.Vector3();
    const color = new THREE.Color();

    scene.racks.forEach((r, i) => {
      q.setFromAxisAngle(yAxis, (r.rotationY * Math.PI) / 180);

      // 柜体：中心在 y = 柜高一半
      posV.set(mmToWorld(r.x), mmToWorld(r.y), mmToWorld(r.z));
      m4.compose(posV, q, scaleV);
      cabinetMesh.setMatrixAt(i, m4);

      // 顶面亮条：贴在柜顶
      posV.set(mmToWorld(r.x), mmToWorld(CABINET_HEIGHT_MM + 10), mmToWorld(r.z));
      m4.compose(posV, q, scaleV);
      capMesh.setMatrixAt(i, m4);

      const isSel = selectedKey === `${r.channel}-${r.rack}`;
      // 选中亮蓝 / 有设备中灰蓝 / 空柜深灰 —— 施工时一眼看出哪几个柜空着
      cabinetMesh.setColorAt(
        i,
        color.set(isSel ? '#3f6fd0' : r.deviceCount === 0 ? '#1c2431' : '#2a3546'),
      );
      capMesh.setColorAt(
        i,
        color.set(isSel ? '#8fb4ff' : r.deviceCount === 0 ? '#28323f' : '#4a6183'),
      );
    });
    cabinetMesh.instanceMatrix.needsUpdate = true;
    if (cabinetMesh.instanceColor) cabinetMesh.instanceColor.needsUpdate = true;
    capMesh.instanceMatrix.needsUpdate = true;
    if (capMesh.instanceColor) capMesh.instanceColor.needsUpdate = true;
    sceneObj.add(cabinetMesh);
    sceneObj.add(capMesh);

    // ---- 设备：按 U 数分组，每组 1 个 InstancedMesh ----
    // 为什么按 U 数分组：InstancedMesh 要求所有实例共用同一个几何体。
    // 1U/2U/4U/10U 高度不同，只能各建一组。实测 V13 只有 6 种 U 数。
    const bySize = new Map<string, typeof scene.devices>();
    for (const d of scene.devices) {
      const k = sizeKey(d.sizeU);
      const arr = bySize.get(k) ?? [];
      arr.push(d);
      bySize.set(k, arr);
    }

    const rackByIdx = new Map<string, (typeof scene.racks)[number]>();
    for (const r of scene.racks) rackByIdx.set(`${r.channel}-${r.rack}`, r);

    const deviceMeshes: THREE.InstancedMesh[] = [];
    for (const [key, items] of bySize) {
      const first = items[0];
      if (!first) continue;
      const sizeU = first.sizeU;
      const box = deviceBoxMm(1, sizeU);
      const geo = new THREE.BoxGeometry(
        mmToWorld(box.widthMm),
        mmToWorld(box.heightMm),
        mmToWorld(box.depthMm),
      );
      // 设备用 Lambert（要立体感，看得出是"一台机器"而不是一块色块）。
      // 灯光已设两盏方向光，六个面都有明暗差。
      const mat = new THREE.MeshLambertMaterial({ color: 0xffffff });
      const mesh = new THREE.InstancedMesh(geo, mat, items.length);
      mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
      // 实例颜色：一个 draw call 出全场景配色
      items.forEach((d, i) => {
        const rack = rackByIdx.get(`${d.channel}-${d.rack}`);
        if (!rack) {
          m4.identity();
          mesh.setMatrixAt(i, m4);
          return;
        }
        const b = deviceBoxMm(d.startU, d.sizeU);
        // 设备在柜内：x/z 跟柜，y 由 U 位算。
        // 柜旋转 180° 时设备位置不变（设备在柜内，不随门转）——
        // 随门转会把它甩到柜外，3D 里看就是「设备飘在空中」。
        posV.set(mmToWorld(rack.x), mmToWorld(b.centerYMm), mmToWorld(rack.z));
        q.identity();
        m4.compose(posV, q, scaleV);
        mesh.setMatrixAt(i, m4);
        mesh.setColorAt(i, color.set(categoryColor(d.category)));
      });
      mesh.instanceMatrix.needsUpdate = true;
      if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true;
      mesh.userData.sizeKey = key;
      deviceMeshes.push(mesh);
      sceneObj.add(mesh);
    }

    // ---- 相机控制：拖动转、滚轮缩、双击回正 ----
    let dragging = false;
    let lastX = 0;
    let lastY = 0;

    const applyCamera = () => {
      const c = camRef.current!;
      fit = fitCamera(
        scene.rooms,
        c.theta,
        c.phi,
        mount.clientWidth / Math.max(1, mount.clientHeight),
      );
      const r = fit.distance * c.radiusScale;
      camera.position.set(
        fit.targetX + r * Math.sin(c.phi) * Math.cos(c.theta),
        fit.targetY + r * Math.cos(c.phi),
        fit.targetZ + r * Math.sin(c.phi) * Math.sin(c.theta),
      );
      // 注视包围盒中心（柜高一半），不是地板 —— 看地板会让场景挤在画面上面
      camera.lookAt(fit.targetX, fit.targetY, fit.targetZ);
    };
    applyCamera();

    const onPointerDown = (e: PointerEvent) => {
      dragging = true;
      lastX = e.clientX;
      lastY = e.clientY;
      renderer.domElement.setPointerCapture(e.pointerId);
    };
    const onPointerMove = (e: PointerEvent) => {
      if (!dragging) return;
      const c = camRef.current!;
      c.theta -= (e.clientX - lastX) * 0.005;
      // phi 卡在 (0.05, PI/2 - 0.02)：不让相机穿到地板下面，
      // 也不让它正对天花板（那时只能看到一个平面，没有立体感）
      c.phi = Math.max(0.05, Math.min(Math.PI / 2 - 0.02, c.phi - (e.clientY - lastY) * 0.004));
      lastX = e.clientX;
      lastY = e.clientY;
      applyCamera();
    };
    const onPointerUp = (e: PointerEvent) => {
      dragging = false;
      try {
        renderer.domElement.releasePointerCapture(e.pointerId);
      } catch {
        /* 指针已释放，无害 */
      }
    };
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const c = camRef.current!;
      // 指数缩放：线性缩放时远处滚一格跳太多、近处几乎不动
      c.radiusScale = Math.max(0.15, Math.min(4, c.radiusScale * (e.deltaY > 0 ? 1.1 : 0.9)));
      applyCamera();
    };

    const onResize = () => {
      if (!mount) return;
      camera.aspect = mount.clientWidth / Math.max(1, mount.clientHeight);
      camera.updateProjectionMatrix();
      renderer.setSize(mount.clientWidth, mount.clientHeight);
      // 宽高比变了 → 「装下场景」所需的距离也变了，必须重算，
      // 否则窗口拉窄之后左右两侧的柜子被裁掉
      applyCamera();
    };

    renderer.domElement.addEventListener('pointerdown', onPointerDown);
    renderer.domElement.addEventListener('pointermove', onPointerMove);
    renderer.domElement.addEventListener('pointerup', onPointerUp);
    renderer.domElement.addEventListener('pointercancel', onPointerUp);
    renderer.domElement.addEventListener('wheel', onWheel, { passive: false });
    window.addEventListener('resize', onResize);

    // ---- 拾取：点机柜 → 通知父组件切 2D（FR-7.3） ----
    const raycaster = new THREE.Raycaster();
    const ndc = new THREE.Vector2();
    const onClick = (e: MouseEvent) => {
      // 拖动结束那一下也会触发 click。用「按下到抬起位移 < 4px」区分
      // —— 不加这个判断，每次转视角都会误跳去某个机柜。
      const rect = renderer.domElement.getBoundingClientRect();
      ndc.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      ndc.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
      raycaster.setFromCamera(ndc, camera);
      // 先打柜体、再打顶面亮条 —— 从正上方俯视时射线只擦到顶面、
      // 打不进柜体侧面，所以两者都要试。
      const hit =
        raycaster.intersectObject(cabinetMesh, false)[0] ??
        raycaster.intersectObject(capMesh, false)[0];
      if (hit && hit.instanceId !== undefined) {
        const r = scene.racks[hit.instanceId];
        if (r) onPickRack(r.channel, r.rack);
      }
    };
    let downAt: { x: number; y: number } | null = null;
    const onDownForClick = (e: PointerEvent) => {
      downAt = { x: e.clientX, y: e.clientY };
    };
    const onClickGuarded = (e: MouseEvent) => {
      if (!downAt) return;
      const moved = Math.hypot(e.clientX - downAt.x, e.clientY - downAt.y);
      downAt = null;
      if (moved < 4) onClick(e);
    };
    renderer.domElement.addEventListener('pointerdown', onDownForClick);
    renderer.domElement.addEventListener('click', onClickGuarded);

    let raf = 0;
    const loop = () => {
      renderer.render(sceneObj, camera);
      raf = requestAnimationFrame(loop);
    };
    loop();

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', onResize);
      renderer.domElement.removeEventListener('pointerdown', onPointerDown);
      renderer.domElement.removeEventListener('pointermove', onPointerMove);
      renderer.domElement.removeEventListener('pointerup', onPointerUp);
      renderer.domElement.removeEventListener('pointercancel', onPointerUp);
      renderer.domElement.removeEventListener('wheel', onWheel);
      renderer.domElement.removeEventListener('pointerdown', onDownForClick);
      renderer.domElement.removeEventListener('click', onClickGuarded);
      // 显式释放 GPU 资源：反复切页签而不 dispose 会持续吃掉显存，
      // 几百次之后 WebGL 上下文被浏览器回收，页面变空白。
      cabGeo.dispose();
      cabMat.dispose();
      floorMat.dispose();
      for (const m of deviceMeshes) {
        m.geometry.dispose();
        (m.material as THREE.Material).dispose();
        m.dispose();
      }
      cabinetMesh.dispose();
      capGeo.dispose();
      capMat.dispose();
      capMesh.dispose();
      renderer.dispose();
      if (renderer.domElement.parentNode === mount) {
        mount.removeChild(renderer.domElement);
      }
    };
    // 依赖 `selectedKey` 是故意的：选中变化要重绘实例颜色。
    // 全部场景重建约 30ms（834 个矩阵），可接受；换来的是「2D 选中 → 3D 高亮」
    // 这条联动不需要另写一套「只更新颜色」的增量代码。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scene, selectedKey, onPickRack]);

  if (error) {
    return (
      <div className="view-placeholder scene3d-error">
        <b>3D 不可用</b>
        <p>{error}</p>
      </div>
    );
  }

  return (
    <div className="scene3d">
      <div ref={mountRef} className="scene3d-canvas" />
      <div className="scene3d-hint">
        拖动旋转 · 滚轮缩放 · 点机柜看它的 U 位图
      </div>
    </div>
  );
}
