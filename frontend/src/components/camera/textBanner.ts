export type TextBannerStyle = 'classic' | 'neon' | 'clean';

export interface TextBanner {
    text: string;
    style: TextBannerStyle;
    position: number;
}

export const EMPTY_TEXT_BANNER: TextBanner = {
    text: '',
    style: 'classic',
    position: 46,
};

interface BannerTheme {
    background: string;
    foreground: string;
    shadow: string;
}

const THEMES: Record<TextBannerStyle, BannerTheme> = {
    classic: { background: 'rgba(0, 0, 0, 0.68)', foreground: '#ffffff', shadow: 'rgba(0, 0, 0, 0.5)' },
    neon: { background: 'rgba(255, 28, 151, 0.82)', foreground: '#ffffff', shadow: 'rgba(64, 0, 45, 0.75)' },
    clean: { background: 'rgba(255, 255, 255, 0.9)', foreground: '#11131d', shadow: 'rgba(0, 0, 0, 0.24)' },
};

function roundedRectangle(
    context: CanvasRenderingContext2D,
    x: number,
    y: number,
    width: number,
    height: number,
    radius: number,
): void {
    context.beginPath();
    context.moveTo(x + radius, y);
    context.lineTo(x + width - radius, y);
    context.quadraticCurveTo(x + width, y, x + width, y + radius);
    context.lineTo(x + width, y + height - radius);
    context.quadraticCurveTo(x + width, y + height, x + width - radius, y + height);
    context.lineTo(x + radius, y + height);
    context.quadraticCurveTo(x, y + height, x, y + height - radius);
    context.lineTo(x, y + radius);
    context.quadraticCurveTo(x, y, x + radius, y);
    context.closePath();
}

export function drawTextBanner(
    context: CanvasRenderingContext2D,
    width: number,
    height: number,
    banner: TextBanner,
): void {
    const text = banner.text.trim();
    if (!text || width <= 0 || height <= 0) return;

    context.save();
    const theme = THEMES[banner.style];
    const maximumWidth = width * 0.86;
    let fontSize = Math.max(24, Math.min(76, width * 0.064));
    context.font = `800 ${fontSize}px Inter, system-ui, sans-serif`;
    while (fontSize > 20 && context.measureText(text).width > maximumWidth) {
        fontSize -= 2;
        context.font = `800 ${fontSize}px Inter, system-ui, sans-serif`;
    }

    const textWidth = Math.min(context.measureText(text).width, maximumWidth);
    const horizontalPadding = Math.max(18, fontSize * 0.48);
    const bannerWidth = Math.min(width * 0.94, textWidth + horizontalPadding * 2);
    const bannerHeight = fontSize * 1.62;
    const x = (width - bannerWidth) / 2;
    const y = (height * banner.position) / 100 - bannerHeight / 2;

    context.shadowColor = theme.shadow;
    context.shadowBlur = fontSize * 0.34;
    context.fillStyle = theme.background;
    roundedRectangle(context, x, y, bannerWidth, bannerHeight, bannerHeight * 0.22);
    context.fill();
    context.shadowBlur = 0;
    context.fillStyle = theme.foreground;
    context.textAlign = 'center';
    context.textBaseline = 'middle';
    context.fillText(text, width / 2, y + bannerHeight / 2, maximumWidth);
    context.restore();
}
