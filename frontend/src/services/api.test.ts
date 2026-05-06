/**
 * Unit tests for the API service module.
 *
 * Covers:
 * - Token management (getAccessToken, getRefreshToken, setTokens, clearTokens)
 * - Error message extraction (getErrorMessage)
 * - Experiment normalization (normalizeExperiment via experimentsAPI)
 * - Cluster normalization (normalizeCluster via clustersAPI)
 * - Template normalization (normalizeTemplate via templatesAPI)
 * - Paginated response normalization (normalizePaginatedResponse via API lists)
 *
 * Note: Normalization functions are internal to the api module and are tested
 * indirectly through the exported API objects by mocking the internal axios client.
 */

import {
  getAccessToken,
  getRefreshToken,
  setTokens,
  clearTokens,
  getErrorMessage,
  experimentsAPI,
  clustersAPI,
  templatesAPI,
} from '@/services/api';
import type {
  Experiment,
  AttackTemplate,
  TemplateParameter,
  ExperimentResult as ExperimentResultType,
} from '@/types';

// ---------------------------------------------------------------------------
// Mock axios so we can control the internal apiClient
// ---------------------------------------------------------------------------

const mockGet = jest.fn();
const mockPost = jest.fn();
const mockPut = jest.fn();
const mockDelete = jest.fn();
const mockPatch = jest.fn();

const actualAxios = jest.requireActual('axios');

jest.mock('axios', () => ({
  __esModule: true,
  default: {
    create: jest.fn(() => ({
      get: mockGet,
      post: mockPost,
      put: mockPut,
      delete: mockDelete,
      patch: mockPatch,
      interceptors: {
        request: { use: jest.fn(), eject: jest.fn() },
        response: { use: jest.fn(), eject: jest.fn() },
      },
      defaults: { headers: { common: {} } },
    })),
    isAxiosError: actualAxios.isAxiosError,
  },
  isAxiosError: actualAxios.isAxiosError,
  AxiosError: actualAxios.AxiosError,
}));

// ---------------------------------------------------------------------------
// Helpers for creating typed error objects
// ---------------------------------------------------------------------------

/**
 * Create an object that `axios.isAxiosError()` recognises as an AxiosError.
 * We set `isAxiosError = true` and attach a `response` if needed.
 */
function createAxiosError(
  opts: {
    message?: string;
    response?: { data?: { message?: string; error?: string }; status?: number };
  } = {},
) {
  const err = new Error(opts.message ?? 'Request failed');
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (err as any).isAxiosError = true;
  if (opts.response) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (err as any).response = opts.response;
  }
  return err;
}

// ---------------------------------------------------------------------------
// Token key constants (must match the source)
// ---------------------------------------------------------------------------

const TOKEN_KEY = 'chaos_sec_access_token';
const REFRESH_TOKEN_KEY = 'chaos_sec_refresh_token';

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Type-safe item extraction helpers
// ---------------------------------------------------------------------------
// The API normalization functions convert backend types at runtime, but
// TypeScript's inference from the mocked axios client doesn't reflect the
// transformed types. These helpers explicitly cast to the frontend types.
// ---------------------------------------------------------------------------

function getTemplate(result: { data: { items: unknown[] } }): AttackTemplate {
  return result.data.items[0] as unknown as AttackTemplate;
}

function getTemplateList(result: { data: { items: unknown[] } }): AttackTemplate[] {
  return result.data.items as unknown as AttackTemplate[];
}

function getExperiment(
  result: { data: { items: unknown[] } },
  index: number = 0,
): Experiment {
  return result.data.items[index] as unknown as Experiment;
}

// ---------------------------------------------------------------------------

beforeEach(() => {
  localStorage.clear();
  jest.clearAllMocks();
});

// ===========================================================================
// Token Management
// ===========================================================================

describe('Token management', () => {
  describe('getAccessToken', () => {
    it('returns null when no token is stored', () => {
      expect(getAccessToken()).toBeNull();
    });

    it('returns the stored access token', () => {
      localStorage.setItem(TOKEN_KEY, 'my-access-token');
      expect(getAccessToken()).toBe('my-access-token');
    });
  });

  describe('getRefreshToken', () => {
    it('returns null when no refresh token is stored', () => {
      expect(getRefreshToken()).toBeNull();
    });

    it('returns the stored refresh token', () => {
      localStorage.setItem(REFRESH_TOKEN_KEY, 'my-refresh-token');
      expect(getRefreshToken()).toBe('my-refresh-token');
    });
  });

  describe('setTokens', () => {
    it('stores the access token', () => {
      setTokens('access-123');
      expect(localStorage.getItem(TOKEN_KEY)).toBe('access-123');
    });

    it('stores the refresh token when provided', () => {
      setTokens('access-123', 'refresh-456');
      expect(localStorage.getItem(TOKEN_KEY)).toBe('access-123');
      expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe('refresh-456');
    });

    it('does not store a refresh token when it is not provided', () => {
      setTokens('access-123');
      expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBeNull();
    });

    it('does not overwrite an existing refresh token when refresh is undefined', () => {
      localStorage.setItem(REFRESH_TOKEN_KEY, 'existing-refresh');
      setTokens('new-access');
      expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe('existing-refresh');
    });

    it('overwrites existing tokens with new values', () => {
      localStorage.setItem(TOKEN_KEY, 'old-access');
      localStorage.setItem(REFRESH_TOKEN_KEY, 'old-refresh');
      setTokens('new-access', 'new-refresh');
      expect(localStorage.getItem(TOKEN_KEY)).toBe('new-access');
      expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe('new-refresh');
    });
  });

  describe('clearTokens', () => {
    it('removes both access and refresh tokens', () => {
      localStorage.setItem(TOKEN_KEY, 'access');
      localStorage.setItem(REFRESH_TOKEN_KEY, 'refresh');
      clearTokens();
      expect(localStorage.getItem(TOKEN_KEY)).toBeNull();
      expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBeNull();
    });

    it('does not throw when tokens are not set', () => {
      expect(() => clearTokens()).not.toThrow();
    });

    it('leaves other localStorage keys untouched', () => {
      localStorage.setItem(TOKEN_KEY, 'access');
      localStorage.setItem('other_key', 'other_value');
      clearTokens();
      expect(localStorage.getItem('other_key')).toBe('other_value');
    });
  });
});

// ===========================================================================
// getErrorMessage
// ===========================================================================

describe('getErrorMessage', () => {
  it('extracts server message from AxiosError response.data.message', () => {
    const err = createAxiosError({
      response: { data: { message: 'Server says no' } },
    });
    expect(getErrorMessage(err)).toBe('Server says no');
  });

  it('extracts server error from AxiosError response.data.error', () => {
    const err = createAxiosError({
      response: { data: { error: 'Something went wrong' } },
    });
    expect(getErrorMessage(err)).toBe('Something went wrong');
  });

  it('prefers response.data.message over response.data.error', () => {
    const err = createAxiosError({
      response: {
        data: { message: 'Priority message', error: 'Secondary error' },
      },
    });
    expect(getErrorMessage(err)).toBe('Priority message');
  });

  it('falls back to error.message when response has no server message', () => {
    const err = createAxiosError({
      message: 'Network Error',
      response: { data: {} },
    });
    expect(getErrorMessage(err)).toBe('Network Error');
  });

  it('falls back to error.message when AxiosError has no response', () => {
    const err = createAxiosError({ message: 'Timeout' });
    expect(getErrorMessage(err)).toBe('Timeout');
  });

  it('falls back to error.message for a regular Error instance', () => {
    const err = new Error('Something broke');
    expect(getErrorMessage(err)).toBe('Something broke');
  });

  it('handles a plain string error', () => {
    expect(getErrorMessage('plain string')).toBe('An unexpected error occurred');
  });

  it('handles null', () => {
    expect(getErrorMessage(null)).toBe('An unexpected error occurred');
  });

  it('handles undefined', () => {
    expect(getErrorMessage(undefined)).toBe('An unexpected error occurred');
  });

  it('handles a number', () => {
    expect(getErrorMessage(42)).toBe('An unexpected error occurred');
  });

  it('handles an object without a message property', () => {
    expect(getErrorMessage({ code: 500 })).toBe('An unexpected error occurred');
  });

  it('handles an AxiosError with response.data.message as an empty string', () => {
    const err = createAxiosError({
      message: 'Fallback message',
      response: { data: { message: '' } },
    });
    // Empty string is falsy, so it falls back to error.message
    expect(getErrorMessage(err)).toBe('Fallback message');
  });
});

// ===========================================================================
// normalizeExperiment (tested via experimentsAPI.list)
// ===========================================================================

describe('normalizeExperiment', () => {
  it('normalizes status to lowercase', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'RUNNING' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    const experiment = result.data.items[0];
    expect(experiment.status).toBe('running');
  });

  it('defaults unknown status to "pending"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'unknown_status' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].status).toBe('pending');
  });

  it('defaults status to "pending" when status is undefined', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].status).toBe('pending');
  });

  it('maps all known statuses correctly', async () => {
    const statuses = [
      'draft',
      'active',
      'pending',
      'queued',
      'running',
      'completed',
      'failed',
      'stopped',
      'timed_out',
      'archived',
    ];

    mockGet.mockResolvedValueOnce({
      data: {
        items: statuses.map((s, i) => ({ id: String(i), name: `Exp ${i}`, status: s })),
        totalCount: statuses.length,
      },
    });

    const result = await experimentsAPI.list({});
    result.data.items.forEach((exp: { status: string }, i: number) => {
      expect(exp.status).toBe(statuses[i]);
    });
  });

  it('resolves template_id from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', template_id: 'tmpl-42' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].templateId).toBe('tmpl-42');
  });

  it('resolves template_name from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', template_name: 'Network Chaos' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].templateName).toBe('Network Chaos');
  });

  it('resolves cluster_id from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', cluster_id: 'cluster-1' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].clusterId).toBe('cluster-1');
  });

  it('resolves cluster_name from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', cluster_name: 'Prod Cluster' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].clusterName).toBe('Prod Cluster');
  });

  it('resolves dates from snake_case fields', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
            started_at: '2024-01-03T00:00:00Z',
            completed_at: '2024-01-04T00:00:00Z',
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    const exp = result.data.items[0];
    expect(exp.createdAt).toBe('2024-01-01T00:00:00Z');
    expect(exp.updatedAt).toBe('2024-01-02T00:00:00Z');
    expect(exp.startedAt).toBe('2024-01-03T00:00:00Z');
    expect(exp.completedAt).toBe('2024-01-04T00:00:00Z');
  });

  it('resolves dates from camelCase fields', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            createdAt: '2024-02-01T00:00:00Z',
            updatedAt: '2024-02-02T00:00:00Z',
            startedAt: '2024-02-03T00:00:00Z',
            completedAt: '2024-02-04T00:00:00Z',
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    const exp = result.data.items[0];
    expect(exp.createdAt).toBe('2024-02-01T00:00:00Z');
    expect(exp.updatedAt).toBe('2024-02-02T00:00:00Z');
    expect(exp.startedAt).toBe('2024-02-03T00:00:00Z');
    expect(exp.completedAt).toBe('2024-02-04T00:00:00Z');
  });

  it('handles numeric progress', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'running', progress: 75 }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].progress).toBe(75);
  });

  it('handles string progress by converting to number', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'running', progress: '50' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].progress).toBe(50);
  });

  it('defaults progress to 0 when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'pending' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].progress).toBe(0);
  });

  it('sets progress to 100 when effective status is completed', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            status: 'running',
            progress: 50,
            result: {
              success: true,
              score: 95,
              summary: 'Done',
              details: [],
              siemValidation: {
                expectedAlertCount: 1,
                receivedAlertCount: 1,
                alerts: [],
                detected: true,
                detectionLatencyMs: 0,
                coverage: 100,
                details: [],
              },
              startedAt: '2024-01-01T00:00:00Z',
              completedAt: '2024-01-01T01:00:00Z',
              duration: 3600000,
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    // When a result exists and status is running/queued/pending,
    // the effective status becomes 'completed' and progress becomes 100
    expect(result.data.items[0].status).toBe('completed');
    expect(result.data.items[0].progress).toBe(100);
  });

  it('does not override draft status even when result exists', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            status: 'draft',
            result: {
              success: true,
              score: 95,
              summary: 'Done',
              details: [],
              siemValidation: {
                expectedAlertCount: 1,
                receivedAlertCount: 1,
                alerts: [],
                detected: true,
                detectionLatencyMs: 0,
                coverage: 100,
                details: [],
              },
              startedAt: '2024-01-01T00:00:00Z',
              completedAt: '2024-01-01T01:00:00Z',
              duration: 3600000,
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    // Draft status is authoritative even with a result
    expect(result.data.items[0].status).toBe('draft');
  });

  it('does not override completed status when result exists', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            status: 'completed',
            result: {
              success: true,
              score: 90,
              summary: 'Done',
              details: [],
              siemValidation: {
                expectedAlertCount: 1,
                receivedAlertCount: 1,
                alerts: [],
                detected: true,
                detectionLatencyMs: 0,
                coverage: 90,
                details: [],
              },
              startedAt: '2024-01-01T00:00:00Z',
              completedAt: '2024-01-01T01:00:00Z',
              duration: 3600000,
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].status).toBe('completed');
  });

  it('handles tags array', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', tags: ['network', 'latency', 'prod'] }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].tags).toEqual(['network', 'latency', 'prod']);
  });

  it('defaults tags to empty array when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].tags).toEqual([]);
  });

  it('handles duration as a number', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'completed', duration: 3600000 }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].duration).toBe(3600000);
  });

  it('handles duration as a string by converting to number', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'completed', duration: '7200000' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].duration).toBe(7200000);
  });

  it('handles null or undefined experiment gracefully', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [null, undefined],
        totalCount: 2,
      },
    });

    const result = await experimentsAPI.list({});
    // normalizeExperiment handles null/undefined by defaulting to empty object
    expect(result.data.items).toHaveLength(2);
    expect(result.data.items[0].id).toBe('');
    expect(result.data.items[1].id).toBe('');
  });

  it('derives namespace from target_namespaces in experiment_templates', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            experiment_templates: [
              {
                id: 'tmpl-1',
                attack_template_id: 'at-1',
                target_namespaces: ['custom-ns'],
                configuration: {},
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].namespace).toBe('custom-ns');
  });

  it('derives cluster_id from target_labels.cluster_id in experiment_templates', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            experiment_templates: [
              {
                id: 'tmpl-1',
                attack_template_id: 'at-1',
                target_labels: { cluster_id: 'cluster-from-labels' },
                configuration: {},
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].clusterId).toBe('cluster-from-labels');
  });

  it('prefers top-level namespace over template-derived namespace', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            namespace: 'top-level-ns',
            experiment_templates: [
              {
                id: 'tmpl-1',
                attack_template_id: 'at-1',
                target_namespaces: ['template-ns'],
                configuration: {},
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].namespace).toBe('top-level-ns');
  });

  it('prefers top-level cluster_id over template-derived cluster_id', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            cluster_id: 'top-level-cluster',
            experiment_templates: [
              {
                id: 'tmpl-1',
                attack_template_id: 'at-1',
                target_labels: { cluster_id: 'label-cluster' },
                configuration: {},
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.items[0].clusterId).toBe('top-level-cluster');
  });
});

// ===========================================================================
// normalizeCluster (tested via clustersAPI.list)
// ===========================================================================

describe('normalizeCluster', () => {
  it('maps "connected" status to "healthy"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'connected' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('healthy');
  });

  it('maps "pending" status to "unknown"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'pending' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('unknown');
  });

  it('maps "error" status to "unreachable"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'error' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('unreachable');
  });

  it('maps "disabled" status to "unreachable"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'disabled' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('unreachable');
  });

  it('passes through "healthy" status unchanged', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'healthy' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('healthy');
  });

  it('passes through "degraded" status unchanged', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'degraded' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('degraded');
  });

  it('passes through "unreachable" status unchanged', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'unreachable' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('unreachable');
  });

  it('defaults unknown status to "unknown"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster', status: 'something_else' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('unknown');
  });

  it('defaults to "unknown" when status is not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test Cluster' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].status).toBe('unknown');
  });

  it('uses default_namespace when provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', default_namespace: 'my-ns' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].namespaces).toContain('my-ns');
  });

  it('defaults to "default" namespace when default_namespace is empty', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', default_namespace: '' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].namespaces).toContain('default');
  });

  it('uses namespaces array when provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', namespaces: ['ns-1', 'ns-2', 'ns-3'] }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].namespaces).toEqual(['ns-1', 'ns-2', 'ns-3']);
  });

  it('filters empty strings from namespaces array', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', namespaces: ['ns-1', '', '  ', 'ns-2'] }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].namespaces).toEqual(['ns-1', 'ns-2']);
  });

  it('defaults name to "Unnamed Cluster" when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].name).toBe('Unnamed Cluster');
  });

  it('defaults name to "Unnamed Cluster" when name is empty string', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: '   ' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].name).toBe('Unnamed Cluster');
  });

  it('resolves kubernetes_version from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', kubernetes_version: '1.28.0' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].version).toBe('1.28.0');
  });

  it('resolves version from camelCase field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', version: '1.27.0' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].version).toBe('1.27.0');
  });

  it('defaults version to "unknown" when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].version).toBe('unknown');
  });

  it('resolves node_count from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', node_count: 5 }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].nodeCount).toBe(5);
  });

  it('defaults nodeCount to 0 when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].nodeCount).toBe(0);
  });

  it('resolves namespace_count from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', namespace_count: 10 }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].namespaceCount).toBe(10);
  });

  it('derives namespaceCount from namespaces array length when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', namespaces: ['a', 'b', 'c'] }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].namespaceCount).toBe(3);
  });

  it('resolves provider from snake_case field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', provider: 'aws' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].provider).toBe('aws');
  });

  it('defaults provider to "other" when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].provider).toBe('other');
  });

  it('defaults provider to "other" when provider is empty string', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', provider: '  ' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].provider).toBe('other');
  });

  it('defaults region to "unknown" when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].region).toBe('unknown');
  });

  it('handles labels object', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', labels: { env: 'prod', team: 'platform' } }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].labels).toEqual({ env: 'prod', team: 'platform' });
  });

  it('defaults labels to empty object when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].labels).toEqual({});
  });

  it('defaults labels to empty object when labels is an array', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', labels: ['not', 'an', 'object'] }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].labels).toEqual({});
  });

  it('uses last_connected_at for lastHealthCheck', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', last_connected_at: '2024-01-01T00:00:00Z' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].lastHealthCheck).toBe('2024-01-01T00:00:00Z');
  });

  it('falls back to updated_at for lastHealthCheck', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', updated_at: '2024-02-01T00:00:00Z' }],
        totalCount: 1,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items[0].lastHealthCheck).toBe('2024-02-01T00:00:00Z');
  });

  it('handles null/undefined cluster gracefully', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [null, undefined],
        totalCount: 2,
      },
    });

    const result = await clustersAPI.list({});
    expect(result.data.items).toHaveLength(2);
    expect(result.data.items[0].id).toBe('');
    expect(result.data.items[0].name).toBe('Unnamed Cluster');
  });
});

// ===========================================================================
// normalizeTemplate (tested via templatesAPI.list)
// ===========================================================================

describe('normalizeTemplate', () => {
  it('maps "network" category correctly', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Net Test', category: 'network' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('network');
  });

  it('maps "application" category correctly', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'App Test', category: 'application' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('application');
  });

  it('maps "data" category correctly', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Data Test', category: 'data' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('data');
  });

  it('maps "rbac" category to "identity"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'RBAC Test', category: 'rbac' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('identity');
  });

  it('maps "privilege" category to "identity"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Priv Test', category: 'privilege' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('identity');
  });

  it('maps "resource" category to "infrastructure"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Res Test', category: 'resource' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('infrastructure');
  });

  it('maps "availability" category to "infrastructure"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Avail Test', category: 'availability' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('infrastructure');
  });

  it('defaults unknown category to "custom"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', category: 'unknown_category' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('custom');
  });

  it('defaults to "custom" when category is not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).category).toBe('custom');
  });

  it('normalizes "low" severity', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', severity: 'low' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('low');
  });

  it('normalizes "medium" severity', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', severity: 'medium' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('medium');
  });

  it('normalizes "high" severity', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', severity: 'high' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('high');
  });

  it('normalizes "critical" severity', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', severity: 'critical' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('critical');
  });

  it('defaults unknown severity to "medium"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', severity: 'extreme' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('medium');
  });

  it('defaults severity to "medium" when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('medium');
  });

  it('handles case-insensitive severity', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', severity: 'HIGH' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).severity).toBe('high');
  });

  it('derives tags from slug', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', category: 'network', slug: 'pod-kill-test' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const tmpl = getTemplate(result);
    // Tags should include category + slug parts
    expect(tmpl.tags).toContain('network');
    expect(tmpl.tags).toContain('pod');
    expect(tmpl.tags).toContain('kill');
    expect(tmpl.tags).toContain('test');
  });

  it('falls back to category-only tags when slug is not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', category: 'application' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).tags).toEqual(['application']);
  });

  it('sets isOfficial to true when is_system is true', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', is_system: true }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).isOfficial).toBe(true);
  });

  it('sets isOfficial to true when is_module is true', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', is_module: true }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).isOfficial).toBe(true);
  });

  it('sets isOfficial to false when neither is_system nor is_module', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', is_system: false, is_module: false }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).isOfficial).toBe(false);
  });

  it('sets author to "Chaos-Sec Team" for system templates', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', is_system: true }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).author).toBe('Chaos-Sec Team');
  });

  it('sets author to "Chaos-Sec Team" for module templates', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', is_module: true }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).author).toBe('Chaos-Sec Team');
  });

  it('sets author to "User" for non-system, non-module templates', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', is_system: false, is_module: false }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).author).toBe('User');
  });

  it('defaults description to empty string when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).description).toBe('');
  });

  it('uses created_at and updated_at from the template', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-02-01T00:00:00Z',
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).createdAt).toBe('2024-01-01T00:00:00Z');
    expect(getTemplate(result).updatedAt).toBe('2024-02-01T00:00:00Z');
  });

  it('falls back to created_at for updated_at when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).updatedAt).toBe('2024-01-01T00:00:00Z');
  });

  it('sets version to "1.0.0"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).version).toBe('1.0.0');
  });

  it('sets usageCount to 0', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).usageCount).toBe(0);
  });

  it('maps category to the correct icon', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', category: 'network' }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).icon).toBe('network');
  });

  it('normalizes template parameters from array format', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [
              {
                key: 'duration',
                label: 'Duration',
                type: 'int',
                defaultValue: 60,
                required: true,
                description: 'Duration in seconds',
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params).toHaveLength(1);
    expect(params[0].key).toBe('duration');
    expect(params[0].type).toBe('number'); // 'int' normalized to 'number'
    expect(params[0].required).toBe(true);
  });

  it('normalizes template parameters from JSON Schema format', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: {
              required: ['target_namespace'],
              properties: {
                target_namespace: { type: 'string', description: 'The target namespace' },
              },
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params).toHaveLength(1);
    expect(params[0].key).toBe('target_namespace');
    expect(params[0].required).toBe(true);
  });

  it('handles empty parameters gracefully', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', parameters: [] }],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).parameters).toEqual([]);
  });
});

// ===========================================================================
// normalizePaginatedResponse (tested via API list endpoints)
// ===========================================================================

describe('normalizePaginatedResponse', () => {
  it('normalizes items from "items" field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(getTemplateList(result)).toHaveLength(1);
  });

  it('normalizes items from "data" field when items is not present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        data: [{ id: '1', name: 'Test' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(getTemplateList(result)).toHaveLength(1);
  });

  it('prefers "items" over "data" when both are present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: 'from-items', name: 'Items' }],
        data: [{ id: 'from-data', name: 'Data' }],
        totalCount: 2,
      },
    });

    const result = await experimentsAPI.list({});
    expect((result.data.items[0] as unknown as { id: string }).id).toContain(
      'from-items',
    );
  });

  it('defaults items to empty array when neither items nor data is present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        totalCount: 0,
      },
    });

    const result = await experimentsAPI.list({});
    expect(getTemplateList(result)).toEqual([]);
  });

  it('uses "totalCount" field when present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 42,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalCount).toBe(42);
  });

  it('uses "total" field when totalCount is not present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        total: 99,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalCount).toBe(99);
  });

  it('falls back to items.length when neither totalCount nor total is present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          { id: '1', name: 'Test' },
          { id: '2', name: 'Test 2' },
        ],
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalCount).toBe(2);
  });

  it('defaults page to 1 when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 0,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.page).toBe(1);
  });

  it('uses "page" field when provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 100,
        page: 3,
        pageSize: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.page).toBe(3);
  });

  it('uses "pageSize" field when present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
        pageSize: 25,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.pageSize).toBe(25);
  });

  it('uses "page_size" field when pageSize is not present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        totalCount: 1,
        page_size: 50,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.pageSize).toBe(50);
  });

  it('defaults pageSize to 10 when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 0,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.pageSize).toBe(10);
  });

  it('uses "totalPages" field when present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 100,
        pageSize: 10,
        totalPages: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalPages).toBe(10);
  });

  it('uses "total_pages" field when totalPages is not present', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 100,
        pageSize: 10,
        total_pages: 5,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalPages).toBe(5);
  });

  it('calculates totalPages from totalCount and pageSize when not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 45,
        pageSize: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalPages).toBe(5); // ceil(45/10) = 5
  });

  it('defaults totalPages to 1 when totalCount and pageSize are missing', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
      },
    });

    const result = await experimentsAPI.list({});
    // items.length = 0, default pageSize = 10, ceil(0/10) = max(1, 0) = 1
    expect(result.data.totalPages).toBe(1);
  });

  it('computes hasNextPage as true when page < totalPages', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 100,
        page: 1,
        pageSize: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.hasNextPage).toBe(true);
  });

  it('computes hasNextPage as false when page >= totalPages', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 10,
        page: 1,
        pageSize: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.hasNextPage).toBe(false);
  });

  it('uses explicit hasNextPage when provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 10,
        page: 1,
        pageSize: 10,
        hasNextPage: true,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.hasNextPage).toBe(true);
  });

  it('computes hasPreviousPage as true when page > 1', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 100,
        page: 2,
        pageSize: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.hasPreviousPage).toBe(true);
  });

  it('computes hasPreviousPage as false when page is 1', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 100,
        page: 1,
        pageSize: 10,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.hasPreviousPage).toBe(false);
  });

  it('uses explicit hasPreviousPage when provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [],
        totalCount: 100,
        page: 1,
        pageSize: 10,
        hasPreviousPage: true,
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.hasPreviousPage).toBe(true);
  });

  it('handles pagination from nested pagination object', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test' }],
        pagination: { total: 50, page: 3, limit: 10 },
      },
    });

    const result = await experimentsAPI.list({});
    expect(result.data.totalCount).toBe(50);
    expect(result.data.page).toBe(3);
    expect(result.data.pageSize).toBe(10);
  });
});

// ===========================================================================
// normalizeExperimentResult (tested via experimentsAPI.list)
// ===========================================================================

describe('normalizeExperimentResult', () => {
  it('passes through fully-formed ExperimentResult objects', async () => {
    const fullResult = {
      success: true,
      score: 95,
      summary: 'All good',
      details: ['Check 1 passed'],
      siemValidation: {
        expectedAlertCount: 1,
        receivedAlertCount: 1,
        alerts: [],
        detected: true,
        detectionLatencyMs: 500,
        coverage: 100,
        details: [],
      },
      startedAt: '2024-01-01T00:00:00Z',
      completedAt: '2024-01-01T01:00:00Z',
      duration: 3600000,
    };

    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'completed', result: fullResult }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(getExperiment(result).result).toEqual(fullResult);
  });

  it('normalizes result with siem_alerts_expected field', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            status: 'completed',
            result: {
              siem_alerts_expected: 3,
              siem_alerts_received: 2,
              overall_status: 'partial_success',
              findings: [],
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    const expResult = getExperiment(result).result as ExperimentResultType;
    expect(expResult).toBeDefined();
    expect(expResult.siemValidation.expectedAlertCount).toBe(3);
    expect(expResult.siemValidation.receivedAlertCount).toBe(2);
  });

  it('returns undefined when result is null', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'pending', result: null }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(getExperiment(result).result).toBeUndefined();
  });

  it('returns undefined when result is not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [{ id: '1', name: 'Test', status: 'pending' }],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    expect(getExperiment(result).result).toBeUndefined();
  });

  it('derives detection rate from alert counts', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            status: 'completed',
            result: {
              siem_alerts_expected: 4,
              siem_alerts_received: 3,
              overall_status: 'completed',
              findings: [],
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await experimentsAPI.list({});
    const expResult = getExperiment(result).result as ExperimentResultType;
    expect(expResult).toBeDefined();
    // detectionRate = min(100, round(3/4 * 100)) = 75
    expect(expResult.score).toBe(75);
    expect(expResult.siemValidation.coverage).toBe(75);
  });
});

// ===========================================================================
// normalizeTemplateParameters (tested via templatesAPI.list)
// ===========================================================================

describe('normalizeTemplateParameters', () => {
  it('normalizes "int" type to "number"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'count', type: 'int' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('number');
  });

  it('normalizes "integer" type to "number"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'count', type: 'integer' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('number');
  });

  it('normalizes "bool" type to "boolean"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'enabled', type: 'bool' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('boolean');
  });

  it('normalizes "boolean" type to "boolean"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'enabled', type: 'boolean' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('boolean');
  });

  it('normalizes "select" type to "select"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'mode', type: 'select' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('select');
  });

  it('normalizes "multi_select" type to "multi-select"', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'targets', type: 'multi_select' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('multi-select');
  });

  it('defaults type to "string" for unknown types', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ key: 'custom', type: 'custom_type' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].type).toBe('string');
  });

  it('normalizes string options to {label, value} pairs', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [
              {
                key: 'env',
                type: 'select',
                options: ['dev', 'staging', 'prod'],
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    const options = params[0].options;
    expect(options).toBeDefined();
    expect(options).toEqual([
      { label: 'dev', value: 'dev' },
      { label: 'staging', value: 'staging' },
      { label: 'prod', value: 'prod' },
    ]);
  });

  it('normalizes object options to {label, value} pairs', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [
              {
                key: 'env',
                type: 'select',
                options: [
                  { label: 'Development', value: 'dev' },
                  { label: 'Production', value: 'prod' },
                ],
              },
            ],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    const options = params[0].options;
    expect(options).toBeDefined();
    expect(options).toEqual([
      { label: 'Development', value: 'dev' },
      { label: 'Production', value: 'prod' },
    ]);
  });

  it('uses "name" as key fallback when "key" is not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ name: 'duration', type: 'number' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].key).toBe('duration');
  });

  it('uses "label" as label fallback when "label" is not provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ name: 'my_param', title: 'My Parameter', type: 'string' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].label).toBe('My Parameter');
  });

  it('defaults to key as label when neither label nor title is provided', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: [{ name: 'my_param', type: 'string' }],
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params[0].label).toBe('my_param');
  });

  it('handles parameters as JSON Schema with required fields', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: {
              required: ['namespace', 'duration'],
              properties: {
                namespace: { type: 'string', description: 'Target namespace' },
                duration: { type: 'integer', description: 'Duration in seconds' },
                optional_field: { type: 'string', description: 'Optional' },
              },
            },
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    const params = getTemplate(result).parameters as TemplateParameter[];
    expect(params).toHaveLength(3);
    const namespaceParam = params.find((p) => p.key === 'namespace');
    const durationParam = params.find((p) => p.key === 'duration');
    const optionalParam = params.find((p) => p.key === 'optional_field');
    expect(namespaceParam).toBeDefined();
    expect(namespaceParam?.required).toBe(true);
    expect(durationParam).toBeDefined();
    expect(durationParam?.required).toBe(true);
    expect(optionalParam).toBeDefined();
    expect(optionalParam?.required).toBe(false);
  });

  it('handles non-object, non-array parameters gracefully', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: '1',
            name: 'Test',
            parameters: 'not-an-object',
          },
        ],
        totalCount: 1,
      },
    });

    const result = await templatesAPI.list({});
    expect(getTemplate(result).parameters).toEqual([]);
  });
});
