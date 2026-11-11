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
import { logger } from './logger';

const MAX_RETRIES = 3;
const RETRY_DELAY = 200;
// 自动刷新最小间隔： chunk 永久缺失（如旧版本部署残留）时避免无限刷新循环
const RELOAD_COOLDOWN = 30000;
const RELOAD_KEY = 'lazyRetryReloadAt';

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
      // 重试耗尽后仍失败 → 刷新页面让浏览器重新获取模块图（带冷却，防死循环）
      if (isDynamicImportError(error)) {
        const lastReload = Number(sessionStorage.getItem(RELOAD_KEY) || 0);
        if (Date.now() - lastReload > RELOAD_COOLDOWN) {
          sessionStorage.setItem(RELOAD_KEY, String(Date.now()));
          logger.error('[lazyRetry] Dynamic import failed after retries, reloading page...', error);
          setTimeout(() => window.location.reload(), 0);
        }
      }
      throw error;
    }
  };

  return lazy(retryableImport);
}
