import { fitDimensions } from '../cameraUtils';
import type { FaceFrame } from '../lenses/facePose';

interface ImagePreparationOptions {
    dataURL: string;
    isCurrent: () => boolean;
    sourceCanvas: HTMLCanvasElement | null;
    onPrepared: (source: HTMLCanvasElement, width: number, height: number) => Promise<void>;
    onError: () => void;
}

export function prepareImageForFilters({
    dataURL,
    isCurrent,
    sourceCanvas,
    onPrepared,
    onError,
}: ImagePreparationOptions): void {
    const image = new Image();
    image.onload = async () => {
        if (!isCurrent() || !sourceCanvas) return;
        const dimensions = fitDimensions(image.naturalWidth, image.naturalHeight);
        sourceCanvas.width = dimensions.width;
        sourceCanvas.height = dimensions.height;
        const context = sourceCanvas.getContext('2d');
        if (!context) return;
        context.drawImage(image, 0, 0, dimensions.width, dimensions.height);
        await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
        if (!isCurrent()) return;
        await onPrepared(sourceCanvas, dimensions.width, dimensions.height);
    };
    image.onerror = () => {
        if (isCurrent()) onError();
    };
    image.src = dataURL;
}

interface CapturePhotoOptions {
    video: HTMLVideoElement | null;
    overlay: HTMLCanvasElement | null;
    captureCanvas: HTMLCanvasElement | null;
    sourceCanvas: HTMLCanvasElement | null;
    renderer: { render: (frame: FaceFrame | null) => void } | null;
    frame: FaceFrame | null;
    /** Horizontally flip the composition so front-camera photos match the
     *  mirrored live preview instead of the raw sensor feed. */
    mirror?: boolean;
}

export function capturePhotoFrame({
    video,
    overlay,
    captureCanvas,
    sourceCanvas,
    renderer,
    frame,
    mirror = false,
}: CapturePhotoOptions): string | null {
    if (!video || !overlay || !captureCanvas || video.videoWidth === 0) return null;
    const context = captureCanvas.getContext('2d');
    if (!context) return null;
    renderer?.render(frame);
    captureCanvas.width = video.videoWidth;
    captureCanvas.height = video.videoHeight;
    if (mirror) {
        context.setTransform(-1, 0, 0, 1, captureCanvas.width, 0);
    }
    context.drawImage(video, 0, 0, captureCanvas.width, captureCanvas.height);
    context.drawImage(overlay, 0, 0, captureCanvas.width, captureCanvas.height);
    if (mirror) {
        context.setTransform(1, 0, 0, 1, 0, 0);
    }
    const sourceContext = sourceCanvas?.getContext('2d');
    if (sourceCanvas && sourceContext) {
        sourceCanvas.width = captureCanvas.width;
        sourceCanvas.height = captureCanvas.height;
        sourceContext.drawImage(captureCanvas, 0, 0);
    }
    return captureCanvas.toDataURL('image/jpeg', 0.9);
}
