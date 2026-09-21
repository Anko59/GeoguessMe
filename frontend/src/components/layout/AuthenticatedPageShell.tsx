import { type ElementType, type ReactNode } from 'react';
import TopNavigation from '../navigation/TopNavigation';
import './AuthenticatedPageShell.css';

interface AuthenticatedPageShellProps {
    children: ReactNode;
    className?: string;
    contentClassName?: string;
    contentAs?: 'div' | 'main';
    showSettings?: boolean;
    ariaBusy?: boolean;
}

/**
 * Shared geometry contract for pages available after authentication.
 *
 * The shell owns the viewport width, page padding, and top-navigation slot.
 * Page-specific content remains in its own element so each page can keep its
 * established content width and semantics without moving the navigation.
 */
export default function AuthenticatedPageShell({
    children,
    className = '',
    contentClassName,
    contentAs = 'div',
    showSettings = true,
    ariaBusy,
}: AuthenticatedPageShellProps) {
    const Content = contentAs as ElementType;
    const shellClassName = ['authenticated-page-shell', className].filter(Boolean).join(' ');

    return (
        <div className={shellClassName} aria-busy={!contentClassName ? ariaBusy : undefined}>
            <TopNavigation showSettings={showSettings} />
            {contentClassName ? (
                <Content className={contentClassName} aria-busy={ariaBusy}>
                    {children}
                </Content>
            ) : (
                children
            )}
        </div>
    );
}
