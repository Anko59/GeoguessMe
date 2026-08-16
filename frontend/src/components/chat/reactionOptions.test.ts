import { describe, expect, it } from 'vitest';
import { reactionOptions, sortReactionOptions } from './reactionOptions';

describe('reaction options', () => {
    it('keeps every custom reaction available and sorts by usage', () => {
        const ordered = sortReactionOptions([
            { reaction: 'party', count: 1 },
            { reaction: 'dislike', count: 8 },
            { reaction: 'like', count: 12 },
        ]);
        expect(ordered.map((option) => option.reaction).slice(0, 3)).toEqual(['like', 'dislike', 'party']);
        expect(ordered).toHaveLength(reactionOptions.length);
    });

    it('uses the curated order for ties and unused reactions', () => {
        const ordered = sortReactionOptions([]);
        expect(ordered.map((option) => option.reaction)).toEqual(reactionOptions.map((option) => option.reaction));
    });
});
