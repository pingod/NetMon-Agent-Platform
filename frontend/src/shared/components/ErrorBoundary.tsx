import type { ErrorInfo, ReactNode } from 'react';
import { Component } from 'react';
import { AlertTriangle, RefreshCw } from 'lucide-react';
import { logger } from '@/lib/logger';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  retried: boolean;
}

/**
 * 判断是否为 Vite 开发模式下动态导入（lazy import）失败。
 * 这类错误通常因 HMR 模块图更新后浏览器仍缓存旧模块 URL 导致，
 * 刷新页面即可恢复，无需展示错误页。
 */
function isDynamicImportError(error: Error): boolean {
  const msg = error.message || '';
  return (
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed') ||
    msg.includes('error loading dynamically imported module')
  );
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null, retried: false };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error, retried: false };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    logger.error('ErrorBoundary caught:', error, errorInfo);

    // 动态导入失败时自动刷新页面（仅一次，避免无限循环）
    if (isDynamicImportError(error) && !this.state.retried) {
      this.setState({ retried: true });
      // 短暂延迟后刷新，避免在渲染周期内直接跳转
      setTimeout(() => {
        window.location.reload();
      }, 100);
    }
  }

  render() {
    if (this.state.hasError) {
      // 动态导入失败且已触发自动刷新 → 显示简短加载态而非错误页
      if (this.state.error && isDynamicImportError(this.state.error) && this.state.retried) {
        return (
          <div className="flex items-center justify-center h-screen bg-background">
            <div className="flex flex-col items-center gap-3">
              <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
              <p className="text-text-secondary text-sm">正在重新加载模块...</p>
            </div>
          </div>
        );
      }

      return (
        <div className="min-h-screen flex items-center justify-center bg-background p-4">
          <div className="max-w-md w-full bg-surface rounded-2xl shadow-2xl p-8 text-center border border-border">
            <div className="flex justify-center mb-4">
              <div className="w-16 h-16 bg-red-500/10 rounded-full flex items-center justify-center">
                <AlertTriangle className="w-8 h-8 text-red-500" />
              </div>
            </div>
            <h2 className="text-xl font-bold text-text-primary mb-2">系统出现异常</h2>
            <p className="text-text-secondary mb-6 text-sm">
              页面渲染时发生错误，请尝试刷新页面。
            </p>
            {this.state.error && (
              <div className="mb-6 p-3 bg-background rounded-xl border border-border text-left">
                <p className="text-xs text-text-secondary font-mono break-all">
                  {this.state.error.message}
                </p>
              </div>
            )}
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center gap-2 px-6 py-2.5 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600 text-white rounded-xl transition-all text-sm font-medium shadow-lg shadow-blue-500/25 hover:shadow-blue-500/40"
            >
              <RefreshCw className="w-4 h-4" />
              刷新页面
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
