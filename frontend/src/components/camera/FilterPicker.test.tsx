import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import FilterPicker from './FilterPicker';
import type { LensId } from './lenses/lensCatalog';

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

        expect(screen.getAllByRole('button')).toHaveLength(16);
        expect(screen.getByRole('button', { name: 'Original' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.queryByText('Loading 3D face tracking…')).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'Butterfly' }));
        expect(onSelect).toHaveBeenCalledWith('butterfly');
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
