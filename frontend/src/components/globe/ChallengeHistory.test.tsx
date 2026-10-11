import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { GroupChallenge } from '../../types';
import { DEFAULT_MAP_PIN_IMAGE_URL } from '../../utils/mapPins';
import ChallengeHistory from './ChallengeHistory';

const items: GroupChallenge[] = Array.from({ length: 123 }, (_, index) => ({
    photo_id: `p${index}`,
    group_id: 'g',
    user_id: 'u',
    username: `Player ${index}`,
    created_at: '2026-09-12T12:00:00Z',
    expires_at: '2026-09-13T12:00:00Z',
    status: index % 2 ? 'available' : 'results',
    ...(index % 2 ? {} : { lat: 0, long: 0 }),
}));

describe('Challenge history', () => {
    it('shows the standard marker for located challenges and hides artwork for hidden locations', () => {
        render(
            <ChallengeHistory
                items={[
                    { ...items[0], lat: 48.8, long: 2.3 },
                    { ...items[1], lat: undefined, long: undefined },
                ]}
                selectedID={null}
                onSelect={vi.fn()}
            />,
        );
        expect(document.querySelector('.globe-location-pin')).toHaveAttribute('src', DEFAULT_MAP_PIN_IMAGE_URL);
        expect(document.querySelector('.globe-location-dot.is-hidden')).toBeInTheDocument();
    });

    it('bounds the rendered list and keeps every challenge reachable', () => {
        const select = vi.fn();
        render(<ChallengeHistory items={items} selectedID={null} onSelect={select} />);
        const pages = within(screen.getByRole('navigation', { name: 'Challenge pages' }));
        expect(screen.getAllByRole('listitem')).toHaveLength(50);
        expect(pages.getByRole('button', { name: 'Previous' })).toBeDisabled();
        fireEvent.click(pages.getByRole('button', { name: 'Next' }));
        expect(screen.getByText('Page 2 of 3')).toBeInTheDocument();
        fireEvent.click(pages.getByRole('button', { name: 'Next' }));
        expect(screen.getAllByRole('listitem')).toHaveLength(23);
        expect(pages.getByRole('button', { name: 'Next' })).toBeDisabled();
        fireEvent.click(screen.getByRole('button', { name: /Player 122 / }));
        expect(select).toHaveBeenCalledWith('p122');
        fireEvent.click(pages.getByRole('button', { name: 'Previous' }));
        expect(screen.getByText('Page 2 of 3')).toBeInTheDocument();
    });
    it('searches the full history, resets pagination and distinguishes playable and revealed challenges', () => {
        render(<ChallengeHistory items={items} selectedID={null} onSelect={vi.fn()} />);
        const pages = within(screen.getByRole('navigation', { name: 'Challenge pages' }));
        fireEvent.click(pages.getByRole('button', { name: 'Next' }));
        fireEvent.change(screen.getByLabelText('Find a player'), { target: { value: ' PLAYER 122 ' } });
        expect(screen.getAllByRole('listitem')).toHaveLength(1);
        expect(screen.getByText('Player 122')).toBeInTheDocument();
        fireEvent.change(screen.getByLabelText('Filter challenge list'), { target: { value: 'available' } });
        expect(screen.getByRole('status')).toHaveTextContent('No challenges match');
        fireEvent.change(screen.getByLabelText('Filter challenge list'), { target: { value: 'located' } });
        expect(screen.getByText('Player 122')).toBeInTheDocument();
    });
});
