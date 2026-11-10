import type { NavGroup } from '../config/navigation';
import { navigationGroups } from '../config/navigation';

/**
 * 构建侧边栏菜单：过滤隐藏项，并按 href 去重（配置里存在跨分组重复入口）。
 * 只影响展示，不影响路由。
 */
export function buildVisibleNavigation(
  hiddenGroups: string[],
  hiddenItems: string[]
): NavGroup[] {
  const seen = new Set<string>();
  return navigationGroups
    .filter((group) => !hiddenGroups.includes(group.name))
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => {
        if (hiddenItems.includes(item.href) || seen.has(item.href)) return false;
        seen.add(item.href);
        return true;
      }),
    }))
    .filter((group) => group.items.length > 0);
}

/** 完整菜单目录（去重后），用于系统设置里的显隐管理 */
export function getMenuCatalog(): NavGroup[] {
  const seen = new Set<string>();
  return navigationGroups.map((group) => ({
    ...group,
    items: group.items.filter((item) => {
      if (seen.has(item.href)) return false;
      seen.add(item.href);
      return true;
    }),
  }));
}

/** 找到当前路由所属的分组与菜单项，用于顶栏面包屑 */
export function findActiveNav(pathname: string) {
  for (const group of navigationGroups) {
    const item = group.items.find(
      (entry) => pathname === entry.href || pathname.startsWith(`${entry.href}/`)
    );
    if (item) return { group, item };
  }
  return null;
}
