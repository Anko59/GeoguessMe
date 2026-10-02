import * as THREE from 'three';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { globePosition } from '../src/components/globe/globeScene';
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
    const maxTextureSize = await globe.locator('canvas').evaluate((canvas) => {
        const context = canvas.getContext('webgl2') ?? canvas.getContext('webgl');
        return context?.getParameter(context.MAX_TEXTURE_SIZE) ?? 0;
    });
    // The renderer must have a usable bounded texture path on every supported
    // WebGL device; high-limit devices use the sharper asset and smaller ones
    // stay on the bundled fallback.
    expect(maxTextureSize).toBeGreaterThanOrEqual(2048);
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

function projectGlobePin(
    width: number,
    height: number,
    latitude: number,
    longitude: number,
): { x: number; y: number; width: number; height: number } {
    const camera = new THREE.PerspectiveCamera(42, width / height, 0.1, 50);
    camera.position.copy(globePosition(22, 12, 3.5));
    camera.lookAt(0, 0, 0);
    camera.updateMatrixWorld();
    const marker = globePosition(latitude, longitude, 1.016);
    const markerView = marker.clone().applyMatrix4(camera.matrixWorldInverse);
    const tanHalfFov = Math.tan(THREE.MathUtils.degToRad(camera.getEffectiveFOV() / 2));
    const depth = Math.max(-markerView.z, 0.1);
    const pinRadius = (6 * depth * tanHalfFov) / height;
    const pinHeight = pinRadius * 7;
    marker.addScaledVector(marker.clone().normalize(), pinHeight * 0.48);
    const projected = marker.project(camera);
    const screenHeight = (pinHeight * height) / (2 * depth * tanHalfFov);

    return {
        x: ((projected.x + 1) * width) / 2,
        y: ((1 - projected.y) * height) / 2,
        width: screenHeight * 0.78,
        height: screenHeight,
    };
}

const GLOBE_SCREENSHOT_TIMEOUT_MS = 20_000;

async function captureGlobe(page: Page, testInfo: TestInfo, state: string) {
    const name = `globe-${state}`;
    const path = testInfo.outputPath(`${name}.png`);
    const fallbackNotice = page.locator('.globe-detail-notice');
    if (await fallbackNotice.isVisible()) {
        const [noticeBox, creditBox] = await Promise.all([
            fallbackNotice.boundingBox(),
            page.locator('.globe-credit').boundingBox(),
        ]);
        expect(noticeBox).not.toBeNull();
        expect(creditBox).not.toBeNull();
        const overlaps =
            noticeBox!.left < creditBox!.right &&
            noticeBox!.right > creditBox!.left &&
            noticeBox!.top < creditBox!.bottom &&
            noticeBox!.bottom > creditBox!.top;
        expect(overlaps, 'fallback status must not cover the imagery attribution').toBe(false);
    }
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
        await expect(emptyGlobe.locator('.globe-pin-marker')).toHaveCount(0);
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
        const defaultPinResponse = uploader.waitForResponse((response) =>
            response.url().endsWith('/assets/map-pins/standard-marker-v1.svg'),
        );
        const globe = await openGlobe(uploader, testInfo);
        expect((await defaultPinResponse).status(), 'the globe should load its default pin artwork').toBe(200);
        await expect(globe).toContainText('1 challenge · 1 on the globe');
        const refresh = globe.getByRole('button', { name: 'Refresh geochallenges' });
        const refreshed = uploader.waitForResponse(
            (response) => response.url().includes('/api/v1/group/challenges') && response.request().method() === 'GET',
        );
        await refresh.click();
        expect((await refreshed).status()).toBe(200);
        await expect(globe).toContainText('1 challenge · 1 on the globe');
        const globePin = globe.locator('.globe-pin-marker');
        await expect(globePin).toHaveCount(1);
        await expect(globePin).toBeVisible();
        await expect(globePin).toHaveAttribute('src', '/assets/map-pins/standard-marker-v1.svg');
        await expect
            .poll(
                () =>
                    globePin.evaluate((element) => {
                        const image = element as HTMLImageElement;
                        return image.complete && image.naturalWidth > 0 && image.naturalHeight > 0;
                    }),
                { message: 'the designed pin artwork should decode before it is shown', timeout: 8_000 },
            )
            .toBe(true);
        const [canvasBounds, pinBounds] = await Promise.all([
            globe.locator('canvas').boundingBox(),
            globePin.boundingBox(),
        ]);
        if (!canvasBounds || !pinBounds) throw new Error('The visible globe pin and canvas must have bounding boxes');
        const expectedPin = projectGlobePin(canvasBounds.width, canvasBounds.height, 48.8566, 2.3522);
        expect(Math.abs(pinBounds.x + pinBounds.width / 2 - canvasBounds.x - expectedPin.x)).toBeLessThanOrEqual(1.5);
        expect(Math.abs(pinBounds.y + pinBounds.height / 2 - canvasBounds.y - expectedPin.y)).toBeLessThanOrEqual(1.5);
        expect(Math.abs(pinBounds.width - expectedPin.width)).toBeLessThanOrEqual(1.5);
        expect(Math.abs(pinBounds.height - expectedPin.height)).toBeLessThanOrEqual(1.5);
        await captureGlobe(uploader, testInfo, 'overview');
        if (mobile) {
            const sheet = globe.locator('.globe-sheet-grabber');
            await sheet.click();
            await expect(sheet).toHaveAttribute('aria-expanded', 'true');
            await expect(globe.locator('.globe-challenge-list button')).toBeVisible();
            await sheet.focus();
            await uploader.keyboard.press('Enter');
            await expect(sheet).toHaveAttribute('aria-expanded', 'false');
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
        await expect(privateGlobe.locator('.globe-pin-marker')).toHaveCount(0);
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
