import { randomBytes } from 'node:crypto';
import { test, expect, type BrowserContext, type Page, type TestInfo } from '@playwright/test';

const password = 'QaAgentPass123';
const image = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg==',
    'base64',
);

interface Credentials {
    username: string;
    email: string;
}

interface Diagnostics {
    consoleErrors: string[];
    pageErrors: string[];
    failedRequests: string[];
    serverErrors: string[];
    cloudflareTelemetrySeen: boolean;
}

function isCloudflareTelemetry(url: string): boolean {
    return url.includes('static.cloudflareinsights.com/');
}

function suffix(): string {
    return `${Date.now()}_${randomBytes(3).toString('hex')}`;
}

function credentials(label: string): Credentials {
    const id = `${label}_${suffix()}`;
    return { username: id.slice(0, 48), email: `${id}@qa.geoguessme.invalid` };
}

function observe(page: Page): Diagnostics {
    const diagnostics: Diagnostics = {
        consoleErrors: [],
        pageErrors: [],
        failedRequests: [],
        serverErrors: [],
        cloudflareTelemetrySeen: false,
    };
    page.on('request', (request) => {
        if (isCloudflareTelemetry(request.url())) diagnostics.cloudflareTelemetrySeen = true;
    });
    page.on('console', (message) => {
        if (message.type() !== 'error') return;
        const text = message.text();
        const isExternalTelemetryNoise =
            text.includes('static.cloudflareinsights.com/') ||
            (diagnostics.cloudflareTelemetrySeen &&
                /Failed to load resource: the server responded with a status of (401|429)/.test(text));
        if (!isExternalTelemetryNoise) diagnostics.consoleErrors.push(text);
    });
    page.on('pageerror', (error) => diagnostics.pageErrors.push(error.message));
    page.on('requestfailed', (request) => {
        if (request.failure()?.errorText !== 'net::ERR_ABORTED' && !isCloudflareTelemetry(request.url()))
            diagnostics.failedRequests.push(`${request.method()} ${request.url()}`);
    });
    page.on('response', (response) => {
        if (response.status() >= 500)
            diagnostics.serverErrors.push(`${response.status()} ${response.request().method()} ${response.url()}`);
    });
    return diagnostics;
}

async function record(page: Page, testInfo: TestInfo, diagnostics: Diagnostics): Promise<void> {
    const screenshot = testInfo.outputPath('journey.png');
    await page.screenshot({ path: screenshot, fullPage: true }).catch(() => undefined);
    await testInfo.attach('journey-screenshot', { path: screenshot }).catch(() => undefined);
    const diagnosticsPath = testInfo.outputPath('diagnostics.json');
    await import('node:fs/promises').then(({ writeFile }) =>
        writeFile(diagnosticsPath, `${JSON.stringify(diagnostics, null, 2)}\n`, 'utf8'),
    );
    await testInfo.attach('browser-diagnostics', { path: diagnosticsPath });
}

function assertClean(diagnostics: Diagnostics): void {
    const problems = [
        ...diagnostics.consoleErrors.map((item) => `console.error: ${item}`),
        ...diagnostics.pageErrors.map((item) => `pageerror: ${item}`),
        ...diagnostics.failedRequests.map((item) => `requestfailed: ${item}`),
        ...diagnostics.serverErrors.map((item) => `server: ${item}`),
    ];
    expect(problems, problems.join('\n')).toEqual([]);
}

async function signup(page: Page, account: Credentials): Promise<void> {
    await page.goto('/signup');
    await page.getByLabel('Username').fill(account.username);
    await page.getByLabel(/Recovery email/).fill(account.email);
    await page.getByLabel('Password').fill(password);
    await page.getByRole('button', { name: 'Sign Up' }).click();
    await page.waitForURL(/\/groups(?:$|\?)/);
    await expect(page.getByRole('heading', { name: 'My Groups' })).toBeVisible();
    const installPrompt = page.getByRole('dialog', { name: 'Install GeoGuessMe' });
    const dismissInstallPrompt = installPrompt.getByRole('button', { name: /dismiss|close/i });
    if (await dismissInstallPrompt.isVisible().catch(() => false)) await dismissInstallPrompt.click();
}

async function createGroup(page: Page): Promise<string> {
    await page.goto('/group/create');
    await page.getByPlaceholder('Group Name').fill(`QA Circle ${suffix()}`);
    await page.locator('form').getByRole('button', { name: 'Create Group' }).click();
    await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    return page.url().split('/group/')[1];
}

async function createInvite(page: Page): Promise<string> {
    await page.getByRole('button', { name: 'Open group settings' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'Create invite link' }).click();
    const invite = dialog.getByLabel('Invite link');
    await expect(invite).toHaveValue(/#invite=/);
    const value = await invite.inputValue();
    await dialog.getByRole('button', { name: 'Close settings' }).click();
    return value;
}

async function joinInvite(page: Page, inviteURL: string, groupID: string): Promise<void> {
    await page.goto(inviteURL);
    await page.getByRole('button', { name: /join/i }).click();
    await page.waitForURL(new RegExp(`/group/${groupID}$`));
}

async function closeContexts(contexts: BrowserContext[]): Promise<void> {
    await Promise.all(contexts.map((context) => context.close()));
}

test.describe('Independent black-box release QA', () => {
    test('auth session, email dispatch state, security headers, and navigation', async ({ browser }, testInfo) => {
        const context = await browser.newContext();
        const page = await context.newPage();
        const diagnostics = observe(page);
        try {
            const home = await page.goto('/');
            expect(home?.status()).toBe(200);
            const headers = home?.headers() ?? {};
            expect(headers['x-content-type-options']).toBe('nosniff');
            expect(headers['x-frame-options']).toBe('DENY');
            expect(headers['referrer-policy']).toBe('strict-origin-when-cross-origin');
            expect(headers['content-security-policy']).toContain("default-src 'self'");

            const account = credentials('qa_auth');
            await signup(page, account);
            await page.goto('/settings');
            await expect(page.getByText('Pending verification')).toBeVisible();
            await expect(page.getByText(new RegExp(`Verification was requested for ${account.email}`))).toBeVisible();

            await page.reload();
            await page.waitForURL(/\/settings$/);
            await page.goto('/');
            await page.goto('/settings');
            await page.getByRole('button', { name: /log ?out/i }).click();
            await page.waitForURL(/\/$/);
            await page.goto('/groups');
            await page.waitForURL(/\/login/);

            await page.getByLabel('Username').fill(account.username);
            await page.getByLabel('Password').fill(password);
            await page.getByRole('button', { name: 'Login' }).click();
            await page.waitForURL(/\/groups/);
            await page.reload();
            await page.waitForURL(/\/groups/);
            assertClean(diagnostics);
        } finally {
            await record(page, testInfo, diagnostics);
            await context.close();
        }
    });

    test('groups, authorization, R2 media read/write, WebSocket chat, and refresh', async ({ browser }, testInfo) => {
        const contexts: BrowserContext[] = [];
        const ownerContext = await browser.newContext();
        contexts.push(ownerContext);
        const owner = await ownerContext.newPage();
        const ownerDiagnostics = observe(owner);
        try {
            await signup(owner, credentials('qa_owner'));
            const groupID = await createGroup(owner);
            const inviteURL = await createInvite(owner);
            await expect(owner.getByRole('status')).toHaveText('Connected', { timeout: 20000 });

            const mediaDialog = async (): Promise<void> => {
                await owner.getByRole('button', { name: 'Open group settings' }).click();
                const dialog = owner.getByRole('dialog');
                await dialog.locator('input[type="file"]').setInputFiles({
                    name: 'qa-group.png',
                    mimeType: 'image/png',
                    buffer: image,
                });
                await expect(owner.locator('img.header-logo')).toHaveAttribute('src', /^blob:/);
                await dialog.getByRole('button', { name: 'Close settings' }).click();
            };
            await mediaDialog();
            await owner.goto('/groups');
            const groupCard = owner.locator(`[data-group-id="${groupID}"]`);
            await expect(groupCard).toBeVisible();
            await expect(groupCard.locator('img')).toHaveAttribute('src', /^blob:/);

            const memberContext = await browser.newContext();
            contexts.push(memberContext);
            const member = await memberContext.newPage();
            const memberDiagnostics = observe(member);
            await signup(member, credentials('qa_member'));
            await joinInvite(member, inviteURL, groupID);
            await expect(member.getByRole('status')).toHaveText('Connected', { timeout: 20000 });
            await owner.goto(`/group/${groupID}`);
            await expect(owner.getByRole('status')).toHaveText('Connected', { timeout: 20000 });

            const message = `QA realtime ${suffix()}`;
            await owner.getByLabel('Message').fill(message);
            await owner.getByRole('button', { name: 'Send message' }).click();
            await expect(owner.getByText(message, { exact: true })).toBeVisible();
            await expect(member.getByText(message, { exact: true })).toBeVisible();

            await member.reload();
            await member.waitForURL(new RegExp(`/group/${groupID}$`));
            await expect(member.getByRole('status')).toHaveText('Connected', { timeout: 20000 });
            await expect(member.getByText(message, { exact: true })).toBeVisible();

            const outsiderContext = await browser.newContext();
            contexts.push(outsiderContext);
            const outsider = await outsiderContext.newPage();
            const outsiderDiagnostics = observe(outsider);
            await signup(outsider, credentials('qa_outsider'));
            await outsider.goto(`/group/${groupID}`);
            await expect(outsider.getByRole('alert')).toContainText('not a member');
            await member.goto('/group/join#invite=not-a-real-qa-invite');
            await expect(member.getByRole('alert')).toContainText('invalid or has expired');

            await owner.goto('/groups');
            await owner.goBack();
            await owner.goForward();
            await expect(owner.getByRole('heading', { name: 'My Groups' })).toBeVisible();
            assertClean(ownerDiagnostics);
            assertClean(memberDiagnostics);
            assertClean(outsiderDiagnostics);
        } finally {
            await record(owner, testInfo, ownerDiagnostics);
            await closeContexts(contexts);
        }
    });

    test('mobile layout remains usable without horizontal overflow', async ({ browser }, testInfo) => {
        const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
        const page = await context.newPage();
        const diagnostics = observe(page);
        try {
            await signup(page, credentials('qa_mobile'));
            await expect
                .poll(() =>
                    page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
                )
                .toBe(true);
            await page.goto('/group/create');
            await expect(page.getByPlaceholder('Group Name')).toBeVisible();
            await expect(page.locator('form').getByRole('button', { name: 'Create Group' })).toBeVisible();
            assertClean(diagnostics);
        } finally {
            await record(page, testInfo, diagnostics);
            await context.close();
        }
    });

    test('identity rate limiting rejects repeated invalid logins', async ({ browser }, testInfo) => {
        test.skip(testInfo.project.name !== 'desktop', 'Run the abuse probe once on desktop.');
        const context = await browser.newContext();
        const page = await context.newPage();
        const diagnostics = observe(page);
        try {
            await page.goto('/login');
            const statuses: number[] = [];
            const identity = `not-a-user-${suffix()}`;
            for (let attempt = 0; attempt < 11; attempt += 1) {
                const responsePromise = page.waitForResponse(
                    (response) =>
                        response.url().endsWith('/api/v1/auth/login') && response.request().method() === 'POST',
                );
                await page.getByLabel('Username').fill(identity);
                await page.getByLabel('Password').fill('DefinitelyWrong123');
                await page.getByRole('button', { name: 'Login' }).click();
                statuses.push((await responsePromise).status());
                await expect(page).toHaveURL(/\/login/);
            }
            expect(statuses).toContain(429);
            assertClean(diagnostics);
        } finally {
            await record(page, testInfo, diagnostics);
            await context.close();
        }
    });
});
