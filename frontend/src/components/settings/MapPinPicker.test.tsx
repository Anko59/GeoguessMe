import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MapPinPicker from './MapPinPicker';
import type { MapPinCatalog } from '../../types';

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }));
vi.mock('../../api', () => ({
    default: { get: mocks.get, post: mocks.post, put: mocks.put, delete: mocks.delete },
    getAPIErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

const catalog: MapPinCatalog = {
    selected_pin_key: null,
    pins: [
        {
            key: 'north-star',
            name: 'North Star',
            description: 'A clear sky marker.',
            image_url: '/map-pins/north-star.svg',
            unlocked: true,
            challenges: [
                {
                    key: 'perfect-score',
                    name: 'Perfect score',
                    description: 'Get the maximum score once.',
                    unlocked_at: '2026-09-27T10:00:00Z',
                },
            ],
        },
        {
            key: 'deep-blue',
            name: 'Deep Blue',
            description: 'A cool ocean marker.',
            image_url: '/map-pins/deep-blue.svg',
            unlocked: false,
            challenges: [
                {
                    key: 'weekly-winner',
                    name: 'Weekly winner',
                    description: 'Finish first in a group week.',
                },
            ],
        },
    ],
};

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockResolvedValue({ data: catalog });
    mocks.post.mockResolvedValue({ data: catalog });
    mocks.put.mockResolvedValue({ data: { ...catalog, selected_pin_key: 'north-star' } });
    mocks.delete.mockResolvedValue({ data: undefined });
});

describe('MapPinPicker', () => {
    it('lists unlock progress and equips only unlocked pins', async () => {
        render(<MapPinPicker />);
        expect(await screen.findByText('North Star')).toBeInTheDocument();
        const unlocked = screen.getByText('North Star').closest('article');
        const locked = screen.getByText('Deep Blue').closest('article');
        expect(unlocked).not.toBeNull();
        expect(within(unlocked as HTMLElement).getByText(/Completed: Perfect score/)).toBeInTheDocument();
        expect(within(locked as HTMLElement).getByText('Locked')).toBeInTheDocument();
        expect(within(locked as HTMLElement).queryByRole('button')).not.toBeInTheDocument();

        fireEvent.click(within(unlocked as HTMLElement).getByRole('button', { name: 'Use this' }));
        await waitFor(() => expect(mocks.put).toHaveBeenCalledWith('/auth/pins', { pin_key: 'north-star' }));
        await waitFor(() =>
            expect(within(unlocked as HTMLElement).getByRole('button', { name: 'Selected' })).toBeDisabled(),
        );
    });

    it('restores the standard marker and handles an empty catalog', async () => {
        mocks.post.mockResolvedValueOnce({ data: catalog });
        mocks.put.mockResolvedValueOnce({ data: { ...catalog, selected_pin_key: 'north-star' } });
        const view = render(<MapPinPicker />);
        await screen.findByText('North Star');
        const unlocked = screen.getByText('North Star').closest('article');
        fireEvent.click(within(unlocked as HTMLElement).getByRole('button', { name: 'Use this' }));
        await waitFor(() => expect(mocks.put).toHaveBeenCalled());
        await waitFor(() =>
            expect(within(unlocked as HTMLElement).getByRole('button', { name: 'Selected' })).toBeDisabled(),
        );

        const standard = screen.getByText('Standard marker').closest('article');
        fireEvent.click(within(standard as HTMLElement).getByRole('button', { name: 'Use this' }));
        await waitFor(() => expect(mocks.delete).toHaveBeenCalledWith('/auth/pins'));

        view.unmount();
        mocks.post.mockResolvedValueOnce({ data: { selected_pin_key: null, pins: [] } });
        render(<MapPinPicker />);
        expect(await screen.findByText('No map pins are available yet.')).toBeInTheDocument();
    });
});
