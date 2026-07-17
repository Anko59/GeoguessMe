import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

async function expectAccessible(page: Page): Promise<void> {
    const results = await new AxeBuilder({ page }).analyze();
    const seriousOrCritical = results.violations.filter((violation) =>
        ['serious', 'critical'].includes(violation.impact ?? ''),
    );
    expect(seriousOrCritical, JSON.stringify(seriousOrCritical, null, 2)).toEqual([]);
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
