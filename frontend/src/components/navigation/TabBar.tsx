import './TabBar.css';

export type TabType = 'camera' | 'chat' | 'leaderboard';

interface TabBarProps {
    activeTab: TabType;
    onTabChange: (tab: TabType) => void;
}

export default function TabBar({ activeTab, onTabChange }: TabBarProps) {
    return (
        <nav className="tab-bar" aria-label="Group sections">
            <button
                aria-pressed={activeTab === 'chat'}
                className={`tab ${activeTab === 'chat' ? 'active' : ''}`}
                onClick={() => onTabChange('chat')}
            >
                <img src="/chat_bubbl_icon.png" alt="" className="tab-icon" />
                <span>Chat</span>
            </button>
            <button
                aria-pressed={activeTab === 'camera'}
                className={`tab ${activeTab === 'camera' ? 'active' : ''}`}
                onClick={() => onTabChange('camera')}
            >
                <img src="/camera_icon.png" alt="" className="tab-icon" />
                <span>Camera</span>
            </button>
            <button
                aria-pressed={activeTab === 'leaderboard'}
                className={`tab ${activeTab === 'leaderboard' ? 'active' : ''}`}
                onClick={() => onTabChange('leaderboard')}
            >
                <img src="/cup_icon.png" alt="" className="tab-icon" />
                <span>Leaderboard</span>
            </button>
        </nav>
    );
}
