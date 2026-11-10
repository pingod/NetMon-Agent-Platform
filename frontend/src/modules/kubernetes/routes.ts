import { lazyRetry } from '@/lib/lazyRetry';

const Kubernetes = lazyRetry(() => import('./pages/Kubernetes'));

export const kubernetesRoutes = [
  { path: 'kubernetes', element: Kubernetes },
];
