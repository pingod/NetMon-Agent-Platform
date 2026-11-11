import type { ReactNode } from 'react';
import { createContext, useContext, useEffect, useState } from 'react';
import { DEFAULT_THEME, getThemeMeta, isThemeId, type ThemeId } from '../lib/themes';

interface ThemeContextType {
  theme: ThemeId;
  setTheme: (t: ThemeId) => void;
  isDark: boolean;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

const STORAGE_KEY = 'theme';

function readStoredTheme(): ThemeId {
  const stored = localStorage.getItem(STORAGE_KEY);
  return isThemeId(stored) ? stored : DEFAULT_THEME;
}

function applyTheme(id: ThemeId) {
  const root = document.documentElement;
  const meta = getThemeMeta(id);
  root.setAttribute('data-theme', id);
  root.classList.toggle('dark', meta.dark);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeId>(readStoredTheme);

  useEffect(() => {
    applyTheme(theme);
    localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  const setTheme = (t: ThemeId) => {
    if (isThemeId(t)) setThemeState(t);
  };

  return (
    <ThemeContext.Provider value={{ theme, setTheme, isDark: getThemeMeta(theme).dark }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
}