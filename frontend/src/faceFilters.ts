export type FaceFilterId = 'none' | 'sunglasses' | 'crown' | 'dog';

export interface FaceFilterOption {
    id: FaceFilterId;
    label: string;
}

export interface FaceDetectionState {
    detected: number;
    x: number;
    y: number;
    s: number;
    rz: number;
}

export const FACE_FILTER_OPTIONS: FaceFilterOption[] = [
    { id: 'none', label: 'None' },
    { id: 'sunglasses', label: 'Sunglasses' },
    { id: 'crown', label: 'Crown' },
    { id: 'dog', label: 'Puppy' },
];

interface FaceFrame {
    centerX: number;
    centerY: number;
    size: number;
}

function getFaceFrame(state: FaceDetectionState, width: number, height: number): FaceFrame {
    return {
        centerX: (0.5 + 0.5 * state.x) * width,
        centerY: (0.5 + 0.5 * state.y) * height,
        size: Math.max(1, state.s * width),
    };
}

function drawSunglasses(context: CanvasRenderingContext2D, size: number): void {
    const lensWidth = size * 0.38;
    const lensHeight = size * 0.2;
    const lensY = size * 0.01;
    const lensOffset = size * 0.22;

    context.fillStyle = 'rgba(12, 17, 31, 0.9)';
    context.strokeStyle = '#f5c542';
    context.lineWidth = Math.max(2, size * 0.025);
    for (const lensX of [-lensOffset, lensOffset]) {
        context.beginPath();
        context.roundRect(lensX - lensWidth / 2, lensY - lensHeight / 2, lensWidth, lensHeight, lensHeight * 0.25);
        context.fill();
        context.stroke();
    }

    context.beginPath();
    context.moveTo(-lensOffset + lensWidth / 2, lensY);
    context.lineTo(lensOffset - lensWidth / 2, lensY);
    context.stroke();

    context.strokeStyle = 'rgba(255, 255, 255, 0.62)';
    context.lineWidth = Math.max(1, size * 0.018);
    for (const lensX of [-lensOffset, lensOffset]) {
        context.beginPath();
        context.moveTo(lensX - lensWidth * 0.23, lensY - lensHeight * 0.2);
        context.lineTo(lensX + lensWidth * 0.1, lensY - lensHeight * 0.2);
        context.stroke();
    }
}

function drawCrown(context: CanvasRenderingContext2D, size: number): void {
    const crownWidth = size * 0.9;
    const crownHeight = size * 0.45;
    const left = -crownWidth / 2;
    const bottom = -size * 0.34;

    context.fillStyle = '#f5c542';
    context.strokeStyle = '#8f5f00';
    context.lineWidth = Math.max(2, size * 0.02);
    context.beginPath();
    context.moveTo(left, bottom);
    context.lineTo(left + crownWidth * 0.08, bottom - crownHeight * 0.8);
    context.lineTo(left + crownWidth * 0.34, bottom - crownHeight * 0.48);
    context.lineTo(0, bottom - crownHeight);
    context.lineTo(crownWidth * 0.34, bottom - crownHeight * 0.48);
    context.lineTo(left + crownWidth * 0.92, bottom - crownHeight * 0.8);
    context.lineTo(left + crownWidth, bottom);
    context.closePath();
    context.fill();
    context.stroke();

    context.fillStyle = '#ec5b6f';
    for (const x of [-crownWidth * 0.28, 0, crownWidth * 0.28]) {
        context.beginPath();
        context.arc(x, bottom - crownHeight * (x === 0 ? 0.72 : 0.36), size * 0.045, 0, Math.PI * 2);
        context.fill();
    }
}

function drawPuppy(context: CanvasRenderingContext2D, size: number): void {
    const earWidth = size * 0.34;
    const earHeight = size * 0.58;
    const earY = -size * 0.03;

    context.fillStyle = '#9b5b35';
    context.strokeStyle = '#5a321f';
    context.lineWidth = Math.max(2, size * 0.018);
    for (const side of [-1, 1]) {
        const x = side * size * 0.43;
        context.beginPath();
        context.moveTo(x, earY - earHeight / 2);
        context.quadraticCurveTo(
            x + side * earWidth,
            earY - earHeight * 0.15,
            x + side * earWidth * 0.65,
            earY + earHeight / 2,
        );
        context.quadraticCurveTo(x + side * earWidth * 0.12, earY + earHeight * 0.35, x, earY + earHeight * 0.18);
        context.closePath();
        context.fill();
        context.stroke();
    }

    context.fillStyle = '#241b1a';
    context.beginPath();
    context.ellipse(0, size * 0.28, size * 0.09, size * 0.065, 0, 0, Math.PI * 2);
    context.fill();

    context.strokeStyle = '#ec5b6f';
    context.lineWidth = Math.max(2, size * 0.025);
    context.beginPath();
    context.moveTo(0, size * 0.34);
    context.quadraticCurveTo(0, size * 0.55, size * 0.12, size * 0.43);
    context.stroke();
}

export function clearFaceFilter(context: CanvasRenderingContext2D, width: number, height: number): void {
    context.clearRect(0, 0, width, height);
}

export function drawFaceFilter(
    context: CanvasRenderingContext2D,
    filterId: FaceFilterId,
    state: FaceDetectionState | null,
    width: number,
    height: number,
): void {
    clearFaceFilter(context, width, height);
    if (filterId === 'none' || !state || state.detected < 0.8) return;

    const frame = getFaceFrame(state, width, height);
    context.save();
    context.translate(frame.centerX, frame.centerY);
    context.rotate(state.rz);
    if (filterId === 'sunglasses') drawSunglasses(context, frame.size);
    if (filterId === 'crown') drawCrown(context, frame.size);
    if (filterId === 'dog') drawPuppy(context, frame.size);
    context.restore();
}
