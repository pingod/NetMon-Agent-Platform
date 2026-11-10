import type { ReactNode } from 'react';
import { createContext, useCallback, useContext, useMemo, useState } from 'react';

/**
 * 界面偏好：侧边栏菜单的显示/隐藏。
 * 仅保存在当前浏览器（localStorage），不改后端数据。
 */
type UIPreferences = {
  hiddenGroups: string[];
  hiddenItems: string[];
};

type UIPreferencesContextValue = UIPreferences & {
  hiddenCount: number;
  toggleGroup: (groupName: string) => void;
  toggleItem: (href: string) => void;
  setGroupHidden: (groupName: string, hidden: boolean) => void;
  setItemHidden: (href: string, hidden: boolean) => void;
  showAll: () => void;
  isGroupHidden: (groupName: string) => boolean;
  isItemHidden: (href: string) => boolean;
};

const STORAGE_KEY = 'ui:nav-visibility';

function readPrefs(): UIPreferences {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { hiddenGroups: [], hiddenItems: [] };
    const parsed = JSON.parse(raw) as Partial<UIPreferences>;
    return {
      hiddenGroups: Array.isArray(parsed.hiddenGroups) ? parsed.hiddenGroups : [],
      hiddenItems: Array.isArray(parsed.hiddenItems) ? parsed.hiddenItems : [],
    };
  } catch {
    return { hiddenGroups: [], hiddenItems: [] };
  }
}

function persist(next: UIPreferences) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch {
    /* localStorage 不可用时仅本次生效 */
  }
}

const UIPreferencesContext = createContext<UIPreferencesContextValue | undefined>(undefined);

export function UIPreferencesProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<UIPreferences>(() => readPrefs());

  const update = useCallback((mutate: (prev: UIPreferences) => UIPreferences) => {
    setPrefs((prev) => {
      const next = mutate(prev);
      persist(next);
      return next;
    });
  }, []);

  const toggleGroup = useCallback(
    (groupName: string) => {
      update((prev) => ({
        ...prev,
        hiddenGroups: prev.hiddenGroups.includes(groupName)
          ? prev.hiddenGroups.filter((g) => g !== groupName)
          : [...prev.hiddenGroups, groupName],
      }));
    },
    [update]
  );

  const toggleItem = useCallback(
    (href: string) => {
      update((prev) => ({
        ...prev,
        hiddenItems: prev.hiddenItems.includes(href)
          ? prev.hiddenItems.filter((h) => h !== href)
          : [...prev.hiddenItems, href],
      }));
    },
    [update]
  );

  const setGroupHidden = useCallback(
    (groupName: string, hidden: boolean) => {
      update((prev) => {
        const exists = prev.hiddenGroups.includes(groupName);
        if (hidden === exists) return prev;
        return {
          ...prev,
          hiddenGroups: hidden
            ? [...prev.hiddenGroups, groupName]
            : prev.hiddenGroups.filter((g) => g !== groupName),
        };
      });
    },
    [update]
  );

  const setItemHidden = useCallback(
    (href: string, hidden: boolean) => {
      update((prev) => {
        const exists = prev.hiddenItems.includes(href);
        if (hidden === exists) return prev;
        return {
          ...prev,
          hiddenItems: hidden
            ? [...prev.hiddenItems, href]
            : prev.hiddenItems.filter((h) => h !== href),
        };
      });
    },
    [update]
  );

  const showAll = useCallback(() => update(() => ({ hiddenGroups: [], hiddenItems: [] })), [update]);


  const value = useMemo<UIPreferencesContextValue>(
    () => ({
      ...prefs,
      hiddenCount: prefs.hiddenGroups.length + prefs.hiddenItems.length,
      toggleGroup,
      toggleItem,
      setGroupHidden,
      setItemHidden,
      showAll,
      isGroupHidden: (groupName) => prefs.hiddenGroups.includes(groupName),
      isItemHidden: (href) => prefs.hiddenItems.includes(href),
    }),
    [prefs, toggleGroup, toggleItem, setGroupHidden, setItemHidden, showAll]
  );

  return <UIPreferencesContext.Provider value={value}>{children}</UIPreferencesContext.Provider>;
}

export function useUIPreferences() {
  const ctx = useContext(UIPreferencesContext);
  if (!ctx) {
    throw new Error('useUIPreferences must be used within a UIPreferencesProvider');
  }
  return ctx;
}
