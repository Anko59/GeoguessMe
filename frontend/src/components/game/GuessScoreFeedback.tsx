import type { AnimationEvent } from 'react';
import type { GuessFeedback } from './guessFeedback';
import './GuessScoreFeedback.css';

interface GuessScoreFeedbackProps {
    feedback: GuessFeedback;
    score: number;
    onDismiss: () => void;
}

export default function GuessScoreFeedback({ feedback, score, onDismiss }: GuessScoreFeedbackProps) {
    const handleAnimationEnd = (event: AnimationEvent<HTMLDivElement>) => {
        if (event.animationName === 'guessFeedbackExit') onDismiss();
    };

    return (
        <div className="guess-feedback" role="status" aria-live="polite">
            <div
                className={`guess-feedback-card guess-feedback-card--${feedback.tone}`}
                onAnimationEnd={handleAnimationEnd}
            >
                <span className="guess-feedback-kicker">Guess scored</span>
                <strong>{feedback.label}</strong>
                <span className="guess-feedback-score">{score.toLocaleString()} points</span>
                <span className="guess-feedback-subtitle">{feedback.subtitle}</span>
                <button type="button" className="guess-feedback-dismiss" onClick={onDismiss}>
                    Continue
                </button>
            </div>
        </div>
    );
}
