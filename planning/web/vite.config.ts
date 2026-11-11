import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  // 嵌入 NetMon 时由 `/planning-app/` 提供静态资源；独立开发/运行仍是 `/`。
  base: process.env.VITE_BASE ?? '/',
  server: {
    port: 5173,
    proxy: {
      // 独立开发时直连规划引擎；嵌入 NetMon 时由 Go 反向代理 `/api/v1/planning`。
      '/api': { target: 'http://127.0.0.1:18001', changeOrigin: true },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
});
