import { test, expect, type Page } from '@playwright/test';
import { uniqueGroup, signupViaUI, newAuthContext } from './helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Chat via WebSocket', () => {
    let userAPage: Page;
    let userBPage: Page;
    let groupId: string;
    let groupCode: string;

    test.beforeAll(async ({ browser }) => {
        // User A: signup, create group
        const ctxA = await newAuthContext(browser);
        userAPage = await ctxA.newPage();
        await signupViaUI(userAPage);

        await userAPage.goto('/group/create');
        await userAPage.fill('input[placeholder="Group Name"]', uniqueGroup());
        await userAPage.click('button[type="submit"]');
        await userAPage.waitForURL(/\/group\//, { timeout: 10000 });
        const match = userAPage.url().match(/\/group\/(.+)/);
        groupId = match![1];

        // Read group code from settings
        await userAPage.click('.settings-btn');
        await userAPage.waitForSelector('.modal-content', { state: 'visible' });
        groupCode = (await userAPage.locator('.modal-content .group-code').textContent()) ?? '';
        await userAPage.click('.modal-close');
    });

    test('chat connect, send message, receive in real-time', async ({ browser }) => {
        // User B: signup and join
        const ctxB = await newAuthContext(browser);
        userBPage = await ctxB.newPage();
        await signupViaUI(userBPage);

        await userBPage.goto('/group/join');
        await userBPage.fill('input[placeholder="6-character code"]', groupCode);
        await userBPage.click('button[type="submit"]');
        await userBPage.waitForURL(/\/group\//, { timeout: 10000 });

        // Both users are now on the group view
        await userAPage.goto(`/group/${groupId}`);
        await userBPage.goto(`/group/${groupId}`);

        // Wait for WebSocket connection
        await expect(userAPage.locator('.chat-status')).toBeVisible({ timeout: 15000 });
        await expect(userBPage.locator('.chat-status')).toBeVisible({ timeout: 15000 });

        // User A sends a message
        const msgText = `Hello from A at ${Date.now()}`;
        await userAPage.fill('#chat-message', msgText);
        await userAPage.click('button[type="submit"]');

        // Verify User A sees the message (sent optimistically via WS)
        await expect(userAPage.locator('.message-container').last()).toContainText(msgText, { timeout: 5000 });

        // Verify User B receives the message
        await expect(userBPage.locator('.message-container').last()).toContainText(msgText, { timeout: 5000 });
    });

    test('reload restores message history', async () => {
        // Reload User B's page
        await userBPage.reload();
        await userBPage.waitForURL(/\/group\//, { timeout: 15000 });
        await expect(userBPage.locator('.chat-status')).toBeVisible({ timeout: 15000 });

        // The previously sent message should appear (loaded from REST API history)
        await expect(userBPage.locator('.message-container').last()).toContainText('Hello from A at', { timeout: 10000 });
    });

    test.afterAll(async () => {
        if (userAPage) await userAPage.close();
        if (userBPage) await userBPage.close();
    });

    test('one-time WS ticket reuse is rejected', async () => {
        // Intercept the WS ticket endpoint to capture the ticket value
        let usedTicket = '';
        await userAPage.route('**/ws/ticket', async (route) => {
            const response = await route.fetch();
            const body = await response.json() as { ticket: string };
            usedTicket = body.ticket;
            await route.fulfill({ response });
        });

        // Reload to get a fresh connection with ticket interception
        await userAPage.reload();
        await userAPage.waitForURL(/\/group\//, { timeout: 15000 });
        await expect(userAPage.locator('.chat-status')).toBeVisible({ timeout: 15000 });

        // Now the ticket has been consumed by the WebSocket above.
        // Attempt to open a new WebSocket with the same ticket.
        const baseUrl = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
        const wsBase = baseUrl.replace(/^http/, 'ws');
        const groupParam = `group_id=${encodeURIComponent(groupId)}`;
        const ticketParam = `ticket=${encodeURIComponent(usedTicket)}`;

        const rejected = await userAPage.evaluate(async ({ wsBase, groupParam, ticketParam }) => {
            return new Promise<boolean>((resolve) => {
                const ws = new WebSocket(`${wsBase}/api/v1/ws?${groupParam}&${ticketParam}`);
                ws.onopen = () => { ws.close(); resolve(false); };
                ws.onerror = () => resolve(true);
                ws.onclose = (e: CloseEvent) => {
                    // Reconnecting with a consumed ticket → should be closed/refused
                    if (e.code !== 1000) resolve(true);
                    else resolve(false);
                };
                // Timeout after 5s
                setTimeout(() => resolve(true), 5000);
            });
        }, { wsBase, groupParam, ticketParam });

        expect(rejected).toBe(true);
    });
});
