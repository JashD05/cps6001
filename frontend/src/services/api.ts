import axios, {
  type AxiosInstance,
  type AxiosError,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios';
import type {
  LoginRequest,
  LoginResponse,
  User,
  Experiment,
  ExperimentRun,
  ExperimentStepStatus,
  AttackTemplate,
  Cluster,
  ClusterHealth,
  DashboardSummary,
  Report,
  SIEMAlert,
  APIResponse,
  PaginatedResponse,
  TemplateCategory,
  TemplateParameter,
  TemplateSeverity,
  CreateExperimentRequest,
  ExperimentResult,
  SIEMValidationResult,
  PodStatus,
} from '@/types';

// ---------------------------------------------------------------------------
// Token helpers
// ---------------------------------------------------------------------------

const TOKEN_KEY = 'chaos_sec_access_token';
const REFRESH_TOKEN_KEY = 'chaos_sec_refresh_token';

export const getAccessToken = (): string | null => localStorage.getItem(TOKEN_KEY);

export const getRefreshToken = (): string | null =>
  localStorage.getItem(REFRESH_TOKEN_KEY);

export const setTokens = (access: string, refresh?: string): void => {
  localStorage.setItem(TOKEN_KEY, access);
  if (refresh) {
    localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
  }
};

export const clearTokens = (): void => {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
};

// ---------------------------------------------------------------------------
// Base Axios instance
// ---------------------------------------------------------------------------

const BASE_URL =
  import.meta.env.VITE_API_BASE_URL ??
  import.meta.env.VITE_API_URL ??
  'http://localhost:8081/api/v1';

type BackendPaginatedResponse<T> = {
  data?: T[];
  items?: T[];
  total?: number;
  totalCount?: number;
  page?: number;
  page_size?: number;
  pageSize?: number;
  total_pages?: number;
  totalPages?: number;
  hasNextPage?: boolean;
  hasPreviousPage?: boolean;
};

type BackendTemplateParameter = {
  key?: string;
  name?: string;
  label?: string;
  type?: string;
  defaultValue?: unknown;
  default?: unknown;
  required?: boolean;
  description?: string;
  options?: Array<{ label?: string; value?: unknown } | string>;
  validation?: TemplateParameter['validation'];
};

type BackendAttackTemplate = {
  id: string;
  name: string;
  slug?: string;
  category?: string;
  severity?: string;
  description?: string;
  mitre_attack_id?: string | null;
  k8s_manifest?: unknown;
  parameters?: BackendTemplateParameter[] | unknown;
  prerequisites?: unknown;
  expected_behavior?: string;
  mitigation?: string;
  is_active?: boolean;
  is_system?: boolean;
  is_module?: boolean;
  schema?: unknown;
  created_at?: string;
  updated_at?: string;
};

type BackendCreateExperimentRequest = {
  name: string;
  description: string;
  schedule_cron?: string | null;
  auto_cleanup?: boolean;
  notification_config?: Record<string, unknown>;
  templates: Array<{
    attack_template_id: string;
    order_index: number;
    configuration: Record<string, unknown>;
    target_namespaces: string[];
    target_labels: Record<string, unknown>;
    duration_seconds: number;
    cleanup_policy: string;
    siem_validation: Record<string, unknown>;
    enabled: boolean;
  }>;
};

type BackendExperiment = {
  id: string;
  name: string;
  description?: string;
  template_id?: string;
  template_name?: string;
  cluster_id?: string;
  cluster_name?: string;
  namespace?: string;
  status?: string;
  progress?: number;
  parameters?: Record<string, unknown>;
  steps?: unknown[];
  tags?: string[];
  created_by?: string;
  created_at?: string;
  updated_at?: string;
  started_at?: string | null;
  completed_at?: string | null;
  duration?: number | null;
  result?: Experiment['result'];
};

type BackendCluster = {
  id: string;
  name: string;
  description?: string | null;
  api_endpoint?: string;
  default_namespace?: string;
  status?: string;
  kubernetes_version?: string | null;
  node_count?: number | null;
  last_connected_at?: string | null;
  created_at?: string;
  updated_at?: string;
  provider?: string;
  region?: string;
  namespace_count?: number | null;
  namespaces?: string[] | null;
  labels?: Record<string, string> | null;
  lastHealthCheck?: string | null;
};

const TEMPLATE_CATEGORY_MAP: Record<string, TemplateCategory> = {
  network: 'network',
  application: 'application',
  data: 'data',
  rbac: 'identity',
  privilege: 'identity',
  security: 'identity',
  resource: 'infrastructure',
  availability: 'infrastructure',
  identity: 'identity',
};

const TEMPLATE_ICON_MAP: Record<TemplateCategory, string> = {
  network: 'network',
  application: 'web',
  infrastructure: 'server',
  data: 'database',
  identity: 'shield',
  custom: 'template',
};

const normalizeTemplateCategory = (category?: string): TemplateCategory =>
  TEMPLATE_CATEGORY_MAP[category?.toLowerCase() ?? ''] ?? 'custom';

const normalizeTemplateSeverity = (severity?: string): TemplateSeverity => {
  const value = severity?.toLowerCase();
  if (value === 'low' || value === 'medium' || value === 'high' || value === 'critical') {
    return value;
  }
  return 'medium';
};

const normalizeExperimentStatus = (status?: string): Experiment['status'] => {
  const value = status?.toLowerCase();
  switch (value) {
    case 'draft':
    case 'active':
    case 'archived':
    case 'pending':
    case 'queued':
    case 'running':
    case 'completed':
    case 'failed':
    case 'stopped':
    case 'timed_out':
      return value;
    default:
      return 'pending';
  }
};

const normalizeExperimentResult = (
  value: unknown,
  timing?: { startedAt?: string; completedAt?: string; duration?: number | null },
): ExperimentResult | undefined => {
  if (!value || typeof value !== 'object') return undefined;

  const raw = value as Record<string, unknown>;
  if (
    typeof raw.success === 'boolean' &&
    typeof raw.score === 'number' &&
    typeof raw.summary === 'string' &&
    Array.isArray(raw.details) &&
    raw.siemValidation &&
    typeof raw.siemValidation === 'object'
  ) {
    return raw as unknown as ExperimentResult;
  }

  const summary = raw;
  const expectedAlertCount =
    typeof summary.siem_alerts_expected === 'number'
      ? summary.siem_alerts_expected
      : typeof summary.expectedAlertCount === 'number'
        ? summary.expectedAlertCount
        : 0;
  const receivedAlertCount =
    typeof summary.siem_alerts_received === 'number'
      ? summary.siem_alerts_received
      : typeof summary.receivedAlertCount === 'number'
        ? summary.receivedAlertCount
        : 0;
  const detectionRate =
    typeof summary.detection_rate === 'number'
      ? summary.detection_rate
      : typeof summary.coverage === 'number'
        ? summary.coverage
        : expectedAlertCount > 0
          ? Math.min(100, Math.round((receivedAlertCount / expectedAlertCount) * 100))
          : 0;
  const findings = Array.isArray(summary.findings)
    ? (summary.findings as Array<Record<string, unknown>>)
    : [];

  const details = findings.length
    ? findings.map((finding, index) => {
        const severity =
          typeof finding.severity === 'string' && finding.severity.trim() !== ''
            ? finding.severity
            : `Finding ${index + 1}`;
        const description =
          typeof finding.description === 'string' && finding.description.trim() !== ''
            ? finding.description
            : '';
        const recommendation =
          typeof finding.recommendation === 'string' &&
          finding.recommendation.trim() !== ''
            ? finding.recommendation
            : '';

        if (description && recommendation) {
          return `${severity}: ${description} (${recommendation})`;
        }
        if (description) return `${severity}: ${description}`;
        if (recommendation) return `${severity}: ${recommendation}`;
        return severity;
      })
    : [
        typeof summary.overall_status === 'string' && summary.overall_status.trim() !== ''
          ? summary.overall_status
          : 'Experiment completed successfully.',
      ];

  const overallStatus =
    typeof summary.overall_status === 'string' && summary.overall_status.trim() !== ''
      ? summary.overall_status
      : 'completed';
  const success = /pass|success|completed|complete|blocked|mitigated/i.test(
    overallStatus,
  );

  const siemValidation: SIEMValidationResult = {
    expectedAlertCount,
    receivedAlertCount,
    alerts: [],
    detected: receivedAlertCount >= expectedAlertCount && expectedAlertCount > 0,
    detectionLatencyMs: 0,
    coverage: detectionRate,
    details,
  };

  return {
    success,
    score: Math.max(0, Math.min(100, Math.round(detectionRate))),
    summary: overallStatus,
    details,
    siemValidation,
    startedAt: timing?.startedAt ?? new Date().toISOString(),
    completedAt: timing?.completedAt ?? new Date().toISOString(),
    duration: timing?.duration ?? 0,
  };
};

const normalizeExperiment = (
  experiment: BackendExperiment | Experiment | undefined | null,
): Experiment => {
  const raw = (experiment ?? {}) as Record<string, unknown>;
  const nowIso = new Date().toISOString();

  const experimentTemplates = Array.isArray(raw.experiment_templates)
    ? (raw.experiment_templates as Array<Record<string, unknown>>)
    : [];
  const rawRuns = Array.isArray(raw.runs)
    ? (raw.runs as Array<Record<string, unknown>>)
    : [];
  const latestCompletedRun =
    rawRuns.find((run) => String(run.status ?? '').toLowerCase() === 'completed') ??
    rawRuns[0] ??
    null;
  const latestRunTiming =
    raw.latest_run_started_at ||
    raw.latest_run_completed_at ||
    raw.latest_run_duration_ms ||
    raw.latestRunStartedAt ||
    raw.latestRunCompletedAt ||
    raw.latestRunDurationMs
      ? {
          startedAt:
            typeof raw.latest_run_started_at === 'string'
              ? raw.latest_run_started_at
              : typeof raw.latestRunStartedAt === 'string'
                ? raw.latestRunStartedAt
                : typeof latestCompletedRun?.started_at === 'string'
                  ? latestCompletedRun.started_at
                  : typeof latestCompletedRun?.startedAt === 'string'
                    ? latestCompletedRun.startedAt
                    : undefined,
          completedAt:
            typeof raw.latest_run_completed_at === 'string'
              ? raw.latest_run_completed_at
              : typeof raw.latestRunCompletedAt === 'string'
                ? raw.latestRunCompletedAt
                : typeof latestCompletedRun?.completed_at === 'string'
                  ? latestCompletedRun.completed_at
                  : typeof latestCompletedRun?.completedAt === 'string'
                    ? latestCompletedRun.completedAt
                    : undefined,
          duration:
            typeof raw.latest_run_duration_ms === 'number'
              ? raw.latest_run_duration_ms
              : typeof raw.latestRunDurationMs === 'number'
                ? raw.latestRunDurationMs
                : typeof latestCompletedRun?.duration_ms === 'number'
                  ? latestCompletedRun.duration_ms
                  : typeof latestCompletedRun?.durationMs === 'number'
                    ? latestCompletedRun.durationMs
                    : typeof latestCompletedRun?.duration === 'number'
                      ? latestCompletedRun.duration
                      : null,
        }
      : undefined;
  const normalizedResult =
    normalizeExperimentResult(raw.result) ??
    normalizeExperimentResult(
      raw.latest_run_result_summary ??
        raw.latestRunResultSummary ??
        latestCompletedRun?.result_summary ??
        latestCompletedRun?.resultSummary,
      latestRunTiming,
    );
  const normalizedStatus = normalizeExperimentStatus(String(raw.status ?? 'pending'));
  // The effective status should reflect the experiment's actual lifecycle state.
  // Only override to 'completed' if the experiment is currently running/queued/pending
  // (meaning the run finished but the DB status hasn't caught up yet).
  // Never override draft, active, or terminal statuses — they are authoritative.
  const effectiveStatus =
    normalizedResult &&
    (normalizedStatus === 'running' ||
      normalizedStatus === 'queued' ||
      normalizedStatus === 'pending')
      ? 'completed'
      : normalizedStatus;

  const firstTemplate = experimentTemplates[0] ?? null;
  const firstTemplateConfig =
    firstTemplate && typeof firstTemplate.configuration === 'object'
      ? (firstTemplate.configuration as Record<string, unknown>)
      : {};
  const firstTargetLabels =
    firstTemplate && typeof firstTemplate.target_labels === 'object'
      ? (firstTemplate.target_labels as Record<string, unknown>)
      : {};
  const firstAttackTemplate =
    firstTemplate && typeof firstTemplate.attack_template === 'object'
      ? (firstTemplate.attack_template as Record<string, unknown>)
      : {};
  const firstTargetNamespaces = Array.isArray(firstTemplate?.target_namespaces)
    ? (firstTemplate?.target_namespaces as string[])
    : [];

  const parameters =
    raw.parameters && typeof raw.parameters === 'object'
      ? (raw.parameters as Record<string, unknown>)
      : Object.keys(firstTemplateConfig).length > 0
        ? firstTemplateConfig
        : {};

  const derivedNamespace =
    typeof raw.namespace === 'string' && raw.namespace.trim() !== ''
      ? raw.namespace
      : (firstTargetNamespaces[0] ??
        (typeof firstTargetLabels.namespace === 'string'
          ? firstTargetLabels.namespace
          : ''));

  const derivedClusterId =
    typeof raw.cluster_id === 'string' && raw.cluster_id.trim() !== ''
      ? raw.cluster_id
      : typeof firstTargetLabels.cluster_id === 'string'
        ? firstTargetLabels.cluster_id
        : '';

  const resolvedTemplateId =
    typeof raw.template_id === 'string' && raw.template_id.trim() !== ''
      ? raw.template_id
      : typeof firstTemplate?.attack_template_id === 'string'
        ? firstTemplate.attack_template_id
        : typeof raw.templateId === 'string'
          ? raw.templateId
          : '';

  const resolvedTemplateName =
    typeof raw.template_name === 'string' && raw.template_name.trim() !== ''
      ? raw.template_name
      : typeof raw.templateName === 'string' && raw.templateName.trim() !== ''
        ? raw.templateName
        : experimentTemplates.length > 0
          ? 'Custom'
          : undefined;

  const stepStatusMap: Record<string, ExperimentStepStatus> = {
    draft: 'pending',
    active: 'pending',
    pending: 'pending',
    queued: 'pending',
    running: 'in_progress',
    completed: 'completed',
    failed: 'failed',
    stopped: 'pending',
    timed_out: 'pending',
    archived: 'pending',
  };
  const derivedStepStatus = stepStatusMap[effectiveStatus] ?? 'pending';
  const derivedSteps =
    Array.isArray(raw.steps) && (raw.steps as Experiment['steps']).length > 0
      ? (raw.steps as Experiment['steps'])
      : experimentTemplates.map((template, index) => ({
          id: String(template.id ?? template.attack_template_id ?? `step-${index + 1}`),
          name:
            typeof template.name === 'string' && template.name.trim() !== ''
              ? template.name
              : typeof firstAttackTemplate.name === 'string' &&
                  firstAttackTemplate.name.trim() !== ''
                ? String(firstAttackTemplate.name)
                : `Step ${index + 1}`,
          description:
            typeof template.description === 'string' && template.description.trim() !== ''
              ? template.description
              : typeof firstAttackTemplate.description === 'string' &&
                  firstAttackTemplate.description.trim() !== ''
                ? String(firstAttackTemplate.description)
                : 'Experiment step configuration',
          status: derivedStepStatus,
          order: typeof template.order_index === 'number' ? template.order_index : index,
        }));

  const tags = Array.isArray(raw.tags) ? (raw.tags as string[]) : [];

  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    description: String(raw.description ?? ''),
    templateId: resolvedTemplateId,
    templateName: resolvedTemplateName,
    clusterId: derivedClusterId,
    clusterName:
      typeof raw.cluster_name === 'string'
        ? raw.cluster_name
        : typeof raw.clusterName === 'string'
          ? raw.clusterName
          : undefined,
    namespace: derivedNamespace,
    status: effectiveStatus,
    progress:
      effectiveStatus === 'completed'
        ? 100
        : typeof raw.progress === 'number'
          ? raw.progress
          : typeof raw.progress === 'string'
            ? Number(raw.progress) || 0
            : 0,
    parameters,
    steps: derivedSteps,
    tags,
    createdBy: String(raw.created_by ?? raw.createdBy ?? ''),
    createdAt:
      typeof raw.created_at === 'string'
        ? raw.created_at
        : typeof raw.createdAt === 'string'
          ? raw.createdAt
          : nowIso,
    updatedAt:
      typeof raw.updated_at === 'string'
        ? raw.updated_at
        : typeof raw.updatedAt === 'string'
          ? raw.updatedAt
          : nowIso,
    startedAt:
      typeof raw.started_at === 'string'
        ? raw.started_at
        : typeof raw.startedAt === 'string'
          ? raw.startedAt
          : latestRunTiming?.startedAt,
    completedAt:
      typeof raw.completed_at === 'string'
        ? raw.completed_at
        : typeof raw.completedAt === 'string'
          ? raw.completedAt
          : latestRunTiming?.completedAt,
    duration:
      typeof raw.duration === 'number'
        ? raw.duration
        : typeof raw.duration === 'string'
          ? Number(raw.duration) || undefined
          : (latestRunTiming?.duration ?? undefined),
    result: normalizedResult,
    runs: normalizeRuns(rawRuns),
  };
};

const normalizeRuns = (
  rawRuns: Array<Record<string, unknown>>,
): ExperimentRun[] | undefined => {
  if (!rawRuns || rawRuns.length === 0) return undefined;

  return rawRuns.map((run) => {
    const rawPods = Array.isArray(run.attack_pods)
      ? (run.attack_pods as Array<Record<string, unknown>>)
      : Array.isArray(run.AttackPods)
        ? (run.AttackPods as Array<Record<string, unknown>>)
        : [];

    const podStatuses: PodStatus[] = rawPods.map((pod) => ({
      name: String(pod.pod_name ?? pod.PodName ?? ''),
      namespace: String(pod.namespace ?? pod.Namespace ?? ''),
      status: String(pod.status ?? pod.Status ?? 'Unknown') as PodStatus['status'],
      ready: String(pod.phase ?? pod.Phase ?? '') === 'Running',
      restarts: 0,
      age: pod.started_at
        ? new Date(String(pod.started_at)).toLocaleDateString()
        : pod.StartedAt
          ? new Date(String(pod.StartedAt)).toLocaleDateString()
          : '',
      ip: pod.ip_address
        ? String(pod.ip_address)
        : pod.IPAddress
          ? String(pod.IPAddress)
          : undefined,
      node: pod.node_name
        ? String(pod.node_name)
        : pod.NodeName
          ? String(pod.NodeName)
          : undefined,
    }));

    return {
      id: String(run.id ?? ''),
      experimentId: String(run.experiment_id ?? run.ExperimentID ?? ''),
      status: normalizeExperimentStatus(String(run.status ?? run.Status ?? 'pending')),
      progress: typeof run.progress === 'number' ? run.progress : 0,
      logs: [],
      startedAt: run.started_at
        ? String(run.started_at)
        : run.StartedAt
          ? String(run.StartedAt)
          : '',
      completedAt: run.completed_at
        ? String(run.completed_at)
        : run.CompletedAt
          ? String(run.CompletedAt)
          : undefined,
      podStatuses,
      steps: [],
      result: normalizeExperimentResult(
        run.result_summary ?? run.ResultSummary ?? run.result,
      ),
    };
  });
};

const normalizeExperimentPaginatedResponse = (
  response:
    | AxiosResponse<BackendPaginatedResponse<BackendExperiment>>
    | AxiosResponse<PaginatedResponse<Experiment>>,
): AxiosResponse<PaginatedResponse<Experiment>> => {
  const payload = response.data as BackendPaginatedResponse<BackendExperiment> &
    Partial<PaginatedResponse<Experiment>>;
  const items = (payload.items ?? payload.data ?? []) as BackendExperiment[];

  return {
    ...response,
    data: normalizePaginatedResponse({
      ...payload,
      items: items.map(normalizeExperiment),
    } as PaginatedResponse<Experiment>),
  };
};

const normalizeClusterStatus = (status?: string): Cluster['status'] => {
  switch (status?.toLowerCase() ?? '') {
    case 'healthy':
    case 'degraded':
    case 'unreachable':
    case 'unknown':
      return status as Cluster['status'];
    case 'connected':
      return 'healthy';
    case 'pending':
      return 'unknown';
    case 'error':
    case 'disabled':
      return 'unreachable';
    default:
      return 'unknown';
  }
};

const normalizeCluster = (
  cluster: BackendCluster | Cluster | undefined | null,
): Cluster => {
  const raw = (cluster ?? {}) as Record<string, unknown>;
  const defaultNamespace =
    typeof raw.default_namespace === 'string' && raw.default_namespace.trim() !== ''
      ? raw.default_namespace
      : typeof raw.defaultNamespace === 'string' && raw.defaultNamespace.trim() !== ''
        ? raw.defaultNamespace
        : 'default';
  const lastHealthCheck =
    typeof raw.last_connected_at === 'string' && raw.last_connected_at.trim() !== ''
      ? raw.last_connected_at
      : typeof raw.lastHealthCheck === 'string' && raw.lastHealthCheck.trim() !== ''
        ? raw.lastHealthCheck
        : typeof raw.updated_at === 'string' && raw.updated_at.trim() !== ''
          ? raw.updated_at
          : new Date().toISOString();
  const namespaces = Array.isArray(raw.namespaces)
    ? raw.namespaces.filter(
        (ns): ns is string => typeof ns === 'string' && ns.trim() !== '',
      )
    : defaultNamespace
      ? [defaultNamespace]
      : [];

  return {
    id: typeof raw.id === 'string' ? raw.id : '',
    name:
      typeof raw.name === 'string' && raw.name.trim() !== ''
        ? raw.name
        : 'Unnamed Cluster',
    description:
      typeof raw.description === 'string'
        ? raw.description
        : typeof raw.description === 'undefined' || raw.description === null
          ? ''
          : String(raw.description),
    status: normalizeClusterStatus(
      typeof raw.status === 'string' ? raw.status : undefined,
    ),
    provider:
      typeof raw.provider === 'string' && raw.provider.trim() !== ''
        ? (raw.provider as Cluster['provider'])
        : 'other',
    region:
      typeof raw.region === 'string' && raw.region.trim() !== '' ? raw.region : 'unknown',
    version:
      typeof raw.kubernetes_version === 'string' && raw.kubernetes_version.trim() !== ''
        ? raw.kubernetes_version
        : typeof raw.version === 'string' && raw.version.trim() !== ''
          ? raw.version
          : 'unknown',
    nodeCount:
      typeof raw.node_count === 'number'
        ? raw.node_count
        : typeof raw.nodeCount === 'number'
          ? raw.nodeCount
          : 0,
    namespaceCount:
      typeof raw.namespace_count === 'number'
        ? raw.namespace_count
        : typeof raw.namespaceCount === 'number'
          ? raw.namespaceCount
          : namespaces.length,
    namespaces,
    labels:
      raw.labels && typeof raw.labels === 'object' && !Array.isArray(raw.labels)
        ? (raw.labels as Record<string, string>)
        : {},
    lastHealthCheck,
    createdAt:
      typeof raw.created_at === 'string' && raw.created_at.trim() !== ''
        ? raw.created_at
        : new Date().toISOString(),
    updatedAt:
      typeof raw.updated_at === 'string' && raw.updated_at.trim() !== ''
        ? raw.updated_at
        : new Date().toISOString(),
  };
};

const normalizeTemplateParameters = (value: unknown): TemplateParameter[] => {
  const buildParameter = (
    record: Record<string, unknown>,
    keyHint: string,
    required: boolean,
  ): TemplateParameter => {
    const rawType = String(record.type ?? 'string').toLowerCase();
    const type: TemplateParameter['type'] =
      rawType === 'int' || rawType === 'integer' || rawType === 'number'
        ? 'number'
        : rawType === 'bool' || rawType === 'boolean'
          ? 'boolean'
          : rawType === 'multi_select' || rawType === 'multi-select'
            ? 'multi-select'
            : rawType === 'select'
              ? 'select'
              : 'string';

    const options = Array.isArray(record.options)
      ? record.options.map((option) => {
          if (typeof option === 'string') {
            return { label: option, value: option };
          }
          if (option && typeof option === 'object') {
            const optionRecord = option as Record<string, unknown>;
            const optionValue = optionRecord.value ?? optionRecord.label ?? '';
            return {
              label: String(optionRecord.label ?? optionValue),
              value: optionValue,
            };
          }
          return { label: String(option ?? ''), value: option ?? '' };
        })
      : undefined;

    return {
      key: String(record.key ?? record.name ?? keyHint),
      label: String(record.label ?? record.title ?? record.name ?? record.key ?? keyHint),
      type,
      defaultValue: record.defaultValue ?? record.default ?? '',
      required,
      description: String(record.description ?? ''),
      options,
      validation:
        record.validation && typeof record.validation === 'object'
          ? (record.validation as TemplateParameter['validation'])
          : undefined,
    };
  };

  if (Array.isArray(value)) {
    return value.map((param, index) => {
      if (!param || typeof param !== 'object') {
        return buildParameter({}, `param-${index + 1}`, false);
      }
      const record = param as Record<string, unknown>;
      return buildParameter(record, `param-${index + 1}`, Boolean(record.required));
    });
  }

  if (value && typeof value === 'object') {
    const schema = value as Record<string, unknown>;
    const requiredFields = new Set(
      Array.isArray(schema.required) ? schema.required.map((item) => String(item)) : [],
    );
    const properties = schema.properties as Record<string, unknown> | undefined;

    if (properties && typeof properties === 'object') {
      return Object.entries(properties).map(([key, prop]) => {
        const record =
          prop && typeof prop === 'object' ? (prop as Record<string, unknown>) : {};
        return buildParameter(record, key, requiredFields.has(key));
      });
    }
  }

  return [];
};

const normalizePaginatedResponse = <T>(
  payload: BackendPaginatedResponse<T> | PaginatedResponse<T>,
): PaginatedResponse<T> => {
  const items = 'items' in payload ? (payload.items ?? []) : (payload.data ?? []);
  const totalCount =
    'totalCount' in payload
      ? (payload.totalCount ?? items.length)
      : (payload.total ?? items.length);
  const page = payload.page ?? 1;
  const pageSize =
    'pageSize' in payload ? (payload.pageSize ?? 10) : (payload.page_size ?? 10);
  const totalPages =
    'totalPages' in payload
      ? (payload.totalPages ?? 1)
      : (payload.total_pages ??
        Math.max(1, Math.ceil(totalCount / Math.max(pageSize, 1))));

  return {
    items,
    totalCount,
    page,
    pageSize,
    totalPages,
    hasNextPage: payload.hasNextPage ?? page < totalPages,
    hasPreviousPage: payload.hasPreviousPage ?? page > 1,
  };
};

const normalizeAxiosPaginatedResponse = <T>(
  response: AxiosResponse<BackendPaginatedResponse<T> | PaginatedResponse<T>>,
): AxiosResponse<PaginatedResponse<T>> => ({
  ...response,
  data: normalizePaginatedResponse(response.data),
});

const normalizeTemplate = (template: BackendAttackTemplate): AttackTemplate => {
  const category = normalizeTemplateCategory(template.category);
  const nowIso = new Date().toISOString();
  const tagsFromSlug = template.slug
    ? template.slug
        .split('-')
        .map((part) => part.trim())
        .filter(Boolean)
    : [];

  return {
    id: template.id,
    name: template.name,
    description: template.description ?? '',
    category,
    severity: normalizeTemplateSeverity(template.severity),
    icon: TEMPLATE_ICON_MAP[category],
    version: '1.0.0',
    author: template.is_system || template.is_module ? 'Chaos-Sec Team' : 'User',
    parameters: normalizeTemplateParameters(template.parameters),
    attackPhases: [],
    expectedDetections: [],
    tags: tagsFromSlug.length > 0 ? [category, ...tagsFromSlug] : [category],
    isOfficial: Boolean(template.is_system || template.is_module),
    usageCount: 0,
    createdAt: template.created_at ?? nowIso,
    updatedAt: template.updated_at ?? template.created_at ?? nowIso,
  };
};

const AUTH_SESSION_EXPIRED_EVENT = 'chaos-sec:auth-expired';

export const emitAuthSessionExpired = (reason: string): void => {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(
    new CustomEvent(AUTH_SESSION_EXPIRED_EVENT, {
      detail: { reason },
    }),
  );
};

const apiClient: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 30_000,
  headers: {
    'Content-Type': 'application/json',
    Accept: 'application/json',
  },
});

// ---------------------------------------------------------------------------
// Request interceptor – attach Authorization header
// ---------------------------------------------------------------------------

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getAccessToken();
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error: AxiosError) => Promise.reject(error),
);

// ---------------------------------------------------------------------------
// Response interceptor – handle 401 & token refresh
// ---------------------------------------------------------------------------

let isRefreshing = false;
let failedQueue: Array<{
  resolve: (token: string) => void;
  reject: (error: unknown) => void;
}> = [];

const processQueue = (error: unknown, token: string | null = null): void => {
  failedQueue.forEach(({ resolve, reject }) => {
    if (token) {
      resolve(token);
    } else {
      reject(error);
    }
  });
  failedQueue = [];
};

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & {
      _retry?: boolean;
    };

    // Only attempt refresh on 401 from a non-refresh endpoint
    if (
      error.response?.status === 401 &&
      !originalRequest._retry &&
      originalRequest.url !== '/auth/refresh' &&
      originalRequest.url !== '/auth/login'
    ) {
      // If we are already refreshing, queue this request
      if (isRefreshing) {
        return new Promise<AxiosResponse>((resolve, reject) => {
          failedQueue.push({
            resolve: (token: string) => {
              originalRequest.headers.Authorization = `Bearer ${token}`;
              resolve(apiClient(originalRequest));
            },
            reject,
          });
        });
      }

      originalRequest._retry = true;
      isRefreshing = true;

      try {
        const refreshToken = getRefreshToken();
        if (!refreshToken) {
          emitAuthSessionExpired('No refresh token available');
          throw new Error('No refresh token available');
        }

        const { data } = await axios.post<Record<string, unknown>>(
          `${BASE_URL}/auth/refresh`,
          { refreshToken },
        );

        // The backend returns tokens in snake_case directly (no data wrapper).
        const payload = (data.data as Record<string, unknown>) ?? data;
        const newAccessToken = String(payload.accessToken ?? payload.access_token);
        const newRefreshToken =
          String(payload.refreshToken ?? payload.refresh_token) ?? refreshToken;
        setTokens(newAccessToken, newRefreshToken);

        processQueue(null, newAccessToken);

        originalRequest.headers.Authorization = `Bearer ${newAccessToken}`;
        return apiClient(originalRequest);
      } catch (refreshError) {
        processQueue(refreshError, null);
        clearTokens();
        emitAuthSessionExpired(getErrorMessage(refreshError));
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    // For 401 on login/refresh or other status codes, reject normally.
    // Only treat 401s from authenticated requests as a session-expired event.
    if (
      error.response?.status === 401 &&
      originalRequest.url !== '/auth/refresh' &&
      originalRequest.url !== '/auth/login'
    ) {
      clearTokens();
      emitAuthSessionExpired(getErrorMessage(error));
    }

    return Promise.reject(error);
  },
);

// ---------------------------------------------------------------------------
// Auth API
// ---------------------------------------------------------------------------

export const authAPI = {
  login: (payload: LoginRequest) =>
    apiClient.post<APIResponse<LoginResponse>>('/auth/login', payload),

  logout: () => apiClient.post<APIResponse<void>>('/auth/logout'),

  refresh: (refreshToken: string) =>
    apiClient.post<APIResponse<LoginResponse>>('/auth/refresh', {
      refreshToken,
    }),

  me: () => apiClient.get<APIResponse<User>>('/auth/me'),

  updateProfile: (data: Partial<User>) =>
    apiClient.put<APIResponse<User>>('/auth/profile', data),

  changePassword: (data: { currentPassword: string; newPassword: string }) =>
    apiClient.put<APIResponse<void>>('/auth/password', data),

  register: (data: {
    name: string;
    email: string;
    password: string;
    organization: string;
  }) => apiClient.post<APIResponse<LoginResponse>>('/auth/register', data),
};

// ---------------------------------------------------------------------------
// Experiments API
// ---------------------------------------------------------------------------

export const experimentsAPI = {
  list: (params?: {
    page?: number;
    limit?: number;
    status?: string;
    search?: string;
    clusterId?: string;
    sortBy?: string;
    sortOrder?: 'asc' | 'desc';
  }): Promise<AxiosResponse<PaginatedResponse<Experiment>>> => {
    // Map frontend camelCase param names to backend snake_case param names
    const mappedParams: Record<string, unknown> = {};
    if (params?.page !== undefined) mappedParams.page = params.page;
    if (params?.limit !== undefined) mappedParams.page_size = params.limit;
    if (params?.status !== undefined) mappedParams.status = params.status;
    if (params?.search !== undefined) mappedParams.search = params.search;
    if (params?.clusterId !== undefined) mappedParams.cluster_id = params.clusterId;
    if (params?.sortBy !== undefined) mappedParams.sort_by = params.sortBy;
    if (params?.sortOrder !== undefined) mappedParams.sort_order = params.sortOrder;

    return apiClient
      .get<BackendPaginatedResponse<BackendExperiment>>('/experiments', {
        params: mappedParams,
      })
      .then((response) => normalizeExperimentPaginatedResponse(response));
  },

  getById: (id: string): Promise<AxiosResponse<APIResponse<Experiment>>> =>
    apiClient
      .get<APIResponse<BackendExperiment>>(`/experiments/${id}`)
      .then((response) => ({
        ...response,
        data: {
          ...response.data,
          data: normalizeExperiment(response.data.data),
        },
      })),

  create: (
    data: CreateExperimentRequest,
  ): Promise<AxiosResponse<APIResponse<Experiment>>> => {
    const payload: BackendCreateExperimentRequest = {
      name: data.name,
      description: data.description,
      schedule_cron: data.schedule ?? null,
      auto_cleanup: true,
      notification_config: {},
      templates: [
        {
          attack_template_id: data.templateId,
          order_index: 0,
          configuration: data.parameters ?? {},
          target_namespaces: data.namespace ? [data.namespace] : [],
          target_labels: data.clusterId
            ? { cluster_id: data.clusterId, namespace: data.namespace }
            : { namespace: data.namespace },
          duration_seconds: data.validation?.timeWindowSeconds ?? 300,
          cleanup_policy: 'immediate',
          siem_validation: {
            siem_alert_type: data.validation?.siemAlertType ?? '',
            time_window_seconds: data.validation?.timeWindowSeconds ?? 60,
            expected_alert_count: data.validation?.expectedAlertCount ?? 1,
            custom_rules: data.validation?.customRules ?? {},
          },
          enabled: true,
        },
      ],
    };

    return apiClient
      .post<APIResponse<BackendExperiment>>('/experiments', payload)
      .then((response) => ({
        ...response,
        data: {
          ...response.data,
          data: normalizeExperiment(response.data.data),
        },
      }));
  },

  update: (
    id: string,
    data: Partial<Experiment>,
  ): Promise<AxiosResponse<APIResponse<Experiment>>> =>
    apiClient
      .put<APIResponse<BackendExperiment>>(`/experiments/${id}`, data)
      .then((response) => ({
        ...response,
        data: {
          ...response.data,
          data: normalizeExperiment(response.data.data),
        },
      })),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/experiments/${id}`),

  execute: (
    id: string,
    clusterId?: string,
  ): Promise<AxiosResponse<APIResponse<ExperimentRun>>> =>
    apiClient.post<APIResponse<ExperimentRun>>(
      `/experiments/${id}/execute`,
      clusterId ? { cluster_id: clusterId } : {},
    ),

  stop: (id: string): Promise<AxiosResponse<APIResponse<Experiment>>> =>
    apiClient
      .post<APIResponse<BackendExperiment>>(`/experiments/${id}/stop`)
      .then((response) => ({
        ...response,
        data: {
          ...response.data,
          data: normalizeExperiment(response.data.data),
        },
      })),

  getRuns: (
    id: string,
    params?: { page?: number; limit?: number },
  ): Promise<AxiosResponse<PaginatedResponse<ExperimentRun>>> =>
    apiClient
      .get<BackendPaginatedResponse<ExperimentRun>>(`/experiments/${id}/runs`, {
        params,
      })
      .then((response) => normalizeAxiosPaginatedResponse(response)),

  getRunById: (
    experimentId: string,
    runId: string,
  ): Promise<AxiosResponse<APIResponse<ExperimentRun>>> =>
    apiClient.get<APIResponse<ExperimentRun>>(
      `/experiments/${experimentId}/runs/${runId}`,
    ),

  getLogs: (
    id: string,
    params?: { tail?: number; follow?: boolean },
  ): Promise<AxiosResponse<APIResponse<string[]>>> =>
    apiClient.get<APIResponse<string[]>>(`/experiments/${id}/logs`, { params }),

  getResults: (id: string): Promise<AxiosResponse<APIResponse<Experiment>>> =>
    apiClient
      .get<APIResponse<BackendExperiment>>(`/experiments/${id}/results`)
      .then((response) => ({
        ...response,
        data: {
          ...response.data,
          data: normalizeExperiment(response.data.data),
        },
      })),

  cancelStaleRuns: (params?: {
    clusterId?: string;
  }): Promise<AxiosResponse<APIResponse<{ cancelled_count: number }>>> =>
    apiClient.post<APIResponse<{ cancelled_count: number }>>(
      '/experiments/stale-runs/cancel',
      null,
      { params: { cluster_id: params?.clusterId } },
    ),
};

// ---------------------------------------------------------------------------
// Templates API
// ---------------------------------------------------------------------------

export const templatesAPI = {
  list: (params?: { category?: string; search?: string; severity?: string }) =>
    apiClient
      .get<BackendPaginatedResponse<BackendAttackTemplate>>('/attack-templates', {
        params: {
          ...params,
          include_modules: 'true',
        },
      })
      .then((response) =>
        normalizeAxiosPaginatedResponse({
          ...response,
          data: {
            ...response.data,
            items: (response.data.data ?? []).map(normalizeTemplate),
          },
        }),
      ),

  getById: (id: string) =>
    apiClient.get<APIResponse<AttackTemplate>>(`/attack-templates/${id}`),

  create: (data: Partial<AttackTemplate>) =>
    apiClient.post<APIResponse<AttackTemplate>>('/attack-templates', data),

  update: (id: string, data: Partial<AttackTemplate>) =>
    apiClient.put<APIResponse<AttackTemplate>>(`/attack-templates/${id}`, data),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/attack-templates/${id}`),

  getCategories: () => apiClient.get<APIResponse<string[]>>('/templates/categories'),
};

// ---------------------------------------------------------------------------
// Clusters API
// ---------------------------------------------------------------------------

export const clustersAPI = {
  list: (params?: {
    search?: string;
    status?: string;
  }): Promise<AxiosResponse<PaginatedResponse<Cluster>>> =>
    apiClient
      .get<BackendPaginatedResponse<BackendCluster>>('/clusters', { params })
      .then((response) => ({
        ...response,
        data: normalizePaginatedResponse({
          ...response.data,
          items: (
            (response.data.items ?? response.data.data ?? []) as BackendCluster[]
          ).map(normalizeCluster),
        } as PaginatedResponse<Cluster>),
      })),

  getById: (id: string): Promise<AxiosResponse<APIResponse<Cluster>>> =>
    apiClient.get<APIResponse<BackendCluster>>(`/clusters/${id}`).then((response) => ({
      ...response,
      data: {
        ...response.data,
        data: normalizeCluster(response.data.data),
      },
    })),

  register: (data: Partial<Cluster>): Promise<AxiosResponse<APIResponse<Cluster>>> =>
    apiClient.post<APIResponse<BackendCluster>>('/clusters', data).then((response) => ({
      ...response,
      data: {
        ...response.data,
        data: normalizeCluster(response.data.data),
      },
    })),

  update: (
    id: string,
    data: Partial<Cluster>,
  ): Promise<AxiosResponse<APIResponse<Cluster>>> =>
    apiClient
      .put<APIResponse<BackendCluster>>(`/clusters/${id}`, data)
      .then((response) => ({
        ...response,
        data: {
          ...response.data,
          data: normalizeCluster(response.data.data),
        },
      })),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/clusters/${id}`),

  healthCheck: (id: string) =>
    apiClient.get<APIResponse<ClusterHealth>>(`/clusters/${id}/health`),

  getNamespaces: (id: string) =>
    apiClient.get<APIResponse<string[]>>(`/clusters/${id}/namespaces`),

  getResources: (id: string, params?: { namespace?: string }) =>
    apiClient.get<APIResponse<Record<string, unknown>>>(`/clusters/${id}/resources`, {
      params,
    }),
};

// ---------------------------------------------------------------------------
// Dashboard API
// ---------------------------------------------------------------------------

export const dashboardAPI = {
  getSummary: () => apiClient.get<APIResponse<DashboardSummary>>('/dashboard/summary'),

  getSecurityPosture: () =>
    apiClient.get<
      APIResponse<{
        score: number;
        trend: number;
        history: Array<{ date: string; score: number }>;
      }>
    >('/dashboard/security-posture'),

  getRecentExperiments: (limit?: number) =>
    apiClient.get<APIResponse<Experiment[]>>('/dashboard/recent-experiments', {
      params: { limit: limit ?? 5 },
    }),

  getClusterHealth: () =>
    apiClient.get<
      APIResponse<
        Array<{ id: string; name: string; status: string; health: ClusterHealth }>
      >
    >('/dashboard/cluster-health'),

  getActivityTimeline: (params?: { days?: number }) =>
    apiClient.get<
      APIResponse<Array<{ date: string; total: number; passed: number; failed: number }>>
    >('/dashboard/activity-timeline', { params }),

  getMetrics: () =>
    apiClient.get<
      APIResponse<{
        experimentsPerDay: number;
        avgDuration: number;
        successRate: number;
        activeUsers: number;
      }>
    >('/dashboard/metrics'),
};

// ---------------------------------------------------------------------------
// Reports API
// ---------------------------------------------------------------------------

export const reportsAPI = {
  list: (params?: {
    page?: number;
    limit?: number;
    experimentId?: string;
    type?: string;
    dateFrom?: string;
    dateTo?: string;
  }) =>
    apiClient
      .get<BackendPaginatedResponse<Report>>('/reports', { params })
      .then((response) => normalizeAxiosPaginatedResponse(response)),

  getById: (id: string) => apiClient.get<APIResponse<Report>>(`/reports/${id}`),

  generate: (data: {
    title: string;
    type: string;
    format: string;
    description?: string;
    experiment_ids?: string[];
    date_from?: string;
    date_to?: string;
  }) => apiClient.post<APIResponse<Report>>('/reports', data),

  download: (id: string, format: string = 'pdf') =>
    apiClient.get<Blob>(`/reports/${id}/download`, {
      params: { format },
      responseType: 'blob',
    }),

  delete: (id: string) => apiClient.delete<APIResponse<void>>(`/reports/${id}`),

  share: (id: string, data: { recipients: string[]; message?: string }) =>
    apiClient.post<APIResponse<void>>(`/reports/${id}/share`, data),

  schedule: (data: {
    experimentId: string;
    cron: string;
    format: string;
    recipients: string[];
  }) => apiClient.post<APIResponse<void>>('/reports/schedule', data),
};

// ---------------------------------------------------------------------------
// SIEM API
// ---------------------------------------------------------------------------

export const siemAPI = {
  getAlerts: (params?: {
    experimentId?: string;
    status?: string;
    severity?: string;
    page?: number;
    limit?: number;
  }) =>
    apiClient
      .get<BackendPaginatedResponse<SIEMAlert>>('/siem/alerts', { params })
      .then((response) => normalizeAxiosPaginatedResponse(response)),

  getAlertById: (id: string) =>
    apiClient.get<APIResponse<SIEMAlert>>(`/siem/alerts/${id}`),

  acknowledgeAlert: (id: string) =>
    apiClient.post<APIResponse<SIEMAlert>>(`/siem/alerts/${id}/acknowledge`),

  dismissAlert: (id: string, reason?: string) =>
    apiClient.post<APIResponse<void>>(`/siem/alerts/${id}/dismiss`, { reason }),

  getValidationResults: (experimentId: string) =>
    apiClient.get<
      APIResponse<{
        experimentId: string;
        expectedAlerts: number;
        receivedAlerts: number;
        validated: boolean;
        details: SIEMAlert[];
      }>
    >(`/siem/validation/${experimentId}`),

  testConnection: (data: { type: string; endpoint: string; apiKey?: string }) =>
    apiClient.post<APIResponse<{ connected: boolean; version?: string }>>(
      '/siem/test-connection',
      data,
    ),

  getConfigurations: () =>
    apiClient.get<
      APIResponse<Array<{ id: string; name: string; type: string; enabled: boolean }>>
    >('/siem/configurations'),

  updateConfiguration: (id: string, data: Record<string, unknown>) =>
    apiClient.put<APIResponse<void>>(`/siem/configurations/${id}`, data),
};

// ---------------------------------------------------------------------------
// Utility: extract error message from Axios error
// ---------------------------------------------------------------------------

export const getErrorMessage = (error: unknown): string => {
  if (axios.isAxiosError(error)) {
    const serverMessage =
      (error.response?.data as { message?: string })?.message ??
      (error.response?.data as { error?: string })?.error;
    if (serverMessage) return serverMessage;
    if (error.message) return error.message;
  }
  if (error instanceof Error) return error.message;
  return 'An unexpected error occurred';
};

// ---------------------------------------------------------------------------
// Export default instance for custom use-cases
// ---------------------------------------------------------------------------

export default apiClient;
