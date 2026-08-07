import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import FullScreenImage from './FullScreenImage';

describe('FullScreenImage', () => {
    it('opens the dialog on click and closes via the close button', () => {
        render(
            <FullScreenImage src="/photo.png" alt="alice's avatar">
                <img src="/photo.png" alt="alice" />
            </FullScreenImage>,
        );
        fireEvent.click(screen.getByRole('button', { name: "View alice's avatar full screen" }));
        const dialog = screen.getByRole('dialog', { name: "alice's avatar full screen" });
        expect(dialog.querySelector('img')).toHaveAttribute('src', '/photo.png');
        fireEvent.click(screen.getByRole('button', { name: 'Close full-screen photo' }));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('closes the dialog with the Escape key', () => {
        render(
            <FullScreenImage src="/photo.png" alt="photo">
                <img src="/photo.png" alt="thumbnail" />
            </FullScreenImage>,
        );
        fireEvent.click(screen.getByRole('button', { name: 'View photo full screen' }));
        expect(screen.getByRole('dialog')).toBeInTheDocument();
        fireEvent.keyDown(window, { key: 'Escape' });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('renders the thumbnail without an expand button while the source is loading', () => {
        render(
            <FullScreenImage src={undefined} alt="photo">
                <img src="/thumb.png" alt="thumbnail" />
            </FullScreenImage>,
        );
        expect(screen.queryByRole('button', { name: 'View photo full screen' })).not.toBeInTheDocument();
        expect(screen.getByAltText('thumbnail')).toBeInTheDocument();
    });
});
