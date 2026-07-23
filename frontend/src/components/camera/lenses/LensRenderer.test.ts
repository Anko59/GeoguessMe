import type { NormalizedLandmark } from '@mediapipe/tasks-vision';
import * as THREE from 'three';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { FaceFrame } from './facePose';
import { LENS_OPTIONS } from './lensCatalog';

interface RendererDouble {
    clear: ReturnType<typeof vi.fn>;
    dispose: ReturnType<typeof vi.fn>;
    render: ReturnType<typeof vi.fn>;
    setClearColor: ReturnType<typeof vi.fn>;
    setSize: ReturnType<typeof vi.fn>;
    outputColorSpace: unknown;
}

const rendererState = vi.hoisted(() => ({ instances: [] as RendererDouble[] }));

vi.mock('three', async (importOriginal) => {
    const actual = await importOriginal<typeof import('three')>();
    class WebGLRenderer {
        clear = vi.fn();
        dispose = vi.fn();
        render = vi.fn();
        setClearColor = vi.fn();
        setSize = vi.fn();
        outputColorSpace: unknown;

        constructor() {
            rendererState.instances.push(this);
        }
    }
    return { ...actual, WebGLRenderer };
});

import { LensRenderer } from './LensRenderer';

function landmark(x = 0.5, y = 0.5, z = 0): NormalizedLandmark {
    return { x, y, z, visibility: 1 };
}

function faceFrame(): FaceFrame {
    const landmarks = Array.from({ length: 478 }, () => landmark());
    landmarks[234] = landmark(0.25, 0.5);
    landmarks[454] = landmark(0.75, 0.5);
    landmarks[10] = landmark(0.5, 0.2);
    landmarks[152] = landmark(0.5, 0.8);
    landmarks[33] = landmark(0.35, 0.4);
    landmarks[263] = landmark(0.65, 0.4);
    landmarks[168] = landmark(0.5, 0.42);
    landmarks[1] = landmark(0.5, 0.55);
    landmarks[13] = landmark(0.5, 0.64);
    landmarks[14] = landmark(0.5, 0.68);
    return { landmarks, blendshapes: { jawOpen: 0.8 } };
}

describe('LensRenderer', () => {
    beforeEach(() => {
        rendererState.instances.length = 0;
    });

    it('configures the transparent renderer and caps large source dimensions', () => {
        const renderer = new LensRenderer(document.createElement('canvas'));
        const webgl = rendererState.instances[0];

        expect(webgl.setClearColor).toHaveBeenCalledWith(0x000000, 0);
        expect(webgl.outputColorSpace).toBe(THREE.SRGBColorSpace);

        renderer.resize(2560, 1440);
        expect(webgl.setSize).toHaveBeenLastCalledWith(1280, 720, false);
        renderer.resize(0, 0);
        expect(webgl.setSize).toHaveBeenLastCalledWith(1, 1, false);
    });

    it('renders original, missing-face, detected-face, and smoothed-face states', () => {
        const renderer = new LensRenderer(document.createElement('canvas'));
        const webgl = rendererState.instances[0];
        renderer.resize(640, 480);

        expect(renderer.render(null, 100)).toBe(true);
        renderer.setLens('cyber');
        renderer.setLens('cyber');
        expect(renderer.render(null, 200)).toBe(false);
        expect(renderer.render(faceFrame(), 300)).toBe(true);

        const moved = faceFrame();
        moved.landmarks[168] = landmark(0.6, 0.46);
        expect(renderer.render(moved, 400)).toBe(true);
        expect(webgl.clear).toHaveBeenCalledTimes(4);
        expect(webgl.render).toHaveBeenCalledTimes(4);

        const scene = webgl.render.mock.calls.at(-1)?.[0] as THREE.Scene;
        const cyberRoot = scene.children.find((child) => child instanceof THREE.Group);
        expect(cyberRoot?.visible).toBe(true);
        expect(cyberRoot?.position.x).toBeGreaterThan(320);
        expect(cyberRoot?.scale.x).toBeGreaterThan(100);
    });

    it('clears tracking state, replaces lens resources, and disposes cleanly', () => {
        const geometryDispose = vi.spyOn(THREE.BufferGeometry.prototype, 'dispose');
        const materialDispose = vi.spyOn(THREE.Material.prototype, 'dispose');
        const renderer = new LensRenderer(document.createElement('canvas'));
        const webgl = rendererState.instances[0];
        renderer.resize(640, 480);
        renderer.setLens('puppy');
        expect(renderer.render(faceFrame())).toBe(true);

        renderer.clear();
        expect(webgl.clear).toHaveBeenCalled();
        renderer.setLens('glam');
        expect(geometryDispose).toHaveBeenCalled();
        expect(materialDispose).toHaveBeenCalled();

        renderer.dispose();
        expect(webgl.dispose).toHaveBeenCalledOnce();
        geometryDispose.mockRestore();
        materialDispose.mockRestore();
    });

    it('renders every catalog lens against a detected face', () => {
        const renderer = new LensRenderer(document.createElement('canvas'));
        const webgl = rendererState.instances[0];
        renderer.resize(640, 480);

        for (const [index, lens] of LENS_OPTIONS.entries()) {
            renderer.setLens(lens.id);
            expect(renderer.render(faceFrame(), index * 100)).toBe(true);
        }

        expect(webgl.render).toHaveBeenCalledTimes(LENS_OPTIONS.length);
    });
});
