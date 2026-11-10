import React from 'react';

export interface DeviceIconProps {
  size?: number;
  status?: 'online' | 'offline' | 'warning' | 'error' | string;
  hasAlert?: boolean;
  downCount?: number;
  className?: string;
}

// 核心交换机 (Chassis Modular Core Switch)
export const CoreSwitchIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-xl bg-red-500/20 animate-ping" style={{ transform: 'scale(1.2)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="chassisGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#1e293b" />
            <stop offset="50%" stopColor="#0f172a" />
            <stop offset="100%" stopColor="#090d16" />
          </linearGradient>
          <linearGradient id="slotGrad" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="#334155" />
            <stop offset="100%" stopColor="#1e293b" />
          </linearGradient>
          <filter id="coreGlow">
            <feDropShadow dx="0" dy="2" stdDeviation="3" floodColor="#0284c7" floodOpacity="0.4" />
          </filter>
        </defs>

        {/* 外壳机箱 */}
        <rect x="8" y="12" width="84" height="76" rx="6" fill="url(#chassisGrad)" stroke="#38bdf8" strokeWidth="1.5" filter="url(#coreGlow)" />

        {/* 顶部散热排孔 */}
        <g fill="#475569">
          <circle cx="20" cy="18" r="1.5" /><circle cx="26" cy="18" r="1.5" /><circle cx="32" cy="18" r="1.5" />
          <circle cx="38" cy="18" r="1.5" /><circle cx="44" cy="18" r="1.5" /><circle cx="50" cy="18" r="1.5" />
          <circle cx="56" cy="18" r="1.5" /><circle cx="62" cy="18" r="1.5" /><circle cx="68" cy="18" r="1.5" />
        </g>

        {/* 机框插槽 Slot 1 - 4 */}
        <rect x="14" y="24" width="72" height="12" rx="2" fill="url(#slotGrad)" stroke="#475569" strokeWidth="0.8" />
        <rect x="14" y="39" width="72" height="12" rx="2" fill="url(#slotGrad)" stroke="#475569" strokeWidth="0.8" />
        <rect x="14" y="54" width="72" height="12" rx="2" fill="url(#slotGrad)" stroke="#475569" strokeWidth="0.8" />
        <rect x="14" y="69" width="72" height="12" rx="2" fill="url(#slotGrad)" stroke="#475569" strokeWidth="0.8" />

        {/* 端口光口组 */}
        {[27, 42, 57, 72].map((y, row) => (
          <g key={row}>
            <rect x="18" y={y} width="7" height="6" rx="1" fill="#0284c7" />
            <rect x="28" y={y} width="7" height="6" rx="1" fill="#0284c7" />
            <rect x="38" y={y} width="7" height="6" rx="1" fill="#0284c7" />
            <rect x="48" y={y} width="7" height="6" rx="1" fill="#0284c7" />
            <circle cx="60" cy={y + 3} r="1.5" fill="#10b981" />
            <circle cx="66" cy={y + 3} r="1.5" fill="#10b981" />
            <circle cx="72" cy={y + 3} r="1.5" fill={isDown && row === 1 ? '#ef4444' : '#10b981'} />
            <circle cx="78" cy={y + 3} r="1.5" fill="#10b981" />
          </g>
        ))}

        {/* 主控指示灯 */}
        <circle cx="82" cy="18" r="2.5" fill={statusColor} />
      </svg>
    </div>
  );
};

// 接入/汇聚交换机 (1U Rack Switch)
export const AccessSwitchIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-lg bg-red-500/20 animate-ping" style={{ transform: 'scale(1.15)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="switchGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#1e293b" />
            <stop offset="100%" stopColor="#0f172a" />
          </linearGradient>
        </defs>

        {/* 机架耳朵与机箱 */}
        <rect x="4" y="28" width="92" height="44" rx="4" fill="url(#switchGrad)" stroke="#64748b" strokeWidth="1.5" />
        <rect x="4" y="36" width="4" height="28" fill="#334155" />
        <rect x="92" y="36" width="4" height="28" fill="#334155" />

        {/* RJ45 网口矩阵 */}
        {[0, 1].map(row => (
          <g key={row}>
            {[0, 1, 2, 3, 4, 5, 6, 7].map(col => (
              <rect
                key={col}
                x={14 + col * 7.5}
                y={36 + row * 14}
                width="5.5"
                height="9"
                rx="0.8"
                fill="#0f172a"
                stroke={isDown && row === 1 && col === 3 ? '#ef4444' : '#38bdf8'}
                strokeWidth="0.8"
              />
            ))}
          </g>
        ))}

        {/* SFP+ 光口 */}
        <rect x="76" y="36" width="6" height="12" rx="1" fill="#0284c7" stroke="#38bdf8" strokeWidth="0.8" />
        <rect x="83" y="36" width="6" height="12" rx="1" fill="#0284c7" stroke="#38bdf8" strokeWidth="0.8" />

        {/* LED 状态灯 */}
        <circle cx="10" cy="34" r="2" fill={statusColor} />
        <circle cx="78" cy="56" r="1.2" fill="#10b981" />
        <circle cx="85" cy="56" r="1.2" fill={isDown ? '#ef4444' : '#10b981'} />
      </svg>
    </div>
  );
};

// 路由器 (Enterprise Router)
export const RouterIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-full bg-red-500/20 animate-ping" style={{ transform: 'scale(1.2)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="routerGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#0369a1" />
            <stop offset="100%" stopColor="#082f49" />
          </linearGradient>
        </defs>

        {/* 圆盘底座 */}
        <circle cx="50" cy="50" r="38" fill="url(#routerGrad)" stroke="#38bdf8" strokeWidth="2" />
        <circle cx="50" cy="50" r="28" fill="#0f172a" stroke="#0284c7" strokeWidth="1.2" />

        {/* 路由交叉箭头 (经典 4 箭头交叉交互) */}
        <path d="M50 28 L50 42 M50 28 L45 34 M50 28 L55 34" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M50 72 L50 58 M50 72 L45 66 M50 72 L55 66" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M28 50 L42 50 M28 50 L34 45 M28 50 L34 55" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M72 50 L58 50 M72 50 L66 45 M72 50 L66 55" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />

        {/* 状态指示点 */}
        <circle cx="74" cy="26" r="3.5" fill={statusColor} />
      </svg>
    </div>
  );
};

// 防火墙 (Next-Gen Security Firewall)
export const FirewallIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-xl bg-red-500/20 animate-ping" style={{ transform: 'scale(1.2)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="fwGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#991b1b" />
            <stop offset="50%" stopColor="#7f1d1d" />
            <stop offset="100%" stopColor="#450a0a" />
          </linearGradient>
        </defs>

        {/* 砖墙机箱 */}
        <rect x="10" y="20" width="80" height="60" rx="6" fill="url(#fwGrad)" stroke="#f87171" strokeWidth="1.5" />

        {/* 砖块纹理 */}
        <path d="M10 40 H90 M10 60 H90" stroke="#b91c1c" strokeWidth="1.5" />
        <path d="M30 20 V40 M70 20 V40 M50 40 V60 M30 60 V80 M70 60 V80" stroke="#b91c1c" strokeWidth="1.5" />

        {/* 中央盾牌 */}
        <path d="M50 32 L64 38 V50 C64 60 50 68 50 68 C50 68 36 60 36 50 V38 Z" fill="#0f172a" stroke="#fca5a5" strokeWidth="1.8" />
        <path d="M50 40 V58 M44 48 H56" stroke="#ef4444" strokeWidth="2" strokeLinecap="round" />

        {/* 状态 LED */}
        <circle cx="82" cy="26" r="3" fill={statusColor} />
      </svg>
    </div>
  );
};

// 服务器 (Rack Server)
export const ServerIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-lg bg-red-500/20 animate-ping" style={{ transform: 'scale(1.15)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="srvGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#1e293b" />
            <stop offset="100%" stopColor="#0f172a" />
          </linearGradient>
        </defs>

        {/* 2U 机架服务器 */}
        <rect x="6" y="24" width="88" height="52" rx="4" fill="url(#srvGrad)" stroke="#94a3b8" strokeWidth="1.5" />

        {/* 4 个热插拔硬盘盒 */}
        {[0, 1, 2, 3].map(i => (
          <g key={i}>
            <rect x={12 + i * 14} y="32" width="11" height="36" rx="1.5" fill="#334155" stroke="#475569" strokeWidth="0.8" />
            <rect x={14 + i * 14} y="36" width="7" height="2" fill="#64748b" />
            <circle cx={17.5 + i * 14} cy="62" r="1.2" fill={isDown && i === 1 ? '#ef4444' : '#10b981'} />
          </g>
        ))}

        {/* 控制面板与电源按钮 */}
        <rect x="70" y="32" width="18" height="36" rx="1.5" fill="#0f172a" stroke="#475569" strokeWidth="0.8" />
        <circle cx="79" cy="40" r="3.5" fill="#38bdf8" />
        <circle cx="79" cy="50" r="1.5" fill="#10b981" />
        <circle cx="79" cy="56" r="1.5" fill={statusColor} />
      </svg>
    </div>
  );
};

// 无线 AP (Wireless AP)
export const WirelessApIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-full bg-red-500/20 animate-ping" style={{ transform: 'scale(1.2)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        {/* 吸顶 AP 椭圆主体 */}
        <circle cx="50" cy="50" r="34" fill="#1e293b" stroke="#38bdf8" strokeWidth="2" />
        <circle cx="50" cy="50" r="22" fill="#0f172a" stroke="#64748b" strokeWidth="1" />

        {/* Wi-Fi 辐射波纹 */}
        <path d="M36 44 C44 36 56 36 64 44" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" />
        <path d="M41 51 C46 46 54 46 59 51" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" />
        <circle cx="50" cy="58" r="2.5" fill={statusColor} />
      </svg>
    </div>
  );
};

// 存储阵列 (Storage SAN Array)
export const StorageSanIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-lg bg-red-500/20 animate-ping" style={{ transform: 'scale(1.15)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <rect x="8" y="18" width="84" height="64" rx="4" fill="#1e293b" stroke="#a855f7" strokeWidth="1.5" />
        {[0, 1, 2].map(row => (
          <g key={row}>
            <rect x="14" y={24 + row * 18} width="72" height="14" rx="2" fill="#0f172a" stroke="#475569" strokeWidth="0.8" />
            <rect x="18" y={28 + row * 18} width="12" height="6" rx="1" fill="#a855f7" />
            <rect x="34" y={28 + row * 18} width="12" height="6" rx="1" fill="#a855f7" />
            <rect x="50" y={28 + row * 18} width="12" height="6" rx="1" fill="#a855f7" />
            <circle cx="78" cy={31 + row * 18} r="2" fill={isDown && row === 1 ? '#ef4444' : '#10b981'} />
          </g>
        ))}
        <circle cx="86" cy="22" r="2.5" fill={statusColor} />
      </svg>
    </div>
  );
};

// 云网关 / WAN 出口 (Cloud Gateway)
export const CloudGatewayIcon: React.FC<DeviceIconProps> = ({ size = 64, status = 'online', hasAlert, downCount, className }) => {
  const isDown = hasAlert || (downCount && downCount > 0) || status === 'error';
  const statusColor = isDown ? '#ef4444' : status === 'warning' ? '#f59e0b' : status === 'offline' ? '#64748b' : '#10b981';

  return (
    <div className={`relative inline-flex items-center justify-center ${className || ''}`} style={{ width: size, height: size }}>
      {isDown && (
        <div className="absolute inset-0 rounded-full bg-red-500/20 animate-ping" style={{ transform: 'scale(1.2)' }} />
      )}
      <svg width={size} height={size} viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="cloudGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#38bdf8" />
            <stop offset="100%" stopColor="#0284c7" />
          </linearGradient>
        </defs>
        <path
          d="M28 62 C22 62 18 56 18 50 C18 44 24 40 29 40 C31 32 38 26 48 26 C58 26 66 33 68 41 C74 41 80 46 80 52 C80 58 74 62 68 62 Z"
          fill="url(#cloudGrad)"
          stroke="#e0f2fe"
          strokeWidth="1.5"
        />
        {/* 网关双向管道 */}
        <path d="M42 62 V76 M58 62 V76 M34 76 H66" stroke="#0f172a" strokeWidth="3" strokeLinecap="round" />
        <circle cx="50" cy="76" r="3" fill={statusColor} />
      </svg>
    </div>
  );
};

// 辅助函数：根据设备类型与状态渲染对应图标
export function renderDeviceIcon(type: string, props: DeviceIconProps = {}) {
  const normType = (type || 'switch').toLowerCase();
  if (normType.includes('core') || normType === 'core_switch') {
    return <CoreSwitchIcon {...props} />;
  }
  if (normType.includes('router') || normType.includes('gateway') || normType === 'route') {
    return <RouterIcon {...props} />;
  }
  if (normType.includes('firewall') || normType.includes('fw') || normType === 'security') {
    return <FirewallIcon {...props} />;
  }
  if (normType.includes('server') || normType.includes('host') || normType === 'compute') {
    return <ServerIcon {...props} />;
  }
  if (normType.includes('ap') || normType.includes('wireless') || normType.includes('wlc')) {
    return <WirelessApIcon {...props} />;
  }
  if (normType.includes('storage') || normType.includes('san') || normType.includes('nas')) {
    return <StorageSanIcon {...props} />;
  }
  if (normType.includes('cloud') || normType.includes('wan') || normType.includes('internet')) {
    return <CloudGatewayIcon {...props} />;
  }
  // 默认使用接入/汇聚交换机
  return <AccessSwitchIcon {...props} />;
}
