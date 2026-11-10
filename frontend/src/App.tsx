import { Suspense, isValidElement } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider, theme as antdTheme } from 'antd';
import { AuthProvider } from './contexts/AuthContext';
import { ToastProvider } from './contexts/ToastContext';
import { ThemeProvider, useTheme } from './contexts/ThemeContext';
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
    <div className="flex items-center justify-center h-screen bg-slate-950">
      <div className="flex flex-col items-center gap-3">
        <div className="w-8 h-8 border-2 border-purple-500 border-t-transparent rounded-full animate-spin" />
        <p className="text-slate-400 text-xs tracking-wider">NetMon 系统加载中...</p>
      </div>
    </div>
  );
}

function ContentLoader() {
  return (
    <div className="flex-1 p-6 h-full flex flex-col gap-4 animate-pulse">
      <div className="h-8 bg-slate-800/40 rounded-lg w-1/4" />
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="h-24 bg-slate-800/30 rounded-xl" />
        <div className="h-24 bg-slate-800/30 rounded-xl" />
        <div className="h-24 bg-slate-800/30 rounded-xl" />
        <div className="h-24 bg-slate-800/30 rounded-xl" />
      </div>
      <div className="h-64 bg-slate-800/20 rounded-xl w-full flex-1" />
    </div>
  );
}

function SuspenseRoute({ children, isInner = false }: { children: React.ReactNode; isInner?: boolean }) {
  return <Suspense fallback={isInner ? <ContentLoader /> : <PageLoader />}>{children}</Suspense>;
}

function ThemedConfigProvider({ children }: { children: React.ReactNode }) {
  const { theme } = useTheme();
  return (
    <ConfigProvider
      theme={{
        algorithm: theme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#8b5cf6',
          borderRadius: 6,
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
    </ThemeProvider>
  </ErrorBoundary>
  );
}

export default App;
