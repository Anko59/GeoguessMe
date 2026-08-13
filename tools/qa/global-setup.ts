import fs from 'node:fs/promises';
import { chromium, type FullConfig, type Page } from '@playwright/test';
import { accountFile, QA_PASSWORD, type Credentials, type QaAccounts } from './accounts';

function credentials(username: string, email?: string): Credentials {
    return { username, ...(email ? { email } : {}) };
}

const QA_ACCOUNTS: QaAccounts = {
    owner: credentials('qa_release_owner', 'qa-release-owner@example.test'),
    member: credentials('qa_release_member'),
    outsider: credentials('qa_release_outsider'),
};

async function login(page: Page, account: Credentials): Promise<void> {
    await page.goto('/login');
    await page.getByLabel('Username').fill(account.username);
    await page.getByLabel('Password').fill(QA_PASSWORD);
    await page.getByRole('button', { name: 'Login' }).click();
    await page.waitForURL(/\/groups(?:$|\?)/);
}

async function signup(page: Page, account: Credentials): Promise<void> {
    await page.goto('/signup');
    await page.getByLabel('Username').fill(account.username);
    if (account.email) await page.getByLabel(/Recovery email/).fill(account.email);
    await page.getByLabel('Password').fill(QA_PASSWORD);
    const responsePromise = page.waitForResponse((response) =>
        new URL(response.url()).pathname.endsWith('/api/v1/auth/signup'),
    );
    await page.getByRole('button', { name: 'Sign Up' }).click();
    const response = await responsePromise;
    if (response.status() === 409) {
        await login(page, account);
        return;
    }
    if (!response.ok()) throw new Error(`QA account signup failed with HTTP ${response.status()}`);
    try {
        await page.waitForURL(/\/(?:groups|login)(?:$|\?)/, { timeout: 15000 });
    } catch {
        const alert = await page
            .getByRole('alert')
            .textContent()
            .catch(() => null);
        throw new Error(`QA account signup did not navigate from ${page.url()}${alert ? `: ${alert}` : ''}`);
    }
    if (new URL(page.url()).pathname === '/login') await login(page, account);
}

export default async function globalSetup(config: FullConfig): Promise<void> {
    const baseURL = String(config.projects[0]?.use.baseURL || process.env.QA_BASE_URL || '');
    if (!baseURL) throw new Error('QA_BASE_URL is required');

    const extraHTTPHeaders: Record<string, string> = {};
    if (process.env.QA_ACCESS_CLIENT_ID && process.env.QA_ACCESS_CLIENT_SECRET) {
        extraHTTPHeaders['CF-Access-Client-Id'] = process.env.QA_ACCESS_CLIENT_ID;
        extraHTTPHeaders['CF-Access-Client-Secret'] = process.env.QA_ACCESS_CLIENT_SECRET;
    }

    const accounts = QA_ACCOUNTS;
    const browser = await chromium.launch();
    try {
        for (const account of Object.values(accounts)) {
            const context = await browser.newContext({ baseURL, extraHTTPHeaders });
            try {
                await signup(await context.newPage(), account);
            } finally {
                await context.close();
            }
        }
        await fs.mkdir(new URL('.', `file://${accountFile()}`).pathname, { recursive: true });
        await fs.writeFile(accountFile(), `${JSON.stringify(accounts, null, 2)}\n`, 'utf8');
    } finally {
        await browser.close();
    }
}
