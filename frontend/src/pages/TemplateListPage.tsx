import {
  Search as SearchIcon,
  Add as AddIcon,
  Clear as ClearIcon,
  Science as TemplateIcon,
  Security as SecurityIcon,
  NetworkCheck as NetworkIcon,
  Apps as AppIcon,
  Storage as InfraIcon,
  Dataset as DataIcon,
  Person as IdentityIcon,
  Build as CustomIcon,
  Verified as VerifiedIcon,
  TrendingUp as TrendingUpIcon,
  Download as DownloadIcon,
  GridView as GridViewIcon,
  ViewList as ViewListViewIcon,
} from '@mui/icons-material';
import {
  Box,
  Typography,
  Card,
  CardContent,
  CardActions,
  Grid,
  Stack,
  Button,
  TextField,
  InputAdornment,
  IconButton,
  Chip,
  Tooltip,
  Skeleton,
  Paper,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Tabs,
  Tab,
  Divider,
  Avatar,
  Badge,
  Alert,
  type SelectChangeEvent,
} from '@mui/material';
import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { templatesAPI, getErrorMessage } from '@/services/api';
import type {
  AttackTemplate,
  TemplateCategory,
  TemplateSeverity,
  TemplateParameter,
} from '@/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const SKELETON_TEMPLATE_IDS = ['st-1', 'st-2', 'st-3', 'st-4', 'st-5', 'st-6'];

const CATEGORY_CONFIG: Record<
  TemplateCategory,
  { label: string; icon: React.ReactElement; color: string }
> = {
  network: {
    label: 'Network',
    icon: <NetworkIcon sx={{ fontSize: 20 }} />,
    color: '#2563EB',
  },
  application: {
    label: 'Application',
    icon: <AppIcon sx={{ fontSize: 20 }} />,
    color: '#7C3AED',
  },
  infrastructure: {
    label: 'Infrastructure',
    icon: <InfraIcon sx={{ fontSize: 20 }} />,
    color: '#F59E0B',
  },
  data: {
    label: 'Data',
    icon: <DataIcon sx={{ fontSize: 20 }} />,
    color: '#10B981',
  },
  identity: {
    label: 'Identity',
    icon: <IdentityIcon sx={{ fontSize: 20 }} />,
    color: '#EF4444',
  },
  custom: {
    label: 'Custom',
    icon: <CustomIcon sx={{ fontSize: 20 }} />,
    color: '#64748B',
  },
};

const SEVERITY_ORDER: Record<TemplateSeverity, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
};

const SEVERITY_CONFIG: Record<
  TemplateSeverity,
  { label: string; color: string; bgColor: string }
> = {
  critical: {
    label: 'Critical',
    color: '#DC2626',
    bgColor: 'rgba(220, 38, 38, 0.08)',
  },
  high: {
    label: 'High',
    color: '#EF4444',
    bgColor: 'rgba(239, 68, 68, 0.08)',
  },
  medium: {
    label: 'Medium',
    color: '#F59E0B',
    bgColor: 'rgba(245, 158, 11, 0.08)',
  },
  low: {
    label: 'Low',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.08)',
  },
};

const SORT_OPTIONS = [
  { value: 'popular', label: 'Most Popular' },
  { value: 'newest', label: 'Newest' },
  { value: 'name-asc', label: 'Name (A-Z)' },
  { value: 'name-desc', label: 'Name (Z-A)' },
  { value: 'severity-desc', label: 'Severity (High → Low)' },
  { value: 'severity-asc', label: 'Severity (Low → High)' },
];

// ---------------------------------------------------------------------------
// Mock Data
// ---------------------------------------------------------------------------

const MOCK_TEMPLATES: AttackTemplate[] = [
  {
    id: 'tmpl-1',
    name: 'DNS Exfiltration Test',
    description:
      'Simulates DNS tunneling data exfiltration to validate detection of unusual DNS query patterns, high-volume DNS traffic, and encoded subdomain queries.',
    category: 'network',
    severity: 'high',
    icon: 'dns',
    version: '2.1.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'domain',
        label: 'Target Domain',
        type: 'string',
        defaultValue: 'exfil.test.local',
        required: true,
        description: 'The domain used for DNS exfiltration simulation',
      },
      {
        key: 'queryRate',
        label: 'Query Rate (qps)',
        type: 'number',
        defaultValue: 100,
        required: true,
        description: 'Number of DNS queries per second',
        validation: { min: 1, max: 10000 },
      },
      {
        key: 'encodingMethod',
        label: 'Encoding Method',
        type: 'select',
        defaultValue: 'base64',
        required: true,
        description: 'Encoding method for exfiltrated data',
        options: [
          { label: 'Base64', value: 'base64' },
          { label: 'Hex', value: 'hex' },
          { label: 'Custom', value: 'custom' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Setup DNS Tunnel',
        description: 'Establish DNS tunnel infrastructure',
        technique: 'T1071.004',
        tactic: 'Command and Control',
        duration: 30,
      },
      {
        name: 'Data Exfiltration',
        description: 'Encode and exfiltrate data via DNS queries',
        technique: 'T1048.001',
        tactic: 'Exfiltration',
        duration: 120,
      },
    ],
    expectedDetections: [
      {
        source: 'SIEM',
        type: 'dns_anomaly',
        description: 'High volume of DNS queries to unusual domains',
        confidence: 0.9,
      },
      {
        source: 'IDS',
        type: 'dns_tunnel',
        description: 'DNS tunneling signature detected',
        confidence: 0.85,
      },
    ],
    tags: ['dns', 'exfiltration', 'network', 'mitre-att&ck'],
    isOfficial: true,
    usageCount: 1247,
    createdAt: '2024-01-15T10:00:00Z',
    updatedAt: '2024-11-20T14:30:00Z',
  },
  {
    id: 'tmpl-2',
    name: 'Brute Force Attack',
    description:
      'Simulates SSH and HTTP brute force login attempts to validate account lockout mechanisms, rate limiting, and intrusion detection alerting.',
    category: 'identity',
    severity: 'critical',
    icon: 'lock',
    version: '1.5.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'targetService',
        label: 'Target Service',
        type: 'select',
        defaultValue: 'ssh',
        required: true,
        description: 'Service to target for brute force',
        options: [
          { label: 'SSH', value: 'ssh' },
          { label: 'HTTP', value: 'http' },
          { label: 'RDP', value: 'rdp' },
        ],
      },
      {
        key: 'attemptRate',
        label: 'Attempts per Second',
        type: 'number',
        defaultValue: 10,
        required: true,
        description: 'Number of login attempts per second',
        validation: { min: 1, max: 100 },
      },
    ],
    attackPhases: [
      {
        name: 'Credential Stuffing',
        description: 'Attempt login with common credential lists',
        technique: 'T1110.004',
        tactic: 'Credential Access',
        duration: 60,
      },
    ],
    expectedDetections: [
      {
        source: 'SIEM',
        type: 'brute_force',
        description: 'Multiple failed login attempts detected',
        confidence: 0.95,
      },
    ],
    tags: ['brute-force', 'identity', 'ssh', 'mitre-att&ck'],
    isOfficial: true,
    usageCount: 2156,
    createdAt: '2024-02-10T08:00:00Z',
    updatedAt: '2024-10-15T11:00:00Z',
  },
  {
    id: 'tmpl-3',
    name: 'Pod Kill Chaos',
    description:
      'Randomly kills Kubernetes pods to validate pod auto-healing, PDB compliance, and alerting on unexpected pod terminations.',
    category: 'infrastructure',
    severity: 'medium',
    icon: 'kubernetes',
    version: '3.0.2',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'killCount',
        label: 'Pods to Kill',
        type: 'number',
        defaultValue: 1,
        required: true,
        description: 'Number of pods to kill per iteration',
        validation: { min: 1, max: 10 },
      },
      {
        key: 'interval',
        label: 'Interval (seconds)',
        type: 'number',
        defaultValue: 30,
        required: true,
        description: 'Time between kill iterations',
        validation: { min: 10, max: 600 },
      },
    ],
    attackPhases: [
      {
        name: 'Pod Termination',
        description: 'Kill target pods in the cluster',
        technique: 'T1489',
        tactic: 'Impact',
        duration: 180,
      },
    ],
    expectedDetections: [
      {
        source: 'Monitoring',
        type: 'pod_restart',
        description: 'Unexpected pod restart detected',
        confidence: 0.8,
      },
    ],
    tags: ['kubernetes', 'chaos', 'pod-kill', 'resilience'],
    isOfficial: true,
    usageCount: 856,
    createdAt: '2024-03-01T12:00:00Z',
    updatedAt: '2024-09-28T16:00:00Z',
  },
  {
    id: 'tmpl-4',
    name: 'Network Partition',
    description:
      'Simulates network partition between Kubernetes nodes or namespaces to validate service mesh resilience, failover, and monitoring detection.',
    category: 'network',
    severity: 'high',
    icon: 'network_partition',
    version: '1.2.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'partitionDirection',
        label: 'Partition Direction',
        type: 'select',
        defaultValue: 'both',
        required: true,
        description: 'Direction of network partition',
        options: [
          { label: 'Ingress', value: 'ingress' },
          { label: 'Egress', value: 'egress' },
          { label: 'Both', value: 'both' },
        ],
      },
      {
        key: 'duration',
        label: 'Duration (seconds)',
        type: 'number',
        defaultValue: 60,
        required: true,
        description: 'How long the partition lasts',
        validation: { min: 10, max: 600 },
      },
    ],
    attackPhases: [
      {
        name: 'Partition Injection',
        description: 'Inject network partition between targets',
        technique: 'T1498',
        tactic: 'Impact',
        duration: 90,
      },
    ],
    expectedDetections: [
      {
        source: 'Monitoring',
        type: 'connectivity_loss',
        description: 'Service connectivity loss detected',
        confidence: 0.85,
      },
    ],
    tags: ['network', 'partition', 'resilience', 'service-mesh'],
    isOfficial: true,
    usageCount: 634,
    createdAt: '2024-04-05T09:00:00Z',
    updatedAt: '2024-08-12T10:00:00Z',
  },
  {
    id: 'tmpl-5',
    name: 'SQL Injection Test',
    description:
      'Simulates SQL injection attacks against web application endpoints to validate WAF rules, input validation, and database monitoring.',
    category: 'application',
    severity: 'critical',
    icon: 'sql_injection',
    version: '1.0.3',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'targetEndpoint',
        label: 'Target Endpoint',
        type: 'string',
        defaultValue: '/api/users',
        required: true,
        description: 'The API endpoint to test',
      },
      {
        key: 'injectionType',
        label: 'Injection Type',
        type: 'multi-select',
        defaultValue: ['error-based', 'union-based'],
        required: true,
        description: 'SQL injection techniques to use',
        options: [
          { label: 'Error-Based', value: 'error-based' },
          { label: 'Union-Based', value: 'union-based' },
          { label: 'Blind', value: 'blind' },
          { label: 'Time-Based', value: 'time-based' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Injection Attempt',
        description: 'Execute SQL injection payloads',
        technique: 'T1190',
        tactic: 'Initial Access',
        duration: 45,
      },
    ],
    expectedDetections: [
      {
        source: 'WAF',
        type: 'sqli',
        description: 'SQL injection signature detected',
        confidence: 0.92,
      },
    ],
    tags: ['sql-injection', 'web', 'application', 'owasp'],
    isOfficial: true,
    usageCount: 1834,
    createdAt: '2024-02-20T14:00:00Z',
    updatedAt: '2024-11-05T09:00:00Z',
  },
  {
    id: 'tmpl-6',
    name: 'Data Encryption Ransomware',
    description:
      'Simulates ransomware-style data encryption to validate endpoint detection, file integrity monitoring, and backup alerting systems.',
    category: 'data',
    severity: 'critical',
    icon: 'ransomware',
    version: '1.1.0',
    author: 'Security Research',
    parameters: [
      {
        key: 'targetDirectory',
        label: 'Target Directory',
        type: 'string',
        defaultValue: '/tmp/test-data',
        required: true,
        description: 'Directory containing test files to encrypt',
      },
    ],
    attackPhases: [
      {
        name: 'File Encryption',
        description: 'Encrypt target files with ransomware simulation',
        technique: 'T1486',
        tactic: 'Impact',
        duration: 60,
      },
    ],
    expectedDetections: [
      {
        source: 'EDR',
        type: 'file_encryption',
        description: 'Mass file encryption activity detected',
        confidence: 0.88,
      },
    ],
    tags: ['ransomware', 'data', 'encryption', 'edr'],
    isOfficial: false,
    usageCount: 412,
    createdAt: '2024-06-10T11:00:00Z',
    updatedAt: '2024-10-20T15:00:00Z',
  },
  {
    id: 'tmpl-7',
    name: 'Privilege Escalation via Container Escape',
    description:
      'Simulates container escape techniques to validate container runtime security, namespace isolation, and privilege escalation detection.',
    category: 'infrastructure',
    severity: 'critical',
    icon: 'container_escape',
    version: '2.0.1',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'escapeMethod',
        label: 'Escape Method',
        type: 'select',
        defaultValue: 'docker-socket',
        required: true,
        description: 'Container escape technique',
        options: [
          { label: 'Docker Socket', value: 'docker-socket' },
          { label: 'Kernel Exploit', value: 'kernel-exploit' },
          { label: 'Mount Namespace', value: 'mount-namespace' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Container Breakout',
        description: 'Escape container to host system',
        technique: 'T1611',
        tactic: 'Privilege Escalation',
        duration: 45,
      },
    ],
    expectedDetections: [
      {
        source: 'Runtime Security',
        type: 'container_escape',
        description: 'Container escape attempt detected',
        confidence: 0.9,
      },
    ],
    tags: ['container', 'escape', 'privilege-escalation', 'kubernetes'],
    isOfficial: true,
    usageCount: 945,
    createdAt: '2024-03-20T13:00:00Z',
    updatedAt: '2024-11-10T08:00:00Z',
  },
  {
    id: 'tmpl-8',
    name: 'API Rate Limit Bypass',
    description:
      'Tests API rate limiting by sending high-volume requests with various bypass techniques to validate API gateway and rate limiting controls.',
    category: 'application',
    severity: 'medium',
    icon: 'api_rate_limit',
    version: '1.3.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'targetApi',
        label: 'Target API Path',
        type: 'string',
        defaultValue: '/api/v1/data',
        required: true,
        description: 'API endpoint to test',
      },
      {
        key: 'requestRate',
        label: 'Requests per Second',
        type: 'number',
        defaultValue: 500,
        required: true,
        description: 'Number of requests to send per second',
        validation: { min: 10, max: 10000 },
      },
    ],
    attackPhases: [
      {
        name: 'Rate Limit Test',
        description: 'Send high volume of requests to API',
        technique: 'T1190',
        tactic: 'Initial Access',
        duration: 120,
      },
    ],
    expectedDetections: [
      {
        source: 'API Gateway',
        type: 'rate_limit',
        description: 'Rate limit threshold exceeded',
        confidence: 0.82,
      },
    ],
    tags: ['api', 'rate-limit', 'application', 'ddos'],
    isOfficial: true,
    usageCount: 723,
    createdAt: '2024-05-15T10:00:00Z',
    updatedAt: '2024-10-01T14:00:00Z',
  },
  {
    id: 'tmpl-9',
    name: 'Credential Dumping Simulation',
    description:
      'Simulates credential dumping from memory and disk to validate endpoint detection, SIEM correlation rules, and credential protection mechanisms.',
    category: 'identity',
    severity: 'high',
    icon: 'credential_dump',
    version: '1.0.0',
    author: 'Security Research',
    parameters: [
      {
        key: 'dumpMethod',
        label: 'Dump Method',
        type: 'select',
        defaultValue: 'mimikatz-style',
        required: true,
        description: 'Credential dump technique to simulate',
        options: [
          { label: 'Mimikatz-Style', value: 'mimikatz-style' },
          { label: 'LSASS Memory', value: 'lsass-memory' },
          { label: 'SAM Database', value: 'sam-database' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Credential Access',
        description: 'Dump credentials from target system',
        technique: 'T1003',
        tactic: 'Credential Access',
        duration: 30,
      },
    ],
    expectedDetections: [
      {
        source: 'EDR',
        type: 'credential_access',
        description: 'Credential dumping activity detected',
        confidence: 0.93,
      },
    ],
    tags: ['credential', 'dumping', 'identity', 'mitre-att&ck'],
    isOfficial: false,
    usageCount: 567,
    createdAt: '2024-07-01T09:00:00Z',
    updatedAt: '2024-09-15T12:00:00Z',
  },
  {
    id: 'tmpl-10',
    name: 'Lateral Movement via Service Mesh',
    description:
      'Simulates lateral movement through a service mesh to validate network segmentation policies, mTLS enforcement, and micro-segmentation controls.',
    category: 'network',
    severity: 'high',
    icon: 'lateral_movement',
    version: '1.4.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'sourceService',
        label: 'Source Service',
        type: 'string',
        defaultValue: 'frontend-svc',
        required: true,
        description: 'Service to initiate lateral movement from',
      },
      {
        key: 'targetService',
        label: 'Target Service',
        type: 'string',
        defaultValue: 'backend-svc',
        required: true,
        description: 'Destination service for lateral movement',
      },
    ],
    attackPhases: [
      {
        name: 'Service Discovery',
        description: 'Discover services in the mesh',
        technique: 'T1046',
        tactic: 'Discovery',
        duration: 20,
      },
      {
        name: 'Lateral Movement',
        description: 'Move laterally to target service',
        technique: 'T1021',
        tactic: 'Lateral Movement',
        duration: 60,
      },
    ],
    expectedDetections: [
      {
        source: 'Service Mesh',
        type: 'policy_violation',
        description: 'Unauthorized service-to-service communication',
        confidence: 0.87,
      },
    ],
    tags: ['lateral-movement', 'service-mesh', 'network', 'istio'],
    isOfficial: true,
    usageCount: 389,
    createdAt: '2024-08-01T10:00:00Z',
    updatedAt: '2024-11-18T16:00:00Z',
  },
];

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Category tab icon and label */
function CategoryTabLabel({
  category,
  showIcon = true,
}: {
  category: TemplateCategory | 'all';
  showIcon?: boolean;
}) {
  if (category === 'all') {
    return (
      <Stack direction="row" spacing={1} alignItems="center">
        {showIcon && <SecurityIcon sx={{ fontSize: 18 }} />}
        <span>All</span>
      </Stack>
    );
  }

  const config = CATEGORY_CONFIG[category];
  return (
    <Stack direction="row" spacing={1} alignItems="center">
      {showIcon && config.icon}
      <span>{config.label}</span>
    </Stack>
  );
}

/** Severity badge chip */
function SeverityBadge({ severity }: { severity: TemplateSeverity }) {
  const config = SEVERITY_CONFIG[severity];
  return (
    <Chip
      label={config.label}
      size="small"
      sx={{
        height: 24,
        fontSize: '0.6875rem',
        fontWeight: 700,
        backgroundColor: config.bgColor,
        color: config.color,
        border: `1px solid ${config.color}30`,
      }}
    />
  );
}

/** Template parameter count indicator */
function ParameterCountBadge({ parameters }: { parameters: TemplateParameter[] }) {
  return (
    <Tooltip title={`${parameters.length} configurable parameters`}>
      <Chip
        icon={<CustomIcon sx={{ fontSize: 14 }} />}
        label={parameters.length}
        size="small"
        variant="outlined"
        sx={{
          height: 22,
          fontSize: '0.6875rem',
          fontWeight: 500,
          '& .MuiChip-icon': { fontSize: 14 },
        }}
      />
    </Tooltip>
  );
}

/** Template card – grid view */
function TemplateCard({
  template,
  onSelect,
}: {
  template: AttackTemplate;
  onSelect: (template: AttackTemplate) => void;
}) {
  const navigate = useNavigate();
  const categoryConfig = CATEGORY_CONFIG[template.category];

  const handleUseTemplate = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigate('/experiments/new', { state: { templateId: template.id } });
  };

  return (
    <Card
      onClick={() => onSelect(template)}
      sx={{
        cursor: 'pointer',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
        position: 'relative',
        overflow: 'hidden',
        '&:hover': {
          borderColor: 'primary.main',
          transform: 'translateY(-3px)',
          boxShadow: '0 8px 24px rgba(0,0,0,0.1)',
        },
      }}
    >
      {/* Category accent bar */}
      <Box
        sx={{
          height: 4,
          backgroundColor: categoryConfig.color,
        }}
      />

      <CardContent sx={{ p: 2.5, flex: 1, display: 'flex', flexDirection: 'column' }}>
        {/* Header: Icon + Name + Official Badge */}
        <Stack direction="row" spacing={1.5} alignItems="flex-start" mb={1.5}>
          <Avatar
            variant="rounded"
            sx={{
              width: 40,
              height: 40,
              backgroundColor: `${categoryConfig.color}14`,
              color: categoryConfig.color,
              flexShrink: 0,
            }}
          >
            {categoryConfig.icon}
          </Avatar>

          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Stack direction="row" spacing={0.75} alignItems="center" mb={0.25}>
              <Typography
                variant="subtitle1"
                fontWeight={700}
                noWrap
                sx={{ fontSize: '0.9375rem', lineHeight: 1.3 }}
              >
                {template.name}
              </Typography>
              {template.isOfficial && (
                <Tooltip title="Official Chaos-Sec Template">
                  <VerifiedIcon
                    sx={{ fontSize: 16, color: 'primary.main', flexShrink: 0 }}
                  />
                </Tooltip>
              )}
            </Stack>
            <Stack direction="row" spacing={0.75} alignItems="center">
              <Chip
                label={categoryConfig.label}
                size="small"
                sx={{
                  height: 20,
                  fontSize: '0.625rem',
                  fontWeight: 600,
                  backgroundColor: `${categoryConfig.color}0D`,
                  color: categoryConfig.color,
                  border: `1px solid ${categoryConfig.color}24`,
                }}
              />
              <SeverityBadge severity={template.severity} />
            </Stack>
          </Box>
        </Stack>

        {/* Description */}
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{
            mb: 2,
            flex: 1,
            display: '-webkit-box',
            WebkitLineClamp: 3,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
            lineHeight: 1.6,
            fontSize: '0.8125rem',
          }}
        >
          {template.description}
        </Typography>

        {/* Techniques */}
        <Stack spacing={1} mb={2}>
          {template.attackPhases.length > 0 && (
            <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
              {template.attackPhases.slice(0, 2).map((phase) => (
                <Chip
                  key={phase.technique}
                  label={phase.technique}
                  size="small"
                  variant="outlined"
                  sx={{
                    height: 20,
                    fontSize: '0.625rem',
                    fontFamily: 'monospace',
                  }}
                />
              ))}
              {template.attackPhases.length > 2 && (
                <Chip
                  label={`+${template.attackPhases.length - 2}`}
                  size="small"
                  variant="outlined"
                  sx={{ height: 20, fontSize: '0.625rem' }}
                />
              )}
            </Stack>
          )}
        </Stack>

        {/* Footer: Meta info */}
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          sx={{ mt: 'auto' }}
        >
          <Stack direction="row" spacing={1.5}>
            <Stack direction="row" spacing={0.5} alignItems="center">
              <TrendingUpIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
              <Typography variant="caption" color="text.secondary" fontWeight={500}>
                {template.usageCount.toLocaleString()} uses
              </Typography>
            </Stack>
            <ParameterCountBadge parameters={template.parameters} />
          </Stack>
          <Typography variant="caption" color="text.disabled">
            v{template.version}
          </Typography>
        </Stack>
      </CardContent>

      <CardActions
        sx={{ px: 2.5, pb: 2, pt: 0, justifyContent: 'flex-end' }}
        onClick={(e) => e.stopPropagation()}
      >
        <Button
          size="small"
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleUseTemplate}
          sx={{ textTransform: 'none', fontWeight: 600, borderRadius: 1.5 }}
        >
          Use Template
        </Button>
      </CardActions>
    </Card>
  );
}

/** Template list row – list view */
function TemplateListItem({
  template,
  onSelect,
}: {
  template: AttackTemplate;
  onSelect: (template: AttackTemplate) => void;
}) {
  const navigate = useNavigate();
  const categoryConfig = CATEGORY_CONFIG[template.category];

  const handleUseTemplate = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigate('/experiments/new', { state: { templateId: template.id } });
  };

  return (
    <Paper
      variant="outlined"
      onClick={() => onSelect(template)}
      sx={{
        cursor: 'pointer',
        p: 2,
        borderRadius: 2,
        transition: 'all 150ms cubic-bezier(0.4, 0, 0.2, 1)',
        borderLeft: '3px solid',
        borderLeftColor: categoryConfig.color,
        '&:hover': {
          borderColor: 'primary.main',
          backgroundColor: 'action.hover',
        },
      }}
    >
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        spacing={2}
        alignItems={{ xs: 'flex-start', sm: 'center' }}
        justifyContent="space-between"
      >
        {/* Left side */}
        <Stack
          direction="row"
          spacing={1.5}
          alignItems="flex-start"
          sx={{ flex: 1, minWidth: 0 }}
        >
          <Avatar
            variant="rounded"
            sx={{
              width: 36,
              height: 36,
              backgroundColor: `${categoryConfig.color}14`,
              color: categoryConfig.color,
              flexShrink: 0,
            }}
          >
            {categoryConfig.icon}
          </Avatar>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Stack direction="row" spacing={0.75} alignItems="center" mb={0.25}>
              <Typography variant="body1" fontWeight={700} noWrap>
                {template.name}
              </Typography>
              {template.isOfficial && (
                <VerifiedIcon sx={{ fontSize: 14, color: 'primary.main' }} />
              )}
            </Stack>
            <Typography
              variant="body2"
              color="text.secondary"
              noWrap
              sx={{ fontSize: '0.8125rem', mb: 0.5 }}
            >
              {template.description}
            </Typography>
            <Stack
              direction="row"
              spacing={0.75}
              alignItems="center"
              flexWrap="wrap"
              useFlexGap
            >
              <Chip
                label={categoryConfig.label}
                size="small"
                sx={{
                  height: 20,
                  fontSize: '0.625rem',
                  fontWeight: 600,
                  backgroundColor: `${categoryConfig.color}0D`,
                  color: categoryConfig.color,
                }}
              />
              <SeverityBadge severity={template.severity} />
              {template.attackPhases.slice(0, 2).map((phase) => (
                <Chip
                  key={phase.technique}
                  label={phase.technique}
                  size="small"
                  variant="outlined"
                  sx={{ height: 20, fontSize: '0.625rem', fontFamily: 'monospace' }}
                />
              ))}
            </Stack>
          </Box>
        </Stack>

        {/* Right side */}
        <Stack direction="row" spacing={2} alignItems="center" sx={{ flexShrink: 0 }}>
          <Stack direction="row" spacing={0.5} alignItems="center">
            <TrendingUpIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
            <Typography variant="caption" color="text.secondary" fontWeight={500}>
              {template.usageCount.toLocaleString()}
            </Typography>
          </Stack>
          <ParameterCountBadge parameters={template.parameters} />
          <Button
            size="small"
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleUseTemplate}
            sx={{ textTransform: 'none', fontWeight: 600 }}
          >
            Use
          </Button>
        </Stack>
      </Stack>
    </Paper>
  );
}

/** Skeleton card for loading state */
function TemplateCardSkeleton() {
  return (
    <Card sx={{ height: '100%' }}>
      <Box sx={{ height: 4 }}>
        <Skeleton variant="rectangular" height={4} animation="wave" />
      </Box>
      <CardContent sx={{ p: 2.5 }}>
        <Stack direction="row" spacing={1.5} mb={2}>
          <Skeleton variant="rounded" width={40} height={40} animation="wave" />
          <Box sx={{ flex: 1 }}>
            <Skeleton variant="text" width="70%" height={24} animation="wave" />
            <Stack direction="row" spacing={1} mt={0.5}>
              <Skeleton variant="rounded" width={60} height={20} animation="wave" />
              <Skeleton variant="rounded" width={50} height={20} animation="wave" />
            </Stack>
          </Box>
        </Stack>
        <Skeleton variant="text" height={16} animation="wave" />
        <Skeleton variant="text" height={16} width="80%" animation="wave" />
        <Skeleton
          variant="text"
          height={16}
          width="60%"
          animation="wave"
          sx={{ mb: 2 }}
        />
        <Stack direction="row" justifyContent="space-between">
          <Skeleton variant="text" width="30%" height={16} animation="wave" />
          <Skeleton variant="text" width="15%" height={16} animation="wave" />
        </Stack>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Main Page Component
// ---------------------------------------------------------------------------

const TemplateListPage: React.FC = () => {
  const navigate = useNavigate();

  // -----------------------------------------------------------------------
  // State
  // -----------------------------------------------------------------------

  const [templates, setTemplates] = useState<AttackTemplate[]>(MOCK_TEMPLATES);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [searchQuery, setSearchQuery] = useState('');
  const [activeCategory, setActiveCategory] = useState<TemplateCategory | 'all'>('all');
  const [severityFilter, setSeverityFilter] = useState<TemplateSeverity | 'all'>('all');
  const [sortBy, setSortBy] = useState('popular');
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [selectedTemplate, setSelectedTemplate] = useState<AttackTemplate | null>(null);

  // -----------------------------------------------------------------------
  // Fetch Templates
  // -----------------------------------------------------------------------

  const fetchTemplates = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const response = await templatesAPI.list({
        category: activeCategory === 'all' ? undefined : activeCategory,
        search: searchQuery || undefined,
        severity: severityFilter === 'all' ? undefined : severityFilter,
      });
      const data = response.data as unknown as { items: AttackTemplate[] };
      setTemplates(data.items ?? MOCK_TEMPLATES);
    } catch (err) {
      setError(getErrorMessage(err));
      // Use mock data as fallback
      setTemplates(MOCK_TEMPLATES);
    } finally {
      setIsLoading(false);
    }
  }, [activeCategory, searchQuery, severityFilter]);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

  // -----------------------------------------------------------------------
  // Filtered & Sorted Templates
  // -----------------------------------------------------------------------

  const filteredTemplates = useMemo(() => {
    let result = [...templates];

    // Search filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase().trim();
      result = result.filter(
        (t) =>
          t.name.toLowerCase().includes(query) ||
          t.description.toLowerCase().includes(query) ||
          t.tags.some((tag) => tag.toLowerCase().includes(query)) ||
          t.category.toLowerCase().includes(query) ||
          t.attackPhases.some(
            (p) =>
              p.technique.toLowerCase().includes(query) ||
              p.tactic.toLowerCase().includes(query),
          ),
      );
    }

    // Category filter
    if (activeCategory !== 'all') {
      result = result.filter((t) => t.category === activeCategory);
    }

    // Severity filter
    if (severityFilter !== 'all') {
      result = result.filter((t) => t.severity === severityFilter);
    }

    // Sort
    switch (sortBy) {
      case 'popular':
        result.sort((a, b) => b.usageCount - a.usageCount);
        break;
      case 'newest':
        result.sort(
          (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
        );
        break;
      case 'name-asc':
        result.sort((a, b) => a.name.localeCompare(b.name));
        break;
      case 'name-desc':
        result.sort((a, b) => b.name.localeCompare(a.name));
        break;
      case 'severity-desc':
        result.sort((a, b) => SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity]);
        break;
      case 'severity-asc':
        result.sort((a, b) => SEVERITY_ORDER[b.severity] - SEVERITY_ORDER[a.severity]);
        break;
    }

    return result;
  }, [templates, searchQuery, activeCategory, severityFilter, sortBy]);

  // -----------------------------------------------------------------------
  // Category counts
  // -----------------------------------------------------------------------

  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: templates.length };
    Object.keys(CATEGORY_CONFIG).forEach((cat) => {
      counts[cat] = templates.filter((t) => t.category === cat).length;
    });
    return counts;
  }, [templates]);

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(e.target.value);
  };

  const handleClearSearch = () => {
    setSearchQuery('');
  };

  const handleCategoryChange = (_: React.SyntheticEvent, newValue: number | string) => {
    setActiveCategory(newValue as TemplateCategory | 'all');
  };

  const handleSeverityChange = (e: SelectChangeEvent<TemplateSeverity | 'all'>) => {
    setSeverityFilter(e.target.value as TemplateSeverity | 'all');
  };

  const handleSortChange = (e: SelectChangeEvent<string>) => {
    setSortBy(e.target.value);
  };

  const handleSelectTemplate = (template: AttackTemplate) => {
    setSelectedTemplate(template);
  };

  const handleUseTemplate = (templateId: string) => {
    navigate('/experiments/new', { state: { templateId } });
  };

  const handleClearFilters = () => {
    setSearchQuery('');
    setActiveCategory('all');
    setSeverityFilter('all');
    setSortBy('popular');
  };

  const hasActiveFilters =
    searchQuery !== '' ||
    activeCategory !== 'all' ||
    severityFilter !== 'all' ||
    sortBy !== 'popular';

  // -----------------------------------------------------------------------
  // Render
  // -----------------------------------------------------------------------

  return (
    <Box>
      {/* Page Header */}
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        justifyContent="space-between"
        alignItems={{ xs: 'flex-start', sm: 'center' }}
        spacing={2}
        mb={3}
      >
        <Box>
          <Typography variant="h4" fontWeight={700} gutterBottom>
            Attack Templates
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Browse and select attack templates to validate your security controls. Each
            template is mapped to MITRE ATT&CK techniques.
          </Typography>
        </Box>

        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => navigate('/experiments/new')}
          sx={{ borderRadius: 2, px: 3, textTransform: 'none', fontWeight: 600 }}
        >
          Custom Experiment
        </Button>
      </Stack>

      {/* Error Alert */}
      {error && (
        <Alert
          severity="warning"
          onClose={() => setError(null)}
          sx={{ mb: 2, borderRadius: 2 }}
        >
          Could not load templates from the server. Showing cached templates.
        </Alert>
      )}

      {/* Search & Filter Bar */}
      <Paper
        variant="outlined"
        sx={{
          borderRadius: 2,
          mb: 3,
          overflow: 'hidden',
        }}
      >
        {/* Search + Controls Row */}
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          spacing={1.5}
          sx={{ p: 2 }}
          alignItems={{ xs: 'stretch', md: 'center' }}
        >
          {/* Search Field */}
          <TextField
            size="small"
            placeholder="Search templates by name, technique, tag..."
            value={searchQuery}
            onChange={handleSearchChange}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                </InputAdornment>
              ),
              endAdornment: searchQuery ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={handleClearSearch}>
                    <ClearIcon sx={{ fontSize: 16 }} />
                  </IconButton>
                </InputAdornment>
              ) : null,
            }}
            sx={{
              flex: 1,
              minWidth: { xs: '100%', md: 300 },
              '& .MuiOutlinedInput-root': { borderRadius: 1.5 },
            }}
          />

          {/* Severity Filter */}
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel>Severity</InputLabel>
            <Select
              value={severityFilter}
              label="Severity"
              onChange={handleSeverityChange}
              sx={{ borderRadius: 1.5 }}
            >
              <MenuItem value="all">All Severities</MenuItem>
              <MenuItem value="critical">
                <Stack direction="row" spacing={1} alignItems="center">
                  <SeverityBadge severity="critical" />
                </Stack>
              </MenuItem>
              <MenuItem value="high">
                <Stack direction="row" spacing={1} alignItems="center">
                  <SeverityBadge severity="high" />
                </Stack>
              </MenuItem>
              <MenuItem value="medium">
                <Stack direction="row" spacing={1} alignItems="center">
                  <SeverityBadge severity="medium" />
                </Stack>
              </MenuItem>
              <MenuItem value="low">
                <Stack direction="row" spacing={1} alignItems="center">
                  <SeverityBadge severity="low" />
                </Stack>
              </MenuItem>
            </Select>
          </FormControl>

          {/* Sort */}
          <FormControl size="small" sx={{ minWidth: 160 }}>
            <InputLabel>Sort By</InputLabel>
            <Select
              value={sortBy}
              label="Sort By"
              onChange={handleSortChange}
              sx={{ borderRadius: 1.5 }}
            >
              {SORT_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* View Mode Toggle */}
          <Stack direction="row" spacing={0.5}>
            <Tooltip title="Grid View">
              <IconButton
                size="small"
                onClick={() => setViewMode('grid')}
                sx={{
                  border: '1px solid',
                  borderColor: viewMode === 'grid' ? 'primary.main' : 'divider',
                  backgroundColor: viewMode === 'grid' ? 'primary.50' : 'transparent',
                  color: viewMode === 'grid' ? 'primary.main' : 'text.secondary',
                  borderRadius: 1.5,
                }}
              >
                <GridViewIcon sx={{ fontSize: 18 }} />
              </IconButton>
            </Tooltip>
            <Tooltip title="List View">
              <IconButton
                size="small"
                onClick={() => setViewMode('list')}
                sx={{
                  border: '1px solid',
                  borderColor: viewMode === 'list' ? 'primary.main' : 'divider',
                  backgroundColor: viewMode === 'list' ? 'primary.50' : 'transparent',
                  color: viewMode === 'list' ? 'primary.main' : 'text.secondary',
                  borderRadius: 1.5,
                }}
              >
                <ViewListViewIcon sx={{ fontSize: 18 }} />
              </IconButton>
            </Tooltip>
          </Stack>

          {/* Clear Filters */}
          {hasActiveFilters && (
            <Button
              size="small"
              variant="text"
              startIcon={<ClearIcon />}
              onClick={handleClearFilters}
              sx={{ textTransform: 'none', whiteSpace: 'nowrap' }}
            >
              Clear all
            </Button>
          )}
        </Stack>

        {/* Category Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider', px: 2 }}>
          <Tabs
            value={activeCategory}
            onChange={handleCategoryChange}
            variant="scrollable"
            scrollButtons="auto"
            sx={{
              minHeight: 44,
              '& .MuiTab-root': {
                minHeight: 44,
                textTransform: 'none',
                fontWeight: 500,
                fontSize: '0.875rem',
              },
            }}
          >
            <Tab
              value="all"
              label={
                <Badge
                  badgeContent={categoryCounts['all']}
                  color="primary"
                  max={99}
                  sx={{
                    '& .MuiBadge-badge': {
                      fontSize: '0.625rem',
                      minWidth: 16,
                      height: 16,
                      right: -12,
                      top: 2,
                    },
                  }}
                >
                  <CategoryTabLabel category="all" />
                </Badge>
              }
            />
            {(Object.keys(CATEGORY_CONFIG) as TemplateCategory[]).map((category) => (
              <Tab
                key={category}
                value={category}
                label={
                  <Badge
                    badgeContent={categoryCounts[category] ?? 0}
                    color="primary"
                    max={99}
                    sx={{
                      '& .MuiBadge-badge': {
                        fontSize: '0.625rem',
                        minWidth: 16,
                        height: 16,
                        right: -12,
                        top: 2,
                      },
                    }}
                  >
                    <CategoryTabLabel category={category} />
                  </Badge>
                }
              />
            ))}
          </Tabs>
        </Box>
      </Paper>

      {/* Active Filter Chips */}
      {hasActiveFilters && (
        <Stack direction="row" spacing={1} mb={2} flexWrap="wrap" useFlexGap>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ lineHeight: '26px' }}
          >
            Active filters:
          </Typography>
          {searchQuery && (
            <Chip
              label={`Search: "${searchQuery}"`}
              size="small"
              onDelete={handleClearSearch}
              sx={{ height: 26 }}
            />
          )}
          {activeCategory !== 'all' && (
            <Chip
              label={`Category: ${CATEGORY_CONFIG[activeCategory].label}`}
              size="small"
              onDelete={() => setActiveCategory('all')}
              sx={{ height: 26 }}
            />
          )}
          {severityFilter !== 'all' && (
            <Chip
              label={`Severity: ${SEVERITY_CONFIG[severityFilter].label}`}
              size="small"
              onDelete={() => setSeverityFilter('all')}
              sx={{ height: 26 }}
            />
          )}
        </Stack>
      )}

      {/* Results Summary */}
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="body2" color="text.secondary">
          {isLoading
            ? 'Loading templates...'
            : `${filteredTemplates.length} template${filteredTemplates.length !== 1 ? 's' : ''} found`}
        </Typography>
      </Stack>

      {/* Template Content */}
      {isLoading ? (
        <Grid container spacing={2.5}>
          {SKELETON_TEMPLATE_IDS.map((id) => (
            <Grid item xs={12} sm={6} md={4} lg={3} key={id}>
              <TemplateCardSkeleton />
            </Grid>
          ))}
        </Grid>
      ) : filteredTemplates.length === 0 ? (
        /* Empty State */
        <Box sx={{ py: 8, textAlign: 'center' }}>
          <TemplateIcon sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }} />
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No templates found
          </Typography>
          <Typography
            variant="body2"
            color="text.disabled"
            sx={{ mb: 3, maxWidth: 420, mx: 'auto' }}
          >
            {hasActiveFilters
              ? 'Try adjusting your filters or search terms to find templates.'
              : 'No attack templates are available yet. Check back later or create a custom experiment.'}
          </Typography>
          {hasActiveFilters ? (
            <Button
              variant="outlined"
              startIcon={<ClearIcon />}
              onClick={handleClearFilters}
              sx={{ mr: 1, textTransform: 'none' }}
            >
              Clear Filters
            </Button>
          ) : null}
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => navigate('/experiments/new')}
            sx={{ textTransform: 'none' }}
          >
            Create Custom Experiment
          </Button>
        </Box>
      ) : viewMode === 'grid' ? (
        /* Grid View */
        <Grid container spacing={2.5}>
          {filteredTemplates.map((template) => (
            <Grid item xs={12} sm={6} md={4} lg={3} key={template.id}>
              <TemplateCard template={template} onSelect={handleSelectTemplate} />
            </Grid>
          ))}
        </Grid>
      ) : (
        /* List View */
        <Stack spacing={1.5}>
          {filteredTemplates.map((template) => (
            <TemplateListItem
              key={template.id}
              template={template}
              onSelect={handleSelectTemplate}
            />
          ))}
        </Stack>
      )}

      {/* Template Detail Drawer / Preview */}
      {selectedTemplate && (
        <Box
          sx={{
            position: 'fixed',
            inset: 0,
            zIndex: 1300,
            display: 'flex',
            justifyContent: 'flex-end',
          }}
        >
          {/* Backdrop */}
          <Box
            onClick={() => setSelectedTemplate(null)}
            sx={{
              position: 'absolute',
              inset: 0,
              backgroundColor: 'rgba(0,0,0,0.5)',
            }}
          />

          {/* Panel */}
          <Paper
            elevation={24}
            sx={{
              position: 'relative',
              width: { xs: '100%', sm: 480, md: 560 },
              maxWidth: '100%',
              height: '100%',
              overflowY: 'auto',
              borderRadius: 0,
              borderTopLeftRadius: 16,
              borderBottomLeftRadius: 16,
            }}
          >
            {/* Header with accent */}
            <Box
              sx={{
                p: 3,
                pb: 2,
                borderBottom: '1px solid',
                borderColor: 'divider',
                borderLeft: '4px solid',
                borderLeftColor: CATEGORY_CONFIG[selectedTemplate.category].color,
              }}
            >
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="flex-start"
              >
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Stack direction="row" spacing={1} alignItems="center" mb={1}>
                    <Avatar
                      variant="rounded"
                      sx={{
                        width: 36,
                        height: 36,
                        backgroundColor: `${CATEGORY_CONFIG[selectedTemplate.category].color}14`,
                        color: CATEGORY_CONFIG[selectedTemplate.category].color,
                      }}
                    >
                      {CATEGORY_CONFIG[selectedTemplate.category].icon}
                    </Avatar>
                    <Typography
                      variant="h5"
                      fontWeight={800}
                      sx={{ fontSize: '1.25rem' }}
                    >
                      {selectedTemplate.name}
                    </Typography>
                  </Stack>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Chip
                      label={CATEGORY_CONFIG[selectedTemplate.category].label}
                      size="small"
                      sx={{
                        height: 22,
                        fontSize: '0.6875rem',
                        fontWeight: 600,
                        backgroundColor: `${CATEGORY_CONFIG[selectedTemplate.category].color}0D`,
                        color: CATEGORY_CONFIG[selectedTemplate.category].color,
                      }}
                    />
                    <SeverityBadge severity={selectedTemplate.severity} />
                    {selectedTemplate.isOfficial && (
                      <Chip
                        icon={<VerifiedIcon sx={{ fontSize: 14 }} />}
                        label="Official"
                        size="small"
                        color="primary"
                        variant="outlined"
                        sx={{ height: 22, fontSize: '0.6875rem' }}
                      />
                    )}
                  </Stack>
                </Box>
                <IconButton onClick={() => setSelectedTemplate(null)} sx={{ ml: 2 }}>
                  <ClearIcon />
                </IconButton>
              </Stack>
            </Box>

            {/* Content */}
            <Box sx={{ p: 3 }}>
              {/* Description */}
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mb: 3, lineHeight: 1.7 }}
              >
                {selectedTemplate.description}
              </Typography>

              {/* Key Metrics */}
              <Stack direction="row" spacing={2} mb={3}>
                <Box
                  sx={{
                    flex: 1,
                    p: 1.5,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                    textAlign: 'center',
                  }}
                >
                  <Typography variant="h5" fontWeight={700} color="primary">
                    {selectedTemplate.usageCount.toLocaleString()}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Uses
                  </Typography>
                </Box>
                <Box
                  sx={{
                    flex: 1,
                    p: 1.5,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                    textAlign: 'center',
                  }}
                >
                  <Typography variant="h5" fontWeight={700}>
                    {selectedTemplate.parameters.length}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Parameters
                  </Typography>
                </Box>
                <Box
                  sx={{
                    flex: 1,
                    p: 1.5,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                    textAlign: 'center',
                  }}
                >
                  <Typography variant="h5" fontWeight={700}>
                    {selectedTemplate.attackPhases.length}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Phases
                  </Typography>
                </Box>
              </Stack>

              {/* Attack Phases */}
              <Typography variant="subtitle2" fontWeight={700} sx={{ mb: 1.5 }}>
                Attack Phases
              </Typography>
              <Stack spacing={1} mb={3}>
                {selectedTemplate.attackPhases.map((phase) => (
                  <Paper
                    key={phase.name}
                    variant="outlined"
                    sx={{ p: 1.5, borderRadius: 1.5 }}
                  >
                    <Stack
                      direction="row"
                      justifyContent="space-between"
                      alignItems="center"
                    >
                      <Box>
                        <Typography variant="body2" fontWeight={600}>
                          {phase.name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {phase.tactic} · {phase.technique}
                        </Typography>
                      </Box>
                      <Chip
                        label={`${phase.duration}s`}
                        size="small"
                        variant="outlined"
                        sx={{
                          height: 22,
                          fontSize: '0.6875rem',
                          fontFamily: 'monospace',
                        }}
                      />
                    </Stack>
                  </Paper>
                ))}
              </Stack>

              {/* Expected Detections */}
              <Typography variant="subtitle2" fontWeight={700} sx={{ mb: 1.5 }}>
                Expected Detections
              </Typography>
              <Stack spacing={1} mb={3}>
                {selectedTemplate.expectedDetections.map((detection) => (
                  <Paper
                    key={`${detection.source}-${detection.type}`}
                    variant="outlined"
                    sx={{ p: 1.5, borderRadius: 1.5 }}
                  >
                    <Stack direction="row" spacing={1} alignItems="flex-start">
                      <SecurityIcon
                        sx={{ fontSize: 16, color: 'success.main', mt: 0.25 }}
                      />
                      <Box sx={{ flex: 1 }}>
                        <Typography variant="body2" fontWeight={500}>
                          {detection.source}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {detection.description}
                        </Typography>
                      </Box>
                      <Chip
                        label={`${Math.round(detection.confidence * 100)}%`}
                        size="small"
                        color="success"
                        variant="outlined"
                        sx={{ height: 22, fontSize: '0.6875rem' }}
                      />
                    </Stack>
                  </Paper>
                ))}
              </Stack>

              {/* Parameters */}
              {selectedTemplate.parameters.length > 0 && (
                <>
                  <Typography variant="subtitle2" fontWeight={700} sx={{ mb: 1.5 }}>
                    Configurable Parameters
                  </Typography>
                  <Stack spacing={1} mb={3}>
                    {selectedTemplate.parameters.map((param) => (
                      <Paper
                        key={param.key}
                        variant="outlined"
                        sx={{ p: 1.5, borderRadius: 1.5 }}
                      >
                        <Stack
                          direction="row"
                          justifyContent="space-between"
                          alignItems="center"
                        >
                          <Box>
                            <Typography variant="body2" fontWeight={600}>
                              {param.label}
                              {param.required && (
                                <Typography
                                  component="span"
                                  sx={{ color: 'error.main', ml: 0.5 }}
                                >
                                  *
                                </Typography>
                              )}
                            </Typography>
                            <Typography variant="caption" color="text.secondary">
                              {param.description}
                            </Typography>
                          </Box>
                          <Chip
                            label={param.type}
                            size="small"
                            variant="outlined"
                            sx={{
                              height: 20,
                              fontSize: '0.625rem',
                              fontFamily: 'monospace',
                            }}
                          />
                        </Stack>
                      </Paper>
                    ))}
                  </Stack>
                </>
              )}

              {/* Tags */}
              {selectedTemplate.tags.length > 0 && (
                <>
                  <Typography variant="subtitle2" fontWeight={700} sx={{ mb: 1 }}>
                    Tags
                  </Typography>
                  <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap mb={3}>
                    {selectedTemplate.tags.map((tag) => (
                      <Chip
                        key={tag}
                        label={tag}
                        size="small"
                        variant="outlined"
                        sx={{ fontSize: '0.75rem', mb: 0.5 }}
                      />
                    ))}
                  </Stack>
                </>
              )}

              {/* Author & Version */}
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="center"
                mb={3}
              >
                <Typography variant="caption" color="text.secondary">
                  By {selectedTemplate.author}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.disabled"
                  sx={{ fontFamily: 'monospace' }}
                >
                  v{selectedTemplate.version}
                </Typography>
              </Stack>

              <Divider sx={{ mb: 3 }} />

              {/* Action Buttons */}
              <Stack direction="row" spacing={1.5}>
                <Button
                  variant="contained"
                  fullWidth
                  startIcon={<AddIcon />}
                  onClick={() => handleUseTemplate(selectedTemplate.id)}
                  sx={{ textTransform: 'none', fontWeight: 700, py: 1.25 }}
                >
                  Use This Template
                </Button>
                <Tooltip title="Download Template">
                  <IconButton
                    sx={{
                      border: '1px solid',
                      borderColor: 'divider',
                      borderRadius: 1.5,
                    }}
                  >
                    <DownloadIcon />
                  </IconButton>
                </Tooltip>
              </Stack>
            </Box>
          </Paper>
        </Box>
      )}
    </Box>
  );
};

export default TemplateListPage;
