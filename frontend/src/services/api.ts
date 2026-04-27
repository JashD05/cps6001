import axios, {
  type AxiosInstance,
  type AxiosError,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
} from 'axios';
import type {
  LoginRequest,
  LoginResponse,
  User,
  Experiment,
  ExperimentRun,
  AttackTemplate,
  Cluster,
  ClusterHealth,
  DashboardSummary,
  Report,
  SIEMAlert,
  APIResponse,
  PaginatedResponse,
} from '@/types';

// ---------------------------------------------------------------------------
// Token helpers
// ---------------------------------------------------------------------------

const TOKEN_KEY = 'chaos_sec_access_token';
const REFRESH_TOKEN_KEY = 'chaos_sec_refresh_token';

export const getAccessToken = (): string | null => localStorage.getItem(TOKEN_KEY);

export const getRefreshToken = (): string | null =>
  localStorage.getItem(REFRESH_TOKEN_KEY);

export const setTokens = (access: string, refresh?: string): void => {
  localStorage.setItem(TOKEN_KEY, access);
  if (refresh) {
    localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
  }
};

export const clearTokens = (): void => {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
};

// ---------------------------------------------------------------------------
// Base Axios instance
// ---------------------------------------------------------------------------

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api';

const apiClient: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 30_000,
  headers: {
    'Content-Type': 'application/json',
    Accept: 'application/json',
  },
});

// ---------------------------------------------------------------------------
// Request interceptor – attach Authorization header
// ---------------------------------------------------------------------------

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getAccessToken();
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error: AxiosError) => Promise.reject(error),
);

// ---------------------------------------------------------------------------
// Response interceptor – handle 401 & token refresh
// ---------------------------------------------------------------------------

let isRefreshing = false;
let failedQueue: Array<{
  resolve: (token: string) => void;
  reject: (error: unknown) => void;
}> = [];

const processQueue = (error: unknown, token: string | null = null): void => {
  failedQueue.forEach(({ resolve, reject }) => {
    if (token) {
      resolve(token);
    } else {
      reject(error);
    }
  });
  failedQueue = [];
};

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & {
      _retry?: boolean;
    };

    // Only attempt refresh on 401 from a non-refresh endpoint
    if (
      error.response?.status === 401 &&
      !originalRequest._retry &&
      originalRequest.url !== '/auth/refresh' &&
      originalRequest.url !== '/auth/login'
    ) {
      // If we are already refreshing, queue this request
      if (isRefreshing) {
        return new Promise<AxiosRequestConfig>((resolve, reject) => {
          failedQueue.push({
            resolve: (token: string) => {
              originalRequest.headers.Authorization = `Bearer ${token}`;
              resolve(apiClient(originalRequest));
            },
            reject,
          });
        });
      }

      originalRequest._retry = true;
      isRefreshing = true;

      try {
        const refreshToken = getRefreshToken();
        if (!refreshToken) {
          throw new Error('No refresh token available');
        }

        const { data } = await axios.post<APIResponse<LoginResponse>>(
          `${BASE_URL}/auth/refresh`,
          { refreshToken },
        );

        const newAccessToken = data.data.accessToken;
        const newRefreshToken = data.data.refreshToken ?? refreshToken;
        setTokens(newAccessToken, newRefreshToken);

        processQueue(null, newAccessToken);

        originalRequest.headers.Authorization = `Bearer ${newAccessToken}`;
        return apiClient(originalRequest);
      } catch (refreshError) {
        processQueue(refreshError, null);
        clearTokens();
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    // For 401 on login/refresh or other status codes, reject normally
    if (error.response?.status === 401) {
      clearTokens();
    }

    return Promise.reject(error);
  },
);

// ---------------------------------------------------------------------------
// Auth API
// ---------------------------------------------------------------------------

export const authAPI = {
  login: (payload: LoginRequest) =>
    apiClient.post<APIResponse<LoginResponse>>('/auth/login', payload),

  logout: () => apiClient.post<APIResponse<void>>('/auth/logout'),

  refresh: (refreshToken: string) =>
    apiClient.post<APIResponse<LoginResponse>>('/auth/refresh', {
      refreshToken,
    }),

  me: () => apiClient.get<APIResponse<User>>('/auth/me'),

  updateProfile: (data: Partial<User>) =>
    apiClient.put<APIResponse<User>>('/auth/profile', data),

  changePassword: (data: { currentPassword: string; newPassword: string }) =>
    apiClient.put<APIResponse<void>>('/auth/password', data),
};

// ---------------------------------------------------------------------------
// Experiments API
// ---------------------------------------------------------------------------

export const experimentsAPI = {
  list: (params?: {
    page?: number;
    limit?: number;
    status?: string;
    search?: string;
    clusterId?: string;
    sortBy?: string;
    sortOrder?: 'asc' | 'desc';
  }) => apiClient.get<PaginatedResponse<Experiment>>('/experiments', { params }),

  getById: (id: string) => apiClient.get<APIResponse<Experiment>>(`/experiments/${id}`),

  create: (data: Partial<Experiment>) =>
    apiClient.post<APIResponse<Experiment>>('/experiments', data),

  update: (id: string, data: Partial<Experiment>) =>
    apiClient.put<APIResponse<Experiment>>(`/experiments/${id}`, data),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/experiments/${id}`),

  execute: (id: string) =>
    apiClient.post<APIResponse<ExperimentRun>>(`/experiments/${id}/execute`),

  stop: (id: string) =>
    apiClient.post<APIResponse<Experiment>>(`/experiments/${id}/stop`),

  getRuns: (id: string, params?: { page?: number; limit?: number }) =>
    apiClient.get<PaginatedResponse<ExperimentRun>>(`/experiments/${id}/runs`, {
      params,
    }),

  getRunById: (experimentId: string, runId: string) =>
    apiClient.get<APIResponse<ExperimentRun>>(
      `/experiments/${experimentId}/runs/${runId}`,
    ),

  getLogs: (id: string, params?: { tail?: number; follow?: boolean }) =>
    apiClient.get<APIResponse<string[]>>(`/experiments/${id}/logs`, { params }),

  getResults: (id: string) =>
    apiClient.get<APIResponse<Experiment>>(`/experiments/${id}/results`),
};

// ---------------------------------------------------------------------------
// Templates API
// ---------------------------------------------------------------------------

export const templatesAPI = {
  list: (params?: { category?: string; search?: string; severity?: string }) =>
    apiClient.get<PaginatedResponse<AttackTemplate>>('/templates', { params }),

  getById: (id: string) => apiClient.get<APIResponse<AttackTemplate>>(`/templates/${id}`),

  create: (data: Partial<AttackTemplate>) =>
    apiClient.post<APIResponse<AttackTemplate>>('/templates', data),

  update: (id: string, data: Partial<AttackTemplate>) =>
    apiClient.put<APIResponse<AttackTemplate>>(`/templates/${id}`, data),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/templates/${id}`),

  getCategories: () => apiClient.get<APIResponse<string[]>>('/templates/categories'),
};

// ---------------------------------------------------------------------------
// Clusters API
// ---------------------------------------------------------------------------

export const clustersAPI = {
  list: (params?: { search?: string; status?: string }) =>
    apiClient.get<PaginatedResponse<Cluster>>('/clusters', { params }),

  getById: (id: string) => apiClient.get<APIResponse<Cluster>>(`/clusters/${id}`),

  register: (data: Partial<Cluster>) =>
    apiClient.post<APIResponse<Cluster>>('/clusters', data),

  update: (id: string, data: Partial<Cluster>) =>
    apiClient.put<APIResponse<Cluster>>(`/clusters/${id}`, data),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/clusters/${id}`),

  healthCheck: (id: string) =>
    apiClient.get<APIResponse<ClusterHealth>>(`/clusters/${id}/health`),

  getNamespaces: (id: string) =>
    apiClient.get<APIResponse<string[]>>(`/clusters/${id}/namespaces`),

  getResources: (id: string, params?: { namespace?: string }) =>
    apiClient.get<APIResponse<Record<string, unknown>>>(`/clusters/${id}/resources`, {
      params,
    }),
};

// ---------------------------------------------------------------------------
// Dashboard API
// ---------------------------------------------------------------------------

export const dashboardAPI = {
  getSummary: () => apiClient.get<APIResponse<DashboardSummary>>('/dashboard/summary'),

  getSecurityPosture: () =>
    apiClient.get<
      APIResponse<{
        score: number;
        trend: number;
        history: Array<{ date: string; score: number }>;
      }>
    >('/dashboard/security-posture'),

  getRecentExperiments: (limit?: number) =>
    apiClient.get<APIResponse<Experiment[]>>('/dashboard/recent-experiments', {
      params: { limit: limit ?? 5 },
    }),

  getClusterHealth: () =>
    apiClient.get<
      APIResponse<
        Array<{ id: string; name: string; status: string; health: ClusterHealth }>
      >
    >('/dashboard/cluster-health'),

  getActivityTimeline: (params?: { days?: number }) =>
    apiClient.get<
      APIResponse<Array<{ date: string; total: number; passed: number; failed: number }>>
    >('/dashboard/activity-timeline', { params }),

  getMetrics: () =>
    apiClient.get<
      APIResponse<{
        experimentsPerDay: number;
        avgDuration: number;
        successRate: number;
        activeUsers: number;
      }>
    >('/dashboard/metrics'),
};

// ---------------------------------------------------------------------------
// Reports API
// ---------------------------------------------------------------------------

export const reportsAPI = {
  list: (params?: {
    page?: number;
    limit?: number;
    experimentId?: string;
    type?: string;
    dateFrom?: string;
    dateTo?: string;
  }) => apiClient.get<PaginatedResponse<Report>>('/reports', { params }),

  getById: (id: string) => apiClient.get<APIResponse<Report>>(`/reports/${id}`),

  generate: (experimentId: string, options?: { format?: string }) =>
    apiClient.post<APIResponse<Report>>('/reports/generate', {
      experimentId,
      ...options,
    }),

  download: (id: string, format: string = 'pdf') =>
    apiClient.get<Blob>(`/reports/${id}/download`, {
      params: { format },
      responseType: 'blob',
    }),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/reports/${id}`),

  share: (id: string, data: { recipients: string[]; message?: string }) =>
    apiClient.post<APIResponse<void>>(`/reports/${id}/share`, data),

  schedule: (data: {
    experimentId: string;
    cron: string;
    format: string;
    recipients: string[];
  }) => apiClient.post<APIResponse<void>>('/reports/schedule', data),
};

// ---------------------------------------------------------------------------
// SIEM API
// ---------------------------------------------------------------------------

export const siemAPI = {
  getAlerts: (params?: {
    experimentId?: string;
    status?: string;
    severity?: string;
    page?: number;
    limit?: number;
  }) => apiClient.get<PaginatedResponse<SIEMAlert>>('/siem/alerts', { params }),

  getAlertById: (id: string) =>
    apiClient.get<APIResponse<SIEMAlert>>(`/siem/alerts/${id}`),

  acknowledgeAlert: (id: string) =>
    apiClient.post<APIResponse<SIEMAlert>>(`/siem/alerts/${id}/acknowledge`),

  dismissAlert: (id: string, reason?: string) =>
    apiClient.post<APIResponse<void>>(`/siem/alerts/${id}/dismiss`, { reason }),

  getValidationResults: (experimentId: string) =>
    apiClient.get<
      APIResponse<{
        experimentId: string;
        expectedAlerts: number;
        receivedAlerts: number;
        validated: boolean;
        details: SIEMAlert[];
      }>
    >(`/siem/validation/${experimentId}`),

  testConnection: (data: { type: string; endpoint: string; apiKey?: string }) =>
    apiClient.post<APIResponse<{ connected: boolean; version?: string }>>(
      '/siem/test-connection',
      data,
    ),

  getConfigurations: () =>
    apiClient.get<
      APIResponse<Array<{ id: string; name: string; type: string; enabled: boolean }>>
    >('/siem/configurations'),

  updateConfiguration: (id: string, data: Record<string, unknown>) =>
    apiClient.put<APIResponse<void>>(`/siem/configurations/${id}`, data),
};

// ---------------------------------------------------------------------------
// Utility: extract error message from Axios error
// ---------------------------------------------------------------------------

export const getErrorMessage = (error: unknown): string => {
  if (axios.isAxiosError(error)) {
    const serverMessage =
      (error.response?.data as { message?: string })?.message ??
      (error.response?.data as { error?: string })?.error;
    if (serverMessage) return serverMessage;
    if (error.message) return error.message;
  }
  if (error instanceof Error) return error.message;
  return 'An unexpected error occurred';
};

// ---------------------------------------------------------------------------
// Export default instance for custom use-cases
// ---------------------------------------------------------------------------

export default apiClient;
