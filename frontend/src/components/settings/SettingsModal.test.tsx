import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import SettingsModal from './SettingsModal';
import type { User } from '../../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get, post: mocks.post, put: mocks.put, delete: mocks.delete },
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
    mocks.post.mockReset();
    mocks.put.mockReset();
    mocks.delete.mockReset();
    mocks.get.mockImplementation((url: string) => {
        if (url === '/group/notifications') return Promise.resolve({ data: { enabled: true } });
        if (url === '/group/invites') return Promise.resolve({ data: [] });
        if (url === '/group/members') return Promise.resolve({ data: [] });
        return Promise.resolve({ data: {} });
    });
    mocks.post.mockResolvedValue({ data: {} });
    mocks.put.mockResolvedValue({ data: { enabled: true } });
    mocks.delete.mockResolvedValue({});
});

describe('SettingsModal', () => {
    it('creates an invite link (shown once) and lists invites without the token', async () => {
        Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: { writeText: vi.fn().mockResolvedValue(undefined) },
        });
        mocks.get.mockImplementation((url: string) => {
            if (url === '/group/notifications') return Promise.resolve({ data: { enabled: true } });
            if (url === '/group/invites') {
                return Promise.resolve({
                    data: [
                        {
                            id: 'inv-1',
                            creator_user_id: 'u1',
                            created_at: '2026-08-01T00:00:00Z',
                            expires_at: '2026-08-08T00:00:00Z',
                            revoked: null,
                        },
                        {
                            id: 'inv-2',
                            creator_user_id: 'u1',
                            created_at: '2026-08-02T00:00:00Z',
                            expires_at: '2026-08-09T00:00:00Z',
                            revoked: '2026-08-03T00:00:00Z',
                        },
                    ],
                });
            }
            if (url === '/group/members') return Promise.resolve({ data: [] });
            return Promise.resolve({ data: {} });
        });
        mocks.post.mockImplementation((url: string) => {
            if (url === '/group/invites') {
                return Promise.resolve({
                    data: {
                        id: 'new-inv',
                        group_id: 'group-1',
                        token: 'secret-token',
                        invite_url: '/group/join#invite=secret-token',
                        created_at: '2026-08-05T00:00:00Z',
                        expires_at: '2026-08-12T00:00:00Z',
                    },
                });
            }
            return Promise.resolve({ data: {} });
        });
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal isOpen onClose={vi.fn()} groupName="Friends" groupId="group-1" />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        // The invite list shows IDs/creators/dates/state but never the token.
        expect(await screen.findByText('Active')).toBeInTheDocument();
        expect(screen.getByText('Revoked')).toBeInTheDocument();
        expect(screen.getByText(/inv-1 · by u1/)).toBeInTheDocument();
        expect(screen.queryByText('secret-token')).toBeNull();

        fireEvent.click(screen.getByRole('button', { name: 'Create invite link' }));
        expect(await screen.findByText('This invite link is shown only once. Copy and share it.')).toBeInTheDocument();
        expect(
            screen.getByDisplayValue(`${window.location.origin}/group/join#invite=secret-token`),
        ).toBeInTheDocument();
        expect(mocks.post).toHaveBeenCalledWith('/group/invites', { group_id: 'group-1' });

        fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0]);
        expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
            `${window.location.origin}/group/join#invite=secret-token`,
        );
        expect(await screen.findByText('Copied!')).toBeInTheDocument();
    });

    it('revokes an invite and refreshes the list', async () => {
        mocks.get.mockImplementation((url: string) => {
            if (url === '/group/notifications') return Promise.resolve({ data: { enabled: true } });
            if (url === '/group/invites') {
                return Promise.resolve({
                    data: [
                        {
                            id: 'inv-1',
                            creator_user_id: 'u1',
                            created_at: '2026-08-01T00:00:00Z',
                            expires_at: '2026-08-08T00:00:00Z',
                            revoked: null,
                        },
                    ],
                });
            }
            if (url === '/group/members') return Promise.resolve({ data: [] });
            return Promise.resolve({ data: {} });
        });
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal isOpen onClose={vi.fn()} groupName="Friends" groupId="group-1" />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        fireEvent.click(await screen.findByRole('button', { name: 'Revoke' }));
        await waitFor(() => expect(mocks.delete).toHaveBeenCalledWith('/group/invites/inv-1'));
    });

    it('loads group members', async () => {
        mocks.get.mockImplementation((url: string) => {
            if (url === '/group/notifications') return Promise.resolve({ data: { enabled: true } });
            if (url === '/group/invites') return Promise.resolve({ data: [] });
            if (url.startsWith('/group/members')) {
                return Promise.resolve({
                    data: [{ id: 'member-1', username: 'bob', avatar: 'avatar.png' }],
                });
            }
            return Promise.resolve({ data: {} });
        });
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal isOpen onClose={vi.fn()} groupName="Group" groupId="g" />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        fireEvent.click(screen.getByText('Group Members'));
        expect(await screen.findByText('bob')).toBeInTheDocument();
    });

    it('reports member load failures', async () => {
        mocks.get.mockImplementation((url: string) => {
            if (url === '/group/notifications') return Promise.resolve({ data: { enabled: true } });
            if (url === '/group/invites') return Promise.resolve({ data: [] });
            if (url.startsWith('/group/members')) return Promise.reject(new Error('members unavailable'));
            return Promise.resolve({ data: {} });
        });
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal isOpen onClose={vi.fn()} groupName="Group" groupId="g" />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        fireEvent.click(screen.getByText('Group Members'));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load members');
    });

    it('updates the group photo and notification preference', async () => {
        const onGroupPhotoUpdated = vi.fn();
        render(
            <AuthContext.Provider value={authValue}>
                <MemoryRouter>
                    <SettingsModal
                        isOpen
                        onClose={vi.fn()}
                        groupName="Group"
                        groupId="g"
                        onGroupPhotoUpdated={onGroupPhotoUpdated}
                    />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        const file = new File(['image'], 'group.jpg', { type: 'image/jpeg' });
        fireEvent.change(screen.getByLabelText('Choose a group photo'), { target: { files: [file] } });
        await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/group/photo', expect.any(FormData)));
        expect(onGroupPhotoUpdated).toHaveBeenCalled();
        fireEvent.click(screen.getByRole('checkbox', { name: 'Group notifications' }));
        await waitFor(() =>
            expect(mocks.put).toHaveBeenCalledWith('/group/notifications?group_id=g', { enabled: false }),
        );
    });
});
