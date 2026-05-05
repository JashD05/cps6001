import {
  Dashboard as DashboardIcon,
  Science as ExperimentIcon,
  Dns as ClusterIcon,
  Description as TemplateIcon,
  Assessment as ReportIcon,
  Settings as SettingsIcon,
  Notifications as NotificationsIcon,
  NotificationsActive as NotificationsActiveIcon,
  ExitToApp as LogoutIcon,
  Person as ProfileIcon,
  Menu as MenuIcon,
  ChevronLeft as ChevronLeftIcon,
  Add as AddIcon,
  Speed as SpeedIcon,
  Security as SecurityIcon,
  Wifi as WifiIcon,
  WifiOff as WifiOffIcon,
  FiberManualRecord as DotIcon,
  HelpOutline as HelpIcon,
  Search as SearchIcon,
} from '@mui/icons-material';
import {
  Box,
  AppBar,
  Toolbar,
  Typography,
  IconButton,
  Badge,
  Avatar,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Divider,
  Drawer,
  List,
  ListItemButton,
  ListItemIcon as SidebarListItemIcon,
  ListItemText as SidebarListItemText,
  Chip,
  Tooltip,
  useMediaQuery,
  useTheme,
  Button,
  Popover,
  LinearProgress,
  Stack,
  Paper,
  Dialog,
  DialogContent,
  InputBase,
} from '@mui/material';
import { useState, useEffect, useRef } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useNavigate, useLocation, Outlet } from 'react-router-dom';
import { logout, selectCurrentUser } from '@/store/authSlice';
import { fetchExperiments, selectExperimentList } from '@/store/experimentSlice';
import type { AppDispatch } from '@/store';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const SIDEBAR_WIDTH = 260;
const SIDEBAR_WIDTH_COLLAPSED = 64;
const APP_BAR_HEIGHT = 64;
const STATUS_BAR_HEIGHT = 32;

const NAV_ITEMS = [
  { label: 'Dashboard', path: '/', icon: <DashboardIcon /> },
  { label: 'Experiments', path: '/experiments', icon: <ExperimentIcon /> },
  { label: 'Clusters', path: '/clusters', icon: <ClusterIcon /> },
  { label: 'Templates', path: '/templates', icon: <TemplateIcon /> },
  { label: 'Reports', path: '/reports', icon: <ReportIcon /> },
  { label: 'Settings', path: '/settings', icon: <SettingsIcon /> },
];

// ---------------------------------------------------------------------------
// Notification data (mock — would come from a real-time source in production)
// ---------------------------------------------------------------------------

interface Notification {
  id: string;
  title: string;
  message: string;
  timestamp: string;
  read: boolean;
  severity: 'info' | 'warning' | 'error' | 'success';
}

const MOCK_NOTIFICATIONS: Notification[] = [
  {
    id: '1',
    title: 'Experiment Completed',
    message: 'DNS Exfiltration Test completed successfully.',
    timestamp: '2 min ago',
    read: false,
    severity: 'success',
  },
  {
    id: '2',
    title: 'Cluster Degraded',
    message: 'Cluster prod-us-east-1 is in a degraded state.',
    timestamp: '15 min ago',
    read: false,
    severity: 'warning',
  },
  {
    id: '3',
    title: 'SIEM Alert Missed',
    message: 'Brute Force experiment did not trigger an expected SIEM alert.',
    timestamp: '1 hr ago',
    read: true,
    severity: 'error',
  },
  {
    id: '4',
    title: 'Report Ready',
    message: 'Weekly compliance report is ready for download.',
    timestamp: '3 hr ago',
    read: true,
    severity: 'info',
  },
];

// ---------------------------------------------------------------------------
// Cluster health data (mock)
// ---------------------------------------------------------------------------

interface ClusterHealthInfo {
  name: string;
  status: 'healthy' | 'degraded' | 'unreachable' | 'unknown';
  cpuUsage: number;
  memoryUsage: number;
}

const MOCK_CLUSTER_HEALTH: ClusterHealthInfo[] = [
  { name: 'prod-us-east-1', status: 'healthy', cpuUsage: 45, memoryUsage: 62 },
  { name: 'staging-eu-west-1', status: 'healthy', cpuUsage: 22, memoryUsage: 38 },
  { name: 'dev-local', status: 'degraded', cpuUsage: 78, memoryUsage: 85 },
];

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatusBar() {
  const theme = useTheme();
  const [systemOnline] = useState(true);
  const healthyCount = MOCK_CLUSTER_HEALTH.filter((c) => c.status === 'healthy').length;
  const totalClusters = MOCK_CLUSTER_HEALTH.length;

  return (
    <Box
      sx={{
        height: STATUS_BAR_HEIGHT,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        px: 2,
        py: 0,
        borderTop: '1px solid',
        borderColor: theme.palette.divider,
        backgroundColor:
          theme.palette.mode === 'dark'
            ? 'rgba(15, 23, 42, 0.9)'
            : 'rgba(248, 250, 252, 0.95)',
        backdropFilter: 'blur(8px)',
        fontSize: '0.6875rem',
        color: theme.palette.text.secondary,
        userSelect: 'none',
      }}
    >
      <Stack direction="row" spacing={2} alignItems="center">
        <Stack direction="row" spacing={0.5} alignItems="center">
          {systemOnline ? (
            <WifiIcon sx={{ fontSize: 14, color: 'success.main' }} />
          ) : (
            <WifiOffIcon sx={{ fontSize: 14, color: 'error.main' }} />
          )}
          <Typography variant="caption" sx={{ fontSize: '0.6875rem' }}>
            {systemOnline ? 'Connected' : 'Disconnected'}
          </Typography>
        </Stack>

        <Divider orientation="vertical" flexItem sx={{ height: 14, mx: 0.5 }} />

        <Stack direction="row" spacing={0.5} alignItems="center">
          <SecurityIcon sx={{ fontSize: 14, color: 'primary.main' }} />
          <Typography variant="caption" sx={{ fontSize: '0.6875rem' }}>
            Clusters: {healthyCount}/{totalClusters} healthy
          </Typography>
        </Stack>

        <Divider orientation="vertical" flexItem sx={{ height: 14, mx: 0.5 }} />

        <Stack direction="row" spacing={0.5} alignItems="center">
          <SpeedIcon sx={{ fontSize: 14, color: 'warning.main' }} />
          <Typography variant="caption" sx={{ fontSize: '0.6875rem' }}>
            API: 42ms avg
          </Typography>
        </Stack>
      </Stack>

      <Stack direction="row" spacing={2} alignItems="center">
        <Typography variant="caption" sx={{ fontSize: '0.6875rem' }}>
          v1.0.0
        </Typography>
        <DotIcon sx={{ fontSize: 6, color: 'success.main' }} />
      </Stack>
    </Box>
  );
}

function NotificationItem({ notification }: { notification: Notification }) {
  const theme = useTheme();

  const severityColor = {
    info: theme.palette.info.main,
    warning: theme.palette.warning.main,
    error: theme.palette.error.main,
    success: theme.palette.success.main,
  }[notification.severity];

  return (
    <Box
      sx={{
        px: 2,
        py: 1.5,
        cursor: 'pointer',
        transition: 'background-color 150ms',
        '&:hover': {
          backgroundColor: theme.palette.action.hover,
        },
        borderLeft: notification.read ? 'none' : `3px solid ${severityColor}`,
      }}
    >
      <Stack direction="row" spacing={1} alignItems="flex-start">
        {!notification.read && (
          <DotIcon sx={{ fontSize: 10, color: severityColor, mt: 0.5, flexShrink: 0 }} />
        )}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography
            variant="subtitle2"
            sx={{
              fontSize: '0.8125rem',
              fontWeight: notification.read ? 400 : 600,
              lineHeight: 1.3,
            }}
          >
            {notification.title}
          </Typography>
          <Typography
            variant="caption"
            sx={{
              display: 'block',
              color: theme.palette.text.secondary,
              mt: 0.25,
              lineHeight: 1.4,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {notification.message}
          </Typography>
          <Typography
            variant="caption"
            sx={{
              display: 'block',
              mt: 0.5,
              fontSize: '0.6875rem',
              color: theme.palette.text.disabled,
            }}
          >
            {notification.timestamp}
          </Typography>
        </Box>
      </Stack>
    </Box>
  );
}

// ---------------------------------------------------------------------------
// Main Layout Component
// ---------------------------------------------------------------------------

export default function Layout() {
  const theme = useTheme();
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch<AppDispatch>();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const currentUser = useSelector(selectCurrentUser);

  const [sidebarOpen, setSidebarOpen] = useState(!isMobile);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [userMenuAnchor, setUserMenuAnchor] = useState<HTMLElement | null>(null);
  const [notifAnchor, setNotifAnchor] = useState<HTMLElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const experiments = useSelector(selectExperimentList);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const unreadCount = MOCK_NOTIFICATIONS.filter((n) => !n.read).length;
  const currentPath = location.pathname;

  // Keyboard shortcut: Cmd+K / Ctrl+K to open search, Escape to close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (e.key === 'Escape' && searchOpen) {
        setSearchOpen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [searchOpen]);

  // Debounced search effect
  useEffect(() => {
    if (!searchQuery.trim()) return;
    const timer = setTimeout(() => {
      dispatch(fetchExperiments({ search: searchQuery, limit: 10 }));
    }, 300);
    return () => clearTimeout(timer);
  }, [searchQuery, dispatch]);

  const handleNavigate = (path: string) => {
    navigate(path);
    if (isMobile) {
      setSidebarOpen(false);
    }
  };

  const handleLogout = async () => {
    setUserMenuAnchor(null);
    await dispatch(logout());
  };

  const isActivePath = (path: string) => {
    if (path === '/') return currentPath === '/';
    return currentPath.startsWith(path);
  };

  const sidebarWidth = sidebarCollapsed ? SIDEBAR_WIDTH_COLLAPSED : SIDEBAR_WIDTH;

  const sidebarContent = (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor:
          theme.palette.mode === 'dark' ? theme.palette.background.paper : '#FFFFFF',
      }}
    >
      {/* Sidebar Header */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: sidebarCollapsed ? 'center' : 'space-between',
          px: sidebarCollapsed ? 0 : 2,
          py: 1.5,
          minHeight: APP_BAR_HEIGHT,
          borderBottom: '1px solid',
          borderColor: theme.palette.divider,
        }}
      >
        {!sidebarCollapsed && (
          <Stack direction="row" spacing={1} alignItems="center">
            <SecurityIcon sx={{ fontSize: 28, color: 'primary.main' }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 800,
                fontSize: '1.125rem',
                background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                WebkitBackgroundClip: 'text',
                WebkitTextFillColor: 'transparent',
              }}
            >
              Chaos-Sec
            </Typography>
          </Stack>
        )}
        {!isMobile && (
          <IconButton
            size="small"
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            sx={{
              transition: 'transform 200ms',
              transform: sidebarCollapsed ? 'rotate(180deg)' : 'none',
            }}
          >
            <ChevronLeftIcon fontSize="small" />
          </IconButton>
        )}
      </Box>

      {/* Quick Actions */}
      {!sidebarCollapsed && (
        <Box sx={{ px: 2, pt: 2, pb: 1 }}>
          <Button
            variant="contained"
            fullWidth
            startIcon={<AddIcon />}
            onClick={() => handleNavigate('/experiments/new')}
            sx={{
              py: 1,
              borderRadius: 2,
              fontWeight: 600,
              textTransform: 'none',
            }}
          >
            New Experiment
          </Button>
        </Box>
      )}
      {sidebarCollapsed && (
        <Box sx={{ display: 'flex', justifyContent: 'center', pt: 2, pb: 1 }}>
          <Tooltip title="New Experiment" placement="right">
            <IconButton
              color="primary"
              onClick={() => handleNavigate('/experiments/new')}
              sx={{
                backgroundColor: 'primary.main',
                color: '#fff',
                '&:hover': { backgroundColor: 'primary.dark' },
                width: 40,
                height: 40,
              }}
            >
              <AddIcon />
            </IconButton>
          </Tooltip>
        </Box>
      )}

      {/* Navigation Links */}
      <List sx={{ flex: 1, px: sidebarCollapsed ? 0.5 : 1, pt: 1 }}>
        {NAV_ITEMS.map((item) => {
          const isActive = isActivePath(item.path);
          return (
            <Tooltip
              key={item.path}
              title={sidebarCollapsed ? item.label : ''}
              placement="right"
              arrow
            >
              <ListItemButton
                selected={isActive}
                onClick={() => handleNavigate(item.path)}
                sx={{
                  borderRadius: 1.5,
                  mb: 0.5,
                  justifyContent: sidebarCollapsed ? 'center' : 'flex-start',
                  px: sidebarCollapsed ? 0 : 1.5,
                  py: 1,
                  '&.Mui-selected': {
                    backgroundColor: `${theme.palette.primary.main}0D`,
                    '&:hover': {
                      backgroundColor: `${theme.palette.primary.main}14`,
                    },
                  },
                }}
              >
                <SidebarListItemIcon
                  sx={{
                    minWidth: sidebarCollapsed ? 0 : 36,
                    color: isActive ? 'primary.main' : 'text.secondary',
                    justifyContent: 'center',
                  }}
                >
                  {item.icon}
                </SidebarListItemIcon>
                {!sidebarCollapsed && (
                  <SidebarListItemText
                    primary={item.label}
                    sx={{
                      '& .MuiListItemText-primary': {
                        fontWeight: isActive ? 600 : 500,
                        color: isActive ? 'primary.main' : 'text.primary',
                        fontSize: '0.875rem',
                      },
                    }}
                  />
                )}
              </ListItemButton>
            </Tooltip>
          );
        })}
      </List>

      {/* Cluster Health Section */}
      {!sidebarCollapsed && (
        <Box sx={{ px: 2, pb: 2 }}>
          <Typography variant="overline" sx={{ px: 1, mb: 1, display: 'block' }}>
            Cluster Health
          </Typography>
          <Stack spacing={1}>
            {MOCK_CLUSTER_HEALTH.map((cluster) => {
              const statusColor =
                cluster.status === 'healthy'
                  ? 'success'
                  : cluster.status === 'degraded'
                    ? 'warning'
                    : 'error';
              return (
                <Paper
                  key={cluster.name}
                  variant="outlined"
                  sx={{
                    p: 1.25,
                    borderRadius: 1.5,
                    cursor: 'pointer',
                    transition: 'all 150ms',
                    '&:hover': {
                      borderColor: 'primary.main',
                      backgroundColor: theme.palette.action.hover,
                    },
                  }}
                  onClick={() => handleNavigate('/clusters')}
                >
                  <Stack
                    direction="row"
                    justifyContent="space-between"
                    alignItems="center"
                    sx={{ mb: 0.75 }}
                  >
                    <Typography
                      variant="caption"
                      sx={{ fontWeight: 600, fontSize: '0.75rem' }}
                    >
                      {cluster.name}
                    </Typography>
                    <Chip
                      label={cluster.status}
                      size="small"
                      color={statusColor as 'success' | 'warning' | 'error'}
                      sx={{ height: 20, fontSize: '0.625rem' }}
                    />
                  </Stack>
                  <Stack direction="row" spacing={1.5}>
                    <Box sx={{ flex: 1 }}>
                      <Typography
                        variant="caption"
                        sx={{
                          fontSize: '0.625rem',
                          color: 'text.secondary',
                          mb: 0.25,
                          display: 'block',
                        }}
                      >
                        CPU
                      </Typography>
                      <LinearProgress
                        variant="determinate"
                        value={cluster.cpuUsage}
                        sx={{
                          height: 4,
                          borderRadius: 2,
                          backgroundColor: theme.palette.divider,
                          '& .MuiLinearProgress-bar': {
                            backgroundColor:
                              cluster.cpuUsage > 70
                                ? theme.palette.warning.main
                                : theme.palette.success.main,
                            borderRadius: 2,
                          },
                        }}
                      />
                    </Box>
                    <Box sx={{ flex: 1 }}>
                      <Typography
                        variant="caption"
                        sx={{
                          fontSize: '0.625rem',
                          color: 'text.secondary',
                          mb: 0.25,
                          display: 'block',
                        }}
                      >
                        MEM
                      </Typography>
                      <LinearProgress
                        variant="determinate"
                        value={cluster.memoryUsage}
                        sx={{
                          height: 4,
                          borderRadius: 2,
                          backgroundColor: theme.palette.divider,
                          '& .MuiLinearProgress-bar': {
                            backgroundColor:
                              cluster.memoryUsage > 80
                                ? theme.palette.error.main
                                : theme.palette.success.main,
                            borderRadius: 2,
                          },
                        }}
                      />
                    </Box>
                  </Stack>
                </Paper>
              );
            })}
          </Stack>
        </Box>
      )}

      {/* Collapsed cluster indicator */}
      {sidebarCollapsed && (
        <Box sx={{ px: 1, pb: 2 }}>
          <Tooltip title="View Clusters" placement="right">
            <IconButton
              size="small"
              onClick={() => handleNavigate('/clusters')}
              sx={{ width: '100%' }}
            >
              <ClusterIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
            </IconButton>
          </Tooltip>
        </Box>
      )}
    </Box>
  );

  return (
    <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      {/* Sidebar */}
      {isMobile ? (
        <Drawer
          variant="temporary"
          open={sidebarOpen}
          onClose={() => setSidebarOpen(false)}
          ModalProps={{ keepMounted: true }}
          sx={{
            '& .MuiDrawer-paper': {
              width: SIDEBAR_WIDTH,
              boxSizing: 'border-box',
            },
          }}
        >
          {sidebarContent}
        </Drawer>
      ) : (
        <Box
          component="nav"
          sx={{
            width: sidebarWidth,
            flexShrink: 0,
            transition: theme.transitions.create('width', {
              easing: theme.transitions.easing.sharp,
              duration: theme.transitions.duration.enteringScreen,
            }),
          }}
        >
          <Box
            sx={{
              width: sidebarWidth,
              height: '100%',
              position: 'fixed',
              top: 0,
              left: 0,
              borderRight: '1px solid',
              borderColor: theme.palette.divider,
              overflowY: 'auto',
              overflowX: 'hidden',
              zIndex: theme.zIndex.drawer,
            }}
          >
            {sidebarContent}
          </Box>
        </Box>
      )}

      {/* Main Content Area */}
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          height: '100vh',
          overflow: 'hidden',
          transition: theme.transitions.create('margin', {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.leavingScreen,
          }),
        }}
      >
        {/* Top Navigation Bar */}
        <AppBar
          position="sticky"
          elevation={0}
          sx={{
            backgroundColor:
              theme.palette.mode === 'dark'
                ? 'rgba(30, 41, 59, 0.85)'
                : 'rgba(255, 255, 255, 0.85)',
            backdropFilter: 'blur(12px)',
            borderBottom: '1px solid',
            borderColor: theme.palette.divider,
            color: theme.palette.text.primary,
            zIndex: theme.zIndex.appBar,
          }}
        >
          <Toolbar sx={{ minHeight: `${APP_BAR_HEIGHT}px !important`, gap: 1 }}>
            {/* Mobile hamburger */}
            {isMobile && (
              <IconButton
                edge="start"
                onClick={() => setSidebarOpen(true)}
                sx={{ mr: 1 }}
              >
                <MenuIcon />
              </IconButton>
            )}

            {/* Breadcrumb / Page Title */}
            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', gap: 1 }}>
              <SecurityIcon
                sx={{
                  display: { xs: 'flex', md: 'none' },
                  fontSize: 24,
                  color: 'primary.main',
                }}
              />
              <Typography
                variant="h6"
                sx={{
                  display: { xs: 'flex', md: 'none' },
                  fontWeight: 800,
                  fontSize: '1rem',
                  background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                  WebkitBackgroundClip: 'text',
                  WebkitTextFillColor: 'transparent',
                }}
              >
                Chaos-Sec
              </Typography>

              {/* Breadcrumb for desktop */}
              <Typography
                variant="body2"
                sx={{
                  display: { xs: 'none', md: 'block' },
                  color: 'text.secondary',
                  fontWeight: 500,
                }}
              >
                {NAV_ITEMS.find((item) => isActivePath(item.path))?.label ?? 'Dashboard'}
              </Typography>
            </Box>

            {/* Global Search */}
            <Box
              onClick={() => setSearchOpen(true)}
              sx={{
                display: { xs: 'none', lg: 'flex' },
                alignItems: 'center',
                backgroundColor:
                  theme.palette.mode === 'dark'
                    ? 'rgba(255,255,255,0.05)'
                    : 'rgba(0,0,0,0.04)',
                borderRadius: 2,
                px: 2,
                py: 0.5,
                width: 280,
                cursor: 'pointer',
                transition: 'background-color 150ms',
                '&:hover': {
                  backgroundColor:
                    theme.palette.mode === 'dark'
                      ? 'rgba(255,255,255,0.08)'
                      : 'rgba(0,0,0,0.06)',
                },
              }}
            >
              <Typography
                variant="body2"
                sx={{
                  color: 'text.disabled',
                  fontSize: '0.8125rem',
                  flex: 1,
                }}
              >
                Search experiments, templates...
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  color: 'text.disabled',
                  border: '1px solid',
                  borderColor: 'divider',
                  borderRadius: 1,
                  px: 0.75,
                  py: 0.25,
                  fontSize: '0.6875rem',
                  fontFamily: 'monospace',
                }}
              >
                ⌘K
              </Typography>
            </Box>

            {/* Search Dialog */}
            <Dialog
              open={searchOpen}
              onClose={() => {
                setSearchOpen(false);
                setSearchQuery('');
              }}
              maxWidth="sm"
              fullWidth
              PaperProps={{
                sx: {
                  mt: '10vh',
                  borderRadius: 3,
                  maxHeight: '60vh',
                },
              }}
            >
              <DialogContent sx={{ p: 0 }}>
                <Box sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider' }}>
                  <Stack direction="row" alignItems="center" spacing={1.5}>
                    <SearchIcon sx={{ color: 'text.secondary' }} />
                    <InputBase
                      inputRef={searchInputRef}
                      autoFocus
                      placeholder="Search experiments, templates..."
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      sx={{ flex: 1, fontSize: '1rem' }}
                    />
                  </Stack>
                </Box>

                {searchQuery.trim() ? (
                  experiments.length > 0 ? (
                    <Box sx={{ maxHeight: 400, overflowY: 'auto' }}>
                      {experiments.map((exp) => (
                        <Box
                          key={exp.id}
                          onClick={() => {
                            navigate(`/experiments/${exp.id}`);
                            setSearchOpen(false);
                            setSearchQuery('');
                          }}
                          sx={{
                            px: 2,
                            py: 1.5,
                            cursor: 'pointer',
                            '&:hover': {
                              backgroundColor:
                                theme.palette.mode === 'dark'
                                  ? 'rgba(255,255,255,0.05)'
                                  : 'rgba(0,0,0,0.04)',
                            },
                            borderBottom: '1px solid',
                            borderColor: 'divider',
                          }}
                        >
                          <Typography variant="body2" fontWeight={600}>
                            {exp.name}
                          </Typography>
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                            mt={0.25}
                          >
                            <Chip
                              label={exp.status}
                              size="small"
                              variant="outlined"
                              sx={{ fontSize: '0.6875rem', height: 20 }}
                            />
                            {exp.templateName && (
                              <Typography variant="caption" color="text.secondary">
                                {exp.templateName}
                              </Typography>
                            )}
                          </Stack>
                        </Box>
                      ))}
                    </Box>
                  ) : (
                    <Box sx={{ py: 4, textAlign: 'center' }}>
                      <Typography variant="body2" color="text.secondary">
                        No experiments found matching &quot;{searchQuery}&quot;
                      </Typography>
                    </Box>
                  )
                ) : (
                  <Box sx={{ py: 4, textAlign: 'center' }}>
                    <Typography variant="body2" color="text.secondary">
                      Start typing to search...
                    </Typography>
                  </Box>
                )}
              </DialogContent>
            </Dialog>

            <Box sx={{ flex: 0, display: 'flex', alignItems: 'center', gap: 0.5 }}>
              {/* Notifications */}
              <Tooltip title="Notifications">
                <IconButton
                  onClick={(e) => setNotifAnchor(e.currentTarget)}
                  sx={{ ml: 0.5 }}
                >
                  <Badge
                    badgeContent={unreadCount}
                    color="error"
                    max={9}
                    sx={{
                      '& .MuiBadge-badge': {
                        fontSize: '0.625rem',
                        minWidth: 18,
                        height: 18,
                      },
                    }}
                  >
                    {unreadCount > 0 ? (
                      <NotificationsActiveIcon
                        sx={{ fontSize: 22, color: 'warning.main' }}
                      />
                    ) : (
                      <NotificationsIcon sx={{ fontSize: 22 }} />
                    )}
                  </Badge>
                </IconButton>
              </Tooltip>

              {/* Notification Popover */}
              <Popover
                open={Boolean(notifAnchor)}
                anchorEl={notifAnchor}
                onClose={() => setNotifAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                slotProps={{
                  paper: {
                    sx: {
                      width: 380,
                      maxHeight: 480,
                      borderRadius: 2,
                      boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
                      overflow: 'hidden',
                    },
                  },
                }}
              >
                <Box
                  sx={{
                    px: 2,
                    py: 1.5,
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                    Notifications
                  </Typography>
                  {unreadCount > 0 && (
                    <Chip
                      label={`${unreadCount} new`}
                      size="small"
                      color="primary"
                      sx={{ height: 22, fontSize: '0.6875rem' }}
                    />
                  )}
                </Box>
                <Box sx={{ maxHeight: 360, overflowY: 'auto' }}>
                  {MOCK_NOTIFICATIONS.length === 0 ? (
                    <Box sx={{ py: 4, textAlign: 'center' }}>
                      <NotificationsIcon
                        sx={{ fontSize: 40, color: 'text.disabled', mb: 1 }}
                      />
                      <Typography variant="body2" color="text.secondary">
                        No notifications yet
                      </Typography>
                    </Box>
                  ) : (
                    MOCK_NOTIFICATIONS.map((notification, idx) => (
                      <Box key={notification.id}>
                        <NotificationItem notification={notification} />
                        {idx < MOCK_NOTIFICATIONS.length - 1 && (
                          <Divider sx={{ mx: 2 }} />
                        )}
                      </Box>
                    ))
                  )}
                </Box>
                <Box
                  sx={{
                    p: 1.5,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    textAlign: 'center',
                  }}
                >
                  <Button
                    size="small"
                    sx={{ textTransform: 'none', fontWeight: 600 }}
                    onClick={() => {
                      setNotifAnchor(null);
                      navigate('/settings');
                    }}
                  >
                    View All Notifications
                  </Button>
                </Box>
              </Popover>

              {/* Help */}
              <Tooltip title="Help & Documentation">
                <IconButton>
                  <HelpIcon sx={{ fontSize: 22 }} />
                </IconButton>
              </Tooltip>

              {/* User Menu */}
              <Tooltip title="Account">
                <IconButton
                  onClick={(e) => setUserMenuAnchor(e.currentTarget)}
                  sx={{ ml: 0.5 }}
                >
                  {currentUser?.avatarUrl ? (
                    <Avatar src={currentUser.avatarUrl} sx={{ width: 32, height: 32 }} />
                  ) : (
                    <Avatar
                      sx={{
                        width: 32,
                        height: 32,
                        bgcolor: 'primary.main',
                        fontSize: '0.875rem',
                        fontWeight: 600,
                      }}
                    >
                      {currentUser?.name
                        ?.split(' ')
                        .map((n) => n[0])
                        .join('')
                        .substring(0, 2) ?? 'CS'}
                    </Avatar>
                  )}
                </IconButton>
              </Tooltip>

              {/* User Menu Dropdown */}
              <Menu
                anchorEl={userMenuAnchor}
                open={Boolean(userMenuAnchor)}
                onClose={() => setUserMenuAnchor(null)}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
                slotProps={{
                  paper: {
                    sx: {
                      width: 240,
                      borderRadius: 2,
                      boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
                      mt: 1,
                    },
                  },
                }}
              >
                {/* User Info Header */}
                <Box sx={{ px: 2, py: 1.5 }}>
                  <Typography
                    variant="subtitle2"
                    sx={{ fontWeight: 600, lineHeight: 1.3 }}
                  >
                    {currentUser?.name ?? 'User'}
                  </Typography>
                  <Typography
                    variant="caption"
                    sx={{ color: 'text.secondary', display: 'block' }}
                  >
                    {currentUser?.email ?? 'user@chaos-sec.io'}
                  </Typography>
                  {currentUser?.role && (
                    <Chip
                      label={currentUser.role}
                      size="small"
                      color="primary"
                      variant="outlined"
                      sx={{
                        mt: 0.75,
                        height: 20,
                        fontSize: '0.625rem',
                        textTransform: 'capitalize',
                      }}
                    />
                  )}
                </Box>

                <Divider />

                <MenuItem
                  onClick={() => {
                    setUserMenuAnchor(null);
                    navigate('/settings');
                  }}
                >
                  <ListItemIcon>
                    <ProfileIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText>Profile & Settings</ListItemText>
                </MenuItem>

                <Divider />

                <MenuItem
                  onClick={handleLogout}
                  sx={{ color: 'error.main', '&:hover': { backgroundColor: 'error.50' } }}
                >
                  <ListItemIcon>
                    <LogoutIcon fontSize="small" sx={{ color: 'error.main' }} />
                  </ListItemIcon>
                  <ListItemText>Sign Out</ListItemText>
                </MenuItem>
              </Menu>
            </Box>
          </Toolbar>
        </AppBar>

        {/* Main Content */}
        <Box
          component="main"
          sx={{
            flex: 1,
            overflow: 'auto',
            backgroundColor:
              theme.palette.mode === 'dark'
                ? theme.palette.background.default
                : '#F8FAFC',
          }}
        >
          <Box
            sx={{
              p: { xs: 2, sm: 3 },
              maxWidth: 1440,
              mx: 'auto',
              width: '100%',
              minHeight: '100vh',
            }}
          >
            <Outlet />
          </Box>
        </Box>

        {/* Status Bar */}
        <StatusBar />
      </Box>
    </Box>
  );
}
