import { lazyRetry } from '@/lib/lazyRetry';

const Notifications = lazyRetry(() => import('./pages/Notifications'));
const NotificationSettings = lazyRetry(() => import('./pages/NotificationSettings'));

export const notificationRoutes = [
  { path: 'notifications', element: Notifications },
  { path: 'notifications/settings', element: NotificationSettings },
];
