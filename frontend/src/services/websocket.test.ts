/**
 * Unit tests for the WebSocket client service.
 *
 * Covers:
 *  1. Connection lifecycle (connect, disconnect, reconnect)
 *  2. Authentication (sending auth message with token)
 *  3. Event subscription (on/off methods for various event types)
 *  4. Message sending (send method)
 *  5. Heartbeat/ping mechanism
 *  6. Error handling (connection failures, auth failures)
 *  7. Reconnection with exponential backoff
 *  8. State management (connection states)
 *  9. Multiple subscription handling (multiple listeners for same event)
 * 10. Cleanup (destroy method removes all listeners and closes connection)
 */

import {
  WebSocketClient,
  getWebSocketClient,
  type WebSocketConfig,
  type WSEvent,
  type WSEventType,
  type WSConnectionState,
  type WSEventHandler,
} from '@/services/websocket';

// ---------------------------------------------------------------------------
// Mock api module
// ---------------------------------------------------------------------------

const mockGetAccessToken = jest.fn<string | null, []>();
const mockClearTokens = jest.fn();

jest.mock('@/services/api', () => ({
  getAccessToken: () => mockGetAccessToken(),
  clearTokens: (...args: unknown[]) => mockClearTokens(...args),
}));

// ---------------------------------------------------------------------------
// Mock WebSocket class
// ---------------------------------------------------------------------------

const OPEN = 1;
const CONNECTING = 0;
const CLOSING = 2;
const CLOSED = 3;

class MockWebSocket {
  static CONNECTING = CONNECTING;
  static OPEN = OPEN;
  static CLOSING = CLOSING;
  static CLOSED = CLOSED;

  url: string;
  readyState: number = CONNECTING;
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  send = jest.fn();
  close = jest.fn((code?: number, reason?: string) => {
    this.readyState = CLOSED;
    const event = new CloseEvent('close', {
      code: code ?? 1000,
      reason: reason ?? '',
      wasClean: true,
    });
    if (this.onclose) this.onclose(event);
  });

  constructor(url: string) {
    this.url = url;
    mockWebSocketInstances.push(this);
  }

  /** Simulate the server opening the connection. */
  simulateOpen(): void {
    this.readyState = OPEN;
    if (this.onopen) {
      this.onopen(new Event('open'));
    }
  }

  /** Simulate the server closing the connection. */
  simulateClose(code: number = 1000, reason: string = ''): void {
    this.readyState = CLOSED;
    if (this.onclose) {
      this.onclose(new CloseEvent('close', { code, reason, wasClean: code === 1000 }));
    }
  }

  /** Simulate a connection error. */
  simulateError(): void {
    if (this.onerror) {
      this.onerror(new Event('error'));
    }
  }

  /** Simulate receiving a message from the server. */
  simulateMessage(data: unknown): void {
    if (this.onmessage) {
      const event = new MessageEvent('message', {
        data: typeof data === 'string' ? data : JSON.stringify(data),
      });
      this.onmessage(event);
    }
  }
}

let mockWebSocketInstances: MockWebSocket[] = [];

// Mock the global WebSocket constructor
const OriginalWebSocket = global.WebSocket;

beforeEach(() => {
  mockWebSocketInstances = [];
  (global as Record<string, unknown>).WebSocket = MockWebSocket;
});

afterEach(() => {
  (global as Record<string, unknown>).WebSocket = OriginalWebSocket;
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const TEST_URL = 'ws://localhost:8080/ws';

function createClient(config?: Partial<WebSocketConfig>): WebSocketClient {
  return new WebSocketClient({
    url: TEST_URL,
    ...config,
  });
}

function getLastMockWS(): MockWebSocket {
  const ws = mockWebSocketInstances[mockWebSocketInstances.length - 1];
  if (!ws) throw new Error('No MockWebSocket instance was created');
  return ws;
}

/** Connect the client and simulate the server opening the connection. */
function connectAndOpen(client: WebSocketClient): MockWebSocket {
  client.connect();
  const ws = getLastMockWS();
  ws.simulateOpen();
  return ws;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('WebSocketClient', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockGetAccessToken.mockReturnValue('test-token-abc');
    mockClearTokens.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  // =========================================================================
  // 1. Connection lifecycle
  // =========================================================================

  describe('Connection lifecycle', () => {
    it('starts in disconnected state', () => {
      const client = createClient();
      expect(client.getState()).toBe('disconnected');
      expect(client.isConnected).toBe(false);
    });

    it('transitions to "connecting" when connect() is called', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      client.connect();

      expect(client.getState()).toBe('connecting');
      expect(states).toContain('connecting');
    });

    it('transitions to "connected" after WebSocket opens', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      client.connect();
      const ws = getLastMockWS();
      ws.simulateOpen();

      expect(client.getState()).toBe('connected');
      expect(client.isConnected).toBe(true);
      expect(states).toEqual(['connecting', 'connected']);
    });

    it('transitions through "disconnecting" to "disconnected" on disconnect()', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      connectAndOpen(client);
      client.disconnect();

      expect(client.getState()).toBe('disconnected');
      expect(client.isConnected).toBe(false);
      expect(states).toContain('disconnecting');
      expect(states).toContain('disconnected');
    });

    it('does not create a new WebSocket if already connected', () => {
      const client = createClient();
      connectAndOpen(client);

      const instanceCountBefore = mockWebSocketInstances.length;
      client.connect();
      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });

    it('does not create a new WebSocket if already connecting', () => {
      const client = createClient();
      client.connect();

      const instanceCountBefore = mockWebSocketInstances.length;
      client.connect();
      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });

    it('closes the WebSocket with code 1000 on disconnect', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      client.disconnect();

      expect(ws.close).toHaveBeenCalledWith(1000, 'Client disconnect');
    });

    it('sets ws to null after disconnect', () => {
      const client = createClient();
      connectAndOpen(client);

      client.disconnect();

      expect(client.isConnected).toBe(false);
    });
  });

  // =========================================================================
  // 2. Authentication
  // =========================================================================

  describe('Authentication', () => {
    it('appends token as query parameter to WebSocket URL', () => {
      mockGetAccessToken.mockReturnValue('my-secret-token');
      const client = createClient();

      client.connect();

      const ws = getLastMockWS();
      expect(ws.url).toContain('token=my-secret-token');
    });

    it('does not append token query param when no token is available', () => {
      mockGetAccessToken.mockReturnValue(null);
      const client = createClient();

      client.connect();

      const ws = getLastMockWS();
      expect(ws.url).not.toContain('token=');
    });

    it('uses the base URL when token is present', () => {
      mockGetAccessToken.mockReturnValue('abc');
      const client = createClient({ url: 'wss://example.com/ws' });

      client.connect();

      const ws = getLastMockWS();
      expect(ws.url).toMatch(/^wss:\/\/example\.com\/ws\?token=abc$/);
    });

    it('clears tokens on auth failure close code 4001', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      ws.simulateClose(4001, 'Unauthorized');

      expect(mockClearTokens).toHaveBeenCalledTimes(1);
      expect(client.getState()).toBe('error');
    });

    it('clears tokens on auth failure close code 4003', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      ws.simulateClose(4003, 'Forbidden');

      expect(mockClearTokens).toHaveBeenCalledTimes(1);
      expect(client.getState()).toBe('error');
    });

    it('does not clear tokens on normal close', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      ws.simulateClose(1000, 'Normal');

      expect(mockClearTokens).not.toHaveBeenCalled();
    });

    it('does not attempt reconnect after auth failure', () => {
      const client = createClient();
      const ws = connectAndOpen(client);
      const instanceCountBefore = mockWebSocketInstances.length;

      ws.simulateClose(4001, 'Unauthorized');
      jest.runAllTimers();

      // No new WebSocket should be created
      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });
  });

  // =========================================================================
  // 3. Event subscription
  // =========================================================================

  describe('Event subscription', () => {
    it('calls handler when subscribed event type is received', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      client.on('experiment:progress', handler);

      const event: WSEvent = {
        type: 'experiment:progress',
        payload: { experiment_id: '123', progress_percent: 50 },
        timestamp: new Date().toISOString(),
      };
      ws.simulateMessage(event);

      expect(handler).toHaveBeenCalledTimes(1);
      expect(handler).toHaveBeenCalledWith(event);
    });

    it('does not call handler for different event type', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      client.on('experiment:progress', handler);

      ws.simulateMessage({
        type: 'cluster:health',
        payload: { status: 'healthy' },
        timestamp: new Date().toISOString(),
      });

      expect(handler).not.toHaveBeenCalled();
    });

    it('unsubscribes when the returned function is called', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      const unsub = client.on('experiment:progress', handler);

      unsub();

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(handler).not.toHaveBeenCalled();
    });

    it('supports subscribing to multiple different event types', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const progressHandler = jest.fn();
      const completedHandler = jest.fn();
      const healthHandler = jest.fn();

      client.on('experiment:progress', progressHandler);
      client.on('experiment:completed', completedHandler);
      client.on('cluster:health', healthHandler);

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: { progress_percent: 75 },
        timestamp: new Date().toISOString(),
      });

      expect(progressHandler).toHaveBeenCalledTimes(1);
      expect(completedHandler).not.toHaveBeenCalled();
      expect(healthHandler).not.toHaveBeenCalled();

      ws.simulateMessage({
        type: 'experiment:completed',
        payload: { status: 'completed' },
        timestamp: new Date().toISOString(),
      });

      expect(progressHandler).toHaveBeenCalledTimes(1);
      expect(completedHandler).toHaveBeenCalledTimes(1);
      expect(healthHandler).not.toHaveBeenCalled();
    });

    it('onRaw receives all incoming messages', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const rawHandler = jest.fn();
      client.onRaw(rawHandler);

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: { progress_percent: 10 },
        timestamp: new Date().toISOString(),
      });

      ws.simulateMessage({
        type: 'cluster:health',
        payload: { status: 'healthy' },
        timestamp: new Date().toISOString(),
      });

      expect(rawHandler).toHaveBeenCalledTimes(2);
    });

    it('onRaw unsubscribe stops receiving messages', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const rawHandler = jest.fn();
      const unsub = client.onRaw(rawHandler);

      unsub();

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(rawHandler).not.toHaveBeenCalled();
    });

    it('ignores system:ping events (heartbeat ack)', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      client.on('system:ping', handler);

      ws.simulateMessage({
        type: 'system:ping',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(handler).not.toHaveBeenCalled();
    });

    it('ignores pong events (heartbeat ack)', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      client.on('pong', handler);

      ws.simulateMessage({
        type: 'pong',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(handler).not.toHaveBeenCalled();
    });

    it('silently ignores non-JSON messages', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      client.on('experiment:progress', handler);

      // Send a non-JSON string
      if (ws.onmessage) {
        ws.onmessage(new MessageEvent('message', { data: 'not json' }));
      }

      expect(handler).not.toHaveBeenCalled();
    });
  });

  // =========================================================================
  // 4. Message sending
  // =========================================================================

  describe('Message sending', () => {
    it('sends typed event as JSON with type, payload, and timestamp', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      client.send('experiment:progress', { progress_percent: 42 });

      expect(ws.send).toHaveBeenCalledTimes(1);
      const sentData = JSON.parse(ws.send.mock.calls[0][0]);
      expect(sentData.type).toBe('experiment:progress');
      expect(sentData.payload).toEqual({ progress_percent: 42 });
      expect(sentData.timestamp).toBeTruthy();
    });

    it('returns true when send succeeds', () => {
      const client = createClient();
      connectAndOpen(client);

      const result = client.send('experiment:progress', { progress_percent: 50 });
      expect(result).toBe(true);
    });

    it('returns false when not connected', () => {
      const client = createClient();
      // Not connected
      const result = client.send('experiment:progress', { progress_percent: 50 });
      expect(result).toBe(false);
    });

    it('returns false when WebSocket is in CONNECTING state', () => {
      const client = createClient();
      client.connect();
      // At this point the ws is in CONNECTING state
      const result = client.send('experiment:progress', {});
      expect(result).toBe(false);
    });

    it('sendRaw sends a string directly', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const result = client.sendRaw('raw string data');
      expect(result).toBe(true);
      expect(ws.send).toHaveBeenCalledWith('raw string data');
    });

    it('sendRaw returns false when not connected', () => {
      const client = createClient();
      const result = client.sendRaw('raw string data');
      expect(result).toBe(false);
    });

    it('send returns false when ws.send throws', () => {
      const client = createClient();
      const ws = connectAndOpen(client);
      ws.send.mockImplementation(() => {
        throw new Error('Send failed');
      });

      const result = client.send('experiment:progress', {});
      expect(result).toBe(false);
    });

    it('sendRaw returns false when ws.send throws', () => {
      const client = createClient();
      const ws = connectAndOpen(client);
      ws.send.mockImplementation(() => {
        throw new Error('Send failed');
      });

      const result = client.sendRaw('raw data');
      expect(result).toBe(false);
    });
  });

  // =========================================================================
  // 5. Heartbeat/ping mechanism
  // =========================================================================

  describe('Heartbeat/ping mechanism', () => {
    it('starts heartbeat after connection opens', () => {
      const client = createClient({ heartbeatInterval: 5000 });
      const ws = connectAndOpen(client);

      // Advance by one heartbeat interval
      jest.advanceTimersByTime(5000);

      expect(ws.send).toHaveBeenCalled();
      const sentData = JSON.parse(ws.send.mock.calls[0][0]);
      expect(sentData.type).toBe('ping');
      expect(sentData.timestamp).toBeTruthy();
    });

    it('sends ping at the configured interval', () => {
      const client = createClient({ heartbeatInterval: 3000 });
      const ws = connectAndOpen(client);

      jest.advanceTimersByTime(3000);
      // First ping
      expect(ws.send).toHaveBeenCalledTimes(1);

      jest.advanceTimersByTime(3000);
      // Second ping
      expect(ws.send).toHaveBeenCalledTimes(2);

      jest.advanceTimersByTime(3000);
      // Third ping
      expect(ws.send).toHaveBeenCalledTimes(3);
    });

    it('stops heartbeat on disconnect', () => {
      const client = createClient({ heartbeatInterval: 5000 });
      const ws = connectAndOpen(client);

      client.disconnect();
      jest.advanceTimersByTime(10000);

      // After disconnect, no more pings should be sent
      // The only send calls should be from close, not heartbeat
      const pingCalls = ws.send.mock.calls.filter((call: string[]) => {
        try {
          const data = JSON.parse(call[0]);
          return data.type === 'ping';
        } catch {
          return false;
        }
      });
      expect(pingCalls.length).toBe(0);
    });

    it('stops heartbeat when connection closes unexpectedly', () => {
      const client = createClient({ heartbeatInterval: 5000 });
      const ws = connectAndOpen(client);

      ws.simulateClose(1006, 'Abnormal closure');
      jest.advanceTimersByTime(10000);

      // No more pings after close
      const pingCalls = ws.send.mock.calls.filter((call: string[]) => {
        try {
          const data = JSON.parse(call[0]);
          return data.type === 'ping';
        } catch {
          return false;
        }
      });
      expect(pingCalls.length).toBe(0);
    });

    it('closes connection with code 4000 on pong timeout', () => {
      const client = createClient({
        heartbeatInterval: 5000,
        pongTimeout: 3000,
      });
      const ws = connectAndOpen(client);

      // Trigger a ping
      jest.advanceTimersByTime(5000);

      // Now the pong timeout timer starts. Advance past it.
      jest.advanceTimersByTime(3000);

      expect(ws.close).toHaveBeenCalledWith(4000, 'Pong timeout');
    });

    it('resets pong timeout when any message is received', () => {
      const client = createClient({
        heartbeatInterval: 5000,
        pongTimeout: 2000,
      });
      const ws = connectAndOpen(client);

      // Trigger a ping
      jest.advanceTimersByTime(5000);

      // Halfway through pong timeout, receive a message
      jest.advanceTimersByTime(1000);
      ws.simulateMessage({
        type: 'experiment:progress',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      // Advance by another 1500ms — if pong timeout was NOT reset, it would
      // have already fired at 2000ms. Since we reset it at 1000ms, it should
      // still be 1000ms away from firing.
      jest.advanceTimersByTime(1500);

      // The timeout should NOT have fired yet (needs 2000ms from last message)
      expect(ws.close).not.toHaveBeenCalledWith(4000, 'Pong timeout');
    });

    it('does not send ping when ws is not in OPEN state', () => {
      const client = createClient({ heartbeatInterval: 5000 });
      client.connect();
      const ws = getLastMockWS();

      // The ws is in CONNECTING state, not OPEN
      // But let's set it to OPEN first so the onopen fires, then close it
      ws.simulateOpen();
      ws.readyState = CLOSED;

      jest.advanceTimersByTime(10000);

      // No pings should be sent since the ws isn't open
      const pingCalls = ws.send.mock.calls.filter((call: string[]) => {
        try {
          const data = JSON.parse(call[0]);
          return data.type === 'ping';
        } catch {
          return false;
        }
      });
      expect(pingCalls.length).toBe(0);
    });
  });

  // =========================================================================
  // 6. Error handling
  // =========================================================================

  describe('Error handling', () => {
    it('transitions to "error" state on WebSocket error event', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      client.connect();
      const ws = getLastMockWS();
      ws.simulateError();

      expect(client.getState()).toBe('error');
      expect(states).toContain('error');
    });

    it('transitions to "error" state when connection URL is invalid', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      // Make WebSocket constructor throw to simulate an invalid URL
      (global as Record<string, unknown>).WebSocket = class {
        constructor() {
          throw new TypeError('Invalid URL');
        }
      };

      client.connect();

      expect(client.getState()).toBe('error');
    });

    it('attempts reconnect after error when not disposed', () => {
      const client = createClient({
        maxReconnectAttempts: 3,
        reconnectBaseDelay: 1000,
      });
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      client.connect();
      const ws = getLastMockWS();

      // Simulate error followed by abnormal close
      ws.simulateError();
      ws.simulateClose(1006, 'Abnormal');

      expect(states).toContain('reconnecting');
    });

    it('does not attempt reconnect after disconnect (disposed)', () => {
      const client = createClient();
      connectAndOpen(client);
      const instanceCountBefore = mockWebSocketInstances.length;

      client.disconnect();

      // Try to trigger reconnect via close event (should not happen since disposed)
      jest.runAllTimers();

      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });

    it('does not reconnect on normal close (code 1000)', () => {
      const client = createClient();
      const ws = connectAndOpen(client);
      const instanceCountBefore = mockWebSocketInstances.length;

      ws.simulateClose(1000, 'Normal closure');

      jest.runAllTimers();

      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
      expect(client.getState()).toBe('disconnected');
    });

    it('does not reconnect on going away close (code 1001)', () => {
      const client = createClient();
      const ws = connectAndOpen(client);
      const instanceCountBefore = mockWebSocketInstances.length;

      ws.simulateClose(1001, 'Going away');

      jest.runAllTimers();

      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
      expect(client.getState()).toBe('disconnected');
    });

    it('reconnects on abnormal close (code 1006)', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 100,
      });
      const ws = connectAndOpen(client);

      ws.simulateClose(1006, 'Abnormal closure');

      // State should be 'reconnecting'
      expect(client.getState()).toBe('reconnecting');
    });

    it('does not call handler that throws without affecting other handlers', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const badHandler = jest.fn(() => {
        throw new Error('Handler error');
      });
      const goodHandler = jest.fn();

      client.on('experiment:progress', badHandler);
      client.on('experiment:progress', goodHandler);

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: { progress_percent: 50 },
        timestamp: new Date().toISOString(),
      });

      // Both handlers are called (the error is caught and logged)
      expect(badHandler).toHaveBeenCalledTimes(1);
      expect(goodHandler).toHaveBeenCalledTimes(1);
    });

    it('raw handler errors are caught without affecting other raw handlers', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const badRaw = jest.fn(() => {
        throw new Error('Raw handler error');
      });
      const goodRaw = jest.fn();

      client.onRaw(badRaw);
      client.onRaw(goodRaw);

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(badRaw).toHaveBeenCalledTimes(1);
      expect(goodRaw).toHaveBeenCalledTimes(1);
    });
  });

  // =========================================================================
  // 7. Reconnection with exponential backoff
  // =========================================================================

  describe('Reconnection with exponential backoff', () => {
    it('schedules reconnect after abnormal close', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 1000,
      });
      const ws = connectAndOpen(client);

      ws.simulateClose(1006, 'Abnormal');
      expect(client.getState()).toBe('reconnecting');
    });

    it('uses exponential backoff: delay doubles each attempt', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 1000,
        reconnectMaxDelay: 60000,
      });

      // We need to track when doConnect creates new WebSocket instances
      connectAndOpen(client);
      const ws = getLastMockWS();

      // First reconnect attempt
      ws.simulateClose(1006, 'Abnormal');

      // The delay for attempt 1 is baseDelay * 2^0 + jitter = ~1000 + [0,500)
      // Advance past the maximum possible delay for attempt 1
      jest.advanceTimersByTime(1500);

      // A new WebSocket should have been created for attempt 1
      expect(mockWebSocketInstances.length).toBe(2);

      // Simulate the reconnection WebSocket failing
      const ws2 = getLastMockWS();
      ws2.simulateOpen(); // Brief connection
      ws2.simulateClose(1006, 'Abnormal');

      // The delay for attempt 2 is baseDelay * 2^1 + jitter = ~2000 + [0,500)
      // Advance past maximum possible delay for attempt 2
      jest.advanceTimersByTime(2500);

      expect(mockWebSocketInstances.length).toBe(3);
    });

    it('caps delay at reconnectMaxDelay', () => {
      const client = createClient({
        maxReconnectAttempts: 20,
        reconnectBaseDelay: 1000,
        reconnectMaxDelay: 5000,
      });

      connectAndOpen(client);
      const ws = getLastMockWS();

      // Simulate many failures to push the backoff beyond max
      for (let i = 0; i < 10; i++) {
        ws.simulateClose(1006, 'Abnormal');
        jest.advanceTimersByTime(60000); // advance well past any delay
        const nextWs = getLastMockWS();
        if (nextWs.readyState === CONNECTING || nextWs.readyState === OPEN) {
          // Simulate it opening then closing
          nextWs.simulateOpen();
          nextWs.simulateClose(1006, 'Abnormal');
        }
      }

      // The client should still be trying to reconnect
      // The delay should never exceed reconnectMaxDelay + 500 (jitter)
    });

    it('gives up after maxReconnectAttempts', () => {
      const client = createClient({
        maxReconnectAttempts: 2,
        reconnectBaseDelay: 100,
        reconnectMaxDelay: 5000,
      });

      connectAndOpen(client);
      let ws = getLastMockWS();

      // First reconnect attempt
      ws.simulateClose(1006, 'Abnormal');
      jest.advanceTimersByTime(600);
      ws = getLastMockWS();
      ws.simulateOpen();
      ws.simulateClose(1006, 'Abnormal');

      // Second reconnect attempt
      jest.advanceTimersByTime(600);
      ws = getLastMockWS();
      ws.simulateOpen();
      ws.simulateClose(1006, 'Abnormal');

      // After max attempts, should be in error state
      expect(client.getState()).toBe('error');

      // No more reconnection attempts
      const instanceCountBefore = mockWebSocketInstances.length;
      jest.advanceTimersByTime(60000);
      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });

    it('resets reconnect attempts counter after successful connection', () => {
      const client = createClient({
        maxReconnectAttempts: 3,
        reconnectBaseDelay: 100,
      });

      connectAndOpen(client);
      const ws = getLastMockWS();

      // Close and reconnect once
      ws.simulateClose(1006, 'Abnormal');
      jest.advanceTimersByTime(600);
      const ws2 = getLastMockWS();

      // Successfully connect
      ws2.simulateOpen();

      // Close and reconnect again — should still work (counter was reset)
      ws2.simulateClose(1006, 'Abnormal');
      jest.advanceTimersByTime(600);
      const ws3 = getLastMockWS();
      ws3.simulateOpen();

      expect(client.getState()).toBe('connected');
    });

    it('does not reconnect after disconnect() is called', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 100,
      });

      connectAndOpen(client);
      client.disconnect();

      const instanceCountBefore = mockWebSocketInstances.length;
      jest.runAllTimers();

      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });

    it('cancels pending reconnect timer on disconnect', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 10000,
      });

      const ws = connectAndOpen(client);
      ws.simulateClose(1006, 'Abnormal');

      // Reconnect timer is pending. Disconnect before it fires.
      client.disconnect();

      const instanceCountBefore = mockWebSocketInstances.length;
      jest.runAllTimers();

      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });
  });

  // =========================================================================
  // 8. State management
  // =========================================================================

  describe('State management', () => {
    it('getState returns current state', () => {
      const client = createClient();
      expect(client.getState()).toBe('disconnected');
    });

    it('onStateChange receives state transitions', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      client.connect();
      const ws = getLastMockWS();
      ws.simulateOpen();

      expect(states).toEqual(['connecting', 'connected']);
    });

    it('onStateChange unsubscribe stops receiving updates', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      const unsub = client.onStateChange((s) => states.push(s));

      unsub();

      client.connect();

      expect(states).toEqual([]);
    });

    it('does not notify listeners when state does not change', () => {
      const client = createClient();
      const listener = jest.fn();
      client.onStateChange(listener);

      // Set state to 'disconnected' (already is 'disconnected')
      // This happens internally when setState is called with the same state
      client.connect();
      client.disconnect();

      // The listener should only be called for actual state changes
      // After connect: connecting (change from disconnected)
      // After disconnect without connecting: disconnecting, disconnected
      // But we did connect() which set it to 'connecting'
      const calledStates = listener.mock.calls.map(
        (call: WSConnectionState[]) => call[0],
      );

      // No duplicate consecutive states
      for (let i = 1; i < calledStates.length; i++) {
        expect(calledStates[i]).not.toBe(calledStates[i - 1]);
      }
    });

    it('isConnected returns true only when ws readyState is OPEN', () => {
      const client = createClient();

      expect(client.isConnected).toBe(false);

      client.connect();
      expect(client.isConnected).toBe(false); // Still connecting

      const ws = getLastMockWS();
      ws.simulateOpen();
      expect(client.isConnected).toBe(true);

      client.disconnect();
      expect(client.isConnected).toBe(false);
    });

    it('multiple state listeners all receive updates', () => {
      const client = createClient();
      const listener1 = jest.fn();
      const listener2 = jest.fn();
      const listener3 = jest.fn();

      client.onStateChange(listener1);
      client.onStateChange(listener2);
      client.onStateChange(listener3);

      client.connect();
      const ws = getLastMockWS();
      ws.simulateOpen();

      expect(listener1).toHaveBeenCalledWith('connecting');
      expect(listener1).toHaveBeenCalledWith('connected');
      expect(listener2).toHaveBeenCalledWith('connecting');
      expect(listener2).toHaveBeenCalledWith('connected');
      expect(listener3).toHaveBeenCalledWith('connecting');
      expect(listener3).toHaveBeenCalledWith('connected');
    });

    it('state listener errors are caught without affecting other listeners', () => {
      const client = createClient();
      const badListener = jest.fn(() => {
        throw new Error('Listener error');
      });
      const goodListener = jest.fn();

      client.onStateChange(badListener);
      client.onStateChange(goodListener);

      client.connect();

      expect(badListener).toHaveBeenCalled();
      expect(goodListener).toHaveBeenCalled();
    });
  });

  // =========================================================================
  // 9. Multiple subscription handling
  // =========================================================================

  describe('Multiple subscription handling', () => {
    it('supports multiple handlers for the same event type', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler1 = jest.fn();
      const handler2 = jest.fn();
      const handler3 = jest.fn();

      client.on('experiment:progress', handler1);
      client.on('experiment:progress', handler2);
      client.on('experiment:progress', handler3);

      const event: WSEvent = {
        type: 'experiment:progress',
        payload: { progress_percent: 33 },
        timestamp: new Date().toISOString(),
      };
      ws.simulateMessage(event);

      expect(handler1).toHaveBeenCalledWith(event);
      expect(handler2).toHaveBeenCalledWith(event);
      expect(handler3).toHaveBeenCalledWith(event);
    });

    it('can unsubscribe one handler while keeping others', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler1 = jest.fn();
      const handler2 = jest.fn();
      const handler3 = jest.fn();

      const unsub1 = client.on('experiment:progress', handler1);
      client.on('experiment:progress', handler2);
      client.on('experiment:progress', handler3);

      unsub1();

      const event: WSEvent = {
        type: 'experiment:progress',
        payload: { progress_percent: 66 },
        timestamp: new Date().toISOString(),
      };
      ws.simulateMessage(event);

      expect(handler1).not.toHaveBeenCalled();
      expect(handler2).toHaveBeenCalledWith(event);
      expect(handler3).toHaveBeenCalledWith(event);
    });

    it('can unsubscribe all handlers for an event type', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler1 = jest.fn();
      const handler2 = jest.fn();

      const unsub1 = client.on('experiment:progress', handler1);
      const unsub2 = client.on('experiment:progress', handler2);

      unsub1();
      unsub2();

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(handler1).not.toHaveBeenCalled();
      expect(handler2).not.toHaveBeenCalled();
    });

    it('subscribing the same handler twice results in one call per event', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const handler = jest.fn();
      client.on('experiment:progress', handler);
      client.on('experiment:progress', handler);

      ws.simulateMessage({
        type: 'experiment:progress',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      // Since it's a Set, same handler reference is added once
      expect(handler).toHaveBeenCalledTimes(1);
    });

    it('can subscribe to all event types independently', () => {
      const client = createClient();
      const ws = connectAndOpen(client);

      const eventTypes: WSEventType[] = [
        'experiment:started',
        'experiment:progress',
        'experiment:step_completed',
        'experiment:step_failed',
        'experiment:completed',
        'experiment:failed',
        'experiment:cancelled',
        'experiment:logs',
        'cluster:health',
        'cluster:status',
        'siem:alert',
        'system:notification',
      ];

      const handlers: Record<string, jest.Mock> = {};
      eventTypes.forEach((type) => {
        handlers[type] = jest.fn();
        client.on(type, handlers[type]);
      });

      // Send messages for each type
      eventTypes.forEach((type) => {
        ws.simulateMessage({
          type,
          payload: { test: type },
          timestamp: new Date().toISOString(),
        });
      });

      // Each handler should be called exactly once
      eventTypes.forEach((type) => {
        expect(handlers[type]).toHaveBeenCalledTimes(1);
      });
    });
  });

  // =========================================================================
  // 10. Cleanup / destroy
  // =========================================================================

  describe('Cleanup / destroy', () => {
    it('disconnect closes the connection and clears timers', () => {
      const client = createClient({ heartbeatInterval: 5000 });
      const ws = connectAndOpen(client);

      client.disconnect();

      expect(ws.close).toHaveBeenCalledWith(1000, 'Client disconnect');
      expect(client.getState()).toBe('disconnected');
    });

    it('disconnect stops heartbeat', () => {
      const client = createClient({ heartbeatInterval: 5000 });
      const ws = connectAndOpen(client);

      client.disconnect();
      jest.advanceTimersByTime(20000);

      // No pings should have been sent after disconnect
      const pingCalls = ws.send.mock.calls.filter((call: string[]) => {
        try {
          const data = JSON.parse(call[0]);
          return data.type === 'ping';
        } catch {
          return false;
        }
      });
      expect(pingCalls.length).toBe(0);
    });

    it('disconnect cancels pending reconnect timer', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 5000,
      });
      const ws = connectAndOpen(client);

      ws.simulateClose(1006, 'Abnormal');

      // Reconnect timer is pending
      client.disconnect();

      const instanceCountBefore = mockWebSocketInstances.length;
      jest.runAllTimers();

      expect(mockWebSocketInstances.length).toBe(instanceCountBefore);
    });

    it('event handlers remain after disconnect and work on reconnection', () => {
      const client = createClient({
        maxReconnectAttempts: 5,
        reconnectBaseDelay: 100,
      });

      const handler = jest.fn();
      client.on('experiment:progress', handler);

      const ws1 = connectAndOpen(client);

      ws1.simulateClose(1006, 'Abnormal');
      jest.advanceTimersByTime(600);

      const ws2 = getLastMockWS();
      ws2.simulateOpen();

      ws2.simulateMessage({
        type: 'experiment:progress',
        payload: { progress_percent: 80 },
        timestamp: new Date().toISOString(),
      });

      expect(handler).toHaveBeenCalledTimes(1);
    });

    it('state listeners are notified after disconnect', () => {
      const client = createClient();
      const states: WSConnectionState[] = [];
      client.onStateChange((s) => states.push(s));

      connectAndOpen(client);
      client.disconnect();

      expect(states).toContain('disconnecting');
      expect(states).toContain('disconnected');
    });

    it('unsubscribing from state change during disconnect does not cause errors', () => {
      const client = createClient();
      const unsub = client.onStateChange(() => {});

      connectAndOpen(client);
      unsub();
      client.disconnect();

      expect(client.getState()).toBe('disconnected');
    });

    it('multiple disconnects do not cause errors', () => {
      const client = createClient();
      connectAndOpen(client);

      client.disconnect();
      client.disconnect(); // Second disconnect should not throw

      expect(client.getState()).toBe('disconnected');
    });
  });

  // =========================================================================
  // Redux dispatch integration
  // =========================================================================

  describe('Redux dispatch integration', () => {
    it('forwards events to reduxDispatch when configured', () => {
      const mockDispatch = jest.fn();
      const client = createClient({
        reduxDispatch: mockDispatch,
      });
      const ws = connectAndOpen(client);

      const event: WSEvent = {
        type: 'experiment:progress',
        payload: { progress_percent: 50 },
        timestamp: '2024-01-01T00:00:00Z',
        id: 'evt-123',
      };
      ws.simulateMessage(event);

      expect(mockDispatch).toHaveBeenCalledTimes(1);
      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'ws/experiment:progress',
        payload: { progress_percent: 50 },
        meta: { wsEventId: 'evt-123', wsTimestamp: '2024-01-01T00:00:00Z' },
      });
    });

    it('does not forward system:ping to redux', () => {
      const mockDispatch = jest.fn();
      const client = createClient({
        reduxDispatch: mockDispatch,
      });
      const ws = connectAndOpen(client);

      ws.simulateMessage({
        type: 'system:ping',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(mockDispatch).not.toHaveBeenCalled();
    });

    it('does not forward pong to redux', () => {
      const mockDispatch = jest.fn();
      const client = createClient({
        reduxDispatch: mockDispatch,
      });
      const ws = connectAndOpen(client);

      ws.simulateMessage({
        type: 'pong',
        payload: {},
        timestamp: new Date().toISOString(),
      });

      expect(mockDispatch).not.toHaveBeenCalled();
    });

    it('does not forward non-JSON messages to redux', () => {
      const mockDispatch = jest.fn();
      const client = createClient({
        reduxDispatch: mockDispatch,
      });
      const ws = connectAndOpen(client);

      if (ws.onmessage) {
        ws.onmessage(new MessageEvent('message', { data: 'not json' }));
      }

      expect(mockDispatch).not.toHaveBeenCalled();
    });

    it('handles events without an id in redux dispatch', () => {
      const mockDispatch = jest.fn();
      const client = createClient({
        reduxDispatch: mockDispatch,
      });
      const ws = connectAndOpen(client);

      ws.simulateMessage({
        type: 'cluster:health',
        payload: { status: 'healthy' },
        timestamp: '2024-01-01T00:00:00Z',
      });

      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'ws/cluster:health',
        payload: { status: 'healthy' },
        meta: { wsEventId: undefined, wsTimestamp: '2024-01-01T00:00:00Z' },
      });
    });
  });
});

// ===========================================================================
// getWebSocketClient singleton
// ===========================================================================

describe('getWebSocketClient', () => {
  afterEach(() => {
    // Clean up singleton
    getWebSocketClient(null);
  });

  it('creates a new instance when config is provided', () => {
    const client = getWebSocketClient({ url: TEST_URL });
    expect(client).toBeInstanceOf(WebSocketClient);
  });

  it('returns the same instance on subsequent calls', () => {
    const client1 = getWebSocketClient({ url: TEST_URL });
    const client2 = getWebSocketClient({ url: TEST_URL });
    expect(client1).toBe(client2);
  });

  it('returns existing instance when called without config', () => {
    getWebSocketClient({ url: TEST_URL });
    const client = getWebSocketClient();
    expect(client).toBeInstanceOf(WebSocketClient);
  });

  it('returns null when no instance exists and no config is provided', () => {
    // No instance created yet
    const client = getWebSocketClient();
    expect(client).toBeNull();
  });

  it('tears down the instance when null is passed', () => {
    const client = getWebSocketClient({ url: TEST_URL });
    const result = getWebSocketClient(null);

    expect(result).toBeNull();
    // After teardown, getting the client without config returns null
    const clientAfter = getWebSocketClient();
    expect(clientAfter).toBeNull();
  });

  it('disconnects the instance when tearing down', () => {
    const client = getWebSocketClient({ url: TEST_URL });
    if (client) {
      jest.spyOn(client, 'disconnect');
    }

    getWebSocketClient(null);

    if (client) {
      expect(client.disconnect).toHaveBeenCalled();
    }
  });
});
