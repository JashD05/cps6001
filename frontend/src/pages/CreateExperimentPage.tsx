import {
  Science as TemplateIcon,
  Settings as ConfigIcon,
  Security as ValidationIcon,
  FactCheck as ReviewIcon,
  NavigateNext as NextIcon,
  NavigateBefore as BackIcon,
  RocketLaunch as CreateIcon,
  Dns as ClusterIcon,
  Cloud as NamespaceIcon,
  Add as AddIcon,
  CheckCircle as CheckIcon,
  Search as SearchIcon,
  Clear as ClearIcon,
  Warning as WarningIcon,
  Shield as ShieldIcon,
} from '@mui/icons-material';
import {
  Box,
  Typography,
  Button,
  Paper,
  Stepper,
  Step,
  StepLabel,
  Grid,
  Card,
  CardContent,
  Stack,
  TextField,
  MenuItem,
  FormControl,
  InputLabel,
  Select,
  Slider,
  Switch,
  FormControlLabel,
  Chip,
  Divider,
  Alert,
  AlertTitle,
  CircularProgress,
  Avatar,
  InputAdornment,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  useTheme,
  useMediaQuery,
} from '@mui/material';
import React, { useState, useCallback, useMemo, useRef } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import StatusBadge from '@/components/StatusBadge';
import { MOCK_CLUSTERS } from '@/data/mockClusters';
import {
  clustersAPI,
  templatesAPI,
  experimentsAPI,
  getErrorMessage,
} from '@/services/api';
import {
  createExperiment,
  selectCreateStatus,
  selectCreateError,
  resetCreateStatus,
} from '@/store/experimentSlice';
import type { AppDispatch } from '@/store';
import type {
  AttackTemplate,
  TemplateParameter,
  TemplateCategory,
  TemplateSeverity,
  CreateExperimentRequest,
  Cluster,
} from '@/types';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STEPS = [
  {
    label: 'Select Template',
    icon: <TemplateIcon />,
    description: 'Choose an attack template',
  },
  { label: 'Configure', icon: <ConfigIcon />, description: 'Set parameters and target' },
  {
    label: 'Validation',
    icon: <ValidationIcon />,
    description: 'Define SIEM validation rules',
  },
  { label: 'Review', icon: <ReviewIcon />, description: 'Confirm and create' },
];

const CATEGORY_OPTIONS: {
  value: TemplateCategory | 'all';
  label: string;
  color: string;
}[] = [
  { value: 'all', label: 'All', color: '#64748B' },
  { value: 'network', label: 'Network', color: '#2563EB' },
  { value: 'application', label: 'Application', color: '#7C3AED' },
  { value: 'infrastructure', label: 'Infrastructure', color: '#F59E0B' },
  { value: 'data', label: 'Data', color: '#10B981' },
  { value: 'identity', label: 'Identity', color: '#EF4444' },
  { value: 'custom', label: 'Custom', color: '#06B6D4' },
];

const SEVERITY_COLORS: Record<TemplateSeverity, string> = {
  low: '#10B981',
  medium: '#F59E0B',
  high: '#F97316',
  critical: '#EF4444',
};

const FRIENDLY_PARAMETER_LABELS: Record<string, { label: string; description: string }> =
  {
    attempts: { label: 'Attempts', description: 'How many tries to make.' },
    target_namespace: {
      label: 'Namespace',
      description: 'Where the test should run inside the cluster.',
    },
    test_port: { label: 'Port', description: 'Which port to check.' },
    test_cidr: {
      label: 'IP range',
      description: 'Which address range to test against the policy.',
    },
    policy_name: {
      label: 'Policy name',
      description: 'Leave blank to test every policy in the namespace.',
    },
    timeout_seconds: {
      label: 'Timeout',
      description: 'How long to wait before stopping the test.',
    },
  };

const isAdvancedParameter = (param: TemplateParameter): boolean => {
  if (param.required) return false;
  const key = param.key.toLowerCase();
  return (
    key.includes('timeout') ||
    key.includes('cidr') ||
    key.includes('policy') ||
    key.includes('selector') ||
    key.includes('interval') ||
    key.includes('concurrency') ||
    key.includes('rule') ||
    key.includes('config')
  );
};

const getParameterCopy = (param: TemplateParameter) => {
  const friendly = FRIENDLY_PARAMETER_LABELS[param.key.toLowerCase()];
  return {
    label: friendly?.label ?? param.label,
    description: friendly?.description ?? param.description,
  };
};

// ---------------------------------------------------------------------------
// Mock Data (would come from API in production)
// ---------------------------------------------------------------------------

const MOCK_TEMPLATES: AttackTemplate[] = [
  {
    id: 'tmpl-dns-exfil',
    name: 'DNS Exfiltration',
    description:
      'Simulates data exfiltration via DNS queries to detect DLP and monitoring gaps.',
    category: 'network',
    severity: 'high',
    icon: '📡',
    version: '1.2.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'targetDomain',
        label: 'Target Domain',
        type: 'string',
        defaultValue: 'example.com',
        required: true,
        description: 'The domain to use for DNS exfiltration simulation',
      },
      {
        key: 'dataSize',
        label: 'Data Size (KB)',
        type: 'number',
        defaultValue: 10,
        required: true,
        description: 'Amount of data to exfiltrate in kilobytes',
        validation: { min: 1, max: 100 },
      },
      {
        key: 'queryInterval',
        label: 'Query Interval (ms)',
        type: 'number',
        defaultValue: 500,
        required: false,
        description: 'Interval between DNS queries',
        validation: { min: 100, max: 5000 },
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
          { label: 'Raw', value: 'raw' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Setup',
        description: 'Deploy DNS exfil pod',
        technique: 'T1048',
        tactic: 'Exfiltration',
        duration: 30,
      },
      {
        name: 'Execute',
        description: 'Perform DNS queries',
        technique: 'T1048.003',
        tactic: 'Exfiltration',
        duration: 120,
      },
      {
        name: 'Cleanup',
        description: 'Remove artifacts',
        technique: 'T1070',
        tactic: 'Defense Evasion',
        duration: 20,
      },
    ],
    expectedDetections: [
      {
        source: 'SIEM',
        type: 'dns_anomaly',
        description: 'Unusual DNS query volume or pattern',
        confidence: 0.85,
      },
      {
        source: 'DLP',
        type: 'data_loss',
        description: 'Data loss prevention alert',
        confidence: 0.7,
      },
    ],
    tags: ['dns', 'exfiltration', 'network', 'dlp'],
    isOfficial: true,
    usageCount: 247,
    createdAt: '2024-01-15T00:00:00Z',
    updatedAt: '2024-03-01T00:00:00Z',
  },
  {
    id: 'tmpl-brute-force',
    name: 'Brute Force Attack',
    description:
      'Simulates credential brute force attacks against Kubernetes service accounts.',
    category: 'identity',
    severity: 'critical',
    icon: '🔓',
    version: '1.1.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'targetService',
        label: 'Target Service',
        type: 'string',
        defaultValue: 'api-server',
        required: true,
        description: 'Service to target for brute force',
      },
      {
        key: 'attempts',
        label: 'Login Attempts',
        type: 'number',
        defaultValue: 50,
        required: true,
        description: 'Number of login attempts',
        validation: { min: 5, max: 500 },
      },
      {
        key: 'concurrency',
        label: 'Concurrency',
        type: 'number',
        defaultValue: 5,
        required: false,
        description: 'Concurrent login threads',
        validation: { min: 1, max: 20 },
      },
    ],
    attackPhases: [
      {
        name: 'Recon',
        description: 'Identify target service',
        technique: 'T1595',
        tactic: 'Reconnaissance',
        duration: 15,
      },
      {
        name: 'Attack',
        description: 'Brute force credentials',
        technique: 'T1110',
        tactic: 'Credential Access',
        duration: 180,
      },
    ],
    expectedDetections: [
      {
        source: 'SIEM',
        type: 'brute_force',
        description: 'Multiple failed login attempts',
        confidence: 0.95,
      },
      {
        source: 'IDS',
        type: 'intrusion',
        description: 'Intrusion detection alert',
        confidence: 0.8,
      },
    ],
    tags: ['brute-force', 'identity', 'credentials'],
    isOfficial: true,
    usageCount: 189,
    createdAt: '2024-01-20T00:00:00Z',
    updatedAt: '2024-02-28T00:00:00Z',
  },
  {
    id: 'tmpl-pod-kill',
    name: 'Pod Kill',
    description:
      'Randomly kills pods to test cluster resilience and auto-healing capabilities.',
    category: 'infrastructure',
    severity: 'medium',
    icon: '💀',
    version: '2.0.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'killCount',
        label: 'Pods to Kill',
        type: 'number',
        defaultValue: 1,
        required: true,
        description: 'Number of pods to kill',
        validation: { min: 1, max: 10 },
      },
      {
        key: 'selector',
        label: 'Pod Selector (labels)',
        type: 'string',
        defaultValue: 'app=web',
        required: true,
        description: 'Label selector to target specific pods',
      },
      {
        key: 'interval',
        label: 'Kill Interval (s)',
        type: 'number',
        defaultValue: 30,
        required: false,
        description: 'Seconds between each pod kill',
        validation: { min: 5, max: 300 },
      },
      {
        key: 'gracePeriod',
        label: 'Grace Period (s)',
        type: 'number',
        defaultValue: 5,
        required: false,
        description: 'Grace period before force kill',
        validation: { min: 0, max: 60 },
      },
    ],
    attackPhases: [
      {
        name: 'Identify',
        description: 'Find target pods',
        technique: 'T1087',
        tactic: 'Discovery',
        duration: 10,
      },
      {
        name: 'Kill',
        description: 'Terminate pods',
        technique: 'T1489',
        tactic: 'Impact',
        duration: 60,
      },
      {
        name: 'Observe',
        description: 'Monitor recovery',
        technique: 'T1046',
        tactic: 'Discovery',
        duration: 120,
      },
    ],
    expectedDetections: [
      {
        source: 'Monitoring',
        type: 'pod_down',
        description: 'Pod downtime alert',
        confidence: 0.99,
      },
      {
        source: 'SIEM',
        type: 'service_degradation',
        description: 'Service degradation alert',
        confidence: 0.75,
      },
    ],
    tags: ['pod-kill', 'infrastructure', 'resilience', 'chaos'],
    isOfficial: true,
    usageCount: 312,
    createdAt: '2024-01-10T00:00:00Z',
    updatedAt: '2024-03-05T00:00:00Z',
  },
  {
    id: 'tmpl-network-partition',
    name: 'Network Partition',
    description:
      'Simulates network partitions between pods to test service mesh failover and redundancy.',
    category: 'network',
    severity: 'high',
    icon: '🌐',
    version: '1.0.1',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'sourceApp',
        label: 'Source Application',
        type: 'string',
        defaultValue: 'frontend',
        required: true,
        description: 'Source application label',
      },
      {
        key: 'targetApp',
        label: 'Target Application',
        type: 'string',
        defaultValue: 'backend',
        required: true,
        description: 'Target application label',
      },
      {
        key: 'duration',
        label: 'Partition Duration (s)',
        type: 'number',
        defaultValue: 60,
        required: true,
        description: 'How long the partition lasts',
        validation: { min: 10, max: 600 },
      },
      {
        key: 'direction',
        label: 'Partition Direction',
        type: 'select',
        defaultValue: 'both',
        required: true,
        description: 'Direction of network block',
        options: [
          { label: 'Both', value: 'both' },
          { label: 'Ingress Only', value: 'ingress' },
          { label: 'Egress Only', value: 'egress' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Inject',
        description: 'Apply network rules',
        technique: 'T1498',
        tactic: 'Impact',
        duration: 10,
      },
      {
        name: 'Hold',
        description: 'Maintain partition',
        technique: 'T1498',
        tactic: 'Impact',
        duration: 60,
      },
      {
        name: 'Recover',
        description: 'Remove network rules',
        technique: 'T1070',
        tactic: 'Defense Evasion',
        duration: 10,
      },
    ],
    expectedDetections: [
      {
        source: 'Service Mesh',
        type: 'circuit_breaker',
        description: 'Circuit breaker activation',
        confidence: 0.9,
      },
      {
        source: 'Monitoring',
        type: 'latency_spike',
        description: 'Latency increase alert',
        confidence: 0.85,
      },
    ],
    tags: ['network', 'partition', 'service-mesh', 'resilience'],
    isOfficial: true,
    usageCount: 156,
    createdAt: '2024-02-01T00:00:00Z',
    updatedAt: '2024-03-10T00:00:00Z',
  },
  {
    id: 'tmpl-priv-escalation',
    name: 'Privilege Escalation',
    description:
      'Tests for container privilege escalation vectors and RBAC misconfigurations.',
    category: 'application',
    severity: 'critical',
    icon: '⬆️',
    version: '1.3.0',
    author: 'Security Research',
    parameters: [
      {
        key: 'targetPod',
        label: 'Target Pod',
        type: 'string',
        defaultValue: '',
        required: true,
        description: 'Name of the target pod for escalation attempt',
      },
      {
        key: 'method',
        label: 'Escalation Method',
        type: 'select',
        defaultValue: 'container_escape',
        required: true,
        description: 'Privilege escalation technique',
        options: [
          { label: 'Container Escape', value: 'container_escape' },
          { label: 'RBAC Abuse', value: 'rbac_abuse' },
          { label: 'Service Account Token', value: 'sa_token' },
        ],
      },
      {
        key: 'exploitPayload',
        label: 'Custom Payload',
        type: 'string',
        defaultValue: '',
        required: false,
        description: 'Optional custom exploit payload (leave empty for default)',
      },
    ],
    attackPhases: [
      {
        name: 'Recon',
        description: 'Enumerate permissions',
        technique: 'T1087',
        tactic: 'Discovery',
        duration: 20,
      },
      {
        name: 'Exploit',
        description: 'Attempt escalation',
        technique: 'T1548',
        tactic: 'Privilege Escalation',
        duration: 60,
      },
      {
        name: 'Verify',
        description: 'Check elevated access',
        technique: 'T1087',
        tactic: 'Discovery',
        duration: 15,
      },
    ],
    expectedDetections: [
      {
        source: 'SIEM',
        type: 'privilege_escalation',
        description: 'Privilege escalation attempt detected',
        confidence: 0.9,
      },
      {
        source: 'Runtime Security',
        type: 'container_escape',
        description: 'Container escape attempt',
        confidence: 0.85,
      },
    ],
    tags: ['privilege-escalation', 'rbac', 'container-security'],
    isOfficial: true,
    usageCount: 98,
    createdAt: '2024-02-15T00:00:00Z',
    updatedAt: '2024-03-12T00:00:00Z',
  },
  {
    id: 'tmpl-data-access',
    name: 'Unauthorized Data Access',
    description:
      'Attempts to access sensitive data stores to validate access controls and audit logging.',
    category: 'data',
    severity: 'high',
    icon: '🗃️',
    version: '1.0.0',
    author: 'Chaos-Sec Team',
    parameters: [
      {
        key: 'dataStore',
        label: 'Data Store Type',
        type: 'select',
        defaultValue: 's3',
        required: true,
        description: 'Type of data store to target',
        options: [
          { label: 'AWS S3', value: 's3' },
          { label: 'Azure Blob', value: 'blob' },
          { label: 'GCS Bucket', value: 'gcs' },
          { label: 'Database', value: 'db' },
        ],
      },
      {
        key: 'resourceName',
        label: 'Resource Name',
        type: 'string',
        defaultValue: '',
        required: true,
        description: 'Name of the target resource (bucket, table, etc.)',
      },
      {
        key: 'accessMethod',
        label: 'Access Method',
        type: 'select',
        defaultValue: 'api',
        required: false,
        description: 'Method to attempt access',
        options: [
          { label: 'API Call', value: 'api' },
          { label: 'Direct Connection', value: 'direct' },
          { label: 'Assume Role', value: 'assume_role' },
        ],
      },
    ],
    attackPhases: [
      {
        name: 'Probe',
        description: 'Discover data resources',
        technique: 'T1526',
        tactic: 'Discovery',
        duration: 15,
      },
      {
        name: 'Access',
        description: 'Attempt unauthorized read',
        technique: 'T1530',
        tactic: 'Collection',
        duration: 30,
      },
      {
        name: 'Exfiltrate',
        description: 'Attempt data copy',
        technique: 'T1537',
        tactic: 'Exfiltration',
        duration: 20,
      },
    ],
    expectedDetections: [
      {
        source: 'CloudTrail',
        type: 'unauthorized_access',
        description: 'Unauthorized access attempt logged',
        confidence: 0.95,
      },
      {
        source: 'SIEM',
        type: 'data_access_anomaly',
        description: 'Anomalous data access pattern',
        confidence: 0.8,
      },
    ],
    tags: ['data-access', 'cloud', 'audit', 'compliance'],
    isOfficial: true,
    usageCount: 134,
    createdAt: '2024-03-01T00:00:00Z',
    updatedAt: '2024-03-15T00:00:00Z',
  },
];
// ---------------------------------------------------------------------------
// Mock Data
// ---------------------------------------------------------------------------

const SIEM_ALERT_TYPES = [
  'DNS Anomaly',
  'Brute Force Attempt',
  'Privilege Escalation',
  'Data Exfiltration',
  'Network Lateral Movement',
  'Container Escape',
  'Unauthorized Access',
  'Service Degradation',
  'Configuration Change',
  'Malware Detection',
];

// ---------------------------------------------------------------------------
// Wizard State
// ---------------------------------------------------------------------------

interface WizardState {
  // Step 0: Template Selection
  selectedTemplateId: string | null;
  templateSearch: string;
  templateCategory: TemplateCategory | 'all';
  templateSeverity: TemplateSeverity | 'all';

  // Step 1: Configuration
  name: string;
  description: string;
  clusterId: string;
  namespace: string;
  parameters: Record<string, unknown>;
  tags: string[];
  tagInput: string;

  // Step 2: Validation Settings
  siemAlertType: string;
  timeWindowSeconds: number;
  expectedAlertCount: number;
  enableCustomRules: boolean;
  customRules: Record<string, string>;
  newRuleKey: string;
  newRuleValue: string;

  // Step 3: Review (no separate state, derived from above)
}

const initialWizardState: WizardState = {
  selectedTemplateId: null,
  templateSearch: '',
  templateCategory: 'all',
  templateSeverity: 'all',

  name: '',
  description: '',
  clusterId: '',
  namespace: '',
  parameters: {},
  tags: [],
  tagInput: '',

  siemAlertType: '',
  timeWindowSeconds: 300,
  expectedAlertCount: 1,
  enableCustomRules: false,
  customRules: {},
  newRuleKey: '',
  newRuleValue: '',
};

// ---------------------------------------------------------------------------
// Component: CreateExperimentPage
// ---------------------------------------------------------------------------

const CreateExperimentPage: React.FC = () => {
  const theme = useTheme();
  const navigate = useNavigate();
  const dispatch = useDispatch<AppDispatch>();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  const createStatus = useSelector(selectCreateStatus);
  const createError = useSelector(selectCreateError);

  const [activeStep, setActiveStep] = useState(0);
  const [showAdvancedParameters, setShowAdvancedParameters] = useState(false);
  const [launchState, setLaunchState] = useState<
    'idle' | 'waiting' | 'running' | 'failed'
  >('idle');
  const [launchMessage, setLaunchMessage] = useState<string | null>(null);
  const [launchError, setLaunchError] = useState<string | null>(null);
  const launchCancelRef = useRef(false);
  const [wizard, setWizard] = useState<WizardState>(initialWizardState);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [templates, setTemplates] = useState<AttackTemplate[]>(MOCK_TEMPLATES);
  const [clusters, setClusters] = useState<Cluster[]>(MOCK_CLUSTERS);

  React.useEffect(() => {
    let isMounted = true;

    const loadTemplates = async () => {
      try {
        const response = await templatesAPI.list();
        if (!isMounted) return;

        const loadedTemplates = (response.data.items ??
          response.data.data ??
          []) as AttackTemplate[];

        if (loadedTemplates.length > 0) {
          setTemplates(loadedTemplates);
        }
      } catch {
        // Keep fallback templates when the API is unavailable.
      }
    };

    const loadClusters = async () => {
      try {
        const response = await clustersAPI.list();
        if (!isMounted) return;

        const loadedClusters = (response.data.items ??
          response.data.data ??
          []) as typeof response.data.items;

        if (loadedClusters.length > 0) {
          setClusters(loadedClusters);
        }
      } catch {
        // Keep fallback clusters when the API is unavailable.
      }
    };

    loadTemplates();
    loadClusters();

    return () => {
      isMounted = false;
    };
  }, []);

  // -----------------------------------------------------------------------
  // Computed Values
  // -----------------------------------------------------------------------

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === wizard.selectedTemplateId) ?? null,
    [templates, wizard.selectedTemplateId],
  );

  const selectedCluster = useMemo(
    () => clusters.find((c) => c.id === wizard.clusterId) ?? null,
    [clusters, wizard.clusterId],
  );

  const filteredTemplates = useMemo(() => {
    let filtered = templates;

    if (wizard.templateCategory !== 'all') {
      filtered = filtered.filter((t) => t.category === wizard.templateCategory);
    }

    if (wizard.templateSeverity !== 'all') {
      filtered = filtered.filter((t) => t.severity === wizard.templateSeverity);
    }

    if (wizard.templateSearch.trim()) {
      const search = wizard.templateSearch.toLowerCase();
      filtered = filtered.filter(
        (t) =>
          t.name.toLowerCase().includes(search) ||
          t.description.toLowerCase().includes(search) ||
          t.tags.some((tag) => tag.toLowerCase().includes(search)),
      );
    }

    return filtered;
  }, [
    templates,
    wizard.templateCategory,
    wizard.templateSeverity,
    wizard.templateSearch,
  ]);

  const isSubmitting = createStatus === 'loading' || launchState !== 'idle';
  const isCreateLoading = createStatus === 'loading';
  const isLaunchWaiting = launchState === 'waiting';
  const isLaunchRunning = launchState === 'running';

  const isStepValid = useMemo(() => {
    switch (activeStep) {
      case 0:
        return wizard.selectedTemplateId !== null;
      case 1:
        return (
          wizard.name.trim().length >= 3 &&
          wizard.clusterId !== '' &&
          wizard.namespace !== '' &&
          Object.keys(fieldErrors).filter((k) => k.startsWith('step1.')).length === 0
        );
      case 2:
        return wizard.siemAlertType !== '';
      case 3:
        return true;
      default:
        return false;
    }
  }, [activeStep, wizard, fieldErrors]);

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const updateWizard = useCallback((updates: Partial<WizardState>) => {
    setWizard((prev) => ({ ...prev, ...updates }));
  }, []);

  const handleNext = useCallback(() => {
    // Validate current step before advancing
    const newErrors: Record<string, string> = {};

    if (activeStep === 0 && !wizard.selectedTemplateId) {
      newErrors['step0.template'] = 'Please select a template';
    }

    if (activeStep === 1) {
      if (wizard.name.trim().length < 3) {
        newErrors['step1.name'] = 'Name must be at least 3 characters';
      }
      if (!wizard.clusterId) {
        newErrors['step1.cluster'] = 'Please select a cluster';
      }
      if (!wizard.namespace) {
        newErrors['step1.namespace'] = 'Please select a namespace';
      }
      // Validate template parameters
      if (selectedTemplate) {
        selectedTemplate.parameters.forEach((param) => {
          if (
            param.required &&
            (wizard.parameters[param.key] === undefined ||
              wizard.parameters[param.key] === '')
          ) {
            newErrors[`step1.param.${param.key}`] = `${param.label} is required`;
          }
        });
      }
    }

    if (activeStep === 2 && !wizard.siemAlertType) {
      newErrors['step2.siemAlertType'] = 'Please select a SIEM alert type';
    }

    setFieldErrors((prev) => {
      const filtered = Object.fromEntries(
        Object.entries(prev).filter(([k]) => !k.startsWith(`step${activeStep}.`)),
      );
      return { ...filtered, ...newErrors };
    });

    if (Object.values(newErrors).every((v) => !v)) {
      setActiveStep((prev) => Math.min(prev + 1, STEPS.length - 1));
    }
  }, [activeStep, wizard, selectedTemplate]);

  const handleBack = useCallback(() => {
    setActiveStep((prev) => Math.max(prev - 1, 0));
  }, []);

  const handleCreate = useCallback(async () => {
    if (!selectedTemplate || !wizard.clusterId) return;

    launchCancelRef.current = false;
    setLaunchState('idle');
    setLaunchMessage(null);
    setLaunchError(null);

    const request: CreateExperimentRequest = {
      name: wizard.name.trim(),
      description: wizard.description.trim(),
      templateId: wizard.selectedTemplateId!,
      clusterId: wizard.clusterId,
      namespace: wizard.namespace,
      parameters: wizard.parameters,
      validation: {
        siemAlertType: wizard.siemAlertType,
        timeWindowSeconds: wizard.timeWindowSeconds,
        expectedAlertCount: wizard.expectedAlertCount,
        ...(wizard.enableCustomRules && { customRules: wizard.customRules }),
      },
      tags: wizard.tags.length > 0 ? wizard.tags : undefined,
    };

    try {
      const created = await dispatch(createExperiment(request)).unwrap();

      const maxAttempts = 60;
      for (let attempt = 0; attempt < maxAttempts; attempt++) {
        if (launchCancelRef.current) {
          setLaunchState('idle');
          setLaunchMessage(null);
          setLaunchError(null);
          return;
        }

        try {
          setLaunchState('running');
          setLaunchMessage('Starting experiment...');
          await experimentsAPI.execute(created.id, wizard.clusterId);
          navigate(`/experiments/${created.id}`);
          return;
        } catch (error) {
          const message = getErrorMessage(error);
          const isConcurrencyLimit =
            /concurrency_limit|Maximum concurrent experiments/i.test(message);
          if (!isConcurrencyLimit) {
            setLaunchState('failed');
            setLaunchError(message);
            return;
          }

          setLaunchState('waiting');
          setLaunchMessage(
            `Maximum concurrent experiments reached. Waiting for a free slot... (${attempt + 1}/${maxAttempts})`,
          );
          await new Promise((resolve) => setTimeout(resolve, 5000));
        }
      }

      if (launchCancelRef.current) {
        setLaunchState('idle');
        setLaunchMessage(null);
        setLaunchError(null);
        return;
      }

      setLaunchState('failed');
      setLaunchError(
        'Timed out waiting for a free slot. Please try again once another run finishes.',
      );
    } catch (error) {
      setLaunchState('failed');
      setLaunchError(getErrorMessage(error));
    }
  }, [dispatch, navigate, selectedTemplate, wizard]);

  const handleCancelLaunch = useCallback(() => {
    launchCancelRef.current = true;
    setLaunchState('idle');
    setLaunchMessage(null);
    setLaunchError(null);
  }, []);

  const handleCancel = useCallback(() => {
    navigate('/experiments');
  }, [navigate]);

  const handleTemplateSelect = useCallback(
    (templateId: string) => {
      const template = templates.find((t) => t.id === templateId);
      const defaultParams: Record<string, unknown> = {};
      if (template) {
        template.parameters.forEach((param) => {
          defaultParams[param.key] = param.defaultValue;
        });
      }

      setWizard((prev) => ({
        ...prev,
        selectedTemplateId: templateId,
        name: prev.name || template?.name || '',
        description: prev.description || template?.description || '',
        parameters: { ...defaultParams },
      }));

      // Clear template selection error
      setFieldErrors((prev) => {
        const { ['step0.template']: _, ...rest } = prev;
        return rest;
      });
    },
    [templates],
  );

  const handleParameterChange = useCallback((key: string, value: unknown) => {
    setWizard((prev) => ({
      ...prev,
      parameters: { ...prev.parameters, [key]: value },
    }));
    // Clear param error
    setFieldErrors((prev) => {
      const { [`step1.param.${key}`]: _, ...rest } = prev;
      return rest;
    });
  }, []);

  const handleAddTag = useCallback(() => {
    const tag = wizard.tagInput.trim().toLowerCase();
    if (tag && !wizard.tags.includes(tag)) {
      updateWizard({ tags: [...wizard.tags, tag], tagInput: '' });
    }
  }, [wizard.tagInput, wizard.tags, updateWizard]);

  const handleRemoveTag = useCallback(
    (tag: string) => {
      updateWizard({ tags: wizard.tags.filter((t) => t !== tag) });
    },
    [updateWizard],
  );

  const handleAddCustomRule = useCallback(() => {
    const key = wizard.newRuleKey.trim();
    const value = wizard.newRuleValue.trim();
    if (key && value) {
      updateWizard({
        customRules: { ...wizard.customRules, [key]: value },
        newRuleKey: '',
        newRuleValue: '',
      });
    }
  }, [wizard.newRuleKey, wizard.newRuleValue, wizard.customRules, updateWizard]);

  const handleRemoveCustomRule = useCallback(
    (key: string) => {
      const { [key]: _, ...rest } = wizard.customRules;
      updateWizard({ customRules: rest });
    },
    [wizard.customRules, updateWizard],
  );

  const handleClusterChange = useCallback(
    (clusterId: string) => {
      const cluster = clusters.find((c) => c.id === clusterId);
      setWizard((prev) => ({
        ...prev,
        clusterId,
        namespace: cluster?.namespaces.includes('chaos-sec')
          ? 'chaos-sec'
          : (cluster?.namespaces[0] ?? ''),
      }));
      setFieldErrors((prev) => {
        const { ['step1.cluster']: __, ['step1.namespace']: ___, ...rest } = prev;
        return rest;
      });
    },
    [clusters],
  );

  // Reset create status on unmount
  React.useEffect(() => {
    return () => {
      dispatch(resetCreateStatus());
    };
  }, [dispatch]);

  // -----------------------------------------------------------------------
  // Step Renderers
  // -----------------------------------------------------------------------

  const renderTemplateSelection = () => (
    <Box>
      <Typography variant="h6" fontWeight={700} sx={{ mb: 0.5 }}>
        Select an Attack Template
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Choose a pre-built template that defines the attack scenario, parameters, and
        expected detections.
      </Typography>

      {fieldErrors['step0.template'] && (
        <Alert severity="error" sx={{ mb: 2, borderRadius: 2 }}>
          {fieldErrors['step0.template']}
        </Alert>
      )}

      {/* Search & Filters */}
      <Stack
        direction={{ xs: 'column', md: 'row' }}
        spacing={1.5}
        sx={{ mb: 2.5 }}
        alignItems={{ xs: 'stretch', md: 'center' }}
      >
        <TextField
          size="small"
          placeholder="Search templates..."
          value={wizard.templateSearch}
          onChange={(e) => updateWizard({ templateSearch: e.target.value })}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
              </InputAdornment>
            ),
            endAdornment: wizard.templateSearch ? (
              <InputAdornment position="end">
                <IconButton
                  size="small"
                  onClick={() => updateWizard({ templateSearch: '' })}
                >
                  <ClearIcon sx={{ fontSize: 16 }} />
                </IconButton>
              </InputAdornment>
            ) : null,
          }}
          sx={{ flex: 1, minWidth: { xs: '100%', md: 260 } }}
        />

        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Category</InputLabel>
          <Select
            value={wizard.templateCategory}
            label="Category"
            onChange={(e) =>
              updateWizard({
                templateCategory: e.target.value as TemplateCategory | 'all',
              })
            }
          >
            {CATEGORY_OPTIONS.map((opt) => (
              <MenuItem key={opt.value} value={opt.value}>
                {opt.label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 130 }}>
          <InputLabel>Severity</InputLabel>
          <Select
            value={wizard.templateSeverity}
            label="Severity"
            onChange={(e) =>
              updateWizard({
                templateSeverity: e.target.value as TemplateSeverity | 'all',
              })
            }
          >
            <MenuItem value="all">All</MenuItem>
            <MenuItem value="low">Low</MenuItem>
            <MenuItem value="medium">Medium</MenuItem>
            <MenuItem value="high">High</MenuItem>
            <MenuItem value="critical">Critical</MenuItem>
          </Select>
        </FormControl>
      </Stack>

      {/* Template Cards Grid */}
      {filteredTemplates.length === 0 ? (
        <Box sx={{ py: 6, textAlign: 'center' }}>
          <TemplateIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
          <Typography variant="body1" color="text.secondary">
            No templates match your filters.
          </Typography>
          <Button
            variant="text"
            onClick={() =>
              updateWizard({
                templateSearch: '',
                templateCategory: 'all',
                templateSeverity: 'all',
              })
            }
            sx={{ mt: 1 }}
          >
            Clear filters
          </Button>
        </Box>
      ) : (
        <Grid container spacing={2}>
          {filteredTemplates.map((template) => {
            const isSelected = wizard.selectedTemplateId === template.id;
            return (
              <Grid item xs={12} sm={6} md={4} key={template.id}>
                <Card
                  sx={{
                    height: '100%',
                    cursor: 'pointer',
                    position: 'relative',
                    transition: 'all 200ms cubic-bezier(0.4, 0, 0.2, 1)',
                    border: '2px solid',
                    borderColor: isSelected ? 'primary.main' : 'divider',
                    backgroundColor: isSelected
                      ? 'rgba(37, 99, 235, 0.03)'
                      : 'background.paper',
                    '&:hover': {
                      borderColor: isSelected ? 'primary.main' : 'primary.light',
                      transform: 'translateY(-2px)',
                      boxShadow: isSelected
                        ? `0 4px 16px ${theme.palette.primary.main}25`
                        : '0 4px 16px rgba(0,0,0,0.08)',
                    },
                  }}
                  onClick={() => handleTemplateSelect(template.id)}
                >
                  {isSelected && (
                    <Box
                      sx={{
                        position: 'absolute',
                        top: 8,
                        right: 8,
                        zIndex: 1,
                      }}
                    >
                      <CheckIcon
                        sx={{
                          fontSize: 24,
                          color: 'primary.main',
                          backgroundColor: 'background.paper',
                          borderRadius: '50%',
                        }}
                      />
                    </Box>
                  )}

                  <CardContent sx={{ p: 2.5, '&:last-child': { pb: 2.5 } }}>
                    {/* Template Header */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start" mb={1.5}>
                      <Avatar
                        variant="rounded"
                        sx={{
                          width: 44,
                          height: 44,
                          fontSize: '1.25rem',
                          backgroundColor: `${SEVERITY_COLORS[template.severity]}14`,
                        }}
                      >
                        {template.icon}
                      </Avatar>
                      <Box sx={{ minWidth: 0, flex: 1 }}>
                        <Typography variant="subtitle2" fontWeight={700} noWrap>
                          {template.name}
                        </Typography>
                        <Stack direction="row" spacing={0.5} mt={0.5}>
                          <Chip
                            label={template.severity}
                            size="small"
                            sx={{
                              height: 20,
                              fontSize: '0.625rem',
                              fontWeight: 600,
                              color: SEVERITY_COLORS[template.severity],
                              backgroundColor: `${SEVERITY_COLORS[template.severity]}14`,
                              border: `1px solid ${SEVERITY_COLORS[template.severity]}30`,
                            }}
                          />
                          <Chip
                            label={template.category}
                            size="small"
                            variant="outlined"
                            sx={{
                              height: 20,
                              fontSize: '0.625rem',
                              textTransform: 'capitalize',
                            }}
                          />
                        </Stack>
                      </Box>
                    </Stack>

                    {/* Description */}
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{
                        mb: 1.5,
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        overflow: 'hidden',
                        fontSize: '0.8125rem',
                        lineHeight: 1.5,
                      }}
                    >
                      {template.description}
                    </Typography>

                    {/* Meta */}
                    <Stack
                      direction="row"
                      justifyContent="space-between"
                      alignItems="center"
                    >
                      <Typography
                        variant="caption"
                        color="text.disabled"
                        sx={{ fontSize: '0.6875rem' }}
                      >
                        {template.usageCount} uses · v{template.version}
                      </Typography>
                      {template.isOfficial && (
                        <Chip
                          icon={<ShieldIcon sx={{ fontSize: '12px !important' }} />}
                          label="Official"
                          size="small"
                          color="primary"
                          variant="outlined"
                          sx={{ height: 20, fontSize: '0.625rem' }}
                        />
                      )}
                    </Stack>

                    {/* Tags */}
                    <Stack
                      direction="row"
                      spacing={0.5}
                      mt={1.5}
                      flexWrap="wrap"
                      useFlexGap
                    >
                      {(template.tags ?? []).slice(0, 3).map((tag) => (
                        <Chip
                          key={tag}
                          label={tag}
                          size="small"
                          variant="outlined"
                          sx={{ height: 20, fontSize: '0.625rem' }}
                        />
                      ))}
                      {(template.tags?.length ?? 0) > 3 && (
                        <Chip
                          label={`+${(template.tags?.length ?? 0) - 3}`}
                          size="small"
                          variant="outlined"
                          sx={{ height: 20, fontSize: '0.625rem' }}
                        />
                      )}
                    </Stack>
                  </CardContent>
                </Card>
              </Grid>
            );
          })}
        </Grid>
      )}
    </Box>
  );

  const renderConfiguration = () => {
    if (!selectedTemplate) {
      return (
        <Alert severity="warning" sx={{ borderRadius: 2 }}>
          Please select a template first.
        </Alert>
      );
    }

    return (
      <Box>
        <Typography variant="h6" fontWeight={700} sx={{ mb: 0.5 }}>
          Configure Experiment
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          Set the few important options needed to run this test.
        </Typography>

        <Grid container spacing={3}>
          {/* Experiment Name */}
          <Grid item xs={12}>
            <TextField
              label="Experiment Name"
              value={wizard.name}
              onChange={(e) => {
                updateWizard({ name: e.target.value });
                if (e.target.value.trim().length >= 3) {
                  setFieldErrors((prev) => {
                    const { ['step1.name']: _, ...rest } = prev;
                    return rest;
                  });
                }
              }}
              error={Boolean(fieldErrors['step1.name'])}
              helperText={
                fieldErrors['step1.name'] || 'Give your experiment a descriptive name'
              }
              fullWidth
              required
              placeholder="e.g., DNS Exfil Test - Prod US East"
            />
          </Grid>

          {/* Description */}
          <Grid item xs={12}>
            <TextField
              label="Description"
              value={wizard.description}
              onChange={(e) => updateWizard({ description: e.target.value })}
              fullWidth
              multiline
              rows={2}
              placeholder="Optional description of the experiment's purpose and expected outcomes"
            />
          </Grid>

          {/* Target Cluster */}
          <Grid item xs={12} sm={6}>
            <FormControl fullWidth required error={Boolean(fieldErrors['step1.cluster'])}>
              <InputLabel>Target Cluster</InputLabel>
              <Select
                value={wizard.clusterId}
                label="Target Cluster"
                onChange={(e) => handleClusterChange(e.target.value)}
              >
                {clusters.map((cluster) => (
                  <MenuItem key={cluster.id} value={cluster.id}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <ClusterIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                      <Box>
                        <Typography variant="body2" fontWeight={500}>
                          {cluster.name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {cluster.provider} · {cluster.region} · {cluster.nodeCount}{' '}
                          nodes
                        </Typography>
                      </Box>
                      <Box sx={{ ml: 'auto' }}>
                        <StatusBadge
                          status={cluster.status}
                          variant="dot"
                          size="small"
                          showLabel={false}
                        />
                      </Box>
                    </Stack>
                  </MenuItem>
                ))}
              </Select>
              {fieldErrors['step1.cluster'] && (
                <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 2 }}>
                  {fieldErrors['step1.cluster']}
                </Typography>
              )}
            </FormControl>
          </Grid>

          {/* Namespace */}
          <Grid item xs={12} sm={6}>
            <FormControl
              fullWidth
              required
              error={Boolean(fieldErrors['step1.namespace'])}
            >
              <InputLabel>Where to run it</InputLabel>
              <Select
                value={wizard.namespace}
                label="Where to run it"
                onChange={(e) => {
                  updateWizard({ namespace: e.target.value });
                  setFieldErrors((prev) => {
                    const { ['step1.namespace']: _, ...rest } = prev;
                    return rest;
                  });
                }}
                disabled={!wizard.clusterId}
              >
                {selectedCluster?.namespaces.map((ns) => (
                  <MenuItem key={ns} value={ns}>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <NamespaceIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                      <Typography variant="body2">{ns}</Typography>
                      {ns === 'chaos-sec' && (
                        <Chip
                          label="recommended"
                          size="small"
                          color="primary"
                          sx={{ height: 18, fontSize: '0.625rem' }}
                        />
                      )}
                    </Stack>
                  </MenuItem>
                )) ?? (
                  <MenuItem disabled value="">
                    Select a cluster first
                  </MenuItem>
                )}
              </Select>
              {fieldErrors['step1.namespace'] && (
                <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 2 }}>
                  {fieldErrors['step1.namespace']}
                </Typography>
              )}
            </FormControl>
          </Grid>

          {/* Tags */}
          <Grid item xs={12}>
            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
              Tags
            </Typography>
            <Stack direction="row" spacing={1} alignItems="center" mb={1}>
              <TextField
                size="small"
                placeholder="Add a tag..."
                value={wizard.tagInput}
                onChange={(e) => updateWizard({ tagInput: e.target.value })}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleAddTag();
                  }
                }}
                sx={{ flex: 1 }}
              />
              <Button
                variant="outlined"
                size="small"
                onClick={handleAddTag}
                disabled={!wizard.tagInput.trim()}
                startIcon={<AddIcon />}
              >
                Add
              </Button>
            </Stack>
            <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
              {wizard.tags.map((tag) => (
                <Chip
                  key={tag}
                  label={tag}
                  size="small"
                  onDelete={() => handleRemoveTag(tag)}
                  sx={{ mb: 0.5 }}
                />
              ))}
              {wizard.tags.length === 0 && (
                <Typography variant="caption" color="text.disabled">
                  No tags added yet
                </Typography>
              )}
            </Stack>
          </Grid>

          {/* Template Parameters */}
          {selectedTemplate.parameters.length > 0 && (
            <Grid item xs={12}>
              <Divider sx={{ mb: 2 }} />
              <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 0.5 }}>
                Important settings
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: 'block', mb: 2 }}
              >
                Fill in the main fields first. Extra settings are hidden unless you need
                them.
              </Typography>

              <Grid container spacing={2}>
                {selectedTemplate.parameters
                  .filter((param) => !isAdvancedParameter(param))
                  .map((param) => (
                    <Grid item xs={12} sm={6} key={param.key}>
                      {renderParameterField(param)}
                    </Grid>
                  ))}
              </Grid>

              {selectedTemplate.parameters.some((param) =>
                isAdvancedParameter(param),
              ) && (
                <Box sx={{ mt: 2 }}>
                  <Button
                    size="small"
                    onClick={() => setShowAdvancedParameters((prev) => !prev)}
                    sx={{ textTransform: 'none', fontWeight: 600 }}
                  >
                    {showAdvancedParameters
                      ? 'Hide extra settings'
                      : 'Show extra settings'}
                  </Button>
                  {showAdvancedParameters && (
                    <Grid container spacing={2} sx={{ mt: 1 }}>
                      {selectedTemplate.parameters
                        .filter((param) => isAdvancedParameter(param))
                        .map((param) => (
                          <Grid item xs={12} sm={6} key={param.key}>
                            {renderParameterField(param)}
                          </Grid>
                        ))}
                    </Grid>
                  )}
                </Box>
              )}
            </Grid>
          )}
        </Grid>
      </Box>
    );
  };

  const renderParameterField = (param: TemplateParameter) => {
    const value = wizard.parameters[param.key] ?? param.defaultValue;
    const error = fieldErrors[`step1.param.${param.key}`];
    const display = getParameterCopy(param);

    switch (param.type) {
      case 'select':
        return (
          <FormControl fullWidth required={param.required} error={Boolean(error)}>
            <InputLabel>{display.label}</InputLabel>
            <Select
              value={String(value)}
              label={display.label}
              onChange={(e) => handleParameterChange(param.key, e.target.value)}
            >
              {param.options?.map((opt) => (
                <MenuItem key={String(opt.value)} value={String(opt.value)}>
                  {opt.label}
                </MenuItem>
              ))}
            </Select>
            {error && (
              <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 2 }}>
                {error}
              </Typography>
            )}
            {!error && display.description && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 0.5, ml: 2, display: 'block' }}
              >
                {display.description}
              </Typography>
            )}
          </FormControl>
        );

      case 'boolean':
        return (
          <FormControl component="fieldset">
            <FormControlLabel
              control={
                <Switch
                  checked={Boolean(value)}
                  onChange={(e) => handleParameterChange(param.key, e.target.checked)}
                  color="primary"
                />
              }
              label={
                <Box>
                  <Typography variant="body2" fontWeight={500}>
                    {display.label}
                  </Typography>
                  {display.description && (
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ display: 'block' }}
                    >
                      {display.description}
                    </Typography>
                  )}
                </Box>
              }
            />
          </FormControl>
        );

      case 'number':
        return (
          <Box>
            <Typography variant="body2" fontWeight={500} sx={{ mb: 0.5 }}>
              {display.label} {param.required && '*'}
            </Typography>
            {param.validation?.min !== undefined &&
            param.validation?.max !== undefined &&
            param.validation.max - param.validation.min <= 100 ? (
              <Box sx={{ px: 1 }}>
                <Slider
                  value={Number(value)}
                  onChange={(_, newVal) => handleParameterChange(param.key, newVal)}
                  min={param.validation.min}
                  max={param.validation.max}
                  valueLabelDisplay="auto"
                  valueLabelFormat={(v) =>
                    `${v}${param.key.toLowerCase().includes('interval') || param.key.toLowerCase().includes('duration') || param.key.toLowerCase().includes('timeout') ? 's' : ''}`
                  }
                  marks={[
                    { value: param.validation.min, label: String(param.validation.min) },
                    { value: param.validation.max, label: String(param.validation.max) },
                  ]}
                />
              </Box>
            ) : (
              <TextField
                type="number"
                value={String(value)}
                onChange={(e) => handleParameterChange(param.key, Number(e.target.value))}
                fullWidth
                size="small"
                required={param.required}
                error={Boolean(error)}
                helperText={error || display.description}
                InputProps={{
                  inputProps: {
                    min: param.validation?.min,
                    max: param.validation?.max,
                  },
                }}
              />
            )}
          </Box>
        );

      default: // string
        return (
          <TextField
            label={display.label}
            value={String(value)}
            onChange={(e) => handleParameterChange(param.key, e.target.value)}
            fullWidth
            required={param.required}
            error={Boolean(error)}
            helperText={error || display.description}
            placeholder={`Enter ${display.label.toLowerCase()}`}
          />
        );
    }
  };

  const renderValidationSettings = () => (
    <Box>
      <Typography variant="h6" fontWeight={700} sx={{ mb: 0.5 }}>
        Validation Settings
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Define how the SIEM should validate the security control detection for this
        experiment.
      </Typography>

      <Grid container spacing={3}>
        {/* SIEM Alert Type */}
        <Grid item xs={12} sm={6}>
          <FormControl
            fullWidth
            required
            error={Boolean(fieldErrors['step2.siemAlertType'])}
          >
            <InputLabel>Expected SIEM Alert Type</InputLabel>
            <Select
              value={wizard.siemAlertType}
              label="Expected SIEM Alert Type"
              onChange={(e) => {
                updateWizard({ siemAlertType: e.target.value });
                setFieldErrors((prev) => {
                  const { ['step2.siemAlertType']: _, ...rest } = prev;
                  return rest;
                });
              }}
            >
              {SIEM_ALERT_TYPES.map((type) => (
                <MenuItem key={type} value={type}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <ValidationIcon sx={{ fontSize: 18, color: 'primary.main' }} />
                    {type}
                  </Stack>
                </MenuItem>
              ))}
            </Select>
            {fieldErrors['step2.siemAlertType'] && (
              <Typography variant="caption" color="error" sx={{ mt: 0.5, ml: 2 }}>
                {fieldErrors['step2.siemAlertType']}
              </Typography>
            )}
          </FormControl>
        </Grid>

        {/* Expected Alert Count */}
        <Grid item xs={12} sm={6}>
          <TextField
            label="Expected Alert Count"
            type="number"
            value={wizard.expectedAlertCount}
            onChange={(e) =>
              updateWizard({ expectedAlertCount: Math.max(1, Number(e.target.value)) })
            }
            fullWidth
            InputProps={{
              inputProps: { min: 1, max: 50 },
              startAdornment: (
                <InputAdornment position="start">
                  <ShieldIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                </InputAdornment>
              ),
            }}
            helperText="How many alerts you expect from the SIEM"
          />
        </Grid>

        {/* Time Window */}
        <Grid item xs={12}>
          <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
            Detection Time Window
          </Typography>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: 'block', mb: 2 }}
          >
            Maximum time to wait for SIEM alerts after the attack is executed.
          </Typography>
          <Box sx={{ px: 1 }}>
            <Slider
              value={wizard.timeWindowSeconds}
              onChange={(_, value) =>
                updateWizard({ timeWindowSeconds: value as number })
              }
              min={30}
              max={3600}
              step={30}
              valueLabelDisplay="auto"
              valueLabelFormat={(v) => {
                if (v >= 3600) return `${(v / 3600).toFixed(1)}h`;
                if (v >= 60) return `${Math.round(v / 60)}m`;
                return `${v}s`;
              }}
              marks={[
                { value: 60, label: '1m' },
                { value: 300, label: '5m' },
                { value: 600, label: '10m' },
                { value: 1800, label: '30m' },
                { value: 3600, label: '1h' },
              ]}
            />
            <Stack direction="row" justifyContent="space-between" mt={1}>
              <Typography variant="caption" color="text.secondary">
                Selected:{' '}
                {wizard.timeWindowSeconds >= 3600
                  ? `${(wizard.timeWindowSeconds / 3600).toFixed(1)}h`
                  : wizard.timeWindowSeconds >= 60
                    ? `${Math.floor(wizard.timeWindowSeconds / 60)}m ${wizard.timeWindowSeconds % 60 > 0 ? `${wizard.timeWindowSeconds % 60}s` : ''}`
                    : `${wizard.timeWindowSeconds}s`}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {wizard.timeWindowSeconds < 300 ? (
                  <Stack
                    direction="row"
                    spacing={0.5}
                    alignItems="center"
                    component="span"
                  >
                    <WarningIcon sx={{ fontSize: 14, color: 'warning.main' }} />
                    <span style={{ color: theme.palette.warning.main }}>
                      Short window may miss delayed detections
                    </span>
                  </Stack>
                ) : null}
              </Typography>
            </Stack>
          </Box>
        </Grid>

        {/* Custom Rules Toggle */}
        <Grid item xs={12}>
          <Divider sx={{ mb: 2 }} />
          <FormControlLabel
            control={
              <Switch
                checked={wizard.enableCustomRules}
                onChange={(e) => updateWizard({ enableCustomRules: e.target.checked })}
                color="primary"
              />
            }
            label={
              <Box>
                <Typography variant="body2" fontWeight={600}>
                  Custom Validation Rules
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  Add custom key-value rules for advanced SIEM validation.
                </Typography>
              </Box>
            }
          />

          {wizard.enableCustomRules && (
            <Box sx={{ mt: 2 }}>
              <Stack direction="row" spacing={1} mb={1.5}>
                <TextField
                  size="small"
                  placeholder="Rule key"
                  value={wizard.newRuleKey}
                  onChange={(e) => updateWizard({ newRuleKey: e.target.value })}
                  sx={{ flex: 1 }}
                />
                <TextField
                  size="small"
                  placeholder="Rule value"
                  value={wizard.newRuleValue}
                  onChange={(e) => updateWizard({ newRuleValue: e.target.value })}
                  sx={{ flex: 1 }}
                />
                <Button
                  variant="outlined"
                  size="small"
                  onClick={handleAddCustomRule}
                  disabled={!wizard.newRuleKey.trim() || !wizard.newRuleValue.trim()}
                  startIcon={<AddIcon />}
                >
                  Add
                </Button>
              </Stack>

              {Object.keys(wizard.customRules).length > 0 ? (
                <TableContainer
                  component={Paper}
                  variant="outlined"
                  sx={{ borderRadius: 1.5 }}
                >
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Key</TableCell>
                        <TableCell>Value</TableCell>
                        <TableCell width={60} align="center">
                          Action
                        </TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {Object.entries(wizard.customRules).map(([key, val]) => (
                        <TableRow key={key}>
                          <TableCell>
                            <Typography
                              variant="body2"
                              sx={{ fontFamily: 'monospace', fontSize: '0.8125rem' }}
                            >
                              {key}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography
                              variant="body2"
                              sx={{ fontFamily: 'monospace', fontSize: '0.8125rem' }}
                            >
                              {val}
                            </Typography>
                          </TableCell>
                          <TableCell align="center">
                            <IconButton
                              size="small"
                              color="error"
                              onClick={() => handleRemoveCustomRule(key)}
                            >
                              <ClearIcon sx={{ fontSize: 16 }} />
                            </IconButton>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              ) : (
                <Typography
                  variant="caption"
                  color="text.disabled"
                  sx={{ display: 'block', textAlign: 'center', py: 2 }}
                >
                  No custom rules added yet
                </Typography>
              )}
            </Box>
          )}
        </Grid>

        {/* Expected Detections (from template) */}
        {selectedTemplate && selectedTemplate.expectedDetections.length > 0 && (
          <Grid item xs={12}>
            <Divider sx={{ mb: 2 }} />
            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
              Expected Detections (from template)
            </Typography>
            <Stack spacing={1}>
              {selectedTemplate.expectedDetections.map((det, idx) => (
                <Paper key={idx} variant="outlined" sx={{ p: 1.5, borderRadius: 1.5 }}>
                  <Stack
                    direction="row"
                    justifyContent="space-between"
                    alignItems="center"
                  >
                    <Box>
                      <Stack direction="row" spacing={1} alignItems="center" mb={0.5}>
                        <Typography variant="body2" fontWeight={600}>
                          {det.source}
                        </Typography>
                        <Chip
                          label={det.type}
                          size="small"
                          variant="outlined"
                          sx={{
                            height: 20,
                            fontSize: '0.625rem',
                            fontFamily: 'monospace',
                          }}
                        />
                      </Stack>
                      <Typography variant="caption" color="text.secondary">
                        {det.description}
                      </Typography>
                    </Box>
                    <Chip
                      label={`${Math.round(det.confidence * 100)}% confidence`}
                      size="small"
                      sx={{
                        height: 22,
                        fontSize: '0.6875rem',
                        color:
                          det.confidence >= 0.8
                            ? 'success.main'
                            : det.confidence >= 0.5
                              ? 'warning.main'
                              : 'error.main',
                        backgroundColor:
                          det.confidence >= 0.8
                            ? 'rgba(16,185,129,0.08)'
                            : det.confidence >= 0.5
                              ? 'rgba(245,158,11,0.08)'
                              : 'rgba(239,68,68,0.08)',
                      }}
                    />
                  </Stack>
                </Paper>
              ))}
            </Stack>
          </Grid>
        )}
      </Grid>
    </Box>
  );

  const renderReview = () => (
    <Box>
      <Typography variant="h6" fontWeight={700} sx={{ mb: 0.5 }}>
        Review & Create
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Review your experiment configuration before creating it.
      </Typography>

      {/* Error Display */}
      {createError && (
        <Alert severity="error" sx={{ mb: 3, borderRadius: 2 }}>
          <AlertTitle>Creation Failed</AlertTitle>
          {createError}
        </Alert>
      )}

      <Grid container spacing={2.5}>
        {/* Template Summary */}
        <Grid item xs={12} md={6}>
          <Paper variant="outlined" sx={{ p: 2.5, borderRadius: 2, height: '100%' }}>
            <Stack direction="row" spacing={1} alignItems="center" mb={2}>
              <TemplateIcon sx={{ fontSize: 20, color: 'primary.main' }} />
              <Typography variant="subtitle2" fontWeight={700}>
                Template
              </Typography>
            </Stack>
            {selectedTemplate ? (
              <Stack spacing={1.5}>
                <Stack direction="row" spacing={1.5} alignItems="center">
                  <Avatar
                    variant="rounded"
                    sx={{
                      width: 40,
                      height: 40,
                      fontSize: '1.25rem',
                      backgroundColor: `${SEVERITY_COLORS[selectedTemplate.severity]}14`,
                    }}
                  >
                    {selectedTemplate.icon}
                  </Avatar>
                  <Box>
                    <Typography variant="body2" fontWeight={700}>
                      {selectedTemplate.name}
                    </Typography>
                    <Stack direction="row" spacing={0.5} mt={0.25}>
                      <Chip
                        label={selectedTemplate.severity}
                        size="small"
                        sx={{
                          height: 20,
                          fontSize: '0.625rem',
                          color: SEVERITY_COLORS[selectedTemplate.severity],
                          backgroundColor: `${SEVERITY_COLORS[selectedTemplate.severity]}14`,
                        }}
                      />
                      <Chip
                        label={selectedTemplate.category}
                        size="small"
                        variant="outlined"
                        sx={{
                          height: 20,
                          fontSize: '0.625rem',
                          textTransform: 'capitalize',
                        }}
                      />
                    </Stack>
                  </Box>
                </Stack>
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ fontSize: '0.8125rem' }}
                >
                  {selectedTemplate.description}
                </Typography>
                <Typography variant="caption" color="text.disabled">
                  {selectedTemplate.attackPhases.length} phases ·{' '}
                  {selectedTemplate.expectedDetections.length} expected detections
                </Typography>
              </Stack>
            ) : (
              <Typography variant="body2" color="error">
                No template selected
              </Typography>
            )}
          </Paper>
        </Grid>

        {/* Configuration Summary */}
        <Grid item xs={12} md={6}>
          <Paper variant="outlined" sx={{ p: 2.5, borderRadius: 2, height: '100%' }}>
            <Stack direction="row" spacing={1} alignItems="center" mb={2}>
              <ConfigIcon sx={{ fontSize: 20, color: 'primary.main' }} />
              <Typography variant="subtitle2" fontWeight={700}>
                Configuration
              </Typography>
            </Stack>
            <Stack spacing={1.5}>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: 'block' }}
                >
                  Name
                </Typography>
                <Typography variant="body2" fontWeight={600}>
                  {wizard.name}
                </Typography>
              </Box>
              {wizard.description && (
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block' }}
                  >
                    Description
                  </Typography>
                  <Typography variant="body2">{wizard.description}</Typography>
                </Box>
              )}
              <Divider />
              <Stack direction="row" spacing={3}>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block' }}
                  >
                    Cluster
                  </Typography>
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    <ClusterIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                    <Typography variant="body2" fontWeight={500}>
                      {selectedCluster?.name ?? '—'}
                    </Typography>
                  </Stack>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block' }}
                  >
                    Namespace
                  </Typography>
                  <Typography
                    variant="body2"
                    sx={{ fontFamily: 'monospace', fontSize: '0.8125rem' }}
                  >
                    {wizard.namespace || '—'}
                  </Typography>
                </Box>
              </Stack>
              {Object.keys(wizard.parameters).length > 0 && (
                <>
                  <Divider />
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: 'block', mb: 0.5 }}
                  >
                    Parameters
                  </Typography>
                  <Stack spacing={0.5}>
                    {Object.entries(wizard.parameters).map(([key, value]) => (
                      <Stack key={key} direction="row" spacing={1}>
                        <Typography
                          variant="caption"
                          fontWeight={600}
                          sx={{
                            fontFamily: 'monospace',
                            minWidth: 140,
                            color: 'text.secondary',
                          }}
                        >
                          {key}:
                        </Typography>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                          {String(value)}
                        </Typography>
                      </Stack>
                    ))}
                  </Stack>
                </>
              )}
              {wizard.tags.length > 0 && (
                <>
                  <Divider />
                  <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                    {wizard.tags.map((tag) => (
                      <Chip
                        key={tag}
                        label={tag}
                        size="small"
                        variant="outlined"
                        sx={{ fontSize: '0.6875rem' }}
                      />
                    ))}
                  </Stack>
                </>
              )}
            </Stack>
          </Paper>
        </Grid>

        {/* Validation Summary */}
        <Grid item xs={12}>
          <Paper variant="outlined" sx={{ p: 2.5, borderRadius: 2 }}>
            <Stack direction="row" spacing={1} alignItems="center" mb={2}>
              <ValidationIcon sx={{ fontSize: 20, color: 'primary.main' }} />
              <Typography variant="subtitle2" fontWeight={700}>
                Validation Settings
              </Typography>
            </Stack>
            <Grid container spacing={2}>
              <Grid item xs={12} sm={4}>
                <Box
                  sx={{
                    textAlign: 'center',
                    p: 1.5,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                  }}
                >
                  <Typography variant="h5" fontWeight={700} color="primary.main">
                    {wizard.expectedAlertCount}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Expected Alerts
                  </Typography>
                </Box>
              </Grid>
              <Grid item xs={12} sm={4}>
                <Box
                  sx={{
                    textAlign: 'center',
                    p: 1.5,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                  }}
                >
                  <Typography variant="h5" fontWeight={700} color="primary.main">
                    {wizard.timeWindowSeconds >= 3600
                      ? `${(wizard.timeWindowSeconds / 3600).toFixed(1)}h`
                      : wizard.timeWindowSeconds >= 60
                        ? `${Math.floor(wizard.timeWindowSeconds / 60)}m`
                        : `${wizard.timeWindowSeconds}s`}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Time Window
                  </Typography>
                </Box>
              </Grid>
              <Grid item xs={12} sm={4}>
                <Box
                  sx={{
                    textAlign: 'center',
                    p: 1.5,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'divider',
                  }}
                >
                  <Typography
                    variant="body2"
                    fontWeight={700}
                    color="primary.main"
                    sx={{ fontSize: '1rem', lineHeight: 1.6 }}
                  >
                    {wizard.siemAlertType || '—'}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Alert Type
                  </Typography>
                </Box>
              </Grid>
            </Grid>
            {wizard.enableCustomRules && Object.keys(wizard.customRules).length > 0 && (
              <Box sx={{ mt: 2 }}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: 'block', mb: 0.5 }}
                >
                  Custom Rules
                </Typography>
                <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                  {Object.entries(wizard.customRules).map(([key, val]) => (
                    <Chip
                      key={key}
                      label={`${key}=${val}`}
                      size="small"
                      variant="outlined"
                      sx={{ fontSize: '0.6875rem', fontFamily: 'monospace' }}
                    />
                  ))}
                </Stack>
              </Box>
            )}
          </Paper>
        </Grid>
      </Grid>
    </Box>
  );

  // -----------------------------------------------------------------------
  // Main Render
  // -----------------------------------------------------------------------

  return (
    <Box sx={{ maxWidth: 1100, mx: 'auto' }}>
      {/* Page Header */}
      <Stack direction="row" justifyContent="space-between" alignItems="center" mb={3}>
        <Box>
          <Typography variant="h4" fontWeight={800} gutterBottom>
            Create Experiment
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Configure and launch a new security control validation experiment.
          </Typography>
        </Box>
        <Button
          variant="text"
          onClick={handleCancel}
          sx={{ textTransform: 'none', color: 'text.secondary' }}
        >
          Cancel
        </Button>
      </Stack>

      {/* Horizontal Stepper (desktop) */}
      {!isMobile && (
        <Paper variant="outlined" sx={{ borderRadius: 2, mb: 3, overflow: 'hidden' }}>
          <Stepper
            activeStep={activeStep}
            alternativeLabel
            sx={{
              py: 3,
              px: 2,
              '& .MuiStepLabel-label': {
                fontWeight: 600,
                fontSize: '0.8125rem',
              },
              '& .MuiStepLabel-label.Mui-active': {
                fontWeight: 700,
                color: 'primary.main',
              },
              '& .MuiStepLabel-label.Mui-completed': {
                color: 'success.main',
              },
            }}
          >
            {STEPS.map((step, index) => (
              <Step key={step.label} completed={activeStep > index}>
                <StepLabel
                  StepIconComponent={() => (
                    <Avatar
                      sx={{
                        width: 36,
                        height: 36,
                        backgroundColor:
                          activeStep > index
                            ? 'success.main'
                            : activeStep === index
                              ? 'primary.main'
                              : 'grey.200',
                        color:
                          activeStep > index || activeStep === index
                            ? '#fff'
                            : 'text.secondary',
                        transition: 'all 300ms',
                        fontSize: '0.875rem',
                      }}
                    >
                      {activeStep > index ? (
                        <CheckIcon sx={{ fontSize: 20 }} />
                      ) : (
                        step.icon
                      )}
                    </Avatar>
                  )}
                >
                  {step.label}
                  <br />
                  <Typography
                    variant="caption"
                    color="text.disabled"
                    sx={{ fontSize: '0.6875rem' }}
                  >
                    {step.description}
                  </Typography>
                </StepLabel>
              </Step>
            ))}
          </Stepper>
        </Paper>
      )}

      {/* Step Content */}
      <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
        <Box sx={{ p: { xs: 2, sm: 3 } }}>
          {/* Mobile step indicator */}
          {isMobile && (
            <Stack direction="row" spacing={1} alignItems="center" mb={2}>
              {STEPS.map((step, index) => (
                <Box
                  key={step.label}
                  sx={{
                    flex: 1,
                    height: 4,
                    borderRadius: 2,
                    backgroundColor:
                      activeStep > index
                        ? 'success.main'
                        : activeStep === index
                          ? 'primary.main'
                          : 'grey.200',
                    transition: 'background-color 300ms',
                  }}
                />
              ))}
            </Stack>
          )}

          {activeStep === 0 && renderTemplateSelection()}
          {activeStep === 1 && renderConfiguration()}
          {activeStep === 2 && renderValidationSettings()}
          {activeStep === 3 && renderReview()}
        </Box>

        {/* Navigation Buttons */}
        <Divider />
        <Box
          sx={{
            px: { xs: 2, sm: 3 },
            py: 2,
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            backgroundColor:
              theme.palette.mode === 'dark'
                ? 'rgba(15, 23, 42, 0.4)'
                : 'rgba(248, 250, 252, 0.6)',
          }}
        >
          <Button
            variant="outlined"
            onClick={activeStep === 0 ? handleCancel : handleBack}
            startIcon={activeStep === 0 ? undefined : <BackIcon />}
            sx={{ textTransform: 'none', fontWeight: 600, minWidth: 100 }}
          >
            {activeStep === 0 ? 'Cancel' : 'Back'}
          </Button>

          <Stack direction="row" spacing={1} alignItems="center">
            <Typography variant="caption" color="text.disabled">
              Step {activeStep + 1} of {STEPS.length}
            </Typography>
          </Stack>

          {activeStep < STEPS.length - 1 ? (
            <Button
              variant="contained"
              onClick={handleNext}
              endIcon={<NextIcon />}
              disabled={!isStepValid}
              sx={{ textTransform: 'none', fontWeight: 600, minWidth: 100 }}
            >
              Next
            </Button>
          ) : (
            <Button
              variant="contained"
              onClick={handleCreate}
              disabled={isSubmitting}
              startIcon={
                isSubmitting ? (
                  <CircularProgress size={18} sx={{ color: '#fff' }} />
                ) : (
                  <CreateIcon />
                )
              }
              sx={{
                textTransform: 'none',
                fontWeight: 700,
                minWidth: 140,
                background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                '&:hover': {
                  background: `linear-gradient(135deg, ${theme.palette.primary.dark}, ${theme.palette.secondary.dark})`,
                },
              }}
            >
              {isCreateLoading
                ? 'Creating...'
                : isLaunchWaiting
                  ? 'Waiting for slot...'
                  : isLaunchRunning
                    ? 'Creating and running...'
                    : 'Create Experiment'}
            </Button>
          )}
        </Box>
      </Paper>

      {/* Create / Run Progress */}
      {isCreateLoading && (
        <Alert severity="info" sx={{ mt: 3, borderRadius: 2 }}>
          <AlertTitle>Creating experiment</AlertTitle>
          Your experiment is being created. It will be launched automatically right after.
        </Alert>
      )}
      {isLaunchWaiting && (
        <Alert
          severity="info"
          sx={{ mt: 3, borderRadius: 2 }}
          action={
            <Button color="inherit" size="small" onClick={handleCancelLaunch}>
              Cancel
            </Button>
          }
        >
          <AlertTitle>Waiting for a free slot</AlertTitle>
          {launchMessage ??
            'Maximum concurrent experiments reached. Waiting to launch...'}
        </Alert>
      )}
      {isLaunchRunning && !isCreateLoading && (
        <Alert severity="info" sx={{ mt: 3, borderRadius: 2 }}>
          <AlertTitle>Launching experiment</AlertTitle>
          {launchMessage ?? 'The experiment is starting now.'}
        </Alert>
      )}
      {launchState === 'failed' && launchError && (
        <Alert severity="error" sx={{ mt: 3, borderRadius: 2 }}>
          <AlertTitle>Experiment created, but execution failed</AlertTitle>
          {launchError}
        </Alert>
      )}
    </Box>
  );
};

export default CreateExperimentPage;
