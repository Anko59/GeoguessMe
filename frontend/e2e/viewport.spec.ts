import { test, expect } from './fixtures';

test.describe('Viewport preservation', () => {
    test('page inherits exact project viewport', async ({ page }) => {
        const viewport = page.viewportSize();
        expect(viewport).not.toBeNull();

        const projectName = test.info().project.name;
        if (projectName === 'desktop') {
            expect(viewport!.width).toBe(1280);
            expect(viewport!.height).toBe(720);
        } else if (projectName === 'mobile') {
            expect(viewport!.width).toBe(393);
            expect(viewport!.height).toBe(727);
        }
    });

    test('authenticated context preserves exact project viewport', async ({ authenticatedPage }) => {
        const viewport = authenticatedPage.viewportSize();
        expect(viewport).not.toBeNull();

        const projectName = test.info().project.name;
        if (projectName === 'desktop') {
            expect(viewport!.width).toBe(1280);
            expect(viewport!.height).toBe(720);
        } else if (projectName === 'mobile') {
            expect(viewport!.width).toBe(393);
            expect(viewport!.height).toBe(727);
        }
    });

    test('mobile project has touch and mobile user-agent', async ({ page }) => {
        test.skip(test.info().project.name !== 'mobile', 'desktop project — skipped');

        const userAgent = await page.evaluate(() => navigator.userAgent);
        expect(userAgent).toMatch(/Mobile|Android/);

        const maxTouchPoints = await page.evaluate(() => navigator.maxTouchPoints);
        expect(maxTouchPoints).toBeGreaterThan(0);
    });

    test('mobile project has geolocation and camera permissions granted', async ({ page }) => {
        test.skip(test.info().project.name !== 'mobile', 'desktop project — skipped');

        const geoPerm = await page.evaluate(() =>
            navigator.permissions.query({ name: 'geolocation' }).then((s) => s.state),
        );
        expect(geoPerm).toBe('granted');

        const camPerm = await page.evaluate(() => navigator.permissions.query({ name: 'camera' }).then((s) => s.state));
        expect(camPerm).toBe('granted');
    });

    test('mobile geolocation is configured on the context', async ({ page, context }) => {
        test.skip(test.info().project.name !== 'mobile', 'desktop project — skipped');

        // setGeolocation must succeed and permissions must be granted.
        await context.grantPermissions(['geolocation']);
        await expect(context.setGeolocation({ latitude: 48.8566, longitude: 2.3522 })).resolves.toBeUndefined();

        const geoPerm = await page.evaluate(() =>
            navigator.permissions.query({ name: 'geolocation' }).then((s) => s.state),
        );
        expect(geoPerm).toBe('granted');
    });
});
