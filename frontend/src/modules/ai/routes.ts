/**
 * AI 模块前端路由定义
 */
import { lazyRetry } from '@/lib/lazyRetry';

const Agents = lazyRetry(() => import('./pages/Agents'));
const Knowledge = lazyRetry(() => import('./pages/Knowledge'));
const RootCauseAnalysis = lazyRetry(() => import('./pages/RootCauseAnalysis'));
const AIRootCause = lazyRetry(() => import('./pages/AIRootCause'));
const RCADetail = lazyRetry(() => import('./pages/RCADetail'));
const AIInsights = lazyRetry(() => import('./pages/AIInsights'));
const AIModels = lazyRetry(() => import('./pages/AIModels'));
const AiRemediations = lazyRetry(() => import('./pages/AiRemediations'));

export const aiRoutes = [
  { path: 'agents', element: Agents },
  { path: 'agents/tools', element: Agents },
  { path: 'knowledge', element: Knowledge },
  { path: 'root-cause-analysis', element: RootCauseAnalysis },
  { path: 'ai-root-cause', element: AIRootCause },
  { path: 'ai-root-cause/:id', element: RCADetail },
  { path: 'ai-insights', element: AIInsights },
  { path: 'ai-models', element: AIModels },
  { path: 'ai-remediations', element: AiRemediations },
];
