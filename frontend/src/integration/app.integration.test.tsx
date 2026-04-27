/**
 * App Integration Tests
 *
 * Covers end-to-end routing and authentication guard behaviour:
 *  1. App renders the login page when not authenticated
 *  2. Navigation between pages (using MemoryRouter + mock Layout nav links)
 *  3. Protected routes redirect to login when unauthenticated
 *  4. Authenticated users can access the dashboard
 *  5. Catch-all / unknown route handling
 */

import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import App from '@/App';
import authReducer from '@/store/authSlice';
import experimentReducer from '@/store/experimentSlice';
import {
  authAPI,
  getAccessToken,
  getRefreshToken,
  setTokens,
  clearTokens,
  getErrorMessage,
} from '@/services/api';
import type { AuthState } from '@/types';
import lightTheme from '@/theme';
import { ThemeProvider } from '@mui/material/styles';

// ---------------------------------------------------------------------------
// Mocks – API module
// ---------------------------------------------------------------------------

jest.mock('@/services/api', () => ({
  authAPI: {
    login: jest.fn(),
    logout: jest.fn(),
    refresh: jest.fn(),
    me: jest.fn(),
    updateProfile: jest.fn(),
    changePassword: jest.fn(),
  },
  experimentsAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    execute: jest.fn(),
    stop: jest.fn(),
    getRuns: jest.fn(),
    getRunById: jest.fn(),
    getLogs: jest.fn(),
    getResults: jest.fn(),
  },
  templatesAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    getCategories: jest.fn(),
  },
  clustersAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    register: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
    healthCheck: jest.fn(),
    getNamespaces: jest.fn(),
    getResources: jest.fn(),
  },
  dashboardAPI: {
    getSummary: jest.fn(),
    getSecurityPosture: jest.fn(),
    getRecentExperiments: jest.fn(),
    getClusterHealth: jest.fn(),
    getActivityTimeline: jest.fn(),
    getMetrics: jest.fn(),
  },
  reportsAPI: {
    list: jest.fn(),
    getById: jest.fn(),
    generate: jest.fn(),
    download: jest.fn(),
    delete: jest.fn(),
    share: jest.fn(),
    schedule: jest.fn(),
  },
  siemAPI: {
    getAlerts: jest.fn(),
    getAlertById: jest.fn(),
    acknowledgeAlert: jest.fn(),
    dismissAlert: jest.fn(),
    getValidationResults: jest.fn(),
    testConnection: jest.fn(),
    getConfigurations: jest.fn(),
    updateConfiguration: jest.fn(),
  },
  getAccessToken: jest.fn(),
  getRefreshToken: jest.fn(),
  setTokens: jest.fn(),
  clearTokens: jest.fn(),
  getErrorMessage: jest.fn((_err: unknown) => 'An unexpected error occurred'),
}));

const mockedGetAccessToken = getAccessToken as jest.MockedFunction<typeof getAccessToken>;
const mockedGetRefreshToken = getRefreshToken as jest.MockedFunction<
  typeof getRefreshToken
>;

// ---------------------------------------------------------------------------
// Mocks – Lazy-loaded page components
// ---------------------------------------------------------------------------
// Each mock exports a simple component with a data-testid so the routing
// integration can verify which page rendered without pulling in the full
// page implementation (which would require many more mocks).
// ---------------------------------------------------------------------------

jest.mock('@/pages/LoginPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'login-page' }, 'Login Page'),
  };
});

jest.mock('@/pages/RegisterPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'register-page' }, 'Register Page'),
  };
});

jest.mock('@/pages/DashboardPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'dashboard-page' }, 'Dashboard Page'),
  };
});

jest.mock('@/pages/ExperimentListPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement(
        'div',
        { 'data-testid': 'experiment-list-page' },
        'Experiments',
      ),
  };
});

jest.mock('@/pages/CreateExperimentPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement(
        'div',
        { 'data-testid': 'create-experiment-page' },
        'Create Experiment',
      ),
  };
});

jest.mock('@/pages/ExperimentDetailPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement(
        'div',
        { 'data-testid': 'experiment-detail-page' },
        'Experiment Detail',
      ),
  };
});

jest.mock('@/pages/ClusterListPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'cluster-list-page' }, 'Clusters'),
  };
});

jest.mock('@/pages/TemplateListPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'template-list-page' }, 'Templates'),
  };
});

jest.mock('@/pages/ReportsPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'reports-page' }, 'Reports'),
  };
});

jest.mock('@/pages/SettingsPage', () => {
  const React = require('react');
  return {
    __esModule: true,
    default: () =>
      React.createElement('div', { 'data-testid': 'settings-page' }, 'Settings'),
  };
});

// ---------------------------------------------------------------------------
// Mock – Layout component
// ---------------------------------------------------------------------------
// Simplified Layout that renders navigation links + <Outlet /> so we can
// test route navigation without the full MUI sidebar implementation.
// ---------------------------------------------------------------------------

jest.mock('@/components/Layout', () => {
  const React = require('react');
  const { Outlet, Link } = require('react-router-dom');
  return {
    __esModule: true,
    default: () =>
      React.createElement(
        'div',
        { 'data-testid': 'layout' },
        React.createElement(
          'nav',
          null,
          React.createElement(
            Link,
            { to: '/', 'data-testid': 'nav-dashboard' },
            'Dashboard',
          ),
          React.createElement(
            Link,
            { to: '/experiments', 'data-testid': 'nav-experiments' },
            'Experiments',
          ),
          React.createElement(
            Link,
            { to: '/experiments/new', 'data-testid': 'nav-new-experiment' },
            'New Experiment',
          ),
          React.createElement(
            Link,
            { to: '/clusters', 'data-testid': 'nav-clusters' },
            'Clusters',
          ),
          React.createElement(
            Link,
            { to: '/templates', 'data-testid': 'nav-templates' },
            'Templates',
          ),
          React.createElement(
            Link,
            { to: '/reports', 'data-testid': 'nav-reports' },
            'Reports',
          ),
          React.createElement(
            Link,
            { to: '/settings', 'data-testid': 'nav-settings' },
            'Settings',
          ),
        ),
        React.createElement(Outlet, null),
      ),
  };
});

// ---------------------------------------------------------------------------
// Mock – ToastProvider (keep it lightweight)
// ---------------------------------------------------------------------------

jest.mock('@/services/toast', () => {
  const React = require('react');
  return {
    ToastProvider: (props: any) => props.children,
    useToast: () => ({
      toasts: [],
      showToast: jest.fn(() => 'mock-toast-id'),
      dismissToast: jest.fn(),
      dismissAll: jest.fn(),
      success: jest.fn(() => 'mock-toast-id'),
      error: jest.fn(() => 'mock-toast-id'),
      warning: jest.fn(() => 'mock-toast-id'),
      info: jest.fn(() => 'mock-toast-id'),
    }),
    showToast: jest.fn(() => 'mock-toast-id'),
    __registerToastHandler: jest.fn(),
  };
});

// ---------------------------------------------------------------------------
// Pre-built auth states for test store
// ---------------------------------------------------------------------------

const unauthenticatedAuthState = {
  user: null,
  accessToken: null,
  refreshToken: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

const authenticatedAuthState = {
  user: {
    id: 'user-1',
    email: 'admin@chaos-sec.io',
    name: 'Admin User',
    role: 'admin' as const,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
    lastLoginAt: '2024-06-01T12:00:00Z',
  },
  accessToken: 'test-access-token',
  refreshToken: 'test-refresh-token',
  isAuthenticated: true,
  isLoading: false,
  error: null,
};

const initialExperimentState = {
  list: {
    experiments: [],
    totalCount: 0,
    currentPage: 1,
    pageSize: 10,
    isLoading: false,
    error: null as string | null,
    filters: {
      search: '',
      status: 'all' as const,
      templateId: null as string | null,
      clusterId: null as string | null,
      dateFrom: null as string | null,
      dateTo: null as string | null,
    },
    sortBy: 'createdAt',
    sortOrder: 'desc' as const,
  },
  detail: {
    experiment: null,
    currentRun: null,
    logs: [] as string[],
    isLoading: false,
    error: null as string | null,
  },
  createStatus: 'idle' as const,
  createError: null as string | null,
  executeStatus: 'idle' as const,
  executeError: null as string | null,
  stopStatus: 'idle' as const,
  stopError: null as string | null,
  deleteStatus: 'idle' as const,
  deleteError: null as string | null,
  runs: [],
  runsTotalCount: 0,
  runsPage: 1,
  runsLoading: false,
  runsError: null as string | null,
};

// ---------------------------------------------------------------------------
// Test store factory
// ---------------------------------------------------------------------------

function createTestStore(authState: AuthState = unauthenticatedAuthState) {
  return configureStore({
    reducer: {
      auth: authReducer,
      experiments: experimentReducer,
    },
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware({
        serializableCheck: false,
        immutableCheck: false,
      }),
    preloadedState: {
      auth: authState,
      experiments: initialExperimentState,
    },
  });
}

// ---------------------------------------------------------------------------
// Render helper
// ---------------------------------------------------------------------------

function renderApp(
  initialPath: string = '/',
  authState: AuthState = unauthenticatedAuthState,
) {
  const store = createTestStore(authState);
  const user = userEvent.setup();

  const result = render(
    <Provider store={store}>
      <ThemeProvider theme={lightTheme}>
        <MemoryRouter initialEntries={[initialPath]}>
          <App />
        </MemoryRouter>
      </ThemeProvider>
    </Provider>,
  );

  return { ...result, store, user };
}

// ===========================================================================
// Tests
// ===========================================================================

describe('App Integration – Routing & Authentication', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedGetAccessToken.mockReturnValue(null);
    mockedGetRefreshToken.mockReturnValue(null);
  });

  // -------------------------------------------------------------------------
  // 1. App renders the login page when not authenticated
  // -------------------------------------------------------------------------
  describe('Unauthenticated – login page renders', () => {
    it('renders the login page at /login', async () => {
      renderApp('/login', unauthenticatedAuthState);

      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
      expect(loginPage).toHaveTextContent('Login Page');
    });

    it('renders the register page at /register', async () => {
      renderApp('/register', unauthenticatedAuthState);

      const registerPage = await screen.findByTestId('register-page');
      expect(registerPage).toBeInTheDocument();
    });

    it('does not show protected content on public routes', async () => {
      renderApp('/login', unauthenticatedAuthState);

      await screen.findByTestId('login-page');

      // The Layout (sidebar + outlet) should not appear for unauthenticated users
      expect(screen.queryByTestId('layout')).not.toBeInTheDocument();
      expect(screen.queryByTestId('dashboard-page')).not.toBeInTheDocument();
      expect(screen.queryByTestId('experiment-list-page')).not.toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 2. Protected routes redirect to login when unauthenticated
  // -------------------------------------------------------------------------
  describe('Protected route redirects', () => {
    it('redirects from / to /login when not authenticated', async () => {
      renderApp('/', unauthenticatedAuthState);

      // ProtectedRoute should redirect because isAuthenticated is false
      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
      expect(screen.queryByTestId('dashboard-page')).not.toBeInTheDocument();
    });

    it('redirects from /experiments to /login when not authenticated', async () => {
      renderApp('/experiments', unauthenticatedAuthState);

      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
      expect(screen.queryByTestId('experiment-list-page')).not.toBeInTheDocument();
    });

    it('redirects from /clusters to /login when not authenticated', async () => {
      renderApp('/clusters', unauthenticatedAuthState);

      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
    });

    it('redirects from /settings to /login when not authenticated', async () => {
      renderApp('/settings', unauthenticatedAuthState);

      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
    });

    it('redirects from /experiments/new to /login when not authenticated', async () => {
      renderApp('/experiments/new', unauthenticatedAuthState);

      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
      expect(screen.queryByTestId('create-experiment-page')).not.toBeInTheDocument();
    });

    it('redirects from /experiments/:id to /login when not authenticated', async () => {
      renderApp('/experiments/exp-123', unauthenticatedAuthState);

      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
    });

    it('shows login page instead of any protected page for every guarded route', async () => {
      const protectedPaths = [
        '/reports',
        '/templates',
        '/experiments',
        '/clusters',
        '/settings',
      ];

      for (const path of protectedPaths) {
        const { unmount } = renderApp(path, unauthenticatedAuthState);

        const loginPage = await screen.findByTestId('login-page');
        expect(loginPage).toBeInTheDocument();

        unmount();
      }
    });
  });

  // -------------------------------------------------------------------------
  // 3. Navigation between pages when authenticated
  // -------------------------------------------------------------------------
  describe('Authenticated – navigation between pages', () => {
    beforeEach(() => {
      mockedGetAccessToken.mockReturnValue('test-access-token');
      mockedGetRefreshToken.mockReturnValue('test-refresh-token');
    });

    it('renders the dashboard at the index route /', async () => {
      renderApp('/', authenticatedAuthState);

      const layout = await screen.findByTestId('layout');
      expect(layout).toBeInTheDocument();

      const dashboard = await screen.findByTestId('dashboard-page');
      expect(dashboard).toBeInTheDocument();
    });

    it('renders the experiment list at /experiments', async () => {
      renderApp('/experiments', authenticatedAuthState);

      const experimentList = await screen.findByTestId('experiment-list-page');
      expect(experimentList).toBeInTheDocument();
    });

    it('renders the create experiment page at /experiments/new', async () => {
      renderApp('/experiments/new', authenticatedAuthState);

      const createPage = await screen.findByTestId('create-experiment-page');
      expect(createPage).toBeInTheDocument();
    });

    it('renders experiment detail at /experiments/:id', async () => {
      renderApp('/experiments/exp-42', authenticatedAuthState);

      const detailPage = await screen.findByTestId('experiment-detail-page');
      expect(detailPage).toBeInTheDocument();
    });

    it('renders cluster list at /clusters', async () => {
      renderApp('/clusters', authenticatedAuthState);

      const clusterList = await screen.findByTestId('cluster-list-page');
      expect(clusterList).toBeInTheDocument();
    });

    it('renders templates at /templates', async () => {
      renderApp('/templates', authenticatedAuthState);

      const templateList = await screen.findByTestId('template-list-page');
      expect(templateList).toBeInTheDocument();
    });

    it('renders reports at /reports', async () => {
      renderApp('/reports', authenticatedAuthState);

      const reportsPage = await screen.findByTestId('reports-page');
      expect(reportsPage).toBeInTheDocument();
    });

    it('renders settings at /settings', async () => {
      renderApp('/settings', authenticatedAuthState);

      const settingsPage = await screen.findByTestId('settings-page');
      expect(settingsPage).toBeInTheDocument();
    });

    it('navigates from dashboard to experiments via nav link', async () => {
      const { user } = renderApp('/', authenticatedAuthState);

      // Wait for initial page to settle
      await screen.findByTestId('dashboard-page');

      // Click the Experiments navigation link
      const experimentsLink = screen.getByTestId('nav-experiments');
      await user.click(experimentsLink);

      // Experiments list page should now be rendered
      const experimentList = await screen.findByTestId('experiment-list-page');
      expect(experimentList).toBeInTheDocument();
      // Dashboard should no longer be the active page
      expect(screen.queryByTestId('dashboard-page')).not.toBeInTheDocument();
    });

    it('navigates from experiments to create-experiment via nav link', async () => {
      const { user } = renderApp('/experiments', authenticatedAuthState);

      await screen.findByTestId('experiment-list-page');

      const newExperimentLink = screen.getByTestId('nav-new-experiment');
      await user.click(newExperimentLink);

      const createPage = await screen.findByTestId('create-experiment-page');
      expect(createPage).toBeInTheDocument();
    });

    it('navigates through multiple pages using nav links', async () => {
      const { user } = renderApp('/', authenticatedAuthState);

      // Start at dashboard
      await screen.findByTestId('dashboard-page');

      // Navigate to Clusters
      await user.click(screen.getByTestId('nav-clusters'));
      await screen.findByTestId('cluster-list-page');
      expect(screen.queryByTestId('dashboard-page')).not.toBeInTheDocument();

      // Navigate to Settings
      await user.click(screen.getByTestId('nav-settings'));
      await screen.findByTestId('settings-page');
      expect(screen.queryByTestId('cluster-list-page')).not.toBeInTheDocument();

      // Navigate back to Dashboard
      await user.click(screen.getByTestId('nav-dashboard'));
      await screen.findByTestId('dashboard-page');
      expect(screen.queryByTestId('settings-page')).not.toBeInTheDocument();
    });

    it('navigates to reports and templates sequentially', async () => {
      const { user } = renderApp('/', authenticatedAuthState);

      await screen.findByTestId('dashboard-page');

      // Reports
      await user.click(screen.getByTestId('nav-reports'));
      await screen.findByTestId('reports-page');

      // Templates
      await user.click(screen.getByTestId('nav-templates'));
      await screen.findByTestId('template-list-page');
    });
  });

  // -------------------------------------------------------------------------
  // 4. Authenticated users can access the dashboard
  // -------------------------------------------------------------------------
  describe('Authenticated – dashboard access', () => {
    beforeEach(() => {
      mockedGetAccessToken.mockReturnValue('test-access-token');
    });

    it('renders the layout wrapper for authenticated users', async () => {
      renderApp('/', authenticatedAuthState);

      const layout = await screen.findByTestId('layout');
      expect(layout).toBeInTheDocument();
    });

    it('does not redirect authenticated users to login', async () => {
      renderApp('/', authenticatedAuthState);

      const dashboard = await screen.findByTestId('dashboard-page');
      expect(dashboard).toBeInTheDocument();

      // The login page should NOT be rendered
      expect(screen.queryByTestId('login-page')).not.toBeInTheDocument();
    });

    it('renders the dashboard as the index route content', async () => {
      renderApp('/', authenticatedAuthState);

      const dashboard = await screen.findByTestId('dashboard-page');
      expect(dashboard).toBeInTheDocument();
      expect(dashboard).toHaveTextContent('Dashboard Page');
    });

    it('shows all navigation links for an authenticated user', async () => {
      renderApp('/', authenticatedAuthState);

      await screen.findByTestId('dashboard-page');

      expect(screen.getByTestId('nav-dashboard')).toBeInTheDocument();
      expect(screen.getByTestId('nav-experiments')).toBeInTheDocument();
      expect(screen.getByTestId('nav-new-experiment')).toBeInTheDocument();
      expect(screen.getByTestId('nav-clusters')).toBeInTheDocument();
      expect(screen.getByTestId('nav-templates')).toBeInTheDocument();
      expect(screen.getByTestId('nav-reports')).toBeInTheDocument();
      expect(screen.getByTestId('nav-settings')).toBeInTheDocument();
    });

    it('persists layout across nested route navigation', async () => {
      const { user } = renderApp('/', authenticatedAuthState);

      await screen.findByTestId('dashboard-page');
      expect(screen.getByTestId('layout')).toBeInTheDocument();

      // Navigate to a nested route
      await user.click(screen.getByTestId('nav-experiments'));
      await screen.findByTestId('experiment-list-page');

      // Layout wrapper should still be present
      expect(screen.getByTestId('layout')).toBeInTheDocument();
    });
  });

  // -------------------------------------------------------------------------
  // 5. Catch-all and unknown routes
  // -------------------------------------------------------------------------
  describe('Catch-all / unknown route handling', () => {
    it('redirects unknown routes to dashboard when authenticated', async () => {
      mockedGetAccessToken.mockReturnValue('test-access-token');
      renderApp('/this-route-does-not-exist', authenticatedAuthState);

      // The catch-all route (<Navigate to="/" replace />) redirects to /
      // which renders the dashboard
      const dashboard = await screen.findByTestId('dashboard-page');
      expect(dashboard).toBeInTheDocument();
    });

    it('redirects unknown routes to login when not authenticated', async () => {
      renderApp('/this-route-does-not-exist', unauthenticatedAuthState);

      // Catch-all → / → ProtectedRoute → /login
      const loginPage = await screen.findByTestId('login-page');
      expect(loginPage).toBeInTheDocument();
    });

    it('handles deeply nested unknown paths when authenticated', async () => {
      mockedGetAccessToken.mockReturnValue('test-access-token');
      renderApp('/a/b/c/d/e', authenticatedAuthState);

      const dashboard = await screen.findByTestId('dashboard-page');
      expect(dashboard).toBeInTheDocument();
    });
  });
});
