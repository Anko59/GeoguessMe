import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ForgotPassword from './ForgotPassword';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.post.mockReset();
});

describe('ForgotPassword', () => {
    it('submits forgot-password and displays success or error', async () => {
        mocks.post.mockResolvedValueOnce({ data: { message: 'Check your inbox' } });
        render(
            <MemoryRouter>
                <ForgotPassword />
            </MemoryRouter>,
        );
        fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'alice@example.test' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send reset link' }));
        expect(await screen.findByText('Check your inbox')).toBeInTheDocument();

        mocks.post.mockRejectedValueOnce(new Error('mail unavailable'));
        fireEvent.click(screen.getByRole('button', { name: 'Send reset link' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('mail unavailable');
    });
});
