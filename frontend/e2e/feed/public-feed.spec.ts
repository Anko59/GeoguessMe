import { test, expect } from '../support/fixtures';
import { newAuthContext, signupViaUI } from '../support/helpers';

test('a public post can be guessed, liked, and commented on by someone outside the author’s groups', async ({
    browser,
    contextOptions,
}) => {
    const ownerContext = await newAuthContext(browser, contextOptions);
    const viewerContext = await newAuthContext(browser, contextOptions);
    try {
        const owner = await ownerContext.newPage();
        const viewer = await viewerContext.newPage();
        await signupViaUI(owner);
        await signupViaUI(viewer);
        await owner.goto('/feed');
        await owner.getByRole('button', { name: '+ Post a challenge' }).click();
        const composer = owner.getByRole('dialog', { name: 'Post a geo challenge' });
        await composer.getByLabel('Challenge photo').setInputFiles({
            name: 'place.png',
            mimeType: 'image/png',
            buffer: Buffer.from(
                'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
                'base64',
            ),
        });
        await expect(composer.getByRole('img', { name: 'Photo to publish' })).toBeVisible();
        await composer.getByLabel('Caption').fill('A place worth discovering');
        await composer.getByLabel('Latitude').fill('48.8');
        await composer.getByLabel('Longitude').fill('2.3');
        await composer.getByRole('button', { name: 'Publish challenge' }).click();
        await expect(owner).toHaveURL(/\/feed\/[a-f0-9-]{36}$/);
        await expect(owner.getByText('Your challenge')).toBeVisible();
        const postURL = owner.url();
        await viewer.goto(postURL);
        await expect(viewer.getByAltText('Blurred preview of an unsolved geo challenge')).toBeVisible();
        await viewer.getByRole('button', { name: 'Play challenge' }).click();
        const game = viewer.getByRole('dialog', { name: 'Where in the world?' });
        await expect(game.getByAltText('Geo challenge photo')).toBeVisible();
        await game.getByLabel('Latitude').fill('48.8');
        await game.getByLabel('Longitude').fill('2.3');
        await game.getByRole('button', { name: 'Guess & reveal' }).click();
        await expect(viewer.getByText('5,000 points')).toBeVisible();
        await viewer.getByRole('button', { name: 'Back to the feed' }).click();
        await expect(viewer.getByText('✓ Revealed')).toBeVisible();
        await viewer.getByRole('button', { name: 'Like challenge' }).click();
        await expect(viewer.getByRole('button', { name: 'Unlike challenge' })).toHaveAttribute('aria-pressed', 'true');
        await viewer.getByRole('button', { name: '0 comments' }).click();
        await viewer.getByRole('textbox', { name: 'Add a comment' }).fill('Such a lovely place!');
        await viewer.getByRole('button', { name: 'Post comment' }).click();
        await expect(viewer.getByText('Such a lovely place!')).toBeVisible();
        await viewer.reload();
        await expect(viewer.getByText('✓ Revealed')).toBeVisible();
        await expect(viewer.getByRole('button', { name: 'Unlike challenge' })).toBeVisible();
        await owner.reload();
        await owner.getByRole('button', { name: '1 comment' }).click();
        await expect(owner.getByText('Such a lovely place!')).toBeVisible();
        await owner.getByRole('button', { name: 'Delete post' }).click();
        await owner.getByRole('button', { name: 'Confirm delete' }).click();
        await expect(owner.getByText('This challenge has been removed')).toBeVisible();
    } finally {
        await ownerContext.close();
        await viewerContext.close();
    }
});
