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
        render(<RankBadge rank={rankAt('lost-tourist', 2)} alt="Lost Tourist badge" />);
        expect(screen.getByRole('img', { name: 'Lost Tourist badge' })).toHaveAttribute(
            'src',
            '/rank-badges/lost-tourist.png',
        );
    });

    it('maps each rank to its own emblem', () => {
        const { container } = render(
            <>
                <RankBadge rank={rankAt('completely-lost', 1)} alt="Completely Lost badge" />
                <RankBadge rank={rankAt('explorer', 11)} alt="Explorer badge" />
                <RankBadge rank={rankAt('living-atlas', 30)} alt="Living Atlas badge" />
            </>,
        );
        const srcs = [...container.querySelectorAll<HTMLImageElement>('.rank-badge')].map((img) =>
            img.getAttribute('src'),
        );
        expect(srcs).toEqual([
            '/rank-badges/completely-lost.png',
            '/rank-badges/explorer.png',
            '/rank-badges/living-atlas.png',
        ]);
    });

    it('shows the rank number in roman numerals beneath the badge', () => {
        const { container, rerender } = render(<RankBadge rank={rankAt('lost-tourist', 2)} alt="Lost Tourist badge" />);
        expect(container.querySelector('.rank-badge-numeral')).toHaveTextContent('II');

        rerender(<RankBadge rank={rankAt('map-reader', 7)} alt="Map Reader badge" />);
        expect(container.querySelector('.rank-badge-numeral')).toHaveTextContent('VII');

        rerender(<RankBadge rank={rankAt('explorer', 11)} alt="Explorer badge" />);
        expect(container.querySelector('.rank-badge-numeral')).toHaveTextContent('XI');

        rerender(<RankBadge rank={rankAt('living-atlas', 30)} alt="Living Atlas badge" />);
        expect(container.querySelector('.rank-badge-numeral')).toHaveTextContent('XXX');
    });

    it('renders a large badge for the profile hero', () => {
        render(<RankBadge rank={rankAt('world-sage', 29)} size="large" alt="World Sage badge" />);
        const img = screen.getByRole('img', { name: 'World Sage badge' });
        expect(img).toHaveAttribute('src', '/rank-badges/world-sage.png');
        expect(img).toHaveClass('rank-badge--large');
    });

    it('is decorative by default', () => {
        const { container } = render(<RankBadge rank={rankAt('completely-lost', 1)} />);
        const img = container.querySelector('.rank-badge') as HTMLImageElement;
        expect(img).toHaveAttribute('alt', '');
        expect(img).not.toHaveAttribute('aria-label');
    });
});
