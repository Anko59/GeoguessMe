import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios';
import type { APIErrorBody, AuthResponse } from './types';

let accessToken: string | null = null;
let refreshPromise: Promise<string | null> | null = null;

export const setAccessToken = (token: string | null): void => {
    accessToken = token;
};

export const getAccessToken = (): string | null => accessToken;

export const api = axios.create({
    baseURL: '/api/v1',
    withCredentials: true,
});

const refresh = async (): Promise<string | null> => {
    if (!refreshPromise) {
        refreshPromise = axios
            .post<AuthResponse>('/api/v1/auth/refresh', undefined, { withCredentials: true })
            .then((response) => {
                setAccessToken(response.data.access_token);
                return response.data.access_token;
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

api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
    const isSameOriginAPI = !config.url?.startsWith('http');
    if (accessToken && isSameOriginAPI) {
        config.headers.Authorization = `Bearer ${accessToken}`;
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
            const token = await refresh();
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
        return error.response?.data?.error?.message ?? error.message ?? fallback;
    }
    return error instanceof Error ? error.message : fallback;
};

export default api;
