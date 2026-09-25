import { Link, useLocation } from 'react-router-dom';
import Icon from '../ui/Icon';
import './TopNavigation.css';

type NavigationKey = 'feed' | 'groups' | 'profile' | 'settings';

interface TopNavigationProps {
    /** Hide account settings when this is a public player's profile. */
    showSettings?: boolean;
}

function navigationKey(pathname: string): NavigationKey {
    if (pathname.startsWith('/feed')) return 'feed';
    if (pathname.startsWith('/profile')) return 'profile';
    if (pathname.startsWith('/settings')) return 'settings';
    return 'groups';
}

/** The shared authenticated navigation used by the groups, feed, profile, and
 * settings pages. Keeping active state here makes the page context visible to
 * screen readers through one consistent aria-current contract. */
export default function TopNavigation({ showSettings = true }: TopNavigationProps) {
    const active = navigationKey(useLocation().pathname);
    const links: Array<{ key: NavigationKey; to: string; label: string; icon?: 'user' | 'gear' }> = [
        { key: 'feed', to: '/feed', label: 'Explore feed' },
        { key: 'groups', to: '/groups', label: 'My groups' },
        { key: 'profile', to: '/profile', label: 'Profile', icon: 'user' },
        ...(showSettings
            ? [{ key: 'settings' as const, to: '/settings', label: 'Settings', icon: 'gear' as const }]
            : []),
    ];

    return (
        <header className="app-topbar">
            <Link to="/groups" className="app-brand" aria-label="GeoGuessMe groups">
                <img src="/logo.png" alt="" />
                <span>GeoGuessMe</span>
            </Link>
            <nav className="app-nav-links" aria-label="Main navigation">
                {links.map((link) => (
                    <Link
                        key={link.key}
                        to={link.to}
                        className={`app-nav-link${link.icon ? ' app-nav-icon-link' : ''}`}
                        aria-label={link.icon ? link.label : undefined}
                        aria-current={active === link.key ? 'page' : undefined}
                    >
                        {link.icon && <Icon name={link.icon} className="app-nav-icon" />}
                        <span className="app-nav-label">{link.label}</span>
                    </Link>
                ))}
            </nav>
        </header>
    );
}
