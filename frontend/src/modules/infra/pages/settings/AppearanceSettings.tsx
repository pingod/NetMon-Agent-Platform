import { useMemo, useState } from 'react';
import { Check, LayoutPanelLeft, Moon, RotateCcw, Search, Sun } from 'lucide-react';
import clsx from 'clsx';
import { useTranslation } from 'react-i18next';
import { useTheme } from '../../../../contexts/ThemeContext';
import { useUIPreferences } from '../../../../contexts/UIPreferencesContext';
import { getMenuCatalog } from '../../../../lib/navigation';

export default function AppearanceSettings() {
  const { t } = useTranslation();
  const { theme, toggleTheme } = useTheme();
  const {
    toggleGroup,
    toggleItem,
    showAll,
    hiddenCount,
    isGroupHidden,
    isItemHidden,
  } = useUIPreferences();
  const [query, setQuery] = useState('');

  const catalog = useMemo(() => getMenuCatalog(), []);
  const keyword = query.trim().toLowerCase();
  const filtered = useMemo(
    () =>
      catalog
        .map((group) => ({
          ...group,
          items: group.items.filter((item) =>
            keyword ? t(item.name).toLowerCase().includes(keyword) : true
          ),
        }))
        .filter((group) => group.items.length > 0),
    [catalog, keyword, t]
  );

  return (
    <div className="space-y-6">
      <div>
        <h3 className="section-title">
          <LayoutPanelLeft className="h-4 w-4 text-primary" />
          {t('settings.appearance')}
        </h3>
        <p className="mt-1.5 text-[13px] text-text-secondary">{t('settings.appearanceDesc')}</p>
      </div>

      {/* 主题 */}
      <section className="surface-card p-5">
        <div className="mb-4">
          <h4 className="text-[13.5px] font-semibold text-text-primary">{t('settings.theme')}</h4>
          <p className="mt-0.5 text-[12px] text-text-secondary">{t('settings.themeDesc')}</p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <button
            type="button"
            onClick={() => theme !== 'dark' && toggleTheme()}
            className={clsx(
              'group relative flex items-center gap-3 rounded-xl border p-4 text-left transition-all',
              theme === 'dark'
                ? 'border-primary bg-primary-soft shadow-glow'
                : 'border-border bg-surface-2 hover:border-border-strong'
            )}
          >
            <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-[#0b1220] text-[#eef2fa] ring-1 ring-white/10">
              <Moon className="h-4 w-4" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-medium text-text-primary">
                {t('settings.themeDark')}
              </span>
              <span className="mt-0.5 block text-[11.5px] text-text-tertiary">
                {t('settings.themeDarkDesc')}
              </span>
            </span>
            {theme === 'dark' && (
              <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary text-white">
                <Check className="h-3 w-3" />
              </span>
            )}
          </button>

          <button
            type="button"
            onClick={() => theme !== 'light' && toggleTheme()}
            className={clsx(
              'group relative flex items-center gap-3 rounded-xl border p-4 text-left transition-all',
              theme === 'light'
                ? 'border-primary bg-primary-soft shadow-glow'
                : 'border-border bg-surface-2 hover:border-border-strong'
            )}
          >
            <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-white text-[#0f172a] ring-1 ring-black/5">
              <Sun className="h-4 w-4" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-medium text-text-primary">
                {t('settings.themeLight')}
              </span>
              <span className="mt-0.5 block text-[11.5px] text-text-tertiary">
                {t('settings.themeLightDesc')}
              </span>
            </span>
            {theme === 'light' && (
              <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary text-white">
                <Check className="h-3 w-3" />
              </span>
            )}
          </button>
        </div>
      </section>

      {/* 菜单显隐 */}
      <section className="surface-card overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
          <div>
            <h4 className="text-[13.5px] font-semibold text-text-primary">
              {t('settings.menuVisibility')}
            </h4>
            <p className="mt-0.5 text-[12px] text-text-secondary">
              {t('settings.menuVisibilityDesc')}
              {hiddenCount > 0 && (
                <span className="ml-1 text-primary">
                  {t('settings.menuHiddenCount', { count: hiddenCount })}
                </span>
              )}
            </p>
          </div>
          <button
            type="button"
            className="btn btn-ghost h-8 text-[12px]"
            onClick={showAll}
            disabled={hiddenCount === 0}
          >
            <RotateCcw className="h-3.5 w-3.5" />
            {t('settings.showAllMenus')}
          </button>
        </div>

        <div className="border-b border-border px-5 py-3">
          <div className="relative max-w-sm">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-text-tertiary" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('settings.menuSearch')}
              className="input-base h-9 pl-8 text-[12.5px]"
            />
          </div>
        </div>

        <div className="divide-y divide-border">
          {filtered.map((group) => {
            const GroupIcon = group.icon;
            const groupHidden = isGroupHidden(group.name);
            return (
              <div key={group.name} className="px-5 py-4">
                <div className="flex items-center justify-between gap-3">
                  <label className="flex min-w-0 cursor-pointer items-center gap-2.5">
                    <input
                      type="checkbox"
                      className="peer sr-only"
                      checked={!groupHidden}
                      onChange={() => toggleGroup(group.name)}
                    />
                    <span className="flex h-4 w-4 items-center justify-center rounded border border-border-strong bg-surface-2 text-transparent transition-all peer-checked:border-primary peer-checked:bg-primary peer-checked:text-white">
                      <Check className="h-3 w-3" />
                    </span>
                    <GroupIcon className="h-4 w-4 shrink-0 text-text-tertiary" />
                    <span className="truncate text-[13px] font-medium text-text-primary">
                      {t(group.name)}
                    </span>
                  </label>
                  <span className="text-[11px] text-text-tertiary">
                    {group.items.length} {t('settings.menuItems')}
                  </span>
                </div>

                <div
                  className={clsx(
                    'mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3',
                    groupHidden && 'pointer-events-none opacity-40'
                  )}
                >
                  {group.items.map((item) => {
                    const ItemIcon = item.icon;
                    const itemHidden = isItemHidden(item.href);
                    return (
                      <label
                        key={item.href}
                        className={clsx(
                          'flex cursor-pointer items-center gap-2 rounded-lg border px-2.5 py-2 transition-colors',
                          itemHidden
                            ? 'border-border bg-surface-2 text-text-tertiary'
                            : 'border-border bg-surface text-text-primary hover:border-border-strong'
                        )}
                      >
                        <input
                          type="checkbox"
                          className="peer sr-only"
                          checked={!itemHidden}
                          onChange={() => toggleItem(item.href)}
                        />
                        <span className="flex h-3.5 w-3.5 items-center justify-center rounded border border-border-strong bg-surface-2 text-transparent transition-all peer-checked:border-primary peer-checked:bg-primary peer-checked:text-white">
                          <Check className="h-2.5 w-2.5" />
                        </span>
                        <ItemIcon className="h-3.5 w-3.5 shrink-0 opacity-75" />
                        <span className="truncate text-[12px]">{t(item.name)}</span>
                        {itemHidden && (
                          <span className="ml-auto shrink-0 text-[10px] uppercase tracking-wide text-text-tertiary">
                            {t('settings.hidden')}
                          </span>
                        )}
                      </label>
                    );
                  })}
                </div>
              </div>
            );
          })}

          {filtered.length === 0 && (
            <div className="px-5 py-10 text-center text-[12.5px] text-text-tertiary">
              {t('settings.menuSearchEmpty')}
            </div>
          )}
        </div>
      </section>

      <p className="text-[11.5px] leading-relaxed text-text-tertiary">
        {t('settings.menuVisibilityNote')}
      </p>
    </div>
  );
}
