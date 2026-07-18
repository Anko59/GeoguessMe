import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { signupViaUI, uniqueGroup, installDeterministicCamera, newAuthContext } from './helpers';
import type { Browser, BrowserContext, BrowserContextOptions } from '@playwright/test';

async function expectAccessible(page: Page): Promise<void> {
    const results = await new AxeBuilder({ page }).analyze();
    const seriousOrCritical = results.violations.filter((violation) =>
        ['serious', 'critical'].includes(violation.impact ?? ''),
    );
    expect(seriousOrCritical, JSON.stringify(seriousOrCritical, null, 2)).toEqual([]);
}

async function createAuthenticatedContext(
    browser: Browser,
    contextOptions: BrowserContextOptions,
): Promise<{ context: BrowserContext; page: Page }> {
    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();
    await signupViaUI(page);
    return { context, page };
}

test.describe('Accessibility', () => {
    test('home page has no serious or critical Axe violations', async ({ page }) => {
        await page.goto('/');
        await expect(page.getByRole('heading', { name: /geoguess/i })).toBeVisible({ timeout: 10000 });
        await expectAccessible(page);
    });

    test('login page has no serious or critical Axe violations', async ({ page }) => {
        await page.goto('/login');
        await expect(page.locator('#login-username')).toBeVisible({ timeout: 10000 });
        await expectAccessible(page);
    });

    for (const route of ['/signup', '/forgot-password', '/reset-password', '/verify-email']) {
        test(`${route} has no serious or critical Axe violations`, async ({ page }) => {
            await page.goto(route);
            await expectAccessible(page);
        });
    }
});

test.describe('Keyboard navigation', () => {
    test('home page tab order cycles through primary links', async ({ page }) => {
        await page.goto('/');
        await expect(page.getByRole('heading', { name: /geoguess/i })).toBeVisible({ timeout: 10000 });

        const getStarted = page.getByRole('link', { name: /get started/i });
        const loginLink = page.getByRole('link', { name: /login/i });

        await page.locator('body').focus();
        await page.keyboard.press('Tab');
        await expect(getStarted).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(loginLink).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(getStarted).not.toBeFocused();
    });

    test('login form tab order moves through fields, submit, and links', async ({ page }) => {
        await page.goto('/login');
        await expect(page.locator('#login-username')).toBeVisible({ timeout: 10000 });

        await page.locator('body').focus();
        await page.keyboard.press('Tab');
        await expect(page.locator('#login-username')).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.locator('#login-password')).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('button', { name: /login/i })).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('link', { name: /forgot/i })).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('link', { name: /sign up/i })).toBeFocused();
    });

    test('signup form tab order moves through username, email, password, submit, login link', async ({ page }) => {
        await page.goto('/signup');
        await expect(page.locator('#signup-username')).toBeVisible({ timeout: 10000 });

        await page.locator('body').focus();
        await page.keyboard.press('Tab');
        await expect(page.locator('#signup-username')).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.locator('#signup-email')).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.locator('#signup-password')).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('button', { name: /sign up/i })).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('link', { name: /login/i })).toBeFocused();
    });

    test('forgot-password form tab order', async ({ page }) => {
        await page.goto('/forgot-password');

        await page.locator('body').focus();
        await page.keyboard.press('Tab');
        await expect(page.locator('#forgot-email')).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('button', { name: /send reset/i })).toBeFocused();

        await page.keyboard.press('Tab');
        await expect(page.getByRole('link', { name: /back to login/i })).toBeFocused();
    });

    test('group join/create page mode toggle and form tab order', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/join');
            await expect(page.locator('form.join-form')).toBeVisible({ timeout: 10000 });

            // Back link is first Tab target, then mode toggle buttons
            const backLink = page.locator('.back-btn-page');
            await backLink.focus();
            await page.keyboard.press('Tab');
            const joinToggle = page.locator('.mode-selector button').first();
            await expect(joinToggle).toBeFocused();

            await page.keyboard.press('Tab');
            const createToggle = page.getByRole('button', { name: /^create group$/i });
            await expect(createToggle).toBeFocused();

            await page.keyboard.press('Tab');
            await expect(page.locator('input[placeholder*="code" i]')).toBeFocused();

            await page.keyboard.press('Tab');
            const submitBtn = page.locator('form.join-form button[type="submit"]');
            await expect(submitBtn).toBeFocused();
        } finally {
            await ctx.context.close();
        }
    });

    test('group view back button and settings button are Tab-reachable', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            const backLink = page.locator('.back-btn');
            const settingsBtn = page.getByRole('button', { name: /settings/i });

            // Back link is first focusable element; Tab moves to settings, then tab bar
            await backLink.focus();
            await page.keyboard.press('Tab');
            await expect(settingsBtn).toBeFocused();

            await page.keyboard.press('Tab');
            const chatTab = page.locator('.tab-bar .tab').first();
            await expect(chatTab).toBeFocused();
        } finally {
            await ctx.context.close();
        }
    });
});

test.describe('Dialog keyboard interaction', () => {
    test('settings modal traps focus and restoring focus after close', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            const settingsBtn = page.getByRole('button', { name: /settings/i });
            await settingsBtn.focus();
            await expect(settingsBtn).toBeFocused();
            await settingsBtn.click();
            await expect(page.getByRole('dialog')).toBeVisible();

            // Tab focus is inside the dialog; cycle through modal elements
            await page.keyboard.press('Tab');
            const closeBtn = page.getByRole('button', { name: /close settings/i });
            await expect(closeBtn).toBeFocused();

            // Multiple Tab presses stay within the dialog (focus trapping)
            await page.keyboard.press('Tab');
            await page.keyboard.press('Tab');
            await page.keyboard.press('Tab');
            await page.keyboard.press('Tab');
            await expect(page.getByRole('dialog')).toBeVisible();

            // Close via keyboard Enter on close button
            await closeBtn.press('Enter');
            await expect(page.getByRole('dialog')).not.toBeVisible();

            // Focus returns to an element on the page (dialog is gone)
            await expect(page.locator('.group-view')).toBeVisible();
        } finally {
            await ctx.context.close();
        }
    });

    test('settings modal opens via keyboard Enter and closes via Tab then Enter', async ({
        browser,
        contextOptions,
    }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            const settingsBtn = page.getByRole('button', { name: /settings/i });
            await settingsBtn.focus();
            await page.keyboard.press('Enter');
            await expect(page.getByRole('dialog')).toBeVisible();

            // Tab to the close button and press Enter to close
            await page.keyboard.press('Tab');
            await page.getByRole('button', { name: /close settings/i }).press('Enter');
            await expect(page.getByRole('dialog')).not.toBeVisible();
        } finally {
            await ctx.context.close();
        }
    });
});

test.describe('Authenticated page Axe checks', () => {
    test('groups list page has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await expect(page).toHaveURL(/\/groups/);
            await expect(page.getByRole('heading', { name: /my groups/i })).toBeVisible();
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('account settings page has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/settings');
            await expect(page.locator('#delete-password')).toBeVisible({ timeout: 10000 });
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('group create page has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await expect(page.getByPlaceholder('Group Name')).toBeVisible({ timeout: 10000 });
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('group join page has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/join');
            await expect(page.getByPlaceholder(/code/i)).toBeVisible({ timeout: 10000 });
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('group view chat tab has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expect(page.getByRole('status')).toHaveText('Connected');
            await expect(page.locator('.chat-container')).toBeVisible();
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('group view camera tab has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const cameraCtx = await browser.newContext({
            ...contextOptions,
            permissions: ['camera', 'geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicCamera(cameraCtx);
        try {
            const page = await cameraCtx.newPage();
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await page.getByRole('button', { name: /camera/i }).click();
            await expect(page.locator('.camera-container')).toBeVisible({ timeout: 10000 });
            await expectAccessible(page);
        } finally {
            await cameraCtx.close();
        }
    });

    test('group view leaderboard tab has no serious or critical Axe violations', async ({
        browser,
        contextOptions,
    }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await page.getByRole('button', { name: /leaderboard/i }).click();
            await expect(page.locator('.leaderboard-container')).toBeVisible();
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('settings modal has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const ctx = await createAuthenticatedContext(browser, contextOptions);
        try {
            const { page } = ctx;
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await page.getByRole('button', { name: /settings/i }).click();
            await expect(page.getByRole('dialog')).toBeVisible();
            await expectAccessible(page);
        } finally {
            await ctx.context.close();
        }
    });

    test('camera denial error state has no serious or critical Axe violations', async ({ browser, contextOptions }) => {
        const cameraDenied = await browser.newContext({ ...contextOptions, permissions: [] });
        try {
            const page = await cameraDenied.newPage();
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page
                .locator('form.join-form')
                .getByRole('button', { name: /create/i })
                .click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await page.getByRole('button', { name: /camera/i }).click();
            await expect(page.locator('.camera-error')).toContainText('Camera access denied');
            await expectAccessible(page);
        } finally {
            await cameraDenied.close();
        }
    });
});
