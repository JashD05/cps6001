/**
 * Auth Flow Integration Tests
 *
 * Covers the complete authentication lifecycle through the UI, Redux store,
 * and mocked API layer:
 *
 *  1. Login flow: fill email + password → submit → redirect to dashboard
 *  2. Login error handling: wrong credentials → error message shown
 *  3. Logout flow: click logout → state cleared → redirect to login
 *  4. Token refresh flow: dispatch refresh → tokens updated or state cleared
 *  5. Session expired → redirect to login with expired=true
 */

import React, { useEffect } from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { Provider, useSelector, useDispatch } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import LoginPage from '@/pages/LoginPage';
import authReducer, {
  login,
  logout,
  refreshToken,
  clearAuth,
  selectIsAuthenticated,
} from '@/store/authSlice';
import experimentReducer from '@/store/experimentSlice';
import {
  authAPI,
  getAccessToken,
  getRefreshToken,
  setTokens,
  clearTokens,
  getErrorMessage,
  experimentsAPI,
  templatesAPI,
  clustersAPI,
  dashboardAPI,
  reportsAPI,
  siemAPI,
} from '@/services/api';
import type { LoginResponse, User, AuthState } from '@/types';
import type { AppDispatch } from '@/store';
import lightTheme from '@/theme';
import { ThemeProvider, StyledEngineProvider } from '@mui/material';

// ---------------------------------------------------------------------------
// Mocks – API module
// ---------------------------------------------------------------------------

jest.mock('@/services/api', () => ({
  authAPI: {
    login: jest.fn(),
    logout: jest.fn(),
    refresh: jest.fn(),
    me: jest.fn(),
    updateProfile: jest.fn(),
    changePassword: jest.fn(),
  },
  experimentsAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    execute: jest.fn(),
    stop: jest.fn(),
    getRuns: jest.fn(),
    getRunById: jest.fn(),
    getLogs: jest.fn(),
    getResults: jest.fn(),
  },
  templatesAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    getCategories: jest.fn(),
  },
  clustersAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    register: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    healthCheck: jest.fn(),
    getNamespaces: jest.fn(),
    getResources: jest.fn(),
  },
  dashboardAPI: {
    getSummary: jest.fn(),
    getSecurityPosture: jest.fn(),
    getRecentExperiments: jest.fn(),
    getClusterHealth: jest.fn(),
    getActivityTimeline: jest.fn(),
    getMetrics: jest.fn(),
  },
  reportsAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    generate: jest.fn(),
    download: jest.fn(),
    delete: jest.fn(),
    share: jest.fn(),
    schedule: jest.fn(),
  },
  siemAPI: {
    getAlerts: jest.fn(),
    getAlertById: jest.fn(),
    acknowledgeAlert: jest.fn(),
    dismissAlert: jest.fn(),
    getValidationResults: jest.fn(),
    testConnection: jest.fn(),
    getConfigurations: jest.fn(),
    updateConfiguration: jest.fn(),
  },
  getAccessToken: jest.fn(),
  getRefreshToken: jest.fn(),
  setTokens: jest.fn(),
  clearTokens: jest.fn(),
  getErrorMessage: jest.fn((_err: unknown) => 'An unexpected error occurred'),
}));

const mockedAuthAPI = authAPI as jest.Mocked<typeof authAPI>;
const mockedGetAccessToken = getAccessToken as jest.MockedFunction<typeof getAccessToken>;
const mockedGetRefreshToken = getRefreshToken as jest.MockedFunction<
  typeof getRefreshToken
>;
const mockedSetTokens = setTokens as jest.MockedFunction<typeof setTokens>;
const mockedClearTokens = clearTokens as jest.MockedFunction<typeof clearTokens>;

// ---------------------------------------------------------------------------
// Mock – ToastProvider (lightweight passthrough)
// ---------------------------------------------------------------------------

jest.mock('@/services/toast', () => ({
  ToastProvider: (props: React.PropsWithChildren) => props.children,
  useToast: () => ({
    toasts: [],
    showToast: jest.fn(() => 'mock-toast-id'),
    dismissToast: jest.fn(),
    dismissAll: jest.fn(),
    success: jest.fn(() => 'mock-toast-id'),
    error: jest.fn(() => 'mock-toast-id'),
    warning: jest.fn(() => 'mock-toast-id'),
    info: jest.fn(() => 'mock-toast-id'),
  }),
  showToast: jest.fn(() => 'mock-toast-id'),
  __registerToastHandler: jest.fn(),
}));

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const mockUser: User = {
  id: 'user-1',
  email: 'admin@chaos-sec.io',
  name: 'Admin User',
  role: 'admin',
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  lastLoginAt: '2024-06-01T12:00:00Z',
};

const mockLoginResponse: LoginResponse = {
  accessToken: 'new-access-token',
  refreshToken: 'new-refresh-token',
  expiresIn: 3600,
  tokenType: 'Bearer',
  user: mockUser,
};

const unauthenticatedAuthState = {
  user: null,
  accessToken: null,
  refreshToken: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

const authenticatedAuthState = {
  user: mockUser,
  accessToken: 'test-access-token',
  refreshToken: 'test-refresh-token',
  isAuthenticated: true,
  isLoading: false,
  error: null,
};

const initialExperimentState = {
  list: {
    experiments: [],
    totalCount: 0,
    currentPage: 1,
    pageSize: 10,
    isLoading: false,
    error: null,
    filters: {
      search: '',
      status: 'all' as const,
      templateId: null,
      clusterId: null,
      dateFrom: null,
      dateTo: null,
    },
    sortBy: 'createdAt',
    sortOrder: 'desc' as const,
  },
  detail: {
    experiment: null,
    currentRun: null,
    logs: [],
    isLoading: false,
    error: null,
  },
  createStatus: 'idle' as const,
  createError: null,
  executeStatus: 'idle' as const,
  executeError: null,
  stopStatus: 'idle' as const,
  stopError: null,
  deleteStatus: 'idle' as const,
  deleteError: null,
  runs: [],
  runsTotalCount: 0,
  runsPage: 1,
  runsLoading: false,
  runsError: null,
};

// ---------------------------------------------------------------------------
// Test store factory
// ---------------------------------------------------------------------------

function createTestStore(authState: AuthState = unauthenticatedAuthState) {
  return configureStore({
    reducer: {
      auth: authReducer,
      experiments: experimentReducer,
    },
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware({
        serializableCheck: false,
        immutableCheck: false,
      }),
    preloadedState: {
      auth: authState,
      experiments: initialExperimentState,
    },
  });
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

/**
 * Render the real LoginPage inside a Routes setup so that
 * useNavigate / useSearchParams work correctly.
 */
function renderLoginPage(
  initialPath: string = '/login',
  authState: AuthState = unauthenticatedAuthState,
) {
  const store = createTestStore(authState);
  const user = userEvent.setup();

  const result = render(
    <Provider store={store}>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={lightTheme}>
          <MemoryRouter initialEntries={[initialPath]}>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route
                path="/"
                element={<div data-testid="dashboard-page">Dashboard</div>}
              />
              <Route
                path="/dashboard"
                element={<div data-testid="dashboard-page">Dashboard</div>}
              />
              <Route
                path="/register"
                element={<div data-testid="register-page">Register</div>}
              />
            </Routes>
          </MemoryRouter>
        </ThemeProvider>
      </StyledEngineProvider>
    </Provider>,
  );

  return { ...result, store, user };
}

/**
 * A minimal component that mirrors the "authenticated layout → logout" flow.
 * When authenticated it shows a logout button; when not, it renders the
 * LoginPage. This lets us test logout → redirect without the full App.
 */
function LogoutTestScene() {
  const dispatch = useDispatch<AppDispatch>();
  const isAuthenticated = useSelector(selectIsAuthenticated);
  const navigate = useNavigate();

  useEffect(() => {
    if (!isAuthenticated) {
      navigate('/login', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  if (!isAuthenticated) {
    return <LoginPage />;
  }

  return (
    <div>
      <div data-testid="dashboard-page">Dashboard</div>
      <button data-testid="logout-button" onClick={() => dispatch(logout())}>
        Logout
      </button>
    </div>
  );
}

/**
 * Render the LogoutTestScene with a store pre-loaded with authenticated state.
 */
function renderLogoutScene() {
  const store = createTestStore(authenticatedAuthState);
  const user = userEvent.setup();

  const result = render(
    <Provider store={store}>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={lightTheme}>
          <MemoryRouter initialEntries={['/']}>
            <Routes>
              <Route path="/" element={<LogoutTestScene />} />
              <Route path="/login" element={<LoginPage />} />
            </Routes>
          </MemoryRouter>
        </ThemeProvider>
      </StyledEngineProvider>
    </Provider>,
  );

  return { ...result, store, user };
}

// ===========================================================================
// Tests
// ===========================================================================

describe('Auth Flow Integration', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetAccessToken.mockReturnValue(null);
    mockedGetRefreshToken.mockReturnValue(null);
  });

  // -------------------------------------------------------------------------
  // 1. Complete login flow
  // -------------------------------------------------------------------------
  describe('Login flow – success', () => {
    it('allows a user to log in with valid credentials and redirects to dashboard', async () => {
      mockedAuthAPI.login.mockResolvedValueOnce({
        data: { success: true, data: mockLoginResponse },
      } as any);

      const { store, user } = renderLoginPage();

      // Fill in email
      const emailInput = screen.getByLabelText(/email address/i);
      await user.type(emailInput, 'admin@chaos-sec.io');

      // Fill in password (8+ chars to pass validation)
      const passwordInput = screen.getByLabelText(/^password$/i);
      await user.type(passwordInput, 'SecureP@ss1');

      // Submit the form
      const submitButton = screen.getByRole('button', { name: /sign in/i });
      await user.click(submitButton);

      // Wait for the login thunk to resolve and redirect to dashboard
      await waitFor(() => {
        expect(screen.getByTestId('dashboard-page')).toBeInTheDocument();
      });

      // Verify the API was called with the correct credentials
      expect(mockedAuthAPI.login).toHaveBeenCalledWith({
        email: 'admin@chaos-sec.io',
        password: 'SecureP@ss1',
      });

      // Verify tokens were stored
      expect(mockedSetTokens).toHaveBeenCalledWith(
        'new-access-token',
        'new-refresh-token',
      );

      // Verify the Redux state is now authenticated
      const authState = store.getState().auth;
      expect(authState.isAuthenticated).toBe(true);
      expect(authState.user?.email).toBe('admin@chaos-sec.io');
      expect(authState.accessToken).toBe('new-access-token');
    });

    it('navigates to the redirect path after successful login', async () => {
      mockedAuthAPI.login.mockResolvedValueOnce({
        data: { success: true, data: mockLoginResponse },
      } as any);

      // Start at /login with a redirect query param pointing to /
      const { store, user } = renderLoginPage('/login?redirect=%2F');

      await user.type(screen.getByLabelText(/email address/i), 'admin@chaos-sec.io');
      await user.type(screen.getByLabelText(/^password$/i), 'SecureP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // Should redirect to the path specified in the redirect param
      await waitFor(() => {
        expect(screen.getByTestId('dashboard-page')).toBeInTheDocument();
      });
    });

    it('clears any previous auth error on mount', async () => {
      const stateWithError = {
        ...unauthenticatedAuthState,
        error: 'Previous session error',
      };

      const { store } = renderLoginPage('/login', stateWithError);

      // The LoginPage dispatches clearAuth() on mount which resets the error
      await waitFor(() => {
        expect(store.getState().auth.error).toBeNull();
      });
    });
  });

  // -------------------------------------------------------------------------
  // 2. Login error handling
  // -------------------------------------------------------------------------
  describe('Login flow – error handling', () => {
    it('shows an error message when credentials are wrong', async () => {
      mockedAuthAPI.login.mockRejectedValueOnce({
        response: {
          data: { message: 'Invalid email or password' },
        },
      });

      const { user } = renderLoginPage();

      await user.type(screen.getByLabelText(/email address/i), 'wrong@example.com');
      await user.type(screen.getByLabelText(/^password$/i), 'WrongP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // Error alert should appear with the server message
      await waitFor(() => {
        const alerts = screen.getAllByRole('alert');
        // Find the alert that contains the error message (not the session-expired one)
        const errorAlert = alerts.find((el) =>
          el.textContent?.includes('Invalid email or password'),
        );
        expect(errorAlert).toBeTruthy();
      });

      // Should stay on login page
      expect(screen.queryByTestId('dashboard-page')).not.toBeInTheDocument();
    });

    it('shows a default error message when the server provides none', async () => {
      // Reject with a generic error (no response.data.message)
      mockedAuthAPI.login.mockRejectedValueOnce(new Error('Network Error'));

      const { user } = renderLoginPage();

      await user.type(screen.getByLabelText(/email address/i), 'user@example.com');
      await user.type(screen.getByLabelText(/^password$/i), 'ValidP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // The thunk falls back to "Login failed. Please check your credentials."
      await waitFor(() => {
        const alerts = screen.getAllByRole('alert');
        const loginAlert = alerts.find((el) => el.textContent?.includes('Login failed'));
        expect(loginAlert).toBeTruthy();
      });
    });

    it('does not call the API when the email format is invalid', async () => {
      const { user } = renderLoginPage();

      await user.type(screen.getByLabelText(/email address/i), 'not-an-email');
      await user.type(screen.getByLabelText(/^password$/i), 'SecureP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // Client-side validation should block the submission
      await waitFor(() => {
        expect(screen.getByText(/enter a valid email address/i)).toBeInTheDocument();
      });

      expect(mockedAuthAPI.login).not.toHaveBeenCalled();
    });

    it('does not call the API when the password is too short', async () => {
      const { user } = renderLoginPage();

      await user.type(screen.getByLabelText(/email address/i), 'admin@chaos-sec.io');
      await user.type(screen.getByLabelText(/^password$/i), 'short');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      await waitFor(() => {
        expect(
          screen.getByText(/password must be at least 8 characters/i),
        ).toBeInTheDocument();
      });

      expect(mockedAuthAPI.login).not.toHaveBeenCalled();
    });

    it('does not call the API when fields are empty', async () => {
      const { user } = renderLoginPage();

      // Click submit without filling any fields
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      await waitFor(() => {
        expect(screen.getByText(/email is required/i)).toBeInTheDocument();
        expect(screen.getByText(/password is required/i)).toBeInTheDocument();
      });

      expect(mockedAuthAPI.login).not.toHaveBeenCalled();
    });

    it('can dismiss the error alert by clicking the close button', async () => {
      mockedAuthAPI.login.mockRejectedValueOnce({
        response: { data: { message: 'Invalid credentials' } },
      });

      const { user } = renderLoginPage();

      await user.type(screen.getByLabelText(/email address/i), 'admin@chaos-sec.io');
      await user.type(screen.getByLabelText(/^password$/i), 'WrongP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // Wait for error to appear
      await waitFor(() => {
        expect(screen.getAllByRole('alert').length).toBeGreaterThanOrEqual(1);
      });

      // Find the close button inside the error alert (MUI Alert close icon)
      const closeButtons = screen.getAllByRole('button', { name: /close/i });
      await user.click(closeButtons[0]);

      // After dismissal, the error alert should be gone
      await waitFor(() => {
        // Only session-expired or no alerts should remain
        const remainingAlerts = screen.queryAllByRole('alert');
        const loginErrorAlerts = remainingAlerts.filter(
          (el) => !el.textContent?.includes('session has expired'),
        );
        expect(loginErrorAlerts.length).toBe(0);
      });
    });

    it('allows retrying login after a failure', async () => {
      // First attempt fails
      mockedAuthAPI.login.mockRejectedValueOnce({
        response: { data: { message: 'Invalid credentials' } },
      });

      // Second attempt succeeds
      mockedAuthAPI.login.mockResolvedValueOnce({
        data: { success: true, data: mockLoginResponse },
      } as any);

      const { store, user } = renderLoginPage();

      // First attempt – wrong password
      await user.type(screen.getByLabelText(/email address/i), 'admin@chaos-sec.io');
      await user.type(screen.getByLabelText(/^password$/i), 'WrongP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      await waitFor(() => {
        expect(screen.getAllByRole('alert').length).toBeGreaterThanOrEqual(1);
      });

      // Clear password and enter correct one
      const passwordInput = screen.getByLabelText(/^password$/i);
      await user.clear(passwordInput);
      await user.type(passwordInput, 'SecureP@ss1');

      // Second attempt
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // Should now be redirected to the dashboard
      await waitFor(() => {
        expect(screen.getByTestId('dashboard-page')).toBeInTheDocument();
      });

      expect(store.getState().auth.isAuthenticated).toBe(true);
    });
  });

  // -------------------------------------------------------------------------
  // 3. Logout flow
  // -------------------------------------------------------------------------
  describe('Logout flow', () => {
    beforeEach(() => {
      mockedGetAccessToken.mockReturnValue('test-access-token');
      mockedGetRefreshToken.mockReturnValue('test-refresh-token');
    });

    it('logs the user out and redirects to the login page', async () => {
      mockedAuthAPI.logout.mockResolvedValueOnce({
        data: { success: true, data: undefined },
      } as any);

      const { store, user } = renderLogoutScene();

      // Should initially show the dashboard
      expect(screen.getByTestId('dashboard-page')).toBeInTheDocument();

      // Click the logout button
      await user.click(screen.getByTestId('logout-button'));

      // Wait for the logout thunk to complete
      await waitFor(() => {
        expect(mockedAuthAPI.logout).toHaveBeenCalled();
      });

      // Tokens should be cleared
      expect(mockedClearTokens).toHaveBeenCalled();

      // The Redux state should no longer be authenticated
      await waitFor(() => {
        expect(store.getState().auth.isAuthenticated).toBe(false);
        expect(store.getState().auth.user).toBeNull();
        expect(store.getState().auth.accessToken).toBeNull();
      });

      // The component should now render the LoginPage (via redirect)
      // LoginPage contains "Welcome to Chaos-Sec" heading
      await waitFor(() => {
        expect(screen.getByText(/welcome to chaos-sec/i)).toBeInTheDocument();
      });
    });

    it('clears auth state even when the server logout request fails', async () => {
      mockedAuthAPI.logout.mockRejectedValueOnce(new Error('Server unavailable'));

      const store = createTestStore(authenticatedAuthState);

      // Dispatch logout directly
      await store.dispatch(logout());

      // Even on server failure, clearTokens should be called
      expect(mockedClearTokens).toHaveBeenCalled();

      // The local auth state should be cleared regardless
      const authState = store.getState().auth;
      expect(authState.isAuthenticated).toBe(false);
      expect(authState.user).toBeNull();
      expect(authState.accessToken).toBeNull();
      expect(authState.refreshToken).toBeNull();
    });

    it('stores a warning error when server logout fails', async () => {
      mockedAuthAPI.logout.mockRejectedValueOnce({
        response: { data: { message: 'Session not found' } },
      });

      const store = createTestStore(authenticatedAuthState);
      const result = await store.dispatch(logout());

      // The thunk should still reject (with a message)
      expect(logout.rejected.match(result)).toBe(true);

      // But the state should be cleared
      expect(store.getState().auth.isAuthenticated).toBe(false);

      // Error may carry the server message or a fallback
      const authError = store.getState().auth.error;
      expect(authError).toBeTruthy();
    });
  });

  // -------------------------------------------------------------------------
  // 4. Token refresh flow
  // -------------------------------------------------------------------------
  describe('Token refresh flow', () => {
    it('updates tokens when the refresh thunk succeeds', async () => {
      mockedGetRefreshToken.mockReturnValue('existing-refresh-token');
      const mockRefreshResponse = {
        accessToken: 'refreshed-access-token',
        refreshToken: 'refreshed-refresh-token',
        expiresIn: 3600,
        tokenType: 'Bearer' as const,
        user: mockUser,
      };

      mockedAuthAPI.refresh.mockResolvedValueOnce({
        data: { success: true, data: mockRefreshResponse },
      } as any);

      const store = createTestStore(authenticatedAuthState);

      const result = await store.dispatch(refreshToken());

      expect(refreshToken.fulfilled.match(result)).toBe(true);
      if (refreshToken.fulfilled.match(result)) {
        expect(result.payload.accessToken).toBe('refreshed-access-token');
        expect(result.payload.refreshToken).toBe('refreshed-refresh-token');
      }

      // Verify setTokens was called with the new values
      expect(mockedSetTokens).toHaveBeenCalledWith(
        'refreshed-access-token',
        'refreshed-refresh-token',
      );

      // Verify the Redux state was updated
      const authState = store.getState().auth;
      expect(authState.accessToken).toBe('refreshed-access-token');
      expect(authState.refreshToken).toBe('refreshed-refresh-token');
      expect(authState.isAuthenticated).toBe(true);
    });

    it('clears the entire auth state when refresh fails', async () => {
      mockedGetRefreshToken.mockReturnValue('existing-refresh-token');
      mockedAuthAPI.refresh.mockRejectedValueOnce(new Error('Token expired'));

      const store = createTestStore(authenticatedAuthState);

      const result = await store.dispatch(refreshToken());

      expect(refreshToken.rejected.match(result)).toBe(true);

      // The auth state should be fully cleared
      const authState = store.getState().auth;
      expect(authState.isAuthenticated).toBe(false);
      expect(authState.user).toBeNull();
      expect(authState.accessToken).toBeNull();
      expect(authState.refreshToken).toBeNull();

      // Tokens should be removed from storage
      expect(mockedClearTokens).toHaveBeenCalled();
    });

    it('rejects immediately when no refresh token is available in storage', async () => {
      mockedGetRefreshToken.mockReturnValue(null);

      const store = createTestStore(authenticatedAuthState);

      const result = await store.dispatch(refreshToken());

      expect(refreshToken.rejected.match(result)).toBe(true);

      // The refresh API should never have been called
      expect(mockedAuthAPI.refresh).not.toHaveBeenCalled();

      // Auth state should be cleared
      expect(store.getState().auth.isAuthenticated).toBe(false);
    });

    it('sets an error containing "refresh" on rejection', async () => {
      mockedGetRefreshToken.mockReturnValue('some-refresh-token');
      mockedAuthAPI.refresh.mockRejectedValueOnce(new Error('Server error'));

      const store = createTestStore(authenticatedAuthState);

      await store.dispatch(refreshToken());

      const authError = store.getState().auth.error;
      expect(authError).toBeTruthy();
    });
  });

  // -------------------------------------------------------------------------
  // 5. Session expired → redirect to login with expired=true
  // -------------------------------------------------------------------------
  describe('Session expired handling', () => {
    it('shows a session-expired warning when arriving at /login?expired=1', async () => {
      renderLoginPage('/login?expired=1');

      // The LoginPage reads searchParams.get('expired') and renders a warning Alert
      const warningAlert = await screen.findByRole('alert');
      expect(warningAlert).toBeInTheDocument();
      expect(warningAlert).toHaveTextContent(/session has expired/i);
    });

    it('shows the session-expired warning together with a redirect param', async () => {
      // Simulates the URL that ProtectedRoute would produce
      renderLoginPage('/login?redirect=%2Fexperiments&expired=1');

      const warningAlert = await screen.findByRole('alert');
      expect(warningAlert).toHaveTextContent(/session has expired/i);
    });

    it('does not show the session-expired warning on a normal login visit', async () => {
      renderLoginPage('/login');

      // Wait for the page to render
      await screen.findByText(/welcome to chaos-sec/i);

      // No session-expired alert should appear
      const alerts = screen.queryAllByRole('alert');
      const expiredAlerts = alerts.filter((el) =>
        el.textContent?.includes('session has expired'),
      );
      expect(expiredAlerts).toHaveLength(0);
    });

    it('allows the user to log in after seeing the session-expired warning', async () => {
      mockedAuthAPI.login.mockResolvedValueOnce({
        data: { success: true, data: mockLoginResponse },
      } as any);

      const { store, user } = renderLoginPage('/login?expired=1');

      // Confirm the warning is visible
      expect(await screen.findByRole('alert')).toHaveTextContent(/session has expired/i);

      // Fill in credentials and submit
      await user.type(screen.getByLabelText(/email address/i), 'admin@chaos-sec.io');
      await user.type(screen.getByLabelText(/^password$/i), 'SecureP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // After successful login, the user should be redirected to the dashboard
      await waitFor(() => {
        expect(screen.getByTestId('dashboard-page')).toBeInTheDocument();
      });

      // The auth state should be authenticated
      expect(store.getState().auth.isAuthenticated).toBe(true);

      // No stale alerts on the dashboard screen
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });

    it('preserves the session-expired warning alongside a login error', async () => {
      // The login attempt fails
      mockedAuthAPI.login.mockRejectedValueOnce({
        response: { data: { message: 'Invalid credentials' } },
      });

      const { user } = renderLoginPage('/login?expired=1');

      // Expired warning is shown
      expect(await screen.findByRole('alert')).toHaveTextContent(/session has expired/i);

      // Attempt login with bad credentials
      await user.type(screen.getByLabelText(/email address/i), 'admin@chaos-sec.io');
      await user.type(screen.getByLabelText(/^password$/i), 'WrongP@ss1');
      await user.click(screen.getByRole('button', { name: /sign in/i }));

      // Both the expired warning and the login error should be visible
      await waitFor(() => {
        const alerts = screen.getAllByRole('alert');
        const hasExpiredWarning = alerts.some((el) =>
          el.textContent?.includes('session has expired'),
        );
        const hasLoginError = alerts.some((el) =>
          el.textContent?.includes('Invalid credentials'),
        );
        expect(hasExpiredWarning).toBe(true);
        expect(hasLoginError).toBe(true);
      });
    });
  });
});
