import type { ProgressionRank } from '../../types';
import './RankBadge.css';

interface RankBadgeProps {
    rank: ProgressionRank;
    size?: 'inline' | 'large';
    alt?: string;
    className?: string;
}

/** Renders the AI-generated badge artwork for a progression rank. Each of the
 *  20 ranks has its own emblem; the material step-up (bronze → silver → gold
 *  shields, then royal → imperial crowns) keeps the ordering readable. Inline
 *  badges sit next to rank names and are decorative; the large badge is the
 *  progression artwork on the profile hero. */
export default function RankBadge({ rank, size = 'inline', alt = '', className = '' }: RankBadgeProps) {
    return (
        <img
            src={`/rank-badges/${rank.trophy_key}.png`}
            alt={alt}
            className={`rank-badge rank-badge--${size} ${className}`}
            width={size === 'large' ? 144 : 18}
            height={size === 'large' ? 144 : 18}
            loading="lazy"
            decoding="async"
        />
    );
}
