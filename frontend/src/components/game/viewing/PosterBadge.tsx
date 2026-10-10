import Avatar from '../../common/Avatar';
import type { GamePoster } from '../gameState';
import './PosterBadge.css';

/** The "Posted by" badge on the viewing view: it shows the submitting
 *  player's avatar and name in the top-left corner while the private photo
 *  counts down, so the guesser can see whose challenge they are playing. The
 *  identity comes from the data the challenge source already carries (the
 *  chat message or feed post), so no extra request is needed. */
export default function PosterBadge({ poster }: { poster: GamePoster }) {
    return (
        <div className="challenge-poster">
            <span className="challenge-poster__avatar" aria-hidden="true">
                <Avatar
                    userID={poster.userId}
                    avatar={poster.avatar}
                    username={poster.username}
                    className="challenge-poster__avatar-img"
                />
            </span>
            <span className="challenge-poster__text">
                <span className="challenge-poster__label">Posted by</span>
                <span className="challenge-poster__name">{poster.username ?? 'Unknown player'}</span>
            </span>
        </div>
    );
}
