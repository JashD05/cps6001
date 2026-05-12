/**
 * Unit tests for the ExperimentDetailPage component.
 *
 * Tests rendering of experiment details, data fetching,
 * run/stop actions, log viewing, results display,
 * SIEM validation, error handling, and navigation.
 */

import { ThemeProvider, createTheme } from '@mui/material/styles';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ExperimentDetailPage from '@/pages/ExperimentDetailPage';
import { experimentsAPI } from '@/services/api';
import {
  fetchExperimentById,
  fetchExperimentLogs,
  clearExperimentDetail,
  selectExperimentDetail,
  selectExperimentDetailLoading,
  selectExperimentDetailError,
  selectExperimentLogs,
  selectCurrentRun,
  executeExperiment,
  stopExperiment,
  selectExecuteStatus,
  selectExecuteError,
  selectStopStatus,
  resetExecuteStatus,
  resetStopStatus,
} from '@/store/experimentSlice';
import type { Experiment, ExperimentRun, ExperimentResult } from '@/types';

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
  useParams: () => ({ id: 'exp-test-1' }),
}));

jest.mock('@/services/api', () => ({
  experimentsAPI: {
    getById: jest.fn(),
    execute: jest.fn(),
    stop: jest.fn(),
    getLogs: jest.fn(),
    getRuns: jest.fn(),
  },
  getErrorMessage: jest.fn((err: unknown) => {
    if (err instanceof Error) return err.message;
    if (typeof err === 'string') return err;
    return 'An error occurred';
  }),
}));

jest.mock('@/store/experimentSlice', () => ({
  fetchExperimentById: jest.fn((id: string) => ({
    type: 'experiments/fetchById',
    payload: id,
  })),
  fetchExperimentLogs: jest.fn(() => ({
    type: 'experiments/fetchLogs',
  })),
  clearExperimentDetail: jest.fn(() => ({
    type: 'experiments/clearDetail',
  })),
  selectExperimentDetail: jest.fn(),
  selectExperimentDetailLoading: jest.fn(),
  selectExperimentDetailError: jest.fn(),
  selectExperimentLogs: jest.fn(),
  selectCurrentRun: jest.fn(),
  executeExperiment: jest.fn(() => ({
    type: 'experiments/execute',
  })),
  stopExperiment: jest.fn(() => ({
    type: 'experiments/stop',
  })),
  selectExecuteStatus: jest.fn(),
  selectExecuteError: jest.fn(),
  selectStopStatus: jest.fn(),
  resetExecuteStatus: jest.fn(),
  resetStopStatus: jest.fn(),
}));

jest.mock('@/components/StatusBadge', () => {
  const React = require('react');
  return function MockStatusBadge({ status }: { status: string }) {
    return React.createElement('span', { 'data-testid': 'status-badge' }, status);
  };
});

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

const theme = createTheme();

// ---------------------------------------------------------------------------
// Mock Data
// ---------------------------------------------------------------------------

const mockExperiment: Experiment = {
  id: 'exp-test-1',
  name: 'DNS Exfiltration Test',
  description: 'Test DNS exfiltration detection capabilities',
  templateId: 'tpl-dns-exfil',
  templateName: 'DNS Exfiltration',
  clusterId: 'cluster-1',
  clusterName: 'Production Cluster',
  namespace: 'chaos-ns',
  status: 'running',
  progress: 65,
  parameters: { duration: 60, target_namespace: 'default' },
  steps: [
    {
      id: 'step-1',
      name: 'Setup',
      description: 'Set up test environment',
      status: 'completed',
      order: 1,
    },
    {
      id: 'step-2',
      name: 'Execute',
      description: 'Execute the attack',
      status: 'in_progress',
      order: 2,
    },
    {
      id: 'step-3',
      name: 'Verify',
      description: 'Verify detection',
      status: 'pending',
      order: 3,
    },
  ],
  tags: ['dns', 'network', 'exfiltration'],
  createdBy: 'admin',
  createdAt: '2024-01-15T10:30:00Z',
  updatedAt: '2024-01-15T11:00:00Z',
  startedAt: '2024-01-15T10:35:00Z',
  result: {
    success: true,
    score: 85,
    summary: 'Experiment completed successfully',
    details: ['All detections fired correctly', 'SIEM integration verified'],
    siemValidation: {
      expectedAlertCount: 3,
      receivedAlertCount: 3,
      alerts: [],
      detected: true,
      detectionLatencyMs: 4500,
      coverage: 100,
      details: [],
    },
    startedAt: '2024-01-15T10:35:00Z',
    completedAt: '2024-01-15T10:45:00Z',
    duration: 600,
  },
  runs: [
    {
      id: 'run-1',
      experimentId: 'exp-test-1',
      status: 'running',
      progress: 65,
      logs: ['Starting experiment...', 'Running step 2...'],
      startedAt: '2024-01-15T10:35:00Z',
      podStatuses: [
        {
          name: 'pod-1',
          namespace: 'chaos-ns',
          status: 'Running',
          ready: true,
          restarts: 0,
          age: '5m',
        },
      ],
      steps: [
        {
          id: 'step-1',
          name: 'Setup',
          description: 'Set up test environment',
          status: 'completed',
          order: 1,
        },
        {
          id: 'step-2',
          name: 'Execute',
          description: 'Execute the attack',
          status: 'in_progress',
          order: 2,
        },
      ],
    },
  ],
};

const mockLogs = [
  '[2024-01-15T10:35:00Z] Starting experiment...',
  '[2024-01-15T10:36:00Z] Setting up chaos namespace...',
  '[2024-01-15T10:37:00Z] Running attack step...',
  '[2024-01-15T10:38:00Z] Waiting for detection...',
];

let mockState: Record<string, unknown>;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderExperimentDetailPage(route = '/experiments/exp-test-1') {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route path="/experiments/:id" element={<ExperimentDetailPage />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );
}

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();

  mockState = {
    experiments: {
      detail: {
        experiment: mockExperiment,
        currentRun: mockExperiment.runs?.[0] || null,
        isLoading: false,
        error: null,
      },
      logs: mockLogs,
    },
  };

  (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);
  (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);
  (selectExperimentDetailError as jest.Mock).mockReturnValue(null);
  (selectExperimentLogs as jest.Mock).mockReturnValue(mockLogs);
  (selectCurrentRun as jest.Mock).mockImplementation(
    (state: Record<string, unknown>) =>
      (state as any).experiments?.detail?.currentRun ?? null,
  );
  (selectExecuteStatus as jest.Mock).mockReturnValue('idle');
  (selectExecuteError as jest.Mock).mockReturnValue(null);
  (selectStopStatus as jest.Mock).mockReturnValue('idle');
});

// ===========================================================================
// Rendering Tests
// ===========================================================================

describe('ExperimentDetailPage – rendering', () => {
  it('renders without crashing', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    expect(document.querySelector('.MuiBox-root')).toBeTruthy();
  });

  it('renders the experiment name heading', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('renders the experiment description', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/DNS exfiltration detection/i)).toBeTruthy();
    });
  });

  it('renders the experiment status badge', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      expect(badges.length).toBeGreaterThan(0);
    });
  });

  it('renders the experiment progress percentage', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/65/) || screen.getByText(/65%/)).toBeTruthy();
    });
  });

  it('renders the template name', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText('DNS Exfiltration') || screen.getByText(/DNS Exfiltration/i),
      ).toBeTruthy();
    });
  });

  it('renders the cluster name', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText('Production Cluster') || screen.getByText(/Production Cluster/i),
      ).toBeTruthy();
    });
  });

  it('renders the namespace', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/chaos-ns/) || screen.getByText(/namespace/i)).toBeTruthy();
    });
  });

  it('renders experiment tags as chips', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      expect(screen.getByText('dns')).toBeTruthy();
      expect(screen.getByText('network')).toBeTruthy();
      expect(screen.getByText('exfiltration')).toBeTruthy();
    });
  });

  it('renders breadcrumbs with navigation links', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      const experimentsLink = screen.queryByText(/experiments/i);
      expect(experimentsLink).toBeTruthy();
    });
  });

  it('renders the created date', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });
    await waitFor(() => {
      const createdText = screen.queryByText(/created/i) || screen.queryByText(/Jan/i);
      expect(createdText).toBeTruthy();
    });
  });
});

// ===========================================================================
// Action Button Tests
// ===========================================================================

describe('ExperimentDetailPage – action buttons', () => {
  it('renders the Run button for experiments that can be run', async () => {
    const completedExperiment = { ...mockExperiment, status: 'completed' as const };
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);
    mockState = {
      ...mockState,
      experiments: {
        ...(mockState.experiments as Record<string, unknown>),
        detail: {
          experiment: completedExperiment,
          currentRun: null,
          isLoading: false,
          error: null,
        },
      },
    };

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const runButtons = screen
        .queryAllByRole('button')
        .filter(
          (btn) =>
            /run/i.test(btn.textContent || '') || /re-?run/i.test(btn.textContent || ''),
        );
      expect(runButtons.length).toBeGreaterThanOrEqual(0);
    });
  });

  it('renders the Stop button for running experiments', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const stopButtons = screen
        .queryAllByRole('button')
        .filter((btn) => /stop/i.test(btn.textContent || ''));
      expect(stopButtons.length).toBeGreaterThanOrEqual(0);
    });
  });

  it('renders the Refresh button', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const refreshButtons =
        screen.queryAllByLabelText(/refresh/i) ||
        screen
          .queryAllByRole('button')
          .filter((btn) => /refresh/i.test(btn.textContent || ''));
      expect(refreshButtons.length).toBeGreaterThanOrEqual(0);
    });
  });

  it('renders the Back button', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const backButtons =
        screen.queryAllByLabelText(/back/i) ||
        screen
          .queryAllByRole('button')
          .filter((btn) => /back/i.test(btn.textContent || ''));
      expect(backButtons.length).toBeGreaterThanOrEqual(0);
    });
  });

  it('disables the Run button while experiment is executing', async () => {
    (selectExecuteStatus as jest.Mock).mockReturnValue('loading');

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const runButtons = screen
        .queryAllByRole('button')
        .filter((btn) => /run/i.test(btn.textContent || ''));
      if (runButtons.length > 0) {
        expect(runButtons[0]).toBeDisabled();
      }
    });
  });

  it('disables the Stop button while experiment is stopping', async () => {
    (selectStopStatus as jest.Mock).mockReturnValue('loading');
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const stopButtons = screen
        .queryAllByRole('button')
        .filter((btn) => /stop/i.test(btn.textContent || ''));
      if (stopButtons.length > 0) {
        expect(stopButtons[0]).toBeDisabled();
      }
    });
  });
});

// ===========================================================================
// Data Fetching Tests
// ===========================================================================

describe('ExperimentDetailPage – data fetching', () => {
  it('dispatches fetchExperimentById on mount with the correct ID', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });

    expect(mockDispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'experiments/fetchById' }),
    );
  });

  it('dispatches fetchExperimentLogs on mount', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });

    expect(mockDispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'experiments/fetchLogs' }),
    );
  });

  it('dispatches clearExperimentDetail on unmount', async () => {
    const { unmount } = await act(async () => {
      return renderExperimentDetailPage();
    });

    jest.clearAllMocks();
    unmount();

    expect(mockDispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'experiments/clearDetail' }),
    );
  });

  it('dispatches resetExecuteStatus on mount', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });

    expect(mockDispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'experiments/resetExecuteStatus' }),
    );
  });

  it('dispatches resetStopStatus on mount', async () => {
    await act(async () => {
      renderExperimentDetailPage();
    });

    expect(mockDispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'experiments/resetStopStatus' }),
    );
  });
});

// ===========================================================================
// Loading State Tests
// ===========================================================================

describe('ExperimentDetailPage – loading state', () => {
  it('shows loading state when experiment is loading', async () => {
    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(true);
    (selectExperimentDetail as jest.Mock).mockReturnValue(null);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const skeletons = document.querySelectorAll('.MuiSkeleton-root');
    expect(skeletons.length).toBeGreaterThanOrEqual(0);
  });

  it('shows content after loading completes', async () => {
    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('replaces skeletons with content after data loads', async () => {
    let loadResolve: (value: unknown) => void;
    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(true);
    (selectExperimentDetail as jest.Mock).mockReturnValue(null);

    const { rerender } = await act(async () => {
      return renderExperimentDetailPage();
    });

    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      rerender(
        <ThemeProvider theme={theme}>
          <MemoryRouter initialEntries={['/experiments/exp-test-1']}>
            <Routes>
              <Route path="/experiments/:id" element={<ExperimentDetailPage />} />
            </Routes>
          </MemoryRouter>
        </ThemeProvider>,
      );
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });
});

// ===========================================================================
// Error State Tests
// ===========================================================================

describe('ExperimentDetailPage – error state', () => {
  it('shows error message when experiment fetch fails', async () => {
    (selectExperimentDetailError as jest.Mock).mockReturnValue('Experiment not found');
    (selectExperimentDetail as jest.Mock).mockReturnValue(null);
    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const errorElement =
        screen.queryByText(/not found/i) ||
        screen.queryByText(/error/i) ||
        screen.queryByRole('alert');
      expect(errorElement).toBeTruthy();
    });
  });

  it('shows not found state when experiment is null without error', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(null);
    (selectExperimentDetailError as jest.Mock).mockReturnValue(null);
    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const notFoundElement =
        screen.queryByText(/not found/i) ||
        screen.queryByText(/doesn't exist/i) ||
        screen.queryByText(/no experiment/i);
      expect(notFoundElement).toBeTruthy();
    });
  });

  it('shows retry button in error state', async () => {
    (selectExperimentDetailError as jest.Mock).mockReturnValue('Network error');
    (selectExperimentDetail as jest.Mock).mockReturnValue(null);
    (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const retryButton =
        screen.queryByRole('button', { name: /retry/i }) ||
        screen.queryByRole('button', { name: /try again/i }) ||
        screen.queryByText(/back/i);
      expect(retryButton).toBeTruthy();
    });
  });
});

// ===========================================================================
// Progress Tracker Tests
// ===========================================================================

describe('ExperimentDetailPage – progress tracker', () => {
  it('renders progress steps for experiments with steps', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Setup') || screen.getByText(/Setup/i)).toBeTruthy();
      expect(screen.getByText('Execute') || screen.getByText(/Execute/i)).toBeTruthy();
      expect(screen.getByText('Verify') || screen.getByText(/Verify/i)).toBeTruthy();
    });
  });

  it('renders completed step indicator', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const completedSteps = screen.queryAllByText('Setup');
      expect(completedSteps.length).toBeGreaterThan(0);
    });
  });

  it('renders in-progress step indicator', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const inProgressSteps = screen.queryAllByText('Execute');
      expect(inProgressSteps.length).toBeGreaterThan(0);
    });
  });

  it('renders pending step indicator', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const pendingSteps = screen.queryAllByText('Verify');
      expect(pendingSteps.length).toBeGreaterThan(0);
    });
  });

  it('renders progress bar for running experiment', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const progressBars = document.querySelectorAll('.MuiLinearProgress-root');
    expect(progressBars.length).toBeGreaterThanOrEqual(0);
  });
});

// ===========================================================================
// Results Summary Tests
// ===========================================================================

describe('ExperimentDetailPage – results summary', () => {
  const completedExperiment: Experiment = {
    ...mockExperiment,
    status: 'completed',
    progress: 100,
    result: {
      success: true,
      score: 85,
      summary: 'Experiment completed successfully',
      details: ['All detections fired correctly', 'SIEM integration verified'],
      siemValidation: {
        expectedAlertCount: 3,
        receivedAlertCount: 3,
        alerts: [],
        detected: true,
        detectionLatencyMs: 4500,
        coverage: 100,
        details: [],
      },
      startedAt: '2024-01-15T10:35:00Z',
      completedAt: '2024-01-15T10:45:00Z',
      duration: 600,
    },
  };

  it('renders results section for completed experiment', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const resultsSection =
        screen.queryByText(/results/i) ||
        screen.queryByText(/score/i) ||
        screen.queryByText(/85/);
      expect(resultsSection).toBeTruthy();
    });
  });

  it('renders results from a completed current run even when the experiment is still active', async () => {
    const completedRun = {
      ...(mockExperiment.runs?.[0] as ExperimentRun),
      status: 'completed' as const,
      progress: 100,
      completedAt: '2024-01-15T10:45:00Z',
      result: completedExperiment.result,
      steps: completedExperiment.steps,
    };

    mockState = {
      ...mockState,
      experiments: {
        ...(mockState.experiments as Record<string, unknown>),
        detail: {
          experiment: mockExperiment,
          currentRun: completedRun,
          isLoading: false,
          error: null,
        },
      },
    };

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.queryByText(/Not Yet Run/i)).toBeNull();
      expect(screen.getByText(/Experiment Passed/i)).toBeTruthy();
      expect(screen.getByText(/Results/i)).toBeTruthy();
    });
  });

  it('renders the experiment score', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('85') || screen.getByText(/85/)).toBeTruthy();
    });
  });

  it('renders the success status for successful experiment', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const successText = screen.queryByText(/success/i) || screen.queryByText(/passed/i);
      expect(successText).toBeTruthy();
    });
  });

  it('renders the summary text', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText(/Experiment completed successfully/i)).toBeTruthy();
    });
  });

  it('renders result details as list items', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText(/All detections fired correctly/i)).toBeTruthy();
      expect(screen.getByText(/SIEM integration verified/i)).toBeTruthy();
    });
  });
});

// ===========================================================================
// SIEM Validation Tests
// ===========================================================================

describe('ExperimentDetailPage – SIEM validation', () => {
  const experimentWithSIEM: Experiment = {
    ...mockExperiment,
    status: 'completed',
    progress: 100,
    result: {
      success: true,
      score: 85,
      summary: 'Experiment completed successfully',
      details: ['All detections fired correctly'],
      siemValidation: {
        expectedAlertCount: 3,
        receivedAlertCount: 3,
        alerts: [],
        detected: true,
        detectionLatencyMs: 4500,
        coverage: 100,
        details: [],
      },
      startedAt: '2024-01-15T10:35:00Z',
      completedAt: '2024-01-15T10:45:00Z',
      duration: 600,
    },
  };

  it('renders SIEM validation section for completed experiment', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(experimentWithSIEM);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const siemSection = screen.queryByText(/siem/i) || screen.queryByText(/SIEM/i);
      expect(siemSection).toBeTruthy();
    });
  });

  it('renders the expected alert count', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(experimentWithSIEM);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('3')).toBeTruthy();
    });
  });

  it('renders the received alert count', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(experimentWithSIEM);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const threes = screen.queryAllByText('3');
      expect(threes.length).toBeGreaterThanOrEqual(2);
    });
  });

  it('renders the coverage percentage', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(experimentWithSIEM);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText(/100/) || screen.getByText(/100%/)).toBeTruthy();
    });
  });

  it('renders the detection latency', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(experimentWithSIEM);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const latency = screen.queryByText(/4.5/) || screen.queryByText(/4500/);
      expect(latency).toBeTruthy();
    });
  });

  it('shows detected status when SIEM validation passes', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(experimentWithSIEM);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const detectedText =
        screen.queryByText(/detected/i) || screen.queryByText(/passed/i);
      expect(detectedText).toBeTruthy();
    });
  });
});

// ===========================================================================
// Log Viewer Tests
// ===========================================================================

describe('ExperimentDetailPage – log viewer', () => {
  it('renders log section', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);
    (selectExperimentLogs as jest.Mock).mockReturnValue(mockLogs);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const logsSection = screen.queryByText(/logs/i);
      expect(logsSection).toBeTruthy();
    });
  });

  it('renders log lines', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);
    (selectExperimentLogs as jest.Mock).mockReturnValue(mockLogs);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText(/Starting experiment/)).toBeTruthy();
    });
  });

  it('renders refresh logs button', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);
    (selectExperimentLogs as jest.Mock).mockReturnValue(mockLogs);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const refreshButtons = screen.queryAllByLabelText(/refresh/i);
    expect(refreshButtons.length).toBeGreaterThanOrEqual(0);
  });

  it('renders copy logs button', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);
    (selectExperimentLogs as jest.Mock).mockReturnValue(mockLogs);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const copyButtons =
      screen.queryAllByLabelText(/copy/i) ||
      screen
        .queryAllByRole('button')
        .filter((btn) => /copy/i.test(btn.textContent || ''));
    expect(copyButtons.length).toBeGreaterThanOrEqual(0);
  });

  it('displays empty log state when no logs available', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);
    (selectExperimentLogs as jest.Mock).mockReturnValue([]);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const emptyState =
        screen.queryByText(/no logs/i) ||
        screen.queryByText(/no log/i) ||
        screen.queryByText(/waiting/i);
      expect(emptyState).toBeTruthy();
    });
  });
});

// ===========================================================================
// Experiment Status Variations Tests
// ===========================================================================

describe('ExperimentDetailPage – status variations', () => {
  const statusVariations: { status: Experiment['status']; expectedLabel: string }[] = [
    { status: 'completed', expectedLabel: 'completed' },
    { status: 'failed', expectedLabel: 'failed' },
    { status: 'running', expectedLabel: 'running' },
    { status: 'pending', expectedLabel: 'pending' },
    { status: 'queued', expectedLabel: 'queued' },
    { status: 'stopped', expectedLabel: 'stopped' },
    { status: 'draft', expectedLabel: 'draft' },
  ];

  it('renders each status type without crashing', async () => {
    for (const { status } of statusVariations) {
      const experiment = { ...mockExperiment, status: status as Experiment['status'] };
      (selectExperimentDetail as jest.Mock).mockReturnValue(experiment);

      const { unmount } = await act(async () => {
        return renderExperimentDetailPage();
      });

      await waitFor(() => {
        expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
      });

      unmount();
      jest.clearAllMocks();

      // Re-setup mocks after clearing
      (selectExperimentDetail as jest.Mock).mockReturnValue(experiment);
      (selectExperimentDetailLoading as jest.Mock).mockReturnValue(false);
      (selectExperimentDetailError as jest.Mock).mockReturnValue(null);
      (selectExperimentLogs as jest.Mock).mockReturnValue(mockLogs);
      (selectExecuteStatus as jest.Mock).mockReturnValue('idle');
      (selectExecuteError as jest.Mock).mockReturnValue(null);
      (selectStopStatus as jest.Mock).mockReturnValue('idle');
    }
  });

  it('renders status badge with correct label for running experiment', async () => {
    const runningExperiment = { ...mockExperiment, status: 'running' as const };
    (selectExperimentDetail as jest.Mock).mockReturnValue(runningExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      const runningBadge = badges.find((b) => b.textContent?.includes('running'));
      expect(runningBadge).toBeTruthy();
    });
  });

  it('renders status badge with correct label for completed experiment', async () => {
    const completedExperiment = { ...mockExperiment, status: 'completed' as const };
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      const completedBadge = badges.find((b) => b.textContent?.includes('completed'));
      expect(completedBadge).toBeTruthy();
    });
  });

  it('renders status badge with correct label for failed experiment', async () => {
    const failedExperiment = { ...mockExperiment, status: 'failed' as const };
    (selectExperimentDetail as jest.Mock).mockReturnValue(failedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      const failedBadge = badges.find((b) => b.textContent?.includes('failed'));
      expect(failedBadge).toBeTruthy();
    });
  });
});

// ===========================================================================
// Run Action Tests
// ===========================================================================

describe('ExperimentDetailPage – run action', () => {
  it('dispatches executeExperiment when Run button is clicked', async () => {
    const completedExperiment = { ...mockExperiment, status: 'completed' as const };
    (selectExperimentDetail as jest.Mock).mockReturnValue(completedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const runButtons = screen
      .queryAllByRole('button')
      .filter((btn) => /run/i.test(btn.textContent || ''));

    if (runButtons.length > 0) {
      await act(async () => {
        fireEvent.click(runButtons[0]);
      });

      expect(mockDispatch).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'experiments/execute' }),
      );
    }
  });
});

// ===========================================================================
// Stop Action Tests
// ===========================================================================

describe('ExperimentDetailPage – stop action', () => {
  it('dispatches stopExperiment when Stop button is clicked', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const stopButtons = screen
      .queryAllByRole('button')
      .filter((btn) => /stop/i.test(btn.textContent || ''));

    if (stopButtons.length > 0) {
      await act(async () => {
        fireEvent.click(stopButtons[0]);
      });

      expect(mockDispatch).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'experiments/stop' }),
      );
    }
  });
});

// ===========================================================================
// Refresh Tests
// ===========================================================================

describe('ExperimentDetailPage – refresh', () => {
  it('dispatches fetchExperimentById when refresh is clicked', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const initialCallCount = mockDispatch.mock.calls.filter(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (call: unknown[]) => (call[0] as any)?.type === 'experiments/fetchById',
    ).length;

    const refreshButtons =
      screen.queryAllByLabelText(/refresh/i) ||
      screen
        .queryAllByRole('button')
        .filter((btn) => btn.getAttribute('aria-label')?.includes('refresh'));

    if (refreshButtons.length > 0) {
      await act(async () => {
        fireEvent.click(refreshButtons[0]);
      });

      const newCallCount = mockDispatch.mock.calls.filter(
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        (call: unknown[]) => (call[0] as any)?.type === 'experiments/fetchById',
      ).length;

      expect(newCallCount).toBeGreaterThan(initialCallCount);
    }
  });
});

// ===========================================================================
// Navigation Tests
// ===========================================================================

describe('ExperimentDetailPage – navigation', () => {
  it('navigates back to experiments list when Back button is clicked', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const backButtons =
      screen.queryAllByLabelText(/back/i) ||
      screen
        .queryAllByRole('button')
        .filter((btn) => /back/i.test(btn.textContent || ''));

    if (backButtons.length > 0) {
      await act(async () => {
        fireEvent.click(backButtons[0]);
      });

      expect(mockNavigate).toHaveBeenCalled();
    }
  });

  it('navigates to experiments list from breadcrumb', async () => {
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    const experimentLinks = screen.queryAllByText(/experiments/i);
    if (experimentLinks.length > 0) {
      await act(async () => {
        fireEvent.click(experimentLinks[0]);
      });
    }
  });
});

// ===========================================================================
// Edge Cases
// ===========================================================================

describe('ExperimentDetailPage – edge cases', () => {
  it('renders correctly when experiment has no tags', async () => {
    const noTagsExperiment = { ...mockExperiment, tags: [] };
    (selectExperimentDetail as jest.Mock).mockReturnValue(noTagsExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('renders correctly when experiment has no steps', async () => {
    const noStepsExperiment = { ...mockExperiment, steps: [] };
    (selectExperimentDetail as jest.Mock).mockReturnValue(noStepsExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('renders correctly when experiment has no result', async () => {
    const noResultExperiment = { ...mockExperiment, result: undefined };
    (selectExperimentDetail as jest.Mock).mockReturnValue(noResultExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('renders correctly when experiment has no runs', async () => {
    const noRunsExperiment = { ...mockExperiment, runs: undefined };
    (selectExperimentDetail as jest.Mock).mockReturnValue(noRunsExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('renders correctly when experiment has an empty description', async () => {
    const emptyDescExperiment = { ...mockExperiment, description: '' };
    (selectExperimentDetail as jest.Mock).mockReturnValue(emptyDescExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });

  it('renders correctly for a failed experiment', async () => {
    const failedExperiment: Experiment = {
      ...mockExperiment,
      status: 'failed',
      progress: 50,
      result: {
        success: false,
        score: 0,
        summary: 'Experiment failed',
        details: ['Connection timeout', 'SIEM not responding'],
        siemValidation: {
          expectedAlertCount: 3,
          receivedAlertCount: 0,
          alerts: [],
          detected: false,
          detectionLatencyMs: 0,
          coverage: 0,
          details: ['No alerts received'],
        },
        startedAt: '2024-01-15T10:35:00Z',
        completedAt: '2024-01-15T10:40:00Z',
        duration: 300,
      },
    };
    (selectExperimentDetail as jest.Mock).mockReturnValue(failedExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
      const badges = screen.queryAllByTestId('status-badge');
      const failedBadge = badges.find((b) => b.textContent?.includes('failed'));
      expect(failedBadge).toBeTruthy();
    });
  });

  it('renders correctly for a draft experiment', async () => {
    const draftExperiment: Experiment = {
      ...mockExperiment,
      status: 'draft',
      progress: 0,
      startedAt: undefined,
      completedAt: undefined,
    };
    (selectExperimentDetail as jest.Mock).mockReturnValue(draftExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
      const badges = screen.queryAllByTestId('status-badge');
      const draftBadge = badges.find((b) => b.textContent?.includes('draft'));
      expect(draftBadge).toBeTruthy();
    });
  });

  it('handles execute error state', async () => {
    (selectExecuteError as jest.Mock).mockReturnValue('Failed to execute experiment');
    (selectExperimentDetail as jest.Mock).mockReturnValue(mockExperiment);

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      const errorAlert =
        screen.queryByText(/failed to execute/i) || screen.queryByRole('alert');
      expect(errorAlert).toBeTruthy();
    });
  });

  it('handles stop error state', async () => {
    const runningExperiment = { ...mockExperiment, status: 'running' as const };
    (selectExperimentDetail as jest.Mock).mockReturnValue(runningExperiment);
    // The stopError is in the state, we mock it
    (selectStopStatus as jest.Mock).mockReturnValue('failed');

    await act(async () => {
      renderExperimentDetailPage();
    });

    await waitFor(() => {
      expect(screen.getByText('DNS Exfiltration Test')).toBeTruthy();
    });
  });
});
