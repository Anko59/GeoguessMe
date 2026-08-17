export interface ReactionOption {
    reaction: string;
    label: string;
    image: string;
}

export const reactionOptions: ReactionOption[] = [
    { reaction: 'like', label: 'thumbs up', image: '/reactions/like.png' },
    { reaction: 'love', label: 'love', image: '/reactions/love.png' },
    { reaction: 'laugh', label: 'laughing', image: '/reactions/laugh.png' },
    { reaction: 'wow', label: 'surprised', image: '/reactions/wow.png' },
    { reaction: 'sad', label: 'sad', image: '/reactions/sad.png' },
    { reaction: 'spot-on', label: 'spot on', image: '/reactions/spot-on.png' },
    { reaction: 'lost', label: 'lost', image: '/reactions/lost.png' },
    { reaction: 'mind-blown', label: 'mind blown', image: '/reactions/mind-blown.png' },
    { reaction: 'wrong-way', label: 'wrong way', image: '/reactions/wrong-way.png' },
    { reaction: 'vacation', label: 'vacation', image: '/reactions/vacation.png' },
    { reaction: 'dislike', label: 'thumbs down', image: '/reactions/dislike.png' },
    { reaction: 'cry', label: 'crying', image: '/reactions/cry.png' },
    { reaction: 'kiss', label: 'kiss', image: '/reactions/kiss.png' },
    { reaction: 'wink', label: 'winking', image: '/reactions/wink.png' },
    { reaction: 'grin', label: 'big grin', image: '/reactions/grin.png' },
    { reaction: 'heart-eyes', label: 'heart eyes', image: '/reactions/heart-eyes.png' },
    { reaction: 'sunglasses', label: 'cool', image: '/reactions/sunglasses.png' },
    { reaction: 'angry', label: 'angry', image: '/reactions/angry.png' },
    { reaction: 'confused', label: 'confused', image: '/reactions/confused.png' },
    { reaction: 'sleepy', label: 'sleepy', image: '/reactions/sleepy.png' },
    { reaction: 'clap', label: 'clapping', image: '/reactions/clap.png' },
    { reaction: 'pray', label: 'grateful', image: '/reactions/pray.png' },
    { reaction: 'fire', label: 'fire', image: '/reactions/fire.png' },
    { reaction: 'party', label: 'party', image: '/reactions/party.png' },
];

export interface ReactionUsage {
    reaction: string;
    count: number;
}

/** Sorts the complete custom set by group usage while preserving the curated
 *  option order for ties and reactions with no history. */
export function sortReactionOptions(usage: ReactionUsage[]): ReactionOption[] {
    const counts = new Map(usage.map((item) => [item.reaction, item.count]));
    return reactionOptions
        .map((option, index) => ({ option, index, count: counts.get(option.reaction) ?? 0 }))
        .sort((a, b) => b.count - a.count || a.index - b.index)
        .map(({ option }) => option);
}

export const reactionByKey = new Map(reactionOptions.map((option) => [option.reaction, option]));
