import { lazyRetry } from '@/lib/lazyRetry';

const DataCenterManage = lazyRetry(() => import('./pages/DataCenterManage'));
const DataRoom = lazyRetry(() => import('./pages/DataRoom'));
const DcRack2D = lazyRetry(() => import('./pages/DcRack2D'));

export const dcRoutes = [
  { path: 'dc-manage', element: DataCenterManage },
  { path: 'dc-rack-2d', element: DcRack2D },
  { path: 'data-room', element: DataRoom },
];

