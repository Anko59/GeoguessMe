import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { vi } from 'vitest';
import Login from './Login';
import { AuthContext } from '../../context/AuthContext';

// Mock the API module
const mockPost = vi.fn();
const mockGet = vi.fn();
vi.mock('../../api', () => ({
    default: {
        get: (...args: unknown[]) => mockGet(...args),
        post: (...args: unknown[]) => mockPost(...args),
    },
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

describe('Login Page', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        mockGet.mockResolvedValue({ data: { enabled: false, login_path: '/oauth2/start' } });
    });

    it('offers Keycloak social login when enabled', async () => {
        mockGet.mockResolvedValueOnce({ data: { enabled: true, login_path: '/oauth2/start' } });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const link = await screen.findByRole('link', { name: 'Continue with Google, Apple, or GitHub' });
        expect(link).toHaveAttribute('href', '/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback');
        expect(screen.queryByPlaceholderText('Username')).not.toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Password')).not.toBeInTheDocument();
        fireEvent.click(link);
        expect(sessionStorage.getItem('geoguessme_oidc_return_to')).toBe('/groups');
    });

    it('only exposes legacy credentials on the dedicated migration page', async () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login migrationMode />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        expect(await screen.findByRole('heading', { name: 'Migrate your account' })).toBeInTheDocument();
        expect(screen.getByRole('note')).toHaveTextContent('legacy session is read-only');
        expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(mockGet).not.toHaveBeenCalled();
    });

    it('renders login form when OIDC is explicitly disabled', async () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument();
    });

    it('handles input changes', async () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const usernameInput = (await screen.findByPlaceholderText('Username')) as HTMLInputElement;
        const passwordInput = screen.getByPlaceholderText('Password') as HTMLInputElement;

        fireEvent.change(usernameInput, { target: { value: 'testuser' } });
        fireEvent.change(passwordInput, { target: { value: 'password123' } });

        expect(usernameInput.value).toBe('testuser');
        expect(passwordInput.value).toBe('password123');
    });

    it('submits form with valid data', async () => {
        mockPost.mockResolvedValue({
            data: {
                token: 'fake-token',
                user: { id: '1', username: 'testuser' },
            },
        });

        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        await screen.findByPlaceholderText('Username');
        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'testuser' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'password123' } });
        fireEvent.click(screen.getByRole('button', { name: /login/i }));

        await waitFor(() => {
            expect(mockPost).toHaveBeenCalledWith('/auth/login', { username: 'testuser', password: 'password123' });
        });
    });

    it('displays error on failed login', async () => {
        mockPost.mockRejectedValue(new Error('Invalid credentials'));

        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        await screen.findByPlaceholderText('Username');
        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'wrong' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'wrong' } });
        fireEvent.click(screen.getByRole('button', { name: /login/i }));

        await waitFor(() => {
            expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
        });
    });
});
