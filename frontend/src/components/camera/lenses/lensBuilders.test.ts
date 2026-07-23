import * as THREE from 'three';
import { describe, expect, it } from 'vitest';
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
    it.each(LENS_OPTIONS)('builds and animates $label', ({ id }) => {
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

        if (id === 'none') {
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

    it('returns an inert group for an unknown identifier', () => {
        const lens = buildLens('future-lens' as LensId);
        lens.update(1, CLOSED_MOUTH);
        expect(lens.root.children).toHaveLength(0);
    });
});
