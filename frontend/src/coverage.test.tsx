import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getAPIErrorMessage, getAccessToken, setAccessToken } from './api';
import { AuthContext, useAuth, type AuthContextValue } from './context/AuthContext';
import AuthProvider from './context/AuthProvider';
import AccountSettings from './pages/account/AccountSettings';
import ForgotPassword from './pages/auth/ForgotPassword';
import ResetPassword from './pages/auth/ResetPassword';
import VerifyEmail from './pages/auth/VerifyEmail';
import GroupsList from './pages/groups/GroupsList';
import GroupJoin from './pages/groups/GroupJoin';
import Chat from './components/chat/Chat';
import Game from './components/game/Game';
import Leaderboard from './components/leaderboard/Leaderboard';
import TabBar from './components/navigation/TabBar';
import SettingsModal from './components/settings/SettingsModal';
import type { AuthResponse, Message, User } from './types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
    setAccessToken: vi.fn(),
    token: null as string | null,
}));

vi.mock('./api', () => ({
    default: { get: mocks.get, post: mocks.post, delete: mocks.delete },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
    getAccessToken: () => mocks.token,
    setAccessToken: (token: string | null) => {
        mocks.token = token;
        mocks.setAccessToken(token);
    },
}));

vi.mock('./components/map/Map', () => ({
    default: ({ onLocationSelect }: { onLocationSelect: (lat: number, long: number) => void }) => (
        <button onClick={() => onLocationSelect(48.8, 2.3)}>Map</button>
    ),
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
    email_verified_at: null,
};

const authResponse: AuthResponse = { access_token: 'access-token', expires_in: 900, user };

const authValue = (overrides: Partial<AuthContextValue> = {}): AuthContextValue => ({
    user: null,
    loading: false,
    isAuthenticated: false,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
    ...overrides,
});

function withAuth(element: React.ReactNode, value = authValue()) {
    return render(
        <AuthContext.Provider value={value}>
            <MemoryRouter>{element}</MemoryRouter>
        </AuthContext.Provider>,
    );
}

function Location() {
    const location = useLocation();
    return <output data-testid="location">{location.pathname}</output>;
}

const message = (overrides: Partial<Message> = {}): Message => ({
    id: 'message-1',
    group_id: 'group-1',
    user_id: 'user-2',
    username: 'bob',
    avatar: 'avatar.png',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
});

beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.delete.mockReset();
    Element.prototype.scrollIntoView = vi.fn();
    setAccessToken(null);
});

describe('API and auth context', () => {
    it('stores access tokens and formats API errors', () => {
        setAccessToken('token');
        expect(getAccessToken()).toBe('token');
        setAccessToken(null);
        expect(getAccessToken()).toBeNull();
        expect(getAPIErrorMessage(new Error('boom'), 'fallback')).toBe('boom');
        expect(getAPIErrorMessage('unknown', 'fallback')).toBe('fallback');
    });

    it('restores, logs in, and logs out through AuthProvider', async () => {
        mocks.post.mockResolvedValueOnce({ data: authResponse }).mockResolvedValueOnce({ data: {} });
        function Consumer() {
            const auth = useAuth();
            return (
                <>
                    <output>{auth.loading ? 'loading' : (auth.user?.username ?? 'signed-out')}</output>
                    <button onClick={() => auth.login(authResponse)}>Login</button>
                    <button onClick={() => void auth.logout()}>Logout</button>
                </>
            );
        }
        render(
            <MemoryRouter>
                <AuthProvider>
                    <Consumer />
                </AuthProvider>
            </MemoryRouter>,
        );
        expect(await screen.findByText('alice')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Logout' }));
        await waitFor(() => expect(screen.getByText('signed-out')).toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', { name: 'Login' }));
        expect(screen.getByText('alice')).toBeInTheDocument();
        expect(mocks.post).toHaveBeenCalledWith('/auth/logout');
    });

    it('clears a failed restored session and guards useAuth', async () => {
        mocks.post.mockRejectedValue(new Error('expired'));
        function Consumer() {
            return <output>{useAuth().isAuthenticated ? 'yes' : 'no'}</output>;
        }
        render(
            <MemoryRouter>
                <AuthProvider>
                    <Consumer />
                </AuthProvider>
            </MemoryRouter>,
        );
        expect(await screen.findByText('no')).toBeInTheDocument();
        expect(() => render(<Consumer />)).toThrow('useAuth must be used inside AuthProvider');
    });
});

describe('password and email pages', () => {
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

describe('group pages', () => {
    it('renders empty and populated groups, including a retry', async () => {
        mocks.get.mockRejectedValueOnce(new Error('temporary failure')).mockResolvedValueOnce({
            data: [{ id: 'group-1', name: 'Friends', code: 'ABC123' }],
        });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load groups');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        expect(await screen.findByText('Friends')).toBeInTheDocument();
        expect(screen.getByText('#ABC123')).toBeInTheDocument();

        mocks.get.mockResolvedValueOnce({ data: [] });
        render(
            <MemoryRouter>
                <GroupsList />
            </MemoryRouter>,
        );
        expect(await screen.findByText("You haven't joined any groups yet")).toBeInTheDocument();
    });

    it('joins and creates groups and reports API errors', async () => {
        mocks.post.mockResolvedValueOnce({ data: { id: 'joined' } });
        const firstRender = render(
            <MemoryRouter initialEntries={['/group/join?code=abc123']}>
                <GroupJoin />
                <Location />
            </MemoryRouter>,
        );
        expect(screen.getByDisplayValue('abc123')).toBeInTheDocument();
        fireEvent.click(screen.getAllByRole('button', { name: 'Join Group' })[1]);
        await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/group/joined'));
        expect(mocks.post).toHaveBeenCalledWith('/group/join', { code: 'abc123' });

        firstRender.unmount();
        mocks.post.mockRejectedValueOnce(new Error('bad group name'));
        render(
            <MemoryRouter initialEntries={['/group/create']}>
                <GroupJoin />
            </MemoryRouter>,
        );
        fireEvent.change(screen.getByPlaceholderText('Group Name'), { target: { value: 'Bad' } });
        fireEvent.click(screen.getAllByRole('button', { name: 'Create Group' })[1]);
        expect(await screen.findByText('bad group name')).toBeInTheDocument();
    });
});

describe('chat, navigation, and leaderboard components', () => {
    it('renders chat states, sends messages, and opens challenges', () => {
        const send = vi.fn();
        const wsRef = { current: { readyState: WebSocket.OPEN, send } } as unknown as React.RefObject<WebSocket | null>;
        const onChallenge = vi.fn();
        render(
            <Chat
                messages={[
                    message(),
                    message({ id: 'system', kind: 'system', content: 'System update' }),
                    message({
                        id: 'challenge',
                        kind: 'challenge',
                        photo_id: 'photo-1',
                        user_id: 'user-1',
                        content: 'Challenge',
                    }),
                ]}
                wsRef={wsRef}
                currentUserId="user-1"
                connectionStatus="connected"
                onChallengeMessage={onChallenge}
            />,
        );
        expect(screen.getByRole('status')).toHaveTextContent('Connected');
        fireEvent.click(screen.getByRole('button', { name: /challenge/i }));
        expect(onChallenge).toHaveBeenCalled();
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: '  hi  ' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).toHaveBeenCalledWith(JSON.stringify({ content: 'hi' }));

        render(<Chat messages={[]} wsRef={{ current: null }} currentUserId="user-1" connectionStatus="offline" />);
        expect(screen.getByText('Offline — retrying')).toBeInTheDocument();
        expect(screen.getByText('No messages yet')).toBeInTheDocument();
    });

    it('renders leaderboard loading, empty, error, and ranked states', async () => {
        mocks.get.mockResolvedValueOnce({ data: [] });
        const emptyLeaderboard = render(
            <AuthContext.Provider value={authValue({ user })}>
                <Leaderboard groupID="group-1" />
            </AuthContext.Provider>,
        );
        expect(await screen.findByText('No scores yet')).toBeInTheDocument();
        emptyLeaderboard.unmount();

        mocks.get.mockResolvedValueOnce({
            data: [
                { user_id: 'user-1', username: 'alice', score: 100, guess_count: 1, average_score: 100 },
                { user_id: 'user-2', username: 'bob', score: 80, guess_count: 1, average_score: 80 },
                { user_id: 'user-3', username: 'eve', score: 60, guess_count: 1, average_score: 60 },
                { user_id: 'user-4', username: 'dan', score: 40, guess_count: 1, average_score: 40 },
            ],
        });
        const rankedLeaderboard = render(
            <AuthContext.Provider value={authValue({ user })}>
                <Leaderboard groupID="group-1" />
            </AuthContext.Provider>,
        );
        expect(await screen.findByText('alice')).toBeInTheDocument();
        expect(screen.getByText('You')).toBeInTheDocument();
        expect(screen.getByText('#4')).toBeInTheDocument();
        rankedLeaderboard.unmount();

        mocks.get.mockRejectedValueOnce(new Error('rankings unavailable'));
        render(
            <AuthContext.Provider value={authValue({ user })}>
                <Leaderboard groupID="group-1" />
            </AuthContext.Provider>,
        );
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load rankings');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    });

    it('changes tabs', () => {
        const onTabChange = vi.fn();
        render(<TabBar activeTab="chat" onTabChange={onTabChange} />);
        fireEvent.click(screen.getByRole('button', { name: /camera/i }));
        fireEvent.click(screen.getByRole('button', { name: /leaderboard/i }));
        expect(onTabChange).toHaveBeenNthCalledWith(1, 'camera');
        expect(onTabChange).toHaveBeenNthCalledWith(2, 'leaderboard');
    });
});

describe('settings and account flows', () => {
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
            <AuthContext.Provider value={authValue({ user })}>
                <MemoryRouter>
                    <SettingsModal isOpen onClose={onClose} groupCode="ABC123" groupName="Friends" groupId="group-1" />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        expect(screen.getByDisplayValue(`${window.location.origin}/group/join?code=ABC123`)).toBeInTheDocument();
        fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0]);
        expect(await screen.findByText('Copied!')).toBeInTheDocument();
        fireEvent.click(screen.getByText('Group Members'));
        expect(await screen.findByText('bob')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close settings' }));
        expect(onClose).toHaveBeenCalled();
    });

    it('shows member load failures and account verification/deletion flows', async () => {
        mocks.get.mockRejectedValueOnce(new Error('members unavailable'));
        render(
            <AuthContext.Provider value={authValue({ user })}>
                <MemoryRouter>
                    <SettingsModal isOpen onClose={vi.fn()} groupCode="ABC" groupName="Group" groupId="g" />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        fireEvent.click(screen.getByText('Group Members'));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load members');

        const refresh = vi.fn(async () => true);
        mocks.post.mockResolvedValueOnce({ data: { message: 'Verification sent' } });
        mocks.delete.mockResolvedValueOnce({ data: {} });
        vi.stubGlobal('confirm', vi.fn().mockReturnValue(true));
        const value = authValue({ user, isAuthenticated: true, refresh });
        withAuth(<AccountSettings />, value);
        fireEvent.click(screen.getByRole('button', { name: 'Resend verification email' }));
        expect(await screen.findByRole('status')).toHaveTextContent('Verification sent');
        fireEvent.change(screen.getByLabelText('Confirm password to delete account'), {
            target: { value: 'password' },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Delete account' }));
        await waitFor(() => expect(refresh).toHaveBeenCalled());
        vi.restoreAllMocks();
    });
});

describe('game state transitions', () => {
    it('loads existing results, closes them, and handles unavailable challenges', async () => {
        const onClose = vi.fn();
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-1',
                group_id: 'group-1',
                actual_lat: 48,
                actual_long: 2,
                media_available: false,
                guesses: [],
                server_time: new Date().toISOString(),
            },
        });
        withAuth(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-1', kind: 'challenge' })}
                onClose={onClose}
            />,
            authValue({ user }),
        );
        expect(await screen.findByText('Challenge results')).toBeInTheDocument();
        expect(screen.getByText('The original image has been removed; scores remain available.')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close' }));
        expect(onClose).toHaveBeenCalled();

        mocks.get.mockRejectedValueOnce(new Error('not ready'));
        mocks.post.mockRejectedValueOnce(new Error('gone'));
        withAuth(
            <Game gameMessage={message({ photo_id: 'photo-2', kind: 'challenge' })} onClose={vi.fn()} />,
            authValue({ user }),
        );
        expect(await screen.findByText('Challenge unavailable')).toBeInTheDocument();
        expect(screen.getByText('gone')).toBeInTheDocument();
    });

    it('accepts a challenge, selects a location, and submits a guess', async () => {
        mocks.get.mockRejectedValueOnce(new Error('results not ready'));
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: 'https://example.test/photo.jpg',
                    server_time: new Date().toISOString(),
                    view_expires_at: new Date(Date.now() + 2000).toISOString(),
                },
            })
            .mockResolvedValueOnce({ data: {} });
        withAuth(
            <Game gameMessage={message({ photo_id: 'photo-3', kind: 'challenge' })} onClose={vi.fn()} />,
            authValue({ user }),
        );
        expect(await screen.findByAltText('Challenge location')).toBeInTheDocument();
    });
});
