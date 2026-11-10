import { lazyRetry } from '@/lib/lazyRetry';

const InspectionTemplates = lazyRetry(() => import('./pages/InspectionTemplates'));

export const inspectionTemplateRoutes = [
  { path: 'inspection-templates', element: InspectionTemplates },
];
