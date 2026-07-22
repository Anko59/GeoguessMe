import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import LogoutButton from './LogoutButton';
import { AuthContext, type AuthContextValue } from '../../context/AuthContext';

function authValue(logout: AuthContextValue['logout']): AuthContextValue {
    return {
        user: null,
        loading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout,
        refresh: vi.fn(async () => true),
    };
}

describe('LogoutButton', () => {
    it('navigates home even when server-side revocation fails', async () => {
        const logout = vi.fn(async () => {
            throw new Error('network unavailable');
        });

        render(
            <AuthContext.Provider value={authValue(logout)}>
                <MemoryRouter initialEntries={['/settings']}>
                    <Routes>
                        <Route path="/settings" element={<LogoutButton />} />
                        <Route path="/" element={<div data-testid="home">Home</div>} />
                    </Routes>
                </MemoryRouter>
            </AuthContext.Provider>,
        );

        fireEvent.click(screen.getByRole('button', { name: 'Logout' }));

        await waitFor(() => expect(screen.getByTestId('home')).toBeInTheDocument());
        expect(logout).toHaveBeenCalledOnce();
    });
});
