import axios from 'axios';
import { describe, expect, it, vi } from 'vitest';
import api, { getAPIErrorMessage, getAccessToken, setAccessToken } from './api';

describe('api client', () => {
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

    it('refreshes a failed request once and coalesces refresh calls', async () => {
        const post = vi.spyOn(axios, 'post').mockResolvedValue({ data: { access_token: 'fresh' } } as never);
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

    it('returns useful error messages', () => {
        expect(getAPIErrorMessage(new Error('plain'), 'fallback')).toBe('plain');
        expect(getAPIErrorMessage({ response: { data: { error: { message: 'api error' } } } }, 'fallback')).toBe(
            'fallback',
        );
        expect(getAPIErrorMessage(null, 'fallback')).toBe('fallback');
        expect(getAPIErrorMessage('unknown', 'fallback')).toBe('fallback');
    });
});
