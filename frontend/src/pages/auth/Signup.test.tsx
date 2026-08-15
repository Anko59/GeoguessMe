import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { vi } from 'vitest';
import Signup from './Signup';
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

describe('Signup Page', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        mockGet.mockResolvedValue({ data: { enabled: false, login_path: '/oauth2/start', social_providers: [] } });
    });

    it('offers distinct social and native email signup when Keycloak is enabled', async () => {
        mockGet.mockResolvedValueOnce({
            data: { enabled: true, login_path: '/oauth2/start', social_providers: ['google', 'apple', 'github'] },
        });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const google = await screen.findByRole('link', { name: 'Sign up with Google' });
        const apple = screen.getByRole('link', { name: 'Sign up with Apple' });
        const github = screen.getByRole('link', { name: 'Sign up with GitHub' });
        expect(google).toHaveAttribute('href', '/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback&kc_idp_hint=google');
        expect(apple).toHaveAttribute('href', '/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback&kc_idp_hint=apple');
        expect(github).toHaveAttribute('href', '/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback&kc_idp_hint=github');
        expect(google.querySelector('.auth-provider-logo-google')).toBeInTheDocument();
        expect(apple.querySelector('.auth-provider-logo-apple')).toBeInTheDocument();
        expect(github.querySelector('.auth-provider-logo-github')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('you@example.com')).toHaveAttribute('name', 'login_hint');
        expect(screen.getByDisplayValue('create')).toHaveAttribute('name', 'prompt');
        expect(screen.getByRole('button', { name: 'Continue to create account' })).toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Username')).not.toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Password')).not.toBeInTheDocument();
        fireEvent.click(google);
        expect(sessionStorage.getItem('geoguessme_oidc_return_to')).toBe('/groups');
    });

    it('renders signup form when OIDC is explicitly disabled', async () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /sign up/i })).toBeInTheDocument();
    });

    it('submits form with valid data', async () => {
        mockPost.mockResolvedValue({
            data: {
                token: 'fake-token',
                user: { id: '1', username: 'newuser' },
            },
        });

        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        await screen.findByPlaceholderText('Username');
        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'newuser' } });
        fireEvent.change(screen.getByPlaceholderText('Email — verify to enable account recovery'), {
            target: { value: 'new@example.com' },
        });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(mockPost).toHaveBeenCalledWith('/auth/signup', {
                username: 'newuser',
                email: 'new@example.com',
                password: 'StrongPass123',
            });
        });
    });

    it('creates an account without a recovery email', async () => {
        mockPost.mockResolvedValue({
            data: { token: 'fake-token', user: { id: '1', username: 'emailfree' } },
        });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );
        await screen.findByPlaceholderText('Username');
        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'emailfree' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));
        await waitFor(() =>
            expect(mockPost).toHaveBeenCalledWith('/auth/signup', {
                username: 'emailfree',
                password: 'StrongPass123',
            }),
        );
    });

    it('displays error on failed signup', async () => {
        mockPost.mockRejectedValue(new Error('Username taken'));

        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        await screen.findByPlaceholderText('Username');
        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'taken' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(screen.getByText('Username taken')).toBeInTheDocument();
        });
    });
});
