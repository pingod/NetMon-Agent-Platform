import { lazyRetry } from '@/lib/lazyRetry';

const BackupSettings = lazyRetry(() => import('./pages/BackupSettings'));

export const backupRoutes = [
  { path: 'backups', element: BackupSettings },
];
