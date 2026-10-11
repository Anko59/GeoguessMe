import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';
import AppErrorBoundary from './AppErrorBoundary';

function BrokenScreen(): never {
    throw new Error('sensitive details must not be displayed');
}

describe('AppErrorBoundary', () => {
    it('renders its children normally', () => {
        render(
            <AppErrorBoundary>
                <p>Ready</p>
            </AppErrorBoundary>,
        );
        expect(screen.getByText('Ready')).toBeInTheDocument();
    });

    it('shows a recoverable screen without exposing exception details', () => {
        const log = vi.spyOn(console, 'error').mockImplementation(() => undefined);
        try {
            render(
                <AppErrorBoundary>
                    <BrokenScreen />
                </AppErrorBoundary>,
            );
            expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong');
            expect(screen.getByRole('button', { name: 'Reload app' })).toBeInTheDocument();
            expect(screen.getByRole('alert')).not.toHaveTextContent('sensitive details');
            expect(log).toHaveBeenCalledWith('GeoGuessMe could not render the current screen');
        } finally {
            log.mockRestore();
        }
    });
});
