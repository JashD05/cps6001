import { createSlice, createAsyncThunk, type PayloadAction } from '@reduxjs/toolkit';
import {
  authAPI,
  setTokens,
  clearTokens,
  getRefreshToken,
  emitAuthSessionExpired,
} from '@/services/api';
import type { AuthState, User, LoginRequest, LoginResponse } from '@/types';

// ---------------------------------------------------------------------------
// API response helpers
// ---------------------------------------------------------------------------

const unwrapResponseData = <T>(value: unknown): T | undefined => {
  if (
    value &&
    typeof value === 'object' &&
    'data' in value &&
    (value as { data?: unknown }).data !== undefined
  ) {
    return (value as { data: T }).data;
  }

  return value as T;
};

const asRecord = (value: unknown): Record<string, unknown> | undefined => {
  const unwrapped = unwrapResponseData<unknown>(value);
  if (!unwrapped || typeof unwrapped !== 'object' || Array.isArray(unwrapped)) {
    return undefined;
  }

  return unwrapped as Record<string, unknown>;
};

const _asUser = (value: unknown): User | undefined => {
  const unwrapped = unwrapResponseData<unknown>(value);
  if (!unwrapped || typeof unwrapped !== 'object' || Array.isArray(unwrapped)) {
    return undefined;
  }

  return unwrapped as User;
};

const notifyAuthSessionExpired = (reason: string): void => {
  if (typeof emitAuthSessionExpired === 'function') {
    emitAuthSessionExpired(reason);
  }
};

// ---------------------------------------------------------------------------
// Async Thunks
// ---------------------------------------------------------------------------

export const login = createAsyncThunk(
  'auth/login',
  async (credentials: LoginRequest, { rejectWithValue }) => {
    try {
      const response = await authAPI.login(credentials);
      const resData = asRecord(response.data) ?? {};
      const accessToken = (resData.accessToken ?? resData.access_token) as string;
      const refreshToken = (resData.refreshToken ?? resData.refresh_token) as string;
      const expiresIn = (resData.expiresIn ?? resData.expires_in ?? 3600) as number;
      const tokenType = (resData.tokenType ?? resData.token_type ?? 'Bearer') as string;

      setTokens(accessToken, refreshToken);

      const directUser = _asUser(resData.user);
      let user: User | null = directUser ?? null;
      if (!user) {
        try {
          const meResponse = await authAPI.me();
          user = _asUser(meResponse.data) ?? (meResponse.data as unknown as User);
        } catch {
          // If /me fails we still proceed — user will be fetched later by ProtectedRoute
        }
      }

      return { accessToken, refreshToken, expiresIn, tokenType, user } as LoginResponse;
    } catch (error: unknown) {
      let message = 'Login failed. Please check your credentials.';
      if (error && typeof error === 'object' && 'response' in error) {
        const axiosError = error as {
          response?: { data?: { message?: string; error?: string } };
        };
        message =
          axiosError.response?.data?.message ??
          axiosError.response?.data?.error ??
          message;
      }
      return rejectWithValue(message);
    }
  },
);

export const register = createAsyncThunk(
  'auth/register',
  async (
    data: { name: string; email: string; password: string; organization: string },
    { rejectWithValue },
  ) => {
    try {
      const response = await authAPI.register(data);
      const resData = asRecord(response.data) ?? {};
      const accessToken = (resData.accessToken ?? resData.access_token) as string;
      const refreshToken = (resData.refreshToken ?? resData.refresh_token) as string;
      const expiresIn = (resData.expiresIn ?? resData.expires_in ?? 3600) as number;
      const tokenType = (resData.tokenType ?? resData.token_type ?? 'Bearer') as string;

      setTokens(accessToken, refreshToken);

      const directUser = _asUser(resData.user);
      let user: User | null = directUser ?? null;
      if (!user) {
        try {
          const meResponse = await authAPI.me();
          user = _asUser(meResponse.data) ?? (meResponse.data as unknown as User);
        } catch {
          // If /me fails we still proceed — user will be fetched later by ProtectedRoute
        }
      }

      return { accessToken, refreshToken, expiresIn, tokenType, user } as LoginResponse;
    } catch (error: unknown) {
      let message = 'Registration failed. Please try again.';
      if (error && typeof error === 'object' && 'response' in error) {
        const axiosError = error as {
          response?: { data?: { message?: string; error?: string } };
        };
        message =
          axiosError.response?.data?.message ??
          axiosError.response?.data?.error ??
          message;
      }
      return rejectWithValue(message);
    }
  },
);

export const logout = createAsyncThunk<void, void>(
  'auth/logout',
  async (_, { rejectWithValue }) => {
    try {
      await authAPI.logout();
      clearTokens();
      return;
    } catch (error: unknown) {
      // Even if the server logout fails, clear local tokens
      clearTokens();
      let message = 'Logout failed on server, but you have been logged out locally.';
      if (error && typeof error === 'object' && 'response' in error) {
        const axiosError = error as { response?: { data?: { message?: string } } };
        message = axiosError.response?.data?.message ?? message;
      }
      return rejectWithValue(message);
    }
  },
);

export const refreshToken = createAsyncThunk(
  'auth/refreshToken',
  async (_, { rejectWithValue }) => {
    try {
      const currentRefreshToken = getRefreshToken();
      if (!currentRefreshToken) {
        throw new Error('No refresh token available');
      }
      const response = await authAPI.refresh(currentRefreshToken);
      const resData = asRecord(response.data) ?? {};
      const accessToken = (resData.accessToken ?? resData.access_token) as string;
      const newRefreshToken = (resData.refreshToken ?? resData.refresh_token) as string;
      setTokens(accessToken, newRefreshToken);
      return { accessToken, refreshToken: newRefreshToken } as {
        accessToken: string;
        refreshToken: string;
      };
    } catch (error: unknown) {
      clearTokens();
      let message = 'Session expired. Please log in again.';
      if (error && typeof error === 'object' && 'response' in error) {
        const axiosError = error as { response?: { data?: { message?: string } } };
        message = axiosError.response?.data?.message ?? message;
      }
      if (error instanceof Error && error.message === 'No refresh token available') {
        message = error.message;
      }
      notifyAuthSessionExpired(message);
      return rejectWithValue(message);
    }
  },
);

export const me = createAsyncThunk('auth/me', async (_, { rejectWithValue }) => {
  try {
    const response = await authAPI.me();
    return _asUser(response.data) ?? (response.data as unknown as User);
  } catch (error: unknown) {
    let message =
      error instanceof Error && error.message
        ? error.message
        : 'Failed to fetch user profile.';
    if (error && typeof error === 'object' && 'response' in error) {
      const axiosError = error as {
        response?: { data?: { message?: string }; status?: number };
      };
      if (axiosError.response?.status === 401) {
        clearTokens();
        message = 'Session expired. Please log in again.';
      } else {
        message = axiosError.response?.data?.message ?? message;
      }
    }
    return rejectWithValue(message);
  }
});

// ---------------------------------------------------------------------------
// Initial State
// ---------------------------------------------------------------------------

const initialState: AuthState = {
  user: null,
  accessToken: null,
  refreshToken: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

// ---------------------------------------------------------------------------
// Slice
// ---------------------------------------------------------------------------

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    clearError(state) {
      state.error = null;
    },
    setAuthFromStorage(
      state,
      action: PayloadAction<{ accessToken: string; refreshToken: string }>,
    ) {
      state.accessToken = action.payload.accessToken;
      state.refreshToken = action.payload.refreshToken;
      state.isAuthenticated = true;
    },
    clearAuth(state) {
      state.user = null;
      state.accessToken = null;
      state.refreshToken = null;
      state.isAuthenticated = false;
      state.error = null;
      state.isLoading = false;
    },
    updateUserProfile(state, action: PayloadAction<Partial<User>>) {
      if (state.user) {
        state.user = { ...state.user, ...action.payload };
      }
    },
  },
  extraReducers: (builder) => {
    // Login
    builder.addCase(login.pending, (state) => {
      state.isLoading = true;
      state.error = null;
    });
    builder.addCase(login.fulfilled, (state, action: PayloadAction<LoginResponse>) => {
      state.isLoading = false;
      state.isAuthenticated = action.payload.user !== null;
      state.user = action.payload.user;
      state.accessToken = action.payload.accessToken;
      state.refreshToken = action.payload.refreshToken;
      state.error = null;
    });
    builder.addCase(login.rejected, (state, action) => {
      state.isLoading = false;
      state.isAuthenticated = false;
      state.user = null;
      state.accessToken = null;
      state.refreshToken = null;
      state.error = (action.payload as string) ?? action.error.message ?? 'Login failed';
    });

    // Register
    builder.addCase(register.pending, (state) => {
      state.isLoading = true;
      state.error = null;
    });
    builder.addCase(register.fulfilled, (state, action: PayloadAction<LoginResponse>) => {
      state.isLoading = false;
      state.user = action.payload.user;
      state.isAuthenticated = true;
      state.accessToken = action.payload.accessToken;
      state.refreshToken = action.payload.refreshToken;
      state.error = null;
    });
    builder.addCase(register.rejected, (state, action) => {
      state.isLoading = false;
      state.error =
        (action.payload as string) ?? action.error.message ?? 'Registration failed';
    });

    // Logout
    builder.addCase(logout.pending, (state) => {
      state.isLoading = true;
    });
    builder.addCase(logout.fulfilled, (state) => {
      state.isLoading = false;
      state.isAuthenticated = false;
      state.user = null;
      state.accessToken = null;
      state.refreshToken = null;
      state.error = null;
    });
    builder.addCase(logout.rejected, (state, action) => {
      // Even on rejection, clear auth state (tokens already cleared in thunk)
      state.isLoading = false;
      state.isAuthenticated = false;
      state.user = null;
      state.accessToken = null;
      state.refreshToken = null;
      state.error = (action.payload as string) ?? null;
    });

    // Refresh Token
    builder.addCase(refreshToken.pending, (state) => {
      state.isLoading = true;
      state.error = null;
    });
    builder.addCase(refreshToken.fulfilled, (state, action) => {
      state.isLoading = false;
      state.accessToken = action.payload.accessToken;
      state.refreshToken = action.payload.refreshToken;
      state.isAuthenticated = true;
      state.error = null;
    });
    builder.addCase(refreshToken.rejected, (state, action) => {
      state.isLoading = false;
      state.isAuthenticated = false;
      state.user = null;
      state.accessToken = null;
      state.refreshToken = null;
      state.error =
        (action.payload as string) ?? action.error.message ?? 'Token refresh failed';
    });

    // Me (fetch current user)
    builder.addCase(me.pending, (state) => {
      state.isLoading = true;
      state.error = null;
    });
    builder.addCase(me.fulfilled, (state, action: PayloadAction<User>) => {
      state.isLoading = false;
      state.user = action.payload;
      state.isAuthenticated = true;
      state.error = null;
    });
    builder.addCase(me.rejected, (state, action) => {
      state.isLoading = false;
      // If fetching user fails with 401, we are no longer authenticated
      const errorMsg =
        (action.payload as string) ??
        action.error.message ??
        'Failed to fetch user profile.';
      if (errorMsg.includes('expired') || errorMsg.includes('401')) {
        state.isAuthenticated = false;
        state.user = null;
        state.accessToken = null;
        state.refreshToken = null;
      }
      state.error = errorMsg || 'Failed to fetch user profile.';
    });
  },
});

// ---------------------------------------------------------------------------
// Actions & Reducer Exports
// ---------------------------------------------------------------------------

export const { clearError, setAuthFromStorage, clearAuth, updateUserProfile } =
  authSlice.actions;

export const selectAuth = (state: { auth: AuthState }): AuthState => state.auth;
export const selectIsAuthenticated = (state: { auth: AuthState }): boolean =>
  state.auth.isAuthenticated;
export const selectCurrentUser = (state: { auth: AuthState }): User | null =>
  state.auth.user;
export const selectAuthLoading = (state: { auth: AuthState }): boolean =>
  state.auth.isLoading;
export const selectAuthError = (state: { auth: AuthState }): string | null =>
  state.auth.error;
export const selectUserRole = (state: { auth: AuthState }): string | null =>
  state.auth.user?.role ?? null;

export default authSlice.reducer;
