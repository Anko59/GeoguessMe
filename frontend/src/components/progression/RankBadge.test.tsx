import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import RankBadge from './RankBadge';
import { tierForLevel } from './rankTiers';
import type { ProgressionRank } from '../../types';

const rankAt = (level: number): ProgressionRank => ({
    level,
    name: 'Rank',
    min_points: 0,
    points_in_rank: 0,
    points_to_next: 100,
    progress_percent: 0,
    trophy_key: 'page',
});

describe('tierForLevel', () => {
    it('groups the 20 levels into the five badge tiers', () => {
        expect(tierForLevel(1)).toBe(1);
        expect(tierForLevel(4)).toBe(1);
        expect(tierForLevel(5)).toBe(2);
        expect(tierForLevel(8)).toBe(2);
        expect(tierForLevel(9)).toBe(3);
        expect(tierForLevel(12)).toBe(3);
        expect(tierForLevel(13)).toBe(4);
        expect(tierForLevel(16)).toBe(4);
        expect(tierForLevel(17)).toBe(5);
        expect(tierForLevel(20)).toBe(5);
    });
});

describe('RankBadge', () => {
    it('renders the tier badge artwork for the rank', () => {
        render(<RankBadge rank={rankAt(6)} alt="Knight badge" />);
        expect(screen.getByRole('img', { name: 'Knight badge' })).toHaveAttribute('src', '/rank-badges/tier-2.png');
    });

    it('renders a large badge for the profile hero', () => {
        render(<RankBadge rank={rankAt(20)} size="large" alt="Emperor badge" />);
        const img = screen.getByRole('img', { name: 'Emperor badge' });
        expect(img).toHaveAttribute('src', '/rank-badges/tier-5.png');
        expect(img).toHaveClass('rank-badge--large');
    });

    it('is decorative by default', () => {
        const { container } = render(<RankBadge rank={rankAt(1)} />);
        const img = container.querySelector('.rank-badge') as HTMLImageElement;
        expect(img).toHaveAttribute('alt', '');
        expect(img).not.toHaveAttribute('aria-label');
    });
});
