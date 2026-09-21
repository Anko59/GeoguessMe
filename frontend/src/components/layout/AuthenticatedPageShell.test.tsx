import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import AuthenticatedPageShell from './AuthenticatedPageShell';

describe('AuthenticatedPageShell', () => {
    it('keeps the navigation outside page-specific content while preserving main semantics', () => {
        render(
            <MemoryRouter initialEntries={['/profile']}>
                <AuthenticatedPageShell contentAs="main" contentClassName="profile-page" showSettings ariaBusy>
                    <h1>Profile</h1>
                </AuthenticatedPageShell>
            </MemoryRouter>,
        );

        const shell = document.querySelector('.authenticated-page-shell');
        expect(shell).not.toBeNull();
        expect(shell?.querySelectorAll(':scope > .app-topbar')).toHaveLength(1);
        expect(shell?.querySelector(':scope > main.profile-page')).toContainElement(screen.getByRole('heading'));
        expect(shell?.querySelector(':scope > main.profile-page')).toHaveAttribute('aria-busy', 'true');
        expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('aria-current', 'page');
    });

    it('renders children directly when no content wrapper is requested', () => {
        render(
            <MemoryRouter>
                <AuthenticatedPageShell>
                    <main data-testid="page-content">Feed</main>
                </AuthenticatedPageShell>
            </MemoryRouter>,
        );

        const shell = document.querySelector('.authenticated-page-shell');
        expect(shell?.querySelector(':scope > [data-testid="page-content"]')).toBe(screen.getByTestId('page-content'));
        expect(shell?.querySelectorAll('.app-topbar')).toHaveLength(1);
    });
});
