const mocks = vi.hoisted(() => ({
    platform: 'web',
    nativePosition: vi.fn(),
    impact: vi.fn(),
    notification: vi.fn(),
    share: vi.fn(),
}));

vi.mock('@capacitor/core', () => ({
    Capacitor: { getPlatform: () => mocks.platform },
}));
vi.mock('@capacitor/geolocation', () => ({
    Geolocation: { getCurrentPosition: mocks.nativePosition },
}));
vi.mock('@capacitor/haptics', () => ({
    Haptics: { impact: mocks.impact, notification: mocks.notification },
    ImpactStyle: { Light: 'LIGHT' },
    NotificationType: { Success: 'SUCCESS' },
}));
vi.mock('@capacitor/share', () => ({ Share: { share: mocks.share } }));
vi.mock('@capacitor/app', () => ({ App: {} }));
vi.mock('@capacitor/status-bar', () => ({ StatusBar: {}, Style: { Dark: 'DARK' } }));

import { captureFeedback, successFeedback } from './haptics';
import { getCurrentPosition } from './location';
import { shouldUseWebPush } from './notifications';
import { currentRuntime, isNativeRuntime } from './runtime';
import { shareOrCopy } from './sharing';
import { routeFromAppURL } from './useNativeAppLifecycle';

beforeEach(() => {
    mocks.platform = 'web';
    mocks.nativePosition.mockReset();
    mocks.impact.mockReset();
    mocks.notification.mockReset();
    mocks.share.mockReset();
});

describe('platform runtime adapters', () => {
    it('normalizes unknown Capacitor platforms to web', () => {
        mocks.platform = 'electron';
        expect(currentRuntime()).toBe('web');
        expect(isNativeRuntime()).toBe(false);
        expect(shouldUseWebPush()).toBe(true);
    });

    it('recognizes both supported native runtimes', () => {
        mocks.platform = 'android';
        expect(currentRuntime()).toBe('android');
        expect(isNativeRuntime()).toBe(true);
        expect(shouldUseWebPush()).toBe(false);
        mocks.platform = 'ios';
        expect(currentRuntime()).toBe('ios');
    });

    it('uses the browser geolocation implementation on the web', async () => {
        const position = { coords: { latitude: 48.8566, longitude: 2.3522 } } as GeolocationPosition;
        const getPosition = vi.fn((success: PositionCallback) => success(position));
        Object.defineProperty(navigator, 'geolocation', {
            configurable: true,
            value: { getCurrentPosition: getPosition },
        });

        await expect(getCurrentPosition()).resolves.toBe(position);
        expect(getPosition).toHaveBeenCalledWith(expect.any(Function), expect.any(Function), {
            enableHighAccuracy: false,
            maximumAge: 60_000,
            timeout: 10_000,
        });
        expect(mocks.nativePosition).not.toHaveBeenCalled();
    });

    it('uses Capacitor geolocation on native platforms', async () => {
        mocks.platform = 'android';
        const position = { coords: { latitude: 48.8566, longitude: 2.3522 } } as GeolocationPosition;
        mocks.nativePosition.mockResolvedValue(position);

        await expect(getCurrentPosition()).resolves.toBe(position);
        expect(mocks.nativePosition).toHaveBeenCalledWith({
            enableHighAccuracy: true,
            maximumAge: 60_000,
            timeout: 10_000,
        });
    });

    it('keeps haptics native-only and treats unavailable vibration as non-fatal', async () => {
        await captureFeedback();
        expect(mocks.impact).not.toHaveBeenCalled();

        mocks.platform = 'android';
        mocks.impact.mockRejectedValue(new Error('unavailable'));
        mocks.notification.mockResolvedValue(undefined);
        await expect(captureFeedback()).resolves.toBeUndefined();
        await expect(successFeedback()).resolves.toBeUndefined();
        expect(mocks.impact).toHaveBeenCalledWith({ style: 'LIGHT' });
        expect(mocks.notification).toHaveBeenCalledWith({ type: 'SUCCESS' });
    });

    it('uses the native share sheet and falls back to the clipboard when dismissed', async () => {
        const writeText = vi.fn().mockResolvedValue(undefined);
        Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
        mocks.platform = 'android';
        mocks.share.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('dismissed'));

        await expect(shareOrCopy('Invite', 'Join me', 'https://geoguessme.com/groups/1')).resolves.toBe('shared');
        await expect(shareOrCopy('Invite', 'Join me', 'https://geoguessme.com/groups/1')).resolves.toBe('copied');
        expect(writeText).toHaveBeenCalledWith('https://geoguessme.com/groups/1');
    });
});

describe('native deep-link routing', () => {
    it.each([
        ['https://geoguessme.com/groups/42?tab=chat#latest', '/groups/42?tab=chat#latest'],
        ['https://www.geoguessme.com//groups//42', '/groups/42'],
        ['geoguessme://group/join/abc', '/group/join/abc'],
    ])('maps %s to %s', (url, route) => {
        expect(routeFromAppURL(url)).toBe(route);
    });

    it.each([
        'https://attacker.example/groups/42',
        'http://geoguessme.com/groups/42',
        'javascript:alert(1)',
        'not a url',
    ])('rejects untrusted URL %s', (url) => expect(routeFromAppURL(url)).toBeNull());
});
