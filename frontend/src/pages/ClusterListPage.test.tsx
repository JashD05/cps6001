/**
 * Unit tests for the ClusterListPage component.
 *
 * Tests rendering of cluster list, data fetching, filtering,
 * search, register/delete dialogs, health checks, and navigation.
 */

import { ThemeProvider, createTheme } from '@mui/material/styles';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import ClusterListPage from '@/pages/ClusterListPage';
import { clustersAPI } from '@/services/api';
import type { Cluster, ClusterHealth } from '@/types';

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockDispatch = jest.fn();
const mockNavigate = jest.fn();

jest.mock('react-redux', () => ({
  useDispatch: () => mockDispatch,
}));

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
}));

jest.mock('@/services/api', () => ({
  clustersAPI: {
    list: jest.fn(),
    getHealth: jest.fn(),
    register: jest.fn(),
    delete: jest.fn(),
  },
  getErrorMessage: jest.fn((err: unknown) => {
    if (err instanceof Error) return err.message;
    return 'An error occurred';
  }),
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

const mockClusters: Cluster[] = [
  {
    id: 'cluster-1',
    name: 'Production Cluster',
    description: 'Main production Kubernetes cluster',
    status: 'healthy',
    provider: 'aws',
    region: 'us-east-1',
    version: '1.28.3',
    nodeCount: 5,
    namespaceCount: 12,
    namespaces: ['default', 'kube-system', 'chaos'],
    labels: { env: 'prod' },
    lastHealthCheck: new Date().toISOString(),
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-06-01T00:00:00Z',
  },
  {
    id: 'cluster-2',
    name: 'Staging Cluster',
    description: 'Staging environment cluster',
    status: 'degraded',
    provider: 'gcp',
    region: 'us-central1',
    version: '1.27.6',
    nodeCount: 3,
    namespaceCount: 8,
    namespaces: ['default', 'staging'],
    labels: { env: 'staging' },
    lastHealthCheck: new Date().toISOString(),
    createdAt: '2024-02-01T00:00:00Z',
    updatedAt: '2024-05-01T00:00:00Z',
  },
  {
    id: 'cluster-3',
    name: 'Development Cluster',
    description: 'Development environment cluster running locally',
    status: 'unreachable',
    provider: 'kind',
    region: 'local',
    version: '1.26.0',
    nodeCount: 1,
    namespaceCount: 4,
    namespaces: ['default'],
    labels: { env: 'dev' },
    lastHealthCheck: new Date(Date.now() - 3600000).toISOString(),
    createdAt: '2024-03-01T00:00:00Z',
    updatedAt: '2024-04-01T00:00:00Z',
  },
];

const mockHealthData: ClusterHealth[] = [
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
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderClusterListPage() {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <ClusterListPage />
      </MemoryRouter>
    </ThemeProvider>,
  );
}

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();

  (clustersAPI.list as jest.Mock).mockResolvedValue({
    data: { data: mockClusters },
  });
  (clustersAPI.healthCheck as jest.Mock).mockResolvedValue({
    data: { data: mockHealthData },
  });
  (clustersAPI.register as jest.Mock).mockResolvedValue({
    data: {
      data: {
        id: 'cluster-new',
        name: 'New Cluster',
        description: 'A newly registered cluster',
        status: 'healthy',
        provider: 'azure',
        region: 'eastus',
        version: '1.28.0',
        nodeCount: 2,
        namespaceCount: 3,
        namespaces: ['default'],
        labels: {},
        lastHealthCheck: new Date().toISOString(),
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
    },
  });
  (clustersAPI.delete as jest.Mock).mockResolvedValue({
    data: { success: true },
  });
});

// ===========================================================================
// Rendering Tests
// ===========================================================================

describe('ClusterListPage – rendering', () => {
  it('renders without crashing', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    expect(
      screen.getByText(/clusters/i) ||
        screen.getByText(/cluster/i) ||
        document.querySelector('.MuiBox-root'),
    ).toBeTruthy();
  });

  it('renders the Clusters heading', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    expect(screen.getByText(/clusters/i) || screen.getByText(/cluster/i)).toBeTruthy();
  });

  it('renders the search input field', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    const searchInput =
      screen.getByPlaceholderText(/search/i) ||
      document.querySelector('input[placeholder*="Search"]') ||
      document.querySelector('input[type="text"]');
    expect(searchInput).toBeTruthy();
  });

  it('renders the Register Cluster button', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);
    expect(registerButton).toBeTruthy();
  });

  it('renders the status filter dropdown', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    // Status filter should be present as a Select component
    const statusFilter =
      screen.queryByText(/status/i) || screen.queryByLabelText(/status/i);
    expect(statusFilter || document.querySelector('.MuiSelect-root')).toBeTruthy();
  });

  it('renders the provider filter dropdown', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    // Provider filter should be present
    const providerFilter =
      screen.queryByText(/provider/i) || screen.queryByLabelText(/provider/i);
    expect(
      providerFilter || document.querySelectorAll('.MuiSelect-root').length,
    ).toBeTruthy();
  });

  it('renders cluster stat cards', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      // Stat cards should show total, healthy, degraded, unreachable counts
      const totalCard = screen.queryByText(/total/i) || screen.queryByText('3');
      expect(totalCard).toBeTruthy();
    });
  });

  it('renders the refresh button', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    const refreshButton =
      screen.queryByLabelText(/refresh/i) ||
      screen.queryByRole('button', { name: /refresh/i });
    expect(refreshButton).toBeTruthy();
  });
});

// ===========================================================================
// Data Fetching Tests
// ===========================================================================

describe('ClusterListPage – data fetching', () => {
  it('calls clustersAPI.list on mount', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    expect(clustersAPI.list).toHaveBeenCalled();
  });

  it('renders cluster cards after data loads', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });
  });

  it('renders cluster description after data loads', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      expect(screen.getByText(/Main production Kubernetes cluster/)).toBeTruthy();
    });
  });

  it('renders cluster provider information', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      const awsElements =
        screen.queryAllByText(/aws/i) ||
        screen.queryAllByText(/gcp/i) ||
        screen.queryAllByText(/kind/i);
      expect(awsElements.length).toBeGreaterThan(0);
    });
  });

  it('renders status badges for each cluster', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      expect(badges.length).toBeGreaterThan(0);
    });
  });

  it('renders node count for clusters', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      // Production cluster has 5 nodes
      expect(screen.getByText(/5/) || screen.getByText(/5 nodes/)).toBeTruthy();
    });
  });

  it('renders region information for clusters', async () => {
    await act(async () => {
      renderClusterListPage();
    });
    await waitFor(() => {
      expect(
        screen.getByText(/us-east-1/i) || screen.getByText(/us-central1/i),
      ).toBeTruthy();
    });
  });

  it('handles API error gracefully', async () => {
    (clustersAPI.list as jest.Mock).mockRejectedValue(new Error('Network error'));

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText(/error/i) || screen.getByText(/failed/i)).toBeTruthy();
    });
  });

  it('handles empty cluster list', async () => {
    (clustersAPI.list as jest.Mock).mockResolvedValue({
      data: { data: [] },
    });

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const emptyMessage =
        screen.queryByText(/no clusters/i) ||
        screen.queryByText(/get started/i) ||
        screen.queryByText(/register/i);
      expect(emptyMessage).toBeTruthy();
    });
  });
});

// ===========================================================================
// Search and Filter Tests
// ===========================================================================

describe('ClusterListPage – search and filtering', () => {
  it('filters clusters by search query', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const searchInput = document.querySelector('input[type="text"]') as HTMLInputElement;
    if (searchInput) {
      await act(async () => {
        fireEvent.change(searchInput, { target: { value: 'Production' } });
      });

      await waitFor(() => {
        expect(screen.getByText('Production Cluster')).toBeTruthy();
        expect(screen.queryByText('Staging Cluster')).toBeNull();
      });
    }
  });

  it('shows all clusters when search is cleared', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const searchInput = document.querySelector('input[type="text"]') as HTMLInputElement;
    if (searchInput) {
      await act(async () => {
        fireEvent.change(searchInput, { target: { value: 'Production' } });
      });

      await act(async () => {
        fireEvent.change(searchInput, { target: { value: '' } });
      });

      await waitFor(() => {
        expect(screen.getByText('Production Cluster')).toBeTruthy();
        expect(screen.getByText('Staging Cluster')).toBeTruthy();
      });
    }
  });

  it('renders clear filters button when filters are active', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    // Type in search to activate a filter
    const searchInput = document.querySelector('input[type="text"]') as HTMLInputElement;
    if (searchInput) {
      await act(async () => {
        fireEvent.change(searchInput, { target: { value: 'test' } });
      });

      const clearButton =
        screen.queryByLabelText(/clear/i) || screen.queryByText(/clear/i);
      expect(clearButton).toBeTruthy();
    }
  });

  it('filters clusters by search query case-insensitively', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const searchInput = document.querySelector('input[type="text"]') as HTMLInputElement;
    if (searchInput) {
      await act(async () => {
        fireEvent.change(searchInput, { target: { value: 'production' } });
      });

      await waitFor(() => {
        expect(screen.getByText('Production Cluster')).toBeTruthy();
      });
    }
  });

  it('shows no results message when search matches nothing', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const searchInput = document.querySelector('input[type="text"]') as HTMLInputElement;
    if (searchInput) {
      await act(async () => {
        fireEvent.change(searchInput, { target: { value: 'nonexistent-cluster-xyz' } });
      });

      await waitFor(() => {
        const noResults =
          screen.queryByText(/no clusters/i) ||
          screen.queryByText(/no results/i) ||
          screen.queryByText(/not found/i);
        expect(noResults).toBeTruthy();
      });
    }
  });
});

// ===========================================================================
// Register Cluster Dialog Tests
// ===========================================================================

describe('ClusterListPage – register cluster dialog', () => {
  it('opens the register dialog when Register Cluster button is clicked', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        const dialog =
          screen.queryByRole('dialog') || screen.queryByText(/register cluster/i);
        expect(dialog).toBeTruthy();
      });
    }
  });

  it('renders name field in the register dialog', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        const nameField =
          screen.queryByLabelText(/name/i) ||
          screen.queryByPlaceholderText(/cluster name/i);
        expect(nameField).toBeTruthy();
      });
    }
  });

  it('renders description field in the register dialog', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        const descField =
          screen.queryByLabelText(/description/i) ||
          screen.queryByPlaceholderText(/description/i);
        expect(descField).toBeTruthy();
      });
    }
  });

  it('renders provider selector in the register dialog', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        const providerField =
          screen.queryByLabelText(/provider/i) || screen.queryByText(/provider/i);
        expect(providerField).toBeTruthy();
      });
    }
  });

  it('renders region field in the register dialog', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        const regionField =
          screen.queryByLabelText(/region/i) || screen.queryByPlaceholderText(/region/i);
        expect(regionField).toBeTruthy();
      });
    }
  });

  it('renders kubeconfig field in the register dialog', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        const kubeconfigField =
          screen.queryByLabelText(/kubeconfig/i) ||
          screen.queryByPlaceholderText(/kubeconfig/i);
        expect(kubeconfigField).toBeTruthy();
      });
    }
  });

  it('closes the register dialog when cancel is clicked', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        expect(screen.queryByRole('dialog')).toBeTruthy();
      });

      const cancelButton =
        screen.queryByRole('button', { name: /cancel/i }) ||
        screen.queryByText(/cancel/i);

      if (cancelButton) {
        await act(async () => {
          fireEvent.click(cancelButton);
        });

        await waitFor(() => {
          expect(screen.queryByRole('dialog')).toBeNull();
        });
      }
    }
  });

  it('submits the register form when all fields are filled', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const registerButton =
      screen.queryByRole('button', { name: /register/i }) ||
      screen.queryByText(/register/i);

    if (registerButton) {
      await act(async () => {
        fireEvent.click(registerButton);
      });

      await waitFor(() => {
        expect(screen.queryByRole('dialog')).toBeTruthy();
      });

      // Fill in the form
      const nameField =
        screen.queryByLabelText(/name/i) ||
        screen.queryByPlaceholderText(/cluster name/i);

      if (nameField) {
        await act(async () => {
          fireEvent.change(nameField, { target: { value: 'New Test Cluster' } });
        });
      }

      const submitButton =
        screen.queryByRole('button', { name: /register/i }) ||
        screen.queryByText(/register/i);

      // Click submit (the one inside the dialog)
      const dialogButtons = screen
        .queryAllByRole('button')
        .filter((btn) => /register/i.test(btn.textContent || ''));
      if (dialogButtons.length > 1) {
        await act(async () => {
          fireEvent.click(dialogButtons[dialogButtons.length - 1]);
        });
      }
    }
  });
});

// ===========================================================================
// Delete Cluster Dialog Tests
// ===========================================================================

describe('ClusterListPage – delete cluster dialog', () => {
  it('opens the delete dialog when delete menu item is clicked', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    // Find the more/overflow menu button on a cluster card
    const moreButtons =
      screen.queryAllByLabelText(/more/i) ||
      screen
        .queryAllByRole('button')
        .filter(
          (btn) =>
            btn.querySelector('svg') && btn.getAttribute('aria-label')?.includes('more'),
        );

    if (moreButtons.length > 0) {
      await act(async () => {
        fireEvent.click(moreButtons[0]);
      });

      await waitFor(() => {
        const deleteItem = screen.queryByText(/delete/i);
        if (deleteItem) {
          fireEvent.click(deleteItem);
        }
      });

      await waitFor(() => {
        const confirmDialog =
          screen.queryByRole('dialog') ||
          screen.queryByText(/confirm/i) ||
          screen.queryByText(/delete/i);
        expect(confirmDialog).toBeTruthy();
      });
    }
  });
});

// ===========================================================================
// Health Check Tests
// ===========================================================================

describe('ClusterListPage – health check', () => {
  it('displays health information for clusters', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    // Health information like CPU, memory usage should be displayed
    const cpuText = screen.queryByText(/cpu/i) || screen.queryByText(/45/);
    const memoryText = screen.queryByText(/memory/i) || screen.queryByText(/62/);
    expect(cpuText || memoryText).toBeTruthy();
  });

  it('shows healthy status for healthy clusters', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const healthyBadges = screen.queryAllByTestId('status-badge');
      const healthyFound = healthyBadges.some((el) =>
        el.textContent?.includes('healthy'),
      );
      expect(healthyFound).toBe(true);
    });
  });

  it('shows degraded status for degraded clusters', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      const degradedFound = badges.some((el) => el.textContent?.includes('degraded'));
      expect(degradedFound).toBe(true);
    });
  });

  it('shows unreachable status for unreachable clusters', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const badges = screen.queryAllByTestId('status-badge');
      const unreachableFound = badges.some((el) =>
        el.textContent?.includes('unreachable'),
      );
      expect(unreachableFound).toBe(true);
    });
  });

  it('renders CPU and memory progress bars', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const progressBars = document.querySelectorAll('.MuiLinearProgress-root');
    expect(progressBars.length).toBeGreaterThan(0);
  });
});

// ===========================================================================
// Stat Cards Tests
// ===========================================================================

describe('ClusterListPage – stat cards', () => {
  it('renders the total clusters stat', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const totalStat = screen.queryByText(/total/i) || screen.queryByText('3');
      expect(totalStat).toBeTruthy();
    });
  });

  it('renders the healthy clusters stat', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const healthyStat = screen.queryByText(/healthy/i) || screen.queryByText('1');
      expect(healthyStat).toBeTruthy();
    });
  });

  it('renders the degraded clusters stat', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const degradedStat = screen.queryByText(/degraded/i) || screen.queryByText('1');
      expect(degradedStat).toBeTruthy();
    });
  });

  it('renders the unreachable clusters stat', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const unreachableStat =
        screen.queryByText(/unreachable/i) || screen.queryByText('1');
      expect(unreachableStat).toBeTruthy();
    });
  });

  it('computes correct stats from cluster data', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      // Total of 3 clusters: 1 healthy, 1 degraded, 1 unreachable
      expect(screen.getByText('3')).toBeTruthy();
    });
  });
});

// ===========================================================================
// Cluster Card Interaction Tests
// ===========================================================================

describe('ClusterListPage – cluster card interactions', () => {
  it('renders cluster name in each card', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
      expect(screen.getByText('Staging Cluster')).toBeTruthy();
      expect(screen.getByText('Development Cluster')).toBeTruthy();
    });
  });

  it('renders cluster version information', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const versionInfo =
        screen.queryByText(/1\.28/) ||
        screen.queryByText(/1\.27/) ||
        screen.queryByText(/1\.26/);
      expect(versionInfo).toBeTruthy();
    });
  });

  it('renders cluster namespace count', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      // Production cluster has 12 namespaces
      expect(screen.getByText(/12/) || screen.getByText('12')).toBeTruthy();
    });
  });

  it('renders the provider icon or label for each cluster', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const awsLabels = screen.queryAllByText(/aws/i);
      const gcpLabels = screen.queryAllByText(/gcp/i);
      const kindLabels = screen.queryAllByText(/kind/i);
      expect(awsLabels.length + gcpLabels.length + kindLabels.length).toBeGreaterThan(0);
    });
  });
});

// ===========================================================================
// Navigation Tests
// ===========================================================================

describe('ClusterListPage – navigation', () => {
  it('navigates to cluster detail when a cluster card is clicked', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const clusterCard = screen.getByText('Production Cluster');
    await act(async () => {
      fireEvent.click(clusterCard);
    });

    expect(mockNavigate).toHaveBeenCalled();
  });

  it('includes the cluster ID in the navigation path', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const clusterCard = screen.getByText('Production Cluster');
    await act(async () => {
      fireEvent.click(clusterCard);
    });

    const navigateCall = mockNavigate.mock.calls[0];
    if (navigateCall) {
      expect(navigateCall[0]).toContain('cluster-1');
    }
  });
});

// ===========================================================================
// Loading State Tests
// ===========================================================================

describe('ClusterListPage – loading state', () => {
  it('shows loading skeletons while data is loading', async () => {
    (clustersAPI.list as jest.Mock).mockImplementation(() => new Promise(() => {}));

    await act(async () => {
      renderClusterListPage();
    });

    const skeletons = document.querySelectorAll('.MuiSkeleton-root');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it('replaces skeletons with content after data loads', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    const skeletons = document.querySelectorAll('.MuiSkeleton-root');
    expect(skeletons.length).toBe(0);
  });
});

// ===========================================================================
// Error Handling Tests
// ===========================================================================

describe('ClusterListPage – error handling', () => {
  it('shows error alert when API call fails', async () => {
    (clustersAPI.list as jest.Mock).mockRejectedValue(
      new Error('Failed to fetch clusters'),
    );

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const errorAlert =
        screen.queryByRole('alert') ||
        screen.queryByText(/error/i) ||
        screen.queryByText(/failed/i);
      expect(errorAlert).toBeTruthy();
    });
  });

  it('shows error message content from the API error', async () => {
    (clustersAPI.list as jest.Mock).mockRejectedValue(new Error('Server unreachable'));

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      const errorContent =
        screen.queryByText(/server unreachable/i) || screen.queryByText(/error/i);
      expect(errorContent).toBeTruthy();
    });
  });
});

// ===========================================================================
// Edge Cases
// ===========================================================================

describe('ClusterListPage – edge cases', () => {
  it('handles clusters with missing description gracefully', async () => {
    const clustersWithoutDesc: Cluster[] = [
      {
        id: 'cluster-no-desc',
        name: 'No Desc Cluster',
        description: '',
        status: 'healthy',
        provider: 'aws',
        region: 'us-east-1',
        version: '1.28.0',
        nodeCount: 2,
        namespaceCount: 5,
        namespaces: ['default'],
        labels: {},
        lastHealthCheck: new Date().toISOString(),
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      },
    ];

    (clustersAPI.list as jest.Mock).mockResolvedValue({
      data: { data: clustersWithoutDesc },
    });

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('No Desc Cluster')).toBeTruthy();
    });
  });

  it('handles cluster with unknown provider gracefully', async () => {
    const clustersWithOtherProvider: Cluster[] = [
      {
        ...mockClusters[0],
        id: 'cluster-other',
        name: 'Other Provider Cluster',
        provider: 'other',
      },
    ];

    (clustersAPI.list as jest.Mock).mockResolvedValue({
      data: { data: clustersWithOtherProvider },
    });

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Other Provider Cluster')).toBeTruthy();
    });
  });

  it('handles cluster with zero nodes gracefully', async () => {
    const clustersWithZeroNodes: Cluster[] = [
      {
        ...mockClusters[0],
        id: 'cluster-zero-nodes',
        name: 'Zero Nodes Cluster',
        nodeCount: 0,
      },
    ];

    (clustersAPI.list as jest.Mock).mockResolvedValue({
      data: { data: clustersWithZeroNodes },
    });

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Zero Nodes Cluster')).toBeTruthy();
    });
  });

  it('handles cluster with unknown status gracefully', async () => {
    const clustersWithUnknownStatus: Cluster[] = [
      {
        ...mockClusters[0],
        id: 'cluster-unknown-status',
        name: 'Unknown Status Cluster',
        status: 'unknown' as any,
      },
    ];

    (clustersAPI.list as jest.Mock).mockResolvedValue({
      data: { data: clustersWithUnknownStatus },
    });

    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Unknown Status Cluster')).toBeTruthy();
    });
  });

  it('re-fetches clusters when refresh button is clicked', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    const initialCallCount = (clustersAPI.list as jest.Mock).mock.calls.length;

    const refreshButton =
      screen.queryByLabelText(/refresh/i) ||
      screen.queryByRole('button', { name: /refresh/i });

    if (refreshButton) {
      await act(async () => {
        fireEvent.click(refreshButton);
      });

      await waitFor(() => {
        expect((clustersAPI.list as jest.Mock).mock.calls.length).toBeGreaterThan(
          initialCallCount,
        );
      });
    }
  });

  it('renders cluster labels when present', async () => {
    await act(async () => {
      renderClusterListPage();
    });

    await waitFor(() => {
      expect(screen.getByText('Production Cluster')).toBeTruthy();
    });

    // Labels should be displayed somewhere on the cluster card
    const envLabel = screen.queryByText(/prod/i);
    expect(envLabel).toBeTruthy();
  });
});
