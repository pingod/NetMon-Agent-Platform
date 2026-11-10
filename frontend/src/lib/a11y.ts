/**
 * 可访问性增强：
 * 历史页面里存在大量“仅图标按钮”，没有 aria-label / title。
 * 这里作为统一兜底，在 DOM 变化时为无名称的交互元素补上可读名称，
 * 并为纯图标按钮补充 title（鼠标悬浮提示）。
 * 注意：这是兜底，不是替代页面内显式 aria-label。
 */

const ICON_LABELS: Record<string, string> = {
  'trash-2': '删除',
  'trash': '删除',
  'pencil': '编辑',
  'edit': '编辑',
  'eye': '查看',
  'eye-off': '隐藏',
  'copy': '复制',
  'download': '下载',
  'upload': '上传',
  'refresh-cw': '刷新',
  'refresh-ccw': '刷新',
  'rotate-ccw': '重置',
  'x': '关闭',
  'plus': '新增',
  'minus': '移除',
  'search': '搜索',
  'settings': '设置',
  'filter': '筛选',
  'more-horizontal': '更多操作',
  'more-vertical': '更多操作',
  'external-link': '打开链接',
  'play': '执行',
  'pause': '暂停',
  'check': '确认',
  'chevron-left': '上一页',
  'chevron-right': '下一页',
  'chevron-down': '展开',
  'chevron-up': '收起',
  'arrow-left': '返回',
  'arrow-right': '前进',
  'menu': '菜单',
  'lock': '锁定',
  'unlock': '解锁',
  'key': '密钥',
  'server': '服务器',
  'terminal': '终端',
  'zap': '执行',
  'save': '保存',
  'send': '发送',
  'bell': '通知',
  'info': '信息',
  'circle-help': '帮助',
  'calendar': '日历',
  'clock': '时间',
  'list': '列表',
  'layout-grid': '网格视图',
  'maximize': '放大',
  'minimize': '缩小',
  'zoom-in': '放大',
  'zoom-out': '缩小',
  'folder-open': '打开目录',
  'file-text': '文件',
  'image': '图片',
  'link': '链接',
  'wifi': '网络',
  'activity': '活动',
  'shield': '安全',
  'user': '用户',
  'users': '用户',
  'log-out': '退出登录',
  'sun': '浅色模式',
  'moon': '深色模式',
  'close-circle': '清除',
  'down': '展开',
  'up': '收起',
};

const ANTD_LABELS: Record<string, string> = {
  'ant-input-clear-icon': '清除输入',
  'ant-tabs-nav-more': '更多标签',
  'ant-switch': '切换',
  'ant-pagination-prev': '上一页',
  'ant-pagination-next': '下一页',
  'ant-modal-close': '关闭',
  'ant-drawer-close': '关闭',
  'ant-select-arrow': '展开选项',
  'ant-picker-clear': '清除',
};

function iconKeyFrom(el: Element): string | null {
  const svg = el.querySelector('svg');
  if (svg) {
    const cls = svg.getAttribute('class') || '';
    const lucide = cls.match(/lucide-([a-z0-9-]+)/);
    if (lucide) return lucide[1];
  }
  const anticon = el.querySelector('.anticon');
  if (anticon) {
    const cls = anticon.getAttribute('class') || '';
    const m = cls.match(/anticon-([a-z0-9-]+)/);
    if (m) return m[1];
    const aria = anticon.getAttribute('aria-label');
    if (aria) return aria;
  }
  return null;
}

function classLabel(el: Element): string | null {
  const cls = el.getAttribute('class') || '';
  for (const [key, label] of Object.entries(ANTD_LABELS)) {
    if (cls.includes(key)) return label;
  }
  return null;
}

function hasAccessibleName(el: Element): boolean {
  if ((el.getAttribute('aria-label') || '').trim()) return true;
  if ((el.getAttribute('title') || '').trim()) return true;
  const text = (el.textContent || '').trim();
  if (text) return true;
  const img = el.querySelector('img[alt]');
  return Boolean(img && (img.getAttribute('alt') || '').trim());
}

function enhanceElement(el: Element) {
  if (!(el instanceof HTMLElement)) return;
  if (el.getAttribute('aria-hidden') === 'true') return;
  if (el.closest('svg')) return;

  if (el.matches('button, a[href], [role="button"]')) {
    if (hasAccessibleName(el)) return;
    const key = iconKeyFrom(el);
    const label =
      (key && ICON_LABELS[key]) || classLabel(el) || (key ? key.replace(/-/g, ' ') : null);
    if (label) {
      el.setAttribute('aria-label', label);
      if (!el.getAttribute('title')) el.setAttribute('title', label);
    }
    return;
  }

  if (el.matches('input, select, textarea')) {
    const input = el as HTMLInputElement;
    if (input.type === 'hidden' || input.disabled) return;
    if (input.getAttribute('aria-label') || input.getAttribute('aria-labelledby')) return;
    if (input.id && document.querySelector(`label[for="${CSS.escape(input.id)}"]`)) return;
    if (input.closest('label')) return;
    const label =
      input.getAttribute('placeholder') ||
      input.getAttribute('name') ||
      input.getAttribute('title') ||
      '';
    if (label.trim()) input.setAttribute('aria-label', label.trim());
  }
}

function enhanceTree(root: ParentNode) {
  if (root instanceof Element) enhanceElement(root);
  root
    .querySelectorAll?.('button, a[href], [role="button"], input, select, textarea')
    .forEach(enhanceElement);
}

/** 安装 DOM 兜底增强（幂等） */
export function installA11yEnhancer() {
  if (typeof window === 'undefined') return;
  const w = window as Window & { __a11yEnhancerInstalled?: boolean };
  if (w.__a11yEnhancerInstalled) return;
  w.__a11yEnhancerInstalled = true;

  let scheduled = false;
  const scan = () => {
    scheduled = false;
    enhanceTree(document.body);
  };

  scan();

  const observer = new MutationObserver((mutations) => {
    for (const m of mutations) {
      m.addedNodes.forEach((node) => {
        if (node.nodeType === 1) enhanceTree(node as Element);
      });
    }
    if (!scheduled) {
      scheduled = true;
      window.requestAnimationFrame(scan);
    }
  });

  observer.observe(document.body, { childList: true, subtree: true });
}
