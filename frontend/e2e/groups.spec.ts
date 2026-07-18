import { test, expect } from './fixtures';
import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';
import { newAuthContext, signupViaUI, uniqueGroup } from './helpers';

interface OwnerScenario {
    context: BrowserContext;
    page: Page;
    groupID: string;
    groupCode: string;
}

async function createOwnerScenario(browser: Browser, contextOptions: BrowserContextOptions): Promise<OwnerScenario> {
    const context = await newAuthContext(browser, contextOptions);
    const page = await context.newPage();
    await signupViaUI(page);
    await page.goto('/group/create');
    await page.getByPlaceholder('Group Name').fill(uniqueGroup());
    await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    const groupID = page.url().split('/group/')[1];

    await page.getByRole('button', { name: 'Open group settings' }).click();
    const settings = page.getByRole('dialog');
    const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
    await settings.getByRole('button', { name: 'Close settings' }).click();
    return { context, page, groupID, groupCode };
}

test.describe('Group operations', () => {
    test('owner can see the group in groups list', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.goto('/groups');
            await expect(owner.page.locator('.groups-grid')).toBeVisible();
        } finally {
            await owner.context.close();
        }
    });

    test('second user can join the group via code and see it', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        const joinerContext = await newAuthContext(browser, contextOptions);
        try {
            const joinerPage = await joinerContext.newPage();
            await signupViaUI(joinerPage);
            await joinerPage.goto('/group/join');
            await joinerPage.getByPlaceholder('6-character code').fill(owner.groupCode);
            await joinerPage.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
            await joinerPage.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await joinerPage.goto('/groups');
            await expect(joinerPage.locator('.groups-grid')).toBeVisible();
        } finally {
            await joinerContext.close();
            await owner.context.close();
        }
    });

    test('non-member cannot access a group route', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        const outsiderContext = await newAuthContext(browser, contextOptions);
        try {
            const outsiderPage = await outsiderContext.newPage();
            await signupViaUI(outsiderPage);
            await outsiderPage.goto(`/group/${owner.groupID}`);
            await expect(outsiderPage.locator('[role="alert"]')).toContainText('You are not a member of this group');
        } finally {
            await outsiderContext.close();
            await owner.context.close();
        }
    });
});
