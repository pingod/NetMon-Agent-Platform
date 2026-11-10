import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

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
        target: process.env.DOCKER_MODE ? 'http://backend:3001' : 'http://localhost:8088',
        changeOrigin: true,
      },
      '/socket.io': {
        target: process.env.DOCKER_MODE ? 'http://backend:3001' : 'http://localhost:8088',
        ws: true,
      },
      '/ws': {
        target: process.env.DOCKER_MODE ? 'ws://backend:3001' : 'ws://localhost:8088',
        ws: true,
      },
    },
  },
});

