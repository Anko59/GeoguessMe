import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import OIDCCallback from './OIDCCallback';

const exchange = vi.fn();
vi.mock('../../api', () => ({
    exchangeOIDCSession: (username?: string) => exchange(username),
    getAPIErrorCode: (error: unknown) =>
        (error as { response?: { data?: { error?: { code?: string } } } }).response?.data?.error?.code,
    getAPIErrorMessage: (error: unknown, fallback: string) =>
        (error as { response?: { data?: { error?: { message?: string } } } }).response?.data?.error?.message ??
        (error instanceof Error ? error.message : fallback),
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
    beforeEach(() => {
        exchange.mockReset();
        authValue.login.mockClear();
        sessionStorage.clear();
    });

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

    it('lets a new verified identity choose an empty-by-default username', async () => {
        const usernameRequired = {
            response: { data: { error: { code: 'username_required', message: 'Choose your username' } } },
        };
        const response = {
            access_token: 'app-token',
            expires_in: 900,
            user: {
                id: 'user-2',
                username: 'map-master',
                avatar: 'avatar.png',
                password_login_enabled: false,
                oidc_linked: true,
                migration_required: false,
            },
        };
        exchange.mockRejectedValueOnce(usernameRequired).mockResolvedValueOnce(response);
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
        const input = await screen.findByLabelText('Username');
        expect(input).toHaveValue('');
        fireEvent.change(input, { target: { value: 'map-master' } });
        fireEvent.click(screen.getByRole('button', { name: 'Start playing' }));
        expect(await screen.findByText('Groups ready')).toBeInTheDocument();
        expect(exchange).toHaveBeenNthCalledWith(1, undefined);
        expect(exchange).toHaveBeenNthCalledWith(2, 'map-master');
    });

    it('keeps the username form open when the chosen name is taken', async () => {
        exchange
            .mockRejectedValueOnce({ response: { data: { error: { code: 'username_required' } } } })
            .mockRejectedValueOnce({
                response: { data: { error: { code: 'username_taken', message: 'Username is already in use' } } },
            });
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <OIDCCallback />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        const input = await screen.findByLabelText('Username');
        fireEvent.change(input, { target: { value: 'alice' } });
        fireEvent.click(screen.getByRole('button', { name: 'Start playing' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Username is already in use');
        expect(input).toHaveValue('alice');
    });
});
