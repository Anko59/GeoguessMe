import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import TopNavigation from './TopNavigation';

function renderNavigation(path: string, showSettings = true) {
    return render(
        <MemoryRouter initialEntries={[path]}>
            <Routes>
                <Route path="*" element={<TopNavigation showSettings={showSettings} />} />
            </Routes>
        </MemoryRouter>,
    );
}

describe('TopNavigation', () => {
    it.each([
        ['/feed', 'Explore feed'],
        ['/groups', 'My groups'],
        ['/profile', 'Profile'],
        ['/settings', 'Settings'],
    ])('marks the %s destination as current', (path, label) => {
        renderNavigation(path);
        expect(screen.getByRole('link', { name: label })).toHaveAttribute('aria-current', 'page');
    });

    it('keeps icon-only account links accessible when their labels are hidden on mobile', () => {
        renderNavigation('/groups');
        expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('aria-label', 'Profile');
        expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('aria-label', 'Settings');
    });

    it('keeps settings out of public profile navigation', () => {
        renderNavigation('/profile/user-2', false);
        expect(screen.queryByRole('link', { name: 'Settings' })).not.toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'Profile' })).toHaveAttribute('aria-current', 'page');
    });
});
