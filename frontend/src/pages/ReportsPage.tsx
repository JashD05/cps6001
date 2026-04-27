import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Box,
  Typography,
  Button,
  Card,
  CardContent,
  Grid,
  Stack,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  TableSortLabel,
  Chip,
  IconButton,
  Tooltip,
  TextField,
  InputAdornment,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControlLabel,
  Checkbox,
  Divider,
  Skeleton,
  Alert,
  Breadcrumbs,
  Link,
  Avatar,
  LinearProgress,
  useTheme,
  type SelectChangeEvent,
} from '@mui/material';
import {
  Assessment as ReportIcon,
  Add as GenerateIcon,
  Download as DownloadIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
  Search as SearchIcon,
  FilterList as FilterIcon,
  Clear as ClearIcon,
  Description as FileIcon,
  PictureAsPdf as PdfIcon,
  TableChart as CsvIcon,
  DataObject as JsonIcon,
  Language as HtmlIcon,
  Schedule as ScheduleIcon,
  Share as ShareIcon,
  MoreVert as MoreIcon,
  CheckCircle as ReadyIcon,
  Error as ErrorIcon,
  HourglassEmpty as GeneratingIcon,
  Visibility as ViewIcon,
  DateRange as DateRangeIcon,
  TrendingUp as TrendIcon,
  Security as SecurityIcon,
  Science as ExperimentIcon,
  BarChart as ComplianceIcon,
  Speed as ExecutiveIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import { reportsAPI, getErrorMessage } from '@/services/api';
import { useAppDispatch, useAppSelector } from '@/store';
import StatusBadge from '@/components/StatusBadge';
import type { Report, ReportType, ReportFormat } from '@/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const REPORT_TYPE_OPTIONS: {
  value: ReportType | 'all';
  label: string;
  icon: React.ReactNode;
}[] = [
  { value: 'all', label: 'All Types', icon: <ReportIcon sx={{ fontSize: 18 }} /> },
  {
    value: 'experiment',
    label: 'Experiment',
    icon: <ExperimentIcon sx={{ fontSize: 18 }} />,
  },
  {
    value: 'compliance',
    label: 'Compliance',
    icon: <ComplianceIcon sx={{ fontSize: 18 }} />,
  },
  {
    value: 'executive',
    label: 'Executive',
    icon: <ExecutiveIcon sx={{ fontSize: 18 }} />,
  },
  { value: 'trend', label: 'Trend Analysis', icon: <TrendIcon sx={{ fontSize: 18 }} /> },
];

const FORMAT_OPTIONS: { value: ReportFormat; label: string; icon: React.ReactNode }[] = [
  {
    value: 'pdf',
    label: 'PDF',
    icon: <PdfIcon sx={{ fontSize: 18, color: '#EF4444' }} />,
  },
  {
    value: 'csv',
    label: 'CSV',
    icon: <CsvIcon sx={{ fontSize: 18, color: '#10B981' }} />,
  },
  {
    value: 'json',
    label: 'JSON',
    icon: <JsonIcon sx={{ fontSize: 18, color: '#F59E0B' }} />,
  },
  {
    value: 'html',
    label: 'HTML',
    icon: <HtmlIcon sx={{ fontSize: 18, color: '#2563EB' }} />,
  },
];

const PAGE_SIZE_OPTIONS = [5, 10, 25, 50];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const formatFileSize = (bytes: number | undefined): string => {
  if (!bytes) return '—';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

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
  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
  });
};

const getReportTypeColor = (
  type: ReportType,
): 'primary' | 'secondary' | 'success' | 'warning' | 'info' => {
  switch (type) {
    case 'experiment':
      return 'primary';
    case 'compliance':
      return 'success';
    case 'executive':
      return 'warning';
    case 'trend':
      return 'info';
    default:
      return 'primary';
  }
};

const getFormatIcon = (format: ReportFormat): React.ReactNode => {
  const option = FORMAT_OPTIONS.find((o) => o.value === format);
  return option?.icon ?? <FileIcon sx={{ fontSize: 18 }} />;
};

const getReportStatusBadge = (status: Report['status']): string => {
  switch (status) {
    case 'generating':
      return 'running';
    case 'ready':
      return 'completed';
    case 'error':
      return 'failed';
    default:
      return 'pending';
  }
};

// ---------------------------------------------------------------------------
// Mock Data
// ---------------------------------------------------------------------------

const MOCK_REPORTS: Report[] = [
  {
    id: 'rpt-001',
    title: 'Weekly Security Posture Report',
    type: 'compliance',
    format: 'pdf',
    description:
      'Comprehensive weekly analysis of security posture and control validation results.',
    experimentIds: ['exp-001', 'exp-002', 'exp-003'],
    dateRange: { from: '2024-01-15', to: '2024-01-22' },
    status: 'ready',
    downloadUrl: '/reports/rpt-001/download',
    fileSize: 2_456_789,
    generatedBy: 'admin@chaos-sec.io',
    createdAt: '2024-01-22T10:30:00Z',
  },
  {
    id: 'rpt-002',
    title: 'DNS Exfiltration Test Results',
    type: 'experiment',
    format: 'pdf',
    description: 'Detailed results from the DNS exfiltration attack simulation.',
    experimentIds: ['exp-004'],
    dateRange: { from: '2024-01-20', to: '2024-01-20' },
    status: 'ready',
    downloadUrl: '/reports/rpt-002/download',
    fileSize: 1_234_567,
    generatedBy: 'operator@chaos-sec.io',
    createdAt: '2024-01-20T14:15:00Z',
  },
  {
    id: 'rpt-003',
    title: 'Q4 Executive Summary',
    type: 'executive',
    format: 'pdf',
    description: 'Quarterly executive summary of security validation program.',
    experimentIds: ['exp-001', 'exp-005', 'exp-008'],
    dateRange: { from: '2023-10-01', to: '2023-12-31' },
    status: 'ready',
    downloadUrl: '/reports/rpt-003/download',
    fileSize: 4_567_890,
    generatedBy: 'admin@chaos-sec.io',
    createdAt: '2024-01-05T09:00:00Z',
  },
  {
    id: 'rpt-004',
    title: 'Monthly Trend Analysis - January',
    type: 'trend',
    format: 'html',
    description:
      'Monthly trend analysis showing security posture improvements and regression areas.',
    experimentIds: [],
    dateRange: { from: '2024-01-01', to: '2024-01-31' },
    status: 'generating',
    generatedBy: 'admin@chaos-sec.io',
    createdAt: '2024-01-31T23:59:00Z',
  },
  {
    id: 'rpt-005',
    title: 'Brute Force Attack Validation',
    type: 'experiment',
    format: 'csv',
    description: 'Raw data export from the brute force attack simulation experiments.',
    experimentIds: ['exp-006'],
    dateRange: { from: '2024-01-18', to: '2024-01-18' },
    status: 'ready',
    downloadUrl: '/reports/rpt-005/download',
    fileSize: 567_890,
    generatedBy: 'operator@chaos-sec.io',
    createdAt: '2024-01-18T16:45:00Z',
  },
  {
    id: 'rpt-006',
    title: 'SOC 2 Compliance Report',
    type: 'compliance',
    format: 'pdf',
    description: 'SOC 2 Type II compliance validation report with evidence mapping.',
    experimentIds: ['exp-001', 'exp-002', 'exp-003', 'exp-004', 'exp-005'],
    dateRange: { from: '2023-07-01', to: '2023-12-31' },
    status: 'ready',
    downloadUrl: '/reports/rpt-006/download',
    fileSize: 8_901_234,
    generatedBy: 'admin@chaos-sec.io',
    createdAt: '2024-01-10T11:30:00Z',
  },
  {
    id: 'rpt-007',
    title: 'Failed Exports Batch',
    type: 'experiment',
    format: 'json',
    description: 'Automated export that encountered processing errors.',
    experimentIds: ['exp-007'],
    dateRange: { from: '2024-01-25', to: '2024-01-25' },
    status: 'error',
    generatedBy: 'system@chaos-sec.io',
    createdAt: '2024-01-25T03:00:00Z',
  },
  {
    id: 'rpt-008',
    title: 'Network Segmentation Validation',
    type: 'experiment',
    format: 'pdf',
    description: 'Results from network segmentation and lateral movement control tests.',
    experimentIds: ['exp-009', 'exp-010'],
    dateRange: { from: '2024-01-12', to: '2024-01-14' },
    status: 'ready',
    downloadUrl: '/reports/rpt-008/download',
    fileSize: 3_210_987,
    generatedBy: 'operator@chaos-sec.io',
    createdAt: '2024-01-14T18:20:00Z',
  },
];

// ---------------------------------------------------------------------------
// Sub-Components
// ---------------------------------------------------------------------------

/** Stat card for report type summary */
interface StatCardProps {
  label: string;
  value: number;
  icon: React.ReactNode;
  color: string;
}

const StatCard: React.FC<StatCardProps> = ({ label, value, icon, color }) => (
  <Card
    elevation={0}
    sx={{
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 2,
      height: '100%',
      transition: 'all 150ms cubic-bezier(0.4, 0, 0.2, 1)',
      '&:hover': {
        borderColor: color,
        transform: 'translateY(-1px)',
      },
    }}
  >
    <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
      <Stack direction="row" spacing={1.5} alignItems="center">
        <Avatar
          variant="rounded"
          sx={{
            width: 40,
            height: 40,
            backgroundColor: `${color}14`,
            color,
          }}
        >
          {icon}
        </Avatar>
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

/** Report row component */
interface ReportRowProps {
  report: Report;
  onDownload: (report: Report) => void;
  onDelete: (report: Report) => void;
  onShare: (report: Report) => void;
}

const ReportRow: React.FC<ReportRowProps> = React.memo(
  ({ report, onDownload, onDelete, onShare }) => {
    const theme = useTheme();

    const typeColor = getReportTypeColor(report.type);
    const formatOption = FORMAT_OPTIONS.find((o) => o.value === report.format);

    return (
      <TableRow
        hover
        sx={{
          transition: 'background-color 150ms',
          '&:hover': {
            backgroundColor: theme.palette.action.hover,
          },
        }}
      >
        {/* Title & Description */}
        <TableCell>
          <Stack direction="row" spacing={1.5} alignItems="flex-start">
            <Avatar
              variant="rounded"
              sx={{
                width: 36,
                height: 36,
                backgroundColor: `${theme.palette[typeColor].main}14`,
                color: `${typeColor}.main`,
                mt: 0.25,
              }}
            >
              {formatOption?.icon ?? <FileIcon sx={{ fontSize: 18 }} />}
            </Avatar>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography
                variant="body2"
                fontWeight={600}
                noWrap
                sx={{ lineHeight: 1.4 }}
              >
                {report.title}
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                noWrap
                sx={{
                  display: '-webkit-box',
                  WebkitLineClamp: 1,
                  WebkitBoxOrient: 'vertical',
                  overflow: 'hidden',
                  maxWidth: 320,
                }}
              >
                {report.description}
              </Typography>
            </Box>
          </Stack>
        </TableCell>

        {/* Type */}
        <TableCell>
          <Chip
            label={report.type}
            size="small"
            color={typeColor}
            variant="outlined"
            sx={{
              height: 24,
              fontSize: '0.6875rem',
              fontWeight: 600,
              textTransform: 'capitalize',
            }}
          />
        </TableCell>

        {/* Format */}
        <TableCell>
          <Stack direction="row" spacing={0.5} alignItems="center">
            {getFormatIcon(report.format)}
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ textTransform: 'uppercase', fontSize: '0.75rem', fontWeight: 600 }}
            >
              {report.format}
            </Typography>
          </Stack>
        </TableCell>

        {/* Status */}
        <TableCell>
          <StatusBadge
            status={getReportStatusBadge(report.status)}
            variant="pill"
            size="small"
            label={report.status}
            animated={report.status === 'generating'}
          />
        </TableCell>

        {/* Size */}
        <TableCell>
          <Typography variant="body2" color="text.secondary">
            {formatFileSize(report.fileSize)}
          </Typography>
        </TableCell>

        {/* Generated By */}
        <TableCell>
          <Typography variant="body2" color="text.secondary" noWrap>
            {report.generatedBy}
          </Typography>
        </TableCell>

        {/* Created */}
        <TableCell>
          <Typography variant="body2" color="text.secondary" noWrap>
            {formatRelativeTime(report.createdAt)}
          </Typography>
        </TableCell>

        {/* Actions */}
        <TableCell align="right">
          <Stack direction="row" spacing={0.25} justifyContent="flex-end">
            {report.status === 'ready' && report.downloadUrl && (
              <Tooltip title="Download Report">
                <IconButton
                  size="small"
                  color="primary"
                  onClick={() => onDownload(report)}
                >
                  <DownloadIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            )}
            <Tooltip title="Share Report">
              <IconButton size="small" onClick={() => onShare(report)}>
                <ShareIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Tooltip title="Delete Report">
              <IconButton size="small" color="error" onClick={() => onDelete(report)}>
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Stack>
        </TableCell>
      </TableRow>
    );
  },
);

ReportRow.displayName = 'ReportRow';

/** Generate Report Dialog */
interface GenerateDialogProps {
  open: boolean;
  onClose: () => void;
  onGenerate: (data: {
    title: string;
    type: ReportType;
    format: ReportFormat;
    experimentIds: string[];
    dateFrom: string;
    dateTo: string;
    description: string;
  }) => void;
  isGenerating: boolean;
}

const GenerateReportDialog: React.FC<GenerateDialogProps> = ({
  open,
  onClose,
  onGenerate,
  isGenerating,
}) => {
  const [title, setTitle] = useState('');
  const [type, setType] = useState<ReportType>('experiment');
  const [format, setFormat] = useState<ReportFormat>('pdf');
  const [description, setDescription] = useState('');
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};
    if (!title.trim()) newErrors.title = 'Title is required';
    if (!dateFrom) newErrors.dateFrom = 'Start date is required';
    if (!dateTo) newErrors.dateTo = 'End date is required';
    if (dateFrom && dateTo && dateFrom > dateTo) {
      newErrors.dateTo = 'End date must be after start date';
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleGenerate = () => {
    if (!validate()) return;
    onGenerate({
      title: title.trim(),
      type,
      format,
      experimentIds: [],
      dateFrom,
      dateTo,
      description: description.trim(),
    });
  };

  const handleClose = () => {
    if (!isGenerating) {
      setTitle('');
      setType('experiment');
      setFormat('pdf');
      setDescription('');
      setDateFrom('');
      setDateTo('');
      setErrors({});
      onClose();
    }
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{ sx: { borderRadius: 3 } }}
    >
      <DialogTitle sx={{ fontWeight: 700 }}>Generate Report</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2.5} sx={{ pt: 1 }}>
          {/* Title */}
          <TextField
            label="Report Title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            error={Boolean(errors.title)}
            helperText={errors.title}
            fullWidth
            required
            autoFocus
          />

          {/* Type & Format */}
          <Stack direction="row" spacing={2}>
            <FormControl fullWidth required>
              <InputLabel>Report Type</InputLabel>
              <Select
                value={type}
                label="Report Type"
                onChange={(e) => setType(e.target.value as ReportType)}
              >
                {REPORT_TYPE_OPTIONS.filter((o) => o.value !== 'all').map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      {option.icon}
                      <span>{option.label}</span>
                    </Stack>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControl fullWidth required>
              <InputLabel>Format</InputLabel>
              <Select
                value={format}
                label="Format"
                onChange={(e) => setFormat(e.target.value as ReportFormat)}
              >
                {FORMAT_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      {option.icon}
                      <span>{option.label}</span>
                    </Stack>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Stack>

          {/* Description */}
          <TextField
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            multiline
            rows={2}
            fullWidth
          />

          {/* Date Range */}
          <Stack direction="row" spacing={2}>
            <TextField
              label="From Date"
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
              error={Boolean(errors.dateFrom)}
              helperText={errors.dateFrom}
              InputLabelProps={{ shrink: true }}
              fullWidth
              required
            />
            <TextField
              label="To Date"
              type="date"
              value={dateTo}
              onChange={(e) => setDateTo(e.target.value)}
              error={Boolean(errors.dateTo)}
              helperText={errors.dateTo}
              InputLabelProps={{ shrink: true }}
              fullWidth
              required
            />
          </Stack>
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2, gap: 1 }}>
        <Button
          onClick={handleClose}
          disabled={isGenerating}
          sx={{ textTransform: 'none' }}
        >
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleGenerate}
          disabled={isGenerating}
          startIcon={isGenerating ? <GeneratingIcon /> : <GenerateIcon />}
          sx={{ textTransform: 'none', fontWeight: 600, minWidth: 140 }}
        >
          {isGenerating ? 'Generating...' : 'Generate Report'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

/** Share Report Dialog */
interface ShareDialogProps {
  open: boolean;
  report: Report | null;
  onClose: () => void;
  onShare: (reportId: string, recipients: string[], message: string) => void;
}

const ShareReportDialog: React.FC<ShareDialogProps> = ({
  open,
  report,
  onClose,
  onShare,
}) => {
  const [recipients, setRecipients] = useState('');
  const [message, setMessage] = useState('');

  const handleShare = () => {
    if (!report || !recipients.trim()) return;
    onShare(
      report.id,
      recipients
        .split(',')
        .map((r) => r.trim())
        .filter(Boolean),
      message.trim(),
    );
    setRecipients('');
    setMessage('');
    onClose();
  };

  const handleClose = () => {
    setRecipients('');
    setMessage('');
    onClose();
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{ sx: { borderRadius: 3 } }}
    >
      <DialogTitle sx={{ fontWeight: 700 }}>Share Report</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2.5} sx={{ pt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Sharing: <strong>{report?.title}</strong>
          </Typography>

          <TextField
            label="Recipients (comma-separated emails)"
            value={recipients}
            onChange={(e) => setRecipients(e.target.value)}
            placeholder="user1@example.com, user2@example.com"
            fullWidth
            required
            autoFocus
          />

          <TextField
            label="Message (optional)"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            multiline
            rows={3}
            fullWidth
            placeholder="Add a note for the recipients..."
          />
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2, gap: 1 }}>
        <Button onClick={handleClose} sx={{ textTransform: 'none' }}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleShare}
          disabled={!recipients.trim()}
          startIcon={<ShareIcon />}
          sx={{ textTransform: 'none', fontWeight: 600 }}
        >
          Share
        </Button>
      </DialogActions>
    </Dialog>
  );
};

/** Delete Confirmation Dialog */
interface DeleteDialogProps {
  open: boolean;
  report: Report | null;
  onClose: () => void;
  onConfirm: (reportId: string) => void;
  isDeleting: boolean;
}

const DeleteDialog: React.FC<DeleteDialogProps> = ({
  open,
  report,
  onClose,
  onConfirm,
  isDeleting,
}) => {
  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="xs"
      fullWidth
      PaperProps={{ sx: { borderRadius: 3 } }}
    >
      <DialogTitle sx={{ fontWeight: 700 }}>Delete Report</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary">
          Are you sure you want to delete <strong>{report?.title}</strong>? This action
          cannot be undone.
        </Typography>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2, gap: 1 }}>
        <Button onClick={onClose} disabled={isDeleting} sx={{ textTransform: 'none' }}>
          Cancel
        </Button>
        <Button
          variant="contained"
          color="error"
          onClick={() => report && onConfirm(report.id)}
          disabled={isDeleting}
          sx={{ textTransform: 'none', fontWeight: 600 }}
        >
          {isDeleting ? 'Deleting...' : 'Delete'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

// ---------------------------------------------------------------------------
// Skeleton Loader
// ---------------------------------------------------------------------------

const TableSkeleton: React.FC<{ rows?: number }> = ({ rows = 5 }) => (
  <>
    {Array.from({ length: rows }).map((_, rowIdx) => (
      <TableRow key={`skeleton-${rowIdx}`}>
        <TableCell>
          <Stack direction="row" spacing={1.5} alignItems="center">
            <Skeleton variant="rounded" width={36} height={36} sx={{ borderRadius: 1 }} />
            <Box sx={{ flex: 1 }}>
              <Skeleton variant="text" width="70%" />
              <Skeleton variant="text" width="40%" height={14} />
            </Box>
          </Stack>
        </TableCell>
        <TableCell>
          <Skeleton variant="rounded" width={80} height={24} sx={{ borderRadius: 12 }} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={40} />
        </TableCell>
        <TableCell>
          <Skeleton variant="rounded" width={70} height={20} sx={{ borderRadius: 12 }} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={60} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={120} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={80} />
        </TableCell>
        <TableCell>
          <Skeleton variant="rounded" width={80} height={28} sx={{ borderRadius: 1 }} />
        </TableCell>
      </TableRow>
    ))}
  </>
);

// ---------------------------------------------------------------------------
// Main Page Component
// ---------------------------------------------------------------------------

const ReportsPage: React.FC = () => {
  const theme = useTheme();
  const navigate = useNavigate();

  // -----------------------------------------------------------------------
  // State
  // -----------------------------------------------------------------------

  const [reports, setReports] = useState<Report[]>(MOCK_REPORTS);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState<ReportType | 'all'>('all');
  const [statusFilter, setStatusFilter] = useState<Report['status'] | 'all'>('all');
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');

  // Pagination
  const [page, setPage] = useState(0); // 0-based for MUI
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [sortBy, setSortBy] = useState('createdAt');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

  // Dialogs
  const [generateDialogOpen, setGenerateDialogOpen] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [shareReport, setShareReport] = useState<Report | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteReport, setDeleteReport] = useState<Report | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // Feedback
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // -----------------------------------------------------------------------
  // Computed Values
  // -----------------------------------------------------------------------

  const filteredReports = useMemo(() => {
    let result = [...reports];

    // Search
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter(
        (r) =>
          r.title.toLowerCase().includes(query) ||
          r.description.toLowerCase().includes(query) ||
          r.generatedBy.toLowerCase().includes(query),
      );
    }

    // Type filter
    if (typeFilter !== 'all') {
      result = result.filter((r) => r.type === typeFilter);
    }

    // Status filter
    if (statusFilter !== 'all') {
      result = result.filter((r) => r.status === statusFilter);
    }

    // Date range
    if (dateFrom) {
      result = result.filter((r) => r.createdAt >= dateFrom);
    }
    if (dateTo) {
      result = result.filter((r) => r.createdAt <= `${dateTo}T23:59:59Z`);
    }

    // Sort
    result.sort((a, b) => {
      let aVal: string | number;
      let bVal: string | number;

      switch (sortBy) {
        case 'title':
          aVal = a.title.toLowerCase();
          bVal = b.title.toLowerCase();
          break;
        case 'type':
          aVal = a.type;
          bVal = b.type;
          break;
        case 'status':
          aVal = a.status;
          bVal = b.status;
          break;
        case 'fileSize':
          aVal = a.fileSize ?? 0;
          bVal = b.fileSize ?? 0;
          break;
        case 'createdAt':
        default:
          aVal = a.createdAt;
          bVal = b.createdAt;
          break;
      }

      const cmp = aVal < bVal ? -1 : aVal > bVal ? 1 : 0;
      return sortOrder === 'asc' ? cmp : -cmp;
    });

    return result;
  }, [
    reports,
    searchQuery,
    typeFilter,
    statusFilter,
    dateFrom,
    dateTo,
    sortBy,
    sortOrder,
  ]);

  const paginatedReports = useMemo(() => {
    const start = page * rowsPerPage;
    return filteredReports.slice(start, start + rowsPerPage);
  }, [filteredReports, page, rowsPerPage]);

  const stats = useMemo(() => {
    return {
      total: reports.length,
      ready: reports.filter((r) => r.status === 'ready').length,
      generating: reports.filter((r) => r.status === 'generating').length,
      error: reports.filter((r) => r.status === 'error').length,
      experiment: reports.filter((r) => r.type === 'experiment').length,
      compliance: reports.filter((r) => r.type === 'compliance').length,
      executive: reports.filter((r) => r.type === 'executive').length,
      trend: reports.filter((r) => r.type === 'trend').length,
    };
  }, [reports]);

  const hasActiveFilters =
    searchQuery || typeFilter !== 'all' || statusFilter !== 'all' || dateFrom || dateTo;

  // -----------------------------------------------------------------------
  // Effects
  // -----------------------------------------------------------------------

  // Auto-dismiss success messages
  useEffect(() => {
    if (successMessage) {
      const timer = setTimeout(() => setSuccessMessage(null), 5000);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [successMessage]);

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const handleRefresh = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      // In production: const response = await reportsAPI.list();
      // For now, use mock data
      await new Promise((resolve) => setTimeout(resolve, 600));
      setReports(MOCK_REPORTS);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  }, []);

  const handleSortChange = useCallback(
    (column: string) => {
      const isCurrentSortColumn = sortBy === column;
      const newOrder = isCurrentSortColumn && sortOrder === 'asc' ? 'desc' : 'asc';
      setSortBy(column);
      setSortOrder(newOrder);
      setPage(0);
    },
    [sortBy, sortOrder],
  );

  const handleClearFilters = useCallback(() => {
    setSearchQuery('');
    setTypeFilter('all');
    setStatusFilter('all');
    setDateFrom('');
    setDateTo('');
    setPage(0);
  }, []);

  const handleGenerate = useCallback(
    async (data: {
      title: string;
      type: ReportType;
      format: ReportFormat;
      experimentIds: string[];
      dateFrom: string;
      dateTo: string;
      description: string;
    }) => {
      setIsGenerating(true);
      try {
        // In production: await reportsAPI.generate(...)
        await new Promise((resolve) => setTimeout(resolve, 2000));

        const newReport: Report = {
          id: `rpt-${Date.now()}`,
          title: data.title,
          type: data.type,
          format: data.format,
          description: data.description,
          experimentIds: data.experimentIds,
          dateRange: { from: data.dateFrom, to: data.dateTo },
          status: 'generating',
          generatedBy: 'current-user@chaos-sec.io',
          createdAt: new Date().toISOString(),
        };

        setReports((prev) => [newReport, ...prev]);
        setGenerateDialogOpen(false);
        setSuccessMessage('Report generation started. It will appear here when ready.');

        // Simulate report becoming ready after a few seconds
        setTimeout(() => {
          setReports((prev) =>
            prev.map((r) =>
              r.id === newReport.id
                ? {
                    ...r,
                    status: 'ready' as const,
                    downloadUrl: `/reports/${r.id}/download`,
                    fileSize: Math.floor(Math.random() * 5_000_000) + 500_000,
                  }
                : r,
            ),
          );
        }, 5000);
      } catch (err) {
        setError(getErrorMessage(err));
      } finally {
        setIsGenerating(false);
      }
    },
    [],
  );

  const handleDownload = useCallback(async (report: Report) => {
    if (!report.downloadUrl) return;
    try {
      // In production: const blob = await reportsAPI.download(report.id, report.format);
      // For now, simulate download
      setSuccessMessage(
        `Downloading "${report.title}" as ${report.format.toUpperCase()}...`,
      );

      // Create a mock download link
      const link = document.createElement('a');
      link.href = report.downloadUrl;
      link.download = `${report.title}.${report.format}`;
      link.click();
    } catch (err) {
      setError(getErrorMessage(err));
    }
  }, []);

  const handleShare = useCallback(
    (reportId: string, recipients: string[], message: string) => {
      // In production: await reportsAPI.share(reportId, { recipients, message })
      setSuccessMessage(
        `Report shared with ${recipients.length} recipient${recipients.length !== 1 ? 's' : ''}.`,
      );
    },
    [],
  );

  const handleDelete = useCallback(async (reportId: string) => {
    setIsDeleting(true);
    try {
      // In production: await reportsAPI.delete(reportId)
      await new Promise((resolve) => setTimeout(resolve, 800));
      setReports((prev) => prev.filter((r) => r.id !== reportId));
      setDeleteDialogOpen(false);
      setDeleteReport(null);
      setSuccessMessage('Report deleted successfully.');
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setIsDeleting(false);
    }
  }, []);

  const openShareDialog = useCallback((report: Report) => {
    setShareReport(report);
    setShareDialogOpen(true);
  }, []);

  const openDeleteDialog = useCallback((report: Report) => {
    setDeleteReport(report);
    setDeleteDialogOpen(true);
  }, []);

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
            Reports
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Generate, download, and share security validation reports.
          </Typography>
        </Box>

        <Button
          variant="contained"
          startIcon={<GenerateIcon />}
          onClick={() => setGenerateDialogOpen(true)}
          sx={{ borderRadius: 2, px: 3, textTransform: 'none', fontWeight: 600 }}
        >
          Generate Report
        </Button>
      </Stack>

      {/* Stats Summary */}
      <Grid container spacing={1.5} mb={3}>
        <Grid item xs={6} sm={3} md={2.4}>
          <StatCard
            label="Total"
            value={stats.total}
            color="#2563EB"
            icon={<ReportIcon sx={{ fontSize: 20, color: '#2563EB' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={3} md={2.4}>
          <StatCard
            label="Ready"
            value={stats.ready}
            color="#10B981"
            icon={<ReadyIcon sx={{ fontSize: 20, color: '#10B981' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={3} md={2.4}>
          <StatCard
            label="Generating"
            value={stats.generating}
            color="#F59E0B"
            icon={<GeneratingIcon sx={{ fontSize: 20, color: '#F59E0B' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={3} md={2.4}>
          <StatCard
            label="Errors"
            value={stats.error}
            color="#EF4444"
            icon={<ErrorIcon sx={{ fontSize: 20, color: '#EF4444' }} />}
          />
        </Grid>
        <Grid item xs={6} sm={3} md={2.4}>
          <StatCard
            label="Compliance"
            value={stats.compliance}
            color="#7C3AED"
            icon={<SecurityIcon sx={{ fontSize: 20, color: '#7C3AED' }} />}
          />
        </Grid>
      </Grid>

      {/* Success Message */}
      {successMessage && (
        <Alert
          severity="success"
          onClose={() => setSuccessMessage(null)}
          sx={{ mb: 2, borderRadius: 2 }}
        >
          {successMessage}
        </Alert>
      )}

      {/* Error Alert */}
      {error && (
        <Alert
          severity="error"
          onClose={() => setError(null)}
          sx={{ mb: 2, borderRadius: 2 }}
        >
          {error}
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
          {/* Search */}
          <TextField
            size="small"
            placeholder="Search reports..."
            value={searchQuery}
            onChange={(e) => {
              setSearchQuery(e.target.value);
              setPage(0);
            }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                </InputAdornment>
              ),
              endAdornment: searchQuery ? (
                <InputAdornment position="end">
                  <IconButton
                    size="small"
                    onClick={() => {
                      setSearchQuery('');
                      setPage(0);
                    }}
                  >
                    <ClearIcon sx={{ fontSize: 16 }} />
                  </IconButton>
                </InputAdornment>
              ) : null,
            }}
            sx={{
              flex: 1,
              minWidth: { xs: '100%', md: 280 },
              '& .MuiOutlinedInput-root': { borderRadius: 1.5 },
            }}
          />

          {/* Type Filter */}
          <FormControl size="small" sx={{ minWidth: 140 }}>
            <InputLabel>Type</InputLabel>
            <Select
              value={typeFilter}
              label="Type"
              onChange={(e) => {
                setTypeFilter(e.target.value as ReportType | 'all');
                setPage(0);
              }}
              sx={{ borderRadius: 1.5 }}
            >
              {REPORT_TYPE_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    {option.icon}
                    <span>{option.label}</span>
                  </Stack>
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Status Filter */}
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel>Status</InputLabel>
            <Select
              value={statusFilter}
              label="Status"
              onChange={(e) => {
                setStatusFilter(e.target.value as Report['status'] | 'all');
                setPage(0);
              }}
              sx={{ borderRadius: 1.5 }}
            >
              <MenuItem value="all">All Statuses</MenuItem>
              <MenuItem value="generating">Generating</MenuItem>
              <MenuItem value="ready">Ready</MenuItem>
              <MenuItem value="error">Error</MenuItem>
            </Select>
          </FormControl>

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
          {hasActiveFilters && (
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

        {/* Date Range Row */}
        <Box
          sx={{
            px: 2,
            pb: 2,
            pt: 0,
            borderTop: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Stack direction="row" spacing={2} alignItems="center" sx={{ pt: 1.5 }}>
            <DateRangeIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
            <TextField
              size="small"
              label="From"
              type="date"
              value={dateFrom}
              onChange={(e) => {
                setDateFrom(e.target.value);
                setPage(0);
              }}
              InputLabelProps={{ shrink: true }}
              sx={{ width: 180 }}
            />
            <Typography variant="body2" color="text.secondary">
              to
            </Typography>
            <TextField
              size="small"
              label="To"
              type="date"
              value={dateTo}
              onChange={(e) => {
                setDateTo(e.target.value);
                setPage(0);
              }}
              InputLabelProps={{ shrink: true }}
              sx={{ width: 180 }}
            />
            {(dateFrom || dateTo) && (
              <IconButton
                size="small"
                onClick={() => {
                  setDateFrom('');
                  setDateTo('');
                  setPage(0);
                }}
              >
                <ClearIcon sx={{ fontSize: 16 }} />
              </IconButton>
            )}
          </Stack>
        </Box>
      </Paper>

      {/* Active Filters Chips */}
      {hasActiveFilters && (
        <Stack direction="row" spacing={1} mb={2} flexWrap="wrap" useFlexGap>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ lineHeight: '26px' }}
          >
            Active filters:
          </Typography>
          {searchQuery && (
            <Chip
              label={`Search: "${searchQuery}"`}
              size="small"
              onDelete={() => {
                setSearchQuery('');
                setPage(0);
              }}
              sx={{ height: 26 }}
            />
          )}
          {typeFilter !== 'all' && (
            <Chip
              label={`Type: ${typeFilter}`}
              size="small"
              onDelete={() => {
                setTypeFilter('all');
                setPage(0);
              }}
              sx={{ height: 26 }}
            />
          )}
          {statusFilter !== 'all' && (
            <Chip
              label={`Status: ${statusFilter}`}
              size="small"
              onDelete={() => {
                setStatusFilter('all');
                setPage(0);
              }}
              sx={{ height: 26 }}
            />
          )}
        </Stack>
      )}

      {/* Reports Table */}
      <Paper
        elevation={0}
        sx={{
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
          overflow: 'hidden',
        }}
      >
        {/* Results Summary */}
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          sx={{ px: 2, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}
        >
          <Typography variant="body2" color="text.secondary">
            {isLoading
              ? 'Loading reports...'
              : `${filteredReports.length} report${filteredReports.length !== 1 ? 's' : ''} found`}
          </Typography>
          <Typography variant="caption" color="text.disabled">
            Page {page + 1} of{' '}
            {Math.max(1, Math.ceil(filteredReports.length / rowsPerPage))}
          </Typography>
        </Stack>

        <TableContainer>
          <Table stickyHeader aria-label="reports table">
            <TableHead>
              <TableRow>
                <TableCell sx={{ minWidth: 260 }}>
                  <TableSortLabel
                    active={sortBy === 'title'}
                    direction={sortBy === 'title' ? sortOrder : 'asc'}
                    onClick={() => handleSortChange('title')}
                  >
                    Report
                  </TableSortLabel>
                </TableCell>
                <TableCell sx={{ minWidth: 110 }}>
                  <TableSortLabel
                    active={sortBy === 'type'}
                    direction={sortBy === 'type' ? sortOrder : 'asc'}
                    onClick={() => handleSortChange('type')}
                  >
                    Type
                  </TableSortLabel>
                </TableCell>
                <TableCell sx={{ minWidth: 80 }}>Format</TableCell>
                <TableCell sx={{ minWidth: 100 }}>
                  <TableSortLabel
                    active={sortBy === 'status'}
                    direction={sortBy === 'status' ? sortOrder : 'asc'}
                    onClick={() => handleSortChange('status')}
                  >
                    Status
                  </TableSortLabel>
                </TableCell>
                <TableCell sx={{ minWidth: 80 }}>
                  <TableSortLabel
                    active={sortBy === 'fileSize'}
                    direction={sortBy === 'fileSize' ? sortOrder : 'asc'}
                    onClick={() => handleSortChange('fileSize')}
                  >
                    Size
                  </TableSortLabel>
                </TableCell>
                <TableCell sx={{ minWidth: 140 }}>Generated By</TableCell>
                <TableCell sx={{ minWidth: 100 }}>
                  <TableSortLabel
                    active={sortBy === 'createdAt'}
                    direction={sortBy === 'createdAt' ? sortOrder : 'asc'}
                    onClick={() => handleSortChange('createdAt')}
                  >
                    Created
                  </TableSortLabel>
                </TableCell>
                <TableCell align="right" sx={{ minWidth: 120 }}>
                  Actions
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {isLoading ? (
                <TableSkeleton rows={rowsPerPage} />
              ) : paginatedReports.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} align="center">
                    <Box sx={{ py: 6, px: 2, textAlign: 'center' }}>
                      <ReportIcon
                        sx={{ fontSize: 56, color: 'text.disabled', mb: 1.5 }}
                      />
                      <Typography variant="h6" color="text.secondary" gutterBottom>
                        No reports found
                      </Typography>
                      <Typography
                        variant="body2"
                        color="text.disabled"
                        sx={{ mb: 2, maxWidth: 400, mx: 'auto' }}
                      >
                        {hasActiveFilters
                          ? 'Try adjusting your filters or search terms.'
                          : 'Generate your first report to document your security validation results.'}
                      </Typography>
                      {hasActiveFilters ? (
                        <Button
                          variant="outlined"
                          startIcon={<ClearIcon />}
                          onClick={handleClearFilters}
                          sx={{ mr: 1, textTransform: 'none' }}
                        >
                          Clear Filters
                        </Button>
                      ) : null}
                      <Button
                        variant="contained"
                        startIcon={<GenerateIcon />}
                        onClick={() => setGenerateDialogOpen(true)}
                        sx={{ textTransform: 'none' }}
                      >
                        Generate Report
                      </Button>
                    </Box>
                  </TableCell>
                </TableRow>
              ) : (
                paginatedReports.map((report) => (
                  <ReportRow
                    key={report.id}
                    report={report}
                    onDownload={handleDownload}
                    onDelete={openDeleteDialog}
                    onShare={openShareDialog}
                  />
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>

        {/* Pagination */}
        {!isLoading && filteredReports.length > 0 && (
          <TablePagination
            component="div"
            count={filteredReports.length}
            page={page}
            rowsPerPage={rowsPerPage}
            onPageChange={(_, newPage) => setPage(newPage)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
            rowsPerPageOptions={PAGE_SIZE_OPTIONS}
            sx={{
              borderTop: '1px solid',
              borderColor: 'divider',
              '& .MuiTablePagination-toolbar': { minHeight: 52 },
              '& .MuiTablePagination-selectLabel, & .MuiTablePagination-displayedRows': {
                fontSize: '0.8125rem',
              },
            }}
          />
        )}
      </Paper>

      {/* Quick Reports Section */}
      <Box sx={{ mt: 3 }}>
        <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
          Quick Generate
        </Typography>
        <Grid container spacing={2}>
          {/* Experiment Report */}
          <Grid item xs={12} sm={6} md={3}>
            <Card
              sx={{
                cursor: 'pointer',
                transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
                '&:hover': {
                  borderColor: 'primary.main',
                  transform: 'translateY(-2px)',
                  boxShadow: '0 4px 16px rgba(37, 99, 235, 0.15)',
                },
              }}
              onClick={() => setGenerateDialogOpen(true)}
            >
              <CardContent
                sx={{ p: 2.5, '&:last-child': { pb: 2.5 }, textAlign: 'center' }}
              >
                <Avatar
                  sx={{
                    width: 48,
                    height: 48,
                    mx: 'auto',
                    mb: 1.5,
                    backgroundColor: 'primary.main',
                  }}
                >
                  <ExperimentIcon />
                </Avatar>
                <Typography variant="subtitle2" fontWeight={700}>
                  Experiment Report
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: 'block', mt: 0.5 }}
                >
                  Detailed results for a specific experiment
                </Typography>
              </CardContent>
            </Card>
          </Grid>

          {/* Compliance Report */}
          <Grid item xs={12} sm={6} md={3}>
            <Card
              sx={{
                cursor: 'pointer',
                transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
                '&:hover': {
                  borderColor: 'success.main',
                  transform: 'translateY(-2px)',
                  boxShadow: '0 4px 16px rgba(16, 185, 129, 0.15)',
                },
              }}
              onClick={() => setGenerateDialogOpen(true)}
            >
              <CardContent
                sx={{ p: 2.5, '&:last-child': { pb: 2.5 }, textAlign: 'center' }}
              >
                <Avatar
                  sx={{
                    width: 48,
                    height: 48,
                    mx: 'auto',
                    mb: 1.5,
                    backgroundColor: 'success.main',
                  }}
                >
                  <SecurityIcon />
                </Avatar>
                <Typography variant="subtitle2" fontWeight={700}>
                  Compliance Report
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: 'block', mt: 0.5 }}
                >
                  Framework mapping & evidence
                </Typography>
              </CardContent>
            </Card>
          </Grid>

          {/* Executive Summary */}
          <Grid item xs={12} sm={6} md={3}>
            <Card
              sx={{
                cursor: 'pointer',
                transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
                '&:hover': {
                  borderColor: 'warning.main',
                  transform: 'translateY(-2px)',
                  boxShadow: '0 4px 16px rgba(245, 158, 11, 0.15)',
                },
              }}
              onClick={() => setGenerateDialogOpen(true)}
            >
              <CardContent
                sx={{ p: 2.5, '&:last-child': { pb: 2.5 }, textAlign: 'center' }}
              >
                <Avatar
                  sx={{
                    width: 48,
                    height: 48,
                    mx: 'auto',
                    mb: 1.5,
                    backgroundColor: 'warning.main',
                  }}
                >
                  <ExecutiveIcon />
                </Avatar>
                <Typography variant="subtitle2" fontWeight={700}>
                  Executive Summary
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: 'block', mt: 0.5 }}
                >
                  High-level overview for leadership
                </Typography>
              </CardContent>
            </Card>
          </Grid>

          {/* Trend Analysis */}
          <Grid item xs={12} sm={6} md={3}>
            <Card
              sx={{
                cursor: 'pointer',
                transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
                '&:hover': {
                  borderColor: 'info.main',
                  transform: 'translateY(-2px)',
                  boxShadow: '0 4px 16px rgba(6, 182, 212, 0.15)',
                },
              }}
              onClick={() => setGenerateDialogOpen(true)}
            >
              <CardContent
                sx={{ p: 2.5, '&:last-child': { pb: 2.5 }, textAlign: 'center' }}
              >
                <Avatar
                  sx={{
                    width: 48,
                    height: 48,
                    mx: 'auto',
                    mb: 1.5,
                    backgroundColor: 'info.main',
                  }}
                >
                  <TrendIcon />
                </Avatar>
                <Typography variant="subtitle2" fontWeight={700}>
                  Trend Analysis
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: 'block', mt: 0.5 }}
                >
                  Security posture over time
                </Typography>
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      </Box>

      {/* Dialogs */}
      <GenerateReportDialog
        open={generateDialogOpen}
        onClose={() => setGenerateDialogOpen(false)}
        onGenerate={handleGenerate}
        isGenerating={isGenerating}
      />

      <ShareReportDialog
        open={shareDialogOpen}
        report={shareReport}
        onClose={() => {
          setShareDialogOpen(false);
          setShareReport(null);
        }}
        onShare={handleShare}
      />

      <DeleteDialog
        open={deleteDialogOpen}
        report={deleteReport}
        onClose={() => {
          setDeleteDialogOpen(false);
          setDeleteReport(null);
        }}
        onConfirm={handleDelete}
        isDeleting={isDeleting}
      />
    </Box>
  );
};

export default ReportsPage;
