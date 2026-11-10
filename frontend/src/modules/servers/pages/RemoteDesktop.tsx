import { useState, useEffect, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, MonitorPlay, PowerOff, AlertCircle, Cable } from 'lucide-react';
import { logger } from '@/lib/logger';
import { useAuth } from '../../../contexts/AuthContext';

interface Server {
  id: string;
  name: string;
  hostname: string;
  vnc_port?: number;
  os_type?: string;
}

interface VNCConfig {
  hostname: string;
  vnc_port: number;
  vnc_password?: string;
}

type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'error';

export default function RemoteDesktop() {
  const { serverId } = useParams<{ serverId: string }>();
  const navigate = useNavigate();
  const { token } = useAuth();
  const [servers, setServers] = useState<Server[]>([]);
  const [selectedServer, setSelectedServer] = useState<string | undefined>(serverId);
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [error, setError] = useState<string | null>(null);
  const [vncConfig, setVncConfig] = useState<VNCConfig | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);

  // 加载服务器列表
  useEffect(() => {
    const loadServers = async () => {
      try {
        const res = await fetch('/api/v1/servers', {
          headers: { 'Authorization': `Bearer ${token}` }
        });
        const result = await res.json();
        if (result.success) {
          const vncServers = result.data.filter((s: Server) =>
            s.os_type === 'windows' || s.vnc_port
          );
          setServers(vncServers);

          if (serverId && vncServers.some((s: Server) => s.id === serverId)) {
            setSelectedServer(serverId);
          }
        }
      } catch (err) {
        logger.error('Failed to load servers:', err);
      }
    };
    loadServers();
  }, [token, serverId]);

  // 加载服务器 VNC 配置
  const loadVncConfig = async (id: string): Promise<VNCConfig | null> => {
    try {
      const res = await fetch(`/api/v1/vnc/config/${id}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      const result = await res.json();
      if (result.success) {
        setVncConfig(result.data);
        return result.data;
      }
      return null;
    } catch (err) {
      console.error('Failed to load VNC config:', err);
      return null;
    }
  };

  // 断开 VNC 连接
  const disconnectVNC = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setConnectionState('disconnected');
    setVncConfig(null);
  }, []);

  // 连接 VNC（通过后端 WebSocket 代理）
  const connectToVNC = useCallback(async (id: string) => {
    // 先断开旧连接
    disconnectVNC();
    setError(null);
    setConnectionState('connecting');

    const config = await loadVncConfig(id);
    if (!config) {
      setError('无法加载 VNC 配置');
      setConnectionState('error');
      return;
    }

    try {
      // 连接后端 WebSocket VNC 代理（标准 WebSocket，不再用 socket.io）
      const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${wsProtocol}//${window.location.host}/ws/vnc?token=${encodeURIComponent(token!)}`;
      const ws = new WebSocket(wsUrl);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.onopen = () => {
        // 发送 VNC 连接请求
        ws.send(JSON.stringify({
          serverId: id,
          vncHost: config.hostname,
          vncPort: config.vnc_port
        }));
      };

      ws.onmessage = (event) => {
        try {
          if (typeof event.data === 'string') {
            const msg = JSON.parse(event.data);
            if (msg.type === 'connected') {
              setConnectionState('connected');
            } else if (msg.type === 'error') {
              setError(msg.message || 'VNC 连接错误');
              setConnectionState('error');
              ws.close();
            }
          }
          // 二进制数据（RFB 协议帧）由 noVNC 处理
        } catch {
          // 忽略非 JSON 消息
        }
      };

      ws.onerror = () => {
        setError('WebSocket 连接失败');
        setConnectionState('error');
      };

      ws.onclose = () => {
        if (connectionState !== 'error') {
          setConnectionState('disconnected');
        }
      };
    } catch (err) {
      logger.error('Failed to connect:', err);
      setError('连接失败: ' + (err instanceof Error ? err.message : 'Unknown error'));
      setConnectionState('error');
    }
  }, [token, disconnectVNC, connectionState]);

  // 当选择服务器变化时自动连接
  useEffect(() => {
    if (selectedServer && servers.length > 0) {
      connectToVNC(selectedServer);
    }
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedServer]);

  // 组件卸载清理
  useEffect(() => {
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, []);

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <button
            onClick={() => navigate('/servers')}
            className="flex items-center gap-2 px-4 py-2 bg-background border border-border rounded-lg hover:bg-background-hover transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
            <span>返回</span>
          </button>
          <div>
            <h1 className="text-2xl font-bold flex items-center gap-2 text-text-primary">
              <MonitorPlay className="w-6 h-6" />
              远程桌面
            </h1>
            <p className="text-text-secondary mt-1">
              通过 VNC 连接远程服务器桌面
            </p>
          </div>
        </div>
      </div>

      <div className="grid gap-4">
        {/* 控制面板 */}
        <div className="bg-card border border-border rounded-lg p-4">
          <div className="flex flex-wrap gap-4 items-center">
            <select
              value={selectedServer || ''}
              onChange={(e) => setSelectedServer(e.target.value || undefined)}
              className="px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:border-primary text-text-primary"
              style={{ minWidth: '300px' }}
            >
              <option value="">选择服务器</option>
              {servers.map(server => (
                <option key={server.id} value={server.id}>
                  {server.name} ({server.hostname})
                </option>
              ))}
            </select>

            {connectionState === 'connecting' && (
              <div className="flex items-center gap-2 text-yellow-400">
                <Cable className="w-4 h-4 animate-pulse" />
                <span>连接中...</span>
              </div>
            )}

            {connectionState === 'connected' && (
              <div className="flex items-center gap-3">
                <div className="flex items-center gap-2 text-green-400">
                  <Cable className="w-4 h-4" />
                  <span>已连接</span>
                </div>
                <button
                  onClick={disconnectVNC}
                  className="flex items-center gap-2 px-4 py-2 bg-red-500 hover:bg-red-600 text-white rounded-lg transition-colors"
                >
                  <PowerOff className="w-4 h-4" />
                  <span>断开连接</span>
                </button>
              </div>
            )}

            {connectionState === 'disconnected' && selectedServer && (
              <button
                onClick={() => connectToVNC(selectedServer)}
                className="flex items-center gap-2 px-4 py-2 bg-primary hover:bg-primary-hover text-white rounded-lg transition-colors"
              >
                <Cable className="w-4 h-4" />
                <span>重新连接</span>
              </button>
            )}
          </div>
        </div>

        {/* 错误提示 */}
        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-4">
            <div className="flex gap-3">
              <AlertCircle className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" />
              <div>
                <h3 className="text-sm font-medium text-red-400">连接错误</h3>
                <p className="text-sm text-red-300 mt-1">{error}</p>
              </div>
            </div>
          </div>
        )}

        {/* 提示信息 */}
        <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4">
          <div className="flex gap-3">
            <AlertCircle className="w-5 h-5 text-blue-400 flex-shrink-0 mt-0.5" />
            <div>
              <h3 className="text-sm font-medium text-blue-400">提示</h3>
              <p className="text-sm text-blue-300 mt-1">
                Windows 服务器需要先安装并启动 VNC 服务器（推荐 TightVNC 或 RealVNC），默认端口 5900。
              </p>
              <p className="text-sm text-blue-300 mt-2">
                后端通过 WebSocket 代理将浏览器连接桥接到 VNC 服务器的 TCP 连接，实现远程桌面访问。
              </p>
            </div>
          </div>
        </div>

        {/* VNC 显示区域 */}
        <div className="bg-card border border-border rounded-lg overflow-hidden">
          <div
            ref={containerRef}
            className="w-full h-[600px] bg-gray-900 flex items-center justify-center"
          >
            {connectionState === 'disconnected' && !selectedServer && (
              <div className="text-center text-gray-400">
                <MonitorPlay className="w-16 h-16 mx-auto mb-4 opacity-50" />
                <p>选择服务器开始连接</p>
              </div>
            )}
            {connectionState === 'connecting' && (
              <div className="text-center text-gray-400">
                <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary mx-auto mb-4"></div>
                <p>正在连接 VNC 服务器...</p>
                {vncConfig && (
                  <p className="text-sm text-gray-500 mt-2">
                    {vncConfig.hostname}:{vncConfig.vnc_port}
                  </p>
                )}
              </div>
            )}
            {connectionState === 'connected' && (
              <div className="text-center text-gray-300">
                <MonitorPlay className="w-16 h-16 mx-auto mb-4 text-green-400" />
                <p className="text-lg mb-2">VNC 连接已建立</p>
                {vncConfig && (
                  <p className="text-sm text-gray-400">
                    服务器: {vncConfig.hostname}:{vncConfig.vnc_port}
                  </p>
                )}
                <p className="text-xs text-gray-500 mt-4">
                  完整的 noVNC 画面渲染需集成 @novnc/novnc 库
                </p>
              </div>
            )}
            {connectionState === 'error' && (
              <div className="text-center text-gray-400">
                <AlertCircle className="w-16 h-16 mx-auto mb-4 text-red-400 opacity-50" />
                <p>连接失败</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
