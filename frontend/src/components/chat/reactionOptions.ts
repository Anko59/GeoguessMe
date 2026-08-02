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
];

export const reactionByKey = new Map(reactionOptions.map((option) => [option.reaction, option]));
