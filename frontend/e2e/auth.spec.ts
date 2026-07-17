import { test, expect } from '@playwright/test';
import { signupViaUI, loginViaUI } from './helpers';

test.describe('Authentication', () => {
    test('signup creates account and redirects to groups', async ({ page }) => {
        await signupViaUI(page);
        // Should be on /groups page now
        await expect(page.locator('.groups-header')).toBeVisible();
    });

    test('login with valid credentials redirects to groups', async ({ page }) => {
        const creds = await signupViaUI(page);

        // Logout first
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/', { timeout: 10000 });

        // Now login with the same credentials
        await loginViaUI(page, creds.username, creds.password);
        await expect(page.locator('.groups-header')).toBeVisible();
    });

    test('logout clears session and protects /groups', async ({ page }) => {
        await signupViaUI(page);

        // Navigate to settings and click logout
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForLoadState('networkidle');
        await page.waitForTimeout(2000);

        // Must NOT be on the settings page after logout
        await expect(page).not.toHaveURL(/\/settings/);

        // Navigating to /groups must redirect to /login
        await page.goto('/groups');
        await page.waitForURL(/\/login/, { timeout: 10000 });
        await expect(page.locator('#login-username')).toBeVisible();
    });

    test('invalid login credentials show error', async ({ page }) => {
        await page.goto('/login');
        await page.fill('#login-username', 'nonexistent_user');
        await page.fill('#login-password', 'WrongPass1');
        await page.click('button.btn-primary[type="submit"]');

        // Wait for error message
        await expect(page.locator('.auth-error')).toBeVisible();
        // Should stay on /login
        await expect(page).toHaveURL(/\/login/);
    });
});
