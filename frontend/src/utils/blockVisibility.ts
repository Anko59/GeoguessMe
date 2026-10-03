// No player IDs or relationship details are sent across tabs.
export const BLOCK_VISIBILITY_EVENT = 'geoguessme:block-visibility';
export const BLOCK_VISIBILITY_KEY = 'geoguessme:block-visibility:v1';

export function notifyBlockVisibilityChanged(): void {
    window.dispatchEvent(new Event(BLOCK_VISIBILITY_EVENT));
    try {
        window.localStorage.setItem(BLOCK_VISIBILITY_KEY, crypto.randomUUID());
    } catch {
        // Storage can be disabled; local invalidation still always runs.
    }
}
