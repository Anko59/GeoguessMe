import * as THREE from 'three';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DEFAULT_MAP_PIN_IMAGE_URL } from '../../../utils/mapPins';
import { createGlobeScene } from '../globeScene';

const mocks = vi.hoisted(() => ({
    render: vi.fn(),
    dispose: vi.fn(),
    forceContextLoss: vi.fn(),
    disconnect: vi.fn(),
    clearOverlay: vi.fn(),
    drawArtwork: vi.fn(),
    setOverlayTransform: vi.fn(),
    maxTextureSize: 8192,
}));

vi.mock('three', async (importOriginal) => {
    const actual = await importOriginal<typeof import('three')>();
    return {
        ...actual,
        WebGLRenderer: class {
            domElement = document.createElement('canvas');
            capabilities = {
                maxTextureSize: mocks.maxTextureSize,
                getMaxAnisotropy: () => 8,
            };
            setPixelRatio() {}
            setSize() {}
            render = mocks.render;
            dispose = mocks.dispose;
            forceContextLoss = mocks.forceContextLoss;
        },
    };
});

function useLoadedPinArtwork() {
    const images: Array<{ src: string }> = [];
    class LoadedPinImage {
        complete = true;
        naturalWidth = 128;
        decoding = 'async';
        onload: ((event: Event) => void) | null = null;
        onerror: ((event: Event) => void) | null = null;
        private source = '';

        constructor() {
            images.push(this);
        }

        get src() {
            return this.source;
        }

        set src(url: string) {
            this.source = url;
            this.onload?.(new Event('load'));
        }

        getAttribute(name: string) {
            return name === 'src' ? this.source : null;
        }

        removeAttribute(name: string) {
            if (name === 'src') this.source = '';
        }
    }
    vi.stubGlobal('Image', LoadedPinImage);
    return images;
}

function makeChallenge(photoID: string, lat = 22, long = 12) {
    return {
        photo_id: photoID,
        group_id: 'group-1',
        user_id: 'user-1',
        username: 'Alice',
        created_at: '',
        expires_at: '',
        status: 'results' as const,
        lat,
        long,
    };
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.maxTextureSize = 8192;
    const overlayContext = {
        clearRect: mocks.clearOverlay,
        drawImage: mocks.drawArtwork,
        setTransform: mocks.setOverlayTransform,
    } as unknown as CanvasRenderingContext2D;
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation((contextID) =>
        contextID === '2d' ? overlayContext : null,
    );
    vi.stubGlobal(
        'ResizeObserver',
        class {
            observe() {}
            disconnect = mocks.disconnect;
        },
    );
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

describe('globe pin overlay', () => {
    it('projects unique artwork URLs onto one canvas and releases the image cache', () => {
        const images = useLoadedPinArtwork();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        const items = [
            {
                ...makeChallenge('north-star'),
                lat: 48.8,
                long: 2.3,
                map_pin: { key: 'north-star', name: 'North Star', image_url: '/map-pins/north-star.svg' },
            },
            makeChallenge('standard', 51.5, -0.1),
        ];
        globe.update(items, null);

        const overlay = host.querySelector<HTMLCanvasElement>('.globe-pin-overlay');
        expect(overlay).not.toBeNull();
        expect(overlay).toHaveAttribute('aria-hidden', 'true');
        expect(host.querySelectorAll('.globe-pin-marker')).toHaveLength(0);
        expect(images.map((image) => image.src)).toEqual(['/map-pins/north-star.svg', DEFAULT_MAP_PIN_IMAGE_URL]);
        expect(mocks.drawArtwork).toHaveBeenCalledTimes(2);
        expect(mocks.drawArtwork.mock.calls[0][3]).toBeGreaterThan(0);
        expect(mocks.drawArtwork.mock.calls[0][4]).toBeGreaterThan(0);

        globe.dispose();
        expect(images.map((image) => image.src)).toEqual(['', '']);
        expect(host.children).toHaveLength(0);
    });

    it('reuses artwork and one overlay for large challenge sets', () => {
        const images = useLoadedPinArtwork();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        const items = Array.from({ length: 1000 }, (_, index) =>
            makeChallenge(
                `photo-${index}`,
                index === 0 ? 22 : ((index * 37) % 160) - 80,
                index === 0 ? 12 : ((index * 71) % 360) - 180,
            ),
        );
        globe.update(items, null);

        expect(host.querySelectorAll('canvas.globe-pin-overlay')).toHaveLength(1);
        expect(host.querySelectorAll('.globe-pin-marker')).toHaveLength(0);
        expect(images).toHaveLength(1);
        expect(mocks.drawArtwork.mock.calls.length).toBeGreaterThan(0);
        expect(mocks.drawArtwork.mock.calls.length).toBeLessThan(items.length);
        expect(mocks.clearOverlay).toHaveBeenCalled();
        globe.dispose();
        expect(host.children).toHaveLength(0);
    });

    it('reprojects pins on movement and clears their previous canvas positions', () => {
        useLoadedPinArtwork();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        globe.update([makeChallenge('front', 0, 0), makeChallenge('back', 0, 180)], null);
        expect(mocks.drawArtwork).toHaveBeenCalledTimes(1);
        const firstDraw = mocks.drawArtwork.mock.lastCall!;
        const firstCenterX = firstDraw[1] + firstDraw[3] / 2;

        globe.rotate(0.6, 0);

        expect(mocks.clearOverlay.mock.calls.length).toBeGreaterThan(1);
        expect(mocks.drawArtwork).toHaveBeenCalledTimes(2);
        const secondDraw = mocks.drawArtwork.mock.lastCall!;
        expect(secondDraw[1] + secondDraw[3] / 2).not.toBe(firstCenterX);
        globe.dispose();
        expect(host.children).toHaveLength(0);
    });

    it('selects visible artwork within a 44px target and ignores far-side pins and drags', () => {
        useLoadedPinArtwork();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        document.body.append(host);
        const select = vi.fn();
        const globe = createGlobeScene(host, select, vi.fn());
        const front = makeChallenge('front', 0, 0);
        const back = makeChallenge('back', 0, 180);
        globe.update([front, back], null);
        globe.focus(front);
        const canvas = host.querySelector<HTMLCanvasElement>('canvas:not(.globe-pin-overlay)')!;
        canvas.setPointerCapture = vi.fn();
        canvas.releasePointerCapture = vi.fn();
        vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
            left: 0,
            top: 0,
            width: 500,
            height: 400,
        } as DOMRect);
        const imageDraw = mocks.drawArtwork.mock.lastCall!;
        const markerX = imageDraw[1] + imageDraw[3] / 2;
        const markerY = imageDraw[2] + imageDraw[4] / 2;
        const click = (offsetX = 0, offsetY = 0, drag = false) => {
            canvas.dispatchEvent(
                new PointerEvent('pointerdown', {
                    clientX: markerX + offsetX,
                    clientY: markerY + offsetY,
                    pointerId: 1,
                    pointerType: 'touch',
                }),
            );
            if (drag)
                canvas.dispatchEvent(
                    new PointerEvent('pointermove', {
                        clientX: markerX + offsetX + 30,
                        clientY: markerY + offsetY,
                        pointerId: 1,
                    }),
                );
            canvas.dispatchEvent(
                new PointerEvent('pointerup', {
                    clientX: markerX + offsetX,
                    clientY: markerY + offsetY,
                    pointerId: 1,
                    pointerType: 'touch',
                }),
            );
        };

        click();
        expect(select).toHaveBeenCalledWith('front');
        select.mockClear();
        click(18);
        expect(select).toHaveBeenCalledWith('front');
        select.mockClear();
        click(30);
        expect(select).not.toHaveBeenCalled();
        click(0, 0, true);
        expect(select).not.toHaveBeenCalled();
        const artworkDraws = mocks.drawArtwork.mock.calls.length;
        globe.update([back], null);
        expect(mocks.drawArtwork).toHaveBeenCalledTimes(artworkDraws);
        click();
        expect(select).not.toHaveBeenCalled();
        globe.dispose();
        host.remove();
    });
});
