import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import RankBadge from './RankBadge';
import type { ProgressionRank } from '../../types';

const rankAt = (trophyKey: string, level: number): ProgressionRank => ({
    level,
    name: 'Rank',
    min_points: 0,
    points_in_rank: 0,
    points_to_next: 100,
    progress_percent: 0,
    trophy_key: trophyKey,
});

describe('RankBadge', () => {
    it('renders the badge artwork for the rank', () => {
        render(<RankBadge rank={rankAt('squire', 2)} alt="Squire badge" />);
        expect(screen.getByRole('img', { name: 'Squire badge' })).toHaveAttribute('src', '/rank-badges/squire.png');
    });

    it('maps each rank to its own emblem', () => {
        const { container } = render(
            <>
                <RankBadge rank={rankAt('page', 1)} alt="Page badge" />
                <RankBadge rank={rankAt('knight-errant', 5)} alt="Knight Errant badge" />
                <RankBadge rank={rankAt('emperor', 20)} alt="Emperor badge" />
            </>,
        );
        const srcs = [...container.querySelectorAll<HTMLImageElement>('.rank-badge')].map((img) =>
            img.getAttribute('src'),
        );
        expect(srcs).toEqual(['/rank-badges/page.png', '/rank-badges/knight-errant.png', '/rank-badges/emperor.png']);
    });

    it('renders a large badge for the profile hero', () => {
        render(<RankBadge rank={rankAt('high-king', 19)} size="large" alt="High King badge" />);
        const img = screen.getByRole('img', { name: 'High King badge' });
        expect(img).toHaveAttribute('src', '/rank-badges/high-king.png');
        expect(img).toHaveClass('rank-badge--large');
    });

    it('is decorative by default', () => {
        const { container } = render(<RankBadge rank={rankAt('page', 1)} />);
        const img = container.querySelector('.rank-badge') as HTMLImageElement;
        expect(img).toHaveAttribute('alt', '');
        expect(img).not.toHaveAttribute('aria-label');
    });
});
