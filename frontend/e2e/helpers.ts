import { expect, type Browser, type BrowserContext, type BrowserContextOptions, type Page } from '@playwright/test';

/** Generate a unique username for test isolation. */
export function uniqueUsername(): string {
    return `user_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`;
}

/** Generate a unique email for test isolation. */
export function uniqueEmail(): string {
    return `e2e_${Date.now()}_${Math.random().toString(36).slice(2, 6)}@test.geoguessme`;
}

/** Generate a unique group name for test isolation. */
export function uniqueGroup(): string {
    return `TestGroup_${Date.now()}`;
}

/**
 * Create an invite in the open group-settings dialog and return the full
 * invite URL (e.g. `http://localhost:8080/group/join#invite=TOKEN`). The raw
 * token is only ever carried in the URL fragment; callers pass this URL to
 * joiner pages, never the bare token.
 */
export async function createInviteFromSettings(page: Page): Promise<string> {
    const settings = page.getByRole('dialog');
    await settings.getByTestId('create-invite-btn').click();
    const urlInput = settings.getByTestId('invite-url');
    await urlInput.waitFor();
    return await urlInput.inputValue();
}

/**
 * Have an already-authenticated page join a group through an invite link.
 * Navigating with the fragment lets useInviteFragmentCapture stash the token
 * in sessionStorage; the join button appears once the preview resolves.
 */
export async function joinGroupViaInvite(page: Page, inviteUrl: string, groupId: string): Promise<void> {
    await page.goto(inviteUrl);
    await page.getByTestId('join-btn').click();
    await page.waitForURL(new RegExp(`/group/${groupId}$`));
}

/** Credentials bag returned after signup or login. */
export interface Credentials {
    username: string;
    email: string;
    password: string;
}

// The group WebSocket handshake can take several seconds under CI parallel
// load. Waiting for the actual 'Connected' state with a generous timeout is
// deterministic state-based synchronization (never a fixed sleep); the default
// 5s expectation proved flaky for second and third pages in multi-user tests.
const CONNECTED_TIMEOUT_MS = 15000;

/** Wait until the group WebSocket reports Connected. */
export async function expectConnected(page: Page): Promise<void> {
    await expect(page.getByRole('status')).toHaveText('Connected', { timeout: CONNECTED_TIMEOUT_MS });
}

/**
 * Sign up a new user entirely through the UI.
 * Returns the credentials used and the page (already at /groups on success).
 */
export async function signupViaUI(page: Page, creds?: Partial<Credentials>): Promise<Credentials> {
    const username = creds?.username ?? uniqueUsername();
    const email = creds?.email ?? uniqueEmail();
    const password = creds?.password ?? 'TestPass123';

    await page.goto('/signup');
    await page.waitForSelector('#signup-username', { state: 'visible' });
    await page.fill('#signup-username', username);
    await page.fill('#signup-email', email);
    await page.fill('#signup-password', password);
    await page.click('button.btn-primary[type="submit"]');
    await page.waitForURL(/\/groups/, { timeout: 15000 });

    return { username, email, password };
}

/**
 * Sign up through the UI and capture the access token from the signup
 * response, for tests that drive authenticated API calls directly.
 */
/**
 * Seed text messages over a group WebSocket so long-conversation tests can
 * exercise pagination without driving the UI. Each send is persisted and
 * broadcast by the server; the caller's own chat socket receives them live.
 */
export async function seedChatMessages(page: Page, groupId: string, token: string, count: number): Promise<void> {
    const ticket = await page.evaluate(
        async ({ groupId, token }) => {
            const response = await fetch(`/api/v1/ws/ticket?group_id=${groupId}`, {
                method: 'POST',
                headers: { Authorization: `Bearer ${token}` },
            });
            if (!response.ok) throw new Error('ticket failed: ' + response.status);
            return (await response.json()) as { ticket: string };
        },
        { groupId, token },
    );
    await page.evaluate(
        async ({ groupId, ticket, count }) => {
            const socket = new WebSocket(
                `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws?group_id=${encodeURIComponent(groupId)}&ticket=${encodeURIComponent(ticket)}`,
            );
            await new Promise<void>((resolve, reject) => {
                socket.onopen = () => resolve();
                socket.onerror = () => reject(new Error('seeding socket failed'));
            });
            for (let i = 1; i <= count; i += 1) {
                socket.send(JSON.stringify({ content: `seed message ${i}` }));
                await new Promise((resolve) => setTimeout(resolve, 10));
            }
            socket.close();
        },
        { groupId, ticket: ticket.ticket, count },
    );
}

/**
 * Sign up through the UI and capture the access token from the signup
 * response, for tests that drive authenticated API calls directly.
 */
export async function signupWithToken(context: BrowserContext): Promise<{ page: Page; token: string }> {
    const page = await context.newPage();
    const signupResponsePromise = page.waitForResponse(
        (r) => r.url().endsWith('/api/v1/auth/signup') && r.request().method() === 'POST',
    );
    await page.goto('/signup');
    await page.waitForSelector('#signup-username', { state: 'visible' });
    await page.fill('#signup-username', uniqueUsername());
    await page.fill('#signup-email', uniqueEmail());
    await page.fill('#signup-password', 'TestPass123');
    await page.click('button.btn-primary[type="submit"]');
    const signupResponse = await signupResponsePromise;
    const token = ((await signupResponse.json()) as { access_token: string }).access_token;
    await page.waitForURL(/\/groups/, { timeout: 15000 });
    return { page, token };
}

/**
 * Log in through the UI.
 */
export async function loginViaUI(page: Page, username: string, password: string): Promise<void> {
    await page.goto('/login');
    await page.waitForSelector('#login-username', { state: 'visible' });
    await page.fill('#login-username', username);
    await page.fill('#login-password', password);
    await page.click('button.btn-primary[type="submit"]');
    await page.waitForURL(/\/groups/, { timeout: 15000 });
}

/**
 * Create an isolated browser context for a second user, inheriting the base
 * URL and (optionally) geolocation/permissions from the current project.
 */
export async function newAuthContext(
    browser: Browser,
    contextOptions: BrowserContextOptions = {},
): Promise<BrowserContext> {
    const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
    return browser.newContext({ ...contextOptions, baseURL: contextOptions.baseURL ?? baseURL });
}

/** Install a deterministic canvas camera before any page is created. */
export async function installDeterministicCamera(context: BrowserContext): Promise<void> {
    await context.addInitScript(() => {
        const canvas = document.createElement('canvas');
        canvas.width = 320;
        canvas.height = 240;
        const canvasContext = canvas.getContext('2d');
        if (!canvasContext) return;
        canvasContext.fillStyle = '#4A90D9';
        canvasContext.fillRect(0, 0, canvas.width, canvas.height);
        canvasContext.fillStyle = '#FFFFFF';
        canvasContext.font = '20px sans-serif';
        canvasContext.fillText('TEST', 120, 120);
        const stream = canvas.captureStream(30);
        let frame = 0;
        const renderFrame = () => {
            canvasContext.fillStyle = '#4A90D9';
            canvasContext.fillRect(0, 0, canvas.width, canvas.height);
            canvasContext.fillStyle = '#FFFFFF';
            canvasContext.fillText(`TEST ${frame++}`, 120, 120);
            requestAnimationFrame(renderFrame);
        };
        requestAnimationFrame(renderFrame);
        const getUserMedia = async () => stream;
        if (!navigator.mediaDevices) {
            Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: {} });
        }
        Object.defineProperty(navigator.mediaDevices, 'getUserMedia', {
            configurable: true,
            value: getUserMedia,
            writable: true,
        });
    });
}

/** Install deterministic geolocation for scenarios that permit location access. */
export async function installDeterministicGeolocation(context: BrowserContext): Promise<void> {
    await context.addInitScript(() => {
        const getCurrentPosition = (success: PositionCallback) => {
            success({
                coords: {
                    accuracy: 1,
                    altitude: null,
                    altitudeAccuracy: null,
                    heading: null,
                    latitude: 48.8566,
                    longitude: 2.3522,
                    speed: null,
                },
                timestamp: Date.now(),
            });
        };
        Object.defineProperty(navigator.geolocation, 'getCurrentPosition', {
            configurable: true,
            value: getCurrentPosition,
            writable: true,
        });
    });
}

/**
 * Extract a verification or password-reset link from a Mailpit-delivered plain-
 * text email. The application sends plain-text (not HTML) messages, so the
 * body is in the `Text` field and the URL is the entire body content.
 */
export async function getMailpitLink(subject: string, pathFragment: string): Promise<string> {
    const mailpitHost = process.env.MAILPIT_BASE_URL || 'http://localhost:8025';
    const query = encodeURIComponent(`subject:"${subject}"`);

    let link: string | null = null;
    await expect
        .poll(
            async () => {
                try {
                    const searchRes = await fetch(`${mailpitHost}/api/v1/search?query=${query}`);
                    const searchBody = (await searchRes.json()) as { messages: Array<{ ID: string }> };
                    const messages = searchBody.messages ?? [];
                    if (messages.length === 0) return null;
                    const msgRes = await fetch(`${mailpitHost}/api/v1/message/${messages[0].ID}`);
                    const msgBody = (await msgRes.json()) as { Text: string };
                    const url = (msgBody.Text ?? '')
                        .match(/https?:\/\/\S+/g)
                        ?.find((value) => value.includes(pathFragment));
                    if (!url) return null;
                    const testBaseURL = process.env.PLAYWRIGHT_BASE_URL;
                    if (!testBaseURL) {
                        link = url;
                        return link;
                    }
                    const result = new URL(url);
                    const base = new URL(testBaseURL);
                    result.protocol = base.protocol;
                    result.host = base.host;
                    link = result.toString();
                    return link;
                } catch {
                    return null;
                }
            },
            { timeout: 30000, intervals: [250, 500, 1000] },
        )
        .toBeTruthy();
    return link!;
}

/**
 * Reset the backend rate limiter via the test-only control endpoint so
 * subsequent auth requests are not throttled by prior test activity.
 */
export async function resetRateLimiter(page: Page): Promise<void> {
    await page.evaluate(async () => {
        await fetch('/api/v1/test/rate-limit/reset', { method: 'POST' });
    });
}

/**
 * Return a 1×1 red PNG as a Buffer (valid image for file-chooser uploads).
 */
export function deterministicTestImage(): Buffer {
    return Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg==',
        'base64',
    );
}

/** Bytes that are not a valid image — triggers "invalid image" from the backend. */
export function malformedFileBytes(): Buffer {
    return Buffer.from('this is not a valid image file');
}

/** Minimal 1×1 GIF — the backend rejects GIF as an unsupported format. */
export function unsupportedFormatBytes(): Buffer {
    // Base64-encoded 1×1 transparent GIF.
    return Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7', 'base64');
}

/**
 * Buffer that slightly exceeds the default UPLOAD_MAX_BYTES (5 MiB). The
 * backend rejects the file in NormalizeUpload before image decoding.
 */
export function oversizedUploadBytes(): Buffer {
    // 5 MiB + 1 byte — guaranteed to exceed the 5 MiB limit.
    return Buffer.alloc(5 * 1024 * 1024 + 1);
}
