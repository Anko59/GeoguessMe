import { test, expect } from '@playwright/test';
import {
    deterministicTestImage,
    signupViaUI,
    loginViaUI,
    uniqueUsername,
    uniqueEmail,
    resetRateLimiter,
} from './helpers';

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
        await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/');

        // Must NOT be on the settings page after logout
        await expect(page).not.toHaveURL(/\/settings/);
        await expect(page.getByRole('heading', { name: /Guess the place\. Share the story\./ })).toBeVisible();
        await expect(page.getByRole('heading', { name: 'Settings' })).not.toBeVisible();

        // Navigating to /groups must redirect to /login
        await page.goto('/groups');
        await page.waitForURL(/\/login/, { timeout: 10000 });
        await expect(page.locator('#login-username')).toBeVisible();
    });

    test('invalid login credentials show error', async ({ page }) => {
        await page.goto('/login');
        const submit = page.locator('button.btn-primary[type="submit"]');
        await expect(submit).toBeEnabled();
        await page.fill('#login-username', `missing_${uniqueUsername()}`);
        const password = page.locator('#login-password');
        await password.fill('WrongPass1');
        const loginResponse = page.waitForResponse(
            (response) => response.url().endsWith('/api/v1/auth/login') && response.request().method() === 'POST',
        );
        // The valid-login scenario above covers pointer submission. Submit
        // this error-path scenario through the keyboard so Firefox cannot lose
        // a synthetic click while the full multi-browser suite is under load.
        await password.press('Enter');
        expect((await loginResponse).status()).toBe(401);

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

    test('signup with a duplicate pending email succeeds (no enumeration)', async ({ page }) => {
        const creds = await signupViaUI(page);

        // Logout
        await page.goto('/settings');
        await page.waitForSelector('.logout-btn', { state: 'visible' });
        await page.click('.logout-btn');
        await page.waitForURL('/', { timeout: 10000 });

        await resetRateLimiter(page);

        // F-09: an unverified address is only a pending contact claim, not an
        // identity, and signup must not reveal whether an address is already
        // registered. The same unverified email with a different username is
        // therefore accepted (multiple pending claims may share an address
        // until one of them verifies it).
        await page.goto('/signup');
        await page.waitForSelector('#signup-username', { state: 'visible' });
        await page.fill('#signup-username', uniqueUsername());
        await page.fill('#signup-email', creds.email);
        await page.fill('#signup-password', 'TestPass123');
        await page.click('button.btn-primary[type="submit"]');

        await expect(page).toHaveURL(/\/groups/);
        await expect(page.locator('.groups-header')).toBeVisible();
    });
});

test.describe('Session lifecycle', () => {
    test('uploads an authenticated profile photo and reports success', async ({ page }) => {
        await signupViaUI(page);
        await page.goto('/settings');

        const uploadResponse = page.waitForResponse(
            (response) =>
                response.url().endsWith('/api/v1/auth/profile/avatar') && response.request().method() === 'POST',
        );
        await page.locator('input[type="file"]').setInputFiles({
            name: 'profile.png',
            mimeType: 'image/png',
            buffer: deterministicTestImage(),
        });

        expect((await uploadResponse).status()).toBe(200);
        await expect(page.getByRole('status')).toHaveText('Profile photo updated.');
    });

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

test.describe('Local social-auth visual validation', () => {
    const username = process.env.OIDC_VISUAL_USERNAME;
    const password = process.env.OIDC_VISUAL_PASSWORD;
    const legacyUsername = process.env.LEGACY_MIGRATION_VISUAL_USERNAME;
    const legacyPassword = process.env.LEGACY_MIGRATION_VISUAL_PASSWORD;

    test.skip(!username || !password, 'requires the local dev-social Keycloak fixture');

    test('creates a social account, signs it back in, and captures the key screens', async ({ page }, testInfo) => {
        test.setTimeout(120_000);
        await page.goto('/login');
        const socialLogin = page.getByRole('link', { name: 'Continue with Google, Apple, or GitHub' });
        await expect(socialLogin).toBeVisible();
        await expect(page.locator('#login-username')).toHaveCount(0);
        await page.locator('.auth-card').evaluate(async (element) => {
            await Promise.all(element.getAnimations().map((animation) => animation.finished));
        });
        await page.screenshot({ path: testInfo.outputPath('01-app-login.png'), fullPage: true });

        await page.goto('/signup');
        const socialSignup = page.getByRole('link', { name: 'Sign up with Google, Apple, or GitHub' });
        await expect(socialSignup).toBeVisible();
        await expect(page.locator('#signup-username')).toHaveCount(0);
        await page.locator('.auth-card').evaluate(async (element) => {
            await Promise.all(element.getAnimations().map((animation) => animation.finished));
        });
        await page.screenshot({ path: testInfo.outputPath('02-app-signup.png'), fullPage: true });

        await Promise.all([
            page.waitForURL(/auth\.geoguessme\.localhost:8083/, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            socialSignup.click(),
        ]);
        await expect(page.locator('#kc-form-login')).toBeVisible();
        await expect(page.getByText('Google', { exact: true })).toBeVisible();
        await expect(page.getByText('GitHub', { exact: true })).toBeVisible();
        await expect(page.getByText('Apple', { exact: true })).toBeVisible();
        await page.screenshot({ path: testInfo.outputPath('03-keycloak-login.png'), fullPage: true });

        await page.locator('#username').fill(username ?? '');
        await page.locator('#password').fill(password ?? '');
        await Promise.all([
            page.waitForURL(/\/groups$/, { waitUntil: 'domcontentloaded', timeout: 60_000 }),
            page.locator('#kc-login').click(),
        ]);
        await expect(page.locator('.groups-header')).toBeVisible();
        const dismissInstall = page.getByRole('button', { name: 'Dismiss install prompt' });
        if (await dismissInstall.isVisible()) await dismissInstall.click();
        await page.screenshot({ path: testInfo.outputPath('04-social-session.png'), fullPage: true });

        await page.goto('/settings');
        await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
        await expect(page.getByText('Social login is connected.')).toBeVisible();
        await expect(page.getByRole('link', { name: /Manage 2FA and passkeys/ })).toBeVisible();
        await page.screenshot({ path: testInfo.outputPath('05-account-security.png'), fullPage: true });

        // A fresh cookie jar proves the identity created through the signup
        // entry point resolves to the same account on a later login.
        await page.context().clearCookies();
        await page.goto('/login');
        const returningSocialLogin = page.getByRole('link', { name: 'Continue with Google, Apple, or GitHub' });
        await expect(returningSocialLogin).toBeVisible();
        await Promise.all([
            page.waitForURL(/auth\.geoguessme\.localhost:8083/, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            returningSocialLogin.click(),
        ]);
        await page.locator('#username').fill(username ?? '');
        await page.locator('#password').fill(password ?? '');
        await Promise.all([
            page.waitForURL(/\/groups$/, { waitUntil: 'domcontentloaded', timeout: 60_000 }),
            page.locator('#kc-login').click(),
        ]);
        await expect(page.locator('.groups-header')).toBeVisible();
        await page.screenshot({ path: testInfo.outputPath('06-returning-social-login.png'), fullPage: true });
    });

    test('keeps legacy credentials hidden except for the read-only migration flow', async ({ page }, testInfo) => {
        test.skip(!legacyUsername || !legacyPassword, 'requires an unmigrated local application fixture');

        await page.goto('/login');
        await expect(page.getByRole('link', { name: 'Continue with Google, Apple, or GitHub' })).toBeVisible();
        await expect(page.locator('#login-username')).toHaveCount(0);

        await page.goto('/migrate-account');
        await expect(page.getByRole('heading', { name: 'Migrate your account' })).toBeVisible();
        await expect(page.getByRole('note')).toContainText('legacy session is read-only');
        await page.locator('#login-username').fill(legacyUsername ?? '');
        await page.locator('#login-password').fill(legacyPassword ?? '');
        await page.screenshot({ path: testInfo.outputPath('07-hidden-migration-login.png'), fullPage: true });

        await Promise.all([
            page.waitForURL(/\/settings$/, { timeout: 30_000 }),
            page.getByRole('button', { name: 'Login' }).click(),
        ]);
        await expect(page.getByText('This legacy account is read-only until Keycloak is connected.')).toBeVisible();
        await expect(page.getByRole('heading', { name: 'Finish account migration' })).toBeVisible();
        await expect(page.getByRole('button', { name: 'Connect Google, Apple, or GitHub' })).toBeVisible();
        await expect(page.getByRole('button', { name: 'Save profile' })).toHaveCount(0);
        await expect(page.getByRole('button', { name: 'Change password' })).toHaveCount(0);
        await page.screenshot({ path: testInfo.outputPath('08-read-only-migration-settings.png'), fullPage: true });
    });
});
