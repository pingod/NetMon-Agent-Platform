import { useEffect, useRef, useState } from 'react';
import type { IDisposable } from 'xterm';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebLinksAddon } from 'xterm-addon-web-links';
import 'xterm/css/xterm.css';

interface TerminalProps {
  serverId: string;
  serverName: string;
  token: string;
  onClose: () => void;
  deviceType?: 'server' | 'network-device';
}

export default function WebTerminal({ serverId, serverName, token, onClose, deviceType = 'server' }: TerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const onDataDisposeRef = useRef<IDisposable | null>(null);
  const onResizeDisposeRef = useRef<IDisposable | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectCountRef = useRef(0);
  const maxReconnectAttempts = 3;
  const mountedRef = useRef(true);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'error' | 'disconnected'>('connecting');
  const [error, setError] = useState<string>('');

  useEffect(() => {
    if (!terminalRef.current) return;

    mountedRef.current = true;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78',
        black: '#000000',
        red: '#cd3131',
        green: '#0dbc79',
        yellow: '#e5e510',
        blue: '#2472c8',
        magenta: '#bc3fbc',
        cyan: '#11a8cd',
        white: '#e5e5e5',
        brightBlack: '#666666',
        brightRed: '#f14c4c',
        brightGreen: '#23d18b',
        brightYellow: '#f5f543',
        brightBlue: '#3b8eea',
        brightMagenta: '#d670d6',
        brightCyan: '#29b8db',
        brightWhite: '#e5e5e5'
      },
      allowProposedApi: true,
      scrollback: 5000
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(webLinksAddon);

    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    term.open(terminalRef.current);
    fitAddon.fit();

    // 构建 WebSocket URL
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/ws/terminal?token=${encodeURIComponent(token)}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      if (!mountedRef.current) return;
      reconnectCountRef.current = 0;

      // 发送 terminal:open 消息
      const t = xtermRef.current;
      if (!t) return;
      ws.send(JSON.stringify({
        type: 'open',
        deviceId: serverId,
        cols: t.cols,
        rows: t.rows,
        deviceType,
      }));
    };

    ws.onmessage = (event) => {
      if (!mountedRef.current) return;
      try {
        const msg = JSON.parse(event.data);
        switch (msg.type) {
          case 'session':
            sessionIdRef.current = msg.sessionId || null;
            setStatus('connected');
            break;
          case 'data':
            if (xtermRef.current) {
              xtermRef.current.write(msg.data || '');
            }
            break;
          case 'error':
            setStatus('error');
            setError(msg.error || 'Unknown error');
            break;
          case 'closed':
            setStatus('disconnected');
            break;
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onerror = () => {
      if (!mountedRef.current) return;
      setStatus('error');
      setError('websocket error');
    };

    ws.onclose = (event) => {
      if (!mountedRef.current) return;
      // 正常关闭
      if (event.code === 1000) {
        setStatus('disconnected');
        return;
      }
      // 异常关闭，尝试重连
      if (reconnectCountRef.current < maxReconnectAttempts) {
        setStatus('connecting');
        reconnectCountRef.current++;
        reconnectTimerRef.current = setTimeout(() => {
          if (!mountedRef.current) return;
          // 重新创建 WebSocket 连接
          const newWs = new WebSocket(wsUrl);
          wsRef.current = newWs;
          // 复用事件处理
          newWs.onopen = ws.onopen;
          newWs.onmessage = ws.onmessage;
          newWs.onerror = ws.onerror;
          newWs.onclose = ws.onclose;
        }, Math.min(1000 * Math.pow(2, reconnectCountRef.current), 5000));
      } else {
        setStatus('disconnected');
        setError('Terminal connection lost');
      }
    };

    const onDataDispose = term.onData((data) => {
      if (!mountedRef.current || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN || !sessionIdRef.current) return;
      wsRef.current.send(JSON.stringify({
        type: 'data',
        sessionId: sessionIdRef.current,
        data,
      }));
    });
    onDataDisposeRef.current = onDataDispose;

    const onResizeDispose = term.onResize(({ cols, rows }) => {
      if (!mountedRef.current || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN || !sessionIdRef.current) return;
      wsRef.current.send(JSON.stringify({
        type: 'resize',
        sessionId: sessionIdRef.current,
        cols,
        rows,
      }));
    });
    onResizeDisposeRef.current = onResizeDispose;

    const handleResize = () => {
      fitAddon.fit();
    };
    window.addEventListener('resize', handleResize);

    return () => {
      mountedRef.current = false;

      window.removeEventListener('resize', handleResize);
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      reconnectCountRef.current = 0;

      onDataDisposeRef.current?.dispose();
      onDataDisposeRef.current = null;
      onResizeDisposeRef.current?.dispose();
      onResizeDisposeRef.current = null;

      // 发送关闭消息
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN && sessionIdRef.current) {
        wsRef.current.send(JSON.stringify({
          type: 'close',
          sessionId: sessionIdRef.current,
        }));
        wsRef.current.close(1000, 'terminal closed');
      }

      sessionIdRef.current = null;
      wsRef.current = null;

      if (xtermRef.current) {
        xtermRef.current.dispose();
        xtermRef.current = null;
      }
    };
  }, [serverId, token]);

  return (
    <div className="flex flex-col h-full bg-gray-900">
      <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700">
        <div className="flex items-center gap-2">
          <div className={`w-2 h-2 rounded-full ${
            status === 'connected' ? 'bg-green-500' :
            status === 'connecting' ? 'bg-yellow-500 animate-pulse' :
            status === 'error' ? 'bg-red-500' :
            'bg-gray-500'
          }`} />
          <span className="text-sm text-gray-300 font-medium">{serverName}</span>
          <span className="text-xs text-gray-500">({status})</span>
        </div>
        <button
          onClick={() => {
            if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN && sessionIdRef.current) {
              wsRef.current.send(JSON.stringify({
                type: 'close',
                sessionId: sessionIdRef.current,
              }));
              wsRef.current.close(1000, 'terminal closed');
            }
            wsRef.current = null;
            onDataDisposeRef.current?.dispose();
            onResizeDisposeRef.current?.dispose();
            if (xtermRef.current) {
              xtermRef.current.dispose();
              xtermRef.current = null;
            }
            onClose();
          }}
          className="px-3 py-1 text-xs text-gray-400 hover:text-white bg-gray-700 hover:bg-gray-600 rounded transition-colors"
        >
          关闭终端
        </button>
      </div>

      <div className="flex-1 overflow-hidden p-2">
        {status === 'error' && (
          <div className="flex flex-col items-center justify-center h-full text-red-400">
            <svg className="w-12 h-12 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            <p className="text-lg font-medium mb-2">连接失败</p>
            <p className="text-sm text-gray-500 mb-4">{error}</p>
            <button
              onClick={onClose}
              className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded text-sm transition-colors"
            >
              返回
            </button>
          </div>
        )}
        {status !== 'error' && (
          <div ref={terminalRef} className="w-full h-full" />
        )}
      </div>
    </div>
  );
}
