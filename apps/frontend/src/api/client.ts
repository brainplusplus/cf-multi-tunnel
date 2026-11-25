import axios, { type AxiosError, type InternalAxiosRequestConfig } from 'axios';

declare global {
  interface Window {
    __RUNTIME_CONFIG__?: { API_URL?: string };
  }
}

// Runtime config first (the reverse proxy can inject it), then build-time env,
// then an empty base URL meaning "same origin".
const API_BASE_URL = window.__RUNTIME_CONFIG__?.API_URL ?? import.meta.env.VITE_API_URL ?? '';

const client = axios.create({
  baseURL: API_BASE_URL,
  headers: { 'Content-Type': 'application/json' },
});

const NO_REFRESH = ['/api/auth/login', '/api/auth/refresh'];

interface RetryableConfig extends InternalAxiosRequestConfig {
  _retried?: boolean;
}

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Single-flight refresh: several requests can 401 at the same moment, and each
// would otherwise rotate the refresh token and invalidate the others.
let refreshPromise: Promise<string> | null = null;

function refreshAccessToken(): Promise<string> {
  if (refreshPromise) return refreshPromise;

  const refreshToken = localStorage.getItem('refresh_token');
  if (!refreshToken) return Promise.reject(new Error('no refresh token'));

  refreshPromise = axios
    .post(`${API_BASE_URL}/api/auth/refresh`, { refresh_token: refreshToken })
    .then((res) => {
      localStorage.setItem('token', res.data.access_token);
      localStorage.setItem('refresh_token', res.data.refresh_token);
      return res.data.access_token as string;
    })
    .finally(() => {
      refreshPromise = null;
    });

  return refreshPromise;
}

function clearSession() {
  localStorage.removeItem('token');
  localStorage.removeItem('refresh_token');
  localStorage.removeItem('user');
  if (window.location.pathname !== '/login') {
    window.location.href = '/login';
  }
}

client.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const config = error.config as RetryableConfig | undefined;
    const status = error.response?.status;

    if (status === 401 && config && !config._retried && !NO_REFRESH.some((p) => config.url?.includes(p))) {
      config._retried = true;
      try {
        const token = await refreshAccessToken();
        config.headers.Authorization = `Bearer ${token}`;
        return client(config);
      } catch {
        clearSession();
        return Promise.reject(error);
      }
    }

    if (status === 401 && !NO_REFRESH.some((p) => config?.url?.includes(p))) {
      clearSession();
    }

    return Promise.reject(error);
  },
);

export default client;

export function apiMessage(err: unknown): string {
  const axiosErr = err as AxiosError<{ message?: string; fields?: Record<string, string> }>;
  const data = axiosErr?.response?.data;
  if (data?.message) {
    if (data.fields && Object.keys(data.fields).length > 0) {
      return `${data.message}: ${Object.entries(data.fields).map(([k, v]) => `${k} ${v}`).join(', ')}`;
    }
    return data.message;
  }
  if (axiosErr?.message) return axiosErr.message;
  return 'Unexpected error';
}

export function apiFields(err: unknown): Record<string, string> {
  const axiosErr = err as AxiosError<{ fields?: Record<string, string> }>;
  return axiosErr?.response?.data?.fields ?? {};
}
