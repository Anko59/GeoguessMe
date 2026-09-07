import { test, expect } from './support/fixtures';
import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';
import { newAuthContext, signupViaUI, uniqueGroup } from './support/helpers';

interface OwnerScenario {
    context: BrowserContext;
    page: Page;
}

async function createOwnerScenario(browser: Browser, contextOptions: BrowserContextOptions): Promise<OwnerScenario> {
    const context = await newAuthContext(browser, contextOptions);
    const page = await context.newPage();
    await signupViaUI(page);
    await page.goto('/group/create');
    await page.getByPlaceholder('Group Name').fill(uniqueGroup());
    await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    return { context, page };
}

test.describe('Party Time', () => {
    test('a member starts a party and sees the announcement and the neon border', async ({
        browser,
        contextOptions,
    }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            const partyButton = owner.page.getByRole('button', { name: 'Start party time' });
            await expect(partyButton).toBeEnabled();

            // The start is deliberate: a confirmation dialog guards the
            // group-wide action.
            owner.page.once('dialog', (dialog) => void dialog.accept());
            await partyButton.click();

            // The persisted system message announces the party in chat.
            await expect(owner.page.getByText(/started Party Time!/).first()).toBeVisible();

            // While the party runs the button is disabled and the neon
            // border frames the screen.
            await expect(owner.page.getByRole('button', { name: /Party time is active/ })).toBeDisabled();
            await expect(owner.page.locator('.party-border')).toHaveCount(1);
        } finally {
            await owner.context.close();
        }
    });

    test('the party state survives a reload while it is active', async ({ browser, contextOptions }) => {
        const owner = await createOwnerScenario(browser, contextOptions);
        try {
            const partyButton = owner.page.getByRole('button', { name: 'Start party time' });
            await expect(partyButton).toBeEnabled();
            owner.page.once('dialog', (dialog) => void dialog.accept());
            await partyButton.click();
            await expect(owner.page.locator('.party-border')).toHaveCount(1);

            await owner.page.reload();
            await expect(owner.page.locator('.party-border')).toHaveCount(1);
            await expect(owner.page.getByRole('button', { name: /Party time is active/ })).toBeDisabled();
        } finally {
            await owner.context.close();
        }
    });
});
