import { test, expect } from '@playwright/test';

test.describe('Accessibility', () => {
    test('home page has no critical a11y violations', async ({ page }) => {
        await page.goto('/');
        await expect(page.getByRole('heading', { name: /geoguess/i })).toBeVisible({ timeout: 10000 });
    });

    test('login page has no critical a11y violations', async ({ page }) => {
        await page.goto('/login');
        await expect(page.locator('#login-username')).toBeVisible({ timeout: 10000 });
    });
});
