import { useAuth } from '../../context/AuthContext';
import type { Message } from '../../types';
import GameView from './GameViews';
import GuessScoreFeedback from './GuessScoreFeedback';
import { groupTimedGameAdapter } from '../../hooks/groupTimedGameAdapter';
import { useTimedGame } from '../../hooks/useTimedGame';
import './Game.css';

interface GameProps {
    gameMessage: Message | null;
    onChallengeStatusChange?: (photoId: string, status: NonNullable<Message['challenge_status']>) => void;
    onClose: () => void;
}

/** Group challenge shell. All source-specific HTTP details live in the group
 * adapter; the timed-game orchestration is shared with the public feed. */
export default function Game({ gameMessage, onChallengeStatusChange, onClose }: GameProps) {
    const { user } = useAuth();
    const timedGame = useTimedGame({
        challengeId: gameMessage?.photo_id ?? undefined,
        currentUserId: user?.id,
        isOwner: gameMessage?.user_id === user?.id,
        adapter: groupTimedGameAdapter(),
        onStatusChange: onChallengeStatusChange,
        onClose,
    });

    return (
        <GameView
            state={timedGame.state}
            loadingMedia={timedGame.loadingMedia}
            remaining={timedGame.remaining}
            guessRemaining={timedGame.guessRemaining}
            guessTotalSeconds={timedGame.guessTotalSeconds}
            potentialScore={timedGame.potentialScore}
            scoreNotice={timedGame.scoreNotice}
            serverNowMs={timedGame.serverNowMs}
            feedback={
                timedGame.state.feedback ? (
                    <GuessScoreFeedback
                        feedback={timedGame.state.feedback.feedback}
                        score={timedGame.state.feedback.score}
                        partyDoubled={timedGame.state.feedback.partyDoubled}
                        onDismiss={timedGame.dismissFeedback}
                    />
                ) : null
            }
            currentUserId={user?.id}
            onSelectLocation={timedGame.selectLocation}
            onSubmitGuess={timedGame.submitGuess}
            onClose={timedGame.close}
        />
    );
}
