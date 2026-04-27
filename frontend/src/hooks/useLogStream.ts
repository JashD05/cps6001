// ---------------------------------------------------------------------------
// useLogStream – real-time log streaming via WebSocket with line accumulation,
// auto-scroll, and memory limiting.
// ---------------------------------------------------------------------------

import { useEffect, useLayoutEffect, useState, useRef, useCallback } from 'react';
import {
  getWebSocketClient,
  WebSocketClient,
  WebSocketConfig,
  WSConnectionState,
  WSEvent,
  ExperimentLogPayload,
} from '@/services/websocket';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Structured representation of a single received log line. */
export interface LogLine {
  /** ID of the experiment this log belongs to */
  experimentId: string;
  /** ID of the experiment run */
  runId: string;
  /** The raw log text */
  line: string;
  /** Log level: info, warn, error, debug, etc. */
  level: string;
  /** ISO 8601 timestamp from the server */
  timestamp: string;
  /** Kubernetes pod that emitted the log (if available) */
  podName?: string;
  /** ISO 8601 timestamp of when the client received this log */
  receivedAt: string;
}

/** Options for the {@link useLogStream} hook. */
export interface UseLogStreamOptions {
  /** WebSocket config – only required the first time the singleton is created */
  config?: WebSocketConfig;
  /** Auto-connect on mount (default true) */
  autoConnect?: boolean;
  /**
   * Auto-disconnect the singleton when the hook unmounts.
   * Defaults to `false` since the client is shared across subscribers.
   */
  autoDisconnect?: boolean;
  /** Maximum number of log lines to retain (default 1000) */
  maxLines?: number;
  /** Automatically scroll to bottom when new logs arrive (default true) */
  autoScroll?: boolean;
  /**
   * Distance in pixels from the bottom of the scroll container within which
   * auto-scroll kicks in (default 80).
   */
  scrollThreshold?: number;
}

/** Return value of the {@link useLogStream} hook. */
export interface UseLogStreamReturn {
  /** Accumulated log lines (oldest → newest) */
  logs: LogLine[];
  /** True when the WebSocket is connected and can receive logs */
  isStreaming: boolean;
  /** Current WebSocket connection state */
  connectionState: WSConnectionState;
  /** Clear all accumulated log lines */
  clearLogs: () => void;
  /**
   * Ref to attach to the scrollable log container element.
   *
   * ```tsx
   * const { scrollRef } = useLogStream(experimentId);
   * return <div ref={scrollRef} style={{ overflow: 'auto', maxHeight: 400 }}>…</div>;
   * ```
   */
  scrollRef: React.Ref<HTMLDivElement>;
  /** Manually (re)connect the WebSocket */
  connect: () => void;
  /** Manually disconnect the WebSocket */
  disconnect: () => void;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_MAX_LINES = 1000;
const DEFAULT_SCROLL_THRESHOLD = 80;

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Custom hook that subscribes to `experiment:logs` WebSocket events, accumulates
 * log lines into an array with a configurable cap, and provides auto-scroll
 * behaviour with "sticky bottom" detection.
 *
 * **Batching** – incoming log lines are buffered and flushed to React state
 * once per animation frame (`requestAnimationFrame`) to avoid excessive
 * re-renders when logs arrive in rapid bursts.
 *
 * **Sticky scroll** – auto-scroll only fires when the user is already near the
 * bottom of the scroll container. If the user scrolls up to read older logs,
 * new arrivals will *not* force the view to jump to the bottom. Scrolling back
 * down re-enables auto-scroll.
 *
 * ```tsx
 * const { logs, isStreaming, scrollRef, clearLogs } = useLogStream(experimentId, {
 *   config: { url: 'wss://api.chaos-sec.com/ws' },
 * });
 *
 * return (
 *   <div ref={scrollRef} style={{ overflow: 'auto', height: 400 }}>
 *     {logs.map((l, i) => (
 *       <div key={i} style={{ fontFamily: 'monospace' }}>{l.line}</div>
 *     ))}
 *   </div>
 * );
 * ```
 */
export function useLogStream(
  experimentId: string | undefined,
  options: UseLogStreamOptions = {},
): UseLogStreamReturn {
  const {
    config,
    autoConnect = true,
    autoDisconnect = false,
    maxLines = DEFAULT_MAX_LINES,
    autoScroll = true,
    scrollThreshold = DEFAULT_SCROLL_THRESHOLD,
  } = options;

  // ---------------------------------------------------------------------------
  // State
  // ---------------------------------------------------------------------------

  const [connectionState, setConnectionState] = useState<WSConnectionState>('disconnected');
  const [logs, setLogs] = useState<LogLine[]>([]);

  // ---------------------------------------------------------------------------
  // Refs
  // ---------------------------------------------------------------------------

  const clientRef = useRef<WebSocketClient | null>(null);
  const unsubFnsRef = useRef<Array<() => void>>([]);
  const experimentIdRef = useRef(experimentId);
  experimentIdRef.current = experimentId;

  // Scroll tracking
  const scrollElementRef = useRef<HTMLElement | null>(null);
  const isStickyRef = useRef(true);
  const scrollThresholdRef = useRef(scrollThreshold);
  scrollThresholdRef.current = scrollThreshold;
  const scrollCleanupRef = useRef<(() => void) | null>(null);

  // Batching
  const pendingLogsRef = useRef<LogLine[]>([]);
  const flushRafRef = useRef<number | null>(null);

  // Reconnection tracking
  const prevConnectionStateRef = useRef<WSConnectionState>(connectionState);

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  /** Get or create the WebSocket client singleton. */
  const ensureClient = useCallback((): WebSocketClient | null => {
    let client = getWebSocketClient();
    if (client) return client;

    if (config) {
      client = getWebSocketClient(config);
    }

    if (!client) {
      console.warn(
        '[useLogStream] No WebSocket client available. ' +
          'Provide a `config` option or initialise the singleton elsewhere.',
      );
    }

    return client;
  }, [config]);

  /** Flush buffered log lines into React state (called from rAF). */
  const flushPendingLogs = useCallback(() => {
    flushRafRef.current = null;

    const pending = pendingLogsRef.current;
    if (pending.length === 0) return;

    pendingLogsRef.current = [];

    setLogs((prev) => {
      const next = [...prev, ...pending];
      // Trim to maxLines, keeping the most recent entries
      return next.length > maxLines ? next.slice(-maxLines) : next;
    });
  }, [maxLines]);

  /** Schedule a flush on the next animation frame (deduped). */
  const scheduleFlush = useCallback(() => {
    if (flushRafRef.current !== null) return;
    flushRafRef.current = requestAnimationFrame(flushPendingLogs);
  }, [flushPendingLogs]);

  // ---------------------------------------------------------------------------
  // Scroll container ref (callback pattern)
  //
  // Using a callback ref instead of a RefObject so we can reliably set up the
  // scroll-position listener the moment the DOM element mounts – without having
  // to poll or re-run effects.
  // ---------------------------------------------------------------------------

  const scrollRef = useCallback((node: HTMLDivElement | null) => {
    // Tear down previous element's listener
    if (scrollCleanupRef.current) {
      scrollCleanupRef.current();
      scrollCleanupRef.current = null;
    }

    scrollElementRef.current = node;

    if (node) {
      const handleScroll = () => {
        isStickyRef.current =
          node.scrollHeight - node.scrollTop - node.clientHeight <=
          scrollThresholdRef.current;
      };

      node.addEventListener('scroll', handleScroll, { passive: true });

      // Initialise sticky state from the current scroll position
      handleScroll();

      scrollCleanupRef.current = () => {
        node.removeEventListener('scroll', handleScroll);
      };
    }
  }, []);

  // ---------------------------------------------------------------------------
  // Auto-scroll (layout effect → no visual flicker)
  //
  // Runs synchronously after DOM mutations but before the browser paints, so
  // the user never sees a flash of un-scrolled content.
  // ---------------------------------------------------------------------------

  useLayoutEffect(() => {
    if (!autoScroll) return;

    const el = scrollElementRef.current;
    if (!el || !isStickyRef.current) return;

    el.scrollTop = el.scrollHeight;
  }, [logs, autoScroll]);

  // ---------------------------------------------------------------------------
  // Event subscription
  // ---------------------------------------------------------------------------

  const subscribeToLogs = useCallback(
    (client: WebSocketClient) => {
      // Clear previous subscriptions
      unsubFnsRef.current.forEach((unsub) => unsub());
      unsubFnsRef.current = [];

      const currentId = experimentIdRef.current;
      if (!currentId) return;

      const unsub = client.on<ExperimentLogPayload>(
        'experiment:logs',
        (event: WSEvent<ExperimentLogPayload>) => {
          if (event.payload.experiment_id !== currentId) return;

          pendingLogsRef.current.push({
            experimentId: event.payload.experiment_id,
            runId: event.payload.run_id,
            line: event.payload.log_line,
            level: event.payload.log_level,
            timestamp: event.payload.timestamp,
            podName: event.payload.pod_name,
            receivedAt: new Date().toISOString(),
          });

          scheduleFlush();
        },
      );

      unsubFnsRef.current.push(unsub);
    },
    [scheduleFlush],
  );

  // ---------------------------------------------------------------------------
  // Main lifecycle effect
  // ---------------------------------------------------------------------------

  useEffect(() => {
    // Reset accumulated data when the experiment changes
    setLogs([]);
    pendingLogsRef.current = [];
    isStickyRef.current = true; // default to sticky for new experiment

    if (!experimentId) return;

    const client = ensureClient();
    if (!client) return;

    clientRef.current = client;

    // Connection state
    const unsubState = client.onStateChange(setConnectionState);
    setConnectionState(client.getState());

    if (autoConnect && !client.isConnected) {
      client.connect();
    }

    subscribeToLogs(client);

    return () => {
      // Flush any remaining buffered logs so nothing is lost on unmount/re-switch
      if (flushRafRef.current !== null) {
        cancelAnimationFrame(flushRafRef.current);
        flushRafRef.current = null;
      }
      if (pendingLogsRef.current.length > 0) {
        flushPendingLogs();
      }

      // Unsubscribe from event handlers
      unsubFnsRef.current.forEach((unsub) => unsub());
      unsubFnsRef.current = [];

      // Unsubscribe from state listener
      unsubState();

      // Optionally tear down the singleton
      if (autoDisconnect) {
        client.disconnect();
        getWebSocketClient(null);
        clientRef.current = null;
      }
    };
  }, [experimentId, autoConnect, autoDisconnect, ensureClient, subscribeToLogs, flushPendingLogs]);

  // ---------------------------------------------------------------------------
  // Re-subscribe after a transient disconnect / reconnect
  // ---------------------------------------------------------------------------

  useEffect(() => {
    const client = clientRef.current;
    if (!client || !experimentId) return;

    const wasReconnected =
      prevConnectionStateRef.current !== 'connected' && connectionState === 'connected';
    prevConnectionStateRef.current = connectionState;

    if (wasReconnected) {
      subscribeToLogs(client);
    }
  }, [connectionState, experimentId, subscribeToLogs]);

  // ---------------------------------------------------------------------------
  // Final cleanup on unmount
  // ---------------------------------------------------------------------------

  useEffect(() => {
    return () => {
      if (flushRafRef.current !== null) {
        cancelAnimationFrame(flushRafRef.current);
      }
      if (scrollCleanupRef.current) {
        scrollCleanupRef.current();
      }
    };
  }, []);

  // ---------------------------------------------------------------------------
  // Imperative actions
  // ---------------------------------------------------------------------------

  const connect = useCallback(() => {
    const client = clientRef.current ?? ensureClient();
    client?.connect();
  }, [ensureClient]);

  const disconnect = useCallback(() => {
    clientRef.current?.disconnect();
  }, []);

  const clearLogs = useCallback(() => {
    setLogs([]);
    pendingLogsRef.current = [];
  }, []);

  // ---------------------------------------------------------------------------
  // Return
  // ---------------------------------------------------------------------------

  return {
    logs,
    isStreaming: connectionState === 'connected',
    connectionState,
    clearLogs,
    scrollRef,
    connect,
    disconnect,
  };
}

export default useLogStream;
