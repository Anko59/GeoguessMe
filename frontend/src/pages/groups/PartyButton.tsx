import { useCallback, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import type { PartyStatus } from '../../types';

interface PartyButtonProps {
    groupId: string;
    status: PartyStatus | null;
    /** Notify the owner that a party was started so it can re-sync state. */
    onStarted: () => void;
    /** Ask the hook for authoritative state after a conflict or failure. */
    onRefresh: () => void;
}

/**
 * The header Party Time control between the group name and the player's
 * avatar. Three visual states fall out of the wire contract: an active party
 * (celebrating, disabled), a recharging cooldown (grayed, disabled, with the
 * ready time in its accessible label), and available to start. Starting
 * requires a deliberate confirmation because it notifies the whole group and
 * locks the group's next party for 48 hours.
 */
export default function PartyButton({ groupId, status, onStarted, onRefresh }: PartyButtonProps) {
    const [starting, setStarting] = useState(false);
    const [error, setError] = useState('');

    const active = status?.active ?? false;
    const recharging = !active && !!status?.next_available_at;

    const startParty = useCallback(async (): Promise<void> => {
        if (
            !window.confirm(
                'Start Party Time? Points are doubled for challenge posters for the next hour, and the group must wait 48 hours after it ends before starting another.',
            )
        ) {
            return;
        }
        setStarting(true);
        setError('');
        try {
            const response = await api.post<PartyStatus>('/group/party', { group_id: groupId });
            onStarted(response.data);
        } catch (requestError: unknown) {
            const code = (requestError as { response?: { data?: { error?: { code?: string } } } })?.response?.data
                ?.error?.code;
            if (code === 'party_active' || code === 'party_recharging') {
                // Someone else started a party first, or the cooldown just
                // ended differently than locally assumed: re-sync.
                onRefresh();
            }
            setError(getAPIErrorMessage(requestError, 'Party time could not be started.'));
        } finally {
            setStarting(false);
        }
    }, [groupId, onRefresh, onStarted]);

    const label = active
        ? 'Party time is active — points are doubled for challenge posters'
        : recharging
          ? `Party time is recharging${rechargeLabel(status)}`
          : 'Start party time';

    return (
        <span className="party-button-anchor">
            <button
                type="button"
                className={`party-btn${active ? ' party-active' : ''}${recharging ? ' party-recharging' : ''}`}
                onClick={() => void startParty()}
                disabled={active || recharging || starting}
                aria-label={label}
                title={error || label}
            >
                <span className="party-emoji" aria-hidden="true">
                    🎉
                </span>
            </button>
        </span>
    );
}

function rechargeLabel(status: PartyStatus | null): string {
    if (!status?.next_available_at) return '';
    const formatted = new Date(status.next_available_at);
    const timeText = Number.isNaN(formatted.getTime())
        ? status.next_available_at
        : formatted.toLocaleString(undefined, { weekday: 'short', hour: '2-digit', minute: '2-digit' });
    return ` — ready ${timeText}`;
}
