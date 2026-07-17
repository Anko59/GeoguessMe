import { test, expect, type Browser, type BrowserContextOptions, type Page } from '@playwright/test';
import { newAuthContext, signupViaUI, uniqueGroup } from './helpers';

interface ChatScenario {
    ownerContext: Awaited<ReturnType<typeof newAuthContext>>;
    owner: Page;
    groupId: string;
    groupCode: string;
}

async function createScenario(browser: Browser, active: BrowserContextOptions): Promise<ChatScenario> {
    const ownerContext = await newAuthContext(browser, active);
    const owner = await ownerContext.newPage();
    await signupViaUI(owner);
    await owner.goto('/group/create');
    await owner.getByPlaceholder('Group Name').fill(uniqueGroup());
    await owner.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await owner.waitForURL(/\/group\/[0-9a-f-]{36}$/);

    const groupId = owner.url().split('/group/')[1];
    await owner.getByRole('button', { name: 'Open group settings' }).click();
    const settings = owner.getByRole('dialog');
    const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
    await settings.getByRole('button', { name: 'Close settings' }).click();
    await expect(owner.getByRole('status')).toHaveText('Connected');

    return { ownerContext, owner, groupId, groupCode };
}

async function addMember(
    browser: Browser,
    active: BrowserContextOptions,
    scenario: ChatScenario,
): Promise<{ context: Awaited<ReturnType<typeof newAuthContext>>; page: Page }> {
    const context = await newAuthContext(browser, active);
    const page = await context.newPage();
    await signupViaUI(page);
    await page.goto('/group/join');
    await page.getByPlaceholder('6-character code').fill(scenario.groupCode);
    await page.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
    await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    await page.goto(`/group/${scenario.groupId}`);
    await expect(page.getByRole('status')).toHaveText('Connected');
    return { context, page };
}

test.describe('Chat via WebSocket', () => {
    test('chat connect, send message, receive in real-time', async ({ browser }, testInfo) => {
        const active = testInfo.project.use as BrowserContextOptions;
        const scenario = await createScenario(browser, active);
        const member = await addMember(browser, active, scenario);
        try {
            await expect(scenario.owner.locator('.chat-container')).toBeVisible();
            await expect(member.page.locator('.chat-container')).toBeVisible();

            const msgText = `Hello from A at ${Date.now()}`;
            await scenario.owner.locator('#chat-message').fill(msgText);
            await scenario.owner.locator('form.message-input-container').getByRole('button', { name: 'Send' }).click();

            await expect(scenario.owner.locator('.message-container').filter({ hasText: msgText })).toBeVisible();
            await expect(member.page.locator('.message-container').filter({ hasText: msgText })).toBeVisible();
        } finally {
            await member.context.close();
            await scenario.ownerContext.close();
        }
    });

    test('reload restores message history', async ({ browser }, testInfo) => {
        const active = testInfo.project.use as BrowserContextOptions;
        const scenario = await createScenario(browser, active);
        const member = await addMember(browser, active, scenario);
        try {
            const msgText = `Hello before reload at ${Date.now()}`;
            await scenario.owner.locator('#chat-message').fill(msgText);
            await scenario.owner.locator('form.message-input-container').getByRole('button', { name: 'Send' }).click();
            await expect(member.page.locator('.message-container').filter({ hasText: msgText })).toBeVisible();

            await member.page.reload();
            await member.page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expect(member.page.getByRole('status')).toHaveText('Connected');
            await expect(member.page.locator('.message-container').filter({ hasText: msgText })).toBeVisible();
        } finally {
            await member.context.close();
            await scenario.ownerContext.close();
        }
    });

    test('one-time WS ticket reuse is rejected', async ({ browser }, testInfo) => {
        const active = testInfo.project.use as BrowserContextOptions;
        const scenario = await createScenario(browser, active);
        try {
            let usedTicket = '';
            await scenario.owner.route('**/api/v1/ws/ticket*', async (route) => {
                const response = await route.fetch();
                const body = (await response.json()) as { ticket: string };
                usedTicket = body.ticket;
                await route.fulfill({ response });
            });

            await scenario.owner.reload();
            await scenario.owner.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expect(scenario.owner.getByRole('status')).toHaveText('Connected');
            expect(usedTicket).toMatch(/^\S+$/);

            const baseUrl = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
            const wsBase = baseUrl.replace(/^http/, 'ws');
            const groupParam = `group_id=${encodeURIComponent(scenario.groupId)}`;
            const ticketParam = `ticket=${encodeURIComponent(usedTicket)}`;
            const rejected = await scenario.owner.evaluate(
                async ({ wsBase, groupParam, ticketParam }) =>
                    new Promise<boolean>((resolve) => {
                        let settled = false;
                        const finish = (value: boolean) => {
                            if (!settled) {
                                settled = true;
                                resolve(value);
                            }
                        };
                        const ws = new WebSocket(`${wsBase}/api/v1/ws?${groupParam}&${ticketParam}`);
                        ws.onopen = () => {
                            ws.close();
                            finish(false);
                        };
                        ws.onerror = () => finish(true);
                        ws.onclose = (event: CloseEvent) => finish(event.code !== 1000);
                        setTimeout(() => finish(true), 5000);
                    }),
                { wsBase, groupParam, ticketParam },
            );

            expect(rejected).toBe(true);
        } finally {
            await scenario.ownerContext.close();
        }
    });
});
