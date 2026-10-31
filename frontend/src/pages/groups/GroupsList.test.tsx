import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GroupsList from './GroupsList';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    inbox: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
    groupsAPI: { inbox: mocks.inbox },
    getAPIErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset().mockResolvedValue({ data: [] });
    mocks.inbox.mockReset().mockResolvedValue([]);
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:group-photo');
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
});

afterEach(() => vi.useRealTimers());

describe('GroupsList', () => {
    it('preserves server message order and updates it when another group becomes active', async () => {
        const older = { id: 'old', name: 'Older', unread_count: 0 };
        const recent = { id: 'new', name: 'Recent', unread_count: 0 };
        const empty = { id: 'empty', name: 'Empty', unread_count: 0 };
        mocks.inbox.mockResolvedValue([recent, older, empty]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await screen.findByText('Recent');
        const order = () =>
            Array.from(document.querySelectorAll('.group-card')).map((card) => card.getAttribute('data-group-id'));
        expect(order()).toEqual(['new', 'old', 'empty']);
        mocks.inbox.mockResolvedValue([older, recent, empty]);
        fireEvent.focus(window);
        await waitFor(() => expect(order()).toEqual(['old', 'new', 'empty']));
    });
    it('shows accessible server counts, caps large badges, and hides zero counts', async () => {
        mocks.inbox.mockResolvedValue([
            { id: 'unread', name: 'Friends', unread_count: 1 },
            { id: 'many', name: 'Travel', unread_count: 123 },
            { id: 'read', name: 'Family', unread_count: 0 },
        ]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByLabelText('1 unread message')).toHaveTextContent('1');
        expect(screen.getByLabelText('123 unread messages')).toHaveTextContent('99+');
        expect(screen.getByRole('img', { name: '123 unread messages' })).not.toHaveAttribute('aria-live');
        expect(screen.getByRole('link', { name: /Travel/ }).querySelector('[role="status"]')).toBeNull();
        expect(screen.getByRole('link', { name: /Family/ }).querySelector('.group-unread-badge')).toBeNull();
        expect(mocks.inbox).toHaveBeenCalledWith(expect.any(AbortSignal));
    });

    it('refreshes on focus and clears the badge after reading on another device', async () => {
        mocks.inbox.mockResolvedValue([{ id: 'focus', name: 'Friends', unread_count: 2 }]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await screen.findByLabelText('2 unread messages');
        mocks.inbox.mockResolvedValue([{ id: 'focus', name: 'Friends', unread_count: 0 }]);
        fireEvent.focus(window);
        await waitFor(() => expect(screen.queryByLabelText('2 unread messages')).not.toBeInTheDocument());
        expect(screen.getByRole('link', { name: /Friends/ })).toBeInTheDocument();
    });

    it('keeps cards during a failed background refresh and recovers on focus', async () => {
        mocks.inbox.mockResolvedValue([{ id: 'recover', name: 'Friends', unread_count: 3 }]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await screen.findByLabelText('3 unread messages');
        mocks.inbox.mockRejectedValueOnce(new Error('offline'));
        fireEvent.focus(window);
        expect(await screen.findByText('Unable to refresh unread counts. Try again.')).toBeInTheDocument();
        expect(screen.getByLabelText('3 unread messages')).toBeInTheDocument();
        fireEvent.focus(window);
        await waitFor(() =>
            expect(screen.queryByText('Unable to refresh unread counts. Try again.')).not.toBeInTheDocument(),
        );
    });

    it('polls while visible, refreshes on return, and cleans up all work on unmount', async () => {
        vi.useFakeTimers();
        mocks.inbox.mockResolvedValue([{ id: 'poll', name: 'Friends', unread_count: 1 }]);
        const visibility = vi.spyOn(document, 'visibilityState', 'get');
        const { unmount } = render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(screen.getByLabelText('1 unread message')).toBeInTheDocument();
        mocks.inbox.mockResolvedValue([{ id: 'poll', name: 'Friends', unread_count: 2 }]);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(15000);
        });
        expect(screen.getByLabelText('2 unread messages')).toBeInTheDocument();
        visibility.mockReturnValue('hidden');
        await act(async () => {
            await vi.advanceTimersByTimeAsync(30000);
        });
        expect(mocks.inbox).toHaveBeenCalledTimes(2);
        visibility.mockReturnValue('visible');
        await act(async () => {
            document.dispatchEvent(new Event('visibilitychange'));
        });
        expect(mocks.inbox).toHaveBeenCalledTimes(3);
        const signal = mocks.inbox.mock.calls[2][0] as AbortSignal;
        unmount();
        expect(signal.aborted).toBe(true);
        fireEvent.focus(window);
        await vi.advanceTimersByTimeAsync(30000);
        expect(mocks.inbox).toHaveBeenCalledTimes(3);
    });

    it('does not overlap requests and aborts the pending request on unmount', async () => {
        vi.useFakeTimers();
        let complete: (items: unknown[]) => void = () => undefined;
        mocks.inbox.mockImplementation(
            () =>
                new Promise((resolve) => {
                    complete = resolve;
                }),
        );
        const { unmount } = render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await act(async () => {
            await vi.advanceTimersByTimeAsync(45000);
        });
        fireEvent.focus(window);
        expect(mocks.inbox).toHaveBeenCalledTimes(1);
        const signal = mocks.inbox.mock.calls[0][0] as AbortSignal;
        expect(signal.aborted).toBe(false);
        unmount();
        expect(signal.aborted).toBe(true);
        await act(async () => {
            complete([{ id: 'stale', name: 'Stale', unread_count: 4 }]);
        });
        fireEvent.focus(window);
        expect(mocks.inbox).toHaveBeenCalledTimes(1);
    });

    it('loads initially in a hidden tab, then pauses background polling', async () => {
        vi.useFakeTimers();
        vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
        mocks.inbox.mockResolvedValue([{ id: 'hidden', name: 'Friends', unread_count: 0 }]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(screen.getByText('Friends')).toBeInTheDocument();
        expect(screen.queryByText('Loading your groups…')).not.toBeInTheDocument();
        await act(async () => {
            await vi.advanceTimersByTimeAsync(30000);
        });
        expect(mocks.inbox).toHaveBeenCalledTimes(1);
    });

    it('keeps loaded cards and refresh wording while retrying another failure', async () => {
        mocks.inbox.mockResolvedValue([{ id: 'retry', name: 'Friends', unread_count: 3 }]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await screen.findByText('Friends');
        mocks.inbox.mockRejectedValueOnce(new Error('offline'));
        fireEvent.focus(window);
        await screen.findByText('Unable to refresh unread counts. Try again.');
        let rejectRetry!: (reason: Error) => void;
        mocks.inbox.mockImplementationOnce(
            () =>
                new Promise((_resolve, reject) => {
                    rejectRetry = reject;
                }),
        );
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        expect(screen.getByLabelText('3 unread messages')).toBeInTheDocument();
        expect(screen.queryByText('Loading your groups…')).not.toBeInTheDocument();
        await act(async () => {
            rejectRetry(new Error('still offline'));
        });
        expect(screen.getByText('Unable to refresh unread counts. Try again.')).toBeInTheDocument();
        expect(screen.getByRole('link', { name: /Friends/ })).toBeInTheDocument();
    });

    it('resumes polling after a request times out', async () => {
        vi.useFakeTimers();
        mocks.inbox.mockResolvedValue([{ id: 'timeout', name: 'Friends', unread_count: 1 }]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        mocks.inbox.mockImplementationOnce(
            () =>
                new Promise((_resolve, reject) => {
                    window.setTimeout(() => reject(new Error('timeout')), 10000);
                }),
        );
        fireEvent.focus(window);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(10000);
        });
        expect(screen.getByText('Unable to refresh unread counts. Try again.')).toBeInTheDocument();
        expect(screen.getByLabelText('1 unread message')).toBeInTheDocument();
        await act(async () => {
            await vi.advanceTimersByTimeAsync(5000);
        });
        expect(mocks.inbox).toHaveBeenCalledTimes(3);
        expect(screen.queryByText('Unable to refresh unread counts. Try again.')).not.toBeInTheDocument();
    });

    it('renders empty and populated groups, including a retry', async () => {
        mocks.inbox
            .mockRejectedValueOnce(new Error('temporary failure'))
            .mockResolvedValueOnce([{ id: 'group-1', name: 'Friends', unread_count: 0 }])
            .mockResolvedValue([]);
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

        mocks.inbox.mockResolvedValueOnce([]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByText("You haven't joined any groups yet")).toBeInTheDocument();
    });

    it('links to the own profile and personal settings from the topbar', async () => {
        mocks.inbox.mockResolvedValue([]);
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        await screen.findByText("You haven't joined any groups yet");

        expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('href', '/profile');
        expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
        expect(screen.getByRole('link', { name: 'Explore feed' })).toHaveAttribute('href', '/feed');
        expect(screen.getByRole('link', { name: 'Play public challenges' })).toHaveAttribute('href', '/feed');
    });
});
