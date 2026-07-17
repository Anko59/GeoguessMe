import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { signupViaUI, uniqueGroup, newAuthContext } from './helpers';

test.describe('Accessibility', () => {
    test('home page has no critical a11y violations', async ({ page }) => {
        await page.goto('/');
        await page.waitForSelector('.home-container', { state: 'visible' });

        const results = await new AxeBuilder({ page }).analyze();
        // Filter out only critical and serious violations
        const serious = results.violations.filter(
            (v) => v.impact === 'critical' || v.impact === 'serious',
        );
        expect(serious).toEqual([]);
    });

    test('login page has no critical a11y violations', async ({ page }) => {
        await page.goto('/login');
        await page.waitForSelector('#login-username', { state: 'visible' });

        const results = await new AxeBuilder({ page }).analyze();
        const serious = results.violations.filter(
            (v) => v.impact === 'critical' || v.impact === 'serious',
        );
        expect(serious).toEqual([]);
    });

    test('authenticated group view has no critical a11y violations', async ({ browser }) => {
        const ctx = await newAuthContext(browser);
        const page = await ctx.newPage();
        await signupViaUI(page);

        // Create a group first
        await page.goto('/group/create');
        await page.fill('input[placeholder="Group Name"]', uniqueGroup());
        await page.click('button[type="submit"]');
        await page.waitForURL(/\/group\//, { timeout: 10000 });

        // Wait for group view to fully load with chat
        await expect(page.locator('.chat-status')).toHaveText('Connected', { timeout: 10000 });

        const results = await new AxeBuilder({ page }).analyze();
        const serious = results.violations.filter(
            (v) => v.impact === 'critical' || v.impact === 'serious',
        );
        expect(serious).toEqual([]);
    });
});
