import { Languages as _Languages, Globe } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function GeneralSettings() {
  const { t, i18n } = useTranslation();

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between p-5 bg-slate-100/70 border border-border rounded-xl shadow-sm">
        <div>
          <h4 className="font-semibold text-text-primary flex items-center gap-2">
            <Globe className="w-4 h-4 text-primary" />
            {t('settings.language')}
          </h4>
          <p className="text-xs text-text-secondary mt-1">
            {t('settings.languageDesc')}
          </p>
        </div>
        <select
          value={i18n.language}
          onChange={(e) => i18n.changeLanguage(e.target.value)}
          className="px-4 py-2 bg-surface-2 border border-border rounded-xl text-text-primary focus:outline-none focus:border-primary text-sm"
        >
          <option value="zh-CN">中文 (简体)</option>
          <option value="en">English</option>
        </select>
      </div>
    </div>
  );
}