import {
  createSlice,
  createAsyncThunk,
  type PayloadAction,
  type createSelector,
} from '@reduxjs/toolkit';
import { experimentsAPI, getErrorMessage } from '@/services/api';
import type {
  Experiment,
  ExperimentRun,
  ExperimentFilters,
  ExperimentListState,
  ExperimentDetailState,
  CreateExperimentRequest,
  PaginatedResponse,
} from '@/types';

// ---------------------------------------------------------------------------
// Async Thunks – Experiment List
// ---------------------------------------------------------------------------

export const fetchExperiments = createAsyncThunk(
  'experiments/fetchList',
  async (
    params: {
      page?: number;
      limit?: number;
      status?: string;
      search?: string;
      clusterId?: string;
      sortBy?: string;
      sortOrder?: 'asc' | 'desc';
    } = {},
    { rejectWithValue },
  ) => {
    try {
      const response = await experimentsAPI.list(params);
      return response.data as unknown as PaginatedResponse<Experiment>;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const fetchExperimentById = createAsyncThunk(
  'experiments/fetchById',
  async (id: string, { rejectWithValue }) => {
    try {
      const response = await experimentsAPI.getById(id);
      return response.data.data as Experiment;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const createExperiment = createAsyncThunk(
  'experiments/create',
  async (payload: CreateExperimentRequest, { rejectWithValue }) => {
    try {
      const response = await experimentsAPI.create(payload);
      return response.data.data as Experiment;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const updateExperiment = createAsyncThunk(
  'experiments/update',
  async (
    { id, data }: { id: string; data: Partial<Experiment> },
    { rejectWithValue },
  ) => {
    try {
      const response = await experimentsAPI.update(id, data);
      return response.data.data as Experiment;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const deleteExperiment = createAsyncThunk(
  'experiments/delete',
  async (id: string, { rejectWithValue }) => {
    try {
      await experimentsAPI.delete(id);
      return id;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const executeExperiment = createAsyncThunk(
  'experiments/execute',
  async (id: string, { rejectWithValue }) => {
    try {
      const response = await experimentsAPI.execute(id);
      return { id, run: response.data.data as ExperimentRun };
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const stopExperiment = createAsyncThunk(
  'experiments/stop',
  async (id: string, { rejectWithValue }) => {
    try {
      const response = await experimentsAPI.stop(id);
      return response.data.data as Experiment;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const fetchExperimentRuns = createAsyncThunk(
  'experiments/fetchRuns',
  async (
    { id, params }: { id: string; params?: { page?: number; limit?: number } },
    { rejectWithValue },
  ) => {
    try {
      const response = await experimentsAPI.getRuns(id, params);
      return response.data as unknown as PaginatedResponse<ExperimentRun>;
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

export const fetchExperimentLogs = createAsyncThunk(
  'experiments/fetchLogs',
  async ({ id, tail }: { id: string; tail?: number }, { rejectWithValue }) => {
    try {
      const response = await experimentsAPI.getLogs(id, { tail });
      return response.data.data as string[];
    } catch (error) {
      return rejectWithValue(getErrorMessage(error));
    }
  },
);

// ---------------------------------------------------------------------------
// Initial State
// ---------------------------------------------------------------------------

const initialFilters: ExperimentFilters = {
  search: '',
  status: 'all',
  templateId: null,
  clusterId: null,
  dateFrom: null,
  dateTo: null,
};

const initialListState: ExperimentListState = {
  experiments: [],
  totalCount: 0,
  currentPage: 1,
  pageSize: 10,
  isLoading: false,
  error: null,
  filters: initialFilters,
  sortBy: 'createdAt',
  sortOrder: 'desc',
};

const initialDetailState: ExperimentDetailState = {
  experiment: null,
  currentRun: null,
  logs: [],
  isLoading: false,
  error: null,
};

export interface ExperimentState {
  list: ExperimentListState;
  detail: ExperimentDetailState;
  createStatus: 'idle' | 'loading' | 'succeeded' | 'failed';
  createError: string | null;
  executeStatus: 'idle' | 'loading' | 'succeeded' | 'failed';
  executeError: string | null;
  stopStatus: 'idle' | 'loading' | 'succeeded' | 'failed';
  stopError: string | null;
  deleteStatus: 'idle' | 'loading' | 'succeeded' | 'failed';
  deleteError: string | null;
  runs: ExperimentRun[];
  runsTotalCount: number;
  runsPage: number;
  runsLoading: boolean;
  runsError: string | null;
}

const initialState: ExperimentState = {
  list: initialListState,
  detail: initialDetailState,
  createStatus: 'idle',
  createError: null,
  executeStatus: 'idle',
  executeError: null,
  stopStatus: 'idle',
  stopError: null,
  deleteStatus: 'idle',
  deleteError: null,
  runs: [],
  runsTotalCount: 0,
  runsPage: 1,
  runsLoading: false,
  runsError: null,
};

// ---------------------------------------------------------------------------
// Slice
// ---------------------------------------------------------------------------

const experimentSlice = createSlice({
  name: 'experiments',
  initialState,
  reducers: {
    // Filter actions
    setExperimentFilters(state, action: PayloadAction<Partial<ExperimentFilters>>) {
      state.list.filters = { ...state.list.filters, ...action.payload };
      state.list.currentPage = 1; // Reset to first page on filter change
    },
    resetExperimentFilters(state) {
      state.list.filters = initialFilters;
      state.list.currentPage = 1;
    },
    setExperimentPage(state, action: PayloadAction<number>) {
      state.list.currentPage = action.payload;
    },
    setExperimentPageSize(state, action: PayloadAction<number>) {
      state.list.pageSize = action.payload;
      state.list.currentPage = 1;
    },
    setExperimentSort(
      state,
      action: PayloadAction<{ sortBy: string; sortOrder: 'asc' | 'desc' }>,
    ) {
      state.list.sortBy = action.payload.sortBy;
      state.list.sortOrder = action.payload.sortOrder;
    },

    // Detail actions
    clearExperimentDetail(state) {
      state.detail = initialDetailState;
    },
    clearExperimentLogs(state) {
      state.detail.logs = [];
    },
    appendExperimentLog(state, action: PayloadAction<string>) {
      state.detail.logs.push(action.payload);
    },

    // Create / Execute / Stop reset actions
    resetCreateStatus(state) {
      state.createStatus = 'idle';
      state.createError = null;
    },
    resetExecuteStatus(state) {
      state.executeStatus = 'idle';
      state.executeError = null;
    },
    resetStopStatus(state) {
      state.stopStatus = 'idle';
      state.stopError = null;
    },
    resetDeleteStatus(state) {
      state.deleteStatus = 'idle';
      state.deleteError = null;
    },

    // Optimistic status updates for real-time feel
    updateExperimentStatus(
      state,
      action: PayloadAction<{
        id: string;
        status: Experiment['status'];
        progress?: number;
      }>,
    ) {
      const { id, status, progress } = action.payload;
      // Update in list
      const listExperiment = state.list.experiments.find((e) => e.id === id);
      if (listExperiment) {
        listExperiment.status = status;
        if (progress !== undefined) {
          listExperiment.progress = progress;
        }
      }
      // Update in detail
      if (state.detail.experiment?.id === id) {
        state.detail.experiment.status = status;
        if (progress !== undefined) {
          state.detail.experiment.progress = progress;
        }
      }
    },
  },

  extraReducers: (builder) => {
    // -----------------------------------------------------------------------
    // fetchExperiments
    // -----------------------------------------------------------------------
    builder
      .addCase(fetchExperiments.pending, (state) => {
        state.list.isLoading = true;
        state.list.error = null;
      })
      .addCase(fetchExperiments.fulfilled, (state, action) => {
        state.list.isLoading = false;
        const payload = action.payload ?? {};
        state.list.experiments = payload.items ?? [];
        state.list.totalCount = payload.totalCount ?? 0;
        state.list.currentPage = payload.page ?? 1;
        state.list.pageSize = payload.pageSize ?? 10;
      })
      .addCase(fetchExperiments.rejected, (state, action) => {
        state.list.isLoading = false;
        state.list.experiments = state.list.experiments ?? [];
        state.list.error = (action.payload as string) ?? 'Failed to fetch experiments';
      });

    // -----------------------------------------------------------------------
    // fetchExperimentById
    // -----------------------------------------------------------------------
    builder
      .addCase(fetchExperimentById.pending, (state) => {
        state.detail.isLoading = true;
        state.detail.error = null;
      })
      .addCase(fetchExperimentById.fulfilled, (state, action) => {
        state.detail.isLoading = false;
        state.detail.experiment = action.payload;
      })
      .addCase(fetchExperimentById.rejected, (state, action) => {
        state.detail.isLoading = false;
        state.detail.error = (action.payload as string) ?? 'Failed to fetch experiment';
      });

    // -----------------------------------------------------------------------
    // createExperiment
    // -----------------------------------------------------------------------
    builder
      .addCase(createExperiment.pending, (state) => {
        state.createStatus = 'loading';
        state.createError = null;
      })
      .addCase(createExperiment.fulfilled, (state, action) => {
        state.createStatus = 'succeeded';
        // Prepend the new experiment to the list
        state.list.experiments.unshift(action.payload);
        state.list.totalCount += 1;
      })
      .addCase(createExperiment.rejected, (state, action) => {
        state.createStatus = 'failed';
        state.createError = (action.payload as string) ?? 'Failed to create experiment';
      });

    // -----------------------------------------------------------------------
    // updateExperiment
    // -----------------------------------------------------------------------
    builder.addCase(updateExperiment.fulfilled, (state, action) => {
      const updated = action.payload;
      // Update in list
      const index = state.list.experiments.findIndex((e) => e.id === updated.id);
      if (index !== -1) {
        state.list.experiments[index] = updated;
      }
      // Update in detail
      if (state.detail.experiment?.id === updated.id) {
        state.detail.experiment = updated;
      }
    });

    // -----------------------------------------------------------------------
    // deleteExperiment
    // -----------------------------------------------------------------------
    builder
      .addCase(deleteExperiment.pending, (state) => {
        state.deleteStatus = 'loading';
        state.deleteError = null;
      })
      .addCase(deleteExperiment.fulfilled, (state, action) => {
        state.deleteStatus = 'succeeded';
        const deletedId = action.payload;
        state.list.experiments = state.list.experiments.filter((e) => e.id !== deletedId);
        state.list.totalCount = Math.max(0, state.list.totalCount - 1);
        // Clear detail if it was the deleted experiment
        if (state.detail.experiment?.id === deletedId) {
          state.detail = initialDetailState;
        }
      })
      .addCase(deleteExperiment.rejected, (state, action) => {
        state.deleteStatus = 'failed';
        state.deleteError = (action.payload as string) ?? 'Failed to delete experiment';
      });

    // -----------------------------------------------------------------------
    // executeExperiment
    // -----------------------------------------------------------------------
    builder
      .addCase(executeExperiment.pending, (state) => {
        state.executeStatus = 'loading';
        state.executeError = null;
      })
      .addCase(executeExperiment.fulfilled, (state, action) => {
        state.executeStatus = 'succeeded';
        const { id, run } = action.payload;
        // Update experiment status in list
        const listExperiment = state.list.experiments.find((e) => e.id === id);
        if (listExperiment) {
          listExperiment.status = 'running';
          listExperiment.progress = 0;
          listExperiment.startedAt = run.startedAt;
        }
        // Update experiment status in detail
        if (state.detail.experiment?.id === id) {
          state.detail.experiment.status = 'running';
          state.detail.experiment.progress = 0;
          state.detail.experiment.startedAt = run.startedAt;
          state.detail.currentRun = run;
        }
      })
      .addCase(executeExperiment.rejected, (state, action) => {
        state.executeStatus = 'failed';
        state.executeError = (action.payload as string) ?? 'Failed to execute experiment';
      });

    // -----------------------------------------------------------------------
    // stopExperiment
    // -----------------------------------------------------------------------
    builder
      .addCase(stopExperiment.pending, (state) => {
        state.stopStatus = 'loading';
        state.stopError = null;
      })
      .addCase(stopExperiment.fulfilled, (state, action) => {
        state.stopStatus = 'succeeded';
        const updated = action.payload;
        // Update in list
        const listExperiment = state.list.experiments.find((e) => e.id === updated.id);
        if (listExperiment) {
          listExperiment.status = updated.status;
          listExperiment.progress = updated.progress;
          listExperiment.completedAt = updated.completedAt;
        }
        // Update in detail
        if (state.detail.experiment?.id === updated.id) {
          state.detail.experiment = updated;
        }
      })
      .addCase(stopExperiment.rejected, (state, action) => {
        state.stopStatus = 'failed';
        state.stopError = (action.payload as string) ?? 'Failed to stop experiment';
      });

    // -----------------------------------------------------------------------
    // fetchExperimentRuns
    // -----------------------------------------------------------------------
    builder
      .addCase(fetchExperimentRuns.pending, (state) => {
        state.runsLoading = true;
        state.runsError = null;
      })
      .addCase(fetchExperimentRuns.fulfilled, (state, action) => {
        state.runsLoading = false;
        state.runs = action.payload.items;
        state.runsTotalCount = action.payload.totalCount;
        state.runsPage = action.payload.page;
      })
      .addCase(fetchExperimentRuns.rejected, (state, action) => {
        state.runsLoading = false;
        state.runsError = (action.payload as string) ?? 'Failed to fetch runs';
      });

    // -----------------------------------------------------------------------
    // fetchExperimentLogs
    // -----------------------------------------------------------------------
    builder
      .addCase(fetchExperimentLogs.fulfilled, (state, action) => {
        state.detail.logs = action.payload;
      })
      .addCase(fetchExperimentLogs.rejected, (state, action) => {
        state.detail.error = (action.payload as string) ?? 'Failed to fetch logs';
      });
  },
});

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

export const {
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
} = experimentSlice.actions;

// ---------------------------------------------------------------------------
// Selectors
// ---------------------------------------------------------------------------

export const selectExperimentList = (state: {
  experiments: ExperimentState;
}): Experiment[] => state.experiments?.list?.experiments ?? [];

export const selectExperimentListLoading = (state: {
  experiments: ExperimentState;
}): boolean => state.experiments?.list?.isLoading ?? false;

export const selectExperimentListError = (state: {
  experiments: ExperimentState;
}): string | null => state.experiments?.list?.error ?? null;

export const selectExperimentListTotalCount = (state: {
  experiments: ExperimentState;
}): number => state.experiments?.list?.totalCount ?? 0;

export const selectExperimentListPage = (state: {
  experiments: ExperimentState;
}): number => state.experiments?.list?.currentPage ?? 1;

export const selectExperimentListPageSize = (state: {
  experiments: ExperimentState;
}): number => state.experiments?.list?.pageSize ?? 10;

export const selectExperimentFilters = (state: {
  experiments: ExperimentState;
}): ExperimentFilters => state.experiments?.list?.filters ?? initialFilters;

export const selectExperimentSort = (state: {
  experiments: ExperimentState;
}): {
  sortBy: string;
  sortOrder: 'asc' | 'desc';
} => ({
  sortBy: state.experiments?.list?.sortBy ?? 'createdAt',
  sortOrder: state.experiments?.list?.sortOrder ?? 'desc',
});

export const selectExperimentDetail = (state: {
  experiments: ExperimentState;
}): Experiment | null => state.experiments?.detail?.experiment ?? null;

export const selectExperimentDetailLoading = (state: {
  experiments: ExperimentState;
}): boolean => state.experiments?.detail?.isLoading ?? false;

export const selectExperimentDetailError = (state: {
  experiments: ExperimentState;
}): string | null => state.experiments?.detail?.error ?? null;

export const selectCurrentRun = (state: {
  experiments: ExperimentState;
}): ExperimentRun | null => state.experiments?.detail?.currentRun ?? null;

export const selectExperimentLogs = (state: { experiments: ExperimentState }): string[] =>
  state.experiments?.detail?.logs ?? [];

export const selectCreateStatus = (state: {
  experiments: ExperimentState;
}): 'idle' | 'loading' | 'succeeded' | 'failed' => state.experiments.createStatus;

export const selectCreateError = (state: {
  experiments: ExperimentState;
}): string | null => state.experiments.createError;

export const selectExecuteStatus = (state: {
  experiments: ExperimentState;
}): 'idle' | 'loading' | 'succeeded' | 'failed' => state.experiments.executeStatus;

export const selectExecuteError = (state: {
  experiments: ExperimentState;
}): string | null => state.experiments.executeError;

export const selectStopStatus = (state: {
  experiments: ExperimentState;
}): 'idle' | 'loading' | 'succeeded' | 'failed' => state.experiments.stopStatus;

export const selectStopError = (state: { experiments: ExperimentState }): string | null =>
  state.experiments.stopError;

export const selectDeleteStatus = (state: {
  experiments: ExperimentState;
}): 'idle' | 'loading' | 'succeeded' | 'failed' => state.experiments.deleteStatus;

export const selectDeleteError = (state: {
  experiments: ExperimentState;
}): string | null => state.experiments.deleteError;

export const selectExperimentById =
  (id: string) =>
  (state: { experiments: ExperimentState }): Experiment | undefined =>
    (state.experiments?.list?.experiments ?? []).find((e) => e.id === id);

export const selectRunningExperiments = (state: {
  experiments: ExperimentState;
}): Experiment[] =>
  (state.experiments?.list?.experiments ?? []).filter((e) => e.status === 'running');

export const selectRecentExperiments =
  (limit: number = 5) =>
  (state: { experiments: ExperimentState }): Experiment[] =>
    [...(state.experiments?.list?.experiments ?? [])]
      .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
      .slice(0, limit);

export const selectExperimentStats = (state: { experiments: ExperimentState }) => {
  const experiments = state.experiments?.list?.experiments ?? [];
  return {
    total: experiments.length,
    pending: experiments.filter((e) => e.status === 'pending').length,
    running: experiments.filter((e) => e.status === 'running').length,
    completed: experiments.filter((e) => e.status === 'completed').length,
    failed: experiments.filter((e) => e.status === 'failed').length,
    stopped: experiments.filter((e) => e.status === 'stopped').length,
  };
};

// ---------------------------------------------------------------------------
// Reducer
// ---------------------------------------------------------------------------

export default experimentSlice.reducer;
