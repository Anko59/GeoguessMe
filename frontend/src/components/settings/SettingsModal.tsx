import { useState, useCallback, useEffect, type ChangeEvent } from 'react';
import { Link } from 'react-router-dom';
import api from '../../api';
import type { InviteCreateResponse, InviteListItem, Member } from '../../types';
import Avatar from '../common/Avatar';
import LogoutButton from '../navigation/LogoutButton';
import Icon from '../ui/Icon';
import './SettingsModal.css';

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    groupName: string;
    groupId: string;
    onGroupPhotoUpdated?: () => void;
}

function formatDate(iso: string): string {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return iso;
    return date.toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' });
}

export default function SettingsModal({
    isOpen,
    onClose,
    groupName,
    groupId,
    onGroupPhotoUpdated,
}: SettingsModalProps) {
    const [copied, setCopied] = useState(false);
    const [membersExpanded, setMembersExpanded] = useState(false);
    const [members, setMembers] = useState<Member[]>([]);
    const [loadingMembers, setLoadingMembers] = useState(false);
    const [memberError, setMemberError] = useState('');
    const [notificationsEnabled, setNotificationsEnabled] = useState(true);
    const [loadingNotifications, setLoadingNotifications] = useState(false);
    const [notificationError, setNotificationError] = useState('');
    const [uploadingPhoto, setUploadingPhoto] = useState(false);
    const [photoError, setPhotoError] = useState('');
    const [invites, setInvites] = useState<InviteListItem[]>([]);
    // Tracks the group whose invites have finished loading so the loading
    // indicator can be derived in render instead of a setState-in-effect
    // round trip (react-hooks lint).
    const [invitesLoadedGroupId, setInvitesLoadedGroupId] = useState<string | null>(null);
    const [inviteListError, setInviteListError] = useState('');
    const [createdInvite, setCreatedInvite] = useState<InviteCreateResponse | null>(null);
    const [creatingInvite, setCreatingInvite] = useState(false);
    const [inviteError, setInviteError] = useState('');

    useEffect(() => {
        if (!isOpen) return;
        let active = true;
        queueMicrotask(() => {
            if (active) {
                setLoadingNotifications(true);
                setNotificationError('');
            }
        });
        void api
            .get('/group/notifications', { params: { group_id: groupId } })
            .then((response) => {
                if (active) setNotificationsEnabled(response.data?.enabled !== false);
            })
            .catch(() => {
                if (active) setNotificationError('Unable to load notification settings. Try again.');
            })
            .finally(() => {
                if (active) setLoadingNotifications(false);
            });
        return () => {
            active = false;
        };
    }, [groupId, isOpen]);

    useEffect(() => {
        let active = true;
        // A raw invite is intentionally ephemeral. Clear it whenever the modal
        // opens, closes, or moves to another group so it cannot reappear later.
        queueMicrotask(() => {
            if (active) {
                setCreatedInvite(null);
                setCopied(false);
                setInviteError('');
            }
        });
        if (!isOpen) {
            return () => {
                active = false;
            };
        }
        void api
            .get<InviteListItem[]>('/group/invites', { params: { group_id: groupId } })
            .then((response) => {
                if (active) {
                    setInvites(response.data || []);
                    setInviteListError('');
                    setInvitesLoadedGroupId(groupId);
                }
            })
            .catch(() => {
                if (active) {
                    setInviteListError('Unable to load invites. Try again.');
                    setInvitesLoadedGroupId(groupId);
                }
            });
        return () => {
            active = false;
        };
    }, [groupId, isOpen]);

    const refreshInvites = useCallback(async () => {
        try {
            const res = await api.get<InviteListItem[]>('/group/invites', { params: { group_id: groupId } });
            setInvites(res.data || []);
            setInviteListError('');
        } catch {
            setInviteListError('Unable to load invites. Try again.');
        }
    }, [groupId]);

    const fetchMembers = useCallback(async () => {
        setLoadingMembers(true);
        setMemberError('');
        try {
            const res = await api.get(`/group/members?id=${groupId}`);
            setMembers(res.data || []);
        } catch {
            setMemberError('Unable to load members. Try again.');
        } finally {
            setLoadingMembers(false);
        }
    }, [groupId]);

    const toggleMembers = () => {
        const expanding = !membersExpanded;
        setMembersExpanded(expanding);
        if (expanding && members.length === 0) void fetchMembers();
    };

    if (!isOpen) return null;

    const createInvite = () => {
        if (creatingInvite) return;
        setCreatingInvite(true);
        setInviteError('');
        void api
            .post<InviteCreateResponse>('/group/invites', { group_id: groupId })
            .then((response) => {
                setCreatedInvite(response.data);
                void refreshInvites();
            })
            .catch(() => setInviteError('Unable to create an invite. Try again.'))
            .finally(() => setCreatingInvite(false));
    };

    const revokeInvite = (inviteID: string) => {
        void api
            .delete(`/group/invites/${inviteID}`)
            .then(() => void refreshInvites())
            .catch(() => setInviteError('Unable to revoke that invite. Try again.'));
    };

    const inviteLink = createdInvite ? `${window.location.origin}${createdInvite.invite_url}` : '';
    const copyInvite = () => {
        if (!createdInvite) return;
        if (!navigator.clipboard) {
            setInviteError('Clipboard access is unavailable. Select and copy the link manually.');
            return;
        }
        setInviteError('');
        void navigator.clipboard
            .writeText(inviteLink)
            .then(() => {
                setCopied(true);
                setTimeout(() => setCopied(false), 2000);
            })
            .catch(() => setInviteError('Unable to copy the invite. Select and copy the link manually.'));
    };

    const toggleNotifications = () => {
        if (loadingNotifications) return;
        const enabled = !notificationsEnabled;
        setLoadingNotifications(true);
        setNotificationError('');
        void api
            .put(`/group/notifications?group_id=${encodeURIComponent(groupId)}`, { enabled })
            .then(() => setNotificationsEnabled(enabled))
            .catch(() => setNotificationError('Unable to save notification settings. Try again.'))
            .finally(() => setLoadingNotifications(false));
    };

    const uploadPhoto = (event: ChangeEvent<HTMLInputElement>) => {
        const photo = event.target.files?.[0];
        event.target.value = '';
        if (!photo) return;
        setUploadingPhoto(true);
        setPhotoError('');
        const body = new FormData();
        body.append('group_id', groupId);
        body.append('photo', photo);
        void api
            .post('/group/photo', body)
            .then(() => onGroupPhotoUpdated?.())
            .catch(() => setPhotoError('Unable to update the group photo. Try again.'))
            .finally(() => setUploadingPhoto(false));
    };

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div
                className="modal-content"
                role="dialog"
                aria-modal="true"
                aria-labelledby="group-settings-title"
                onClick={(e) => e.stopPropagation()}
            >
                <button className="modal-close" onClick={onClose} aria-label="Close settings">
                    <Icon name="close" />
                </button>

                <h2 className="modal-title">
                    <img src="/settings_gear_icon.png" alt="" className="modal-icon" />
                    <span id="group-settings-title">Group Settings</span>
                </h2>
                <h3 className="group-name-display">{groupName}</h3>

                <Link to="/settings" className="personal-settings-link" onClick={onClose}>
                    Personal settings
                </Link>

                <div className="settings-section">
                    <h4 className="section-title">
                        <Icon name="image" />
                        Group Photo
                    </h4>
                    <label className="group-photo-upload">
                        <span>{uploadingPhoto ? 'Uploading...' : 'Choose a group photo'}</span>
                        <input type="file" accept="image/*" onChange={uploadPhoto} disabled={uploadingPhoto} />
                    </label>
                    {photoError && (
                        <div className="settings-error" role="alert">
                            {photoError}
                        </div>
                    )}
                </div>

                <div className="settings-section">
                    <h4 className="section-title">
                        <Icon name="bell" />
                        Notifications
                    </h4>
                    <label className="notification-toggle">
                        <span>Group notifications</span>
                        <input
                            type="checkbox"
                            aria-label="Group notifications"
                            checked={notificationsEnabled}
                            onChange={toggleNotifications}
                            disabled={loadingNotifications}
                        />
                    </label>
                    {notificationError && (
                        <div className="settings-error" role="alert">
                            {notificationError}
                        </div>
                    )}
                </div>

                <div className="settings-section">
                    <h4 className="section-title">
                        <img src="/invite_link_icon.png" alt="" className="section-icon" />
                        Invites
                    </h4>
                    <button
                        type="button"
                        className="btn btn-secondary"
                        onClick={createInvite}
                        disabled={creatingInvite}
                        data-testid="create-invite-btn"
                    >
                        {creatingInvite ? 'Creating…' : 'Create invite link'}
                    </button>
                    {inviteError && (
                        <div className="settings-error" role="alert">
                            {inviteError}
                        </div>
                    )}
                    {createdInvite && (
                        <div className="invite-box">
                            <p className="invite-once-notice">
                                This invite link is shown only once. Copy and share it.
                            </p>
                            <input
                                type="text"
                                value={inviteLink}
                                readOnly
                                className="invite-input"
                                aria-label="Invite link"
                                data-testid="invite-url"
                            />
                            <button onClick={copyInvite} className="copy-btn">
                                {copied ? (
                                    <>
                                        <img src="/check.png" alt="" className="copy-icon" />
                                        Copied!
                                    </>
                                ) : (
                                    <>
                                        <img src="/copy_text_icon.png" alt="" className="copy-icon" />
                                        Copy
                                    </>
                                )}
                            </button>
                        </div>
                    )}
                    {invitesLoadedGroupId !== groupId ? (
                        <div className="invites-loading">Loading invites...</div>
                    ) : inviteListError ? (
                        <div className="invites-empty" role="alert">
                            {inviteListError}
                        </div>
                    ) : invites.length === 0 ? (
                        <div className="invites-empty">No active invites yet.</div>
                    ) : (
                        <ul className="invites-list">
                            {invites.map((invite) => (
                                <li key={invite.id} className="invite-item">
                                    <span className="invite-meta">
                                        <span
                                            className={
                                                invite.revoked
                                                    ? 'invite-state-label revoked'
                                                    : 'invite-state-label active'
                                            }
                                        >
                                            {invite.revoked ? 'Revoked' : 'Active'}
                                        </span>
                                        <span className="invite-dates">
                                            {invite.id} · by {invite.creator_user_id} · created{' '}
                                            {formatDate(invite.created_at)}· expires {formatDate(invite.expires_at)}
                                        </span>
                                    </span>
                                    <button
                                        type="button"
                                        className="copy-btn"
                                        onClick={() => revokeInvite(invite.id)}
                                        disabled={Boolean(invite.revoked)}
                                    >
                                        Revoke
                                    </button>
                                </li>
                            ))}
                        </ul>
                    )}
                </div>

                <div className="settings-section">
                    <button
                        type="button"
                        className="section-title members-toggle"
                        aria-expanded={membersExpanded}
                        onClick={toggleMembers}
                    >
                        <Icon name="users" />
                        Group Members
                        <span className="toggle-icon" aria-hidden="true">
                            ⌄
                        </span>
                    </button>
                    {membersExpanded && (
                        <div className="members-list">
                            {loadingMembers ? (
                                <div className="members-loading">Loading...</div>
                            ) : memberError ? (
                                <div className="members-empty" role="alert">
                                    {memberError}
                                </div>
                            ) : members.length > 0 ? (
                                members.map((member) => (
                                    <div key={member.id} className="member-item">
                                        <Avatar
                                            userID={member.id}
                                            avatar={member.avatar}
                                            username={member.username}
                                            className="member-avatar"
                                        />
                                        <span className="member-name">{member.username}</span>
                                    </div>
                                ))
                            ) : (
                                <div className="members-empty">No members found</div>
                            )}
                        </div>
                    )}
                </div>

                <div className="settings-section logout-section">
                    <LogoutButton />
                </div>
            </div>
        </div>
    );
}
