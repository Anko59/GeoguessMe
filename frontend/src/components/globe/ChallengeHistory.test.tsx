import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { GroupChallenge } from '../../types';
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
    it('bounds the rendered list and keeps every challenge reachable', () => {
        const select = vi.fn();
        render(<ChallengeHistory items={items} selectedID={null} onSelect={select} />);
        expect(screen.getAllByRole('listitem')).toHaveLength(50);
        expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled();
        fireEvent.click(screen.getByRole('button', { name: 'Next' }));
        expect(screen.getByText('Page 2 of 3')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Next' }));
        expect(screen.getAllByRole('listitem')).toHaveLength(23);
        expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled();
        fireEvent.click(screen.getByRole('button', { name: /Player 122 / }));
        expect(select).toHaveBeenCalledWith('p122');
        fireEvent.click(screen.getByRole('button', { name: 'Previous' }));
        expect(screen.getByText('Page 2 of 3')).toBeInTheDocument();
    });
    it('searches the full history, resets pagination and distinguishes playable and revealed challenges', () => {
        render(<ChallengeHistory items={items} selectedID={null} onSelect={vi.fn()} />);
        fireEvent.click(screen.getByRole('button', { name: 'Next' }));
        fireEvent.change(screen.getByRole('searchbox'), { target: { value: ' PLAYER 122 ' } });
        expect(screen.getAllByRole('listitem')).toHaveLength(1);
        expect(screen.getByText('Player 122')).toBeInTheDocument();
        fireEvent.change(screen.getByRole('combobox'), { target: { value: 'available' } });
        expect(screen.getByRole('status')).toHaveTextContent('No challenges match');
        fireEvent.change(screen.getByRole('combobox'), { target: { value: 'located' } });
        expect(screen.getByText('Player 122')).toBeInTheDocument();
    });
});
