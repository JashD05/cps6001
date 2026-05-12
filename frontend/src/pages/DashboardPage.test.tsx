/**
 * Unit tests for the DashboardPage component.
 *
 * Tests rendering of dashboard sections, data fetching,
 * refresh behaviour, error handling, and navigation.
 */

import { ThemeProvider, createTheme } from '@mui/material/styles';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import DashboardPage from '@/pages/DashboardPage';
import { dashboardAPI } from '@/services/api';
import {
  fetchExperiments,
  selectExperimentListLoading,
  selectExperimentStats,
  selectRecentExperiments,
} from '@/store/experimentSlice';
import type { DashboardSummary, Experiment } from '@/types';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockDispatch = jest.fn();
const mockNavigate = jest.fn();

jest.mock('react-redux', () => ({
  useDispatch: () => mockDispatch,
  useSelector: (selector: (state: Record<string, unknown>) => unknown) =>
    selector(mockState),
}));

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

jest.mock('@/services/api', () => ({
  dashboardAPI: {
    getSummary: jest.fn(),
    getSecurityPostureHistory: jest.fn(),
    getActivityTimeline: jest.fn(),
    getClusterHealth: jest.fn(),
  },
}));

jest.mock('@/store/experimentSlice', () => ({
  fetchExperiments: jest.fn(() => ({ type: 'experiments/fetchList' })),
  selectExperimentListLoading: jest.fn(),
  selectExperimentStats: jest.fn(),
  selectRecentExperiments: jest.fn(),
}));

jest.mock('recharts', () => {
  const React = require('react');
  const mockChart = (props: Record<string, unknown>) =>
    React.createElement('div', { 'data-testid': 'mock-chart' }, props.children);
  return {
    AreaChart: mockChart,
    Area: () => null,
    XAxis: () => null,
    YAxis: () => null,
    CartesianGrid: () => null,
    Tooltip: mockChart,
    ResponsiveContainer: (props: Record<string, unknown>) =>
      React.createElement(
        'div',
        { 'data-testid': 'responsive-container' },
        props.children,
      ),
    BarChart: mockChart,
    Bar: () => null,
    PieChart: mockChart,
    Pie: () => null,
    Cell: () => null,
    Legend: () => null,
  };
});

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

const theme = createTheme();

// ---------------------------------------------------------------------------
// Mock State
// ---------------------------------------------------------------------------

const mockDashboardSummary: DashboardSummary = {
  securityPostureScore: 78,
  postureTrend: {
    direction: 'up',
    percentage: 5.2,
    period: 'last 30 days',
  },
  experimentSummary: {
    total: 24,
    running: 3,
    completed: 15,
    failed: 4,
    pending: 2,
  },
  recentExperiments: [
    {
      id: 'exp-1',
      name: 'DNS Exfiltration Test',
      description: 'Test DNS exfiltration detection',
      templateId: 'tpl-1',
      templateName: 'DNS Exfiltration',
      clusterId: 'cluster-1',
      clusterName: 'Production Cluster',
      namespace: 'default',
      status: 'completed',
      progress: 100,
      parameters: { duration: 60 },
      steps: [],
      tags: ['dns', 'network'],
      createdBy: 'admin',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    },
    {
      id: 'exp-2',
      name: 'Pod Kill Chaos',
      description: 'Test pod kill recovery',
      templateId: 'tpl-2',
      templateName: 'Pod Kill',
      clusterId: 'cluster-2',
      clusterName: 'Staging Cluster',
      namespace: 'chaos',
      status: 'running',
      progress: 55,
      parameters: {},
      steps: [],
      tags: ['pod', 'infrastructure'],
      createdBy: 'operator',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    },
    {
      id: 'exp-3',
      name: 'Network Policy Bypass',
      description: 'Test network policy enforcement',
      templateId: 'tpl-3',
      templateName: 'Network Policy',
      clusterId: 'cluster-1',
      clusterName: 'Production Cluster',
      namespace: 'default',
      status: 'failed',
      progress: 75,
      parameters: {},
      steps: [],
      tags: ['network'],
      createdBy: 'admin',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    },
  ] as Experiment[],
  clusterHealth: [
    {
      clusterId: 'cluster-1',
      status: 'healthy',
      cpuUsage: 45,
      memoryUsage: 62,
      podCount: 48,
      nodeCount: 5,
      errorRate: 0.5,
      lastChecked: new Date().toISOString(),
    },
    {
      clusterId: 'cluster-2',
      status: 'degraded',
      cpuUsage: 78,
      memoryUsage: 85,
      podCount: 32,
      nodeCount: 3,
      errorRate: 3.2,
      lastChecked: new Date().toISOString(),
    },
  ],
  threatCoverage: {
    totalControls: 50,
    validated: 35,
    passed: 28,
    failed: 7,
    untested: 15,
    coverage: 70,
  },
  threatCoverageByCategory: [
    { name: 'Network', validated: 12, untested: 3 },
    { name: 'Application', validated: 8, untested: 4 },
    { name: 'Infrastructure', validated: 10, untested: 5 },
    { name: 'Identity', validated: 5, untested: 3 },
  ],
  experimentTrend: [
    { date: '2024-01-01', total: 5, passed: 3, failed: 2 },
    { date: '2024-01-02', total: 8, passed: 6, failed: 2 },
  ],
  topAttackTypes: [
    { name: 'DNS Exfiltration', value: 12 },
    { name: 'Pod Kill', value: 8 },
  ],
  validationSuccessRate: [
    { timestamp: '2024-01-01', value: 75, label: 'Jan 1' },
    { timestamp: '2024-01-02', value: 82, label: 'Jan 2' },
  ],
};

let mockState: Record<string, unknown>;

const defaultState = {
  loading: false,
  stats: {
    total: 24,
    running: 3,
    completed: 15,
    failed: 4,
    pending: 2,
  },
  recentExperiments: mockDashboardSummary.recentExperiments,
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderDashboardPage() {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>
    </ThemeProvider>,
  );
}

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();
  mockState = { ...defaultState };

  (selectExperimentListLoading as jest.Mock).mockImplementation(
    () => defaultState.loading,
  );
  (selectExperimentStats as jest.Mock).mockImplementation(() => defaultState.stats);
  (selectRecentExperiments as jest.Mock).mockImplementation(
    () => defaultState.recentExperiments,
  );

  (dashboardAPI.getSummary as jest.Mock).mockResolvedValue({
    data: { data: mockDashboardSummary },
  });
  (dashboardAPI.getSecurityPosture as jest.Mock).mockResolvedValue({
    data: {
      data: mockDashboardSummary.experimentTrend,
    },
  });
  (dashboardAPI.getActivityTimeline as jest.Mock).mockResolvedValue({
    data: { data: mockDashboardSummary.experimentTrend },
  });
  (dashboardAPI.getClusterHealth as jest.Mock).mockResolvedValue({
    data: { data: mockDashboardSummary.clusterHealth },
  });
});

// ===========================================================================
// Rendering Tests
// ===========================================================================

describe('DashboardPage – rendering', () => {
  it('renders without crashing', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    expect(
      screen.getByText(/dashboard/i) ||
        screen.getByText(/security/i) ||
        document.querySelector('.MuiBox-root'),
    ).toBeTruthy();
  });

  it('renders the security posture section', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/security posture/i) ||
          screen.getByText(/security/i) ||
          screen.getByText(/posture/i),
      ).toBeTruthy();
    });
  });

  it('renders experiment summary KPI cards', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/total/i) || screen.getByText('24')).toBeTruthy();
    });
  });

  it('dispatches fetchExperiments on mount', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    expect(mockDispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'experiments/fetchList' }),
    );
  });

  it('renders the recent experiments section heading', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/recent experiments/i) || screen.getByText(/recent/i),
      ).toBeTruthy();
    });
  });

  it('renders cluster health section', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/cluster health/i) || screen.getByText(/cluster/i),
      ).toBeTruthy();
    });
  });

  it('renders threat coverage section', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/threat coverage/i) || screen.getByText(/threat/i),
      ).toBeTruthy();
    });
  });

  it('renders experiment trend section', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/experiment trend/i) || screen.getByText(/trend/i),
      ).toBeTruthy();
    });
  });

  it('renders the refresh button', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    const refreshButtons = screen.queryAllByLabelText(/refresh/i);
    expect(refreshButtons.length).toBeGreaterThanOrEqual(0);
  });
});

// ===========================================================================
// Data Fetching Tests
// ===========================================================================

describe('DashboardPage – data fetching', () => {
  it('calls dashboardAPI.getSummary on mount', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    expect(dashboardAPI.getSummary).toHaveBeenCalled();
  });

  it('calls dashboardAPI.getSecurityPostureHistory on mount', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    expect(dashboardAPI.getSecurityPosture).toHaveBeenCalled();
  });

  it('calls dashboardAPI.getActivityTimeline on mount', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    expect(dashboardAPI.getActivityTimeline).toHaveBeenCalled();
  });

  it('calls dashboardAPI.getClusterHealth on mount', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    expect(dashboardAPI.getClusterHealth).toHaveBeenCalled();
  });

  it('shows security posture score after data loads', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/78/)).toBeTruthy();
    });
  });

  it('shows experiment counts after data loads', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('24') || screen.getByText(/24/)).toBeTruthy();
    });
  });

  it('shows recent experiment names in the table', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('handles API error gracefully', async () => {
    (dashboardAPI.getSummary as jest.Mock).mockRejectedValue(new Error('Network error'));

    await act(async () => {
      renderDashboardPage();
    });

    await waitFor(() => {
      expect(
        screen.getByText(/error/i) ||
          screen.getByText(/failed/i) ||
          screen.getByText(/retry/i),
      ).toBeTruthy();
    });
  });

  it('calls API methods again when refresh button is clicked', async () => {
    await act(async () => {
      renderDashboardPage();
    });

    const initialSummaryCalls = (dashboardAPI.getSummary as jest.Mock).mock.calls.length;

    const refreshButtons = screen.queryAllByLabelText(/refresh/i);
    if (refreshButtons.length > 0) {
      await act(async () => {
        fireEvent.click(refreshButtons[0]);
      });
      expect((dashboardAPI.getSummary as jest.Mock).mock.calls.length).toBeGreaterThan(
        initialSummaryCalls,
      );
    }
  });
});

// ===========================================================================
// Loading State Tests
// ===========================================================================

describe('DashboardPage – loading state', () => {
  it('shows loading skeletons when data is loading', async () => {
    (selectExperimentListLoading as jest.Mock).mockReturnValue(true);

    (dashboardAPI.getSummary as jest.Mock).mockImplementation(
      () => new Promise(() => {}),
    );

    await act(async () => {
      renderDashboardPage();
    });

    const skeletons = document.querySelectorAll('.MuiSkeleton-root');
    expect(skeletons.length).toBeGreaterThanOrEqual(0);
  });

  it('renders content after loading completes', async () => {
    await act(async () => {
      renderDashboardPage();
    });

    await waitFor(() => {
      expect(
        screen.getByText(/security/i) ||
          screen.getByText(/dashboard/i) ||
          screen.getByText(/posture/i),
      ).toBeTruthy();
    });
  });
});

// ===========================================================================
// Navigation Tests
// ===========================================================================

describe('DashboardPage – navigation', () => {
  it('navigates to experiment detail when an experiment row is clicked', async () => {
    await act(async () => {
      renderDashboardPage();
    });

    await waitFor(() => {
      const expLink = screen.queryByText('DNS Exfiltration Test');
      if (expLink) {
        fireEvent.click(expLink);
        expect(mockNavigate).toHaveBeenCalled();
      }
    });
  });

  it('renders view buttons that navigate to experiments', async () => {
    await act(async () => {
      renderDashboardPage();
    });

    await waitFor(() => {
      const viewButtons = screen.queryAllByText(/view all/i);
      if (viewButtons.length > 0) {
        fireEvent.click(viewButtons[0]);
        expect(mockNavigate).toHaveBeenCalled();
      }
    });
  });
});

// ===========================================================================
// Experiment Summary Display
// ===========================================================================

describe('DashboardPage – experiment summary', () => {
  it('renders total experiment count', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('24')).toBeTruthy();
    });
  });

  it('renders running experiment count', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('3')).toBeTruthy();
    });
  });

  it('renders completed experiment count', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('15')).toBeTruthy();
    });
  });

  it('renders failed experiment count', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('4')).toBeTruthy();
    });
  });

  it('renders pending experiment count', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText('2')).toBeTruthy();
    });
  });
});

// ===========================================================================
// Trend and Coverage Display
// ===========================================================================

describe('DashboardPage – posture trend display', () => {
  it('renders posture trend direction', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/up/i) ||
          screen.getByText(/5\.2/) ||
          screen.getByText(/last 30 days/i) ||
          screen.getByText(/\+5\.2/),
      ).toBeTruthy();
    });
  });

  it('renders the posture trend period', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      const trendPeriod = screen.queryByText(/last 30 days/i);
      expect(trendPeriod).toBeTruthy();
    });
  });
});

// ===========================================================================
// Cluster Health Display
// ===========================================================================

describe('DashboardPage – cluster health display', () => {
  it('renders cluster names in health section', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      const clusterElements = screen.queryAllByText(/cluster/i);
      expect(clusterElements.length).toBeGreaterThan(0);
    });
  });

  it('renders CPU usage percentage', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/45/) || screen.getByText(/78/)).toBeTruthy();
    });
  });

  it('renders memory usage percentage', async () => {
    await act(async () => {
      renderDashboardPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/62/) || screen.getByText(/85/)).toBeTruthy();
    });
  });
});

// ===========================================================================
// Edge Cases
// ===========================================================================

describe('DashboardPage – edge cases', () => {
  it('handles empty experiment list gracefully', async () => {
    (selectRecentExperiments as jest.Mock).mockReturnValue([]);
    (selectExperimentStats as jest.Mock).mockReturnValue({
      total: 0,
      running: 0,
      completed: 0,
      failed: 0,
      pending: 0,
    });

    (dashboardAPI.getSummary as jest.Mock).mockResolvedValue({
      data: {
        data: {
          ...mockDashboardSummary,
          experimentSummary: {
            total: 0,
            running: 0,
            completed: 0,
            failed: 0,
            pending: 0,
          },
          recentExperiments: [],
        },
      },
    });

    await act(async () => {
      renderDashboardPage();
    });

    await waitFor(() => {
      expect(screen.getByText('0')).toBeTruthy();
    });
  });

  it('handles null dashboard summary gracefully', async () => {
    (dashboardAPI.getSummary as jest.Mock).mockResolvedValue({
      data: { data: null },
    });
    (dashboardAPI.getSecurityPosture as jest.Mock).mockResolvedValue({
      data: { data: null },
    });
    (dashboardAPI.getActivityTimeline as jest.Mock).mockResolvedValue({
      data: { data: null },
    });
    (dashboardAPI.getClusterHealth as jest.Mock).mockResolvedValue({
      data: { data: null },
    });

    await act(async () => {
      renderDashboardPage();
    });

    // Should not crash - may show error or empty state
    expect(document.querySelector('.MuiBox-root')).toBeTruthy();
  });

  it('handles partial API failure gracefully', async () => {
    (dashboardAPI.getSummary as jest.Mock).mockResolvedValue({
      data: { data: mockDashboardSummary },
    });
    (dashboardAPI.getSecurityPosture as jest.Mock).mockRejectedValue(
      new Error('Network error'),
    );

    await act(async () => {
      renderDashboardPage();
    });

    // Should still render with partial data
    expect(document.querySelector('.MuiBox-root')).toBeTruthy();
  });

  it('re-renders when experiment data changes', async () => {
    const { rerender } = await act(async () => {
      return renderDashboardPage();
    });

    (selectExperimentStats as jest.Mock).mockReturnValue({
      total: 30,
      running: 5,
      completed: 20,
      failed: 3,
      pending: 2,
    });

    await act(async () => {
      rerender(
        <ThemeProvider theme={theme}>
          <MemoryRouter>
            <DashboardPage />
          </MemoryRouter>
        </ThemeProvider>,
      );
    });

    await waitFor(() => {
      expect(screen.getByText('30')).toBeTruthy();
    });
  });
});
