/**
 * 带自动重试的 lazy 动态导入包装器
 *
 * 解决 Vite 开发模式下 `Failed to fetch dynamically imported module` 问题。
 * 该错误通常因 HMR 模块图更新后浏览器仍缓存旧模块 URL 导致，
 * 重新请求一次即可恢复，不应因此中断用户体验。
 *
 * 策略：
 * 1. 首次导入失败 → 间隔 200ms 重试，最多 3 次
 * 2. 仍失败 → 刷新整个页面（恢复模块图）
 */
import { lazy, type ComponentType } from 'react';

const MAX_RETRIES = 3;
const RETRY_DELAY = 200;

function isDynamicImportError(error: unknown): boolean {
  const msg = error instanceof Error ? error.message : String(error);
  return (
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed') ||
    msg.includes('error loading dynamically imported module')
  );
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function lazyRetry<T extends ComponentType<any>>(
  dynamicImport: () => Promise<{ default: T }>
) {
  let retryCount = 0;

  const retryableImport = async (): Promise<{ default: T }> => {
    try {
      return await dynamicImport();
    } catch (error) {
      if (isDynamicImportError(error) && retryCount < MAX_RETRIES) {
        retryCount++;
        // 等待短暂时间后重试（Vite 可能正在重新编译）
        await sleep(RETRY_DELAY * retryCount);
        return retryableImport();
      }
      // 重试耗尽后仍失败 → 刷新页面让浏览器重新获取模块图
      if (isDynamicImportError(error)) {
        console.error('[lazyRetry] Dynamic import failed after retries, reloading page...', error);
        setTimeout(() => window.location.reload(), 0);
      }
      throw error;
    }
  };

  return lazy(retryableImport);
}
