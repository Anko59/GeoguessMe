import { expect, type BrowserContext, type Locator, type Page, type TestInfo } from '@playwright/test';

// Keep tile delivery deterministic while exercising real Leaflet rendering,
// sizing, zooming, and markers. Application API and media requests stay live.
export async function installMapTiles(context: BrowserContext): Promise<void> {
    await context.route('https://*.tile.openstreetmap.org/**', (route) =>
        route.fulfill({
            contentType: 'image/svg+xml',
            body: `<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">
            <rect width="256" height="256" fill="#e9e7df"/>
            <path d="M0 180L256 125L256 160L0 215Z" fill="#b7d9e6"/>
            <path d="M30 0V256M160 0V256M0 55H256M0 115H256M0 240H256" stroke="#fff" stroke-width="12"/>
            <path d="M30 0V256M160 0V256M0 55H256M0 115H256M0 240H256" stroke="#d2caba" stroke-width="2"/>
            <path d="M48 10H105V37H48ZM176 70H234V99H176ZM50 70H130V99H50Z" fill="#d7cec2"/>
            <rect x="182" y="8" width="55" height="30" rx="8" fill="#bed5b2"/>
        </svg>`,
        }),
    );
}

export async function expectPhotoDecoded(photo: Locator): Promise<void> {
    await expect(photo).toBeVisible();
    await expect
        .poll(() => photo.evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0))
        .toBe(true);
}

async function expectMapFilled(dialog: Locator): Promise<void> {
    const map = dialog.locator('.leaflet-container');
    await expect
        .poll(() =>
            map.evaluate((element) => {
                const bounds = element.getBoundingClientRect();
                const tiles = Array.from(element.querySelectorAll('.leaflet-tile-loaded'), (tile) =>
                    tile.getBoundingClientRect(),
                );
                const corners = [
                    [bounds.left + 1, bounds.top + 1],
                    [bounds.right - 1, bounds.top + 1],
                    [bounds.left + 1, bounds.bottom - 1],
                    [bounds.right - 1, bounds.bottom - 1],
                ];
                return corners.every(([x, y]) =>
                    tiles.some((tile) => tile.left <= x && tile.right >= x && tile.top <= y && tile.bottom >= y),
                );
            }),
        )
        .toBe(true);
}

export async function captureFeedState(page: Page, testInfo: TestInfo, name: string, dialog?: Locator): Promise<void> {
    if (!dialog) {
        await expect(page.getByRole('region', { name: 'Group inbox' }).getByText('No groups yet.')).toBeVisible();
    }
    await expect(page.getByRole('alert')).toHaveCount(0);
    await page.evaluate(() => document.fonts.ready);
    expect(page.viewportSize()).toEqual(testInfo.project.use.viewport);
    // Layout assertions fail automatically; the attached image provides the
    // corresponding visual evidence in the HTML report on every successful run.
    expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth),
    ).toBe(true);
    if (dialog) {
        await expect(dialog).toBeVisible();
        await expectMapFilled(dialog);
        await dialog.evaluate((element) => {
            element.scrollTop = 0;
        });
        const bounds = await dialog.boundingBox();
        const viewport = page.viewportSize();
        expect(bounds).not.toBeNull();
        expect(viewport).not.toBeNull();
        expect(bounds!.x).toBeGreaterThanOrEqual(0);
        expect(bounds!.y).toBeGreaterThanOrEqual(0);
        expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport!.width);
        expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport!.height);
    }
    const path = testInfo.outputPath(`${name}.png`);
    await page.screenshot({ path, fullPage: !dialog, animations: 'disabled', caret: 'hide', scale: 'css' });
    await testInfo.attach(name, { path, contentType: 'image/png' });
    if (dialog && (await dialog.evaluate((element) => element.scrollHeight > element.clientHeight))) {
        await dialog.evaluate((element) => {
            element.scrollTop = element.scrollHeight;
        });
        const controls = testInfo.outputPath(`${name}-controls.png`);
        await page.screenshot({ path: controls, animations: 'disabled', caret: 'hide', scale: 'css' });
        await testInfo.attach(`${name}-controls`, { path: controls, contentType: 'image/png' });
    }
}

export async function captureAudienceControls(page: Page, testInfo: TestInfo, composer: Locator): Promise<void> {
    const audience = composer.getByRole('group', { name: 'Who can see this challenge?' });
    await audience.scrollIntoViewIfNeeded();
    const radios = audience.getByRole('radio');
    await expect(radios).toHaveCount(2);
    for (const radio of await radios.all()) {
        await expect(radio).toBeInViewport();
        const bounds = await radio.boundingBox();
        expect(bounds).not.toBeNull();
        expect(bounds!.width).toBeLessThanOrEqual(24);
        expect(bounds!.height).toBeLessThanOrEqual(24);
        const labelHeight = await radio.evaluate(
            (control) => control.closest('label')?.getBoundingClientRect().height ?? 0,
        );
        expect(labelHeight).toBeGreaterThanOrEqual(44);
    }
    const path = testInfo.outputPath('01-audience.png');
    await page.screenshot({ path, animations: 'disabled', caret: 'hide', scale: 'css' });
    await testInfo.attach('01-audience', { path, contentType: 'image/png' });
}
