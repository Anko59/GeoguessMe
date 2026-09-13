import type { Page, TestInfo } from '@playwright/test';
import { test, expect } from './support/fixtures';
import { createScenario, closeScenario } from './support/challengeScenario';

async function openGlobe(page: Page) {
    const texture = page.waitForResponse((response) => new URL(response.url()).pathname === '/globe/earth.jpg');
    await page.getByRole('button', { name: 'Open group globe' }).click();
    const response = await texture;
    expect(response.ok()).toBe(true);
    expect(await response.finished()).toBeNull();
    const globe = page.getByRole('dialog', { name: "Your group's world" });
    await expect(globe).toBeVisible();
    await expect(globe).toBeInViewport({ ratio: 1 });
    await expect(globe.locator('canvas')).toBeVisible();
    await expect(globe.getByRole('group', { name: 'Globe controls' })).toBeInViewport({ ratio: 1 });
    await expect(globe.getByRole('button', { name: 'Close group globe' })).toBeInViewport({ ratio: 1 });
    await expect
        .poll(() => globe.evaluate((element) => element.scrollWidth - element.clientWidth))
        .toBeLessThanOrEqual(1);
    return globe;
}

async function captureGlobe(page: Page, testInfo: TestInfo, state: string) {
    await page.evaluate(() => document.fonts.ready.then(() => undefined));
    const name = `globe-${state}`;
    const path = testInfo.outputPath(`${name}.png`);
    await page.screenshot({ path, animations: 'disabled' });
    await testInfo.attach(name, { path, contentType: 'image/png' });
}

test('explores group challenges on Earth without revealing an unplayed location', async ({
    browser,
    contextOptions,
}, testInfo) => {
    const scenario = await createScenario(browser, contextOptions);
    try {
        const { uploader, guesser } = scenario;
        const emptyGlobe = await openGlobe(uploader);
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
        const globe = await openGlobe(uploader);
        await expect(globe).toContainText('1 challenge · 1 on the globe');
        await captureGlobe(uploader, testInfo, 'overview');
        await globe.getByRole('button', { name: 'Rotate globe left' }).click();
        await globe.getByRole('button', { name: 'Zoom in' }).click();
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

        const privateGlobe = await openGlobe(guesser);
        await expect(privateGlobe).toContainText('1 challenge · 0 on the globe');
        await expect(privateGlobe).toContainText('Guess this challenge to reveal its location');
        await privateGlobe.locator('.globe-challenge-list button').click();
        await expect(privateGlobe.getByRole('button', { name: 'Play challenge' })).toBeInViewport({ ratio: 1 });
        await captureGlobe(guesser, testInfo, 'hidden-location');
        await privateGlobe.getByRole('button', { name: 'Play challenge' }).click();
        await expect(privateGlobe).toHaveCount(0);
        await expect(guesser.locator('.photo-view')).toBeVisible();
    } finally {
        await closeScenario(scenario);
    }
});
