import { useId, type CSSProperties } from 'react';
import type { ProgressionRank } from '../../types';
import './RankTrophy.css';

interface RankTrophyProps {
    rank: ProgressionRank;
    className?: string;
}

const palettes = [
    ['#9ca3af', '#e5e7eb', '#4b5563'],
    ['#d97706', '#fbbf24', '#92400e'],
    ['#0f766e', '#5eead4', '#134e4a'],
    ['#2563eb', '#93c5fd', '#1e3a8a'],
    ['#7c3aed', '#c4b5fd', '#4c1d95'],
];

export default function RankTrophy({ rank, className = '' }: RankTrophyProps) {
    const id = useId().replaceAll(':', '');
    const [main, highlight, shadow] = palettes[(rank.level - 1) % palettes.length];
    const showCrown = rank.level >= 11;
    const showGem = rank.level >= 16;
    const gradientID = `trophy-gradient-${id}`;

    return (
        <svg
            className={`rank-trophy ${className}`}
            viewBox="0 0 128 128"
            role="img"
            aria-label={`${rank.name} trophy`}
            data-rank-level={rank.level}
            style={
                { '--trophy-main': main, '--trophy-highlight': highlight, '--trophy-shadow': shadow } as CSSProperties
            }
        >
            <defs>
                <linearGradient id={gradientID} x1="0" x2="1" y1="0" y2="1">
                    <stop offset="0" stopColor={highlight} />
                    <stop offset="0.48" stopColor={main} />
                    <stop offset="1" stopColor={shadow} />
                </linearGradient>
            </defs>
            {showCrown && (
                <path className="rank-trophy-crown" d="m44 20 8-11 12 9 12-9 8 11-5 9H49z" fill={highlight} />
            )}
            <path
                d="M43 30H28c0 16 7 25 18 27M85 30h15c0 16-7 25-18 27"
                fill="none"
                stroke={shadow}
                strokeLinecap="round"
                strokeWidth="8"
            />
            <path d="M42 27h44v27c0 17-9 28-22 28S42 71 42 54z" fill={`url(#${gradientID})`} />
            <path d="M56 80h16v12H56z" fill={shadow} />
            <path d="M42 92h44l8 10H34z" fill={shadow} />
            <path d="M34 102h60" fill="none" stroke={highlight} strokeLinecap="round" strokeWidth="5" />
            {showGem ? (
                <path d="m64 37 8 11-8 16-8-16z" fill={highlight} stroke={shadow} strokeWidth="2" />
            ) : (
                <circle cx="64" cy="50" r="9" fill={highlight} opacity="0.9" />
            )}
            <text x="64" y="75" fill={shadow} fontSize="12" fontWeight="900" textAnchor="middle">
                {rank.level}
            </text>
        </svg>
    );
}
