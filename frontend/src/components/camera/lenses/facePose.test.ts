import type { NormalizedLandmark } from '@mediapipe/tasks-vision';
import { describe, expect, it } from 'vitest';
import { deriveFacePose, smoothFacePose, type FaceFrame } from './facePose';
import { LENS_OPTIONS } from './lensCatalog';

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
    return {
        landmarks,
        blendshapes: {
            jawOpen: 0.7,
            mouthSmileLeft: 0.2,
            mouthSmileRight: 0.6,
            eyeBlinkLeft: 0.1,
            eyeBlinkRight: 0.3,
        },
    };
}

describe('face pose', () => {
    it('anchors a lens to real face landmarks and expressions', () => {
        const pose = deriveFacePose(faceFrame(), 1000, 500);
        expect(pose).not.toBeNull();
        expect(pose?.centerX).toBe(500);
        expect(pose?.centerY).toBe(210);
        expect(pose?.faceWidth).toBe(500);
        expect(pose?.faceHeight).toBeCloseTo(300);
        expect(pose?.roll).toBe(0);
        expect(pose?.yaw).toBe(0);
        expect(pose?.pitch).toBeCloseTo(-0.252);
        expect(pose?.mouthOpen).toBe(0.7);
        expect(pose?.smile).toBe(0.6);
        expect(pose?.blinkRight).toBe(0.3);
    });

    it('rejects missing or implausibly small faces', () => {
        expect(deriveFacePose({ landmarks: [], blendshapes: {} }, 640, 480)).toBeNull();
        expect(deriveFacePose(faceFrame(), 0, 480)).toBeNull();
        expect(deriveFacePose(faceFrame(), 640, 0)).toBeNull();
        const frame = faceFrame();
        frame.landmarks[234] = landmark(0.5, 0.5);
        frame.landmarks[454] = landmark(0.501, 0.5);
        expect(deriveFacePose(frame, 640, 480)).toBeNull();
        const flatFrame = faceFrame();
        flatFrame.landmarks[10] = landmark(0.5, 0.5);
        flatFrame.landmarks[152] = landmark(0.5, 0.501);
        expect(deriveFacePose(flatFrame, 640, 480)).toBeNull();
    });

    it('smooths movement while retaining expression response', () => {
        const previous = deriveFacePose(faceFrame(), 1000, 500);
        const changedFrame = faceFrame();
        changedFrame.landmarks[168] = landmark(0.6, 0.5);
        changedFrame.blendshapes = { ...changedFrame.blendshapes, jawOpen: 1 };
        const next = deriveFacePose(changedFrame, 1000, 500);
        const smoothed = smoothFacePose(previous, next);
        expect(smoothed?.centerX).toBeCloseTo(542);
        expect(smoothed?.centerY).toBeCloseTo(226.8);
        expect(smoothed?.mouthOpen).toBeCloseTo(0.826);
    });

    it('clamps extreme pose and missing expression values', () => {
        const right = faceFrame();
        right.landmarks[1] = landmark(1, 1);
        right.blendshapes = {};
        expect(deriveFacePose(right, 1000, 500)).toMatchObject({
            yaw: 0.75,
            pitch: 0.55,
            smile: 0,
            blinkLeft: 0,
            blinkRight: 0,
        });

        const left = faceFrame();
        left.landmarks[1] = landmark(0, 0);
        left.blendshapes = {
            jawOpen: 2,
            mouthSmileLeft: -1,
            mouthSmileRight: 2,
            eyeBlinkLeft: -1,
            eyeBlinkRight: 2,
        };
        expect(deriveFacePose(left, 1000, 500)).toMatchObject({
            yaw: -0.75,
            pitch: -0.55,
            mouthOpen: 1,
            smile: 1,
            blinkLeft: 0,
            blinkRight: 1,
        });
    });

    it('handles empty smoothing input and wraps roll across both angle boundaries', () => {
        const pose = deriveFacePose(faceFrame(), 1000, 500);
        expect(smoothFacePose(null, pose)).toBe(pose);
        expect(smoothFacePose(pose, null)).toBeNull();
        if (!pose) throw new Error('fixture should produce a pose');

        const clockwise = smoothFacePose({ ...pose, roll: -3 }, { ...pose, roll: 3 });
        const counterClockwise = smoothFacePose({ ...pose, roll: 3 }, { ...pose, roll: -3 });
        expect(clockwise?.roll).toBeLessThan(-3);
        expect(counterClockwise?.roll).toBeGreaterThan(3);
    });
});

describe('lens catalog', () => {
    it('offers a substantial catalog with unique identifiers', () => {
        expect(LENS_OPTIONS).toHaveLength(22);
        expect(new Set(LENS_OPTIONS.map((option) => option.id)).size).toBe(LENS_OPTIONS.length);
        expect(LENS_OPTIONS[0].id).toBe('none');
        expect(LENS_OPTIONS.filter((option) => option.kind === 'deformation')).toHaveLength(3);
        expect(LENS_OPTIONS.filter((option) => option.preview)).toHaveLength(3);
    });
});
