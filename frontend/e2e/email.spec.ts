import { test, expect } from '@playwright/test';
import { uniqueEmail, signupViaUI, loginViaUI, getMailpitLink } from './helpers';

test.describe('Email flows via Mailpit', () => {
    test('signup with email, verify via Mailpit link', async ({ page }) => {
        const email = uniqueEmail();
        await signupViaUI(page, { email });

        // Navigate to settings to see the pending-claim verification status.
        // F-09: a fresh signup holds a pending contact claim, not a verified email.
        await page.goto('/settings');
        await expect(page.locator('text=No verified email')).toBeVisible();
        await expect(page.locator('text=Pending verification')).toBeVisible();
        await expect(page.getByText(`Verification was requested for ${email}.`)).toBeVisible();

        // Get the verification link from Mailpit
        const verifyUrl = await getMailpitLink('Verify your GeoGuessMe email', '/verify-email');
        await page.goto(verifyUrl);

        // Should show success message
        await expect(page.locator('text=Email verified')).toBeVisible({ timeout: 10000 });

        // Navigate back to settings - the pending claim is now the verified recovery email
        await page.goto('/settings');
        await expect(page.locator('text=Verified recovery email')).toBeVisible();
        await expect(page.locator('text=Pending verification')).not.toBeVisible();
    });

    test('password reset via Mailpit link allows new login', async ({ page }) => {
        const email = uniqueEmail();
        const creds = await signupViaUI(page, { email });

        // F-09: recovery only ever acts on a VERIFIED address. Confirm the
        // email via Mailpit first so the reset mail is actually delivered.
        const verifyUrl = await getMailpitLink('Verify your GeoGuessMe email', '/verify-email');
        await page.goto(verifyUrl);
        await expect(page.locator('text=Email verified')).toBeVisible({ timeout: 10000 });

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
        const resetUrl = await getMailpitLink('Reset your GeoGuessMe password', '/reset-password');
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
