import { isStandaloneDisplay, isIosSafari, readDismissed } from './usePwaInstall';

describe('isStandaloneDisplay', () => {
    it('returns false in a browser test environment', () => {
        expect(isStandaloneDisplay()).toBe(false);
    });

    it('returns true when matchMedia reports standalone', () => {
        const orig = window.matchMedia;
        window.matchMedia = ((query: string) => ({
            matches: query === '(display-mode: standalone)',
            media: query,
            onchange: null,
            addListener: vi.fn(),
            removeListener: vi.fn(),
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
            dispatchEvent: vi.fn(() => true),
        })) as typeof window.matchMedia;
        const result = isStandaloneDisplay();
        window.matchMedia = orig;
        expect(result).toBe(true);
    });
});

describe('isIosSafari', () => {
    it('returns false for non-iOS user agent', () => {
        expect(isIosSafari()).toBe(false);
    });
});

describe('readDismissed', () => {
    beforeEach(() => {
        localStorage.clear();
    });

    it('returns false by default', () => {
        expect(readDismissed()).toBe(false);
    });

    it('returns true when the key is set', () => {
        localStorage.setItem('geoguessme:pwa-onboarding-dismissed', '1');
        expect(readDismissed()).toBe(true);
    });
});
