import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import api, { refreshAuthSession, setAccessToken } from '../api';
import { AuthContext, type AuthContextValue } from './AuthContext';
import type { AuthResponse } from '../types';
import { clearCachedSession, readSessionHint, saveSessionHint } from '../utils/pwaSessionCache';
import { backendURL } from '../platform/endpoints';

const LOGOUT_KEY = 'geoguessme:logout:v1';

export default function AuthProvider({ children }: { children: ReactNode }) {
    const [user, setUser] = useState<AuthContextValue['user']>(() => readSessionHint());
    const [loading, setLoading] = useState(() => readSessionHint() === null);
    // Invalidate refreshes started before a login or logout; a late response must not restore a signed-out tab.
    const sessionVersion = useRef(0);
    const clearSession = useCallback(() => {
        sessionVersion.current += 1;
        setAccessToken(null);
        setUser(null);
        clearCachedSession();
        setLoading(false);
    }, []);

    const login = useCallback((response: AuthResponse): void => {
        sessionVersion.current += 1;
        setAccessToken(response.access_token);
        setUser(response.user);
        saveSessionHint(response.user);
    }, []);
    const refreshSession = useCallback(async (): Promise<boolean> => {
        const version = sessionVersion.current;
        const response = await refreshAuthSession();
        if (version !== sessionVersion.current) return false;
        if (!response) {
            clearSession();
            return false;
        }
        setUser(response.user);
        saveSessionHint(response.user);
        return true;
    }, [clearSession]);
    const logout = useCallback(async (): Promise<void> => {
        try {
            await api.post('/auth/logout');
        } finally {
            clearSession();
            // A dedicated event also reaches tabs without a cached session hint.
            try {
                window.localStorage.setItem(LOGOUT_KEY, `${Date.now()}:${Math.random()}`);
            } catch {
                // Restricted storage cannot synchronize tabs; local sign-out still succeeds.
            }
            if (typeof fetch === 'function') {
                await fetch(backendURL('/oauth2/sign_out'), {
                    credentials: 'include',
                    redirect: 'manual',
                }).catch(() => undefined);
            }
        }
    }, [clearSession]);
    useEffect(() => {
        const onStorage = (event: StorageEvent) => {
            if (event.key === LOGOUT_KEY && event.newValue !== null) {
                clearSession();
            }
        };
        window.addEventListener('storage', onStorage);
        return () => window.removeEventListener('storage', onStorage);
    }, [clearSession]);
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
