import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import api, { setAccessToken } from '../api';
import { AuthContext, type AuthContextValue } from './AuthContext';
import type { AuthResponse } from '../types';

export default function AuthProvider({ children }: { children: ReactNode }) {
    const [user, setUser] = useState<AuthContextValue['user']>(null);
    const [loading, setLoading] = useState(true);

    const login = useCallback((response: AuthResponse): void => {
        setAccessToken(response.access_token);
        setUser(response.user);
    }, []);
    const refreshSession = useCallback(async (): Promise<boolean> => {
        try {
            const response = await api.post<AuthResponse>('/auth/refresh');
            login(response.data);
            return true;
        } catch {
            setAccessToken(null);
            setUser(null);
            return false;
        }
    }, [login]);
    const logout = useCallback(async (): Promise<void> => {
        try {
            await api.post('/auth/logout');
        } finally {
            setAccessToken(null);
            setUser(null);
        }
    }, []);
    useEffect(() => {
        void refreshSession().finally(() => setLoading(false));
    }, [refreshSession]);
    const value = useMemo(
        () => ({ user, loading, isAuthenticated: user !== null, login, logout, refresh: refreshSession }),
        [loading, login, logout, refreshSession, user],
    );
    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
