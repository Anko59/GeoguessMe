export interface User {
    id: string;
    username: string;
    email: string;
    email_verified_at?: string | null;
    avatar: string;
}

export interface ProgressionRank {
    level: number;
    name: string;
    min_points: number;
    next_points?: number;
    points_in_rank: number;
    points_to_next: number;
    progress_percent: number;
    trophy_key: string;
    next_rank?: ProgressionRank;
}

export interface GlobalRank {
    rank: number;
    total_players: number;
}

export interface Profile extends User {
    total_points: number;
    guess_count: number;
    average_score: number;
    elo: number;
    rank: ProgressionRank;
    global_rank: GlobalRank;
    global_average_rank: GlobalRank;
    global_elo_rank: GlobalRank;
}

/** Another player's profile: identity and progression, never email. */
export interface PublicProfile {
    id: string;
    username: string;
    avatar: string;
    total_points: number;
    guess_count: number;
    average_score: number;
    elo: number;
    rank: ProgressionRank;
    global_rank: GlobalRank;
    global_average_rank: GlobalRank;
    global_elo_rank: GlobalRank;
}

export interface AuthResponse {
    access_token: string;
    expires_in: number;
    user: User;
}

export interface Message {
    id: string;
    group_id: string;
    user_id: string;
    username?: string;
    avatar?: string;
    kind: 'text' | 'challenge' | 'media' | 'system';
    photo_id?: string;
    media_id?: string;
    media_type?: 'image/jpeg' | 'image/png' | 'video/mp4' | 'video/webm';
    reply_to_id?: string;
    error_code?: string;
    content: string;
    created_at: string;
    challenge_status?: 'available' | 'accepted' | 'guessed' | 'results' | 'expired';
    challenge_resolved?: boolean;
    challenge_expires_at?: string;
    challenge_ttl_seconds?: number;
    reactions?: Reaction[];
    reaction_update?: {
        user_id: string;
        reaction: string;
        active: boolean;
    };
}

export interface Reaction {
    reaction: string;
    count: number;
    reacted: boolean;
    usernames?: string[];
}

export interface Group {
    id: string;
    name: string;
    code: string;
    created_at?: string;
}

export interface LeaderboardEntry {
    user_id: string;
    username: string;
    avatar: string;
    score: number;
    guess_count: number;
    average_score: number;
    total_points: number;
    elo: number;
    rank: ProgressionRank;
}

export type LeaderboardPeriod = 'week' | 'month' | 'all';

export type LeaderboardMetric = 'total' | 'average' | 'elo';

export interface Member {
    id: string;
    username: string;
    avatar: string;
}

export interface ChallengeNotice {
    id: string;
    photo_id: string;
    uploader_id?: string;
    expires_at: string;
}

export interface ChallengeAcceptance {
    photo_id: string;
    media_url: string;
    media_type: string;
    accepted_at: string;
    view_expires_at: string;
    guess_after: string;
    challenge_expires_at: string;
    server_time: string;
}

export interface ChallengeMediaDelivered {
    view_expires_at: string;
    guess_after: string;
    server_time: string;
}

export interface GuessResult {
    guess_id: string;
    photo_id: string;
    score: number;
    distance: number;
    created_at: string;
    duplicate: boolean;
}

export interface ChallengeGuess {
    id: string;
    photo_id: string;
    user_id: string;
    username: string;
    avatar: string;
    /** Omitted for other players' guesses while a hidden-location challenge is
     *  still hidden: only the viewer's own guessed point is returned. */
    lat?: number;
    long?: number;
    score: number;
    /** Omitted alongside the coordinates while the location is hidden. */
    distance?: number;
    created_at: string;
}

export interface ChallengeResults {
    photo_id: string;
    group_id: string;
    actual_lat?: number;
    actual_long?: number;
    location_hidden?: boolean;
    location_reveals_at?: string;
    guesses: ChallengeGuess[];
    media_available: boolean;
    media_url?: string;
    media_type?: string;
    server_time: string;
}

export interface APIErrorBody {
    error?: { code: string; message: string };
    code?: string;
    message?: string;
}

export interface MessagesPage {
    items: Message[];
    next_cursor: string;
}
