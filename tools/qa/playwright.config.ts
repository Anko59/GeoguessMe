import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.QA_BASE_URL;
if (!baseURL) throw new Error('QA_BASE_URL is required');

const extraHTTPHeaders: Record<string, string> = {};
if (process.env.QA_ACCESS_CLIENT_ID && process.env.QA_ACCESS_CLIENT_SECRET) {
    extraHTTPHeaders['CF-Access-Client-Id'] = process.env.QA_ACCESS_CLIENT_ID;
    extraHTTPHeaders['CF-Access-Client-Secret'] = process.env.QA_ACCESS_CLIENT_SECRET;
}

const artifactDir = process.env.QA_ARTIFACT_DIR || '/tmp/qa-artifacts';

export default defineConfig({
    testDir: '.',
    testMatch: /agent\.spec\.ts$/,
    globalSetup: './global-setup.ts',
    forbidOnly: true,
    retries: 0,
    workers: 1,
    timeout: 90000,
    outputDir: `${artifactDir}/test-results`,
    reporter: [
        ['list'],
        ['json', { outputFile: `${artifactDir}/playwright.json` }],
        ['./reporter.ts', { outputFile: `${artifactDir}/qa-report.json` }],
    ],
    use: {
        baseURL,
        extraHTTPHeaders,
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
        video: 'retain-on-failure',
        actionTimeout: 15000,
    },
    projects: [
        {
            name: 'desktop',
            use: { ...devices['Desktop Chrome'] },
        },
        {
            name: 'mobile',
            use: { ...devices['Pixel 5'] },
        },
    ],
});
