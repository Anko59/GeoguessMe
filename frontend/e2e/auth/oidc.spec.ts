import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { waitForKeycloakVerificationLink } from '../support/helpers';

const keycloakURL = process.env.KEYCLOAK_ADMIN_URL || 'https://auth-dev.geoguessme.com';

async function keycloakAdminToken(request: APIRequestContext): Promise<string> {
    const response = await request.post(`${keycloakURL}/realms/master/protocol/openid-connect/token`, {
        form: {
            grant_type: 'password',
            client_id: 'admin-cli',
            username: process.env.KEYCLOAK_ADMIN_USERNAME || 'local-admin',
            password: process.env.KEYCLOAK_ADMIN_PASSWORD || 'local-admin-password',
        },
    });
    expect(response.ok()).toBeTruthy();
    const body = (await response.json()) as { access_token: string };
    return body.access_token;
}

async function keycloakUsersByEmail(
    request: APIRequestContext,
    token: string,
    email: string,
): Promise<Array<{ id: string }>> {
    const response = await request.get(`${keycloakURL}/admin/realms/geoguessme/users`, {
        headers: { Authorization: `Bearer ${token}` },
        params: { email, exact: 'true' },
    });
    expect(response.ok()).toBeTruthy();
    return (await response.json()) as Array<{ id: string }>;
}

async function waitForAuthCardVisuals(page: Page): Promise<void> {
    await page.locator('.auth-card').evaluate(async (element) => {
        await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
        await Promise.all(element.getAnimations().map((animation) => animation.finished));
        await document.fonts.ready;
        await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
    });
}

async function expectLightKeycloakTheme(
    page: Page,
    options: { control?: boolean; checkbox?: boolean; alert?: boolean } = {},
): Promise<void> {
    const palette = await page.evaluate(() => {
        document.documentElement.classList.add('pf-v5-theme-dark');
        const formControl = document.querySelector<HTMLElement>('.pf-v5-c-form-control');
        const input = document.querySelector<HTMLInputElement>('input');
        const checkboxLabel = document.querySelector<HTMLElement>('.pf-v5-c-check__label');
        const alert = document.querySelector<HTMLElement>('.pf-v5-c-alert');
        const style = (element: Element | null): CSSStyleDeclaration | null =>
            element ? getComputedStyle(element) : null;

        return {
            colorScheme: getComputedStyle(document.documentElement).colorScheme,
            formControlBackground: style(formControl)?.backgroundColor,
            inputColor: style(input)?.color,
            checkboxColor: style(checkboxLabel)?.color,
            alertBackground: style(alert)?.backgroundColor,
            alertColor: style(alert)?.color,
        };
    });

    expect(palette.colorScheme).toBe('light');
    if (options.control !== false) {
        expect(palette.formControlBackground).toBe('rgb(255, 255, 255)');
        expect(palette.inputColor).toBe('rgb(17, 26, 77)');
    }
    if (options.checkbox) expect(palette.checkboxColor).toBe('rgb(17, 26, 77)');
    if (options.alert) {
        expect(palette.alertBackground).toBe('rgb(255, 248, 229)');
        expect(palette.alertColor).toBe('rgb(111, 77, 0)');
    }
}

async function completeKeycloakEmailRegistration(
    page: Page,
    request: APIRequestContext,
    email: string,
    password: string,
): Promise<void> {
    await expect(page.locator('#kc-register-form')).toBeVisible();
    await expect(page.locator('#email')).toHaveValue(email);
    await expect(page.locator('#password')).toHaveCount(0);
    await page.getByRole('button', { name: 'Create account' }).click();
    await expect(page.locator('#kc-passwd-update-form')).toBeVisible();
    await page.locator('#password-new').fill(password);
    await page.locator('#password-confirm').fill(password);
    await expectLightKeycloakTheme(page, { checkbox: true });
    await page.getByRole('button', { name: 'Submit' }).click();
    await expect(
        page.getByText('You need to verify your email address to activate your account.', { exact: true }),
    ).toBeVisible();
    await expectLightKeycloakTheme(page, { control: false, alert: true });

    const verificationLink = await waitForKeycloakVerificationLink(request, email);
    await page.goto(verificationLink, { waitUntil: 'domcontentloaded' });
}

test.describe('Local social-auth visual validation', () => {
    const username = process.env.OIDC_VISUAL_USERNAME;
    const password = process.env.OIDC_VISUAL_PASSWORD;
    const legacyUsername = process.env.LEGACY_MIGRATION_VISUAL_USERNAME;
    const legacyPassword = process.env.LEGACY_MIGRATION_VISUAL_PASSWORD;

    test.skip(!username || !password, 'requires the local dev-social Keycloak fixture');

    test('starts the configured Google identity-provider flow', async ({ page }) => {
        await page.goto('/login');
        const googleLogin = page.getByRole('link', { name: 'Continue with Google' });
        await expect(googleLogin).toBeVisible();
        await Promise.all([
            page.waitForURL(/^https:\/\/accounts\.google\.com\//, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            googleLogin.click(),
        ]);
        await expect(page).not.toHaveURL(/\/error(?:[/?#]|$)/);
        await expect(page.locator('body')).not.toContainText(/invalid_client|redirect_uri_mismatch/i);
    });

    test('creates, names, and fully deletes a Keycloak email account', async ({ page, request }, testInfo) => {
        test.setTimeout(120_000);
        const email = `oidc-signup-${Date.now()}@example.com`;
        const playerName = `map-player-${Date.now()}`;

        await page.goto('/signup');
        await page.getByPlaceholder('you@example.com').fill(email);
        await Promise.all([
            page.waitForURL(/https:\/\/auth-dev\.geoguessme\.com\//, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            page.getByRole('button', { name: 'Continue to create account' }).click(),
        ]);
        await completeKeycloakEmailRegistration(page, request, email, 'TestPass123!');
        await page.waitForURL(/https:\/\/geoguessme\.localhost\/auth\/oidc\/callback$/, {
            waitUntil: 'domcontentloaded',
            timeout: 60_000,
        });
        await expect(page.getByRole('heading', { name: 'Choose your username' })).toBeVisible();
        await expect(page.locator('#oidc-username')).toHaveValue('');
        await page.screenshot({ path: testInfo.outputPath('username-onboarding.png'), fullPage: true });
        await page.locator('#oidc-username').fill(playerName);
        await Promise.all([
            page.waitForURL(/https:\/\/geoguessme\.localhost\/groups$/, {
                waitUntil: 'domcontentloaded',
                timeout: 60_000,
            }),
            page.getByRole('button', { name: 'Start playing' }).click(),
        ]);
        await expect(page.locator('.groups-header')).toBeVisible();

        const adminToken = await keycloakAdminToken(request);
        const keycloakUsers = await keycloakUsersByEmail(request, adminToken, email);
        expect(keycloakUsers).toHaveLength(1);
        const deletedKeycloakUserID = keycloakUsers[0].id;

        await page.goto('/settings');
        await expect(page.getByRole('link', { name: 'Manage 2FA and passkeys' })).toHaveAttribute(
            'href',
            'https://auth-dev.geoguessme.com/realms/geoguessme/account/',
        );
        await page.getByLabel(`Type ${playerName} to delete account`).fill(playerName);
        page.once('dialog', (dialog) => dialog.accept());
        const deletion = page.waitForResponse(
            (response) => response.url().endsWith('/api/v1/auth/account') && response.request().method() === 'DELETE',
        );
        await page.getByRole('button', { name: 'Delete account' }).click();
        expect((await deletion).status()).toBe(204);
        await page.waitForURL('https://geoguessme.localhost/', { timeout: 30_000 });

        await expect
            .poll(async () => {
                const response = await request.get(
                    `${keycloakURL}/admin/realms/geoguessme/users/${deletedKeycloakUserID}`,
                    { headers: { Authorization: `Bearer ${adminToken}` } },
                );
                return response.status();
            })
            .toBe(404);
        await expect.poll(async () => (await keycloakUsersByEmail(request, adminToken, email)).length).toBe(0);

        await page.goto('/login');
        await page.getByPlaceholder('you@example.com').fill(email);
        await Promise.all([
            page.waitForURL(/https:\/\/auth-dev\.geoguessme\.com\//, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            page.getByRole('button', { name: 'Continue to password' }).click(),
        ]);
        await page.locator('#password').fill('TestPass123!');
        await page.locator('#kc-login').click();
        await expect(page.getByText('Invalid username or password.', { exact: true })).toBeVisible();
    });

    test('captures Keycloak email auth screens and the optional account journey', async ({ page }, testInfo) => {
        test.setTimeout(120_000);
        await page.goto('/login');
        await expect(page.getByText('Continue with Google', { exact: true })).toBeVisible();
        await expect(page.getByText('Continue with Apple', { exact: true })).toHaveCount(0);
        await expect(page.getByText('Continue with GitHub', { exact: true })).toHaveCount(0);
        await expect(page.getByRole('button', { name: 'Continue to password' })).toBeVisible();
        await expect(page.locator('#login-username')).toHaveCount(0);
        await waitForAuthCardVisuals(page);
        await page.screenshot({ path: testInfo.outputPath('01-app-login.png'), fullPage: true });

        await page.goto('/signup');
        await expect(page.getByText('Sign up with Google', { exact: true })).toBeVisible();
        await expect(page.getByText('Sign up with Apple', { exact: true })).toHaveCount(0);
        await expect(page.getByText('Sign up with GitHub', { exact: true })).toHaveCount(0);
        const emailSignup = page.getByRole('button', { name: 'Continue to create account' });
        await expect(emailSignup).toBeVisible();
        await expect(page.locator('#signup-username')).toHaveCount(0);
        await waitForAuthCardVisuals(page);
        await page.screenshot({ path: testInfo.outputPath('02-app-signup.png'), fullPage: true });

        await Promise.all([
            page.waitForURL(/https:\/\/auth-dev\.geoguessme\.com\//, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            page
                .getByPlaceholder('you@example.com')
                .fill('visual-signup@example.com')
                .then(() => emailSignup.click()),
        ]);
        await expect(page.locator('#kc-register-form')).toBeVisible();
        await expect(page.locator('#email')).toHaveValue('visual-signup@example.com');
        await expect(page.locator('#password')).toHaveCount(0);
        await expect(page.locator('#password-confirm')).toHaveCount(0);
        await expect(page.locator('#firstName')).toHaveCount(0);
        await expect(page.locator('#lastName')).toHaveCount(0);
        await expect(page.locator('#kc-social-providers')).toHaveCount(0);
        await expectLightKeycloakTheme(page);
        await page.screenshot({ path: testInfo.outputPath('03-keycloak-signup.png'), fullPage: true });

        await page.goto('/login');
        const emailLogin = page.getByRole('button', { name: 'Continue to password' });
        await page.getByPlaceholder('you@example.com').fill('visual-login@example.com');
        await Promise.all([
            page.waitForURL(/https:\/\/auth-dev\.geoguessme\.com\//, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            emailLogin.click(),
        ]);
        await expect(page.locator('#kc-form-login')).toBeVisible();
        await expect(page.locator('#username')).toHaveValue('visual-login@example.com');
        await expect(page.locator('#kc-social-providers')).toHaveCount(0);
        await expect(page.locator('#kc-registration')).toContainText("Don't have an account?");
        await expectLightKeycloakTheme(page, { checkbox: true });
        await page.screenshot({ path: testInfo.outputPath('04-keycloak-login.png'), fullPage: true });

        if (process.env.OIDC_VISUAL_AUTH_ONLY === 'true') return;

        await page.locator('#username').fill(username ?? '');
        await page.locator('#password').fill(password ?? '');
        await Promise.all([
            page.waitForURL(/\/groups$/, { waitUntil: 'domcontentloaded', timeout: 60_000 }),
            page.locator('#kc-login').click(),
        ]);
        await expect(page.locator('.groups-header')).toBeVisible();
        const dismissInstall = page.getByRole('button', { name: 'Dismiss install prompt' });
        if (await dismissInstall.isVisible()) await dismissInstall.click();
        await page.screenshot({ path: testInfo.outputPath('05-keycloak-session.png'), fullPage: true });

        await page.goto('/settings');
        await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
        await expect(page.getByText('Keycloak login is connected.')).toBeVisible();
        await expect(page.getByRole('link', { name: /Manage 2FA and passkeys/ })).toBeVisible();
        await page.screenshot({ path: testInfo.outputPath('06-account-security.png'), fullPage: true });

        await page.context().clearCookies();
        await page.goto('/login');
        await page.goto('/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback');
        await expect(page.locator('#kc-form-login')).toBeVisible();
        await page.locator('#username').fill(username ?? '');
        await page.locator('#password').fill(password ?? '');
        await Promise.all([
            page.waitForURL(/\/groups$/, { waitUntil: 'domcontentloaded', timeout: 60_000 }),
            page.locator('#kc-login').click(),
        ]);
        await expect(page.locator('.groups-header')).toBeVisible();
        await page.screenshot({ path: testInfo.outputPath('07-returning-keycloak-login.png'), fullPage: true });
    });

    test('keeps legacy credentials hidden and completes the read-only migration flow', async ({
        page,
        request,
    }, testInfo) => {
        test.setTimeout(120_000);
        test.skip(!legacyUsername || !legacyPassword, 'requires an unmigrated local application fixture');

        await page.goto('/login');
        await expect(page.getByRole('link', { name: 'Continue with Google' })).toBeVisible();
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
        const continueWithIdentity = page.getByRole('button', { name: 'Continue with GeoGuessMe ID' });
        await expect(continueWithIdentity).toBeVisible();
        await expect(page.getByRole('button', { name: 'Save profile' })).toHaveCount(0);
        await expect(page.getByRole('button', { name: 'Change password' })).toHaveCount(0);
        await page.screenshot({ path: testInfo.outputPath('08-read-only-migration-settings.png'), fullPage: true });

        const migrationEmail = `legacy-migration-${Date.now()}@example.com`;
        await Promise.all([
            page.waitForURL(/https:\/\/auth-dev\.geoguessme\.com\//, {
                waitUntil: 'domcontentloaded',
                timeout: 30_000,
            }),
            continueWithIdentity.click(),
        ]);
        await page.locator('#kc-registration a').click();
        await page.locator('#email').fill(migrationEmail);
        await completeKeycloakEmailRegistration(page, request, migrationEmail, 'TestPass123!');
        await page.waitForURL(/https:\/\/geoguessme\.localhost\/settings$/, {
            waitUntil: 'domcontentloaded',
            timeout: 60_000,
        });
        await expect(page.getByText('This legacy account is read-only until Keycloak is connected.')).toHaveCount(0);
        const saveProfile = page.getByRole('button', { name: 'Save profile' });
        await expect(saveProfile).toBeVisible();
        const profileWrite = page.waitForResponse(
            (response) => response.url().endsWith('/api/v1/auth/profile') && response.request().method() === 'PATCH',
        );
        await saveProfile.click();
        expect((await profileWrite).status()).toBe(200);
        await expect(page.locator('.auth-success')).toHaveText('Profile updated.');
        await page.screenshot({ path: testInfo.outputPath('09-migrated-account-unlocked.png'), fullPage: true });

        await page.getByLabel(`Type ${legacyUsername ?? ''} to delete account`).fill(legacyUsername ?? '');
        page.once('dialog', (dialog) => dialog.accept());
        const deletion = page.waitForResponse(
            (response) => response.url().endsWith('/api/v1/auth/account') && response.request().method() === 'DELETE',
        );
        await page.getByRole('button', { name: 'Delete account' }).click();
        expect((await deletion).status()).toBe(204);
        await page.waitForURL('https://geoguessme.localhost/', { timeout: 30_000 });
    });
});
