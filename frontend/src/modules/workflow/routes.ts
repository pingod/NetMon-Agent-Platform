import { lazyRetry } from '@/lib/lazyRetry';

const Workflows = lazyRetry(() => import('./pages/Workflows'));
const WorkflowEditor = lazyRetry(() => import('./pages/WorkflowEditor'));
const Tasks = lazyRetry(() => import('./pages/Tasks'));
const ScheduledTasks = lazyRetry(() => import('./pages/ScheduledTasks'));
const WorkflowProviders = lazyRetry(() => import('./pages/WorkflowProviders'));

export const workflowRoutes = [
  { path: 'workflows', element: Workflows },
  { path: 'workflows/providers', element: WorkflowProviders },
  { path: 'workflows/:id', element: WorkflowEditor },
  { path: 'tasks', element: Tasks },
  { path: 'scheduled-tasks', element: ScheduledTasks },
];
