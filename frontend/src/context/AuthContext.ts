import { createContext, useContext } from 'react';
import type { AuthResponse, User } from '../types';

export interface AuthContextValue {
    user: User | null;
    loading: boolean;
    isAuthenticated: boolean;
    login: (response: AuthResponse) => void;
    logout: () => Promise<void>;
    refresh: () => Promise<boolean>;
}

export const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export function useAuth(): AuthContextValue {
    const value = useContext(AuthContext);
    if (!value) throw new Error('useAuth must be used inside AuthProvider');
    return value;
}
