import { useEffect, useState } from 'react';
import './ChallengeTimer.css';

interface ChallengeTimerProps {
    expiresAt: string;
    ttlSeconds: number;
}

const TICK_MS = 30_000;
const RADIUS = 15.5;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

function formatRemaining(remainingMs: number): string {
    if (remainingMs <= 0) return '0h';
    const hours = remainingMs / 3_600_000;
    if (hours < 1) return `${Math.max(1, Math.ceil(remainingMs / 60_000))}m`;
    return `${Math.max(1, Math.round(hours))}h`;
}

/** Circular countdown showing how much of the challenge's deadline remains.
 *  The ring depletes once per full TTL and refreshes every 30 seconds. */
export default function ChallengeTimer({ expiresAt, ttlSeconds }: ChallengeTimerProps) {
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        const timer = window.setInterval(() => setNow(Date.now()), TICK_MS);
        return () => window.clearInterval(timer);
    }, []);
    const totalMs = ttlSeconds * 1000;
    const remainingMs = Math.max(0, Date.parse(expiresAt) - now);
    const fraction = Math.min(1, Math.max(0, remainingMs / totalMs));
    return (
        <span className="challenge-timer" role="img" aria-label={`${formatRemaining(remainingMs)} remaining`}>
            <svg viewBox="0 0 36 36" aria-hidden="true">
                <circle className="challenge-timer-track" cx="18" cy="18" r={RADIUS} />
                <circle
                    className="challenge-timer-value"
                    cx="18"
                    cy="18"
                    r={RADIUS}
                    strokeDasharray={CIRCUMFERENCE}
                    strokeDashoffset={CIRCUMFERENCE * (1 - fraction)}
                />
            </svg>
            <span className="challenge-timer-label">{formatRemaining(remainingMs)}</span>
        </span>
    );
}
