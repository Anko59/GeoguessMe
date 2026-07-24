import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import SettingsModal from './SettingsModal';
import type { User } from '../../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
    email_verified_at: null,
};

const authValue = {
    user,
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
};

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
});

describe('SettingsModal', () => {
    it('copies invite data and loads members', async () => {
        Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: { writeText: vi.fn().mockResolvedValue(undefined) },
        });
        mocks.get.mockResolvedValueOnce({
            data: [{ id: 'member-1', username: 'bob', avatar: 'avatar.png' }],
        });
        const onClose = vi.fn();
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal
                        isOpen
                        onClose={onClose}
                        groupCode="ABC123"
                        groupName="Friends"
                        groupId="group-1"
                        currentUserName="alice"
                    />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        expect(screen.getByDisplayValue(`${window.location.origin}/invite/ABC123?from=alice`)).toBeInTheDocument();
        fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0]);
        expect(await screen.findByText('Copied!')).toBeInTheDocument();
        const membersToggle = screen.getByRole('button', { name: 'Group Members' });
        expect(membersToggle).toHaveAttribute('aria-expanded', 'false');
        fireEvent.click(membersToggle);
        expect(membersToggle).toHaveAttribute('aria-expanded', 'true');
        expect(await screen.findByText('bob')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close settings' }));
        expect(onClose).toHaveBeenCalled();
    });

    it('shows member load failures', async () => {
        mocks.get.mockRejectedValueOnce(new Error('members unavailable'));
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal
                        isOpen
                        onClose={vi.fn()}
                        groupCode="ABC"
                        groupName="Group"
                        groupId="g"
                        currentUserName="bob"
                    />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        fireEvent.click(screen.getByText('Group Members'));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load members');
    });
});
