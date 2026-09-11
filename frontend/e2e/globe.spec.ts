import { test, expect } from './support/fixtures';
import { createScenario, closeScenario } from './support/challengeScenario';

test('explores group challenges on Earth without revealing an unplayed location', async ({
    browser,
    contextOptions,
}) => {
    const scenario = await createScenario(browser, contextOptions);
    try {
        const { uploader, guesser } = scenario;
        await uploader.getByRole('button', { name: 'Camera', exact: true }).click();
        await uploader.locator('.capture-button').click();
        const upload = uploader.waitForResponse(
            (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
        );
        await uploader.getByRole('button', { name: /Send/ }).click();
        expect((await upload).status()).toBe(201);
        const globeButton = uploader.getByRole('button', { name: 'Open group globe' });
        await globeButton.click();
        const globe = uploader.getByRole('dialog', { name: "Your group's world" });
        await expect(globe).toBeVisible();
        await expect(globe).toContainText('1 challenge · 1 on the globe');
        await expect(globe.locator('canvas')).toBeVisible();
        await globe.getByRole('button', { name: 'Rotate globe left' }).click();
        await globe.getByRole('button', { name: 'Zoom in' }).click();
        // Native modal focus stays inside even when tabbing beyond the last control.
        await globe.locator('.globe-challenge-list button').click();
        await uploader.keyboard.press('Tab');
        await expect(globe.getByRole('button', { name: 'Close group globe' })).toBeFocused();
        await uploader.keyboard.press('Escape');
        await expect(globe).toHaveCount(0);
        await expect(globeButton).toBeFocused();

        await guesser.getByRole('button', { name: 'Open group globe' }).click();
        const privateGlobe = guesser.getByRole('dialog', { name: "Your group's world" });
        await expect(privateGlobe).toContainText('1 challenge · 0 on the globe');
        await expect(privateGlobe).toContainText('Guess this challenge to reveal its location');
        await privateGlobe.locator('.globe-challenge-list button').click();
        await privateGlobe.getByRole('button', { name: 'Play challenge' }).click();
        await expect(privateGlobe).toHaveCount(0);
        await expect(guesser.locator('.photo-view')).toBeVisible();
    } finally {
        await closeScenario(scenario);
    }
});
