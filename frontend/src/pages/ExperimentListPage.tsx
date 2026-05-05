import {
  Search as SearchIcon,
  Add as AddIcon,
  FilterList as FilterIcon,
  Refresh as RefreshIcon,
  Visibility as ViewIcon,
  PlayArrow as RunIcon,
  Stop as StopIcon,
  Delete as DeleteIcon,
  Clear as ClearIcon,
  Science as ScienceIcon,
  CleaningServices as CleanIcon,
} from '@mui/icons-material';
import {
  Box,
  Typography,
  Button,
  TextField,
  InputAdornment,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  TableSortLabel,
  Paper,
  MenuItem,
  Select,
  FormControl,
  InputLabel,
  Chip,
  Stack,
  IconButton,
  Tooltip,
  Skeleton,
  Alert,
  Card,
  CardContent,
  Grid,
  type SelectChangeEvent,
} from '@mui/material';
import React, { useEffect, useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import StatusBadge from '@/components/StatusBadge';
import { experimentsAPI } from '@/services/api';
import { useAppDispatch, useAppSelector, type RootState } from '@/store';
import {
  fetchExperiments,
  setExperimentFilters,
  resetExperimentFilters,
  setExperimentPage,
  setExperimentPageSize,
  setExperimentSort,
  selectExperimentList,
  selectExperimentListLoading,
  selectExperimentListError,
  selectExperimentListTotalCount,
  selectExperimentListPage,
  selectExperimentListPageSize,
  selectExperimentFilters,
  selectExperimentSortBy,
  selectExperimentSortOrder,
  executeExperiment,
  stopExperiment,
  resetExecuteStatus,
  resetStopStatus,
  selectExecuteError,
} from '@/store/experimentSlice';
import type { Experiment, ExperimentStatus } from '@/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STATUS_OPTIONS: { value: ExperimentStatus | 'all'; label: string }[] = [
  { value: 'all', label: 'All Statuses' },
  { value: 'draft', label: 'Draft' },
  { value: 'active', label: 'Active' },
  { value: 'pending', label: 'Pending' },
  { value: 'queued', label: 'Queued' },
  { value: 'running', label: 'Running' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
  { value: 'stopped', label: 'Stopped' },
  { value: 'timed_out', label: 'Timed Out' },
  { value: 'archived', label: 'Archived' },
];

const SORT_COLUMNS = [
  { id: 'name', label: 'Name' },
  { id: 'templateName', label: 'Template' },
  { id: 'status', label: 'Outcome' },
  { id: 'clusterName', label: 'Cluster' },
  { id: 'createdAt', label: 'Created' },
  { id: 'startedAt', label: 'Started' },
];

const PAGE_SIZE_OPTIONS = [5, 10, 25, 50];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const formatRelativeTime = (dateStr: string | undefined | null): string => {
  if (!dateStr) return '—';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSeconds = Math.floor(diffMs / 1000);
  const diffMinutes = Math.floor(diffSeconds / 60);
  const diffHours = Math.floor(diffMinutes / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSeconds < 60) return 'Just now';
  if (diffMinutes < 60) return `${diffMinutes}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
  });
};

const formatDuration = (ms: number | undefined): string => {
  if (ms === undefined || ms === null) return '—';
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainingSeconds}s`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours}h ${remainingMinutes}m`;
};

const getOutcomeLabel = (experiment: Experiment): string => {
  if (experiment.result) {
    return experiment.result.success ? 'Passed' : 'Failed';
  }
  if (experiment.status === 'running') return 'In progress';
  if (experiment.status === 'queued' || experiment.status === 'pending') return 'Not run';
  if (experiment.status === 'completed') return 'Completed';
  if (experiment.status === 'failed') return 'Error';
  if (experiment.status === 'stopped') return 'Stopped';
  if (experiment.status === 'timed_out') return 'Timeout';
  return '—';
};

const getOutcomeColor = (
  experiment: Experiment,
): 'success' | 'error' | 'warning' | 'default' => {
  if (experiment.result) {
    return experiment.result.success ? 'success' : 'error';
  }
  if (experiment.status === 'completed') return 'success';
  if (experiment.status === 'failed') return 'error';
  if (experiment.status === 'running') return 'warning';
  if (experiment.status === 'stopped' || experiment.status === 'timed_out')
    return 'warning';
  return 'default';
};

const canRunExperiment = (status: ExperimentStatus): boolean =>
  [
    'draft',
    'active',
    'pending',
    'queued',
    'completed',
    'failed',
    'stopped',
    'timed_out',
    'archived',
  ].includes(status);

const canStopExperiment = (status: ExperimentStatus): boolean =>
  ['running', 'queued', 'pending'].includes(status);

// ---------------------------------------------------------------------------
// Skeleton Loader
// ---------------------------------------------------------------------------

const TableSkeleton: React.FC<{ rows?: number; columns?: number }> = ({
  rows = 5,
  columns = 6,
}) => (
  <>
    {Array.from({ length: rows }).map((_, rowIdx) => (
      <TableRow key={`skeleton-${rowIdx}`}>
        {Array.from({ length: columns }).map((_, colIdx) => (
          <TableCell key={`skeleton-${rowIdx}-${colIdx}`}>
            <Skeleton
              variant="rounded"
              height={20}
              width={colIdx === 0 ? '70%' : '50%'}
              animation="wave"
            />
          </TableCell>
        ))}
      </TableRow>
    ))}
  </>
);

// ---------------------------------------------------------------------------
// Experiment Row Component
// ---------------------------------------------------------------------------

interface ExperimentRowProps {
  experiment: Experiment;
  onView: (id: string) => void;
  onRun: (experiment: Experiment) => void;
  onStop: (id: string) => void;
  executing: boolean;
  stopping: boolean;
}

const ExperimentRow: React.FC<ExperimentRowProps> = React.memo(
  ({ experiment, onView, onRun, onStop, executing, stopping }) => {
    const navigate = useNavigate();

    const handleClick = (): void => {
      onView(experiment.id);
    };

    return (
      <TableRow
        hover
        onClick={handleClick}
        sx={{
          cursor: 'pointer',
          transition: 'background-color 150ms cubic-bezier(0.4, 0, 0.2, 1)',
          '&:hover': {
            backgroundColor: 'action.hover',
          },
        }}
      >
        {/* Name */}
        <TableCell>
          <Stack direction="row" spacing={1} alignItems="center">
            <ScienceIcon sx={{ fontSize: 18, color: 'primary.main', flexShrink: 0 }} />
            <Box sx={{ minWidth: 0 }}>
              <Typography
                variant="body2"
                fontWeight={600}
                noWrap
                sx={{ lineHeight: 1.3 }}
              >
                {experiment.name}
              </Typography>
              {experiment.description && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  noWrap
                  sx={{ display: 'block', maxWidth: 240 }}
                >
                  {experiment.description}
                </Typography>
              )}
            </Box>
          </Stack>
        </TableCell>

        {/* Template */}
        <TableCell>
          <Typography variant="body2" noWrap>
            {experiment.templateName ?? 'Custom'}
          </Typography>
        </TableCell>

        {/* Outcome */}
        <TableCell>
          {experiment.status === 'running' ? (
            <Typography variant="body2" color="warning.main" fontWeight={500}>
              In progress {experiment.progress}%
            </Typography>
          ) : experiment.status === 'pending' || experiment.status === 'queued' ? (
            <Typography variant="body2" color="text.disabled">
              Not run
            </Typography>
          ) : (
            <Stack direction="row" spacing={0.5} alignItems="center">
              <Chip
                label={getOutcomeLabel(experiment)}
                size="small"
                color={getOutcomeColor(experiment)}
                variant="outlined"
                sx={{
                  height: 22,
                  fontSize: '0.6875rem',
                  fontWeight: 600,
                }}
              />
              {experiment.duration !== undefined && (
                <Typography variant="caption" color="text.secondary">
                  {formatDuration(experiment.duration)}
                </Typography>
              )}
            </Stack>
          )}
        </TableCell>

        {/* Cluster */}
        <TableCell>
          <Typography variant="body2" noWrap>
            {experiment.clusterName ?? experiment.clusterId}
          </Typography>
        </TableCell>

        {/* Created */}
        <TableCell>
          <Typography variant="body2" color="text.secondary" noWrap>
            {formatRelativeTime(experiment.createdAt)}
          </Typography>
        </TableCell>

        {/* Started */}
        <TableCell>
          <Typography variant="body2" color="text.secondary" noWrap>
            {experiment.startedAt ? formatRelativeTime(experiment.startedAt) : '—'}
          </Typography>
        </TableCell>

        {/* Actions */}
        <TableCell align="right">
          <Stack direction="row" spacing={0.25} justifyContent="flex-end">
            <Tooltip title="View Details">
              <IconButton
                size="small"
                onClick={(e) => {
                  e.stopPropagation();
                  navigate(`/experiments/${experiment.id}`);
                }}
              >
                <ViewIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            {canRunExperiment(experiment.status) && (
              <Tooltip title="Run Experiment">
                <IconButton
                  size="small"
                  color="primary"
                  disabled={executing}
                  onClick={(e) => {
                    e.stopPropagation();
                    onRun(experiment);
                  }}
                >
                  <RunIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            )}
            {canStopExperiment(experiment.status) && (
              <Tooltip title="Stop Experiment">
                <IconButton
                  size="small"
                  color="error"
                  disabled={stopping}
                  onClick={(e) => {
                    e.stopPropagation();
                    onStop(experiment.id);
                  }}
                >
                  <StopIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            )}
          </Stack>
        </TableCell>
      </TableRow>
    );
  },
);

ExperimentRow.displayName = 'ExperimentRow';

// ---------------------------------------------------------------------------
// Stats Summary Cards
// ---------------------------------------------------------------------------

interface StatCardProps {
  label: string;
  value: number;
  color: string;
  icon: React.ReactNode;
}

const StatCard: React.FC<StatCardProps> = ({ label, value, color, icon }) => (
  <Card
    elevation={0}
    sx={{
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 2,
      height: '100%',
    }}
  >
    <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
      <Stack direction="row" spacing={1.5} alignItems="center">
        <Box
          sx={{
            width: 40,
            height: 40,
            borderRadius: 1.5,
            backgroundColor: `${color}14`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
          }}
        >
          {icon}
        </Box>
        <Box>
          <Typography variant="h5" fontWeight={700} sx={{ lineHeight: 1.2, color }}>
            {value}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
            {label}
          </Typography>
        </Box>
      </Stack>
    </CardContent>
  </Card>
);

// ---------------------------------------------------------------------------
// Main Page Component
// ---------------------------------------------------------------------------

const ExperimentListPage: React.FC = () => {
  const dispatch = useAppDispatch();
  const navigate = useNavigate();

  // -----------------------------------------------------------------------
  // Redux State
  // -----------------------------------------------------------------------

  const experiments = useAppSelector(selectExperimentList);
  const isLoading = useAppSelector(selectExperimentListLoading);
  const error = useAppSelector(selectExperimentListError);
  const totalCount = useAppSelector(selectExperimentListTotalCount);
  const page = useAppSelector(selectExperimentListPage);
  const pageSize = useAppSelector(selectExperimentListPageSize);
  const filters = useAppSelector(selectExperimentFilters);
  const sortBy = useAppSelector(selectExperimentSortBy);
  const sortOrder = useAppSelector(selectExperimentSortOrder);
  const executeStatus = useAppSelector(
    (state: RootState) => state.experiments.executeStatus,
  );
  const executeError = useAppSelector(selectExecuteError);
  const stopStatus = useAppSelector((state: RootState) => state.experiments.stopStatus);
  const stopError = useAppSelector((state: RootState) => state.experiments.stopError);

  // -----------------------------------------------------------------------
  // Local State
  // -----------------------------------------------------------------------

  const [searchInput, setSearchInput] = useState(filters.search);
  const [showFilters, setShowFilters] = useState(false);
  const [actionExperimentId, setActionExperimentId] = useState<string | null>(null);

  // -----------------------------------------------------------------------
  // Computed Values
  // -----------------------------------------------------------------------

  const stats = useMemo(() => {
    // Per-status counts are from the current page only, not the full dataset
    const visible = experiments;
    return {
      total: totalCount,
      pending: visible.filter((e) => e.status === 'pending' || e.status === 'queued')
        .length,
      running: visible.filter((e) => e.status === 'running').length,
      completed: visible.filter((e) => e.status === 'completed').length,
      failed: visible.filter((e) => e.status === 'failed').length,
    };
  }, [experiments, totalCount]);

  const totalPages = Math.ceil(totalCount / pageSize);

  // -----------------------------------------------------------------------
  // Effects
  // -----------------------------------------------------------------------

  // Clear any stale action alerts when entering/leaving the page.
  useEffect(() => {
    dispatch(resetExecuteStatus());
    dispatch(resetStopStatus());
    setActionExperimentId(null);

    return () => {
      dispatch(resetExecuteStatus());
      dispatch(resetStopStatus());
    };
  }, [dispatch]);

  useEffect(() => {
    dispatch(
      fetchExperiments({
        page,
        limit: pageSize,
        status: filters.status === 'all' ? undefined : filters.status,
        search: filters.search || undefined,
        clusterId: filters.clusterId ?? undefined,
        sortBy,
        sortOrder,
      }),
    );
  }, [dispatch, page, pageSize, filters, sortBy, sortOrder]);

  // Reset action status after completion and refresh list on success
  useEffect(() => {
    if (executeStatus === 'succeeded') {
      dispatch(
        fetchExperiments({
          page,
          limit: pageSize,
          status: filters.status === 'all' ? undefined : filters.status,
          search: filters.search || undefined,
          clusterId: filters.clusterId ?? undefined,
          sortBy,
          sortOrder,
        }),
      );
      const timer = setTimeout(() => {
        dispatch(resetExecuteStatus());
        setActionExperimentId(null);
      }, 3000);
      return () => clearTimeout(timer);
    }
    if (executeStatus === 'failed') {
      const timer = setTimeout(() => {
        dispatch(resetExecuteStatus());
        setActionExperimentId(null);
      }, 3000);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [executeStatus, dispatch, page, pageSize, filters, sortBy, sortOrder]);

  useEffect(() => {
    if (stopStatus === 'succeeded' || stopStatus === 'failed') {
      const timer = setTimeout(() => {
        dispatch(resetStopStatus());
        setActionExperimentId(null);
      }, 3000);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [stopStatus, dispatch]);

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const handleSearchChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setSearchInput(event.target.value);
  }, []);

  const handleSearchSubmit = useCallback(() => {
    dispatch(setExperimentFilters({ search: searchInput }));
    dispatch(setExperimentPage(1));
  }, [dispatch, searchInput]);

  const handleSearchKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (event.key === 'Enter') {
        handleSearchSubmit();
      }
    },
    [handleSearchSubmit],
  );

  const handleClearSearch = useCallback(() => {
    setSearchInput('');
    dispatch(setExperimentFilters({ search: '' }));
    dispatch(setExperimentPage(1));
  }, [dispatch]);

  const handleStatusFilterChange = useCallback(
    (event: SelectChangeEvent<ExperimentStatus | 'all'>) => {
      dispatch(
        setExperimentFilters({ status: event.target.value as ExperimentStatus | 'all' }),
      );
      dispatch(setExperimentPage(1));
    },
    [dispatch],
  );

  const handleClearFilters = useCallback(() => {
    setSearchInput('');
    dispatch(resetExperimentFilters());
    dispatch(setExperimentPage(1));
  }, [dispatch]);

  const handleSortChange = useCallback(
    (column: string) => {
      const isCurrentSortColumn = sortBy === column;
      const newOrder = isCurrentSortColumn && sortOrder === 'asc' ? 'desc' : 'asc';
      dispatch(setExperimentSort({ sortBy: column, sortOrder: newOrder }));
      dispatch(setExperimentPage(1));
    },
    [dispatch, sortBy, sortOrder],
  );

  const handlePageChange = useCallback(
    (_event: React.MouseEvent<HTMLButtonElement> | null, newPage: number) => {
      dispatch(setExperimentPage(newPage + 1)); // MUI uses 0-based, API uses 1-based
    },
    [dispatch],
  );

  const handlePageSizeChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      dispatch(setExperimentPageSize(parseInt(event.target.value, 10)));
      dispatch(setExperimentPage(1));
    },
    [dispatch],
  );

  const handleRefresh = useCallback(() => {
    dispatch(
      fetchExperiments({
        page,
        limit: pageSize,
        status: filters.status === 'all' ? undefined : filters.status,
        search: filters.search || undefined,
        clusterId: filters.clusterId ?? undefined,
        sortBy,
        sortOrder,
      }),
    );
  }, [dispatch, page, pageSize, filters, sortBy, sortOrder]);

  const handleViewExperiment = useCallback(
    (id: string) => {
      navigate(`/experiments/${id}`);
    },
    [navigate],
  );

  const handleRunExperiment = useCallback(
    (experiment: Experiment) => {
      setActionExperimentId(experiment.id);
      dispatch(resetExecuteStatus());
      dispatch(executeExperiment({ id: experiment.id, clusterId: experiment.clusterId }));
    },
    [dispatch],
  );

  const handleStopExperiment = useCallback(
    async (id: string) => {
      setActionExperimentId(id);
      await dispatch(stopExperiment(id));
    },
    [dispatch],
  );

  const handleNewExperiment = useCallback(() => {
    navigate('/experiments/new');
  }, [navigate]);

  // -----------------------------------------------------------------------
  // Render
  // -----------------------------------------------------------------------

  return (
    <Box>
      {/* Page Header */}
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        justifyContent="space-between"
        alignItems={{ xs: 'flex-start', sm: 'center' }}
        spacing={2}
        mb={3}
      >
        <Box>
          <Typography variant="h4" fontWeight={700} gutterBottom>
            Experiments
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Manage and monitor your security control validation experiments.
          </Typography>
        </Box>

        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleNewExperiment}
          sx={{ borderRadius: 2, px: 3 }}
        >
          New Experiment
        </Button>
      </Stack>

      {/* Stats Summary */}
      <Grid container spacing={1.5} mb={3}>
        <Grid item xs={6} sm={4} md={2.4}>
          <StatCard
            label="Total"
            value={stats.total}
            color="#2563EB"
            icon={<ScienceIcon sx={{ fontSize: 20, color: '#2563EB' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={4} md={2.4}>
          <StatCard
            label="Pending"
            value={stats.pending}
            color="#8B5CF6"
            icon={<FilterIcon sx={{ fontSize: 20, color: '#8B5CF6' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={4} md={2.4}>
          <StatCard
            label="Running"
            value={stats.running}
            color="#2563EB"
            icon={<RunIcon sx={{ fontSize: 20, color: '#2563EB' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={4} md={2.4}>
          <StatCard
            label="Completed"
            value={stats.completed}
            color="#10B981"
            icon={<ViewIcon sx={{ fontSize: 20, color: '#10B981' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={4} md={2.4}>
          <StatCard
            label="Failed"
            value={stats.failed}
            color="#EF4444"
            icon={<DeleteIcon sx={{ fontSize: 20, color: '#EF4444' }} />}
          />
        </Grid>
      </Grid>

      {/* Error Alert */}
      {error && (
        <Alert severity="error" onClose={handleRefresh} sx={{ mb: 2, borderRadius: 2 }}>
          {error}
        </Alert>
      )}

      {/* Execute/Stop Status Alert */}
      {executeStatus === 'succeeded' && actionExperimentId && (
        <Alert severity="success" sx={{ mb: 2, borderRadius: 2 }}>
          Experiment started successfully. Results will appear when the run completes.
        </Alert>
      )}
      {executeStatus === 'failed' && (
        <Alert
          severity={executeError?.includes('concurrency_limit') ? 'warning' : 'error'}
          onClose={() => dispatch(resetExecuteStatus())}
          sx={{ mb: 2, borderRadius: 2 }}
        >
          {executeError?.includes('concurrency_limit') ? (
            <>
              {executeError}
              <br />
              <strong>Tip:</strong> Stop experiments that are no longer needed, or wait
              for running experiments to complete.{' '}
              <Button
                size="small"
                variant="text"
                sx={{
                  textTransform: 'none',
                  fontWeight: 600,
                  p: 0,
                  minWidth: 0,
                  verticalAlign: 'baseline',
                }}
                onClick={() => {
                  dispatch(setExperimentFilters({ status: 'running' }));
                  dispatch(setExperimentPage(1));
                }}
              >
                View active runs →
              </Button>
              <Button
                size="small"
                variant="outlined"
                color="warning"
                startIcon={<CleanIcon />}
                sx={{ textTransform: 'none', fontWeight: 600, ml: 1 }}
                onClick={async () => {
                  try {
                    const res = await experimentsAPI.cancelStaleRuns();
                    const count = (res.data?.data as any)?.cancelled_count ?? 0;
                    if (count > 0) {
                      dispatch(resetExecuteStatus());
                      await dispatch(
                        fetchExperiments({
                          page: experiments.length > 0 ? 1 : 1,
                          limit: 10,
                        }),
                      );
                    }
                    alert(
                      count > 0
                        ? `Cancelled ${count} stale run${count === 1 ? '' : 's'}. You can now try running your experiment again.`
                        : 'No stale runs found. All active runs are currently in progress.',
                    );
                  } catch {
                    alert(
                      'Failed to cancel stale runs. Please try again or contact your administrator.',
                    );
                  }
                }}
              >
                Cancel stale runs
              </Button>
            </>
          ) : (
            executeError || 'Failed to start experiment. Please try again.'
          )}
        </Alert>
      )}
      {stopStatus === 'succeeded' && actionExperimentId && (
        <Alert severity="success" sx={{ mb: 2, borderRadius: 2 }}>
          Experiment stopped successfully.
        </Alert>
      )}
      {stopStatus === 'failed' && (
        <Alert severity="error" sx={{ mb: 2, borderRadius: 2 }}>
          {stopError || 'Failed to stop experiment. Please refresh and try again.'}
        </Alert>
      )}

      {/* Search & Filter Bar */}
      <Paper
        elevation={0}
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
          mb: 2,
          overflow: 'hidden',
        }}
      >
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={1.5}
          sx={{ p: 2 }}
          alignItems={{ xs: 'stretch', md: 'center' }}
        >
          {/* Search Field */}
          <TextField
            size="small"
            placeholder="Search experiments by name..."
            value={searchInput}
            onChange={handleSearchChange}
            onKeyDown={handleSearchKeyDown}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                </InputAdornment>
              ),
              endAdornment: searchInput ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={handleClearSearch}>
                    <ClearIcon sx={{ fontSize: 16 }} />
                  </IconButton>
                </InputAdornment>
              ) : null,
            }}
            sx={{
              flex: 1,
              minWidth: { xs: '100%', md: 280 },
              '& .MuiOutlinedInput-root': {
                borderRadius: 1.5,
              },
            }}
          />

          {/* Status Filter */}
          <FormControl size="small" sx={{ minWidth: 160 }}>
            <InputLabel id="status-filter-label">Status</InputLabel>
            <Select
              labelId="status-filter-label"
              value={filters.status}
              label="Status"
              onChange={handleStatusFilterChange}
              sx={{ borderRadius: 1.5 }}
            >
              {STATUS_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    {option.value !== 'all' && (
                      <StatusBadge
                        status={option.value}
                        variant="dot"
                        size="small"
                        showLabel={false}
                      />
                    )}
                    <span>{option.label}</span>
                  </Stack>
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Filter Toggle */}
          <Tooltip title={showFilters ? 'Hide filters' : 'Show more filters'}>
            <IconButton
              onClick={() => setShowFilters(!showFilters)}
              color={showFilters ? 'primary' : 'default'}
              sx={{
                border: '1px solid',
                borderColor: showFilters ? 'primary.main' : 'divider',
                borderRadius: 1.5,
              }}
            >
              <FilterIcon fontSize="small" />
            </IconButton>
          </Tooltip>

          {/* Refresh */}
          <Tooltip title="Refresh">
            <IconButton
              onClick={handleRefresh}
              sx={{
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1.5,
              }}
            >
              <RefreshIcon fontSize="small" />
            </IconButton>
          </Tooltip>

          {/* Clear Filters */}
          {(filters.search || filters.status !== 'all' || filters.clusterId) && (
            <Button
              size="small"
              variant="text"
              startIcon={<ClearIcon />}
              onClick={handleClearFilters}
              sx={{ textTransform: 'none', whiteSpace: 'nowrap' }}
            >
              Clear all
            </Button>
          )}
        </Stack>

        {/* Expanded Filter Section */}
        {showFilters && (
          <Box
            sx={{
              px: 2,
              pb: 2,
              pt: 0,
              borderTop: '1px solid',
              borderColor: 'divider',
            }}
          >
            <Grid container spacing={2} sx={{ mt: 0 }}>
              <Grid item xs={12} sm={6} md={3}>
                <FormControl size="small" fullWidth>
                  <InputLabel>Cluster</InputLabel>
                  <Select
                    value={filters.clusterId ?? ''}
                    label="Cluster"
                    onChange={(e) => {
                      dispatch(
                        setExperimentFilters({
                          clusterId: e.target.value || null,
                        }),
                      );
                      dispatch(setExperimentPage(1));
                    }}
                  >
                    <MenuItem value="">All Clusters</MenuItem>
                    {/* In production, this would come from the cluster slice */}
                    <MenuItem value="cluster-1">prod-us-east-1</MenuItem>
                    <MenuItem value="cluster-2">staging-eu-west-1</MenuItem>
                    <MenuItem value="cluster-3">dev-local</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <FormControl size="small" fullWidth>
                  <InputLabel>Template</InputLabel>
                  <Select
                    value={filters.templateId ?? ''}
                    label="Template"
                    onChange={(e) => {
                      dispatch(
                        setExperimentFilters({
                          templateId: e.target.value || null,
                        }),
                      );
                      dispatch(setExperimentPage(1));
                    }}
                  >
                    <MenuItem value="">All Templates</MenuItem>
                    <MenuItem value="tmpl-1">DNS Exfiltration</MenuItem>
                    <MenuItem value="tmpl-2">Brute Force</MenuItem>
                    <MenuItem value="tmpl-3">Pod Kill</MenuItem>
                    <MenuItem value="tmpl-4">Network Partition</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <TextField
                  size="small"
                  label="From Date"
                  type="date"
                  fullWidth
                  InputLabelProps={{ shrink: true }}
                  value={filters.dateFrom ?? ''}
                  onChange={(e) => {
                    dispatch(
                      setExperimentFilters({
                        dateFrom: e.target.value || null,
                      }),
                    );
                    dispatch(setExperimentPage(1));
                  }}
                />
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <TextField
                  size="small"
                  label="To Date"
                  type="date"
                  fullWidth
                  InputLabelProps={{ shrink: true }}
                  value={filters.dateTo ?? ''}
                  onChange={(e) => {
                    dispatch(
                      setExperimentFilters({
                        dateTo: e.target.value || null,
                      }),
                    );
                    dispatch(setExperimentPage(1));
                  }}
                />
              </Grid>
            </Grid>
          </Box>
        )}
      </Paper>

      {/* Active Filters Chips */}
      {(filters.search || filters.status !== 'all') && (
        <Stack direction="row" spacing={1} mb={2} flexWrap="wrap" useFlexGap>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ lineHeight: '26px' }}
          >
            Active filters:
          </Typography>
          {filters.search && (
            <Chip
              label={`Search: "${filters.search}"`}
              size="small"
              onDelete={() => {
                setSearchInput('');
                dispatch(setExperimentFilters({ search: '' }));
                dispatch(setExperimentPage(1));
              }}
              sx={{ height: 26 }}
            />
          )}
          {filters.status !== 'all' && (
            <Chip
              label={`Status: ${filters.status}`}
              size="small"
              onDelete={() => {
                dispatch(setExperimentFilters({ status: 'all' }));
                dispatch(setExperimentPage(1));
              }}
              sx={{ height: 26 }}
            />
          )}
          {filters.clusterId && (
            <Chip
              label={`Cluster: ${filters.clusterId}`}
              size="small"
              onDelete={() => {
                dispatch(setExperimentFilters({ clusterId: null }));
                dispatch(setExperimentPage(1));
              }}
              sx={{ height: 26 }}
            />
          )}
        </Stack>
      )}

      {/* Experiments Table */}
      <Paper
        elevation={0}
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
          overflow: 'hidden',
        }}
      >
        {/* Results Summary Header */}
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          sx={{ px: 2, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}
        >
          <Typography variant="body2" color="text.secondary">
            {isLoading
              ? 'Loading experiments...'
              : `${totalCount} experiment${totalCount !== 1 ? 's' : ''} found`}
          </Typography>
          <Typography variant="caption" color="text.disabled">
            Page {page} of {totalPages || 1}
          </Typography>
        </Stack>

        <TableContainer>
          <Table stickyHeader aria-label="experiments table">
            <TableHead>
              <TableRow>
                {SORT_COLUMNS.map((column) => (
                  <TableCell key={column.id}>
                    <TableSortLabel
                      active={sortBy === column.id}
                      direction={sortBy === column.id ? sortOrder : 'asc'}
                      onClick={() => handleSortChange(column.id)}
                      sx={{
                        '&.Mui-active': {
                          color: 'primary.main',
                        },
                        '& .MuiTableSortLabel-icon': {
                          color: 'primary.main !important',
                        },
                      }}
                    >
                      <Typography variant="inherit" fontWeight={600} fontSize="inherit">
                        {column.label}
                      </Typography>
                    </TableSortLabel>
                  </TableCell>
                ))}
                <TableCell align="right" sx={{ whiteSpace: 'nowrap' }}>
                  <Typography variant="inherit" fontWeight={600} fontSize="inherit">
                    Actions
                  </Typography>
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {isLoading ? (
                <TableSkeleton rows={pageSize} columns={SORT_COLUMNS.length + 1} />
              ) : experiments.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={SORT_COLUMNS.length + 1} align="center">
                    <Box sx={{ py: 6, px: 2, textAlign: 'center' }}>
                      <ScienceIcon
                        sx={{ fontSize: 56, color: 'text.disabled', mb: 1.5 }}
                      />
                      <Typography variant="h6" color="text.secondary" gutterBottom>
                        No experiments found
                      </Typography>
                      <Typography
                        variant="body2"
                        color="text.disabled"
                        sx={{ mb: 2, maxWidth: 400, mx: 'auto' }}
                      >
                        {filters.search || filters.status !== 'all'
                          ? 'Try adjusting your filters or search terms.'
                          : 'Create your first experiment to start validating security controls.'}
                      </Typography>
                      {filters.search || filters.status !== 'all' ? (
                        <Button
                          variant="outlined"
                          startIcon={<ClearIcon />}
                          onClick={handleClearFilters}
                          sx={{ mr: 1 }}
                        >
                          Clear Filters
                        </Button>
                      ) : null}
                      <Button
                        variant="contained"
                        startIcon={<AddIcon />}
                        onClick={handleNewExperiment}
                      >
                        Create Experiment
                      </Button>
                    </Box>
                  </TableCell>
                </TableRow>
              ) : (
                experiments.map((experiment) => (
                  <ExperimentRow
                    key={experiment.id}
                    experiment={experiment}
                    onView={handleViewExperiment}
                    onRun={handleRunExperiment}
                    onStop={handleStopExperiment}
                    executing={
                      executeStatus === 'loading' && actionExperimentId === experiment.id
                    }
                    stopping={
                      stopStatus === 'loading' && actionExperimentId === experiment.id
                    }
                  />
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>

        {/* Pagination */}
        {!isLoading && experiments.length > 0 && (
          <TablePagination
            component="div"
            count={totalCount}
            page={page - 1} // Convert 1-based to 0-based
            rowsPerPage={pageSize}
            onPageChange={handlePageChange}
            onRowsPerPageChange={handlePageSizeChange}
            rowsPerPageOptions={PAGE_SIZE_OPTIONS}
            sx={{
              borderTop: '1px solid',
              borderColor: 'divider',
              '& .MuiTablePagination-toolbar': {
                minHeight: 52,
              },
              '& .MuiTablePagination-selectLabel, & .MuiTablePagination-displayedRows': {
                fontSize: '0.8125rem',
              },
            }}
          />
        )}
      </Paper>

      {/* Quick Action Floating Button (for mobile) */}
      <Box
        sx={{
          display: { xs: 'flex', md: 'none' },
          position: 'fixed',
          bottom: 56,
          right: 16,
          zIndex: (theme) => theme.zIndex.fab,
        }}
      >
        <Tooltip title="New Experiment">
          <Button
            variant="contained"
            onClick={handleNewExperiment}
            sx={{
              borderRadius: '50%',
              minWidth: 56,
              width: 56,
              height: 56,
              p: 0,
              boxShadow: '0 4px 16px rgba(37, 99, 235, 0.35)',
            }}
          >
            <AddIcon />
          </Button>
        </Tooltip>
      </Box>
    </Box>
  );
};

export default ExperimentListPage;
