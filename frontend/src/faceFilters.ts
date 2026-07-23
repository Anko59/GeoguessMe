export type FaceFilterId = 'none' | 'sunglasses' | 'hearts' | 'dog' | 'flowers' | 'rainbow';

export interface FaceFilterOption {
    id: FaceFilterId;
    label: string;
    icon: string;
}

export interface FaceDetectionState {
    detected: number;
    x: number;
    y: number;
    s: number;
    rz: number;
    mouthOpening?: number;
}

export const FACE_FILTER_OPTIONS: FaceFilterOption[] = [
    { id: 'none', label: 'Original', icon: '✦' },
    { id: 'sunglasses', label: 'Comedy', icon: '🕶️' },
    { id: 'hearts', label: 'Heart eyes', icon: '💖' },
    { id: 'dog', label: 'Puppy', icon: '🐶' },
    { id: 'flowers', label: 'Flower crown', icon: '🌸' },
    { id: 'rainbow', label: 'Rainbow', icon: '🌈' },
];

interface FaceFrame {
    centerX: number;
    centerY: number;
    size: number;
}

function getFaceFrame(state: FaceDetectionState, width: number, height: number): FaceFrame {
    return {
        centerX: (0.5 + 0.5 * state.x) * width,
        centerY: (0.5 - 0.5 * state.y) * height,
        size: Math.max(1, state.s * width),
    };
}

function drawHeart(context: CanvasRenderingContext2D, x: number, y: number, size: number): void {
    context.beginPath();
    context.moveTo(x, y + size * 0.35);
    context.bezierCurveTo(x - size * 0.85, y - size * 0.1, x - size * 0.55, y - size * 0.85, x, y - size * 0.35);
    context.bezierCurveTo(x + size * 0.55, y - size * 0.85, x + size * 0.85, y - size * 0.1, x, y + size * 0.35);
    context.closePath();
    context.fill();
}

function drawSparkle(context: CanvasRenderingContext2D, x: number, y: number, size: number): void {
    context.beginPath();
    context.moveTo(x, y - size);
    context.lineTo(x + size * 0.28, y - size * 0.28);
    context.lineTo(x + size, y);
    context.lineTo(x + size * 0.28, y + size * 0.28);
    context.lineTo(x, y + size);
    context.lineTo(x - size * 0.28, y + size * 0.28);
    context.lineTo(x - size, y);
    context.lineTo(x - size * 0.28, y - size * 0.28);
    context.closePath();
    context.fill();
}

function drawFlower(context: CanvasRenderingContext2D, x: number, y: number, size: number, color: string): void {
    context.fillStyle = color;
    for (let index = 0; index < 5; index += 1) {
        const angle = (index / 5) * Math.PI * 2;
        context.beginPath();
        context.ellipse(
            x + Math.cos(angle) * size * 0.52,
            y + Math.sin(angle) * size * 0.52,
            size * 0.36,
            size * 0.62,
            angle,
            0,
            Math.PI * 2,
        );
        context.fill();
    }
    context.fillStyle = '#ffd86b';
    context.beginPath();
    context.arc(x, y, size * 0.3, 0, Math.PI * 2);
    context.fill();
}

function drawComedyGlasses(context: CanvasRenderingContext2D, size: number): void {
    const lensWidth = size * 0.36;
    const lensHeight = size * 0.29;
    const lensOffset = size * 0.23;
    const lensY = -size * 0.08;

    context.lineWidth = Math.max(3, size * 0.026);
    context.strokeStyle = '#151a30';
    context.fillStyle = 'rgba(32, 42, 72, 0.9)';
    for (const lensX of [-lensOffset, lensOffset]) {
        context.beginPath();
        context.roundRect(lensX - lensWidth / 2, lensY - lensHeight / 2, lensWidth, lensHeight, lensHeight * 0.28);
        context.fill();
        context.stroke();
    }

    context.strokeStyle = '#ff6ca8';
    context.lineWidth = Math.max(2, size * 0.018);
    context.beginPath();
    context.moveTo(-lensOffset + lensWidth * 0.36, lensY);
    context.quadraticCurveTo(0, lensY - size * 0.075, lensOffset - lensWidth * 0.36, lensY);
    context.stroke();

    context.strokeStyle = '#56e6f2';
    context.lineWidth = Math.max(3, size * 0.035);
    context.beginPath();
    context.moveTo(-lensOffset - lensWidth * 0.48, lensY - lensHeight * 0.2);
    context.lineTo(-size * 0.56, lensY - lensHeight * 0.32);
    context.moveTo(lensOffset + lensWidth * 0.48, lensY - lensHeight * 0.2);
    context.lineTo(size * 0.56, lensY - lensHeight * 0.32);
    context.stroke();

    context.fillStyle = 'rgba(255, 255, 255, 0.76)';
    context.lineWidth = Math.max(2, size * 0.012);
    for (const lensX of [-lensOffset, lensOffset]) {
        context.beginPath();
        context.moveTo(lensX - lensWidth * 0.3, lensY - lensHeight * 0.2);
        context.lineTo(lensX - lensWidth * 0.06, lensY - lensHeight * 0.28);
        context.stroke();
        drawSparkle(context, lensX + lensWidth * 0.27, lensY + lensHeight * 0.22, size * 0.025);
    }

    context.fillStyle = '#e8b878';
    context.strokeStyle = '#9c6136';
    context.lineWidth = Math.max(2, size * 0.016);
    context.beginPath();
    context.moveTo(-size * 0.08, size * 0.02);
    context.quadraticCurveTo(0, -size * 0.03, size * 0.08, size * 0.02);
    context.quadraticCurveTo(size * 0.12, size * 0.23, 0, size * 0.28);
    context.quadraticCurveTo(-size * 0.12, size * 0.23, -size * 0.08, size * 0.02);
    context.fill();
    context.stroke();
}

function drawHeartEyes(context: CanvasRenderingContext2D, size: number): void {
    const eyeY = -size * 0.08;
    context.fillStyle = '#ff477e';
    context.strokeStyle = '#9e204f';
    context.lineWidth = Math.max(2, size * 0.014);
    for (const eyeX of [-size * 0.22, size * 0.22]) {
        drawHeart(context, eyeX, eyeY, size * 0.15);
        context.stroke();
    }

    context.fillStyle = 'rgba(255, 102, 161, 0.48)';
    context.beginPath();
    context.ellipse(-size * 0.29, size * 0.2, size * 0.105, size * 0.045, -0.18, 0, Math.PI * 2);
    context.ellipse(size * 0.29, size * 0.2, size * 0.105, size * 0.045, 0.18, 0, Math.PI * 2);
    context.fill();

    context.fillStyle = '#ffe16b';
    drawSparkle(context, -size * 0.46, -size * 0.34, size * 0.055);
    drawSparkle(context, size * 0.47, -size * 0.26, size * 0.04);

    context.strokeStyle = '#ff477e';
    context.lineWidth = Math.max(2, size * 0.012);
    context.beginPath();
    context.arc(0, size * 0.19, size * 0.12, 0.1, Math.PI - 0.1);
    context.stroke();
}

function drawPuppy(context: CanvasRenderingContext2D, size: number, mouthOpening: number): void {
    const earWidth = size * 0.34;
    const earHeight = size * 0.68;
    const earY = size * 0.02;

    context.fillStyle = '#9f633e';
    context.strokeStyle = '#543322';
    context.lineWidth = Math.max(2, size * 0.018);
    for (const side of [-1, 1]) {
        const x = side * size * 0.4;
        context.beginPath();
        context.moveTo(x, earY - earHeight * 0.5);
        context.bezierCurveTo(
            x + side * earWidth,
            earY - earHeight * 0.5,
            x + side * earWidth * 1.15,
            earY + earHeight * 0.15,
            x + side * earWidth * 0.7,
            earY + earHeight * 0.56,
        );
        context.bezierCurveTo(
            x + side * earWidth * 0.3,
            earY + earHeight * 0.48,
            x - side * earWidth * 0.05,
            earY + earHeight * 0.12,
            x,
            earY - earHeight * 0.5,
        );
        context.fill();
        context.stroke();
    }

    context.fillStyle = '#e9b285';
    context.beginPath();
    context.ellipse(0, size * 0.19, size * 0.2, size * 0.14, 0, 0, Math.PI * 2);
    context.fill();

    context.fillStyle = '#2a1d1c';
    context.beginPath();
    context.ellipse(0, size * 0.13, size * 0.075, size * 0.052, 0, 0, Math.PI * 2);
    context.fill();

    context.strokeStyle = '#2a1d1c';
    context.lineWidth = Math.max(2, size * 0.014);
    context.beginPath();
    context.arc(0, size * 0.22, size * 0.1, 0.15, Math.PI - 0.15);
    context.stroke();
    if (mouthOpening > 0.5) {
        context.fillStyle = '#f27591';
        context.beginPath();
        context.ellipse(0, size * 0.29, size * 0.055, size * 0.1, 0, 0, Math.PI * 2);
        context.fill();
    }

    context.fillStyle = 'rgba(255, 139, 160, 0.5)';
    context.beginPath();
    context.arc(-size * 0.24, size * 0.22, size * 0.05, 0, Math.PI * 2);
    context.arc(size * 0.24, size * 0.22, size * 0.05, 0, Math.PI * 2);
    context.fill();
}

function drawFlowerCrown(context: CanvasRenderingContext2D, size: number): void {
    context.strokeStyle = '#48a96b';
    context.lineWidth = Math.max(4, size * 0.035);
    context.beginPath();
    context.arc(0, -size * 0.18, size * 0.43, Math.PI * 1.12, Math.PI * 1.88);
    context.stroke();

    const flowers = [
        [-size * 0.42, -size * 0.34, size * 0.12, '#ff719d'],
        [-size * 0.21, -size * 0.5, size * 0.105, '#ffd55f'],
        [0, -size * 0.55, size * 0.14, '#e65d89'],
        [size * 0.22, -size * 0.49, size * 0.105, '#78d7c2'],
        [size * 0.43, -size * 0.33, size * 0.12, '#ff9f68'],
    ] as const;
    for (const [x, y, flowerSize, color] of flowers) drawFlower(context, x, y, flowerSize, color);

    context.fillStyle = '#78d58b';
    for (const side of [-1, 1]) {
        context.beginPath();
        context.ellipse(side * size * 0.31, -size * 0.36, size * 0.055, size * 0.14, side * 0.65, 0, Math.PI * 2);
        context.fill();
    }
}

function drawRainbow(context: CanvasRenderingContext2D, size: number): void {
    const bands = [
        ['#ff6f91', 0.06],
        ['#ffad5c', 0.045],
        ['#ffe06b', 0.03],
        ['#71d58a', 0.015],
        ['#65c7ff', 0],
    ] as const;
    for (const [color, offset] of bands) {
        context.strokeStyle = color;
        context.lineWidth = Math.max(3, size * 0.045);
        context.beginPath();
        context.arc(0, size * 0.1, size * (0.55 - offset), Math.PI * 1.12, Math.PI * 1.88);
        context.stroke();
    }

    context.fillStyle = '#ffffff';
    for (const side of [-1, 1]) {
        const cloudX = side * size * 0.47;
        const cloudY = -size * 0.02;
        context.beginPath();
        context.arc(cloudX - side * size * 0.06, cloudY, size * 0.1, 0, Math.PI * 2);
        context.arc(cloudX, cloudY - size * 0.055, size * 0.13, 0, Math.PI * 2);
        context.arc(cloudX + side * size * 0.09, cloudY, size * 0.085, 0, Math.PI * 2);
        context.fill();
    }
    context.fillStyle = '#ffe16b';
    drawSparkle(context, -size * 0.48, -size * 0.42, size * 0.04);
    drawSparkle(context, size * 0.49, -size * 0.36, size * 0.052);
}

export function clearFaceFilter(context: CanvasRenderingContext2D, width: number, height: number): void {
    context.clearRect(0, 0, width, height);
}

export function smoothFaceDetection(
    previous: FaceDetectionState | null,
    next: FaceDetectionState,
): FaceDetectionState | null {
    if (next.detected < 0.35) return null;
    if (!previous) return { ...next };
    const amount = next.detected > 0.68 ? 0.34 : 0.2;
    const blend = (from: number, to: number): number => from + (to - from) * amount;
    return {
        ...next,
        x: blend(previous.x, next.x),
        y: blend(previous.y, next.y),
        s: blend(previous.s, next.s),
        rz: blend(previous.rz, next.rz),
        mouthOpening: blend(previous.mouthOpening ?? 0, next.mouthOpening ?? 0),
    };
}

export function drawFaceFilter(
    context: CanvasRenderingContext2D,
    filterId: FaceFilterId,
    state: FaceDetectionState | null,
    width: number,
    height: number,
): void {
    clearFaceFilter(context, width, height);
    if (filterId === 'none' || !state || state.detected < 0.55) return;

    const frame = getFaceFrame(state, width, height);
    context.save();
    context.translate(frame.centerX, frame.centerY);
    context.rotate(state.rz);
    if (filterId === 'sunglasses') drawComedyGlasses(context, frame.size);
    if (filterId === 'hearts') drawHeartEyes(context, frame.size);
    if (filterId === 'dog') drawPuppy(context, frame.size, state.mouthOpening ?? 0);
    if (filterId === 'flowers') drawFlowerCrown(context, frame.size);
    if (filterId === 'rainbow') drawRainbow(context, frame.size);
    context.restore();
}
