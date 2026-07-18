import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import ProtectedRoute from './ProtectedRoute';
import { AuthContext, type AuthContextValue } from '../../context/AuthContext';
import type { User } from '../../types';

function authValue(overrides: Partial<AuthContextValue> = {}): AuthContextValue {
    return {
        user: null,
        loading: false,
        isAuthenticated: false,
        login: vi.fn(),
        logout: vi.fn(async () => undefined),
        refresh: vi.fn(async () => false),
        ...overrides,
    };
}

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
};

describe('ProtectedRoute', () => {
    it('shows a loading indicator while auth is still resolving', () => {
        render(
            <AuthContext.Provider value={authValue({ loading: true })}>
                <MemoryRouter>
                    <ProtectedRoute>
                        <div>Secret content</div>
                    </ProtectedRoute>
                </MemoryRouter>
            </AuthContext.Provider>,
        );

        expect(screen.getByText('Restoring session…')).toBeInTheDocument();
        expect(screen.queryByText('Secret content')).not.toBeInTheDocument();
    });

    it('redirects to /login when not authenticated, preserving the attempted path in state', () => {
        render(
            <AuthContext.Provider value={authValue({ loading: false, isAuthenticated: false })}>
                <MemoryRouter initialEntries={['/settings']}>
                    <Routes>
                        <Route
                            path="/settings"
                            element={
                                <ProtectedRoute>
                                    <div>Secret content</div>
                                </ProtectedRoute>
                            }
                        />
                        <Route path="/login" element={<div data-testid="login-page">Login page</div>} />
                    </Routes>
                </MemoryRouter>
            </AuthContext.Provider>,
        );

        expect(screen.getByTestId('login-page')).toBeInTheDocument();
        expect(screen.queryByText('Secret content')).not.toBeInTheDocument();
    });

    it('renders children when the user is authenticated', () => {
        render(
            <AuthContext.Provider value={authValue({ loading: false, isAuthenticated: true, user })}>
                <MemoryRouter>
                    <ProtectedRoute>
                        <div>Secret content</div>
                    </ProtectedRoute>
                </MemoryRouter>
            </AuthContext.Provider>,
        );

        expect(screen.getByText('Secret content')).toBeInTheDocument();
        expect(screen.queryByText('Restoring session…')).not.toBeInTheDocument();
    });
});
