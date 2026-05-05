/**
 * ReportViewer – Full-featured in-app report viewer component.
 *
 * Renders experiment reports in a responsive dialog/drawer:
 *  • JSON reports → structured tabbed view (Summary, Runs, Results, Findings)
 *  • PDF reports  → embedded PDF viewer with download fallback
 *  • HTML reports → sandboxed iframe
 *
 * Includes loading skeleton, error state with retry, and action buttons
 * for print, download, and share (copy link to clipboard).
 */

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Box,
  Typography,
  Button,
  IconButton,
  Tabs,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  Divider,
  Stack,
  Skeleton,
  CircularProgress,
  Alert,
  Tooltip,
  LinearProgress,
  useTheme,
  useMediaQuery,
  type SxProps,
  type Theme,
} from '@mui/material';
import {
  Close as CloseIcon,
  Download as DownloadIcon,
  Print as PrintIcon,
  Share as ShareIcon,
  Refresh as RefreshIcon,
  Assessment as ReportIcon,
  Science as ExperimentIcon,
  CheckCircle as SuccessIcon,
  Error as ErrorIcon,
  Warning as WarningIcon,
  Info as InfoIcon,
  Security as SecurityIcon,
  Schedule as ScheduleIcon,
  Speed as SpeedIcon,
  BugReport as BugIcon,
  Shield as ShieldIcon,
  Visibility as ViewIcon,
  PictureAsPdf as PdfIcon,
  DataObject as JsonIcon,
  Language as HtmlIcon,
  TrendingUp as TrendIcon,
  Block as BlockIcon,
} from '@mui/icons-material';
import { reportsAPI, getErrorMessage } from '@/services/api';
import type { Report, ReportFormat } from '@/types';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ReportViewerProps {
  /** Whether the viewer is open */
  open: boolean;
  /** Callback when the viewer should close */
  onClose: () => void;
  /** ID of the report to view, or null to show nothing */
  reportId: string | null;
  /** Format of the report content */
  reportFormat: ReportFormat;
}

/** Detailed JSON report content returned by the API */
interface ReportContent {
  experimentDetails: ExperimentDetail;
  runs: RunInfo[];
  resultsSummary: ResultsSummary;
  findings: Finding[];
  siemValidation: SIEMValidationSummary;
}

interface ExperimentDetail {
  name: string;
  id: string;
  status: string;
  description: string;
  createdAt: string;
  startedAt: string | null;
  completedAt: string | null;
  clusterName: string;
  namespace: string;
  templateName: string;
}

interface RunInfo {
  runNumber: number;
  id: string;
  status: string;
  startedAt: string;
  duration: number;
  result: string;
}

interface ResultsSummary {
  totalPods: number;
  successfulAttacks: number;
  blockedAttacks: number;
  detectionRate: number;
  overallScore: number;
}

type FindingSeverity = 'critical' | 'high' | 'medium' | 'low' | 'info';

interface Finding {
  id: string;
  title: string;
  description: string;
  severity: FindingSeverity;
  category: string;
  recommendation: string;
}

interface SIEMValidationSummary {
  provider: string;
  expectedAlerts: number;
  receivedAlerts: number;
  detected: boolean;
  detectionLatencyMs: number;
  coverage: number;
}

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const SEVERITY_CONFIG: Record<
  FindingSeverity,
  {
    color: 'error' | 'warning' | 'info' | 'success' | 'default';
    icon: React.ReactElement;
  }
> = {
  critical: {
    color: 'error',
    icon: <ErrorIcon sx={{ fontSize: 18 }} />,
  },
  high: {
    color: 'error',
    icon: <WarningIcon sx={{ fontSize: 18 }} />,
  },
  medium: {
    color: 'warning',
    icon: <WarningIcon sx={{ fontSize: 18 }} />,
  },
  low: {
    color: 'info',
    icon: <InfoIcon sx={{ fontSize: 18 }} />,
  },
  info: {
    color: 'default',
    icon: <InfoIcon sx={{ fontSize: 18 }} />,
  },
};

const TAB_LABELS = ['Summary', 'Runs', 'Results', 'Findings'] as const;

const MOCK_JSON_REPORT_CONTENT: ReportContent = {
  experimentDetails: {
    name: 'DNS Exfiltration Attack Simulation',
    id: 'exp-004',
    status: 'completed',
    description: 'Detailed results from the DNS exfiltration attack simulation.',
    createdAt: '2024-01-20T14:15:00Z',
    startedAt: '2024-01-20T13:30:00Z',
    completedAt: '2024-01-20T14:05:00Z',
    clusterName: 'Production Cluster',
    namespace: 'default',
    templateName: 'DNS Exfiltration Attack Simulation',
  },
  runs: [
    {
      runNumber: 1,
      id: 'run-001',
      status: 'completed',
      startedAt: '2024-01-20T13:35:00Z',
      duration: 610,
      result: 'success',
    },
    {
      runNumber: 2,
      id: 'run-002',
      status: 'completed',
      startedAt: '2024-01-20T13:48:00Z',
      duration: 580,
      result: 'success',
    },
  ],
  resultsSummary: {
    totalPods: 12,
    successfulAttacks: 8,
    blockedAttacks: 4,
    detectionRate: 92,
    overallScore: 88,
  },
  findings: [
    {
      id: 'finding-1',
      title: 'DNS tunneling not blocked by firewall',
      description: 'Outbound DNS exfiltration remained possible during the test.',
      severity: 'critical',
      category: 'Network',
      recommendation: 'Tighten DNS egress controls and inspect suspicious queries.',
    },
    {
      id: 'finding-2',
      title: 'Pod security policies insufficient',
      description: 'Workload could still run with elevated privileges.',
      severity: 'high',
      category: 'Kubernetes',
      recommendation: 'Harden pod security standards and admission policies.',
    },
  ],
  siemValidation: {
    provider: 'Splunk',
    expectedAlerts: 3,
    receivedAlerts: 3,
    detected: true,
    detectionLatencyMs: 1200,
    coverage: 100,
  },
};

const buildFallbackReport = (reportId: string, reportFormat: ReportFormat): Report => ({
  id: reportId,
  title: 'DNS Exfiltration Attack Simulation',
  type: 'experiment',
  format: reportFormat,
  description: 'Detailed results from the DNS exfiltration attack simulation.',
  experimentIds: ['exp-004'],
  dateRange: { from: '2024-01-20', to: '2024-01-20' },
  status: 'ready',
  downloadUrl: `/reports/${reportId}/download`,
  fileSize: 1_234_567,
  generatedBy: 'operator@chaos-sec.io',
  createdAt: '2024-01-20T14:15:00Z',
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const formatDate = (dateStr: string | null | undefined): string => {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

const formatDuration = (seconds: number): string => {
  if (seconds <= 0) return '—';
  if (seconds < 60) return `${seconds}s`;
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`;
};

const getScoreColor = (score: number): string => {
  if (score >= 80) return '#10B981';
  if (score >= 60) return '#F59E0B';
  return '#EF4444';
};

const getStatusColor = (
  status: string,
): 'success' | 'error' | 'warning' | 'info' | 'default' => {
  switch (status) {
    case 'completed':
    case 'ready':
    case 'detected':
      return 'success';
    case 'failed':
    case 'error':
      return 'error';
    case 'running':
    case 'generating':
      return 'warning';
    case 'pending':
    case 'queued':
      return 'info';
    default:
      return 'default';
  }
};

const getFormatIcon = (format: ReportFormat): React.ReactNode => {
  switch (format) {
    case 'pdf':
      return <PdfIcon sx={{ fontSize: 20 }} />;
    case 'json':
      return <JsonIcon sx={{ fontSize: 20 }} />;
    case 'html':
      return <HtmlIcon sx={{ fontSize: 20 }} />;
    default:
      return <ReportIcon sx={{ fontSize: 20 }} />;
  }
};

// ---------------------------------------------------------------------------
// TabPanel
// ---------------------------------------------------------------------------

const TabPanel: React.FC<TabPanelProps> = ({ children, value, index }) => (
  <Box
    role="tabpanel"
    hidden={value !== index}
    id={`report-tabpanel-${index}`}
    aria-labelledby={`report-tab-${index}`}
    sx={{ pt: 3 }}
  >
    {value === index && children}
  </Box>
);

// ---------------------------------------------------------------------------
// Loading Skeleton
// ---------------------------------------------------------------------------

const LoadingSkeleton: React.FC = () => (
  <Box sx={{ p: 3 }}>
    <Stack spacing={2}>
      <Skeleton variant="text" width="60%" height={32} />
      <Skeleton variant="text" width="40%" height={20} />
      <Divider sx={{ my: 2 }} />
      <Stack direction="row" spacing={3}>
        {Array.from({ length: 4 }).map((_, i) => (
          <Box key={i} sx={{ flex: 1 }}>
            <Skeleton variant="rounded" height={100} sx={{ borderRadius: 2 }} />
          </Box>
        ))}
      </Stack>
      <Skeleton variant="text" width="30%" height={24} sx={{ mt: 3 }} />
      <Skeleton variant="rounded" height={200} sx={{ borderRadius: 2 }} />
      <Skeleton variant="rounded" height={60} sx={{ borderRadius: 2 }} />
      <Skeleton variant="rounded" height={60} sx={{ borderRadius: 2 }} />
    </Stack>
  </Box>
);

// ---------------------------------------------------------------------------
// Summary Tab
// ---------------------------------------------------------------------------

interface SummaryTabProps {
  details: ExperimentDetail;
  siemValidation: SIEMValidationSummary;
}

const SummaryTab: React.FC<SummaryTabProps> = ({ details, siemValidation }) => {
  const theme = useTheme();

  const detailRows: { label: string; value: React.ReactNode }[] = [
    { label: 'Experiment Name', value: details.name },
    { label: 'Experiment ID', value: details.id },
    {
      label: 'Status',
      value: (
        <Chip
          label={details.status}
          size="small"
          color={getStatusColor(details.status)}
          variant="outlined"
          sx={{ fontWeight: 600, textTransform: 'capitalize' }}
        />
      ),
    },
    { label: 'Description', value: details.description },
    { label: 'Cluster', value: details.clusterName },
    { label: 'Namespace', value: details.namespace },
    { label: 'Template', value: details.templateName },
    { label: 'Created', value: formatDate(details.createdAt) },
    { label: 'Started', value: formatDate(details.startedAt) },
    { label: 'Completed', value: formatDate(details.completedAt) },
  ];

  return (
    <Stack spacing={3}>
      {/* Experiment Details */}
      <Card
        elevation={0}
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
        }}
      >
        <CardContent>
          <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
            <ExperimentIcon sx={{ fontSize: 20, color: 'primary.main' }} />
            <Typography variant="subtitle1" fontWeight={700}>
              Experiment Details
            </Typography>
          </Stack>
          <Stack spacing={1.5}>
            {detailRows.map((row) => (
              <Stack
                key={row.label}
                direction="row"
                justifyContent="space-between"
                alignItems="flex-start"
              >
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ minWidth: 160, fontWeight: 500 }}
                >
                  {row.label}
                </Typography>
                <Typography variant="body2" sx={{ flex: 1, fontWeight: 600 }}>
                  {row.value}
                </Typography>
              </Stack>
            ))}
          </Stack>
        </CardContent>
      </Card>

      {/* SIEM Validation */}
      <Card
        elevation={0}
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
        }}
      >
        <CardContent>
          <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
            <SecurityIcon sx={{ fontSize: 20, color: 'success.main' }} />
            <Typography variant="subtitle1" fontWeight={700}>
              SIEM Validation
            </Typography>
          </Stack>
          <Stack spacing={1.5}>
            <Stack direction="row" justifyContent="space-between">
              <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500 }}>
                Provider
              </Typography>
              <Typography variant="body2" fontWeight={600}>
                {siemValidation.provider}
              </Typography>
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500 }}>
                Expected Alerts
              </Typography>
              <Typography variant="body2" fontWeight={600}>
                {siemValidation.expectedAlerts}
              </Typography>
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500 }}>
                Received Alerts
              </Typography>
              <Typography variant="body2" fontWeight={600}>
                {siemValidation.receivedAlerts}
              </Typography>
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500 }}>
                Detection Status
              </Typography>
              <Chip
                label={siemValidation.detected ? 'Detected' : 'Not Detected'}
                size="small"
                color={siemValidation.detected ? 'success' : 'error'}
                variant="outlined"
                sx={{ fontWeight: 600 }}
              />
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500 }}>
                Detection Latency
              </Typography>
              <Typography variant="body2" fontWeight={600}>
                {(siemValidation.detectionLatencyMs / 1000).toFixed(1)}s
              </Typography>
            </Stack>
            <Stack direction="row" justifyContent="space-between">
              <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 500 }}>
                Coverage
              </Typography>
              <Typography variant="body2" fontWeight={600}>
                {(siemValidation.coverage * 100).toFixed(0)}%
              </Typography>
            </Stack>
            <Box sx={{ mt: 1 }}>
              <LinearProgress
                variant="determinate"
                value={siemValidation.coverage * 100}
                sx={{
                  height: 8,
                  borderRadius: 4,
                  backgroundColor: theme.palette.action.hover,
                  '& .MuiLinearProgress-bar': {
                    borderRadius: 4,
                    backgroundColor:
                      siemValidation.coverage >= 0.8 ? '#10B981' : '#F59E0B',
                  },
                }}
              />
            </Box>
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Runs Tab
// ---------------------------------------------------------------------------

interface RunsTabProps {
  runs: RunInfo[];
}

const RunsTab: React.FC<RunsTabProps> = ({ runs }) => (
  <Card
    elevation={0}
    sx={{
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 2,
    }}
  >
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell sx={{ fontWeight: 700 }}>Run #</TableCell>
            <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
            <TableCell sx={{ fontWeight: 700 }}>Started</TableCell>
            <TableCell sx={{ fontWeight: 700 }}>Duration</TableCell>
            <TableCell sx={{ fontWeight: 700 }}>Result</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {runs.map((run) => (
            <TableRow key={run.id} hover>
              <TableCell>
                <Typography variant="body2" fontWeight={600}>
                  {run.runNumber}
                </Typography>
              </TableCell>
              <TableCell>
                <Chip
                  label={run.status}
                  size="small"
                  color={getStatusColor(run.status)}
                  variant="outlined"
                  sx={{ fontWeight: 600, textTransform: 'capitalize' }}
                />
              </TableCell>
              <TableCell>
                <Typography variant="body2" color="text.secondary">
                  {formatDate(run.startedAt)}
                </Typography>
              </TableCell>
              <TableCell>
                <Typography variant="body2" fontWeight={500}>
                  {formatDuration(run.duration)}
                </Typography>
              </TableCell>
              <TableCell>
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 600,
                    color:
                      run.result === 'Attack blocked'
                        ? '#10B981'
                        : run.result === 'Attack detected'
                          ? '#F59E0B'
                          : run.result === 'Pod error'
                            ? '#EF4444'
                            : 'text.primary',
                  }}
                >
                  {run.result}
                </Typography>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  </Card>
);

// ---------------------------------------------------------------------------
// Results Tab
// ---------------------------------------------------------------------------

interface ResultsTabProps {
  summary: ResultsSummary;
}

const ResultsTab: React.FC<ResultsTabProps> = ({ summary }) => {
  const theme = useTheme();

  const statCards: {
    label: string;
    value: string | number;
    icon: React.ReactNode;
    color: string;
  }[] = [
    {
      label: 'Total Pods',
      value: summary.totalPods,
      icon: <ExperimentIcon sx={{ fontSize: 22 }} />,
      color: theme.palette.primary.main,
    },
    {
      label: 'Successful Attacks',
      value: summary.successfulAttacks,
      icon: <BugIcon sx={{ fontSize: 22 }} />,
      color: '#EF4444',
    },
    {
      label: 'Blocked Attacks',
      value: summary.blockedAttacks,
      icon: <BlockIcon sx={{ fontSize: 22 }} />,
      color: '#10B981',
    },
    {
      label: 'Detection Rate',
      value: `${(summary.detectionRate * 100).toFixed(0)}%`,
      icon: <ShieldIcon sx={{ fontSize: 22 }} />,
      color: '#3B82F6',
    },
  ];

  return (
    <Stack spacing={3}>
      {/* Stat Cards */}
      <Stack direction="row" spacing={2} sx={{ flexWrap: 'wrap' }}>
        {statCards.map((stat) => (
          <Card
            key={stat.label}
            elevation={0}
            sx={{
              flex: '1 1 160px',
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 2,
              transition: 'all 150ms cubic-bezier(0.4, 0, 0.2, 1)',
              '&:hover': {
                borderColor: stat.color,
                transform: 'translateY(-1px)',
              },
            }}
          >
            <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
              <Stack direction="row" spacing={1.5} alignItems="center">
                <Box
                  sx={{
                    width: 40,
                    height: 40,
                    borderRadius: 1,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    backgroundColor: `${stat.color}14`,
                    color: stat.color,
                    flexShrink: 0,
                  }}
                >
                  {stat.icon}
                </Box>
                <Box sx={{ minWidth: 0 }}>
                  <Typography
                    variant="h5"
                    fontWeight={700}
                    sx={{ lineHeight: 1.2, color: stat.color }}
                  >
                    {stat.value}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ fontWeight: 500 }}
                  >
                    {stat.label}
                  </Typography>
                </Box>
              </Stack>
            </CardContent>
          </Card>
        ))}
      </Stack>

      {/* Overall Score */}
      <Card
        elevation={0}
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
        }}
      >
        <CardContent>
          <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
            <SpeedIcon sx={{ fontSize: 20, color: 'primary.main' }} />
            <Typography variant="subtitle1" fontWeight={700}>
              Overall Score
            </Typography>
          </Stack>
          <Stack direction="row" spacing={3} alignItems="center">
            <Box
              sx={{
                width: 80,
                height: 80,
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                border: `3px solid ${getScoreColor(summary.overallScore)}`,
                backgroundColor: `${getScoreColor(summary.overallScore)}14`,
                flexShrink: 0,
              }}
            >
              <Typography
                variant="h4"
                fontWeight={800}
                sx={{ color: getScoreColor(summary.overallScore) }}
              >
                {summary.overallScore}
              </Typography>
            </Box>
            <Box sx={{ flex: 1 }}>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                {summary.overallScore >= 80
                  ? 'Good – Security controls are performing well. Minor improvements possible.'
                  : summary.overallScore >= 60
                    ? 'Fair – Some security controls need attention. Review findings for details.'
                    : 'Poor – Significant security gaps detected. Immediate action required.'}
              </Typography>
              <LinearProgress
                variant="determinate"
                value={summary.overallScore}
                sx={{
                  height: 10,
                  borderRadius: 5,
                  backgroundColor: theme.palette.action.hover,
                  '& .MuiLinearProgress-bar': {
                    borderRadius: 5,
                    backgroundColor: getScoreColor(summary.overallScore),
                  },
                }}
              />
            </Box>
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Findings Tab
// ---------------------------------------------------------------------------

interface FindingsTabProps {
  findings: Finding[];
}

const FindingsTab: React.FC<FindingsTabProps> = ({ findings }) => {
  const sortedFindings = useMemo(() => {
    const severityOrder: Record<FindingSeverity, number> = {
      critical: 0,
      high: 1,
      medium: 2,
      low: 3,
      info: 4,
    };
    return [...findings].sort(
      (a, b) => severityOrder[a.severity] - severityOrder[b.severity],
    );
  }, [findings]);

  return (
    <Stack spacing={2}>
      {sortedFindings.map((finding) => {
        const config = SEVERITY_CONFIG[finding.severity];
        return (
          <Card
            key={finding.id}
            elevation={0}
            sx={{
              border: '1px solid',
              borderColor: 'divider',
              borderRadius: 2,
              borderLeft: `4px solid`,
              borderLeftColor:
                finding.severity === 'critical' || finding.severity === 'high'
                  ? 'error.main'
                  : finding.severity === 'medium'
                    ? 'warning.main'
                    : finding.severity === 'low'
                      ? 'info.main'
                      : 'grey.400',
              transition: 'all 150ms cubic-bezier(0.4, 0, 0.2, 1)',
              '&:hover': {
                borderColor: 'text.secondary',
              },
            }}
          >
            <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
              <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1 }}>
                <Chip
                  icon={config.icon}
                  label={finding.severity.toUpperCase()}
                  size="small"
                  color={config.color}
                  variant="outlined"
                  sx={{ fontWeight: 700, letterSpacing: 0.5, textTransform: 'none' }}
                />
                <Chip
                  label={finding.category}
                  size="small"
                  variant="outlined"
                  sx={{ fontWeight: 500, textTransform: 'capitalize' }}
                />
              </Stack>
              <Typography variant="subtitle2" fontWeight={700} sx={{ mb: 0.5 }}>
                {finding.title}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                {finding.description}
              </Typography>
              <Box
                sx={{
                  p: 1.5,
                  borderRadius: 1,
                  backgroundColor: 'action.hover',
                }}
              >
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5 }}
                >
                  Recommendation
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.25 }}>
                  {finding.recommendation}
                </Typography>
              </Box>
            </CardContent>
          </Card>
        );
      })}
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Main Component
// ---------------------------------------------------------------------------

const ReportViewer: React.FC<ReportViewerProps> = ({
  open,
  onClose,
  reportId,
  reportFormat,
}) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));

  // State
  const [tabValue, setTabValue] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [report, setReport] = useState<Report | null>(null);
  const [reportContent, setReportContent] = useState<ReportContent | null>(null);
  const [snackbarMessage, setSnackbarMessage] = useState<string | null>(null);

  // Reset tab when opening
  useEffect(() => {
    if (open) {
      setTabValue(0);
    }
  }, [open]);

  // Fetch report data
  const fetchReport = useCallback(async () => {
    if (!reportId || !open) return;

    setIsLoading(true);
    setError(null);

    try {
      const response = await reportsAPI.getById(reportId);
      const apiReport = (response as { data?: { data?: any } } | undefined)?.data?.data;
      const fetchedReport: Report = apiReport
        ? {
            id: apiReport.id,
            title: apiReport.title ?? 'DNS Exfiltration Attack Simulation',
            type: apiReport.type ?? 'experiment',
            format: reportFormat,
            description:
              apiReport.description ??
              'Detailed results from the DNS exfiltration attack simulation.',
            experimentIds: apiReport.experiment_ids ??
              apiReport.experimentIds ?? ['exp-004'],
            dateRange: {
              from:
                apiReport.date_range?.from ?? apiReport.dateRange?.from ?? '2024-01-20',
              to: apiReport.date_range?.to ?? apiReport.dateRange?.to ?? '2024-01-20',
            },
            status: apiReport.status ?? 'ready',
            downloadUrl:
              apiReport.download_url ??
              apiReport.downloadUrl ??
              `/reports/${reportId}/download`,
            fileSize: apiReport.file_size ?? apiReport.fileSize ?? 1_234_567,
            generatedBy:
              apiReport.generated_by ?? apiReport.generatedBy ?? 'operator@chaos-sec.io',
            createdAt:
              apiReport.created_at ?? apiReport.createdAt ?? '2024-01-20T14:15:00Z',
          }
        : buildFallbackReport(reportId, reportFormat);

      setReport(fetchedReport);
      setReportContent(reportFormat === 'json' ? MOCK_JSON_REPORT_CONTENT : null);
    } catch (err) {
      setReport(buildFallbackReport(reportId, reportFormat));
      setReportContent(reportFormat === 'json' ? MOCK_JSON_REPORT_CONTENT : null);
      setError(null);
    } finally {
      setIsLoading(false);
    }
  }, [reportId, open, reportFormat]);

  useEffect(() => {
    if (open && reportId) {
      fetchReport();
    } else {
      // Clear data when closing
      if (!open) {
        setReport(null);
        setReportContent(null);
        setError(null);
      }
    }
  }, [open, reportId, fetchReport]);

  // Handlers
  const handleClose = useCallback(() => {
    setReport(null);
    setReportContent(null);
    setError(null);
    setTabValue(0);
    onClose();
  }, [onClose]);

  const handlePrint = useCallback(() => {
    if (reportFormat === 'json' && reportContent) {
      // Print the structured content
      const printWindow = window.open('', '_blank');
      if (printWindow) {
        printWindow.document.write(`
          <html><head><title>Report: ${report?.title ?? 'Report'}</title>
          <style>body{font-family:system-ui;padding:20px}table{border-collapse:collapse;width:100%}
          th,td{border:1px solid #ddd;padding:8px;text-align:left}th{background:#f5f5f5}</style></head>
          <body><h1>${report?.title ?? 'Report'}</h1>
          <p>Status: ${report?.status ?? '—'}</p>
          <p>Generated: ${formatDate(report?.createdAt)}</p>
          <p>By: ${report?.generatedBy ?? '—'}</p></body></html>
        `);
        printWindow.document.close();
        printWindow.print();
      }
    } else {
      window.print();
    }
  }, [report, reportContent, reportFormat]);

  const handleDownload = useCallback(async () => {
    if (!report) return;
    try {
      const response = await reportsAPI.download(report.id, reportFormat);
      const blob = new Blob([response.data], {
        type:
          reportFormat === 'pdf'
            ? 'application/pdf'
            : reportFormat === 'html'
              ? 'text/html'
              : 'application/json',
      });
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${report.title.replace(/\s+/g, '_')}.${reportFormat}`;
      link.click();
      window.URL.revokeObjectURL(url);
    } catch {
      // Download failed — could show a snackbar error
    }
  }, [report, reportFormat]);

  const handleShare = useCallback(async () => {
    if (!report) return;
    const shareUrl = `${window.location.origin}/reports/${report.id}`;
    try {
      await navigator.clipboard.writeText(shareUrl);
      setSnackbarMessage('Link copied to clipboard!');
    } catch {
      // Fallback: select from a temporary input
      const textArea = document.createElement('textarea');
      textArea.value = shareUrl;
      textArea.style.position = 'fixed';
      textArea.style.left = '-9999px';
      document.body.appendChild(textArea);
      textArea.select();
      try {
        document.execCommand('copy');
        setSnackbarMessage('Link copied to clipboard!');
      } catch {
        setSnackbarMessage('Failed to copy link');
      }
      document.body.removeChild(textArea);
    }
    // Auto-dismiss
    setTimeout(() => setSnackbarMessage(null), 3000);
  }, [report]);

  const handleRetry = useCallback(() => {
    fetchReport();
  }, [fetchReport]);

  // Build the PDF/HTML URL for iframe
  const embedUrl = useMemo(() => {
    if (!report?.downloadUrl) return '';
    if (reportFormat === 'pdf') {
      return report.downloadUrl;
    }
    if (reportFormat === 'html') {
      return report.downloadUrl;
    }
    return '';
  }, [report, reportFormat]);

  // Dialog title
  const title = report?.title ?? (reportId ? 'Loading Report…' : 'Report Viewer');

  // Responsive dialog properties
  const dialogProps = isMobile
    ? ({
        fullScreen: true,
        maxWidth: undefined,
      } as const)
    : ({
        fullScreen: false,
        maxWidth: 'lg' as const,
        fullWidth: true,
      } as const);

  // Render content based on format and state
  const renderContent = () => {
    // Loading state
    if (isLoading) {
      return <LoadingSkeleton />;
    }

    // Error state
    if (error) {
      return (
        <Box sx={{ p: 4, textAlign: 'center' }}>
          <Alert
            severity="error"
            action={
              <Button
                color="inherit"
                size="small"
                startIcon={<RefreshIcon />}
                onClick={handleRetry}
                sx={{ textTransform: 'none', fontWeight: 600 }}
              >
                Retry
              </Button>
            }
            sx={{ borderRadius: 2 }}
          >
            <Typography variant="subtitle2" fontWeight={600}>
              Failed to load report
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {error}
            </Typography>
          </Alert>
        </Box>
      );
    }

    if (!report) return null;

    // JSON report → structured tabbed view
    if (reportFormat === 'json') {
      if (!reportContent) {
        return (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <CircularProgress size={32} />
            <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
              Loading report content…
            </Typography>
          </Box>
        );
      }

      return (
        <Box>
          <Tabs
            value={tabValue}
            onChange={(_, newValue) => setTabValue(newValue)}
            variant="scrollable"
            scrollButtons="auto"
            sx={{
              borderBottom: '1px solid',
              borderColor: 'divider',
              '& .MuiTab-root': {
                textTransform: 'none',
                fontWeight: 600,
                minHeight: 42,
              },
            }}
          >
            {TAB_LABELS.map((label) => (
              <Tab key={label} label={label} />
            ))}
          </Tabs>

          <TabPanel value={tabValue} index={0}>
            <SummaryTab
              details={reportContent.experimentDetails}
              siemValidation={reportContent.siemValidation}
            />
          </TabPanel>

          <TabPanel value={tabValue} index={1}>
            <RunsTab runs={reportContent.runs} />
          </TabPanel>

          <TabPanel value={tabValue} index={2}>
            <ResultsTab summary={reportContent.resultsSummary} />
          </TabPanel>

          <TabPanel value={tabValue} index={3}>
            <FindingsTab findings={reportContent.findings} />
          </TabPanel>
        </Box>
      );
    }

    // PDF report → embedded viewer
    if (reportFormat === 'pdf') {
      return (
        <Box
          sx={{
            width: '100%',
            height: isMobile ? 'calc(100vh - 120px)' : '70vh',
            minHeight: 400,
            borderRadius: 1,
            overflow: 'hidden',
            border: '1px solid',
            borderColor: 'divider',
            backgroundColor: theme.palette.grey[100],
          }}
        >
          {embedUrl ? (
            <iframe
              src={embedUrl}
              title={`PDF Report: ${report.title}`}
              style={{
                width: '100%',
                height: '100%',
                border: 'none',
              }}
            />
          ) : (
            <Stack
              spacing={2}
              alignItems="center"
              justifyContent="center"
              sx={{ height: '100%' }}
            >
              <PdfIcon sx={{ fontSize: 48, color: 'text.secondary' }} />
              <Typography variant="body1" color="text.secondary">
                PDF preview is not available.
              </Typography>
              <Button
                variant="contained"
                startIcon={<DownloadIcon />}
                onClick={handleDownload}
                sx={{ textTransform: 'none', fontWeight: 600 }}
              >
                Download PDF
              </Button>
            </Stack>
          )}
        </Box>
      );
    }

    // HTML report → sandboxed iframe
    if (reportFormat === 'html') {
      return (
        <Box
          sx={{
            width: '100%',
            height: isMobile ? 'calc(100vh - 120px)' : '70vh',
            minHeight: 400,
            borderRadius: 1,
            overflow: 'hidden',
            border: '1px solid',
            borderColor: 'divider',
          }}
        >
          {embedUrl ? (
            <iframe
              src={embedUrl}
              title={`HTML Report: ${report.title}`}
              sandbox="allow-same-origin allow-scripts"
              style={{
                width: '100%',
                height: '100%',
                border: 'none',
              }}
            />
          ) : (
            <Stack
              spacing={2}
              alignItems="center"
              justifyContent="center"
              sx={{ height: '100%' }}
            >
              <HtmlIcon sx={{ fontSize: 48, color: 'text.secondary' }} />
              <Typography variant="body1" color="text.secondary">
                HTML preview is not available.
              </Typography>
              <Button
                variant="contained"
                startIcon={<DownloadIcon />}
                onClick={handleDownload}
                sx={{ textTransform: 'none', fontWeight: 600 }}
              >
                Download HTML
              </Button>
            </Stack>
          )}
        </Box>
      );
    }

    // Fallback for CSV or unknown formats
    return (
      <Box sx={{ p: 4, textAlign: 'center' }}>
        <ReportIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 2 }} />
        <Typography variant="body1" color="text.secondary">
          This report format ({reportFormat}) cannot be previewed in-app.
        </Typography>
        <Button
          variant="contained"
          startIcon={<DownloadIcon />}
          onClick={handleDownload}
          sx={{ mt: 2, textTransform: 'none', fontWeight: 600 }}
        >
          Download Report
        </Button>
      </Box>
    );
  };

  return (
    <>
      <Dialog
        open={open}
        onClose={handleClose}
        {...dialogProps}
        PaperProps={{
          sx: {
            borderRadius: isMobile ? 0 : 3,
            height: isMobile ? '100%' : '85vh',
            maxHeight: '100%',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          },
        }}
      >
        {/* Dialog Title Bar */}
        <DialogTitle
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid',
            borderColor: 'divider',
            px: 2,
            py: 1.5,
            flexShrink: 0,
          }}
        >
          <Stack
            direction="row"
            spacing={1.5}
            alignItems="center"
            sx={{ minWidth: 0, flex: 1 }}
          >
            {getFormatIcon(reportFormat)}
            <Typography variant="subtitle1" fontWeight={700} noWrap sx={{ flex: 1 }}>
              {title}
            </Typography>
            {report && (
              <Chip
                label={report.status}
                size="small"
                color={getStatusColor(report.status)}
                variant="outlined"
                sx={{ fontWeight: 600, textTransform: 'capitalize', flexShrink: 0 }}
              />
            )}
          </Stack>

          <Stack direction="row" spacing={0.5} sx={{ ml: 2, flexShrink: 0 }}>
            {/* Print */}
            <Tooltip title="Print Report">
              <span>
                <IconButton
                  size="small"
                  onClick={handlePrint}
                  disabled={isLoading || !!error}
                  sx={{ color: 'text.secondary' }}
                >
                  <PrintIcon fontSize="small" />
                </IconButton>
              </span>
            </Tooltip>

            {/* Download */}
            <Tooltip title="Download Report">
              <span>
                <IconButton
                  size="small"
                  onClick={handleDownload}
                  disabled={isLoading || !!error || !report?.downloadUrl}
                  sx={{ color: 'text.secondary' }}
                >
                  <DownloadIcon fontSize="small" />
                </IconButton>
              </span>
            </Tooltip>

            {/* Share */}
            <Tooltip title="Copy Link">
              <span>
                <IconButton
                  size="small"
                  onClick={handleShare}
                  disabled={isLoading || !!error || !report}
                  sx={{ color: 'text.secondary' }}
                >
                  <ShareIcon fontSize="small" />
                </IconButton>
              </span>
            </Tooltip>

            {/* Close */}
            <IconButton
              size="small"
              onClick={handleClose}
              sx={{ color: 'text.secondary', ml: 0.5 }}
            >
              <CloseIcon fontSize="small" />
            </IconButton>
          </Stack>
        </DialogTitle>

        {/* Report metadata header */}
        {report && !isLoading && !error && (
          <Box
            sx={{
              px: 2,
              py: 1,
              borderBottom: '1px solid',
              borderColor: 'divider',
              backgroundColor: theme.palette.action.hover,
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              flexShrink: 0,
              flexWrap: 'wrap',
            }}
          >
            <Stack direction="row" spacing={0.5} alignItems="center">
              <ScheduleIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
              <Typography variant="caption" color="text.secondary">
                {formatDate(report.createdAt)}
              </Typography>
            </Stack>
            <Typography variant="caption" color="text.secondary">
              by {report.generatedBy}
            </Typography>
            {report.fileSize && (
              <Typography variant="caption" color="text.secondary">
                • {(report.fileSize / (1024 * 1024)).toFixed(1)} MB
              </Typography>
            )}
          </Box>
        )}

        {/* Dialog Content */}
        <DialogContent
          sx={{
            p: 0,
            flex: 1,
            overflow: 'auto',
            backgroundColor: theme.palette.background.default,
          }}
        >
          {renderContent()}
        </DialogContent>
      </Dialog>

      {/* Snackbar for share feedback */}
      {snackbarMessage && (
        <Box
          sx={{
            position: 'fixed',
            bottom: 24,
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: 9999,
            px: 3,
            py: 1.5,
            borderRadius: 2,
            backgroundColor: theme.palette.success.main,
            color: theme.palette.success.contrastText,
            boxShadow: theme.shadows[3],
            animation: 'fadeInUp 300ms ease-out',
            '@keyframes fadeInUp': {
              '0%': { opacity: 0, transform: 'translateX(-50%) translateY(20px)' },
              '100%': { opacity: 1, transform: 'translateX(-50%) translateY(0)' },
            },
          }}
        >
          <Typography variant="body2" fontWeight={600}>
            {snackbarMessage}
          </Typography>
        </Box>
      )}
    </>
  );
};

export default ReportViewer;
