# Chaos-Sec Architecture Document

## 1. System Overview

Chaos-Sec follows a modular, microservices-inspired architecture designed for security, scalability, and maintainability. The system is composed of several interconnected components that work together to orchestrate security control validation experiments within Kubernetes environments.

### 1.1 High-Level Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CHAOS-SEC PLATFORM                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐                 │
│  │   Web UI     │────▶│  REST API    │────▶│  Auth &      │                 │
│  │  Dashboard   │◀────│   Gateway    │◀────│  RBAC Layer  │                 │
│  └──────────────┘     └──────────────┘     └──────────────┘                 │
│                            │                                                 │
│                            ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                        ORCHESTRATION ENGINE                          │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │   │
│  │  │ Experiment  │  │   Attack    │  │  Validation │  │  Scheduler  │  │   │
│  │  │   Manager   │  │   Executor  │  │   Engine    │  │   Service   │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘  │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                            │                                                 │
│                            ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                       KUBERNETES INTEGRATION                         │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │   │
│  │  │   Pod       │  │  Namespace  │  │   Network   │  │   Resource  │  │   │
│  │  │  Controller │  │   Manager   │  │   Policy    │  │   Monitor   │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘  │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                            │                                                 │
│                            ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                      EXTERNAL INTEGRATIONS                           │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │   │
│  │  │   Mock      │  │  SIEM       │  │  Logging &  │                  │   │
│  │  │   SIEM      │  │  Connector  │  │  Metrics    │                  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                  │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │     TARGET KUBERNETES         │
                    │         CLUSTER               │
                    │  ┌─────────┐  ┌─────────┐    │
                    │  │ Attacker│  │ Target  │    │
                    │  │   Pods  │  │  Pods   │    │
                    │  └─────────┘  └─────────┘    │
                    └───────────────────────────────┘
```

## 2. Component Architecture

### 2.1 Frontend Layer (Web Dashboard)

**Purpose:** Provides the user interface for configuring, executing, and monitoring security experiments.

**Components:**

| Component | Responsibility |
|-----------|----------------|
| Dashboard UI | Main landing page with experiment overview and status |
| Experiment Builder | Form-based interface for creating attack scenarios |
| Real-time Monitor | Live visualization of running experiments |
| Results Viewer | Historical data and validation reports |
| Settings Panel | Configuration management for clusters and integrations |

**Technology Stack:**
- Framework: React.js with TypeScript
- State Management: Redux Toolkit
- UI Components: Material-UI or Ant Design
- Real-time Updates: WebSocket connection
- Visualization: D3.js or Chart.js for metrics

### 2.2 API Gateway Layer

**Purpose:** Acts as the single entry point for all client requests, handling routing, authentication, and rate limiting.

**Components:**

| Component | Responsibility |
|-----------|----------------|
| Request Router | Routes incoming requests to appropriate services |
| Authentication | Validates JWT tokens and API keys |
| Rate Limiter | Prevents abuse through request throttling |
| Request Logger | Logs all incoming requests for audit trails |
| Response Formatter | Standardizes response formats across services |

**API Endpoints Overview:**

```
┌─────────────────────────────────────────────────────────────────┐
│                      API GATEWAY ROUTES                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  /api/v1/auth                                                    │
│  ├── POST   /login          - User authentication               │
│  ├── POST   /logout         - Session termination               │
│  ├── POST   /refresh        - Token refresh                     │
│  └── GET    /me             - Current user info                 │
│                                                                  │
│  /api/v1/experiments                                             │
│  ├── GET    /              - List all experiments               │
│  ├── POST   /              - Create new experiment              │
│  ├── GET    /:id           - Get experiment details             │
│  ├── PUT    /:id           - Update experiment                  │
│  ├── DELETE /:id           - Delete experiment                  │
│  ├── POST   /:id/run       - Execute experiment                 │
│  └── POST   /:id/stop      - Stop running experiment            │
│                                                                  │
│  /api/v1/clusters                                                │
│  ├── GET    /              - List registered clusters           │
│  ├── POST   /              - Register new cluster               │
│  ├── GET    /:id           - Get cluster details                │
│  ├── DELETE /:id           - Remove cluster                     │
│  └── GET    /:id/status    - Get cluster health status          │
│                                                                  │
│  /api/v1/results                                                 │
│  ├── GET    /              - List all results                   │
│  ├── GET    /:id           - Get specific result                │
│  └── GET    /export        - Export results (CSV/PDF)           │
│                                                                  │
│  /api/v1/reports                                                 │
│  ├── GET    /              - List generated reports             │
│  ├── POST   /              - Generate new report                │
│  └── GET    /:id           - Download report                    │
│                                                                  │
│  /api/v1/settings                                                │
│  ├── GET    /              - Get system settings                │
│  ├── PUT    /              - Update system settings             │
│  └── GET    /siem          - SIEM configuration                 │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.3 Orchestration Engine

**Purpose:** The core component that manages the lifecycle of security experiments.

**Components:**

#### 2.3.1 Experiment Manager

```
┌─────────────────────────────────────────────────────────────────┐
│                     EXPERIMENT MANAGER                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │ Experiment  │    │ Experiment  │    │ Experiment  │         │
│  │  Template   │───▶│  Instance   │───▶│   History   │         │
│  │   Store     │    │   Manager   │    │   Archive   │         │
│  └─────────────┘    └─────────────┘    └─────────────┘         │
│                                                                  │
│  Responsibilities:                                               │
│  • Store experiment templates (JSON/YAML)                       │
│  • Create experiment instances from templates                   │
│  • Track experiment state (pending, running, completed, failed) │
│  • Maintain historical records for audit                        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.3.2 Attack Executor

```
┌─────────────────────────────────────────────────────────────────┐
│                      ATTACK EXECUTOR                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              ATTACK VECTOR LIBRARY                       │    │
│  ├─────────────────────────────────────────────────────────┤    │
│  │                                                          │    │
│  │  Network Attacks:                                        │    │
│  │  ├── Pod Egress Test                                    │    │
│  │  ├── Pod Ingress Test                                   │    │
│  │  ├── Network Policy Bypass Attempt                      │    │
│  │  └── DNS Spoofing Simulation                            │    │
│  │                                                          │    │
│  │  Access Control Attacks:                                 │    │
│  │  ├── RBAC Privilege Escalation Test                     │    │
│  │  ├── Service Account Token Theft                        │    │
│  │  └── Secret Access Attempt                              │    │
│  │                                                          │    │
│  │  Resource Attacks:                                       │    │
│  │  ├── Resource Exhaustion (CPU/Memory)                   │    │
│  │  ├── Pod Density Test                                   │    │
│  │  └── Storage Quota Test                                 │    │
│  │                                                          │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Execution Flow:                                                 │
│  1. Load attack payload configuration                           │
│  2. Validate target cluster connectivity                        │
│  3. Deploy attacker pod to target namespace                     │
│  4. Execute attack vector                                       │
│  5. Collect execution results                                   │
│  6. Clean up attacker pod                                       │
│  7. Return results to validation engine                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.3.3 Validation Engine

```
┌─────────────────────────────────────────────────────────────────┐
│                     VALIDATION ENGINE                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │   Alert     │    │   Alert     │    │ Validation│         │
│  │  Collector  │───▶│  Correlator │───▶│  Scorer   │         │
│  └─────────────┘    └─────────────┘    └─────────────┘         │
│        │                                      │                 │
│        ▼                                      ▼                 │
│  ┌─────────────┐                        ┌─────────────┐        │
│  │   SIEM      │                        │   Report    │        │
│  │  Query      │                        │ Generator   │        │
│  │  Interface  │                        │             │        │
│  └─────────────┘                        └─────────────┘        │
│                                                                  │
│  Validation Process:                                             │
│  1. Record attack timestamp and signature                       │
│  2. Query SIEM for matching alerts (within time window)         │
│  3. Correlate alerts with attack signature                      │
│  4. Calculate validation score:                                 │
│     - PASS: Alert detected and correlated                       │
│     - FAIL: No alert detected                                   │
│     - PARTIAL: Alert detected but incomplete correlation        │
│  5. Generate validation report                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.3.4 Scheduler Service

```
┌─────────────────────────────────────────────────────────────────┐
│                      SCHEDULER SERVICE                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Scheduling Modes:                                               │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  ON-DEMAND           │ Manual execution via UI/API      │    │
│  │  RECURRING           │ Cron-based scheduled execution   │    │
│  │  EVENT-TRIGGERED     │ Execute on cluster events        │    │
│  │  CONTINUOUS          │ Continuous validation mode       │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Scheduler Components:                                           │
│  • Job Queue: Priority-based experiment queue                   │
│  • Worker Pool: Configurable concurrent execution               │
│  • Retry Logic: Exponential backoff for failed executions       │
│  • Cooldown Period: Prevent rapid successive executions         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.4 Kubernetes Integration Layer

**Purpose:** Provides abstraction for interacting with Kubernetes clusters.

**Components:**

#### 2.4.1 Pod Controller

```go
// Conceptual interface for Pod Controller
type PodController interface {
    // CreateAttackerPod creates a temporary pod for attack execution
    CreateAttackerPod(ctx context.Context, config AttackConfig) (*v1.Pod, error)
    
    // WaitForPodReady waits for pod to reach ready state
    WaitForPodReady(ctx context.Context, podName, namespace string, timeout time.Duration) error
    
    // ExecuteInPod runs a command inside the attacker pod
    ExecuteInPod(ctx context.Context, podName, namespace, command string) (string, error)
    
    // DeletePod cleans up the attacker pod
    DeletePod(ctx context.Context, podName, namespace string) error
    
    // GetPodLogs retrieves logs from the attacker pod
    GetPodLogs(ctx context.Context, podName, namespace string) (string, error)
}
```

#### 2.4.2 Namespace Manager

```
┌─────────────────────────────────────────────────────────────────┐
│                     NAMESPACE MANAGER                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Responsibilities:                                               │
│  • Create isolated namespaces for experiments                   │
│  • Apply resource quotas and limits                             │
│  • Manage namespace lifecycle (create, cleanup)                 │
│  • Ensure experiment isolation from production workloads        │
│                                                                  │
│  Namespace Structure:                                            │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  chaos-sec-{cluster-id}-{experiment-id}                 │    │
│  │                                                          │    │
│  │  Resources:                                              │    │
│  │  ├── Attacker Pod(s)                                    │    │
│  │  ├── Target Pod(s) - if needed for testing              │    │
│  │  ├── Network Policies - experiment-specific             │    │
│  │  ├── Service Accounts - minimal permissions             │    │
│  │  └── ConfigMaps - attack configurations                 │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.4.3 Network Policy Controller

```
┌─────────────────────────────────────────────────────────────────┐
│                  NETWORK POLICY CONTROLLER                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Capabilities:                                                   │
│  • Read existing network policies                               │
│  • Create temporary test policies                               │
│  • Validate policy enforcement                                  │
│  • Report policy gaps                                           │
│                                                                  │
│  Policy Validation Tests:                                        │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  Test Name              │ Expected Result               │    │
│  ├─────────────────────────────────────────────────────────┤    │
│  │  Egress Block           │ Connection should be denied   │    │
│  │  Ingress Block          │ Incoming connection denied    │    │
│  │  Namespace Isolation    │ Cross-ns traffic blocked      │    │
│  │  Port Restriction       │ Specific ports blocked        │    │
│  │  CIDR Restriction       │ IP range restrictions work    │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.4.4 Resource Monitor

```
┌─────────────────────────────────────────────────────────────────┐
│                      RESOURCE MONITOR                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Monitored Metrics:                                              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  Category        │ Metrics                              │    │
│  ├─────────────────────────────────────────────────────────┤    │
│  │  Pod Resources   │ CPU usage, Memory usage, Restart count│   │
│  │  Node Resources  │ CPU pressure, Memory pressure, Disk  │    │
│  │  Network         │ Bandwidth, Packet drops, Latency     │    │
│  │  Storage         │ Volume usage, IOPS, Throughput       │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Integration:                                                    │
│  • Kubernetes Metrics API                                       │
│  • Prometheus (optional)                                        │
│  • Custom metrics endpoint                                      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.5 External Integrations Layer

#### 2.5.1 Mock SIEM System

```
┌─────────────────────────────────────────────────────────────────┐
│                       MOCK SIEM SYSTEM                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Purpose: Simulates a SIEM for development and testing           │
│                                                                  │
│  Components:                                                     │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │   Alert     │    │   Alert     │    │   Query     │         │
│  │  Generator  │───▶│   Store     │───▶│   Engine    │         │
│  └─────────────┘    └─────────────┘    └─────────────┘         │
│                                                                  │
│  Alert Schema:                                                   │
│  {                                                               │
│    "id": "uuid",                                                 │
│    "timestamp": "ISO8601",                                       │
│    "severity": "low|medium|high|critical",                       │
│    "source": "string",                                           │
│    "alert_type": "string",                                       │
│    "description": "string",                                      │
│    "metadata": {}                                                │
│  }                                                               │
│                                                                  │
│  API Endpoints:                                                  │
│  • POST /api/alerts - Ingest new alerts                         │
│  • GET  /api/alerts - Query alerts with filters                 │
│  • GET  /api/alerts/:id - Get specific alert                    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.5.2 SIEM Connector

```
┌─────────────────────────────────────────────────────────────────┐
│                       SIEM CONNECTOR                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Supported SIEM Platforms:                                       │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  SIEM Platform     │ Connection Method                  │    │
│  ├─────────────────────────────────────────────────────────┤    │
│  │  Splunk           │ REST API, HEC                      │    │
│  │  ELK Stack        │ Elasticsearch API                  │    │
│  │  IBM QRadar       │ REST API                           │    │
│  │  Azure Sentinel   │ Azure API, Log Analytics           │    │
│  │  Mock SIEM        │ Internal REST API                  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Connector Interface:                                            │
│  type SIEMConnector interface {                                  │
│      Connect(ctx context.Context, config SIEMConfig) error      │
│      QueryAlerts(ctx context.Context, query AlertQuery)         │
│          ([]Alert, error)                                       │
│      SendAlert(ctx context.Context, alert Alert) error          │
│      HealthCheck(ctx context.Context) error                     │
│  }                                                               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 2.5.3 Logging and Metrics

```
┌─────────────────────────────────────────────────────────────────┐
│                   LOGGING AND METRICS                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Logging Strategy:                                               │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  Level      │ Purpose                                  │    │
│  ├─────────────────────────────────────────────────────────┤    │
│  │  ERROR      │ System errors, failures                  │    │
│  │  WARN       │ Warnings, non-critical issues            │    │
│  │  INFO       │ General operational information          │    │
│  │  DEBUG      │ Detailed debugging information           │    │
│  │  AUDIT      │ Security audit trail                     │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Metrics Collection:                                             │
│  • Experiment execution count                                   │
│  • Success/failure rates                                        │
│  • Average execution time                                       │
│  • SIEM alert correlation rate                                  │
│  • Cluster health metrics                                       │
│                                                                  │
│  Export Formats:                                                 │
│  • Prometheus metrics endpoint                                  │
│  • JSON logs for log aggregation                                │
│  • CSV export for analysis                                      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## 3. Data Flow Architecture

### 3.1 Experiment Execution Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    EXPERIMENT EXECUTION DATA FLOW                            │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
  │  User   │────▶│   Web   │────▶│   API   │────▶│Experiment│────▶│ Attack  │
  │         │     │   UI    │     │ Gateway │     │ Manager │     │Executor │
  └─────────┘     └─────────┘     └─────────┘     └─────────┘     └─────────┘
      │                                                       │
      │                                                       ▼
      │                                              ┌─────────────────┐
      │                                              │   Kubernetes    │
      │                                              │     Cluster     │
      │                                              │  ┌───────────┐  │
      │                                              │  │ Attacker  │  │
      │                                              │  │    Pod    │  │
      │                                              │  └───────────┘  │
      │                                              └─────────────────┘
      │                                                       │
      │                                                       ▼
      │                                              ┌─────────────────┐
      │                                              │   Validation    │
      │                                              │    Engine       │
      │                                              └─────────────────┘
      │                                                       │
      │                                                       ▼
      │                                              ┌─────────────────┐
      │◀─────────────────────────────────────────────│     SIEM        │
      │                                              │   (Query)       │
      │                                              └─────────────────┘
      ▼
  ┌─────────┐     ┌─────────┐     ┌─────────┐
  │ Results │◀────│ Results│◀────│Validation│
  │ Display │     │ Store  │     │  Score  │
  └─────────┘     └─────────┘     └─────────┘
```

### 3.2 Detailed Step-by-Step Flow

```
Step 1: User Configuration
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. User logs into the dashboard                                            │
│ 2. User selects or creates an experiment template                          │
│ 3. User configures experiment parameters:                                  │
│    - Target cluster                                                        │
│    - Attack type                                                           │
│    - Target namespace/pods                                                 │
│    - Validation criteria                                                   │
│ 4. User initiates experiment execution                                     │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Step 2: Request Processing
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. API Gateway receives request                                            │
│ 2. Authentication token is validated                                       │
│ 3. RBAC check confirms user permissions                                    │
│ 4. Request is routed to Experiment Manager                                 │
│ 5. Experiment instance is created with unique ID                           │
│ 6. Experiment state set to "PENDING"                                       │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Step 3: Attack Execution
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. Experiment Manager passes config to Attack Executor                     │
│ 2. Attack Executor validates cluster connectivity                          │
│ 3. Isolated namespace is created (if not exists)                           │
│ 4. Attacker pod is deployed with attack payload                            │
│ 5. Pod waits for ready state                                               │
│ 6. Attack command is executed inside pod                                   │
│ 7. Attack results are captured                                             │
│ 8. Attacker pod is deleted (cleanup)                                       │
│ 9. Experiment state updated to "EXECUTED"                                  │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Step 4: Validation
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. Validation Engine receives attack signature and timestamp               │
│ 2. SIEM Connector queries SIEM for matching alerts                         │
│ 3. Alert correlation is performed:                                         │
│    - Match by timestamp (within configured window)                         │
│    - Match by alert type/signature                                         │
│    - Match by source/destination                                           │
│ 4. Validation score is calculated                                          │
│ 5. Experiment state updated to "VALIDATED"                                 │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Step 5: Results Storage and Display
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. Results are stored in database                                          │
│ 2. Real-time update sent via WebSocket to UI                               │
│ 3. Dashboard displays validation result:                                   │
│    - PASS: Security control working correctly                              │
│    - FAIL: Security control not detecting attack                           │
│    - PARTIAL: Partial detection                                            │
│ 4. Report can be generated and exported                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 Data Flow Diagrams

#### 3.3.1 Read Operations (Query Flow)

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│   User   │────▶│   Web    │────▶│   API    │────▶│  Auth    │────▶│ Database │
│          │     │   UI     │     │ Gateway  │     │  Check   │     │  Query   │
└──────────┘     └──────────┘     └──────────┘     └──────────┘     └──────────┘
     ▲                                                                   │
     │                                                                   ▼
     │                                                           ┌──────────┐
     │                                                           │  Data    │
     │◀──────────────────────────────────────────────────────────│  Return  │
     │                                                           └──────────┘
     │
     │                   Real-time Updates
     │                   ┌──────────┐
     └───────────────────│WebSocket │
                         │ Server   │
                         └──────────┘
```

#### 3.3.2 Write Operations (Experiment Creation)

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│   User   │────▶│   Web    │────▶│   API    │────▶│Validate  │────▶│Experiment│
│          │     │   UI     │     │ Gateway  │     │  Config  │     │  Create  │
└──────────┘     └──────────┘     └──────────┘     └──────────┘     └──────────┘
                                                                         │
                                                                         ▼
                                                                   ┌──────────┐
                                                                   │ Database │
                                                                   │  Insert  │
                                                                   └──────────┘
                                                                         │
                                                                         ▼
                                                                   ┌──────────┐
                                                                   │  Event   │
                                                                   │  Publish │
                                                                   └──────────┘
```

## 4. Deployment Architecture

### 4.1 Deployment Topology

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PRODUCTION DEPLOYMENT                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    CHAOS-SEC CONTROL PLANE                           │    │
│  │                     (Dedicated Namespace)                            │    │
│  │                                                                      │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │    │
│  │  │     Web     │  │     API     │  │  Database   │                  │    │
│  │  │   Frontend  │  │   Backend   │  │  (PostgreSQL│                  │    │
│  │  │   (React)   │  │    (Go)     │  │   + Redis)  │                  │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                  │    │
│  │                                                                      │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                  │    │
│  │  │   Mock      │  │  Message    │  │  Monitoring │                  │    │
│  │  │   SIEM      │  │   Queue     │  │  (Prometheus│                  │    │
│  │  │             │  │  (RabbitMQ) │  │   + Grafana)│                  │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                  │    │
│  │                                                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│                              │    │    │                                     │
│                    ┌─────────┘    │    └─────────┐                          │
│                    ▼              ▼              ▼                          │
│  ┌─────────────────────┐ ┌─────────────────┐ ┌─────────────────────┐       │
│  │   Target Cluster    │ │  Target Cluster │ │   Target Cluster    │       │
│  │   (Development)     │ │   (Staging)     │ │   (Production)      │       │
│  │                     │ │                 │ │                     │       │
│  │  ┌───────────────┐  │ │ ┌─────────────┐ │ │ ┌───────────────┐   │       │
│  │  │   Attacker    │  │ │ │   Attacker  │ │ │ │   Attacker    │   │       │
│  │  │     Pods      │  │ │ │    Pods     │ │ │ │     Pods      │   │       │
│  │  └───────────────┘  │ │ └─────────────┘ │ │ └───────────────┘   │       │
│  └─────────────────────┘ └─────────────────┘ └─────────────────────┘       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Component Deployment Specifications

| Component | Deployment Type | Replicas | Resource Requirements |
|-----------|-----------------|----------|----------------------|
| Web Frontend | Deployment | 2 | 100m CPU, 128Mi Memory |
| API Backend | Deployment | 3 | 500m CPU, 512Mi Memory |
| Database (PostgreSQL) | StatefulSet | 1 | 1 CPU, 2Gi Memory, 20Gi Storage |
| Redis Cache | Deployment | 1 | 250m CPU, 256Mi Memory |
| Mock SIEM | Deployment | 1 | 250m CPU, 512Mi Memory |
| Message Queue | Deployment | 1 | 500m CPU, 512Mi Memory |
| Prometheus | Deployment | 1 | 500m CPU, 1Gi Memory |
| Grafana | Deployment | 1 | 250m CPU, 256Mi Memory |

### 4.3 Network Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          NETWORK ARCHITECTURE                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Internet                                                                    │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────┐                                                            │
│  │   Load      │                                                            │
│  │  Balancer   │                                                            │
│  │  (Ingress)  │                                                            │
│  └─────────────┘                                                            │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    DMZ Network Zone                                   │    │
│  │  ┌─────────────┐  ┌─────────────┐                                   │    │
│  │  │   Ingress   │  │   WAF       │                                   │    │
│  │  │  Controller │  │  (Optional) │                                   │    │
│  │  └─────────────┘  └─────────────┘                                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                   Application Network Zone                          │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │   Web       │  │    API      │  │   Mock      │                 │    │
│  │  │  Frontend   │  │   Backend   │  │   SIEM      │                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     Data Network Zone                               │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │  PostgreSQL │  │    Redis    │  │  Message    │                 │    │
│  │  │  Database   │  │    Cache    │  │   Queue     │                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 5. Security Architecture

### 5.1 Security Layers

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SECURITY LAYERS                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Layer 7: Application Security                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • Input validation                                                   │    │
│  │ • SQL injection prevention                                           │    │
│  │ • XSS protection                                                     │    │
│  │ • CSRF tokens                                                        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Layer 6: API Security                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • JWT authentication                                                 │    │
│  │ • Rate limiting                                                      │    │
│  │ • API key management                                                 │    │
│  │ • Request signing                                                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Layer 5: Network Security                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • Network policies                                                   │    │
│  │ • TLS encryption (mTLS)                                              │    │
│  │ • Service mesh (optional)                                            │    │
│  │ • Pod security policies                                              │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Layer 4: Access Control                                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • RBAC (Role-Based Access Control)                                   │    │
│  │ • Principle of least privilege                                       │    │
│  │ • Service account isolation                                          │    │
│  │ • Audit logging                                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Layer 3: Data Security                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ • Encryption at rest                                                 │    │
│  │ • Encryption in transit                                              │    │
│  │ • Secret management (Vault/K8s Secrets)                              │    │
│  │ • Data masking                                                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Authentication and Authorization Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    AUTHENTICATION & AUTHORIZATION FLOW                       │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
  │  User   │────▶│  Login  │────▶│   Auth  │────▶│   JWT   │
  │         │     │  Page   │     │ Service │     │  Token  │
  └─────────┘     └─────────┘     └─────────┘     └─────────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │  PostgreSQL   │
                                   │  (User DB)    │
                                   └───────────────┘

  Subsequent Requests:

  ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
  │  User   │────▶│Request  │────▶│   API   │────▶│   JWT   │────▶│  RBAC   │
  │         │     │  with   │     │ Gateway │     │Validate │     │  Check  │
  │         │     │  Token  │     │         │     │         │     │         │
  └─────────┘     └─────────┘     └─────────┘     └─────────┘     └─────────┘
                                                                         │
                                          ┌──────────────────────────────┘
                                          │
                                          ▼
                                   ┌─────────────┐     ┌─────────────┐
                                   │   Access    │────▶│   Target    │
                                   │   Granted   │     │   Service   │
                                   └─────────────┘     └─────────────┘
```

### 5.3 RBAC Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            RBAC ROLE DEFINITIONS                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Role: Admin                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Permissions:                                                         │    │
│  │ • Full access to all experiments (CRUD)                             │    │
│  │ • Cluster management (register, remove)                             │    │
│  │ • User management                                                   │    │
│  │ • System configuration                                              │    │
│  │ • View all audit logs                                               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Role: Operator                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Permissions:                                                         │    │
│  │ • Create and execute experiments                                    │    │
│  │ • View experiment results                                           │    │
│  │ • Generate reports                                                  │    │
│  │ • View assigned clusters                                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Role: Viewer                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Permissions:                                                         │    │
│  │ • View experiments (read-only)                                      │    │
│  │ • View results and reports                                          │    │
│  │ • No execution permissions                                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Role: Service Account                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Permissions:                                                         │    │
│  │ • API access with scoped permissions                                │    │
│  │ • Limited to specific experiments                                   │    │
│  │ • Time-bound tokens                                                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 6. Scalability Considerations

### 6.1 Horizontal Scaling Strategy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      HORIZONTAL SCALING ARCHITECTURE                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  API Backend Scaling:                                                        │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐                        │
│  │   API   │  │   API   │  │   API   │  │   API   │   ... (n replicas)     │
│  │ Pod 1   │  │ Pod 2   │  │ Pod 3   │  │ Pod N   │                        │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘                        │
│       │            │            │            │                              │
│       └────────────┴────────────┴────────────┘                              │
│                            │                                                 │
│                            ▼                                                 │
│                    ┌───────────────┐                                        │
│                    │ Load Balancer │                                        │
│                    │   (Service)   │                                        │
│                    └───────────────┘                                        │
│                                                                              │
│  Auto-Scaling Triggers:                                                      │
│  • CPU utilization > 70%                                                    │
│  • Memory utilization > 80%                                                 │
│  • Request queue depth > 100                                                │
│  • Response time > 500ms                                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Database Scaling

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DATABASE SCALING STRATEGY                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Primary Database (PostgreSQL):                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  ┌─────────────┐                                                    │    │
│  │  │   Primary   │ ◀── Write operations                              │    │
│  │  │   Instance  │                                                    │    │
│  │  └─────────────┘                                                    │    │
│  │         │                                                           │    │
│  │         ▼ (Replication)                                             │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │  Replica 1  │  │  Replica 2  │  │  Replica N  │ ◀── Read ops    │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Cache Layer (Redis):                                                        │
│  • Session storage                                                          │
│  • Experiment result caching                                                │
│  • API response caching                                                     │
│  • Rate limiting counters                                                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 7. High Availability Design

### 7.1 Fault Tolerance Mechanisms

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        FAULT TOLERANCE MECHANISMS                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Component Redundancy:                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Component          │ Redundancy Strategy                           │    │
│  ├─────────────────────────────────────────────────────────────────────┤    │
│  │  API Backend        │ Multi-replica deployment (min 3)             │    │
│  │  Web Frontend       │ Multi-replica deployment (min 2)             │    │
│  │  Database           │ Primary-replica with automatic failover      │    │
│  │  Redis              │ Sentinel-based high availability             │    │
│  │  Message Queue      │ Cluster mode with mirroring                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Failure Detection:                                                          │
│  • Kubernetes liveness probes                                               │
│  • Readiness probes                                                         │
│  • Health check endpoints                                                   │
│  • External monitoring (Prometheus)                                         │
│                                                                              │
│  Recovery Mechanisms:                                                        │
│  • Automatic pod restart on failure                                         │
│  • Database failover to replica                                             │
│  • Message queue message replay                                             │
│  • Experiment retry with backoff                                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 8. Architecture Decision Records (ADRs)

### ADR-001: Go for Backend Development

| Attribute | Decision |
|-----------|----------|
| Status | Accepted |
| Context | Need a language suitable for Kubernetes integration, high performance, and concurrent operations |
| Decision | Use Go (Golang) for backend development |
| Rationale | • Native Kubernetes client library (client-go) <br> • Excellent concurrency support (goroutines) <br> • Strong typing and performance <br> • Single binary deployment <br> • Large ecosystem for cloud-native development |

### ADR-002: React for Frontend

| Attribute | Decision |
|-----------|----------|
| Status | Accepted |
| Context | Need a modern, component-based frontend framework with strong ecosystem |
| Decision | Use React.js with TypeScript |
| Rationale | • Large community and ecosystem <br> • Component reusability <br> • TypeScript for type safety <br> • Excellent state management options <br> • Strong support for real-time updates |

### ADR-003: PostgreSQL for Primary Database

| Attribute | Decision |
|-----------|----------|
| Status | Accepted |
| Context | Need a reliable, ACID-compliant database for experiment data and user management |
| Decision | Use PostgreSQL as primary database |
| Rationale | • ACID compliance <br> • Strong consistency guarantees <br> • JSON support for flexible schemas <br> • Excellent reliability and maturity <br> • Good Kubernetes support (operators available) |

### ADR-004: Kubernetes-Native Deployment

| Attribute | Decision |
|-----------|----------|
| Status | Accepted |
| Context | Target environment is Kubernetes; platform should be deployable on Kubernetes |
| Decision | Deploy Chaos-Sec itself on Kubernetes |
| Rationale | • Dogfooding the target environment <br> • Easy scaling and management <br> • Native service discovery <br> • Built-in health checking <br> • Simplified operations |

## 9. Future Architecture Considerations

### 9.1 Planned Enhancements

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        FUTURE ENHANCEMENTS                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Phase 2 Enhancements:                                                       │
│  • Service mesh integration (Istio/Linkerd)                                 │
│  • Multi-cluster orchestration                                              │
│  • Advanced attack chain scenarios                                          │
│  • Machine learning for anomaly detection                                   │
│                                                                              │
│  Phase 3 Enhancements:                                                       │
│  • Cloud provider integrations (AWS, Azure, GCP)                            │
│  • Compliance framework mappings (CIS, NIST, SOC2)                          │
│  • Advanced reporting and analytics                                         │
│  • Plugin architecture for custom attack vectors                            │
│                                                                              │
│  Long-term Vision:                                                           │
│  • Federated chaos engineering across organizations                         │
│  • Threat intelligence integration                                          │
│  • Automated remediation suggestions                                        │
│  • Community attack vector marketplace                                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 10. Appendix

### 10.1 Glossary

| Term | Definition |
|------|------------|
| Chaos Engineering | Discipline of experimenting on systems to build confidence in their capability to withstand turbulent conditions |
| Security Control | Safeguards or countermeasures to avoid, detect, counteract, or minimize security risks |
| SIEM | Security Information and Event Management - software that aggregates and analyzes security alerts |
| Attacker Pod | Temporary Kubernetes pod deployed to simulate malicious activity |
| Experiment | A defined security test scenario with specific attack vectors and validation criteria |
| Validation | Process of verifying that security controls detected the simulated attack |

### 10.2 References

- Kubernetes Documentation: https://kubernetes.io/docs/
- Chaos Engineering Principles: https://principlesofchaos.org/
- NIST Cybersecurity Framework: https://www.nist.gov/cyberframework
- Go Programming Language: https://golang.org/
- React Documentation: https://reactjs.org/