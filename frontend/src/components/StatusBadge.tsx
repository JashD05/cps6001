import ArchiveIcon from '@mui/icons-material/Archive';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import CloudOffIcon from '@mui/icons-material/CloudOff';
import ErrorIcon from '@mui/icons-material/Error';
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord';
import HealthAndSafetyIcon from '@mui/icons-material/HealthAndSafety';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import PauseIcon from '@mui/icons-material/Pause';
import PendingIcon from '@mui/icons-material/Pending';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import SearchIcon from '@mui/icons-material/Search';
import SecurityIcon from '@mui/icons-material/Security';
import StopIcon from '@mui/icons-material/Stop';
import WarningIcon from '@mui/icons-material/Warning';
import { Box, Chip, Tooltip, Typography, type SxProps, type Theme } from '@mui/material';
import React, { memo } from 'react';

// ---------------------------------------------------------------------------
// Status Type Mappings
// ---------------------------------------------------------------------------

export type StatusVariant = 'dot' | 'pill' | 'icon';
export type StatusSize = 'small' | 'medium' | 'large';

export type ExperimentStatus =
  | 'draft'
  | 'active'
  | 'pending'
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'stopped'
  | 'timed_out'
  | 'archived';

export type ClusterStatus = 'healthy' | 'degraded' | 'unreachable' | 'unknown';

export type ValidationStatus =
  | 'validated'
  | 'invalid'
  | 'pending'
  | 'in_progress'
  | 'skipped';

export type AlertStatus =
  | 'new'
  | 'acknowledged'
  | 'investigating'
  | 'resolved'
  | 'false_positive';

export type GeneralStatus =
  | 'active'
  | 'inactive'
  | 'healthy'
  | 'unhealthy'
  | 'success'
  | 'warning'
  | 'error'
  | 'info';

export type StatusType =
  | ExperimentStatus
  | ClusterStatus
  | ValidationStatus
  | AlertStatus
  | GeneralStatus;

// ---------------------------------------------------------------------------
// Status Configuration
// ---------------------------------------------------------------------------

interface StatusConfig {
  label: string;
  color: string;
  bgColor: string;
  borderColor: string;
  icon: React.ElementType;
}

const statusConfigMap: Record<string, StatusConfig> = {
  // Experiment statuses
  draft: {
    label: 'Draft',
    color: '#9CA3AF',
    bgColor: 'rgba(156, 163, 175, 0.12)',
    borderColor: 'rgba(156, 163, 175, 0.24)',
    icon: PendingIcon,
  },
  pending: {
    label: 'Pending',
    color: '#64748B',
    bgColor: 'rgba(100, 116, 139, 0.12)',
    borderColor: 'rgba(100, 116, 139, 0.24)',
    icon: PendingIcon,
  },
  queued: {
    label: 'Queued',
    color: '#8B5CF6',
    bgColor: 'rgba(139, 92, 246, 0.12)',
    borderColor: 'rgba(139, 92, 246, 0.24)',
    icon: PendingIcon,
  },
  running: {
    label: 'Running',
    color: '#2563EB',
    bgColor: 'rgba(37, 99, 235, 0.12)',
    borderColor: 'rgba(37, 99, 235, 0.24)',
    icon: PlayArrowIcon,
  },
  completed: {
    label: 'Completed',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.24)',
    icon: CheckCircleIcon,
  },
  failed: {
    label: 'Failed',
    color: '#EF4444',
    bgColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.24)',
    icon: ErrorIcon,
  },
  stopped: {
    label: 'Stopped',
    color: '#F59E0B',
    bgColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.24)',
    icon: StopIcon,
  },
  timed_out: {
    label: 'Timed Out',
    color: '#F59E0B',
    bgColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.24)',
    icon: PauseIcon,
  },
  archived: {
    label: 'Archived',
    color: '#9CA3AF',
    bgColor: 'rgba(156, 163, 175, 0.12)',
    borderColor: 'rgba(156, 163, 175, 0.24)',
    icon: ArchiveIcon,
  },

  // Cluster statuses
  healthy: {
    label: 'Healthy',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.24)',
    icon: HealthAndSafetyIcon,
  },
  degraded: {
    label: 'Degraded',
    color: '#F59E0B',
    bgColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.24)',
    icon: WarningIcon,
  },
  unreachable: {
    label: 'Unreachable',
    color: '#EF4444',
    bgColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.24)',
    icon: CloudOffIcon,
  },
  unknown: {
    label: 'Unknown',
    color: '#94A3B8',
    bgColor: 'rgba(148, 163, 184, 0.12)',
    borderColor: 'rgba(148, 163, 184, 0.24)',
    icon: HelpOutlineIcon,
  },

  // Validation statuses
  validated: {
    label: 'Validated',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.24)',
    icon: SecurityIcon,
  },
  invalid: {
    label: 'Invalid',
    color: '#EF4444',
    bgColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.24)',
    icon: ErrorIcon,
  },
  in_progress: {
    label: 'In Progress',
    color: '#2563EB',
    bgColor: 'rgba(37, 99, 235, 0.12)',
    borderColor: 'rgba(37, 99, 235, 0.24)',
    icon: SearchIcon,
  },
  skipped: {
    label: 'Skipped',
    color: '#94A3B8',
    bgColor: 'rgba(148, 163, 184, 0.12)',
    borderColor: 'rgba(148, 163, 184, 0.24)',
    icon: PauseIcon,
  },

  // Alert statuses
  new: {
    label: 'New',
    color: '#2563EB',
    bgColor: 'rgba(37, 99, 235, 0.12)',
    borderColor: 'rgba(37, 99, 235, 0.24)',
    icon: FiberManualRecordIcon,
  },
  acknowledged: {
    label: 'Acknowledged',
    color: '#8B5CF6',
    bgColor: 'rgba(139, 92, 246, 0.12)',
    borderColor: 'rgba(139, 92, 246, 0.24)',
    icon: CheckCircleIcon,
  },
  investigating: {
    label: 'Investigating',
    color: '#F59E0B',
    bgColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.24)',
    icon: SearchIcon,
  },
  resolved: {
    label: 'Resolved',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.24)',
    icon: CheckCircleIcon,
  },
  false_positive: {
    label: 'False Positive',
    color: '#94A3B8',
    bgColor: 'rgba(148, 163, 184, 0.12)',
    borderColor: 'rgba(148, 163, 184, 0.24)',
    icon: WarningIcon,
  },

  // General statuses
  active: {
    label: 'Active',
    color: '#2563EB',
    bgColor: 'rgba(37, 99, 235, 0.12)',
    borderColor: 'rgba(37, 99, 235, 0.24)',
    icon: PlayArrowIcon,
  },
  inactive: {
    label: 'Inactive',
    color: '#94A3B8',
    bgColor: 'rgba(148, 163, 184, 0.12)',
    borderColor: 'rgba(148, 163, 184, 0.24)',
    icon: FiberManualRecordIcon,
  },
  unhealthy: {
    label: 'Unhealthy',
    color: '#EF4444',
    bgColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.24)',
    icon: ErrorIcon,
  },
  success: {
    label: 'Success',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.24)',
    icon: CheckCircleIcon,
  },
  warning: {
    label: 'Warning',
    color: '#F59E0B',
    bgColor: 'rgba(245, 158, 11, 0.12)',
    borderColor: 'rgba(245, 158, 11, 0.24)',
    icon: WarningIcon,
  },
  error: {
    label: 'Error',
    color: '#EF4444',
    bgColor: 'rgba(239, 68, 68, 0.12)',
    borderColor: 'rgba(239, 68, 68, 0.24)',
    icon: ErrorIcon,
  },
  info: {
    label: 'Info',
    color: '#06B6D4',
    bgColor: 'rgba(6, 182, 212, 0.12)',
    borderColor: 'rgba(6, 182, 212, 0.24)',
    icon: HelpOutlineIcon,
  },
};

// ---------------------------------------------------------------------------
// Default fallback config
// ---------------------------------------------------------------------------

const defaultConfig: StatusConfig = {
  label: 'Unknown',
  color: '#94A3B8',
  bgColor: 'rgba(148, 163, 184, 0.12)',
  borderColor: 'rgba(148, 163, 184, 0.24)',
  icon: HelpOutlineIcon,
};

// ---------------------------------------------------------------------------
// Size configurations
// ---------------------------------------------------------------------------

interface SizeConfig {
  dotSize: number;
  fontSize: number;
  iconSize: number;
  paddingX: number;
  paddingY: number;
  minHeight: number;
  gap: number;
}

const sizeConfigs: Record<StatusSize, SizeConfig> = {
  small: {
    dotSize: 6,
    fontSize: 11,
    iconSize: 12,
    paddingX: 6,
    paddingY: 1,
    minHeight: 20,
    gap: 4,
  },
  medium: {
    dotSize: 8,
    fontSize: 12,
    iconSize: 14,
    paddingX: 10,
    paddingY: 3,
    minHeight: 26,
    gap: 5,
  },
  large: {
    dotSize: 10,
    fontSize: 13,
    iconSize: 16,
    paddingX: 14,
    paddingY: 4,
    minHeight: 30,
    gap: 6,
  },
};

// ---------------------------------------------------------------------------
// Helper: format raw status string to config key
// ---------------------------------------------------------------------------

const normalizeStatus = (status: string): string => {
  return status
    .toLowerCase()
    .replace(/[\s-]/g, '_')
    .replace(/[^a-z_]/g, '');
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface StatusBadgeProps {
  /** The status value to display */
  status: StatusType | string;
  /** Display variant: dot (circle indicator + text), pill (colored chip), icon (icon + text) */
  variant?: StatusVariant;
  /** Size of the badge */
  size?: StatusSize;
  /** Optional custom label override (defaults to auto-generated from status) */
  label?: string;
  /** Optional tooltip text (defaults to auto-generated from status) */
  tooltip?: string;
  /** Whether to show the label text alongside the indicator */
  showLabel?: boolean;
  /** Optional MUI sx prop overrides */
  sx?: SxProps<Theme>;
  /** Optional class name */
  className?: string;
  /** Click handler */
  onClick?: (event: React.MouseEvent<HTMLDivElement>) => void;
  /** Whether this is an animated badge (e.g., pulsing for running status) */
  animated?: boolean;
}

// ---------------------------------------------------------------------------
// Component: StatusBadge
// ---------------------------------------------------------------------------

const StatusBadge: React.FC<StatusBadgeProps> = ({
  status,
  variant = 'pill',
  size = 'medium',
  label,
  tooltip,
  showLabel = true,
  sx,
  className,
  onClick,
  animated = false,
}) => {
  const normalizedStatus = normalizeStatus(status);
  const config = statusConfigMap[normalizedStatus] ?? defaultConfig;
  const displayLabel = label ?? config.label;
  const displayTooltip = tooltip ?? displayLabel;
  const sizeConfig = sizeConfigs[size];
  const StatusIcon = config.icon;

  // -----------------------------------------------------------------------
  // Dot variant
  // -----------------------------------------------------------------------

  const renderDot = () => (
    <Box
      onClick={onClick}
      className={className}
      data-testid="status-badge-dot"
      data-animation={animated && normalizedStatus === 'running' ? 'pulse' : undefined}
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: sizeConfig.gap,
        cursor: onClick ? 'pointer' : 'default',
        ...(sx as Record<string, unknown>),
      }}
    >
      <Box
        sx={{
          width: sizeConfig.dotSize,
          height: sizeConfig.dotSize,
          borderRadius: '50%',
          backgroundColor: config.color,
          flexShrink: 0,
          '@keyframes pulse': {
            '0%': {
              boxShadow: `0 0 0 0 ${config.color}66`,
            },
            '70%': {
              boxShadow: `0 0 0 ${sizeConfig.dotSize}px ${config.color}00`,
            },
            '100%': {
              boxShadow: `0 0 0 0 ${config.color}00`,
            },
          },
          ...(animated && {
            animation: 'pulse 2s ease-in-out infinite',
          }),
        }}
      />
      {showLabel && (
        <Typography
          sx={{
            fontSize: sizeConfig.fontSize,
            fontWeight: 600,
            lineHeight: 1,
            color: config.color,
            whiteSpace: 'nowrap',
          }}
        >
          {displayLabel}
        </Typography>
      )}
    </Box>
  );

  // -----------------------------------------------------------------------
  // Pill variant (uses MUI Chip)
  // -----------------------------------------------------------------------

  const renderPill = () => (
    <Chip
      icon={
        variant === 'icon' ? (
          <StatusIcon
            sx={{
              fontSize: sizeConfig.iconSize,
              color: `${config.color} !important`,
              marginLeft: '2px',
            }}
          />
        ) : undefined
      }
      label={
        <Box
          component="span"
          sx={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 0.5,
          }}
        >
          {variant !== 'icon' && (
            <Box
              component="span"
              sx={{
                width: sizeConfig.dotSize,
                height: sizeConfig.dotSize,
                borderRadius: '50%',
                backgroundColor: config.color,
                display: 'inline-block',
                flexShrink: 0,
                '@keyframes chipPulse': {
                  '0%': {
                    boxShadow: `0 0 0 0 ${config.color}66`,
                  },
                  '70%': {
                    boxShadow: `0 0 0 ${sizeConfig.dotSize + 2}px ${config.color}00`,
                  },
                  '100%': {
                    boxShadow: `0 0 0 0 ${config.color}00`,
                  },
                },
                ...(animated && {
                  animation: 'chipPulse 2s ease-in-out infinite',
                }),
              }}
            />
          )}
          {displayLabel}
        </Box>
      }
      size={size === 'small' ? 'small' : 'medium'}
      onClick={onClick}
      className={className}
      data-testid="status-badge-pill"
      data-animation={
        animated && normalizedStatus === 'running' ? 'chipPulse' : undefined
      }
      sx={{
        backgroundColor: config.bgColor,
        color: config.color,
        borderColor: config.borderColor,
        border: `1px solid ${config.borderColor}`,
        fontWeight: 600,
        fontSize: sizeConfig.fontSize,
        fontFamily:
          "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
        height: sizeConfig.minHeight,
        minHeight: sizeConfig.minHeight,
        '& .MuiChip-label': {
          padding: `0 ${size === 'small' ? 4 : 8}px`,
          paddingRight: size === 'small' ? 8 : 12,
          whiteSpace: 'nowrap',
        },
        '& .MuiChip-icon': {
          color: config.color,
        },
        cursor: onClick ? 'pointer' : 'default',
        ...(sx as Record<string, unknown>),
      }}
    />
  );

  // -----------------------------------------------------------------------
  // Icon variant
  // -----------------------------------------------------------------------

  const renderIcon = () => (
    <Box
      onClick={onClick}
      className={className}
      data-testid="status-badge-icon"
      data-animation={
        animated && normalizedStatus === 'running' ? 'iconPulse' : undefined
      }
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: sizeConfig.gap,
        cursor: onClick ? 'pointer' : 'default',
        ...(sx as Record<string, unknown>),
      }}
    >
      <StatusIcon
        sx={{
          fontSize: sizeConfig.iconSize,
          color: config.color,
          flexShrink: 0,
          '@keyframes iconPulse': {
            '0%': {
              transform: 'scale(1)',
            },
            '50%': {
              transform: 'scale(1.15)',
            },
            '100%': {
              transform: 'scale(1)',
            },
          },
          ...(animated && {
            animation: 'iconPulse 2s ease-in-out infinite',
          }),
        }}
      />
      {showLabel && (
        <Typography
          sx={{
            fontSize: sizeConfig.fontSize,
            fontWeight: 600,
            lineHeight: 1,
            color: config.color,
            whiteSpace: 'nowrap',
          }}
        >
          {displayLabel}
        </Typography>
      )}
    </Box>
  );

  // -----------------------------------------------------------------------
  // Render the selected variant wrapped in a Tooltip
  // -----------------------------------------------------------------------

  const renderVariant = () => {
    switch (variant) {
      case 'dot':
        return renderDot();
      case 'icon':
        return renderIcon();
      case 'pill':
      default:
        return renderPill();
    }
  };

  return (
    <Tooltip title={displayTooltip} arrow placement="top">
      <Box
        component="span"
        sx={{ display: 'inline-flex', verticalAlign: 'middle' }}
        data-testid="status-badge-root"
      >
        {renderVariant()}
      </Box>
    </Tooltip>
  );
};

// ---------------------------------------------------------------------------
// Utility: get status color without rendering the component
// ---------------------------------------------------------------------------

export const getStatusColor = (status: StatusType | string): string => {
  const normalizedStatus = normalizeStatus(status);
  return (statusConfigMap[normalizedStatus] ?? defaultConfig).color;
};

// const getStatusBgColor = (status: StatusType | string): string => {
//   const normalizedStatus = normalizeStatus(status);
//   return (statusConfigMap[normalizedStatus] ?? defaultConfig).bgColor;
// };

// export { getStatusBgColor };

export const getStatusConfig = (
  status: StatusType | string,
): StatusConfig & { normalizedStatus: string } => {
  const normalizedStatus = normalizeStatus(status);
  const config = statusConfigMap[normalizedStatus] ?? defaultConfig;
  return { ...config, normalizedStatus };
};

// ---------------------------------------------------------------------------
// Memoized export
// ---------------------------------------------------------------------------

export default memo(StatusBadge);
