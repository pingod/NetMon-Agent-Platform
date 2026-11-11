import { Suspense, isValidElement } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider, theme as antdTheme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import enUS from 'antd/locale/en_US';
import { useTranslation } from 'react-i18next';
import { AuthProvider } from './contexts/AuthContext';
import { ToastProvider } from './contexts/ToastContext';
import { ThemeProvider } from './contexts/ThemeContext';
import { UIPreferencesProvider } from './contexts/UIPreferencesContext';
import ErrorBoundary from './shared/components/ErrorBoundary';
import ProtectedRoute from './shared/components/ProtectedRoute';
import Layout from './shared/layouts/Layout';

// 模块路由聚合
import { protectedRoutes, publicRoutes } from './modules/_routes.tsx';
import { lazyRetry } from './lib/lazyRetry';

const NotFound = lazyRetry(() => import('./shared/pages/NotFound'));


// ==================== 加载占位 ====================
function PageLoader() {
  return (
    <div className="flex h-screen items-center justify-center" style={{ background: 'var(--bg-app)' }}>
      <div className="flex flex-col items-center gap-4">
        <div className="h-9 w-9 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        <p className="text-[12px] tracking-[0.12em] text-text-tertiary">NETMON LOADING</p>
      </div>
    </div>
  );
}

function ContentLoader() {
  return (
    <div className="page-shell">
      <div className="flex flex-1 animate-pulse flex-col gap-5">
        <div className="h-8 w-1/4 rounded-lg bg-surface-3" />
        <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
          <div className="h-24 rounded-xl bg-surface-2" />
          <div className="h-24 rounded-xl bg-surface-2" />
          <div className="h-24 rounded-xl bg-surface-2" />
          <div className="h-24 rounded-xl bg-surface-2" />
        </div>
        <div className="h-64 w-full flex-1 rounded-xl bg-surface-2" />
      </div>
    </div>
  );
}

function SuspenseRoute({ children, isInner = false }: { children: React.ReactNode; isInner?: boolean }) {
  return <Suspense fallback={isInner ? <ContentLoader /> : <PageLoader />}>{children}</Suspense>;
}

function ThemedConfigProvider({ children }: { children: React.ReactNode }) {
  const { i18n } = useTranslation();
  return (
    <ConfigProvider
      locale={i18n.language?.startsWith('zh') ? zhCN : enUS}
      theme={{
        algorithm: antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#6d5efc',
          colorInfo: '#6d5efc',
          colorBgBase: '#f4f6fb',
          colorBgContainer: '#ffffff',
          colorBgElevated: '#ffffff',
          colorBorder: '#e3e8f2',
          colorBorderSecondary: '#e3e8f2',
          colorSplit: '#e3e8f2',
          colorText: '#0f172a',
          colorTextSecondary: '#51607a',
          borderRadius: 10,
          borderRadiusLG: 14,
          controlHeight: 36,
          fontSize: 13,
          fontFamily:
            "'Inter', -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif",
          boxShadowSecondary: '0 24px 60px -28px rgba(0,0,0,0.35)',
        },
        components: {
          Button: { primaryShadow: 'none', fontWeight: 500 },
          Card: { colorBgContainer: '#ffffff', colorBorderSecondary: '#e3e8f2' },
          Modal: { borderRadiusLG: 16 },
          Table: {
            headerBg: '#f7f9fd',
            headerColor: '#51607a',
            borderColor: '#e3e8f2',
            rowHoverBg: 'rgba(15,23,42,0.035)',
          },
          Tabs: { itemSelectedColor: '#6d5efc', inkBarColor: '#6d5efc' },
          Tooltip: { colorBgSpotlight: '#0f172a' },
          Select: { optionSelectedBg: 'rgba(109,94,252,0.14)' },
        },
      }}
    >
      {children}
    </ConfigProvider>
  );
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 30 * 1000,     // 30秒内复用缓存数据，菜单切换秒开
      gcTime: 10 * 60 * 1000,   // 10分钟垃圾回收保留期
    },
  },
});


function App() {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const resolveElement = (el: any, isInner = false) => {
    if (!el) return null;
    const content = isValidElement(el) ? el : (() => {
      const Component = el as React.ComponentType;
      return <Component />;
    })();
    if (isInner) {
      return <div className="page-enter w-full h-full min-h-0 flex flex-col flex-1">{content}</div>;
    }
    return <div className="page-enter w-full min-h-screen">{content}</div>;
  };

  return (
    <ErrorBoundary>
      <ThemeProvider>
        <UIPreferencesProvider>
        <ThemedConfigProvider>
          <AuthProvider>
            <ToastProvider>
              <QueryClientProvider client={queryClient}>
                <BrowserRouter>
                <Routes>
                  {/* 公开路由 */}
                  {publicRoutes.map((route) => (
                    <Route
                      key={route.path}
                      path={route.path}
                      element={<SuspenseRoute isInner={false}>{resolveElement(route.element, false)}</SuspenseRoute>}
                    />
                  ))}

                  {/* 受保护路由 */}
                  <Route path="/" element={<ProtectedRoute><Layout /></ProtectedRoute>}>
                    <Route index element={<Navigate to="/dashboard" replace />} />
                    {protectedRoutes.map((route) => (
                      <Route
                        key={route.path}
                        path={route.path}
                        element={
                          <SuspenseRoute isInner={true}>
                            <ProtectedRoute>
                              {resolveElement(route.element, true)}
                            </ProtectedRoute>
                          </SuspenseRoute>
                        }
                      />
                    ))}
                  </Route>

                  {/* 404 */}
                  <Route path="*" element={<SuspenseRoute isInner={false}><NotFound /></SuspenseRoute>} />
                </Routes>
              </BrowserRouter>
            </QueryClientProvider>
          </ToastProvider>
        </AuthProvider>

        </ThemedConfigProvider>
        </UIPreferencesProvider>
      </ThemeProvider>
  </ErrorBoundary>
  );
}

export default App;
