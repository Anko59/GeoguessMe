import { test, expect } from './fixtures';
import type { Browser, BrowserContextOptions, Page } from '@playwright/test';
import {
    deterministicTestImage,
    installDeterministicCamera,
    installDeterministicGeolocation,
    malformedFileBytes,
    newAuthContext,
    oversizedUploadBytes,
    signupViaUI,
    uniqueGroup,
    unsupportedFormatBytes,
} from './helpers';
import { LENS_OPTIONS } from '../src/components/camera/lenses/lensCatalog';

interface Scenario {
    uploader: Page;
    guesser: Page;
    uploaderContext: Awaited<ReturnType<typeof newAuthContext>>;
    guesserContext: Awaited<ReturnType<typeof newAuthContext>>;
}

function cameraOptions(contextOptions: BrowserContextOptions): BrowserContextOptions {
    return {
        ...contextOptions,
        permissions: ['camera', 'geolocation'],
        geolocation: { latitude: 48.8566, longitude: 2.3522 },
        baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080',
    };
}

async function createScenario(browser: Browser, contextOptions: BrowserContextOptions): Promise<Scenario> {
    const options = cameraOptions(contextOptions);
    const uploaderContext = await newAuthContext(browser, options);
    const guesserContext = await newAuthContext(browser, options);
    await installDeterministicCamera(uploaderContext);
    await installDeterministicGeolocation(uploaderContext);

    const uploader = await uploaderContext.newPage();
    await signupViaUI(uploader);
    await uploader.goto('/group/create');
    await uploader.getByPlaceholder('Group Name').fill(uniqueGroup());
    await uploader.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await uploader.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    const groupId = uploader.url().split('/group/')[1];

    await uploader.getByRole('button', { name: 'Open group settings' }).click();
    const settings = uploader.getByRole('dialog');
    const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
    await settings.getByRole('button', { name: 'Close settings' }).click();

    const guesser = await guesserContext.newPage();
    await signupViaUI(guesser);
    await guesser.goto('/group/join');
    await guesser.getByPlaceholder('6-character code').fill(groupCode);
    await guesser.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
    await guesser.waitForURL(/\/group\//);

    await uploader.goto('/group/' + groupId);
    await guesser.goto('/group/' + groupId);
    await expect(uploader.getByRole('status')).toHaveText('Connected');
    await expect(guesser.getByRole('status')).toHaveText('Connected');
    return { uploader, guesser, uploaderContext, guesserContext };
}

async function closeScenario(scenario: Scenario): Promise<void> {
    await scenario.uploaderContext.close();
    await scenario.guesserContext.close();
}

test.describe('Challenge flow', () => {
    test('uploads, accepts, hides media, records a guess, and reopens exact results', async ({
        browser,
        contextOptions,
    }) => {
        const scenario = await createScenario(browser, contextOptions);
        try {
            const { uploader, guesser } = scenario;
            await uploader.getByRole('button', { name: 'Camera' }).click();
            await expect(uploader.locator('.capture-button')).toBeVisible();
            await uploader.locator('.capture-button').click();
            await expect(uploader.locator('.preview-image')).toBeVisible();

            const uploadResponsePromise = uploader.waitForResponse(
                (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
            );
            await uploader.getByRole('button', { name: /Send/ }).click();
            const uploadResponse = await uploadResponsePromise;
            expect(uploadResponse.status()).toBe(201);
            const uploaded = (await uploadResponse.json()) as { id: string };
            expect(uploaded.id).toMatch(/^[0-9a-f-]{36}$/);
            await expect(uploader.locator('.chat-container')).toBeVisible();

            const exactChallenge = uploader.locator('button.photo-challenge[data-photo-id="' + uploaded.id + '"]');
            const receivedChallenge = guesser.locator('button.photo-challenge[data-photo-id="' + uploaded.id + '"]');
            await expect(receivedChallenge).toContainText('New challenge');
            await expect(receivedChallenge).toContainText('Accept challenge');
            const acceptResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + uploaded.id + '/accept') &&
                    response.request().method() === 'POST',
            );
            const mediaResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + uploaded.id + '/media') &&
                    response.request().method() === 'GET',
            );
            await receivedChallenge.click();
            const acceptResponse = await acceptResponsePromise;
            expect(acceptResponse.status()).toBe(200);
            const mediaResponse = await mediaResponsePromise;
            expect(mediaResponse.status()).toBe(200);
            await expect(guesser.locator('.photo-view')).toBeVisible();
            await expect(guesser.locator('.game-photo')).toBeVisible();
            await expect(guesser.locator('.guessing-view')).toHaveCount(0);
            await expect(guesser.locator('.game-photo')).toHaveCount(0, { timeout: 10000 });
            await expect(guesser.locator('.guessing-view')).toBeVisible();

            await guesser.locator('.leaflet-container').click({ position: { x: 200, y: 150 } });
            const guessResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + uploaded.id + '/guess') &&
                    response.request().method() === 'POST',
            );
            await guesser.getByRole('button', { name: /Submit guess/ }).click();
            const guessResponse = await guessResponsePromise;
            expect(guessResponse.status()).toBe(201);
            await expect(guesser.locator('.result-view')).toContainText('Challenge results');

            await expect(exactChallenge).toContainText('Challenge sent');
            await exactChallenge.click();
            await expect(uploader.locator('.result-view')).toContainText('Challenge results');
        } finally {
            await closeScenario(scenario);
        }
    });

    test('completed challenge reopens existing results', async ({ browser, contextOptions }) => {
        const scenario = await createScenario(browser, contextOptions);
        try {
            const { uploader, guesser } = scenario;
            await uploader.getByRole('button', { name: 'Camera' }).click();
            await uploader.locator('.capture-button').click();
            await expect(uploader.locator('.preview-image')).toBeVisible();
            const uploadResponsePromise = uploader.waitForResponse(
                (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
            );
            await uploader.getByRole('button', { name: /Send/ }).click();
            const uploadResponse = await uploadResponsePromise;
            const photoID = ((await uploadResponse.json()) as { id: string }).id;
            const challenge = guesser.locator('button.photo-challenge[data-photo-id="' + photoID + '"]');
            const acceptResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + photoID + '/accept') &&
                    response.request().method() === 'POST',
            );
            const mediaResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + photoID + '/media') &&
                    response.request().method() === 'GET',
            );
            await challenge.click();
            const acceptResponse = await acceptResponsePromise;
            expect(acceptResponse.status()).toBe(200);
            const mediaResponse = await mediaResponsePromise;
            expect(mediaResponse.status()).toBe(200);
            await expect(guesser.locator('.photo-view')).toBeVisible();
            await expect(guesser.locator('.guessing-view')).toBeVisible();
            await guesser.locator('.leaflet-container').click({ position: { x: 250, y: 180 } });
            await guesser.getByRole('button', { name: /Submit guess/ }).click();
            await expect(guesser.locator('.result-view')).toBeVisible();
            await guesser.getByRole('button', { name: 'Close' }).click();
            await challenge.click();
            await expect(guesser.locator('.result-view')).toContainText('Challenge results');
        } finally {
            await closeScenario(scenario);
        }
    });

    test('camera denial is recoverable and geolocation denial is reported', async ({ browser, contextOptions }) => {
        const cameraDenied = await newAuthContext(browser, { ...contextOptions, permissions: [] });
        const cameraPage = await cameraDenied.newPage();
        try {
            await signupViaUI(cameraPage);
            await cameraPage.goto('/group/create');
            await cameraPage.getByPlaceholder('Group Name').fill(uniqueGroup());
            await cameraPage.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await cameraPage.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await cameraPage.getByRole('button', { name: 'Camera' }).click();
            await expect(cameraPage.locator('.camera-error')).toContainText('Camera access denied');
            await expect(cameraPage.getByRole('button', { name: 'Try Again' })).toBeVisible();
        } finally {
            await cameraDenied.close();
        }

        const locationDenied = await newAuthContext(browser, { ...contextOptions, permissions: ['camera'] });
        await installDeterministicCamera(locationDenied);
        const locationPage = await locationDenied.newPage();
        try {
            await signupViaUI(locationPage);
            await locationPage.goto('/group/create');
            await locationPage.getByPlaceholder('Group Name').fill(uniqueGroup());
            await locationPage.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await locationPage.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await locationPage.getByRole('button', { name: 'Camera' }).click();
            await locationPage.locator('.capture-button').click();
            await expect(locationPage.locator('.preview-image')).toBeVisible();
            await locationPage.getByRole('button', { name: /Send/ }).click();
            await expect(locationPage.locator('.camera-error')).toContainText('Unable to retrieve location');
        } finally {
            await locationDenied.close();
        }
    });

    test('file fallback uploads a valid image when camera is denied', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, {
            ...contextOptions,
            permissions: ['geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicGeolocation(ctx);
        const page = await ctx.newPage();
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            await page.getByRole('button', { name: 'Camera' }).click();
            await expect(page.locator('.camera-error')).toContainText('Camera access denied');
            await expect(page.getByRole('button', { name: 'Upload from device' })).toBeVisible();
            await page.getByRole('button', { name: 'Upload from device' }).click();

            const fileInput = page.locator('#camera-file-input');
            await expect(fileInput).toBeAttached();
            await fileInput.setInputFiles({
                name: 'valid.png',
                mimeType: 'image/png',
                buffer: deterministicTestImage(),
            });
            await expect(page.locator('.preview-image')).toBeVisible();

            const uploadResponsePromise = page.waitForResponse(
                (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
            );
            await page.getByRole('button', { name: /Send/ }).click();
            const uploadResponse = await uploadResponsePromise;
            expect(uploadResponse.status()).toBe(201);
            await expect(page.locator('.chat-container')).toBeVisible();
        } finally {
            await ctx.close();
        }
    });

    test('loads and switches every self-hosted 3D lens on demand', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, cameraOptions(contextOptions));
        await installDeterministicCamera(ctx);
        const page = await ctx.newPage();
        const pageErrors: Error[] = [];
        page.on('pageerror', (pageError) => pageErrors.push(pageError));
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await page.getByRole('button', { name: 'Camera' }).click();

            const options = page.locator('.camera-filter-option');
            await expect(options).toHaveCount(16);
            const modelResponse = page.waitForResponse((response) =>
                response.url().endsWith('/vendor/mediapipe/face_landmarker.task'),
            );
            await page.getByRole('button', { name: 'Cyber visor' }).click();
            expect((await modelResponse).status()).toBe(200);
            await expect(page.locator('.camera-filter-picker')).not.toContainText('Loading 3D face tracking', {
                timeout: 30000,
            });
            expect(await page.locator('.camera-filter-overlay').evaluate((canvas) => canvas.width > 1)).toBe(true);

            for (const lens of LENS_OPTIONS.filter(({ id }) => id !== 'none')) {
                const button = page.getByRole('button', { name: lens.label });
                await button.click();
                await expect(button).toHaveAttribute('aria-pressed', 'true');
            }

            expect(pageErrors).toEqual([]);
        } finally {
            await ctx.close();
        }
    });

    test('retake discards the captured photo and reactivates camera', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, {
            ...contextOptions,
            permissions: ['camera', 'geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicCamera(ctx);
        await installDeterministicGeolocation(ctx);
        const page = await ctx.newPage();
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            await page.getByRole('button', { name: 'Camera' }).click();
            await expect(page.locator('.capture-button')).toBeVisible();
            await page.locator('.capture-button').click();
            await expect(page.locator('.preview-image')).toBeVisible();

            await page.getByRole('button', { name: 'Retake' }).click();
            await expect(page.locator('.capture-button')).toBeVisible();
            await expect(page.locator('.preview-image')).not.toBeAttached();

            await page.locator('.capture-button').click();
            await expect(page.locator('.preview-image')).toBeVisible();

            const uploadResponsePromise = page.waitForResponse(
                (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
            );
            await page.getByRole('button', { name: /Send/ }).click();
            const uploadResponse = await uploadResponsePromise;
            expect(uploadResponse.status()).toBe(201);
            await expect(page.locator('.chat-container')).toBeVisible();
        } finally {
            await ctx.close();
        }
    });

    test('upload API failure shows an error and leaves preview visible', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, {
            ...contextOptions,
            permissions: ['camera', 'geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicCamera(ctx);
        await installDeterministicGeolocation(ctx);
        const page = await ctx.newPage();
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            await page.getByRole('button', { name: 'Camera' }).click();
            await expect(page.locator('.capture-button')).toBeVisible();
            await page.locator('.capture-button').click();
            await expect(page.locator('.preview-image')).toBeVisible();

            await page.route('**/api/v1/photo/upload', async (route) => {
                await route.fulfill({
                    status: 502,
                    contentType: 'application/json',
                    body: JSON.stringify({
                        error: { code: 'storage_error', message: 'Unable to store image' },
                    }),
                });
            });

            await page.getByRole('button', { name: /Send/ }).click();
            await expect(page.locator('.camera-error')).toContainText('Unable to store image');
            await expect(page.locator('.preview-image')).toBeVisible();
            await expect(page.getByRole('button', { name: 'Retake' })).toBeVisible();
        } finally {
            await ctx.close();
        }
    });

    test('malformed file from fallback is rejected by the backend', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, {
            ...contextOptions,
            permissions: ['geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicGeolocation(ctx);
        const page = await ctx.newPage();
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            await page.getByRole('button', { name: 'Camera' }).click();
            await expect(page.locator('.camera-error')).toContainText('Camera access denied');
            await page.getByRole('button', { name: 'Upload from device' }).click();

            const fileInput = page.locator('#camera-file-input');
            await expect(fileInput).toBeAttached();
            await fileInput.setInputFiles({
                name: 'malformed.png',
                mimeType: 'image/png',
                buffer: malformedFileBytes(),
            });
            await expect(page.locator('.preview-image')).toBeVisible();

            await page.getByRole('button', { name: /Send/ }).click();
            await expect(page.locator('.camera-error')).toContainText('invalid image');
        } finally {
            await ctx.close();
        }
    });

    test('unsupported image format from fallback is rejected by the backend', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, {
            ...contextOptions,
            permissions: ['geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicGeolocation(ctx);
        const page = await ctx.newPage();
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            await page.getByRole('button', { name: 'Camera' }).click();
            await expect(page.locator('.camera-error')).toContainText('Camera access denied');
            await page.getByRole('button', { name: 'Upload from device' }).click();

            const fileInput = page.locator('#camera-file-input');
            await expect(fileInput).toBeAttached();
            await fileInput.setInputFiles({
                name: 'animated.gif',
                mimeType: 'image/gif',
                buffer: unsupportedFormatBytes(),
            });
            await expect(page.locator('.preview-image')).toBeVisible();

            await page.getByRole('button', { name: /Send/ }).click();
            await expect(page.locator('.camera-error')).toContainText('unknown format');
        } finally {
            await ctx.close();
        }
    });

    test('oversized upload from fallback is rejected by the backend', async ({ browser, contextOptions }) => {
        const ctx = await newAuthContext(browser, {
            ...contextOptions,
            permissions: ['geolocation'],
            geolocation: { latitude: 48.8566, longitude: 2.3522 },
        });
        await installDeterministicGeolocation(ctx);
        const page = await ctx.newPage();
        try {
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);

            await page.getByRole('button', { name: 'Camera' }).click();
            await expect(page.locator('.camera-error')).toContainText('Camera access denied');
            await page.getByRole('button', { name: 'Upload from device' }).click();

            const fileInput = page.locator('#camera-file-input');
            await expect(fileInput).toBeAttached();
            await fileInput.setInputFiles({
                name: 'oversized.png',
                mimeType: 'image/png',
                buffer: oversizedUploadBytes(),
            });
            await expect(page.locator('.preview-image')).toBeVisible();

            await page.getByRole('button', { name: /Send/ }).click();
            await expect(page.locator('.camera-error')).toBeVisible();
        } finally {
            await ctx.close();
        }
    });
});
