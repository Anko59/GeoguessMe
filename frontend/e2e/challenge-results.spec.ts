import { test, expect } from './fixtures';
import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';
import {
    installDeterministicCamera,
    installDeterministicGeolocation,
    newAuthContext,
    uniqueGroup,
    uniqueUsername,
    uniqueEmail,
} from './helpers';

interface ResultScenario {
    uploader: Page;
    guesser: Page;
    uploaderContext: BrowserContext;
    guesserContext: BrowserContext;
    photoId: string;
    groupId: string;
    uploaderToken: string;
    guesserToken: string;
}

function cameraOptions(contextOptions: BrowserContextOptions): BrowserContextOptions {
    return {
        ...contextOptions,
        permissions: ['camera', 'geolocation'],
        geolocation: { latitude: 48.8566, longitude: 2.3522 },
        baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080',
    };
}

/** Sign up via UI and capture the access token from the signup response. */
async function signupWithToken(context: BrowserContext): Promise<{ page: Page; token: string }> {
    const page = await context.newPage();
    const username = uniqueUsername();
    const email = uniqueEmail();
    const password = 'TestPass123';

    const signupResponsePromise = page.waitForResponse(
        (r) => r.url().endsWith('/api/v1/auth/signup') && r.request().method() === 'POST',
    );

    await page.goto('/signup');
    await page.waitForSelector('#signup-username', { state: 'visible' });
    await page.fill('#signup-username', username);
    await page.fill('#signup-email', email);
    await page.fill('#signup-password', password);
    await page.click('button.btn-primary[type="submit"]');

    const signupResponse = await signupResponsePromise;
    const body = (await signupResponse.json()) as { access_token: string };
    await page.waitForURL(/\/groups/, { timeout: 15000 });

    return { page, token: body.access_token };
}

/** Build a scenario: owner uploads, guesser accepts and guesses. */
async function createResultScenario(browser: Browser, contextOptions: BrowserContextOptions): Promise<ResultScenario> {
    const options = cameraOptions(contextOptions);
    const uploaderContext = await newAuthContext(browser, options);
    const guesserContext = await newAuthContext(browser, options);
    await installDeterministicCamera(uploaderContext);
    await installDeterministicGeolocation(uploaderContext);

    const { page: uploader, token: uploaderToken } = await signupWithToken(uploaderContext);
    const { page: guesser, token: guesserToken } = await signupWithToken(guesserContext);

    await uploader.goto('/group/create');
    await uploader.getByPlaceholder('Group Name').fill(uniqueGroup());
    await uploader.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await uploader.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    const groupId = uploader.url().split('/group/')[1];

    await uploader.getByRole('button', { name: 'Open group settings' }).click();
    const settings = uploader.getByRole('dialog');
    const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
    await settings.getByRole('button', { name: 'Close settings' }).click();

    await guesser.goto('/group/join');
    await guesser.getByPlaceholder('6-character code').fill(groupCode);
    await guesser.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
    await guesser.waitForURL(/\/group\//);

    await uploader.goto('/group/' + groupId);
    await guesser.goto('/group/' + groupId);
    await expect(uploader.getByRole('status')).toHaveText('Connected');
    await expect(guesser.getByRole('status')).toHaveText('Connected');

    // Upload photo.
    await uploader.getByRole('button', { name: 'Camera' }).click();
    await expect(uploader.locator('.capture-button')).toBeVisible();
    await uploader.locator('.capture-button').click();
    await expect(uploader.locator('.preview-image')).toBeVisible();
    const uploadResponsePromise = uploader.waitForResponse(
        (r) => r.url().endsWith('/api/v1/photo/upload') && r.request().method() === 'POST',
    );
    await uploader.getByRole('button', { name: /Send/ }).click();
    const uploadResponse = await uploadResponsePromise;
    expect(uploadResponse.status()).toBe(201);
    const photoId = ((await uploadResponse.json()) as { id: string }).id;

    // Guesser accepts, waits for viewing window, and submits a guess.
    const challenge = guesser.locator('button.photo-challenge[data-photo-id="' + photoId + '"]');
    const acceptResponsePromise = guesser.waitForResponse(
        (r) => r.url().endsWith('/api/v1/challenges/' + photoId + '/accept') && r.request().method() === 'POST',
    );
    const mediaResponsePromise = guesser.waitForResponse(
        (r) => r.url().endsWith('/api/v1/challenges/' + photoId + '/media') && r.request().method() === 'GET',
    );
    await challenge.click();
    const acceptResponse = await acceptResponsePromise;
    expect(acceptResponse.status()).toBe(200);
    const mediaResponse = await mediaResponsePromise;
    expect(mediaResponse.status()).toBe(200);

    await expect(guesser.locator('.photo-view')).toBeVisible();
    await expect(guesser.locator('.guessing-view')).toBeVisible();
    await guesser.locator('.leaflet-container').click({ position: { x: 200, y: 150 } });
    const guessResponsePromise = guesser.waitForResponse(
        (r) => r.url().endsWith('/api/v1/challenges/' + photoId + '/guess') && r.request().method() === 'POST',
    );
    await guesser.getByRole('button', { name: /Submit guess/ }).click();
    const guessResponse = await guessResponsePromise;
    expect(guessResponse.status()).toBe(201);

    return { uploader, guesser, uploaderContext, guesserContext, photoId, groupId, uploaderToken, guesserToken };
}

/** Fetch an API path via page.evaluate, optionally with a Bearer token.
 *  The page must have a non-blank origin so relative URLs resolve;
 *  for unauthenticated pages, navigate to a public page first. */
async function apiFetch(page: Page, path: string, token?: string): Promise<{ status: number; body: unknown }> {
    return page.evaluate(
        ({ path, token }) => {
            const headers: Record<string, string> = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            return fetch(path, { headers }).then(async (r) => {
                let body: unknown = null;
                try {
                    body = await r.json();
                } catch {
                    // non-JSON body
                }
                return { status: r.status, body };
            });
        },
        { path, token: token ?? '' },
    );
}

test.describe('Challenge result authorization', () => {
    test('owner sees protected result data', async ({ browser, contextOptions }) => {
        const scenario = await createResultScenario(browser, contextOptions);
        try {
            const { uploader, photoId, uploaderToken } = scenario;

            // Click the challenge button to open results via UI.
            const ownerChallenge = uploader.locator('button.photo-challenge[data-photo-id="' + photoId + '"]');
            await expect(ownerChallenge).toContainText('Challenge sent');
            await ownerChallenge.click();
            await expect(uploader.locator('.result-view')).toContainText('Challenge results');

            // Verify protected fields via API with token.
            const path = '/api/v1/challenges/' + photoId + '/results';
            const { status, body } = await apiFetch(uploader, path, uploaderToken);
            expect(status).toBe(200);
            const data = body as Record<string, unknown>;
            expect(data.photo_id).toBe(photoId);
            expect(typeof data.actual_lat).toBe('number');
            expect(typeof data.actual_long).toBe('number');
            expect(Array.isArray(data.guesses)).toBe(true);
            const guesses = data.guesses as Array<Record<string, unknown>>;
            expect(guesses.length).toBeGreaterThanOrEqual(1);
            expect(typeof guesses[0].username).toBe('string');
            expect(typeof guesses[0].score).toBe('number');
            expect(typeof guesses[0].distance).toBe('number');
            expect(data.media_available).toBe(true);
            expect(typeof data.media_url).toBe('string');
        } finally {
            await scenario.uploaderContext.close();
            await scenario.guesserContext.close();
        }
    });

    test('participant sees results after guessing', async ({ browser, contextOptions }) => {
        const scenario = await createResultScenario(browser, contextOptions);
        try {
            const { guesser, photoId, guesserToken } = scenario;

            // Guesser already sees result-view from the guess flow.
            await expect(guesser.locator('.result-view')).toContainText('Challenge results');

            const path = '/api/v1/challenges/' + photoId + '/results';
            const { status, body } = await apiFetch(guesser, path, guesserToken);
            expect(status).toBe(200);
            const data = body as Record<string, unknown>;
            expect(data.photo_id).toBe(photoId);
            expect(Array.isArray(data.guesses)).toBe(true);
            const guesses = data.guesses as Array<Record<string, unknown>>;
            expect(guesses.length).toBeGreaterThanOrEqual(1);
            expect(typeof guesses[0].score).toBe('number');
            expect(data.media_available).toBe(true);
        } finally {
            await scenario.uploaderContext.close();
            await scenario.guesserContext.close();
        }
    });

    test('unauthenticated visitor gets 401', async ({ browser, contextOptions }) => {
        const scenario = await createResultScenario(browser, contextOptions);
        try {
            const { photoId } = scenario;
            const base = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';

            // Create an unauthenticated context.
            const unauthContext = await browser.newContext({ baseURL: base });
            const unauthPage = await unauthContext.newPage();
            try {
                // Navigate to a public page so the page has an origin for relative fetch.
                await unauthPage.goto('/login');
                await unauthPage.waitForSelector('#login-username', { state: 'visible' });

                const path = '/api/v1/challenges/' + photoId + '/results';
                const { status } = await apiFetch(unauthPage, path);
                expect(status).toBe(401);
            } finally {
                await unauthContext.close();
            }
        } finally {
            await scenario.uploaderContext.close();
            await scenario.guesserContext.close();
        }
    });

    test('unrelated authenticated user gets 403', async ({ browser, contextOptions }) => {
        const scenario = await createResultScenario(browser, contextOptions);
        try {
            const { photoId } = scenario;
            const base = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';

            // Sign up a user not in the group.
            const thirdContext = await browser.newContext({
                ...cameraOptions(contextOptions),
                baseURL: base,
            });
            const { page: thirdPage, token: thirdToken } = await signupWithToken(thirdContext);
            try {
                const path = '/api/v1/challenges/' + photoId + '/results';
                const { status } = await apiFetch(thirdPage, path, thirdToken);
                expect(status).toBe(403);
            } finally {
                await thirdContext.close();
            }
        } finally {
            await scenario.uploaderContext.close();
            await scenario.guesserContext.close();
        }
    });

    test('in-group non-guesser gets 403 before TTL expiry', async ({ browser, contextOptions }) => {
        const base = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
        const options = cameraOptions(contextOptions);
        const uploaderContext = await newAuthContext(browser, options);
        const guesserContext = await newAuthContext(browser, options);
        const thirdContext = await browser.newContext({ ...options, baseURL: base });
        await installDeterministicCamera(uploaderContext);
        await installDeterministicGeolocation(uploaderContext);

        const { page: uploader, token: uploaderToken } = await signupWithToken(uploaderContext);
        const { page: guesser, token: guesserToken } = await signupWithToken(guesserContext);
        const { page: thirdPage, token: thirdToken } = await signupWithToken(thirdContext);

        try {
            await uploader.goto('/group/create');
            await uploader.getByPlaceholder('Group Name').fill(uniqueGroup());
            await uploader.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await uploader.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            const groupId = uploader.url().split('/group/')[1];

            await uploader.getByRole('button', { name: 'Open group settings' }).click();
            const settings = uploader.getByRole('dialog');
            const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
            await settings.getByRole('button', { name: 'Close settings' }).click();

            // Both guesser and third join.
            for (const p of [guesser, thirdPage]) {
                await p.goto('/group/join');
                await p.getByPlaceholder('6-character code').fill(groupCode);
                await p.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
                await p.waitForURL(/\/group\//);
                await p.goto('/group/' + groupId);
                await expect(p.getByRole('status')).toHaveText('Connected');
            }
            await uploader.goto('/group/' + groupId);
            await expect(uploader.getByRole('status')).toHaveText('Connected');

            // Upload photo.
            await uploader.getByRole('button', { name: 'Camera' }).click();
            await expect(uploader.locator('.capture-button')).toBeVisible();
            await uploader.locator('.capture-button').click();
            await expect(uploader.locator('.preview-image')).toBeVisible();
            const uploadResponsePromise = uploader.waitForResponse(
                (r) => r.url().endsWith('/api/v1/photo/upload') && r.request().method() === 'POST',
            );
            await uploader.getByRole('button', { name: /Send/ }).click();
            const uploadResponse = await uploadResponsePromise;
            expect(uploadResponse.status()).toBe(201);
            const photoId = ((await uploadResponse.json()) as { id: string }).id;

            const path = '/api/v1/challenges/' + photoId + '/results';

            // Third member (in group, hasn't guessed) gets 403.
            const { status: thirdStatus } = await apiFetch(thirdPage, path, thirdToken);
            expect(thirdStatus).toBe(403);

            // Owner can see results.
            const { status: ownerStatus } = await apiFetch(uploader, path, uploaderToken);
            expect(ownerStatus).toBe(200);

            // Guesser accepts and guesses.
            const challenge = guesser.locator('button.photo-challenge[data-photo-id="' + photoId + '"]');
            const acceptResponsePromise = guesser.waitForResponse(
                (r) => r.url().endsWith('/api/v1/challenges/' + photoId + '/accept') && r.request().method() === 'POST',
            );
            const mediaResponsePromise = guesser.waitForResponse(
                (r) => r.url().endsWith('/api/v1/challenges/' + photoId + '/media') && r.request().method() === 'GET',
            );
            await challenge.click();
            expect((await acceptResponsePromise).status()).toBe(200);
            expect((await mediaResponsePromise).status()).toBe(200);

            await expect(guesser.locator('.photo-view')).toBeVisible();
            await expect(guesser.locator('.guessing-view')).toBeVisible();
            await guesser.locator('.leaflet-container').click({ position: { x: 200, y: 150 } });
            const guessResponsePromise = guesser.waitForResponse(
                (r) => r.url().endsWith('/api/v1/challenges/' + photoId + '/guess') && r.request().method() === 'POST',
            );
            await guesser.getByRole('button', { name: /Submit guess/ }).click();
            expect((await guessResponsePromise).status()).toBe(201);

            // Guesser now sees results.
            const { status: guesserStatus } = await apiFetch(guesser, path, guesserToken);
            expect(guesserStatus).toBe(200);

            // Third member still gets 403 (hasn't guessed, TTL not expired).
            const { status: thirdAfterStatus } = await apiFetch(thirdPage, path, thirdToken);
            expect(thirdAfterStatus).toBe(403);
        } finally {
            await uploaderContext.close();
            await guesserContext.close();
            await thirdContext.close();
        }
    });
});
