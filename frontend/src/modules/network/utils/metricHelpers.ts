/* eslint-disable @typescript-eslint/no-explicit-any */
import {
  Cpu, HardDrive, Wind, Zap, Thermometer, Activity, Server, Database
} from 'lucide-react';

/**
 * 清理并提取原始采集值
 * 例如：.1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7.16777217=49 => 49
 * 或：hwEntityMemUsage=49 => 49
 */
export function cleanDisplayValue(rawValue: string | null | undefined): string {
  if (!rawValue || rawValue === 'N/A' || rawValue === 'null' || rawValue === 'undefined') {
    return '-';
  }
  let val = rawValue.trim();

  // 如果包含分号多个结果 (如 16777216=0; 16777217=2048; ...)，先拆分每一项
  if (val.includes(';')) {
    const subParts = val.split(';').map(p => p.trim()).filter(Boolean);
    if (subParts.length > 0) {
      // 提取每一项的值
      const extractedList: string[] = subParts.map(sp => {
        if (sp.includes('=')) {
          const eqIdx = sp.lastIndexOf('=');
          return sp.substring(eqIdx + 1).trim();
        }
        return sp.trim();
      });

      // 智能选取：优先匹配第一个非 0 且非空的有效数据 (如 2048)
      const nonZero = extractedList.find(v => {
        const num = parseFloat(v);
        return (!isNaN(num) && num !== 0) || (v !== '' && v !== '0' && v !== '0.0' && v !== 'N/A');
      });

      val = nonZero !== undefined ? nonZero : (extractedList[0] || '-');
      return val || '-';
    }
  }

  // 单项包含等号 (如 .1.3.6...77217=49)，只提取等号右侧的值
  if (val.includes('=')) {
    const eqIdx = val.lastIndexOf('=');
    val = val.substring(eqIdx + 1).trim();
  }

  return val || '-';
}

/**
 * 根据指标名称和 OID 自动侦测图标与主题色 (内存, CPU, 风扇, 电源, 温度, 网络等)
 */
export function getMetricIconConfig(metricName: string = '', oid: string = '') {
  const text = (metricName + ' ' + oid).toLowerCase();

  // 内存 (Memory)
  if (/内存|mem|ram|memory|buffer|cache/i.test(text)) {
    return {
      icon: HardDrive,
      color: '#3b82f6',
      bg: 'bg-blue-500/15 text-blue-400 border-blue-500/30',
      label: '内存',
    };
  }
  // CPU (处理器)
  if (/cpu|处理器|core|processor/i.test(text)) {
    return {
      icon: Cpu,
      color: '#06b6d4',
      bg: 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30',
      label: 'CPU',
    };
  }
  // 风扇 (Fan)
  if (/风扇|fan|blower|cooling/i.test(text)) {
    return {
      icon: Wind,
      color: '#14b8a6',
      bg: 'bg-teal-500/15 text-teal-400 border-teal-500/30',
      label: '风扇',
    };
  }
  // 电源 / 电压 / 电流 (Power/Supply)
  if (/电源|power|psu|voltage|current|电压|电流|watt/i.test(text)) {
    return {
      icon: Zap,
      color: '#f59e0b',
      bg: 'bg-amber-500/15 text-amber-400 border-amber-500/30',
      label: '电源',
    };
  }
  // 温度 (Temperature)
  if (/温度|temp|temperature|thermal/i.test(text)) {
    return {
      icon: Thermometer,
      color: '#ef4444',
      bg: 'bg-red-500/15 text-red-400 border-red-500/30',
      label: '温度',
    };
  }
  // 网络 / 流量 / 速率 (Network/Traffic/Interface)
  if (/网络|流量|速率|带宽|traffic|bandwidth|interface|port|packet|bytes|octets/i.test(text)) {
    return {
      icon: Activity,
      color: '#8b5cf6',
      bg: 'bg-purple-500/15 text-purple-400 border-purple-500/30',
      label: '网络',
    };
  }
  // 磁盘 / 存储 (Disk/Storage)
  if (/磁盘|存储|disk|storage|flash|nvme|ssd/i.test(text)) {
    return {
      icon: Database,
      color: '#ec4899',
      bg: 'bg-pink-500/15 text-pink-400 border-pink-500/30',
      label: '存储',
    };
  }

  // 默认指标
  return {
    icon: Server,
    color: '#3b82f6',
    bg: 'bg-blue-500/15 text-blue-400 border-blue-500/30',
    label: '指标',
  };
}
