import { isNativeRuntime } from './runtime';

function normalizedOrigin(value: string | undefined): string | null {
    const candidate = value?.trim();
    if (!candidate) return null;
    const url = new URL(candidate);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
        throw new Error('Configured application origins must use HTTP or HTTPS');
    }
    return url.origin;
}

const configuredAPIOrigin = normalizedOrigin(import.meta.env.VITE_API_ORIGIN);
const configuredWebOrigin = normalizedOrigin(import.meta.env.VITE_WEB_ORIGIN);

export const apiBaseURL = configuredAPIOrigin ? `${configuredAPIOrigin}/api/v1` : '/api/v1';

export function backendURL(path: string): string {
    if (!configuredAPIOrigin) return path;
    return new URL(path, configuredAPIOrigin).toString();
}

export function websocketURL(path: string): string {
    const base = configuredAPIOrigin ?? window.location.origin;
    const url = new URL(path, base);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.toString();
}

export function publicWebURL(path: string): string {
    const origin = configuredWebOrigin ?? (isNativeRuntime() ? 'https://geoguessme.com' : window.location.origin);
    return new URL(path, origin).toString();
}
