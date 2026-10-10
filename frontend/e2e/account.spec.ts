import { test, expect } from '@playwright/test';
import {
    uniqueUsername,
    uniqueEmail,
    signupViaUI,
    signupWithToken,
    newAuthContext,
    uniqueGroup,
    expectConnected,
} from './support/helpers';

test('profile blocking invalidates tabs and peer chat cache; Settings restores access', async ({
    browser,
    context,
}) => {
    const other = await newAuthContext(browser);
    try {
        // Keep the HTTP-origin regression even when CI serves a secure origin:
        // randomUUID is secure-context-only, unlike getRandomValues and storage.
        await context.addInitScript(() => {
            Object.defineProperty(crypto, 'randomUUID', { configurable: true, value: undefined });
        });
        const first = await signupWithToken(context);
        const second = await signupWithToken(other);
        const firstHeaders = { Authorization: `Bearer ${first.token}` };
        const secondHeaders = { Authorization: `Bearer ${second.token}` };
        const secondProfile = await other.request.get('/api/v1/auth/profile', { headers: secondHeaders });
        expect(secondProfile.ok()).toBeTruthy();
        const player = (await secondProfile.json()) as { id: string; username: string };
        const create = await context.request.post('/api/v1/group/create', {
            headers: firstHeaders,
            data: { name: uniqueGroup() },
        });
        expect(create.ok()).toBeTruthy();
        const group = (await create.json()) as { id: string };
        const invite = await context.request.post('/api/v1/group/invites', {
            headers: firstHeaders,
            data: { group_id: group.id },
        });
        expect(invite.ok()).toBeTruthy();
        const { token } = (await invite.json()) as { token: string };
        const join = await other.request.post('/api/v1/group/join', {
            headers: secondHeaders,
            data: { invite_token: token },
        });
        expect(join.ok()).toBeTruthy();
        await first.page.goto(`/group/${group.id}`);
        await second.page.goto(`/group/${group.id}`);
        await expectConnected(first.page);
        await expectConnected(second.page);
        const retained = `Member retained ${uniqueUsername()}`;
        await second.page.locator('#chat-message').fill(retained);
        await second.page.getByRole('button', { name: 'Send message' }).click();
        await expect(first.page.getByText(retained, { exact: true })).toBeVisible();
        const hidden = `Owner live ${uniqueUsername()}`;
        await first.page.locator('#chat-message').fill(hidden);
        await first.page.getByRole('button', { name: 'Send message' }).click();
        await expect(second.page.getByText(hidden, { exact: true })).toBeVisible();
        await expect
            .poll(() =>
                second.page.evaluate(() =>
                    Object.keys(localStorage)
                        .filter((key) => key.startsWith('geoguessme:pwa-messages:'))
                        .map((key) => localStorage.getItem(key))
                        .join(''),
                ),
            )
            .toContain(hidden);
        await first.page.goto(`/profile/${player.id}`);
        const tab = await context.newPage();
        await tab.goto(`/profile/${player.id}`);
        await expect(tab.getByRole('heading', { name: player.username, exact: true })).toBeVisible();
        const block = first.page.getByRole('button', { name: 'Block player', exact: true });
        await expect(block).toBeEnabled();
        first.page.once('dialog', (dialog) => dialog.accept());
        await block.click();
        await expect(first.page.getByText('Player blocked', { exact: true })).toBeVisible();
        await expect(tab.getByText('Player blocked', { exact: true })).toBeVisible();
        await expect(tab.getByRole('heading', { name: player.username, exact: true })).toHaveCount(0);
        await second.page.reload();
        await expectConnected(second.page);
        await expect(second.page.getByText(retained, { exact: true })).toBeVisible();
        await expect(second.page.getByText(hidden, { exact: true })).toHaveCount(0);
        await second.page.goto('/groups');
        await second.page.goto(`/group/${group.id}`);
        await expectConnected(second.page);
        await expect(second.page.getByText(hidden, { exact: true })).toHaveCount(0);

        const members = await context.request.get(`/api/v1/group/members?id=${group.id}`, { headers: firstHeaders });
        expect(members.ok()).toBeTruthy();
        expect(await members.json()).toEqual(expect.arrayContaining([expect.objectContaining({ id: player.id })]));
        await first.page.goto('/settings');
        await first.page.getByRole('button', { name: `Unblock ${player.username}`, exact: true }).click();
        await expect(first.page.getByText('No blocked users.', { exact: true })).toBeVisible();
        await expect(tab.getByRole('heading', { name: player.username, exact: true })).toBeVisible();
    } finally {
        await other.close();
    }
});

test.describe('Account deletion', () => {
    test('delete account, immediate loss of access, identity can be reused', async ({ page }) => {
        const email = uniqueEmail();
        const username = uniqueUsername();
        const password = 'DeleteMe123';

        await signupViaUI(page, { username, email, password });

        await page.goto('/settings');
        await page.waitForSelector('#delete-password', { state: 'visible' });
        await page.fill('#delete-password', password);
        page.on('dialog', (dialog) => dialog.accept());
        await page.click('button:has-text("Delete account")');

        // After deletion the session is cleared → logged out.
        await page.waitForURL(/\/(login)?$/, { timeout: 15000 });

        // Old login must not work.
        await page.goto('/login');
        await page.waitForSelector('#login-username', { state: 'visible' });
        await page.fill('#login-username', username);
        await page.fill('#login-password', password);
        await page.click('button.btn-primary[type="submit"]');
        await expect(page.locator('#login-username')).toBeVisible();

        // Signing up with the same username/email succeeds (identity released).
        await signupViaUI(page, { username, email, password });
        await expect(page.locator('.groups-header')).toBeVisible();
    });
});
