import { configureStore, combineReducers } from '@reduxjs/toolkit';
import { useDispatch, useSelector, type TypedUseSelectorHook } from 'react-redux';
import authReducer from './authSlice';
import experimentReducer from './experimentSlice';
import type { AuthState, ExperimentListState, ExperimentDetailState } from '@/types';

// ---------------------------------------------------------------------------
// Root Reducer
// ---------------------------------------------------------------------------

const combinedReducer = combineReducers({
  auth: authReducer,
  experiments: experimentReducer,
});

// ---------------------------------------------------------------------------
// Reset-aware Root Reducer
// ---------------------------------------------------------------------------
// Wraps the combined reducer so that dispatching RESET_STORE wipes all state
// back to initial values. This is useful during logout to avoid leaking
// stale data across slices.
// ---------------------------------------------------------------------------

const rootReducer = (
  state: ReturnType<typeof combinedReducer> | undefined,
  action: { type: string },
) => {
  if (action.type === 'RESET_STORE') {
    // Pass `undefined` as state so each slice reducer reverts to its initial state
    return combinedReducer(undefined, action);
  }
  return combinedReducer(state, action);
};

// ---------------------------------------------------------------------------
// Store Configuration
// ---------------------------------------------------------------------------

export const store = configureStore({
  reducer: rootReducer,
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware({
      serializableCheck: {
        ignoredActions: ['auth/login/fulfilled', 'auth/refreshToken/fulfilled'],
        ignoredPaths: ['auth.accessToken', 'auth.refreshToken'],
      },
      immutableCheck: {
        ignoredPaths: ['auth.accessToken', 'auth.refreshToken'],
      },
    }),
  devTools: {
    enabled: import.meta.env.DEV,
    feature: {
      pause: true,
      lock: true,
      persist: true,
      export: true,
      import: 'custom',
      jump: true,
      skip: true,
      reorder: true,
      dispatch: true,
      test: true,
    },
    trace: true,
    traceLimit: 25,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } as any,
});

// ---------------------------------------------------------------------------
// Store Types
// ---------------------------------------------------------------------------

export type RootState = ReturnType<typeof combinedReducer>;
export type AppStore = typeof store;
export type AppDispatch = typeof store.dispatch;

// ---------------------------------------------------------------------------
// State slice types (re-exported for convenience)
// ---------------------------------------------------------------------------

export type { AuthState, ExperimentListState, ExperimentDetailState };

// ---------------------------------------------------------------------------
// Typed Hooks – use these throughout the app instead of plain `useDispatch`
// and `useSelector` for full type safety.
// ---------------------------------------------------------------------------

export const useAppDispatch: () => AppDispatch = useDispatch;
export const useAppSelector: TypedUseSelectorHook<RootState> = useSelector;

// ---------------------------------------------------------------------------
// Store Utilities
// ---------------------------------------------------------------------------

/**
 * Reset the entire store to initial state. Useful for logout.
 */
export const resetStore = () => ({
  type: 'RESET_STORE' as const,
});

export default store;
