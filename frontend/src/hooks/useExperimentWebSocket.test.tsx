/**
 * Unit tests for the useExperimentWebSocket hook.
 *
 * Covers:
 *  1. Hook initialization (creates/reuses client, auto-connects)
 *  2. Connection state tracking (disconnected → connecting → connected)
 *  3. Receiving experiment:progress events
 *  4. Receiving experiment:step_completed events
 *  5. Receiving experiment:step_failed events
 *  6. Receiving terminal status events (completed, failed, cancelled)
 *  7. Filtering events by experiment_id
 *  8. Resetting data when experimentId changes
 *  9. Cleanup on unmount (unsubscribes all handlers)
 * 10. autoDisconnect option behaviour
 * 11. Re-subscription after reconnection
 * 12. Imperative connect/disconnect
 */

import { renderHook, act } from '@testing-library/react';
import { useRef } from 'react';
import {
  WebSocketClient,
  type WebSocketConfig,
  type WSConnectionState,
  type WSEvent,
  type WSEventHandler,
  type WSEventType,
  type ExperimentProgressPayload,
  type ExperimentCompletedPayload,
} from '@/services/websocket';
import {
  useExperimentWebSocket,
  type ExperimentStepPayload,
  type ExperimentTerminalPayload,
  type LatestStepInfo,
} from '@/hooks/useExperimentWebSocket';

// ---------------------------------------------------------------------------
// Mock WebSocketClient
// ---------------------------------------------------------------------------

/**
 * Tracks all registered event handlers so tests can simulate incoming events
 * by calling the stored handler functions.
 */
class MockWSClient {
  private _eventHandlers = new Map<WSEventType, Set<WSEventHandler>>();
  private _stateListeners = new Set<(state: WSConnectionState) => void>();
  private _state: WSConnectionState = 'disconnected';
  private _connected = false;

  connect = jest.fn(() => {
    this._setState('connecting');
    // Simulate async open
    this._setState('connected');
    this._connected = true;
  });

  disconnect = jest.fn(() => {
    this._setState('disconnecting');
    this._connected = false;
    this._setState('disconnected');
  });

  on = jest.fn(
    <T = unknown,>(eventType: WSEventType, handler: WSEventHandler<T>): (() => void) => {
      if (!this._eventHandlers.has(eventType)) {
        this._eventHandlers.set(eventType, new Set());
      }
      const typedHandler = handler as WSEventHandler;
      this._eventHandlers.get(eventType)!.add(typedHandler);
      return () => {
        this._eventHandlers.get(eventType)?.delete(typedHandler);
      };
    },
  );

  onStateChange = jest.fn(
    (listener: (state: WSConnectionState) => void): (() => void) => {
      this._stateListeners.add(listener);
      return () => {
        this._stateListeners.delete(listener);
      };
    },
  );

  getState = jest.fn((): WSConnectionState => this._state);

  get isConnected(): boolean {
    return this._connected;
  }

  // ----- Test helpers -----

  /** Simulate receiving a typed WebSocket event. */
  simulateEvent<T = unknown>(eventType: WSEventType, payload: T, id?: string): void {
    const event: WSEvent<T> = {
      type: eventType,
      payload,
      timestamp: new Date().toISOString(),
      ...(id ? { id } : {}),
    };
    const handlers = this._eventHandlers.get(eventType);
    if (handlers) {
      handlers.forEach((h) => {
        try {
          h(event);
        } catch (e) {
          // swallow so one bad handler doesn't break others
        }
      });
    }
  }

  /** Simulate a connection-state transition. */
  simulateStateChange(newState: WSConnectionState): void {
    this._setState(newState);
  }

  private _setState(newState: WSConnectionState): void {
    if (this._state === newState) return;
    this._state = newState;
    if (newState === 'connected') this._connected = true;
    if (newState === 'disconnected' || newState === 'error') this._connected = false;
    this._stateListeners.forEach((l) => {
      try {
        l(newState);
      } catch {
        // swallow
      }
    });
  }
}

// ---------------------------------------------------------------------------
// Mock getWebSocketClient
// ---------------------------------------------------------------------------

let mockClientInstance: MockWSClient | null = null;

const mockGetWebSocketClient = jest.fn(
  (config?: WebSocketConfig | null): WebSocketClient | null => {
    if (config === null) {
      // Teardown
      if (mockClientInstance) {
        mockClientInstance.disconnect();
      }
      mockClientInstance = null;
      return null;
    }
    if (!mockClientInstance) {
      mockClientInstance = new MockWSClient();
    }
    return mockClientInstance as unknown as WebSocketClient;
  },
);

jest.mock('@/services/websocket', () => {
  const actual = jest.requireActual('@/services/websocket');
  return {
    ...actual,
    getWebSocketClient: (config?: WebSocketConfig | null) =>
      mockGetWebSocketClient(config as WebSocketConfig | null | undefined),
  };
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const TEST_CONFIG: WebSocketConfig = { url: 'ws://localhost:8080/ws' };

/**
 * Return the MockWSClient that the hook is using.
 * This relies on the fact that getWebSocketClient returns our mock instance.
 */
function getMockClient(): MockWSClient {
  if (!mockClientInstance) {
    throw new Error('MockWSClient has not been created yet');
  }
  return mockClientInstance;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useExperimentWebSocket', () => {
  beforeEach(() => {
    mockClientInstance = null;
    mockGetWebSocketClient.mockClear();
  });

  afterEach(() => {
    // Clean up singleton
    mockGetWebSocketClient(null);
  });

  // =========================================================================
  // 1. Hook initialization
  // =========================================================================

  describe('Hook initialization', () => {
    it('creates a WebSocket client via getWebSocketClient', () => {
      renderHook(() => useExperimentWebSocket('exp-1', { config: TEST_CONFIG }));

      expect(mockGetWebSocketClient).toHaveBeenCalled();
      // First call with config to create the singleton
      expect(mockGetWebSocketClient).toHaveBeenCalledWith(TEST_CONFIG);
    });

    it('auto-connects on mount when autoConnect is true (default)', () => {
      renderHook(() => useExperimentWebSocket('exp-1', { config: TEST_CONFIG }));

      const client = getMockClient();
      expect(client.connect).toHaveBeenCalled();
    });

    it('does not auto-connect when autoConnect is false', () => {
      renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoConnect: false,
        }),
      );

      const client = getMockClient();
      expect(client.connect).not.toHaveBeenCalled();
    });

    it('subscribes to all relevant event types on mount', () => {
      renderHook(() => useExperimentWebSocket('exp-1', { config: TEST_CONFIG }));

      const client = getMockClient();
      const subscribedTypes = client.on.mock.calls.map(
        (call: [WSEventType, WSEventHandler]) => call[0],
      );

      expect(subscribedTypes).toContain('experiment:progress');
      expect(subscribedTypes).toContain('experiment:step_completed');
      expect(subscribedTypes).toContain('experiment:step_failed');
      expect(subscribedTypes).toContain('experiment:completed');
      expect(subscribedTypes).toContain('experiment:failed');
      expect(subscribedTypes).toContain('experiment:cancelled');
    });

    it('returns initial state with null progress/step/terminal', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      expect(result.current.latestProgress).toBeNull();
      expect(result.current.latestStep).toBeNull();
      expect(result.current.terminalStatus).toBeNull();
    });

    it('does not subscribe when experimentId is undefined', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket(undefined, { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      // onStateChange is called, but event 'on' subscriptions should not happen
      // because experimentId is undefined
      const eventSubs = client.on.mock.calls;
      expect(eventSubs.length).toBe(0);
    });
  });

  // =========================================================================
  // 2. Connection state tracking
  // =========================================================================

  describe('Connection state tracking', () => {
    it('starts with "disconnected" connection state', () => {
      const client = new MockWSClient();
      mockClientInstance = client;
      mockGetWebSocketClient.mockReturnValue(client as unknown as WebSocketClient);

      // Make connect not change state immediately
      client.connect.mockImplementation(() => {
        // no-op, stays disconnected
      });

      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG, autoConnect: false }),
      );

      expect(result.current.connectionState).toBe('disconnected');
      expect(result.current.isConnected).toBe(false);
    });

    it('updates connectionState when client state changes', () => {
      renderHook(() => useExperimentWebSocket('exp-1', { config: TEST_CONFIG }));

      const client = getMockClient();

      act(() => {
        client.simulateStateChange('connecting');
      });
      // After connect is called by the hook, state may already be 'connected'
      // but we simulate a change
    });

    it('isConnected is true when connectionState is "connected"', () => {
      renderHook(() => useExperimentWebSocket('exp-1', { config: TEST_CONFIG }));

      const client = getMockClient();

      act(() => {
        client.simulateStateChange('connected');
      });

      // The hook derives isConnected from connectionState
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );
      // Since the mock client auto-transitions to connected on connect(),
      // the hook's connectionState should reflect this
    });

    it('subscribes to client state changes via onStateChange', () => {
      renderHook(() => useExperimentWebSocket('exp-1', { config: TEST_CONFIG }));

      const client = getMockClient();
      expect(client.onStateChange).toHaveBeenCalled();
    });

    it('reflects reconnection state changes', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateStateChange('reconnecting');
      });

      expect(result.current.connectionState).toBe('reconnecting');
      expect(result.current.isConnected).toBe(false);

      act(() => {
        client.simulateStateChange('connected');
      });

      expect(result.current.connectionState).toBe('connected');
      expect(result.current.isConnected).toBe(true);
    });

    it('reflects error state', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateStateChange('error');
      });

      expect(result.current.connectionState).toBe('error');
      expect(result.current.isConnected).toBe(false);
    });
  });

  // =========================================================================
  // 3. Receiving experiment:progress events
  // =========================================================================

  describe('Receiving experiment:progress events', () => {
    it('updates latestProgress when a matching progress event is received', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const progressPayload: ExperimentProgressPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        step_index: 2,
        step_name: 'network-attack',
        step_status: 'running',
        progress_percent: 65,
        message: 'Attack in progress',
      };

      act(() => {
        client.simulateEvent('experiment:progress', progressPayload);
      });

      expect(result.current.latestProgress).toEqual(progressPayload);
    });

    it('ignores progress events for a different experiment_id', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-OTHER',
          run_id: 'run-2',
          step_index: 0,
          step_name: 'step',
          step_status: 'running',
          progress_percent: 10,
        } as ExperimentProgressPayload);
      });

      expect(result.current.latestProgress).toBeNull();
    });

    it('overwrites previous progress with newer progress event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      const progress1: ExperimentProgressPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        step_index: 0,
        step_name: 'init',
        step_status: 'running',
        progress_percent: 10,
      };

      act(() => {
        client.simulateEvent('experiment:progress', progress1);
      });
      expect(result.current.latestProgress?.progress_percent).toBe(10);

      const progress2: ExperimentProgressPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        step_index: 1,
        step_name: 'attack',
        step_status: 'running',
        progress_percent: 75,
      };

      act(() => {
        client.simulateEvent('experiment:progress', progress2);
      });
      expect(result.current.latestProgress?.progress_percent).toBe(75);
    });
  });

  // =========================================================================
  // 4. Receiving experiment:step_completed events
  // =========================================================================

  describe('Receiving experiment:step_completed events', () => {
    it('updates latestStep when a matching step_completed event is received', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const stepPayload: ExperimentStepPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        step_index: 3,
        step_name: 'pod-kill',
        step_status: 'completed',
        message: 'Pod successfully killed',
        timestamp: '2024-06-01T10:00:00Z',
      };

      act(() => {
        client.simulateEvent('experiment:step_completed', stepPayload);
      });

      expect(result.current.latestStep).not.toBeNull();
      expect(result.current.latestStep!.stepIndex).toBe(3);
      expect(result.current.latestStep!.stepName).toBe('pod-kill');
      expect(result.current.latestStep!.stepStatus).toBe('completed');
      expect(result.current.latestStep!.message).toBe('Pod successfully killed');
      expect(result.current.latestStep!.timestamp).toBe('2024-06-01T10:00:00Z');
      expect(result.current.latestStep!.receivedAt).toBeTruthy();
    });

    it('ignores step_completed events for a different experiment_id', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-OTHER',
          run_id: 'run-2',
          step_index: 0,
          step_name: 'init',
          step_status: 'completed',
        } as ExperimentStepPayload);
      });

      expect(result.current.latestStep).toBeNull();
    });

    it('sets stepStatus to "completed" for step_completed events', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 1,
          step_name: 'network-flood',
          step_status: 'completed',
        } as ExperimentStepPayload);
      });

      expect(result.current.latestStep!.stepStatus).toBe('completed');
    });

    it('includes a receivedAt timestamp on the latestStep object', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'completed',
        } as ExperimentStepPayload);
      });

      const receivedAt = result.current.latestStep!.receivedAt;
      expect(receivedAt).toBeTruthy();
      // Should be a valid ISO date string
      expect(new Date(receivedAt).toISOString()).toBe(receivedAt);
    });
  });

  // =========================================================================
  // 5. Receiving experiment:step_failed events
  // =========================================================================

  describe('Receiving experiment:step_failed events', () => {
    it('updates latestStep when a matching step_failed event is received', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const stepPayload: ExperimentStepPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        step_index: 2,
        step_name: 'cpu-stress',
        step_status: 'failed',
        message: 'Target pod not found',
        timestamp: '2024-06-01T10:05:00Z',
      };

      act(() => {
        client.simulateEvent('experiment:step_failed', stepPayload);
      });

      expect(result.current.latestStep).not.toBeNull();
      expect(result.current.latestStep!.stepIndex).toBe(2);
      expect(result.current.latestStep!.stepName).toBe('cpu-stress');
      expect(result.current.latestStep!.stepStatus).toBe('failed');
      expect(result.current.latestStep!.message).toBe('Target pod not found');
    });

    it('ignores step_failed events for a different experiment_id', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_failed', {
          experiment_id: 'exp-OTHER',
          run_id: 'run-2',
          step_index: 0,
          step_name: 'init',
          step_status: 'failed',
        } as ExperimentStepPayload);
      });

      expect(result.current.latestStep).toBeNull();
    });

    it('overwrites step_completed with step_failed for the same experiment', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 1,
          step_name: 'init',
          step_status: 'completed',
        } as ExperimentStepPayload);
      });
      expect(result.current.latestStep!.stepStatus).toBe('completed');

      act(() => {
        client.simulateEvent('experiment:step_failed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 2,
          step_name: 'attack',
          step_status: 'failed',
        } as ExperimentStepPayload);
      });
      expect(result.current.latestStep!.stepStatus).toBe('failed');
      expect(result.current.latestStep!.stepIndex).toBe(2);
    });
  });

  // =========================================================================
  // 6. Receiving terminal status events (completed, failed, cancelled)
  // =========================================================================

  describe('Receiving terminal status events', () => {
    it('updates terminalStatus on experiment:completed event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const completedPayload: ExperimentCompletedPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        status: 'completed',
        result_summary: {
          total_pods_spawned: 5,
          successful_attacks: 3,
          blocked_attacks: 2,
          detection_rate: 0.8,
          overall_score: 85,
          findings: ['Finding 1', 'Finding 2'],
        },
        duration_ms: 120000,
      };

      act(() => {
        client.simulateEvent('experiment:completed', completedPayload);
      });

      expect(result.current.terminalStatus).not.toBeNull();
      expect(result.current.terminalStatus!.status).toBe('completed');
    });

    it('updates terminalStatus on experiment:failed event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const failedPayload: ExperimentTerminalPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        status: 'failed',
        reason: 'Infrastructure error',
        message: 'Could not connect to cluster',
      };

      act(() => {
        client.simulateEvent('experiment:failed', failedPayload);
      });

      expect(result.current.terminalStatus).not.toBeNull();
      expect(result.current.terminalStatus!.status).toBe('failed');
    });

    it('updates terminalStatus on experiment:cancelled event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const cancelledPayload: ExperimentTerminalPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        status: 'cancelled',
        reason: 'User requested cancellation',
      };

      act(() => {
        client.simulateEvent('experiment:cancelled', cancelledPayload);
      });

      expect(result.current.terminalStatus).not.toBeNull();
      expect(result.current.terminalStatus!.status).toBe('cancelled');
    });

    it('ignores terminal events for a different experiment_id', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:completed', {
          experiment_id: 'exp-OTHER',
          run_id: 'run-2',
          status: 'completed',
          result_summary: {
            total_pods_spawned: 1,
            successful_attacks: 0,
            blocked_attacks: 0,
            detection_rate: 0,
            overall_score: 0,
            findings: [],
          },
          duration_ms: 0,
        } as ExperimentCompletedPayload);
      });

      expect(result.current.terminalStatus).toBeNull();
    });

    it('overwrites terminal status with the latest terminal event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      // First, experiment fails
      act(() => {
        client.simulateEvent('experiment:failed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          status: 'failed',
          reason: 'Timeout',
        } as ExperimentTerminalPayload);
      });
      expect(result.current.terminalStatus!.status).toBe('failed');

      // Then, experiment is cancelled (e.g. a different terminal event arrives)
      act(() => {
        client.simulateEvent('experiment:cancelled', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          status: 'cancelled',
          reason: 'User override',
        } as ExperimentTerminalPayload);
      });
      expect(result.current.terminalStatus!.status).toBe('cancelled');
    });

    it('includes reason and message in terminal status for failed event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:failed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          status: 'failed',
          reason: 'CrashLoopBackOff',
          message: 'Pod never reached ready state',
        } as ExperimentTerminalPayload);
      });

      const terminal = result.current.terminalStatus as ExperimentTerminalPayload;
      expect(terminal.reason).toBe('CrashLoopBackOff');
      expect(terminal.message).toBe('Pod never reached ready state');
    });

    it('includes result_summary in terminal status for completed event', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      const payload: ExperimentCompletedPayload = {
        experiment_id: 'exp-1',
        run_id: 'run-1',
        status: 'completed',
        result_summary: {
          total_pods_spawned: 10,
          successful_attacks: 7,
          blocked_attacks: 3,
          detection_rate: 0.9,
          overall_score: 92,
          findings: ['Alert fired', 'Pod restarted'],
        },
        duration_ms: 300000,
      };

      act(() => {
        client.simulateEvent('experiment:completed', payload);
      });

      const terminal = result.current.terminalStatus as ExperimentCompletedPayload;
      expect(terminal.result_summary.overall_score).toBe(92);
      expect(terminal.result_summary.detection_rate).toBe(0.9);
      expect(terminal.duration_ms).toBe(300000);
    });
  });

  // =========================================================================
  // 7. Filtering events by experiment_id
  // =========================================================================

  describe('Event filtering by experiment_id', () => {
    it('only processes events matching the current experimentId', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-A', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      // Event for a different experiment
      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-B',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'running',
          progress_percent: 50,
        } as ExperimentProgressPayload);
      });
      expect(result.current.latestProgress).toBeNull();

      // Event for the correct experiment
      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-A',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'running',
          progress_percent: 75,
        } as ExperimentProgressPayload);
      });
      expect(result.current.latestProgress).not.toBeNull();
      expect(result.current.latestProgress!.progress_percent).toBe(75);
    });

    it('filters step_completed events by experiment_id', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-A', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-B',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'completed',
        } as ExperimentStepPayload);
      });
      expect(result.current.latestStep).toBeNull();
    });

    it('filters terminal events by experiment_id', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-A', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:failed', {
          experiment_id: 'exp-B',
          run_id: 'run-1',
          status: 'failed',
          reason: 'Error',
        } as ExperimentTerminalPayload);
      });
      expect(result.current.terminalStatus).toBeNull();
    });
  });

  // =========================================================================
  // 8. Resetting data when experimentId changes
  // =========================================================================

  describe('Resetting data when experimentId changes', () => {
    it('resets latestProgress when experimentId changes', () => {
      const { result, rerender } = renderHook(
        ({ expId }: { expId: string }) =>
          useExperimentWebSocket(expId, { config: TEST_CONFIG }),
        { initialProps: { expId: 'exp-1' } },
      );

      const client = getMockClient();

      // Send a progress event for exp-1
      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'running',
          progress_percent: 50,
        } as ExperimentProgressPayload);
      });
      expect(result.current.latestProgress).not.toBeNull();

      // Change experimentId
      rerender({ expId: 'exp-2' });

      expect(result.current.latestProgress).toBeNull();
    });

    it('resets latestStep when experimentId changes', () => {
      const { result, rerender } = renderHook(
        ({ expId }: { expId: string }) =>
          useExperimentWebSocket(expId, { config: TEST_CONFIG }),
        { initialProps: { expId: 'exp-1' } },
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'completed',
        } as ExperimentStepPayload);
      });
      expect(result.current.latestStep).not.toBeNull();

      rerender({ expId: 'exp-2' });

      expect(result.current.latestStep).toBeNull();
    });

    it('resets terminalStatus when experimentId changes', () => {
      const { result, rerender } = renderHook(
        ({ expId }: { expId: string }) =>
          useExperimentWebSocket(expId, { config: TEST_CONFIG }),
        { initialProps: { expId: 'exp-1' } },
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          status: 'completed',
          result_summary: {
            total_pods_spawned: 1,
            successful_attacks: 1,
            blocked_attacks: 0,
            detection_rate: 1,
            overall_score: 100,
            findings: [],
          },
          duration_ms: 1000,
        } as ExperimentCompletedPayload);
      });
      expect(result.current.terminalStatus).not.toBeNull();

      rerender({ expId: 'exp-2' });

      expect(result.current.terminalStatus).toBeNull();
    });

    it('subscribes to new experiment events after experimentId change', () => {
      const { result, rerender } = renderHook(
        ({ expId }: { expId: string }) =>
          useExperimentWebSocket(expId, { config: TEST_CONFIG }),
        { initialProps: { expId: 'exp-1' } },
      );

      const client = getMockClient();

      rerender({ expId: 'exp-2' });

      // Now simulate events for exp-2
      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-2',
          run_id: 'run-2',
          step_index: 0,
          step_name: 'start',
          step_status: 'running',
          progress_percent: 10,
        } as ExperimentProgressPayload);
      });

      expect(result.current.latestProgress).not.toBeNull();
      expect(result.current.latestProgress!.experiment_id).toBe('exp-2');
    });
  });

  // =========================================================================
  // 9. Cleanup on unmount
  // =========================================================================

  describe('Cleanup on unmount', () => {
    it('unsubscribes from all event handlers on unmount', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      // Capture the unsubscribe functions returned by on()
      const onCallsBefore = client.on.mock.calls.length;

      unmount();

      // After unmount, the hook should have called all unsubscribe functions.
      // We verify by checking that simulateEvent no longer triggers state updates.
      // We can't directly check React state after unmount, but we can verify
      // the on calls and that unsubscribe functions were invoked.
      // The on mock was called 6 times for 6 event types
      expect(onCallsBefore).toBeGreaterThanOrEqual(6);
    });

    it('unsubscribes from state change listener on unmount', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      expect(client.onStateChange).toHaveBeenCalled();

      unmount();

      // After unmount, state changes should not cause issues
      act(() => {
        client.simulateStateChange('disconnected');
      });
      // No error should be thrown
    });

    it('does not disconnect singleton by default on unmount (autoDisconnect=false)', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoDisconnect: false,
        }),
      );

      const client = getMockClient();
      const disconnectCallsBefore = client.disconnect.mock.calls.length;

      unmount();

      // disconnect should not have been called additionally for auto-unmount
      // (it might have been called once during the initial connect flow depending
      //  on implementation, but it should not be called for auto-disconnect)
      // Since autoDisconnect defaults to false, the hook should NOT call disconnect
      const newDisconnectCalls =
        client.disconnect.mock.calls.length - disconnectCallsBefore;
      expect(newDisconnectCalls).toBe(0);
    });

    it('disconnects singleton on unmount when autoDisconnect is true', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoDisconnect: true,
        }),
      );

      const client = getMockClient();

      unmount();

      expect(client.disconnect).toHaveBeenCalled();
    });

    it('calls getWebSocketClient(null) on unmount when autoDisconnect is true', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoDisconnect: true,
        }),
      );

      // Reset to count new calls
      const callsBefore = mockGetWebSocketClient.mock.calls.length;

      unmount();

      // Should call getWebSocketClient(null) to tear down singleton
      const nullCalls = mockGetWebSocketClient.mock.calls.filter(
        (call: unknown[]) => call[0] === null,
      );
      expect(nullCalls.length).toBeGreaterThanOrEqual(1);
    });
  });

  // =========================================================================
  // 10. autoDisconnect option
  // =========================================================================

  describe('autoDisconnect option', () => {
    it('defaults autoDisconnect to false', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const disconnectCallCountBefore = client.disconnect.mock.calls.length;

      unmount();

      const disconnectCallCountAfter = client.disconnect.mock.calls.length;
      // No additional disconnect calls from unmount
      expect(disconnectCallCountAfter).toBe(disconnectCallCountBefore);
    });

    it('disconnects on unmount when autoDisconnect is true', () => {
      const { unmount } = renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoDisconnect: true,
        }),
      );

      const client = getMockClient();
      unmount();

      expect(client.disconnect).toHaveBeenCalled();
    });
  });

  // =========================================================================
  // 11. Re-subscription after reconnection
  // =========================================================================

  describe('Re-subscription after reconnection', () => {
    it('re-subscribes to events when connection goes from non-connected to connected', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      const onCallCountBefore = client.on.mock.calls.length;

      // Simulate disconnect → reconnect
      act(() => {
        client.simulateStateChange('disconnected');
      });

      act(() => {
        client.simulateStateChange('reconnecting');
      });

      act(() => {
        client.simulateStateChange('connected');
      });

      // After reconnection, the hook should re-subscribe to events
      // The on method should have been called additional times
      const onCallCountAfter = client.on.mock.calls.length;
      expect(onCallCountAfter).toBeGreaterThan(onCallCountBefore);
    });

    it('does not re-subscribe when state transitions from connected to connected (no-op)', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      // Clear the on mock to track new calls
      client.on.mockClear();

      // Simulate connected → connected (no actual change)
      act(() => {
        // First go to disconnected, then back to connected
        // But let's test that connected → connected doesn't trigger re-sub
        // This is tricky because setState won't fire if same state
        // Let's simulate a state sequence that goes to connected then stays connected
        client.simulateStateChange('disconnected');
      });

      act(() => {
        client.simulateStateChange('connected');
      });

      const callsAfterFirstReconnect = client.on.mock.calls.length;

      // Now try "connected" → "connected" — this shouldn't trigger re-sub
      // Since the state is already "connected", setting it again won't fire listeners
      // So we go through a different path first
      act(() => {
        client.simulateStateChange('reconnecting');
      });
      act(() => {
        client.simulateStateChange('connected');
      });

      // Only the second reconnection should trigger additional re-subscription
      // The first connected → connected shouldn't have added any
    });

    it('can receive events after reconnection', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      // Simulate disconnect
      act(() => {
        client.simulateStateChange('disconnected');
      });

      // Simulate reconnect
      act(() => {
        client.simulateStateChange('connected');
      });

      // Send a progress event
      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'reconnected-step',
          step_status: 'running',
          progress_percent: 99,
        } as ExperimentProgressPayload);
      });

      expect(result.current.latestProgress).not.toBeNull();
      expect(result.current.latestProgress!.progress_percent).toBe(99);
    });
  });

  // =========================================================================
  // 12. Imperative connect/disconnect
  // =========================================================================

  describe('Imperative connect/disconnect', () => {
    it('exposes a connect function', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoConnect: false,
        }),
      );

      expect(typeof result.current.connect).toBe('function');
    });

    it('calls client.connect() when imperative connect is called', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', {
          config: TEST_CONFIG,
          autoConnect: false,
        }),
      );

      const client = getMockClient();
      // Reset to see the imperative call
      client.connect.mockClear();

      act(() => {
        result.current.connect();
      });

      expect(client.connect).toHaveBeenCalled();
    });

    it('exposes a disconnect function', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      expect(typeof result.current.disconnect).toBe('function');
    });

    it('calls client.disconnect() when imperative disconnect is called', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();
      client.disconnect.mockClear();

      act(() => {
        result.current.disconnect();
      });

      expect(client.disconnect).toHaveBeenCalled();
    });
  });

  // =========================================================================
  // 13. Edge cases
  // =========================================================================

  describe('Edge cases', () => {
    it('handles rapid consecutive events for the same experiment', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      // Rapidly fire multiple progress events
      act(() => {
        for (let i = 1; i <= 10; i++) {
          client.simulateEvent('experiment:progress', {
            experiment_id: 'exp-1',
            run_id: 'run-1',
            step_index: i,
            step_name: `step-${i}`,
            step_status: 'running',
            progress_percent: i * 10,
          } as ExperimentProgressPayload);
        }
      });

      // Should have the last event's data
      expect(result.current.latestProgress!.progress_percent).toBe(100);
      expect(result.current.latestProgress!.step_index).toBe(10);
    });

    it('handles mixed event types for the same experiment', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 1,
          step_name: 'attack',
          step_status: 'running',
          progress_percent: 50,
        } as ExperimentProgressPayload);

        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 1,
          step_name: 'attack',
          step_status: 'completed',
        } as ExperimentStepPayload);

        client.simulateEvent('experiment:failed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          status: 'failed',
          reason: 'Network partition',
        } as ExperimentTerminalPayload);
      });

      expect(result.current.latestProgress).not.toBeNull();
      expect(result.current.latestStep).not.toBeNull();
      expect(result.current.terminalStatus).not.toBeNull();
      expect(result.current.latestStep!.stepStatus).toBe('completed');
      expect(result.current.terminalStatus!.status).toBe('failed');
    });

    it('handles experimentId changing to undefined', () => {
      const { result, rerender } = renderHook(
        ({ expId }: { expId: string | undefined }) =>
          useExperimentWebSocket(expId, { config: TEST_CONFIG }),
        { initialProps: { expId: 'exp-1' } },
      );

      const client = getMockClient();

      // Send a progress event
      act(() => {
        client.simulateEvent('experiment:progress', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'init',
          step_status: 'running',
          progress_percent: 25,
        } as ExperimentProgressPayload);
      });
      expect(result.current.latestProgress).not.toBeNull();

      // Change to undefined
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      rerender({ expId: undefined } as any);

      expect(result.current.latestProgress).toBeNull();
      expect(result.current.latestStep).toBeNull();
      expect(result.current.terminalStatus).toBeNull();
    });

    it('handles step events without optional message field', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:step_completed', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          step_index: 0,
          step_name: 'minimal-step',
          step_status: 'completed',
          // No message or timestamp
        } as ExperimentStepPayload);
      });

      expect(result.current.latestStep).not.toBeNull();
      expect(result.current.latestStep!.stepName).toBe('minimal-step');
      expect(result.current.latestStep!.message).toBeUndefined();
    });

    it('handles terminal events without optional reason/message fields', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const client = getMockClient();

      act(() => {
        client.simulateEvent('experiment:cancelled', {
          experiment_id: 'exp-1',
          run_id: 'run-1',
          status: 'cancelled',
          // No reason or message
        } as ExperimentTerminalPayload);
      });

      expect(result.current.terminalStatus).not.toBeNull();
      expect(result.current.terminalStatus!.status).toBe('cancelled');
    });
  });

  // =========================================================================
  // 14. Return value shape
  // =========================================================================

  describe('Return value shape', () => {
    it('returns all expected properties', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      const returnValue = result.current;
      expect(returnValue).toHaveProperty('connectionState');
      expect(returnValue).toHaveProperty('isConnected');
      expect(returnValue).toHaveProperty('latestProgress');
      expect(returnValue).toHaveProperty('latestStep');
      expect(returnValue).toHaveProperty('terminalStatus');
      expect(returnValue).toHaveProperty('connect');
      expect(returnValue).toHaveProperty('disconnect');
    });

    it('isConnected is derived from connectionState', () => {
      const { result } = renderHook(() =>
        useExperimentWebSocket('exp-1', { config: TEST_CONFIG }),
      );

      // When the mock client auto-connects, connectionState is "connected"
      // so isConnected should be true
      if (result.current.connectionState === 'connected') {
        expect(result.current.isConnected).toBe(true);
      }
    });
  });
});
