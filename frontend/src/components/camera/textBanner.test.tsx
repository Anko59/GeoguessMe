import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import TextBannerEditor, { TextBannerOverlay } from './TextBannerEditor';
import { drawTextBanner, EMPTY_TEXT_BANNER, type TextBanner } from './textBanner';

function canvasContext(): CanvasRenderingContext2D {
    return {
        beginPath: vi.fn(),
        closePath: vi.fn(),
        fill: vi.fn(),
        fillText: vi.fn(),
        lineTo: vi.fn(),
        measureText: vi.fn((text: string) => ({ width: text.length * 28 })),
        moveTo: vi.fn(),
        quadraticCurveTo: vi.fn(),
        restore: vi.fn(),
        save: vi.fn(),
    } as unknown as CanvasRenderingContext2D;
}

describe('text banners', () => {
    it('draws the selected banner theme into the exported photo and ignores blank text', () => {
        const context = canvasContext();
        drawTextBanner(context, 1200, 800, EMPTY_TEXT_BANNER);
        expect(context.save).not.toHaveBeenCalled();

        drawTextBanner(context, 1200, 800, { text: 'CEO OF BAD IDEAS', style: 'neon', position: 42 });
        expect(context.fill).toHaveBeenCalledOnce();
        expect(context.fillText).toHaveBeenCalledWith('CEO OF BAD IDEAS', 600, expect.any(Number), 1032);
        expect(context.restore).toHaveBeenCalledOnce();
    });

    it('edits text, style, and vertical position while previewing the banner', () => {
        let banner: TextBanner = EMPTY_TEXT_BANNER;
        const onChange = vi.fn((next: TextBanner) => {
            banner = next;
        });
        const { rerender } = render(
            <>
                <TextBannerEditor banner={banner} onChange={onChange} />
                <TextBannerOverlay banner={banner} />
            </>,
        );

        fireEvent.click(screen.getByRole('button', { name: /text/i }));
        fireEvent.change(screen.getByPlaceholderText('Say something dangerous…'), {
            target: { value: 'Unsupervised' },
        });
        expect(banner.text).toBe('Unsupervised');

        rerender(
            <>
                <TextBannerEditor banner={banner} onChange={onChange} />
                <TextBannerOverlay banner={banner} />
            </>,
        );
        expect(screen.getByText('Unsupervised')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Neon' }));
        expect(banner.style).toBe('neon');
        fireEvent.change(screen.getByRole('slider'), { target: { value: '60' } });
        expect(banner.position).toBe(60);
    });
});
