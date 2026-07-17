import { defineConfig, devices } from '@playwright/test';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';

export default defineConfig({
    testDir: './e2e',
    forbidOnly: !!process.env.CI,
    retries: process.env.CI ? 2 : 0,
    workers: 1,
    reporter: [
        ['html'],
        ['junit', { outputFile: 'test-results/junit.xml' }],
    ],
    use: {
        baseURL: BASE_URL,
        trace: 'on-first-retry',
        screenshot: 'only-on-failure',
        video: 'retain-on-failure',
    },
    projects: [
        {
            name: 'desktop',
            use: {
                ...devices['Desktop Chrome'],
            },
        },
        {
            name: 'mobile',
            use: {
                ...devices['Pixel 5'],
                permissions: ['camera', 'geolocation'],
                geolocation: { latitude: 48.8566, longitude: 2.3522 },
            },
        },
    ],
});
