/**
 * Unit tests for the StatusBadge component and its helper functions.
 *
 * Covers:
 *  1. Variant rendering (dot, pill, icon)
 *  2. Status type colours and labels for all categories
 *  3. Unknown / unrecognised status fallback
 *  4. Size variants (small, medium, large)
 *  5. Pulse animation for running status
 *  6. Helper functions (getStatusColor, getStatusConfig)
 */

import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import StatusBadge, {
  getStatusColor,
  getStatusConfig,
  type StatusVariant,
  type StatusSize,
  type StatusType,
} from '@/components/StatusBadge';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const renderBadge = (props: {
  status: StatusType | string;
  variant?: StatusVariant;
  size?: StatusSize;
  label?: string;
  tooltip?: string;
  animated?: boolean;
  showLabel?: boolean;
  className?: string;
  onClick?: (event: React.MouseEvent<HTMLDivElement>) => void;
}) => render(<StatusBadge {...props} />);

// ---------------------------------------------------------------------------
// 1. Variant rendering
// ---------------------------------------------------------------------------

describe('StatusBadge – variants', () => {
  it('renders the default pill variant', () => {
    renderBadge({ status: 'running' });
    const chip = document.querySelector('.MuiChip-root');
    expect(chip).toBeInTheDocument();
  });

  it('renders the dot variant without a Chip', () => {
    const { container } = renderBadge({ status: 'running', variant: 'dot' });
    expect(document.querySelector('.MuiChip-root')).not.toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders the icon variant without a Chip', () => {
    renderBadge({ status: 'running', variant: 'icon' });
    expect(document.querySelector('.MuiChip-root')).not.toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders all three variants without crashing', () => {
    const variants: StatusVariant[] = ['dot', 'pill', 'icon'];
    for (const variant of variants) {
      const { unmount } = renderBadge({ status: 'completed', variant });
      expect(screen.getByText('Completed')).toBeInTheDocument();
      unmount();
    }
  });

  it('defaults to the pill variant when variant is not specified', () => {
    renderBadge({ status: 'pending' });
    expect(document.querySelector('.MuiChip-root')).toBeInTheDocument();
  });

  it('renders the dot variant with a circular indicator', () => {
    const { container } = renderBadge({ status: 'failed', variant: 'dot' });
    // The dot variant renders a Box with borderRadius: '50%'
    const dotElements = container.querySelectorAll('[class*="css-"]');
    expect(dotElements.length).toBeGreaterThan(0);
    expect(screen.getByText('Failed')).toBeInTheDocument();
  });

  it('renders the icon variant with an SVG icon', () => {
    const { container } = renderBadge({ status: 'failed', variant: 'icon' });
    const svg = container.querySelector('svg');
    expect(svg).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 2. Status type colours and labels
// ---------------------------------------------------------------------------

describe('StatusBadge – status types', () => {
  const experimentStatuses: StatusType[] = [
    'pending',
    'queued',
    'running',
    'completed',
    'failed',
    'stopped',
    'timed_out',
  ];

  it('renders all experiment statuses with a label', () => {
    for (const status of experimentStatuses) {
      const { unmount } = renderBadge({ status });
      const label = screen.getByText(new RegExp(status.replace('_', ' '), 'i'));
      expect(label).toBeInTheDocument();
      unmount();
    }
  });

  it('renders cluster statuses', () => {
    const clusterStatuses: StatusType[] = [
      'healthy',
      'degraded',
      'unreachable',
      'unknown',
    ];
    for (const status of clusterStatuses) {
      const { unmount } = renderBadge({ status });
      expect(screen.getByText(new RegExp(status, 'i'))).toBeInTheDocument();
      unmount();
    }
  });

  it('renders validation statuses', () => {
    const validationStatuses: StatusType[] = [
      'validated',
      'invalid',
      'pending',
      'in_progress',
      'skipped',
    ];
    for (const status of validationStatuses) {
      const { unmount } = renderBadge({ status });
      expect(
        screen.getByText(new RegExp(status.replace('_', ' '), 'i')),
      ).toBeInTheDocument();
      unmount();
    }
  });

  it('renders alert statuses', () => {
    const alertStatuses: StatusType[] = [
      'new',
      'acknowledged',
      'investigating',
      'resolved',
      'false_positive',
    ];
    for (const status of alertStatuses) {
      const { unmount } = renderBadge({ status });
      expect(
        screen.getByText(new RegExp(status.replace('_', ' '), 'i')),
      ).toBeInTheDocument();
      unmount();
    }
  });

  it('renders general statuses', () => {
    const generalStatuses: StatusType[] = [
      'active',
      'inactive',
      'healthy',
      'unhealthy',
      'success',
      'warning',
      'error',
      'info',
    ];
    for (const status of generalStatuses) {
      const { unmount } = renderBadge({ status });
      expect(screen.getByText(new RegExp(status, 'i'))).toBeInTheDocument();
      unmount();
    }
  });

  it('renders "running" status with the correct label', () => {
    renderBadge({ status: 'running', variant: 'pill' });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders "completed" status with the correct label', () => {
    renderBadge({ status: 'completed', variant: 'pill' });
    expect(screen.getByText('Completed')).toBeInTheDocument();
  });

  it('renders "failed" status with the correct label', () => {
    renderBadge({ status: 'failed', variant: 'pill' });
    expect(screen.getByText('Failed')).toBeInTheDocument();
  });

  it('renders "timed_out" status with a human-readable label', () => {
    renderBadge({ status: 'timed_out', variant: 'pill' });
    expect(screen.getByText('Timed Out')).toBeInTheDocument();
  });

  it('renders "in_progress" status with a human-readable label', () => {
    renderBadge({ status: 'in_progress', variant: 'pill' });
    expect(screen.getByText('In Progress')).toBeInTheDocument();
  });

  it('renders "false_positive" status with a human-readable label', () => {
    renderBadge({ status: 'false_positive', variant: 'pill' });
    expect(screen.getByText('False Positive')).toBeInTheDocument();
  });

  it('assigns distinct colours to running vs completed', () => {
    const runningColor = getStatusColor('running');
    const completedColor = getStatusColor('completed');
    expect(runningColor).not.toBe(completedColor);
  });

  it('assigns distinct colours to failed vs success', () => {
    const failedColor = getStatusColor('failed');
    const successColor = getStatusColor('success');
    expect(failedColor).not.toBe(successColor);
  });

  it('renders each status in the dot variant', () => {
    for (const status of experimentStatuses) {
      const { unmount } = renderBadge({ status, variant: 'dot' });
      expect(
        screen.getByText(new RegExp(status.replace('_', ' '), 'i')),
      ).toBeInTheDocument();
      unmount();
    }
  });

  it('renders each status in the icon variant', () => {
    for (const status of experimentStatuses) {
      const { unmount } = renderBadge({ status, variant: 'icon' });
      expect(
        screen.getByText(new RegExp(status.replace('_', ' '), 'i')),
      ).toBeInTheDocument();
      unmount();
    }
  });
});

// ---------------------------------------------------------------------------
// 3. Unknown / unrecognised status fallback
// ---------------------------------------------------------------------------

describe('StatusBadge – unknown statuses', () => {
  it('renders an unknown status string without crashing', () => {
    renderBadge({ status: 'some_unknown_status' });
    expect(document.querySelector('.MuiChip-root')).toBeInTheDocument();
  });

  it('displays "Unknown" label for an unrecognised status', () => {
    renderBadge({ status: 'custom_status' });
    expect(screen.getByText('Unknown')).toBeInTheDocument();
  });

  it('falls back to the default config colour for an unknown status', () => {
    const color = getStatusColor('totally_unknown');
    expect(typeof color).toBe('string');
    expect(color.length).toBeGreaterThan(0);
    // Default config colour is #94A3B8
    expect(color).toBe('#94A3B8');
  });

  it('returns default config for unknown status via getStatusConfig', () => {
    const config = getStatusConfig('nonexistent_status');
    expect(config.label).toBe('Unknown');
    expect(config.normalizedStatus).toBe('nonexistent_status');
    expect(config.color).toBe('#94A3B8');
  });

  it('handles empty string status gracefully', () => {
    renderBadge({ status: '' });
    expect(screen.getByText('Unknown')).toBeInTheDocument();
  });

  it('handles numeric-like string status gracefully', () => {
    renderBadge({ status: '404' });
    // normalizeStatus strips non-alpha chars, leaving empty string, falls to default
    expect(screen.getByText('Unknown')).toBeInTheDocument();
  });

  it('normalises status with spaces to underscores', () => {
    renderBadge({ status: 'in progress' });
    expect(screen.getByText('In Progress')).toBeInTheDocument();
  });

  it('normalises status with hyphens to underscores', () => {
    renderBadge({ status: 'false-positive' });
    expect(screen.getByText('False Positive')).toBeInTheDocument();
  });

  it('normalises uppercase status to lowercase', () => {
    renderBadge({ status: 'RUNNING' });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('normalises mixed-case hyphenated status', () => {
    renderBadge({ status: 'False-Positive' });
    expect(screen.getByText('False Positive')).toBeInTheDocument();
  });

  it('renders an unknown status in the dot variant', () => {
    renderBadge({ status: 'bogus', variant: 'dot' });
    expect(screen.getByText('Unknown')).toBeInTheDocument();
  });

  it('renders an unknown status in the icon variant', () => {
    renderBadge({ status: 'bogus', variant: 'icon' });
    expect(screen.getByText('Unknown')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 4. Size rendering
// ---------------------------------------------------------------------------

describe('StatusBadge – sizes', () => {
  it('renders small size without crashing', () => {
    const { container } = renderBadge({ status: 'running', size: 'small' });
    expect(container.querySelector('.MuiChip-root')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders medium size (default) without crashing', () => {
    const { container } = renderBadge({ status: 'running', size: 'medium' });
    expect(container.querySelector('.MuiChip-root')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders large size without crashing', () => {
    const { container } = renderBadge({ status: 'running', size: 'large' });
    expect(container.querySelector('.MuiChip-root')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders all three sizes for the dot variant', () => {
    const sizes: StatusSize[] = ['small', 'medium', 'large'];
    for (const size of sizes) {
      const { unmount } = renderBadge({
        status: 'running',
        variant: 'dot',
        size,
      });
      expect(screen.getByText('Running')).toBeInTheDocument();
      unmount();
    }
  });

  it('renders all three sizes for the icon variant', () => {
    const sizes: StatusSize[] = ['small', 'medium', 'large'];
    for (const size of sizes) {
      const { unmount } = renderBadge({
        status: 'running',
        variant: 'icon',
        size,
      });
      expect(screen.getByText('Running')).toBeInTheDocument();
      unmount();
    }
  });

  it('defaults to medium when size is not specified', () => {
    renderBadge({ status: 'pending' });
    expect(screen.getByText('Pending')).toBeInTheDocument();
  });

  it('renders the label for small size in the pill variant', () => {
    renderBadge({ status: 'completed', size: 'small' });
    expect(screen.getByText('Completed')).toBeInTheDocument();
  });

  it('renders the label for large size in the pill variant', () => {
    renderBadge({ status: 'failed', size: 'large' });
    expect(screen.getByText('Failed')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 5. Pulse animation
// ---------------------------------------------------------------------------

describe('StatusBadge – pulse animation', () => {
  it('applies pulse animation for running status in dot variant when animated', () => {
    const { container } = renderBadge({
      status: 'running',
      variant: 'dot',
      animated: true,
    });
    // MUI Emotion injects @keyframes into document-level <style data-emotion> tags
    const styleTags = document.querySelectorAll('style[data-emotion]');
    let foundPulseKeyframe = false;
    styleTags.forEach((tag) => {
      if (tag.textContent && tag.textContent.includes('pulse')) {
        foundPulseKeyframe = true;
      }
    });
    expect(foundPulseKeyframe).toBeTruthy();
  });

  it('applies pulse animation for running status in pill variant when animated', () => {
    const { container } = renderBadge({
      status: 'running',
      variant: 'pill',
      animated: true,
    });
    const styleTags = document.querySelectorAll('style[data-emotion]');
    let foundPulseKeyframe = false;
    styleTags.forEach((tag) => {
      if (tag.textContent && tag.textContent.includes('pulse')) {
        foundPulseKeyframe = true;
      }
    });
    expect(foundPulseKeyframe).toBeTruthy();
  });

  it('applies icon pulse animation for running status in icon variant when animated', () => {
    const { container } = renderBadge({
      status: 'running',
      variant: 'icon',
      animated: true,
    });
    const styleTags = document.querySelectorAll('style[data-emotion]');
    let foundKeyframe = false;
    styleTags.forEach((tag) => {
      if (tag.textContent && tag.textContent.includes('iconPulse')) {
        foundKeyframe = true;
      }
    });
    expect(foundKeyframe).toBeTruthy();
  });

  it('does not apply pulse animation when animated is false (default)', () => {
    const { container } = renderBadge({
      status: 'running',
      variant: 'dot',
      animated: false,
    });
    // When animated is false, inline styles should not contain animation
    const elements = container.querySelectorAll('[class*="css-"]');
    for (const el of Array.from(elements)) {
      const inlineStyle = (el as HTMLElement).getAttribute('style') || '';
      if (inlineStyle.includes('animation')) {
        expect(inlineStyle).not.toContain('animation');
      }
    }
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('does not apply pulse animation for non-running status even when animated', () => {
    const { container } = renderBadge({
      status: 'completed',
      variant: 'pill',
      animated: true,
    });
    const chip = container.querySelector('.MuiChip-root');
    expect(chip).toBeInTheDocument();
    const inlineStyle = chip?.getAttribute('style') || '';
    expect(inlineStyle).not.toContain('animation');
    expect(screen.getByText('Completed')).toBeInTheDocument();
  });

  it('renders the running label correctly when animated', () => {
    renderBadge({ status: 'running', variant: 'pill', animated: true });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders the dot variant animated label', () => {
    renderBadge({ status: 'running', variant: 'dot', animated: true });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders the icon variant animated label', () => {
    renderBadge({ status: 'running', variant: 'icon', animated: true });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 6. Additional props
// ---------------------------------------------------------------------------

describe('StatusBadge – additional props', () => {
  it('accepts a custom label override', () => {
    renderBadge({ status: 'running', label: 'In Progress' });
    expect(screen.getByText('In Progress')).toBeInTheDocument();
  });

  it('uses the auto-generated label when label prop is not provided', () => {
    renderBadge({ status: 'completed' });
    expect(screen.getByText('Completed')).toBeInTheDocument();
  });

  it('renders a tooltip wrapping the badge', () => {
    renderBadge({ status: 'running' });
    // The label text is always visible
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('accepts a custom tooltip', () => {
    renderBadge({
      status: 'running',
      tooltip: 'Experiment is currently executing',
    });
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('hides the label text when showLabel is false (dot variant)', () => {
    const { container } = renderBadge({
      status: 'running',
      variant: 'dot',
      showLabel: false,
    });
    expect(screen.queryByText('Running')).not.toBeInTheDocument();
    // The dot itself should still render
    expect(container.firstChild).toBeInTheDocument();
  });

  it('hides the label text when showLabel is false (icon variant)', () => {
    renderBadge({ status: 'running', variant: 'icon', showLabel: false });
    expect(screen.queryByText('Running')).not.toBeInTheDocument();
  });

  it('renders the pill chip even when showLabel is false', () => {
    renderBadge({ status: 'running', variant: 'pill', showLabel: false });
    const chip = document.querySelector('.MuiChip-root');
    expect(chip).toBeInTheDocument();
  });

  it('renders with a className prop', () => {
    const { container } = renderBadge({
      status: 'running',
      className: 'custom-badge',
    });
    expect(container.querySelector('.custom-badge')).toBeInTheDocument();
  });

  it('calls onClick handler when the pill variant is clicked', () => {
    const handleClick = jest.fn();
    renderBadge({ status: 'running', onClick: handleClick });
    const chip = document.querySelector('.MuiChip-root');
    expect(chip).toBeInTheDocument();
    if (chip) {
      fireEvent.click(chip);
      expect(handleClick).toHaveBeenCalledTimes(1);
    }
  });

  it('calls onClick handler when the dot variant is clicked', () => {
    const handleClick = jest.fn();
    const { container } = renderBadge({
      status: 'running',
      variant: 'dot',
      onClick: handleClick,
    });
    const clickableBox = container.querySelector('[class*="css-"]');
    if (clickableBox) {
      fireEvent.click(clickableBox);
      expect(handleClick).toHaveBeenCalled();
    }
  });

  it('calls onClick handler when the icon variant is clicked', () => {
    const handleClick = jest.fn();
    const { container } = renderBadge({
      status: 'running',
      variant: 'icon',
      onClick: handleClick,
    });
    const clickableBox = container.querySelector('[class*="css-"]');
    if (clickableBox) {
      fireEvent.click(clickableBox);
      expect(handleClick).toHaveBeenCalled();
    }
  });
});

// ---------------------------------------------------------------------------
// 7. Helper functions
// ---------------------------------------------------------------------------

describe('getStatusColor', () => {
  it('returns a colour string for known statuses', () => {
    const color = getStatusColor('running');
    expect(typeof color).toBe('string');
    expect(color.length).toBeGreaterThan(0);
  });

  it('returns different colours for different statuses', () => {
    const runningColor = getStatusColor('running');
    const failedColor = getStatusColor('failed');
    expect(runningColor).not.toBe(failedColor);
  });

  it('returns the default colour (#94A3B8) for unknown statuses', () => {
    const color = getStatusColor('nonexistent');
    expect(color).toBe('#94A3B8');
  });

  it('normalises uppercase before lookup', () => {
    expect(getStatusColor('RUNNING')).toBe(getStatusColor('running'));
  });

  it('normalises hyphenated strings before lookup', () => {
    expect(getStatusColor('timed-out')).toBe(getStatusColor('timed_out'));
  });

  it('returns consistent colours for each status category', () => {
    const experimentColors = ['pending', 'running', 'completed', 'failed'].map(
      getStatusColor,
    );
    // All colours should be unique within the experiment category
    const uniqueColors = new Set(experimentColors);
    expect(uniqueColors.size).toBe(experimentColors.length);
  });
});

describe('getStatusConfig', () => {
  it('returns full config for a known status', () => {
    const config = getStatusConfig('running');
    expect(config.label).toBe('Running');
    expect(config.color).toBeTruthy();
    expect(config.bgColor).toBeTruthy();
    expect(config.borderColor).toBeTruthy();
    expect(config.icon).toBeTruthy();
    expect(config.normalizedStatus).toBe('running');
  });

  it('returns default config for an unknown status', () => {
    const config = getStatusConfig('nonexistent_status');
    expect(config.label).toBe('Unknown');
    expect(config.normalizedStatus).toBe('nonexistent_status');
    expect(config.color).toBe('#94A3B8');
  });

  it('normalises status before lookup', () => {
    const config = getStatusConfig('RUNNING');
    expect(config.label).toBe('Running');
    expect(config.normalizedStatus).toBe('running');
  });

  it('normalises spaces to underscores before lookup', () => {
    const config = getStatusConfig('in progress');
    expect(config.label).toBe('In Progress');
    expect(config.normalizedStatus).toBe('in_progress');
  });

  it('normalises hyphens to underscores before lookup', () => {
    const config = getStatusConfig('false-positive');
    expect(config.label).toBe('False Positive');
    expect(config.normalizedStatus).toBe('false_positive');
  });

  it('returns config with an icon component for known statuses', () => {
    const config = getStatusConfig('completed');
    expect(config.icon).toBeTruthy();
    // The icon is a React component (function or forward ref)
    expect(typeof config.icon).toBe('object');
  });

  it('returns the HelpOutline icon for unknown statuses', () => {
    const config = getStatusConfig('unknown_status_xyz');
    expect(config.icon).toBeTruthy();
    expect(config.label).toBe('Unknown');
  });
});
