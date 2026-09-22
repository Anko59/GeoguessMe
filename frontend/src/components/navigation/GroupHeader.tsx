import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import Avatar from '../common/Avatar';
import FullScreenImage from '../ui/FullScreenImage';
import Icon from '../ui/Icon';

interface GroupHeaderProps {
    groupName: string;
    photoURL: string;
    actions?: ReactNode;
    eyebrow?: string;
    heading?: string;
    headingID?: string;
    headingLevel?: 1 | 2;
    backHref?: string;
    onClose?: () => void;
    photoAlt?: string;
    previewPhoto?: boolean;
}

/** The group page and its globe dialog share this navigation header. */
export default function GroupHeader({
    groupName,
    photoURL,
    actions,
    eyebrow = 'Group',
    heading,
    headingID,
    headingLevel = 1,
    backHref,
    onClose,
    photoAlt,
    previewPhoto = false,
}: GroupHeaderProps) {
    const Heading = headingLevel === 1 ? 'h1' : 'h2';
    const title = heading ?? groupName;
    const logo = <img src={photoURL} alt="" className="header-logo" />;

    return (
        <header className="group-header">
            <div className="header-content">
                {backHref ? (
                    <Link to={backHref} className="back-btn">
                        <Icon name="arrow-left" className="back-arrow-icon" />
                        <span className="visually-hidden">Back to groups</span>
                    </Link>
                ) : (
                    <button type="button" className="back-btn" onClick={onClose} aria-label="Close group globe">
                        <Icon name="close" className="back-arrow-icon" />
                    </button>
                )}
                {previewPhoto ? (
                    <FullScreenImage
                        src={photoURL}
                        alt={photoAlt ?? `${groupName} group photo`}
                        className="header-logo-toggle"
                    >
                        {logo}
                    </FullScreenImage>
                ) : (
                    logo
                )}
                <div className="group-title-block">
                    <span>{eyebrow}</span>
                    <Heading id={headingID} className="group-name">
                        {title}
                    </Heading>
                </div>
                {actions && <div className="group-header-actions">{actions}</div>}
            </div>
        </header>
    );
}

export function GroupHeaderProfile({ userID, avatar, username }: { userID: string; avatar: string; username: string }) {
    return (
        <Link to="/profile" className="header-profile-link" aria-label="Open your profile">
            <Avatar userID={userID} avatar={avatar} username={username} />
        </Link>
    );
}
