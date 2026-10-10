// No player IDs or relationship details are sent across tabs.
export const BLOCK_VISIBILITY_EVENT = 'geoguessme:block-visibility';
export const BLOCK_VISIBILITY_KEY = 'geoguessme:block-visibility:v1';

export function notifyBlockVisibilityChanged(): void {
    window.dispatchEvent(new Event(BLOCK_VISIBILITY_EVENT));
    // Unlike randomUUID, getRandomValues is also available on HTTP origins.
    // This is only an opaque change nonce, never identity or auth state.
    const nonce = crypto.getRandomValues(new Uint8Array(16));
    try {
        window.localStorage.setItem(
            BLOCK_VISIBILITY_KEY,
            Array.from(nonce, (byte) => byte.toString(16).padStart(2, '0')).join(''),
        );
    } catch {
        // Storage can be disabled; local invalidation still always runs.
    }
}
