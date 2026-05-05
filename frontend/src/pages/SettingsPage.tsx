import {
  Person as ProfileIcon,
  Notifications as NotificationsIcon,
  Security as SIEMIcon,
  Science as ExperimentIcon,
  Save as SaveIcon,
  CheckCircle as SuccessIcon,
  Email as EmailIcon,
  Language as LanguageIcon,
  Palette as ThemeIcon,
  Timer as TimerIcon,
  Cloud as CloudIcon,
  Storage as NamespaceIcon,
  DeleteSweep as CleanupIcon,
  Description as LogIcon,
  HelpOutline as HelpIcon,
  Visibility as VisibilityIcon,
  VisibilityOff as VisibilityOffIcon,
  VpnKey as KeyIcon,
  Power as PowerIcon,
  Send as WebhookIcon,
} from '@mui/icons-material';
import {
  Box,
  Typography,
  Card,
  CardContent,
  TextField,
  Button,
  Switch,
  FormControlLabel,
  Grid,
  Stack,
  Divider,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Slider,
  Alert,
  Snackbar,
  Tabs,
  Tab,
  Avatar,
  Chip,
  IconButton,
  Tooltip,
  InputAdornment,
  Paper,
  Breadcrumbs,
  Link,
  useTheme,
  useMediaQuery,
  type SelectChangeEvent,
} from '@mui/material';
import React, { useState, useCallback, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAppSelector, useAppDispatch } from '@/store';
import { selectCurrentUser, updateUserProfile } from '@/store/authSlice';
import type { AppSettings, NotificationSettings, SIEMConfig } from '@/types';

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
      id={`settings-tabpanel-${index}`}
      aria-labelledby={`settings-tab-${index}`}
      style={{ display: value === index ? 'block' : 'none' }}
    >
      {value === index && <Box sx={{ pt: 3 }}>{children}</Box>}
    </div>
  );
};

// ---------------------------------------------------------------------------
// Default Settings
// ---------------------------------------------------------------------------

const DEFAULT_SETTINGS: AppSettings = {
  theme: 'light',
  language: 'en',
  notifications: {
    email: true,
    slack: false,
    webhook: false,
    emailAddress: '',
    slackWebhookUrl: '',
    webhookUrl: '',
    onExperimentComplete: true,
    onExperimentFailed: true,
    onClusterDegraded: true,
    onSIEMAlert: true,
  },
  siem: {
    provider: 'elastic',
    endpoint: '',
    apiKey: '',
    indexName: '',
    enabled: false,
  },
  defaultClusterId: null,
  defaultNamespace: 'default',
  autoRefreshInterval: 30,
  experimentDefaults: {
    defaultTimeWindow: 300,
    defaultNamespace: 'chaos-sec',
    autoCleanup: true,
    retainLogs: true,
  },
};

// ---------------------------------------------------------------------------
// Profile Settings Section
// ---------------------------------------------------------------------------

interface ProfileSettingsProps {
  onSaved: () => void;
}

const ProfileSettings: React.FC<ProfileSettingsProps> = ({ onSaved }) => {
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const _theme = useTheme();

  const [form, setForm] = useState({
    name: currentUser?.name ?? '',
    email: currentUser?.email ?? '',
  });
  const [passwordForm, setPasswordForm] = useState({
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
  });
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [profileSaving, setProfileSaving] = useState(false);
  const [passwordSaving, setPasswordSaving] = useState(false);

  const handleProfileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setForm((prev) => ({ ...prev, [name]: value }));
  }, []);

  const handlePasswordChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setPasswordForm((prev) => ({ ...prev, [name]: value }));
    setPasswordError(null);
  }, []);

  const handleProfileSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      setProfileSaving(true);
      try {
        await dispatch(updateUserProfile({ name: form.name, email: form.email }));
        onSaved();
      } catch {
        // Error handled by slice
      } finally {
        setProfileSaving(false);
      }
    },
    [dispatch, form, onSaved],
  );

  const handlePasswordSubmit = useCallback(
    (e: FormEvent) => {
      e.preventDefault();
      if (passwordForm.newPassword !== passwordForm.confirmPassword) {
        setPasswordError('Passwords do not match');
        return;
      }
      if (passwordForm.newPassword.length < 8) {
        setPasswordError('Password must be at least 8 characters');
        return;
      }
      setPasswordSaving(true);
      // In production this would call the API
      setTimeout(() => {
        setPasswordSaving(false);
        setPasswordForm({
          currentPassword: '',
          newPassword: '',
          confirmPassword: '',
        });
        onSaved();
      }, 1000);
    },
    [passwordForm, onSaved],
  );

  return (
    <Stack spacing={3}>
      {/* Profile Information */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={3}>
            <ProfileIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Profile Information
            </Typography>
          </Stack>

          <Box component="form" onSubmit={handleProfileSubmit}>
            <Grid container spacing={2.5}>
              <Grid item xs={12} sm={4}>
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    gap: 1.5,
                    pt: 1,
                  }}
                >
                  <Avatar
                    sx={{
                      width: 96,
                      height: 96,
                      bgcolor: 'primary.main',
                      fontSize: '2rem',
                      fontWeight: 700,
                    }}
                  >
                    {currentUser?.name
                      ?.split(' ')
                      .map((n) => n[0])
                      .join('')
                      .substring(0, 2) ?? 'CS'}
                  </Avatar>
                  <Button variant="outlined" size="small" sx={{ textTransform: 'none' }}>
                    Change Avatar
                  </Button>
                </Box>
              </Grid>

              <Grid item xs={12} sm={8}>
                <Stack spacing={2.5}>
                  <TextField
                    label="Full Name"
                    name="name"
                    value={form.name}
                    onChange={handleProfileChange}
                    fullWidth
                    InputProps={{
                      startAdornment: (
                        <InputAdornment position="start">
                          <ProfileIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                        </InputAdornment>
                      ),
                    }}
                  />

                  <TextField
                    label="Email Address"
                    name="email"
                    type="email"
                    value={form.email}
                    onChange={handleProfileChange}
                    fullWidth
                    InputProps={{
                      startAdornment: (
                        <InputAdornment position="start">
                          <EmailIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                        </InputAdornment>
                      ),
                    }}
                  />

                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="body2" color="text.secondary">
                      Role:
                    </Typography>
                    <Chip
                      label={currentUser?.role ?? 'viewer'}
                      size="small"
                      color="primary"
                      variant="outlined"
                      sx={{
                        textTransform: 'capitalize',
                        fontWeight: 600,
                        fontSize: '0.75rem',
                      }}
                    />
                  </Stack>

                  <Box>
                    <Button
                      type="submit"
                      variant="contained"
                      startIcon={<SaveIcon />}
                      disabled={profileSaving}
                      sx={{ textTransform: 'none', fontWeight: 600 }}
                    >
                      {profileSaving ? 'Saving...' : 'Save Profile'}
                    </Button>
                  </Box>
                </Stack>
              </Grid>
            </Grid>
          </Box>
        </CardContent>
      </Card>

      {/* Change Password */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={3}>
            <KeyIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Change Password
            </Typography>
          </Stack>

          {passwordError && (
            <Alert severity="error" sx={{ mb: 2, borderRadius: 2 }}>
              {passwordError}
            </Alert>
          )}

          <Box component="form" onSubmit={handlePasswordSubmit}>
            <Stack spacing={2.5} sx={{ maxWidth: 480 }}>
              <TextField
                label="Current Password"
                name="currentPassword"
                type={showCurrentPassword ? 'text' : 'password'}
                value={passwordForm.currentPassword}
                onChange={handlePasswordChange}
                fullWidth
                autoComplete="current-password"
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                        onMouseDown={(e) => e.preventDefault()}
                        edge="end"
                      >
                        {showCurrentPassword ? (
                          <VisibilityOffIcon sx={{ fontSize: 18 }} />
                        ) : (
                          <VisibilityIcon sx={{ fontSize: 18 }} />
                        )}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />

              <TextField
                label="New Password"
                name="newPassword"
                type={showNewPassword ? 'text' : 'password'}
                value={passwordForm.newPassword}
                onChange={handlePasswordChange}
                fullWidth
                autoComplete="new-password"
                helperText="Minimum 8 characters"
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        onClick={() => setShowNewPassword(!showNewPassword)}
                        onMouseDown={(e) => e.preventDefault()}
                        edge="end"
                      >
                        {showNewPassword ? (
                          <VisibilityOffIcon sx={{ fontSize: 18 }} />
                        ) : (
                          <VisibilityIcon sx={{ fontSize: 18 }} />
                        )}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />

              <TextField
                label="Confirm New Password"
                name="confirmPassword"
                type="password"
                value={passwordForm.confirmPassword}
                onChange={handlePasswordChange}
                fullWidth
                autoComplete="new-password"
                error={passwordError !== null}
              />

              <Box>
                <Button
                  type="submit"
                  variant="contained"
                  startIcon={<KeyIcon />}
                  disabled={passwordSaving}
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                >
                  {passwordSaving ? 'Updating...' : 'Update Password'}
                </Button>
              </Box>
            </Stack>
          </Box>
        </CardContent>
      </Card>
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Notification Settings Section
// ---------------------------------------------------------------------------

interface NotificationSettingsProps {
  settings: NotificationSettings;
  onChange: (settings: NotificationSettings) => void;
  onSaved: () => void;
}

const NotificationSettingsSection: React.FC<NotificationSettingsProps> = ({
  settings,
  onChange,
  onSaved,
}) => {
  const [saving, setSaving] = useState(false);

  const handleChange = useCallback(
    (key: keyof NotificationSettings) => (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange({ ...settings, [key]: e.target.checked });
    },
    [settings, onChange],
  );

  const handleTextFieldChange = useCallback(
    (key: keyof NotificationSettings) => (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange({ ...settings, [key]: e.target.value });
    },
    [settings, onChange],
  );

  const handleSave = useCallback(() => {
    setSaving(true);
    setTimeout(() => {
      setSaving(false);
      onSaved();
    }, 800);
  }, [onSaved]);

  return (
    <Stack spacing={3}>
      {/* Email Notifications */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={2.5}>
            <EmailIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Email Notifications
            </Typography>
            <Box sx={{ ml: 'auto' }}>
              <Switch
                checked={settings.email}
                onChange={handleChange('email')}
                color="primary"
              />
            </Box>
          </Stack>

          {settings.email && (
            <Stack spacing={2.5} sx={{ ml: 4 }}>
              <TextField
                label="Notification Email"
                value={settings.emailAddress ?? ''}
                onChange={handleTextFieldChange('emailAddress')}
                type="email"
                fullWidth
                placeholder="Enter email address for notifications"
                helperText="Leave blank to use your profile email"
              />

              <Divider />

              <Typography variant="subtitle2" fontWeight={600}>
                Notify me when:
              </Typography>

              <FormControlLabel
                control={
                  <Switch
                    checked={settings.onExperimentComplete}
                    onChange={handleChange('onExperimentComplete')}
                    size="small"
                    color="success"
                  />
                }
                label={
                  <Stack direction="row" spacing={1} alignItems="center">
                    <SuccessIcon sx={{ fontSize: 16, color: 'success.main' }} />
                    <Typography variant="body2">
                      Experiment completes successfully
                    </Typography>
                  </Stack>
                }
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={settings.onExperimentFailed}
                    onChange={handleChange('onExperimentFailed')}
                    size="small"
                    color="error"
                  />
                }
                label={
                  <Stack direction="row" spacing={1} alignItems="center">
                    <SuccessIcon sx={{ fontSize: 16, color: 'error.main' }} />
                    <Typography variant="body2">Experiment fails</Typography>
                  </Stack>
                }
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={settings.onClusterDegraded}
                    onChange={handleChange('onClusterDegraded')}
                    size="small"
                    color="warning"
                  />
                }
                label={
                  <Stack direction="row" spacing={1} alignItems="center">
                    <SuccessIcon sx={{ fontSize: 16, color: 'warning.main' }} />
                    <Typography variant="body2">Cluster health degrades</Typography>
                  </Stack>
                }
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={settings.onSIEMAlert}
                    onChange={handleChange('onSIEMAlert')}
                    size="small"
                    color="info"
                  />
                }
                label={
                  <Stack direction="row" spacing={1} alignItems="center">
                    <SuccessIcon sx={{ fontSize: 16, color: 'info.main' }} />
                    <Typography variant="body2">SIEM alert is triggered</Typography>
                  </Stack>
                }
              />
            </Stack>
          )}
        </CardContent>
      </Card>

      {/* Slack Notifications */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={2.5}>
            <NotificationsIcon sx={{ color: 'secondary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Slack Notifications
            </Typography>
            <Box sx={{ ml: 'auto' }}>
              <Switch
                checked={settings.slack}
                onChange={handleChange('slack')}
                color="secondary"
              />
            </Box>
          </Stack>

          {settings.slack && (
            <Stack spacing={2.5} sx={{ ml: 4 }}>
              <TextField
                label="Slack Webhook URL"
                value={settings.slackWebhookUrl ?? ''}
                onChange={handleTextFieldChange('slackWebhookUrl')}
                fullWidth
                placeholder="https://hooks.slack.com/services/..."
                helperText="Create a webhook in your Slack workspace settings"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <NotificationsIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                }}
              />
            </Stack>
          )}
        </CardContent>
      </Card>

      {/* Webhook Notifications */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={2.5}>
            <WebhookIcon sx={{ color: 'info.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Webhook Notifications
            </Typography>
            <Box sx={{ ml: 'auto' }}>
              <Switch
                checked={settings.webhook}
                onChange={handleChange('webhook')}
                color="info"
              />
            </Box>
          </Stack>

          {settings.webhook && (
            <Stack spacing={2.5} sx={{ ml: 4 }}>
              <TextField
                label="Webhook URL"
                value={settings.webhookUrl ?? ''}
                onChange={handleTextFieldChange('webhookUrl')}
                fullWidth
                placeholder="https://your-endpoint.example.com/webhook"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <WebhookIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                }}
              />
              <Typography variant="caption" color="text.secondary">
                Webhook payloads are sent as JSON POST requests on the same events as
                email notifications.
              </Typography>
            </Stack>
          )}
        </CardContent>
      </Card>

      {/* Save Button */}
      <Box>
        <Button
          variant="contained"
          startIcon={saving ? undefined : <SaveIcon />}
          onClick={handleSave}
          disabled={saving}
          sx={{ textTransform: 'none', fontWeight: 600 }}
        >
          {saving ? 'Saving...' : 'Save Notification Settings'}
        </Button>
      </Box>
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// SIEM Configuration Section
// ---------------------------------------------------------------------------

interface SIEMSettingsProps {
  settings: SIEMConfig;
  onChange: (settings: SIEMConfig) => void;
  onSaved: () => void;
}

const SIEMSettingsSection: React.FC<SIEMSettingsProps> = ({
  settings,
  onChange,
  onSaved,
}) => {
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<'success' | 'error' | null>(null);
  const [saving, setSaving] = useState(false);

  const handleProviderChange = useCallback(
    (e: SelectChangeEvent) => {
      onChange({ ...settings, provider: e.target.value as SIEMConfig['provider'] });
    },
    [settings, onChange],
  );

  const handleFieldChange = useCallback(
    (field: keyof SIEMConfig) => (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange({ ...settings, [field]: e.target.value });
    },
    [settings, onChange],
  );

  const handleToggle = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange({ ...settings, enabled: e.target.checked });
    },
    [settings, onChange],
  );

  const handleTestConnection = useCallback(() => {
    setTesting(true);
    setTestResult(null);
    setTimeout(() => {
      setTesting(false);
      setTestResult(settings.endpoint ? 'success' : 'error');
    }, 2000);
  }, [settings.endpoint]);

  const handleSave = useCallback(() => {
    setSaving(true);
    setTimeout(() => {
      setSaving(false);
      onSaved();
    }, 800);
  }, [onSaved]);

  const providerOptions = [
    { value: 'elastic', label: 'Elastic SIEM (Elasticsearch)', icon: '🔴' },
    { value: 'splunk', label: 'Splunk', icon: '🟠' },
    { value: 'sentinel', label: 'Microsoft Sentinel', icon: '🔵' },
    { value: 'other', label: 'Other (Custom)', icon: '⚪' },
  ];

  return (
    <Stack spacing={3}>
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={3}>
            <SIEMIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              SIEM Integration
            </Typography>
            <Box sx={{ ml: 'auto' }}>
              <FormControlLabel
                control={
                  <Switch
                    checked={settings.enabled}
                    onChange={handleToggle}
                    color="primary"
                  />
                }
                label={
                  <Typography variant="body2" fontWeight={600}>
                    {settings.enabled ? 'Enabled' : 'Disabled'}
                  </Typography>
                }
                labelPlacement="start"
              />
            </Box>
          </Stack>

          {settings.enabled && (
            <Stack spacing={2.5}>
              {/* Provider Selection */}
              <FormControl fullWidth>
                <InputLabel id="siem-provider-label">SIEM Provider</InputLabel>
                <Select
                  labelId="siem-provider-label"
                  value={settings.provider}
                  label="SIEM Provider"
                  onChange={handleProviderChange}
                >
                  {providerOptions.map((option) => (
                    <MenuItem key={option.value} value={option.value}>
                      <Stack direction="row" spacing={1.5} alignItems="center">
                        <span style={{ fontSize: '1rem' }}>{option.icon}</span>
                        <Typography variant="body2">{option.label}</Typography>
                      </Stack>
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              {/* Endpoint */}
              <TextField
                label="SIEM Endpoint URL"
                value={settings.endpoint}
                onChange={handleFieldChange('endpoint')}
                fullWidth
                placeholder="https://your-siem.example.com/api"
                required
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <CloudIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                }}
              />

              {/* API Key */}
              <TextField
                label="API Key / Token"
                value={settings.apiKey}
                onChange={handleFieldChange('apiKey')}
                fullWidth
                type="password"
                placeholder="Enter your SIEM API key"
                required
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <KeyIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                }}
              />

              {/* Index Name */}
              <TextField
                label="Index / Log Name"
                value={settings.indexName ?? ''}
                onChange={handleFieldChange('indexName')}
                fullWidth
                placeholder="e.g., security-alerts, chaos-sec-logs"
                helperText="The SIEM index or log source name where alerts are indexed"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <NamespaceIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                }}
              />

              {/* Test Connection */}
              <Stack direction="row" spacing={2} alignItems="center">
                <Button
                  variant="outlined"
                  startIcon={<PowerIcon />}
                  onClick={handleTestConnection}
                  disabled={testing || !settings.endpoint || !settings.apiKey}
                  sx={{ textTransform: 'none', fontWeight: 600 }}
                >
                  {testing ? 'Testing...' : 'Test Connection'}
                </Button>
                {testResult === 'success' && (
                  <Chip
                    icon={<SuccessIcon />}
                    label="Connection successful"
                    color="success"
                    size="small"
                    sx={{ fontWeight: 600 }}
                  />
                )}
                {testResult === 'error' && (
                  <Chip
                    label="Connection failed"
                    color="error"
                    size="small"
                    variant="outlined"
                    sx={{ fontWeight: 600 }}
                  />
                )}
              </Stack>
            </Stack>
          )}
        </CardContent>
      </Card>

      {/* Save Button */}
      {settings.enabled && (
        <Box>
          <Button
            variant="contained"
            startIcon={saving ? undefined : <SaveIcon />}
            onClick={handleSave}
            disabled={saving}
            sx={{ textTransform: 'none', fontWeight: 600 }}
          >
            {saving ? 'Saving...' : 'Save SIEM Configuration'}
          </Button>
        </Box>
      )}
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Experiment Defaults Section
// ---------------------------------------------------------------------------

interface ExperimentDefaultsProps {
  settings: AppSettings['experimentDefaults'];
  defaultNamespace: string;
  autoRefreshInterval: number;
  onChange: (update: Partial<AppSettings>) => void;
  onSaved: () => void;
}

const ExperimentDefaultsSection: React.FC<ExperimentDefaultsProps> = ({
  settings,
  defaultNamespace,
  autoRefreshInterval,
  onChange,
  onSaved,
}) => {
  const [saving, setSaving] = useState(false);

  const handleSave = useCallback(() => {
    setSaving(true);
    setTimeout(() => {
      setSaving(false);
      onSaved();
    }, 800);
  }, [onSaved]);

  const refreshMarks = [
    { value: 10, label: '10s' },
    { value: 30, label: '30s' },
    { value: 60, label: '1m' },
    { value: 120, label: '2m' },
    { value: 300, label: '5m' },
  ];

  return (
    <Stack spacing={3}>
      {/* Auto Refresh */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={3}>
            <TimerIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Auto Refresh
            </Typography>
            <Tooltip title="Controls how often the dashboard and experiment data refreshes automatically">
              <IconButton size="small">
                <HelpIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
              </IconButton>
            </Tooltip>
          </Stack>

          <Box sx={{ px: 1 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              Refresh interval: <strong>{autoRefreshInterval}s</strong>
            </Typography>
            <Slider
              value={autoRefreshInterval}
              onChange={(_, value) => onChange({ autoRefreshInterval: value as number })}
              min={10}
              max={300}
              step={10}
              marks={refreshMarks}
              valueLabelDisplay="auto"
              valueLabelFormat={(value) => {
                if (value >= 60) return `${value / 60}m`;
                return `${value}s`;
              }}
              sx={{
                '& .MuiSlider-thumb': {
                  width: 20,
                  height: 20,
                },
              }}
            />
            <Typography variant="caption" color="text.disabled">
              Set to 10s for real-time monitoring or 5m for lower bandwidth usage
            </Typography>
          </Box>
        </CardContent>
      </Card>

      {/* Experiment Defaults */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={3}>
            <ExperimentIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Experiment Defaults
            </Typography>
          </Stack>

          <Stack spacing={2.5}>
            {/* Default Time Window */}
            <TextField
              label="Default Time Window (seconds)"
              value={settings.defaultTimeWindow}
              onChange={(e) =>
                onChange({
                  experimentDefaults: {
                    ...settings,
                    defaultTimeWindow: parseInt(e.target.value, 10) || 300,
                  },
                })
              }
              type="number"
              fullWidth
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <TimerIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                  </InputAdornment>
                ),
                endAdornment: <InputAdornment position="end">sec</InputAdornment>,
              }}
              helperText="The default time window for SIEM alert detection in new experiments"
            />

            {/* Default Namespace */}
            <TextField
              label="Default Target Namespace"
              value={defaultNamespace}
              onChange={(e) => onChange({ defaultNamespace: e.target.value })}
              fullWidth
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <NamespaceIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                  </InputAdornment>
                ),
              }}
              helperText="The default Kubernetes namespace where attack pods are deployed"
            />

            <Divider />

            {/* Auto Cleanup */}
            <Stack
              direction="row"
              justifyContent="space-between"
              alignItems="center"
              sx={{
                p: 2,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
              }}
            >
              <Stack direction="row" spacing={1.5} alignItems="center">
                <CleanupIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                <Box>
                  <Typography variant="body2" fontWeight={600}>
                    Auto Cleanup
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Automatically delete attack pods and resources after experiment
                    completion
                  </Typography>
                </Box>
              </Stack>
              <Switch
                checked={settings.autoCleanup}
                onChange={(e) =>
                  onChange({
                    experimentDefaults: {
                      ...settings,
                      autoCleanup: e.target.checked,
                    },
                  })
                }
                color="primary"
              />
            </Stack>

            {/* Retain Logs */}
            <Stack
              direction="row"
              justifyContent="space-between"
              alignItems="center"
              sx={{
                p: 2,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
              }}
            >
              <Stack direction="row" spacing={1.5} alignItems="center">
                <LogIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                <Box>
                  <Typography variant="body2" fontWeight={600}>
                    Retain Logs
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Keep experiment logs and results even after cleanup
                  </Typography>
                </Box>
              </Stack>
              <Switch
                checked={settings.retainLogs}
                onChange={(e) =>
                  onChange({
                    experimentDefaults: {
                      ...settings,
                      retainLogs: e.target.checked,
                    },
                  })
                }
                color="primary"
              />
            </Stack>
          </Stack>
        </CardContent>
      </Card>

      {/* Appearance */}
      <Card>
        <CardContent sx={{ p: 3, '&:last-child': { pb: 3 } }}>
          <Stack direction="row" alignItems="center" spacing={1.5} mb={3}>
            <ThemeIcon sx={{ color: 'primary.main', fontSize: 20 }} />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1rem' }}>
              Appearance
            </Typography>
          </Stack>

          <Stack spacing={2.5}>
            <FormControl fullWidth>
              <InputLabel id="theme-label">Theme</InputLabel>
              <Select
                labelId="theme-label"
                value="light"
                label="Theme"
                onChange={() => {
                  /* Theme switching would be implemented with a context provider */
                }}
              >
                <MenuItem value="light">
                  <Stack direction="row" spacing={1.5} alignItems="center">
                    <ThemeIcon sx={{ fontSize: 18 }} />
                    <Typography variant="body2">Light</Typography>
                  </Stack>
                </MenuItem>
                <MenuItem value="dark">
                  <Stack direction="row" spacing={1.5} alignItems="center">
                    <ThemeIcon sx={{ fontSize: 18 }} />
                    <Typography variant="body2">Dark</Typography>
                  </Stack>
                </MenuItem>
                <MenuItem value="system">
                  <Stack direction="row" spacing={1.5} alignItems="center">
                    <LanguageIcon sx={{ fontSize: 18 }} />
                    <Typography variant="body2">System</Typography>
                  </Stack>
                </MenuItem>
              </Select>
            </FormControl>

            <FormControl fullWidth>
              <InputLabel id="language-label">Language</InputLabel>
              <Select
                labelId="language-label"
                value="en"
                label="Language"
                onChange={() => {
                  /* i18n would be implemented here */
                }}
              >
                <MenuItem value="en">English</MenuItem>
                <MenuItem value="es">Español</MenuItem>
                <MenuItem value="fr">Français</MenuItem>
                <MenuItem value="de">Deutsch</MenuItem>
                <MenuItem value="ja">日本語</MenuItem>
                <MenuItem value="zh">中文</MenuItem>
              </Select>
            </FormControl>
          </Stack>
        </CardContent>
      </Card>

      {/* Save Button */}
      <Box>
        <Button
          variant="contained"
          startIcon={saving ? undefined : <SaveIcon />}
          onClick={handleSave}
          disabled={saving}
          sx={{ textTransform: 'none', fontWeight: 600 }}
        >
          {saving ? 'Saving...' : 'Save Defaults'}
        </Button>
      </Box>
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Main Settings Page
// ---------------------------------------------------------------------------

const SettingsPage: React.FC = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const navigate = useNavigate();
  const _currentUser = useAppSelector(selectCurrentUser);

  const [activeTab, setActiveTab] = useState(0);
  const [settings, setSettings] = useState<AppSettings>(DEFAULT_SETTINGS);
  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState('');

  const handleSettingsChange = useCallback((update: Partial<AppSettings>) => {
    setSettings((prev) => ({ ...prev, ...update }));
  }, []);

  const handleNotificationChange = useCallback((notifications: NotificationSettings) => {
    setSettings((prev) => ({ ...prev, notifications }));
  }, []);

  const handleSIEMChange = useCallback((siem: SIEMConfig) => {
    setSettings((prev) => ({ ...prev, siem }));
  }, []);

  const handleSaved = useCallback((section: string = 'Settings') => {
    setSnackbarMessage(`${section} saved successfully`);
    setSnackbarOpen(true);
  }, []);

  const TAB_CONFIG = [
    {
      label: 'Profile',
      icon: <ProfileIcon sx={{ fontSize: 18 }} />,
    },
    {
      label: 'Notifications',
      icon: <NotificationsIcon sx={{ fontSize: 18 }} />,
    },
    {
      label: 'SIEM',
      icon: <SIEMIcon sx={{ fontSize: 18 }} />,
    },
    {
      label: 'Defaults',
      icon: <ExperimentIcon sx={{ fontSize: 18 }} />,
    },
  ];

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
          <Breadcrumbs sx={{ mb: 1 }}>
            <Link
              underline="hover"
              color="text.secondary"
              sx={{ cursor: 'pointer' }}
              onClick={() => navigate('/')}
            >
              Dashboard
            </Link>
            <Typography color="text.primary" fontWeight={600}>
              Settings
            </Typography>
          </Breadcrumbs>
          <Typography variant="h4" fontWeight={700}>
            Settings
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Manage your account preferences and platform configuration.
          </Typography>
        </Box>
      </Stack>

      {/* Tab Navigation */}
      <Paper
        variant="outlined"
        sx={{
          borderRadius: 2,
          overflow: 'hidden',
          mb: 0,
        }}
      >
        <Box sx={{ borderBottom: 1, borderColor: 'divider', px: 2 }}>
          <Tabs
            value={activeTab}
            onChange={(_, newValue) => setActiveTab(newValue)}
            variant={isMobile ? 'scrollable' : 'standard'}
            scrollButtons="auto"
            allowScrollButtonsMobile
          >
            {TAB_CONFIG.map((tab, index) => (
              <Tab
                key={index}
                label={tab.label}
                icon={tab.icon}
                iconPosition="start"
                sx={{
                  minHeight: 48,
                  textTransform: 'none',
                  fontWeight: 600,
                  fontSize: '0.875rem',
                }}
              />
            ))}
          </Tabs>
        </Box>

        <Box sx={{ p: { xs: 2, sm: 3 } }}>
          {/* Profile Tab */}
          <TabPanel value={activeTab} index={0}>
            <ProfileSettings onSaved={() => handleSaved('Profile')} />
          </TabPanel>

          {/* Notifications Tab */}
          <TabPanel value={activeTab} index={1}>
            <NotificationSettingsSection
              settings={settings.notifications}
              onChange={handleNotificationChange}
              onSaved={() => handleSaved('Notifications')}
            />
          </TabPanel>

          {/* SIEM Tab */}
          <TabPanel value={activeTab} index={2}>
            <SIEMSettingsSection
              settings={settings.siem}
              onChange={handleSIEMChange}
              onSaved={() => handleSaved('SIEM Configuration')}
            />
          </TabPanel>

          {/* Defaults Tab */}
          <TabPanel value={activeTab} index={3}>
            <ExperimentDefaultsSection
              settings={settings.experimentDefaults}
              defaultNamespace={settings.defaultNamespace}
              autoRefreshInterval={settings.autoRefreshInterval}
              onChange={handleSettingsChange}
              onSaved={() => handleSaved('Experiment Defaults')}
            />
          </TabPanel>
        </Box>
      </Paper>

      {/* Success Snackbar */}
      <Snackbar
        open={snackbarOpen}
        autoHideDuration={4000}
        onClose={() => setSnackbarOpen(false)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={() => setSnackbarOpen(false)}
          severity="success"
          variant="filled"
          icon={<SuccessIcon />}
          sx={{
            borderRadius: 2,
            fontWeight: 600,
            width: '100%',
            minWidth: 300,
          }}
        >
          {snackbarMessage}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default SettingsPage;
