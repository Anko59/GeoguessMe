import axios from 'axios';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import AuthProvider from './AuthProvider';
import { useAuth } from './AuthContext';
import { getAccessToken, setAccessToken } from '../api';
import Home from '../pages/home/Home';

function SessionState() {
    const auth = useAuth();
    return <output>{auth.loading ? 'Restoring session' : auth.isAuthenticated ? 'Signed in' : 'Signed out'}</output>;
}

afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    setAccessToken(null);
});

it('keeps the landing screen after a native refresh POST returns bundled HTML with status 200', async () => {
    localStorage.clear();
    setAccessToken('stale');
    const refresh = vi.spyOn(axios, 'post').mockResolvedValue({
        status: 200,
        headers: { 'content-type': 'text/html' },
        data: '<!doctype html><html><div id="root"></div></html>',
    });
    render(
        <MemoryRouter initialEntries={['/']}>
            <AuthProvider>
                <SessionState />
                <Routes>
                    <Route path="/" element={<Home />} />
                    <Route path="/feed" element={<p>Feed must not open</p>} />
                </Routes>
            </AuthProvider>
        </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText('Signed out')).toBeInTheDocument());
    expect(screen.getByRole('link', { name: 'Already Playing? Login' })).toBeInTheDocument();
    expect(screen.queryByText('Feed must not open')).not.toBeInTheDocument();
    expect(getAccessToken()).toBeNull();
    expect(localStorage.getItem('geoguessme:pwa-session:v1')).toBeNull();
    expect(refresh).toHaveBeenCalledWith('/api/v1/auth/refresh', undefined, { withCredentials: true });
});
