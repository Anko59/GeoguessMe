import { test as base, type BrowserContext, type Page } from '@playwright/test';
import { signupViaUI } from './helpers';

/**
 * Fixtures that extend the base Playwright test with authenticated
 * contexts that preserve the active project's device, viewport,
 * locale, permissions, and context options.
 */
export interface AuthFixtures {
    /** A fresh browser context inheriting the active project's context options. */
    authenticatedContext: BrowserContext;
    /** A page from an authenticated context, signed up and on /groups. */
    authenticatedPage: Page;
}

export const test = base.extend<AuthFixtures>({
    authenticatedContext: async ({ browser, contextOptions }, use) => {
        const context = await browser.newContext(contextOptions);
        await use(context);
        await context.close();
    },

    authenticatedPage: async ({ authenticatedContext }, use) => {
        const page = await authenticatedContext.newPage();
        await signupViaUI(page);
        await use(page);
        await page.close();
    },
});

export { expect } from '@playwright/test';
