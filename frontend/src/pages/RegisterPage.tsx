import {
  Visibility,
  VisibilityOff,
  PersonAdd,
  Security,
  VpnKey,
  Email,
  Badge,
  Business,
  CheckCircle,
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
  Stepper,
  Step,
  StepLabel,
  useTheme,
  useMediaQuery,
} from '@mui/material';
import React, { useState, useEffect } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { RootState, AppDispatch } from '@/store';
import { clearAuth, register } from '@/store/authSlice';

interface RegisterFormData {
  name: string;
  email: string;
  password: string;
  confirmPassword: string;
  organization: string;
  agreeToTerms: boolean;
}

interface FormErrors {
  name?: string;
  email?: string;
  password?: string;
  confirmPassword?: string;
  organization?: string;
  agreeToTerms?: string;
}

const RegisterPage: React.FC = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const dispatch = useDispatch<AppDispatch>();
  const navigate = useNavigate();

  const { isLoading, error } = useSelector((state: RootState) => state.auth);

  // Form state
  const [formData, setFormData] = useState<RegisterFormData>({
    name: '',
    email: '',
    password: '',
    confirmPassword: '',
    organization: '',
    agreeToTerms: false,
  });
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [formErrors, setFormErrors] = useState<FormErrors>({});
  const [registerAttempted, setRegisterAttempted] = useState(false);
  const [registerSuccess, setRegisterSuccess] = useState(false);
  const [activeStep, setActiveStep] = useState(0);

  // Clear auth errors on mount
  useEffect(() => {
    dispatch(clearAuth());
  }, [dispatch]);

  // Password strength calculation
  const getPasswordStrength = (
    pwd: string,
  ): { label: string; color: string; score: number } => {
    let score = 0;
    if (pwd.length >= 8) score++;
    if (pwd.length >= 12) score++;
    if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) score++;
    if (/\d/.test(pwd)) score++;
    if (/[^a-zA-Z0-9]/.test(pwd)) score++;

    const levels: { label: string; color: string }[] = [
      { label: 'Very Weak', color: '#ef4444' },
      { label: 'Weak', color: '#f97316' },
      { label: 'Fair', color: '#eab308' },
      { label: 'Strong', color: '#22c55e' },
      { label: 'Very Strong', color: '#16a34a' },
    ];

    const level = levels[Math.min(score, levels.length - 1)];
    return { ...level, score };
  };

  const passwordStrength = getPasswordStrength(formData.password);

  // Validation
  const validateStep = (step: number): boolean => {
    const errors: FormErrors = {};

    if (step === 0) {
      if (!formData.name.trim()) {
        errors.name = 'Full name is required';
      } else if (formData.name.trim().length < 2) {
        errors.name = 'Name must be at least 2 characters';
      }

      if (!formData.email.trim()) {
        errors.email = 'Email is required';
      } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
        errors.email = 'Enter a valid email address';
      }

      if (!formData.organization.trim()) {
        errors.organization = 'Organization name is required';
      } else if (formData.organization.trim().length < 2) {
        errors.organization = 'Organization name must be at least 2 characters';
      }
    } else if (step === 1) {
      if (!formData.password) {
        errors.password = 'Password is required';
      } else if (formData.password.length < 8) {
        errors.password = 'Password must be at least 8 characters';
      } else if (passwordStrength.score < 2) {
        errors.password =
          'Password is too weak. Use uppercase, lowercase, numbers, and symbols.';
      }

      if (!formData.confirmPassword) {
        errors.confirmPassword = 'Please confirm your password';
      } else if (formData.password !== formData.confirmPassword) {
        errors.confirmPassword = 'Passwords do not match';
      }

      if (!formData.agreeToTerms) {
        errors.agreeToTerms = 'You must agree to the terms and conditions';
      }
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // Handle field changes
  const handleChange =
    (field: keyof RegisterFormData) => (event: React.ChangeEvent<HTMLInputElement>) => {
      const value =
        event.target.type === 'checkbox' ? event.target.checked : event.target.value;
      setFormData((prev) => ({ ...prev, [field]: value }));
      if (formErrors[field]) {
        setFormErrors((prev) => ({ ...prev, [field]: undefined }));
      }
    };

  // Handle step navigation
  const handleNext = () => {
    if (validateStep(activeStep)) {
      setActiveStep((prev) => prev + 1);
    }
  };

  const handleBack = () => {
    setActiveStep((prev) => prev - 1);
  };

  // Handle registration
  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setRegisterAttempted(true);

    if (!validateStep(1)) return;

    try {
      await dispatch(
        register({
          name: formData.name,
          email: formData.email,
          password: formData.password,
          organization: formData.organization,
        }),
      ).unwrap();

      setRegisterSuccess(true);
    } catch (err) {
      // Error is already stored in Redux state by the rejected case
    }
  };

  const handleClickShowPassword = () => setShowPassword(!showPassword);
  const handleClickShowConfirmPassword = () =>
    setShowConfirmPassword(!showConfirmPassword);
  const handleMouseDownPassword = (event: React.MouseEvent) => event.preventDefault();

  const getErrorMessage = (): string | null => {
    if (!error) return null;
    if (typeof error === 'string') return error;
    if (typeof error === 'object' && error !== null) {
      if ('message' in error) return (error as { message: string }).message;
    }
    return 'Registration failed. Please try again.';
  };

  const steps = ['Account Details', 'Security'];

  // Success state
  if (registerSuccess) {
    return (
      <Box
        sx={{
          minHeight: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: `linear-gradient(135deg, ${theme.palette.primary.dark} 0%, ${theme.palette.primary.main} 50%, ${theme.palette.secondary.main} 100%)`,
          px: 2,
        }}
      >
        <Card
          sx={{
            width: '100%',
            maxWidth: 440,
            borderRadius: 3,
            boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          }}
        >
          <CardContent sx={{ p: 4, textAlign: 'center' }}>
            <Box
              sx={{
                width: 72,
                height: 72,
                borderRadius: '50%',
                background: `linear-gradient(135deg, ${theme.palette.success.main}, ${theme.palette.success.dark})`,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                mx: 'auto',
                mb: 3,
                boxShadow: `0 4px 20px ${theme.palette.success.main}40`,
              }}
            >
              <CheckCircle sx={{ color: 'white', fontSize: 40 }} />
            </Box>

            <Typography variant="h5" fontWeight={700} gutterBottom>
              Account Created!
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
              Your Chaos-Sec account has been created successfully. Please check your
              email to verify your account before signing in.
            </Typography>

            <Button
              variant="contained"
              size="large"
              fullWidth
              onClick={() => navigate('/login')}
              sx={{
                py: 1.5,
                borderRadius: 1.5,
                fontWeight: 600,
                textTransform: 'none',
                fontSize: '1rem',
                background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
              }}
            >
              Go to Sign In
            </Button>
          </CardContent>
        </Card>
      </Box>
    );
  }

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
          left: '-10%',
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
          right: '-5%',
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
          maxWidth: 480,
          borderRadius: 3,
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          overflow: 'visible',
          position: 'relative',
          zIndex: 1,
        }}
      >
        <CardContent sx={{ p: { xs: 3, sm: 4 } }}>
          {/* Header */}
          <Stack spacing={1} alignItems="center" sx={{ mb: 3 }}>
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

            <Typography variant="h5" fontWeight={700}>
              Create Account
            </Typography>
            <Typography variant="body2" color="text.secondary" textAlign="center">
              Start your chaos engineering journey with Chaos-Sec
            </Typography>
          </Stack>

          {/* Stepper */}
          <Stepper activeStep={activeStep} sx={{ mb: 3 }}>
            {steps.map((label) => (
              <Step key={label}>
                <StepLabel
                  sx={{
                    '& .MuiStepLabel-label': {
                      fontSize: { xs: '0.75rem', sm: '0.875rem' },
                    },
                  }}
                >
                  {!isMobile ? label : ''}
                </StepLabel>
              </Step>
            ))}
          </Stepper>

          {/* Error Alert */}
          <Collapse in={!!getErrorMessage() && registerAttempted}>
            <Alert
              severity="error"
              sx={{ mb: 2, borderRadius: 1.5 }}
              onClose={() => dispatch(clearAuth())}
            >
              {getErrorMessage()!}
            </Alert>
          </Collapse>

          {/* Registration Form */}
          <Box component="form" onSubmit={handleSubmit} noValidate>
            {/* Step 1: Account Details */}
            {activeStep === 0 && (
              <Stack spacing={2.5}>
                <TextField
                  id="name"
                  label="Full Name"
                  value={formData.name}
                  onChange={handleChange('name')}
                  error={!!formErrors.name}
                  helperText={formErrors.name}
                  fullWidth
                  autoFocus
                  autoComplete="name"
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <Badge sx={{ fontSize: 20, color: 'text.secondary' }} />
                      </InputAdornment>
                    ),
                  }}
                  sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1.5 } }}
                />

                <TextField
                  id="email"
                  label="Work Email"
                  type="email"
                  value={formData.email}
                  onChange={handleChange('email')}
                  error={!!formErrors.email}
                  helperText={formErrors.email}
                  fullWidth
                  autoComplete="email"
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <Email sx={{ fontSize: 20, color: 'text.secondary' }} />
                      </InputAdornment>
                    ),
                  }}
                  sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1.5 } }}
                />

                <TextField
                  id="organization"
                  label="Organization Name"
                  value={formData.organization}
                  onChange={handleChange('organization')}
                  error={!!formErrors.organization}
                  helperText={formErrors.organization}
                  fullWidth
                  autoComplete="organization"
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <Business sx={{ fontSize: 20, color: 'text.secondary' }} />
                      </InputAdornment>
                    ),
                  }}
                  sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1.5 } }}
                />

                <Button
                  variant="contained"
                  size="large"
                  fullWidth
                  onClick={handleNext}
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
                    },
                  }}
                >
                  Continue
                </Button>
              </Stack>
            )}

            {/* Step 2: Security */}
            {activeStep === 1 && (
              <Stack spacing={2.5}>
                <TextField
                  id="password"
                  label="Password"
                  type={showPassword ? 'text' : 'password'}
                  value={formData.password}
                  onChange={handleChange('password')}
                  error={!!formErrors.password}
                  helperText={formErrors.password}
                  fullWidth
                  autoComplete="new-password"
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
                  sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1.5 } }}
                />

                {/* Password strength indicator */}
                {formData.password && (
                  <Box>
                    <Stack
                      direction="row"
                      justifyContent="space-between"
                      alignItems="center"
                      sx={{ mb: 0.5 }}
                    >
                      <Typography variant="caption" color="text.secondary">
                        Password Strength
                      </Typography>
                      <Typography
                        variant="caption"
                        sx={{ color: passwordStrength.color, fontWeight: 600 }}
                      >
                        {passwordStrength.label}
                      </Typography>
                    </Stack>
                    <Box
                      sx={{
                        width: '100%',
                        height: 4,
                        borderRadius: 2,
                        bgcolor: theme.palette.action.hover,
                        overflow: 'hidden',
                      }}
                    >
                      <Box
                        sx={{
                          width: `${(passwordStrength.score / 5) * 100}%`,
                          height: '100%',
                          borderRadius: 2,
                          bgcolor: passwordStrength.color,
                          transition: 'all 0.3s ease',
                        }}
                      />
                    </Box>
                    <Stack direction="row" spacing={1} sx={{ mt: 0.5, flexWrap: 'wrap' }}>
                      {[
                        { label: '8+ chars', met: formData.password.length >= 8 },
                        { label: 'Uppercase', met: /[A-Z]/.test(formData.password) },
                        { label: 'Lowercase', met: /[a-z]/.test(formData.password) },
                        { label: 'Number', met: /\d/.test(formData.password) },
                        { label: 'Symbol', met: /[^a-zA-Z0-9]/.test(formData.password) },
                      ].map((req) => (
                        <Typography
                          key={req.label}
                          variant="caption"
                          sx={{
                            color: req.met
                              ? theme.palette.success.main
                              : theme.palette.text.disabled,
                            fontSize: '0.7rem',
                          }}
                        >
                          {req.met ? '✓' : '○'} {req.label}
                        </Typography>
                      ))}
                    </Stack>
                  </Box>
                )}

                <TextField
                  id="confirmPassword"
                  label="Confirm Password"
                  type={showConfirmPassword ? 'text' : 'password'}
                  value={formData.confirmPassword}
                  onChange={handleChange('confirmPassword')}
                  error={!!formErrors.confirmPassword}
                  helperText={formErrors.confirmPassword}
                  fullWidth
                  autoComplete="new-password"
                  InputProps={{
                    startAdornment: (
                      <InputAdornment position="start">
                        <VpnKey sx={{ fontSize: 20, color: 'text.secondary' }} />
                      </InputAdornment>
                    ),
                    endAdornment: (
                      <InputAdornment position="end">
                        <IconButton
                          aria-label="toggle confirm password visibility"
                          onClick={handleClickShowConfirmPassword}
                          onMouseDown={handleMouseDownPassword}
                          edge="end"
                          size="small"
                        >
                          {showConfirmPassword ? (
                            <VisibilityOff fontSize="small" />
                          ) : (
                            <Visibility fontSize="small" />
                          )}
                        </IconButton>
                      </InputAdornment>
                    ),
                  }}
                  sx={{ '& .MuiOutlinedInput-root': { borderRadius: 1.5 } }}
                />

                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.agreeToTerms}
                      onChange={handleChange('agreeToTerms')}
                      color="primary"
                      size="small"
                    />
                  }
                  label={
                    <Typography variant="body2" color="text.secondary">
                      I agree to the{' '}
                      <Link href="#" color="primary" underline="hover">
                        Terms of Service
                      </Link>{' '}
                      and{' '}
                      <Link href="#" color="primary" underline="hover">
                        Privacy Policy
                      </Link>
                    </Typography>
                  }
                  sx={{ alignItems: 'flex-start' }}
                />
                {formErrors.agreeToTerms && (
                  <Typography variant="caption" color="error">
                    {formErrors.agreeToTerms}
                  </Typography>
                )}

                {/* Navigation buttons */}
                <Stack direction="row" spacing={2}>
                  <Button
                    variant="outlined"
                    size="large"
                    onClick={handleBack}
                    sx={{
                      borderRadius: 1.5,
                      fontWeight: 600,
                      textTransform: 'none',
                      flex: 1,
                    }}
                  >
                    Back
                  </Button>
                  <Button
                    type="submit"
                    variant="contained"
                    size="large"
                    disabled={isLoading}
                    sx={{
                      py: 1.5,
                      borderRadius: 1.5,
                      fontWeight: 600,
                      textTransform: 'none',
                      fontSize: '1rem',
                      flex: 2,
                      background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
                      boxShadow: `0 4px 20px ${theme.palette.primary.main}30`,
                      '&:hover': {
                        background: `linear-gradient(135deg, ${theme.palette.primary.dark}, ${theme.palette.primary.main})`,
                      },
                      '&:disabled': {
                        background: theme.palette.action.disabledBackground,
                      },
                    }}
                  >
                    {isLoading ? (
                      <Stack direction="row" spacing={1} alignItems="center">
                        <CircularProgress size={20} color="inherit" />
                        <span>Creating Account...</span>
                      </Stack>
                    ) : (
                      <Stack direction="row" spacing={1} alignItems="center">
                        <PersonAdd fontSize="small" />
                        <span>Create Account</span>
                      </Stack>
                    )}
                  </Button>
                </Stack>
              </Stack>
            )}
          </Box>

          {/* Divider */}
          <Divider sx={{ my: 3 }}>
            <Typography variant="caption" color="text.secondary">
              OR
            </Typography>
          </Divider>

          {/* Login link */}
          <Stack direction="row" spacing={0.5} justifyContent="center">
            <Typography variant="body2" color="text.secondary">
              Already have an account?
            </Typography>
            <Link
              href="#"
              variant="body2"
              color="primary"
              fontWeight={600}
              underline="hover"
              onClick={(e) => {
                e.preventDefault();
                navigate('/login');
              }}
            >
              Sign in
            </Link>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
};

export default RegisterPage;
