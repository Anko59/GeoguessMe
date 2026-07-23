import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import FilterPicker from './FilterPicker';
import { LENS_OPTIONS, type LensId } from './lenses/lensCatalog';

describe('FilterPicker', () => {
    it('renders the complete catalog, selected state, and lens selection', () => {
        const onSelect = vi.fn();
        render(
            <FilterPicker
                selectedFilter="none"
                filterReady={false}
                filterError=""
                faceDetected={false}
                onSelect={onSelect}
            />,
        );

        expect(screen.getAllByRole('button')).toHaveLength(LENS_OPTIONS.length + 2);
        expect(screen.getByRole('button', { name: 'Original' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.queryByText('Loading 3D face tracking…')).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'Butterfly' }));
        expect(onSelect).toHaveBeenCalledWith('butterfly');
    });

    it('scrolls the lens rail with desktop arrows and a vertical mouse wheel', () => {
        const { container } = render(
            <FilterPicker
                selectedFilter="none"
                filterReady={false}
                filterError=""
                faceDetected={false}
                onSelect={vi.fn()}
            />,
        );
        const rail = container.querySelector<HTMLDivElement>('.camera-filter-options');
        expect(rail).not.toBeNull();
        if (!rail) return;
        rail.scrollBy = vi.fn();
        Object.defineProperty(rail, 'clientWidth', { value: 500 });

        fireEvent.click(screen.getByRole('button', { name: 'Next lenses' }));
        expect(rail.scrollBy).toHaveBeenCalledWith({ left: 360, behavior: 'smooth' });

        fireEvent.wheel(rail, { deltaY: 120, deltaX: 0 });
        expect(rail.scrollLeft).toBe(120);
    });

    it('shows loading, face guidance, success, and error states', () => {
        const props = {
            selectedFilter: 'cyber' as const,
            filterReady: false,
            filterError: '',
            faceDetected: false,
            onSelect: vi.fn(),
        };
        const { rerender } = render(<FilterPicker {...props} />);
        expect(screen.getAllByText('Cyber visor')).toHaveLength(2);
        expect(screen.getByText('Loading 3D face tracking…')).toBeInTheDocument();

        rerender(<FilterPicker {...props} filterReady />);
        expect(screen.getByText('Center your face in good light')).toBeInTheDocument();

        rerender(<FilterPicker {...props} filterReady faceDetected />);
        expect(screen.queryByText('Center your face in good light')).not.toBeInTheDocument();

        rerender(<FilterPicker {...props} filterError="WebGL unavailable" />);
        expect(screen.getByText('WebGL unavailable')).toBeInTheDocument();
        expect(screen.queryByText('Loading 3D face tracking…')).not.toBeInTheDocument();
    });

    it('tolerates a future persisted lens identifier', () => {
        render(
            <FilterPicker
                selectedFilter={'future-lens' as LensId}
                filterReady={false}
                filterError=""
                faceDetected={false}
                onSelect={vi.fn()}
            />,
        );

        expect(screen.getByText('Lenses').nextSibling).toHaveTextContent('');
    });
});
