import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import GroupsList from './GroupsList';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:group-photo');
});

describe('GroupsList', () => {
    it('renders empty and populated groups, including a retry', async () => {
        mocks.get
            .mockRejectedValueOnce(new Error('temporary failure'))
            .mockResolvedValueOnce({ data: [{ id: 'group-1', name: 'Friends' }] })
            .mockResolvedValue({ data: [] });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load groups');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        expect(await screen.findByText('Friends')).toBeInTheDocument();
        await waitFor(() => {
            const card = screen.getByRole('link', { name: /Friends/ });
            expect(card.querySelector('.group-icon')).toHaveAttribute('src', 'blob:group-photo');
        });

        mocks.get.mockResolvedValueOnce({ data: [] });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByText("You haven't joined any groups yet")).toBeInTheDocument();
    });

    it('links to the own profile and personal settings from the topbar', async () => {
        mocks.get.mockResolvedValue({ data: [] });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await screen.findByText("You haven't joined any groups yet");

        expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('href', '/profile');
        expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
    });
});
