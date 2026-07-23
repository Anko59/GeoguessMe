export type LensId =
    | 'none'
    | 'cyber'
    | 'crystal-crown'
    | 'cat'
    | 'puppy'
    | 'devil'
    | 'angel'
    | 'space'
    | 'party'
    | 'butterfly'
    | 'frog'
    | 'robot'
    | 'masquerade'
    | 'ice'
    | 'arcade'
    | 'glam'
    | 'disco-outlaw'
    | 'red-flag-royalty'
    | 'bad-decisions'
    | 'hr-nightmare'
    | 'toxic-ex'
    | 'tax-fraud'
    | 'big-head'
    | 'bug-eyes'
    | 'tiny-face';

export interface LensOption {
    id: LensId;
    label: string;
    icon: string;
    accent: string;
    preview?: string;
    kind?: 'accessory' | 'deformation';
}

export const LENS_OPTIONS: LensOption[] = [
    { id: 'none', label: 'Original', icon: '✦', accent: '#777b91' },
    {
        id: 'hr-nightmare',
        label: 'HR nightmare',
        icon: '😈',
        accent: '#ff214d',
        preview: '/lenses/generated/hr-nightmare.webp',
    },
    {
        id: 'toxic-ex',
        label: 'Toxic ex',
        icon: '☣️',
        accent: '#a7ff16',
        preview: '/lenses/generated/toxic-ex.webp',
    },
    {
        id: 'tax-fraud',
        label: 'Tax fraud',
        icon: '🛥️',
        accent: '#f5d76e',
        preview: '/lenses/generated/tax-fraud.webp',
    },
    {
        id: 'bad-decisions',
        label: 'Bad decisions',
        icon: '🎲',
        accent: '#ff5a21',
        preview: '/lenses/generated/bad-decisions.webp',
    },
    {
        id: 'red-flag-royalty',
        label: 'Red flag royalty',
        icon: '🚩',
        accent: '#ff334f',
        preview: '/lenses/generated/red-flag-royalty.webp',
    },
    {
        id: 'disco-outlaw',
        label: 'Disco outlaw',
        icon: '🤠',
        accent: '#ff3eb5',
        preview: '/lenses/generated/disco-outlaw.webp',
    },
    { id: 'big-head', label: 'Ego inflation', icon: '🗿', accent: '#ffcb55', kind: 'deformation' },
    { id: 'bug-eyes', label: 'Doomscroll damage', icon: '👀', accent: '#8dff72', kind: 'deformation' },
    { id: 'tiny-face', label: 'Budget facelift', icon: '🤏', accent: '#74c8ff', kind: 'deformation' },
    { id: 'cyber', label: 'Cyber visor', icon: '🥽', accent: '#12e7ff' },
    { id: 'crystal-crown', label: 'Crystal crown', icon: '👑', accent: '#b88cff' },
    { id: 'cat', label: 'Neon kitty', icon: '🐱', accent: '#ff72c6' },
    { id: 'puppy', label: 'Jeeliz puppy', icon: '🐶', accent: '#d9905f' },
    { id: 'devil', label: 'Inferno', icon: '😈', accent: '#ff493d' },
    { id: 'angel', label: 'Heavenly', icon: '😇', accent: '#ffe58a' },
    { id: 'space', label: 'Space cadet', icon: '🧑‍🚀', accent: '#6ea8ff' },
    { id: 'party', label: 'Party pop', icon: '🥳', accent: '#ffcf45' },
    { id: 'butterfly', label: 'Butterfly', icon: '🦋', accent: '#a66cff' },
    { id: 'frog', label: 'Frog prince', icon: '🐸', accent: '#76df65' },
    { id: 'robot', label: 'Mecha', icon: '🤖', accent: '#51d8ed' },
    { id: 'masquerade', label: 'Masquerade', icon: '🎭', accent: '#efb34f' },
    { id: 'ice', label: 'Ice queen', icon: '❄️', accent: '#8de7ff' },
    { id: 'arcade', label: 'Pixel hero', icon: '👾', accent: '#79ff84' },
    { id: 'glam', label: 'Superstar', icon: '✨', accent: '#ff8bb5' },
];

const DEFORMATION_LENSES = new Set<LensId>(
    LENS_OPTIONS.filter((option) => option.kind === 'deformation').map((option) => option.id),
);

export function isDeformationLens(id: LensId): boolean {
    return DEFORMATION_LENSES.has(id);
}
