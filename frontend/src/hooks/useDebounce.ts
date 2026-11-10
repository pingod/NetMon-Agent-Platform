import { useState, useEffect } from 'react';

/**
 * useDebounce 钩子 — 延迟更新输入值，避免高频输入导致 UI 卡顿或频繁发送 API 请求
 * @param value 输入值
 * @param delay 延迟毫秒数 (默认 300ms)
 */
export function useDebounce<T>(value: T, delay: number = 300): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedValue(value);
    }, delay);

    return () => {
      clearTimeout(handler);
    };
  }, [value, delay]);

  return debouncedValue;
}
