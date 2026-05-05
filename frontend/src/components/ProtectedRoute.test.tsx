/**
 * Unit tests for the ProtectedRoute component.
 *
 * Covers:
 *  1. Redirects to /login when not authenticated
 *  2. Renders children when authenticated
 *  3. Shows loading spinner while verifying session
 *  4. Preserves redirect URL in query params
 *  5. Auth restoration from stored tokens
 */

import { render, screen, waitFor } from '@testing-library/react';
import React from 'react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { configureStore } from '@reduxjs/toolkit';
import ProtectedRoute from '@/components/ProtectedRoute';
import { getAccessToken, getRefreshToken } from '@/services/api';
import { me, setAuthFromStorage } from '@/store/authSlice';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Mock the api module so we can control getAccessToken().
jest.mock('@/services/api', () => ({
  getAccessToken: jest.fn(),
  getRefreshToken: jest.fn(),
}));

// Mock the authSlice async thunks as plain actions so we can inspect dispatches.
jest.mock('@/store/authSlice', () => {
  const actual = jest.requireActual('@/store/authSlice');
  return {
    ...actual,
    me: jest.fn(() => ({ type: 'auth/me/pending' })),
    setAuthFromStorage: jest.fn(() => ({
      type: 'auth/setAuthFromStorage',
      payload: { accessToken: 'mock-at', refreshToken: 'mock-rt' },
    })),
  };
});

const mockedGetAccessToken = getAccessToken as jest.MockedFunction<typeof getAccessToken>;
const mockedGetRefreshToken = getRefreshToken as jest.MockedFunction<
  typeof getRefreshToken
>;
const mockedMe = me as jest.MockedFunction<typeof me>;
const mockedSetAuthFromStorage = setAuthFromStorage as jest.MockedFunction<
  typeof setAuthFromStorage
>;

// ---------------------------------------------------------------------------
// Mock reducer that supports the shape ProtectedRoute expects
// ---------------------------------------------------------------------------

// ProtectedRoute destructures { isAuthenticated, loading, user } from state.auth.
// The real AuthState has `isLoading: boolean`, but the component reads `loading`.
// We create a flexible mock reducer that provides the shape the component expects.
interface MockAuthState {
  isAuthenticated: boolean;
  loading?: string;
  isLoading?: boolean;
  user: { id: string; name: string; email: string; role: string } | null;
  accessToken: string | null;
  refreshToken: string | null;
  error: string | null;
}

let mockAuthState: MockAuthState = {
  isAuthenticated: false,
  loading: 'idle',
  user: null,
  accessToken: null,
  refreshToken: null,
  error: null,
};

const mockAuthReducer = (
  state: MockAuthState | undefined,
  _action: { type: string; payload?: unknown },
) => {
  if (state === undefined) return mockAuthState;
  // Allow the test to override by just returning the current mock state.
  return mockAuthState;
};

/**
 * Create a Redux store pre-loaded with the given auth state.
 */
function createStore(authState: MockAuthState) {
  mockAuthState = authState;
  return configureStore({
    reducer: {
      auth: mockAuthReducer,
    },
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware({
        serializableCheck: false,
        immutableCheck: false,
      }),
  });
}

// ---------------------------------------------------------------------------
// Location capture helper
// ---------------------------------------------------------------------------

/**
 * A tiny component that records the current location (pathname + search)
 * into a ref so tests can assert where the user was redirected.
 */
function LocationCapture({
  locationRef,
}: {
  locationRef: React.MutableRefObject<string>;
}) {
  const location = useLocation();
  locationRef.current = location.pathname + location.search;
  return null;
}

// ---------------------------------------------------------------------------
// Render helper
// ---------------------------------------------------------------------------

function renderProtectedRoute(
  authState: MockAuthState,
  initialPath: string = '/dashboard',
) {
  const store = createStore(authState);
  const locationRef: React.MutableRefObject<string> = { current: '' };

  const result = render(
    <Provider store={store}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route
            path="*"
            element={
              <ProtectedRoute>
                <div data-testid="protected-content">Protected Page</div>
              </ProtectedRoute>
            }
          />
          <Route
            path="/login"
            element={
              <div>
                <div data-testid="login-page">Login Page</div>
                <LocationCapture locationRef={locationRef} />
              </div>
            }
          />
        </Routes>
      </MemoryRouter>
    </Provider>,
  );

  return { ...result, store, locationRef };
}

// ---------------------------------------------------------------------------
// 1. Redirects to login when not authenticated
// ---------------------------------------------------------------------------

describe('ProtectedRoute – unauthenticated access', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetAccessToken.mockReturnValue(null);
    mockedGetRefreshToken.mockReturnValue(null);
  });

  it('redirects to /login when the user is not authenticated', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    // The protected content should NOT be rendered.
    expect(screen.queryByTestId('protected-content')).not.toBeInTheDocument();
    // The login page should be rendered instead.
    expect(screen.getByTestId('login-page')).toBeInTheDocument();
  });

  it('does not render children when unauthenticated', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(screen.queryByText('Protected Page')).not.toBeInTheDocument();
  });

  it('redirects to login when isAuthenticated is false and no token exists', () => {
    mockedGetAccessToken.mockReturnValue(null);

    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(screen.getByTestId('login-page')).toBeInTheDocument();
  });

  it('redirects to login when error is present and user is unauthenticated', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: 'Session expired',
    });

    expect(screen.getByTestId('login-page')).toBeInTheDocument();
    expect(screen.queryByTestId('protected-content')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2. Renders children when authenticated
// ---------------------------------------------------------------------------

describe('ProtectedRoute – authenticated access', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetAccessToken.mockReturnValue('valid-token');
    mockedGetRefreshToken.mockReturnValue('refresh-token');
  });

  it('renders children when the user is authenticated', () => {
    renderProtectedRoute({
      isAuthenticated: true,
      loading: 'idle',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    expect(screen.getByTestId('protected-content')).toBeInTheDocument();
    expect(screen.getByText('Protected Page')).toBeInTheDocument();
  });

  it('does not redirect to login when authenticated', () => {
    renderProtectedRoute({
      isAuthenticated: true,
      loading: 'idle',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    expect(screen.queryByTestId('login-page')).not.toBeInTheDocument();
  });

  it('renders multiple children inside the protected route', () => {
    const store = createStore({
      isAuthenticated: true,
      loading: 'idle',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    render(
      <Provider store={store}>
        <MemoryRouter initialEntries={['/dashboard']}>
          <Routes>
            <Route
              path="/dashboard"
              element={
                <ProtectedRoute>
                  <div data-testid="child-a">Child A</div>
                  <div data-testid="child-b">Child B</div>
                </ProtectedRoute>
              }
            />
          </Routes>
        </MemoryRouter>
      </Provider>,
    );

    expect(screen.getByTestId('child-a')).toBeInTheDocument();
    expect(screen.getByTestId('child-b')).toBeInTheDocument();
  });

  it('does not dispatch auth restoration when already authenticated', () => {
    mockedGetAccessToken.mockReturnValue('valid-token');

    renderProtectedRoute({
      isAuthenticated: true,
      loading: 'idle',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    // When already authenticated, no need to restore.
    expect(mockedSetAuthFromStorage).not.toHaveBeenCalled();
    expect(mockedMe).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 3. Shows loading spinner while verifying session
// ---------------------------------------------------------------------------

describe('ProtectedRoute – loading state', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetAccessToken.mockReturnValue(null);
    mockedGetRefreshToken.mockReturnValue(null);
  });

  it('shows a loading spinner when loading is pending and not authenticated', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'pending',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    // The component shows CircularProgress when loading === 'pending' and !isAuthenticated.
    const progressBars = document.querySelectorAll('.MuiCircularProgress-root');
    expect(progressBars.length).toBeGreaterThan(0);
  });

  it('shows "Verifying session…" text while loading', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'pending',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(screen.getByText('Verifying session…')).toBeInTheDocument();
  });

  it('shows the Security icon while loading', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'pending',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    // The Security icon from @mui/icons-material is rendered.
    // MUI renders SVG icons; we check for an SVG element.
    const svgIcons = document.querySelectorAll('svg');
    expect(svgIcons.length).toBeGreaterThan(0);
  });

  it('does not render children while loading', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'pending',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(screen.queryByTestId('protected-content')).not.toBeInTheDocument();
  });

  it('does not redirect to login while loading', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'pending',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(screen.queryByTestId('login-page')).not.toBeInTheDocument();
  });

  it('does not show spinner when loading is not pending', () => {
    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    // With loading !== 'pending', there should be no CircularProgress.
    const progressBars = document.querySelectorAll('.MuiCircularProgress-root');
    expect(progressBars.length).toBe(0);
    // Also, "Verifying session…" should not appear.
    expect(screen.queryByText('Verifying session…')).not.toBeInTheDocument();
  });

  it('does not show spinner when authenticated (even if loading is pending)', () => {
    renderProtectedRoute({
      isAuthenticated: true,
      loading: 'pending',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    // When authenticated, children are rendered — no spinner.
    expect(screen.getByTestId('protected-content')).toBeInTheDocument();
    expect(screen.queryByText('Verifying session…')).not.toBeInTheDocument();
  });

  it('renders content immediately when loading succeeded and user is authenticated', () => {
    renderProtectedRoute({
      isAuthenticated: true,
      loading: 'succeeded',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    expect(screen.getByTestId('protected-content')).toBeInTheDocument();
    expect(screen.queryByText('Verifying session…')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 4. Preserves redirect URL in query params
// ---------------------------------------------------------------------------

describe('ProtectedRoute – redirect URL preservation', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetAccessToken.mockReturnValue(null);
    mockedGetRefreshToken.mockReturnValue(null);
  });

  it('includes the original path in the redirect URL', () => {
    const { locationRef } = renderProtectedRoute(
      {
        isAuthenticated: false,
        loading: 'idle',
        user: null,
        accessToken: null,
        refreshToken: null,
        error: null,
      },
      '/dashboard',
    );

    return waitFor(() => {
      expect(locationRef.current).toContain('/login');
      expect(locationRef.current).toContain('redirect=');
      expect(locationRef.current).toContain(encodeURIComponent('/dashboard'));
    });
  });

  it('does not include expired=1 when there was no stored session', () => {
    const { locationRef } = renderProtectedRoute(
      {
        isAuthenticated: false,
        loading: 'idle',
        user: null,
        accessToken: null,
        refreshToken: null,
        error: null,
      },
      '/dashboard',
    );

    return waitFor(() => {
      expect(locationRef.current).not.toContain('expired=1');
    });
  });

  it('preserves query string in the redirect parameter', () => {
    const { locationRef } = renderProtectedRoute(
      {
        isAuthenticated: false,
        loading: 'idle',
        user: null,
        accessToken: null,
        refreshToken: null,
        error: null,
      },
      '/experiments?page=2&status=running',
    );

    return waitFor(() => {
      // The full path + search is encoded together.
      const expected = encodeURIComponent('/experiments?page=2&status=running');
      expect(locationRef.current).toContain(expected);
    });
  });

  it('preserves a deeply nested path in the redirect parameter', () => {
    const { locationRef } = renderProtectedRoute(
      {
        isAuthenticated: false,
        loading: 'idle',
        user: null,
        accessToken: null,
        refreshToken: null,
        error: null,
      },
      '/settings/notifications',
    );

    return waitFor(() => {
      expect(locationRef.current).toContain(
        encodeURIComponent('/settings/notifications'),
      );
    });
  });

  it('redirects to login even when the original path has no query string', () => {
    const { locationRef } = renderProtectedRoute(
      {
        isAuthenticated: false,
        loading: 'idle',
        user: null,
        accessToken: null,
        refreshToken: null,
        error: null,
      },
      '/profile',
    );

    return waitFor(() => {
      expect(locationRef.current).toMatch(/^\/login\?/);
      expect(locationRef.current).toContain(encodeURIComponent('/profile'));
    });
  });

  it('encodes the redirect parameter correctly', () => {
    const { locationRef } = renderProtectedRoute(
      {
        isAuthenticated: false,
        loading: 'idle',
        user: null,
        accessToken: null,
        refreshToken: null,
        error: null,
      },
      '/experiments/123/logs?tab=errors',
    );

    return waitFor(() => {
      const expected = encodeURIComponent('/experiments/123/logs?tab=errors');
      expect(locationRef.current).toContain(expected);
      expect(locationRef.current).not.toContain('expired=1');
    });
  });
});

// ---------------------------------------------------------------------------
// 5. Auth restoration on mount
// ---------------------------------------------------------------------------

describe('ProtectedRoute – auth restoration', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetRefreshToken.mockReturnValue('stored-refresh-token');
  });

  it('dispatches setAuthFromStorage and me when a token exists but user is not authenticated', () => {
    mockedGetAccessToken.mockReturnValue('stored-token');
    mockedGetRefreshToken.mockReturnValue('stored-refresh-token');

    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(mockedSetAuthFromStorage).toHaveBeenCalled();
    expect(mockedMe).toHaveBeenCalled();
  });

  it('does not dispatch auth restoration when no token is stored', () => {
    mockedGetAccessToken.mockReturnValue(null);
    mockedGetRefreshToken.mockReturnValue(null);

    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(mockedSetAuthFromStorage).not.toHaveBeenCalled();
    expect(mockedMe).not.toHaveBeenCalled();
  });

  it('does not dispatch auth restoration when already authenticated', () => {
    mockedGetAccessToken.mockReturnValue('valid-token');

    renderProtectedRoute({
      isAuthenticated: true,
      loading: 'idle',
      user: { id: '1', name: 'Test User', email: 'test@example.com', role: 'admin' },
      accessToken: 'valid-token',
      refreshToken: 'refresh-token',
      error: null,
    });

    // When already authenticated, no need to restore.
    expect(mockedSetAuthFromStorage).not.toHaveBeenCalled();
    expect(mockedMe).not.toHaveBeenCalled();
  });

  it('does not dispatch auth restoration when loading is already pending', () => {
    mockedGetAccessToken.mockReturnValue('stored-token');
    mockedGetRefreshToken.mockReturnValue('stored-refresh-token');

    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'pending',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    // The useEffect checks `loading !== 'pending'` before dispatching.
    expect(mockedSetAuthFromStorage).not.toHaveBeenCalled();
    expect(mockedMe).not.toHaveBeenCalled();
  });

  it('dispatches both setAuthFromStorage and me with a stored token present', () => {
    mockedGetAccessToken.mockReturnValue('my-access-token');
    mockedGetRefreshToken.mockReturnValue('my-refresh-token');

    renderProtectedRoute({
      isAuthenticated: false,
      loading: 'idle',
      user: null,
      accessToken: null,
      refreshToken: null,
      error: null,
    });

    expect(mockedSetAuthFromStorage).toHaveBeenCalledTimes(1);
    expect(mockedMe).toHaveBeenCalledTimes(1);
  });
});
