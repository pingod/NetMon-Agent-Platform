import { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Spin } from 'antd';
import { AlertTriangle, RefreshCw, ServerCog } from 'lucide-react';
import api from '@/lib/api';

type EngineState = {
  enabled: boolean;
  state: string;
  error?: string;
  port?: number;
  root?: string;
};

/**
 * IDC 上架规划与布线的统一入口。
 *
 * 规划引擎是独立的 Python 进程（域层经过真实 Excel 反解与 1328 个用例验证，
 * 重写会丢公式口径），由 Go 后端监管并反向代理。这里用同源 iframe 承载它的
 * 十二个页签：认证沿用 NetMon 的 JWT（localStorage 同源可读），
 * 静态资源由 Go 嵌入二进制，不额外暴露端口。
 */
export default function PlanningApp() {
  const [engine, setEngine] = useState<EngineState | null>(null);
  const [checking, setChecking] = useState(true);
  const [iframeKey, setIframeKey] = useState(0);
  const timer = useRef<number | null>(null);

  const probe = useCallback(async () => {
    try {
      const res = await api.get('/planning-status', { timeout: 8000 });
      const data = res.data?.data as EngineState | undefined;
      if (data) setEngine(data);
      setChecking(false);
      return data?.state === 'up';
    } catch (err) {
      const resp = (err as { response?: { data?: { error?: string; message?: string } } })?.response;
      const message =
        resp?.data?.error ||
        resp?.data?.message ||
        (err instanceof Error ? err.message : '无法连接规划引擎状态接口');
      setEngine({ enabled: true, state: 'down', error: message });
      setChecking(false);
      return false;
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      const up = await probe();
      if (cancelled) return;
      if (!up) {
        timer.current = window.setTimeout(tick, 2000);
      }
    };
    void tick();
    return () => {
      cancelled = true;
      if (timer.current) window.clearTimeout(timer.current);
    };
  }, [probe]);

  const reload = useCallback(async () => {
    setChecking(true);
    await probe();
    setIframeKey((k) => k + 1);
  }, [probe]);

  if (checking) {
    return (
      <div className="flex h-[calc(100vh-4rem)] items-center justify-center">
        <div className="flex flex-col items-center gap-3">
          <Spin size="large" />
          <p className="text-sm text-text-tertiary">规划引擎启动中…</p>
        </div>
      </div>
    );
  }

  if (!engine?.enabled || engine.state !== 'up') {
    return (
      <div className="flex h-[calc(100vh-4rem)] items-center justify-center p-6">
        <div className="w-full max-w-xl rounded-2xl border border-border bg-surface p-8 shadow-sm">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-50 text-amber-600">
              <AlertTriangle className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-text-primary">规划引擎未就绪</h2>
              <p className="text-xs text-text-tertiary">
                上架规划与布线依赖本地 Python 引擎，主平台其余功能不受影响。
              </p>
            </div>
          </div>
          <dl className="space-y-2 rounded-xl bg-surface-2 p-4 text-xs">
            <div className="flex gap-2">
              <dt className="w-16 shrink-0 text-text-tertiary">状态</dt>
              <dd className="text-text-primary">{engine?.state ?? 'unknown'}</dd>
            </div>
            {engine?.error && (
              <div className="flex gap-2">
                <dt className="w-16 shrink-0 text-text-tertiary">原因</dt>
                <dd className="break-all text-text-primary">{engine.error}</dd>
              </div>
            )}
            {engine?.root && (
              <div className="flex gap-2">
                <dt className="w-16 shrink-0 text-text-tertiary">引擎目录</dt>
                <dd className="break-all text-text-primary">{engine.root}</dd>
              </div>
            )}
          </dl>
          <div className="mt-5 flex items-center gap-2">
            <Button type="primary" icon={<RefreshCw size={14} />} onClick={reload}>
              重新检测
            </Button>
            <Button icon={<ServerCog size={14} />} onClick={() => window.open('/dc-manage', '_self')}>
              返回数据中心
            </Button>
          </div>
          <p className="mt-4 text-[11px] leading-relaxed text-text-tertiary">
            开发环境：执行 <code>bash planning/scripts/dev_setup.sh</code>；生产 Docker
            镜像已内置 Python 3.12 与引擎依赖。也可用 <code>PLANNING_PYTHON</code>
            指向已有的解释器。
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-[calc(100vh-4rem)] w-full overflow-hidden bg-white">
      <iframe
        key={iframeKey}
        src="/planning-app/"
        title="IDC 上架规划与布线"
        className="h-full w-full border-0"
        allow="fullscreen; clipboard-read; clipboard-write"
      />
    </div>
  );
}
