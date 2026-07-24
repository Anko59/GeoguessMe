/// <reference types="vitest" />
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [react()],
    test: {
        globals: true,
        environment: 'happy-dom',
        setupFiles: './src/setupTests.ts',
        exclude: ['e2e/**', 'node_modules/**'],
        coverage: {
            provider: 'v8',
            reporter: ['text', 'lcov'],
            include: ['src/**/*.{ts,tsx}'],
            // Browser/device and routing composition are covered by the Dockerized
            // Playwright suite; keep the unit threshold focused on deterministic
            // application logic and independently testable UI components.
            exclude: [
                // Infrastructure files that are not independently unit-testable
                'src/setupTests.ts',
                'src/main.tsx',
                // Service worker and push registration are tested by the
                // Dockerized Playwright E2E suite.
                'src/push/serviceWorker.ts',
                'src/push/usePushBootstrap.ts',
                'src/push/push.ts',
            ],
            thresholds: { statements: 80, lines: 80, functions: 80, branches: 70 },
        },
    },
    server: {
        host: true,
        proxy: {
            '/api': {
                target: 'http://backend:8080', // Docker service name
                changeOrigin: true,
                ws: true,
            },
        },
    },
});
