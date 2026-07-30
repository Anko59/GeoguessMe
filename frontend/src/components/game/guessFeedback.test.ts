import { describe, expect, it } from 'vitest';
import { feedbackForScore } from './guessFeedback';

describe('feedbackForScore', () => {
    it.each([
        [0, 'Cartographic Catastrophe'],
        [499, 'Cartographic Catastrophe'],
        [500, 'Wayward Squire'],
        [1499, 'Wayward Squire'],
        [1500, 'Roaming Herald'],
        [2499, 'Roaming Herald'],
        [2500, 'Roadside Scout'],
        [3499, 'Roadside Scout'],
        [3500, 'Keen Pathfinder'],
        [4199, 'Keen Pathfinder'],
        [4200, 'Royal Cartographer'],
        [4799, 'Royal Cartographer'],
        [4800, 'Masterstroke'],
        [5000, 'Masterstroke'],
    ])('labels score %s as %s', (score, label) => {
        expect(feedbackForScore(score).label).toBe(label);
    });

    it('clamps invalid scores to the supported range', () => {
        expect(feedbackForScore(-10).label).toBe('Cartographic Catastrophe');
        expect(feedbackForScore(9999).label).toBe('Masterstroke');
        expect(feedbackForScore(Number.NaN).label).toBe('Cartographic Catastrophe');
    });
});
