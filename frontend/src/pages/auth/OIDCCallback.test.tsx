import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import OIDCCallback from './OIDCCallback';

const exchange = vi.fn();
vi.mock('../../api', () => ({
    exchangeOIDCSession: () => exchange(),
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

const authValue = {
    user: null,
    loading: false,
    isAuthenticated: false,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
};

describe('OIDCCallback', () => {
    it('exchanges the BFF session and returns to the game', async () => {
        const response = {
            access_token: 'app-token',
            expires_in: 900,
            user: {
                id: 'user-1',
                username: 'alice',
                avatar: 'avatar.png',
                password_login_enabled: false,
                oidc_linked: true,
                migration_required: false,
            },
        };
        exchange.mockResolvedValueOnce(response);
        sessionStorage.setItem('geoguessme_oidc_return_to', '/groups');
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter initialEntries={['/auth/oidc/callback']}>
                    <Routes>
                        <Route path="/auth/oidc/callback" element={<OIDCCallback />} />
                        <Route path="/groups" element={<p>Groups ready</p>} />
                    </Routes>
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        expect(await screen.findByText('Groups ready')).toBeInTheDocument();
        expect(authValue.login).toHaveBeenCalledWith(response);
    });

    it('explains how an existing account can be linked', async () => {
        exchange.mockRejectedValueOnce(new Error('Use the account migration page once'));
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <OIDCCallback />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Use the account migration page once'));
        expect(screen.getByRole('link', { name: 'Migrate existing account' })).toHaveAttribute(
            'href',
            '/migrate-account',
        );
    });
});
