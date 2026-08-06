import { describe, expect, it, vi } from 'vitest';
import { capturePhotoFrame } from './cameraImagePreparation';

function contextStub() {
    return {
        drawImage: vi.fn(),
        setTransform: vi.fn(),
    } as unknown as CanvasRenderingContext2D;
}

function canvasWith(stub: CanvasRenderingContext2D): HTMLCanvasElement {
    const canvas = document.createElement('canvas');
    vi.spyOn(canvas, 'getContext').mockReturnValue(stub);
    return canvas;
}

function videoStub(width = 640, height = 480): HTMLVideoElement {
    return { videoWidth: width, videoHeight: height } as unknown as HTMLVideoElement;
}

describe('capturePhotoFrame', () => {
    it('flips the composition horizontally when mirroring a front camera', () => {
        const context = contextStub();
        const captureCanvas = canvasWith(context);
        const sourceCanvas = canvasWith(contextStub());
        captureCanvas.toDataURL = () => 'data:image/jpeg;base64,photo';

        const photo = capturePhotoFrame({
            video: videoStub(),
            overlay: canvasWith(contextStub()),
            captureCanvas,
            sourceCanvas,
            renderer: null,
            frame: null,
            mirror: true,
        });

        expect(photo).toBe('data:image/jpeg;base64,photo');
        expect(captureCanvas.width).toBe(640);
        expect(captureCanvas.height).toBe(480);
        // Flip on, draw the raw frame and the overlay, then flip back so the
        // source canvas keeps a normally-oriented bitmap for later renders.
        expect(context.setTransform).toHaveBeenNthCalledWith(1, -1, 0, 0, 1, 640, 0);
        expect(context.drawImage).toHaveBeenCalledTimes(2);
        expect(context.setTransform).toHaveBeenNthCalledWith(2, 1, 0, 0, 1, 0, 0);
    });

    it('leaves the composition untouched for a back camera', () => {
        const context = contextStub();
        const captureCanvas = canvasWith(context);
        captureCanvas.toDataURL = () => 'data:image/jpeg;base64,photo';

        const photo = capturePhotoFrame({
            video: videoStub(),
            overlay: canvasWith(contextStub()),
            captureCanvas,
            sourceCanvas: canvasWith(contextStub()),
            renderer: null,
            frame: null,
        });

        expect(photo).toBe('data:image/jpeg;base64,photo');
        expect(context.setTransform).not.toHaveBeenCalled();
    });

    it('returns null when the camera frame is not ready', () => {
        const photo = capturePhotoFrame({
            video: videoStub(0, 0),
            overlay: canvasWith(contextStub()),
            captureCanvas: canvasWith(contextStub()),
            sourceCanvas: canvasWith(contextStub()),
            renderer: null,
            frame: null,
        });
        expect(photo).toBeNull();
    });
});
