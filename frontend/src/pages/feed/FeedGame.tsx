import { useCallback, useContext, type ReactNode } from 'react';
import { AuthContext } from '../../context/AuthContext';
import GameView from '../../components/game/GameViews';
import GuessScoreFeedback from '../../components/game/GuessScoreFeedback';
import { useTimedGame } from '../../hooks/useTimedGame';
import { feedTimedGameAdapter } from '../../hooks/feedTimedGameAdapter';

/** Public feed game shell. It deliberately renders the shared full-screen
 * group game view so feed challenges cannot drift into a second game UX. */
export default function FeedGame({
    id,
    isOwner,
    openResultsDirectly,
    restoreFocus,
    onClose,
    onResolved,
    resultsFooter,
}: {
    id: string;
    isOwner: boolean;
    openResultsDirectly: boolean;
    restoreFocus: () => void;
    onClose: () => void;
    onResolved: () => void;
    resultsFooter?: ReactNode;
}) {
    const auth = useContext(AuthContext);
    const user = auth?.user;
    const handleClose = useCallback(() => {
        onClose();
        restoreFocus();
    }, [onClose, restoreFocus]);
    const onStatusChange = useCallback(
        (_id: string, status: 'accepted' | 'guessed' | 'results') => {
            if (status === 'guessed') onResolved();
        },
        [onResolved],
    );
    const timedGame = useTimedGame({
        challengeId: id,
        currentUserId: user?.id,
        requiresCurrentUser: false,
        isOwner,
        openResultsDirectly,
        checkResultsBeforeAccept: false,
        adapter: feedTimedGameAdapter,
        onStatusChange,
        onClose: handleClose,
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
            resultsFooter={resultsFooter}
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
