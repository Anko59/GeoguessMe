import { fireEvent, render, screen } from '@testing-library/react';
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
});

describe('GroupsList', () => {
    it('renders empty and populated groups, including a retry', async () => {
        mocks.get.mockRejectedValueOnce(new Error('temporary failure')).mockResolvedValueOnce({
            data: [{ id: 'group-1', name: 'Friends', code: 'ABC123' }],
        });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load groups');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        expect(await screen.findByText('Friends')).toBeInTheDocument();
        expect(screen.getByText('#ABC123')).toBeInTheDocument();

        mocks.get.mockResolvedValueOnce({ data: [] });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByText("You haven't joined any groups yet")).toBeInTheDocument();
    });
});
