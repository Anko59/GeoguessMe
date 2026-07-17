import { test, expect } from '@playwright/test';
import { uniqueUsername, uniqueEmail, signupViaUI } from './helpers';

test.describe('Account deletion', () => {
    test('delete account, immediate loss of access, identity can be reused', async ({ page }) => {
        const email = uniqueEmail();
        const username = uniqueUsername();
        const password = 'DeleteMe123';

        await signupViaUI(page, { username, email, password });

        await page.goto('/settings');
        await page.waitForSelector('#delete-password', { state: 'visible' });
        await page.fill('#delete-password', password);
        page.on('dialog', (dialog) => dialog.accept());
        await page.click('button:has-text("Delete account")');

        // After deletion the session is cleared → logged out.
        await page.waitForURL(/\/(login)?$/, { timeout: 15000 });

        // Old login must not work.
        await page.goto('/login');
        await page.waitForSelector('#login-username', { state: 'visible' });
        await page.fill('#login-username', username);
        await page.fill('#login-password', password);
        await page.click('button.btn-primary[type="submit"]');
        await expect(page.locator('#login-username')).toBeVisible();

        // Signing up with the same username/email succeeds (identity released).
        await signupViaUI(page, { username, email, password });
        await expect(page.locator('.groups-header')).toBeVisible();
    });
});
