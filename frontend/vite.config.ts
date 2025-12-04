/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'happy-dom',
    setupFiles: './src/setupTests.ts',
  },
  server: {
    host: true,
    proxy: {
      '/api': {
        target: 'http://backend:8080', // Docker service name
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
      '/ws': {
        target: 'ws://backend:8080',
        ws: true,
      },
      '/uploads': {
        target: 'http://backend:8080',
        changeOrigin: true,
      }
    },
  },
})
