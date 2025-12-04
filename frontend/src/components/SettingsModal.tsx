import { useState } from 'react';
import './SettingsModal.css';

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    groupCode: string;
    groupName: string;
}

export default function SettingsModal({ isOpen, onClose, groupCode, groupName }: SettingsModalProps) {
    const [copied, setCopied] = useState(false);

    if (!isOpen) return null;

    const inviteLink = `${window.location.origin}/group/join?code=${groupCode}`;

    const copyInviteLink = () => {
        navigator.clipboard.writeText(inviteLink);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    const copyCode = () => {
        navigator.clipboard.writeText(groupCode);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content" onClick={(e) => e.stopPropagation()}>
                <button className="modal-close" onClick={onClose}>×</button>

                <h2>⚙️ Group Settings</h2>
                <h3 className="group-name-display">{groupName}</h3>

                <div className="settings-section">
                    <h4>📤 Invite Link</h4>
                    <div className="invite-box">
                        <input
                            type="text"
                            value={inviteLink}
                            readOnly
                            className="invite-input"
                        />
                        <button onClick={copyInviteLink} className="copy-btn">
                            {copied ? '✓ Copied!' : '📋 Copy'}
                        </button>
                    </div>
                </div>

                <div className="settings-section">
                    <h4>🔑 Group Code</h4>
                    <div className="code-box">
                        <span className="group-code">{groupCode}</span>
                        <button onClick={copyCode} className="copy-btn">
                            {copied ? '✓ Copied!' : '📋 Copy'}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
}
