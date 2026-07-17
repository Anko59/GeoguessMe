/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'happy-dom',
    setupFiles: './src/setupTests.ts',
    exclude: ['e2e/**', 'node_modules/**'],
  },
  server: {
    host: true,
    proxy: {
      '/api': {
        target: 'http://backend:8080', // Docker service name
        changeOrigin: true,
        ws: true,
      }
    },
  },
})
