import { expect, type Page } from '@playwright/test';

export const EXPECTED_LENS_ASSETS = [
    '/lenses/generated/disco-outlaw.webp',
    '/lenses/generated/red-flag-royalty.webp',
    '/lenses/generated/bad-decisions.webp',
    '/lenses/generated/hr-nightmare.webp',
    '/lenses/generated/toxic-ex.webp',
    '/lenses/generated/tax-fraud.webp',
    '/lenses/jeeliz-dog/dog_ears.geometry',
    '/lenses/jeeliz-dog/dog_nose.geometry',
    '/lenses/jeeliz-dog/texture_ears.jpg',
    '/lenses/jeeliz-dog/texture_nose.jpg',
];

export async function expectUserCameraOrientation(page: Page): Promise<void> {
    await expect(page.locator('.camera-video')).toHaveCSS('transform', 'matrix(-1, 0, 0, 1, 0, 0)');
    await expect(page.locator('.camera-filter-overlay')).toHaveCSS('transform', 'matrix(-1, 0, 0, 1, 0, 0)');
}

export async function expectNaturalCameraOrientation(page: Page): Promise<void> {
    await expect(page.locator('.camera-video')).toHaveCSS('transform', 'none');
    await expect(page.locator('.camera-filter-overlay')).toHaveCSS('transform', 'none');
}
