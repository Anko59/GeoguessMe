import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { GroupChallenge } from '../../types';
import GroupGlobe from './GroupGlobe';

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('../../api', () => ({ default: { get }, getAPIErrorMessage: (_: unknown, fallback: string) => fallback }));
vi.mock('./Globe', () => ({
    default: ({ items }: { items: GroupChallenge[] }) => (
        <div data-testid="pins">
            {items
                .filter((item) => item.lat !== undefined)
                .map((item) => item.photo_id)
                .join(',')}
        </div>
    ),
}));
const challenge: GroupChallenge = {
    photo_id: 'p1',
    group_id: 'g1',
    user_id: 'u1',
    username: 'Alice',
    created_at: '2026-09-12T12:00:00Z',
    expires_at: '2026-09-13T12:00:00Z',
    status: 'results',
    lat: 0,
    long: 0,
};
const props = { groupID: 'g1', groupName: 'Friends', onClose: vi.fn(), onChallenge: vi.fn() };

beforeEach(() => {
    vi.clearAllMocks();
    get.mockReset();
    HTMLDialogElement.prototype.showModal = function () {
        this.setAttribute('open', '');
    };
    HTMLDialogElement.prototype.close = function () {
        this.removeAttribute('open');
    };
});

describe('GroupGlobe', () => {
    it('loads every page independently of chat history and opens selected results', async () => {
        get.mockResolvedValueOnce({ data: { items: [challenge], next_cursor: 'next' } }).mockResolvedValueOnce({
            data: {
                items: [
                    {
                        ...challenge,
                        photo_id: 'p2',
                        username: 'Bob',
                        lat: undefined,
                        long: undefined,
                        status: 'available',
                    },
                ],
            },
        });
        render(<GroupGlobe {...props} />);
        await screen.findByText('2 challenges · 1 on the globe');
        expect(get).toHaveBeenNthCalledWith(
            2,
            '/group/challenges',
            expect.objectContaining({ params: { group_id: 'g1', cursor: 'next' } }),
        );
        expect(screen.getByTestId('pins')).toHaveTextContent('p1');
        expect(screen.getByTestId('pins')).not.toHaveTextContent('p2');
        fireEvent.click(screen.getByRole('button', { name: /Alice/ }));
        fireEvent.click(screen.getByRole('button', { name: 'View results' }));
        expect(props.onChallenge).toHaveBeenCalledWith(
            expect.objectContaining({ photo_id: 'p1', group_id: 'g1', kind: 'challenge' }),
        );
        fireEvent.click(screen.getByRole('button', { name: /Bob/ }));
        expect(screen.getByRole('button', { name: 'Play challenge' })).toBeInTheDocument();
    });

    it('shows empty and failed states and refreshes after an error', async () => {
        get.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ data: { items: [] } });
        render(<GroupGlobe {...props} />);
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load group challenges');
        fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
        expect(await screen.findByText(/No geochallenges yet/)).toBeInTheDocument();
        expect(screen.queryByRole('alert')).toBeNull();
    });

    it('aborts old group requests and never renders their late responses', async () => {
        let resolveOld!: (response: unknown) => void;
        get.mockImplementationOnce(
            () =>
                new Promise((resolve) => {
                    resolveOld = resolve;
                }),
        ).mockResolvedValueOnce({ data: { items: [] } });
        const view = render(<GroupGlobe {...props} />);
        const signal = get.mock.calls[0][1].signal as AbortSignal;
        view.rerender(<GroupGlobe {...props} groupID="g2" />);
        await screen.findByText(/No geochallenges yet/);
        expect(signal.aborted).toBe(true);
        resolveOld({ data: { items: [challenge] } });
        await waitFor(() => expect(screen.queryByText('Alice')).toBeNull());
        expect(screen.getByTestId('pins')).toBeEmptyDOMElement();
    });

    it('clears partial history when a later page fails and stops repeated cursors', async () => {
        get.mockResolvedValueOnce({ data: { items: [challenge], next_cursor: 'repeat' } }).mockResolvedValueOnce({
            data: { items: [], next_cursor: 'repeat' },
        });
        render(<GroupGlobe {...props} />);
        await screen.findByRole('alert');
        expect(get).toHaveBeenCalledTimes(2);
        expect(screen.queryByText('Alice')).toBeNull();
    });

    it('closes via Escape or the close button and restores focus on unmount', async () => {
        get.mockResolvedValue({ data: { items: [] } });
        const trigger = document.createElement('button');
        document.body.append(trigger);
        trigger.focus();
        const view = render(<GroupGlobe {...props} />);
        await screen.findByText(/No geochallenges yet/);
        fireEvent(screen.getByRole('dialog'), new Event('cancel', { bubbles: true, cancelable: true }));
        expect(props.onClose).toHaveBeenCalledTimes(1);
        fireEvent.click(screen.getByRole('button', { name: 'Close group globe' }));
        expect(props.onClose).toHaveBeenCalledTimes(2);
        view.unmount();
        expect(document.activeElement).toBe(trigger);
        expect(document.body.style.overflow).not.toBe('hidden');
        trigger.remove();
    });
});
