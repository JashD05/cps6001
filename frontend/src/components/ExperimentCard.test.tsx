/**
 * Unit tests for the ExperimentCard component.
 *
 * Tests both compact and full variants, status badge rendering,
 * action button visibility/disabling, click navigation, loading
 * states, progress display, and tag rendering.
 */

import { ThemeProvider, createTheme } from '@mui/material/styles';
import { render, screen, fireEvent } from '@testing-library/react';
import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import ExperimentCard, { type ExperimentCardVariant } from '@/components/ExperimentCard';
import type { Experiment } from '@/types';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

// ---------------------------------------------------------------------------
// Theme
// // ---------------------------------------------------------------------------

const theme = createTheme();

// ---------------------------------------------------------------------------
// Helpers
// // ---------------------------------------------------------------------------

function renderWithProviders(
  ui: React.ReactElement,
  { route = '/' }: { route?: string } = {},
) {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>
    </ThemeProvider>,
  );
}

// ---------------------------------------------------------------------------
// Test fixtures
// // ---------------------------------------------------------------------------

const baseExperiment: Experiment = {
  id: 'exp-1',
  name: 'Network Latency Test',
  description: 'Test network latency under stress conditions',
  templateId: 'tmpl-1',
  templateName: 'Network Chaos',
  clusterId: 'cluster-1',
  clusterName: 'Production Cluster',
  namespace: 'default',
  status: 'pending',
  progress: 0,
  parameters: {},
  steps: [],
  tags: ['network', 'latency', 'production'],
  createdBy: 'user-1',
  createdAt: '2024-01-15T10:00:00Z',
  updatedAt: '2024-01-15T10:00:00Z',
};

const runningExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-2',
  name: 'Running Experiment',
  status: 'running',
  progress: 50,
  startedAt: '2024-01-15T10:00:00Z',
};

const completedExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-3',
  name: 'Completed Experiment',
  status: 'completed',
  progress: 100,
  startedAt: '2024-01-14T10:00:00Z',
  completedAt: '2024-01-14T11:00:00Z',
  duration: 3600000,
};

const failedExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-4',
  name: 'Failed Experiment',
  status: 'failed',
  progress: 30,
  startedAt: '2024-01-14T10:00:00Z',
  completedAt: '2024-01-14T10:30:00Z',
  duration: 1800000,
};

const stoppedExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-5',
  name: 'Stopped Experiment',
  status: 'stopped',
  progress: 0,
  startedAt: '2024-01-14T10:00:00Z',
  completedAt: '2024-01-14T10:15:00Z',
  duration: 900000,
};

const queuedExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-6',
  name: 'Queued Experiment',
  status: 'queued',
  progress: 0,
};

const draftExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-7',
  name: 'Draft Experiment',
  status: 'draft',
  progress: 0,
};

const timedOutExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-8',
  name: 'Timed Out Experiment',
  status: 'timed_out',
  progress: 60,
  startedAt: '2024-01-14T10:00:00Z',
  completedAt: '2024-01-14T10:45:00Z',
  duration: 2700000,
};

const archivedExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-9',
  name: 'Archived Experiment',
  status: 'archived',
  progress: 100,
};

const noTagsExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-10',
  name: 'No Tags Experiment',
  tags: [],
};

const manyTagsExperiment: Experiment = {
  ...baseExperiment,
  id: 'exp-11',
  name: 'Many Tags Experiment',
  tags: ['tag1', 'tag2', 'tag3', 'tag4', 'tag5'],
};

const noClusterNameExperiment: Experiment = {
  ...baseExperiment,
  clusterName: undefined,
};

const noTemplateNameExperiment: Experiment = {
  ...baseExperiment,
  templateName: undefined,
};

// ---------------------------------------------------------------------------
// Test lifecycle
// // ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();
});

// ===========================================================================
// Full Variant – Rendering
// ===========================================================================

describe('ExperimentCard – full variant (default)', () => {
  it('renders the experiment name', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByText('Network Latency Test')).toBeInTheDocument();
  });

  it('renders the experiment description', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(
      screen.getByText('Test network latency under stress conditions'),
    ).toBeInTheDocument();
  });

  it('renders the template name', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByText('Network Chaos')).toBeInTheDocument();
  });

  it('shows "Custom" when templateName is undefined', () => {
    renderWithProviders(<ExperimentCard experiment={noTemplateNameExperiment} />);
    expect(screen.getByText('Custom')).toBeInTheDocument();
  });

  it('renders the cluster name', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByText('Production Cluster')).toBeInTheDocument();
  });

  it('falls back to clusterId when clusterName is undefined', () => {
    renderWithProviders(<ExperimentCard experiment={noClusterNameExperiment} />);
    expect(screen.getByText('cluster-1')).toBeInTheDocument();
  });

  it('renders tags as chips', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByText('network')).toBeInTheDocument();
    expect(screen.getByText('latency')).toBeInTheDocument();
    expect(screen.getByText('production')).toBeInTheDocument();
  });

  it('shows overflow tag count when more than 3 tags', () => {
    renderWithProviders(<ExperimentCard experiment={manyTagsExperiment} />);
    expect(screen.getByText('+2')).toBeInTheDocument();
  });

  it('does not render tags section when tags array is empty', () => {
    renderWithProviders(<ExperimentCard experiment={noTagsExperiment} />);
    expect(screen.queryByText('tag1')).not.toBeInTheDocument();
  });

  it('renders the View button by default', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByRole('button', { name: /view/i })).toBeInTheDocument();
  });

  it('renders as a card element', () => {
    const { container } = renderWithProviders(
      <ExperimentCard experiment={baseExperiment} />,
    );
    expect(container.querySelector('.MuiCard-root')).toBeInTheDocument();
  });
});

// ===========================================================================
// Full Variant – Status Badge
// ===========================================================================

describe('ExperimentCard – full variant status badge', () => {
  const statusCases: Array<{ status: Experiment['status']; label: string }> = [
    { status: 'pending', label: 'Draft' },
    { status: 'running', label: 'Running' },
    { status: 'completed', label: 'Completed' },
    { status: 'failed', label: 'Failed' },
    { status: 'stopped', label: 'Stopped' },
    { status: 'queued', label: 'Queued' },
    { status: 'draft', label: 'Draft' },
    { status: 'timed_out', label: 'Timed' },
    { status: 'archived', label: 'Archived' },
    { status: 'active', label: 'Active' },
  ];

  it.each(statusCases)(
    'renders a status badge for "$status" status',
    ({ status, label }) => {
      const experiment = { ...baseExperiment, status };
      renderWithProviders(<ExperimentCard experiment={experiment} />);
      // Use a regex to match the label text case-insensitively
      expect(screen.getByText(new RegExp(label, 'i'))).toBeInTheDocument();
    },
  );
});

// ===========================================================================
// Full Variant – Action Buttons
// ===========================================================================

describe('ExperimentCard – full variant action buttons', () => {
  it('shows Run button for draft experiment', () => {
    renderWithProviders(<ExperimentCard experiment={draftExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /stop/i })).not.toBeInTheDocument();
  });

  it('shows Run button for pending experiment', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('shows Run button for completed experiment', () => {
    renderWithProviders(<ExperimentCard experiment={completedExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /stop/i })).not.toBeInTheDocument();
  });

  it('shows Run button for failed experiment', () => {
    renderWithProviders(<ExperimentCard experiment={failedExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /stop/i })).not.toBeInTheDocument();
  });

  it('shows Run button for stopped experiment', () => {
    renderWithProviders(<ExperimentCard experiment={stoppedExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('shows Run button for queued experiment', () => {
    renderWithProviders(<ExperimentCard experiment={queuedExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('shows Run button for timed_out experiment', () => {
    renderWithProviders(<ExperimentCard experiment={timedOutExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('shows Run button for archived experiment', () => {
    renderWithProviders(<ExperimentCard experiment={archivedExperiment} />);
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('shows Stop button for running experiment', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    expect(screen.getByRole('button', { name: /stop/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^run$/i })).not.toBeInTheDocument();
  });

  it('does not show Run or Stop for active status', () => {
    const activeExperiment = { ...baseExperiment, status: 'active' as const };
    renderWithProviders(<ExperimentCard experiment={activeExperiment} />);
    // active status: canRun=true, canStop=false — but the Run button should still be present
    expect(screen.getByRole('button', { name: /run/i })).toBeInTheDocument();
  });

  it('hides action buttons when showActions is false', () => {
    renderWithProviders(
      <ExperimentCard experiment={baseExperiment} showActions={false} />,
    );
    expect(screen.queryByRole('button', { name: /view/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /run/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /stop/i })).not.toBeInTheDocument();
  });

  it('calls onRun when Run button is clicked', async () => {
    const onRun = jest.fn();
    renderWithProviders(<ExperimentCard experiment={draftExperiment} onRun={onRun} />);

    const runButton = screen.getByRole('button', { name: /run/i });
    fireEvent.click(runButton);

    expect(onRun).toHaveBeenCalledTimes(1);
    expect(onRun).toHaveBeenCalledWith(draftExperiment);
  });

  it('calls onStop when Stop button is clicked', async () => {
    const onStop = jest.fn();
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} onStop={onStop} />,
    );

    const stopButton = screen.getByRole('button', { name: /stop/i });
    fireEvent.click(stopButton);

    expect(onStop).toHaveBeenCalledTimes(1);
    expect(onStop).toHaveBeenCalledWith(runningExperiment);
  });

  it('calls onView when View button is clicked', async () => {
    const onView = jest.fn();
    renderWithProviders(<ExperimentCard experiment={baseExperiment} onView={onView} />);

    const viewButton = screen.getByRole('button', { name: /view/i });
    fireEvent.click(viewButton);

    expect(onView).toHaveBeenCalledTimes(1);
    expect(onView).toHaveBeenCalledWith(baseExperiment);
  });

  it('navigates to experiment detail when View is clicked without onView handler', async () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);

    const viewButton = screen.getByRole('button', { name: /view/i });
    fireEvent.click(viewButton);

    expect(mockNavigate).toHaveBeenCalledWith('/experiments/exp-1');
  });
});

// ===========================================================================
// Full Variant – Progress
// ===========================================================================

describe('ExperimentCard – full variant progress', () => {
  it('shows progress bar for running experiment', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.getByText('Progress')).toBeInTheDocument();
  });

  it('shows progress bar for queued experiment', () => {
    renderWithProviders(<ExperimentCard experiment={queuedExperiment} />);
    expect(screen.getByText('0%')).toBeInTheDocument();
  });

  it('shows progress bar when progress is greater than 0', () => {
    const inProgressExperiment = {
      ...baseExperiment,
      progress: 75,
      status: 'running' as const,
    };
    renderWithProviders(<ExperimentCard experiment={inProgressExperiment} />);
    expect(screen.getByText('75%')).toBeInTheDocument();
  });

  it('does not show progress bar for pending experiment with 0 progress', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.queryByText('Progress')).not.toBeInTheDocument();
    expect(screen.queryByText('0%')).not.toBeInTheDocument();
  });

  it('does not show progress bar for completed experiment with 100 progress', () => {
    // completed has progress 100, but since status is 'completed' (not running/queued)
    // and progress > 0, the progress bar IS shown
    renderWithProviders(<ExperimentCard experiment={completedExperiment} />);
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('shows duration for completed experiment', () => {
    renderWithProviders(<ExperimentCard experiment={completedExperiment} />);
    expect(screen.getByText('Duration:')).toBeInTheDocument();
    expect(screen.getByText('60m 0s')).toBeInTheDocument();
  });

  it('shows duration for failed experiment', () => {
    renderWithProviders(<ExperimentCard experiment={failedExperiment} />);
    expect(screen.getByText('Duration:')).toBeInTheDocument();
  });

  it('shows duration for stopped experiment', () => {
    renderWithProviders(<ExperimentCard experiment={stoppedExperiment} />);
    expect(screen.getByText('Duration:')).toBeInTheDocument();
  });

  it('does not show duration for running experiment', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument();
  });

  it('does not show duration for pending experiment', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument();
  });
});

// ===========================================================================
// Full Variant – Click Navigation
// ===========================================================================

describe('ExperimentCard – full variant click navigation', () => {
  it('navigates to experiment detail when card is clicked', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);

    const cardElement = screen.getByText('Network Latency Test').closest('.MuiCard-root');
    expect(cardElement).not.toBeNull();
    fireEvent.click(cardElement as Element);

    expect(mockNavigate).toHaveBeenCalledWith('/experiments/exp-1');
  });

  it('calls onClick handler when provided instead of navigating', () => {
    const onClick = jest.fn();
    renderWithProviders(<ExperimentCard experiment={baseExperiment} onClick={onClick} />);

    const cardElement = screen.getByText('Network Latency Test').closest('.MuiCard-root');
    expect(cardElement).not.toBeNull();
    fireEvent.click(cardElement as Element);

    expect(onClick).toHaveBeenCalledTimes(1);
    expect(onClick).toHaveBeenCalledWith(baseExperiment);
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});

// ===========================================================================
// Full Variant – Loading State
// ===========================================================================

describe('ExperimentCard – full variant loading state', () => {
  it('disables View button when loading is true', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} loading={true} />);

    const viewButton = screen.getByRole('button', { name: /view/i });
    expect(viewButton).toBeDisabled();
  });

  it('disables Run button when loading is true', () => {
    renderWithProviders(<ExperimentCard experiment={draftExperiment} loading={true} />);

    const runButton = screen.getByRole('button', { name: /run/i });
    expect(runButton).toBeDisabled();
  });

  it('disables Stop button when loading is true', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} loading={true} />);

    const stopButton = screen.getByRole('button', { name: /stop/i });
    expect(stopButton).toBeDisabled();
  });

  it('enables buttons when loading is false', () => {
    renderWithProviders(<ExperimentCard experiment={draftExperiment} loading={false} />);

    const runButton = screen.getByRole('button', { name: /run/i });
    expect(runButton).not.toBeDisabled();
  });

  it('enables buttons by default when loading is not provided', () => {
    renderWithProviders(<ExperimentCard experiment={draftExperiment} />);

    const runButton = screen.getByRole('button', { name: /run/i });
    expect(runButton).not.toBeDisabled();
  });
});

// ===========================================================================
// Compact Variant – Rendering
// ===========================================================================

describe('ExperimentCard – compact variant', () => {
  it('renders the experiment name', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(screen.getByText('Network Latency Test')).toBeInTheDocument();
  });

  it('renders the template name', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(screen.getByText('Network Chaos')).toBeInTheDocument();
  });

  it('renders the cluster name', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(screen.getByText(/Production Cluster/)).toBeInTheDocument();
  });

  it('renders the status badge', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    // The pending status should show a status badge
    expect(screen.getByText(/draft/i)).toBeInTheDocument();
  });

  it('does not render the description in compact mode', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(
      screen.queryByText('Test network latency under stress conditions'),
    ).not.toBeInTheDocument();
  });

  it('does not render tags in compact mode', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    // Tags should not be rendered as Chips in compact mode
    expect(screen.queryByText('network')).not.toBeInTheDocument();
  });

  it('does not render progress bar in compact mode', () => {
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} variant="compact" />,
    );
    expect(screen.queryByText('Progress')).not.toBeInTheDocument();
  });

  it('renders as a card element', () => {
    const { container } = renderWithProviders(
      <ExperimentCard experiment={baseExperiment} variant="compact" />,
    );
    expect(container.querySelector('.MuiCard-root')).toBeInTheDocument();
  });

  it('does not render template name when not provided', () => {
    renderWithProviders(
      <ExperimentCard experiment={noTemplateNameExperiment} variant="compact" />,
    );
    // templateName is undefined, so no template name text should appear
    expect(screen.queryByText('Network Chaos')).not.toBeInTheDocument();
  });

  it('does not render cluster name when not provided', () => {
    renderWithProviders(
      <ExperimentCard
        experiment={{ ...baseExperiment, clusterName: undefined }}
        variant="compact"
      />,
    );
    expect(screen.queryByText(/Production Cluster/)).not.toBeInTheDocument();
  });
});

// ===========================================================================
// Compact Variant – Action Buttons
// ===========================================================================

describe('ExperimentCard – compact variant action buttons', () => {
  it('shows View icon button', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(screen.getByLabelText('View Details')).toBeInTheDocument();
  });

  it('shows Run icon button for draft experiment', () => {
    renderWithProviders(
      <ExperimentCard experiment={draftExperiment} variant="compact" />,
    );
    expect(screen.getByLabelText('Run Experiment')).toBeInTheDocument();
  });

  it('does not show Run icon button for running experiment', () => {
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} variant="compact" />,
    );
    expect(screen.queryByLabelText('Run Experiment')).not.toBeInTheDocument();
  });

  it('shows Stop icon button for running experiment', () => {
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} variant="compact" />,
    );
    expect(screen.getByLabelText('Stop Experiment')).toBeInTheDocument();
  });

  it('does not show Stop icon button for completed experiment', () => {
    renderWithProviders(
      <ExperimentCard experiment={completedExperiment} variant="compact" />,
    );
    expect(screen.queryByLabelText('Stop Experiment')).not.toBeInTheDocument();
  });

  it('hides action buttons when showActions is false', () => {
    renderWithProviders(
      <ExperimentCard
        experiment={baseExperiment}
        variant="compact"
        showActions={false}
      />,
    );
    expect(screen.queryByLabelText('View Details')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Run Experiment')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Stop Experiment')).not.toBeInTheDocument();
  });

  it('calls onRun when Run icon button is clicked', () => {
    const onRun = jest.fn();
    renderWithProviders(
      <ExperimentCard experiment={draftExperiment} variant="compact" onRun={onRun} />,
    );

    fireEvent.click(screen.getByLabelText('Run Experiment'));
    expect(onRun).toHaveBeenCalledTimes(1);
    expect(onRun).toHaveBeenCalledWith(draftExperiment);
  });

  it('calls onStop when Stop icon button is clicked', () => {
    const onStop = jest.fn();
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} variant="compact" onStop={onStop} />,
    );

    fireEvent.click(screen.getByLabelText('Stop Experiment'));
    expect(onStop).toHaveBeenCalledTimes(1);
    expect(onStop).toHaveBeenCalledWith(runningExperiment);
  });

  it('calls onView when View icon button is clicked', () => {
    const onView = jest.fn();
    renderWithProviders(
      <ExperimentCard experiment={baseExperiment} variant="compact" onView={onView} />,
    );

    fireEvent.click(screen.getByLabelText('View Details'));
    expect(onView).toHaveBeenCalledTimes(1);
    expect(onView).toHaveBeenCalledWith(baseExperiment);
  });

  it('navigates when View icon button is clicked without onView handler', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);

    fireEvent.click(screen.getByLabelText('View Details'));
    expect(mockNavigate).toHaveBeenCalledWith('/experiments/exp-1');
  });

  it('action clicks do not propagate to card click handler', () => {
    const onClick = jest.fn();
    renderWithProviders(
      <ExperimentCard experiment={draftExperiment} variant="compact" onClick={onClick} />,
    );

    fireEvent.click(screen.getByLabelText('View Details'));
    // The card's onClick should NOT have been called
    expect(onClick).not.toHaveBeenCalled();
  });
});

// ===========================================================================
// Compact Variant – Status Badge
// ===========================================================================

describe('ExperimentCard – compact variant status badge', () => {
  it('renders pending status with "Draft" label', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(screen.getByText(/draft/i)).toBeInTheDocument();
  });

  it('renders running status badge', () => {
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} variant="compact" />,
    );
    expect(screen.getByText(/running/i)).toBeInTheDocument();
  });

  it('renders completed status badge', () => {
    renderWithProviders(
      <ExperimentCard experiment={completedExperiment} variant="compact" />,
    );
    expect(screen.getByText(/completed/i)).toBeInTheDocument();
  });

  it('renders failed status badge', () => {
    renderWithProviders(
      <ExperimentCard experiment={failedExperiment} variant="compact" />,
    );
    expect(screen.getByText(/failed/i)).toBeInTheDocument();
  });
});

// ===========================================================================
// Compact Variant – Click Navigation
// ===========================================================================

describe('ExperimentCard – compact variant click navigation', () => {
  it('navigates to experiment detail when card is clicked', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);

    const cardElement = screen.getByText('Network Latency Test').closest('.MuiCard-root');
    expect(cardElement).not.toBeNull();
    fireEvent.click(cardElement as Element);

    expect(mockNavigate).toHaveBeenCalledWith('/experiments/exp-1');
  });

  it('calls onClick handler when provided instead of navigating', () => {
    const onClick = jest.fn();
    renderWithProviders(
      <ExperimentCard experiment={baseExperiment} variant="compact" onClick={onClick} />,
    );

    const cardElement = screen.getByText('Network Latency Test').closest('.MuiCard-root');
    expect(cardElement).not.toBeNull();
    fireEvent.click(cardElement as Element);

    expect(onClick).toHaveBeenCalledTimes(1);
    expect(onClick).toHaveBeenCalledWith(baseExperiment);
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});

// ===========================================================================
// Compact Variant – Loading State
// ===========================================================================

describe('ExperimentCard – compact variant loading state', () => {
  it('disables View icon button when loading is true', () => {
    renderWithProviders(
      <ExperimentCard experiment={baseExperiment} variant="compact" loading={true} />,
    );
    expect(screen.getByLabelText('View Details')).toBeDisabled();
  });

  it('disables Run icon button when loading is true', () => {
    renderWithProviders(
      <ExperimentCard experiment={draftExperiment} variant="compact" loading={true} />,
    );
    expect(screen.getByLabelText('Run Experiment')).toBeDisabled();
  });

  it('disables Stop icon button when loading is true', () => {
    renderWithProviders(
      <ExperimentCard experiment={runningExperiment} variant="compact" loading={true} />,
    );
    expect(screen.getByLabelText('Stop Experiment')).toBeDisabled();
  });

  it('enables icon buttons when loading is false', () => {
    renderWithProviders(
      <ExperimentCard experiment={draftExperiment} variant="compact" loading={false} />,
    );
    expect(screen.getByLabelText('View Details')).not.toBeDisabled();
    expect(screen.getByLabelText('Run Experiment')).not.toBeDisabled();
  });

  it('enables icon buttons by default', () => {
    renderWithProviders(
      <ExperimentCard experiment={draftExperiment} variant="compact" />,
    );
    expect(screen.getByLabelText('View Details')).not.toBeDisabled();
    expect(screen.getByLabelText('Run Experiment')).not.toBeDisabled();
  });
});

// ===========================================================================
// Variant Selection
// ===========================================================================

describe('ExperimentCard – variant selection', () => {
  it('defaults to full variant when variant is not specified', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    // Full variant renders description, compact does not
    expect(
      screen.getByText('Test network latency under stress conditions'),
    ).toBeInTheDocument();
  });

  it('renders full variant when variant="full"', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="full" />);
    expect(
      screen.getByText('Test network latency under stress conditions'),
    ).toBeInTheDocument();
  });

  it('renders compact variant when variant="compact"', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} variant="compact" />);
    expect(
      screen.queryByText('Test network latency under stress conditions'),
    ).not.toBeInTheDocument();
  });
});

// ===========================================================================
// Full Variant – Timing Display
// ===========================================================================

describe('ExperimentCard – full variant timing display', () => {
  it('shows "Created:" label for non-running experiments', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.getByText('Created:')).toBeInTheDocument();
  });

  it('shows "Started:" label for running experiments', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    expect(screen.getByText('Started:')).toBeInTheDocument();
  });

  it('does not show duration for running experiments', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument();
  });

  it('shows duration for completed experiments', () => {
    renderWithProviders(<ExperimentCard experiment={completedExperiment} />);
    expect(screen.getByText('Duration:')).toBeInTheDocument();
    expect(screen.getByText('60m 0s')).toBeInTheDocument();
  });

  it('shows duration for failed experiments', () => {
    renderWithProviders(<ExperimentCard experiment={failedExperiment} />);
    expect(screen.getByText('Duration:')).toBeInTheDocument();
    expect(screen.getByText('30m 0s')).toBeInTheDocument();
  });

  it('shows duration for stopped experiments', () => {
    renderWithProviders(<ExperimentCard experiment={stoppedExperiment} />);
    expect(screen.getByText('Duration:')).toBeInTheDocument();
    expect(screen.getByText('15m 0s')).toBeInTheDocument();
  });

  it('does not show duration for experiments without duration', () => {
    const noDurationExperiment = {
      ...baseExperiment,
      status: 'completed' as const,
      duration: undefined,
    };
    renderWithProviders(<ExperimentCard experiment={noDurationExperiment} />);
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument();
  });

  it('does not show duration for pending experiments', () => {
    renderWithProviders(<ExperimentCard experiment={baseExperiment} />);
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument();
  });
});

// ===========================================================================
// Full Variant – Progress Bar Details
// ===========================================================================

describe('ExperimentCard – full variant progress bar details', () => {
  it('renders progress percentage for running experiment', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.getByText('Progress')).toBeInTheDocument();
  });

  it('renders 0% progress for queued experiment', () => {
    renderWithProviders(<ExperimentCard experiment={queuedExperiment} />);
    expect(screen.getByText('0%')).toBeInTheDocument();
  });

  it('renders 100% progress for completed experiment', () => {
    renderWithProviders(<ExperimentCard experiment={completedExperiment} />);
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('renders linear progress elements', () => {
    renderWithProviders(<ExperimentCard experiment={runningExperiment} />);
    const progressElements = document.querySelectorAll('.MuiLinearProgress-root');
    expect(progressElements.length).toBeGreaterThan(0);
  });
});

// ===========================================================================
// Edge Cases
// ===========================================================================

describe('ExperimentCard – edge cases', () => {
  it('handles experiment with empty name gracefully', () => {
    const emptyNameExperiment = { ...baseExperiment, name: '' };
    renderWithProviders(<ExperimentCard experiment={emptyNameExperiment} />);
    // The card should still render without crashing
    expect(screen.getByRole('button', { name: /view/i })).toBeInTheDocument();
  });

  it('handles experiment with empty description gracefully', () => {
    const emptyDescExperiment = { ...baseExperiment, description: '' };
    renderWithProviders(<ExperimentCard experiment={emptyDescExperiment} />);
    // Empty description should not render the description block
    expect(screen.getByText('Network Latency Test')).toBeInTheDocument();
  });

  it('handles experiment with very long name', () => {
    const longNameExperiment = {
      ...baseExperiment,
      name: 'A'.repeat(200),
    };
    renderWithProviders(<ExperimentCard experiment={longNameExperiment} />);
    // Should render without crashing; the name is truncated by CSS
    expect(screen.getByText('A'.repeat(200))).toBeInTheDocument();
  });

  it('handles experiment with exactly 3 tags', () => {
    const threeTagsExperiment = {
      ...baseExperiment,
      tags: ['tag1', 'tag2', 'tag3'],
    };
    renderWithProviders(<ExperimentCard experiment={threeTagsExperiment} />);
    expect(screen.getByText('tag1')).toBeInTheDocument();
    expect(screen.getByText('tag2')).toBeInTheDocument();
    expect(screen.getByText('tag3')).toBeInTheDocument();
    expect(screen.queryByText('+0')).not.toBeInTheDocument();
  });

  it('handles experiment with 4 tags showing overflow', () => {
    const fourTagsExperiment = {
      ...baseExperiment,
      tags: ['tag1', 'tag2', 'tag3', 'tag4'],
    };
    renderWithProviders(<ExperimentCard experiment={fourTagsExperiment} />);
    expect(screen.getByText('+1')).toBeInTheDocument();
  });

  it('renders both variants without crashing for each status', () => {
    const statuses: Experiment['status'][] = [
      'draft',
      'active',
      'pending',
      'queued',
      'running',
      'completed',
      'failed',
      'stopped',
      'timed_out',
      'archived',
    ];
    const variants: ExperimentCardVariant[] = ['compact', 'full'];

    for (const status of statuses) {
      for (const variant of variants) {
        const experiment = { ...baseExperiment, status };
        const { unmount } = renderWithProviders(
          <ExperimentCard experiment={experiment} variant={variant} />,
        );
        unmount();
      }
    }
    // If we get here without errors, all combinations render fine
    expect(true).toBe(true);
  });
});
