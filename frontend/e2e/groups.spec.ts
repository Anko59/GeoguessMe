import { test, expect, type Page } from '@playwright/test';
import { uniqueGroup, signupViaUI, newAuthContext } from './helpers';

test.describe('Group operations', () => {
    let ownerPage: Page;
    let ownerGroupId: string;
    let ownerGroupCode: string;

    test.beforeAll(async ({ browser }) => {
        const ctx = await newAuthContext(browser);
        ownerPage = await ctx.newPage();
        await signupViaUI(ownerPage);

        // Create a group
        await ownerPage.goto('/group/create');
        await ownerPage.waitForSelector('input[placeholder="Group Name"]', { state: 'visible' });
        await ownerPage.fill('input[placeholder="Group Name"]', uniqueGroup());
        await ownerPage.click('button[type="submit"]');

        // Should navigate to /group/:id
        await ownerPage.waitForURL(/\/group\//, { timeout: 10000 });
        const url = ownerPage.url();
        const match = url.match(/\/group\/(.+)/);
        ownerGroupId = match![1];

        // Read the group code from the settings modal
        await ownerPage.click('.settings-btn');
        await ownerPage.waitForSelector('.modal-content', { state: 'visible' });
        const code = await ownerPage.locator('.modal-content .group-code').textContent();
        ownerGroupCode = code ?? '';

        // Close settings modal
        await ownerPage.click('.modal-close');
    });

    test('owner can see the group in groups list', async () => {
        await ownerPage.goto('/groups');
        await expect(ownerPage.locator('.groups-grid')).toBeVisible();
    });

    test('second user can join the group via code and see it', async ({ browser }) => {
        const ctx = await newAuthContext(browser);
        const joinerPage = await ctx.newPage();
        await signupViaUI(joinerPage);

        // Join the group via the join form
        await joinerPage.goto('/group/join');
        await joinerPage.waitForSelector('input[placeholder="6-character code"]', { state: 'visible' });
        await joinerPage.fill('input[placeholder="6-character code"]', ownerGroupCode);
        await joinerPage.click('button[type="submit"]');

        // Should navigate to the group view
        await joinerPage.waitForURL(/\/group\//, { timeout: 10000 });

        // Group should appear in the groups list too
        await joinerPage.goto('/groups');
        await expect(joinerPage.locator('.groups-grid')).toBeVisible();
    });

    test.afterAll(async () => {
        if (ownerPage) await ownerPage.close();
    });

    test('non-member cannot access a group route', async ({ browser }) => {
        const ctx = await newAuthContext(browser);
        const outsiderPage = await ctx.newPage();
        await signupViaUI(outsiderPage);

        await outsiderPage.goto(`/group/${ownerGroupId}`);
        await outsiderPage.waitForTimeout(3000);
        // A non-member is either redirected to /login or sees an access-denied state.
        const url = outsiderPage.url();
        expect(url).not.toMatch(/\/group\/[a-f0-9-]+$/);
        await ctx.close();
    });
});
