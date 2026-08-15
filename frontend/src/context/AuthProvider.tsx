import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import api, { refreshAuthSession, setAccessToken } from '../api';
import { AuthContext, type AuthContextValue } from './AuthContext';
import type { AuthResponse } from '../types';
import { clearCachedSession, readSessionHint, saveSessionHint } from '../utils/pwaSessionCache';

export default function AuthProvider({ children }: { children: ReactNode }) {
    const [user, setUser] = useState<AuthContextValue['user']>(() => readSessionHint());
    const [loading, setLoading] = useState(() => readSessionHint() === null);

    const login = useCallback((response: AuthResponse): void => {
        setAccessToken(response.access_token);
        setUser(response.user);
        saveSessionHint(response.user);
    }, []);
    const refreshSession = useCallback(async (): Promise<boolean> => {
        const response = await refreshAuthSession();
        if (!response) {
            setAccessToken(null);
            setUser(null);
            clearCachedSession();
            return false;
        }
        setUser(response.user);
        saveSessionHint(response.user);
        return true;
    }, []);
    const logout = useCallback(async (): Promise<void> => {
        try {
            await api.post('/auth/logout');
        } finally {
            if (typeof fetch === 'function') {
                await fetch('/oauth2/sign_out', { credentials: 'include', redirect: 'manual' }).catch(() => undefined);
            }
            setAccessToken(null);
            setUser(null);
            clearCachedSession();
        }
    }, []);
    useEffect(() => {
        let active = true;
        queueMicrotask(() => {
            void refreshSession().finally(() => {
                if (active) setLoading(false);
            });
        });
        return () => {
            active = false;
        };
    }, [refreshSession]);
    const value = useMemo(
        () => ({ user, loading, isAuthenticated: user !== null, login, logout, refresh: refreshSession }),
        [loading, login, logout, refreshSession, user],
    );
    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
