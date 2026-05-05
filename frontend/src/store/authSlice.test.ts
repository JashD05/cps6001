/**
 * Unit tests for the auth Redux slice.
 *
 * Tests the slice reducer logic, async thunks (login, logout,
 * refreshToken, me), and synchronous actions (clearAuth,
 * setAuthFromStorage, clearError, updateUserProfile) using a real
 * Redux store backed by the authSlice reducer.
 */

import { configureStore } from '@reduxjs/toolkit';
import { authAPI, setTokens, clearTokens, getRefreshToken } from '@/services/api';
import authReducer, {
  login,
  logout,
  refreshToken,
  me,
  clearAuth,
  clearError,
  setAuthFromStorage,
  updateUserProfile,
} from '@/store/authSlice';
import type { AuthState, User, LoginResponse } from '@/types';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock('@/services/api', () => ({
  authAPI: {
    login: jest.fn(),
    logout: jest.fn(),
    refresh: jest.fn(),
    me: jest.fn(),
  },
  setTokens: jest.fn(),
  clearTokens: jest.fn(),
  getAccessToken: jest.fn(),
  getRefreshToken: jest.fn(),
}));

const mockedAuthAPI = authAPI as jest.Mocked<typeof authAPI>;
const mockedSetTokens = setTokens as jest.MockedFunction<typeof setTokens>;
const mockedClearTokens = clearTokens as jest.MockedFunction<typeof clearTokens>;
const mockedGetRefreshToken = getRefreshToken as jest.MockedFunction<
  typeof getRefreshToken
>;

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const mockUser: User = {
  id: 'user-1',
  email: 'admin@chaos-sec.io',
  name: 'Admin User',
  role: 'admin',
  createdAt: '2024-01-15T10:00:00Z',
  updatedAt: '2024-06-01T12:00:00Z',
  lastLoginAt: '2024-06-10T08:30:00Z',
};

const mockLoginResponse: LoginResponse = {
  accessToken: 'access-token-abc123',
  refreshToken: 'refresh-token-xyz789',
  expiresIn: 3600,
  tokenType: 'Bearer',
  user: mockUser,
};

/**
 * Helper to wrap data in an APIResponse envelope for mock returns.
 * Cast to `never` because `mockResolvedValueOnce` enforces the full
 * AxiosResponse shape, but only `response.data.data` is used by the thunks.
 */
function apiResponse<T>(data: T) {
  return { data: { success: true as const, data } } as never;
}

// ---------------------------------------------------------------------------
// Store helpers
// ---------------------------------------------------------------------------

/**
 * Create a real Redux store with the authSlice reducer.
 * Optionally provide initial auth state overrides.
 */
function createTestStore(overrides?: Partial<AuthState>) {
  const preloadedState = overrides
    ? { auth: { ...initialState, ...overrides } }
    : undefined;
  return configureStore({
    reducer: { auth: authReducer },
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware({
        serializableCheck: false,
        immutableCheck: false,
      }),
    preloadedState,
  });
}

type _TestStore = ReturnType<typeof createTestStore>;

/** Grab the initial state directly from the slice definition. */
let initialState: AuthState;

beforeEach(() => {
  // Reset the initial state from the reducer so we always have a fresh copy.
  initialState = authReducer(undefined, { type: '@@INIT' }) as AuthState;
  jest.clearAllMocks();
});

// ---------------------------------------------------------------------------
// 1. Initial state
// ---------------------------------------------------------------------------

describe('authSlice – initial state', () => {
  it('has the correct default values', () => {
    expect(initialState.user).toBeNull();
    expect(initialState.accessToken).toBeNull();
    expect(initialState.refreshToken).toBeNull();
    expect(initialState.isAuthenticated).toBe(false);
    expect(initialState.isLoading).toBe(false);
    expect(initialState.error).toBeNull();
  });

  it('returns the same state for an unknown action', () => {
    const state = authReducer(initialState, { type: 'unknown/action' });
    expect(state).toEqual(initialState);
  });
});

// ---------------------------------------------------------------------------
// 2. login async thunk
// ---------------------------------------------------------------------------

describe('authSlice – login', () => {
  const credentials = { email: 'admin@chaos-sec.io', password: 'secret123' };

  it('sets isLoading to true on pending', () => {
    const state = authReducer(initialState, { type: login.pending.type });
    expect(state.isLoading).toBe(true);
    expect(state.error).toBeNull();
  });

  it('updates state on fulfilled', () => {
    const action = {
      type: login.fulfilled.type,
      payload: mockLoginResponse,
    };
    const state = authReducer(initialState, action);

    expect(state.isLoading).toBe(false);
    expect(state.isAuthenticated).toBe(true);
    expect(state.user).toEqual(mockUser);
    expect(state.accessToken).toBe('access-token-abc123');
    expect(state.refreshToken).toBe('refresh-token-xyz789');
    expect(state.error).toBeNull();
  });

  it('calls setTokens with the received tokens on fulfilled', async () => {
    mockedAuthAPI.login.mockResolvedValueOnce(apiResponse(mockLoginResponse));

    const store = createTestStore();
    await store.dispatch(login(credentials));

    expect(mockedSetTokens).toHaveBeenCalledWith(
      'access-token-abc123',
      'refresh-token-xyz789',
    );
  });

  it('updates state on rejected with server error message', () => {
    const action = {
      type: login.rejected.type,
      payload: 'Invalid credentials',
      error: { message: 'Rejected' },
    };
    const state = authReducer(initialState, action);

    expect(state.isLoading).toBe(false);
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.error).toBe('Invalid credentials');
  });

  it('uses default error message when payload is undefined', () => {
    const action = {
      type: login.rejected.type,
      payload: undefined,
      error: { message: 'Network Error' },
    };
    const state = authReducer(initialState, action);

    expect(state.error).toBe('Network Error');
  });

  it('falls back to "Login failed" when both payload and error message are missing', () => {
    const action = {
      type: login.rejected.type,
      payload: undefined,
      error: { message: undefined },
    };
    const state = authReducer(initialState, action);

    expect(state.error).toBe('Login failed');
  });

  it('integrates login thunk end-to-end – success', async () => {
    mockedAuthAPI.login.mockResolvedValueOnce(apiResponse(mockLoginResponse));

    const store = createTestStore();
    const result = await store.dispatch(login(credentials));

    expect(result.type).toBe('auth/login/fulfilled');
    const { auth } = store.getState();
    expect(auth.isAuthenticated).toBe(true);
    expect(auth.user).toEqual(mockUser);
    expect(auth.accessToken).toBe('access-token-abc123');
    expect(auth.refreshToken).toBe('refresh-token-xyz789');
    expect(auth.isLoading).toBe(false);
    expect(auth.error).toBeNull();
  });

  it('integrates login thunk end-to-end – failure', async () => {
    const axiosError = {
      response: { data: { message: 'Invalid email or password' } },
    };
    mockedAuthAPI.login.mockRejectedValueOnce(axiosError);

    const store = createTestStore();
    const result = await store.dispatch(login(credentials));

    expect(result.type).toBe('auth/login/rejected');
    const { auth } = store.getState();
    expect(auth.isAuthenticated).toBe(false);
    expect(auth.user).toBeNull();
    expect(auth.accessToken).toBeNull();
    expect(auth.refreshToken).toBeNull();
    expect(auth.error).toBe('Invalid email or password');
  });
});

// ---------------------------------------------------------------------------
// 3. logout async thunk
// ---------------------------------------------------------------------------

describe('authSlice – logout', () => {
  const authenticatedState: AuthState = {
    user: mockUser,
    accessToken: 'access-token-abc123',
    refreshToken: 'refresh-token-xyz789',
    isAuthenticated: true,
    isLoading: false,
    error: null,
  };

  it('sets isLoading to true on pending', () => {
    const state = authReducer(authenticatedState, { type: logout.pending.type });
    expect(state.isLoading).toBe(true);
  });

  it('clears auth state on fulfilled', () => {
    const action = { type: logout.fulfilled.type };
    const state = authReducer(authenticatedState, action);

    expect(state.isLoading).toBe(false);
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.error).toBeNull();
  });

  it('calls clearTokens on fulfilled', async () => {
    mockedAuthAPI.logout.mockResolvedValueOnce(apiResponse(undefined as unknown as void));

    const store = createTestStore(authenticatedState);
    await store.dispatch(logout());

    expect(mockedClearTokens).toHaveBeenCalled();
  });

  it('clears auth state even on rejected (server failure)', () => {
    const action = {
      type: logout.rejected.type,
      payload: 'Logout failed on server, but you have been logged out locally.',
      error: { message: 'Rejected' },
    };
    const state = authReducer(authenticatedState, action);

    expect(state.isLoading).toBe(false);
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
    // Rejected logout stores the error message but still clears auth
    expect(state.error).toBe(
      'Logout failed on server, but you have been logged out locally.',
    );
  });

  it('uses null error when rejected without payload', () => {
    const action = {
      type: logout.rejected.type,
      payload: undefined,
      error: { message: undefined },
    };
    const state = authReducer(authenticatedState, action);

    expect(state.isAuthenticated).toBe(false);
    expect(state.error).toBeNull();
  });

  it('clears tokens even when the server request fails', async () => {
    mockedAuthAPI.logout.mockRejectedValueOnce(new Error('Network error') as never);

    const store = createTestStore(authenticatedState);
    await store.dispatch(logout());

    // clearTokens is called inside the thunk before the API call can fail
    expect(mockedClearTokens).toHaveBeenCalled();
  });

  it('integrates logout thunk end-to-end – success', async () => {
    mockedAuthAPI.logout.mockResolvedValueOnce(apiResponse(undefined as unknown as void));

    const store = createTestStore(authenticatedState);
    const result = await store.dispatch(logout());

    expect(result.type).toBe('auth/logout/fulfilled');
    const { auth } = store.getState();
    expect(auth.isAuthenticated).toBe(false);
    expect(auth.user).toBeNull();
    expect(auth.accessToken).toBeNull();
    expect(auth.refreshToken).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 4. refreshToken async thunk
// ---------------------------------------------------------------------------

describe('authSlice – refreshToken', () => {
  const authenticatedState: AuthState = {
    user: mockUser,
    accessToken: 'old-access-token',
    refreshToken: 'old-refresh-token',
    isAuthenticated: true,
    isLoading: false,
    error: null,
  };

  const refreshPayload: Pick<LoginResponse, 'accessToken' | 'refreshToken'> = {
    accessToken: 'new-access-token',
    refreshToken: 'new-refresh-token',
  };

  it('sets isLoading to true and clears error on pending', () => {
    const stateWithError: AuthState = {
      ...authenticatedState,
      error: 'Previous error',
    };
    const state = authReducer(stateWithError, { type: refreshToken.pending.type });
    expect(state.isLoading).toBe(true);
    expect(state.error).toBeNull();
  });

  it('updates tokens on fulfilled', () => {
    const action = {
      type: refreshToken.fulfilled.type,
      payload: refreshPayload,
    };
    const state = authReducer(authenticatedState, action);

    expect(state.isLoading).toBe(false);
    expect(state.accessToken).toBe('new-access-token');
    expect(state.refreshToken).toBe('new-refresh-token');
    expect(state.isAuthenticated).toBe(true);
    expect(state.error).toBeNull();
  });

  it('calls setTokens with new tokens on fulfilled', async () => {
    mockedGetRefreshToken.mockReturnValueOnce('old-refresh-token');
    mockedAuthAPI.refresh.mockResolvedValueOnce(
      apiResponse({ ...mockLoginResponse, ...refreshPayload }),
    );

    const store = createTestStore(authenticatedState);
    await store.dispatch(refreshToken());

    expect(mockedSetTokens).toHaveBeenCalledWith('new-access-token', 'new-refresh-token');
  });

  it('clears auth state on rejected', () => {
    const action = {
      type: refreshToken.rejected.type,
      payload: 'Session expired. Please log in again.',
      error: { message: 'Rejected' },
    };
    const state = authReducer(authenticatedState, action);

    expect(state.isLoading).toBe(false);
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.error).toBe('Session expired. Please log in again.');
  });

  it('calls clearTokens on rejected', async () => {
    mockedGetRefreshToken.mockReturnValueOnce('old-refresh-token');
    mockedAuthAPI.refresh.mockRejectedValueOnce(new Error('Token expired'));

    const store = createTestStore(authenticatedState);
    await store.dispatch(refreshToken());

    expect(mockedClearTokens).toHaveBeenCalled();
  });

  it('rejects when no refresh token is available', async () => {
    mockedGetRefreshToken.mockReturnValueOnce(null);

    const store = createTestStore(authenticatedState);
    const result = await store.dispatch(refreshToken());

    expect(result.type).toBe('auth/refreshToken/rejected');
    const { auth } = store.getState();
    expect(auth.error).toBe('No refresh token available');
    expect(auth.isAuthenticated).toBe(false);
  });

  it('uses default error message when payload is missing', () => {
    const action = {
      type: refreshToken.rejected.type,
      payload: undefined,
      error: { message: 'Network failure' },
    };
    const state = authReducer(authenticatedState, action);

    expect(state.error).toBe('Network failure');
  });

  it('falls back to "Token refresh failed" when no error info', () => {
    const action = {
      type: refreshToken.rejected.type,
      payload: undefined,
      error: { message: undefined },
    };
    const state = authReducer(authenticatedState, action);

    expect(state.error).toBe('Token refresh failed');
  });

  it('integrates refreshToken thunk end-to-end – success', async () => {
    mockedGetRefreshToken.mockReturnValueOnce('old-refresh-token');
    mockedAuthAPI.refresh.mockResolvedValueOnce(
      apiResponse({ ...mockLoginResponse, ...refreshPayload }),
    );

    const store = createTestStore(authenticatedState);
    const result = await store.dispatch(refreshToken());

    expect(result.type).toBe('auth/refreshToken/fulfilled');
    const { auth } = store.getState();
    expect(auth.accessToken).toBe('new-access-token');
    expect(auth.refreshToken).toBe('new-refresh-token');
    expect(auth.isAuthenticated).toBe(true);
    expect(auth.isLoading).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 5. me (get current user) async thunk
// ---------------------------------------------------------------------------

describe('authSlice – me', () => {
  const stateWithTokens: AuthState = {
    user: null,
    accessToken: 'valid-access-token',
    refreshToken: 'valid-refresh-token',
    isAuthenticated: true,
    isLoading: false,
    error: null,
  };

  it('sets isLoading to true and clears error on pending', () => {
    const stateWithError: AuthState = {
      ...stateWithTokens,
      error: 'Old error',
    };
    const state = authReducer(stateWithError, { type: me.pending.type });
    expect(state.isLoading).toBe(true);
    expect(state.error).toBeNull();
  });

  it('updates user and isAuthenticated on fulfilled', () => {
    const action = {
      type: me.fulfilled.type,
      payload: mockUser,
    };
    const state = authReducer(stateWithTokens, action);

    expect(state.isLoading).toBe(false);
    expect(state.user).toEqual(mockUser);
    expect(state.isAuthenticated).toBe(true);
    expect(state.error).toBeNull();
  });

  it('integrates me thunk end-to-end – success', async () => {
    mockedAuthAPI.me.mockResolvedValueOnce(apiResponse(mockUser));

    const store = createTestStore(stateWithTokens);
    const result = await store.dispatch(me());

    expect(result.type).toBe('auth/me/fulfilled');
    const { auth } = store.getState();
    expect(auth.user).toEqual(mockUser);
    expect(auth.isAuthenticated).toBe(true);
    expect(auth.isLoading).toBe(false);
  });

  it('sets error on rejected', () => {
    const action = {
      type: me.rejected.type,
      payload: 'Failed to fetch user profile.',
      error: { message: 'Rejected' },
    };
    const state = authReducer(stateWithTokens, action);

    expect(state.isLoading).toBe(false);
    expect(state.error).toBe('Failed to fetch user profile.');
  });

  it('clears auth state when rejection indicates session expiry', () => {
    const action = {
      type: me.rejected.type,
      payload: 'Session expired. Please log in again.',
      error: { message: 'Rejected' },
    };
    const state = authReducer(stateWithTokens, action);

    expect(state.isLoading).toBe(false);
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.error).toBe('Session expired. Please log in again.');
  });

  it('clears auth state when rejection contains "401"', () => {
    const action = {
      type: me.rejected.type,
      payload: 'Request failed with status code 401',
      error: { message: 'Rejected' },
    };
    const state = authReducer(stateWithTokens, action);

    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
  });

  it('does NOT clear auth when rejection is not session-related', () => {
    const action = {
      type: me.rejected.type,
      payload: 'Network error',
      error: { message: 'Rejected' },
    };
    const state = authReducer(stateWithTokens, action);

    // Error message doesn't contain "expired" or "401" so auth is preserved
    expect(state.isAuthenticated).toBe(true);
    expect(state.accessToken).toBe('valid-access-token');
    expect(state.refreshToken).toBe('valid-refresh-token');
    expect(state.error).toBe('Network error');
  });

  it('calls clearTokens on 401 rejection', async () => {
    const axiosError = { response: { status: 401, data: { message: 'Unauthorized' } } };
    mockedAuthAPI.me.mockRejectedValueOnce(axiosError);

    const store = createTestStore(stateWithTokens);
    await store.dispatch(me());

    expect(mockedClearTokens).toHaveBeenCalled();
  });

  it('uses default error message when payload is missing', () => {
    const action = {
      type: me.rejected.type,
      payload: undefined,
      error: { message: undefined },
    };
    const state = authReducer(stateWithTokens, action);

    expect(state.error).toBe('Failed to fetch user profile.');
  });

  it('falls back to error.message when payload is absent', () => {
    const action = {
      type: me.rejected.type,
      payload: undefined,
      error: { message: 'Something went wrong' },
    };
    const state = authReducer(stateWithTokens, action);

    expect(state.error).toBe('Something went wrong');
  });
});

// ---------------------------------------------------------------------------
// 6. clearAuth action
// ---------------------------------------------------------------------------

describe('authSlice – clearAuth', () => {
  it('resets all auth fields to their default values', () => {
    const authenticatedState: AuthState = {
      user: mockUser,
      accessToken: 'some-token',
      refreshToken: 'some-refresh-token',
      isAuthenticated: true,
      isLoading: true,
      error: 'Some error',
    };

    const state = authReducer(authenticatedState, clearAuth());

    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.refreshToken).toBeNull();
    expect(state.isAuthenticated).toBe(false);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
  });

  it('works via dispatched action through the store', () => {
    const store = createTestStore({
      user: mockUser,
      accessToken: 'token',
      refreshToken: 'refresh',
      isAuthenticated: true,
      isLoading: false,
      error: null,
    });

    store.dispatch(clearAuth());
    const { auth } = store.getState();

    expect(auth.user).toBeNull();
    expect(auth.accessToken).toBeNull();
    expect(auth.refreshToken).toBeNull();
    expect(auth.isAuthenticated).toBe(false);
    expect(auth.isLoading).toBe(false);
    expect(auth.error).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 7. setAuthFromStorage action
// ---------------------------------------------------------------------------

describe('authSlice – setAuthFromStorage', () => {
  it('sets tokens and isAuthenticated flag', () => {
    const state = authReducer(
      initialState,
      setAuthFromStorage({
        accessToken: 'restored-access-token',
        refreshToken: 'restored-refresh-token',
      }),
    );

    expect(state.accessToken).toBe('restored-access-token');
    expect(state.refreshToken).toBe('restored-refresh-token');
    expect(state.isAuthenticated).toBe(true);
  });

  it('preserves other state fields', () => {
    const stateWithError: AuthState = {
      ...initialState,
      user: mockUser,
      error: 'some error',
    };

    const state = authReducer(
      stateWithError,
      setAuthFromStorage({
        accessToken: 'new-access',
        refreshToken: 'new-refresh',
      }),
    );

    // user and error should remain unchanged
    expect(state.user).toEqual(mockUser);
    expect(state.error).toBe('some error');
  });

  it('works via dispatched action through the store', () => {
    const store = createTestStore();
    store.dispatch(
      setAuthFromStorage({
        accessToken: 'stored-access',
        refreshToken: 'stored-refresh',
      }),
    );

    const { auth } = store.getState();
    expect(auth.accessToken).toBe('stored-access');
    expect(auth.refreshToken).toBe('stored-refresh');
    expect(auth.isAuthenticated).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 8. clearError action
// ---------------------------------------------------------------------------

describe('authSlice – clearError', () => {
  it('clears the error field', () => {
    const stateWithError: AuthState = {
      ...initialState,
      error: 'Login failed. Please check your credentials.',
    };

    const state = authReducer(stateWithError, clearError());
    expect(state.error).toBeNull();
  });

  it('does not affect other state fields', () => {
    const stateWithAuth: AuthState = {
      user: mockUser,
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
      isAuthenticated: true,
      isLoading: false,
      error: 'Something went wrong',
    };

    const state = authReducer(stateWithAuth, clearError());

    expect(state.user).toEqual(mockUser);
    expect(state.accessToken).toBe('access-token');
    expect(state.refreshToken).toBe('refresh-token');
    expect(state.isAuthenticated).toBe(true);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
  });

  it('is idempotent when error is already null', () => {
    const state = authReducer(initialState, clearError());
    expect(state.error).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 9. updateUserProfile action
// ---------------------------------------------------------------------------

describe('authSlice – updateUserProfile', () => {
  const stateWithUser: AuthState = {
    ...initialState,
    user: mockUser,
    isAuthenticated: true,
  };

  it('updates specified user fields', () => {
    const state = authReducer(
      stateWithUser,
      updateUserProfile({ name: 'Updated Name', email: 'new@email.com' }),
    );

    expect(state.user?.name).toBe('Updated Name');
    expect(state.user?.email).toBe('new@email.com');
    // Unchanged fields are preserved
    expect(state.user?.id).toBe(mockUser.id);
    expect(state.user?.role).toBe(mockUser.role);
  });

  it('does nothing when user is null', () => {
    const stateWithoutUser: AuthState = {
      ...initialState,
      user: null,
    };

    const state = authReducer(stateWithoutUser, updateUserProfile({ name: 'New Name' }));

    expect(state.user).toBeNull();
  });

  it('preserves fields not included in the update', () => {
    const state = authReducer(
      stateWithUser,
      updateUserProfile({ avatarUrl: 'https://example.com/avatar.png' }),
    );

    expect(state.user?.avatarUrl).toBe('https://example.com/avatar.png');
    expect(state.user?.name).toBe(mockUser.name);
    expect(state.user?.email).toBe(mockUser.email);
    expect(state.user?.role).toBe(mockUser.role);
  });
});

// ---------------------------------------------------------------------------
// 10. Integration – login → authenticated → logout → unauthenticated
// ---------------------------------------------------------------------------

describe('authSlice – integration flow', () => {
  it('transitions through login → authenticated → logout → unauthenticated', async () => {
    mockedAuthAPI.login.mockResolvedValueOnce(apiResponse(mockLoginResponse));
    mockedAuthAPI.logout.mockResolvedValueOnce(apiResponse(undefined as unknown as void));

    const store = createTestStore();

    // ---- Login ----
    const loginResult = await store.dispatch(
      login({ email: 'admin@chaos-sec.io', password: 'secret123' }),
    );
    expect(loginResult.type).toBe('auth/login/fulfilled');

    let { auth } = store.getState();
    expect(auth.isAuthenticated).toBe(true);
    expect(auth.user).toEqual(mockUser);
    expect(auth.accessToken).toBe('access-token-abc123');
    expect(auth.isLoading).toBe(false);
    expect(auth.error).toBeNull();

    // ---- Logout ----
    const logoutResult = await store.dispatch(logout());
    expect(logoutResult.type).toBe('auth/logout/fulfilled');

    ({ auth } = store.getState());
    expect(auth.isAuthenticated).toBe(false);
    expect(auth.user).toBeNull();
    expect(auth.accessToken).toBeNull();
    expect(auth.refreshToken).toBeNull();
    expect(auth.error).toBeNull();
  });

  it('handles login failure followed by successful login', async () => {
    // First login attempt fails
    mockedAuthAPI.login.mockRejectedValueOnce({
      response: { data: { message: 'Invalid credentials' } },
    });

    // Second login attempt succeeds
    mockedAuthAPI.login.mockResolvedValueOnce(apiResponse(mockLoginResponse));

    const store = createTestStore();

    // First attempt – failure
    const failResult = await store.dispatch(
      login({ email: 'admin@chaos-sec.io', password: 'wrong' }),
    );
    expect(failResult.type).toBe('auth/login/rejected');

    let { auth } = store.getState();
    expect(auth.isAuthenticated).toBe(false);
    expect(auth.error).toBe('Invalid credentials');

    // Second attempt – success
    const successResult = await store.dispatch(
      login({ email: 'admin@chaos-sec.io', password: 'secret123' }),
    );
    expect(successResult.type).toBe('auth/login/fulfilled');

    ({ auth } = store.getState());
    expect(auth.isAuthenticated).toBe(true);
    expect(auth.user).toEqual(mockUser);
    expect(auth.error).toBeNull();
  });

  it('preserves auth state when me fetch fails with non-session error', async () => {
    const stateWithAuth: AuthState = {
      user: mockUser,
      accessToken: 'valid-token',
      refreshToken: 'valid-refresh',
      isAuthenticated: true,
      isLoading: false,
      error: null,
    };

    mockedAuthAPI.me.mockRejectedValueOnce(new Error('Network timeout'));

    const store = createTestStore(stateWithAuth);
    await store.dispatch(me());

    const { auth } = store.getState();
    // A network error should NOT clear auth state
    expect(auth.isAuthenticated).toBe(true);
    expect(auth.user).toEqual(mockUser);
    expect(auth.accessToken).toBe('valid-token');
    expect(auth.error).toBe('Network timeout');
  });

  it('clears auth state when me fetch fails with 401', async () => {
    const stateWithAuth: AuthState = {
      user: mockUser,
      accessToken: 'expired-token',
      refreshToken: 'expired-refresh',
      isAuthenticated: true,
      isLoading: false,
      error: null,
    };

    mockedAuthAPI.me.mockRejectedValueOnce({
      response: { status: 401, data: { message: 'Unauthorized' } },
    });

    const store = createTestStore(stateWithAuth);
    await store.dispatch(me());

    const { auth } = store.getState();
    // A 401 should clear auth state
    expect(auth.isAuthenticated).toBe(false);
    expect(auth.user).toBeNull();
    expect(auth.accessToken).toBeNull();
    expect(auth.refreshToken).toBeNull();
  });
});
