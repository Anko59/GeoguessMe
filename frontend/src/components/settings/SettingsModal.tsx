import { useState, useEffect, useCallback } from 'react';
import { Link } from 'react-router-dom';
import api from '../../api';
import type { Member } from '../../types';
import LogoutButton from '../navigation/LogoutButton';
import './SettingsModal.css';

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    groupCode: string;
    groupName: string;
    groupId: string;
}

export default function SettingsModal({ isOpen, onClose, groupCode, groupName, groupId }: SettingsModalProps) {
    const [copiedItem, setCopiedItem] = useState<'link' | 'code' | null>(null);
    const [membersExpanded, setMembersExpanded] = useState(false);
    const [members, setMembers] = useState<Member[]>([]);
    const [loadingMembers, setLoadingMembers] = useState(false);
    const [memberError, setMemberError] = useState('');

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

    useEffect(() => {
        if (isOpen && membersExpanded && members.length === 0) void fetchMembers();
    }, [fetchMembers, isOpen, members.length, membersExpanded]);

    if (!isOpen) return null;

    const inviteLink = `${window.location.origin}/group/join?code=${groupCode}`;

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
                    ×
                </button>

                <h2 className="modal-title">
                    <img src="/settings_gear_icon.png" alt="" className="modal-icon" />
                    <span id="group-settings-title">Group Settings</span>
                </h2>
                <h3 className="group-name-display">{groupName}</h3>

                <Link to="/settings" className="personal-settings-link" onClick={onClose}>
                    Open personal settings
                </Link>

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
                    <h4 className="section-title members-toggle" onClick={() => setMembersExpanded(!membersExpanded)}>
                        <span className="toggle-icon">{membersExpanded ? '▼' : '▶'}</span>
                        Group Members
                    </h4>
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
                                        <img
                                            src={`/avatars/${member.avatar || 'avatar.png'}`}
                                            alt=""
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
