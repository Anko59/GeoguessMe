import { test, expect } from './fixtures';
import { expectConnected, seedChatMessages, signupViaUI, signupWithToken, uniqueGroup } from './helpers';
import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';

interface ChatScenario {
    ownerContext: BrowserContext;
    owner: Page;
    groupId: string;
    groupCode: string;
}

async function createScenario(browser: Browser, contextOptions: BrowserContextOptions): Promise<ChatScenario> {
    const ownerContext = await browser.newContext(contextOptions);
    const owner = await ownerContext.newPage();
    await signupViaUI(owner);
    await owner.goto('/group/create');
    await owner.getByPlaceholder('Group Name').fill(uniqueGroup());
    await owner.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await owner.waitForURL(/\/group\/[0-9a-f-]{36}$/);

    const groupId = owner.url().split('/group/')[1];
    await owner.getByRole('button', { name: 'Open group settings' }).click();
    const settings = owner.getByRole('dialog');
    const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
    await settings.getByRole('button', { name: 'Close settings' }).click();
    await expectConnected(owner);

    return { ownerContext, owner, groupId, groupCode };
}

async function addMember(
    browser: Browser,
    contextOptions: BrowserContextOptions,
    scenario: ChatScenario,
): Promise<{ context: BrowserContext; page: Page }> {
    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();
    await signupViaUI(page);
    await page.goto('/group/join');
    await page.getByPlaceholder('6-character code').fill(scenario.groupCode);
    await page.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
    await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    await page.goto(`/group/${scenario.groupId}`);
    await expectConnected(page);
    return { context, page };
}

test.describe('Chat via WebSocket', () => {
    test('chat connect, send message, receive in real-time', async ({ browser, contextOptions }) => {
        const scenario = await createScenario(browser, contextOptions);
        const member = await addMember(browser, contextOptions, scenario);
        try {
            await expect(scenario.owner.locator('.chat-container')).toBeVisible();
            await expect(member.page.locator('.chat-container')).toBeVisible();

            const msgText = `Hello from A at ${Date.now()}`;
            await scenario.owner.locator('#chat-message').fill(msgText);
            await scenario.owner.locator('form.message-input-container').getByRole('button', { name: 'Send' }).click();

            await expect(scenario.owner.locator('.message-container').filter({ hasText: msgText })).toBeVisible();
            await expect(member.page.locator('.message-container').filter({ hasText: msgText })).toBeVisible();
        } finally {
            await member.context.close();
            await scenario.ownerContext.close();
        }
    });

    test('reload restores message history', async ({ browser, contextOptions }) => {
        const scenario = await createScenario(browser, contextOptions);
        const member = await addMember(browser, contextOptions, scenario);
        try {
            const msgText = `Hello before reload at ${Date.now()}`;
            await scenario.owner.locator('#chat-message').fill(msgText);
            await scenario.owner.locator('form.message-input-container').getByRole('button', { name: 'Send' }).click();
            await expect(member.page.locator('.message-container').filter({ hasText: msgText })).toBeVisible();

            await member.page.reload();
            await member.page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expectConnected(member.page);
            await expect(member.page.locator('.message-container').filter({ hasText: msgText })).toBeVisible();
        } finally {
            await member.context.close();
            await scenario.ownerContext.close();
        }
    });

    test('chat opens with the latest page and loads older messages on scroll up', async ({
        browser,
        contextOptions,
    }) => {
        const context = await browser.newContext(contextOptions);
        const { page, token } = await signupWithToken(context);
        try {
            await page.goto('/group/create');
            await page.getByPlaceholder('Group Name').fill(uniqueGroup());
            await page.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            const groupId = page.url().split('/group/')[1];
            await expectConnected(page);

            // Seed 60 text messages over the group WebSocket so the latest page
            // (PAGE_SIZE=50) is full and older history exists below it.
            await seedChatMessages(page, groupId, token, 60);

            // The last broadcast rendering proves every seed was persisted.
            await expect(page.locator('.message-container').filter({ hasText: 'seed message 60' })).toBeVisible();

            // Reload: the initial sync renders only the latest page, anchored at
            // the oldest of the newest 50, with older history absent.
            await page.reload();
            await page.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expectConnected(page);
            await expect(page.locator('.messages-list .message-container')).toHaveCount(50);
            await expect(page.locator('.messages-list .message-container').first()).toContainText('seed message 11');
            await expect(page.getByText('seed message 1', { exact: true })).not.toBeVisible();

            // Scrolling to the top loads the older page and prepends it.
            await page.locator('.messages-list').evaluate((el) => {
                el.scrollTop = 0;
                el.dispatchEvent(new Event('scroll'));
            });
            await expect(page.getByText('seed message 1', { exact: true })).toBeVisible();
            await expect(page.locator('.messages-list .message-container')).toHaveCount(60);
        } finally {
            await context.close();
        }
    });

    test('chat sends and receives a private photo attachment', async ({ browser, contextOptions }) => {
        const scenario = await createScenario(browser, contextOptions);
        const member = await addMember(browser, contextOptions, scenario);
        try {
            const caption = `shared-photo-${Date.now()}`;
            const png = Buffer.from(
                'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
                'base64',
            );
            await scenario.owner.locator('#chat-attachment').setInputFiles({
                name: 'shared.png',
                mimeType: 'image/png',
                buffer: png,
            });
            await scenario.owner.locator('#chat-message').fill(caption);
            await scenario.owner.getByRole('button', { name: 'Send attachment' }).click();

            await expect(scenario.owner.locator('.message-container').filter({ hasText: caption })).toBeVisible();
            await expect(member.page.locator('.message-container').filter({ hasText: caption })).toBeVisible();
            const memberAttachment = member.page
                .locator('.message-container')
                .filter({ hasText: caption })
                .locator('img.chat-attachment');
            await expect(memberAttachment).toBeVisible();
            // Targeted visual assertions: chat rows keep their bottom-aligned
            // flex layout regardless of message kind.
            await expect(scenario.owner.locator('.message-container').first()).toHaveCSS('display', 'flex');
            await expect(scenario.owner.locator('.message-container').first()).toHaveCSS('align-items', 'flex-end');

            // Clicking the shared photo opens it full screen, like the
            // challenge photo on the results page; closing restores the chat.
            await memberAttachment.click();
            const fullScreen = member.page.getByRole('dialog', { name: 'Shared photo full screen' });
            await expect(fullScreen).toBeVisible();
            await expect(fullScreen.locator('img')).toHaveAttribute('src', /^blob:/);
            // Targeted visual assertions: the shared full-screen dialog keeps
            // its fixed, top-of-stack, dark presentation across surfaces.
            await expect(fullScreen).toHaveCSS('position', 'fixed');
            await expect(fullScreen).toHaveCSS('z-index', '2100');
            await expect(fullScreen).toHaveCSS('background-color', 'rgba(5, 7, 18, 0.98)');
            await member.page.getByRole('button', { name: 'Close full-screen photo' }).click();
            await expect(fullScreen).not.toBeVisible();
        } finally {
            await member.context.close();
            await scenario.ownerContext.close();
        }
    });

    test('hidden message actions do not reserve layout space', async ({ browser, contextOptions }) => {
        const scenario = await createScenario(browser, contextOptions);
        try {
            const firstText = `first-${Date.now()}`;
            const secondText = `second-${Date.now()}`;
            for (const text of [firstText, secondText]) {
                await scenario.owner.locator('#chat-message').fill(text);
                await scenario.owner
                    .locator('form.message-input-container')
                    .getByRole('button', { name: 'Send' })
                    .click();
            }

            const firstMessage = scenario.owner.locator('.message-container').filter({ hasText: firstText });
            const secondMessage = scenario.owner.locator('.message-container').filter({ hasText: secondText });
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

            const canHover = await scenario.owner.evaluate(() => window.matchMedia('(hover: hover)').matches);
            if (canHover) {
                await firstMessage.locator('.message-hover-target').hover();
                await expect(actions).toBeVisible();
                expect(await actions.boundingBox()).not.toBeNull();
            }
        } finally {
            await scenario.ownerContext.close();
        }
    });

    test('browser WebSocket recovery: disconnect, renewed ticket, cursor catch-up, overlapping live delivery, exact-once rendering', async ({
        browser,
        contextOptions,
    }) => {
        // ── helpers ────────────────────────────────────────────────────────────
        /** Create a context whose every page monkey-patches WebSocket so the
         *  test can disconnect and inspect instances via window.__wsCtl. */
        async function controlledContext(): Promise<BrowserContext> {
            const ctx = await browser.newContext(contextOptions);
            await ctx.addInitScript(() => {
                const OrigWS = WebSocket;
                const instances: WebSocket[] = [];
                (window as unknown as Record<string, unknown>).__wsCtl = {
                    disconnect(): void {
                        for (const ws of instances.slice()) {
                            try {
                                ws.close();
                            } catch {
                                /* already closed */
                            }
                        }
                    },
                    count(): number {
                        return instances.filter((ws) => ws.readyState === WebSocket.OPEN).length;
                    },
                    ticketUrls(): string[] {
                        return instances.map((ws) => (ws as unknown as { url: string }).url);
                    },
                };
                // Proxy WebSocket so every constructed instance is tracked.
                function PatchedWS(this: WebSocket, url: string, protocols?: string | string[]) {
                    const real: WebSocket =
                        arguments.length === 2 && typeof protocols === 'string'
                            ? new OrigWS(url, protocols)
                            : new OrigWS(url);
                    instances.push(real);
                    return real;
                }
                PatchedWS.prototype = OrigWS.prototype;
                Object.defineProperties(PatchedWS, {
                    CONNECTING: { value: OrigWS.CONNECTING },
                    OPEN: { value: OrigWS.OPEN },
                    CLOSING: { value: OrigWS.CLOSING },
                    CLOSED: { value: OrigWS.CLOSED },
                });
                window.WebSocket = PatchedWS as typeof WebSocket;
            });
            return ctx;
        }

        interface WSControl {
            disconnect(): Promise<void>;
            count(): Promise<number>;
            ticketUrls(): Promise<string[]>;
        }

        function wsControl(page: Page): WSControl {
            return {
                disconnect: () =>
                    page.evaluate(() =>
                        (window as unknown as { __wsCtl: { disconnect(): void } }).__wsCtl.disconnect(),
                    ),
                count: () =>
                    page.evaluate(() => (window as unknown as { __wsCtl: { count(): number } }).__wsCtl.count()),
                ticketUrls: () =>
                    page.evaluate(() =>
                        (window as unknown as { __wsCtl: { ticketUrls(): string[] } }).__wsCtl.ticketUrls(),
                    ),
            };
        }

        /** Extract the ticket query parameter from a WebSocket URL. */
        function ticketFromUrl(raw: string): string {
            try {
                const u = new URL(raw);
                return u.searchParams.get('ticket') ?? '';
            } catch {
                return '';
            }
        }

        // ── setup ─────────────────────────────────────────────────────────────
        const scenario = await createScenario(browser, contextOptions);
        const memberCtx = await controlledContext();
        const memberPage = await memberCtx.newPage();
        await signupViaUI(memberPage);
        await memberPage.goto('/group/join');
        await memberPage.getByPlaceholder('6-character code').fill(scenario.groupCode);
        await memberPage.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
        await memberPage.waitForURL(/\/group\/[0-9a-f-]{36}$/);
        await memberPage.goto(`/group/${scenario.groupId}`);
        await expectConnected(memberPage);

        try {
            // Establish cursor baseline: send two messages so the member has
            // a non-empty cursor to snapshot during reconnect.
            const m1 = `baseline-1-${Date.now()}`;
            const m2 = `baseline-2-${Date.now()}`;
            for (const txt of [m1, m2]) {
                await scenario.owner.locator('#chat-message').fill(txt);
                await scenario.owner
                    .locator('form.message-input-container')
                    .getByRole('button', { name: 'Send' })
                    .click();
            }
            await expect(memberPage.locator('.message-container').filter({ hasText: m1 })).toBeVisible();
            await expect(memberPage.locator('.message-container').filter({ hasText: m2 })).toBeVisible();

            const firstTicket = ticketFromUrl((await wsControl(memberPage).ticketUrls())[0] ?? '');
            expect(firstTicket).toMatch(/^\S+$/);

            // ── 1. disconnect ──────────────────────────────────────────────────
            await wsControl(memberPage).disconnect();
            // The hook immediately sees the close and sets status to offline.
            await expect(memberPage.getByRole('status')).toHaveText('Offline — retrying');

            // ── 2. cursor catch-up: send gap messages while member is offline ──
            const gap1 = `gap-alpha-${Date.now()}`;
            const gap2 = `gap-beta-${Date.now()}`;
            for (const txt of [gap1, gap2]) {
                await scenario.owner.locator('#chat-message').fill(txt);
                await scenario.owner
                    .locator('form.message-input-container')
                    .getByRole('button', { name: 'Send' })
                    .click();
                await expect(scenario.owner.locator('.message-container').filter({ hasText: txt })).toBeVisible();
            }

            // Wait until the hook automatically reconnects.
            await expectConnected(memberPage);

            // ── 3. renewed ticket ──────────────────────────────────────────────
            const urlsAfter = await wsControl(memberPage).ticketUrls();
            expect(urlsAfter.length).toBeGreaterThanOrEqual(2);
            const renewedTicket = ticketFromUrl(urlsAfter[urlsAfter.length - 1]);
            expect(renewedTicket).toMatch(/^\S+$/);
            expect(renewedTicket).not.toBe(firstTicket);

            // ── 4. cursor catch-up delivers missed messages ────────────────────
            await expect(memberPage.locator('.message-container').filter({ hasText: gap1 })).toBeVisible();
            await expect(memberPage.locator('.message-container').filter({ hasText: gap2 })).toBeVisible();

            // Assert exact-once: each message id appears exactly one time.
            const ids = await memberPage.evaluate(() =>
                [...document.querySelectorAll('[data-message-id]')].map((el) => el.getAttribute('data-message-id')),
            );
            const seen = new Set<string>();
            for (const id of ids) {
                if (!id) continue;
                expect(seen.has(id), `duplicate message id ${id}`).toBe(false);
                seen.add(id);
            }

            // ── 5. overlapping live delivery + exact-once under race ───────────
            // Disconnect once more with the REST catch-up blocked. After
            // reconnect the live WebSocket is open but catch-up is stalled.
            // Send the overlap message NOW (while WebSocket is live), let it
            // arrive via the open socket, then release catch-up so the same
            // message is also delivered via catch-up. Dedup must keep one.
            const ticketUrlsBefore = await wsControl(memberPage).ticketUrls();
            const ticketCountBefore = ticketUrlsBefore.length;

            // Block the REST messages endpoint so catch-up stalls.
            let releaseCatchUp!: () => void;
            await memberPage.route('**/api/v1/group/messages*', async (route) => {
                const url = route.request().url();
                // Only delay catch-up calls that carry a cursor. The initial
                // load fetches without cursor and must not be delayed, or the
                // page would stay empty forever.
                if (url.includes('cursor=')) {
                    await new Promise<void>((resolve) => {
                        releaseCatchUp = resolve;
                    });
                }
                await route.continue();
            });

            await wsControl(memberPage).disconnect();
            await expect(memberPage.getByRole('status')).toHaveText('Offline — retrying');

            // Wait for the hook to reconnect. After this the WebSocket is
            // open and receiving live events, but catch-up is still blocked.
            await expectConnected(memberPage);

            // Now send the overlap message while the member is connected via
            // the live WebSocket but the catch-up REST window is still open.
            const overlap = `overlap-${Date.now()}`;
            await scenario.owner.locator('#chat-message').fill(overlap);
            await scenario.owner.locator('form.message-input-container').getByRole('button', { name: 'Send' }).click();
            await expect(scenario.owner.locator('.message-container').filter({ hasText: overlap })).toBeVisible();

            // The overlap message arrives via the live WebSocket first.
            await expect(memberPage.locator('.message-container').filter({ hasText: overlap })).toBeVisible({
                timeout: 10000,
            });

            // Release the blocked catch-up. It will return the same overlap
            // message (and the gap messages), but dedup must keep one copy.
            releaseCatchUp();

            // After catch-up integrates, the overlap message must still
            // appear exactly once.
            await expect(async () => {
                const count = await memberPage.locator('.message-container').filter({ hasText: overlap }).count();
                expect(count).toBe(1);
            }).toPass({ timeout: 5000 });

            // Also verify a new WebSocket was created for this reconnect.
            const finalTicketUrls = await wsControl(memberPage).ticketUrls();
            expect(finalTicketUrls.length).toBeGreaterThan(ticketCountBefore);

            // Final exact-once check across all messages.
            const allIds = await memberPage.evaluate(() =>
                [...document.querySelectorAll('[data-message-id]')].map((el) => el.getAttribute('data-message-id')),
            );
            const allSeen = new Set<string>();
            for (const id of allIds) {
                if (!id) continue;
                if (allSeen.has(id)) {
                    throw new Error(`duplicate message id ${id} after overlapping delivery`);
                }
                allSeen.add(id);
            }
        } finally {
            await memberCtx.close();
            await scenario.ownerContext.close();
        }
    });

    test('one-time WS ticket reuse is rejected', async ({ browser, contextOptions }) => {
        const scenario = await createScenario(browser, contextOptions);
        try {
            const ticketResponse = scenario.owner.waitForResponse(
                (response) =>
                    response.url().includes('/api/v1/ws/ticket?') &&
                    response.request().method() === 'POST' &&
                    response.status() === 201,
            );
            await scenario.owner.reload();
            await scenario.owner.waitForURL(/\/group\/[0-9a-f-]{36}$/);
            await expectConnected(scenario.owner);
            const { ticket: usedTicket } = (await (await ticketResponse).json()) as { ticket: string };
            expect(usedTicket).toMatch(/^\S+$/);

            const baseUrl = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
            const wsBase = baseUrl.replace(/^http/, 'ws');
            const groupParam = `group_id=${encodeURIComponent(scenario.groupId)}`;
            const ticketParam = `ticket=${encodeURIComponent(usedTicket)}`;
            const rejected = await scenario.owner.evaluate(
                async ({ wsBase, groupParam, ticketParam }) =>
                    new Promise<boolean>((resolve) => {
                        let settled = false;
                        const finish = (value: boolean) => {
                            if (!settled) {
                                settled = true;
                                resolve(value);
                            }
                        };
                        const ws = new WebSocket(`${wsBase}/api/v1/ws?${groupParam}&${ticketParam}`);
                        ws.onopen = () => {
                            ws.close();
                            finish(false);
                        };
                        ws.onerror = () => finish(true);
                        ws.onclose = (event: CloseEvent) => finish(event.code !== 1000);
                        setTimeout(() => finish(true), 5000);
                    }),
                { wsBase, groupParam, ticketParam },
            );

            expect(rejected).toBe(true);
        } finally {
            await scenario.ownerContext.close();
        }
    });
});
