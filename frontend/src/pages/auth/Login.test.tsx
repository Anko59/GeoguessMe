import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { vi } from 'vitest';
import Login from './Login';
import { AuthContext } from '../../context/AuthContext';

// Mock the API module
const mockPost = vi.fn();
vi.mock('../../api', () => ({
    default: {
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
    });

    it('renders login form', () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument();
    });

    it('handles input changes', () => {
        render(
            <AuthContext.Provider value={authValue}>
                <BrowserRouter>
                    <Login />
                </BrowserRouter>
            </AuthContext.Provider>,
        );

        const usernameInput = screen.getByPlaceholderText('Username') as HTMLInputElement;
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

        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'wrong' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'wrong' } });
        fireEvent.click(screen.getByRole('button', { name: /login/i }));

        await waitFor(() => {
            expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
        });
    });
});
