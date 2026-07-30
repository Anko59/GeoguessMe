import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import RankTrophy from './RankTrophy';
import type { ProgressionRank } from '../../types';

const rank: ProgressionRank = {
    level: 20,
    name: 'Emperor',
    min_points: 750000,
    points_in_rank: 10,
    points_to_next: 0,
    progress_percent: 100,
    trophy_key: 'emperor',
};

describe('RankTrophy', () => {
    it('renders an accessible trophy image for the current rank', () => {
        render(<RankTrophy rank={rank} />);
        expect(screen.getByRole('img', { name: 'Emperor trophy' })).toHaveAttribute('data-rank-level', '20');
    });
});
