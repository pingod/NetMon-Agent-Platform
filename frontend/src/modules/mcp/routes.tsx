import { lazyRetry } from '@/lib/lazyRetry';

// MCP 模块页面（代码分割）
const McpOverview = lazyRetry(() => import('./pages/McpOverview'));
const ToolBrowser = lazyRetry(() => import('./pages/ToolBrowser'));
const ExternalServers = lazyRetry(() => import('./pages/ExternalServers'));
const ToolTester = lazyRetry(() => import('./pages/ToolTester'));

/**
 * MCP (Model Context Protocol) 模块路由
 * 
 * /mcp/overview         - MCP 服务概览
 * /mcp/tools            - 工具浏览器
 * /mcp/external-servers - 外部 MCP 服务器管理
 * /mcp/tester           - 工具调用测试
 */
export const mcpRoutes = [
  { path: 'mcp/overview', element: <McpOverview /> },
  { path: 'mcp/tools', element: <ToolBrowser /> },
  { path: 'mcp/external-servers', element: <ExternalServers /> },
  { path: 'mcp/tester', element: <ToolTester /> },
];