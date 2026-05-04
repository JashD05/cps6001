// ============================================================================
// Chaos-Sec Frontend - TypeScript Type Definitions
// ============================================================================

// ---------------------------------------------------------------------------
// Auth & User
// ---------------------------------------------------------------------------

export type UserRole = 'admin' | 'operator' | 'viewer';

export interface User {
  id: string;
  email: string;
  name: string;
  role: UserRole;
  avatarUrl?: string;
  createdAt: string;
  updatedAt: string;
  lastLoginAt?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  tokenType: 'Bearer';
  user: User;
}

export interface RefreshTokenRequest {
  refreshToken: string;
}

export interface RefreshTokenResponse {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  tokenType: 'Bearer';
}

export interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
}

// ---------------------------------------------------------------------------
// Experiments
// ---------------------------------------------------------------------------

export type ExperimentStatus =
  | 'draft'
  | 'active'
  | 'pending'
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'stopped'
  | 'timed_out'
  | 'archived';

export type ExperimentStepStatus =
  | 'pending'
  | 'in_progress'
  | 'completed'
  | 'failed'
  | 'skipped';

export interface ExperimentStep {
  id: string;
  name: string;
  description: string;
  status: ExperimentStepStatus;
  startedAt?: string;
  completedAt?: string;
  order: number;
}

export interface Experiment {
  id: string;
  name: string;
  description: string;
  templateId: string;
  templateName?: string;
  clusterId: string;
  clusterName?: string;
  namespace: string;
  status: ExperimentStatus;
  progress: number;
  parameters: Record<string, unknown>;
  steps: ExperimentStep[];
  tags: string[];
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  completedAt?: string;
  duration?: number;
  result?: ExperimentResult;
  runs?: ExperimentRun[];
}

export interface ExperimentResult {
  success: boolean;
  score: number;
  summary: string;
  details: string[];
  siemValidation: SIEMValidationResult;
  startedAt: string;
  completedAt: string;
  duration: number;
}

export interface ExperimentRun {
  id: string;
  experimentId: string;
  status: ExperimentStatus;
  progress: number;
  logs: string[];
  startedAt: string;
  completedAt?: string;
  podStatuses: PodStatus[];
  steps: ExperimentStep[];
  result?: ExperimentResult;
}

export interface PodStatus {
  name: string;
  namespace: string;
  status: 'Pending' | 'Running' | 'Succeeded' | 'Failed' | 'Unknown';
  ready: boolean;
  restarts: number;
  age: string;
  ip?: string;
  node?: string;
}

export interface CreateExperimentRequest {
  name: string;
  description: string;
  templateId: string;
  clusterId: string;
  namespace: string;
  parameters: Record<string, unknown>;
  validation: ValidationSettings;
  tags?: string[];
  schedule?: string;
}

export interface ValidationSettings {
  siemAlertType: string;
  timeWindowSeconds: number;
  expectedAlertCount: number;
  customRules?: Record<string, string>;
}

export interface ExperimentListState {
  experiments: Experiment[];
  totalCount: number;
  currentPage: number;
  pageSize: number;
  isLoading: boolean;
  error: string | null;
  filters: ExperimentFilters;
  sortBy: string;
  sortOrder: 'asc' | 'desc';
}

export interface ExperimentFilters {
  search: string;
  status: ExperimentStatus | 'all';
  templateId: string | null;
  clusterId: string | null;
  dateFrom: string | null;
  dateTo: string | null;
}

export interface ExperimentDetailState {
  experiment: Experiment | null;
  currentRun: ExperimentRun | null;
  logs: string[];
  isLoading: boolean;
  error: string | null;
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

export type TemplateCategory =
  | 'network'
  | 'application'
  | 'infrastructure'
  | 'data'
  | 'identity'
  | 'custom';

export type TemplateSeverity = 'low' | 'medium' | 'high' | 'critical';

export interface TemplateParameter {
  key: string;
  label: string;
  type: 'string' | 'number' | 'boolean' | 'select' | 'multi-select';
  defaultValue: unknown;
  required: boolean;
  description: string;
  options?: { label: string; value: unknown }[];
  validation?: {
    min?: number;
    max?: number;
    pattern?: string;
    minLength?: number;
    maxLength?: number;
  };
}

export interface AttackTemplate {
  id: string;
  name: string;
  description: string;
  category: TemplateCategory;
  severity: TemplateSeverity;
  icon: string;
  version: string;
  author: string;
  parameters: TemplateParameter[];
  attackPhases: AttackPhase[];
  expectedDetections: ExpectedDetection[];
  tags: string[];
  isOfficial: boolean;
  usageCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface AttackPhase {
  name: string;
  description: string;
  technique: string;
  tactic: string;
  duration: number;
}

export interface ExpectedDetection {
  source: string;
  type: string;
  description: string;
  confidence: number;
}

export interface TemplateListState {
  templates: AttackTemplate[];
  isLoading: boolean;
  error: string | null;
  filters: TemplateFilters;
  selectedCategory: TemplateCategory | 'all';
}

export interface TemplateFilters {
  search: string;
  category: TemplateCategory | 'all';
  severity: TemplateSeverity | 'all';
}

// ---------------------------------------------------------------------------
// Clusters
// ---------------------------------------------------------------------------

export type ClusterStatus = 'healthy' | 'degraded' | 'unreachable' | 'unknown';

export interface Cluster {
  id: string;
  name: string;
  description: string;
  status: ClusterStatus;
  provider: 'aws' | 'gcp' | 'azure' | 'on-prem' | 'kind' | 'other';
  region: string;
  version: string;
  nodeCount: number;
  namespaceCount: number;
  namespaces: string[];
  labels: Record<string, string>;
  lastHealthCheck: string;
  createdAt: string;
  updatedAt: string;
}

export interface ClusterHealth {
  clusterId: string;
  status: ClusterStatus;
  cpuUsage: number;
  memoryUsage: number;
  podCount: number;
  nodeCount: number;
  errorRate: number;
  lastChecked: string;
}

export interface ClusterListState {
  clusters: Cluster[];
  isLoading: boolean;
  error: string | null;
  selectedCluster: Cluster | null;
}

// ---------------------------------------------------------------------------
// SIEM & Alerts
// ---------------------------------------------------------------------------

export type SIEMAlertSeverity = 'informational' | 'low' | 'medium' | 'high' | 'critical';
export type SIEMAlertStatus =
  | 'new'
  | 'acknowledged'
  | 'investigating'
  | 'resolved'
  | 'false_positive';

export interface SIEMAlert {
  id: string;
  ruleId: string;
  ruleName: string;
  description: string;
  severity: SIEMAlertSeverity;
  status: SIEMAlertStatus;
  source: string;
  experimentId?: string;
  timestamp: string;
  fields: Record<string, unknown>;
}

export interface SIEMValidationResult {
  expectedAlertCount: number;
  receivedAlertCount: number;
  alerts: SIEMAlert[];
  detected: boolean;
  detectionLatencyMs: number;
  coverage: number;
  details: string[];
}

export interface SIEMConfig {
  provider: 'splunk' | 'elastic' | 'sentinel' | 'other';
  endpoint: string;
  apiKey: string;
  indexName?: string;
  enabled: boolean;
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

export interface DashboardSummary {
  securityPostureScore: number;
  postureTrend: TrendData;
  experimentSummary: {
    total: number;
    running: number;
    completed: number;
    failed: number;
    pending: number;
  };
  recentExperiments: Experiment[];
  clusterHealth: ClusterHealth[];
  threatCoverage: ThreatCoverageData;
  experimentTrend: TimeSeriesData[];
  topAttackTypes: ChartDataPoint[];
  validationSuccessRate: TimeSeriesData[];
}

export interface TrendData {
  direction: 'up' | 'down' | 'stable';
  percentage: number;
  period: string;
}

export interface TimeSeriesData {
  timestamp: string;
  value: number;
  label?: string;
}

export interface ChartDataPoint {
  name: string;
  value: number;
  color?: string;
}

export interface ThreatCoverageData {
  totalControls: number;
  validated: number;
  passed: number;
  failed: number;
  untested: number;
  coverage: number;
}

export interface DashboardState {
  summary: DashboardSummary | null;
  isLoading: boolean;
  error: string | null;
  lastRefreshed: string | null;
}

// ---------------------------------------------------------------------------
// Reports
// ---------------------------------------------------------------------------

export type ReportType = 'experiment' | 'compliance' | 'executive' | 'trend';
export type ReportFormat = 'pdf' | 'csv' | 'json' | 'html';

// Backend report response (snake_case)
export interface ReportBackend {
  id: string;
  title: string;
  type: string;
  format: string;
  description?: string;
  experiment_ids?: string[];
  date_range_from?: string;
  date_range_to?: string;
  status: string;
  error_message?: string;
  download_url?: string;
  file_size?: number;
  generated_by?: string;
  created_at?: string;
  updated_at?: string;
}

// Frontend report interface (camelCase) - accepts both formats
export interface Report {
  id: string;
  title: string;
  type: ReportType;
  format: ReportFormat;
  description: string;
  experimentIds: string[];
  dateRange: {
    from: string;
    to: string;
  };
  status: 'generating' | 'ready' | 'error';
  downloadUrl?: string;
  fileSize?: number;
  generatedBy: string;
  createdAt: string;
}

// Transform backend Report to frontend Report format
export function normalizeReport(raw: ReportBackend): Report {
  return {
    id: raw.id,
    title: raw.title,
    type: raw.type as ReportType,
    format: raw.format as ReportFormat,
    description: raw.description || '',
    experimentIds: raw.experiment_ids || [],
    dateRange: {
      from: raw.date_range_from || '',
      to: raw.date_range_to || '',
    },
    status: (raw.status === 'generating' ||
    raw.status === 'ready' ||
    raw.status === 'error'
      ? raw.status
      : 'ready') as Report['status'],
    downloadUrl: raw.download_url,
    fileSize: raw.file_size,
    generatedBy: raw.generated_by || 'system',
    createdAt: raw.created_at || new Date().toISOString(),
  };
}

export interface ReportListState {
  reports: Report[];
  isLoading: boolean;
  error: string | null;
  filters: ReportFilters;
}

export interface ReportFilters {
  type: ReportType | 'all';
  dateFrom: string | null;
  dateTo: string | null;
}

// ---------------------------------------------------------------------------
// API Responses
// ---------------------------------------------------------------------------

export interface APIResponse<T> {
  success: boolean;
  data: T;
  message?: string;
  errors?: APIError[];
  metadata?: APIMetadata;
}

export interface APIError {
  code: string;
  message: string;
  field?: string;
  details?: string;
}

export interface APIMetadata {
  requestId: string;
  timestamp: string;
  version: string;
}

export interface PaginatedResponse<T> {
  items: T[];
  totalCount: number;
  page: number;
  pageSize: number;
  totalPages: number;
  hasNextPage: boolean;
  hasPreviousPage: boolean;
}

export interface PaginationParams {
  page: number;
  pageSize: number;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

export interface AppSettings {
  theme: 'light' | 'dark' | 'system';
  language: string;
  notifications: NotificationSettings;
  siem: SIEMConfig;
  defaultClusterId: string | null;
  defaultNamespace: string;
  autoRefreshInterval: number;
  experimentDefaults: {
    defaultTimeWindow: number;
    defaultNamespace: string;
    autoCleanup: boolean;
    retainLogs: boolean;
  };
}

export interface NotificationSettings {
  email: boolean;
  slack: boolean;
  webhook: boolean;
  slackWebhookUrl?: string;
  webhookUrl?: string;
  emailAddress?: string;
  onExperimentComplete: boolean;
  onExperimentFailed: boolean;
  onClusterDegraded: boolean;
  onSIEMAlert: boolean;
}

// ---------------------------------------------------------------------------
// Common / Shared
// ---------------------------------------------------------------------------

export type StatusType =
  | ExperimentStatus
  | ClusterStatus
  | SIEMAlertStatus
  | 'healthy'
  | 'unhealthy'
  | 'active'
  | 'inactive';

export interface SelectOption<T = string> {
  label: string;
  value: T;
}

export interface KeyValue {
  key: string;
  value: string;
}

export interface DateRange {
  from: string | null;
  to: string | null;
}

export interface BreadcrumbItem {
  label: string;
  path?: string;
  icon?: string;
}
