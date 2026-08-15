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
    /** Brand icon artwork served from /lenses/icons/, shown in the picker. */
    icon: string;
    accent: string;
    preview?: string;
    kind?: 'accessory' | 'deformation';
}

export const LENS_OPTIONS: LensOption[] = [
    { id: 'none', label: 'Original', icon: '/lenses/icons/original.png', accent: '#777b91' },
    {
        id: 'hr-nightmare',
        label: 'HR nightmare',
        icon: '/lenses/icons/hr-nightmare.png',
        accent: '#ff214d',
        preview: '/lenses/generated/hr-nightmare.thumb.webp',
    },
    {
        id: 'toxic-ex',
        label: 'Toxic ex',
        icon: '/lenses/icons/toxic-ex.png',
        accent: '#a7ff16',
        preview: '/lenses/generated/toxic-ex.thumb.webp',
    },
    {
        id: 'tax-fraud',
        label: 'Tax fraud',
        icon: '/lenses/icons/tax-fraud.png',
        accent: '#f5d76e',
        preview: '/lenses/generated/tax-fraud.thumb.webp',
    },
    {
        id: 'bad-decisions',
        label: 'Bad decisions',
        icon: '/lenses/icons/bad-decisions.png',
        accent: '#ff5a21',
        preview: '/lenses/generated/bad-decisions.thumb.webp',
    },
    {
        id: 'red-flag-royalty',
        label: 'Red flag royalty',
        icon: '/lenses/icons/red-flag-royalty.png',
        accent: '#ff334f',
        preview: '/lenses/generated/red-flag-royalty.thumb.webp',
    },
    {
        id: 'disco-outlaw',
        label: 'Disco outlaw',
        icon: '/lenses/icons/disco-outlaw.png',
        accent: '#ff3eb5',
        preview: '/lenses/generated/disco-outlaw.thumb.webp',
    },
    {
        id: 'big-head',
        label: 'Ego inflation',
        icon: '/lenses/icons/big-head.png',
        accent: '#ffcb55',
        kind: 'deformation',
    },
    {
        id: 'bug-eyes',
        label: 'Doomscroll damage',
        icon: '/lenses/icons/bug-eyes.png',
        accent: '#8dff72',
        kind: 'deformation',
    },
    {
        id: 'tiny-face',
        label: 'Budget facelift',
        icon: '/lenses/icons/tiny-face.png',
        accent: '#74c8ff',
        kind: 'deformation',
    },
    { id: 'cyber', label: 'Cyber visor', icon: '/lenses/icons/cyber.png', accent: '#12e7ff' },
    { id: 'crystal-crown', label: 'Crystal crown', icon: '/lenses/icons/crystal-crown.png', accent: '#b88cff' },
    { id: 'cat', label: 'Neon kitty', icon: '/lenses/icons/cat.png', accent: '#ff72c6' },
    { id: 'puppy', label: 'Jeeliz puppy', icon: '/lenses/icons/puppy.png', accent: '#d9905f' },
    { id: 'devil', label: 'Inferno', icon: '/lenses/icons/devil.png', accent: '#ff493d' },
    { id: 'angel', label: 'Heavenly', icon: '/lenses/icons/angel.png', accent: '#ffe58a' },
    { id: 'space', label: 'Space cadet', icon: '/lenses/icons/space.png', accent: '#6ea8ff' },
    { id: 'party', label: 'Party pop', icon: '/lenses/icons/party.png', accent: '#ffcf45' },
    { id: 'butterfly', label: 'Butterfly', icon: '/lenses/icons/butterfly.png', accent: '#a66cff' },
    { id: 'frog', label: 'Frog prince', icon: '/lenses/icons/frog.png', accent: '#76df65' },
    { id: 'robot', label: 'Mecha', icon: '/lenses/icons/robot.png', accent: '#51d8ed' },
    { id: 'masquerade', label: 'Masquerade', icon: '/lenses/icons/masquerade.png', accent: '#efb34f' },
    { id: 'ice', label: 'Ice queen', icon: '/lenses/icons/ice.png', accent: '#8de7ff' },
    { id: 'arcade', label: 'Pixel hero', icon: '/lenses/icons/arcade.png', accent: '#79ff84' },
    { id: 'glam', label: 'Superstar', icon: '/lenses/icons/glam.png', accent: '#ff8bb5' },
];
