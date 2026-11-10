import { useEffect, useState, useRef, useCallback } from 'react';
import type { Socket } from 'socket.io-client';
import { io } from 'socket.io-client';
import { WS_URL, WS_RECONNECT_INTERVALS } from './types';

export function useAlertSocket(token: string | null, refetch: () => void) {
  const [wsConnected, setWsConnected] = useState(false);
  const socketRef = useRef<Socket | null>(null);
  const reconnectAttemptRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connectWebSocketRef = useRef<ReturnType<typeof connectWebSocketInner> | null>(null);
  const scheduleReconnectRef = useRef<(() => void) | null>(null);

  const connectWebSocketInner = useCallback(() => {
    // 连接时读取最新 token（access token 可能已被刷新轮换）
    const activeToken = localStorage.getItem('token') || token;
    if (!activeToken) return;

    // 重连时先回收旧连接，避免遗留 socket 与重复监听
    if (socketRef.current) {
      socketRef.current.removeAllListeners();
      socketRef.current.disconnect();
      socketRef.current = null;
    }

    let disposed = false;
    const socket: Socket = io(WS_URL, {
      transports: ['websocket', 'polling'],
      auth: { token: activeToken },
      reconnection: false,
    });

    socket.on('connect', () => {
      if (disposed) return;
      setWsConnected(true);
      reconnectAttemptRef.current = 0;
      socket.emit('alert:subscribe');
    });

    socket.on('disconnect', () => {
      if (disposed) return;
      setWsConnected(false);
      scheduleReconnectRef.current?.();
    });

    socket.on('connect_error', () => {
      if (disposed) return;
      setWsConnected(false);
      scheduleReconnectRef.current?.();
    });

    socket.on('alert:new', () => {
      if (disposed) return;
      refetch();
    });

    socket.on('alert:updated', () => {
      if (disposed) return;
      refetch();
    });

    socketRef.current = socket;

    return () => {
      disposed = true;
      socket.removeAllListeners();
      socket.emit('alert:unsubscribe');
      socket.disconnect();
      if (socketRef.current === socket) {
        socketRef.current = null;
      }
    };
  }, [token, refetch]);

  const scheduleReconnect = useCallback(() => {
    if (reconnectAttemptRef.current >= WS_RECONNECT_INTERVALS.length) return;
    // 同一时刻只保留一个重连定时器
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
    }
    const delay = WS_RECONNECT_INTERVALS[reconnectAttemptRef.current];
    reconnectTimerRef.current = setTimeout(() => {
      reconnectTimerRef.current = null;
      reconnectAttemptRef.current++;
      connectWebSocketRef.current?.();
    }, delay);
  }, []);

  useEffect(() => {
    connectWebSocketRef.current = connectWebSocketInner;
    scheduleReconnectRef.current = scheduleReconnect;
    const cleanup = connectWebSocketInner();
    return () => {
      cleanup?.();
      setWsConnected(false);
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      reconnectAttemptRef.current = 0;
    };
  }, [connectWebSocketInner, scheduleReconnect]);

  return { wsConnected };
}