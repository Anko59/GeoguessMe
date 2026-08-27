import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import GuessScoreFeedback from './GuessScoreFeedback';
import { feedbackForScore } from './guessFeedback';

describe('GuessScoreFeedback', () => {
    it('renders the score label and dismisses after the exit animation', () => {
        const onDismiss = vi.fn();
        render(<GuessScoreFeedback feedback={feedbackForScore(4920)} score={4920} onDismiss={onDismiss} />);

        const card = screen.getByText('Masterstroke').closest('.guess-feedback-card');
        expect(card).toHaveClass('guess-feedback-card--excellent');
        expect(screen.getByRole('status')).toHaveTextContent('4,920 points');
        fireEvent.animationEnd(card!, { animationName: 'guessFeedbackExit' });
        expect(onDismiss).toHaveBeenCalledOnce();
    });

    it('shows the party ×2 badge only when the guess was doubled', () => {
        render(<GuessScoreFeedback feedback={feedbackForScore(4920)} score={4920} partyDoubled onDismiss={vi.fn()} />);
        expect(screen.getByText('🎉 ×2')).toBeInTheDocument();

        render(<GuessScoreFeedback feedback={feedbackForScore(100)} score={100} onDismiss={vi.fn()} />);
        expect(screen.getAllByText('4,920 points').length).toBe(1);
        // The second card (score 100) has no badge.
        expect(screen.queryAllByText('🎉 ×2').length).toBe(1);
    });

    it('lets players dismiss the feedback immediately', () => {
        const onDismiss = vi.fn();
        render(<GuessScoreFeedback feedback={feedbackForScore(100)} score={100} onDismiss={onDismiss} />);

        fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
        expect(onDismiss).toHaveBeenCalledOnce();
        expect(screen.getByText('Cartographic Catastrophe')).toBeInTheDocument();
    });
});
