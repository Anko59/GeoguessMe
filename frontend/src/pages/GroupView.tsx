import { useState, useEffect, useRef } from 'react';
import { useParams, Link } from 'react-router-dom';
import api from '../api';
import Chat from '../components/Chat';
import Leaderboard from '../components/Leaderboard';
import Camera from '../components/Camera';
import Game from '../components/Game';
import SettingsModal from '../components/SettingsModal';
import TabBar, { type TabType } from '../components/TabBar';
import './GroupView.css';

interface Message {
    id: string;
    group_id: string;
    user_id: string;
    username?: string;
    content: string;
    created_at: string;
}

export default function GroupView() {
    const { id } = useParams();
    const [activeTab, setActiveTab] = useState<TabType>('chat');
    const [groupName, setGroupName] = useState<string>('');
    const [groupCode, setGroupCode] = useState<string>('');
    const [newMessagesCount, setNewMessagesCount] = useState(0);
    const [messages, setMessages] = useState<Message[]>([]);
    const [gameMessage, setGameMessage] = useState<Message | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [myGuesses, setMyGuesses] = useState<string[]>([]);
    const ws = useRef<WebSocket | null>(null);

    useEffect(() => {
        // Fetch group details to get the name and code
        const fetchGroupDetails = async () => {
            try {
                const res = await api.get(`/group/details?id=${id}`);
                setGroupName(res.data.name || `Group ${id?.substring(0, 6).toUpperCase()}`);
                setGroupCode(res.data.code || '');
            } catch (err) {
                // Fallback to ID if endpoint doesn't exist yet
                setGroupName(`Group ${id?.substring(0, 6).toUpperCase()}`);
            }
        };
        fetchGroupDetails();
    }, [id]);

    useEffect(() => {
        if (!id) return;
        const fetchMyGuesses = async () => {
            try {
                const res = await api.get(`/group/my_guesses?group_id=${id}`);
                setMyGuesses(res.data || []);
            } catch (err) {
                console.error('Failed to fetch guesses', err);
            }
        };
        fetchMyGuesses();
    }, [id, newMessagesCount]);

    // Single WebSocket connection for the entire group view
    useEffect(() => {
        if (!id) return;

        const token = localStorage.getItem('token');
        if (!token) return;

        // Load existing messages first
        const loadMessages = async () => {
            try {
                const res = await api.get(`/group/messages?group_id=${id}`);
                setMessages(res.data || []);
            } catch (err) {
                console.error('Failed to load messages:', err);
            }
        };
        loadMessages();

        // Then connect WebSocket for new messages
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const host = window.location.host;
        const wsUrl = `${protocol}//${host}/ws?group_id=${id}&token=${token}`;

        ws.current = new WebSocket(wsUrl);

        ws.current.onmessage = (event) => {
            const message = JSON.parse(event.data);
            setMessages((prev) => [...prev, message]);

            // Notification logic could go here
            if (message.content && message.content.startsWith('NEW_PHOTO:')) {
                setNewMessagesCount(prev => prev + 1);
            }
        };

        ws.current.onclose = () => {
            console.log('WebSocket disconnected');
        };

        return () => {
            ws.current?.close();
        };
    }, [id]);

    const handleStartChallenge = (message: Message) => {
        setGameMessage(message);
    };

    if (!id) return <div>Invalid Group ID</div>;

    return (
        <div className="group-view">
            {/* Header */}
            <div className="group-header">
                <div className="header-content">
                    <Link to="/groups" className="back-btn">
                        <img src="/back_arrow.png" alt="Back" className="back-arrow-icon" />
                    </Link>
                    <img src="/logo.png" alt="Logo" className="header-logo" />
                    <h1 className="group-name gradient-text">{groupName}</h1>
                    <button className="settings-btn" onClick={() => setSettingsOpen(true)}>
                        <img src="/settings_gear_icon.png" alt="Settings" />
                    </button>
                </div>
            </div>

            {/* Settings Modal */}
            <SettingsModal
                isOpen={settingsOpen}
                onClose={() => setSettingsOpen(false)}
                groupCode={groupCode}
                groupName={groupName}
                groupId={id}
            />

            {/* Tab Content */}
            <div className="tab-content">
                {activeTab === 'camera' && (
                    <div className="tab-panel fade-in">
                        <Camera groupID={id} onUploadComplete={() => setActiveTab('chat')} />
                    </div>
                )}
                {activeTab === 'chat' && (
                    <div className="tab-panel fade-in">
                        <Chat
                            groupID={id}
                            onNewMessage={() => setNewMessagesCount(0)}
                            messages={messages}
                            setMessages={setMessages}
                            wsRef={ws}
                            onChallengeMessage={handleStartChallenge}
                            myGuesses={myGuesses}
                        />
                    </div>
                )}
                {activeTab === 'leaderboard' && (
                    <div className="tab-panel fade-in">
                        <Leaderboard groupID={id} />
                    </div>
                )}
            </div>

            {/* Game Overlay */}
            <Game
                gameMessage={gameMessage}
                onGameComplete={() => {
                    // Refresh guesses
                    setNewMessagesCount(prev => prev + 1); // Hack to trigger useEffect
                }}
                myGuesses={myGuesses}
                onClose={() => setGameMessage(null)}
            />

            {/* Bottom Tab Bar */}
            <TabBar
                activeTab={activeTab}
                onTabChange={setActiveTab}
                newMessagesCount={newMessagesCount}
            />
        </div>
    );
}
