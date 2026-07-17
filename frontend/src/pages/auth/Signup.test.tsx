import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { vi } from 'vitest';
import Signup from './Signup';
import { AuthContext } from '../../context/AuthContext';

// Mock the API module
const mockPost = vi.fn();
vi.mock('../../api', () => ({
    default: {
        post: (...args: unknown[]) => mockPost(...args),
    },
    getAPIErrorMessage: (error: unknown, fallback: string) => error instanceof Error ? error.message : fallback,
}));

const authValue = { user: null, loading: false, isAuthenticated: false, login: vi.fn(), logout: vi.fn(async () => undefined), refresh: vi.fn(async () => false) };

describe('Signup Page', () => {
    beforeEach(() => {
        vi.clearAllMocks();
    });

    it('renders signup form', () => {
        render(
            <AuthContext.Provider value={authValue}><BrowserRouter><Signup /></BrowserRouter></AuthContext.Provider>
        );

        expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /sign up/i })).toBeInTheDocument();
    });

    it('submits form with valid data', async () => {
        mockPost.mockResolvedValue({
            data: {
                token: 'fake-token',
                user: { id: '1', username: 'newuser' },
            }
        });

        render(
            <AuthContext.Provider value={authValue}><BrowserRouter><Signup /></BrowserRouter></AuthContext.Provider>
        );

        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'newuser' } });
        fireEvent.change(screen.getByPlaceholderText('Email'), { target: { value: 'new@example.com' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(mockPost).toHaveBeenCalledWith('/auth/signup', { username: 'newuser', email: 'new@example.com', password: 'StrongPass123' });
        });
    });

    it('displays error on failed signup', async () => {
        mockPost.mockRejectedValue(new Error('Username taken'));

        render(
            <AuthContext.Provider value={authValue}><BrowserRouter><Signup /></BrowserRouter></AuthContext.Provider>
        );

        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'taken' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(screen.getByText('Username taken')).toBeInTheDocument();
        });
    });
});
