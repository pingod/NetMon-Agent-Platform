import { useEffect, useMemo, useState } from 'react';
import { Outlet, NavLink, useLocation, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import clsx from 'clsx';
import {
  ChevronDown,
  ChevronRight,
  LogOut,
  Menu as MenuIcon,
  Search,
  User as UserIcon,
  X,
} from 'lucide-react';
import api from '../../lib/api';
import { useAuth } from '../../contexts/AuthContext';
import { useUIPreferences } from '../../contexts/UIPreferencesContext';
import ChatWidget from '../../modules/ai/components/ChatWidget';
import ThemeSwitcher from '../components/ThemeSwitcher';
import { buildVisibleNavigation, findActiveNav } from '../../lib/navigation';

export default function Layout() {
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const { hiddenGroups, hiddenItems } = useUIPreferences();
  const navigate = useNavigate();
  const location = useLocation();

  const [mobileOpen, setMobileOpen] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState('');

  const activeNav = useMemo(() => findActiveNav(location.pathname), [location.pathname]);

  const groups = useMemo(() => {
    const visible = buildVisibleNavigation(hiddenGroups, hiddenItems);
    const keyword = query.trim().toLowerCase();
    if (!keyword) return visible;
    return visible
      .map((group) => ({
        ...group,
        items: group.items.filter((item) =>
          t(item.name).toLowerCase().includes(keyword)
        ),
      }))
      .filter((group) => group.items.length > 0);
  }, [hiddenGroups, hiddenItems, query, t]);

  // 首次进入/切换路由：自动展开当前分组（搜索时全部展开）
  useEffect(() => {
    if (query.trim()) {
      setExpandedGroups(new Set(groups.map((g) => g.name)));
      return;
    }
    if (activeNav) {
      setExpandedGroups((prev) => {
        if (prev.has(activeNav.group.name)) return prev;
        const next = new Set(prev);
        next.add(activeNav.group.name);
        return next;
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeNav?.group.name, query]);

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  const toggleGroup = (groupName: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(groupName)) next.delete(groupName);
      else next.add(groupName);
      return next;
    });
  };

  const allExpanded = groups.length > 0 && groups.every((g) => expandedGroups.has(g.name));
  const toggleAllGroups = () => {
    setExpandedGroups(allExpanded ? new Set() : new Set(groups.map((g) => g.name)));
  };

  const { data: agentCount } = useQuery({
    queryKey: ['agents-count'],
    queryFn: async () => {
      const res = await api.get('/agents');
      return (res.data.data as Array<{ enabled: number }>).filter((a) => a.enabled === 1).length;
    },
    refetchInterval: 60000,
    staleTime: 5 * 60 * 1000,
  });

  const { data: workflowCount } = useQuery({
    queryKey: ['workflows-count'],
    queryFn: async () => {
      const res = await api.get('/workflows');
      return (res.data.data as Array<{ is_template: number }>).filter((w) => w.is_template === 1)
        .length;
    },
    refetchInterval: 60000,
    staleTime: 5 * 60 * 1000,
  });

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const roleText = (role: string) => {
    const roleMap: Record<string, string> = {
      admin: t('user.admin'),
      operator: t('user.operator'),
      viewer: t('user.viewer'),
    };
    return roleMap[role] || role;
  };

  return (
    <div className="flex h-screen overflow-hidden">
      {mobileOpen && (
        <button
          type="button"
          aria-label="close menu"
          className="fixed inset-0 z-40 bg-black/55 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <aside
        className={clsx(
          'sidebar fixed inset-y-0 left-0 z-50 flex w-[268px] flex-col transition-transform duration-300 lg:static lg:translate-x-0',
          mobileOpen ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        {/* 品牌 */}
        <div className="flex h-16 shrink-0 items-center gap-3 border-b border-border px-4">
          <div className="relative flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-purple-500/90 to-primary-strong shadow-[0_8px_24px_-10px_rgba(109,94,252,.9)]">
            <img
              src="/favicon.svg"
              alt="NetMon"
              className="h-full w-full object-contain p-1"
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = 'none';
              }}
            />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-[15px] font-semibold leading-tight tracking-tight text-text-primary">
              {t('app.title')}
            </p>
            <p className="truncate text-[10.5px] tracking-[0.06em] text-text-tertiary">{t('app.brandTagline')}</p>
          </div>
          <button
            type="button"
            className="btn btn-ghost btn-icon lg:hidden"
            onClick={() => setMobileOpen(false)}
            aria-label="close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 搜索 */}
        <div className="px-3 pt-3">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-text-tertiary" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('app.searchMenu')}
              className="input-base h-9 pl-8 text-[12.5px]"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery('')}
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-text-tertiary hover:text-text-primary"
                aria-label="clear"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
          <div className="mt-2 flex items-center justify-between px-1">
            <span className="text-[11px] font-medium uppercase tracking-[0.12em] text-text-tertiary">
              {t('app.menu')}
            </span>
            <button
              type="button"
              onClick={toggleAllGroups}
              className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-text-tertiary transition-colors hover:bg-surface-hover hover:text-text-primary"
            >
              {allExpanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
              {allExpanded ? t('app.collapseAll') : t('app.expandAll')}
            </button>
          </div>
        </div>

        {/* 导航 */}
        <nav className="mt-2 flex-1 space-y-2 overflow-y-auto px-3 pb-4">
          {groups.length === 0 && (
            <p className="px-2 py-6 text-center text-[12px] leading-relaxed text-text-tertiary">
              {t('app.menuEmpty')}
            </p>
          )}
          {groups.map((group) => {
            const expanded = expandedGroups.has(group.name);
            const GroupIcon = group.icon;
            return (
              <div key={group.name}>
                <button
                  type="button"
                  onClick={() => toggleGroup(group.name)}
                  className="nav-group-label"
                  aria-expanded={expanded}
                >
                  <GroupIcon className="h-3.5 w-3.5 shrink-0" />
                  <span className="flex-1 text-left">{t(group.name)}</span>
                  {expanded ? (
                    <ChevronDown className="h-3.5 w-3.5 shrink-0 opacity-70" />
                  ) : (
                    <ChevronRight className="h-3.5 w-3.5 shrink-0 opacity-70" />
                  )}
                </button>
                {expanded && (
                  <div className="mt-1 space-y-0.5 pl-1">
                    {group.items.map((item) => {
                      const ItemIcon = item.icon;
                      return (
                        <NavLink
                          key={item.href}
                          to={item.href}
                          className={({ isActive }) =>
                            clsx('nav-item', isActive && 'is-active')
                          }
                        >
                          <ItemIcon className="h-[15px] w-[15px] shrink-0 opacity-85" />
                          <span className="truncate">{t(item.name)}</span>
                        </NavLink>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </nav>

        {/* 底部状态 */}
        <div className="shrink-0 border-t border-border px-3 py-3">
          <div className="flex items-center gap-2.5 rounded-xl border border-border bg-surface-2 px-3 py-2.5">
            <span className="relative flex h-2 w-2 shrink-0">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400/60" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-400" />
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-[12px] font-medium leading-tight text-text-primary">
                {t('app.systemNormal')}
              </p>
              <p className="truncate text-[10.5px] leading-tight text-text-tertiary">
                {agentCount ?? '—'} Agent · {workflowCount ?? '—'} Workflow
              </p>
            </div>
          </div>
        </div>
      </aside>

      {/* 主区域 */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border app-header px-4 backdrop-blur-xl lg:px-7">
          <button
            type="button"
            className="btn btn-ghost btn-icon lg:hidden"
            onClick={() => setMobileOpen(true)}
            aria-label="open menu"
          >
            <MenuIcon className="h-4 w-4" />
          </button>

          <div className="min-w-0 flex-1">
            {activeNav ? (
              <div className="flex items-center gap-2 text-[12.5px]">
                <span className="hidden text-text-tertiary sm:inline">
                  {t(activeNav.group.name)}
                </span>
                <ChevronRight className="hidden h-3 w-3 text-text-tertiary sm:inline" />
                <span className="truncate font-medium text-text-primary">
                  {t(activeNav.item.name)}
                </span>
              </div>
            ) : (
              <span className="text-[12.5px] text-text-tertiary">NetMon</span>
            )}
          </div>

          <button
            type="button"
            title={t('app.systemNormal')}
            aria-label="system status"
            className="hidden items-center gap-1.5 rounded-full border border-border bg-surface-2 px-2.5 py-1 md:flex"
          >
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
            <span className="text-[11px] font-medium text-text-secondary">{t('app.systemNormal')}</span>
          </button>

          <ThemeSwitcher />

          {user && (
            <div className="flex items-center gap-2 rounded-xl border border-border bg-surface-2 py-1 pl-1 pr-1.5">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary-soft text-primary">
                <UserIcon className="h-3.5 w-3.5" />
              </div>
              <div className="hidden min-w-0 leading-tight sm:block">
                <p className="max-w-[120px] truncate text-[12px] font-medium text-text-primary">
                  {user.username}
                </p>
                <p className="text-[10px] text-text-tertiary">{roleText(user.role)}</p>
              </div>
              <button
                type="button"
                onClick={handleLogout}
                className="rounded-lg p-1.5 text-text-tertiary transition-colors hover:bg-rose-500/10 hover:text-rose-400"
                title={t('app.logout')}
                aria-label={t('app.logout')}
              >
                <LogOut className="h-3.5 w-3.5" />
              </button>
            </div>
          )}
        </header>

        <main className="min-h-0 flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>

      <ChatWidget />
    </div>
  );
}
