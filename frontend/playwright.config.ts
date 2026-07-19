import { defineConfig, devices } from '@playwright/test';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
const OUTPUT_DIR = process.env.PLAYWRIGHT_OUTPUT_DIR || 'test-results';
const REPORT_DIR = process.env.PLAYWRIGHT_REPORT_DIR || 'playwright-report';
const baseOrigin = new URL(BASE_URL).origin;
const launchArgs = baseOrigin.startsWith('http://') ? [`--unsafely-treat-insecure-origin-as-secure=${baseOrigin}`] : [];

export default defineConfig({
    testDir: './e2e',
    forbidOnly: !!process.env.CI,
    retries: 0,
    workers: 1,
    outputDir: OUTPUT_DIR,
    reporter: [
        ['html', { outputFolder: REPORT_DIR }],
        ['junit', { outputFile: `${OUTPUT_DIR}/junit.xml` }],
    ],
    use: {
        baseURL: BASE_URL,
        trace: 'retain-on-first-failure',
        screenshot: 'only-on-failure',
        video: 'retain-on-failure',
        launchOptions: { args: launchArgs },
    },
    projects: [
        {
            name: 'desktop',
            use: {
                ...devices['Desktop Chrome'],
            },
        },
        {
            name: 'firefox',
            // Firefox is used for the cross-browser session contract. The
            // full camera suite remains Chromium-only because Playwright
            // Firefox rejects the camera permission capability at context
            // creation time.
            testMatch: /auth\.spec\.ts$/,
            use: {
                ...devices['Desktop Firefox'],
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
