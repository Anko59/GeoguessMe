import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext, type AuthContextValue } from '../../context/AuthContext';
import GroupView from './GroupView';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

// Stub useGroupMessages so GroupView tests focus on its own wiring without
// depending on WebSocket lifecycle or reconnect timing.
const mockUseGroupMessages = vi.fn();
vi.mock('../../hooks/useGroupMessages', () => ({
    useGroupMessages: (id?: string) => mockUseGroupMessages(id),
}));

vi.mock('../../components/chat/Chat', () => ({
    default: ({
        connectionStatus,
        onChallengeMessage,
    }: {
        connectionStatus?: string;
        onChallengeMessage?: (msg: unknown) => void;
    }) => (
        <div data-testid="chat">
            <span data-testid="chat-status">{connectionStatus ?? 'offline'}</span>
            <button data-testid="chat-challenge" onClick={() => onChallengeMessage?.({ id: 'challenge-1' })}>
                Trigger challenge
            </button>
        </div>
    ),
}));

vi.mock('../../components/leaderboard/Leaderboard', () => ({
    default: () => <div data-testid="leaderboard">Leaderboard</div>,
}));

vi.mock('../../components/camera/Camera', () => ({
    default: () => <div data-testid="camera">Camera</div>,
}));

vi.mock('../../components/game/Game', () => ({
    default: ({ gameMessage, onClose }: { gameMessage: unknown; onClose: () => void }) =>
        gameMessage ? (
            <div data-testid="game">
                <button onClick={onClose}>Close game</button>
            </div>
        ) : null,
}));

vi.mock('../../components/settings/SettingsModal', () => ({
    default: ({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) =>
        isOpen ? (
            <div data-testid="settings-modal">
                <button data-testid="close-settings" onClick={onClose}>
                    Close
                </button>
            </div>
        ) : null,
}));

vi.mock('../../components/navigation/TabBar', () => ({
    default: ({ activeTab, onTabChange }: { activeTab: string; onTabChange: (tab: string) => void }) => (
        <div data-testid="tab-bar">
            <button data-testid="tab-chat" onClick={() => onTabChange('chat')}>
                Chat
            </button>
            <button data-testid="tab-camera" onClick={() => onTabChange('camera')}>
                Camera
            </button>
            <button data-testid="tab-leaderboard" onClick={() => onTabChange('leaderboard')}>
                Leaderboard
            </button>
            <span data-testid="active-tab">{activeTab}</span>
        </div>
    ),
}));

const authValue = (overrides: Partial<AuthContextValue> = {}): AuthContextValue => ({
    user: { id: 'user-1', username: 'alice', email: 'alice@example.test', avatar: 'avatar.png' },
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => true),
    ...overrides,
});

function renderGroupView(groupId: string, auth = authValue()) {
    return render(
        <AuthContext.Provider value={auth}>
            <MemoryRouter initialEntries={[`/group/${groupId}`]}>
                <Routes>
                    <Route path="/group/:id" element={<GroupView />} />
                </Routes>
            </MemoryRouter>
        </AuthContext.Provider>,
    );
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    mockUseGroupMessages.mockReset();
    mockUseGroupMessages.mockReturnValue({
        messages: [],
        connectionStatus: 'connected',
        wsRef: { current: null },
        error: '',
    });
    mocks.get.mockResolvedValue({
        data: { id: 'group-1', name: 'Test Group', code: 'ABC123', created_at: '2026-01-01T00:00:00Z' },
    });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (Element.prototype as any).scrollIntoView = vi.fn();
});

describe('GroupView', () => {
    it('shows invalid group id message when id is missing', () => {
        render(
            <AuthContext.Provider value={authValue()}>
                <MemoryRouter initialEntries={['/group/']}>
                    <Routes>
                        <Route path="/group/" element={<GroupView />} />
                    </Routes>
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        expect(screen.getByText('Invalid Group ID')).toBeInTheDocument();
    });

    it('displays the group name after fetch', async () => {
        renderGroupView('group-1');
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
    });

    it('shows an error when group details fetch fails', async () => {
        mocks.get.mockRejectedValue(new Error('group unavailable'));
        renderGroupView('group-1');
        expect(await screen.findByRole('alert')).toHaveTextContent('group unavailable');
    });

    it('shows messages error when useGroupMessages reports an error', async () => {
        mockUseGroupMessages.mockReturnValue({
            messages: [],
            connectionStatus: 'offline',
            wsRef: { current: null },
            error: 'reconnect failed',
        });
        renderGroupView('group-1');
        // The group fetch succeeds so the group name is still present.
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
        // The messages error is also displayed.
        expect(screen.getByRole('alert')).toHaveTextContent('reconnect failed');
    });

    it('passes connectionStatus to Chat', async () => {
        mockUseGroupMessages.mockReturnValue({
            messages: [],
            connectionStatus: 'offline',
            wsRef: { current: null },
            error: '',
        });
        renderGroupView('group-1');
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
        expect(screen.getByTestId('chat-status')).toHaveTextContent('offline');
    });

    it('passes connected status through to Chat', async () => {
        mockUseGroupMessages.mockReturnValue({
            messages: [
                {
                    id: 'm1',
                    group_id: 'g1',
                    user_id: 'u1',
                    kind: 'text',
                    content: 'hi',
                    created_at: '2026-01-01T00:00:00Z',
                },
            ],
            connectionStatus: 'connected',
            wsRef: { current: null },
            error: '',
        });
        renderGroupView('group-1');
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
        expect(screen.getByTestId('chat-status')).toHaveTextContent('connected');
    });

    it('opens and closes the settings modal', async () => {
        renderGroupView('group-1');
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
        expect(screen.queryByTestId('settings-modal')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Open group settings' }));
        expect(screen.getByTestId('settings-modal')).toBeInTheDocument();
        fireEvent.click(screen.getByTestId('close-settings'));
        await waitFor(() => expect(screen.queryByTestId('settings-modal')).toBeNull());
    });

    it('switches tabs between chat, camera, and leaderboard', async () => {
        renderGroupView('group-1');
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
        // Default tab is chat.
        expect(screen.getByTestId('active-tab')).toHaveTextContent('chat');
        expect(screen.getByTestId('chat')).toBeInTheDocument();

        fireEvent.click(screen.getByTestId('tab-camera'));
        expect(screen.getByTestId('active-tab')).toHaveTextContent('camera');
        expect(screen.getByTestId('camera')).toBeInTheDocument();

        fireEvent.click(screen.getByTestId('tab-leaderboard'));
        expect(screen.getByTestId('active-tab')).toHaveTextContent('leaderboard');
        expect(screen.getByTestId('leaderboard')).toBeInTheDocument();
    });

    it('opens the game when a challenge message is received', async () => {
        renderGroupView('group-1');
        await waitFor(() => expect(screen.getByText('Test Group')).toBeInTheDocument());
        expect(screen.queryByTestId('game')).toBeNull();
        fireEvent.click(screen.getByTestId('chat-challenge'));
        expect(screen.getByTestId('game')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close game' }));
        expect(screen.queryByTestId('game')).toBeNull();
    });
});
