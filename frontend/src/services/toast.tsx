/**
 * Global toast notification system for Chaos-Sec.
 *
 * Provides:
 *  - React context + provider for app-wide toast state
 *  - `useToast` hook for programmatic toast dispatch from any component
 *  - `ToastProvider` component to mount at the app root
 *  - Configurable severity, duration, position, and actions
 *  - Auto-dismiss with manual close
 *  - Stacking with limit to prevent overflow
 */

import { Close } from '@mui/icons-material';
import {
  Snackbar,
  Alert,
  AlertTitle,
  IconButton,
  Slide,
  SlideProps,
  Stack,
  Button,
  Typography,
} from '@mui/material';
import React, {
  createContext,
  useCallback,
  useContext,
  useState,
  useRef,
  useMemo,
  forwardRef,
} from 'react';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ToastSeverity = 'success' | 'error' | 'warning' | 'info';

export type ToastPosition =
  | 'top-right'
  | 'top-left'
  | 'top-center'
  | 'bottom-right'
  | 'bottom-left'
  | 'bottom-center';

export interface ToastAction {
  label: string;
  onClick: () => void;
  color?: 'primary' | 'secondary' | 'inherit' | 'error' | 'info' | 'success' | 'warning';
}

export interface ToastMessage {
  /** Unique ID – auto-generated if not provided */
  id: string;
  /** Severity / variant */
  severity: ToastSeverity;
  /** Short title shown in bold */
  title?: string;
  /** Body text */
  message: string;
  /** Auto-dismiss duration in ms (0 = manual close only, default 6000) */
  duration?: number;
  /** Position override for this specific toast */
  position?: ToastPosition;
  /** Optional action button */
  action?: ToastAction;
  /** Whether to show the close button (default true) */
  dismissible?: boolean;
  /** Timestamp when the toast was created */
  createdAt: number;
  /** Whether the toast is currently exiting (for animation) */
  exiting?: boolean;
}

export interface ToastContextValue {
  /** Current toasts in the stack */
  toasts: ToastMessage[];
  /** Add a toast – returns the toast ID for programmatic dismissal */
  showToast: (options: ShowToastOptions) => string;
  /** Dismiss a specific toast by ID */
  dismissToast: (id: string) => void;
  /** Dismiss all active toasts */
  dismissAll: () => void;
  /** Convenience helpers */
  success: (message: string, options?: Partial<ShowToastOptions>) => string;
  error: (message: string, options?: Partial<ShowToastOptions>) => string;
  warning: (message: string, options?: Partial<ShowToastOptions>) => string;
  info: (message: string, options?: Partial<ShowToastOptions>) => string;
}

export interface ShowToastOptions {
  severity?: ToastSeverity;
  title?: string;
  message: string;
  duration?: number;
  position?: ToastPosition;
  action?: ToastAction;
  dismissible?: boolean;
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

const DEFAULT_DURATION = 6000;
const MAX_TOASTS = 5;
const DEFAULT_POSITION: ToastPosition = 'bottom-right';

let toastCounter = 0;

function generateId(): string {
  toastCounter += 1;
  return `toast-${Date.now()}-${toastCounter}`;
}

// ---------------------------------------------------------------------------
// Slide transition
// ---------------------------------------------------------------------------

type SlideDirection = 'up' | 'down' | 'left' | 'right';

function getSlideDirection(position: ToastPosition): SlideDirection {
  if (position.startsWith('bottom')) return 'up';
  if (position.startsWith('top')) return 'down';
  return 'left';
}

const _SlideTransition = forwardRef(function SlideTransition(
  props: SlideProps & { position?: ToastPosition },
  ref: React.Ref<unknown>,
) {
  const direction = getSlideDirection(props.position || DEFAULT_POSITION);
  return <Slide {...props} ref={ref} direction={direction} />;
});

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const ToastContext = createContext<ToastContextValue | null>(null);

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

interface ToastProviderProps {
  children: React.ReactNode;
  /** Global default position (default 'bottom-right') */
  defaultPosition?: ToastPosition;
  /** Global default auto-dismiss duration in ms (default 6000) */
  defaultDuration?: number;
  /** Maximum number of toasts visible at once (default 5) */
  maxToasts?: number;
}

export const ToastProvider: React.FC<ToastProviderProps> = ({
  children,
  defaultPosition = DEFAULT_POSITION,
  defaultDuration = DEFAULT_DURATION,
  maxToasts = MAX_TOASTS,
}) => {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const timersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  // Clean up all timers on unmount
  const cleanupAllTimers = useCallback(() => {
    timersRef.current.forEach((timer) => clearTimeout(timer));
    timersRef.current.clear();
  }, []);

  // Dismiss a single toast
  const dismissToast = useCallback((id: string) => {
    // Clear auto-dismiss timer if it exists
    const timer = timersRef.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timersRef.current.delete(id);
    }

    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  // Dismiss all toasts
  const dismissAll = useCallback(() => {
    cleanupAllTimers();
    setToasts([]);
  }, [cleanupAllTimers]);

  // Show a new toast
  const showToast = useCallback(
    (options: ShowToastOptions): string => {
      const id = generateId();
      const duration = options.duration ?? defaultDuration;
      const position = options.position ?? defaultPosition;
      const dismissible = options.dismissible ?? true;

      const newToast: ToastMessage = {
        id,
        severity: options.severity ?? 'info',
        title: options.title,
        message: options.message,
        duration,
        position,
        action: options.action,
        dismissible,
        createdAt: Date.now(),
      };

      setToasts((prev) => {
        // If we've hit the limit, remove the oldest toast
        let updated = [...prev, newToast];
        if (updated.length > maxToasts) {
          // Remove oldest toasts (first in, first out)
          const removed = updated.slice(0, updated.length - maxToasts);
          removed.forEach((t) => {
            const timer = timersRef.current.get(t.id);
            if (timer) {
              clearTimeout(timer);
              timersRef.current.delete(t.id);
            }
          });
          updated = updated.slice(updated.length - maxToasts);
        }
        return updated;
      });

      // Auto-dismiss timer
      if (duration > 0) {
        const timer = setTimeout(() => {
          dismissToast(id);
        }, duration);
        timersRef.current.set(id, timer);
      }

      return id;
    },
    [defaultDuration, defaultPosition, maxToasts, dismissToast],
  );

  // Convenience helpers
  const success = useCallback(
    (message: string, options?: Partial<ShowToastOptions>) =>
      showToast({ ...options, severity: 'success', message }),
    [showToast],
  );

  const error = useCallback(
    (message: string, options?: Partial<ShowToastOptions>) =>
      showToast({ ...options, severity: 'error', message }),
    [showToast],
  );

  const warning = useCallback(
    (message: string, options?: Partial<ShowToastOptions>) =>
      showToast({ ...options, severity: 'warning', message }),
    [showToast],
  );

  const info = useCallback(
    (message: string, options?: Partial<ShowToastOptions>) =>
      showToast({ ...options, severity: 'info', message }),
    [showToast],
  );

  // Context value
  const contextValue = useMemo<ToastContextValue>(
    () => ({
      toasts,
      showToast,
      dismissToast,
      dismissAll,
      success,
      error,
      warning,
      info,
    }),
    [toasts, showToast, dismissToast, dismissAll, success, error, warning, info],
  );

  // Group toasts by position for rendering
  const toastsByPosition = useMemo(() => {
    const groups = new Map<ToastPosition, ToastMessage[]>();
    for (const toast of toasts) {
      const pos = toast.position ?? defaultPosition;
      let group = groups.get(pos);
      if (!group) {
        group = [];
        groups.set(pos, group);
      }
      group.push(toast);
    }
    return groups;
  }, [toasts, defaultPosition]);

  // Map position to MUI Snackbar anchorOrigin
  const positionToAnchor = (
    pos: ToastPosition,
  ): { vertical: 'top' | 'bottom'; horizontal: 'left' | 'right' | 'center' } => {
    const [v, h] = pos.split('-');
    return {
      vertical: v as 'top' | 'bottom',
      horizontal: h as 'left' | 'right' | 'center',
    };
  };

  return (
    <ToastContext.Provider value={contextValue}>
      {children}

      {/* Render toasts grouped by position */}
      {Array.from(toastsByPosition.entries()).map(([position, positionToasts]) => (
        <Snackbar
          key={position}
          open={positionToasts.length > 0}
          anchorOrigin={positionToAnchor(position)}
          sx={{
            '& .MuiSnackbar-root': {
              position: 'fixed',
            },
          }}
        >
          <Stack spacing={1} sx={{ maxWidth: 480, width: '100%' }}>
            {positionToasts.map((toast) => (
              <Alert
                key={toast.id}
                severity={toast.severity}
                variant="filled"
                icon={false}
                action={
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    {toast.action && (
                      <Button
                        size="small"
                        color="inherit"
                        onClick={toast.action.onClick}
                        sx={{
                          fontWeight: 600,
                          textTransform: 'none',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {toast.action.label}
                      </Button>
                    )}
                    {toast.dismissible !== false && (
                      <IconButton
                        size="small"
                        color="inherit"
                        onClick={() => dismissToast(toast.id)}
                        aria-label="Close"
                      >
                        <Close fontSize="small" />
                      </IconButton>
                    )}
                  </Stack>
                }
                sx={{
                  borderRadius: 2,
                  boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
                  '& .MuiAlert-message': {
                    flex: 1,
                    minWidth: 0,
                  },
                  '& .MuiAlert-action': {
                    alignItems: 'flex-start',
                    pt: 0.5,
                    ml: 1,
                  },
                }}
              >
                {toast.title && (
                  <AlertTitle sx={{ fontWeight: 700, mb: 0.25 }}>
                    {toast.title}
                  </AlertTitle>
                )}
                <Typography variant="body2" sx={{ lineHeight: 1.5 }}>
                  {toast.message}
                </Typography>
              </Alert>
            ))}
          </Stack>
        </Snackbar>
      ))}
    </ToastContext.Provider>
  );
};

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Access the toast system from any component inside `<ToastProvider>`.
 *
 * ```tsx
 * const toast = useToast();
 *
 * toast.success('Experiment completed successfully!');
 * toast.error('Failed to connect to cluster', { title: 'Connection Error' });
 * toast.warning('SIEM alert not detected', {
 *   action: { label: 'View', onClick: () => navigate('/experiments/123') },
 *   duration: 0, // manual dismiss only
 * });
 * ```
 */
export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a <ToastProvider>');
  }
  return context;
}

// ---------------------------------------------------------------------------
// Standalone toast functions (outside React tree)
// ---------------------------------------------------------------------------

/**
 * Queue for toasts dispatched before the provider mounts.
 * Consumed by the provider on mount.
 */
let preMountQueue: ShowToastOptions[] = [];
let preMountHandler: ((options: ShowToastOptions) => string) | null = null;

/**
 * Register the handler – called internally by the ToastProvider.
 */
export function __registerToastHandler(
  handler: (options: ShowToastOptions) => string,
): void {
  preMountHandler = handler;
  // Flush any queued toasts
  while (preMountQueue.length > 0) {
    const opts = preMountQueue.shift();
    if (opts) {
      handler(opts);
    }
  }
}

/**
 * Show a toast from outside the React tree (e.g., from a service worker,
 * API interceptor, or vanilla JS module).
 *
 * ```ts
 * import { showToast } from '@/services/toast';
 * showToast({ severity: 'error', message: 'WebSocket disconnected' });
 * ```
 */
export function showToast(options: ShowToastOptions): string {
  if (preMountHandler) {
    return preMountHandler(options);
  }
  // Queue for later if the provider hasn't mounted yet
  preMountQueue.push(options);
  return 'pending';
}
