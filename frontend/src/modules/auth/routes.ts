import { lazyRetry } from '@/lib/lazyRetry';

const Login = lazyRetry(() => import('./pages/Login'));
const ForcePasswordChange = lazyRetry(() => import('./pages/ForcePasswordChange'));
const Users = lazyRetry(() => import('./pages/Users'));

export const authRoutes = [
  { path: 'users', element: Users },
];

export const publicRoutes = [
  { path: '/login', element: Login },
  { path: '/force-password-change', element: ForcePasswordChange },
];
