import { test, expect } from '@playwright/test';
import { uniqueUsername, uniqueEmail, signupViaUI, loginViaUI } from './helpers';

test.describe('Account deletion', () => {
    test('delete account, immediate loss of access, same credentials can be reused', async ({ page }) => {
        const email = uniqueEmail();
        const username = uniqueUsername();
        const password = 'DeleteMe123';

        // Signup
        await signupViaUI(page, { username, email, password });

        // Navigate to settings
        await page.goto('/settings');

        // Wait for the delete-password input and fill it
        await page.waitForSelector('#delete-password', { state: 'visible' });
        await page.fill('#delete-password', password);

        // Dismiss the confirm dialog and proceed with deletion
        page.on('dialog', (dialog) => dialog.accept());

        // Click delete account
        await page.click('button:has-text("Delete account")');

        // After deletion, the session is refreshed → user is logged out.
        // The page should show the home page or login page.
        await page.waitForURL(/\/(login)?$/, { timeout: 15000 });

        // Try to access /groups — should redirect to /login
        await page.goto('/groups');
        await page.waitForURL(/\/login/, { timeout: 10000 });
        await expect(page.locator('#login-username')).toBeVisible();

        // Reuse the same credentials — should succeed (account was deleted)
        await loginViaUI(page, username, password);
        // After successful login, we should be at /groups
        await expect(page.locator('.groups-header')).toBeVisible({ timeout: 10000 });
    });
});
