import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// 后端代理目标:DOCKER_MODE 用容器名,否则可用 API_PROXY_TARGET 指向非默认端口的开发实例
const apiTarget = process.env.DOCKER_MODE
  ? 'http://backend:3001'
  : process.env.API_PROXY_TARGET || 'http://localhost:8088';
const wsTarget = apiTarget.replace(/^http/, 'ws');

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    target: 'es2020',
    cssCodeSplit: true,
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('three')) {
              return 'vendor-three';
            }
            if (id.includes('xterm')) {
              return 'vendor-term';
            }
          }
        },
      },
    },
  },
  server: {
    port: 3000,
    hmr: {
      clientPort: 3000,
    },
    watch: {
      usePolling: true,
      interval: 1000,
    },
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
      '/socket.io': {
        target: apiTarget,
        ws: true,
      },
      '/ws': {
        target: wsTarget,
        ws: true,
      },
    },
  },
});

