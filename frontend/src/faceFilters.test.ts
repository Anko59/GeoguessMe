import { describe, expect, it, vi } from 'vitest';
import { drawFaceFilter, FACE_FILTER_OPTIONS } from './faceFilters';

function contextStub(): CanvasRenderingContext2D {
    return {
        arc: vi.fn(),
        beginPath: vi.fn(),
        closePath: vi.fn(),
        ellipse: vi.fn(),
        fill: vi.fn(),
        lineTo: vi.fn(),
        moveTo: vi.fn(),
        quadraticCurveTo: vi.fn(),
        restore: vi.fn(),
        rotate: vi.fn(),
        roundRect: vi.fn(),
        save: vi.fn(),
        stroke: vi.fn(),
        translate: vi.fn(),
        clearRect: vi.fn(),
    } as unknown as CanvasRenderingContext2D;
}

const detection = { detected: 1, x: 0, y: 0, s: 0.4, rz: 0.1 };

describe('face filters', () => {
    it('offers a no-op option and three face filters', () => {
        expect(FACE_FILTER_OPTIONS.map((option) => option.id)).toEqual(['none', 'sunglasses', 'crown', 'dog']);
    });

    it('clears the overlay when no face filter is selected', () => {
        const context = contextStub();
        drawFaceFilter(context, 'none', detection, 640, 480);
        expect(context.clearRect).toHaveBeenCalledWith(0, 0, 640, 480);
        expect(context.save).not.toHaveBeenCalled();
    });

    it('draws and rotates a detected face filter from Jeeliz pose data', () => {
        const context = contextStub();
        drawFaceFilter(context, 'sunglasses', detection, 640, 480);
        expect(context.save).toHaveBeenCalledOnce();
        expect(context.translate).toHaveBeenCalledWith(320, 240);
        expect(context.rotate).toHaveBeenCalledWith(0.1);
        expect(context.roundRect).toHaveBeenCalledTimes(2);
        expect(context.restore).toHaveBeenCalledOnce();
    });

    it('does not draw when tracking confidence is below the threshold', () => {
        const context = contextStub();
        drawFaceFilter(context, 'dog', { ...detection, detected: 0.5 }, 640, 480);
        expect(context.save).not.toHaveBeenCalled();
    });
});
