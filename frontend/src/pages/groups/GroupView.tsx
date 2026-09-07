import { useEffect, useRef, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { Group, Message } from '../../types';
import Chat from '../../components/chat/Chat';
import Leaderboard from '../../components/leaderboard/Leaderboard';
import { prefetchLeaderboard } from '../../components/leaderboard/leaderboardCache';
import { bustGroupPhotoCache, useGroupPhotoUrl } from './groupPhotoCache';
import Camera from '../../components/camera/Camera';
import Game from '../../components/game/Game';
import SettingsModal from '../../components/settings/SettingsModal';
import TabBar, { type TabType } from '../../components/navigation/TabBar';
import Avatar from '../../components/common/Avatar';
import { useGroupMessages } from '../../hooks/useGroupMessages';
import { useGroupParty } from '../../hooks/useGroupParty';
import PartyButton from './PartyButton';
import Icon from '../../components/ui/Icon';
import FullScreenImage from '../../components/ui/FullScreenImage';
import './GroupView.css';

function isForbiddenError(error: unknown): boolean {
    return (error as { response?: { status?: unknown } })?.response?.status === 403;
}

export default function GroupView() {
    const { id } = useParams();
    const { user } = useAuth();
    const [activeTab, setActiveTab] = useState<TabType>('chat');
    const [groupState, setGroupState] = useState<{
        id: string;
        group: Group | null;
        error: string;
        accessDenied: boolean;
    }>({ id: '', group: null, error: '', accessDenied: false });
    const [gameMessage, setGameMessage] = useState<Message | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [groupPhotoRefreshKey, setGroupPhotoRefreshKey] = useState(0);
    const group = groupState.id === id ? groupState.group : null;
    const groupError = groupState.id === id ? groupState.error : '';
    const groupAccessDenied = groupState.id === id && groupState.accessDenied;
    const {
        messages,
        connectionStatus,
        wsRef,
        error: messagesError,
        updateChallengeStatus,
        updateMessage,
        loadOlder,
        hasMoreOlder,
        loadingOlder,
    } = useGroupMessages(groupError ? undefined : id, user?.id);
    const groupPhotoURL = useGroupPhotoUrl(id ?? '', groupPhotoRefreshKey);
    // Party Time state: fetched per group, refreshed when a persisted system
    // message arrives (a member may have started a party) and after start
    // conflicts. The active flag drives the neon border around the screen.
    const { status: partyStatus, refresh: refreshParty } = useGroupParty(groupError ? undefined : id);
    const lastSeenSystemMessageRef = useRef<{ groupId: string; messageId: string } | null>(null);
    useEffect(() => {
        if (!id) return;
        let latest = '';
        for (let index = messages.length - 1; index >= 0; index -= 1) {
            if (messages[index].kind === 'system' && messages[index].id) {
                latest = messages[index].id;
                break;
            }
        }
        const seen = lastSeenSystemMessageRef.current;
        const firstPassForGroup = seen?.groupId !== id;
        if (!firstPassForGroup && seen?.messageId === latest) return;
        lastSeenSystemMessageRef.current = { groupId: id, messageId: latest };
        // The history loaded on arrival already reflects reality (the party
        // hook fetched authoritative state at the same time); only a system
        // message that newly ARRIVES needs a refresh.
        if (!firstPassForGroup && latest) refreshParty();
    }, [id, messages, refreshParty]);

    useEffect(() => {
        if (!id) return;
        let active = true;
        void api
            .get<Group>('/group/details', { params: { id } })
            .then((response) => {
                if (active) setGroupState({ id, group: response.data, error: '', accessDenied: false });
            })
            .catch((requestError: unknown) => {
                if (active) {
                    setGroupState({
                        id,
                        group: null,
                        error: getAPIErrorMessage(requestError, 'Unable to load group'),
                        accessDenied: isForbiddenError(requestError),
                    });
                }
            });
        return () => {
            active = false;
        };
    }, [id]);

    useEffect(() => {
        if (!id || !user || groupError) return;
        prefetchLeaderboard(user.id, id);
    }, [groupError, id, user]);

    const error = groupError || messagesError;

    if (!id) return <div>Invalid Group ID</div>;
    if (groupError) {
        return (
            <main className="group-view group-error-state">
                <section className="group-error-panel" role="alert" aria-labelledby="group-error-title">
                    <span className="group-error-eyebrow">Group unavailable</span>
                    <h1 id="group-error-title">
                        {groupAccessDenied ? 'You do not have access to this group.' : 'Unable to load this group.'}
                    </h1>
                    {!groupAccessDenied && <p>{groupError}</p>}
                    <Link to="/groups" className="btn btn-secondary">
                        Back to groups
                    </Link>
                </section>
            </main>
        );
    }
    return (
        <>
            <div className="group-view" aria-hidden={gameMessage !== null}>
                <div className="group-header">
                    <div className="header-content">
                        <Link to="/groups" className="back-btn">
                            <Icon name="arrow-left" className="back-arrow-icon" />
                            <span className="visually-hidden">Back to groups</span>
                        </Link>
                        <FullScreenImage
                            src={groupPhotoURL}
                            alt={`${group?.name ?? 'Group'} group photo`}
                            className="header-logo-toggle"
                        >
                            <img src={groupPhotoURL} alt="" className="header-logo" />
                        </FullScreenImage>
                        <div className="group-title-block">
                            <span>Group</span>
                            <h1 className="group-name">{group?.name ?? 'Group'}</h1>
                        </div>
                        {id && (
                            <PartyButton
                                groupId={id}
                                status={partyStatus}
                                onStarted={refreshParty}
                                onRefresh={refreshParty}
                            />
                        )}
                        {user && (
                            <Link to="/profile" className="header-profile-link" aria-label="Open your profile">
                                <Avatar userID={user.id} avatar={user.avatar} username={user.username} />
                            </Link>
                        )}
                        {group && !groupError && (
                            <button
                                className="settings-btn"
                                onClick={() => setSettingsOpen(true)}
                                aria-label="Open group settings"
                            >
                                <img src="/settings_gear_icon.png" alt="" />
                            </button>
                        )}
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
                    groupName={group?.name ?? ''}
                    groupId={id}
                    onGroupPhotoUpdated={() => {
                        bustGroupPhotoCache(id);
                        setGroupPhotoRefreshKey((key) => key + 1);
                    }}
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
                                groupID={id}
                                connectionStatus={connectionStatus}
                                onChallengeMessage={setGameMessage}
                                onMessageUpdated={updateMessage}
                                onLoadOlder={loadOlder}
                                hasMoreOlder={hasMoreOlder}
                                loadingOlder={loadingOlder}
                            />
                        </div>
                    )}
                    {activeTab === 'leaderboard' && (
                        <div className="tab-panel leaderboard-tab-panel fade-in">
                            <Leaderboard key={id} groupID={id} />
                        </div>
                    )}
                </div>
                <TabBar activeTab={activeTab} onTabChange={setActiveTab} />
            </div>
            {partyStatus?.active && <div className="party-border" aria-hidden="true" />}
            <Game
                gameMessage={gameMessage}
                onChallengeStatusChange={updateChallengeStatus}
                onClose={() => setGameMessage(null)}
            />
        </>
    );
}
