import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { Group, Message, MessagesPage } from '../../types';
import Chat from '../../components/chat/Chat';
import Leaderboard from '../../components/leaderboard/Leaderboard';
import Camera from '../../components/camera/Camera';
import Game from '../../components/game/Game';
import SettingsModal from '../../components/settings/SettingsModal';
import TabBar, { type TabType } from '../../components/navigation/TabBar';
import './GroupView.css';

type ConnectionStatus = 'connecting' | 'connected' | 'offline';

export default function GroupView() {
    const { id } = useParams();
    const { user } = useAuth();
    const [activeTab, setActiveTab] = useState<TabType>('chat');
    const [group, setGroup] = useState<Group | null>(null);
    const [messages, setMessages] = useState<Message[]>([]);
    const [gameMessage, setGameMessage] = useState<Message | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('connecting');
    const [error, setError] = useState('');
    const ws = useRef<WebSocket | null>(null);
    const reconnectTimer = useRef<number | undefined>(undefined);
    const stopped = useRef(false);
    const retryCount = useRef(0);
    const messagesRef = useRef<Message[]>([]);

    const mergeMessages = useCallback((incoming: Message[]): void => {
        setMessages((current) => {
            const byId = new Map(current.map((message) => [message.id, message]));
            incoming.forEach((message) => byId.set(message.id, message));
            return [...byId.values()].sort((a, b) => a.created_at.localeCompare(b.created_at));
        });
    }, []);

    useEffect(() => { messagesRef.current = messages; }, [messages]);

    const loadMessages = useCallback(async (afterId?: string): Promise<void> => {
        if (!id) return;
        try {
            const response = await api.get<MessagesPage | Message[]>('/group/messages', { params: { group_id: id, ...(afterId ? { after_id: afterId } : {}) } });
            const payload = response.data;
            const items = Array.isArray(payload) ? payload : payload.items;
            mergeMessages(items ?? []);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to load messages'));
        }
    }, [id, mergeMessages]);

    useEffect(() => {
        if (!id) return;
        void api.get<Group>('/group/details', { params: { id } }).then((response) => setGroup(response.data)).catch((requestError: unknown) => setError(getAPIErrorMessage(requestError, 'Unable to load group')));
    }, [id]);

    useEffect(() => {
        if (!id) return;
        stopped.current = false;
        const connect = async (): Promise<void> => {
            if (stopped.current) return;
            await loadMessages();
            setConnectionStatus('connecting');
            try {
                const ticketResponse = await api.post<{ ticket: string }>('/ws/ticket', undefined, { params: { group_id: id } });
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                const socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws?group_id=${encodeURIComponent(id)}&ticket=${encodeURIComponent(ticketResponse.data.ticket)}`);
                ws.current = socket;
                socket.onopen = () => { retryCount.current = 0; setConnectionStatus('connected'); };
                socket.onmessage = (event: MessageEvent<string>) => {
                    try { const message = JSON.parse(event.data) as Message; if (message.id) mergeMessages([message]); }
                    catch { setError('Received an invalid chat message'); }
                };
                socket.onclose = () => {
                    if (stopped.current) return;
                    setConnectionStatus('offline');
                    const delay = Math.min(30000, 500 * (2 ** retryCount.current)) + Math.floor(Math.random() * 500);
                    retryCount.current += 1;
                    const latest = messagesRef.current[messagesRef.current.length - 1]?.id;
                    reconnectTimer.current = window.setTimeout(() => { void loadMessages(latest); void connect(); }, delay);
                };
                socket.onerror = () => setConnectionStatus('offline');
            } catch {
                setConnectionStatus('offline');
                reconnectTimer.current = window.setTimeout(() => void connect(), 1500);
            }
        };
        void connect();
        return () => { stopped.current = true; if (reconnectTimer.current) window.clearTimeout(reconnectTimer.current); ws.current?.close(); ws.current = null; };
    }, [id, loadMessages, mergeMessages]);

    if (!id) return <div>Invalid Group ID</div>;
    return <div className="group-view">
        <div className="group-header"><div className="header-content"><Link to="/groups" className="back-btn"><img src="/back_arrow.png" alt="Back" className="back-arrow-icon" /></Link><img src="/logo.png" alt="GeoGuessMe" className="header-logo" /><h1 className="group-name gradient-text">{group?.name ?? 'Group'}</h1><button className="settings-btn" onClick={() => setSettingsOpen(true)} aria-label="Open group settings"><img src="/settings_gear_icon.png" alt="" /></button></div></div>
        {error && <div className="error-message" role="alert">{error}</div>}
        <SettingsModal isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} groupCode={group?.code ?? ''} groupName={group?.name ?? ''} groupId={id} />
        <div className="tab-content">
            {activeTab === 'camera' && <div className="tab-panel fade-in"><Camera groupID={id} onUploadComplete={() => setActiveTab('chat')} /></div>}
            {activeTab === 'chat' && <div className="tab-panel fade-in"><Chat messages={messages} wsRef={ws} currentUserId={user?.id ?? ''} connectionStatus={connectionStatus} onChallengeMessage={setGameMessage} /></div>}
            {activeTab === 'leaderboard' && <div className="tab-panel fade-in"><Leaderboard groupID={id} /></div>}
        </div>
        <Game gameMessage={gameMessage} onClose={() => setGameMessage(null)} />
        <TabBar activeTab={activeTab} onTabChange={setActiveTab} />
    </div>;
}
