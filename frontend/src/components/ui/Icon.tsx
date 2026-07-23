export type IconName = 'arrow-left' | 'chevron-right' | 'close' | 'logout' | 'send' | 'users';

interface IconProps {
    name: IconName;
    className?: string;
}

const paths: Record<IconName, React.ReactNode> = {
    'arrow-left': (
        <>
            <path d="M19 12H5" />
            <path d="m12 19-7-7 7-7" />
        </>
    ),
    'chevron-right': <path d="m9 18 6-6-6-6" />,
    close: (
        <>
            <path d="m18 6-12 12" />
            <path d="m6 6 12 12" />
        </>
    ),
    logout: (
        <>
            <path d="M10 17l5-5-5-5" />
            <path d="M15 12H3" />
            <path d="M15 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4" />
        </>
    ),
    send: (
        <>
            <path d="m22 2-7 20-4-9-9-4Z" />
            <path d="M22 2 11 13" />
        </>
    ),
    users: (
        <>
            <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
            <circle cx="9" cy="7" r="4" />
            <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
            <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </>
    ),
};

export default function Icon({ name, className = '' }: IconProps) {
    return (
        <svg aria-hidden="true" className={`ui-icon ${className}`.trim()} viewBox="0 0 24 24" focusable="false">
            {paths[name]}
        </svg>
    );
}
