import React from 'react';
import {
  Box,
  Card,
  CardContent,
  CardActions,
  Typography,
  Button,
  IconButton,
  LinearProgress,
  Tooltip,
  Chip,
  Stack,
  Divider,
} from '@mui/material';
import {
  PlayArrow as PlayIcon,
  Stop as StopIcon,
  Visibility as ViewIcon,
  Schedule as ScheduleIcon,
  Timer as TimerIcon,
  Dns as ClusterIcon,
  Science as TemplateIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import StatusBadge from '@/components/StatusBadge';
import type { Experiment } from '@/types';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ExperimentCardVariant = 'compact' | 'full';

export interface ExperimentCardProps {
  /** The experiment data to display */
  experiment: Experiment;
  /** Card variant – compact shows essential info only, full shows all details */
  variant?: ExperimentCardVariant;
  /** Whether to show action buttons */
  showActions?: boolean;
  /** Custom click handler – overrides default navigation */
  onClick?: (experiment: Experiment) => void;
  /** Handler for the Run button */
  onRun?: (experiment: Experiment) => void;
  /** Handler for the Stop button */
  onStop?: (experiment: Experiment) => void;
  /** Handler for the View button */
  onView?: (experiment: Experiment) => void;
  /** Whether the card is currently loading / performing an action */
  loading?: boolean;
  /** Additional sx styles */
  sx?: object;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const formatRelativeTime = (dateStr: string | undefined): string => {
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
  if (diffDays < 30) return `${diffDays}d ago`;
  return date.toLocaleDateString();
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

const getStatusActions = (
  status: Experiment['status'],
): { canRun: boolean; canStop: boolean } => {
  switch (status) {
    case 'draft':
    case 'active':
    case 'pending':
    case 'queued':
      return { canRun: true, canStop: false };
    case 'running':
      return { canRun: false, canStop: true };
    case 'completed':
    case 'failed':
    case 'stopped':
    case 'timed_out':
    case 'archived':
      return { canRun: true, canStop: false };
    default:
      return { canRun: false, canStop: false };
  }
};

// ---------------------------------------------------------------------------
// Compact Variant
// ---------------------------------------------------------------------------

const CompactExperimentCard: React.FC<ExperimentCardProps> = ({
  experiment,
  showActions = true,
  onClick,
  onRun,
  onStop,
  onView,
  loading,
}) => {
  const navigate = useNavigate();
  const { canRun, canStop } = getStatusActions(experiment.status);

  const handleClick = (): void => {
    if (onClick) {
      onClick(experiment);
    } else {
      navigate(`/experiments/${experiment.id}`);
    }
  };

  const handleView = (e: React.MouseEvent): void => {
    e.stopPropagation();
    if (onView) {
      onView(experiment);
    } else {
      navigate(`/experiments/${experiment.id}`);
    }
  };

  const handleRun = (e: React.MouseEvent): void => {
    e.stopPropagation();
    onRun?.(experiment);
  };

  const handleStop = (e: React.MouseEvent): void => {
    e.stopPropagation();
    onStop?.(experiment);
  };

  return (
    <Card
      sx={{
        cursor: 'pointer',
        transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
        '&:hover': {
          borderColor: 'primary.main',
          transform: 'translateY(-1px)',
        },
      }}
      onClick={handleClick}
    >
      <CardContent sx={{ py: 1.5, px: 2, '&:last-child': { pb: 1.5 } }}>
        <Stack
          direction="row"
          alignItems="center"
          justifyContent="space-between"
          spacing={1.5}
        >
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <Typography
                variant="body2"
                fontWeight={600}
                noWrap
                sx={{ flex: 1, minWidth: 0 }}
              >
                {experiment.name}
              </Typography>
              <StatusBadge
                status={experiment.status}
                variant="pill"
                size="small"
                label={experiment.status === 'pending' ? 'Draft' : undefined}
              />
            </Stack>
            <Stack direction="row" spacing={1.5} mt={0.5}>
              {experiment.templateName && (
                <Typography variant="caption" color="text.secondary" noWrap>
                  {experiment.templateName}
                </Typography>
              )}
              {experiment.clusterName && (
                <Typography variant="caption" color="text.secondary" noWrap>
                  · {experiment.clusterName}
                </Typography>
              )}
            </Stack>
          </Box>

          {showActions && (
            <Stack direction="row" spacing={0.5} onClick={(e) => e.stopPropagation()}>
              <Tooltip title="View Details">
                <IconButton size="small" onClick={handleView} disabled={loading}>
                  <ViewIcon fontSize="small" />
                </IconButton>
              </Tooltip>
              {canRun && (
                <Tooltip title="Run Experiment">
                  <IconButton
                    size="small"
                    color="primary"
                    onClick={handleRun}
                    disabled={loading}
                  >
                    <PlayIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              )}
              {canStop && (
                <Tooltip title="Stop Experiment">
                  <IconButton
                    size="small"
                    color="error"
                    onClick={handleStop}
                    disabled={loading}
                  >
                    <StopIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              )}
            </Stack>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
};

// ---------------------------------------------------------------------------
// Full Variant
// ---------------------------------------------------------------------------

const FullExperimentCard: React.FC<ExperimentCardProps> = ({
  experiment,
  showActions = true,
  onClick,
  onRun,
  onStop,
  onView,
  loading,
}) => {
  const navigate = useNavigate();
  const { canRun, canStop } = getStatusActions(experiment.status);

  const handleClick = (): void => {
    if (onClick) {
      onClick(experiment);
    } else {
      navigate(`/experiments/${experiment.id}`);
    }
  };

  const handleView = (e: React.MouseEvent): void => {
    e.stopPropagation();
    if (onView) {
      onView(experiment);
    } else {
      navigate(`/experiments/${experiment.id}`);
    }
  };

  const handleRun = (e: React.MouseEvent): void => {
    e.stopPropagation();
    onRun?.(experiment);
  };

  const handleStop = (e: React.MouseEvent): void => {
    e.stopPropagation();
    onStop?.(experiment);
  };

  const showProgress =
    experiment.status === 'running' ||
    experiment.status === 'queued' ||
    experiment.progress > 0;

  return (
    <Card
      sx={{
        cursor: 'pointer',
        transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
        '&:hover': {
          borderColor: 'primary.main',
          transform: 'translateY(-2px)',
          boxShadow: '0 8px 24px rgba(0,0,0,0.1)',
        },
      }}
      onClick={handleClick}
    >
      <CardContent>
        {/* Header: Name + Status */}
        <Stack
          direction="row"
          alignItems="flex-start"
          justifyContent="space-between"
          spacing={1}
        >
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography variant="subtitle1" fontWeight={700} noWrap gutterBottom>
              {experiment.name}
            </Typography>
            {experiment.description && (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{
                  display: '-webkit-box',
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: 'vertical',
                  overflow: 'hidden',
                  mb: 1.5,
                }}
              >
                {experiment.description}
              </Typography>
            )}
          </Box>
          <StatusBadge status={experiment.status} variant="pill" size="medium" />
        </Stack>

        {/* Progress Bar */}
        {showProgress && (
          <Box mb={2}>
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
                height: 6,
                borderRadius: 3,
                backgroundColor: 'grey.100',
                '& .MuiLinearProgress-bar': {
                  borderRadius: 3,
                  ...(experiment.status === 'failed' && {
                    backgroundColor: 'error.main',
                  }),
                  ...(experiment.status === 'completed' && {
                    backgroundColor: 'success.main',
                  }),
                },
              }}
            />
          </Box>
        )}

        <Divider sx={{ my: 1.5 }} />

        {/* Metadata Grid */}
        <Stack spacing={1}>
          {/* Template */}
          <Stack direction="row" alignItems="center" spacing={1}>
            <TemplateIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
            <Typography variant="caption" color="text.secondary">
              Template:
            </Typography>
            <Typography variant="caption" fontWeight={600}>
              {experiment.templateName || 'Custom'}
            </Typography>
          </Stack>

          {/* Cluster */}
          <Stack direction="row" alignItems="center" spacing={1}>
            <ClusterIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
            <Typography variant="caption" color="text.secondary">
              Cluster:
            </Typography>
            <Typography variant="caption" fontWeight={600}>
              {experiment.clusterName || experiment.clusterId}
            </Typography>
          </Stack>

          {/* Timing */}
          <Stack direction="row" alignItems="center" spacing={1}>
            <ScheduleIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
            <Typography variant="caption" color="text.secondary">
              {experiment.status === 'running' ? 'Started' : 'Created'}:
            </Typography>
            <Typography variant="caption" fontWeight={600}>
              {formatRelativeTime(
                experiment.status === 'running'
                  ? experiment.startedAt
                  : experiment.createdAt,
              )}
            </Typography>
          </Stack>

          {/* Duration (if completed or running) */}
          {(experiment.status === 'completed' ||
            experiment.status === 'failed' ||
            experiment.status === 'stopped') &&
            experiment.duration !== undefined && (
              <Stack direction="row" alignItems="center" spacing={1}>
                <TimerIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                <Typography variant="caption" color="text.secondary">
                  Duration:
                </Typography>
                <Typography variant="caption" fontWeight={600}>
                  {formatDuration(experiment.duration)}
                </Typography>
              </Stack>
            )}
        </Stack>

        {/* Tags */}
        {experiment.tags.length > 0 && (
          <Stack direction="row" spacing={0.5} mt={1.5} flexWrap="wrap" useFlexGap>
            {experiment.tags.slice(0, 3).map((tag) => (
              <Chip
                key={tag}
                label={tag}
                size="small"
                variant="outlined"
                sx={{ fontSize: '0.6875rem', height: 20 }}
              />
            ))}
            {experiment.tags.length > 3 && (
              <Chip
                label={`+${experiment.tags.length - 3}`}
                size="small"
                variant="outlined"
                sx={{ fontSize: '0.6875rem', height: 20 }}
              />
            )}
          </Stack>
        )}
      </CardContent>

      {/* Actions */}
      {showActions && (
        <>
          <Divider />
          <CardActions sx={{ px: 2, py: 1, justifyContent: 'flex-end' }}>
            <Button
              size="small"
              startIcon={<ViewIcon />}
              onClick={handleView}
              disabled={loading}
            >
              View
            </Button>
            {canRun && (
              <Button
                size="small"
                variant="contained"
                color="primary"
                startIcon={<PlayIcon />}
                onClick={handleRun}
                disabled={loading}
              >
                Run
              </Button>
            )}
            {canStop && (
              <Button
                size="small"
                variant="contained"
                color="error"
                startIcon={<StopIcon />}
                onClick={handleStop}
                disabled={loading}
              >
                Stop
              </Button>
            )}
          </CardActions>
        </>
      )}
    </Card>
  );
};

// ---------------------------------------------------------------------------
// Main Component
// ---------------------------------------------------------------------------

const ExperimentCard: React.FC<ExperimentCardProps> = ({
  variant = 'full',
  ...props
}) => {
  if (variant === 'compact') {
    return <CompactExperimentCard variant={variant} {...props} />;
  }
  return <FullExperimentCard variant={variant} {...props} />;
};

export default ExperimentCard;
