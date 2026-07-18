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

test.describe('Group validation', () => {
    test('join form shows error for a non-existent group code', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/join');
            await page.getByPlaceholder('6-character code').fill('ZZZZZZ');
            await page.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
            await expect(page.locator('.error-message')).toBeVisible();
        } finally {
            await context.close();
        }
    });

    test('join form requires a code before submission', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/join');
            const input = page.getByPlaceholder('6-character code');
            await expect(input).toHaveAttribute('required', '');
            await expect(input).toHaveAttribute('maxLength', '6');
        } finally {
            await context.close();
        }
    });

    test('create form requires a group name before submission', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/create');
            const input = page.getByPlaceholder('Group Name');
            await expect(input).toHaveAttribute('required', '');
        } finally {
            await context.close();
        }
    });

    test('join form code input uppercases on entry', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/join');
            const input = page.getByPlaceholder('6-character code');
            await input.fill('abcdef');
            await expect(input).toHaveValue('ABCDEF');
        } finally {
            await context.close();
        }
    });
});

test.describe('Group empty and error states', () => {
    test('new user sees empty state on groups list', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await expect(page.locator('.empty-state')).toBeVisible();
            await expect(page.locator('.empty-state')).toContainText("You haven't joined any groups yet");
            await expect(page.locator('.groups-grid')).not.toBeVisible();
        } finally {
            await context.close();
        }
    });

    test('groups list shows error state on fetch failure with retry button', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            // Navigate away so a subsequent /groups nav triggers a fresh fetch.
            await page.goto('/group/join');
            // Block the groups endpoint to simulate a server error.
            await page.route('**/api/v1/user/groups**', (route) => route.fulfill({ status: 500, body: '{}' }));
            await page.goto('/groups');
            await expect(page.locator('[role="alert"]')).toBeVisible();
            await expect(page.locator('[role="alert"]')).toContainText('Unable to load groups');
            await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible();
        } finally {
            await context.close();
        }
    });

    test('group view shows error when group details fetch fails', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.route('**/api/v1/group/details**', (route) => route.fulfill({ status: 500, body: '{}' }));
            await owner.page.goto(`/group/${owner.groupID}`);
            await expect(owner.page.locator('[role="alert"]')).toBeVisible();
            await expect(owner.page.locator('[role="alert"]')).toContainText('Unable to load group');
        } finally {
            await owner.context.close();
        }
    });
});

test.describe('Unauthorized access', () => {
    test('unauthenticated user is redirected away from group route', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            const anonContext = await browser.newContext(contextOptions);
            const anonPage = await anonContext.newPage();
            await anonPage.goto(`/group/${owner.groupID}`);
            await anonPage.waitForURL(/\/login/, { timeout: 10000 });
            await expect(anonPage.locator('#login-username')).toBeVisible();
            await anonContext.close();
        } finally {
            await owner.context.close();
        }
    });

    test('unauthenticated user is redirected away from groups list', async ({ browser }) => {
        const context = await browser.newContext();
        try {
            const page = await context.newPage();
            await page.goto('/groups');
            await page.waitForURL(/\/login/, { timeout: 10000 });
            await expect(page.locator('#login-username')).toBeVisible();
        } finally {
            await context.close();
        }
    });
});

test.describe('Membership changes', () => {
    test('settings modal shows owner in members list', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.getByRole('button', { name: 'Open group settings' }).click();
            const settings = owner.page.getByRole('dialog');
            const membersToggle = settings.locator('.members-toggle');
            await membersToggle.click();
            await expect(settings.locator('.members-list')).toBeVisible();
            await expect(settings.locator('.member-item')).toHaveCount(1);
            await settings.getByRole('button', { name: 'Close settings' }).click();
            await expect(settings).not.toBeVisible();
        } finally {
            await owner.context.close();
        }
    });

    test('settings modal members section can be collapsed', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.getByRole('button', { name: 'Open group settings' }).click();
            const settings = owner.page.getByRole('dialog');
            const membersToggle = settings.locator('.members-toggle');
            // Expand
            await membersToggle.click();
            await expect(settings.locator('.members-list')).toBeVisible();
            // Collapse
            await membersToggle.click();
            await expect(settings.locator('.members-list')).not.toBeVisible();
        } finally {
            await owner.context.close();
        }
    });

    test('members list updates when a second user joins', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        const joinerContext = await newAuthContext(browser, contextOptions);
        try {
            const joinerPage = await joinerContext.newPage();
            await signupViaUI(joinerPage);
            await joinerPage.goto('/group/join');
            await joinerPage.getByPlaceholder('6-character code').fill(owner.groupCode);
            await joinerPage.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
            await joinerPage.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            // Owner opens settings and expands members.
            await owner.page.goto(`/group/${owner.groupID}`);
            await owner.page.getByRole('button', { name: 'Open group settings' }).click();
            const settings = owner.page.getByRole('dialog');
            await settings.locator('.members-toggle').click();
            await expect(settings.locator('.member-item')).toHaveCount(2);
        } finally {
            await joinerContext.close();
            await owner.context.close();
        }
    });
});
