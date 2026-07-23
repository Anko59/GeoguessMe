import {
    FaceLandmarker,
    FilesetResolver,
    type Category,
    type FaceLandmarkerResult,
    type ImageSource,
} from '@mediapipe/tasks-vision';
import simdLoaderPath from '@mediapipe/tasks-vision/vision_wasm_internal.js?url';
import simdBinaryPath from '@mediapipe/tasks-vision/vision_wasm_internal.wasm?url';
import noSimdLoaderPath from '@mediapipe/tasks-vision/vision_wasm_nosimd_internal.js?url';
import noSimdBinaryPath from '@mediapipe/tasks-vision/vision_wasm_nosimd_internal.wasm?url';
import type { FaceFrame } from './facePose';

const MODEL_PATH = '/vendor/mediapipe/face_landmarker.task';

function toFrame(result: FaceLandmarkerResult): FaceFrame | null {
    const landmarks = result.faceLandmarks[0];
    if (!landmarks) return null;
    const categories = result.faceBlendshapes[0]?.categories ?? [];
    return {
        landmarks,
        blendshapes: Object.fromEntries(
            categories.map((category: Category) => [category.categoryName, category.score]),
        ),
    };
}

export class FaceTracker {
    private mode: 'IMAGE' | 'VIDEO' = 'VIDEO';
    private readonly landmarker: FaceLandmarker;

    private constructor(landmarker: FaceLandmarker) {
        this.landmarker = landmarker;
    }

    static async create(): Promise<FaceTracker> {
        const simdSupported = await FilesetResolver.isSimdSupported();
        const fileset = simdSupported
            ? { wasmLoaderPath: simdLoaderPath, wasmBinaryPath: simdBinaryPath }
            : { wasmLoaderPath: noSimdLoaderPath, wasmBinaryPath: noSimdBinaryPath };
        const options = {
            baseOptions: { modelAssetPath: MODEL_PATH, delegate: 'GPU' as const },
            runningMode: 'VIDEO' as const,
            numFaces: 1,
            minFaceDetectionConfidence: 0.42,
            minFacePresenceConfidence: 0.42,
            minTrackingConfidence: 0.48,
            outputFaceBlendshapes: true,
            outputFacialTransformationMatrixes: true,
        };

        try {
            return new FaceTracker(await FaceLandmarker.createFromOptions(fileset, options));
        } catch {
            return new FaceTracker(
                await FaceLandmarker.createFromOptions(fileset, {
                    ...options,
                    baseOptions: { modelAssetPath: MODEL_PATH, delegate: 'CPU' },
                }),
            );
        }
    }

    detectVideo(video: HTMLVideoElement, timestamp: number): FaceFrame | null {
        return toFrame(this.landmarker.detectForVideo(video, timestamp));
    }

    async detectImage(image: ImageSource): Promise<FaceFrame | null> {
        if (this.mode !== 'IMAGE') {
            await this.landmarker.setOptions({ runningMode: 'IMAGE' });
            this.mode = 'IMAGE';
        }
        return toFrame(this.landmarker.detect(image));
    }

    close(): void {
        this.landmarker.close();
    }
}
