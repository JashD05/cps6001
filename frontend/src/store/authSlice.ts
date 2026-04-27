import { createSlice, createAsyncThunk, type PayloadAction } from '@reduxjs/toolkit';
import type { AuthState, User, LoginRequest, LoginResponse } from '@/types';
import { authAPI, setTokens, clearTokens, getRefreshToken } from '@/services/api';

// ---------------------------------------------------------------------------
// Async Thunks
// ---------------------------------------------------------------------------

export const login = createAsyncThunk(
  'auth/login',
  async (credentials: LoginRequest, { rejectWithValue }) => {
    try {
      const response = await authAPI.login(credentials);
      const { accessToken, refreshToken, user } = response.data.data;
      setTokens(accessToken, refreshToken);
      return { accessToken, refreshToken, user } as LoginResponse;
    } catch (error: unknown) {
      let message = 'Login failed. Please check your credentials.';
      if (error && typeof error === 'object' && 'response' in error) {
        const axiosError = error as { response?: { data?: { message?: string } } };
        message = axiosError.response?.data?.message ?? message;
      }
      return rejectWithValue(message);
    }
  },
);

export const logout = createAsyncThunk('auth/logout', async (_, { rejectWithValue }) => {
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
});

export const refreshToken = createAsyncThunk(
  'auth/refreshToken',
  async (_, { rejectWithValue }) => {
    try {
      const currentRefreshToken = getRefreshToken();
      if (!currentRefreshToken) {
        throw new Error('No refresh token available');
      }
      const response = await authAPI.refresh(currentRefreshToken);
      const { accessToken, refreshToken: newRefreshToken } = response.data.data;
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
      return rejectWithValue(message);
    }
  },
);

export const me = createAsyncThunk('auth/me', async (_, { rejectWithValue }) => {
  try {
    const response = await authAPI.me();
    return response.data.data as User;
  } catch (error: unknown) {
    let message = 'Failed to fetch user profile.';
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
  isAuthenticated: true,
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
      state.isAuthenticated = true;
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
        (action.payload as string) ?? action.error.message ?? 'Failed to fetch user';
      if (errorMsg.includes('expired') || errorMsg.includes('401')) {
        state.isAuthenticated = false;
        state.user = null;
        state.accessToken = null;
        state.refreshToken = null;
      }
      state.error = errorMsg;
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
