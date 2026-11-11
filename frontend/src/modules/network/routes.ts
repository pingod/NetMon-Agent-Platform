import { lazyRetry } from '@/lib/lazyRetry';

const NetworkDevices = lazyRetry(() => import('./pages/NetworkDevices'));
const Topology = lazyRetry(() => import('./pages/Topology'));
const NetworkDiscoveryPage = lazyRetry(() => import('./pages/NetworkDiscovery'));
const Networks = lazyRetry(() => import('./pages/Networks'));
const ConfigBackup = lazyRetry(() => import('./pages/ConfigBackup'));
const SnmpCollect = lazyRetry(() => import('./pages/SnmpCollect'));
const NetworkDashboard = lazyRetry(() => import('./pages/NetworkDashboard'));
const DeviceMetricsDashboard = lazyRetry(() => import('./pages/DeviceMetricsDashboard'));
const MetricTags = lazyRetry(() => import('./pages/MetricTags'));
const SnmpCommunities = lazyRetry(() => import('./pages/SnmpCommunities'));
const SnmpTraps = lazyRetry(() => import('./pages/SnmpTraps'));

export const networkRoutes = [
  { path: 'network-devices', element: NetworkDevices },
  { path: 'topology', element: Topology },
  { path: 'network-discovery', element: NetworkDiscoveryPage },
  { path: 'networks', element: Networks },
  { path: 'config-backup', element: ConfigBackup },
  { path: 'snmp-collect', element: SnmpCollect },
  { path: 'snmp-communities', element: SnmpCommunities },
  { path: 'snmp-traps', element: SnmpTraps },
  { path: 'metric-tags', element: MetricTags },
  { path: 'network-dashboard', element: NetworkDashboard },
  { path: 'device-metrics/:deviceId', element: DeviceMetricsDashboard },
];
