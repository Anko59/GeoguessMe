import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { Group, Message } from '../../types';
import Chat from '../../components/chat/Chat';
import Leaderboard from '../../components/leaderboard/Leaderboard';
import Camera from '../../components/camera/Camera';
import Game from '../../components/game/Game';
import SettingsModal from '../../components/settings/SettingsModal';
import TabBar, { type TabType } from '../../components/navigation/TabBar';
import { useGroupMessages } from '../../hooks/useGroupMessages';
import Icon from '../../components/ui/Icon';
import './GroupView.css';

export default function GroupView() {
    const { id } = useParams();
    const { user } = useAuth();
    const [activeTab, setActiveTab] = useState<TabType>('chat');
    const [group, setGroup] = useState<Group | null>(null);
    const [gameMessage, setGameMessage] = useState<Message | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [groupError, setGroupError] = useState('');
    const { messages, connectionStatus, wsRef, error: messagesError, updateChallengeStatus } = useGroupMessages(id);

    useEffect(() => {
        if (!id) return;
        void api
            .get<Group>('/group/details', { params: { id } })
            .then((response) => setGroup(response.data))
            .catch((requestError: unknown) => setGroupError(getAPIErrorMessage(requestError, 'Unable to load group')));
    }, [id]);

    const error = groupError || messagesError;

    if (!id) return <div>Invalid Group ID</div>;
    return (
        <div className="group-view">
            <div className="group-header">
                <div className="header-content">
                    <Link to="/groups" className="back-btn">
                        <Icon name="arrow-left" className="back-arrow-icon" />
                        <span className="visually-hidden">Back to groups</span>
                    </Link>
                    <img src="/logo.png" alt="GeoGuessMe" className="header-logo" />
                    <div className="group-title-block">
                        <span>Group</span>
                        <h1 className="group-name">{group?.name ?? 'Group'}</h1>
                    </div>
                    <button
                        className="settings-btn"
                        onClick={() => setSettingsOpen(true)}
                        aria-label="Open group settings"
                    >
                        <img src="/settings_gear_icon.png" alt="" />
                    </button>
                </div>
            </div>
            {error && (
                <div className="error-message" role="alert">
                    {error}
                </div>
            )}
            <SettingsModal
                isOpen={settingsOpen}
                onClose={() => setSettingsOpen(false)}
                groupCode={group?.code ?? ''}
                groupName={group?.name ?? ''}
                groupId={id}
                currentUserName={user?.username ?? ''}
            />
            <div className="tab-content">
                {activeTab === 'camera' && (
                    <div className="tab-panel fade-in">
                        <Camera groupID={id} onUploadComplete={() => setActiveTab('chat')} />
                    </div>
                )}
                {activeTab === 'chat' && (
                    <div className="tab-panel fade-in">
                        <Chat
                            messages={messages}
                            wsRef={wsRef}
                            currentUserId={user?.id ?? ''}
                            connectionStatus={connectionStatus}
                            onChallengeMessage={setGameMessage}
                        />
                    </div>
                )}
                {activeTab === 'leaderboard' && (
                    <div className="tab-panel fade-in">
                        <Leaderboard groupID={id} />
                    </div>
                )}
            </div>
            <Game
                gameMessage={gameMessage}
                onChallengeStatusChange={updateChallengeStatus}
                onClose={() => setGameMessage(null)}
            />
            <TabBar activeTab={activeTab} onTabChange={setActiveTab} />
        </div>
    );
}
