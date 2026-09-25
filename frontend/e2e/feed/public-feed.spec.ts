import { test, expect } from '../support/fixtures';
import {
    installDeterministicCamera,
    installDeterministicGeolocation,
    newAuthContext,
    signupViaUI,
    signupWithToken,
} from '../support/helpers';
import { cameraOptions } from '../support/challengeScenario';
import {
    captureAudienceControls,
    captureFeedState,
    expectFeedDiscussionReachableAtViewports,
    expectPhotoDecoded,
    installMapTiles,
} from './visual-support';

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
        await expect(composer.locator('input[type="file"]')).toHaveCount(0);
        await expect(composer.getByLabel('Latitude')).toHaveCount(0);
        await expect(composer.getByRole('button', { name: 'Take photo' })).toBeVisible();
        await composer.getByRole('button', { name: 'Take photo' }).click();
        await expect(composer.locator('.preview-image')).toBeVisible();
        await composer.getByRole('button', { name: 'Challenge options' }).click();
        await composer.getByRole('textbox', { name: 'Caption (optional)' }).fill('A place worth discovering');
        await expect(composer.getByRole('radio', { name: 'Public feed' })).toBeChecked();
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
        const ownerResults = owner.getByRole('dialog', { name: 'Challenge results' });
        await expect(ownerResults).toBeVisible();
        await expectPhotoDecoded(ownerResults.getByAltText('Challenge location'));
        await captureFeedState(owner, testInfo, '02-authored-post', ownerResults);
        const postURL = owner.url();
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
        await viewer.goto(postURL);
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
        await captureFeedState(viewer, testInfo, '03-guess-dialog', guessing);
        const guessMap = guessing.locator('.leaflet-container');
        const mapBounds = await guessMap.boundingBox();
        expect(mapBounds).not.toBeNull();
        await guessMap.click({
            position: { x: mapBounds!.width * 0.15, y: mapBounds!.height * 0.85 },
        });
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
            '04-guess-result',
            viewer.getByRole('dialog', { name: 'Challenge results' }),
        );
        await viewer.getByRole('button', { name: 'Close' }).click();
        await expect(viewer.getByText('✓ Revealed')).toBeVisible();
        const revealed = viewer.getByAltText('Geo challenge photo');
        await expectPhotoDecoded(revealed);
        await expect(revealed).toHaveCSS('filter', 'none');
        await captureFeedState(viewer, testInfo, '05-revealed-challenge');
        await viewer.getByRole('button', { name: 'Like challenge' }).click();
        await expect(viewer.getByRole('button', { name: 'Unlike challenge' })).toHaveAttribute('aria-pressed', 'true');
        await viewer.getByRole('button', { name: '0 comments' }).click();
        await viewer.getByRole('textbox', { name: 'Add a comment' }).fill('Such a lovely place!');
        await viewer.getByRole('button', { name: 'Post comment' }).click();
        await expect(viewer.getByText('Such a lovely place!')).toBeVisible();
        const extraComments = [
            'A lovely view.',
            'I would visit again.',
            'The light is beautiful.',
            'Thanks for sharing.',
            'One more great memory.',
        ];
        for (const comment of extraComments) {
            await viewer.getByRole('textbox', { name: 'Add a comment' }).fill(comment);
            await viewer.getByRole('button', { name: 'Post comment' }).click();
            await expect(viewer.getByText(comment)).toBeVisible();
        }
        const article = viewer.getByRole('article', { name: /Geo challenge by/ });
        await expect(article.getByRole('button', { name: 'Unlike challenge' })).toHaveAttribute('aria-pressed', 'true');
        await expect(viewer.getByRole('dialog')).toHaveCount(0);
        const author = article.locator('.feed-author');
        const authorBounds = await author.boundingBox();
        expect(authorBounds).not.toBeNull();
        await author.click({ position: { x: authorBounds!.width - 2, y: authorBounds!.height / 2 } });
        const results = viewer.getByRole('dialog', { name: 'Challenge results' });
        await expect(results).toBeVisible();
        await expect(results.getByText('Such a lovely place!')).toBeVisible();
        await expect(results.getByText('One more great memory.')).toBeVisible();
        await captureFeedState(viewer, testInfo, '06-result-discussion', results);
        await expectFeedDiscussionReachableAtViewports(viewer, results, 'One more great memory.');
        await viewer.reload();
        const reopenedCard = viewer.getByRole('article', { name: /Geo challenge by/ });
        await expect(reopenedCard).toBeVisible();
        const reopenedAuthor = reopenedCard.locator('.feed-author');
        const reopenedAuthorBounds = await reopenedAuthor.boundingBox();
        expect(reopenedAuthorBounds).not.toBeNull();
        await reopenedAuthor.click({
            position: { x: reopenedAuthorBounds!.width - 2, y: reopenedAuthorBounds!.height / 2 },
        });
        const reopenedResults = viewer.getByRole('dialog', { name: 'Challenge results' });
        await expect(reopenedResults).toBeVisible();
        await expect(reopenedResults.getByRole('button', { name: 'Unlike challenge' })).toBeVisible();
        await expect(reopenedResults.locator('.feed-comments li')).toHaveCount(6);
        await owner.goto(postURL);
        const ownResults = owner.getByRole('dialog', { name: 'Challenge results' });
        await expect(ownResults).toBeVisible();
        await expect(ownResults.getByText('Such a lovely place!')).toBeVisible();
        await expect(ownResults.locator('.feed-comments li')).toHaveCount(6);
        await owner.goto('/feed');
        await owner.getByRole('button', { name: 'Delete post' }).click();
        await owner.getByRole('button', { name: 'Confirm delete' }).click();
        await expect(owner.getByText('The world is waiting for your first post')).toBeVisible();
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
