import type { Page, TestInfo } from '@playwright/test';
import { test, expect } from './support/fixtures';
import { createScenario, closeScenario } from './support/challengeScenario';

async function openGlobe(page: Page, testInfo: TestInfo) {
    await page.getByRole('button', { name: 'Open group globe' }).click();
    const globe = page.getByRole('dialog', { name: "Your group's world" });
    await expect(globe).toBeVisible();
    // Chromium can serve the Earth texture from its in-process memory cache on a
    // repeat visit, so no response event fires at all. The controls element is
    // rendered only once the texture has decoded and been drawn, which is the
    // readiness signal the app exposes and a stronger guarantee than observing
    // the network. Touch layouts hide it via CSS, so assert attachment here and
    // let each layout branch below assert its own visibility state.
    await expect(globe.locator('.globe-controls')).toBeAttached();
    await expect(globe).toBeInViewport({ ratio: 1 });
    await expect(globe.locator('canvas')).toBeVisible();
    if (testInfo.project.name === 'mobile') {
        // Touch devices navigate with gestures: the arrow pad stays hidden and
        // the globe owns most of the screen behind the collapsed list sheet.
        await expect(globe.locator('.globe-controls')).toBeHidden();
        const globeShare = await globe
            .locator('.globe-stage')
            .evaluate((element) => element.getBoundingClientRect().height / window.innerHeight);
        expect(globeShare).toBeGreaterThan(0.55);
        const collapsedSheetHeight = await globe
            .locator('.globe-history')
            .evaluate((element) => element.getBoundingClientRect().height);
        // The collapsed sheet should expose its handle and title without
        // taking a large white slice out of the globe viewport.
        expect(collapsedSheetHeight).toBeLessThan(120);
        await expect(globe.getByRole('button', { name: 'Expand geochallenge list' })).toHaveAttribute(
            'aria-expanded',
            'false',
        );
    } else {
        await expect(globe.getByRole('group', { name: 'Globe controls' })).toBeInViewport({ ratio: 1 });
    }
    await expect(globe.getByRole('button', { name: 'Close group globe' })).toBeInViewport({ ratio: 1 });
    await expect
        .poll(() => globe.evaluate((element) => element.scrollWidth - element.clientWidth))
        .toBeLessThanOrEqual(1);
    return globe;
}

const GLOBE_SCREENSHOT_TIMEOUT_MS = 20_000;

async function captureGlobe(page: Page, testInfo: TestInfo, state: string) {
    const name = `globe-${state}`;
    const path = testInfo.outputPath(`${name}.png`);
    // Software-rendered WebGL on CI runners can stall Chromium's screenshot
    // readback on the continuously rendered globe, and an unbounded capture
    // then consumes the whole journey budget. Bound each attempt, resync to a
    // freshly rendered frame between attempts, and keep the capture
    // best-effort: the journey assertions stay strict even when this
    // diagnostic evidence cannot be produced.
    for (let attempt = 1; attempt <= 2; attempt += 1) {
        try {
            await page.evaluate(() => document.fonts.ready.then(() => undefined));
            await page.screenshot({ path, animations: 'disabled', timeout: GLOBE_SCREENSHOT_TIMEOUT_MS });
            await testInfo.attach(name, { path, contentType: 'image/png' });
            return;
        } catch (error) {
            if (attempt === 2) {
                await testInfo.attach(`${name}-capture-error`, {
                    body: String(error),
                    contentType: 'text/plain',
                });
                return;
            }
            await page.evaluate(
                () =>
                    new Promise<void>((resolve) => {
                        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
                    }),
            );
        }
    }
}

// The journey covers two signups, a camera capture and upload with media
// processing, and repeated three.js globe interactions. CI renders WebGL in
// software, where the same interactions that take ~10s locally can exceed the
// 30s default when a runner is busy, so this journey carries its own budget.
test.setTimeout(90_000);
test('explores group challenges on Earth without revealing an unplayed location', async ({
    browser,
    contextOptions,
}, testInfo) => {
    const mobile = testInfo.project.name === 'mobile';
    const scenario = await createScenario(browser, contextOptions);
    try {
        const { uploader, guesser } = scenario;
        const emptyGlobe = await openGlobe(uploader, testInfo);
        await expect(emptyGlobe).toContainText('No geochallenges yet.');
        await captureGlobe(uploader, testInfo, 'empty');
        await emptyGlobe.getByRole('button', { name: 'Close group globe' }).click();
        await uploader.getByRole('button', { name: 'Camera', exact: true }).click();
        await uploader.locator('.capture-button').click();
        const upload = uploader.waitForResponse(
            (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
        );
        await uploader.getByRole('button', { name: /Send/ }).click();
        expect((await upload).status()).toBe(201);
        const globeButton = uploader.getByRole('button', { name: 'Open group globe' });
        const globe = await openGlobe(uploader, testInfo);
        await expect(globe).toContainText('1 challenge · 1 on the globe');
        const refresh = globe.getByRole('button', { name: 'Refresh geochallenges' });
        const refreshed = uploader.waitForResponse(
            (response) => response.url().includes('/api/v1/group/challenges') && response.request().method() === 'GET',
        );
        await refresh.click();
        expect((await refreshed).status()).toBe(200);
        await expect(globe).toContainText('1 challenge · 1 on the globe');
        await captureGlobe(uploader, testInfo, 'overview');
        if (mobile) {
            const sheet = globe.getByRole('button', { name: 'Expand geochallenge list' });
            await sheet.dispatchEvent('pointerdown', { clientY: 600, pointerId: 1, pointerType: 'touch' });
            await sheet.dispatchEvent('pointermove', { clientY: 520, pointerId: 1, pointerType: 'touch' });
            await sheet.dispatchEvent('pointerup', { clientY: 520, pointerId: 1, pointerType: 'touch' });
            await expect(globe.getByRole('button', { name: 'Collapse geochallenge list' })).toHaveAttribute(
                'aria-expanded',
                'true',
            );
        } else {
            await globe.getByRole('button', { name: 'Rotate globe left' }).click();
            await globe.getByRole('button', { name: 'Zoom in' }).click();
        }
        await globe.locator('.globe-challenge-list button').click();
        await expect(globe.getByRole('region', { name: 'Selected challenge' })).toBeFocused();
        await uploader.keyboard.press('Tab');
        await expect(globe.getByRole('button', { name: 'View results' })).toBeFocused();
        await expect(globe.getByRole('button', { name: 'View results' })).toBeInViewport({ ratio: 1 });
        await captureGlobe(uploader, testInfo, 'selected');
        // Keyboard focus wraps in both directions within the modal.
        await globe.getByRole('button', { name: 'Close group globe' }).focus();
        await uploader.keyboard.press('Shift+Tab');
        await expect(globe.locator('.globe-challenge-list button')).toBeFocused();
        await uploader.keyboard.press('Tab');
        await expect(globe.getByRole('button', { name: 'Close group globe' })).toBeFocused();
        await uploader.keyboard.press('Escape');
        await expect(globe).toHaveCount(0);
        await expect(globeButton).toBeFocused();

        const privateGlobe = await openGlobe(guesser, testInfo);
        await expect(privateGlobe).toContainText('1 challenge · 0 on the globe');
        await expect(privateGlobe).toContainText('Guess this challenge to reveal its location');
        if (mobile) {
            const expandList = privateGlobe.getByRole('button', { name: 'Expand geochallenge list' });
            await expandList.press('Enter');
            await expect(privateGlobe.getByRole('button', { name: 'Collapse geochallenge list' })).toHaveAttribute(
                'aria-expanded',
                'true',
            );
        }
        const privateChallenge = privateGlobe.locator('.globe-challenge-list button');
        await expect(privateChallenge).toBeVisible({ timeout: 5_000 });
        await privateChallenge.scrollIntoViewIfNeeded();
        await privateChallenge.click();
        await expect(privateGlobe.getByRole('button', { name: 'Play challenge' })).toBeInViewport({ ratio: 1 });
        await captureGlobe(guesser, testInfo, 'hidden-location');
        await privateGlobe.getByRole('button', { name: 'Play challenge' }).click();
        await expect(privateGlobe).toHaveCount(0);
        await expect(guesser.locator('.photo-view')).toBeVisible();
    } finally {
        await closeScenario(scenario);
    }
});
