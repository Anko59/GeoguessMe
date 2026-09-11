import * as THREE from 'three';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createGlobeScene, globePosition } from './globeScene';

const mocks = vi.hoisted(() => ({ render: vi.fn(), dispose: vi.fn(), forceContextLoss: vi.fn(), disconnect: vi.fn() }));
vi.mock('three', async (importOriginal) => {
    const actual = await importOriginal<typeof import('three')>();
    return {
        ...actual,
        WebGLRenderer: class {
            domElement = document.createElement('canvas');
            setPixelRatio() {}
            setSize() {}
            render = mocks.render;
            dispose = mocks.dispose;
            forceContextLoss = mocks.forceContextLoss;
        },
    };
});

beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
        'ResizeObserver',
        class {
            observe() {}
            disconnect = mocks.disconnect;
        },
    );
});
afterEach(() => {
    vi.unstubAllGlobals();
});

describe('Earth scene', () => {
    it('aligns Greenwich, poles and the date line to the Earth texture', () => {
        expect(globePosition(0, 0).x).toBeCloseTo(1);
        expect(globePosition(90, 0).y).toBeCloseTo(1);
        expect(globePosition(-90, 0).y).toBeCloseTo(-1);
        expect(globePosition(0, 90).z).toBeCloseTo(-1);
        expect(globePosition(0, -90).z).toBeCloseTo(1);
        expect(globePosition(0, 180).x).toBeCloseTo(-1);
        expect(globePosition(48.8566, 2.3522, 2).length()).toBeCloseTo(2);
    });

    it('draws only visible coordinates, keeps navigation bounded and releases resources', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(texture);
        const disposeTexture = vi.spyOn(texture, 'dispose');
        const disposeGeometry = vi.spyOn(THREE.SphereGeometry.prototype, 'dispose');
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        document.body.append(host);
        const onError = vi.fn();
        const globe = createGlobeScene(host, vi.fn(), onError);
        const item = {
            photo_id: 'p',
            group_id: 'g',
            user_id: 'u',
            username: 'Alice',
            created_at: '',
            expires_at: '',
            status: 'results' as const,
            lat: 0,
            long: 0,
        };
        globe.update([item, { ...item, photo_id: 'hidden', lat: undefined, long: undefined }], 'p');
        const [scene, camera] = mocks.render.mock.lastCall as [THREE.Scene, THREE.PerspectiveCamera];
        const pins = scene.children.find((child) => child instanceof THREE.InstancedMesh) as THREE.InstancedMesh;
        expect(pins.count).toBe(1);
        globe.focus(item);
        expect(camera.position.x).toBeCloseTo(camera.position.length());
        globe.zoom(100);
        expect(camera.position.length()).toBeCloseTo(6);
        globe.zoom(0.001);
        expect(camera.position.length()).toBeCloseTo(1.6);
        globe.rotate(0.25, 0.25);
        expect(camera.position.z).not.toBeCloseTo(0);
        const renders = mocks.render.mock.calls.length;
        expect(renders).toBeGreaterThan(1);
        host.querySelector('canvas')?.dispatchEvent(new Event('webglcontextlost', { cancelable: true }));
        expect(onError).toHaveBeenCalledWith(expect.stringContaining('3D rendering is unavailable'));
        globe.dispose();
        expect(disposeTexture).toHaveBeenCalledOnce();
        expect(disposeGeometry).toHaveBeenCalledTimes(2);
        expect(mocks.disconnect).toHaveBeenCalledOnce();
        expect(mocks.dispose).toHaveBeenCalledOnce();
        expect(mocks.forceContextLoss).toHaveBeenCalledOnce();
        expect(host.children).toHaveLength(0);
        host.remove();
    });

    it('selects front-facing pins and ignores far-side pins and drags', () => {
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        document.body.append(host);
        const select = vi.fn();
        const globe = createGlobeScene(host, select, vi.fn());
        const front = {
            photo_id: 'front',
            group_id: 'g',
            user_id: 'u',
            username: 'Alice',
            created_at: '',
            expires_at: '',
            status: 'results' as const,
            lat: 0,
            long: 0,
        };
        const back = { ...front, photo_id: 'back', long: 180 };
        globe.update([front, back], null);
        globe.focus(front);
        const [scene, camera] = mocks.render.mock.lastCall as [THREE.Scene, THREE.PerspectiveCamera];
        const canvas = host.querySelector('canvas')!;
        canvas.setPointerCapture = vi.fn();
        canvas.releasePointerCapture = vi.fn();
        vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
            left: 0,
            top: 0,
            width: 500,
            height: 400,
        } as DOMRect);
        const click = (drag = false) => {
            scene.updateMatrixWorld(true);
            camera.updateMatrixWorld(true);
            canvas.dispatchEvent(new PointerEvent('pointerdown', { clientX: 250, clientY: 200, pointerId: 1 }));
            if (drag)
                canvas.dispatchEvent(new PointerEvent('pointermove', { clientX: 280, clientY: 200, pointerId: 1 }));
            canvas.dispatchEvent(new PointerEvent('pointerup', { clientX: 250, clientY: 200, pointerId: 1 }));
        };
        click();
        expect(select).toHaveBeenCalledWith('front');
        select.mockClear();
        click(true);
        expect(select).not.toHaveBeenCalled();
        globe.update([back], null);
        click();
        expect(select).not.toHaveBeenCalled();
        globe.dispose();
        host.remove();
    });
});
