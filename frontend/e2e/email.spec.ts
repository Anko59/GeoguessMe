import { test, expect } from '@playwright/test';
import { uniqueEmail, signupViaUI, getMailpitLink } from './helpers';

test.describe('Email flows via Mailpit', () => {
    test('signup with email, verify via Mailpit link', async ({ page }) => {
        const email = uniqueEmail();
        await signupViaUI(page, { email });

        // Navigate to settings to see verification status
        await page.goto('/settings');
        await expect(page.locator('text=Email not verified')).toBeVisible();

        // Get the verification link from Mailpit
        const verifyUrl = await getMailpitLink(email, '/verify-email');
        await page.goto(verifyUrl);

        // Should show success message
        await expect(page.locator('text=Email verified')).toBeVisible({ timeout: 10000 });

        // Navigate back to settings - should now show verified
        await page.goto('/settings');
        await expect(page.locator('text=Email verified')).toBeVisible();
    });

    test('password reset via Mailpit link allows new login', async ({ page }) => {
        const email = uniqueEmail();
        const creds = await signupViaUI(page, { email });

        // Logout
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/', { timeout: 10000 });

        // Request password reset
        await page.goto('/forgot-password');
        await page.fill('#forgot-email', email);
        await page.click('button.btn-primary[type="submit"]');

        // Wait for success message
        await expect(page.locator('.auth-success')).toBeVisible({ timeout: 10000 });

        // Get the reset link from Mailpit
        const resetUrl = await getMailpitLink(email, '/reset-password');
        await page.goto(resetUrl);

        // Set new password
        const newPassword = 'NewPass456';
        await page.waitForSelector('#reset-password', { state: 'visible' });
        await page.fill('#reset-password', newPassword);
        await page.click('button.btn-primary[type="submit"]');

        // Should show success message then redirect to /login
        await expect(page.locator('text=Password reset')).toBeVisible({ timeout: 5000 });
        await page.waitForURL(/\/login/, { timeout: 15000 });

        // Login with new password
        await loginViaUI(page, creds.username, newPassword);
        await expect(page.locator('.groups-header')).toBeVisible();
    });
});
