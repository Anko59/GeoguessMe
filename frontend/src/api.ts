import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios';
import type {
    APIErrorBody,
    AuthResponse,
    PublicChallenge,
    PublicFeedPage,
    PublicCommentsPage,
    PublicComment,
    PublicGuessResult,
} from './types';

let accessToken: string | null = null;
let refreshPromise: Promise<AuthResponse | null> | null = null;
let oidcExchangePromise: Promise<AuthResponse> | null = null;

export const setAccessToken = (token: string | null): void => {
    accessToken = token;
};

export const getAccessToken = (): string | null => accessToken;

const api = axios.create({
    baseURL: '/api/v1',
    withCredentials: true,
});

const publicAuthPaths = new Set([
    '/auth/signup',
    '/auth/login',
    '/auth/oidc/config',
    '/auth/oidc/session',
    '/auth/verify',
    '/auth/password/forgot',
    '/auth/password/reset',
]);

function isPublicAuthRequest(url: string | undefined): boolean {
    if (!url) return false;
    const path = url.split('?', 1)[0];
    return publicAuthPaths.has(path);
}

export const refreshAuthSession = async (): Promise<AuthResponse | null> => {
    if (!refreshPromise) {
        refreshPromise = axios
            .post<AuthResponse>('/api/v1/auth/refresh', undefined, { withCredentials: true })
            .then((response) => {
                setAccessToken(response.data.access_token);
                return response.data;
            })
            .catch(() => {
                setAccessToken(null);
                return null;
            })
            .finally(() => {
                refreshPromise = null;
            });
    }
    return refreshPromise;
};

export const exchangeOIDCSession = async (username?: string): Promise<AuthResponse> => {
    if (!oidcExchangePromise) {
        oidcExchangePromise = axios
            .post<AuthResponse>('/api/v1/auth/oidc/session', username ? { username } : undefined, {
                withCredentials: true,
            })
            .then((response) => {
                setAccessToken(response.data.access_token);
                return response.data;
            })
            .finally(() => {
                oidcExchangePromise = null;
            });
    }
    return oidcExchangePromise;
};

export const getAPIErrorCode = (error: unknown): string | undefined => {
    if (typeof error !== 'object' || error === null) return undefined;
    const data = (error as { response?: { data?: APIErrorBody } }).response?.data;
    return data?.error?.code;
};

const refreshAccessToken = async (): Promise<string | null> => {
    const response = await refreshAuthSession();
    return response?.access_token ?? null;
};

api.interceptors.request.use(async (config: InternalAxiosRequestConfig) => {
    const isSameOriginAPI = !config.url?.startsWith('http');
    if (isSameOriginAPI && !isPublicAuthRequest(config.url)) {
        // Cached PWA screens can render before the memory-only access token is
        // restored. Hold protected requests behind the existing single-flight
        // refresh so their first attempt has the token and error handling sees
        // the response it actually requested.
        const token = accessToken ?? (await refreshAccessToken());
        if (token) config.headers.Authorization = `Bearer ${token}`;
    }
    if (!isSameOriginAPI) config.withCredentials = false;
    return config;
});

api.interceptors.response.use(
    (response) => response,
    async (error: AxiosError<APIErrorBody>) => {
        const request = error.config as (InternalAxiosRequestConfig & { _retried?: boolean }) | undefined;
        if (error.response?.status === 401 && request && !request._retried && !request.url?.includes('/auth/refresh')) {
            request._retried = true;
            const token = await refreshAccessToken();
            if (token) {
                request.headers.Authorization = `Bearer ${token}`;
                return api(request);
            }
        }
        return Promise.reject(error);
    },
);

export const getAPIErrorMessage = (error: unknown, fallback: string): string => {
    if (error instanceof AxiosError) {
        const response = (error as { response?: { data?: APIErrorBody } }).response;
        // Prefer a server-provided message; never leak Axios internal strings.
        return response?.data?.error?.message ?? response?.data?.message ?? fallback;
    }
    if (typeof error === 'object' && error !== null) {
        const data = (error as { response?: { data?: APIErrorBody } }).response?.data;
        const message = data?.error?.message ?? data?.message;
        if (message) return message;
    }
    return error instanceof Error ? error.message : fallback;
};

const publicPostPath = (id: string) => `/feed/challenges/${encodeURIComponent(id)}`;

export const publicFeedAPI = {
    list: async (cursor: string, signal: AbortSignal) =>
        (await api.get<PublicFeedPage>('/feed', { params: { cursor }, signal })).data,
    get: async (id: string, signal: AbortSignal) =>
        (await api.get<PublicChallenge>(publicPostPath(id), { signal })).data,
    publish: async (form: FormData, signal: AbortSignal) =>
        (await api.post<{ id: string }>('/feed/challenges', form, { signal })).data,
    remove: async (id: string, signal: AbortSignal) => {
        await api.delete(publicPostPath(id), { signal });
        return true;
    },
    media: async (id: string, playing: boolean, signal: AbortSignal) =>
        (await api.get<Blob>(`${publicPostPath(id)}/${playing ? 'play' : 'media'}`, { responseType: 'blob', signal }))
            .data,
    guess: async (id: string, point: { lat: number; long: number }, signal: AbortSignal) =>
        (await api.post<PublicGuessResult>(`${publicPostPath(id)}/guess`, point, { signal })).data,
    result: async (id: string, signal: AbortSignal) =>
        (await api.get<PublicGuessResult>(`${publicPostPath(id)}/guess`, { signal })).data,
    react: async (id: string, liked: boolean, signal: AbortSignal) => {
        if (liked) await api.put(`${publicPostPath(id)}/reaction`, undefined, { signal });
        else await api.delete(`${publicPostPath(id)}/reaction`, { signal });
        return true;
    },
    comments: async (id: string, cursor: string, signal: AbortSignal) =>
        (await api.get<PublicCommentsPage>(`${publicPostPath(id)}/comments`, { params: { cursor }, signal })).data,
    comment: async (id: string, content: string, signal: AbortSignal) =>
        (await api.post<PublicComment>(`${publicPostPath(id)}/comments`, { content }, { signal })).data,
    removeComment: async (id: string, commentID: string, signal: AbortSignal) => {
        await api.delete(`${publicPostPath(id)}/comments/${encodeURIComponent(commentID)}`, { signal });
        return true;
    },
};

export default api;
