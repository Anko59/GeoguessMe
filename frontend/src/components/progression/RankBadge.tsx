import type { ProgressionRank } from '../../types';
import { TIER_COUNT, tierForLevel } from './rankTiers';
import './RankBadge.css';

interface RankBadgeProps {
    rank: ProgressionRank;
    size?: 'inline' | 'large';
    alt?: string;
    className?: string;
}

/** Renders the AI-generated badge artwork for a progression rank. Inline
 *  badges sit next to rank names and are decorative; the large badge is the
 *  progression artwork on the profile hero. */
export default function RankBadge({ rank, size = 'inline', alt = '', className = '' }: RankBadgeProps) {
    const tier = Math.min(tierForLevel(rank.level), TIER_COUNT);
    return (
        <img
            src={`/rank-badges/tier-${tier}.png`}
            alt={alt}
            className={`rank-badge rank-badge--${size} ${className}`}
            width={size === 'large' ? 144 : 18}
            height={size === 'large' ? 144 : 18}
            loading="lazy"
            decoding="async"
        />
    );
}
