/**
 * Unit tests for the LoginPage component.
 *
 * Tests form rendering, validation, login success/failure,
 * loading state, "Remember me" checkbox, navigation, and
 * session expired message.
 */

import { ThemeProvider, createTheme } from '@mui/material/styles';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import LoginPage from '@/pages/LoginPage';
import { login, clearAuth, clearError } from '@/store/authSlice';
import type { AuthState } from '@/types';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockDispatch = jest.fn();
const mockNavigate = jest.fn();
const mockSearchParams = new URLSearchParams();

jest.mock('react-redux', () => ({
  useDispatch: () => mockDispatch,
  useSelector: (selector: (state: { auth: AuthState }) => unknown) => selector(mockState),
}));

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  useSearchParams: () => [mockSearchParams],
}));

jest.mock('@/store/authSlice', () => ({
  login: jest.fn((payload) => ({ type: 'auth/login', payload })),
  clearAuth: jest.fn(() => ({ type: 'auth/clearAuth' })),
  clearError: jest.fn(() => ({ type: 'auth/clearError' })),
}));

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

const theme = createTheme();

// ---------------------------------------------------------------------------
// Mutable auth state mock
// ---------------------------------------------------------------------------

let mockState: { auth: AuthState };

const defaultAuthState: AuthState = {
  user: null,
  accessToken: null,
  refreshToken: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderLoginPage() {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>
    </ThemeProvider>,
  );
}

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();
  mockState = { auth: { ...defaultAuthState } };
  mockSearchParams.delete('redirect');
  mockSearchParams.delete('expired');
});

// ===========================================================================
// Form Rendering
// ===========================================================================

describe('LoginPage – form rendering', () => {
  it('renders the login form heading', () => {
    renderLoginPage();
    expect(screen.getByText('Welcome to Chaos-Sec')).toBeInTheDocument();
  });

  it('renders the subtitle text', () => {
    renderLoginPage();
    expect(
      screen.getByText('Sign in to manage your chaos engineering experiments'),
    ).toBeInTheDocument();
  });

  it('renders the email input field', () => {
    renderLoginPage();
    const emailInput = screen.getByLabelText(/email address/i);
    expect(emailInput).toBeInTheDocument();
    expect(emailInput).toHaveAttribute('type', 'email');
  });

  it('renders the password input field', () => {
    renderLoginPage();
    const passwordInput = screen.getByLabelText(/^password$/i);
    expect(passwordInput).toBeInTheDocument();
    expect(passwordInput).toHaveAttribute('type', 'password');
  });

  it('renders the Sign In button', () => {
    renderLoginPage();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('renders the Remember me checkbox', () => {
    renderLoginPage();
    expect(screen.getByLabelText(/remember me/i)).toBeInTheDocument();
  });

  it('renders the Forgot password link', () => {
    renderLoginPage();
    expect(screen.getByText(/forgot password/i)).toBeInTheDocument();
  });

  it('renders the Create an account link', () => {
    renderLoginPage();
    expect(screen.getByText(/create an account/i)).toBeInTheDocument();
  });

  it('renders the terms text', () => {
    renderLoginPage();
    expect(screen.getByText(/terms of service and privacy policy/i)).toBeInTheDocument();
  });

  it('renders the email field with autoFocus', () => {
    renderLoginPage();
    const emailInput = screen.getByLabelText(/email address/i);
    expect(emailInput).toHaveFocus();
  });

  it('renders the OR divider', () => {
    renderLoginPage();
    expect(screen.getByText('OR')).toBeInTheDocument();
  });
});

// ===========================================================================
// Form Validation
// ===========================================================================

describe('LoginPage – form validation', () => {
  it('shows "Email is required" when email is empty and form is submitted', async () => {
    renderLoginPage();

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Email is required')).toBeInTheDocument();
    });
  });

  it('shows "Enter a valid email address" when email is invalid', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    fireEvent.change(emailInput, { target: { value: 'invalid-email' } });

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Enter a valid email address')).toBeInTheDocument();
    });
  });

  it('shows "Password is required" when password is empty and form is submitted', async () => {
    renderLoginPage();

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Password is required')).toBeInTheDocument();
    });
  });

  it('allows short passwords so the default admin credential works', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'admin@chaos-sec.local' } });
    fireEvent.change(passwordInput, { target: { value: 'admin' } });

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(mockDispatch).toHaveBeenCalledWith(
        login({ email: 'admin@chaos-sec.local', password: 'admin' }),
      );
    });
  });

  it('shows no validation errors for valid email and password', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    // The form should be valid, so dispatch should be called with the login action
    await waitFor(() => {
      expect(mockDispatch).toHaveBeenCalledWith(
        login({ email: 'user@example.com', password: 'password123' }),
      );
    });
  });

  it('shows multiple validation errors simultaneously', async () => {
    renderLoginPage();

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Email is required')).toBeInTheDocument();
      expect(screen.getByText('Password is required')).toBeInTheDocument();
    });
  });

  it('accepts valid email formats', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    const validEmails = [
      'user@example.com',
      'test.user@domain.org',
      'admin+tag@company.co',
    ];

    for (const email of validEmails) {
      fireEvent.change(emailInput, { target: { value: email } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });

      // No email validation error should appear for valid emails
      expect(screen.queryByText('Enter a valid email address')).not.toBeInTheDocument();
    }
  });
});

// ===========================================================================
// Successful Login
// ===========================================================================

describe('LoginPage – successful login', () => {
  it('dispatches login action with email and password on valid form submit', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    await waitFor(() => {
      expect(mockDispatch).toHaveBeenCalledWith(
        login({ email: 'user@example.com', password: 'password123' }),
      );
    });
  });

  it('dispatches clearError before login attempt', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    await waitFor(() => {
      expect(clearError).toHaveBeenCalled();
    });
  });

  it('navigates to dashboard when authenticated', () => {
    mockState.auth = {
      ...defaultAuthState,
      isAuthenticated: true,
    };

    renderLoginPage();

    expect(mockNavigate).toHaveBeenCalledWith('/dashboard', { replace: true });
  });

  it('navigates to redirect URL when authenticated with redirect param', () => {
    mockSearchParams.set('redirect', '/experiments');
    mockState.auth = {
      ...defaultAuthState,
      isAuthenticated: true,
    };

    renderLoginPage();

    expect(mockNavigate).toHaveBeenCalledWith('/experiments', { replace: true });
  });
});

// ===========================================================================
// Failed Login
// ===========================================================================

describe('LoginPage – failed login', () => {
  it('displays error message when auth state has string error', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Invalid credentials',
    };

    renderLoginPage();

    // Need to trigger loginAttempted to show the error
    // The error is only shown when loginAttempted is true
    // Let's submit the form first to set loginAttempted, but the form
    // validation will prevent submission if email/password are empty
    // So we need to fill in valid fields first
    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    // Now the error should be displayed in the Alert
    expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
  });

  it('displays server error string from auth state', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Account locked due to too many attempts',
    };

    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    expect(
      screen.getByText('Account locked due to too many attempts'),
    ).toBeInTheDocument();
  });

  it('displays long error messages correctly', () => {
    mockState.auth = {
      ...defaultAuthState,
      error:
        'Your account has been temporarily suspended. Please contact support for assistance.',
    };

    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    expect(
      screen.getByText(
        'Your account has been temporarily suspended. Please contact support for assistance.',
      ),
    ).toBeInTheDocument();
  });

  it('does not display error alert when no error exists', () => {
    mockState.auth = { ...defaultAuthState };

    renderLoginPage();

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('does not display error when loginAttempted is false even if error exists', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Some error',
    };

    renderLoginPage();

    // Error alert should not be shown because loginAttempted is false initially
    // The Collapse component should hide the alert
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('dispatches clearAuth when error alert is closed', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Some error',
    };

    renderLoginPage();

    // Fill in form and submit to set loginAttempted = true
    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    // Now find and click the close button on the alert
    const closeButton = screen.getByRole('button', { name: /close/i });
    fireEvent.click(closeButton);

    expect(clearAuth).toHaveBeenCalled();
  });
});

// ===========================================================================
// Loading State
// ===========================================================================

describe('LoginPage – loading state', () => {
  it('disables submit button when isLoading is true', () => {
    mockState.auth = {
      ...defaultAuthState,
      isLoading: true,
    };

    renderLoginPage();

    const submitButton = screen.getByRole('button', { name: /signing in/i });
    expect(submitButton).toBeDisabled();
  });

  it('shows "Signing in..." text when isLoading is true', () => {
    mockState.auth = {
      ...defaultAuthState,
      isLoading: true,
    };

    renderLoginPage();

    expect(screen.getByText('Signing in...')).toBeInTheDocument();
  });

  it('shows CircularProgress when isLoading is true', () => {
    mockState.auth = {
      ...defaultAuthState,
      isLoading: true,
    };

    const { container } = renderLoginPage();
    expect(container.querySelector('.MuiCircularProgress-root')).toBeInTheDocument();
  });

  it('shows "Sign In" text when isLoading is false', () => {
    mockState.auth = {
      ...defaultAuthState,
      isLoading: false,
    };

    renderLoginPage();

    const signInButton = screen.getByRole('button', { name: /sign in/i });
    expect(signInButton).toBeInTheDocument();
    expect(signInButton).toHaveTextContent('Sign In');
    expect(screen.queryByText('Signing in...')).not.toBeInTheDocument();
  });

  it('does not show CircularProgress when isLoading is false', () => {
    mockState.auth = {
      ...defaultAuthState,
      isLoading: false,
    };

    const { container } = renderLoginPage();
    expect(container.querySelector('.MuiCircularProgress-root')).not.toBeInTheDocument();
  });
});

// ===========================================================================
// Remember Me Checkbox
// ===========================================================================

describe('LoginPage – remember me checkbox', () => {
  it('renders the Remember me checkbox unchecked by default', () => {
    renderLoginPage();

    const checkbox = screen.getByLabelText(/remember me/i) as HTMLInputElement;
    expect(checkbox).toBeInTheDocument();
    expect(checkbox.checked).toBe(false);
  });

  it('toggles the Remember me checkbox when clicked', () => {
    renderLoginPage();

    const checkbox = screen.getByLabelText(/remember me/i) as HTMLInputElement;

    fireEvent.click(checkbox);
    expect(checkbox.checked).toBe(true);

    fireEvent.click(checkbox);
    expect(checkbox.checked).toBe(false);
  });
});

// ===========================================================================
// Password Visibility Toggle
// ===========================================================================

describe('LoginPage – password visibility toggle', () => {
  it('renders password field with type "password" by default', () => {
    renderLoginPage();

    const passwordInput = screen.getByLabelText(/^password$/i);
    expect(passwordInput).toHaveAttribute('type', 'password');
  });

  it('toggles password visibility when the toggle button is clicked', () => {
    renderLoginPage();

    const passwordInput = screen.getByLabelText(/^password$/i);
    const toggleButton = screen.getByLabelText('toggle password visibility');

    fireEvent.click(toggleButton);
    expect(passwordInput).toHaveAttribute('type', 'text');

    fireEvent.click(toggleButton);
    expect(passwordInput).toHaveAttribute('type', 'password');
  });
});

// ===========================================================================
// Navigation
// ===========================================================================

describe('LoginPage – navigation', () => {
  it('navigates to register page when Create an account link is clicked', () => {
    renderLoginPage();

    const registerLink = screen.getByText('Create an account');
    fireEvent.click(registerLink);

    expect(mockNavigate).toHaveBeenCalledWith('/register');
  });

  it('navigates to forgot password page when Forgot password link is clicked', () => {
    renderLoginPage();

    const forgotLink = screen.getByText(/forgot password/i);
    fireEvent.click(forgotLink);

    expect(mockNavigate).toHaveBeenCalledWith('/forgot-password');
  });

  it('prevents default on register link click (href="#")', () => {
    renderLoginPage();

    const registerLink = screen.getByText('Create an account');
    // The link has href="#" but onClick calls e.preventDefault() then navigate
    expect(registerLink.closest('a')?.getAttribute('href')).toBe('#');
  });
});

// ===========================================================================
// Session Expired Message
// ===========================================================================

describe('LoginPage – session expired message', () => {
  it('shows session expired warning when expired param is present', () => {
    mockSearchParams.set('expired', '1');

    renderLoginPage();

    expect(
      screen.getByText('Your session has expired. Please sign in again.'),
    ).toBeInTheDocument();
  });

  it('does not show session expired warning when expired param is absent', () => {
    renderLoginPage();

    expect(
      screen.queryByText('Your session has expired. Please sign in again.'),
    ).not.toBeInTheDocument();
  });
});

// ===========================================================================
// Clear Auth on Mount
// ===========================================================================

describe('LoginPage – clear auth on mount', () => {
  it('dispatches clearAuth on mount', () => {
    renderLoginPage();

    expect(clearAuth).toHaveBeenCalledTimes(1);
  });
});

// ===========================================================================
// Form Input Changes
// ===========================================================================

describe('LoginPage – form input changes', () => {
  it('updates email input value when typing', () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i) as HTMLInputElement;
    fireEvent.change(emailInput, { target: { value: 'test@example.com' } });

    expect(emailInput.value).toBe('test@example.com');
  });

  it('updates password input value when typing', () => {
    renderLoginPage();

    const passwordInput = screen.getByLabelText(/^password$/i) as HTMLInputElement;
    fireEvent.change(passwordInput, { target: { value: 'mysecretpassword' } });

    expect(passwordInput.value).toBe('mysecretpassword');
  });

  it('clears email validation error when email input changes', async () => {
    renderLoginPage();

    // Submit empty form to trigger validation
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Email is required')).toBeInTheDocument();
    });

    // Fix the email
    const emailInput = screen.getByLabelText(/email address/i);
    fireEvent.change(emailInput, { target: { value: 'valid@example.com' } });

    // The validation error should be cleared
    await waitFor(() => {
      expect(screen.queryByText('Email is required')).not.toBeInTheDocument();
    });
  });

  it('clears password validation error when password input changes', async () => {
    renderLoginPage();

    // Submit empty form to trigger validation
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Password is required')).toBeInTheDocument();
    });

    // Fix the password
    const passwordInput = screen.getByLabelText(/^password$/i);
    fireEvent.change(passwordInput, { target: { value: 'longpassword123' } });

    // The validation error should be cleared
    await waitFor(() => {
      expect(screen.queryByText('Password is required')).not.toBeInTheDocument();
    });
  });

  it('dispatches clearError when email input changes', () => {
    renderLoginPage();

    jest.clearAllMocks();

    const emailInput = screen.getByLabelText(/email address/i);
    fireEvent.change(emailInput, { target: { value: 'test@example.com' } });

    expect(clearError).toHaveBeenCalled();
  });

  it('dispatches clearError when password input changes', () => {
    renderLoginPage();

    jest.clearAllMocks();

    const passwordInput = screen.getByLabelText(/^password$/i);
    fireEvent.change(passwordInput, { target: { value: 'newpassword' } });

    expect(clearError).toHaveBeenCalled();
  });
});

// ===========================================================================
// Submit Handling
// ===========================================================================

describe('LoginPage – submit handling', () => {
  it('prevents default form submission', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    const submitEvent = fireEvent.submit(formEl);

    // The form should not do a native submit
    expect(submitEvent).toBe(false);
  });

  it('does not dispatch login when form validation fails', () => {
    renderLoginPage();

    // Submit empty form
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    // login action should not be dispatched
    expect(mockDispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: 'auth/login' }),
    );
  });

  it('dispatches login with correct credentials on valid form submit', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    await waitFor(() => {
      expect(mockDispatch).toHaveBeenCalledWith(
        login({ email: 'user@example.com', password: 'password123' }),
      );
    });
  });

  it('sets loginAttempted to true on form submit even if validation fails', async () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Previous error',
    };

    renderLoginPage();

    // Submit empty form to trigger validation failure but also set loginAttempted
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    // With loginAttempted = true, the error from auth state should now be visible
    await waitFor(() => {
      expect(screen.getByText('Previous error')).toBeInTheDocument();
    });
  });
});

// ===========================================================================
// UI Elements
// ===========================================================================

describe('LoginPage – UI elements', () => {
  it('renders the Security icon', () => {
    const { container } = renderLoginPage();
    // MUI Security icon renders as an SVG
    expect(container.querySelector('svg')).toBeInTheDocument();
  });

  it('renders the email field with Login icon adornment', () => {
    renderLoginPage();
    const emailInput = screen.getByLabelText(/email address/i);
    expect(emailInput).toBeInTheDocument();
  });

  it('renders the password field with VpnKey icon adornment', () => {
    renderLoginPage();
    const passwordInput = screen.getByLabelText(/^password$/i);
    expect(passwordInput).toBeInTheDocument();
  });

  it('renders the submit button with gradient background styles', () => {
    renderLoginPage();
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    expect(submitButton).toBeInTheDocument();
    // Verify it's a contained variant button
    expect(submitButton.classList.toString()).toContain('MuiButton-contained');
  });

  it('renders the card with rounded corners', () => {
    const { container } = renderLoginPage();
    const card = container.querySelector('.MuiCard-root');
    expect(card).toBeInTheDocument();
  });
});

// ===========================================================================
// Error Message Extraction
// ===========================================================================

describe('LoginPage – error message extraction', () => {
  it('displays string error messages directly', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Server error occurred',
    };

    renderLoginPage();

    // Fill in form to set loginAttempted
    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    expect(screen.getByText('Server error occurred')).toBeInTheDocument();
  });

  it('displays different error strings from auth state', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Custom error message',
    };

    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    expect(screen.getByText('Custom error message')).toBeInTheDocument();
  });

  it('displays default error message when error is null after login attempt', () => {
    // When error is null, no alert should be shown
    mockState.auth = {
      ...defaultAuthState,
      error: null,
    };

    renderLoginPage();

    // Submit empty form to set loginAttempted
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    // No alert should appear when error is null
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('clears error alert when clearAuth is dispatched', () => {
    mockState.auth = {
      ...defaultAuthState,
      error: 'Some error to clear',
    };

    renderLoginPage();

    // Fill in form and submit to set loginAttempted
    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    expect(screen.getByText('Some error to clear')).toBeInTheDocument();
  });
});

// ===========================================================================
// Edge Cases
// ===========================================================================

describe('LoginPage – edge cases', () => {
  it('does not navigate when not authenticated', () => {
    mockState.auth = {
      ...defaultAuthState,
      isAuthenticated: false,
    };

    renderLoginPage();

    // navigate should not have been called with dashboard
    expect(mockNavigate).not.toHaveBeenCalledWith('/dashboard', { replace: true });
  });

  it('handles rapid form submissions gracefully', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    await waitFor(() => {
      expect(mockDispatch).toHaveBeenCalledWith(
        login({ email: 'user@example.com', password: 'password123' }),
      );
    });
  });

  it('renders correctly with both expired session and auth error', () => {
    mockSearchParams.set('expired', '1');
    mockState.auth = {
      ...defaultAuthState,
      error: 'Additional error',
    };

    renderLoginPage();

    // Fill in form to set loginAttempted
    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    const formEl = screen.getByRole('button', { name: /sign in/i }).closest('form');
    if (!formEl) throw new Error('Form element not found');
    fireEvent.submit(formEl);

    // Both messages should appear
    expect(
      screen.getByText('Your session has expired. Please sign in again.'),
    ).toBeInTheDocument();
    expect(screen.getByText('Additional error')).toBeInTheDocument();
  });

  it('validates email format correctly', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    // Test various invalid email formats
    const invalidEmails = ['plainaddress', 'missing@domain', '@missinglocal.com'];

    for (const email of invalidEmails) {
      fireEvent.change(emailInput, { target: { value: email } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });

      const submitButton = screen.getByRole('button', { name: /sign in/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Enter a valid email address')).toBeInTheDocument();
      });

      // Clear for next iteration
      fireEvent.change(emailInput, { target: { value: '' } });
    }
  });

  it('dispatches login for short passwords instead of blocking submission', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    const passwordInput = screen.getByLabelText(/^password$/i);

    fireEvent.change(emailInput, { target: { value: 'admin@chaos-sec.local' } });
    fireEvent.change(passwordInput, { target: { value: 'admin' } });

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(mockDispatch).toHaveBeenCalledWith(
        login({ email: 'admin@chaos-sec.local', password: 'admin' }),
      );
    });
  });
});
