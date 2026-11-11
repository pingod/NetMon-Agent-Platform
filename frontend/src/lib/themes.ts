import type { ThemeConfig } from 'antd';

export type ThemeId = 'light' | 'dark' | 'ocean' | 'sepia' | 'contrast';

export interface ThemeMeta {
  id: ThemeId;
  /** i18n key: theme.<id> */
  nameKey: string;
  descKey: string;
  /** 切换器色板:背景 / 主色 / 点缀 */
  swatch: [string, string, string];
  dark: boolean;
  antd: ThemeConfig;
}

const fontFamily =
  "'Inter', -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif";

const baseToken = {
  borderRadius: 10,
  borderRadiusLG: 14,
  controlHeight: 36,
  fontSize: 13,
  fontFamily,
};

function lightAntd(primary: string, bg: string, surface: string, surface2: string, border: string, text: string, text2: string): ThemeConfig {
  return {
    token: {
      ...baseToken,
      colorPrimary: primary,
      colorInfo: primary,
      colorBgBase: bg,
      colorBgContainer: surface,
      colorBgElevated: surface,
      colorBorder: border,
      colorBorderSecondary: border,
      colorSplit: border,
      colorText: text,
      colorTextSecondary: text2,
      boxShadowSecondary: '0 24px 60px -28px rgba(0,0,0,0.35)',
    },
    components: {
      Button: { primaryShadow: 'none', fontWeight: 500 },
      Card: { colorBgContainer: surface, colorBorderSecondary: border },
      Modal: { borderRadiusLG: 16 },
      Table: {
        headerBg: surface2,
        headerColor: text2,
        borderColor: border,
        rowHoverBg: 'rgba(15,23,42,0.035)',
      },
      Tabs: { itemSelectedColor: primary, inkBarColor: primary },
      Tooltip: { colorBgSpotlight: text },
      Select: { optionSelectedBg: `${primary}24` },
    },
  };
}

function darkAntd(primary: string, bg: string, surface: string, surface2: string, border: string, text: string, text2: string): ThemeConfig {
  return {
    token: {
      ...baseToken,
      colorPrimary: primary,
      colorInfo: primary,
      colorBgBase: bg,
      colorBgContainer: surface,
      colorBgElevated: surface2,
      colorBorder: border,
      colorBorderSecondary: border,
      colorSplit: border,
      colorText: text,
      colorTextSecondary: text2,
      colorBgSpotlight: surface2,
      boxShadowSecondary: '0 24px 60px -28px rgba(0,0,0,0.7)',
    },
    components: {
      Button: { primaryShadow: 'none', fontWeight: 500 },
      Card: { colorBgContainer: surface, colorBorderSecondary: border },
      Modal: { borderRadiusLG: 16, contentBg: surface, headerBg: surface },
      Table: {
        headerBg: surface2,
        headerColor: text2,
        borderColor: border,
        rowHoverBg: 'rgba(255,255,255,0.04)',
        colorBgContainer: surface,
      },
      Tabs: { itemSelectedColor: primary, inkBarColor: primary },
      Tooltip: { colorBgSpotlight: surface2, colorTextLightSolid: text },
      Select: { optionSelectedBg: `${primary}2e`, colorBgElevated: surface2 },
      Dropdown: { colorBgElevated: surface2 },
      Popover: { colorBgElevated: surface2 },
      Message: { contentBg: surface2, colorText: text },
      Notification: { colorBgElevated: surface2 },
    },
  };
}

export const THEMES: ThemeMeta[] = [
  {
    id: 'light',
    nameKey: 'theme.light',
    descKey: 'theme.lightDesc',
    swatch: ['#f4f6fb', '#5a4ae8', '#0891b2'],
    dark: false,
    antd: lightAntd('#6d5efc', '#f4f6fb', '#ffffff', '#f7f9fd', '#e3e8f2', '#0f172a', '#51607a'),
  },
  {
    id: 'dark',
    nameKey: 'theme.dark',
    descKey: 'theme.darkDesc',
    swatch: ['#10141d', '#8f83ff', '#22d3ee'],
    dark: true,
    antd: darkAntd('#8f83ff', '#10141d', '#171c28', '#1d2331', '#2a3245', '#e8ecf4', '#9aa5b8'),
  },
  {
    id: 'ocean',
    nameKey: 'theme.ocean',
    descKey: 'theme.oceanDesc',
    swatch: ['#081222', '#38bdf8', '#2dd4bf'],
    dark: true,
    antd: darkAntd('#38bdf8', '#081222', '#0e1c33', '#132441', '#1e3a5f', '#e6f0fa', '#93a9c4'),
  },
  {
    id: 'sepia',
    nameKey: 'theme.sepia',
    descKey: 'theme.sepiaDesc',
    swatch: ['#f5f1e8', '#b45309', '#0f766e'],
    dark: false,
    antd: lightAntd('#b45309', '#f5f1e8', '#fffdf7', '#f8f4ea', '#e4dcc8', '#3d362a', '#6f6450'),
  },
  {
    id: 'contrast',
    nameKey: 'theme.contrast',
    descKey: 'theme.contrastDesc',
    swatch: ['#ffffff', '#1d4ed8', '#000000'],
    dark: false,
    antd: lightAntd('#1d4ed8', '#ffffff', '#ffffff', '#f5f5f5', '#555555', '#000000', '#1f1f1f'),
  },
];

export const DEFAULT_THEME: ThemeId = 'light';

export function isThemeId(v: unknown): v is ThemeId {
  return typeof v === 'string' && THEMES.some((t) => t.id === v);
}

export function getThemeMeta(id: ThemeId): ThemeMeta {
  return THEMES.find((t) => t.id === id) ?? THEMES[0];
}