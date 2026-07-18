import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ResetPassword from './ResetPassword';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    mocks.post.mockReset();
});

describe('ResetPassword', () => {
    it('resets a password and handles failures', async () => {
        mocks.post.mockResolvedValueOnce({ data: {} });
        render(
            <MemoryRouter initialEntries={['/reset-password?token=reset-token']}>
                <ResetPassword />
            </MemoryRouter>,
        );
        fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'NewStrongPassword1!' } });
        fireEvent.click(screen.getByRole('button', { name: 'Reset password' }));
        expect(await screen.findByRole('status')).toHaveTextContent('Password reset');
        vi.useFakeTimers();
        vi.advanceTimersByTime(1200);
        vi.useRealTimers();

        mocks.post.mockRejectedValueOnce(new Error('invalid reset token'));
        fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'AnotherStrongPassword1!' } });
        fireEvent.click(screen.getByRole('button', { name: 'Reset password' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('invalid reset token');
    });
});
