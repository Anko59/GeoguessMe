import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import GroupJoin from './GroupJoin';
import { PENDING_INVITE_TOKEN_KEY } from '../../hooks/useInviteFragmentCapture';

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
    window.sessionStorage.clear();
});

function LocationDisplay() {
    const location = useLocation();
    return <output data-testid="location">{location.pathname}</output>;
}

describe('GroupJoin', () => {
    it('previews an invite from sessionStorage and joins with the token in the request body', async () => {
        window.sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, 'tok123');
        mocks.post.mockImplementation((url: string) => {
            if (url === '/group/invites/preview') {
                return Promise.resolve({ data: { group_name: 'Friends', member_count: 3 } });
            }
            if (url === '/group/join') {
                return Promise.resolve({ data: { id: 'joined', name: 'Friends' } });
            }
            return Promise.reject(new Error('unexpected POST ' + url));
        });
        render(
            <MemoryRouter initialEntries={['/group/join']}>
                <GroupJoin />
                <LocationDisplay />
            </MemoryRouter>,
        );
        expect(await screen.findByText('Join Friends?')).toBeInTheDocument();
        expect(screen.getByText('3 members')).toBeInTheDocument();
        // The join action button lives in the form; the mode selector also
        // labels a button "Join Group", so scope the click to the form.
        const form = screen.getByText('Join Friends?').closest('.join-form');
        expect(form).not.toBeNull();
        fireEvent.click(within(form as HTMLElement).getByRole('button', { name: 'Join Group' }));
        await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/group/joined'));
        // The token travels only in the request body, never in the URL.
        expect(mocks.post).toHaveBeenCalledWith('/group/join', { invite_token: 'tok123' });
        expect(window.sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBeNull();
    });

    it('shows the invalid state and clears the token when the preview returns 404', async () => {
        window.sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, 'dead-token');
        const notFound = new Error('Request failed with status code 404');
        (notFound as { response?: { status?: number } }).response = { status: 404 };
        mocks.post.mockRejectedValue(notFound);
        render(
            <MemoryRouter initialEntries={['/group/join']}>
                <GroupJoin />
            </MemoryRouter>,
        );
        expect(await screen.findByText('This invite link is invalid or has expired')).toBeInTheDocument();
        expect(window.sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBeNull();
    });

    it('shows the missing state when no token is pending', async () => {
        render(
            <MemoryRouter initialEntries={['/group/join']}>
                <GroupJoin />
            </MemoryRouter>,
        );
        expect(await screen.findByText('No invite link found')).toBeInTheDocument();
    });

    it('creates groups and reports API errors', async () => {
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
