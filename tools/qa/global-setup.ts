import { randomBytes } from 'node:crypto';
import fs from 'node:fs/promises';
import { chromium, type FullConfig, type Page } from '@playwright/test';
import { accountFile, QA_PASSWORD, type Credentials, type QaAccounts } from './accounts';

function suffix(): string {
    return `${Date.now()}_${randomBytes(3).toString('hex')}`;
}

function credentials(label: string, email = false): Credentials {
    const id = `${label}_${suffix()}`;
    return {
        username: id.slice(0, 48),
        ...(email ? { email: `${id}@qa.geoguessme.invalid` } : {}),
    };
}

async function signup(page: Page, account: Credentials): Promise<void> {
    await page.goto('/signup');
    await page.getByLabel('Username').fill(account.username);
    if (account.email) await page.getByLabel(/Recovery email/).fill(account.email);
    await page.getByLabel('Password').fill(QA_PASSWORD);
    await page.getByRole('button', { name: 'Sign Up' }).click();
    await page.waitForURL(/\/groups(?:$|\?)/);
}

export default async function globalSetup(config: FullConfig): Promise<void> {
    const baseURL = String(config.projects[0]?.use.baseURL || process.env.QA_BASE_URL || '');
    if (!baseURL) throw new Error('QA_BASE_URL is required');

    const extraHTTPHeaders: Record<string, string> = {};
    if (process.env.QA_ACCESS_CLIENT_ID && process.env.QA_ACCESS_CLIENT_SECRET) {
        extraHTTPHeaders['CF-Access-Client-Id'] = process.env.QA_ACCESS_CLIENT_ID;
        extraHTTPHeaders['CF-Access-Client-Secret'] = process.env.QA_ACCESS_CLIENT_SECRET;
    }

    const accounts: QaAccounts = {
        owner: credentials('qa_owner', true),
        member: credentials('qa_member'),
        outsider: credentials('qa_outsider'),
    };
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
