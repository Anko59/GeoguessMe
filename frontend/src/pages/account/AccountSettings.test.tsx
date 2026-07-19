import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import AccountSettings from './AccountSettings';
import type { User } from '../../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    patch: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get, patch: mocks.patch, post: mocks.post, delete: mocks.delete },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
    email_verified_at: null,
};

const refresh = vi.fn(async () => true);

const authValue = {
    user,
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh,
};

beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    mocks.get.mockReset();
    mocks.patch.mockReset();
    mocks.post.mockReset();
    mocks.delete.mockReset();
});

describe('AccountSettings', () => {
    it('updates the profile and selected avatar', async () => {
        mocks.patch.mockResolvedValueOnce({
            data: { username: 'alice', email: 'alice@example.test', avatar: 'avatar2.png' },
        });
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <AccountSettings />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        fireEvent.click(screen.getByRole('button', { name: 'Choose avatar2.png' }));
        fireEvent.change(screen.getByLabelText('Current password to save profile changes'), {
            target: { value: 'Password123' },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Save profile' }));
        await waitFor(() =>
            expect(mocks.patch).toHaveBeenCalledWith('/auth/profile', {
                username: 'alice',
                email: 'alice@example.test',
                avatar: 'avatar2.png',
                current_password: 'Password123',
            }),
        );
        expect(await screen.findByRole('status')).toHaveTextContent('Profile updated');
    });

    it('shows verification and deletion flows', async () => {
        mocks.post.mockResolvedValueOnce({ data: { message: 'Verification sent' } });
        mocks.delete.mockResolvedValueOnce({ data: {} });
        vi.stubGlobal('confirm', vi.fn().mockReturnValue(true));

        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <AccountSettings />
                </MemoryRouter>
            </AuthContext.Provider>,
        );

        fireEvent.click(screen.getByRole('button', { name: 'Resend verification email' }));
        expect(await screen.findByRole('status')).toHaveTextContent('Verification sent');

        fireEvent.change(screen.getByLabelText('Confirm password to delete account'), {
            target: { value: 'password' },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Delete account' }));
        await waitFor(() => expect(refresh).toHaveBeenCalled());

        vi.restoreAllMocks();
    });
});
