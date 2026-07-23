import { beforeEach, describe, expect, it, vi } from 'vitest';

const mediaPipe = vi.hoisted(() => ({
    close: vi.fn(),
    createFromOptions: vi.fn(),
    detect: vi.fn(),
    detectForVideo: vi.fn(),
    isSimdSupported: vi.fn(),
    setOptions: vi.fn(),
}));

vi.mock('@mediapipe/tasks-vision', () => ({
    FaceLandmarker: { createFromOptions: mediaPipe.createFromOptions },
    FilesetResolver: { isSimdSupported: mediaPipe.isSimdSupported },
}));

import { FaceTracker } from './faceTracker';

const LANDMARK = { x: 0.5, y: 0.4, z: -0.1, visibility: 1 };
const RESULT = {
    faceLandmarks: [[LANDMARK]],
    faceBlendshapes: [
        {
            categories: [
                { categoryName: 'jawOpen', score: 0.75 },
                { categoryName: 'eyeBlinkLeft', score: 0.2 },
            ],
        },
    ],
    facialTransformationMatrixes: [],
};

describe('FaceTracker', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        mediaPipe.isSimdSupported.mockResolvedValue(true);
        mediaPipe.createFromOptions.mockResolvedValue({
            close: mediaPipe.close,
            detect: mediaPipe.detect,
            detectForVideo: mediaPipe.detectForVideo,
            setOptions: mediaPipe.setOptions,
        });
        mediaPipe.detect.mockReturnValue(RESULT);
        mediaPipe.detectForVideo.mockReturnValue(RESULT);
        mediaPipe.setOptions.mockResolvedValue(undefined);
    });

    it('uses SIMD and GPU, maps expressions, and changes image mode once', async () => {
        const tracker = await FaceTracker.create();
        expect(mediaPipe.createFromOptions).toHaveBeenCalledTimes(1);
        expect(mediaPipe.createFromOptions.mock.calls[0][0].wasmLoaderPath).toContain('vision_wasm_internal');
        expect(mediaPipe.createFromOptions.mock.calls[0][1]).toMatchObject({
            baseOptions: {
                modelAssetPath: '/vendor/mediapipe/face_landmarker.task',
                delegate: 'GPU',
            },
            runningMode: 'VIDEO',
            numFaces: 1,
            outputFaceBlendshapes: true,
        });

        const video = document.createElement('video');
        expect(tracker.detectVideo(video, 1234)).toEqual({
            landmarks: [LANDMARK],
            blendshapes: { jawOpen: 0.75, eyeBlinkLeft: 0.2 },
        });
        expect(mediaPipe.detectForVideo).toHaveBeenCalledWith(video, 1234);

        const image = document.createElement('canvas');
        await expect(tracker.detectImage(image)).resolves.toEqual({
            landmarks: [LANDMARK],
            blendshapes: { jawOpen: 0.75, eyeBlinkLeft: 0.2 },
        });
        await tracker.detectImage(image);
        expect(mediaPipe.setOptions).toHaveBeenCalledOnce();
        expect(mediaPipe.setOptions).toHaveBeenCalledWith({ runningMode: 'IMAGE' });

        tracker.close();
        expect(mediaPipe.close).toHaveBeenCalledOnce();
    });

    it('uses the non-SIMD runtime and falls back to CPU', async () => {
        mediaPipe.isSimdSupported.mockResolvedValue(false);
        mediaPipe.createFromOptions.mockRejectedValueOnce(new Error('GPU unavailable'));

        await FaceTracker.create();

        expect(mediaPipe.createFromOptions).toHaveBeenCalledTimes(2);
        expect(mediaPipe.createFromOptions.mock.calls[0][0].wasmLoaderPath).toContain('nosimd');
        expect(mediaPipe.createFromOptions.mock.calls[1][1]).toMatchObject({
            baseOptions: {
                modelAssetPath: '/vendor/mediapipe/face_landmarker.task',
                delegate: 'CPU',
            },
        });
    });

    it('returns null when MediaPipe does not report a face', async () => {
        mediaPipe.detect.mockReturnValue({
            faceLandmarks: [],
            faceBlendshapes: [],
            facialTransformationMatrixes: [],
        });
        mediaPipe.detectForVideo.mockReturnValue({
            faceLandmarks: [],
            faceBlendshapes: [],
            facialTransformationMatrixes: [],
        });
        const tracker = await FaceTracker.create();

        expect(tracker.detectVideo(document.createElement('video'), 0)).toBeNull();
        await expect(tracker.detectImage(document.createElement('canvas'))).resolves.toBeNull();
    });

    it('handles a detected face without blendshape categories', async () => {
        mediaPipe.detectForVideo.mockReturnValue({
            faceLandmarks: [[LANDMARK]],
            faceBlendshapes: [],
            facialTransformationMatrixes: [],
        });
        const tracker = await FaceTracker.create();

        expect(tracker.detectVideo(document.createElement('video'), 0)).toEqual({
            landmarks: [LANDMARK],
            blendshapes: {},
        });
    });
});
