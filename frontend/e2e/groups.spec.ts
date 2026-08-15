import { test, expect } from './fixtures';
import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';
import {
    createInviteFromSettings,
    joinGroupViaInvite,
    newAuthContext,
    signupViaUI,
    uniqueEmail,
    uniqueGroup,
    uniqueUsername,
} from './helpers';

interface OwnerScenario {
    context: BrowserContext;
    page: Page;
    groupID: string;
    inviteUrl: string;
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
    const inviteUrl = await createInviteFromSettings(page);
    await page.getByRole('button', { name: 'Close settings' }).click();
    return { context, page, groupID, inviteUrl };
}

test.describe('Group operations', () => {
    test('owner can see the group in groups list', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.goto('/groups');
            await expect(owner.page.locator('.groups-grid')).toBeVisible();
            // The topbar links keep their real labels on every viewport (the
            // mobile layout must not collapse both into a generic label).
            await expect(owner.page.getByRole('link', { name: 'Profile' })).toBeVisible();
            await expect(owner.page.getByRole('link', { name: 'Settings' })).toBeVisible();
            await owner.page.getByRole('link', { name: 'Settings' }).click();
            await expect(owner.page).toHaveURL(/\/settings$/);
            await expect(owner.page.getByRole('heading', { name: 'Settings' })).toBeVisible();
        } finally {
            await owner.context.close();
        }
    });

    test('owner can open the profile from a group without a full reload', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.getByRole('link', { name: 'Open your profile' }).click();
            await expect(owner.page).toHaveURL(/\/profile$/);
            await expect(owner.page.getByText('Adventurer card')).toBeVisible();
        } finally {
            await owner.context.close();
        }
    });

    test('uploaded group photo appears in groups list', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            await owner.page.getByRole('button', { name: 'Open group settings' }).click();
            const settings = owner.page.getByRole('dialog');
            const png = Buffer.from(
                'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
                'base64',
            );
            await settings.locator('input[type="file"]').setInputFiles({
                name: 'group-photo.png',
                mimeType: 'image/png',
                buffer: png,
            });
            await expect(owner.page.locator('.header-logo')).toHaveAttribute('src', /^blob:/);
            await settings.getByRole('button', { name: 'Close settings' }).click();

            await owner.page.goto('/groups');
            const card = owner.page.locator(`.group-card[data-group-id="${owner.groupID}"]`);
            await expect(card).toBeVisible();
            await expect(card.locator('.group-icon')).toHaveAttribute('src', /^blob:/);
        } finally {
            await owner.context.close();
        }
    });

    test('second user can join the group via invite link and see it', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        const joinerContext = await newAuthContext(browser, contextOptions);
        try {
            const joinerPage = await joinerContext.newPage();
            await signupViaUI(joinerPage);
            await joinGroupViaInvite(joinerPage, owner.inviteUrl, owner.groupID);
            await joinerPage.goto('/groups');
            await expect(joinerPage.locator('.groups-grid')).toBeVisible();
        } finally {
            await joinerContext.close();
            await owner.context.close();
        }
    });

    test('invite link survives signup and automatically opens the group', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        const inviteContext = await newAuthContext(browser, contextOptions);
        try {
            const invitePage = await inviteContext.newPage();
            await invitePage.goto(owner.inviteUrl);
            await expect(invitePage).toHaveURL(/\/login/);
            await invitePage.getByRole('link', { name: 'Sign up' }).click();
            await invitePage.fill('#signup-username', uniqueUsername());
            await invitePage.fill('#signup-email', uniqueEmail());
            await invitePage.fill('#signup-password', 'TestPass123');
            await invitePage.locator('button.btn-primary[type="submit"]').click();
            // The token survives in sessionStorage; the preview resolves and the
            // join button appears before the group page is reached.
            await expect(invitePage.getByTestId('join-btn')).toBeVisible({ timeout: 10000 });
            await invitePage.getByTestId('join-btn').click();
            await invitePage.waitForURL(new RegExp(`/group/${owner.groupID}$`));
        } finally {
            await inviteContext.close();
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
            await expect(outsiderPage.locator('[role="alert"]')).toContainText('You do not have access to this group.');
        } finally {
            await outsiderContext.close();
            await owner.context.close();
        }
    });
});

test.describe('Group validation', () => {
    test('join page shows an invalid state for a non-existent invite token', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/join#invite=definitely-not-a-real-token');
            await expect(page.locator('[role="alert"]')).toContainText('invalid or has expired');
        } finally {
            await context.close();
        }
    });

    test('join page shows a no-invite state when no token is present', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/join');
            await expect(page.locator('[role="alert"]')).toContainText('No invite link found');
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

    test('a revoked invite link shows an invalid state', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        const context = await newAuthContext(browser, contextOptions);
        try {
            // The owner revokes the invite they created.
            await owner.page.getByRole('button', { name: 'Open group settings' }).click();
            const settings = owner.page.getByRole('dialog');
            await settings
                .locator('.invites-list .invite-item')
                .first()
                .getByRole('button', { name: 'Revoke' })
                .click();
            await expect(settings.locator('.invite-state-label.revoked')).toContainText('Revoked');
            await settings.getByRole('button', { name: 'Close settings' }).click();

            // A joiner opening the revoked invite sees the generic invalid state.
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto(owner.inviteUrl);
            await expect(page.locator('[role="alert"]')).toContainText('invalid or has expired');
        } finally {
            await context.close();
            await owner.context.close();
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
            await expect(settings.getByRole('link', { name: 'Personal settings' })).toHaveAttribute(
                'href',
                '/settings',
            );
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

    test('owner reaches their own profile from the group header', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            const profileLink = owner.page.getByRole('link', { name: 'Open your profile' });
            await expect(profileLink).toBeVisible();
            await profileLink.click();
            await expect(owner.page).toHaveURL(/\/profile$/);
            await expect(owner.page.getByRole('heading', { level: 1 })).toBeVisible();
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
            await joinGroupViaInvite(joinerPage, owner.inviteUrl, owner.groupID);

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
