// Shared type definitions for the frontend

export interface Message {
    id: string;
    group_id: string;
    user_id: string;
    username?: string;
    avatar?: string;
    content: string;
    created_at: string;
}

export interface Group {
    id: string;
    name: string;
    code: string;
}

export interface LeaderboardEntry {
    user_id: string;
    username: string;
    score: number;
}

export interface Member {
    id: string;
    username: string;
    avatar: string;
}
