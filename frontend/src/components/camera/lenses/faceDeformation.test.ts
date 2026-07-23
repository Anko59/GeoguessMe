import type { NormalizedLandmark } from '@mediapipe/tasks-vision';
import * as THREE from 'three';
import { describe, expect, it, vi } from 'vitest';
import { FaceDeformation } from './faceDeformation';
import type { FaceFrame } from './facePose';

function frame(): FaceFrame {
    const landmarks: NormalizedLandmark[] = Array.from({ length: 478 }, () => ({
        x: 0.5,
        y: 0.5,
        z: 0,
        visibility: 1,
    }));
    landmarks[10] = { x: 0.5, y: 0.2, z: 0, visibility: 1 };
    landmarks[152] = { x: 0.5, y: 0.8, z: 0, visibility: 1 };
    landmarks[234] = { x: 0.24, y: 0.5, z: 0, visibility: 1 };
    landmarks[454] = { x: 0.76, y: 0.5, z: 0, visibility: 1 };
    landmarks[33] = { x: 0.35, y: 0.4, z: 0, visibility: 1 };
    landmarks[263] = { x: 0.65, y: 0.4, z: 0, visibility: 1 };
    return { landmarks, blendshapes: {} };
}

describe('FaceDeformation', () => {
    it('maps tracked landmarks into each deformation shader and restores a clean source', () => {
        const deformation = new FaceDeformation();
        const textureDispose = vi.spyOn(THREE.Texture.prototype, 'dispose');
        deformation.setSource(document.createElement('canvas'));
        deformation.resize(1280, 720);

        deformation.update('big-head', frame());
        expect(deformation.mesh.visible).toBe(true);
        expect(deformation.mesh.scale.toArray()).toEqual([1280, 720, 1]);
        expect(deformation.mesh.material.uniforms.effect.value).toBe(1);
        expect(deformation.mesh.material.uniforms.faceCenter.value.toArray()).toEqual([0.5, 0.5]);

        deformation.update('bug-eyes', frame());
        expect(deformation.mesh.material.uniforms.effect.value).toBe(2);
        expect(deformation.mesh.material.uniforms.leftEye.value.toArray()).toEqual([0.35, 0.4]);
        expect(deformation.mesh.material.uniforms.rightEye.value.toArray()).toEqual([0.65, 0.4]);

        deformation.update('tiny-face', frame());
        expect(deformation.mesh.material.uniforms.effect.value).toBe(3);
        deformation.update('tiny-face', null);
        expect(deformation.mesh.material.uniforms.effect.value).toBe(0);

        deformation.clear();
        expect(deformation.mesh.visible).toBe(false);
        deformation.dispose();
        expect(textureDispose).toHaveBeenCalled();
        textureDispose.mockRestore();
    });
});
