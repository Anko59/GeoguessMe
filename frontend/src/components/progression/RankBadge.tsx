import type { ProgressionRank } from '../../types';
import './RankBadge.css';

interface RankBadgeProps {
    rank: ProgressionRank;
    size?: 'inline' | 'large';
    alt?: string;
    className?: string;
}

const ROMAN_NUMERALS = [
    'I',
    'II',
    'III',
    'IV',
    'V',
    'VI',
    'VII',
    'VIII',
    'IX',
    'X',
    'XI',
    'XII',
    'XIII',
    'XIV',
    'XV',
    'XVI',
    'XVII',
    'XVIII',
    'XIX',
    'XX',
];

function toRoman(level: number): string {
    return ROMAN_NUMERALS[Math.min(Math.max(level, 1), ROMAN_NUMERALS.length) - 1] ?? '';
}

/** Renders the AI-generated badge artwork for a progression rank with the
 *  rank number in roman numerals beneath it, so the ordering is readable even
 *  when only the badge is in view. Each of the 20 ranks has its own emblem;
 *  the material step-up (bronze → silver → gold shields, then royal →
 *  imperial crowns) reinforces the ordering. Inline badges sit next to rank
 *  names and are decorative; the large badge is the progression artwork on
 *  the profile hero. */
export default function RankBadge({ rank, size = 'inline', alt = '', className = '' }: RankBadgeProps) {
    return (
        <span className={`rank-badge-stack rank-badge-stack--${size} ${className}`}>
            <img
                src={`/rank-badges/${rank.trophy_key}.png`}
                alt={alt}
                className={`rank-badge rank-badge--${size}`}
                width={size === 'large' ? 144 : 18}
                height={size === 'large' ? 144 : 18}
                loading="lazy"
                decoding="async"
            />
            <span className="rank-badge-numeral">{toRoman(rank.level)}</span>
        </span>
    );
}
