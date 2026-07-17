import { test, expect, type Page } from '@playwright/test';
import { uniqueGroup, signupViaUI, newAuthContext } from './helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Challenge flow', () => {
    let uploaderPage: Page;
    let guesserPage: Page;
    let groupId: string;
    let groupCode: string;

    test.beforeAll(async ({ browser }) => {
        // Uploader signs up, creates group
        const ctxU = await newAuthContext(browser);
        uploaderPage = await ctxU.newPage();

        // Camera mocking: replace getUserMedia with a canvas-based fake stream
        await uploaderPage.addInitScript(() => {
            const canvas = document.createElement('canvas');
            canvas.width = 320;
            canvas.height = 240;
            const ctx = canvas.getContext('2d')!;
            ctx.fillStyle = '#4A90D9';
            ctx.fillRect(0, 0, 320, 240);
            ctx.fillStyle = '#FFFFFF';
            ctx.font = '20px sans-serif';
            ctx.fillText('TEST', 120, 120);
            const stream = canvas.captureStream(30);

            // Override getUserMedia at the instance level (writable in Chromium)
            if (navigator.mediaDevices) {
                navigator.mediaDevices.getUserMedia = () => Promise.resolve(stream);
            }
        });

        await signupViaUI(uploaderPage);

        await uploaderPage.goto('/group/create');
        await uploaderPage.fill('input[placeholder="Group Name"]', uniqueGroup());
        await uploaderPage.click('button[type="submit"]');
        await uploaderPage.waitForURL(/\/group\//, { timeout: 10000 });
        groupId = uploaderPage.url().match(/\/group\/(.+)/)![1];

        await uploaderPage.click('.settings-btn');
        await uploaderPage.waitForSelector('.modal-content', { state: 'visible' });
        groupCode = (await uploaderPage.locator('.modal-content .group-code').textContent()) ?? '';
        await uploaderPage.click('.modal-close');

        // Guesser signs up and joins
        const ctxG = await newAuthContext(browser);
        guesserPage = await ctxG.newPage();
        await signupViaUI(guesserPage);

        await guesserPage.goto('/group/join');
        await guesserPage.fill('input[placeholder="6-character code"]', groupCode);
        await guesserPage.click('button[type="submit"]');
        await guesserPage.waitForURL(/\/group\//, { timeout: 10000 });
    });

    test('upload a challenge, guesser accepts, sees media, guesses after window, scores shown', async () => {
        // Both users navigate to the group
        await uploaderPage.goto(`/group/${groupId}`);
        await guesserPage.goto(`/group/${groupId}`);

        // Wait for both to be connected
        await expect(uploaderPage.locator('.chat-status')).toHaveText('Connected', { timeout: 10000 });
        await expect(guesserPage.locator('.chat-status')).toHaveText('Connected', { timeout: 10000 });

        // Uploader switches to Camera tab
        await uploaderPage.click('.tab:nth-child(2)');
        await uploaderPage.waitForSelector('.camera-container', { state: 'visible' });

        // Wait for camera to be ready
        await expect(uploaderPage.locator('.capture-button')).toBeVisible({ timeout: 10000 });

        // Take a photo
        await uploaderPage.click('.capture-button');
        await expect(uploaderPage.locator('.preview-image')).toBeVisible({ timeout: 5000 });

        // Send the photo (upload with geolocation)
        await uploaderPage.click('button:has-text("Send")');
        // After upload, we switch back to chat tab
        await expect(uploaderPage.locator('.message-container').last()).toContainText('Challenge sent', { timeout: 15000 });

        // Guesser should see the challenge message
        await expect(guesserPage.locator('.message-content.photo-challenge')).toBeVisible({ timeout: 15000 });

        // Guesser clicks to accept the challenge
        await guesserPage.locator('.message-content.photo-challenge').click();

        // Should see the viewing window (photo displayed with timer)
        await expect(guesserPage.locator('.photo-view')).toBeVisible({ timeout: 10000 });
        await expect(guesserPage.locator('.game-photo')).toBeVisible();

        // Wait for the view window to expire (1s in test stack + buffer)
        await guesserPage.waitForSelector('.guessing-view', { timeout: 10000 });

        // Place a guess on the map
        await guesserPage.waitForSelector('.guess-button', { state: 'visible' });

        // Click on the map to place a marker (center of map)
        const map = guesserPage.locator('.leaflet-container');
        await map.click({ position: { x: 200, y: 150 } });

        // Submit guess
        await guesserPage.click('.guess-button:not([disabled])');
        await expect(guesserPage.locator('.guess-button')).toBeDisabled({ timeout: 3000 });

        // Should see the results view
        await expect(guesserPage.locator('.result-view')).toBeVisible({ timeout: 15000 });
        await expect(guesserPage.locator('.result-view')).toContainText('Challenge results');

        // Uploader should also see results
        // Uploader's own challenge message should now say "View results"
        await expect(uploaderPage.locator('.message-container').last()).toContainText('View results', { timeout: 15000 });
    });

    test('duplicate guess is idempotent', async ({ browser }) => {
        // Create a fresh two-user setup for an isolated challenge
        const ctxU2 = await newAuthContext(browser);
        const uploader2 = await ctxU2.newPage();
        await uploader2.addInitScript(() => {
            const canvas = document.createElement('canvas');
            canvas.width = 320;
            canvas.height = 240;
            const ctx = canvas.getContext('2d')!;
            ctx.fillStyle = '#E74C3C';
            ctx.fillRect(0, 0, 320, 240);
            const stream = canvas.captureStream(30);
            if (navigator.mediaDevices) {
                navigator.mediaDevices.getUserMedia = () => Promise.resolve(stream);
            }
        });
        await signupViaUI(uploader2);

        const ctxG2 = await newAuthContext(browser);
        const guesser2 = await ctxG2.newPage();
        await signupViaUI(guesser2);

        // Create fresh group for this test
        await uploader2.goto('/group/create');
        await uploader2.fill('input[placeholder="Group Name"]', uniqueGroup());
        await uploader2.click('button[type="submit"]');
        await uploader2.waitForURL(/\/group\//, { timeout: 10000 });
        const g2Id = uploader2.url().match(/\/group\/(.+)/)![1];

        await uploader2.click('.settings-btn');
        await uploader2.waitForSelector('.modal-content', { state: 'visible' });
        const g2Code = (await uploader2.locator('.modal-content .group-code').textContent()) ?? '';
        await uploader2.click('.modal-close');

        await guesser2.goto('/group/join');
        await guesser2.fill('input[placeholder="6-character code"]', g2Code);
        await guesser2.click('button[type="submit"]');
        await guesser2.waitForURL(/\/group\//, { timeout: 10000 });

        // Both on group view
        await uploader2.goto(`/group/${g2Id}`);
        await guesser2.goto(`/group/${g2Id}`);
        await expect(uploader2.locator('.chat-status')).toHaveText('Connected', { timeout: 10000 });
        await expect(guesser2.locator('.chat-status')).toHaveText('Connected', { timeout: 10000 });

        // Upload challenge
        await uploader2.click('.tab:nth-child(2)');
        await expect(uploader2.locator('.capture-button')).toBeVisible({ timeout: 10000 });
        await uploader2.click('.capture-button');
        await expect(uploader2.locator('.preview-image')).toBeVisible({ timeout: 5000 });
        await uploader2.click('button:has-text("Send")');
        await expect(guesser2.locator('.message-content.photo-challenge')).toBeVisible({ timeout: 15000 });

        // Accept
        await guesser2.locator('.message-content.photo-challenge').click();
        await expect(guesser2.locator('.photo-view')).toBeVisible({ timeout: 10000 });

        // Wait for guessing view
        await guesser2.waitForSelector('.guessing-view', { timeout: 10000 });

        // Place guess and submit
        const map = guesser2.locator('.leaflet-container');
        await map.click({ position: { x: 250, y: 180 } });
        await guesser2.click('.guess-button:not([disabled])');

        // Wait for first guess to complete (results view)
        await expect(guesser2.locator('.result-view')).toBeVisible({ timeout: 15000 });

        // Close the results view
        await guesser2.locator('.next-button').click();

        // Wait for challenge message to reappear (now "View results")
        await expect(guesser2.locator('.message-content.photo-challenge')).toBeVisible({ timeout: 10000 });

        // Click it again — should show results without error (idempotent)
        await guesser2.locator('.message-content.photo-challenge').click();
        await expect(guesser2.locator('.result-view')).toBeVisible({ timeout: 15000 });
    });
});
