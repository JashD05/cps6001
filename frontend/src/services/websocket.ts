/* eslint-disable no-console */
/**
 * WebSocket client for Chaos-Sec real-time features.
 *
 * Provides a singleton WebSocket manager with:
 *  - Automatic reconnection with exponential backoff
 *  - Heartbeat / keep-alive ping-pong
 *  - Typed event subscription API
 *  - React `useWebSocket` hook for component integration
 *  - Redux-compatible event dispatch
 */

import { AnyAction, Dispatch } from '@reduxjs/toolkit';
import { useCallback, useEffect, useRef, useState } from 'react';
import { getAccessToken, clearTokens } from './api';

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

export interface WebSocketConfig {
  /** Full WS URL, e.g. "ws://localhost:8080/ws" or "wss://chaos-sec.example.com/ws" */
  url: string;
  /** Milliseconds between heartbeat pings (default 30 000) */
  heartbeatInterval?: number;
  /** Milliseconds to wait for a pong before considering the connection dead (default 10 000) */
  pongTimeout?: number;
  /** Maximum number of reconnection attempts before giving up (default 10) */
  maxReconnectAttempts?: number;
  /** Base delay in ms for exponential back-off (default 1 000) */
  reconnectBaseDelay?: number;
  /** Maximum back-off delay cap in ms (default 30 000) */
  reconnectMaxDelay?: number;
  /** Optional Redux dispatch for forwarding events to the store */
  reduxDispatch?: Dispatch<AnyAction>;
}

const DEFAULT_CONFIG: Required<Omit<WebSocketConfig, 'url' | 'reduxDispatch'>> = {
  heartbeatInterval: 30_000,
  pongTimeout: 10_000,
  maxReconnectAttempts: 10,
  reconnectBaseDelay: 1_000,
  reconnectMaxDelay: 30_000,
};

// ---------------------------------------------------------------------------
// Event types – mirrors the backend's Redis pub/sub channels
// ---------------------------------------------------------------------------

export type WSEventType =
  | 'experiment:started'
  | 'experiment:progress'
  | 'experiment:step_completed'
  | 'experiment:step_failed'
  | 'experiment:completed'
  | 'experiment:failed'
  | 'experiment:cancelled'
  | 'experiment:notifications'
  | 'experiment:logs'
  | 'cluster:health'
  | 'cluster:status'
  | 'cluster:resource_update'
  | 'siem:alert'
  | 'siem:validation'
  | 'system:notification'
  | 'system:ping'
  | 'pong';

export interface WSEvent<T = unknown> {
  type: WSEventType;
  payload: T;
  timestamp: string; // ISO 8601
  id?: string;
}

// ---------------------------------------------------------------------------
// Connection state
// ---------------------------------------------------------------------------

export type WSConnectionState =
  | 'connecting'
  | 'connected'
  | 'disconnecting'
  | 'disconnected'
  | 'reconnecting'
  | 'error';

// ---------------------------------------------------------------------------
// Event handler type
// ---------------------------------------------------------------------------

export type WSEventHandler<T = unknown> = (event: WSEvent<T>) => void;
export type WSRawHandler = (data: MessageEvent) => void;

// ---------------------------------------------------------------------------
// WebSocket Client
// ---------------------------------------------------------------------------

export class WebSocketClient {
  private config: Required<Omit<WebSocketConfig, 'reduxDispatch'>> &
    Pick<WebSocketConfig, 'reduxDispatch'>;
  private ws: WebSocket | null = null;
  private state: WSConnectionState = 'disconnected';
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private pongTimer: ReturnType<typeof setTimeout> | null = null;
  private eventHandlers = new Map<WSEventType, Set<WSEventHandler>>();
  private rawHandlers = new Set<WSRawHandler>();
  private stateListeners = new Set<(state: WSConnectionState) => void>();
  private disposed = false;

  constructor(private userConfig: WebSocketConfig) {
    this.config = { ...DEFAULT_CONFIG, ...userConfig };
  }

  // -----------------------------------------------------------------------
  // Public API
  // -----------------------------------------------------------------------

  /** Open the WebSocket connection. */
  connect(): void {
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }
    this.disposed = false;
    this.setState('connecting');
    this.doConnect();
  }

  /** Gracefully close the connection. */
  disconnect(): void {
    this.disposed = true;
    this.clearTimers();
    this.setState('disconnecting');

    if (this.ws) {
      this.ws.close(1000, 'Client disconnect');
      this.ws = null;
    }
    this.setState('disconnected');
  }

  /** Subscribe to a typed event. Returns an unsubscribe function. */
  on<T = unknown>(eventType: WSEventType, handler: WSEventHandler<T>): () => void {
    if (!this.eventHandlers.has(eventType)) {
      this.eventHandlers.set(eventType, new Set());
    }
    const typedHandler = handler as WSEventHandler;
    const handlers = this.eventHandlers.get(eventType);
    if (handlers) {
      handlers.add(typedHandler);
    }

    return () => {
      this.eventHandlers.get(eventType)?.delete(typedHandler);
    };
  }

  /** Subscribe to ALL incoming messages (raw). Returns an unsubscribe function. */
  onRaw(handler: WSRawHandler): () => void {
    this.rawHandlers.add(handler);
    return () => this.rawHandlers.delete(handler);
  }

  /** Send a typed event to the server. */
  send<T = unknown>(eventType: WSEventType, payload: T): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn('[WS] Cannot send – connection not open');
      return false;
    }

    const message: WSEvent<T> = {
      type: eventType,
      payload,
      timestamp: new Date().toISOString(),
    };

    try {
      this.ws.send(JSON.stringify(message));
      return true;
    } catch (err) {
      console.error('[WS] Send error:', err);
      return false;
    }
  }

  /** Send raw string data. */
  sendRaw(data: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return false;
    }
    try {
      this.ws.send(data);
      return true;
    } catch {
      return false;
    }
  }

  /** Get current connection state. */
  getState(): WSConnectionState {
    return this.state;
  }

  /** Subscribe to connection state changes. Returns unsubscribe. */
  onStateChange(listener: (state: WSConnectionState) => void): () => void {
    this.stateListeners.add(listener);
    return () => this.stateListeners.delete(listener);
  }

  /** Whether the socket is currently open. */
  get isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  // -----------------------------------------------------------------------
  // Internal – connection
  // -----------------------------------------------------------------------

  private doConnect(): void {
    try {
      const token = getAccessToken();
      const url = new URL(this.config.url);
      if (token) {
        url.searchParams.set('token', token);
      }

      this.ws = new WebSocket(url.toString());

      this.ws.onopen = this.handleOpen;
      this.ws.onclose = this.handleClose;
      this.ws.onerror = this.handleError;
      this.ws.onmessage = this.handleMessage;
    } catch (err) {
      console.error('[WS] Connection error:', err);
      this.setState('error');
      this.scheduleReconnect();
    }
  }

  private handleOpen = (): void => {
    console.info('[WS] Connected');
    this.reconnectAttempts = 0;
    this.setState('connected');
    this.startHeartbeat();
  };

  private handleClose = (event: CloseEvent): void => {
    this.stopHeartbeat();

    if (this.disposed) return;

    // 1000 = normal close, 1001 = going away – don't reconnect
    if (event.code === 1000 || event.code === 1001) {
      this.setState('disconnected');
      return;
    }

    // Auth failure – don't reconnect
    if (event.code === 4001 || event.code === 4003) {
      console.warn('[WS] Auth failure, clearing tokens');
      clearTokens();
      this.setState('error');
      return;
    }

    console.warn(`[WS] Closed (code=${event.code}), reconnecting…`);
    this.scheduleReconnect();
  };

  private handleError = (_event: Event): void => {
    console.error('[WS] Error event');
    this.setState('error');
    // onclose will fire after onerror, which handles reconnect
  };

  private handleMessage = (event: MessageEvent): void => {
    // Reset pong timer – any message proves the server is alive
    this.resetPongTimeout();

    // Notify raw handlers
    this.rawHandlers.forEach((h) => {
      try {
        h(event);
      } catch (e) {
        console.error('[WS] Raw handler error:', e);
      }
    });

    // Try to parse as typed WSEvent
    try {
      const wsEvent: WSEvent = JSON.parse(event.data);

      // Server-side ping response
      if (wsEvent.type === 'system:ping' || wsEvent.type === 'pong') {
        return; // heartbeat ack, already handled above
      }

      // Dispatch to typed handlers
      const handlers = this.eventHandlers.get(wsEvent.type);
      if (handlers) {
        handlers.forEach((h) => {
          try {
            h(wsEvent);
          } catch (e) {
            console.error(`[WS] Handler error for ${wsEvent.type}:`, e);
          }
        });
      }

      // Forward to Redux if configured
      if (this.config.reduxDispatch && wsEvent.type) {
        this.config.reduxDispatch({
          type: `ws/${wsEvent.type}`,
          payload: wsEvent.payload,
          meta: { wsEventId: wsEvent.id, wsTimestamp: wsEvent.timestamp },
        } as AnyAction);
      }
    } catch {
      // Not JSON or not a WSEvent – silently ignore
    }
  };

  // -----------------------------------------------------------------------
  // Internal – reconnection
  // -----------------------------------------------------------------------

  private scheduleReconnect(): void {
    if (this.disposed) return;
    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      console.error(
        `[WS] Max reconnect attempts (${this.config.maxReconnectAttempts}) reached`,
      );
      this.setState('error');
      return;
    }

    this.reconnectAttempts += 1;
    const delay = Math.min(
      this.config.reconnectBaseDelay * Math.pow(2, this.reconnectAttempts - 1) +
        Math.random() * 500, // jitter
      this.config.reconnectMaxDelay,
    );

    this.setState('reconnecting');
    console.info(
      `[WS] Reconnect attempt ${this.reconnectAttempts} in ${Math.round(delay)}ms`,
    );

    this.reconnectTimer = setTimeout(() => {
      if (!this.disposed) {
        this.doConnect();
      }
    }, delay);
  }

  // -----------------------------------------------------------------------
  // Internal – heartbeat
  // -----------------------------------------------------------------------

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(
          JSON.stringify({ type: 'ping', timestamp: new Date().toISOString() }),
        );
        this.startPongTimeout();
      }
    }, this.config.heartbeatInterval);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    this.clearPongTimeout();
  }

  private startPongTimeout(): void {
    this.clearPongTimeout();
    this.pongTimer = setTimeout(() => {
      console.warn('[WS] Pong timeout – server unresponsive, closing connection');
      this.ws?.close(4000, 'Pong timeout');
    }, this.config.pongTimeout);
  }

  private resetPongTimeout(): void {
    this.clearPongTimeout();
  }

  private clearPongTimeout(): void {
    if (this.pongTimer) {
      clearTimeout(this.pongTimer);
      this.pongTimer = null;
    }
  }

  // -----------------------------------------------------------------------
  // Internal – utilities
  // -----------------------------------------------------------------------

  private setState(newState: WSConnectionState): void {
    if (this.state === newState) return;
    this.state = newState;
    this.stateListeners.forEach((l) => {
      try {
        l(newState);
      } catch (e) {
        console.error('[WS] State listener error:', e);
      }
    });
  }

  private clearTimers(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopHeartbeat();
  }
}

// ---------------------------------------------------------------------------
// Singleton instance
// ---------------------------------------------------------------------------

let _instance: WebSocketClient | null = null;

/**
 * Get or create the shared WebSocket client singleton.
 * Pass `null` to tear down the existing instance.
 */
export function getWebSocketClient(
  config?: WebSocketConfig | null,
): WebSocketClient | null {
  if (config === null) {
    _instance?.disconnect();
    _instance = null;
    return null;
  }

  if (!_instance && config) {
    _instance = new WebSocketClient(config);
  }

  return _instance;
}

// ---------------------------------------------------------------------------
// React hook – useWebSocket
// ---------------------------------------------------------------------------

export interface UseWebSocketOptions {
  /** Auto-connect on mount (default true) */
  autoConnect?: boolean;
  /** Auto-disconnect on unmount (default true) */
  autoDisconnect?: boolean;
}

export interface UseWebSocketReturn {
  connectionState: WSConnectionState;
  isConnected: boolean;
  connect: () => void;
  disconnect: () => void;
  send: <T = unknown>(eventType: WSEventType, payload: T) => boolean;
  subscribe: <T = unknown>(eventType: WSEventType, handler: WSEventHandler<T>) => void;
}

/**
 * React hook for WebSocket integration.
 *
 * ```tsx
 * const { connectionState, isConnected, subscribe, send } = useWebSocket({
 *   url: 'ws://localhost:8080/ws',
 * });
 *
 * useEffect(() => {
 *   const unsub = subscribe('experiment:progress', (event) => {
 *     console.log('Progress:', event.payload);
 *   });
 *   return unsub;
 * }, [subscribe]);
 * ```
 */
export function useWebSocket(
  config: WebSocketConfig,
  options: UseWebSocketOptions = {},
): UseWebSocketReturn {
  const { autoConnect = true, autoDisconnect = true } = options;
  const clientRef = useRef<WebSocketClient | null>(null);
  const [connectionState, setConnectionState] =
    useState<WSConnectionState>('disconnected');

  // Create / reuse client
  useEffect(() => {
    clientRef.current = getWebSocketClient(config) ?? new WebSocketClient(config);

    const unsubState = clientRef.current.onStateChange(setConnectionState);

    if (autoConnect) {
      clientRef.current.connect();
    }

    return () => {
      unsubState();
      if (autoDisconnect) {
        clientRef.current?.disconnect();
        getWebSocketClient(null); // tear down singleton
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config.url]);

  const connect = useCallback(() => {
    clientRef.current?.connect();
  }, []);

  const disconnect = useCallback(() => {
    clientRef.current?.disconnect();
  }, []);

  const send = useCallback(<T = unknown>(eventType: WSEventType, payload: T): boolean => {
    return clientRef.current?.send(eventType, payload) ?? false;
  }, []);

  const subscribe = useCallback(
    <T = unknown>(eventType: WSEventType, handler: WSEventHandler<T>): (() => void) => {
      if (!clientRef.current) return () => {};
      return clientRef.current.on(eventType, handler);
    },
    [],
  );

  return {
    connectionState,
    isConnected: connectionState === 'connected',
    connect,
    disconnect,
    send,
    subscribe,
  };
}

// ---------------------------------------------------------------------------
// React hook – useWSEvent (convenience)
// ---------------------------------------------------------------------------

/**
 * Subscribe to a single WebSocket event type.
 *
 * ```tsx
 * useWSEvent('experiment:completed', (event) => {
 *   showToast(`Experiment ${event.payload.name} completed!`);
 * });
 * ```
 */
export function useWSEvent<T = unknown>(
  eventType: WSEventType,
  handler: WSEventHandler<T>,
  deps: React.DependencyList = [],
): void {
  const clientRef = useRef<WebSocketClient | null>(null);

  useEffect(() => {
    // Lazily get the singleton (assumes useWebSocket was used higher up)
    clientRef.current = getWebSocketClient();

    if (!clientRef.current) return;

    const unsub = clientRef.current.on(eventType, handler);
    return unsub;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eventType, handler, ...deps]);
}

// ---------------------------------------------------------------------------
// Convenience: typed event channels for experiment monitoring
// ---------------------------------------------------------------------------

export interface ExperimentProgressPayload {
  experiment_id: string;
  run_id: string;
  step_index: number;
  step_name: string;
  step_status: string;
  progress_percent: number;
  message?: string;
}

export interface ExperimentLogPayload {
  experiment_id: string;
  run_id: string;
  log_line: string;
  log_level: string;
  timestamp: string;
  pod_name?: string;
}

export interface ExperimentCompletedPayload {
  experiment_id: string;
  run_id: string;
  status: string;
  result_summary: {
    total_pods_spawned: number;
    successful_attacks: number;
    blocked_attacks: number;
    detection_rate: number;
    overall_score: number;
    findings: string[];
  };
  duration_ms: number;
}

export interface ClusterHealthPayload {
  cluster_id: string;
  status: string;
  node_count: number;
  healthy_nodes: number;
  cpu_percent: number;
  memory_percent: number;
}

export interface SIEMAlertPayload {
  alert_id: string;
  alert_type: string;
  severity: string;
  source: string;
  experiment_id?: string;
  run_id?: string;
  timestamp: string;
  raw_data?: Record<string, unknown>;
}

export interface SystemNotificationPayload {
  id: string;
  title: string;
  message: string;
  severity: 'info' | 'warning' | 'error' | 'success';
  category: string;
  timestamp: string;
  read: boolean;
}
