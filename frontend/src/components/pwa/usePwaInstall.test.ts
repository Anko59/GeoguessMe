import { act, renderHook } from '@testing-library/react';
import { isStandaloneDisplay, isIosSafari, readDismissed, usePwaInstall } from './usePwaInstall';

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

    it('recognizes iPadOS desktop-mode Safari', () => {
        const originalUserAgent = navigator.userAgent;
        const originalPlatform = navigator.platform;
        const originalMaxTouchPoints = navigator.maxTouchPoints;
        try {
            Object.defineProperty(navigator, 'userAgent', {
                configurable: true,
                value: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 Safari/605.1.15',
            });
            Object.defineProperty(navigator, 'platform', { configurable: true, value: 'MacIntel' });
            Object.defineProperty(navigator, 'maxTouchPoints', { configurable: true, value: 5 });

            expect(isIosSafari()).toBe(true);
        } finally {
            Object.defineProperty(navigator, 'userAgent', { configurable: true, value: originalUserAgent });
            Object.defineProperty(navigator, 'platform', { configurable: true, value: originalPlatform });
            Object.defineProperty(navigator, 'maxTouchPoints', { configurable: true, value: originalMaxTouchPoints });
        }
    });
});

describe('usePwaInstall', () => {
    it('captures the native install event and prompts the browser', async () => {
        const prompt = vi.fn().mockResolvedValue(undefined);
        const userChoice = Promise.resolve({ outcome: 'accepted' as const });
        const { result } = renderHook(() => usePwaInstall());
        const event = Object.assign(new Event('beforeinstallprompt'), { prompt, userChoice });

        act(() => {
            window.dispatchEvent(event);
        });
        expect(result.current.installable).toBe(true);

        await act(async () => {
            await expect(result.current.promptInstall()).resolves.toBe('accepted');
        });

        expect(prompt).toHaveBeenCalledOnce();
        expect(result.current.installed).toBe(true);
        expect(result.current.installable).toBe(false);
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
