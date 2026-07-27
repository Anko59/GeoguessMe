import { useAvatarUrl } from './avatarCache';

interface AvatarProps {
    userID: string;
    avatar?: string;
    username?: string;
    className?: string;
}

/** Renders a user avatar, fetching custom photos once per session. */
export default function Avatar({ userID, avatar, username, className }: AvatarProps) {
    const url = useAvatarUrl(userID, avatar);

    if (url === undefined) {
        return <span className={`avatar avatar--placeholder ${className ?? ''}`} aria-hidden="true" />;
    }

    return <img className={className} src={url} alt={username || ''} />;
}
