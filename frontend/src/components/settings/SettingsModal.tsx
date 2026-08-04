import { useState, useCallback, useEffect, type ChangeEvent } from 'react';
import { Link } from 'react-router-dom';
import api from '../../api';
import type { Member } from '../../types';
import Avatar from '../common/Avatar';
import LogoutButton from '../navigation/LogoutButton';
import Icon from '../ui/Icon';
import './SettingsModal.css';

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    groupCode: string;
    groupName: string;
    groupId: string;
    currentUserName: string;
    onGroupPhotoUpdated?: () => void;
}

export default function SettingsModal({
    isOpen,
    onClose,
    groupCode,
    groupName,
    groupId,
    currentUserName,
    onGroupPhotoUpdated,
}: SettingsModalProps) {
    const [copiedItem, setCopiedItem] = useState<'link' | 'code' | null>(null);
    const [membersExpanded, setMembersExpanded] = useState(false);
    const [members, setMembers] = useState<Member[]>([]);
    const [loadingMembers, setLoadingMembers] = useState(false);
    const [memberError, setMemberError] = useState('');
    const [notificationsEnabled, setNotificationsEnabled] = useState(true);
    const [loadingNotifications, setLoadingNotifications] = useState(false);
    const [notificationError, setNotificationError] = useState('');
    const [uploadingPhoto, setUploadingPhoto] = useState(false);
    const [photoError, setPhotoError] = useState('');

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

    const inviteLink = `${window.location.origin}/invite/${groupCode}?from=${encodeURIComponent(currentUserName)}`;

    const copyInviteLink = () => {
        navigator.clipboard.writeText(inviteLink);
        setCopiedItem('link');
        setTimeout(() => setCopiedItem(null), 2000);
    };

    const copyCode = () => {
        navigator.clipboard.writeText(groupCode);
        setCopiedItem('code');
        setTimeout(() => setCopiedItem(null), 2000);
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
                        Invite Link
                    </h4>
                    <div className="invite-box">
                        <input
                            type="text"
                            value={inviteLink}
                            readOnly
                            className="invite-input"
                            aria-label="Invite link"
                        />
                        <button onClick={copyInviteLink} className="copy-btn">
                            {copiedItem === 'link' ? (
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
                </div>

                <div className="settings-section">
                    <h4 className="section-title">
                        <img src="/group_code_icon.png" alt="" className="section-icon" />
                        Group Code
                    </h4>
                    <div className="code-box">
                        <span className="group-code">{groupCode}</span>
                        <button onClick={copyCode} className="copy-btn">
                            {copiedItem === 'code' ? (
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
