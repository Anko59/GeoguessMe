import { test, expect } from './fixtures';
import { expectUserCameraOrientation } from './cameraAssertions';
import { closeScenario, createScenario } from './challengeScenario';

test.describe('Video challenge flow', () => {
    test('records a playable video, uploads it, and serves it for playback', async ({ browser, contextOptions }) => {
        // Async media processing (claim -> ffprobe validate -> ffmpeg transcode
        // -> canonical write -> record+broadcast) plus the 60s ready wait must
        // fit inside the test budget; the suite default is 30s.
        test.setTimeout(120000);
        const scenario = await createScenario(browser, contextOptions);
        try {
            const { uploader, guesser } = scenario;
            await uploader.getByRole('button', { name: 'Camera' }).click();
            const captureButton = uploader.locator('.capture-button');
            await expect(captureButton).toBeVisible();
            await expectUserCameraOrientation(uploader);

            await captureButton.dispatchEvent('pointerdown');
            await expect(captureButton).toHaveClass(/recording/);
            await expect
                .poll(() => uploader.locator('.camera-video').evaluate((video) => video.readyState), { timeout: 5000 })
                .toBeGreaterThanOrEqual(2);
            // Let a rendered half-second pass after recording starts. This is
            // state-based synchronization and avoids a timing sleep while
            // ensuring the canvas stream has delivered enough frames for a
            // self-contained media container.
            await uploader.evaluate(
                () =>
                    new Promise<void>((resolve) =>
                        (function waitForFrames(remaining: number) {
                            if (remaining === 0) {
                                resolve();
                                return;
                            }
                            requestAnimationFrame(() => waitForFrames(remaining - 1));
                        })(30),
                    ),
            );
            await captureButton.dispatchEvent('pointerup');

            const preview = uploader.locator('video[aria-label="Recorded video preview"]');
            await expect(preview).toBeVisible();
            await expect
                .poll(() => preview.evaluate((video) => video.readyState >= 2 && video.error === null), {
                    timeout: 5000,
                })
                .toBe(true);

            const uploadResponsePromise = uploader.waitForResponse(
                (response) => response.url().endsWith('/api/v1/photo/upload') && response.request().method() === 'POST',
            );
            await uploader.getByRole('button', { name: /Send/ }).click();
            const uploadResponse = await uploadResponsePromise;
            // F-10: videos are quarantined and processed asynchronously, so the
            // upload returns a 202 processing job instead of a 201 record.
            expect(uploadResponse.status()).toBe(202);
            const job = (await uploadResponse.json()) as { id: string; kind: 'challenge' | 'chat'; status: string };
            expect(job.kind).toBe('challenge');
            expect(['queued', 'processing']).toContain(job.status);

            // The worker validates, transcodes, writes the canonical object,
            // creates the challenge record, and broadcasts it to the group.
            // The guesser feed revealing the challenge is the deterministic
            // ready signal (transcode is bounded to 60s server-side), and the
            // accept + media flow below proves the canonical video is served.
            const challengeButton = guesser.locator('button.photo-challenge[data-photo-id]').first();
            await expect(challengeButton).toBeVisible({ timeout: 60000 });
            const uploaded = { id: (await challengeButton.getAttribute('data-photo-id')) ?? '' };

            const challenge = guesser.locator('button.photo-challenge[data-photo-id="' + uploaded.id + '"]');
            const acceptResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + uploaded.id + '/accept') &&
                    response.request().method() === 'POST',
            );
            const mediaResponsePromise = guesser.waitForResponse(
                (response) =>
                    response.url().endsWith('/api/v1/challenges/' + uploaded.id + '/media') &&
                    response.request().method() === 'GET',
            );
            await challenge.click();
            expect((await acceptResponsePromise).status()).toBe(200);
            const mediaResponse = await mediaResponsePromise;
            expect(mediaResponse.status()).toBe(200);

            const challengeVideo = guesser.locator('video[aria-label="Challenge video"]');
            await expect(challengeVideo).toBeVisible();
            await expect
                .poll(() => challengeVideo.evaluate((video) => video.readyState >= 2 && video.error === null), {
                    timeout: 5000,
                })
                .toBe(true);
        } finally {
            await closeScenario(scenario);
        }
    });
});
