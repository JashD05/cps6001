/**
 * Unit tests for the experiment Redux slice.
 *
 * Tests the slice reducer logic, async thunks (fetchExperiments,
 * fetchExperimentById, createExperiment, updateExperiment,
 * deleteExperiment, executeExperiment, stopExperiment,
 * fetchExperimentRuns, fetchExperimentLogs), synchronous actions,
 * and selectors using a real Redux store backed by the
 * experimentSlice reducer.
 */

import { configureStore } from '@reduxjs/toolkit';
import { experimentsAPI, getErrorMessage } from '@/services/api';
import experimentReducer, {
  fetchExperiments,
  fetchExperimentById,
  createExperiment,
  updateExperiment,
  deleteExperiment,
  executeExperiment,
  stopExperiment,
  fetchExperimentRuns,
  fetchExperimentLogs,
  setExperimentFilters,
  resetExperimentFilters,
  setExperimentPage,
  setExperimentPageSize,
  setExperimentSort,
  clearExperimentDetail,
  clearExperimentLogs,
  appendExperimentLog,
  resetCreateStatus,
  resetExecuteStatus,
  resetStopStatus,
  resetDeleteStatus,
  updateExperimentStatus,
  selectExperimentList,
  selectExperimentListLoading,
  selectExperimentListError,
  selectExperimentListTotalCount,
  selectExperimentListPage,
  selectExperimentListPageSize,
  selectExperimentFilters,
  selectExperimentSort,
  selectExperimentSortBy,
  selectExperimentSortOrder,
  selectExperimentDetail,
  selectExperimentDetailLoading,
  selectExperimentDetailError,
  selectCurrentRun,
  selectExperimentLogs,
  selectCreateStatus,
  selectCreateError,
  selectExecuteStatus,
  selectExecuteError,
  selectStopStatus,
  selectStopError,
  selectDeleteStatus,
  selectDeleteError,
  selectExperimentById,
  selectRunningExperiments,
  selectRecentExperiments,
  selectExperimentStats,
} from '@/store/experimentSlice';
import type {
  Experiment,
  ExperimentRun,
  PaginatedResponse,
  CreateExperimentRequest,
} from '@/types';
import type { ExperimentState } from '@/store/experimentSlice';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock('@/services/api', () => ({
  experimentsAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    execute: jest.fn(),
    stop: jest.fn(),
    getRuns: jest.fn(),
    getRunById: jest.fn(),
    getLogs: jest.fn(),
    getResults: jest.fn(),
    cancelStaleRuns: jest.fn(),
  },
  getErrorMessage: jest.fn(),
}));

const mockedExperimentsAPI = experimentsAPI as jest.Mocked<typeof experimentsAPI>;
const mockedGetErrorMessage = getErrorMessage as jest.MockedFunction<
  typeof getErrorMessage
>;

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const mockExperiment: Experiment = {
  id: 'exp-1',
  name: 'Network Latency Test',
  description: 'Test network latency under stress',
  templateId: 'tmpl-1',
  templateName: 'Network Chaos',
  clusterId: 'cluster-1',
  clusterName: 'Production Cluster',
  namespace: 'default',
  status: 'pending',
  progress: 0,
  parameters: {},
  steps: [
    {
      id: 'step-1',
      name: 'Inject Latency',
      description: 'Inject network latency',
      status: 'pending',
      order: 0,
    },
  ],
  tags: ['network', 'latency'],
  createdBy: 'user-1',
  createdAt: '2024-01-15T10:00:00Z',
  updatedAt: '2024-01-15T10:00:00Z',
  duration: undefined,
};

const mockExperiment2: Experiment = {
  id: 'exp-2',
  name: 'Pod Kill Test',
  description: 'Test pod resilience',
  templateId: 'tmpl-2',
  templateName: 'Pod Chaos',
  clusterId: 'cluster-2',
  clusterName: 'Staging Cluster',
  namespace: 'staging',
  status: 'running',
  progress: 50,
  parameters: {},
  steps: [],
  tags: ['pod', 'resilience'],
  createdBy: 'user-2',
  createdAt: '2024-01-16T10:00:00Z',
  updatedAt: '2024-01-16T10:00:00Z',
  startedAt: '2024-01-16T10:00:00Z',
  duration: undefined,
};

const mockExperiment3: Experiment = {
  id: 'exp-3',
  name: 'Completed Test',
  description: 'A completed experiment',
  templateId: 'tmpl-1',
  templateName: 'Network Chaos',
  clusterId: 'cluster-1',
  clusterName: 'Production Cluster',
  namespace: 'default',
  status: 'completed',
  progress: 100,
  parameters: {},
  steps: [],
  tags: ['network'],
  createdBy: 'user-1',
  createdAt: '2024-01-14T10:00:00Z',
  updatedAt: '2024-01-14T11:00:00Z',
  startedAt: '2024-01-14T10:00:00Z',
  completedAt: '2024-01-14T11:00:00Z',
  duration: 3600000,
  result: {
    success: true,
    score: 95,
    summary: 'Experiment completed successfully',
    details: ['All checks passed'],
    siemValidation: {
      expectedAlertCount: 1,
      receivedAlertCount: 1,
      alerts: [],
      detected: true,
      detectionLatencyMs: 500,
      coverage: 95,
      details: [],
    },
    startedAt: '2024-01-14T10:00:00Z',
    completedAt: '2024-01-14T11:00:00Z',
    duration: 3600000,
  },
};

const mockRun: ExperimentRun = {
  id: 'run-1',
  experimentId: 'exp-1',
  status: 'running',
  progress: 50,
  logs: ['Starting experiment...'],
  startedAt: '2024-01-15T10:00:00Z',
  podStatuses: [],
  steps: [],
};

const mockCreateRequest: CreateExperimentRequest = {
  name: 'New Experiment',
  description: 'A new test',
  templateId: 'tmpl-1',
  clusterId: 'cluster-1',
  namespace: 'default',
  parameters: { duration: 60 },
  validation: {
    siemAlertType: 'prometheus',
    timeWindowSeconds: 300,
    expectedAlertCount: 1,
    customRules: {},
  },
};

/**
 * Helper to wrap data in a paginated response for mock returns.
 */
function paginatedResponse<T>(items: T[], totalCount?: number): PaginatedResponse<T> {
  return {
    items,
    totalCount: totalCount ?? items.length,
    page: 1,
    pageSize: 10,
    totalPages: 1,
    hasNextPage: false,
    hasPreviousPage: false,
  };
}

/**
 * Helper to create a non-typed paginated response for testing
 * flexible payload shapes in fetchExperiments.fulfilled.
 */
function flexiblePaginatedResponse(
  overrides: Record<string, unknown>,
): Record<string, unknown> {
  return overrides;
}

/**
 * Helper to wrap data in an Axios-like response for mock returns.
 * Cast to `never` because `mockResolvedValueOnce` enforces the full
 * AxiosResponse shape, but only `response.data` is used by the thunks.
 */
function apiResponse<T>(data: T) {
  return { data } as never;
}

// ---------------------------------------------------------------------------
// Store helpers
// ---------------------------------------------------------------------------

/**
 * Create a real Redux store with the experimentSlice reducer.
 * Optionally provide initial state overrides.
 */
function createTestStore(overrides?: Partial<ExperimentState>) {
  const preloadedState = overrides
    ? { experiments: { ...initialState, ...overrides } }
    : undefined;
  return configureStore({
    reducer: { experiments: experimentReducer },
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware({
        serializableCheck: false,
        immutableCheck: false,
      }),
    preloadedState,
  });
}

/** Grab the initial state directly from the slice definition. */
let initialState: ExperimentState;

beforeEach(() => {
  initialState = experimentReducer(undefined, { type: '@@INIT' }) as ExperimentState;
  jest.clearAllMocks();
});

// ---------------------------------------------------------------------------
// Initial State
// ---------------------------------------------------------------------------

describe('experimentSlice – initial state', () => {
  it('has the correct default values', () => {
    expect(initialState.list.experiments).toEqual([]);
    expect(initialState.list.totalCount).toBe(0);
    expect(initialState.list.currentPage).toBe(1);
    expect(initialState.list.pageSize).toBe(10);
    expect(initialState.list.isLoading).toBe(false);
    expect(initialState.list.error).toBeNull();
    expect(initialState.list.filters).toEqual({
      search: '',
      status: 'all',
      templateId: null,
      clusterId: null,
      dateFrom: null,
      dateTo: null,
    });
    expect(initialState.list.sortBy).toBe('createdAt');
    expect(initialState.list.sortOrder).toBe('desc');
    expect(initialState.detail.experiment).toBeNull();
    expect(initialState.detail.currentRun).toBeNull();
    expect(initialState.detail.logs).toEqual([]);
    expect(initialState.detail.isLoading).toBe(false);
    expect(initialState.detail.error).toBeNull();
    expect(initialState.createStatus).toBe('idle');
    expect(initialState.createError).toBeNull();
    expect(initialState.executeStatus).toBe('idle');
    expect(initialState.executeError).toBeNull();
    expect(initialState.stopStatus).toBe('idle');
    expect(initialState.stopError).toBeNull();
    expect(initialState.deleteStatus).toBe('idle');
    expect(initialState.deleteError).toBeNull();
    expect(initialState.runs).toEqual([]);
    expect(initialState.runsTotalCount).toBe(0);
    expect(initialState.runsPage).toBe(1);
    expect(initialState.runsLoading).toBe(false);
    expect(initialState.runsError).toBeNull();
  });

  it('returns the same state for an unknown action', () => {
    const state = experimentReducer(initialState, { type: 'unknown/action' });
    expect(state).toEqual(initialState);
  });
});

// ---------------------------------------------------------------------------
// Synchronous Reducers
// ---------------------------------------------------------------------------

describe('experimentSlice – setExperimentFilters', () => {
  it('merges partial filters into the current filters', () => {
    const state = experimentReducer(
      initialState,
      setExperimentFilters({ search: 'network', status: 'running' }),
    );
    expect(state.list.filters.search).toBe('network');
    expect(state.list.filters.status).toBe('running');
    expect(state.list.filters.templateId).toBeNull();
  });

  it('resets the current page to 1 when filters change', () => {
    const stateOnPage3 = experimentReducer(initialState, setExperimentPage(3));
    expect(stateOnPage3.list.currentPage).toBe(3);

    const stateAfterFilter = experimentReducer(
      stateOnPage3,
      setExperimentFilters({ search: 'test' }),
    );
    expect(stateAfterFilter.list.currentPage).toBe(1);
  });
});

describe('experimentSlice – resetExperimentFilters', () => {
  it('resets filters to initial values', () => {
    const stateWithFilters = experimentReducer(
      initialState,
      setExperimentFilters({ search: 'test', clusterId: 'c-1' }),
    );
    expect(stateWithFilters.list.filters.search).toBe('test');

    const stateAfterReset = experimentReducer(stateWithFilters, resetExperimentFilters());
    expect(stateAfterReset.list.filters.search).toBe('');
    expect(stateAfterReset.list.filters.status).toBe('all');
    expect(stateAfterReset.list.filters.templateId).toBeNull();
    expect(stateAfterReset.list.filters.clusterId).toBeNull();
    expect(stateAfterReset.list.filters.dateFrom).toBeNull();
    expect(stateAfterReset.list.filters.dateTo).toBeNull();
  });

  it('resets the current page to 1', () => {
    const stateOnPage5 = experimentReducer(initialState, setExperimentPage(5));
    const stateAfterReset = experimentReducer(stateOnPage5, resetExperimentFilters());
    expect(stateAfterReset.list.currentPage).toBe(1);
  });
});

describe('experimentSlice – setExperimentPage', () => {
  it('sets the current page', () => {
    const state = experimentReducer(initialState, setExperimentPage(3));
    expect(state.list.currentPage).toBe(3);
  });
});

describe('experimentSlice – setExperimentPageSize', () => {
  it('sets the page size', () => {
    const state = experimentReducer(initialState, setExperimentPageSize(25));
    expect(state.list.pageSize).toBe(25);
  });

  it('resets the current page to 1', () => {
    const stateOnPage3 = experimentReducer(initialState, setExperimentPage(3));
    const stateAfterPageSize = experimentReducer(stateOnPage3, setExperimentPageSize(25));
    expect(stateAfterPageSize.list.currentPage).toBe(1);
    expect(stateAfterPageSize.list.pageSize).toBe(25);
  });
});

describe('experimentSlice – setExperimentSort', () => {
  it('sets sort by and sort order', () => {
    const state = experimentReducer(
      initialState,
      setExperimentSort({ sortBy: 'name', sortOrder: 'asc' }),
    );
    expect(state.list.sortBy).toBe('name');
    expect(state.list.sortOrder).toBe('asc');
  });
});

describe('experimentSlice – clearExperimentDetail', () => {
  it('resets detail to initial state', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        experiment: mockExperiment,
        currentRun: mockRun,
        logs: ['log line 1'],
        isLoading: false,
        error: null,
      },
    };

    const state = experimentReducer(stateWithDetail, clearExperimentDetail());
    expect(state.detail.experiment).toBeNull();
    expect(state.detail.currentRun).toBeNull();
    expect(state.detail.logs).toEqual([]);
    expect(state.detail.isLoading).toBe(false);
    expect(state.detail.error).toBeNull();
  });
});

describe('experimentSlice – clearExperimentLogs', () => {
  it('clears the logs array', () => {
    const stateWithLogs: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        logs: ['line 1', 'line 2'],
      },
    };

    const state = experimentReducer(stateWithLogs, clearExperimentLogs());
    expect(state.detail.logs).toEqual([]);
  });
});

describe('experimentSlice – appendExperimentLog', () => {
  it('appends a log entry to the existing logs', () => {
    const stateWithLogs: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        logs: ['line 1'],
      },
    };

    const state = experimentReducer(stateWithLogs, appendExperimentLog('line 2'));
    expect(state.detail.logs).toEqual(['line 1', 'line 2']);
  });

  it('appends to an empty logs array', () => {
    const state = experimentReducer(initialState, appendExperimentLog('first line'));
    expect(state.detail.logs).toEqual(['first line']);
  });
});

describe('experimentSlice – resetCreateStatus', () => {
  it('resets createStatus to idle and clears createError', () => {
    const state: ExperimentState = {
      ...initialState,
      createStatus: 'failed',
      createError: 'Something went wrong',
    };

    const result = experimentReducer(state, resetCreateStatus());
    expect(result.createStatus).toBe('idle');
    expect(result.createError).toBeNull();
  });
});

describe('experimentSlice – resetExecuteStatus', () => {
  it('resets executeStatus to idle and clears executeError', () => {
    const state: ExperimentState = {
      ...initialState,
      executeStatus: 'failed',
      executeError: 'Execution failed',
    };

    const result = experimentReducer(state, resetExecuteStatus());
    expect(result.executeStatus).toBe('idle');
    expect(result.executeError).toBeNull();
  });
});

describe('experimentSlice – resetStopStatus', () => {
  it('resets stopStatus to idle and clears stopError', () => {
    const state: ExperimentState = {
      ...initialState,
      stopStatus: 'failed',
      stopError: 'Stop failed',
    };

    const result = experimentReducer(state, resetStopStatus());
    expect(result.stopStatus).toBe('idle');
    expect(result.stopError).toBeNull();
  });
});

describe('experimentSlice – resetDeleteStatus', () => {
  it('resets deleteStatus to idle and clears deleteError', () => {
    const state: ExperimentState = {
      ...initialState,
      deleteStatus: 'failed',
      deleteError: 'Delete failed',
    };

    const result = experimentReducer(state, resetDeleteStatus());
    expect(result.deleteStatus).toBe('idle');
    expect(result.deleteError).toBeNull();
  });
});

describe('experimentSlice – updateExperimentStatus', () => {
  it('updates the status of an experiment in the list', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };

    const state = experimentReducer(
      stateWithExperiments,
      updateExperimentStatus({ id: 'exp-1', status: 'running' }),
    );
    expect(state.list.experiments[0].status).toBe('running');
  });

  it('updates the progress of an experiment in the list', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };

    const state = experimentReducer(
      stateWithExperiments,
      updateExperimentStatus({ id: 'exp-1', status: 'running', progress: 75 }),
    );
    expect(state.list.experiments[0].status).toBe('running');
    expect(state.list.experiments[0].progress).toBe(75);
  });

  it('does not modify progress when progress is not provided', () => {
    const stateWithProgress: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [{ ...mockExperiment, progress: 30 }],
      },
    };

    const state = experimentReducer(
      stateWithProgress,
      updateExperimentStatus({ id: 'exp-1', status: 'running' }),
    );
    expect(state.list.experiments[0].status).toBe('running');
    expect(state.list.experiments[0].progress).toBe(30);
  });

  it('updates the detail view if the experiment matches', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        experiment: mockExperiment,
      },
    };

    const state = experimentReducer(
      stateWithDetail,
      updateExperimentStatus({ id: 'exp-1', status: 'completed', progress: 100 }),
    );
    expect(state.detail.experiment?.status).toBe('completed');
    expect(state.detail.experiment?.progress).toBe(100);
  });

  it('does not update detail if the experiment id does not match', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        experiment: mockExperiment,
      },
    };

    const state = experimentReducer(
      stateWithDetail,
      updateExperimentStatus({ id: 'exp-999', status: 'running', progress: 50 }),
    );
    expect(state.detail.experiment?.status).toBe('pending');
  });

  it('does not crash when experiment is not in the list', () => {
    const state = experimentReducer(
      initialState,
      updateExperimentStatus({ id: 'non-existent', status: 'running' }),
    );
    expect(state.list.experiments).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – fetchExperiments
// ---------------------------------------------------------------------------

describe('experimentSlice – fetchExperiments', () => {
  it('sets isLoading to true on pending', () => {
    const state = experimentReducer(initialState, fetchExperiments.pending('', {}));
    expect(state.list.isLoading).toBe(true);
    expect(state.list.error).toBeNull();
  });

  it('updates state on fulfilled with items field', () => {
    const payload = paginatedResponse([mockExperiment, mockExperiment2], 2);
    const action = fetchExperiments.fulfilled(payload, '', {});
    const state = experimentReducer(initialState, action);
    expect(state.list.isLoading).toBe(false);
    expect(state.list.experiments).toEqual([mockExperiment, mockExperiment2]);
    expect(state.list.totalCount).toBe(2);
    expect(state.list.currentPage).toBe(1);
  });

  it('updates state on fulfilled with data field', () => {
    const payload = flexiblePaginatedResponse({
      data: [mockExperiment],
      totalCount: 1,
      page: 2,
      pageSize: 10,
    });
    const action = fetchExperiments.fulfilled(
      payload as unknown as PaginatedResponse<Experiment>,
      '',
      {},
    );
    const state = experimentReducer(initialState, action);
    expect(state.list.experiments).toEqual([mockExperiment]);
    expect(state.list.totalCount).toBe(1);
    expect(state.list.currentPage).toBe(2);
  });

  it('updates state on fulfilled with experiments field', () => {
    const payload = flexiblePaginatedResponse({
      experiments: [mockExperiment],
      total: 5,
      pagination: { page: 3, limit: 10, total: 5 },
    });
    const action = fetchExperiments.fulfilled(
      payload as unknown as PaginatedResponse<Experiment>,
      '',
      {},
    );
    const state = experimentReducer(initialState, action);
    expect(state.list.experiments).toEqual([mockExperiment]);
    expect(state.list.totalCount).toBe(5);
    expect(state.list.currentPage).toBe(3);
  });

  it('handles empty items gracefully', () => {
    const payload = paginatedResponse([], 0);
    const action = fetchExperiments.fulfilled(payload, '', {});
    const state = experimentReducer(initialState, action);
    expect(state.list.experiments).toEqual([]);
    expect(state.list.totalCount).toBe(0);
  });

  it('handles non-array items gracefully', () => {
    const payload = flexiblePaginatedResponse({ items: 'not-an-array' });
    const action = fetchExperiments.fulfilled(payload as never, '', {});
    const state = experimentReducer(initialState, action);
    expect(state.list.experiments).toEqual([]);
  });

  it('sets error on rejected with payload', () => {
    const action = fetchExperiments.rejected(new Error('fail'), '', {}, 'Network error');
    const state = experimentReducer(initialState, action);
    expect(state.list.isLoading).toBe(false);
    expect(state.list.error).toBe('Network error');
  });

  it('sets default error message on rejected without payload', () => {
    const action = fetchExperiments.rejected(new Error('fail'), '', {});
    const state = experimentReducer(initialState, action);
    expect(state.list.error).toBe('Failed to fetch experiments');
  });

  it('preserves existing experiments array when rejected', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };
    const action = fetchExperiments.rejected(new Error('fail'), '', {}, 'Network error');
    const state = experimentReducer(stateWithExperiments, action);
    expect(state.list.experiments).toEqual([mockExperiment]);
  });

  it('integrates fetchExperiments thunk end-to-end – success', async () => {
    const store = createTestStore();
    const response = apiResponse(paginatedResponse([mockExperiment], 1));

    mockedExperimentsAPI.list.mockResolvedValueOnce(response);

    const result = await store.dispatch(fetchExperiments({}));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/fetchList/fulfilled');
    expect(experiments.list.experiments).toHaveLength(1);
    expect(experiments.list.experiments[0].id).toBe('exp-1');
    expect(experiments.list.isLoading).toBe(false);
  });

  it('integrates fetchExperiments thunk end-to-end – failure', async () => {
    const store = createTestStore();

    mockedExperimentsAPI.list.mockRejectedValueOnce(new Error('Network error'));
    mockedGetErrorMessage.mockReturnValue('Network error');

    const result = await store.dispatch(fetchExperiments({}));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/fetchList/rejected');
    expect(experiments.list.error).toBe('Network error');
    expect(experiments.list.isLoading).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – fetchExperimentById
// ---------------------------------------------------------------------------

describe('experimentSlice – fetchExperimentById', () => {
  it('sets isLoading to true on pending', () => {
    const state = experimentReducer(
      initialState,
      fetchExperimentById.pending('', 'exp-1'),
    );
    expect(state.detail.isLoading).toBe(true);
    expect(state.detail.error).toBeNull();
  });

  it('sets experiment on fulfilled', () => {
    const action = fetchExperimentById.fulfilled(mockExperiment, '', 'exp-1');
    const state = experimentReducer(initialState, action);
    expect(state.detail.isLoading).toBe(false);
    expect(state.detail.experiment).toEqual(mockExperiment);
  });

  it('sets error on rejected with payload', () => {
    const action = fetchExperimentById.rejected(
      new Error('fail'),
      '',
      'exp-1',
      'Not found',
    );
    const state = experimentReducer(initialState, action);
    expect(state.detail.isLoading).toBe(false);
    expect(state.detail.error).toBe('Not found');
  });

  it('sets default error message on rejected without payload', () => {
    const action = fetchExperimentById.rejected(new Error('fail'), '', 'exp-1');
    const state = experimentReducer(initialState, action);
    expect(state.detail.error).toBe('Failed to fetch experiment');
  });

  it('integrates fetchExperimentById thunk end-to-end – success', async () => {
    const store = createTestStore();
    mockedExperimentsAPI.getById.mockResolvedValueOnce(
      apiResponse({ data: mockExperiment }),
    );

    const result = await store.dispatch(fetchExperimentById('exp-1'));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/fetchById/fulfilled');
    expect(experiments.detail.experiment).toEqual(mockExperiment);
    expect(experiments.detail.isLoading).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – createExperiment
// ---------------------------------------------------------------------------

describe('experimentSlice – createExperiment', () => {
  it('sets createStatus to loading on pending', () => {
    const state = experimentReducer(
      initialState,
      createExperiment.pending('', mockCreateRequest),
    );
    expect(state.createStatus).toBe('loading');
    expect(state.createError).toBeNull();
  });

  it('prepends new experiment to list on fulfilled', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment2],
        totalCount: 1,
      },
    };

    const newExperiment: Experiment = {
      ...mockExperiment,
      id: 'exp-new',
      name: 'New Experiment',
    };
    const action = createExperiment.fulfilled(newExperiment, '', mockCreateRequest);
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.createStatus).toBe('succeeded');
    expect(state.list.experiments).toHaveLength(2);
    expect(state.list.experiments[0].id).toBe('exp-new');
    expect(state.list.totalCount).toBe(2);
  });

  it('sets createError on rejected with payload', () => {
    const action = createExperiment.rejected(
      new Error('fail'),
      '',
      mockCreateRequest,
      'Creation failed',
    );
    const state = experimentReducer(initialState, action);
    expect(state.createStatus).toBe('failed');
    expect(state.createError).toBe('Creation failed');
  });

  it('sets default createError on rejected without payload', () => {
    const action = createExperiment.rejected(new Error('fail'), '', mockCreateRequest);
    const state = experimentReducer(initialState, action);
    expect(state.createStatus).toBe('failed');
    expect(state.createError).toBe('Failed to create experiment');
  });

  it('integrates createExperiment thunk end-to-end – success', async () => {
    const store = createTestStore();
    const newExperiment: Experiment = {
      ...mockExperiment,
      id: 'exp-new',
      name: 'New Experiment',
    };

    mockedExperimentsAPI.create.mockResolvedValueOnce(
      apiResponse({ data: newExperiment }),
    );

    const result = await store.dispatch(createExperiment(mockCreateRequest));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/create/fulfilled');
    expect(experiments.list.experiments).toHaveLength(1);
    expect(experiments.list.experiments[0].id).toBe('exp-new');
    expect(experiments.createStatus).toBe('succeeded');
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – updateExperiment
// ---------------------------------------------------------------------------

describe('experimentSlice – updateExperiment', () => {
  it('updates the experiment in the list on fulfilled', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };

    const updatedExperiment: Experiment = {
      ...mockExperiment,
      name: 'Updated Name',
      status: 'running',
    };

    const action = updateExperiment.fulfilled(updatedExperiment, '', {
      id: 'exp-1',
      data: { name: 'Updated Name' },
    });
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.list.experiments[0].name).toBe('Updated Name');
    expect(state.list.experiments[0].status).toBe('running');
  });

  it('updates the detail view on fulfilled', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        experiment: mockExperiment,
      },
    };

    const updatedExperiment: Experiment = {
      ...mockExperiment,
      name: 'Updated Name',
    };

    const action = updateExperiment.fulfilled(updatedExperiment, '', {
      id: 'exp-1',
      data: { name: 'Updated Name' },
    });
    const state = experimentReducer(stateWithDetail, action);

    expect(state.detail.experiment?.name).toBe('Updated Name');
  });

  it('does not modify list when experiment id is not found', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };

    const updatedExperiment: Experiment = {
      ...mockExperiment,
      id: 'exp-999',
      name: 'Updated Name',
    };

    const action = updateExperiment.fulfilled(updatedExperiment, '', {
      id: 'exp-999',
      data: { name: 'Updated Name' },
    });
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.list.experiments[0].name).toBe('Network Latency Test');
  });

  it('integrates updateExperiment thunk end-to-end – success', async () => {
    const store = createTestStore({
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    });

    const updatedExperiment: Experiment = { ...mockExperiment, name: 'Updated' };
    mockedExperimentsAPI.update.mockResolvedValueOnce(
      apiResponse({ data: updatedExperiment }),
    );

    const result = await store.dispatch(
      updateExperiment({ id: 'exp-1', data: { name: 'Updated' } }),
    );
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/update/fulfilled');
    expect(experiments.list.experiments[0].name).toBe('Updated');
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – deleteExperiment
// ---------------------------------------------------------------------------

describe('experimentSlice – deleteExperiment', () => {
  it('sets deleteStatus to loading on pending', () => {
    const state = experimentReducer(initialState, deleteExperiment.pending('', 'exp-1'));
    expect(state.deleteStatus).toBe('loading');
    expect(state.deleteError).toBeNull();
  });

  it('removes experiment from list on fulfilled', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment, mockExperiment2],
        totalCount: 2,
      },
    };

    const action = deleteExperiment.fulfilled('exp-1', '', 'exp-1');
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.deleteStatus).toBe('succeeded');
    expect(state.list.experiments).toHaveLength(1);
    expect(state.list.experiments[0].id).toBe('exp-2');
    expect(state.list.totalCount).toBe(1);
  });

  it('does not go below 0 for totalCount', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [],
        totalCount: 0,
      },
    };

    const action = deleteExperiment.fulfilled('exp-1', '', 'exp-1');
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.list.totalCount).toBe(0);
  });

  it('clears detail if it was the deleted experiment', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        experiment: mockExperiment,
      },
    };

    const action = deleteExperiment.fulfilled('exp-1', '', 'exp-1');
    const state = experimentReducer(stateWithDetail, action);

    expect(state.detail.experiment).toBeNull();
  });

  it('does not clear detail if it was a different experiment', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        experiment: mockExperiment,
      },
    };

    const action = deleteExperiment.fulfilled('exp-2', '', 'exp-2');
    const state = experimentReducer(stateWithDetail, action);

    expect(state.detail.experiment).toEqual(mockExperiment);
  });

  it('sets deleteError on rejected with payload', () => {
    const action = deleteExperiment.rejected(
      new Error('fail'),
      '',
      'exp-1',
      'Delete failed',
    );
    const state = experimentReducer(initialState, action);
    expect(state.deleteStatus).toBe('failed');
    expect(state.deleteError).toBe('Delete failed');
  });

  it('sets default deleteError on rejected without payload', () => {
    const action = deleteExperiment.rejected(new Error('fail'), '', 'exp-1');
    const state = experimentReducer(initialState, action);
    expect(state.deleteError).toBe('Failed to delete experiment');
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – executeExperiment
// ---------------------------------------------------------------------------

describe('experimentSlice – executeExperiment', () => {
  it('sets executeStatus to loading on pending', () => {
    const state = experimentReducer(
      initialState,
      executeExperiment.pending('', { id: 'exp-1' }),
    );
    expect(state.executeStatus).toBe('loading');
    expect(state.executeError).toBeNull();
  });

  it('updates experiment status in list on fulfilled', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };

    const run: ExperimentRun = {
      ...mockRun,
      status: 'running',
      progress: 0,
    };

    const action = executeExperiment.fulfilled({ id: 'exp-1', run }, '', { id: 'exp-1' });
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.executeStatus).toBe('succeeded');
    expect(state.list.experiments[0].status).toBe('running');
    expect(state.list.experiments[0].progress).toBe(0);
  });

  it('sets progress to 100 for completed run on fulfilled', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    };

    const run: ExperimentRun = {
      ...mockRun,
      status: 'completed',
      progress: 100,
    };

    const action = executeExperiment.fulfilled({ id: 'exp-1', run }, '', { id: 'exp-1' });
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.list.experiments[0].status).toBe('completed');
    expect(state.list.experiments[0].progress).toBe(100);
  });

  it('updates detail experiment on fulfilled', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        ...initialState.detail,
        experiment: mockExperiment,
      },
    };

    const run: ExperimentRun = {
      ...mockRun,
      status: 'running',
      progress: 0,
    };

    const action = executeExperiment.fulfilled({ id: 'exp-1', run }, '', { id: 'exp-1' });
    const state = experimentReducer(stateWithDetail, action);

    expect(state.detail.experiment?.status).toBe('running');
    expect(state.detail.currentRun?.status).toBe('running');
  });

  it('sets executeError on rejected with payload', () => {
    const action = executeExperiment.rejected(
      new Error('fail'),
      '',
      { id: 'exp-1' },
      'Execution failed',
    );
    const state = experimentReducer(initialState, action);
    expect(state.executeStatus).toBe('failed');
    expect(state.executeError).toBe('Execution failed');
  });

  it('sets default executeError on rejected without payload', () => {
    const action = executeExperiment.rejected(new Error('fail'), '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);
    expect(state.executeError).toBe('Failed to execute experiment');
  });

  it('integrates executeExperiment thunk end-to-end – success', async () => {
    const store = createTestStore({
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
      },
    });

    const run: ExperimentRun = { ...mockRun, status: 'running', progress: 0 };
    mockedExperimentsAPI.execute.mockResolvedValueOnce(apiResponse({ data: run }));

    const result = await store.dispatch(
      executeExperiment({ id: 'exp-1', clusterId: 'cluster-1' }),
    );
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/execute/fulfilled');
    expect(experiments.executeStatus).toBe('succeeded');
    expect(experiments.list.experiments[0].status).toBe('running');
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – stopExperiment
// ---------------------------------------------------------------------------

describe('experimentSlice – stopExperiment', () => {
  it('sets stopStatus to loading on pending', () => {
    const state = experimentReducer(initialState, stopExperiment.pending('', 'exp-1'));
    expect(state.stopStatus).toBe('loading');
    expect(state.stopError).toBeNull();
  });

  it('updates experiment status to stopped on fulfilled', () => {
    const stateWithExperiments: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [{ ...mockExperiment, status: 'running' }],
      },
    };

    const stoppedExperiment: Experiment = {
      ...mockExperiment,
      status: 'stopped',
    };

    const action = stopExperiment.fulfilled(stoppedExperiment, '', 'exp-1');
    const state = experimentReducer(stateWithExperiments, action);

    expect(state.stopStatus).toBe('succeeded');
    expect(state.list.experiments[0].status).toBe('stopped');
    expect(state.executeStatus).toBe('idle');
    expect(state.executeError).toBeNull();
  });

  it('clears currentRun and logs in detail on fulfilled', () => {
    const stateWithDetail: ExperimentState = {
      ...initialState,
      detail: {
        experiment: { ...mockExperiment, status: 'running' },
        currentRun: mockRun,
        logs: ['line 1', 'line 2'],
        isLoading: false,
        error: null,
      },
    };

    const stoppedExperiment: Experiment = {
      ...mockExperiment,
      status: 'stopped',
    };

    const action = stopExperiment.fulfilled(stoppedExperiment, '', 'exp-1');
    const state = experimentReducer(stateWithDetail, action);

    expect(state.detail.currentRun).toBeNull();
    expect(state.detail.logs).toEqual([]);
  });

  it('sets stopError on rejected with payload', () => {
    const action = stopExperiment.rejected(new Error('fail'), '', 'exp-1', 'Stop failed');
    const state = experimentReducer(initialState, action);
    expect(state.stopStatus).toBe('failed');
    expect(state.stopError).toBe('Stop failed');
  });

  it('sets default stopError on rejected without payload', () => {
    const action = stopExperiment.rejected(new Error('fail'), '', 'exp-1');
    const state = experimentReducer(initialState, action);
    expect(state.stopError).toContain('Failed to stop experiment');
  });

  it('integrates stopExperiment thunk end-to-end – success', async () => {
    const store = createTestStore({
      list: {
        ...initialState.list,
        experiments: [{ ...mockExperiment, status: 'running' }],
      },
    });

    const stoppedExperiment: Experiment = {
      ...mockExperiment,
      status: 'stopped',
    };
    mockedExperimentsAPI.stop.mockResolvedValueOnce(
      apiResponse({ data: stoppedExperiment }),
    );

    const result = await store.dispatch(stopExperiment('exp-1'));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/stop/fulfilled');
    expect(experiments.stopStatus).toBe('succeeded');
    expect(experiments.list.experiments[0].status).toBe('stopped');
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – fetchExperimentRuns
// ---------------------------------------------------------------------------

describe('experimentSlice – fetchExperimentRuns', () => {
  it('sets runsLoading to true on pending', () => {
    const state = experimentReducer(
      initialState,
      fetchExperimentRuns.pending('', { id: 'exp-1' }),
    );
    expect(state.runsLoading).toBe(true);
    expect(state.runsError).toBeNull();
  });

  it('sets runs on fulfilled with items field', () => {
    const payload = paginatedResponse([mockRun], 1);
    const action = fetchExperimentRuns.fulfilled(payload, '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);

    expect(state.runsLoading).toBe(false);
    expect(state.runs).toHaveLength(1);
    expect(state.runs[0].id).toBe('run-1');
    expect(state.runsTotalCount).toBe(1);
    expect(state.runsPage).toBe(1);
  });

  it('sets runs on fulfilled with data field', () => {
    const payload = {
      data: [mockRun] as unknown as ExperimentRun[],
      totalCount: 1,
      page: 1,
      pageSize: 10,
      items: [mockRun],
      totalPages: 1,
      hasNextPage: false,
      hasPreviousPage: false,
    };
    const action = fetchExperimentRuns.fulfilled(
      payload as PaginatedResponse<ExperimentRun>,
      '',
      { id: 'exp-1' },
    );
    const state = experimentReducer(initialState, action);

    expect(state.runs).toHaveLength(1);
    expect(state.runsTotalCount).toBe(1);
  });

  it('sets runs on fulfilled with runs field', () => {
    const payload = {
      runs: [mockRun] as unknown as ExperimentRun[],
      total: 5,
      pagination: { page: 2, limit: 10, total: 5 },
      items: [mockRun],
      totalCount: 5,
      page: 2,
      pageSize: 10,
      totalPages: 1,
      hasNextPage: false,
      hasPreviousPage: false,
    };
    const action = fetchExperimentRuns.fulfilled(
      payload as PaginatedResponse<ExperimentRun>,
      '',
      { id: 'exp-1' },
    );
    const state = experimentReducer(initialState, action);

    expect(state.runs).toHaveLength(1);
    expect(state.runsTotalCount).toBe(5);
    expect(state.runsPage).toBe(2);
  });

  it('handles empty runs gracefully', () => {
    const payload = paginatedResponse([], 0);
    const action = fetchExperimentRuns.fulfilled(payload, '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);

    expect(state.runs).toEqual([]);
    expect(state.runsTotalCount).toBe(0);
  });

  it('handles non-array runs gracefully', () => {
    const payload = { items: 'not-an-array' as unknown as ExperimentRun[] };
    const action = fetchExperimentRuns.fulfilled(payload as never, '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);
    expect(state.runs).toEqual([]);
  });

  it('sets runsError on rejected with payload', () => {
    const action = fetchExperimentRuns.rejected(
      new Error('fail'),
      '',
      { id: 'exp-1' },
      'Failed to fetch runs',
    );
    const state = experimentReducer(initialState, action);
    expect(state.runsLoading).toBe(false);
    expect(state.runsError).toBe('Failed to fetch runs');
  });

  it('sets default runsError on rejected without payload', () => {
    const action = fetchExperimentRuns.rejected(new Error('fail'), '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);
    expect(state.runsError).toBe('Failed to fetch runs');
  });

  it('integrates fetchExperimentRuns thunk end-to-end – success', async () => {
    const store = createTestStore();
    mockedExperimentsAPI.getRuns.mockResolvedValueOnce(
      apiResponse(paginatedResponse([mockRun], 1)),
    );

    const result = await store.dispatch(fetchExperimentRuns({ id: 'exp-1' }));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/fetchRuns/fulfilled');
    expect(experiments.runs).toHaveLength(1);
    expect(experiments.runsLoading).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Async Thunks – fetchExperimentLogs
// ---------------------------------------------------------------------------

describe('experimentSlice – fetchExperimentLogs', () => {
  it('sets logs on fulfilled', () => {
    const logs = ['line 1', 'line 2', 'line 3'];
    const action = fetchExperimentLogs.fulfilled(logs, '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);
    expect(state.detail.logs).toEqual(['line 1', 'line 2', 'line 3']);
  });

  it('sets detail error on rejected with payload', () => {
    const action = fetchExperimentLogs.rejected(
      new Error('fail'),
      '',
      { id: 'exp-1' },
      'Failed to fetch logs',
    );
    const state = experimentReducer(initialState, action);
    expect(state.detail.error).toBe('Failed to fetch logs');
  });

  it('sets default error on rejected without payload', () => {
    const action = fetchExperimentLogs.rejected(new Error('fail'), '', { id: 'exp-1' });
    const state = experimentReducer(initialState, action);
    expect(state.detail.error).toBe('Failed to fetch logs');
  });

  it('integrates fetchExperimentLogs thunk end-to-end – success', async () => {
    const store = createTestStore();
    mockedExperimentsAPI.getLogs.mockResolvedValueOnce(
      apiResponse({ data: ['log line 1', 'log line 2'] }),
    );

    const result = await store.dispatch(fetchExperimentLogs({ id: 'exp-1', tail: 100 }));
    const { experiments } = store.getState();

    expect(result.type).toBe('experiments/fetchLogs/fulfilled');
    expect(experiments.detail.logs).toEqual(['log line 1', 'log line 2']);
  });
});

// ---------------------------------------------------------------------------
// Selectors
// ---------------------------------------------------------------------------

describe('experimentSlice – selectors', () => {
  let stateWithExperiments: { experiments: ExperimentState };

  beforeEach(() => {
    stateWithExperiments = {
      experiments: {
        ...initialState,
        list: {
          ...initialState.list,
          experiments: [mockExperiment, mockExperiment2, mockExperiment3],
          totalCount: 3,
          currentPage: 2,
          pageSize: 10,
          isLoading: false,
          error: null,
          filters: {
            search: 'network',
            status: 'running',
            templateId: 'tmpl-1',
            clusterId: null,
            dateFrom: null,
            dateTo: null,
          },
          sortBy: 'name',
          sortOrder: 'asc',
        },
        detail: {
          ...initialState.detail,
          experiment: mockExperiment,
          currentRun: mockRun,
          logs: ['log line'],
        },
        createStatus: 'succeeded',
        createError: null,
        executeStatus: 'idle',
        executeError: null,
        stopStatus: 'idle',
        stopError: null,
        deleteStatus: 'idle',
        deleteError: null,
        runs: [mockRun],
        runsTotalCount: 1,
        runsPage: 1,
        runsLoading: false,
        runsError: null,
      },
    };
  });

  it('selectExperimentList returns the list of experiments', () => {
    const result = selectExperimentList(stateWithExperiments);
    expect(result).toHaveLength(3);
    expect(result[0].id).toBe('exp-1');
  });

  it('selectExperimentListLoading returns loading state', () => {
    expect(selectExperimentListLoading(stateWithExperiments)).toBe(false);
  });

  it('selectExperimentListError returns error', () => {
    expect(selectExperimentListError(stateWithExperiments)).toBeNull();
  });

  it('selectExperimentListTotalCount returns total count', () => {
    expect(selectExperimentListTotalCount(stateWithExperiments)).toBe(3);
  });

  it('selectExperimentListPage returns current page', () => {
    expect(selectExperimentListPage(stateWithExperiments)).toBe(2);
  });

  it('selectExperimentListPageSize returns page size', () => {
    expect(selectExperimentListPageSize(stateWithExperiments)).toBe(10);
  });

  it('selectExperimentFilters returns filters', () => {
    const filters = selectExperimentFilters(stateWithExperiments);
    expect(filters.search).toBe('network');
    expect(filters.status).toBe('running');
    expect(filters.templateId).toBe('tmpl-1');
  });

  it('selectExperimentSort returns sort config', () => {
    const sort = selectExperimentSort(stateWithExperiments);
    expect(sort.sortBy).toBe('name');
    expect(sort.sortOrder).toBe('asc');
  });

  it('selectExperimentSortBy returns sort by field', () => {
    expect(selectExperimentSortBy(stateWithExperiments)).toBe('name');
  });

  it('selectExperimentSortOrder returns sort order', () => {
    expect(selectExperimentSortOrder(stateWithExperiments)).toBe('asc');
  });

  it('selectExperimentDetail returns the experiment', () => {
    expect(selectExperimentDetail(stateWithExperiments)).toEqual(mockExperiment);
  });

  it('selectExperimentDetailLoading returns loading state', () => {
    expect(selectExperimentDetailLoading(stateWithExperiments)).toBe(false);
  });

  it('selectExperimentDetailError returns error', () => {
    expect(selectExperimentDetailError(stateWithExperiments)).toBeNull();
  });

  it('selectCurrentRun returns current run', () => {
    expect(selectCurrentRun(stateWithExperiments)).toEqual(mockRun);
  });

  it('selectExperimentLogs returns logs', () => {
    expect(selectExperimentLogs(stateWithExperiments)).toEqual(['log line']);
  });

  it('selectCreateStatus returns create status', () => {
    expect(selectCreateStatus(stateWithExperiments)).toBe('succeeded');
  });

  it('selectCreateError returns create error', () => {
    expect(selectCreateError(stateWithExperiments)).toBeNull();
  });

  it('selectExecuteStatus returns execute status', () => {
    expect(selectExecuteStatus(stateWithExperiments)).toBe('idle');
  });

  it('selectExecuteError returns execute error', () => {
    expect(selectExecuteError(stateWithExperiments)).toBeNull();
  });

  it('selectStopStatus returns stop status', () => {
    expect(selectStopStatus(stateWithExperiments)).toBe('idle');
  });

  it('selectStopError returns stop error', () => {
    expect(selectStopError(stateWithExperiments)).toBeNull();
  });

  it('selectDeleteStatus returns delete status', () => {
    expect(selectDeleteStatus(stateWithExperiments)).toBe('idle');
  });

  it('selectDeleteError returns delete error', () => {
    expect(selectDeleteError(stateWithExperiments)).toBeNull();
  });

  it('selectExperimentById returns the matching experiment', () => {
    const result = selectExperimentById('exp-2')(stateWithExperiments);
    expect(result?.id).toBe('exp-2');
    expect(result?.name).toBe('Pod Kill Test');
  });

  it('selectExperimentById returns undefined for non-existent id', () => {
    const result = selectExperimentById('non-existent')(stateWithExperiments);
    expect(result).toBeUndefined();
  });

  it('selectRunningExperiments returns only running experiments', () => {
    const result = selectRunningExperiments(stateWithExperiments);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('exp-2');
    expect(result[0].status).toBe('running');
  });

  it('selectRecentExperiments returns experiments sorted by createdAt', () => {
    const result = selectRecentExperiments(2)(stateWithExperiments);
    expect(result).toHaveLength(2);
    expect(result[0].id).toBe('exp-2'); // 2024-01-16
    expect(result[1].id).toBe('exp-1'); // 2024-01-15
  });

  it('selectRecentExperiments respects limit parameter', () => {
    const result = selectRecentExperiments(1)(stateWithExperiments);
    expect(result).toHaveLength(1);
  });

  it('selectRecentExperiments uses default limit of 5', () => {
    const result = selectRecentExperiments()(stateWithExperiments);
    expect(result).toHaveLength(3); // Only 3 experiments in total
  });

  it('selectExperimentStats returns correct statistics', () => {
    const stats = selectExperimentStats(stateWithExperiments);
    expect(stats.total).toBe(3);
    expect(stats.pending).toBe(1);
    expect(stats.running).toBe(1);
    expect(stats.completed).toBe(1);
    expect(stats.failed).toBe(0);
    expect(stats.stopped).toBe(0);
  });
});

describe('experimentSlice – selectors with undefined/null state', () => {
  let emptyState: { experiments: ExperimentState };

  beforeEach(() => {
    emptyState = { experiments: initialState };
  });

  it('selectExperimentList returns empty array for initial state', () => {
    expect(selectExperimentList(emptyState)).toEqual([]);
  });

  it('selectExperimentListLoading returns false for initial state', () => {
    expect(selectExperimentListLoading(emptyState)).toBe(false);
  });

  it('selectExperimentListError returns null for initial state', () => {
    expect(selectExperimentListError(emptyState)).toBeNull();
  });

  it('selectExperimentListTotalCount returns 0 for initial state', () => {
    expect(selectExperimentListTotalCount(emptyState)).toBe(0);
  });

  it('selectExperimentListPage returns 1 for initial state', () => {
    expect(selectExperimentListPage(emptyState)).toBe(1);
  });

  it('selectExperimentListPageSize returns 10 for initial state', () => {
    expect(selectExperimentListPageSize(emptyState)).toBe(10);
  });

  it('selectExperimentDetail returns null for initial state', () => {
    expect(selectExperimentDetail(emptyState)).toBeNull();
  });

  it('selectCurrentRun returns null for initial state', () => {
    expect(selectCurrentRun(emptyState)).toBeNull();
  });

  it('selectExperimentStats returns zeroed stats for initial state', () => {
    const stats = selectExperimentStats(emptyState);
    expect(stats.total).toBe(0);
    expect(stats.pending).toBe(0);
    expect(stats.running).toBe(0);
    expect(stats.completed).toBe(0);
    expect(stats.failed).toBe(0);
    expect(stats.stopped).toBe(0);
  });

  it('selectRunningExperiments returns empty array for initial state', () => {
    expect(selectRunningExperiments(emptyState)).toEqual([]);
  });

  it('selectRecentExperiments returns empty array for initial state', () => {
    expect(selectRecentExperiments()(emptyState)).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Integration flow
// ---------------------------------------------------------------------------

describe('experimentSlice – integration flow', () => {
  it('fetches, filters, paginates, and resets', () => {
    // Start with initial state
    let state = initialState;

    // Apply filters
    state = experimentReducer(
      state,
      setExperimentFilters({ search: 'network', status: 'running' }),
    );
    expect(state.list.filters.search).toBe('network');
    expect(state.list.filters.status).toBe('running');
    expect(state.list.currentPage).toBe(1);

    // Set page
    state = experimentReducer(state, setExperimentPage(2));
    expect(state.list.currentPage).toBe(2);

    // Set page size (resets page to 1)
    state = experimentReducer(state, setExperimentPageSize(25));
    expect(state.list.pageSize).toBe(25);
    expect(state.list.currentPage).toBe(1);

    // Set sort
    state = experimentReducer(
      state,
      setExperimentSort({ sortBy: 'name', sortOrder: 'asc' }),
    );
    expect(state.list.sortBy).toBe('name');
    expect(state.list.sortOrder).toBe('asc');

    // Reset filters (also resets page)
    state = experimentReducer(state, setExperimentPage(3));
    expect(state.list.currentPage).toBe(3);

    state = experimentReducer(state, resetExperimentFilters());
    expect(state.list.filters.search).toBe('');
    expect(state.list.filters.status).toBe('all');
    expect(state.list.currentPage).toBe(1);
  });

  it('transitions through create → execute → stop lifecycle', () => {
    // Start with an experiment in the list
    let state: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
        totalCount: 1,
      },
    };

    // Create a new experiment (pending)
    state = experimentReducer(state, createExperiment.pending('', mockCreateRequest));
    expect(state.createStatus).toBe('loading');

    // Create fulfilled - new experiment prepended to list
    const newExperiment: Experiment = {
      ...mockExperiment,
      id: 'exp-new',
      name: 'New Experiment',
    };
    state = experimentReducer(
      state,
      createExperiment.fulfilled(newExperiment, '', mockCreateRequest),
    );
    expect(state.createStatus).toBe('succeeded');
    expect(state.list.experiments).toHaveLength(2);
    expect(state.list.totalCount).toBe(2);

    // Execute experiment (pending)
    state = experimentReducer(state, executeExperiment.pending('', { id: 'exp-1' }));
    expect(state.executeStatus).toBe('loading');

    // Execute fulfilled
    const run: ExperimentRun = { ...mockRun, status: 'running', progress: 0 };
    state = experimentReducer(
      state,
      executeExperiment.fulfilled({ id: 'exp-1', run }, '', { id: 'exp-1' }),
    );
    expect(state.executeStatus).toBe('succeeded');
    expect(state.list.experiments.find((exp) => exp.id === 'exp-1')?.status).toBe(
      'running',
    );

    // Stop experiment (pending)
    state = experimentReducer(state, stopExperiment.pending('', 'exp-1'));
    expect(state.stopStatus).toBe('loading');

    // Stop fulfilled - status changes to stopped
    const stoppedExperiment: Experiment = { ...mockExperiment, status: 'stopped' };
    state = experimentReducer(
      state,
      stopExperiment.fulfilled(stoppedExperiment, '', 'exp-1'),
    );
    expect(state.stopStatus).toBe('succeeded');
    expect(state.executeStatus).toBe('idle');
  });

  it('transitions through fetch detail → update → delete', () => {
    let state: ExperimentState = {
      ...initialState,
      list: {
        ...initialState.list,
        experiments: [mockExperiment],
        totalCount: 1,
      },
    };

    // Fetch experiment detail (pending)
    state = experimentReducer(state, fetchExperimentById.pending('', 'exp-1'));
    expect(state.detail.isLoading).toBe(true);

    // Fetch experiment detail (fulfilled)
    state = experimentReducer(
      state,
      fetchExperimentById.fulfilled(mockExperiment, '', 'exp-1'),
    );
    expect(state.detail.isLoading).toBe(false);
    expect(state.detail.experiment).toEqual(mockExperiment);

    // Update experiment
    const updatedExperiment: Experiment = {
      ...mockExperiment,
      name: 'Updated Name',
    };
    state = experimentReducer(
      state,
      updateExperiment.fulfilled(updatedExperiment, '', {
        id: 'exp-1',
        data: { name: 'Updated Name' },
      }),
    );
    expect(state.detail.experiment?.name).toBe('Updated Name');
    expect(state.list.experiments[0].name).toBe('Updated Name');

    // Delete experiment
    state = experimentReducer(state, deleteExperiment.fulfilled('exp-1', '', 'exp-1'));
    expect(state.list.experiments).toHaveLength(0);
    expect(state.list.totalCount).toBe(0);
    expect(state.detail.experiment).toBeNull();
  });

  it('resets status flags independently', () => {
    let state: ExperimentState = {
      ...initialState,
      createStatus: 'failed',
      createError: 'Create failed',
      executeStatus: 'failed',
      executeError: 'Execute failed',
      stopStatus: 'failed',
      stopError: 'Stop failed',
      deleteStatus: 'failed',
      deleteError: 'Delete failed',
    };

    // Reset create status only
    state = experimentReducer(state, resetCreateStatus());
    expect(state.createStatus).toBe('idle');
    expect(state.createError).toBeNull();
    expect(state.executeStatus).toBe('failed'); // Unchanged

    // Reset execute status only
    state = experimentReducer(state, resetExecuteStatus());
    expect(state.executeStatus).toBe('idle');
    expect(state.executeError).toBeNull();
    expect(state.stopStatus).toBe('failed'); // Unchanged

    // Reset stop status only
    state = experimentReducer(state, resetStopStatus());
    expect(state.stopStatus).toBe('idle');
    expect(state.stopError).toBeNull();
    expect(state.deleteStatus).toBe('failed'); // Unchanged

    // Reset delete status only
    state = experimentReducer(state, resetDeleteStatus());
    expect(state.deleteStatus).toBe('idle');
    expect(state.deleteError).toBeNull();
  });
});
