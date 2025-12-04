import './TabBar.css';

export type TabType = 'camera' | 'chat' | 'leaderboard';

interface TabBarProps {
    activeTab: TabType;
    onTabChange: (tab: TabType) => void;
    newMessagesCount?: number;
}

export default function TabBar({ activeTab, onTabChange }: TabBarProps) {
    return (
        <div className="tab-bar">
            <button
                className={`tab ${activeTab === 'chat' ? 'active' : ''}`}
                onClick={() => onTabChange('chat')}
            >
                <img src="/chat_bubbl_icon.png" alt="Chat" className="tab-icon" />
                <span>Chat</span>
            </button>
            <button
                className={`tab ${activeTab === 'camera' ? 'active' : ''}`}
                onClick={() => onTabChange('camera')}
            >
                <img src="/camera_icon.png" alt="Camera" className="tab-icon" />
                <span>Camera</span>
            </button>
            <button
                className={`tab ${activeTab === 'leaderboard' ? 'active' : ''}`}
                onClick={() => onTabChange('leaderboard')}
            >
                <img src="/cup_icon.png" alt="Leaderboard" className="tab-icon" />
                <span>Leaderboard</span>
            </button>
        </div>
    );
}
