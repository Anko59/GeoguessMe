import type { Browser, Page } from '@playwright/test';

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

    // After successful signup the app redirects to /groups
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
 * Create an isolated browser context for a second user (same browser instance).
 */
export async function newAuthContext(browser: Browser) {
    return browser.newContext();
}

/**
 * Extract a link (verification, password-reset) from Mailpit for a given recipient.
 *
 * @param email     – the recipient's email address
 * @param pathFragment – a string that appears in the target link (e.g. '/verify-email')
 * @returns the full URL from the email body, or throws if not found within the timeout.
 */
export async function getMailpitLink(email: string, pathFragment: string): Promise<string> {
    const mailpitHost = process.env.MAILPIT_HOST || 'http://localhost:8025';

    // Poll for the email (search by recipient)
    const maxAttempts = 30;
    for (let i = 0; i < maxAttempts; i++) {
        const searchRes = await fetch(`${mailpitHost}/api/v1/search?query=to:${encodeURIComponent(email)}`);
        const searchBody = await searchRes.json() as { messages: Array<{ ID: string }> };
        const messages = searchBody.messages ?? [];

        if (messages.length > 0) {
            // Fetch the most recent message
            const msgId = messages[messages.length - 1].ID;
            const msgRes = await fetch(`${mailpitHost}/api/v1/message/${msgId}`);
            const msgBody = await msgRes.json() as { Body: string };
            const html = msgBody.Body ?? '';

            // Find every href in the email body
            const hrefRegex = /href="([^"]+)"/g;
            const matches = [...html.matchAll(hrefRegex)];

            for (const match of matches) {
                const href = match[1];
                if (href.includes(pathFragment)) {
                    // Mailpit sometimes uses raw http://localhost:8080; ensure it's usable
                    return href;
                }
            }
        }

        // Wait before polling again
        await new Promise((r) => setTimeout(r, 1000));
    }

    throw new Error(`Could not find Mailpit email with fragment "${pathFragment}" for "${email}"`);
}

/**
 * Return a deterministic 1×1 red PNG as a Buffer.
 * Useful for file-chooser uploads when the UI expects an image file.
 */
export function deterministicTestImage(): Buffer {
    // Minimal 1×1 red PNG (base64)
    return Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPj/HwADBwIAMCbHYQAAAABJRU5ErkJggg==',
        'base64',
    );
}

/**
 * Wait until the server-provided `server_time` has elapsed past a certain
 * duration.  Useful for waiting out the view window or guess-after delay.
 */
export async function waitForServerTime(
    page: Page,
    deadlineIso: string,
    extraMs = 500,
): Promise<void> {
    const deadline = Date.parse(deadlineIso);
    const wait = Math.max(0, deadline - Date.now() + extraMs);
    if (wait > 0) {
        await new Promise((r) => setTimeout(r, wait));
    }
}
