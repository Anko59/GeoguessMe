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
            data: { enabled: true, login_path: '/oauth2/start', social_providers: ['google'] },
        });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const google = await screen.findByRole('link', { name: 'Sign up with Google' });
        expect(google).toHaveAttribute('href', '/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback&kc_idp_hint=google');
        expect(google.querySelector('.auth-provider-logo-google')).toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Sign up with Apple' })).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Sign up with GitHub' })).not.toBeInTheDocument();
        expect(screen.getByPlaceholderText('you@example.com')).toHaveAttribute('name', 'login_hint');
        expect(screen.getByDisplayValue('create')).toHaveAttribute('name', 'prompt');
        expect(screen.getByLabelText(/I confirm I am at least 15 years old/i)).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Continue to create account' })).toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Username')).not.toBeInTheDocument();
        expect(screen.queryByPlaceholderText('Password')).not.toBeInTheDocument();
    });

    it('blocks the provider signup redirect until the age attestation is confirmed', async () => {
        mockGet.mockResolvedValueOnce({
            data: { enabled: true, login_path: '/oauth2/start', social_providers: ['google'] },
        });
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const google = await screen.findByRole('link', { name: 'Sign up with Google' });
        const attestation = screen.getByLabelText(/I confirm I am at least 15 years old/i);

        fireEvent.click(google);
        expect(sessionStorage.getItem('geoguessme_oidc_return_to')).toBeNull();

        fireEvent.click(attestation);
        fireEvent.click(google);
        expect(sessionStorage.getItem('geoguessme_oidc_return_to')).toBe('/groups');
    });

    it('blocks native signup until the age attestation is confirmed', async () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Signup />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        await screen.findByPlaceholderText('Username');
        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'newuser' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        const attestation = screen.getByLabelText(/I confirm I am at least 15 years old/i);
        expect(attestation).toBeRequired();
        fireEvent.submit(screen.getByRole('button', { name: /sign up/i }).closest('form')!);
        expect(mockPost).not.toHaveBeenCalled();
        expect(screen.getByText('Please confirm the minimum age to create an account.')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));
    });

    it('sends the age attestation with native signup', async () => {
        mockPost.mockResolvedValue({
            data: { token: 'fake-token', user: { id: '1', username: 'newuser' } },
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
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByLabelText(/I confirm I am at least 15 years old/i));
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(mockPost).toHaveBeenCalledWith('/auth/signup', {
                username: 'newuser',
                password: 'StrongPass123',
                age_attested: true,
            });
        });
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
        fireEvent.click(screen.getByLabelText(/I confirm I am at least 15 years old/i));
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(mockPost).toHaveBeenCalledWith('/auth/signup', {
                username: 'newuser',
                email: 'new@example.com',
                password: 'StrongPass123',
                age_attested: true,
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
        fireEvent.click(screen.getByLabelText(/I confirm I am at least 15 years old/i));
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));
        await waitFor(() =>
            expect(mockPost).toHaveBeenCalledWith('/auth/signup', {
                username: 'emailfree',
                password: 'StrongPass123',
                age_attested: true,
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
        fireEvent.click(screen.getByLabelText(/I confirm I am at least 15 years old/i));
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(screen.getByText('Username taken')).toBeInTheDocument();
        });
    });
});
