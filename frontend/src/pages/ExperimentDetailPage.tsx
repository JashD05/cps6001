import {
  Refresh as RefreshIcon,
  ArrowBack as BackIcon,
  PlayArrow as RunIcon,
  Science as ExperimentIcon,
  Schedule as ScheduleIcon,
  Timer as TimerIcon,
  Stop as StopIcon,
  Dns as ClusterIcon,
  Person as PersonIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Pending as PendingIcon,
  RadioButtonChecked as RunningIcon,
  SkipNext as SkipIcon,
  Description as LogIcon,
  Assessment as ResultsIcon,
  Security as SIEMIcon,
  ContentCopy as CopyIcon,
  NavigateNext as NavigateNextIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Circle as CircleIcon,
} from '@mui/icons-material';
import {
  Box,
  Typography,
  Button,
  IconButton,
  Chip,
  Paper,
  Stack,
  Divider,
  Tooltip,
  LinearProgress,
  Alert,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Breadcrumbs,
  Link,
  Collapse,
  Skeleton,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  useTheme,
  useMediaQuery,
} from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useParams, useNavigate } from 'react-router-dom';
import StatusBadge from '@/components/StatusBadge';
import { experimentsAPI } from '@/services/api';
import {
  fetchExperimentById,
  fetchExperimentLogs,
  clearExperimentDetail,
  selectExperimentDetail,
  selectExperimentDetailLoading,
  selectExperimentDetailError,
  selectExperimentLogs,
  executeExperiment,
  stopExperiment,
  selectExecuteStatus,
  selectExecuteError,
  selectStopStatus,
  resetExecuteStatus,
  resetStopStatus,
} from '@/store/experimentSlice';
import type { AppDispatch } from '@/store';
import type { ExperimentStep, SIEMValidationResult, ExperimentResult } from '@/types';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const formatDate = (dateStr: string | undefined | null): string => {
  if (!dateStr) return '—';
  const d = new Date(dateStr);
  const month = d.toLocaleString('default', { month: 'short' });
  const day = d.getDate();
  const hour = d.getHours().toString().padStart(2, '0');
  const minute = d.getMinutes().toString().padStart(2, '0');
  const second = d.getSeconds().toString().padStart(2, '0');
  return `${month} ${day}, ${d.getFullYear()} ${hour}:${minute}:${second}`;
};

const formatDuration = (ms: number): string => {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  if (hours > 0) return `${hours}h ${remainingMinutes}m ${remainingSeconds}s`;
  if (minutes > 0) return `${minutes}m ${remainingSeconds}s`;
  return `${seconds}s`;
};

const formatParameterValue = (value: unknown): string => {
  if (value === null || value === undefined || value === '') return '—';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (Array.isArray(value)) return value.map(formatParameterValue).join(', ');

  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
};

const getStepIcon = (status: ExperimentStep['status']): React.ReactElement => {
  switch (status) {
    case 'completed':
      return <CheckCircleIcon sx={{ fontSize: 20, color: 'success.main' }} />;
    case 'in_progress':
      return <RunningIcon sx={{ fontSize: 20, color: 'primary.main' }} />;
    case 'failed':
      return <ErrorIcon sx={{ fontSize: 20, color: 'error.main' }} />;
    case 'skipped':
      return <SkipIcon sx={{ fontSize: 20, color: 'text.disabled' }} />;
    default:
      return <PendingIcon sx={{ fontSize: 20, color: 'text.secondary' }} />;
  }
};

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Execution Steps – vertical stepper showing each experiment step */
interface ProgressTrackerProps {
  steps: ExperimentStep[];
}

const ProgressTracker: React.FC<ProgressTrackerProps> = ({ steps }) => {
  const sortedSteps = [...steps].sort((a, b) => a.order - b.order);

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <ExperimentIcon sx={{ fontSize: 18, color: 'primary.main' }} />
          <Typography variant="subtitle2" fontWeight={700}>
            Execution Steps
          </Typography>
          <Chip
            label={`${steps.filter((s) => s.status === 'completed').length}/${steps.length}`}
            size="small"
            sx={{ ml: 'auto', height: 22, fontSize: '0.6875rem' }}
          />
        </Stack>
      </Box>

      {sortedSteps.length === 0 ? (
        <Box sx={{ py: 4, textAlign: 'center' }}>
          <Typography variant="body2" color="text.secondary">
            No execution steps defined for this experiment.
          </Typography>
        </Box>
      ) : (
        <List disablePadding>
          {sortedSteps.map((step, index) => (
            <ListItem
              key={step.id}
              sx={{
                py: 1.5,
                px: 2.5,
                borderBottom: index < sortedSteps.length - 1 ? '1px solid' : 'none',
                borderColor: 'divider',
                backgroundColor:
                  step.status === 'in_progress'
                    ? 'rgba(37, 99, 235, 0.03)'
                    : 'transparent',
                transition: 'background-color 200ms',
              }}
            >
              <ListItemIcon sx={{ minWidth: 36 }}>
                {getStepIcon(step.status)}
              </ListItemIcon>
              <ListItemText
                primary={
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Typography
                      variant="body2"
                      fontWeight={step.status === 'in_progress' ? 700 : 500}
                      sx={{
                        color:
                          step.status === 'in_progress' ? 'primary.main' : 'text.primary',
                      }}
                    >
                      {step.name}
                    </Typography>
                    <StatusBadge status={step.status} variant="pill" size="small" />
                  </Stack>
                }
                secondary={
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block', mt: 0.25 }}
                  >
                    {step.description}
                  </Typography>
                }
              />
              <Stack
                direction="row"
                spacing={1}
                alignItems="center"
                sx={{ flexShrink: 0 }}
              >
                {step.startedAt && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ whiteSpace: 'nowrap' }}
                  >
                    {formatDate(step.startedAt)}
                  </Typography>
                )}
                {step.completedAt && step.startedAt && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ whiteSpace: 'nowrap' }}
                  >
                    (
                    {formatDuration(
                      new Date(step.completedAt).getTime() -
                        new Date(step.startedAt).getTime(),
                    )}
                    )
                  </Typography>
                )}
              </Stack>
            </ListItem>
          ))}
        </List>
      )}
    </Paper>
  );
};

/** Static read-only Log Viewer */
interface LogViewerProps {
  logs: string[];
  isLoading: boolean;
  onRefresh: () => void;
  experimentId: string;
}

const LogViewer: React.FC<LogViewerProps> = ({
  logs,
  isLoading,
  onRefresh,
  experimentId,
}) => {
  const [expanded, setExpanded] = useState(true);

  const handleCopyLogs = () => {
    navigator.clipboard.writeText(logs.join('\n')).catch(() => {
      /* clipboard API might not be available */
    });
  };

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Box
        sx={{
          px: 2.5,
          py: 1.5,
          borderBottom: '1px solid',
          borderColor: 'divider',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Stack direction="row" alignItems="center" spacing={1}>
          <LogIcon sx={{ fontSize: 18, color: 'primary.main' }} />
          <Typography variant="subtitle2" fontWeight={700}>
            Run Logs
          </Typography>
          {logs.length > 0 && (
            <Chip
              label={`${logs.length} lines`}
              size="small"
              variant="outlined"
              sx={{ height: 22, fontSize: '0.6875rem' }}
            />
          )}
        </Stack>
        <Stack direction="row" spacing={0.5}>
          <Tooltip title="Copy Logs">
            <IconButton
              size="small"
              onClick={handleCopyLogs}
              disabled={logs.length === 0}
            >
              <CopyIcon sx={{ fontSize: 18 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Refresh Logs">
            <IconButton size="small" onClick={onRefresh} disabled={isLoading}>
              <RefreshIcon sx={{ fontSize: 18 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title={expanded ? 'Collapse' : 'Expand'}>
            <IconButton size="small" onClick={() => setExpanded(!expanded)}>
              {expanded ? (
                <ExpandLessIcon sx={{ fontSize: 18 }} />
              ) : (
                <ExpandMoreIcon sx={{ fontSize: 18 }} />
              )}
            </IconButton>
          </Tooltip>
        </Stack>
      </Box>

      <Collapse in={expanded}>
        <Box
          sx={{
            backgroundColor: '#0F172A',
            color: '#E2E8F0',
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
            fontSize: '0.75rem',
            lineHeight: 1.7,
            p: 2,
            maxHeight: 400,
            overflowY: 'auto',
            minHeight: 120,
          }}
        >
          {isLoading && logs.length === 0 ? (
            <Box sx={{ py: 2, textAlign: 'center' }}>
              <Typography
                sx={{
                  color: '#64748B',
                  fontFamily: 'inherit',
                  fontSize: '0.75rem',
                }}
              >
                Loading logs...
              </Typography>
            </Box>
          ) : logs.length === 0 ? (
            <Box sx={{ py: 2, textAlign: 'center' }}>
              <Typography
                sx={{
                  color: '#64748B',
                  fontFamily: 'inherit',
                  fontSize: '0.75rem',
                }}
              >
                No logs available yet.{' '}
                {experimentId ? 'Run the experiment to see logs.' : ''}
              </Typography>
            </Box>
          ) : (
            logs.map((line, index) => (
              <Box
                key={`${index}-${line.substring(0, 20)}`}
                component="pre"
                sx={{
                  margin: 0,
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-all',
                  color: line.toLowerCase().includes('error')
                    ? '#F87171'
                    : line.toLowerCase().includes('warn')
                      ? '#FBBF24'
                      : line.toLowerCase().includes('success') ||
                          line.toLowerCase().includes('completed')
                        ? '#34D399'
                        : '#CBD5E1',
                }}
              >
                <Typography
                  component="span"
                  sx={{
                    color: '#475569',
                    display: 'inline-block',
                    width: 48,
                    textAlign: 'right',
                    marginRight: 1.5,
                    userSelect: 'none',
                    fontSize: '0.6875rem',
                  }}
                >
                  {index + 1}
                </Typography>
                {line}
              </Box>
            ))
          )}
        </Box>
      </Collapse>
    </Paper>
  );
};

/** Results Summary – outcome banner with score, pass/fail, summary, and details */
interface ResultsSummaryProps {
  result: ExperimentResult;
}

const ResultsSummary: React.FC<ResultsSummaryProps> = ({ result }) => {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <ResultsIcon sx={{ fontSize: 18, color: 'primary.main' }} />
          <Typography variant="subtitle2" fontWeight={700}>
            Results
          </Typography>
        </Stack>
      </Box>

      <Box sx={{ p: 2.5 }}>
        {/* Score Card */}
        <Stack
          direction="row"
          spacing={3}
          sx={{
            p: 2,
            borderRadius: 2,
            backgroundColor: result.success
              ? 'rgba(16, 185, 129, 0.06)'
              : 'rgba(239, 68, 68, 0.06)',
            border: '1px solid',
            borderColor: result.success
              ? 'rgba(16, 185, 129, 0.2)'
              : 'rgba(239, 68, 68, 0.2)',
            mb: 2,
          }}
        >
          <Box sx={{ textAlign: 'center', minWidth: 80 }}>
            <Typography
              variant="h3"
              fontWeight={800}
              sx={{
                color: result.success ? 'success.main' : 'error.main',
                lineHeight: 1.2,
              }}
            >
              {result.score}
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.5 }}
            >
              Score
            </Typography>
          </Box>

          <Divider orientation="vertical" flexItem />

          <Box sx={{ flex: 1 }}>
            <Stack direction="row" alignItems="center" spacing={1} mb={1}>
              {result.success ? (
                <CheckCircleIcon sx={{ fontSize: 20, color: 'success.main' }} />
              ) : (
                <ErrorIcon sx={{ fontSize: 20, color: 'error.main' }} />
              )}
              <Typography variant="subtitle2" fontWeight={700}>
                {result.success ? 'Experiment Passed' : 'Experiment Failed'}
              </Typography>
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {result.summary}
            </Typography>
            <Stack direction="row" spacing={2}>
              <Stack direction="row" spacing={0.5} alignItems="center">
                <ScheduleIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                <Typography variant="caption" color="text.secondary">
                  Completed: {formatDate(result.completedAt)}
                </Typography>
              </Stack>
              <Stack direction="row" spacing={0.5} alignItems="center">
                <TimerIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                <Typography variant="caption" color="text.secondary">
                  Duration: {formatDuration(result.duration)}
                </Typography>
              </Stack>
            </Stack>
          </Box>
        </Stack>

        {/* Details list */}
        {result.details.length > 0 && (
          <Box>
            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
              Details
            </Typography>
            <List disablePadding dense>
              {result.details.map((detail, idx) => (
                <ListItem key={idx} sx={{ py: 0.5, px: 0 }}>
                  <ListItemIcon sx={{ minWidth: 28 }}>
                    <CircleIcon sx={{ fontSize: 6, color: 'primary.main' }} />
                  </ListItemIcon>
                  <ListItemText
                    primary={detail}
                    primaryTypographyProps={{ variant: 'body2', color: 'text.secondary' }}
                  />
                </ListItem>
              ))}
            </List>
          </Box>
        )}
      </Box>
    </Paper>
  );
};

/** SIEM Validation Results Section */
interface SIEMValidationProps {
  validation: SIEMValidationResult;
}

const SIEMValidationSection: React.FC<SIEMValidationProps> = ({ validation }) => {
  const coveragePercent = validation.coverage * 100;
  const latencySeconds = (validation.detectionLatencyMs / 1000).toFixed(1);

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <SIEMIcon sx={{ fontSize: 18, color: 'primary.main' }} />
          <Typography variant="subtitle2" fontWeight={700}>
            SIEM Validation
          </Typography>
          <Box sx={{ ml: 'auto' }}>
            <StatusBadge
              status={validation.detected ? 'validated' : 'invalid'}
              variant="pill"
              size="medium"
            />
          </Box>
        </Stack>
      </Box>

      <Box sx={{ p: 2.5 }}>
        {/* Summary metrics */}
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mb: 2.5 }}>
          {/* Alert Count */}
          <Box
            sx={{
              flex: 1,
              p: 2,
              borderRadius: 2,
              border: '1px solid',
              borderColor: 'divider',
              textAlign: 'center',
            }}
          >
            <Typography variant="h4" fontWeight={700} sx={{ color: 'primary.main' }}>
              {validation.receivedAlertCount}/{validation.expectedAlertCount}
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.5 }}
            >
              Alerts Received
            </Typography>
          </Box>

          {/* Coverage */}
          <Box
            sx={{
              flex: 1,
              p: 2,
              borderRadius: 2,
              border: '1px solid',
              borderColor: 'divider',
              textAlign: 'center',
            }}
          >
            <Typography
              variant="h4"
              fontWeight={700}
              sx={{
                color:
                  coveragePercent >= 80
                    ? 'success.main'
                    : coveragePercent >= 50
                      ? 'warning.main'
                      : 'error.main',
              }}
            >
              {coveragePercent.toFixed(0)}%
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.5 }}
            >
              Detection Coverage
            </Typography>
            <LinearProgress
              variant="determinate"
              value={coveragePercent}
              sx={{
                mt: 1,
                height: 6,
                borderRadius: 3,
                backgroundColor: 'grey.100',
                '& .MuiLinearProgress-bar': {
                  borderRadius: 3,
                  backgroundColor:
                    coveragePercent >= 80
                      ? 'success.main'
                      : coveragePercent >= 50
                        ? 'warning.main'
                        : 'error.main',
                },
              }}
            />
          </Box>

          {/* Detection Latency */}
          <Box
            sx={{
              flex: 1,
              p: 2,
              borderRadius: 2,
              border: '1px solid',
              borderColor: 'divider',
              textAlign: 'center',
            }}
          >
            <Typography
              variant="h4"
              fontWeight={700}
              sx={{
                color:
                  validation.detectionLatencyMs < 5000
                    ? 'success.main'
                    : validation.detectionLatencyMs < 30000
                      ? 'warning.main'
                      : 'error.main',
              }}
            >
              {latencySeconds}s
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.5 }}
            >
              Detection Latency
            </Typography>
          </Box>
        </Stack>

        {/* Alerts Detail */}
        {validation.alerts.length > 0 && (
          <Box>
            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
              Received Alerts
            </Typography>
            <TableContainer
              component={Paper}
              variant="outlined"
              sx={{ borderRadius: 1.5 }}
            >
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Rule</TableCell>
                    <TableCell>Severity</TableCell>
                    <TableCell>Source</TableCell>
                    <TableCell>Time</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {validation.alerts.map((alert) => (
                    <TableRow key={alert.id}>
                      <TableCell>
                        <Typography variant="body2" fontWeight={500}>
                          {alert.ruleName}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          status={
                            alert.severity === 'critical'
                              ? 'failed'
                              : alert.severity === 'high'
                                ? 'error'
                                : alert.severity === 'medium'
                                  ? 'warning'
                                  : 'info'
                          }
                          variant="pill"
                          size="small"
                          label={alert.severity}
                        />
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2">{alert.source}</Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" color="text.secondary">
                          {formatDate(alert.timestamp)}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </Box>
        )}

        {/* Validation details */}
        {validation.details.length > 0 && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
              Validation Notes
            </Typography>
            <List disablePadding dense>
              {validation.details.map((detail, idx) => (
                <ListItem key={idx} sx={{ py: 0.25, px: 0 }}>
                  <ListItemIcon sx={{ minWidth: 28 }}>
                    <CircleIcon sx={{ fontSize: 6, color: 'text.secondary' }} />
                  </ListItemIcon>
                  <ListItemText
                    primary={detail}
                    primaryTypographyProps={{ variant: 'body2', color: 'text.secondary' }}
                  />
                </ListItem>
              ))}
            </List>
          </Box>
        )}
      </Box>
    </Paper>
  );
};

// ---------------------------------------------------------------------------
// Main Page Component
// ---------------------------------------------------------------------------

const ExperimentDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const dispatch = useDispatch<AppDispatch>();
  const theme = useTheme();
  const _isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  const experiment = useSelector(selectExperimentDetail);
  const isLoading = useSelector(selectExperimentDetailLoading);
  const error = useSelector(selectExperimentDetailError);
  const logs = useSelector(selectExperimentLogs);
  const executeStatus = useSelector(selectExecuteStatus);
  const executeError = useSelector(selectExecuteError);
  const stopStatus = useSelector(selectStopStatus);

  const canRun =
    experiment &&
    !isLoading &&
    executeStatus !== ('loading' as string) &&
    [
      'draft',
      'active',
      'pending',
      'queued',
      'stopped',
      'completed',
      'failed',
      'timed_out',
      'archived',
    ].includes(experiment.status);
  const canStop =
    experiment && ['running', 'pending', 'queued'].includes(experiment.status);

  const handleRun = useCallback(async () => {
    if (!experiment || !id) return;
    dispatch(resetExecuteStatus());
    try {
      await dispatch(
        executeExperiment({ id: experiment.id, clusterId: experiment.clusterId }),
      ).unwrap();
      // Await the refetch so the detail page picks up the completed
      // result, logs and pod status from the server.
      await dispatch(fetchExperimentById(id)).unwrap();
      dispatch(fetchExperimentLogs({ id, tail: 200 }));
    } catch {
      // Error is captured in the slice state and shown below
    }
  }, [dispatch, experiment, id]);

  const handleStop = useCallback(async () => {
    if (!experiment || !id) return;
    dispatch(resetStopStatus());
    try {
      await dispatch(stopExperiment(experiment.id)).unwrap();
      await dispatch(fetchExperimentById(id)).unwrap();
      dispatch(fetchExperimentLogs({ id, tail: 200 }));
    } catch {
      // Error is captured in the slice state
    }
  }, [dispatch, experiment, id]);
  const isNotFound = Boolean(error && /not found|404/i.test(error));

  // ---------------------------------------------------------------------------
  // Data Fetching
  // ---------------------------------------------------------------------------

  useEffect(() => {
    if (id) {
      dispatch(fetchExperimentById(id));
      dispatch(fetchExperimentLogs({ id, tail: 200 }));
    }
    return () => {
      dispatch(clearExperimentDetail());
    };
  }, [dispatch, id]);

  const handleRefresh = () => {
    if (!id) return;
    dispatch(fetchExperimentById(id));
    dispatch(fetchExperimentLogs({ id, tail: 200 }));
  };

  const handleRefreshLogs = () => {
    if (!id) return;
    dispatch(fetchExperimentLogs({ id, tail: 200 }));
  };

  // ---------------------------------------------------------------------------
  // Loading State
  // ---------------------------------------------------------------------------

  if (isLoading && !experiment) {
    return (
      <Box sx={{ py: 4 }}>
        <Stack spacing={2}>
          <Skeleton variant="rectangular" height={80} sx={{ borderRadius: 2 }} />
          <Skeleton variant="rectangular" height={400} sx={{ borderRadius: 2 }} />
          <Skeleton variant="rectangular" height={200} sx={{ borderRadius: 2 }} />
        </Stack>
      </Box>
    );
  }

  // ---------------------------------------------------------------------------
  // Error State
  // ---------------------------------------------------------------------------

  if (error && !experiment) {
    return (
      <Box sx={{ py: 4, textAlign: 'center' }}>
        <ErrorIcon sx={{ fontSize: 64, color: 'error.main', mb: 2 }} />
        <Typography variant="h5" fontWeight={700} gutterBottom>
          {isNotFound ? 'Experiment Not Found' : 'Failed to Load Experiment'}
        </Typography>
        <Typography
          variant="body1"
          color="text.secondary"
          sx={{ mb: 3, maxWidth: 480, mx: 'auto' }}
        >
          {isNotFound
            ? 'The experiment you requested does not exist or was removed.'
            : error}
        </Typography>
        <Stack direction="row" spacing={2} justifyContent="center">
          <Button
            variant="outlined"
            startIcon={<BackIcon />}
            onClick={() => navigate('/experiments')}
          >
            Back to Experiments
          </Button>
          <Button variant="contained" startIcon={<RefreshIcon />} onClick={handleRefresh}>
            Try Again
          </Button>
        </Stack>
      </Box>
    );
  }

  if (!experiment) {
    return null;
  }

  // ---------------------------------------------------------------------------
  // Derived State
  // ---------------------------------------------------------------------------

  const isCompleted = experiment.status === 'completed';
  const isFailed = experiment.status === 'failed';
  const hasResult = experiment.result !== undefined && experiment.result !== null;
  const hasSIEMValidation = hasResult && experiment.result!.siemValidation !== undefined;
  const hasOutcome = isCompleted || isFailed;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <Box>
      {/* Breadcrumbs */}
      <Breadcrumbs separator={<NavigateNextIcon sx={{ fontSize: 16 }} />} sx={{ mb: 2 }}>
        <Link
          underline="hover"
          color="text.secondary"
          sx={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 0.5 }}
          onClick={() => navigate('/experiments')}
        >
          <ExperimentIcon sx={{ fontSize: 16 }} />
          Experiments
        </Link>
        <Typography color="text.primary" fontWeight={600}>
          {experiment.name}
        </Typography>
      </Breadcrumbs>

      {/* ----------------------------------------------------------------- */}
      {/* Header                                                            */}
      {/* ----------------------------------------------------------------- */}

      <Paper
        variant="outlined"
        sx={{
          borderRadius: 2,
          overflow: 'hidden',
          mb: 3,
          borderLeft: '4px solid',
          borderLeftColor: isFailed
            ? 'error.main'
            : isCompleted
              ? 'success.main'
              : 'divider',
        }}
      >
        <Box sx={{ p: 2.5 }}>
          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            spacing={{ xs: 2, sm: 2 }}
            alignItems={{ xs: 'flex-start', sm: 'center' }}
            justifyContent="space-between"
          >
            {/* Left: Name + meta */}
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Stack direction="row" alignItems="center" spacing={1.5} mb={1}>
                <Typography
                  variant="h5"
                  fontWeight={800}
                  sx={{
                    fontSize: { xs: '1.125rem', sm: '1.5rem' },
                    lineHeight: 1.3,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {experiment.name}
                </Typography>
                <StatusBadge
                  status={experiment.status}
                  variant="pill"
                  size="medium"
                  label={experiment.status === 'pending' ? 'Draft' : undefined}
                />
              </Stack>

              {experiment.description && (
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{
                    mb: 1.5,
                    maxWidth: 600,
                    display: '-webkit-box',
                    WebkitLineClamp: 2,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                  }}
                >
                  {experiment.description}
                </Typography>
              )}

              <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap>
                {experiment.templateName && (
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    <ExperimentIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                    <Typography variant="caption" color="text.secondary">
                      {experiment.templateName}
                    </Typography>
                  </Stack>
                )}
                {experiment.clusterName && (
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    <ClusterIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                    <Typography variant="caption" color="text.secondary">
                      {experiment.clusterName}
                    </Typography>
                  </Stack>
                )}
                <Stack direction="row" spacing={0.5} alignItems="center">
                  <ScheduleIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                  <Typography variant="caption" color="text.secondary">
                    Created: {formatDate(experiment.createdAt)}
                  </Typography>
                </Stack>
                {experiment.startedAt && (
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    <TimerIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                    <Typography variant="caption" color="text.secondary">
                      Started: {formatDate(experiment.startedAt)}
                    </Typography>
                  </Stack>
                )}
                {experiment.duration !== undefined && (
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    <TimerIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                    <Typography variant="caption" color="text.secondary">
                      Duration: {formatDuration(experiment.duration)}
                    </Typography>
                  </Stack>
                )}
                <Stack direction="row" spacing={0.5} alignItems="center">
                  <PersonIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                  <Typography variant="caption" color="text.secondary">
                    {experiment.createdBy}
                  </Typography>
                </Stack>
              </Stack>
            </Box>

            {/* Right: Action buttons */}
            <Stack direction="row" spacing={1} alignItems="center">
              {canRun && (
                <Button
                  variant="contained"
                  size="small"
                  startIcon={<RunIcon />}
                  onClick={handleRun}
                  disabled={executeStatus === 'loading'}
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                >
                  {experiment.status === 'draft' || experiment.status === 'pending'
                    ? 'Run'
                    : 'Re-run'}
                </Button>
              )}
              {canStop && (
                <Button
                  variant="outlined"
                  color="error"
                  size="small"
                  startIcon={<StopIcon />}
                  onClick={handleStop}
                  disabled={stopStatus === 'loading'}
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                >
                  Stop
                </Button>
              )}
              <Tooltip title="Refresh">
                <IconButton onClick={handleRefresh} disabled={isLoading}>
                  <RefreshIcon />
                </IconButton>
              </Tooltip>
            </Stack>
          </Stack>
        </Box>
      </Paper>

      {/* ----------------------------------------------------------------- */}
      {/* Outcome — results shown inline when available                     */}
      {/* ----------------------------------------------------------------- */}

      {hasOutcome && hasResult ? (
        <Box sx={{ mb: 3 }}>
          <ResultsSummary result={experiment.result!} />
        </Box>
      ) : hasOutcome && !hasResult ? (
        <Paper
          variant="outlined"
          sx={{ borderRadius: 2, mb: 3, p: 3, textAlign: 'center' }}
        >
          <ErrorIcon sx={{ fontSize: 40, color: 'warning.main', mb: 1 }} />
          <Typography variant="subtitle1" fontWeight={600} gutterBottom>
            {isFailed ? 'Experiment Failed' : 'No Results'}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {isFailed
              ? 'This experiment ran but did not produce a result. Check the logs below for details.'
              : 'Results will appear here once the experiment completes.'}
          </Typography>
        </Paper>
      ) : (
        <Paper
          variant="outlined"
          sx={{ borderRadius: 2, mb: 3, p: 4, textAlign: 'center' }}
        >
          <ExperimentIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
          <Typography variant="subtitle1" fontWeight={600} gutterBottom>
            Not Yet Run
          </Typography>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ maxWidth: 400, mx: 'auto' }}
          >
            This experiment hasn't been executed yet. Results and outcome details will
            appear here after it runs.
          </Typography>
        </Paper>
      )}

      {/* Execute / Stop Error Alerts */}
      {executeStatus === 'failed' && executeError && (
        <Alert
          severity={executeError.includes('concurrency_limit') ? 'warning' : 'error'}
          onClose={() => dispatch(resetExecuteStatus())}
          sx={{ mb: 3, borderRadius: 2 }}
        >
          {executeError.includes('concurrency_limit') ? (
            <>
              {executeError}
              <br />
              <Stack
                direction="row"
                spacing={1}
                sx={{ mt: 1 }}
                flexWrap="wrap"
                useFlexGap
              >
                <Button
                  size="small"
                  variant="outlined"
                  color="warning"
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                  onClick={async () => {
                    try {
                      const res = await experimentsAPI.cancelStaleRuns(
                        experiment.clusterId
                          ? { clusterId: experiment.clusterId }
                          : undefined,
                      );
                      const count = (res.data?.data as any)?.cancelled_count ?? 0;
                      if (count > 0) {
                        dispatch(resetExecuteStatus());
                        dispatch(fetchExperimentById(id!));
                      }
                      alert(
                        count > 0
                          ? `Cancelled ${count} stale run${count === 1 ? '' : 's'}. Try running again.`
                          : 'No stale runs found. All active runs are currently in progress.',
                      );
                    } catch {
                      alert('Failed to cancel stale runs. Please try again.');
                    }
                  }}
                >
                  Cancel Stale Runs
                </Button>
                <Button
                  size="small"
                  variant="text"
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                  onClick={() => navigate('/experiments')}
                >
                  View All Experiments →
                </Button>
              </Stack>
            </>
          ) : (
            executeError
          )}
        </Alert>
      )}
      {stopStatus === 'failed' && (
        <Alert
          severity="error"
          onClose={() => dispatch(resetStopStatus())}
          sx={{ mb: 3, borderRadius: 2 }}
        >
          Failed to stop experiment. Please refresh and try again.
        </Alert>
      )}

      {/* SIEM Validation — shown inline when data exists */}
      {hasSIEMValidation && (
        <Box sx={{ mb: 3 }}>
          <SIEMValidationSection validation={experiment.result!.siemValidation} />
        </Box>
      )}

      {/* ----------------------------------------------------------------- */}
      {/* Execution Steps                                                    */}
      {/* ----------------------------------------------------------------- */}

      <Box sx={{ mb: 3 }}>
        <ProgressTracker steps={experiment.steps} />
      </Box>

      {/* ----------------------------------------------------------------- */}
      {/* Quick Info Cards                                                   */}
      {/* ----------------------------------------------------------------- */}

      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mb: 3 }}>
        {/* Namespace */}
        <Box
          sx={{
            flex: 1,
            p: 2,
            borderRadius: 2,
            border: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Typography variant="overline" sx={{ mb: 0.5, display: 'block' }}>
            Namespace
          </Typography>
          <Typography variant="body1" fontWeight={600} sx={{ fontFamily: 'monospace' }}>
            {experiment.namespace}
          </Typography>
        </Box>

        {/* Tags */}
        <Box
          sx={{
            flex: 2,
            p: 2,
            borderRadius: 2,
            border: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Typography variant="overline" sx={{ mb: 0.5, display: 'block' }}>
            Tags
          </Typography>
          <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
            {experiment.tags.length > 0 ? (
              experiment.tags.map((tag) => (
                <Chip
                  key={tag}
                  label={tag}
                  size="small"
                  variant="outlined"
                  sx={{ fontSize: '0.75rem' }}
                />
              ))
            ) : (
              <Typography variant="body2" color="text.secondary">
                No tags
              </Typography>
            )}
          </Stack>
        </Box>

        {/* Parameters */}
        <Box
          sx={{
            flex: 2,
            p: 2,
            borderRadius: 2,
            border: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Typography variant="overline" sx={{ mb: 0.5, display: 'block' }}>
            Parameters
          </Typography>
          <Stack spacing={0.5}>
            {Object.entries(experiment.parameters).map(([key, value]) => (
              <Stack key={key} direction="row" spacing={1}>
                <Typography
                  variant="caption"
                  fontWeight={600}
                  sx={{ fontFamily: 'monospace', minWidth: 120 }}
                >
                  {key}:
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    fontFamily: 'monospace',
                    whiteSpace: 'pre-wrap',
                    overflowWrap: 'anywhere',
                  }}
                >
                  {formatParameterValue(value)}
                </Typography>
              </Stack>
            ))}
            {Object.keys(experiment.parameters).length === 0 && (
              <Typography variant="body2" color="text.secondary">
                No parameters
              </Typography>
            )}
          </Stack>
        </Box>
      </Stack>

      {/* ----------------------------------------------------------------- */}
      {/* Logs                                                               */}
      {/* ----------------------------------------------------------------- */}

      <LogViewer
        logs={logs}
        isLoading={isLoading}
        onRefresh={handleRefreshLogs}
        experimentId={id ?? ''}
      />
    </Box>
  );
};

export default ExperimentDetailPage;
