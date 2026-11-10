import { lazyRetry } from '@/lib/lazyRetry';

const ConfigTemplates = lazyRetry(() => import('./pages/ConfigTemplates'));

export const configManagementRoutes = [
  { path: 'config-templates', element: ConfigTemplates },
];
