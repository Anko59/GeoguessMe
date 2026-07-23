import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import GroupJoin from './GroupJoin';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.post.mockReset();
});

function LocationDisplay() {
    const location = useLocation();
    return <output data-testid="location">{location.pathname}</output>;
}

describe('GroupJoin', () => {
    it('joins and creates groups and reports API errors', async () => {
        mocks.post.mockResolvedValueOnce({ data: { id: 'joined' } });
        const firstRender = render(
            <MemoryRouter initialEntries={['/group/join?code=abc123']}>
                <GroupJoin />
                <LocationDisplay />
            </MemoryRouter>,
        );
        expect(screen.getByDisplayValue('abc123')).toBeInTheDocument();
        await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/group/joined'));
        expect(mocks.post).toHaveBeenCalledWith('/group/join', { code: 'ABC123' });

        firstRender.unmount();
        mocks.post.mockRejectedValueOnce(new Error('bad group name'));
        render(
            <MemoryRouter initialEntries={['/group/create']}>
                <GroupJoin />
            </MemoryRouter>,
        );
        expect(screen.getAllByRole('button', { name: 'Create Group' })[0]).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('button', { name: 'Join Group' })).toHaveAttribute('aria-pressed', 'false');
        fireEvent.change(screen.getByPlaceholderText('Group Name'), { target: { value: 'Bad' } });
        fireEvent.click(screen.getAllByRole('button', { name: 'Create Group' })[1]);
        expect(await screen.findByText('bad group name')).toBeInTheDocument();
    });
});
