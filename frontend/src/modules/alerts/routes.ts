import { lazyRetry } from '@/lib/lazyRetry';

const Alerts = lazyRetry(() => import('./pages/Alerts'));
const AlertMappings = lazyRetry(() => import('./pages/AlertMappings'));
const AlertNoiseManagement = lazyRetry(() => import('./pages/AlertNoiseManagement'));
const AlertSilences = lazyRetry(() => import('./pages/AlertSilences'));
const AlertAutoAnalysis = lazyRetry(() => import('./pages/AlertAutoAnalysis'));
const InspectionCenter = lazyRetry(() => import('./pages/InspectionCenter'));
const AlertCorrelationGroupsPage = lazyRetry(() => import('./pages/AlertCorrelationGroups'));
const AlertProviders = lazyRetry(() => import('./pages/AlertProviders'));
const AlertRules = lazyRetry(() => import('./pages/AlertRules'));

export const alertRoutes = [
  { path: 'alerts', element: Alerts },
  { path: 'alerts/providers', element: AlertProviders },
  { path: 'alert-rules', element: AlertRules },
  { path: 'alert-mappings', element: AlertMappings },
  { path: 'alert-noise', element: AlertNoiseManagement },
  { path: 'alert-silences', element: AlertSilences },
  { path: 'alert-auto-analysis', element: AlertAutoAnalysis },
  { path: 'inspection-center', element: InspectionCenter },
  { path: 'alert-correlation-groups', element: AlertCorrelationGroupsPage },
];
