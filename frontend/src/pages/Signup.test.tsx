import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { vi } from 'vitest';
import Signup from './Signup';

// Mock the API module
const mockPost = vi.fn();
vi.mock('../api', () => ({
    default: {
        post: (...args: any[]) => mockPost(...args),
    },
}));

describe('Signup Page', () => {
    beforeEach(() => {
        vi.clearAllMocks();
    });

    it('renders signup form', () => {
        render(
            <BrowserRouter>
                <Signup />
            </BrowserRouter>
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
            <BrowserRouter>
                <Signup />
            </BrowserRouter>
        );

        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'newuser' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(mockPost).toHaveBeenCalledWith('/signup', { username: 'newuser', password: 'StrongPass123' });
        });
    });

    it('displays error on failed signup', async () => {
        mockPost.mockRejectedValue(new Error('Username taken'));

        render(
            <BrowserRouter>
                <Signup />
            </BrowserRouter>
        );

        fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'taken' } });
        fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'StrongPass123' } });
        fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

        await waitFor(() => {
            expect(screen.getByText('Username taken')).toBeInTheDocument();
        });
    });
});
