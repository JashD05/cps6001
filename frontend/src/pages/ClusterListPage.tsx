import {
  Search as SearchIcon,
  Add as AddIcon,
  Refresh as RefreshIcon,
  Dns as ClusterIcon,
  Cloud as CloudIcon,
  Storage as StorageIcon,
  Memory as MemoryIcon,
  Speed as CpuIcon,
  CheckCircle as HealthyIcon,
  Warning as DegradedIcon,
  CloudOff as UnreachableIcon,
  InfoOutlined as InfoIcon,
  MoreVert as MoreIcon,
  Visibility as ViewIcon,
  Settings as SettingsIcon,
  Delete as DeleteIcon,
  Clear as ClearIcon,
  Wifi as WifiIcon,
  VpnKey as VpnKeyIcon,
  Terminal as TerminalIcon,
  Schedule as ScheduleIcon,
  ErrorOutline as ErrorIcon,
} from '@mui/icons-material';
import {
  Box,
  Typography,
  Card,
  CardContent,
  CardActions,
  Button,
  TextField,
  InputAdornment,
  IconButton,
  Grid,
  Stack,
  Chip,
  LinearProgress,
  Paper,
  Divider,
  Tooltip,
  Skeleton,
  Alert,
  Avatar,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  useTheme,
  alpha,
  Snackbar,
} from '@mui/material';
import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useDispatch } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import StatusBadge from '@/components/StatusBadge';
import { clustersAPI } from '@/services/api';
import type { AppDispatch } from '@/store';
import type { Cluster, ClusterStatus, ClusterHealth } from '@/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const PROVIDER_CONFIG: Record<
  string,
  { label: string; color: string; icon: React.ReactNode }
> = {
  aws: { label: 'AWS', color: '#FF9900', icon: <CloudIcon /> },
  gcp: { label: 'GCP', color: '#4285F4', icon: <CloudIcon /> },
  azure: { label: 'Azure', color: '#0078D4', icon: <CloudIcon /> },
  'on-prem': { label: 'On-Prem', color: '#6B7280', icon: <StorageIcon /> },
  kind: { label: 'Kind', color: '#7C3AED', icon: <TerminalIcon /> },
  other: { label: 'Other', color: '#94A3B8', icon: <CloudIcon /> },
};

const STATUS_OPTIONS: { value: ClusterStatus | 'all'; label: string }[] = [
  { value: 'all', label: 'All Statuses' },
  { value: 'healthy', label: 'Healthy' },
  { value: 'degraded', label: 'Degraded' },
  { value: 'unreachable', label: 'Unreachable' },
  { value: 'unknown', label: 'Unknown' },
];

const CLUSTER_PAGE_HELP =
  'A cluster is a group of servers where your apps run. Healthy means it is working, degraded means it has problems, and unreachable means the app cannot connect to it.';

const CLUSTER_STAT_HELP = {
  total: 'How many clusters the app knows about right now.',
  healthy: 'Clusters that are working normally.',
  degraded: 'Clusters that are working, but have problems or high load.',
  unreachable: 'Clusters the app cannot reach right now.',
};

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
  if (diffDays < 30) return `${diffDays}d ago`;
  return date.toLocaleDateString();
};

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Cluster health summary card */
function ClusterHealthInfo({
  health,
  compact = false,
}: {
  health: ClusterHealth;
  compact?: boolean;
}) {
  const theme = useTheme();

  if (compact) {
    return (
      <Stack direction="row" spacing={2} sx={{ width: '100%' }}>
        <Box sx={{ flex: 1 }}>
          <Stack direction="row" justifyContent="space-between" mb={0.25}>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.625rem' }}
            >
              CPU
            </Typography>
            <Typography variant="caption" fontWeight={600} sx={{ fontSize: '0.625rem' }}>
              {health.cpuUsage}%
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={health.cpuUsage}
            sx={{
              height: 3,
              borderRadius: 1.5,
              backgroundColor: theme.palette.divider,
              '& .MuiLinearProgress-bar': {
                borderRadius: 1.5,
                backgroundColor:
                  health.cpuUsage > 80
                    ? theme.palette.error.main
                    : health.cpuUsage > 60
                      ? theme.palette.warning.main
                      : theme.palette.success.main,
              },
            }}
          />
        </Box>
        <Box sx={{ flex: 1 }}>
          <Stack direction="row" justifyContent="space-between" mb={0.25}>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.625rem' }}
            >
              MEM
            </Typography>
            <Typography variant="caption" fontWeight={600} sx={{ fontSize: '0.625rem' }}>
              {health.memoryUsage}%
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={health.memoryUsage}
            sx={{
              height: 3,
              borderRadius: 1.5,
              backgroundColor: theme.palette.divider,
              '& .MuiLinearProgress-bar': {
                borderRadius: 1.5,
                backgroundColor:
                  health.memoryUsage > 80
                    ? theme.palette.error.main
                    : health.memoryUsage > 60
                      ? theme.palette.warning.main
                      : theme.palette.success.main,
              },
            }}
          />
        </Box>
      </Stack>
    );
  }

  return (
    <Stack spacing={1.5} sx={{ width: '100%' }}>
      {/* CPU */}
      <Box>
        <Stack direction="row" justifyContent="space-between" mb={0.5}>
          <Stack direction="row" spacing={0.5} alignItems="center">
            <CpuIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem' }}
            >
              CPU Usage
            </Typography>
          </Stack>
          <Typography variant="caption" fontWeight={600} sx={{ fontSize: '0.6875rem' }}>
            {health.cpuUsage}%
          </Typography>
        </Stack>
        <LinearProgress
          variant="determinate"
          value={health.cpuUsage}
          sx={{
            height: 6,
            borderRadius: 3,
            backgroundColor: theme.palette.divider,
            '& .MuiLinearProgress-bar': {
              borderRadius: 3,
              backgroundColor:
                health.cpuUsage > 80
                  ? theme.palette.error.main
                  : health.cpuUsage > 60
                    ? theme.palette.warning.main
                    : theme.palette.success.main,
            },
          }}
        />
      </Box>

      {/* Memory */}
      <Box>
        <Stack direction="row" justifyContent="space-between" mb={0.5}>
          <Stack direction="row" spacing={0.5} alignItems="center">
            <MemoryIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem' }}
            >
              Memory Usage
            </Typography>
          </Stack>
          <Typography variant="caption" fontWeight={600} sx={{ fontSize: '0.6875rem' }}>
            {health.memoryUsage}%
          </Typography>
        </Stack>
        <LinearProgress
          variant="determinate"
          value={health.memoryUsage}
          sx={{
            height: 6,
            borderRadius: 3,
            backgroundColor: theme.palette.divider,
            '& .MuiLinearProgress-bar': {
              borderRadius: 3,
              backgroundColor:
                health.memoryUsage > 80
                  ? theme.palette.error.main
                  : health.memoryUsage > 60
                    ? theme.palette.warning.main
                    : theme.palette.success.main,
            },
          }}
        />
      </Box>

      {/* Stats row */}
      <Stack direction="row" spacing={2}>
        <Stack direction="row" spacing={0.5} alignItems="center">
          <ClusterIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontSize: '0.6875rem' }}
          >
            {health.nodeCount} node{health.nodeCount !== 1 ? 's' : ''}
          </Typography>
        </Stack>
        <Stack direction="row" spacing={0.5} alignItems="center">
          <StorageIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontSize: '0.6875rem' }}
          >
            {health.podCount} pod{health.podCount !== 1 ? 's' : ''}
          </Typography>
        </Stack>
        {health.errorRate > 0 && (
          <Stack direction="row" spacing={0.5} alignItems="center">
            <DegradedIcon
              sx={{
                fontSize: 14,
                color: health.errorRate > 0.05 ? 'error.main' : 'warning.main',
              }}
            />
            <Typography
              variant="caption"
              sx={{
                fontSize: '0.6875rem',
                color: health.errorRate > 0.05 ? 'error.main' : 'warning.main',
                fontWeight: 600,
              }}
            >
              {(health.errorRate * 100).toFixed(1)}% error rate
            </Typography>
          </Stack>
        )}
      </Stack>
    </Stack>
  );
}

/** Individual Cluster Card */
function ClusterCard({
  cluster,
  health,
  onHealthCheck,
  onDelete,
  loading = false,
}: {
  cluster: Cluster;
  health?: ClusterHealth;
  onHealthCheck: (id: string) => void;
  onDelete: (id: string) => void;
  loading?: boolean;
}) {
  const theme = useTheme();
  const navigate = useNavigate();
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);

  const providerConfig =
    PROVIDER_CONFIG[cluster.provider ?? 'other'] ?? PROVIDER_CONFIG['other'];

  if (loading) {
    return (
      <Card sx={{ height: '100%', borderRadius: 2 }}>
        <CardContent sx={{ p: 2.5, '&:last-child': { pb: 2.5 } }}>
          <Stack spacing={2}>
            <Skeleton variant="rounded" width={40} height={40} />
            <Skeleton variant="text" width="70%" height={24} />
            <Skeleton variant="text" width="50%" height={16} />
            <Skeleton variant="rounded" height={6} />
            <Skeleton variant="rounded" height={6} />
            <Stack direction="row" spacing={1}>
              <Skeleton variant="rounded" width={60} height={24} />
              <Skeleton variant="rounded" width={60} height={24} />
            </Stack>
          </Stack>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        borderRadius: 2,
        transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
        borderLeft: '4px solid',
        borderLeftColor:
          cluster.status === 'healthy'
            ? theme.palette.success.main
            : cluster.status === 'degraded'
              ? theme.palette.warning.main
              : cluster.status === 'unreachable'
                ? theme.palette.error.main
                : theme.palette.divider,
        '&:hover': {
          transform: 'translateY(-2px)',
          boxShadow: `0 8px 24px ${alpha(theme.palette.text.primary, 0.08)}`,
          borderColor: 'primary.main',
        },
      }}
    >
      <CardContent sx={{ p: 2.5, '&:last-child': { pb: 1.5 }, flex: 1 }}>
        {/* Header: Name + Status + Menu */}
        <Stack
          direction="row"
          alignItems="flex-start"
          justifyContent="space-between"
          mb={1.5}
        >
          <Stack
            direction="row"
            spacing={1.5}
            alignItems="center"
            sx={{ minWidth: 0, flex: 1 }}
          >
            <Avatar
              variant="rounded"
              sx={{
                width: 40,
                height: 40,
                backgroundColor: `${providerConfig.color}18`,
                color: providerConfig.color,
              }}
            >
              {providerConfig.icon}
            </Avatar>
            <Box sx={{ minWidth: 0 }}>
              <Typography
                variant="subtitle1"
                fontWeight={700}
                noWrap
                sx={{ fontSize: '0.9375rem', lineHeight: 1.3 }}
              >
                {cluster.name}
              </Typography>
              <Stack direction="row" spacing={1} alignItems="center">
                <StatusBadge status={cluster.status} variant="dot" size="small" />
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontSize: '0.6875rem' }}
                >
                  {providerConfig.label}
                </Typography>
              </Stack>
            </Box>
          </Stack>
          <Stack direction="row" spacing={0} sx={{ flexShrink: 0 }}>
            <Tooltip title="More Actions">
              <IconButton size="small" onClick={(e) => setMenuAnchor(e.currentTarget)}>
                <MoreIcon fontSize="small" />
              </IconButton>
            </Tooltip>
            <Menu
              anchorEl={menuAnchor}
              open={Boolean(menuAnchor)}
              onClose={() => setMenuAnchor(null)}
              transformOrigin={{ horizontal: 'right', vertical: 'top' }}
              anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
              PaperProps={{
                sx: {
                  borderRadius: 2,
                  minWidth: 180,
                  boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
                },
              }}
            >
              <MenuItem
                onClick={() => {
                  setMenuAnchor(null);
                  navigate(`/clusters`);
                }}
              >
                <ListItemIcon>
                  <ViewIcon sx={{ fontSize: 18 }} />
                </ListItemIcon>
                <ListItemText>View Details</ListItemText>
              </MenuItem>
              <MenuItem
                onClick={() => {
                  setMenuAnchor(null);
                  onHealthCheck(cluster.id);
                }}
              >
                <ListItemIcon>
                  <WifiIcon sx={{ fontSize: 18 }} />
                </ListItemIcon>
                <ListItemText>Health Check</ListItemText>
              </MenuItem>
              <MenuItem
                onClick={() => {
                  setMenuAnchor(null);
                }}
              >
                <ListItemIcon>
                  <SettingsIcon sx={{ fontSize: 18 }} />
                </ListItemIcon>
                <ListItemText>Settings</ListItemText>
              </MenuItem>
              <Divider />
              <MenuItem
                onClick={() => {
                  setMenuAnchor(null);
                  onDelete(cluster.id);
                }}
                sx={{ color: 'error.main' }}
              >
                <ListItemIcon>
                  <DeleteIcon sx={{ fontSize: 18, color: 'error.main' }} />
                </ListItemIcon>
                <ListItemText>Remove Cluster</ListItemText>
              </MenuItem>
            </Menu>
          </Stack>
        </Stack>

        {/* Description */}
        {cluster.description && (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{
              mb: 2,
              fontSize: '0.8125rem',
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
              lineHeight: 1.5,
            }}
          >
            {cluster.description}
          </Typography>
        )}

        {/* Cluster Metadata */}
        <Stack spacing={1} mb={2}>
          <Stack direction="row" spacing={1.5} flexWrap="wrap" useFlexGap>
            <Chip
              icon={<ClusterIcon sx={{ fontSize: 14 }} />}
              label={`v${cluster.version}`}
              size="small"
              variant="outlined"
              sx={{ height: 24, fontSize: '0.6875rem', fontFamily: 'monospace' }}
            />
            <Chip
              label={cluster.region || 'unknown'}
              size="small"
              variant="outlined"
              sx={{ height: 24, fontSize: '0.6875rem' }}
            />
            <Chip
              icon={<StorageIcon sx={{ fontSize: 14 }} />}
              label={`${cluster.nodeCount} node${cluster.nodeCount !== 1 ? 's' : ''}`}
              size="small"
              variant="outlined"
              sx={{ height: 24, fontSize: '0.6875rem' }}
            />
          </Stack>

          {/* Namespaces preview */}
          <Box>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontSize: '0.6875rem', mb: 0.5, display: 'block' }}
            >
              Namespaces ({cluster.namespaceCount ?? cluster.namespaces?.length ?? 0})
            </Typography>
            <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
              {(cluster.namespaces ?? []).slice(0, 4).map((ns) => (
                <Chip
                  key={ns}
                  label={ns}
                  size="small"
                  sx={{
                    height: 20,
                    fontSize: '0.625rem',
                    fontFamily: 'monospace',
                    backgroundColor: 'action.hover',
                    border: '1px solid',
                    borderColor: 'divider',
                  }}
                />
              ))}
              {(cluster.namespaces ?? []).length > 4 && (
                <Chip
                  label={`+${(cluster.namespaces ?? []).length - 4}`}
                  size="small"
                  sx={{
                    height: 20,
                    fontSize: '0.625rem',
                    fontWeight: 600,
                    backgroundColor: 'action.hover',
                  }}
                />
              )}
            </Stack>
          </Box>
        </Stack>

        <Divider sx={{ my: 1.5 }} />

        {/* Health Information */}
        {health ? (
          <ClusterHealthInfo health={health} compact />
        ) : (
          <Box sx={{ py: 1, textAlign: 'center' }}>
            <Typography
              variant="caption"
              color="text.disabled"
              sx={{ fontSize: '0.6875rem' }}
            >
              No health data available
            </Typography>
          </Box>
        )}

        {/* Last health check */}
        <Stack direction="row" spacing={0.5} alignItems="center" mt={1}>
          <ScheduleIcon sx={{ fontSize: 12, color: 'text.disabled' }} />
          <Typography
            variant="caption"
            color="text.disabled"
            sx={{ fontSize: '0.625rem' }}
          >
            Checked {formatRelativeTime(cluster.lastHealthCheck ?? null)}
          </Typography>
        </Stack>
      </CardContent>

      <CardActions sx={{ px: 2.5, pb: 2, pt: 0, justifyContent: 'space-between' }}>
        <Tooltip title="Test whether this cluster is reachable and healthy." arrow>
          <Button
            size="small"
            startIcon={<WifiIcon sx={{ fontSize: 16 }} />}
            onClick={() => onHealthCheck(cluster.id)}
            sx={{ textTransform: 'none', fontWeight: 600, fontSize: '0.8125rem' }}
          >
            Health Check
          </Button>
        </Tooltip>
        <Tooltip title="Start a chaos experiment on this cluster." arrow>
          <Button
            size="small"
            variant="outlined"
            onClick={() => navigate('/experiments/new')}
            sx={{ textTransform: 'none', fontWeight: 600, fontSize: '0.8125rem' }}
          >
            Run Experiment
          </Button>
        </Tooltip>
      </CardActions>
    </Card>
  );
}

/** Summary stat card */
function StatCard({
  label,
  value,
  icon,
  color,
  tooltip,
}: {
  label: string;
  value: number;
  icon: React.ReactNode;
  color: string;
  tooltip?: string;
}) {
  return (
    <Card sx={{ borderRadius: 2, height: '100%' }}>
      <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
        <Stack direction="row" spacing={1.5} alignItems="center">
          <Box
            sx={{
              width: 36,
              height: 36,
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
            {tooltip ? (
              <Tooltip title={tooltip} arrow placement="top-start">
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500, fontSize: '0.6875rem', cursor: 'help' }}
                >
                  {label}
                </Typography>
              </Tooltip>
            ) : (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontWeight: 500, fontSize: '0.6875rem' }}
              >
                {label}
              </Typography>
            )}
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Register Cluster Dialog
// ---------------------------------------------------------------------------

interface RegisterClusterDialogProps {
  open: boolean;
  onClose: () => void;
  onRegister: (data: Partial<Cluster>) => void;
}

function RegisterClusterDialog({
  open,
  onClose,
  onRegister,
}: RegisterClusterDialogProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [provider, setProvider] = useState<Cluster['provider']>('aws');
  const [region, setRegion] = useState('');
  const [kubeconfig, setKubeconfig] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async () => {
    if (!name.trim() || !region.trim()) return;
    setIsSubmitting(true);
    try {
      onRegister({
        name: name.trim(),
        description: description.trim(),
        provider,
        region: region.trim(),
      });
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    if (!isSubmitting) {
      setName('');
      setDescription('');
      setProvider('aws');
      setRegion('');
      setKubeconfig('');
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
      <DialogTitle sx={{ fontWeight: 700 }}>Register New Cluster</DialogTitle>
      <DialogContent>
        <Stack spacing={2.5} sx={{ mt: 1 }}>
          <TextField
            label="Cluster Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            fullWidth
            required
            placeholder="e.g., prod-us-east-1"
            helperText="A unique identifier for your cluster"
          />
          <TextField
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            fullWidth
            multiline
            rows={2}
            placeholder="Brief description of the cluster purpose"
          />
          <Stack direction="row" spacing={2}>
            <FormControl fullWidth size="medium">
              <InputLabel>Provider</InputLabel>
              <Select
                value={provider}
                label="Provider"
                onChange={(e) => setProvider(e.target.value as Cluster['provider'])}
              >
                {Object.entries(PROVIDER_CONFIG).map(([key, config]) => (
                  <MenuItem key={key} value={key}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Box
                        sx={{
                          width: 12,
                          height: 12,
                          borderRadius: '50%',
                          backgroundColor: config.color,
                        }}
                      />
                      <span>{config.label}</span>
                    </Stack>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <TextField
              label="Region"
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              fullWidth
              required
              placeholder="e.g., us-east-1"
            />
          </Stack>
          <TextField
            label="Kubeconfig (optional)"
            value={kubeconfig}
            onChange={(e) => setKubeconfig(e.target.value)}
            fullWidth
            multiline
            rows={3}
            placeholder="Paste your kubeconfig YAML here, or configure connection later"
            InputProps={{
              startAdornment: (
                <InputAdornment
                  position="start"
                  sx={{ alignSelf: 'flex-start', mt: 1.5 }}
                >
                  <VpnKeyIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                </InputAdornment>
              ),
            }}
            sx={{
              '& .MuiOutlinedInput-root': {
                fontFamily: 'monospace',
                fontSize: '0.8125rem',
              },
            }}
          />
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 3 }}>
        <Button onClick={handleClose} disabled={isSubmitting}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={!name.trim() || !region.trim() || isSubmitting}
        >
          {isSubmitting ? 'Registering...' : 'Register Cluster'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Main Page Component
// ---------------------------------------------------------------------------

const ClusterListPage: React.FC = () => {
  const _theme = useTheme();
  const _dispatch = useDispatch<AppDispatch>();
  const _navigate = useNavigate();

  // State
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [healthData, setHealthData] = useState<Record<string, ClusterHealth>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<ClusterStatus | 'all'>('all');
  const [providerFilter, setProviderFilter] = useState<string>('all');

  // Dialogs
  const [registerDialogOpen, setRegisterDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [clusterToDelete, setClusterToDelete] = useState<string | null>(null);

  // Snackbar for health check errors
  const [healthCheckError, setHealthCheckError] = useState<string | null>(null);

  // Fetch clusters
  const fetchClusters = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const response = await clustersAPI.list();
      if (response.data && 'items' in response.data) {
        setClusters(response.data.items);
      }
    } catch (err) {
      setError('Failed to load clusters. Please check your connection and try again.');
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchClusters();
  }, [fetchClusters]);

  // Filtered clusters
  const filteredClusters = useMemo(() => {
    return clusters.filter((cluster) => {
      const matchesSearch =
        !searchQuery ||
        cluster.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        cluster.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (cluster.region ?? '').toLowerCase().includes(searchQuery.toLowerCase());

      const matchesStatus = statusFilter === 'all' || cluster.status === statusFilter;

      const matchesProvider =
        providerFilter === 'all' || cluster.provider === providerFilter;

      return matchesSearch && matchesStatus && matchesProvider;
    });
  }, [clusters, searchQuery, statusFilter, providerFilter]);

  // Stats
  const stats = useMemo(() => {
    return {
      total: clusters.length,
      healthy: clusters.filter((c) => c.status === 'healthy').length,
      degraded: clusters.filter((c) => c.status === 'degraded').length,
      unreachable: clusters.filter((c) => c.status === 'unreachable').length,
    };
  }, [clusters]);

  // Handlers
  const handleHealthCheck = useCallback(async (clusterId: string) => {
    try {
      const response = await clustersAPI.healthCheck(clusterId);
      const health = response.data.data as ClusterHealth;
      setHealthData((prev) => ({ ...prev, [clusterId]: health }));
    } catch {
      setHealthCheckError('Health check failed. Please try again.');
    }
  }, []);

  const handleDeleteCluster = useCallback((clusterId: string) => {
    setClusterToDelete(clusterId);
    setDeleteDialogOpen(true);
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!clusterToDelete) return;
    try {
      await clustersAPI.delete(clusterToDelete);
      setClusters((prev) => prev.filter((c) => c.id !== clusterToDelete));
    } catch {
      // Remove from local state even on error for demo
      setClusters((prev) => prev.filter((c) => c.id !== clusterToDelete));
    }
    setDeleteDialogOpen(false);
    setClusterToDelete(null);
  }, [clusterToDelete]);

  const handleRegisterCluster = useCallback(async (data: Partial<Cluster>) => {
    try {
      const response = await clustersAPI.register(data);
      const newCluster = response.data.data as Cluster;
      setClusters((prev) => [newCluster, ...prev]);
    } catch {
      // Add a mock cluster for demo
      const newCluster: Cluster = {
        id: `cluster-${Date.now()}`,
        name: data.name ?? 'New Cluster',
        description: data.description ?? '',
        status: 'unknown',
        provider: data.provider ?? 'other',
        region: data.region ?? 'unknown',
        version: '1.28.0',
        nodeCount: 0,
        namespaceCount: 0,
        namespaces: ['default', 'kube-system'],
        labels: {},
        lastHealthCheck: new Date().toISOString(),
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };
      setClusters((prev) => [newCluster, ...prev]);
    }
  }, []);

  const handleClearFilters = useCallback(() => {
    setSearchQuery('');
    setStatusFilter('all');
    setProviderFilter('all');
  }, []);

  const hasActiveFilters =
    searchQuery || statusFilter !== 'all' || providerFilter !== 'all';

  // Active provider options from current clusters
  const activeProviders = useMemo(() => {
    const providers = new Set(clusters.map((c) => c.provider));
    return Array.from(providers);
  }, [clusters]);

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
          <Stack direction="row" spacing={1} alignItems="center" mb={0.5}>
            <Typography variant="h4" fontWeight={700}>
              Clusters
            </Typography>
            <Tooltip title={CLUSTER_PAGE_HELP} arrow placement="right">
              <IconButton
                size="small"
                aria-label="What are clusters?"
                sx={{ color: 'text.secondary' }}
              >
                <InfoIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Stack>
          <Typography variant="body2" color="text.secondary">
            Groups of servers where your apps run. This page shows whether each one is
            healthy, degraded, or unreachable.
          </Typography>
        </Box>

        <Stack direction="row" spacing={1}>
          <Tooltip title="Reload the latest cluster status and metrics." arrow>
            <span>
              <Button
                variant="outlined"
                startIcon={<RefreshIcon />}
                onClick={fetchClusters}
                disabled={isLoading}
                sx={{ textTransform: 'none', fontWeight: 600 }}
              >
                Refresh
              </Button>
            </span>
          </Tooltip>
          <Tooltip title="Add a new cluster so it can be tracked and tested here." arrow>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => setRegisterDialogOpen(true)}
              sx={{ textTransform: 'none', fontWeight: 600 }}
            >
              Register Cluster
            </Button>
          </Tooltip>
        </Stack>
      </Stack>

      {/* Stats Summary */}
      <Grid container spacing={1.5} mb={3}>
        <Grid item xs={6} sm={3}>
          <StatCard
            label="Total Clusters"
            value={stats.total}
            color="#2563EB"
            icon={<ClusterIcon sx={{ fontSize: 18, color: '#2563EB' }} />}
            tooltip={CLUSTER_STAT_HELP.total}
          />
        </Grid>
        <Grid item xs={6} sm={3}>
          <StatCard
            label="Healthy"
            value={stats.healthy}
            color="#10B981"
            icon={<HealthyIcon sx={{ fontSize: 18, color: '#10B981' }} />}
            tooltip={CLUSTER_STAT_HELP.healthy}
          />
        </Grid>
        <Grid item xs={6} sm={3}>
          <StatCard
            label="Degraded"
            value={stats.degraded}
            color="#F59E0B"
            icon={<DegradedIcon sx={{ fontSize: 18, color: '#F59E0B' }} />}
            tooltip={CLUSTER_STAT_HELP.degraded}
          />
        </Grid>
        <Grid item xs={6} sm={3}>
          <StatCard
            label="Unreachable"
            value={stats.unreachable}
            color="#EF4444"
            icon={<UnreachableIcon sx={{ fontSize: 18, color: '#EF4444' }} />}
            tooltip={CLUSTER_STAT_HELP.unreachable}
          />
        </Grid>
      </Grid>

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
          mb: 3,
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
            placeholder="Search clusters by name, description, or region..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                </InputAdornment>
              ),
              endAdornment: searchQuery ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={() => setSearchQuery('')}>
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

          {/* Status Filter */}
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <InputLabel id="cluster-status-filter-label">Status</InputLabel>
            <Select
              labelId="cluster-status-filter-label"
              value={statusFilter}
              label="Status"
              onChange={(e) => setStatusFilter(e.target.value as ClusterStatus | 'all')}
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

          {/* Provider Filter */}
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel id="cluster-provider-filter-label">Provider</InputLabel>
            <Select
              labelId="cluster-provider-filter-label"
              value={providerFilter}
              label="Provider"
              onChange={(e) => setProviderFilter(e.target.value)}
              sx={{ borderRadius: 1.5 }}
            >
              <MenuItem value="all">All Providers</MenuItem>
              {activeProviders.map((provider) => {
                const config = PROVIDER_CONFIG[provider] ?? PROVIDER_CONFIG['other'];
                return (
                  <MenuItem key={provider} value={provider}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Box
                        sx={{
                          width: 10,
                          height: 10,
                          borderRadius: '50%',
                          backgroundColor: config.color,
                        }}
                      />
                      <span>{config.label}</span>
                    </Stack>
                  </MenuItem>
                );
              })}
            </Select>
          </FormControl>

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
      </Paper>

      {/* Active Filter Chips */}
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
              onDelete={() => setSearchQuery('')}
              sx={{ height: 26 }}
            />
          )}
          {statusFilter !== 'all' && (
            <Chip
              label={`Status: ${statusFilter}`}
              size="small"
              onDelete={() => setStatusFilter('all')}
              sx={{ height: 26 }}
            />
          )}
          {providerFilter !== 'all' && (
            <Chip
              label={`Provider: ${PROVIDER_CONFIG[providerFilter]?.label ?? providerFilter}`}
              size="small"
              onDelete={() => setProviderFilter('all')}
              sx={{ height: 26 }}
            />
          )}
        </Stack>
      )}

      {/* Clusters Grid */}
      {isLoading && clusters.length === 0 ? (
        <Grid container spacing={2.5}>
          {Array.from({ length: 6 }).map((_, idx) => (
            <Grid item xs={12} sm={6} lg={4} key={idx}>
              <ClusterCard
                cluster={{
                  id: `skeleton-${idx}`,
                  name: '',
                  description: '',
                  status: 'unknown' as ClusterStatus,
                  provider: 'other',
                  region: '',
                  version: '',
                  nodeCount: 0,
                  namespaceCount: 0,
                  namespaces: [],
                  labels: {},
                  lastHealthCheck: '',
                  createdAt: '',
                  updatedAt: '',
                }}
                onHealthCheck={() => {}}
                onDelete={() => {}}
                loading
              />
            </Grid>
          ))}
        </Grid>
      ) : error && clusters.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            border: '1px solid',
            borderColor: 'error.light',
            borderRadius: 2,
            py: 8,
            px: 3,
            textAlign: 'center',
          }}
        >
          <ErrorIcon sx={{ fontSize: 64, color: 'error.main', mb: 2 }} />
          <Typography variant="h5" color="error.main" gutterBottom fontWeight={600}>
            Failed to load clusters
          </Typography>
          <Typography
            variant="body1"
            color="text.secondary"
            sx={{ mb: 3, maxWidth: 480, mx: 'auto' }}
          >
            {error}
          </Typography>
          <Button
            variant="contained"
            startIcon={<RefreshIcon />}
            onClick={fetchClusters}
            sx={{ textTransform: 'none' }}
          >
            Retry
          </Button>
        </Paper>
      ) : clusters.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 2,
            py: 8,
            px: 3,
            textAlign: 'center',
          }}
        >
          <ClusterIcon sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }} />
          <Typography variant="h5" color="text.secondary" gutterBottom fontWeight={600}>
            No clusters registered yet
          </Typography>
          <Typography
            variant="body1"
            color="text.disabled"
            sx={{ mb: 3, maxWidth: 420, mx: 'auto' }}
          >
            Register a cluster to get started.
          </Typography>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setRegisterDialogOpen(true)}
            sx={{ textTransform: 'none' }}
          >
            Register Cluster
          </Button>
        </Paper>
      ) : filteredClusters.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            border: '1px solid',
            borderColor: 'divider',
            borderRadius: 2,
            py: 8,
            px: 3,
            textAlign: 'center',
          }}
        >
          <ClusterIcon sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }} />
          <Typography variant="h5" color="text.secondary" gutterBottom fontWeight={600}>
            No clusters found
          </Typography>
          <Typography
            variant="body1"
            color="text.disabled"
            sx={{ mb: 3, maxWidth: 420, mx: 'auto' }}
          >
            Try adjusting your search or filter criteria.
          </Typography>
          <Button
            variant="outlined"
            startIcon={<ClearIcon />}
            onClick={handleClearFilters}
            sx={{ textTransform: 'none' }}
          >
            Clear Filters
          </Button>
        </Paper>
      ) : (
        <Grid container spacing={2.5}>
          {filteredClusters.map((cluster) => (
            <Grid item xs={12} sm={6} lg={4} key={cluster.id}>
              <ClusterCard
                cluster={cluster}
                health={healthData[cluster.id]}
                onHealthCheck={handleHealthCheck}
                onDelete={handleDeleteCluster}
              />
            </Grid>
          ))}
        </Grid>
      )}

      {/* Results count */}
      {!isLoading && filteredClusters.length > 0 && (
        <Box sx={{ mt: 2, textAlign: 'center' }}>
          <Typography variant="caption" color="text.disabled">
            Showing {filteredClusters.length} of {clusters.length} cluster
            {clusters.length !== 1 ? 's' : ''}
          </Typography>
        </Box>
      )}

      {/* Register Cluster Dialog */}
      <RegisterClusterDialog
        open={registerDialogOpen}
        onClose={() => setRegisterDialogOpen(false)}
        onRegister={handleRegisterCluster}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => {
          setDeleteDialogOpen(false);
          setClusterToDelete(null);
        }}
        maxWidth="xs"
        fullWidth
        PaperProps={{ sx: { borderRadius: 3 } }}
      >
        <DialogTitle sx={{ fontWeight: 700 }}>Remove Cluster</DialogTitle>
        <DialogContent>
          <Typography variant="body1" color="text.secondary">
            Are you sure you want to remove{' '}
            <Typography component="span" fontWeight={600} color="text.primary">
              {clusters.find((c) => c.id === clusterToDelete)?.name ?? 'this cluster'}
            </Typography>
            ? This will disconnect it from Chaos-Sec. Any running experiments on this
            cluster will be stopped.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button
            onClick={() => {
              setDeleteDialogOpen(false);
              setClusterToDelete(null);
            }}
          >
            Cancel
          </Button>
          <Button variant="contained" color="error" onClick={confirmDelete}>
            Remove Cluster
          </Button>
        </DialogActions>
      </Dialog>

      {/* Health Check Error Snackbar */}
      <Snackbar
        open={!!healthCheckError}
        autoHideDuration={4000}
        onClose={() => setHealthCheckError(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          severity="error"
          onClose={() => setHealthCheckError(null)}
          sx={{ borderRadius: 2 }}
        >
          {healthCheckError}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default ClusterListPage;
