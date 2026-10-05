import axios, { AxiosError } from 'axios';
import { describe, expect, it, vi } from 'vitest';
import api, {
    getAPIErrorCode,
    getAPIErrorCodeAsync,
    getAPIErrorMessage,
    getAccessToken,
    publicFeedAPI,
    moderationAPI,
    refreshAuthSession,
    userBlocksAPI,
    setAccessToken,
    exchangeOIDCSession,
} from './api';

const validSession = { access_token: 'fresh', user: { id: 'player-1', username: 'Explorer' }, expires_in: 900 };

describe('api client', () => {
    it('uses typed block endpoints, preserves 204 and broadcasts only successful changes', async () => {
        const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { items: [] } });
        const post = vi.spyOn(api, 'post').mockResolvedValue({ status: 204 });
        const remove = vi.spyOn(api, 'delete').mockResolvedValue({ status: 204 });
        const listener = vi.fn();
        window.addEventListener('geoguessme:block-visibility', listener);
        const signal = new AbortController().signal;
        await expect(userBlocksAPI.list(signal)).resolves.toEqual({ items: [] });
        await expect(userBlocksAPI.block('id/space here', signal)).resolves.toBeUndefined();
        await expect(userBlocksAPI.unblock('id/space here', signal)).resolves.toBeUndefined();
        expect(get).toHaveBeenCalledWith('/users/blocks', { signal });
        expect(post).toHaveBeenCalledWith('/users/id%2Fspace%20here/block', undefined, { signal });
        expect(remove).toHaveBeenCalledWith('/users/id%2Fspace%20here/block', { signal });
        expect(listener).toHaveBeenCalledTimes(2);
        expect(localStorage.getItem('geoguessme:block-visibility:v1')).not.toContain('id/space');
        post.mockRejectedValueOnce(new Error('Denied'));
        await expect(userBlocksAPI.block('id')).rejects.toThrow('Denied');
        expect(listener).toHaveBeenCalledTimes(2);
        window.removeEventListener('geoguessme:block-visibility', listener);
        get.mockRestore();
        post.mockRestore();
        remove.mockRestore();
    });
    it('sends typed content reports on the authenticated client', async () => {
        const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { id: 'notice-1' } });
        const controller = new AbortController();
        await expect(
            moderationAPI.report('messages', 'id/with slash', { reason: 'other', details: '' }, controller.signal),
        ).resolves.toEqual({ id: 'notice-1' });
        expect(post).toHaveBeenCalledWith(
            '/messages/id%2Fwith%20slash/report',
            { reason: 'other', details: '' },
            { signal: controller.signal },
        );
        post.mockRestore();
    });
    it('stores tokens and exposes secure defaults', () => {
        setAccessToken('token');
        expect(getAccessToken()).toBe('token');
        setAccessToken(null);
        expect(getAccessToken()).toBeNull();
        expect(api.defaults.baseURL).toBe('/api/v1');
        expect(api.defaults.withCredentials).toBe(true);
    });

    it('handles request headers and external URLs', async () => {
        setAccessToken('token');
        const request = await api.interceptors.request.handlers![0]!.fulfilled!({
            url: '/groups',
            headers: {},
        } as never);
        expect(request.headers.Authorization).toBe('Bearer token');
        const external = await api.interceptors.request.handlers![0]!.fulfilled!({
            url: 'https://example.test/data',
            headers: {},
        } as never);
        expect(external.withCredentials).toBe(false);
    });

    it('keeps protected auth routes authenticated while leaving public auth routes open', async () => {
        setAccessToken('token');
        const request = await api.interceptors.request.handlers![0]!.fulfilled!({
            url: '/auth/profile/avatar',
            headers: {},
        } as never);
        expect(request.headers.Authorization).toBe('Bearer token');

        const publicRequest = await api.interceptors.request.handlers![0]!.fulfilled!({
            url: '/auth/login',
            headers: {},
        } as never);
        expect(publicRequest.headers.Authorization).toBeUndefined();
    });

    it('refreshes a failed request once and coalesces refresh calls', async () => {
        const post = vi.spyOn(axios, 'post').mockResolvedValue({ data: validSession } as never);
        const adapter = vi.fn().mockResolvedValue({ status: 200, data: { ok: true }, headers: {}, config: {} });
        api.defaults.adapter = adapter;
        setAccessToken(null);
        const errorHandler = api.interceptors.response.handlers![0]!.rejected!;
        const request = { url: '/groups', headers: {} } as never;
        const result = await errorHandler({ response: { status: 401 }, config: request });
        expect(result.data.ok).toBe(true);
        expect(post).toHaveBeenCalledWith('/api/v1/auth/refresh', undefined, { withCredentials: true });
        expect(getAccessToken()).toBe('fresh');
        post.mockRestore();
    });

    it('restores a memory-only token before sending a protected startup request', async () => {
        const post = vi.spyOn(axios, 'post').mockResolvedValue({ data: validSession } as never);
        setAccessToken(null);

        const request = await api.interceptors.request.handlers![0]!.fulfilled!({
            url: '/user/groups',
            headers: {},
        } as never);

        expect(request.headers.Authorization).toBe('Bearer fresh');
        expect(post).toHaveBeenCalledWith('/api/v1/auth/refresh', undefined, { withCredentials: true });
        post.mockRestore();
    });

    it('does not restore a token from a refresh completed after logout', async () => {
        let finishRefresh!: (response: { data: typeof validSession }) => void;
        const post = vi
            .spyOn(axios, 'post')
            .mockReturnValue(new Promise((resolve) => (finishRefresh = resolve)) as never);
        setAccessToken(null);
        const refresh = refreshAuthSession();
        setAccessToken(null);
        finishRefresh({ data: { ...validSession, access_token: 'stale' } });
        await expect(refresh).resolves.toBeNull();
        expect(getAccessToken()).toBeNull();
        post.mockRestore();
    });

    it.each([
        '<!doctype html><html><div id="root"></div></html>',
        { access_token: 'fresh' },
        { access_token: '', user: { id: 'player-1', username: 'Explorer' } },
        { access_token: 'fresh', user: null },
    ])('rejects an invalid restored session instead of authenticating with an undefined user: %j', async (data) => {
        const post = vi.spyOn(axios, 'post').mockResolvedValue({ status: 200, data } as never);
        setAccessToken('previous');
        try {
            await expect(refreshAuthSession()).resolves.toBeNull();
            expect(getAccessToken()).toBeNull();
        } finally {
            post.mockRestore();
        }
    });

    it('rejects HTML from an OIDC exchange without replacing the current token', async () => {
        const post = vi.spyOn(axios, 'post').mockResolvedValue({ data: '<!doctype html>bundled SPA' } as never);
        setAccessToken('previous');
        try {
            await expect(exchangeOIDCSession()).rejects.toThrow('invalid sign-in response');
            expect(getAccessToken()).toBe('previous');
        } finally {
            post.mockRestore();
        }
    });

    it('rejects an HTML API response before a feed component can consume it', () => {
        const handle = api.interceptors.response.handlers![0]!.fulfilled!;
        expect(() =>
            handle({
                data: '<!doctype html>bundled SPA',
                headers: { 'content-type': 'text/html; charset=utf-8' },
                config: { url: '/feed/challenges' },
            } as never),
        ).toThrow('web page instead of application data');
        const media = { data: new Blob(['image']), headers: { 'content-type': 'image/jpeg' }, config: {} };
        expect(handle(media as never)).toBe(media);
    });

    it('returns useful error messages', () => {
        expect(getAPIErrorCode({ response: { data: { error: { code: 'username_required' } } } })).toBe(
            'username_required',
        );
        expect(getAPIErrorCode(null)).toBeUndefined();
        expect(getAPIErrorMessage(new Error('plain'), 'fallback')).toBe('plain');
        expect(getAPIErrorMessage(new AxiosError('request failed'), 'fallback')).toBe('fallback');
        expect(getAPIErrorMessage({ response: { data: { error: { message: 'api error' } } } }, 'fallback')).toBe(
            'api error',
        );
        expect(
            getAPIErrorMessage(
                { response: { data: { code: 'invalid_upload', message: 'upload failed' } } },
                'fallback',
            ),
        ).toBe('upload failed');
        expect(getAPIErrorMessage(null, 'fallback')).toBe('fallback');
        expect(getAPIErrorMessage('unknown', 'fallback')).toBe('fallback');
    });

    it('decodes a JSON API error returned as a blob by a binary request', async () => {
        const errorBody = new Blob([JSON.stringify({ error: { code: 'media_removed' } })], {
            type: 'application/json',
        });

        await expect(getAPIErrorCodeAsync({ response: { status: 410, data: errorBody } })).resolves.toBe(
            'media_removed',
        );
        await expect(
            getAPIErrorCodeAsync({ response: { status: 503, data: new Blob(['unavailable']) } }),
        ).resolves.toBe(undefined);
    });

    it('requests a profile-scoped feed leaderboard with a stable cursor', async () => {
        const previousAdapter = api.defaults.adapter;
        const adapter = vi.fn().mockResolvedValue({
            status: 200,
            statusText: 'OK',
            headers: {},
            config: {},
            data: { items: [], next_cursor: '' },
        });
        api.defaults.adapter = adapter;
        try {
            await publicFeedAPI.profileLeaderboard('profile/id', 'cursor-token', new AbortController().signal);
        } finally {
            api.defaults.adapter = previousAdapter;
        }
        expect(adapter).toHaveBeenCalledWith(
            expect.objectContaining({
                url: '/feed/leaderboard/profile%2Fid',
                params: { cursor: 'cursor-token' },
            }),
        );
    });
});
