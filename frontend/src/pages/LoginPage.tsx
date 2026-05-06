import {
  Visibility,
  VisibilityOff,
  Login as LoginIcon,
  Security,
  VpnKey,
} from '@mui/icons-material';
import {
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Divider,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Link,
  Stack,
  TextField,
  Typography,
  Alert,
  Collapse,
  CircularProgress,
  useTheme,
  useMediaQuery,
} from '@mui/material';
import React, { useState, useEffect } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { RootState, AppDispatch } from '@/store';
import { login, clearAuth, clearError } from '@/store/authSlice';

const LoginPage: React.FC = () => {
  const theme = useTheme();
  const _isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const dispatch = useDispatch<AppDispatch>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const { isAuthenticated, isLoading, error } = useSelector(
    (state: RootState) => state.auth,
  );

  // Form state
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [rememberMe, setRememberMe] = useState(false);
  const [formErrors, setFormErrors] = useState<{ email?: string; password?: string }>({});
  const [loginAttempted, setLoginAttempted] = useState(false);

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated) {
      const redirectTo = searchParams.get('redirect') || '/dashboard';
      navigate(redirectTo, { replace: true });
    }
  }, [isAuthenticated, navigate, searchParams]);

  // Clear auth errors on mount
  useEffect(() => {
    dispatch(clearAuth());
  }, [dispatch]);

  // Validation
  const validateForm = (): boolean => {
    const errors: { email?: string; password?: string } = {};

    if (!email.trim()) {
      errors.email = 'Email is required';
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      errors.email = 'Enter a valid email address';
    }

    if (!password) {
      errors.password = 'Password is required';
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // Handle login
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setLoginAttempted(true);
    dispatch(clearError());

    if (!validateForm()) return;

    try {
      await dispatch(login({ email, password })).unwrap();
      // Navigation handled by useEffect above
    } catch (err) {
      // Error is handled by the slice and displayed below
    }
  };

  const handleClickShowPassword = () => setShowPassword(!showPassword);
  const handleMouseDownPassword = (event: React.MouseEvent) => event.preventDefault();

  const getErrorMessage = (): string | null => {
    if (!error) return null;
    if (typeof error === 'string') return error;
    if (typeof error === 'object' && error !== null) {
      if ('message' in error) return (error as { message: string }).message;
    }
    return 'Login failed. Please check your credentials and try again.';
  };

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: `linear-gradient(135deg, ${theme.palette.primary.dark} 0%, ${theme.palette.primary.main} 50%, ${theme.palette.secondary.main} 100%)`,
        px: 2,
        py: 4,
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      {/* Background decoration */}
      <Box
        sx={{
          position: 'absolute',
          top: '-20%',
          right: '-10%',
          width: 500,
          height: 500,
          borderRadius: '50%',
          background: 'rgba(255,255,255,0.03)',
          pointerEvents: 'none',
        }}
      />
      <Box
        sx={{
          position: 'absolute',
          bottom: '-15%',
          left: '-5%',
          width: 400,
          height: 400,
          borderRadius: '50%',
          background: 'rgba(255,255,255,0.02)',
          pointerEvents: 'none',
        }}
      />

      <Card
        sx={{
          width: '100%',
          maxWidth: 440,
          borderRadius: 3,
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          overflow: 'visible',
          position: 'relative',
          zIndex: 1,
        }}
      >
        <CardContent sx={{ p: { xs: 3, sm: 4 } }}>
          {/* Header */}
          <Stack spacing={1} alignItems="center" sx={{ mb: 4 }}>
            <Box
              sx={{
                width: 56,
                height: 56,
                borderRadius: 2,
                background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                mb: 1,
                boxShadow: `0 4px 20px ${theme.palette.primary.main}40`,
              }}
            >
              <Security sx={{ color: 'white', fontSize: 32 }} />
            </Box>

            <Typography
              variant="h5"
              fontWeight={700}
              sx={{ color: theme.palette.text.primary }}
            >
              Welcome to Chaos-Sec
            </Typography>
            <Typography variant="body2" color="text.secondary" textAlign="center">
              Sign in to manage your chaos engineering experiments
            </Typography>
          </Stack>

          {/* Error Alert */}
          <Collapse in={!!getErrorMessage() && loginAttempted}>
            <Alert
              severity="error"
              sx={{ mb: 2, borderRadius: 1.5 }}
              onClose={() => dispatch(clearAuth())}
            >
              {getErrorMessage() ?? ''}
            </Alert>
          </Collapse>

          {/* Session expired message */}
          {searchParams.get('expired') && (
            <Alert severity="warning" sx={{ mb: 2, borderRadius: 1.5 }}>
              Your session has expired. Please sign in again.
            </Alert>
          )}

          {/* Login Form */}
          <Box component="form" onSubmit={handleSubmit} noValidate>
            <Stack spacing={2.5}>
              {/* Email field */}
              <TextField
                id="email"
                label="Email Address"
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  dispatch(clearError());
                  if (formErrors.email)
                    setFormErrors((prev) => ({ ...prev, email: undefined }));
                }}
                error={!!formErrors.email}
                helperText={formErrors.email}
                fullWidth
                autoFocus
                autoComplete="email"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <LoginIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                }}
                sx={{
                  '& .MuiOutlinedInput-root': {
                    borderRadius: 1.5,
                  },
                }}
              />

              {/* Password field */}
              <TextField
                id="password"
                label="Password"
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  dispatch(clearError());
                  if (formErrors.password)
                    setFormErrors((prev) => ({ ...prev, password: undefined }));
                }}
                error={!!formErrors.password}
                helperText={formErrors.password}
                fullWidth
                autoComplete="current-password"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <VpnKey sx={{ fontSize: 20, color: 'text.secondary' }} />
                    </InputAdornment>
                  ),
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        aria-label="toggle password visibility"
                        onClick={handleClickShowPassword}
                        onMouseDown={handleMouseDownPassword}
                        edge="end"
                        size="small"
                      >
                        {showPassword ? (
                          <VisibilityOff fontSize="small" />
                        ) : (
                          <Visibility fontSize="small" />
                        )}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
                sx={{
                  '& .MuiOutlinedInput-root': {
                    borderRadius: 1.5,
                  },
                }}
              />

              {/* Remember me & Forgot password */}
              <Stack direction="row" alignItems="center" justifyContent="space-between">
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={rememberMe}
                      onChange={(e) => setRememberMe(e.target.checked)}
                      color="primary"
                      size="small"
                    />
                  }
                  label={
                    <Typography variant="body2" color="text.secondary">
                      Remember me
                    </Typography>
                  }
                  sx={{ mr: 0 }}
                />
                <Link
                  href="#"
                  variant="body2"
                  color="primary"
                  underline="hover"
                  onClick={(e) => {
                    e.preventDefault();
                    navigate('/forgot-password');
                  }}
                >
                  Forgot password?
                </Link>
              </Stack>

              {/* Submit button */}
              <Button
                type="submit"
                variant="contained"
                size="large"
                fullWidth
                disabled={isLoading}
                sx={{
                  py: 1.5,
                  borderRadius: 1.5,
                  fontWeight: 600,
                  textTransform: 'none',
                  fontSize: '1rem',
                  background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
                  boxShadow: `0 4px 20px ${theme.palette.primary.main}30`,
                  '&:hover': {
                    background: `linear-gradient(135deg, ${theme.palette.primary.dark}, ${theme.palette.primary.main})`,
                    boxShadow: `0 6px 24px ${theme.palette.primary.main}40`,
                  },
                  '&:disabled': {
                    background: theme.palette.action.disabledBackground,
                  },
                }}
              >
                {isLoading ? (
                  <Stack direction="row" spacing={1} alignItems="center">
                    <CircularProgress size={20} color="inherit" />
                    <span>Signing in...</span>
                  </Stack>
                ) : (
                  'Sign In'
                )}
              </Button>
            </Stack>
          </Box>

          {/* Divider */}
          <Divider sx={{ my: 3 }}>
            <Typography variant="caption" color="text.secondary">
              OR
            </Typography>
          </Divider>

          {/* Register link */}
          <Stack direction="row" spacing={0.5} justifyContent="center">
            <Typography variant="body2" color="text.secondary">
              Don't have an account?
            </Typography>
            <Link
              href="#"
              variant="body2"
              color="primary"
              fontWeight={600}
              underline="hover"
              onClick={(e) => {
                e.preventDefault();
                navigate('/register');
              }}
            >
              Create an account
            </Link>
          </Stack>

          {/* Terms */}
          <Typography
            variant="caption"
            color="text.disabled"
            textAlign="center"
            sx={{ mt: 3, display: 'block' }}
          >
            By signing in, you agree to our Terms of Service and Privacy Policy.
          </Typography>
        </CardContent>
      </Card>
    </Box>
  );
};

export default LoginPage;
