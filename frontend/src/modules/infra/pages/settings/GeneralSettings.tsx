import { Sun, Moon, Languages as _Languages, Globe } from 'lucide-react';
import { useTheme } from '../../../../hooks/useTheme';
import { useTranslation } from 'react-i18next';

export default function GeneralSettings() {
  const { t, i18n } = useTranslation();
  const { theme, toggleTheme } = useTheme();

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between p-5 bg-slate-800/40 border border-slate-700/50 rounded-xl shadow-sm">
        <div>
          <h4 className="font-semibold text-white flex items-center gap-2">
            <Globe className="w-4 h-4 text-blue-400" />
            {t('settings.language')}
          </h4>
          <p className="text-xs text-slate-400 mt-1">
            {t('settings.languageDesc')}
          </p>
        </div>
        <select
          value={i18n.language}
          onChange={(e) => i18n.changeLanguage(e.target.value)}
          className="px-4 py-2 bg-slate-900/80 border border-slate-700 rounded-xl text-white focus:outline-none focus:border-blue-500/50 text-sm"
        >
          <option value="zh-CN">中文 (简体)</option>
          <option value="en">English</option>
        </select>
      </div>

      <div className="flex items-center justify-between p-5 bg-slate-800/40 border border-slate-700/50 rounded-xl shadow-sm">
        <div>
          <h4 className="font-semibold text-white flex items-center gap-2">
            {theme === 'dark' ? <Moon className="w-4 h-4 text-purple-400" /> : <Sun className="w-4 h-4 text-amber-400" />}
            {t('settings.theme')}
          </h4>
          <p className="text-xs text-slate-400 mt-1">
            {t('settings.themeDesc')}
          </p>
        </div>
        <button
          onClick={toggleTheme}
          className="flex items-center gap-2 px-4 py-2 bg-slate-900/80 border border-slate-700 rounded-xl hover:bg-slate-700/50 transition-colors"
        >
          {theme === 'dark' ? (
            <Moon className="w-4 h-4 text-purple-400" />
          ) : (
            <Sun className="w-4 h-4 text-amber-400" />
          )}
          <span className="text-sm text-slate-200">
            {theme === 'dark' ? '深色主题' : '浅色主题'}
          </span>
        </button>
      </div>
    </div>
  );
}
