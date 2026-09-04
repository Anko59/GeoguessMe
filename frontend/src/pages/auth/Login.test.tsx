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
        mockGet.mockResolvedValue({ data: { enabled: false, login_path: '/oauth2/start', social_providers: [] } });
    });

    it('offers distinct Keycloak social and native email login when enabled', async () => {
        mockGet.mockResolvedValueOnce({
            data: { enabled: true, login_path: '/oauth2/start', social_providers: ['google'] },
        });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const google = await screen.findByRole('link', { name: 'Continue with Google' });
        expect(google).toHaveAttribute('href', '/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback&kc_idp_hint=google');
        expect(google.querySelector('.auth-provider-logo-google')).toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Continue with Apple' })).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Continue with GitHub' })).not.toBeInTheDocument();
        expect(screen.getByPlaceholderText('you@example.com')).toHaveAttribute('name', 'login_hint');
        expect(screen.getByRole('button', { name: 'Continue to password' })).toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Username')).not.toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Password')).not.toBeInTheDocument();
        fireEvent.click(google);
        expect(sessionStorage.getItem('geoguessme_oidc_return_to')).toBe('/groups');
    });

    it('hides unconfigured social providers while keeping native email available', async () => {
        mockGet.mockResolvedValueOnce({ data: { enabled: true, login_path: '/oauth2/start', social_providers: [] } });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        expect(await screen.findByRole('button', { name: 'Continue to password' })).toBeInTheDocument();
        expect(screen.queryByText('Continue with Google')).not.toBeInTheDocument();
        expect(screen.queryByText('Continue with Apple')).not.toBeInTheDocument();
        expect(screen.queryByText('Continue with GitHub')).not.toBeInTheDocument();
        expect(screen.getByText(/Social sign-in is not configured in this environment/)).toBeInTheDocument();
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
        expect(screen.getByPlaceholderText('Username or email')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'Forgot your username or password?' })).toBeInTheDocument();
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

        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
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

        const usernameInput = (await screen.findByPlaceholderText('Username or email')) as HTMLInputElement;
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

        await screen.findByPlaceholderText('Username or email');
        fireEvent.change(screen.getByPlaceholderText('Username or email'), { target: { value: 'testuser' } });
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

        await screen.findByPlaceholderText('Username or email');
        fireEvent.change(screen.getByPlaceholderText('Username or email'), { target: { value: 'wrong' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'wrong' } });
        fireEvent.click(screen.getByRole('button', { name: /login/i }));

        await waitFor(() => {
            expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
        });
    });
});
