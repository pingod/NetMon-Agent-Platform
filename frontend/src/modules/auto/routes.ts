import { lazyRetry } from '@/lib/lazyRetry';

const RemediationPolicies = lazyRetry(() => import('./pages/RemediationPolicies'));
const RemediationPolicyEditor = lazyRetry(() => import('./pages/RemediationPolicyEditor'));
const RemediationExecutions = lazyRetry(() => import('./pages/RemediationExecutions'));
const RemediationDashboard = lazyRetry(() => import('./pages/RemediationDashboard'));
const RemediationWorkbench = lazyRetry(() => import('./pages/RemediationWorkbench'));
const AutoScale = lazyRetry(() => import('./pages/AutoScale'));

export const autoRoutes = [
  { path: 'remediation-policies', element: RemediationPolicies },
  { path: 'remediation-policies/:id', element: RemediationPolicyEditor },
  { path: 'remediation-executions', element: RemediationExecutions },
  { path: 'remediation-dashboard', element: RemediationDashboard },
  { path: 'remediation-workbench', element: RemediationWorkbench },
  { path: 'auto-scale', element: AutoScale },
];
