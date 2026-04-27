// ---------------------------------------------------------------------------
// useExperimentWebSocket – subscribes to experiment lifecycle events via the
// shared WebSocketClient singleton and exposes real-time progress / step data.
// ---------------------------------------------------------------------------

import { useEffect, useState, useRef, useCallback } from 'react';
import {
  getWebSocketClient,
  WebSocketClient,
  WebSocketConfig,
  WSConnectionState,
  WSEvent,
  ExperimentProgressPayload,
  ExperimentCompletedPayload,
} from '@/services/websocket';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Payload shape for experiment:step_completed and experiment:step_failed events */
export interface ExperimentStepPayload {
  experiment_id: string;
  run_id: string;
  step_index: number;
  step_name: string;
  step_status: 'completed' | 'failed';
  message?: string;
  timestamp?: string;
}

/** Aggregated "latest step" information exposed by the hook */
export interface LatestStepInfo {
  stepIndex: number;
  stepName: string;
  stepStatus: 'completed' | 'failed';
  message?: string;
  timestamp?: string;
  /** ISO 8601 timestamp of when the hook received this update */
  receivedAt: string;
}

/** Payload shape for experiment:failed and experiment:cancelled events */
export interface ExperimentTerminalPayload {
  experiment_id: string;
  run_id: string;
  status: string;
  reason?: string;
  message?: string;
}

export interface UseExperimentWebSocketOptions {
  /** WebSocket config – only required the first time the singleton is created */
  config?: WebSocketConfig;
  /** Auto-connect on mount (default true) */
  autoConnect?: boolean;
  /**
   * Auto-disconnect the singleton when the hook unmounts.
   * WARNING: since the client is a singleton, disconnecting affects ALL
   * subscribers.  Defaults to `false` so other consumers stay connected.
   */
  autoDisconnect?: boolean;
}

export interface UseExperimentWebSocketReturn {
  /** Current WebSocket connection state */
  connectionState: WSConnectionState;
  /** Convenience boolean – true when connected */
  isConnected: boolean;
  /** Most recent progress update for this experiment (null until first event) */
  latestProgress: ExperimentProgressPayload | null;
  /** Most recent step-completed / step-failed update (null until first event) */
  latestStep: LatestStepInfo | null;
  /** Terminal status if the experiment has completed/failed/cancelled */
  terminalStatus: ExperimentCompletedPayload | ExperimentTerminalPayload | null;
  /** Manually (re)connect the WebSocket */
  connect: () => void;
  /** Manually disconnect the WebSocket */
  disconnect: () => void;
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Custom hook that subscribes to real-time experiment lifecycle events via
 * the shared `WebSocketClient` singleton.
 *
 * ```tsx
 * const { isConnected, latestProgress, latestStep } = useExperimentWebSocket(experimentId, {
 *   config: { url: 'wss://api.chaos-sec.com/ws' },
 * });
 *
 * useEffect(() => {
 *   if (latestProgress) {
 *     console.log(`Step ${latestProgress.step_name}: ${latestProgress.progress_percent}%`);
 *   }
 * }, [latestProgress]);
 * ```
 */
export function useExperimentWebSocket(
  experimentId: string | undefined,
  options: UseExperimentWebSocketOptions = {},
): UseExperimentWebSocketReturn {
  const { config, autoConnect = true, autoDisconnect = false } = options;

  // ---- state ----
  const [connectionState, setConnectionState] =
    useState<WSConnectionState>('disconnected');
  const [latestProgress, setLatestProgress] = useState<ExperimentProgressPayload | null>(
    null,
  );
  const [latestStep, setLatestStep] = useState<LatestStepInfo | null>(null);
  const [terminalStatus, setTerminalStatus] = useState<
    ExperimentCompletedPayload | ExperimentTerminalPayload | null
  >(null);

  // ---- refs ----
  const clientRef = useRef<WebSocketClient | null>(null);
  const unsubFnsRef = useRef<Array<() => void>>([]);
  const experimentIdRef = useRef(experimentId);
  experimentIdRef.current = experimentId;

  // ---- helpers ----
  const ensureClient = useCallback((): WebSocketClient | null => {
    // Try the existing singleton first
    let client = getWebSocketClient();
    if (client) return client;

    // Create singleton if config was supplied
    if (config) {
      client = getWebSocketClient(config);
    }

    if (!client) {
      console.warn(
        '[useExperimentWebSocket] No WebSocket client available. ' +
          'Provide a `config` option or mount <useWebSocket> higher in the tree.',
      );
    }

    return client;
  }, [config]);

  // ---- subscribe / unsubscribe ----
  const subscribeToExperimentEvents = useCallback((client: WebSocketClient) => {
    // Clear previous subscriptions
    unsubFnsRef.current.forEach((unsub) => unsub());
    unsubFnsRef.current = [];

    const currentId = experimentIdRef.current;
    if (!currentId) return;

    // -- experiment:progress --
    const unsubProgress = client.on<ExperimentProgressPayload>(
      'experiment:progress',
      (event: WSEvent<ExperimentProgressPayload>) => {
        if (event.payload.experiment_id === currentId) {
          setLatestProgress(event.payload);
        }
      },
    );
    unsubFnsRef.current.push(unsubProgress);

    // -- experiment:step_completed --
    const unsubStepCompleted = client.on<ExperimentStepPayload>(
      'experiment:step_completed',
      (event: WSEvent<ExperimentStepPayload>) => {
        if (event.payload.experiment_id === currentId) {
          setLatestStep({
            stepIndex: event.payload.step_index,
            stepName: event.payload.step_name,
            stepStatus: 'completed',
            message: event.payload.message,
            timestamp: event.payload.timestamp,
            receivedAt: new Date().toISOString(),
          });
        }
      },
    );
    unsubFnsRef.current.push(unsubStepCompleted);

    // -- experiment:step_failed --
    const unsubStepFailed = client.on<ExperimentStepPayload>(
      'experiment:step_failed',
      (event: WSEvent<ExperimentStepPayload>) => {
        if (event.payload.experiment_id === currentId) {
          setLatestStep({
            stepIndex: event.payload.step_index,
            stepName: event.payload.step_name,
            stepStatus: 'failed',
            message: event.payload.message,
            timestamp: event.payload.timestamp,
            receivedAt: new Date().toISOString(),
          });
        }
      },
    );
    unsubFnsRef.current.push(unsubStepFailed);

    // -- experiment:completed --
    const unsubCompleted = client.on<ExperimentCompletedPayload>(
      'experiment:completed',
      (event: WSEvent<ExperimentCompletedPayload>) => {
        if (event.payload.experiment_id === currentId) {
          setTerminalStatus(event.payload);
        }
      },
    );
    unsubFnsRef.current.push(unsubCompleted);

    // -- experiment:failed --
    const unsubFailed = client.on<ExperimentTerminalPayload>(
      'experiment:failed',
      (event: WSEvent<ExperimentTerminalPayload>) => {
        if (event.payload.experiment_id === currentId) {
          setTerminalStatus(event.payload);
        }
      },
    );
    unsubFnsRef.current.push(unsubFailed);

    // -- experiment:cancelled --
    const unsubCancelled = client.on<ExperimentTerminalPayload>(
      'experiment:cancelled',
      (event: WSEvent<ExperimentTerminalPayload>) => {
        if (event.payload.experiment_id === currentId) {
          setTerminalStatus(event.payload);
        }
      },
    );
    unsubFnsRef.current.push(unsubCancelled);
  }, []);

  // ---- main effect: initialise client, connect, subscribe ----
  useEffect(() => {
    // Reset data when experimentId changes
    setLatestProgress(null);
    setLatestStep(null);
    setTerminalStatus(null);

    if (!experimentId) return;

    const client = ensureClient();
    if (!client) return;

    clientRef.current = client;

    // Listen for connection-state changes
    const unsubState = client.onStateChange(setConnectionState);

    // Set initial state synchronously
    setConnectionState(client.getState());

    // Connect if autoConnect and not already connected
    if (autoConnect && !client.isConnected) {
      client.connect();
    }

    // Subscribe to experiment-scoped events
    subscribeToExperimentEvents(client);

    return () => {
      // Unsubscribe from all event handlers
      unsubFnsRef.current.forEach((unsub) => unsub());
      unsubFnsRef.current = [];

      // Unsubscribe from state listener
      unsubState();

      // Optionally tear down the singleton
      if (autoDisconnect) {
        client.disconnect();
        getWebSocketClient(null); // destroy singleton
        clientRef.current = null;
      }
    };
  }, [
    experimentId,
    autoConnect,
    autoDisconnect,
    ensureClient,
    subscribeToExperimentEvents,
  ]);

  // Re-subscribe when the client reconnects (connectionState goes back to
  // "connected") so we don't miss events after a transient disconnect.
  const prevConnectionStateRef = useRef<WSConnectionState>(connectionState);
  useEffect(() => {
    const client = clientRef.current;
    if (!client || !experimentId) return;

    const wasReconnected =
      prevConnectionStateRef.current !== 'connected' && connectionState === 'connected';

    prevConnectionStateRef.current = connectionState;

    if (wasReconnected) {
      subscribeToExperimentEvents(client);
    }
  }, [connectionState, experimentId, subscribeToExperimentEvents]);

  // ---- imperative actions ----
  const connect = useCallback(() => {
    const client = clientRef.current ?? ensureClient();
    client?.connect();
  }, [ensureClient]);

  const disconnect = useCallback(() => {
    clientRef.current?.disconnect();
  }, []);

  return {
    connectionState,
    isConnected: connectionState === 'connected',
    latestProgress,
    latestStep,
    terminalStatus,
    connect,
    disconnect,
  };
}

export default useExperimentWebSocket;
