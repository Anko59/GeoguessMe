import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { PartyStatus } from '../../types';
import PartyButton from './PartyButton';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => {
        const serverMessage = (error as { response?: { data?: { error?: { message?: string } } } })?.response?.data
            ?.error?.message;
        return serverMessage ?? (error instanceof Error ? error.message : fallback);
    },
}));

function status(overrides: Partial<PartyStatus> = {}): PartyStatus {
    return { active: false, server_time: '2026-06-20T20:00:00Z', ...overrides };
}

const started = vi.fn();
const refresh = vi.fn();

beforeEach(() => {
    mocks.post.mockReset();
    started.mockClear();
    refresh.mockClear();
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true));
});

function renderButton(partyStatus: PartyStatus | null = status()) {
    return render(<PartyButton groupId="g1" status={partyStatus} onStarted={started} onRefresh={refresh} />);
}

describe('PartyButton', () => {
    it('offers starting when no party is recorded', () => {
        renderButton(status());
        const button = screen.getByRole('button', { name: 'Start party time' });
        expect(button).toBeEnabled();
    });

    it('disables and labels the button while a party is active', () => {
        renderButton(status({ active: true, ends_at: '2026-06-20T21:00:00Z' }));
        expect(screen.getByRole('button', { name: /Party time is active/ })).toBeDisabled();
    });

    it('grays out while recharging and publishes the ready time', () => {
        renderButton(status({ next_available_at: '2026-06-22T21:00:00Z' }));
        const button = screen.getByRole('button', { name: /recharging/ });
        expect(button).toBeDisabled();
        expect(button.className).toContain('party-recharging');
        expect(screen.getByRole('button', { name: /ready/ })).toBeInTheDocument();
    });

    it('starts a party after confirmation and reports it', async () => {
        mocks.post.mockResolvedValue({ data: status({ active: true, ends_at: '2026-06-20T21:00:00Z' }) });
        renderButton();
        fireEvent.click(screen.getByRole('button', { name: 'Start party time' }));
        await waitFor(() => expect(started).toHaveBeenCalledTimes(1));
        expect(mocks.post).toHaveBeenCalledWith('/group/party', { group_id: 'g1' });
        expect(refresh).not.toHaveBeenCalled();
    });

    it('does nothing when the confirmation is dismissed', async () => {
        vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));
        renderButton();
        fireEvent.click(screen.getByRole('button', { name: 'Start party time' }));
        expect(mocks.post).not.toHaveBeenCalled();
        expect(started).not.toHaveBeenCalled();
    });

    it('re-syncs authoritative state when the start conflicts', async () => {
        mocks.post.mockRejectedValue({
            response: { data: { error: { code: 'party_recharging', message: 'Party time is recharging' } } },
        });
        renderButton();
        fireEvent.click(screen.getByRole('button', { name: 'Start party time' }));
        await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
        expect(started).not.toHaveBeenCalled();
        // The error surfaces through the title tooltip for sighted users.
        await waitFor(() => expect(screen.getByRole('button').title).toContain('recharging'));
    });
});
