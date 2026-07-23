import * as THREE from 'three';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import type { FacePose } from './facePose';
import { buildLens } from './lensBuilders';
import { LENS_OPTIONS, type LensId } from './lensCatalog';

const CLOSED_MOUTH: FacePose = {
    centerX: 320,
    centerY: 240,
    faceWidth: 240,
    faceHeight: 300,
    roll: 0,
    yaw: 0,
    pitch: 0,
    mouthOpen: 0,
    smile: 0,
    blinkLeft: 0,
    blinkRight: 0,
};

describe('3D lens builders', () => {
    beforeAll(() => {
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture());
        vi.spyOn(THREE.BufferGeometryLoader.prototype, 'load').mockImplementation((_url, onLoad) => {
            onLoad(new THREE.BufferGeometry());
        });
    });

    afterAll(() => vi.restoreAllMocks());

    it.each(LENS_OPTIONS)('builds and animates $label', ({ id, kind }) => {
        const lens = buildLens(id);
        expect(lens.root).toBeInstanceOf(THREE.Group);

        lens.update(0, CLOSED_MOUTH);
        lens.update(2.75, {
            ...CLOSED_MOUTH,
            mouthOpen: 1,
            smile: 1,
            blinkLeft: 1,
            blinkRight: 1,
        });

        if (id === 'none' || kind === 'deformation') {
            expect(lens.root.children).toHaveLength(0);
        } else {
            expect(lens.root.children.length).toBeGreaterThan(0);
            expect(
                lens.root.children.some((child) => child instanceof THREE.Mesh || child instanceof THREE.Group),
            ).toBe(true);
        }
    });

    it('makes the puppy tongue react to mouth opening', () => {
        const lens = buildLens('puppy');
        const tongue = lens.root.children.at(-1);
        expect(tongue).toBeInstanceOf(THREE.Mesh);

        lens.update(0, CLOSED_MOUTH);
        expect(tongue?.visible).toBe(false);
        lens.update(0.5, { ...CLOSED_MOUTH, mouthOpen: 0.8 });
        expect(tongue?.visible).toBe(true);
        expect(tongue?.scale.y).toBeGreaterThan(1);
    });

    it('keeps flat generated accessories upright in the top-left camera coordinate system', () => {
        for (const id of ['disco-outlaw', 'hr-nightmare', 'toxic-ex', 'tax-fraud'] as const) {
            const lens = buildLens(id);
            const accessory = lens.root.children[0] as THREE.Mesh<THREE.PlaneGeometry, THREE.MeshBasicMaterial>;
            expect(accessory.material.map?.flipY).toBe(false);
            expect(accessory.material.transparent).toBe(true);
        }
    });

    it('returns an inert group for an unknown identifier', () => {
        const lens = buildLens('future-lens' as LensId);
        lens.update(1, CLOSED_MOUTH);
        expect(lens.root.children).toHaveLength(0);
    });
});
