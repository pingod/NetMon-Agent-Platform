import { lazyRetry } from '@/lib/lazyRetry';

const Containers = lazyRetry(() => import('./pages/Containers'));
const ContainerMonitor = lazyRetry(() => import('./pages/ContainerMonitor'));
const ContainerLogs = lazyRetry(() => import('./pages/ContainerLogs'));
const Images = lazyRetry(() => import('./pages/Images'));
const Volumes = lazyRetry(() => import('./pages/Volumes'));
const ComposeEditor = lazyRetry(() => import('./pages/ComposeEditor'));
const SnapshotPolicies = lazyRetry(() => import('./pages/SnapshotPolicies'));
const ImageRegistry = lazyRetry(() => import('./pages/ImageRegistry'));

export const containerRoutes = [
  { path: 'containers', element: Containers },
  { path: 'container-monitor', element: ContainerMonitor },
  { path: 'container-logs', element: ContainerLogs },
  { path: 'images', element: Images },
  { path: 'volumes', element: Volumes },
  { path: 'compose', element: ComposeEditor },
  { path: 'snapshot-policies', element: SnapshotPolicies },
  { path: 'image-registry', element: ImageRegistry },
];
