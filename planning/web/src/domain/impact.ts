/**
 * 拖动影响面的「人话」描述（纯函数，可单测）。
 *
 * ## 为什么这件事要单独一个纯函数
 *
 * 这段文案出现在用户松手后的第一眼。写错的表现不是报错，而是**说了假话**：
 *
 * - 说「线长没变」而其实**没算**（源文件缺失时 `lengthChecked=false`）——
 *   用户以为安全，现场拿到的是错的长度；
 * - 漏掉端口重编号 —— 用户以为只动了一台，实际 91 行端口变了，
 *   按旧表插线全插错口；
 * - 移动失败时还说「删除了 N 行」—— 一个失败的动作配一段最坏的消息。
 *
 * 这三种错误都不会崩溃，只会让人做出错误判断。所以文案逻辑抽出来单测，
 * 而不是散在组件的三元表达式里。
 *
 * ## 为什么数字要「分句」而不是合并成一个总数
 *
 * 「共 91 处变化」对用户毫无意义。他要知道的是三件不同的事：
 * 端口号变了（**改标签**）、线长变了（**改采购/施工**）、行增删了（**改设计**）。
 * 这三件事的处置完全不同，所以必须分开说。
 */

import type { ImpactPayload } from '../api/client';

/** 影响面里最要紧的那一句。`null` = 没有值得提醒的事。 */
export function impactHeadline(impact: ImpactPayload | null): string | null {
  if (!impact) return null;
  if (!impact.ok) return null;
  if (!impact.hasHiddenImpact) return null;

  const parts: string[] = [];
  const c = impact.counts;

  if (c.added > 0 || c.removed > 0) {
    const bits: string[] = [];
    if (c.added > 0) bits.push(`新增 ${c.added} 行`);
    if (c.removed > 0) bits.push(`删除 ${c.removed} 行`);
    parts.push(`${bits.join('、')}（接线拓扑变了，要重新看设计）`);
  }
  if (c.portChanges > 0) {
    parts.push(
      `${c.portChanges} 行端口号变了` +
        (c.affectedSwitches > 0 ? `（涉及 ${c.affectedSwitches} 台交换机，标签要重打）` : ''),
    );
  }
  if (impact.lengthChecked) {
    if (c.lengthChanges > 0) parts.push(`${c.lengthChanges} 行线长变了`);
  } else if (c.positionChanges > 0) {
    // 不能说「线长没变」—— 我们根本没算。这一句是防止误导的关键。
    parts.push('线长未校验（缺平面图源文件）');
  }

  return parts.length > 0 ? `这次改动影响：${parts.join('；')}。` : null;
}

/**
 * 干跑预演的提示语（**还没发生**，与 `impactHeadline` 的「已经改了」区分）。
 *
 * ## 为什么必须换一套措辞
 *
 * 同一个数字，两句话的意思完全不同：
 * - 「这次改动影响：91 行端口号变了」 = **已经改了**，只能撤销
 * - 「松手会改 91 行端口号」 = **还没改**，还能松手到别处
 *
 * 混用会让用户分不清自己是在读结果还是在读预测，从而错过还能反悔的窗口。
 */
export function previewHeadline(impact: ImpactPayload | null): string | null {
  if (!impact || !impact.ok) return null;
  if (!impact.hasHiddenImpact) return null;
  const parts: string[] = [];
  const c = impact.counts;
  if (c.added > 0 || c.removed > 0) parts.push(`接线会变（+${c.added}/-${c.removed} 行）`);
  if (c.portChanges > 0) {
    parts.push(
      `${c.portChanges} 行端口号会变` +
        (c.affectedSwitches > 0 ? `（涉及 ${c.affectedSwitches} 台交换机）` : ''),
    );
  }
  // 预演恒为 `deep=false`，所以线长一律「未校验」，不能说「线长不变」
  if (c.positionChanges > 0) parts.push('线长未校验');
  return parts.length > 0 ? `松手会：${parts.join('；')}。改完可撤销。` : null;
}

/**
 * 影响面的明细行（给横幅里的第二行，比 headline 细但仍是人话）。
 *
 * 为什么单独拆出来：`headline` 要短到一眼能读完，明细要考虑「哪台交换机」。
 * 两件事的文字长度差一个数量级，塞在同一句里必然要截断。
 */
export function impactDetail(impact: ImpactPayload | null, limit = 3): string[] {
  if (!impact || !impact.ok) return [];
  const out: string[] = [];

  if (impact.affectedSwitches.length > 0) {
    const shown = impact.affectedSwitches.slice(0, limit).join('、');
    const more = impact.affectedSwitches.length - limit;
    out.push(`端口被动过的交换机：${shown}${more > 0 ? ` 等 ${impact.affectedSwitches.length} 台` : ''}`);
  }

  for (const s of impact.portSamples.slice(0, limit)) {
    if (!s.before || !s.after) continue;
    out.push(`${s.switch} → ${s.node}（${s.nic}）：${s.before} → ${s.after}`);
  }

  for (const s of impact.lengthSamples.slice(0, limit)) {
    const sign = s.delta > 0 ? '+' : '';
    out.push(`${s.switch} → ${s.node}（${s.nic}）：${s.before} 米 → ${s.after} 米（${sign}${s.delta}）`);
  }

  for (const s of impact.addedSamples.slice(0, limit)) {
    out.push(`新增：${s.switch} ${s.port ?? '?'} → ${s.node} ${s.peerPort}`);
  }
  for (const s of impact.removedSamples.slice(0, limit)) {
    out.push(`删除：${s.switch} ${s.port ?? '?'} → ${s.node} ${s.peerPort}`);
  }

  return out;
}

/**
 * 横幅的严重程度 —— 决定用不用红色。
 *
 * 判据是「有没有动到接线拓扑」：只有端口重编号与线长变化时用黄色提醒
 * （要重打标签，但设计是对的）；一旦增删了连线就是改设计，用红色。
 */
export function impactSeverity(impact: ImpactPayload | null): 'none' | 'warn' | 'danger' {
  if (!impact || !impact.ok) return 'none';
  if (!impact.hasHiddenImpact) return 'none';
  if (impact.counts.added > 0 || impact.counts.removed > 0) return 'danger';
  return 'warn';
}
