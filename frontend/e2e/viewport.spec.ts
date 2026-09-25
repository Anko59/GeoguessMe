import { test, expect } from './support/fixtures';
import type { Browser, BrowserContextOptions } from '@playwright/test';
import { newAuthContext, signupViaUI, uniqueGroup } from './support/helpers';

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

    test('authenticated page shell keeps top navigation geometry consistent', async ({ authenticatedPage }) => {
        const geometry: Array<{ left: number; top: number; width: number; height: number }> = [];

        for (const path of ['/feed', '/groups', '/profile', '/settings']) {
            await authenticatedPage.goto(path);
            const navigation = authenticatedPage.locator('.authenticated-page-shell > .app-topbar');
            await expect(navigation).toBeVisible();
            const box = await navigation.boundingBox();
            expect(box).not.toBeNull();
            geometry.push({
                left: Math.round(box!.left),
                top: Math.round(box!.top),
                width: Math.round(box!.width),
                height: Math.round(box!.height),
            });
        }

        expect(geometry).toEqual([geometry[0], geometry[0], geometry[0], geometry[0]]);
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

    test('groups and settings content keeps side gutters while top navigation geometry stays stable', async ({
        authenticatedPage,
    }) => {
        for (const viewport of [
            { width: 320, height: 700 },
            { width: 1280, height: 720 },
        ]) {
            await authenticatedPage.setViewportSize(viewport);
            await authenticatedPage.goto('/groups');
            await expect(authenticatedPage.locator('.groups-actions')).toBeVisible();
            await expect(authenticatedPage.locator('.empty-state')).toBeVisible();

            const groupsGeometry = await authenticatedPage.evaluate(() => {
                const bounds = (selector: string) => {
                    const element = document.querySelector(selector);
                    if (!element) return null;
                    const { left, right } = element.getBoundingClientRect();
                    return { left, right };
                };
                const navigation = document.querySelector('.authenticated-page-shell > .app-topbar');
                if (!navigation) return null;
                const { left, right, width } = navigation.getBoundingClientRect();
                return {
                    navigation: { left, right, width },
                    actions: bounds('.groups-actions'),
                    emptyState: bounds('.empty-state'),
                };
            });
            expect(groupsGeometry).not.toBeNull();
            expect(groupsGeometry!.actions).not.toBeNull();
            expect(groupsGeometry!.emptyState).not.toBeNull();
            for (const content of [groupsGeometry!.actions!, groupsGeometry!.emptyState!]) {
                expect(content.left).toBeGreaterThanOrEqual(12);
                expect(content.right).toBeLessThanOrEqual(viewport.width - 12);
            }
            const expectedNavigationWidth = Math.min(viewport.width, 70 * 16);
            expect(groupsGeometry!.navigation.width).toBe(expectedNavigationWidth);
            expect(groupsGeometry!.navigation.left).toBe((viewport.width - expectedNavigationWidth) / 2);

            await authenticatedPage.goto('/settings');
            await expect(authenticatedPage.locator('.account-settings-card')).toBeVisible();
            const settingsGeometry = await authenticatedPage.evaluate(() => {
                const card = document.querySelector('.account-settings-card');
                const navigation = document.querySelector('.authenticated-page-shell > .app-topbar');
                if (!card || !navigation) return null;
                const cardBounds = card.getBoundingClientRect();
                const navigationBounds = navigation.getBoundingClientRect();
                const cardStyles = getComputedStyle(card);
                return {
                    card: { left: cardBounds.left, right: cardBounds.right },
                    backgroundColor: cardStyles.backgroundColor,
                    borderRadius: cardStyles.borderRadius,
                    navigation: {
                        left: navigationBounds.left,
                        right: navigationBounds.right,
                        width: navigationBounds.width,
                    },
                };
            });
            expect(settingsGeometry).not.toBeNull();
            expect(settingsGeometry!.card.left).toBeGreaterThanOrEqual(12);
            expect(settingsGeometry!.card.right).toBeLessThanOrEqual(viewport.width - 12);
            expect(settingsGeometry!.backgroundColor).not.toBe('rgba(0, 0, 0, 0)');
            expect(settingsGeometry!.borderRadius).not.toBe('0px');
            expect(settingsGeometry!.navigation).toEqual(groupsGeometry!.navigation);

            const documentHeight = await authenticatedPage.evaluate(() => document.documentElement.scrollHeight);
            expect(documentHeight).toBeGreaterThan(viewport.height);
            const footer = authenticatedPage.locator('.account-footer-actions');
            await footer.scrollIntoViewIfNeeded();
            const footerBounds = await footer.boundingBox();
            expect(footerBounds).not.toBeNull();
            expect(footerBounds!.y).toBeGreaterThanOrEqual(0);
            expect(footerBounds!.y + footerBounds!.height).toBeLessThanOrEqual(viewport.height);
        }
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
        expect(formBox!.x).toBeGreaterThanOrEqual(12);
        expect(formBox!.x + formBox!.width).toBeLessThanOrEqual(viewport!.width - 12);
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
        expect(formBox!.x).toBeGreaterThanOrEqual(12);
        expect(formBox!.x + formBox!.width).toBeLessThanOrEqual(viewport!.width - 12);
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
