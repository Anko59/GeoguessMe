import { test, expect } from '../support/fixtures';
import {
    installDeterministicCamera,
    installDeterministicGeolocation,
    newAuthContext,
    signupViaUI,
    signupWithToken,
} from '../support/helpers';
import { cameraOptions } from '../support/challengeScenario';
import { captureAudienceControls, captureFeedState, expectPhotoDecoded, installMapTiles } from './visual-support';

test('a public post can be guessed, liked, and commented on by someone outside the author’s groups', async ({
    browser,
    contextOptions,
}, testInfo) => {
    const ownerContext = await newAuthContext(browser, cameraOptions(contextOptions));
    const viewerContext = await newAuthContext(browser, contextOptions);
    let publishedID: string | undefined;
    let ownerToken = '';
    try {
        await Promise.all([
            installMapTiles(ownerContext),
            installMapTiles(viewerContext),
            installDeterministicCamera(ownerContext),
            installDeterministicGeolocation(ownerContext),
        ]);
        await ownerContext.addInitScript(() => {
            const append = FormData.prototype.append;
            FormData.prototype.append = function (name, value, filename) {
                if (name === 'lat' || name === 'long') {
                    const windowWithCapture = window as Window & {
                        __feedLocationFields?: Record<string, string>;
                    };
                    windowWithCapture.__feedLocationFields ??= {};
                    windowWithCapture.__feedLocationFields[name] = String(value);
                }
                if (filename === undefined) return append.call(this, name, value);
                return append.call(this, name, value, filename);
            };
        });
        const ownerSignup = await signupWithToken(ownerContext);
        const owner = ownerSignup.page;
        ownerToken = ownerSignup.token;
        const viewer = await viewerContext.newPage();
        await signupViaUI(viewer);
        await owner.goto('/feed');
        await expect(owner.getByText('The world is waiting for your first post')).toBeVisible();
        await captureFeedState(owner, testInfo, '00-empty-feed');
        await owner.getByRole('button', { name: '+ Post a challenge' }).click();
        const composer = owner.getByRole('dialog', { name: 'Post a geo challenge' });
        await composer.getByLabel('Caption').fill('A place worth discovering');
        await expect(composer.locator('input[type="file"]')).toHaveCount(0);
        await expect(composer.getByLabel('Latitude')).toHaveCount(0);
        await expect(composer.getByRole('button', { name: 'Take photo' })).toBeVisible();
        await composer.getByRole('button', { name: 'Take photo' }).click();
        await expect(composer.locator('.preview-image')).toBeVisible();
        await expect(composer.getByRole('radio', { name: 'Everyone on GeoGuessMe' })).toBeChecked();
        await captureFeedState(owner, testInfo, '01-composer', composer);
        await captureAudienceControls(owner, testInfo, composer);
        const [publication] = await Promise.all([
            owner.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/feed/challenges') && response.request().method() === 'POST',
            ),
            composer.getByRole('button', { name: 'Send' }).click(),
        ]);
        expect(publication.ok()).toBe(true);
        publishedID = (await publication.json()).id;
        await expect
            .poll(() =>
                owner.evaluate(() => {
                    const windowWithCapture = window as Window & {
                        __feedLocationFields?: Record<string, string>;
                    };
                    return windowWithCapture.__feedLocationFields ?? {};
                }),
            )
            .toEqual({ lat: '48.8566', long: '2.3522' });
        await expect(owner).toHaveURL(/\/feed\/[a-f0-9-]{36}$/);
        await expect(owner.getByText('Your challenge')).toBeVisible();
        await expectPhotoDecoded(owner.getByAltText('Geo challenge photo'));
        await captureFeedState(owner, testInfo, '02-authored-post');
        const postURL = owner.url();
        await viewer.goto(postURL);
        const blurred = viewer.getByAltText('Blurred preview of an unsolved geo challenge');
        await expectPhotoDecoded(blurred);
        await expect(blurred).toHaveCSS('filter', 'blur(12px) brightness(0.85)');
        await captureFeedState(viewer, testInfo, '03-blurred-challenge');
        const accept = viewer.waitForResponse(
            (response) =>
                response.url().endsWith('/api/v1/feed/challenges/' + publishedID + '/accept') &&
                response.request().method() === 'POST',
        );
        const timedMedia = viewer.waitForResponse(
            (response) =>
                response.url().endsWith('/api/v1/feed/challenges/' + publishedID + '/timed-media') &&
                response.request().method() === 'GET',
        );
        const delivered = viewer.waitForResponse(
            (response) =>
                response.url().endsWith('/api/v1/feed/challenges/' + publishedID + '/media-delivered') &&
                response.request().method() === 'POST',
        );
        await viewer.getByRole('button', { name: 'Play challenge' }).click();
        expect((await accept).ok()).toBe(true);
        expect((await timedMedia).ok()).toBe(true);
        const game = viewer.getByRole('dialog', { name: 'Challenge photo' });
        await expectPhotoDecoded(game.getByAltText('Challenge location'));
        // The test stack deliberately uses a one-second viewing window. Assert
        // the decoded image before doing any further response bookkeeping so
        // the test cannot consume the entire server-owned viewing window.
        expect((await delivered).ok()).toBe(true);
        await expect(viewer.getByRole('dialog', { name: 'Challenge guessing' })).toBeVisible();
        const guessing = viewer.getByRole('dialog', { name: 'Challenge guessing' });
        await captureFeedState(viewer, testInfo, '04-guess-dialog', guessing);
        await guessing.locator('.leaflet-container').click({ position: { x: 200, y: 150 } });
        const [guess] = await Promise.all([
            viewer.waitForResponse(
                (response) => response.url().endsWith('/timed-guess') && response.request().method() === 'POST',
            ),
            guessing.getByRole('button', { name: /Submit guess/ }).click(),
        ]);
        expect(guess.ok()).toBe(true);
        const result = await guess.json();
        expect(result.score).toBeLessThan(5000);
        expect(result.distance).toBeGreaterThan(1_000_000);
        await expect(viewer.getByText(/\d[\d,]* (?:points|pts)/)).toBeVisible();
        await captureFeedState(
            viewer,
            testInfo,
            '05-guess-result',
            viewer.getByRole('dialog', { name: 'Challenge results' }),
        );
        await viewer.getByRole('button', { name: 'Close' }).click();
        await expect(viewer.getByText('✓ Revealed')).toBeVisible();
        const revealed = viewer.getByAltText('Geo challenge photo');
        await expectPhotoDecoded(revealed);
        await expect(revealed).toHaveCSS('filter', 'none');
        await captureFeedState(viewer, testInfo, '06-revealed-challenge');
        await viewer.getByRole('button', { name: 'Like challenge' }).click();
        await expect(viewer.getByRole('button', { name: 'Unlike challenge' })).toHaveAttribute('aria-pressed', 'true');
        await viewer.getByRole('button', { name: '0 comments' }).click();
        await viewer.getByRole('textbox', { name: 'Add a comment' }).fill('Such a lovely place!');
        await viewer.getByRole('button', { name: 'Post comment' }).click();
        await expect(viewer.getByText('Such a lovely place!')).toBeVisible();
        await captureFeedState(viewer, testInfo, '07-reaction-and-comments');
        await viewer.reload();
        await expect(viewer.getByText('✓ Revealed')).toBeVisible();
        await expect(viewer.getByRole('button', { name: 'Unlike challenge' })).toBeVisible();
        await owner.reload();
        await owner.getByRole('button', { name: '1 comment' }).click();
        await expect(owner.getByText('Such a lovely place!')).toBeVisible();
        await owner.getByRole('button', { name: 'Delete post' }).click();
        await owner.getByRole('button', { name: 'Confirm delete' }).click();
        await expect(owner.getByText('This challenge has been removed')).toBeVisible();
        publishedID = undefined;
    } finally {
        try {
            if (publishedID) {
                const cleanup = await ownerContext.request.delete(`/api/v1/feed/challenges/${publishedID}`, {
                    headers: { Authorization: `Bearer ${ownerToken}` },
                });
                expect([204, 404]).toContain(cleanup.status());
            }
        } finally {
            await ownerContext.close();
            await viewerContext.close();
        }
    }
});
