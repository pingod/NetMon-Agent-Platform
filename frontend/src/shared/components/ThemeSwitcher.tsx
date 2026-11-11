import { Dropdown } from 'antd';
import { Check, Palette } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '../../contexts/ThemeContext';
import { THEMES } from '../../lib/themes';

export default function ThemeSwitcher() {
  const { theme, setTheme } = useTheme();
  const { t } = useTranslation();

  const items = THEMES.map((meta) => ({
    key: meta.id,
    label: (
      <div className="flex items-center gap-2.5 py-0.5">
        <span className="flex shrink-0 overflow-hidden rounded-full border border-border" style={{ width: 26, height: 14 }}>
          {meta.swatch.map((c) => (
            <span key={c} style={{ background: c, width: '33.4%', height: '100%' }} />
          ))}
        </span>
        <span className="flex min-w-0 flex-col leading-tight">
          <span className="text-[12.5px] font-medium">{t(meta.nameKey)}</span>
          <span className="text-[10.5px] opacity-60">{t(meta.descKey)}</span>
        </span>
        {theme === meta.id && <Check className="ml-auto h-3.5 w-3.5 shrink-0 text-primary" />}
      </div>
    ),
    onClick: () => setTheme(meta.id),
  }));

  return (
    <Dropdown menu={{ items, selectedKeys: [theme] }} trigger={['click']} placement="bottomRight">
      <button
        type="button"
        className="flex h-8 w-8 items-center justify-center rounded-lg border border-border bg-surface-2 text-text-secondary transition-colors hover:bg-surface-hover hover:text-text-primary"
        title={t('theme.title')}
        aria-label={t('theme.title')}
      >
        <Palette className="h-3.5 w-3.5" />
      </button>
    </Dropdown>
  );
}