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
    | 'glam';

export interface LensOption {
    id: LensId;
    label: string;
    icon: string;
    accent: string;
}

export const LENS_OPTIONS: LensOption[] = [
    { id: 'none', label: 'Original', icon: '✦', accent: '#777b91' },
    { id: 'cyber', label: 'Cyber visor', icon: '🥽', accent: '#12e7ff' },
    { id: 'crystal-crown', label: 'Crystal crown', icon: '👑', accent: '#b88cff' },
    { id: 'cat', label: 'Neon kitty', icon: '🐱', accent: '#ff72c6' },
    { id: 'puppy', label: '3D puppy', icon: '🐶', accent: '#d9905f' },
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
