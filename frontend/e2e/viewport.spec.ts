import { test, expect } from './fixtures';
import type { Browser, BrowserContextOptions } from '@playwright/test';
import { newAuthContext, signupViaUI, uniqueGroup } from './helpers';

test.describe('Viewport preservation', () => {
    test('landing page remains usable at narrow-phone and tablet widths', async ({ page }) => {
        for (const viewport of [
            { width: 320, height: 568 },
            { width: 768, height: 1024 },
        ]) {
            await page.setViewportSize(viewport);
            await page.goto('/');
            await expect(page.locator('.home-container')).toBeVisible();

            const metrics = await page.evaluate(() => ({
                clientWidth: document.documentElement.clientWidth,
                clientHeight: document.documentElement.clientHeight,
                scrollWidth: document.documentElement.scrollWidth,
                scrollHeight: document.documentElement.scrollHeight,
            }));
            expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
            expect(metrics.scrollHeight).toBeLessThanOrEqual(metrics.clientHeight);

            for (const link of [
                page.getByRole('link', { name: /get started/i }),
                page.getByRole('link', { name: /login/i }),
            ]) {
                const box = await link.boundingBox();
                expect(box).not.toBeNull();
                expect(box!.height).toBeGreaterThanOrEqual(44);
            }
        }
    });

    test('public home page fits one viewport without asset overflow', async ({ page }) => {
        await page.goto('/');
        await expect(page.locator('.home-container')).toBeVisible();
        const metrics = await page.evaluate(() => {
            const container = document.querySelector('.home-container')?.getBoundingClientRect();
            const asset = document.querySelector('.home-welcome-asset')?.getBoundingClientRect();
            const image = document.querySelector('.welcome-asset-img')?.getBoundingClientRect();
            return {
                clientWidth: document.documentElement.clientWidth,
                clientHeight: document.documentElement.clientHeight,
                scrollWidth: document.documentElement.scrollWidth,
                scrollHeight: document.documentElement.scrollHeight,
                container,
                asset,
                image,
            };
        });
        expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
        expect(metrics.scrollHeight).toBeLessThanOrEqual(metrics.clientHeight);
        expect(metrics.container).not.toBeNull();
        expect(metrics.asset).not.toBeNull();
        expect(metrics.image).not.toBeNull();
        expect(metrics.image!.left).toBeGreaterThanOrEqual(metrics.asset!.left - 1);
        expect(metrics.image!.right).toBeLessThanOrEqual(metrics.asset!.right + 1);
        expect(metrics.image!.top).toBeGreaterThanOrEqual(metrics.asset!.top - 1);
        expect(metrics.image!.bottom).toBeLessThanOrEqual(metrics.asset!.bottom + 1);
    });

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

test.describe('Group responsive overflow', () => {
    test('groups list page has no horizontal overflow at configured viewport', async ({ authenticatedPage }) => {
        await authenticatedPage.goto('/groups');
        await expect(authenticatedPage.locator('.groups-list-container')).toBeVisible();
        const noHorizontalOverflow = await authenticatedPage.evaluate(
            () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        );
        expect(noHorizontalOverflow).toBe(true);
    });

    test('groups list action buttons are visible within viewport', async ({ authenticatedPage }) => {
        await authenticatedPage.goto('/groups');
        const viewport = authenticatedPage.viewportSize();
        expect(viewport).not.toBeNull();
        const createBtn = authenticatedPage.getByRole('link', { name: 'Create Group' });
        await expect(createBtn).toBeVisible();
        const createBox = await createBtn.boundingBox();
        expect(createBox).not.toBeNull();
        expect(createBox!.x + createBox!.width).toBeLessThanOrEqual(viewport!.width);
        const joinBtn = authenticatedPage.getByRole('link', { name: 'Join Group' });
        await expect(joinBtn).toBeVisible();
        const joinBox = await joinBtn.boundingBox();
        expect(joinBox).not.toBeNull();
        expect(joinBox!.x + joinBox!.width).toBeLessThanOrEqual(viewport!.width);
    });

    test('group join page form fits within viewport', async ({ authenticatedPage }) => {
        await authenticatedPage.goto('/group/join');
        await expect(authenticatedPage.locator('.group-join-container')).toBeVisible();
        const noHorizontalOverflow = await authenticatedPage.evaluate(
            () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        );
        expect(noHorizontalOverflow).toBe(true);
        const form = authenticatedPage.locator('.join-form');
        await expect(form).toBeVisible();
        const formBox = await form.boundingBox();
        expect(formBox).not.toBeNull();
        const viewport = authenticatedPage.viewportSize();
        expect(viewport).not.toBeNull();
        expect(formBox!.x + formBox!.width).toBeLessThanOrEqual(viewport!.width);
    });

    test('group create page form fits within viewport', async ({ authenticatedPage }) => {
        await authenticatedPage.goto('/group/create');
        await expect(authenticatedPage.locator('.group-join-container')).toBeVisible();
        const form = authenticatedPage.locator('.join-form');
        await expect(form).toBeVisible();
        const formBox = await form.boundingBox();
        expect(formBox).not.toBeNull();
        const viewport = authenticatedPage.viewportSize();
        expect(viewport).not.toBeNull();
        expect(formBox!.x + formBox!.width).toBeLessThanOrEqual(viewport!.width);
    });

    test('group view layout has no horizontal overflow', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            const groupId = page.url().split('/group/')[1];
            await page.goto(`/group/${groupId}`);
            await expect(page.locator('.group-view')).toBeVisible();
            const noHorizontalOverflow = await page.evaluate(
                () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
            );
            expect(noHorizontalOverflow).toBe(true);
            // Header content must not overflow.
            const header = page.locator('.header-content');
            await expect(header).toBeVisible();
            const headerBox = await header.boundingBox();
            const viewport = page.viewportSize();
            expect(viewport).not.toBeNull();
            expect(headerBox).not.toBeNull();
            expect(headerBox!.width).toBeLessThanOrEqual(viewport!.width);

            const navigationBox = await page.locator('.tab-bar').boundingBox();
            expect(navigationBox).not.toBeNull();
            if (viewport!.width >= 768) {
                expect(navigationBox!.x).toBe(0);
                expect(navigationBox!.width).toBeLessThanOrEqual(100);
                expect(headerBox!.x).toBeGreaterThanOrEqual(navigationBox!.width);
            } else {
                expect(navigationBox!.x).toBe(0);
                expect(navigationBox!.width).toBeLessThanOrEqual(viewport!.width);
                expect(navigationBox!.y + navigationBox!.height).toBeLessThanOrEqual(viewport!.height + 1);
            }
        } finally {
            await context.close();
        }
    });

    test('group card with long name truncates without horizontal scroll', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            const longName = 'A_Very_Long_Group_Name_That_Should_Truncate_In_The_UI';
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(longName);
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await page.goto('/groups');
            await expect(page.locator('.groups-grid')).toBeVisible();
            const cardTitle = page.locator('.group-card .group-info h3').first();
            await expect(cardTitle).toBeVisible();
            const overflow = await cardTitle.evaluate((el) => {
                const style = window.getComputedStyle(el);
                return (
                    style.overflow === 'hidden' && style.textOverflow === 'ellipsis' && style.whiteSpace === 'nowrap'
                );
            });
            expect(overflow).toBe(true);
            const noHorizontalOverflow = await page.evaluate(
                () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
            );
            expect(noHorizontalOverflow).toBe(true);
        } finally {
            await context.close();
        }
    });

    test('group view header elements remain accessible at both viewports', async ({ browser, contextOptions }) => {
        const context = await newAuthContext(browser, contextOptions);
        try {
            const page = await context.newPage();
            await signupViaUI(page);
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expect(page.locator('.back-btn')).toBeVisible();
            await expect(page.locator('.header-logo')).toBeVisible();
            await expect(page.locator('.group-name')).toBeVisible();
            await expect(page.getByRole('button', { name: 'Open group settings' })).toBeVisible();
            const headerContent = page.locator('.header-content');
            const headerBox = await headerContent.boundingBox();
            const viewport = page.viewportSize();
            expect(viewport).not.toBeNull();
            expect(headerBox).not.toBeNull();
            expect(headerBox!.width).toBeLessThanOrEqual(viewport!.width);
        } finally {
            await context.close();
        }
    });
});
