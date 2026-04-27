import React, { useEffect, useState, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
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
  Card,
  CardContent,
  LinearProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Breadcrumbs,
  Link,
  Tabs,
  Tab,
  Alert,
  AlertTitle,
  Collapse,
  Fade,
  Skeleton,
  Avatar,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  useTheme,
  useMediaQuery,
} from '@mui/material';
import {
  PlayArrow as RunIcon,
  Stop as StopIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
  ArrowBack as BackIcon,
  Science as ExperimentIcon,
  Schedule as ScheduleIcon,
  Timer as TimerIcon,
  Dns as ClusterIcon,
  Person as PersonIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Pending as PendingIcon,
  RadioButtonChecked as RunningIcon,
  SkipNext as SkipIcon,
  Description as LogIcon,
  Widgets as PodIcon,
  Assessment as ResultsIcon,
  Security as SIEMIcon,
  TrendingUp as TrendUpIcon,
  TrendingDown as TrendDownIcon,
  CloudDownload as DownloadIcon,
  ContentCopy as CopyIcon,
  NavigateNext as NavigateNextIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Circle as CircleIcon,
  KeyboardArrowRight as ChevronRightIcon,
  DeleteOutline as TrashIcon,
  Replay as ReplayIcon,
} from '@mui/icons-material';
import { useDispatch, useSelector } from 'react-redux';
import type { AppDispatch, RootState } from '@/store';
import {
  fetchExperimentById,
  fetchExperimentLogs,
  executeExperiment,
  stopExperiment,
  clearExperimentDetail,
  selectExperimentDetail,
  selectExperimentDetailLoading,
  selectExperimentDetailError,
  selectCurrentRun,
  selectExperimentLogs,
  selectExecuteStatus,
  selectStopStatus,
  resetExecuteStatus,
  resetStopStatus,
  updateExperimentStatus,
} from '@/store/experimentSlice';
import { experimentsAPI, getErrorMessage } from '@/services/api';
import StatusBadge from '@/components/StatusBadge';
import type {
  Experiment,
  ExperimentStep,
  ExperimentRun,
  PodStatus,
  SIEMValidationResult,
  ExperimentResult,
} from '@/types';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const formatDate = (dateStr: string | undefined | null): string => {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
};

const formatDuration = (ms: number | undefined | null): string => {
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
    case 'pending':
    default:
      return <PendingIcon sx={{ fontSize: 20, color: 'text.secondary' }} />;
  }
};

const getPodStatusColor = (
  status: PodStatus['status'],
): 'success' | 'error' | 'warning' | 'default' | 'primary' => {
  switch (status) {
    case 'Running':
      return 'primary';
    case 'Succeeded':
      return 'success';
    case 'Failed':
      return 'error';
    case 'Pending':
      return 'warning';
    default:
      return 'default';
  }
};

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Progress Tracker – vertical stepper showing each experiment step */
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

/** Live Log Viewer */
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
  const [autoScroll, setAutoScroll] = useState(true);
  const logEndRef = useRef<HTMLDivElement>(null);
  const logContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (autoScroll && logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  const handleScroll = useCallback(() => {
    if (!logContainerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = logContainerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    setAutoScroll(isAtBottom);
  }, []);

  const handleCopyLogs = useCallback(() => {
    navigator.clipboard.writeText(logs.join('\n')).catch(() => {
      /* clipboard API might not be available */
    });
  }, [logs]);

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
            Live Logs
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
          ref={logContainerRef}
          onScroll={handleScroll}
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
                {experimentId ? 'Start the experiment to see logs.' : ''}
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
          <div ref={logEndRef} />
        </Box>
        {logs.length > 0 && !autoScroll && (
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'center',
              py: 0.75,
              borderTop: '1px solid',
              borderColor: 'divider',
            }}
          >
            <Button
              size="small"
              startIcon={<ChevronRightIcon sx={{ transform: 'rotate(90deg)' }} />}
              onClick={() => {
                setAutoScroll(true);
                logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
              }}
              sx={{ textTransform: 'none', fontSize: '0.75rem' }}
            >
              Scroll to bottom
            </Button>
          </Box>
        )}
      </Collapse>
    </Paper>
  );
};

/** Attack Pod Status Section */
interface PodStatusSectionProps {
  pods: PodStatus[];
  isLoading: boolean;
}

const PodStatusSection: React.FC<PodStatusSectionProps> = ({ pods, isLoading }) => {
  if (isLoading) {
    return (
      <Paper variant="outlined" sx={{ borderRadius: 2, p: 2.5 }}>
        <Stack spacing={1}>
          {[1, 2, 3].map((i) => (
            <Skeleton
              key={i}
              variant="rectangular"
              height={40}
              sx={{ borderRadius: 1 }}
            />
          ))}
        </Stack>
      </Paper>
    );
  }

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <PodIcon sx={{ fontSize: 18, color: 'primary.main' }} />
          <Typography variant="subtitle2" fontWeight={700}>
            Attack Pod Status
          </Typography>
          {pods.length > 0 && (
            <Chip
              label={`${pods.length} pod${pods.length !== 1 ? 's' : ''}`}
              size="small"
              sx={{ ml: 'auto', height: 22, fontSize: '0.6875rem' }}
            />
          )}
        </Stack>
      </Box>

      {pods.length === 0 ? (
        <Box sx={{ py: 4, textAlign: 'center' }}>
          <PodIcon sx={{ fontSize: 40, color: 'text.disabled', mb: 1 }} />
          <Typography variant="body2" color="text.secondary">
            No attack pods deployed yet.
          </Typography>
          <Typography
            variant="caption"
            color="text.disabled"
            sx={{ display: 'block', mt: 0.5 }}
          >
            Pods will appear once the experiment starts running.
          </Typography>
        </Box>
      ) : (
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Namespace</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Ready</TableCell>
                <TableCell>Restarts</TableCell>
                <TableCell>Age</TableCell>
                <TableCell>Node</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {pods.map((pod) => (
                <TableRow key={pod.name}>
                  <TableCell>
                    <Typography
                      variant="body2"
                      fontWeight={500}
                      sx={{ fontFamily: 'monospace', fontSize: '0.8125rem' }}
                    >
                      {pod.name}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: 'monospace', fontSize: '0.8125rem' }}
                    >
                      {pod.namespace}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      status={
                        pod.status === 'Running'
                          ? 'running'
                          : pod.status === 'Succeeded'
                            ? 'completed'
                            : pod.status === 'Failed'
                              ? 'failed'
                              : 'pending'
                      }
                      variant="pill"
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {pod.ready ? (
                        <CheckCircleIcon
                          sx={{
                            fontSize: 16,
                            color: 'success.main',
                            verticalAlign: 'middle',
                          }}
                        />
                      ) : (
                        <ErrorIcon
                          sx={{
                            fontSize: 16,
                            color: 'warning.main',
                            verticalAlign: 'middle',
                          }}
                        />
                      )}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography
                      variant="body2"
                      sx={{
                        color: pod.restarts > 0 ? 'warning.main' : 'text.secondary',
                        fontWeight: pod.restarts > 0 ? 600 : 400,
                      }}
                    >
                      {pod.restarts}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">{pod.age}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" sx={{ fontSize: '0.8125rem' }}>
                      {pod.node ?? '—'}
                    </Typography>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Paper>
  );
};

/** Results Summary Section (shown when experiment is completed) */
interface ResultsSummaryProps {
  result: ExperimentResult;
}

const ResultsSummary: React.FC<ResultsSummaryProps> = ({ result }) => {
  const theme = useTheme();

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <ResultsIcon sx={{ fontSize: 18, color: 'primary.main' }} />
          <Typography variant="subtitle2" fontWeight={700}>
            Results Summary
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
            SIEM Validation Results
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
// Tab Panel Helper
// ---------------------------------------------------------------------------

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

const TabPanel: React.FC<TabPanelProps> = ({ children, value, index }) => {
  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`experiment-tabpanel-${index}`}
      aria-labelledby={`experiment-tab-${index}`}
      style={{ display: value === index ? 'block' : 'none' }}
    >
      {value === index && <Box sx={{ pt: 3 }}>{children}</Box>}
    </div>
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
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  const experiment = useSelector(selectExperimentDetail);
  const isLoading = useSelector(selectExperimentDetailLoading);
  const error = useSelector(selectExperimentDetailError);
  const currentRun = useSelector(selectCurrentRun);
  const logs = useSelector(selectExperimentLogs);
  const executeStatus = useSelector(selectExecuteStatus);
  const stopStatus = useSelector(selectStopStatus);

  const [activeTab, setActiveTab] = useState(0);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

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

  // Refresh logs periodically when experiment is running
  useEffect(() => {
    if (!id || experiment?.status !== 'running') return;

    const interval = setInterval(() => {
      dispatch(fetchExperimentLogs({ id, tail: 200 }));
    }, 5000);

    return () => clearInterval(interval);
  }, [dispatch, id, experiment?.status]);

  // Reset action statuses on unmount
  useEffect(() => {
    return () => {
      dispatch(resetExecuteStatus());
      dispatch(resetStopStatus());
    };
  }, [dispatch]);

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  const handleRun = async () => {
    if (!id) return;
    dispatch(resetExecuteStatus());
    await dispatch(executeExperiment(id));
    dispatch(fetchExperimentById(id));
  };

  const handleStop = async () => {
    if (!id) return;
    dispatch(resetStopStatus());
    await dispatch(stopExperiment(id));
    dispatch(fetchExperimentById(id));
  };

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
          Failed to Load Experiment
        </Typography>
        <Typography
          variant="body1"
          color="text.secondary"
          sx={{ mb: 3, maxWidth: 480, mx: 'auto' }}
        >
          {error}
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

  const canRun =
    experiment.status === 'pending' ||
    experiment.status === 'completed' ||
    experiment.status === 'failed' ||
    experiment.status === 'stopped' ||
    experiment.status === 'timed_out';

  const canStop = experiment.status === 'running' || experiment.status === 'queued';

  const isRunning = experiment.status === 'running';
  const isCompleted = experiment.status === 'completed';
  const isFailed = experiment.status === 'failed';

  const pods: PodStatus[] = currentRun?.podStatuses ?? [];
  const hasResult = experiment.result !== undefined && experiment.result !== null;
  const hasSIEMValidation = hasResult && experiment.result!.siemValidation !== undefined;

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
            : isRunning
              ? 'primary.main'
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
            {/* Left side: Name + meta */}
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
                  animated={isRunning}
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

            {/* Right side: Action buttons */}
            <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
              <Tooltip title="Refresh">
                <IconButton onClick={handleRefresh} disabled={isLoading}>
                  <RefreshIcon />
                </IconButton>
              </Tooltip>

              {canRun && (
                <Button
                  variant="contained"
                  color="primary"
                  startIcon={<RunIcon />}
                  onClick={handleRun}
                  disabled={executeStatus === 'loading'}
                  sx={{ minWidth: 100 }}
                >
                  {executeStatus === 'loading' ? 'Starting...' : 'Run'}
                </Button>
              )}

              {canStop && (
                <Button
                  variant="contained"
                  color="error"
                  startIcon={<StopIcon />}
                  onClick={handleStop}
                  disabled={stopStatus === 'loading'}
                  sx={{ minWidth: 100 }}
                >
                  {stopStatus === 'loading' ? 'Stopping...' : 'Stop'}
                </Button>
              )}

              <Tooltip title="Edit Experiment">
                <IconButton>
                  <EditIcon />
                </IconButton>
              </Tooltip>

              <Tooltip title="Delete Experiment">
                <IconButton color="error">
                  <TrashIcon />
                </IconButton>
              </Tooltip>
            </Stack>
          </Stack>

          {/* Progress bar for running experiments */}
          {isRunning && (
            <Box sx={{ mt: 2 }}>
              <Stack direction="row" justifyContent="space-between" mb={0.5}>
                <Typography variant="caption" color="text.secondary">
                  Progress
                </Typography>
                <Typography variant="caption" fontWeight={600} color="primary">
                  {experiment.progress}%
                </Typography>
              </Stack>
              <LinearProgress
                variant="determinate"
                value={experiment.progress}
                sx={{
                  height: 8,
                  borderRadius: 4,
                  backgroundColor: theme.palette.divider,
                  '& .MuiLinearProgress-bar': {
                    borderRadius: 4,
                  },
                }}
              />
            </Box>
          )}
        </Box>
      </Paper>

      {/* ----------------------------------------------------------------- */}
      {/* Quick Status Alerts                                               */}
      {/* ----------------------------------------------------------------- */}

      {executeStatus === 'succeeded' && (
        <Fade in>
          <Alert
            severity="success"
            sx={{ mb: 2, borderRadius: 2 }}
            onClose={() => dispatch(resetExecuteStatus())}
          >
            <AlertTitle>Experiment Started</AlertTitle>
            The experiment is now running. You can monitor progress below.
          </Alert>
        </Fade>
      )}

      {executeStatus === 'failed' && (
        <Fade in>
          <Alert
            severity="error"
            sx={{ mb: 2, borderRadius: 2 }}
            onClose={() => dispatch(resetExecuteStatus())}
          >
            <AlertTitle>Failed to Start</AlertTitle>
            The experiment could not be started. Please check the configuration and try
            again.
          </Alert>
        </Fade>
      )}

      {stopStatus === 'succeeded' && (
        <Fade in>
          <Alert
            severity="warning"
            sx={{ mb: 2, borderRadius: 2 }}
            onClose={() => dispatch(resetStopStatus())}
          >
            <AlertTitle>Experiment Stopped</AlertTitle>
            The experiment has been stopped successfully.
          </Alert>
        </Fade>
      )}

      {/* ----------------------------------------------------------------- */}
      {/* Tab Navigation                                                    */}
      {/* ----------------------------------------------------------------- */}

      <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
        <Box sx={{ borderBottom: 1, borderColor: 'divider', px: 2 }}>
          <Tabs
            value={activeTab}
            onChange={(_, newValue) => setActiveTab(newValue)}
            variant={isMobile ? 'scrollable' : 'standard'}
            scrollButtons="auto"
          >
            <Tab
              label="Overview"
              icon={<ExperimentIcon sx={{ fontSize: 18 }} />}
              iconPosition="start"
              sx={{ minHeight: 48, textTransform: 'none', fontWeight: 600 }}
            />
            <Tab
              label="Logs"
              icon={<LogIcon sx={{ fontSize: 18 }} />}
              iconPosition="start"
              sx={{ minHeight: 48, textTransform: 'none', fontWeight: 600 }}
            />
            <Tab
              label="Pods"
              icon={<PodIcon sx={{ fontSize: 18 }} />}
              iconPosition="start"
              sx={{ minHeight: 48, textTransform: 'none', fontWeight: 600 }}
            />
            {(isCompleted || isFailed) && (
              <Tab
                label="Results"
                icon={<ResultsIcon sx={{ fontSize: 18 }} />}
                iconPosition="start"
                sx={{ minHeight: 48, textTransform: 'none', fontWeight: 600 }}
              />
            )}
            {hasSIEMValidation && (
              <Tab
                label="SIEM Validation"
                icon={<SIEMIcon sx={{ fontSize: 18 }} />}
                iconPosition="start"
                sx={{ minHeight: 48, textTransform: 'none', fontWeight: 600 }}
              />
            )}
          </Tabs>
        </Box>

        <Box sx={{ p: 2.5 }}>
          {/* Tab 0: Overview (Progress Tracker) */}
          <TabPanel value={activeTab} index={0}>
            <ProgressTracker steps={experiment.steps} />

            {/* Quick info cards */}
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mt: 3 }}>
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
                <Typography
                  variant="body1"
                  fontWeight={600}
                  sx={{ fontFamily: 'monospace' }}
                >
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
                        sx={{ fontFamily: 'monospace' }}
                      >
                        {JSON.stringify(value)}
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
          </TabPanel>

          {/* Tab 1: Logs */}
          <TabPanel value={activeTab} index={1}>
            <LogViewer
              logs={logs}
              isLoading={isLoading}
              onRefresh={handleRefreshLogs}
              experimentId={id ?? ''}
            />
          </TabPanel>

          {/* Tab 2: Pods */}
          <TabPanel value={activeTab} index={2}>
            <PodStatusSection pods={pods} isLoading={isLoading} />
          </TabPanel>

          {/* Tab 3: Results (only when completed or failed) */}
          <TabPanel value={activeTab} index={3}>
            {hasResult ? (
              <ResultsSummary result={experiment.result!} />
            ) : (
              <Box sx={{ py: 6, textAlign: 'center' }}>
                <ResultsIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
                <Typography variant="body1" color="text.secondary">
                  No results available yet.
                </Typography>
                <Typography variant="body2" color="text.disabled" sx={{ mt: 0.5 }}>
                  Results will be shown once the experiment completes.
                </Typography>
              </Box>
            )}
          </TabPanel>

          {/* Tab 4: SIEM Validation (only when available) */}
          <TabPanel value={activeTab} index={4}>
            {hasSIEMValidation ? (
              <SIEMValidationSection validation={experiment.result!.siemValidation} />
            ) : (
              <Box sx={{ py: 6, textAlign: 'center' }}>
                <SIEMIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
                <Typography variant="body1" color="text.secondary">
                  No SIEM validation data available.
                </Typography>
              </Box>
            )}
          </TabPanel>
        </Box>
      </Paper>
    </Box>
  );
};

export default ExperimentDetailPage;
