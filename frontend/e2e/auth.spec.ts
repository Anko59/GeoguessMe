import { test, expect } from '@playwright/test';
import { signupViaUI, loginViaUI, uniqueUsername, uniqueEmail, resetRateLimiter } from './helpers';

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
        await page.waitForURL('/');

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

test.describe('Authentication validation', () => {
    test('signup with short username shows validation error', async ({ page }) => {
        await page.goto('/signup');
        await page.waitForSelector('#signup-username', { state: 'visible' });
        await page.fill('#signup-username', 'ab');
        await page.fill('#signup-email', uniqueEmail());
        await page.fill('#signup-password', 'TestPass123');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page.locator('.auth-error')).toBeVisible();
        await expect(page).toHaveURL(/\/signup/);
    });

    test('signup with weak password shows validation error', async ({ page }) => {
        await page.goto('/signup');
        await page.waitForSelector('#signup-username', { state: 'visible' });
        await page.fill('#signup-username', uniqueUsername());
        await page.fill('#signup-email', uniqueEmail());
        await page.fill('#signup-password', 'short');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page.locator('.auth-error')).toBeVisible();
        await expect(page).toHaveURL(/\/signup/);
    });

    test('signup with password missing uppercase shows validation error', async ({ page }) => {
        await page.goto('/signup');
        await page.waitForSelector('#signup-username', { state: 'visible' });
        await page.fill('#signup-username', uniqueUsername());
        await page.fill('#signup-email', uniqueEmail());
        await page.fill('#signup-password', 'nouppercase1');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page.locator('.auth-error')).toBeVisible();
        await expect(page).toHaveURL(/\/signup/);
    });
});

test.describe('Incorrect credentials', () => {
    test('login with correct username but wrong password shows error', async ({ page }) => {
        const creds = await signupViaUI(page);

        // Logout first
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/', { timeout: 10000 });

        // Reset rate limiter so the login attempt is not throttled by the
        // preceding signup call that shared the same identity key.
        await resetRateLimiter(page);

        // Try login with correct username but wrong password
        await page.goto('/login');
        await page.waitForSelector('#login-username', { state: 'visible' });
        await page.fill('#login-username', creds.username);
        await page.fill('#login-password', 'WrongPassword123');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page.locator('.auth-error')).toBeVisible();
        await expect(page).toHaveURL(/\/login/);

        // Verify correct password still works
        await resetRateLimiter(page);
        await loginViaUI(page, creds.username, creds.password);
        await expect(page.locator('.groups-header')).toBeVisible();
    });
});

test.describe('Duplicate registration', () => {
    test('signup with taken username shows conflict error', async ({ page }) => {
        const creds = await signupViaUI(page);

        // Logout
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/', { timeout: 10000 });

        // Use the same identity (username→email) so the rate-limit bucket
        // for signup carries over. Reset before the second signup.
        await resetRateLimiter(page);

        // Try signing up with the same username, different email
        await page.goto('/signup');
        await page.waitForSelector('#signup-username', { state: 'visible' });
        await page.fill('#signup-username', creds.username);
        await page.fill('#signup-email', uniqueEmail());
        await page.fill('#signup-password', 'TestPass123');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page.locator('.auth-error')).toBeVisible();
        await expect(page).toHaveURL(/\/signup/);
    });

    test('signup with taken email shows conflict error', async ({ page }) => {
        const creds = await signupViaUI(page);

        // Logout
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/', { timeout: 10000 });

        await resetRateLimiter(page);

        // Try signing up with the same email, different username
        await page.goto('/signup');
        await page.waitForSelector('#signup-username', { state: 'visible' });
        await page.fill('#signup-username', uniqueUsername());
        await page.fill('#signup-email', creds.email);
        await page.fill('#signup-password', 'TestPass123');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page.locator('.auth-error')).toBeVisible();
        await expect(page).toHaveURL(/\/signup/);
    });
});

test.describe('Session lifecycle', () => {
    test('page reload restores authenticated session', async ({ page }) => {
        await signupViaUI(page);
        await expect(page.locator('.groups-header')).toBeVisible();

        // Reload the page — AuthProvider calls /auth/refresh on mount
        await page.reload();
        await page.waitForURL(/\/groups/, { timeout: 15000 });
        await expect(page.locator('.groups-header')).toBeVisible();
    });

    test('expired session redirects to login on page reload', async ({ page }) => {
        await signupViaUI(page);
        await expect(page.locator('.groups-header')).toBeVisible();

        // Clear the refresh cookie so session restoration fails.
        await page.context().clearCookies();

        // Reload — AuthProvider tries /auth/refresh, which fails because
        // the cookie is missing, clearing the in-memory access token.
        await page.reload();
        await page.waitForURL(/\/login/, { timeout: 15000 });
        await expect(page.locator('#login-username')).toBeVisible();

        // Protected routes remain inaccessible.
        await page.goto('/groups');
        await page.waitForURL(/\/login/, { timeout: 10000 });
        await expect(page.locator('#login-username')).toBeVisible();
    });

    test('refresh failure logs out user when cookie is tampered', async ({ page }) => {
        await signupViaUI(page);
        await expect(page.locator('.groups-header')).toBeVisible();

        // Replace the refresh_token cookie with a tampered (invalid) value
        // so the /auth/refresh endpoint rejects it as unauthorized.
        const cookies = await page.context().cookies();
        const refreshCookie = cookies.find((c) => c.name === 'refresh_token');
        if (refreshCookie) {
            await page.context().addCookies([
                {
                    ...refreshCookie,
                    value: 'tampered_invalid_token_value',
                },
            ]);
        }

        // Reload — the tampered cookie causes /auth/refresh to return 401.
        await page.reload();
        await page.waitForURL(/\/login/, { timeout: 15000 });
        await expect(page.locator('#login-username')).toBeVisible();
    });
});
