import { test, expect } from './support/fixtures';
import { expectConnected, signupViaUI, uniqueGroup } from './support/helpers';
import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';

/** A single-player group chat: enough to exercise the per-message reply and
 *  reaction panel without a second member. */
async function createSoloScenario(
    browser: Browser,
    contextOptions: BrowserContextOptions,
): Promise<{ context: BrowserContext; page: Page }> {
    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();
    await signupViaUI(page);
    await page.goto('/group/create');
    await page.getByPlaceholder('Group Name').fill(uniqueGroup());
    await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    await expectConnected(page);
    return { context, page };
}

test.describe('Chat message actions panel', () => {
    test('opens on a message tap as an overlay without changing its layout', async ({ browser, contextOptions }) => {
        const scenario = await createSoloScenario(browser, contextOptions);
        try {
            const firstText = `first-${Date.now()}`;
            const secondText = `second-${Date.now()}`;
            for (const text of [firstText, secondText]) {
                await scenario.page.locator('#chat-message').fill(text);
                await scenario.page
                    .locator('form.message-input-container')
                    .getByRole('button', { name: 'Send' })
                    .click();
            }

            const firstMessage = scenario.page.locator('.message-container').filter({ hasText: firstText });
            const secondMessage = scenario.page.locator('.message-container').filter({ hasText: secondText });
            await expect(firstMessage).toBeVisible();
            await expect(secondMessage).toBeVisible();

            const actions = firstMessage.locator('.message-actions');
            await expect(actions).toBeHidden();
            expect(await actions.boundingBox()).toBeNull();

            const firstBox = await firstMessage.boundingBox();
            const secondBox = await secondMessage.boundingBox();
            expect(firstBox).not.toBeNull();
            expect(secondBox).not.toBeNull();
            expect(secondBox!.y - (firstBox!.y + firstBox!.height)).toBeLessThan(24);

            // Tapping the message itself (not its row) opens the actions.
            const bubble = firstMessage.locator('.message-content');
            const bubbleBox = await bubble.boundingBox();
            expect(bubbleBox).not.toBeNull();
            await bubble.click();
            await expect(actions).toBeVisible();

            // The panel is an overlay: the message it belongs to keeps its
            // exact position and size, and the panel never overflows the
            // viewport horizontally.
            const bubbleAfter = await bubble.boundingBox();
            expect(bubbleAfter!.x).toBeCloseTo(bubbleBox!.x);
            expect(bubbleAfter!.width).toBeCloseTo(bubbleBox!.width);
            await expect(actions).toHaveCSS('position', 'absolute');
            const pageOverflow = await scenario.page.evaluate(
                () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
            );
            expect(pageOverflow).toBeLessThanOrEqual(0);

            // The reactions sit in one horizontally scrollable row instead of
            // all being visible at once.
            const reactionRow = actions.locator('.reaction-actions');
            const reactionOverflow = await reactionRow.evaluate((el) => ({
                scrollWidth: el.scrollWidth,
                clientWidth: el.clientWidth,
            }));
            expect(reactionOverflow.scrollWidth).toBeGreaterThan(reactionOverflow.clientWidth);

            // Tapping outside the open message dismisses the panel again.
            await scenario.page.locator('.messages-list').click({ position: { x: 8, y: 8 } });
            await expect(actions).toBeHidden();
        } finally {
            await scenario.context.close();
        }
    });
});
