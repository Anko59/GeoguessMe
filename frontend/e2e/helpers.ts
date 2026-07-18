import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';

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

/** Credentials bag returned after signup or login. */
export interface Credentials {
    username: string;
    email: string;
    password: string;
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
export async function getMailpitLink(email: string, pathFragment: string): Promise<string> {
    const mailpitHost = process.env.MAILPIT_BASE_URL || 'http://localhost:8025';
    const query = encodeURIComponent(`to:${email}`);

    for (let i = 0; i < 30; i++) {
        const searchRes = await fetch(`${mailpitHost}/api/v1/search?query=${query}`);
        const searchBody = (await searchRes.json()) as { messages: Array<{ ID: string }> };
        const messages = searchBody.messages ?? [];

        if (messages.length > 0) {
            const msgId = messages[0].ID;
            const msgRes = await fetch(`${mailpitHost}/api/v1/message/${msgId}`);
            const msgBody = (await msgRes.json()) as { Text: string };
            const text = msgBody.Text ?? '';

            // The plain-text body is a single URL (or contains one). Extract it
            // by scanning for http(s) URLs containing the expected path.
            const urlRegex = /https?:\/\/\S+/g;
            for (const match of text.matchAll(urlRegex)) {
                const url = match[0];
                if (url.includes(pathFragment)) {
                    const testBaseURL = process.env.PLAYWRIGHT_BASE_URL;
                    if (!testBaseURL) return url;
                    const link = new URL(url);
                    const base = new URL(testBaseURL);
                    link.protocol = base.protocol;
                    link.host = base.host;
                    return link.toString();
                }
            }
        }
        await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(`Could not find Mailpit email with fragment "${pathFragment}" for "${email}"`);
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
