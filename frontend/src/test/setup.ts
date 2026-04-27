// ============================================================================
// Chaos-Sec Frontend - Jest/Vitest Test Setup
// ============================================================================
// This file is loaded before each test suite via the Jest/Vitest configuration.
// It extends the Jest expect() with DOM-specific matchers from
// @testing-library/jest-dom and sets up global mocks for browser APIs that are
// not available in the jsdom test environment.
// ============================================================================

import '@testing-library/jest-dom';

// ---------------------------------------------------------------------------
// Jest ↔ Vitest compatibility shim
// ---------------------------------------------------------------------------
// When running under Vitest, the global `jest` object is not available.
// We alias it from `vi` so the rest of this file and test code can use either.
// When running under Jest, the global is already present and we skip the shim.
// ---------------------------------------------------------------------------

if (typeof jest === 'undefined') {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { vi } = await import('vitest');
  (globalThis as Record<string, unknown>).jest = vi;
}

// ---------------------------------------------------------------------------
// Mock window.matchMedia (not implemented in jsdom)
// ---------------------------------------------------------------------------

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
});

// ---------------------------------------------------------------------------
// Mock window.IntersectionObserver (not implemented in jsdom)
// ---------------------------------------------------------------------------

class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | null = null;
  readonly rootMargin: string = '';
  readonly thresholds: ReadonlyArray<number> = [];

  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }
}

Object.defineProperty(window, 'IntersectionObserver', {
  writable: true,
  value: MockIntersectionObserver,
});

// ---------------------------------------------------------------------------
// Mock window.ResizeObserver (not implemented in jsdom)
// ---------------------------------------------------------------------------

class MockResizeObserver implements ResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

Object.defineProperty(window, 'ResizeObserver', {
  writable: true,
  value: MockResizeObserver,
});

// ---------------------------------------------------------------------------
// Mock window.scrollTo (jsdom doesn't implement smooth scrolling)
// ---------------------------------------------------------------------------

window.scrollTo = jest.fn();

// ---------------------------------------------------------------------------
// Mock HTMLElement.prototype.scrollIntoView
// ---------------------------------------------------------------------------

HTMLElement.prototype.scrollIntoView = jest.fn();

// ---------------------------------------------------------------------------
// Mock HTMLDialogElement (not implemented in jsdom)
// ---------------------------------------------------------------------------

HTMLDialogElement.prototype.showModal = jest.fn();
HTMLDialogElement.prototype.close = jest.fn();

// ---------------------------------------------------------------------------
// Suppress console.error for expected React act() warnings in tests
// ---------------------------------------------------------------------------

const originalConsoleError = console.error;

console.error = (...args: unknown[]) => {
  // Filter out known noisy React warnings that are not real issues
  if (typeof args[0] === 'string') {
    const message = args[0];
    if (
      message.includes('Warning: An update to') ||
      message.includes('Warning: Cannot update a component') ||
      message.includes('Not implemented: HTMLFormElement.prototype.submit')
    ) {
      return;
    }
  }
  originalConsoleError(...args);
};

// ---------------------------------------------------------------------------
// Clean up after each test
// ---------------------------------------------------------------------------

afterEach(() => {
  jest.clearAllTimers();
});
