import {
  Add as AddIcon,
  Assessment as ReportIcon,
  Security as SecurityIcon,
  TrendingUp as TrendingUpIcon,
  TrendingDown as TrendingDownIcon,
  TrendingFlat as TrendingFlatIcon,
  Science as ExperimentIcon,
  PlayArrow as RunningIcon,
  CheckCircle as CompletedIcon,
  Error as FailedIcon,
  Dns as ClusterIcon,
  ArrowForward as ArrowForwardIcon,
  Refresh as RefreshIcon,
  Shield as ShieldIcon,
  Speed as SpeedIcon,
  Warning as WarningIcon,
} from '@mui/icons-material';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Grid,
  Stack,
  Button,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  LinearProgress,
  Chip,
  IconButton,
  Tooltip,
  Skeleton,
  Avatar,
  Divider,
  useTheme,
} from '@mui/material';
import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  Legend,
} from 'recharts';
import StatusBadge from '@/components/StatusBadge';
import { dashboardAPI } from '@/services/api';
import { useAppDispatch, useAppSelector } from '@/store';
import {
  fetchExperiments,
  selectExperimentListLoading,
  selectExperimentStats,
  selectRecentExperiments,
} from '@/store/experimentSlice';
import type {
  Experiment,
  ClusterHealth,
  DashboardSummary,
  ActivityTimelinePoint,
  ThreatCoverageCategory,
} from '@/types';

// ---------------------------------------------------------------------------
// Prototype dashboard data
// ---------------------------------------------------------------------------

const DAY_IN_MS = 24 * 60 * 60 * 1000;
const daysAgo = (days: number) => new Date(Date.now() - days * DAY_IN_MS).toISOString();

const _PROTOTYPE_SECURITY_POSTURE_HISTORY = [
  { date: daysAgo(28), score: 70 },
  { date: daysAgo(21), score: 72 },
  { date: daysAgo(14), score: 75 },
  { date: daysAgo(7), score: 77 },
  { date: daysAgo(0), score: 78 },
];

const _PROTOTYPE_DASHBOARD_SUMMARY: DashboardSummary = {
  securityPostureScore: 78,
  postureTrend: {
    direction: 'up',
    percentage: 8,
    period: 'last 30 days',
  },
  experimentSummary: {
    total: 5,
    running: 1,
    completed: 3,
    failed: 1,
    pending: 0,
  },
  recentExperiments: [
    {
      id: 'demo-exp-1',
      name: 'Pod Restart Drill',
      description: 'Validate alerting when pods are restarted during a run.',
      templateId: 'tpl-demo-1',
      templateName: 'Kubernetes Pod Drill',
      clusterId: 'cluster-prod',
      clusterName: 'Production Cluster',
      namespace: 'default',
      status: 'completed',
      progress: 100,
      parameters: { duration: '5m' },
      steps: [],
      tags: ['kubernetes', 'alerts'],
      createdBy: 'demo',
      createdAt: daysAgo(2),
      updatedAt: daysAgo(2),
      startedAt: daysAgo(2),
      completedAt: daysAgo(2),
      result: {
        success: true,
        score: 96,
        summary: 'Alerts fired within the expected detection window.',
        details: ['SIEM rule triggered', 'Alert acknowledged by analyst'],
        siemValidation: {
          expectedAlertCount: 1,
          receivedAlertCount: 1,
          alerts: [],
          detected: true,
          detectionLatencyMs: 42,
          coverage: 100,
          details: ['Validated in simulation'],
        },
        startedAt: daysAgo(2),
        completedAt: daysAgo(2),
        duration: 300,
      },
    },
    {
      id: 'demo-exp-2',
      name: 'Network Latency Spike',
      description: 'Simulated latency injection against backend services.',
      templateId: 'tpl-demo-2',
      templateName: 'Latency Injection',
      clusterId: 'cluster-prod',
      clusterName: 'Production Cluster',
      namespace: 'backend',
      status: 'running',
      progress: 58,
      parameters: { latency: '250ms', duration: '10m' },
      steps: [],
      tags: ['network', 'resilience'],
      createdBy: 'demo',
      createdAt: daysAgo(1),
      updatedAt: daysAgo(1),
      startedAt: daysAgo(1),
    },
    {
      id: 'demo-exp-3',
      name: 'DNS Failure Injection',
      description: 'Checks fallback DNS resolution under outage conditions.',
      templateId: 'tpl-demo-3',
      templateName: 'DNS Failure',
      clusterId: 'cluster-staging',
      clusterName: 'Staging Cluster',
      namespace: 'testing',
      status: 'failed',
      progress: 75,
      parameters: { window: '3m' },
      steps: [],
      tags: ['dns', 'failure'],
      createdBy: 'demo',
      createdAt: daysAgo(4),
      updatedAt: daysAgo(4),
      startedAt: daysAgo(4),
      completedAt: daysAgo(4),
      result: {
        success: false,
        score: 41,
        summary: 'The alert was delayed beyond the acceptable threshold.',
        details: ['Alert arrived after 2m 15s', 'Validation flagged as partial'],
        siemValidation: {
          expectedAlertCount: 1,
          receivedAlertCount: 0,
          alerts: [],
          detected: false,
          detectionLatencyMs: -1,
          coverage: 0,
          details: ['Alert missing in simulation'],
        },
        startedAt: daysAgo(4),
        completedAt: daysAgo(4),
        duration: 180,
      },
    },
    {
      id: 'demo-exp-4',
      name: 'CPU Pressure Test',
      description: 'Burst CPU load to validate auto-scaling response.',
      templateId: 'tpl-demo-4',
      templateName: 'Resource Stress',
      clusterId: 'cluster-qa',
      clusterName: 'QA Cluster',
      namespace: 'stress',
      status: 'pending',
      progress: 0,
      parameters: { cpu: '85%' },
      steps: [],
      tags: ['cpu', 'autoscaling'],
      createdBy: 'demo',
      createdAt: daysAgo(0),
      updatedAt: daysAgo(0),
    },
    {
      id: 'demo-exp-5',
      name: 'Memory Exhaustion Trial',
      description: 'Simulates memory pressure to verify graceful degradation.',
      templateId: 'tpl-demo-5',
      templateName: 'Memory Hog',
      clusterId: 'cluster-qa',
      clusterName: 'QA Cluster',
      namespace: 'stress',
      status: 'stopped',
      progress: 40,
      parameters: { memory: '2Gi' },
      steps: [],
      tags: ['memory'],
      createdBy: 'demo',
      createdAt: daysAgo(6),
      updatedAt: daysAgo(6),
      startedAt: daysAgo(6),
      completedAt: daysAgo(6),
    },
  ],
  clusterHealth: [
    {
      clusterId: 'cluster-prod',
      status: 'healthy',
      cpuUsage: 46,
      memoryUsage: 54,
      podCount: 82,
      nodeCount: 12,
      errorRate: 0.4,
      lastChecked: daysAgo(0),
    },
    {
      clusterId: 'cluster-staging',
      status: 'degraded',
      cpuUsage: 71,
      memoryUsage: 67,
      podCount: 24,
      nodeCount: 5,
      errorRate: 3.2,
      lastChecked: daysAgo(0),
    },
    {
      clusterId: 'cluster-qa',
      status: 'unreachable',
      cpuUsage: 0,
      memoryUsage: 0,
      podCount: 0,
      nodeCount: 0,
      errorRate: 100,
      lastChecked: daysAgo(1),
    },
  ],
  threatCoverage: {
    totalControls: 5,
    validated: 4,
    passed: 3,
    failed: 1,
    untested: 1,
    coverage: 80,
  },
  threatCoverageByCategory: [
    { name: 'Network', validated: 2, untested: 0 },
    { name: 'Compute', validated: 1, untested: 0 },
    { name: 'Access', validated: 1, untested: 1 },
    { name: 'SIEM', validated: 0, untested: 1 },
  ],
  experimentTrend: [
    { date: 'W1', total: 2, passed: 2, failed: 0 },
    { date: 'W2', total: 4, passed: 3, failed: 1 },
    { date: 'W3', total: 5, passed: 4, failed: 1 },
    { date: 'W4', total: 6, passed: 5, failed: 1 },
    { date: 'W5', total: 7, passed: 6, failed: 1 },
    { date: 'W6', total: 8, passed: 7, failed: 1 },
    { date: 'W7', total: 9, passed: 8, failed: 1 },
    { date: 'W8', total: 10, passed: 9, failed: 1 },
  ],
  topAttackTypes: [
    { name: 'Pod Restart', value: 12 },
    { name: 'DNS Failure', value: 9 },
    { name: 'CPU Stress', value: 8 },
    { name: 'Memory Hog', value: 6 },
    { name: 'Latency Spike', value: 5 },
  ],
  validationSuccessRate: [
    { timestamp: daysAgo(28), value: 70, label: 'Week 1' },
    { timestamp: daysAgo(21), value: 72, label: 'Week 2' },
    { timestamp: daysAgo(14), value: 75, label: 'Week 3' },
    { timestamp: daysAgo(7), value: 77, label: 'Week 4' },
    { timestamp: daysAgo(0), value: 78, label: 'Week 5' },
  ],
};

function mergeDashboardSummaryWithDemo(summary: DashboardSummary): DashboardSummary {
  const demo = _PROTOTYPE_DASHBOARD_SUMMARY;
  return {
    ...demo,
    ...summary,
    securityPostureScore:
      summary.securityPostureScore > 0
        ? summary.securityPostureScore
        : demo.securityPostureScore,
    postureTrend: summary.postureTrend ?? demo.postureTrend,
    experimentSummary: {
      total:
        summary.experimentSummary.total > 0
          ? summary.experimentSummary.total
          : demo.experimentSummary.total,
      running:
        summary.experimentSummary.running > 0
          ? summary.experimentSummary.running
          : demo.experimentSummary.running,
      completed:
        summary.experimentSummary.completed > 0
          ? summary.experimentSummary.completed
          : demo.experimentSummary.completed,
      failed:
        summary.experimentSummary.failed > 0
          ? summary.experimentSummary.failed
          : demo.experimentSummary.failed,
      pending:
        summary.experimentSummary.pending > 0
          ? summary.experimentSummary.pending
          : demo.experimentSummary.pending,
    },
    recentExperiments: summary.recentExperiments?.length
      ? summary.recentExperiments
      : demo.recentExperiments,
    clusterHealth: summary.clusterHealth?.length
      ? summary.clusterHealth
      : demo.clusterHealth,
    threatCoverage: {
      ...demo.threatCoverage,
      ...summary.threatCoverage,
      totalControls:
        summary.threatCoverage.totalControls > 0
          ? summary.threatCoverage.totalControls
          : demo.threatCoverage.totalControls,
      validated:
        summary.threatCoverage.validated > 0
          ? summary.threatCoverage.validated
          : demo.threatCoverage.validated,
      passed:
        summary.threatCoverage.passed > 0
          ? summary.threatCoverage.passed
          : demo.threatCoverage.passed,
      failed:
        summary.threatCoverage.failed > 0
          ? summary.threatCoverage.failed
          : demo.threatCoverage.failed,
      untested:
        summary.threatCoverage.untested > 0
          ? summary.threatCoverage.untested
          : demo.threatCoverage.untested,
      coverage:
        summary.threatCoverage.coverage > 0
          ? summary.threatCoverage.coverage
          : demo.threatCoverage.coverage,
    },
    threatCoverageByCategory: summary.threatCoverageByCategory?.length
      ? summary.threatCoverageByCategory
      : demo.threatCoverageByCategory,
    experimentTrend: summary.experimentTrend?.length
      ? summary.experimentTrend
      : demo.experimentTrend,
    topAttackTypes: summary.topAttackTypes?.length
      ? summary.topAttackTypes
      : demo.topAttackTypes,
    validationSuccessRate: summary.validationSuccessRate?.length
      ? summary.validationSuccessRate
      : demo.validationSuccessRate,
  };
}

function normalizeDashboardSummary(summary: DashboardSummary): DashboardSummary {
  return {
    ...summary,
    recentExperiments: summary.recentExperiments ?? [],
    clusterHealth: summary.clusterHealth ?? [],
    threatCoverageByCategory: summary.threatCoverageByCategory ?? [],
    experimentTrend: summary.experimentTrend ?? [],
    topAttackTypes: summary.topAttackTypes ?? [],
    validationSuccessRate: summary.validationSuccessRate ?? [],
    threatCoverage: summary.threatCoverage ?? {
      totalControls: 0,
      validated: 0,
      passed: 0,
      failed: 0,
      untested: 0,
      coverage: 0,
    },
  };
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Large number card for KPI display */
function KPICard({
  title,
  value,
  subtitle,
  icon,
  trend,
  color = 'primary',
  loading = false,
}: {
  title: string;
  value: number | string;
  subtitle?: string;
  icon: React.ReactNode;
  trend?: { direction: 'up' | 'down' | 'stable'; percentage: number; period: string };
  color?: 'primary' | 'secondary' | 'success' | 'warning' | 'error' | 'info';
  loading?: boolean;
}) {
  const theme = useTheme();

  const colorMap = {
    primary: theme.palette.primary,
    secondary: theme.palette.secondary,
    success: theme.palette.success,
    warning: theme.palette.warning,
    error: theme.palette.error,
    info: theme.palette.info,
  };

  const palette = colorMap[color];

  const TrendIcon =
    trend?.direction === 'up'
      ? TrendingUpIcon
      : trend?.direction === 'down'
        ? TrendingDownIcon
        : TrendingFlatIcon;

  const trendColor =
    trend?.direction === 'up'
      ? theme.palette.success.main
      : trend?.direction === 'down'
        ? theme.palette.error.main
        : theme.palette.text.secondary;

  return (
    <Card
      sx={{
        height: '100%',
        position: 'relative',
        overflow: 'hidden',
        transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
        '&:hover': {
          transform: 'translateY(-2px)',
          boxShadow: `0 8px 24px ${palette.main}18`,
        },
      }}
    >
      <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
        {loading ? (
          <Stack spacing={1.5}>
            <Skeleton variant="rounded" width={40} height={40} />
            <Skeleton variant="text" width="60%" />
            <Skeleton variant="text" width="40%" height={36} />
            <Skeleton variant="text" width="30%" />
          </Stack>
        ) : (
          <>
            <Stack
              direction="row"
              justifyContent="space-between"
              alignItems="flex-start"
              mb={2}
            >
              <Typography
                variant="overline"
                sx={{
                  fontWeight: 600,
                  letterSpacing: '0.06em',
                  color: 'text.secondary',
                  fontSize: '0.6875rem',
                }}
              >
                {title}
              </Typography>
              <Avatar
                variant="rounded"
                sx={{
                  width: 40,
                  height: 40,
                  backgroundColor: `${palette.main}14`,
                  color: palette.main,
                }}
              >
                {icon}
              </Avatar>
            </Stack>

            <Typography
              variant="h3"
              sx={{
                fontWeight: 800,
                lineHeight: 1.1,
                mb: 0.5,
                color: 'text.primary',
                fontSize: { xs: '2rem', sm: '2.5rem' },
              }}
            >
              {typeof value === 'number' ? value.toLocaleString() : value}
            </Typography>

            {subtitle && (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mb: 1, fontSize: '0.8125rem' }}
              >
                {subtitle}
              </Typography>
            )}

            {trend && (
              <Stack direction="row" spacing={0.5} alignItems="center">
                <TrendIcon sx={{ fontSize: 16, color: trendColor }} />
                <Typography
                  variant="caption"
                  sx={{ fontWeight: 600, color: trendColor, fontSize: '0.75rem' }}
                >
                  {trend.direction === 'stable'
                    ? '—'
                    : `${trend.direction === 'up' ? '+' : '-'}${trend.percentage}%`}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontSize: '0.6875rem' }}
                >
                  {trend.period}
                </Typography>
              </Stack>
            )}
          </>
        )}
      </CardContent>

      {/* Decorative gradient accent */}
      {!loading && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            right: 0,
            width: 100,
            height: 100,
            background: `radial-gradient(circle at top right, ${palette.main}0D, transparent 70%)`,
            pointerEvents: 'none',
          }}
        />
      )}
    </Card>
  );
}

/** Security Posture Score – large radial display */
function SecurityPostureCard({
  score,
  trend,
  history,
  loading = false,
}: {
  score: number;
  trend: DashboardSummary['postureTrend'];
  history: { date: string; score: number }[];
  loading?: boolean;
}) {
  const theme = useTheme();

  const scoreColor =
    score >= 80
      ? theme.palette.success.main
      : score >= 60
        ? theme.palette.warning.main
        : theme.palette.error.main;

  const scoreLabel =
    score >= 80 ? 'Strong' : score >= 60 ? 'Moderate' : score >= 40 ? 'Weak' : 'Critical';

  return (
    <Card sx={{ height: '100%' }}>
      <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
        {loading ? (
          <Stack spacing={2}>
            <Skeleton variant="text" width="50%" height={28} />
            <Skeleton variant="rounded" height={200} />
          </Stack>
        ) : (
          <>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              mb={2}
            >
              <Stack direction="row" spacing={1} alignItems="center">
                <ShieldIcon sx={{ color: 'primary.main', fontSize: 20 }} />
                <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
                  Security Posture
                </Typography>
              </Stack>
              <Chip
                label={scoreLabel}
                size="small"
                sx={{
                  backgroundColor: `${scoreColor}14`,
                  color: scoreColor,
                  fontWeight: 600,
                  fontSize: '0.6875rem',
                  border: `1px solid ${scoreColor}30`,
                }}
              />
            </Stack>

            {/* Large score display */}
            <Stack direction="row" alignItems="baseline" spacing={0.5} mb={0.5}>
              <Typography
                variant="h1"
                sx={{
                  fontWeight: 900,
                  fontSize: { xs: '3.5rem', sm: '4.5rem' },
                  lineHeight: 1,
                  background: `linear-gradient(135deg, ${scoreColor}, ${theme.palette.primary.main})`,
                  WebkitBackgroundClip: 'text',
                  WebkitTextFillColor: 'transparent',
                }}
              >
                {score}
              </Typography>
              <Typography
                variant="h4"
                sx={{
                  color: 'text.secondary',
                  fontWeight: 400,
                  fontSize: '1.5rem',
                }}
              >
                /100
              </Typography>
            </Stack>

            {/* Trend */}
            <Stack direction="row" spacing={0.5} alignItems="center" mb={3}>
              {trend.direction === 'up' && (
                <TrendingUpIcon sx={{ fontSize: 16, color: 'success.main' }} />
              )}
              {trend.direction === 'down' && (
                <TrendingDownIcon sx={{ fontSize: 16, color: 'error.main' }} />
              )}
              {trend.direction === 'stable' && (
                <TrendingFlatIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
              )}
              <Typography
                variant="caption"
                sx={{
                  fontWeight: 600,
                  color:
                    trend.direction === 'up'
                      ? 'success.main'
                      : trend.direction === 'down'
                        ? 'error.main'
                        : 'text.secondary',
                  fontSize: '0.75rem',
                }}
              >
                {trend.direction === 'stable'
                  ? 'No change'
                  : `${trend.direction === 'up' ? '+' : '-'}${trend.percentage}%`}
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontSize: '0.6875rem' }}
              >
                {trend.period}
              </Typography>
            </Stack>

            {/* Sparkline Chart */}
            <Box sx={{ height: 120, width: '100%' }}>
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart
                  data={history}
                  margin={{ top: 0, right: 0, left: 0, bottom: 0 }}
                >
                  <defs>
                    <linearGradient id="postureGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop
                        offset="5%"
                        stopColor={theme.palette.primary.main}
                        stopOpacity={0.2}
                      />
                      <stop
                        offset="95%"
                        stopColor={theme.palette.primary.main}
                        stopOpacity={0}
                      />
                    </linearGradient>
                  </defs>
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke={theme.palette.divider}
                    vertical={false}
                  />
                  <XAxis
                    dataKey="date"
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 11, fill: theme.palette.text.secondary }}
                    dy={8}
                  />
                  <YAxis
                    domain={[0, 100]}
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 11, fill: theme.palette.text.secondary }}
                    width={30}
                  />
                  <RechartsTooltip
                    contentStyle={{
                      backgroundColor: theme.palette.background.paper,
                      border: `1px solid ${theme.palette.divider}`,
                      borderRadius: 8,
                      boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
                      fontSize: 12,
                      fontFamily: "'Inter', sans-serif",
                    }}
                    formatter={(value: number) => [`${value}`, 'Score']}
                  />
                  <Area
                    type="monotone"
                    dataKey="score"
                    stroke={theme.palette.primary.main}
                    strokeWidth={2.5}
                    fill="url(#postureGradient)"
                    dot={false}
                    activeDot={{
                      r: 4,
                      fill: theme.palette.primary.main,
                      stroke: theme.palette.background.paper,
                      strokeWidth: 2,
                    }}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </Box>
          </>
        )}
      </CardContent>
    </Card>
  );
}

/** Cluster health summary row */
function ClusterHealthCard({
  cluster,
  loading = false,
}: {
  cluster: ClusterHealth;
  loading?: boolean;
}) {
  const theme = useTheme();

  if (loading) {
    return (
      <Card sx={{ p: 2 }}>
        <Stack spacing={1}>
          <Skeleton variant="text" width="60%" />
          <Skeleton variant="rounded" height={4} />
          <Skeleton variant="rounded" height={4} />
        </Stack>
      </Card>
    );
  }

  return (
    <Card
      sx={{
        p: 2,
        transition: 'all 150ms',
        '&:hover': {
          borderColor: 'primary.main',
          transform: 'translateY(-1px)',
        },
      }}
    >
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={1.5}>
        <Typography variant="subtitle2" sx={{ fontWeight: 600, fontSize: '0.8125rem' }}>
          Cluster {cluster.clusterId}
        </Typography>
        <StatusBadge status={cluster.status} variant="pill" size="small" />
      </Stack>

      <Stack spacing={1.25}>
        <Box>
          <Stack direction="row" justifyContent="space-between" mb={0.5}>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem' }}
            >
              CPU Usage
            </Typography>
            <Typography variant="caption" fontWeight={600} sx={{ fontSize: '0.6875rem' }}>
              {cluster.cpuUsage}%
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={cluster.cpuUsage}
            sx={{
              height: 4,
              borderRadius: 2,
              backgroundColor: theme.palette.divider,
              '& .MuiLinearProgress-bar': {
                borderRadius: 2,
                backgroundColor:
                  cluster.cpuUsage > 80
                    ? theme.palette.error.main
                    : cluster.cpuUsage > 60
                      ? theme.palette.warning.main
                      : theme.palette.success.main,
              },
            }}
          />
        </Box>

        <Box>
          <Stack direction="row" justifyContent="space-between" mb={0.5}>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem' }}
            >
              Memory Usage
            </Typography>
            <Typography variant="caption" fontWeight={600} sx={{ fontSize: '0.6875rem' }}>
              {cluster.memoryUsage}%
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={cluster.memoryUsage}
            sx={{
              height: 4,
              borderRadius: 2,
              backgroundColor: theme.palette.divider,
              '& .MuiLinearProgress-bar': {
                borderRadius: 2,
                backgroundColor:
                  cluster.memoryUsage > 80
                    ? theme.palette.error.main
                    : cluster.memoryUsage > 60
                      ? theme.palette.warning.main
                      : theme.palette.success.main,
              },
            }}
          />
        </Box>

        <Stack direction="row" spacing={2} mt={0.5}>
          <Stack direction="row" spacing={0.5} alignItems="center">
            <ClusterIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem' }}
            >
              {cluster.nodeCount} nodes
            </Typography>
          </Stack>
          <Stack direction="row" spacing={0.5} alignItems="center">
            <SpeedIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem' }}
            >
              {cluster.podCount} pods
            </Typography>
          </Stack>
        </Stack>
      </Stack>
    </Card>
  );
}

/** Recent experiments table */
function RecentExperimentsTable({
  experiments,
  loading = false,
}: {
  experiments: Experiment[];
  loading?: boolean;
}) {
  const navigate = useNavigate();
  const _theme = useTheme();

  const formatRelativeTime = (dateStr: string): string => {
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

  if (loading) {
    return (
      <Card>
        <CardContent sx={{ p: 0 }}>
          <Box sx={{ p: 3, pb: 1.5 }}>
            <Skeleton variant="text" width="40%" height={28} />
          </Box>
          {[1, 2, 3, 4, 5].map((i) => (
            <Box key={i} sx={{ px: 3, py: 1.5 }}>
              <Stack direction="row" spacing={2}>
                <Skeleton variant="text" width="25%" />
                <Skeleton variant="text" width="20%" />
                <Skeleton variant="text" width="15%" />
                <Skeleton variant="text" width="20%" />
              </Stack>
            </Box>
          ))}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          sx={{ px: 3, pt: 2.5, pb: 1.5 }}
        >
          <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
            Recent Experiments
          </Typography>
          <Button
            size="small"
            endIcon={<ArrowForwardIcon />}
            onClick={() => navigate('/experiments')}
            sx={{ textTransform: 'none', fontWeight: 600 }}
          >
            View All
          </Button>
        </Stack>

        {experiments.length === 0 ? (
          <Box sx={{ px: 3, pb: 3, pt: 4, textAlign: 'center' }}>
            <ExperimentIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
            <Typography variant="body2" color="text.secondary">
              No experiments yet. Create your first experiment to get started.
            </Typography>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => navigate('/experiments/new')}
              sx={{ mt: 2, textTransform: 'none' }}
            >
              New Experiment
            </Button>
          </Box>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Name</TableCell>
                  <TableCell>Template</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Cluster</TableCell>
                  <TableCell>Started</TableCell>
                  <TableCell align="right">Result</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {experiments.map((exp) => (
                  <TableRow
                    key={exp.id}
                    hover
                    onClick={() => navigate(`/experiments/${exp.id}`)}
                    sx={{ cursor: 'pointer' }}
                  >
                    <TableCell>
                      <Typography variant="body2" fontWeight={600} noWrap>
                        {exp.name}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary" noWrap>
                        {exp.templateName || 'Custom'}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={exp.status} variant="pill" size="small" />
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary" noWrap>
                        {exp.clusterName || exp.clusterId}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary" noWrap>
                        {formatRelativeTime(exp.startedAt ?? exp.createdAt)}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      {exp.result ? (
                        <Chip
                          label={exp.result.success ? 'Passed' : 'Failed'}
                          size="small"
                          color={exp.result.success ? 'success' : 'error'}
                          variant="outlined"
                          sx={{ height: 22, fontSize: '0.6875rem' }}
                        />
                      ) : (
                        <Typography variant="body2" color="text.disabled">
                          —
                        </Typography>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Chart Colors
// ---------------------------------------------------------------------------

const CHART_COLORS = [
  '#2563EB',
  '#7C3AED',
  '#10B981',
  '#F59E0B',
  '#EF4444',
  '#06B6D4',
  '#8B5CF6',
];

const getChartColor = (label: string) => {
  const hash = Array.from(label).reduce((acc, char) => acc + char.charCodeAt(0), 0);
  return CHART_COLORS[hash % CHART_COLORS.length];
};

// ---------------------------------------------------------------------------
// Main Dashboard Page
// ---------------------------------------------------------------------------

export default function DashboardPage() {
  const theme = useTheme();
  const navigate = useNavigate();
  const dispatch = useAppDispatch();

  const experiments = useAppSelector(selectRecentExperiments(5));
  const _listLoading = useAppSelector(selectExperimentListLoading);
  const realStats = useAppSelector(selectExperimentStats) ?? {};

  // Dashboard API state
  const [summary, setSummary] = useState<DashboardSummary | null>(
    _PROTOTYPE_DASHBOARD_SUMMARY,
  );
  const [securityPostureHistory, setSecurityPostureHistory] = useState(
    _PROTOTYPE_SECURITY_POSTURE_HISTORY,
  );
  const [activityTimeline, setActivityTimeline] = useState<ActivityTimelinePoint[]>(
    _PROTOTYPE_DASHBOARD_SUMMARY.experimentTrend,
  );
  const [clusterHealthData, setClusterHealthData] = useState<ClusterHealth[]>(
    _PROTOTYPE_DASHBOARD_SUMMARY.clusterHealth,
  );
  const [threatCoverageByCategory, setThreatCoverageByCategory] = useState<
    ThreatCoverageCategory[]
  >(_PROTOTYPE_DASHBOARD_SUMMARY.threatCoverageByCategory);
  const [_error, setError] = useState<string | null>(null);
  const [lastRefreshed, setLastRefreshed] = useState<Date>(new Date());
  const [refreshing, setRefreshing] = useState(false);

  // Load the current experiment list if the store is empty so the dashboard
  // always reflects real experiment data.
  useEffect(() => {
    if (experiments.length === 0 && !_listLoading) {
      dispatch(fetchExperiments({ limit: 10 }));
    }
  }, [dispatch, experiments.length, _listLoading]);

  // Fetch dashboard data from API
  const fetchDashboardData = useCallback(async () => {
    setError(null);
    try {
      const [summaryRes, postureRes, timelineRes, clusterRes] = await Promise.allSettled([
        dashboardAPI.getSummary(),
        dashboardAPI.getSecurityPosture(),
        dashboardAPI.getActivityTimeline(),
        dashboardAPI.getClusterHealth(),
      ]);

      if (summaryRes.status === 'fulfilled') {
        const res = summaryRes.value;
        if (res.data?.success && res.data.data) {
          const normalized = normalizeDashboardSummary(res.data.data);
          const merged = mergeDashboardSummaryWithDemo(normalized);
          setSummary(merged);
          setThreatCoverageByCategory(merged.threatCoverageByCategory);
        }
      }

      if (postureRes.status === 'fulfilled') {
        const res = postureRes.value;
        if (res.data?.success && res.data.data?.history?.length) {
          const hasRealScores = res.data.data.history.some((point) => point.score > 0);
          setSecurityPostureHistory(
            hasRealScores ? res.data.data.history : _PROTOTYPE_SECURITY_POSTURE_HISTORY,
          );
        }
      }

      if (timelineRes.status === 'fulfilled') {
        const res = timelineRes.value;
        if (res.data?.success && res.data.data?.length) {
          const hasRealData = res.data.data.some(
            (point: { total: number }) => point.total > 0,
          );
          setActivityTimeline(
            hasRealData ? res.data.data : _PROTOTYPE_DASHBOARD_SUMMARY.experimentTrend,
          );
        }
      }

      if (clusterRes.status === 'fulfilled') {
        const res = clusterRes.value;
        if (res.data?.success && res.data.data?.length) {
          const hasRealData = res.data.data.some(
            (c: { podCount: number; cpuUsage: number }) =>
              c.podCount > 0 || c.cpuUsage > 0,
          );
          setClusterHealthData(
            hasRealData ? res.data.data : _PROTOTYPE_DASHBOARD_SUMMARY.clusterHealth,
          );
        }
      }

      setLastRefreshed(new Date());
    } catch {
      setError('Failed to load dashboard data. Please try again.');
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchDashboardData();
  }, [fetchDashboardData]);

  // Refresh handler
  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    dispatch(fetchExperiments({ limit: 10 }));
    await fetchDashboardData();
  }, [dispatch, fetchDashboardData]);

  // Derive display values from API data, with Redux store fallback for experiments
  const displayExperiments = experiments;
  const experimentSummary = summary?.experimentSummary ?? {
    total: realStats.total ?? 0,
    running: realStats.running ?? 0,
    completed: realStats.completed ?? 0,
    failed: realStats.failed ?? 0,
    pending: realStats.pending ?? 0,
  };

  return (
    <Box>
      {/* ----------------------------------------------------------------- */}
      {/* Page Header                                                       */}
      {/* ----------------------------------------------------------------- */}
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems={{ xs: 'flex-start', sm: 'center' }}
        spacing={2}
        mb={3}
        flexWrap="wrap"
      >
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 800, mb: 0.5 }}>
            Dashboard
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Security control validation overview{' '}
            <Typography
              component="span"
              variant="caption"
              color="text.disabled"
              sx={{ ml: 1 }}
            >
              Last refreshed: {lastRefreshed.toLocaleTimeString()}
            </Typography>
          </Typography>
        </Box>

        <Stack direction="row" spacing={1}>
          <Tooltip title="Refresh dashboard data">
            <IconButton
              onClick={handleRefresh}
              disabled={refreshing}
              sx={{
                animation: refreshing ? 'spin 1s linear infinite' : 'none',
                '@keyframes spin': {
                  from: { transform: 'rotate(0deg)' },
                  to: { transform: 'rotate(360deg)' },
                },
              }}
            >
              <RefreshIcon />
            </IconButton>
          </Tooltip>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => navigate('/experiments/new')}
            sx={{ textTransform: 'none', fontWeight: 600 }}
          >
            New Experiment
          </Button>
          <Button
            variant="outlined"
            startIcon={<ReportIcon />}
            onClick={() => navigate('/reports')}
            sx={{ textTransform: 'none', fontWeight: 600 }}
          >
            Reports
          </Button>
        </Stack>
      </Stack>

      {/* ----------------------------------------------------------------- */}
      {/* KPI Summary Cards                                                 */}
      {/* ----------------------------------------------------------------- */}
      <Grid container spacing={2.5} mb={3}>
        {/* Security Posture Score */}
        <Grid item xs={12} lg={5}>
          <SecurityPostureCard
            score={summary?.securityPostureScore ?? 0}
            trend={
              summary?.postureTrend ?? {
                direction: 'stable' as const,
                percentage: 0,
                period: '',
              }
            }
            history={securityPostureHistory}
            loading={false}
          />
        </Grid>

        {/* Experiment KPI Cards */}
        <Grid item xs={12} lg={7}>
          <Grid container spacing={2.5}>
            <Grid item xs={6} sm={3}>
              <KPICard
                title="Total Experiments"
                value={experimentSummary.total}
                icon={<ExperimentIcon />}
                color="primary"
                loading={false}
              />
            </Grid>
            <Grid item xs={6} sm={3}>
              <KPICard
                title="Running"
                value={experimentSummary.running}
                icon={<RunningIcon />}
                color="info"
                loading={false}
              />
            </Grid>
            <Grid item xs={6} sm={3}>
              <KPICard
                title="Completed"
                value={experimentSummary.completed}
                icon={<CompletedIcon />}
                color="success"
                loading={false}
              />
            </Grid>
            <Grid item xs={6} sm={3}>
              <KPICard
                title="Failed"
                value={experimentSummary.failed}
                icon={<FailedIcon />}
                color="error"
                loading={false}
              />
            </Grid>
          </Grid>

          {/* Threat Coverage Card */}
          <Card sx={{ mt: 2.5 }}>
            <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="center"
                mb={2}
              >
                <Stack direction="row" spacing={1} alignItems="center">
                  <SecurityIcon sx={{ color: 'primary.main', fontSize: 20 }} />
                  <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
                    Threat Coverage
                  </Typography>
                </Stack>
                <Typography variant="h5" sx={{ fontWeight: 800, color: 'primary.main' }}>
                  {summary?.threatCoverage.coverage.toFixed(0) ?? '0'}%
                </Typography>
              </Stack>

              <Box sx={{ height: 200, width: '100%' }}>
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart
                    data={threatCoverageByCategory}
                    margin={{ top: 0, right: 0, left: -10, bottom: 0 }}
                    barGap={2}
                  >
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke={theme.palette.divider}
                      vertical={false}
                    />
                    <XAxis
                      dataKey="name"
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 11, fill: theme.palette.text.secondary }}
                    />
                    <YAxis
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 11, fill: theme.palette.text.secondary }}
                    />
                    <RechartsTooltip
                      contentStyle={{
                        backgroundColor: theme.palette.background.paper,
                        border: `1px solid ${theme.palette.divider}`,
                        borderRadius: 8,
                        fontSize: 12,
                        fontFamily: "'Inter', sans-serif",
                      }}
                    />
                    <Legend
                      wrapperStyle={{ fontSize: 11, fontFamily: "'Inter', sans-serif" }}
                    />
                    <Bar
                      dataKey="validated"
                      name="Validated"
                      fill={theme.palette.primary.main}
                      radius={[4, 4, 0, 0]}
                      barSize={20}
                    />
                    <Bar
                      dataKey="untested"
                      name="Untested"
                      fill={theme.palette.divider}
                      radius={[4, 4, 0, 0]}
                      barSize={20}
                    />
                  </BarChart>
                </ResponsiveContainer>
              </Box>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* ----------------------------------------------------------------- */}
      {/* Experiment Trend & Top Attack Types                               */}
      {/* ----------------------------------------------------------------- */}
      <Grid container spacing={2.5} mb={3}>
        {/* Experiment Trend Chart */}
        <Grid item xs={12} md={8}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="center"
                mb={2}
              >
                <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
                  Experiment Activity
                </Typography>
                <Chip
                  label="Last 8 weeks"
                  size="small"
                  variant="outlined"
                  sx={{ fontSize: '0.6875rem', height: 24 }}
                />
              </Stack>

              <Box sx={{ height: 260, width: '100%' }}>
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart
                    data={activityTimeline}
                    margin={{ top: 5, right: 5, left: -15, bottom: 5 }}
                  >
                    <defs>
                      <linearGradient id="passedGradient" x1="0" y1="0" x2="0" y2="1">
                        <stop
                          offset="5%"
                          stopColor={theme.palette.success.main}
                          stopOpacity={0.15}
                        />
                        <stop
                          offset="95%"
                          stopColor={theme.palette.success.main}
                          stopOpacity={0}
                        />
                      </linearGradient>
                      <linearGradient id="failedGradient" x1="0" y1="0" x2="0" y2="1">
                        <stop
                          offset="5%"
                          stopColor={theme.palette.error.main}
                          stopOpacity={0.15}
                        />
                        <stop
                          offset="95%"
                          stopColor={theme.palette.error.main}
                          stopOpacity={0}
                        />
                      </linearGradient>
                    </defs>
                    <CartesianGrid
                      strokeDasharray="3 3"
                      stroke={theme.palette.divider}
                      vertical={false}
                    />
                    <XAxis
                      dataKey="date"
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 11, fill: theme.palette.text.secondary }}
                    />
                    <YAxis
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 11, fill: theme.palette.text.secondary }}
                    />
                    <RechartsTooltip
                      contentStyle={{
                        backgroundColor: theme.palette.background.paper,
                        border: `1px solid ${theme.palette.divider}`,
                        borderRadius: 8,
                        fontSize: 12,
                        fontFamily: "'Inter', sans-serif",
                      }}
                    />
                    <Legend
                      wrapperStyle={{ fontSize: 11, fontFamily: "'Inter', sans-serif" }}
                    />
                    <Area
                      type="monotone"
                      dataKey="passed"
                      name="Passed"
                      stroke={theme.palette.success.main}
                      strokeWidth={2}
                      fill="url(#passedGradient)"
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    <Area
                      type="monotone"
                      dataKey="failed"
                      name="Failed"
                      stroke={theme.palette.error.main}
                      strokeWidth={2}
                      fill="url(#failedGradient)"
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </Box>
            </CardContent>
          </Card>
        </Grid>

        {/* Top Attack Types */}
        <Grid item xs={12} md={4}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
              <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem', mb: 2 }}>
                Top Attack Types
              </Typography>

              <Box sx={{ height: 180, width: '100%', mb: 2 }}>
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={summary?.topAttackTypes ?? []}
                      cx="50%"
                      cy="50%"
                      innerRadius={50}
                      outerRadius={80}
                      paddingAngle={3}
                      dataKey="value"
                      stroke="none"
                    >
                      {(summary?.topAttackTypes ?? []).map((entry, _index) => (
                        <Cell
                          key={entry.name}
                          fill={entry.color ?? getChartColor(entry.name)}
                        />
                      ))}
                    </Pie>
                    <RechartsTooltip
                      contentStyle={{
                        backgroundColor: theme.palette.background.paper,
                        border: `1px solid ${theme.palette.divider}`,
                        borderRadius: 8,
                        fontSize: 12,
                        fontFamily: "'Inter', sans-serif",
                      }}
                      formatter={(value: number, name: string) => [`${value}`, name]}
                    />
                  </PieChart>
                </ResponsiveContainer>
              </Box>

              <Stack spacing={1}>
                {(summary?.topAttackTypes ?? []).map((item, idx) => (
                  <Stack
                    key={item.name}
                    direction="row"
                    justifyContent="space-between"
                    alignItems="center"
                  >
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Box
                        sx={{
                          width: 10,
                          height: 10,
                          borderRadius: '50%',
                          backgroundColor:
                            item.color ?? CHART_COLORS[idx % CHART_COLORS.length],
                          flexShrink: 0,
                        }}
                      />
                      <Typography variant="body2" noWrap sx={{ fontSize: '0.8125rem' }}>
                        {item.name}
                      </Typography>
                    </Stack>
                    <Typography
                      variant="body2"
                      fontWeight={600}
                      sx={{ fontSize: '0.8125rem', color: 'text.secondary' }}
                    >
                      {item.value}
                    </Typography>
                  </Stack>
                ))}
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* ----------------------------------------------------------------- */}
      {/* Recent Experiments & Cluster Health                                */}
      {/* ----------------------------------------------------------------- */}
      <Grid container spacing={2.5} mb={3}>
        {/* Recent Experiments Table */}
        <Grid item xs={12} lg={8}>
          <RecentExperimentsTable experiments={displayExperiments} loading={false} />
        </Grid>

        {/* Cluster Health */}
        <Grid item xs={12} lg={4}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="center"
                mb={2}
              >
                <Stack direction="row" spacing={1} alignItems="center">
                  <ClusterIcon sx={{ color: 'primary.main', fontSize: 20 }} />
                  <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
                    Cluster Health
                  </Typography>
                </Stack>
                <Button
                  size="small"
                  endIcon={<ArrowForwardIcon />}
                  onClick={() => navigate('/clusters')}
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                >
                  All
                </Button>
              </Stack>

              <Stack spacing={1.5}>
                {clusterHealthData.length > 0 ? (
                  clusterHealthData.map((cluster) => (
                    <ClusterHealthCard key={cluster.clusterId} cluster={cluster} />
                  ))
                ) : (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ textAlign: 'center', py: 2 }}
                  >
                    No clusters found.
                  </Typography>
                )}
              </Stack>

              {/* Overall health summary */}
              <Divider sx={{ my: 2 }} />
              <Stack direction="row" justifyContent="space-around">
                <Stack alignItems="center" spacing={0.5}>
                  <Typography
                    variant="h5"
                    sx={{ fontWeight: 800, color: 'success.main' }}
                  >
                    {clusterHealthData.filter((c) => c.status === 'healthy').length}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ fontSize: '0.6875rem' }}
                  >
                    Healthy
                  </Typography>
                </Stack>
                <Stack alignItems="center" spacing={0.5}>
                  <Typography
                    variant="h5"
                    sx={{ fontWeight: 800, color: 'warning.main' }}
                  >
                    {clusterHealthData.filter((c) => c.status === 'degraded').length}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ fontSize: '0.6875rem' }}
                  >
                    Degraded
                  </Typography>
                </Stack>
                <Stack alignItems="center" spacing={0.5}>
                  <Typography variant="h5" sx={{ fontWeight: 800, color: 'error.main' }}>
                    {clusterHealthData.filter((c) => c.status === 'unreachable').length}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ fontSize: '0.6875rem' }}
                  >
                    Unreachable
                  </Typography>
                </Stack>
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* ----------------------------------------------------------------- */}
      {/* Validation Success Rate & Quick Actions                           */}
      {/* ----------------------------------------------------------------- */}
      <Grid container spacing={2.5}>
        {/* Validation Success Rate */}
        <Grid item xs={12} md={8}>
          <Card>
            <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="center"
                mb={2}
              >
                <Stack direction="row" spacing={1} alignItems="center">
                  <WarningIcon sx={{ color: 'warning.main', fontSize: 20 }} />
                  <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
                    Validation Summary
                  </Typography>
                </Stack>
              </Stack>

              <Grid container spacing={2.5}>
                <Grid item xs={6} sm={3}>
                  <Box sx={{ textAlign: 'center', p: 2 }}>
                    <Typography
                      variant="h4"
                      sx={{ fontWeight: 800, color: 'text.primary', mb: 0.5 }}
                    >
                      {summary?.threatCoverage.totalControls ?? 0}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontSize: '0.6875rem' }}
                    >
                      Total Controls
                    </Typography>
                  </Box>
                </Grid>
                <Grid item xs={6} sm={3}>
                  <Box sx={{ textAlign: 'center', p: 2 }}>
                    <Typography
                      variant="h4"
                      sx={{ fontWeight: 800, color: 'success.main', mb: 0.5 }}
                    >
                      {summary?.threatCoverage.passed ?? 0}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontSize: '0.6875rem' }}
                    >
                      Passed
                    </Typography>
                  </Box>
                </Grid>
                <Grid item xs={6} sm={3}>
                  <Box sx={{ textAlign: 'center', p: 2 }}>
                    <Typography
                      variant="h4"
                      sx={{ fontWeight: 800, color: 'error.main', mb: 0.5 }}
                    >
                      {summary?.threatCoverage.failed ?? 0}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontSize: '0.6875rem' }}
                    >
                      Failed
                    </Typography>
                  </Box>
                </Grid>
                <Grid item xs={6} sm={3}>
                  <Box sx={{ textAlign: 'center', p: 2 }}>
                    <Typography
                      variant="h4"
                      sx={{ fontWeight: 800, color: 'text.disabled', mb: 0.5 }}
                    >
                      {summary?.threatCoverage.untested ?? 0}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontSize: '0.6875rem' }}
                    >
                      Untested
                    </Typography>
                  </Box>
                </Grid>
              </Grid>

              {/* Overall coverage bar */}
              <Box mt={2}>
                <Stack direction="row" justifyContent="space-between" mb={0.5}>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ fontSize: '0.6875rem' }}
                  >
                    Overall Coverage
                  </Typography>
                  <Typography
                    variant="caption"
                    fontWeight={600}
                    sx={{ fontSize: '0.6875rem' }}
                  >
                    {summary?.threatCoverage.coverage.toFixed(1) ?? '0.0'}%
                  </Typography>
                </Stack>
                <LinearProgress
                  variant="determinate"
                  value={summary?.threatCoverage.coverage ?? 0}
                  sx={{
                    height: 8,
                    borderRadius: 4,
                    backgroundColor: theme.palette.divider,
                    '& .MuiLinearProgress-bar': {
                      borderRadius: 4,
                      background: `linear-gradient(90deg, ${theme.palette.error.main}, ${theme.palette.warning.main}, ${theme.palette.success.main})`,
                    },
                  }}
                />
              </Box>
            </CardContent>
          </Card>
        </Grid>

        {/* Quick Actions */}
        <Grid item xs={12} md={4}>
          <Card sx={{ height: '100%' }}>
            <CardContent
              sx={{
                p: 3,
                '&:last-child': { pb: 3 },
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                height: '100%',
              }}
            >
              <Typography
                variant="h6"
                sx={{ fontWeight: 700, fontSize: '1rem', mb: 2.5 }}
              >
                Quick Actions
              </Typography>

              <Stack spacing={1.5}>
                <Button
                  variant="contained"
                  fullWidth
                  startIcon={<AddIcon />}
                  onClick={() => navigate('/experiments/new')}
                  sx={{
                    py: 1.25,
                    textTransform: 'none',
                    fontWeight: 600,
                    justifyContent: 'flex-start',
                  }}
                >
                  Create New Experiment
                </Button>
                <Button
                  variant="outlined"
                  fullWidth
                  startIcon={<ReportIcon />}
                  onClick={() => navigate('/reports')}
                  sx={{
                    py: 1.25,
                    textTransform: 'none',
                    fontWeight: 600,
                    justifyContent: 'flex-start',
                  }}
                >
                  View Reports
                </Button>
                <Button
                  variant="outlined"
                  fullWidth
                  startIcon={<ClusterIcon />}
                  onClick={() => navigate('/clusters')}
                  sx={{
                    py: 1.25,
                    textTransform: 'none',
                    fontWeight: 600,
                    justifyContent: 'flex-start',
                  }}
                >
                  Manage Clusters
                </Button>
                <Button
                  variant="outlined"
                  fullWidth
                  startIcon={<SecurityIcon />}
                  onClick={() => navigate('/templates')}
                  sx={{
                    py: 1.25,
                    textTransform: 'none',
                    fontWeight: 600,
                    justifyContent: 'flex-start',
                  }}
                >
                  Browse Templates
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
}
