import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import VerifyEmail from './VerifyEmail';

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

describe('VerifyEmail', () => {
    it('verifies valid, invalid, and missing email tokens', async () => {
        mocks.post.mockResolvedValueOnce({ data: {} });
        render(
            <MemoryRouter initialEntries={['/verify-email?token=verify-token']}>
                <VerifyEmail />
            </MemoryRouter>,
        );
        expect(await screen.findByText('Email verified.')).toBeInTheDocument();

        mocks.post.mockRejectedValueOnce(new Error('expired verification'));
        render(
            <MemoryRouter initialEntries={['/verify-email?token=expired']}>
                <VerifyEmail />
            </MemoryRouter>,
        );
        expect(await screen.findByText('expired verification')).toBeInTheDocument();

        render(
            <MemoryRouter initialEntries={['/verify-email']}>
                <VerifyEmail />
            </MemoryRouter>,
        );
        expect(screen.getByText('Verification token is missing.')).toBeInTheDocument();
    });
});
