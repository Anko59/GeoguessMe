import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import BlockedUsersManager from './BlockedUsersManager';

const mocks = vi.hoisted(() => ({ list: vi.fn(), block: vi.fn(), unblock: vi.fn() }));
vi.mock('../../api', () => ({
    userBlocksAPI: mocks,
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));
const item = { user_id: 'bob-id', username: 'bob', avatar: 'custom', created_at: '2026-01-01T00:00:00Z' };

beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockReset().mockResolvedValue({ items: [item] });
    mocks.unblock.mockReset().mockResolvedValue(undefined);
});

describe('BlockedUsersManager', () => {
    it('lists identities without fetching private avatars and removes a successful unblock', async () => {
        render(<BlockedUsersManager />);
        fireEvent.click(await screen.findByRole('button', { name: 'Unblock bob' }));
        expect(await screen.findByText('No blocked users.')).toBeInTheDocument();
        expect(mocks.unblock).toHaveBeenCalledWith('bob-id', expect.any(AbortSignal));
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
    });
    it('preserves entries after failure and permits retry', async () => {
        mocks.unblock.mockRejectedValueOnce(new Error('Unavailable'));
        render(<BlockedUsersManager />);
        fireEvent.click(await screen.findByRole('button', { name: 'Unblock bob' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unavailable');
        fireEvent.click(screen.getByRole('button', { name: 'Unblock bob' }));
        expect(await screen.findByText('No blocked users.')).toBeInTheDocument();
    });
    it('retries failed loads and aborts list requests on unmount', async () => {
        mocks.list.mockRejectedValueOnce(new Error('Load failed'));
        const view = render(<BlockedUsersManager />);
        expect(await screen.findByRole('alert')).toHaveTextContent('Load failed');
        fireEvent.click(screen.getByRole('button', { name: 'Retry blocked users' }));
        expect(await screen.findByText('bob')).toBeInTheDocument();
        const signal = mocks.list.mock.calls[0][0] as AbortSignal;
        view.unmount();
        expect(signal.aborted).toBe(true);
    });
    it('disables duplicate mutations and ignores a late result after unmount', async () => {
        let resolve!: () => void;
        mocks.unblock.mockImplementationOnce(
            () =>
                new Promise<void>((done) => {
                    resolve = done;
                }),
        );
        const view = render(<BlockedUsersManager />);
        const button = await screen.findByRole('button', { name: 'Unblock bob' });
        fireEvent.click(button);
        await waitFor(() => expect(button).toBeDisabled());
        fireEvent.click(button);
        expect(mocks.unblock).toHaveBeenCalledTimes(1);
        const signal = mocks.unblock.mock.calls[0][1] as AbortSignal;
        view.unmount();
        expect(signal.aborted).toBe(true);
        await act(async () => resolve());
    });
});
