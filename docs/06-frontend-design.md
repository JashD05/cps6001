# Frontend Design Document

## Chaos-Sec Web Dashboard

**Version:** 1.0  
**Last Updated:** 2026-01-15  
**Status:** Draft

---

## Table of Contents

1. [Overview](#overview)
2. [Design Principles](#design-principles)
3. [Information Architecture](#information-architecture)
4. [User Interface Design](#user-interface-design)
5. [Wireframes](#wireframes)
6. [Component Architecture](#component-architecture)
7. [User Flows](#user-flows)
8. [Design System](#design-system)
9. [Responsive Design](#responsive-design)
10. [Accessibility](#accessibility)

---

## 1. Overview

### 1.1 Purpose

This document defines the user interface and user experience design for the Chaos-Sec web dashboard. The dashboard serves as the primary control interface for security teams to orchestrate, execute, and monitor security control validation experiments within Kubernetes environments.

### 1.2 Target Users

| User Type | Description | Primary Tasks |
|-----------|-------------|---------------|
| **Security Administrator** | Manages security policies and validations | Configure experiments, review results, generate compliance reports |
| **DevOps Engineer** | Manages Kubernetes clusters and deployments | Execute experiments, monitor cluster health, troubleshoot failures |
| **Security Analyst** | Monitors security posture and alerts | View dashboards, analyze trends, investigate failures |
| **Executive/Manager** | Oversees security program | View high-level reports, compliance status, risk metrics |

### 1.3 Platform Goals

- **Intuitive**: Users should be able to create and execute their first experiment within 5 minutes
- **Informative**: Real-time visibility into experiment status and security posture
- **Actionable**: Clear indicators of success/failure with recommended next steps
- **Professional**: Enterprise-grade UI suitable for security operations centers

---

## 2. Design Principles

### 2.1 Core Principles

| Principle | Description | Application |
|-----------|-------------|-------------|
| **Clarity Over Cleverness** | Prioritize clear communication over creative design | Use standard UI patterns, avoid ambiguous icons |
| **Progressive Disclosure** | Show essential information first, reveal details on demand | Dashboard summaries with drill-down capabilities |
| **Feedback Rich** | Provide immediate feedback for all user actions | Loading states, success/error notifications, progress indicators |
| **Security First** | Design with security considerations in mind | Clear permission indicators, audit trail visibility, secure defaults |
| **Consistency** | Maintain consistent patterns throughout the application | Unified component library, standardized interactions |

### 2.2 UX Heuristics

1. **Visibility of System Status**: Always show experiment states, progress, and system health
2. **Match Between System and Real World**: Use security terminology familiar to the target audience
3. **User Control and Freedom**: Allow users to cancel experiments, undo actions, navigate freely
4. **Error Prevention**: Validate inputs, confirm destructive actions, provide clear error messages
5. **Recognition Rather Than Recall**: Show recent experiments, save drafts, provide templates

---

## 3. Information Architecture

### 3.1 Site Map

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CHAOS-SEC DASHBOARD                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         TOP LEVEL NAVIGATION                        │    │
│  ├─────────────────────────────────────────────────────────────────────┤    │
│  │                                                                      │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐            │    │
│  │  │Dashboard │  │Experiments│  │Clusters  │  │Reports   │            │    │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘            │    │
│  │                                                                      │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐            │    │
│  │  │Templates │  │SIEM      │  │Settings  │  │Help      │            │    │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘            │    │
│  │                                                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  DASHBOARD (Home)                                                            │
│  ├── Summary Cards                                                           │
│  ├── Recent Experiments                                                      │
│  ├── Security Posture Overview                                               │
│  └── Quick Actions                                                           │
│                                                                              │
│  EXPERIMENTS                                                                 │
│  ├── List View                                                               │
│  │   ├── All Experiments                                                     │
│  │   ├── Running                                                             │
│  │   ├── Scheduled                                                           │
│  │   ├── Completed                                                           │
│  │   └── Failed                                                              │
│  ├── Experiment Detail                                                       │
│  │   ├── Overview                                                            │
│  │   ├── Configuration                                                       │
│  │   ├── Execution Results                                                   │
│  │   ├── SIEM Validation                                                     │
│  │   └── Logs                                                                │
│  └── Create Experiment                                                       │
│      ├── From Template                                                       │
│      └── Custom Experiment                                                   │
│                                                                              │
│  CLUSTERS                                                                    │
│  ├── Cluster List                                                            │
│  ├── Cluster Detail                                                          │
│  │   ├── Overview                                                            │
│  │   ├── Namespaces                                                          │
│  │   ├── Network Policies                                                    │
│  │   └── Health Status                                                       │
│  └── Register Cluster                                                        │
│                                                                              │
│  TEMPLATES                                                                   │
│  ├── Template Library                                                        │
│  ├── Template Detail                                                         │
│  └── Create Template                                                         │
│                                                                              │
│  SIEM                                                                        │
│  ├── Connection Status                                                       │
│  ├── Alert History                                                           │
│  ├── Query Builder                                                           │
│  └── Integration Settings                                                    │
│                                                                              │
│  REPORTS                                                                     │
│  ├── Report Library                                                          │
│  ├── Generate Report                                                         │
│  └── Scheduled Reports                                                       │
│                                                                              │
│  SETTINGS                                                                    │
│  ├── General Settings                                                        │
│  ├── User Management                                                         │
│  ├── API Keys                                                                │
│  ├── Notifications                                                           │
│  └── Audit Logs                                                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Navigation Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           NAVIGATION LAYOUT                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  [Logo] Chaos-Sec          Dashboard  Experiments  Clusters  Reports   │  │
│  │  ────────────────────────  Templates  SIEM        Settings  [User ▼]  │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌─────────────┬──────────────────────────────────────────────────────────┐  │
│  │             │                                                          │  │
│  │  Sidebar    │              MAIN CONTENT AREA                           │  │
│  │  (Optional) │                                                          │  │
│  │             │                                                          │  │
│  │  • Quick    │  ┌────────────────────────────────────────────────────┐  │  │
│  │    Links    │  │                                                    │  │  │
│  │             │  │              Page Content                          │  │  │
│  │  • Filters  │  │                                                    │  │  │
│  │             │  │                                                    │  │  │
│  │  • Recent   │  └────────────────────────────────────────────────────┘  │  │
│  │    Items    │                                                          │  │
│  │             │                                                          │  │
│  └─────────────┴──────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │  Status Bar: System Health | Connected Clusters | Active Experiments   │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. User Interface Design

### 4.1 Layout System

#### 4.1.1 Grid System

- **Base Grid**: 12-column fluid grid
- **Gutter Width**: 24px (desktop), 16px (tablet), 16px (mobile)
- **Margin**: 32px (desktop), 24px (tablet), 16px (mobile)
- **Breakpoints**:
  - Mobile: < 768px
  - Tablet: 768px - 1024px
  - Desktop: 1024px - 1440px
  - Large Desktop: > 1440px

#### 4.1.2 Spacing Scale

| Token | Value (px) | Usage |
|-------|------------|-------|
| `space-1` | 4 | Tight spacing, icon padding |
| `space-2` | 8 | Component internal spacing |
| `space-3` | 12 | Form element spacing |
| `space-4` | 16 | Standard spacing |
| `space-5` | 20 | Section spacing |
| `space-6` | 24 | Card padding |
| `space-8` | 32 | Section margins |
| `space-10` | 40 | Large section spacing |
| `space-12` | 48 | Page margins |
| `space-16` | 64 | Major section dividers |

### 4.2 Color Palette

#### 4.2.1 Primary Colors

| Color | Hex | Usage |
|-------|-----|-------|
| Primary | `#2563EB` | Primary actions, links, active states |
| Primary Dark | `#1E40AF` | Hover states, pressed states |
| Primary Light | `#DBEAFE` | Backgrounds, highlights |

#### 4.2.2 Semantic Colors

| Color | Hex | Usage |
|-------|-----|-------|
| Success | `#10B981` | Passed validations, healthy status |
| Warning | `#F59E0B` | Warnings, partial success |
| Error | `#EF4444` | Failures, errors, critical alerts |
| Info | `#3B82F6` | Informational messages |

#### 4.2.3 Neutral Colors

| Color | Hex | Usage |
|-------|-----|-------|
| Gray-900 | `#111827` | Primary text |
| Gray-700 | `#374151` | Secondary text |
| Gray-500 | `#6B7280` | Placeholder text, disabled |
| Gray-300 | `#D1D5DB` | Borders, dividers |
| Gray-100 | `#F3F4F6` | Backgrounds |
| White | `#FFFFFF` | Card backgrounds |

#### 4.2.4 Experiment Status Colors

| Status | Color | Hex |
|--------|-------|-----|
| Pending | Gray | `#9CA3AF` |
| Running | Blue | `#3B82F6` |
| Completed | Green | `#10B981` |
| Failed | Red | `#EF4444` |
| Cancelled | Orange | `#F59E0B` |

### 4.3 Typography

#### 4.3.1 Font Families

- **Primary Font**: Inter (sans-serif)
- **Monospace Font**: JetBrains Mono (code, logs)
- **Fallback**: system-ui, -apple-system, sans-serif

#### 4.3.2 Type Scale

| Token | Size (px) | Weight | Line Height | Usage |
|-------|-----------|--------|-------------|-------|
| `text-xs` | 12 | 400 | 1.5 | Captions, labels |
| `text-sm` | 14 | 400 | 1.5 | Body text, form labels |
| `text-base` | 16 | 400 | 1.5 | Default body text |
| `text-lg` | 18 | 400 | 1.5 | Subsections |
| `text-xl` | 20 | 500 | 1.4 | Card titles |
| `text-2xl` | 24 | 600 | 1.3 | Page titles |
| `text-3xl` | 30 | 600 | 1.3 | Dashboard headers |
| `text-4xl` | 36 | 700 | 1.2 | Hero text |

### 4.4 Iconography

- **Icon Library**: Heroicons or Material Icons
- **Icon Sizes**: 16px, 20px, 24px, 32px
- **Style**: Outlined for navigation, filled for status

#### Common Icons

| Icon | Usage |
|------|-------|
| 📊 Dashboard | Home/Dashboard |
| 🔬 Experiments | Experiment management |
| ☸️ Clusters | Kubernetes clusters |
| 📋 Templates | Experiment templates |
| 🛡️ SIEM | Security integration |
| 📄 Reports | Report generation |
| ⚙️ Settings | Configuration |
| ➕ Create | Add new item |
| ▶️ Run | Execute experiment |
| ⏹️ Stop | Stop execution |
| 🔄 Refresh | Reload data |
| 🔍 Search | Search functionality |
| 🔔 Notifications | Alerts and notifications |
| 👤 User | User profile |

---

## 5. Wireframes

### 5.1 Dashboard (Home Page)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo] Chaos-Sec    Dashboard  Experiments  Clusters  Reports  Settings   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                 [🔔] [Help] [User Name ▼]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Welcome back, Admin                              [+ New Experiment]         │
│  Last login: January 15, 2026 at 10:30 AM                                   │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        SECURITY POSTURE SCORE                          │  │
│  │                                                                       │  │
│  │     ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐        │  │
│  │     │   94%   │    │   25    │    │   22    │    │    3    │        │  │
│  │     │ Overall │    │ Controls│    │Validated│    │ Failed  │        │  │
│  │     │  Score  │    │  Tested │    │         │    │         │        │  │
│  │     └─────────┘    └─────────┘    └─────────┘    └─────────┘        │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌─────────────────────────────┐  ┌─────────────────────────────────────┐  │
│  │    EXPERIMENT ACTIVITY      │  │      SYSTEM HEALTH                  │  │
│  │                             │  │                                     │  │
│  │     [Line Chart]            │  │  ┌─────────────────────────────┐   │  │
│  │                             │  │  │ Clusters    │ 3 Online     │   │  │
│  │     ───────────────────     │  │  ├─────────────────────────────┤   │  │
│  │     Mon  Tue  Wed  Thu  Fri │  │  │ Experiments │ 12 Running   │   │  │
│  │                             │  │  ├─────────────────────────────┤   │  │
│  │     Total: 245 this week    │  │  │ SIEM        │ Connected    │   │  │
│  │     ↑ 12% from last week    │  │  └─────────────────────────────┘   │  │
│  │                             │  │                                     │  │
│  └─────────────────────────────┘  └─────────────────────────────────────┘  │
│                                                                              │
│  RECENT EXPERIMENTS                                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Name                  │ Status    │ Cluster      │ Time     │ Result │  │
│  ├───────────────────────────────────────────────────────────────────────┤  │
│  │ Production Egress     │ ● Running │ prod-cluster │ 2 min    │   —    │  │
│  │ Network Policy Test   │ ✓ Passed  │ staging      │ 1 hour   │  100%  │  │
│  │ RBAC Validation       │ ✗ Failed  │ prod-cluster │ 3 hours  │   45%  │  │
│  │ Secret Access Test    │ ✓ Passed  │ dev-cluster  │ 5 hours  │  100%  │  │
│  │ Pod Isolation Check   │ ✓ Passed  │ staging      │ 1 day    │  100%  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│  [View All Experiments →]                                                   │
│                                                                              │
│  QUICK ACTIONS                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│  │  [▶️] Run   │  │  [📋] New   │  │  [📊] View  │  │  [📄] Generate│     │
│  │  Experiment │  │  Template   │  │  Reports    │  │  Report       │     │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Experiment List Page

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo] Chaos-Sec    Dashboard  Experiments  Clusters  Reports  Settings   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                 [🔔] [Help] [User Name ▼]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Experiments                                              [+ New Experiment] │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ 🔍 Search experiments...                          [Filters ▼] [⋮]    │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Filter: [All Status ▼] [All Clusters ▼] [Date Range ▼] [Template ▼]       │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ ☐ │ Name/Template          │ Status    │ Cluster     │ Scheduled   │  │
│  ├───┼───────────────────────────────────────────────────────────────────│  │
│  │ ☐ │ Production Egress      │ ● Running │ prod-cluster│ 10:00 AM    │  │
│  │   │ Pod Egress Test        │           │             │             │  │
│  │   │                        │ Progress: 60%           │ Started 2m  │  │
│  ├───┼───────────────────────────────────────────────────────────────────│  │
│  │ ☐ │ Staging Network        │ ✓ Passed  │ staging     │ 09:00 AM    │  │
│  │   │ Network Policy Test    │           │             │             │  │
│  │   │                        │ Duration: 5m 12s        │ Today       │  │
│  ├───┼───────────────────────────────────────────────────────────────────│  │
│  │ ☐ │ RBAC Privilege Check   │ ✗ Failed  │ prod-cluster│ 08:00 AM    │  │
│  │   │ RBAC Validation        │           │             │             │  │
│  │   │                        │ Error: Permission denied│ Today       │  │
│  ├───┼───────────────────────────────────────────────────────────────────│  │
│  │ ☐ │ Dev Secret Access      │ ✓ Passed  │ dev-cluster │ Yesterday   │  │
│  │   │ Secret Access Test     │           │             │             │  │
│  │   │                        │ Duration: 3m 45s        │ Jan 14      │  │
│  └───┴───────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Showing 1-10 of 156 experiments                                            │
│  [◀ Previous] [1] [2] [3] ... [16] [Next ▶]                                 │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Selected: 0 items    [Bulk Delete] [Export Selected] [Run Selected]  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.3 Experiment Detail Page

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo] Chaos-Sec    Dashboard  Experiments  Clusters  Reports  Settings   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                 [🔔] [Help] [User Name ▼]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ← Back to Experiments                                                      │
│                                                                              │
│  Production Egress Test                                   [Edit] [Run] [⋮] │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│  Status: ● Running                          Experiment ID: exp_abc123xyz    │
│  Started: Jan 15, 2026 at 10:30:05 AM       Template: Pod Egress Test       │
│  Cluster: prod-cluster                        Namespace: production         │
│                                                                              │
│  Progress: ████████████████░░░░░░░░░░░░░░░░ 60%                             │
│  Estimated completion: 3 minutes                                            │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ [Overview] [Configuration] [Results] [SIEM Validation] [Logs]        │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  EXECUTION TIMELINE                                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │  ✓ Pod Creation           10:30:05 AM (5s)                          │  │
│  │    Attacker pod chaos-sec-attacker-abc123 created successfully       │  │
│  │                                                                       │  │
│  │  ✓ Network Setup          10:30:10 AM (3s)                          │  │
│  │    Network policies applied, connectivity verified                   │  │
│  │                                                                       │  │
│  │  ● Attack Execution       10:30:13 AM (running)                     │  │
│  │    Executing egress test to 8.8.8.8:53                               │  │
│  │                                                                       │  │
│  │  ○ SIEM Validation        (pending)                                 │  │
│  │    Waiting for attack completion                                     │  │
│  │                                                                       │  │
│  │  ○ Cleanup                (pending)                                 │  │
│  │    Will remove attacker pod and temporary resources                  │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  LIVE LOGS                                                                  │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ [10:30:13] INFO  Executing curl command to 8.8.8.8:53                │  │
│  │ [10:30:14] INFO  Connection attempt initiated...                     │  │
│  │ [10:30:15] WARN  Connection timeout - traffic may be blocked         │  │
│  │ [10:30:16] INFO  Retrying connection (attempt 2/3)                   │  │
│  │                                                                       │  │
│  │                                                                       │  │
│  │                                        [Auto-scroll] [Download Logs]  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ATTACK POD STATUS                                                          │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Pod Name: chaos-sec-attacker-abc123                                   │  │
│  │ Namespace: production                                                 │  │
│  │ Node: worker-node-03                                                  │  │
│  │ IP: 10.244.3.45                                                       │  │
│  │                                                                       │  │
│  │ Resource Usage:                                                       │  │
│  │ CPU:    ████████░░ 25m / 500m                                        │  │
│  │ Memory: ████░░░░░░ 64Mi / 512Mi                                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  [Stop Experiment]                                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.4 Create Experiment Wizard

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo] Chaos-Sec    Dashboard  Experiments  Clusters  Reports  Settings   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                 [🔔] [Help] [User Name ▼]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Create New Experiment                                                      │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│  Step 1 of 4: Select Template                                               │
│  ─────────────────────────────────────                                      │
│                                                                              │
│  Choose an attack template to use for this experiment:                      │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ 🔍 Search templates...                            [All Categories ▼] │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐            │  │
│  │  🌐 NETWORK     │  │  🔐 ACCESS      │  │  📦 RESOURCE    │            │  │
│  │                 │  │                 │  │                 │            │  │
│  │  Pod Egress     │  │  RBAC Test      │  │  CPU Stress     │            │  │
│  │  Test           │  │                 │  │                 │            │  │
│  │  ─────────────  │  │  ─────────────  │  │  ─────────────  │            │  │
│  │  Tests egress   │  │  Validates RBAC │  │  Tests resource │            │  │
│  │  network pol.   │  │  permissions    │  │  limits         │            │  │
│  │                 │  │                 │  │                 │            │  │
│  │  [Medium]       │  │  [High]         │  │  [Low]          │            │  │
│  │  [Select]       │  │  [Select]       │  │  [Select]       │            │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘            │  │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐            │  │
│  │  🌐 NETWORK     │  │  🔐 ACCESS      │  │  🗄️ DATA        │            │  │
│  │                 │  │                 │  │                 │            │  │
│  │  Pod Ingress    │  │  Secret Access  │  │  Data Exfil     │            │  │
│  │  Test           │  │  Test           │  │  Simulation     │            │  │
│  │  ─────────────  │  │  ─────────────  │  │  ─────────────  │            │  │
│  │  Tests ingress  │  │  Tests secret   │  │  Simulates data │            │  │
│  │  network pol.   │  │  access controls│  │  exfiltration   │            │  │
│  │                 │  │                 │  │                 │            │  │
│  │  [Medium]       │  │  [High]         │  │  [Critical]     │            │  │
│  │  [Select]       │  │  [Select]       │  │  [Select]       │            │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘            │  │
│                                                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [Cancel]                                           [Continue →]           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘


STEP 2: CONFIGURE
─────────────────
┌─────────────────────────────────────────────────────────────────────────────┐
│  Step 2 of 4: Configure Experiment                                          │
│  ─────────────────────────────────────                                      │
│                                                                              │
│  Basic Information                                                          │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Experiment Name *                                                     │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ Production Egress Validation                                      │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │ Description                                                           │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ Validating egress network policies in production namespace        │ │  │
│  │ │                                                                   │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Target Configuration                                                       │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Cluster *                                                             │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ prod-cluster (gke_project_us-central1_prod)               [▼]    │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │ Target Namespace *                                                    │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ production                                                  [▼]  │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │ Target Pod Labels (optional)                                          │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ app=api, tier=backend                                             │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │ Format: key=value, key2=value2                                        │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Attack Parameters                                                          │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Destination IP/Domain *                                               │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ 8.8.8.8                                                           │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │ Destination Port *                                                    │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ 53 (DNS)                                                    [▼]  │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │ Duration                                                              │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ 300 seconds (5 minutes)                                     [▼]  │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [← Back]                              [Save Draft]     [Continue →]       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘


STEP 3: VALIDATION
──────────────────
┌─────────────────────────────────────────────────────────────────────────────┐
│  Step 3 of 4: Validation Settings                                           │
│  ─────────────────────────────────────                                      │
│                                                                              │
│  SIEM Validation                                                            │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ ☑ Enable SIEM Alert Validation                                        │  │
│  │                                                                       │  │
│  │ Expected Alert Type *                                                 │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ NetworkPolicyViolation                                      [▼]  │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │ Expected Severity                                                     │  │
│  │ ○ Low  ● Medium  ○ High  ○ Critical                                  │  │
│  │                                                                       │  │
│  │ Alert Time Window                                                     │  │
│  │ ┌───────────────────────────────────────────────────────────────────┐ │  │
│  │ │ 60 seconds after attack execution                           [▼]  │ │  │
│  │ └───────────────────────────────────────────────────────────────────┘ │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Success Criteria                                                           │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Validation passes when:                                               │  │
│  │                                                                       │  │
│  │ ☑ Attack is blocked by security control                               │  │
│  │ ☑ SIEM alert is generated within time window                          │  │
│  │ ☐ Alert severity matches expected severity                            │  │
│  │ ☐ Additional custom conditions                                        │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Notification Settings                                                      │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ ☑ Notify on completion                                                │  │
│  │ ☑ Notify on failure                                                   │  │
│  │                                                                       │  │
│  │ Notification Channels:                                                │  │
│  │ ☑ Email (admin@example.com)                                          │  │
│  │ ☑ Slack (#security-alerts)                                           │  │
│  │ ☐ Webhook                                                            │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [← Back]                              [Save Draft]     [Continue →]       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘


STEP 4: REVIEW
──────────────
┌─────────────────────────────────────────────────────────────────────────────┐
│  Step 4 of 4: Review and Confirm                                            │
│  ─────────────────────────────────────                                      │
│                                                                              │
│  Experiment Summary                                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │ Name: Production Egress Validation                                    │  │
│  │ Template: Pod Egress Test                                             │  │
│  │ Cluster: prod-cluster (gke_project_us-central1_prod)                  │  │
│  │ Namespace: production                                                 │  │
│  │                                                                       │  │
│  │ Attack Configuration:                                                 │  │
│  │ • Destination: 8.8.8.8:53 (DNS)                                       │  │
│  │ • Duration: 300 seconds (5 minutes)                                   │  │
│  │                                                                       │  │
│  │ Validation:                                                           │  │
│  │ • SIEM Alert: NetworkPolicyViolation (Medium)                         │  │
│  │ • Time Window: 60 seconds                                             │  │
│  │                                                                       │  │
│  │ Notifications: Email, Slack                                           │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Schedule                                                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ ○ Run immediately                                                     │  │
│  │ ● Schedule for later                                                  │  │
│  │                                                                       │  │
│  │ Start Time: ┌─────────────────────────────────────────────────────┐  │  │
│  │             │ January 15, 2026 at 11:00 AM                  [📅] │  │  │
│  │             └─────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  │ ☐ Recurring experiment                                                │  │
│  │   ┌───────────────────────────────────────────────────────────────┐  │  │
│  │   │ Every [1] [Week ▼] on [Monday ▼] at [02:00 AM ▼]             │  │  │
│  │   └───────────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Safety Checks                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ ✓ Target namespace exists                                             │  │
│  │ ✓ Cluster is connected and healthy                                    │  │
│  │ ✓ User has required permissions                                       │  │
│  │ ✓ Resource quotas are sufficient                                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [← Back]                              [Save Draft]     [Create Experiment] │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.5 Cluster Management Page

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo] Chaos-Sec    Dashboard  Experiments  Clusters  Reports  Settings   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                 [🔔] [Help] [User Name ▼]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Kubernetes Clusters                                      [+ Register Cluster]│
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ 🔍 Search clusters...                             [Filters ▼] [⋮]   │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │  ● prod-cluster                        Kubernetes 1.28.3       │  │  │
│  │  │  ────────────────────────────────────────────────────────────   │  │  │
│  │  │  Status: Healthy                                                │  │  │
│  │  │  Context: gke_project_us-central1_prod                          │  │  │
│  │  │                                                                 │  │  │
│  │  │  Nodes: 5     Namespaces: 12     Experiments: 89               │  │  │
│  │  │                                                                 │  │  │
│  │  │  Last Heartbeat: 30 seconds ago                                 │  │  │
│  │  │                                                                 │  │  │
│  │  │  [View Details] [Run Experiment] [Edit] [⋮]                    │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │  ● staging-cluster                     Kubernetes 1.28.1       │  │  │
│  │  │  ────────────────────────────────────────────────────────────   │  │  │
│  │  │  Status: Healthy                                                │  │  │
│  │  │  Context: gke_project_us-east1_staging                          │  │  │
│  │  │                                                                 │  │  │
│  │  │  Nodes: 3     Namespaces: 8      Experiments: 45               │  │  │
│  │  │                                                                 │  │  │
│  │  │  Last Heartbeat: 1 minute ago                                   │  │  │
│  │  │                                                                 │  │  │
│  │  │  [View Details] [Run Experiment] [Edit] [⋮]                    │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │  ○ dev-cluster                         Kubernetes 1.27.5       │  │  │
│  │  │  ────────────────────────────────────────────────────────────   │  │  │
│  │  │  Status: Disconnected                                           │  │  │
│  │  │  Context: kind-dev-cluster                                      │  │  │
│  │  │                                                                 │  │  │
│  │  │  Nodes: 1     Namespaces: 5      Experiments: 22               │  │  │
│  │  │                                                                 │  │  │
│  │  │  Last Heartbeat: 2 hours ago                                    │  │  │
│  │  │                                                                 │  │  │
│  │  │  [View Details] [Reconnect] [Edit] [⋮]                         │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  CLUSTER STATISTICS                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│  │     3       │  │     2       │  │     1       │  │    156      │       │
│  │   Total     │  │  Healthy    │  │ Disconnected│  │ Experiments │       │
│  │  Clusters   │  │  Clusters   │  │  Clusters   │  │   Total     │       │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.6 SIEM Integration Page

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo] Chaos-Sec    Dashboard  Experiments  Clusters  Reports  Settings   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                 [🔔] [Help] [User Name ▼]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SIEM Integration                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│  CONNECTION STATUS                                                          │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │  ┌─────────┐                                                          │  │
│  │  │   ✓     │  SIEM Connection: Active                                 │  │
│  │  │ CONNECT │  Provider: Mock SIEM                                     │  │
│  │  └─────────┘  Endpoint: https://siem.chaos-sec.local:8089            │  │
│  │               Last Sync: January 15, 2026 at 10:35:00 AM             │  │
│  │               Latency: 45ms                                           │  │
│  │                                                                       │  │
│  │  [Test Connection] [Edit Configuration] [View Logs]                   │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ [Overview] [Alert History] [Query Builder] [Settings]                │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  RECENT ALERTS (Last 24 Hours)                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Time           │ Severity │ Type                  │ Experiment      │  │
│  ├───────────────────────────────────────────────────────────────────────┤  │
│  │ 10:30:16 AM    │ HIGH     │ NetworkPolicyViolation│ exp_abc123      │  │
│  │ 10:15:22 AM    │ MEDIUM   │ SuspiciousEgress      │ exp_def456      │  │
│  │ 09:45:10 AM    │ HIGH     │ RBACViolation         │ exp_ghi789      │  │
│  │ 09:30:05 AM    │ LOW      │ InfoEvent             │ exp_jkl012      │  │
│  │ 08:00:00 AM    │ MEDIUM   │ NetworkAnomaly        │ exp_mno345      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│  [View All Alerts →]                                                        │
│                                                                              │
│  VALIDATION STATISTICS                                                      │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │     ┌─────────────┐    ┌─────────────┐    ┌─────────────┐            │  │
│  │     │    156      │    │    142      │    │     14      │            │  │
│  │     │   Total     │    │   Matched   │    │   Missing   │            │  │
│  │     │   Alerts    │    │   Alerts    │    │   Alerts    │            │  │
│  │     └─────────────┘    └─────────────┘    └─────────────┘            │  │
│  │                                                                       │  │
│  │     Alert Correlation Rate: 91%                                       │  │
│  │     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 91%                 │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  QUICK QUERY                                                                │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Query: ┌────────────────────────────────────────────────────────────┐ │  │
│  │        │ SELECT * FROM alerts WHERE experiment_id = 'exp_abc123'   │ │  │
│  │        └────────────────────────────────────────────────────────────┘ │  │
│  │                                                                       │  │
│  │        [Run Query] [Save Query] [Export Results]                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. Component Architecture

### 6.1 Component Hierarchy

```
App
├── Layout
│   ├── Header
│   │   ├── Logo
│   │   ├── Navigation
│   │   ├── SearchBar
│   │   ├── NotificationBell
│   │   └── UserMenu
│   ├── Sidebar (optional)
│   │   ├── QuickLinks
│   │   └── FilterPanel
│   ├── MainContent
│   └── StatusBar
│
├── Pages
│   ├── Dashboard
│   │   ├── SecurityPostureCard
│   │   ├── ExperimentActivityChart
│   │   ├── SystemHealthPanel
│   │   ├── RecentExperimentsTable
│   │   └── QuickActionsGrid
│   │
│   ├── Experiments
│   │   ├── ExperimentList
│   │   │   ├── ExperimentTable
│   │   │   ├── ExperimentRow
│   │   │   ├── StatusBadge
│   │   │   └── Pagination
│   │   ├── ExperimentDetail
│   │   │   ├── ExperimentHeader
│   │   │   ├── ProgressTracker
│   │   │   ├── ExecutionTimeline
│   │   │   ├── LiveLogs
│   │   │   ├── AttackPodStatus
│   │   │   └── ResultSummary
│   │   └── CreateExperimentWizard
│   │       ├── TemplateSelector
│   │       ├── ConfigurationForm
│   │       ├── ValidationSettings
│   │       └── ReviewSummary
│   │
│   ├── Clusters
│   │   ├── ClusterList
│   │   ├── ClusterCard
│   │   ├── ClusterDetail
│   │   └── RegisterClusterForm
│   │
│   ├── Templates
│   │   ├── TemplateLibrary
│   │   ├── TemplateCard
│   │   └── TemplateEditor
│   │
│   ├── SIEM
│   │   ├── ConnectionStatus
│   │   ├── AlertHistory
│   │   ├── QueryBuilder
│   │   └── IntegrationSettings
│   │
│   ├── Reports
│   │   ├── ReportList
│   │   ├── ReportGenerator
│   │   └── ReportViewer
│   │
│   └── Settings
│       ├── GeneralSettings
│       ├── UserManagement
│       ├── APIKeys
│       ├── Notifications
│       └── AuditLogs
│
└── Shared Components
    ├── Buttons
    ├── Forms
    │   ├── Input
    │   ├── Select
    │   ├── Checkbox
    │   ├── Radio
    │   └── Toggle
    ├── Tables
    ├── Cards
    ├── Modals
    ├── Toasts
    ├── Charts
    ├── Icons
    └── Loaders
```

### 6.2 Key Component Specifications

#### 6.2.1 Status Badge Component

```
Component: StatusBadge
Purpose: Display experiment/cluster/system status with color coding

Props:
  - status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  - variant: 'dot' | 'pill' | 'icon'
  - size: 'small' | 'medium' | 'large'
  - showLabel: boolean

Visual States:
  ┌─────────────────────────────────────────────────────────────────┐
  │  Dot Variant:                                                   │
  │  ● Running    ✓ Completed    ✗ Failed    ○ Pending    ⦸ Cancelled│
  │                                                                 │
  │  Pill Variant:                                                  │
  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
  │  │● Running │ │✓ Completed│ │✗ Failed  │ │○ Pending │          │
  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
  │                                                                 │
  │  Icon Variant:                                                  │
  │  🔄 Running   ✅ Completed   ❌ Failed   ⏳ Pending   🚫 Cancelled│
  └─────────────────────────────────────────────────────────────────┘
```

#### 6.2.2 Progress Tracker Component

```
Component: ProgressTracker
Purpose: Display multi-step process progress (experiment execution, wizards)

Props:
  - steps: Array<{name, status, timestamp}>
  - currentStep: number
  - orientation: 'horizontal' | 'vertical'
  - showTimestamp: boolean

Visual States:
  Horizontal:
  ┌─────────────────────────────────────────────────────────────────┐
  │                                                                 │
  │  ✓ Pod Creation  ────  ✓ Network Setup  ────  ● Attack Exec    │
  │    10:30:05             10:30:10             10:30:13          │
  │                                                                 │
  │  ○ SIEM Validation ────  ○ Cleanup                              │
  │                                                                 │
  └─────────────────────────────────────────────────────────────────┘

  Vertical:
  ┌─────────────────────────────────────────────────────────────────┐
  │                                                                 │
  │  ✓ Pod Creation                                                 │
  │    Attacker pod created successfully                            │
  │    10:30:05 AM (5s)                                             │
  │    │                                                            │
  │  ✓ Network Setup                                                │
  │    Network policies applied                                     │
  │    10:30:10 AM (3s)                                             │
  │    │                                                            │
  │  ● Attack Execution                                             │
  │    Executing egress test to 8.8.8.8:53                          │
  │    10:30:13 AM (running)                                        │
  │    │                                                            │
  │  ○ SIEM Validation                                              │
  │    Waiting for attack completion                                │
  │    (pending)                                                    │
  │    │                                                            │
  │  ○ Cleanup                                                      │
  │    Will remove attacker pod                                     │
  │    (pending)                                                    │
  │                                                                 │
  └─────────────────────────────────────────────────────────────────┘
```

#### 6.2.3 Experiment Card Component

```
Component: ExperimentCard
Purpose: Display experiment summary in list/grid views

Props:
  - experiment: Experiment object
  - variant: 'list' | 'card' | 'compact'
  - selectable: boolean
  - actions: Array<Action>

List Variant:
┌─────────────────────────────────────────────────────────────────────────┐
│ ☐ │ Production Egress Test              │ ● Running │ prod-cluster │    │
│     Pod Egress Test                     │           │              │    │
│     ─────────────────────────────────────────────────────────────────  │
│     Progress: ████████████░░░░░░░░ 60%  │ Started 2m ago │ [View] [⋮] │
└─────────────────────────────────────────────────────────────────────────┘

Card Variant:
┌─────────────────────────────────────────┐
│  Production Egress Test           [⋮]  │
│  ─────────────────────────────────────  │
│  ● Running                              │
│                                         │
│  Template: Pod Egress Test              │
│  Cluster: prod-cluster                  │
│  Namespace: production                  │
│                                         │
│  Progress: ████████████░░░░░░░░ 60%    │
│  Started: 2 minutes ago                 │
│                                         │
│  [View Details]  [Stop]                 │
└─────────────────────────────────────────┘
```

#### 6.2.4 Security Posture Score Component

```
Component: SecurityPostureScore
Purpose: Display overall security posture as a gauge/score

Props:
  - score: number (0-100)
  - trend: 'up' | 'down' | 'stable'
  - trendValue: number (percentage change)
  - breakdown: Array<{name, score}>

Visual:
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│         SECURITY POSTURE SCORE                                          │
│                                                                         │
│              ┌─────────────┐                                           │
│         ╱''' │     94      │ '''╲                                      │
│       ╱      │   Excellent │      ╲                                    │
│      │       └─────────────┘       │                                   │
│      │                             │                                   │
│      │    ↑ 5% from last week     │                                   │
│      │                             │                                   │
│       ╲                           ╱                                    │
│         ╲_______         _______╱                                      │
│                 ''''''''''                                              │
│                                                                         │
│  Breakdown:                                                             │
│  Network Policies    ████████████████████░░ 92%                        │
│  RBAC Controls       ██████████████████████ 100%                       │
│  Secret Management   ██████████████████░░░░ 85%                        │
│  Pod Security        ███████████████████░░░ 90%                        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 7. User Flows

### 7.1 First-Time User Onboarding

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    FIRST-TIME USER ONBOARDING FLOW                          │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────┐
  │   User      │
  │  Receives   │
  │   Invite    │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │   Click     │
  │ Invitation  │
  │    Link     │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │   Set Up    │
  │  Password   │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │   Complete  │
  │   Profile   │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │  Welcome    │
  │  Tutorial   │◀────── Interactive walkthrough of dashboard
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │  Connect    │
  │   First     │◀────── Guided cluster registration
  │  Cluster    │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │  Run First  │
  │ Experiment  │◀────── Pre-configured sample experiment
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │  View       │
  │  Results    │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │ Onboarding  │
  │  Complete   │
  └─────────────┘
```

### 7.2 Create and Execute Experiment Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                  CREATE AND EXECUTE EXPERIMENT FLOW                         │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
  │   Navigate  │────▶│   Select    │────▶│  Configure  │
  │  to Create  │     │  Template   │     │  Parameters │
  │  Experiment │     │             │     │             │
  └─────────────┘     └─────────────┘     └─────────────┘
                                                 │
                                                 ▼
  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
  │   Execute   │◀────│   Review    │◀────│   Set Up    │
  │ Experiment  │     │  Summary    │     │ Validation  │
  └─────────────┘     └─────────────┘     └─────────────┘
       │
       ▼
  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
  │   Export    │◀────│   Generate  │◀────│   Query     │
  │   Report    │     │   Report    │     │   SIEM      │
  └─────────────┘     └─────────────┘     └─────────────┘
```

### 7.3 Incident Response Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      INCIDENT RESPONSE FLOW                                 │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────┐
  │  Experiment │
  │   Fails     │
  └─────────────┘
       │
       ▼
  ┌─────────────┐     ┌─────────────┐
  │  Alert      │────▶│  Security   │
  │ Generated   │     │  Team       │
  │ (Slack/     │     │ Notified    │
  │  Email)     │     └─────────────┘
  └─────────────┘            │
                             ▼
                      ┌─────────────┐
                      │   Review    │
                      │ Experiment  │
                      │   Details   │
                      └─────────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
       ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
       │  Security   │ │  Configure  │ │   Re-run    │
       │  Control    │ │  Remediation│ │  Experiment │
       │  is Broken  │ │             │ │  to Verify  │
       └─────────────┘ └─────────────┘ └─────────────┘
              │              │              │
              └──────────────┼──────────────┘
                             ▼
                      ┌─────────────┐
                      │   Update    │
                      │  Security   │
                      │  Posture    │
                      └─────────────┘
```

### 7.4 Scheduled Experiment Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      SCHEDULED EXPERIMENT FLOW                              │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────┐
  │  Experiment │
  │  Scheduled  │
  │  (Cron)     │
  └─────────────┘
       │
       ▼
  ┌─────────────┐
  │  Scheduler  │
  │   Triggers  │
  │  Execution  │
  └─────────────┘
       │
       ▼
  ┌─────────────┐     ┌─────────────┐
  │  Check      │────▶│  Abort if   │
  │  Cluster    │     │  Unhealthy  │
  │  Health     │     └─────────────┘
  └─────────────┘            │
       │                     │ No
       │ Yes                 ▼
       │            ┌─────────────┐
       │            │   Execute   │
       │            │ Experiment  │
       │            └─────────────┘
       │                 │
       │                 ▼
       │            ┌─────────────┐
       │            │   Store     │
       │            │   Results   │
       │            └─────────────┘
       │                 │
       │                 ▼
       │            ┌─────────────┐
       │            │   Send      │
       │            │ Notification│
       │            └─────────────┘
       │
       ▼
  ┌─────────────┐
  │   Log       │
  │  Schedule   │
  │   Status    │
  └─────────────┘
```

---

## 8. Design System

### 8.1 Component Library

The design system is built on top of Material-UI (MUI) v5 with custom theme extensions.

#### Button Variants

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BUTTON VARIANTS                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Primary Buttons:                                                            │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │  Create          │  │  [Disabled]      │  │   Loading...     │          │
│  │  Experiment      │  │  Create          │  │   ●●●            │          │
│  └──────────────────┘  │  Experiment      │  └──────────────────┘          │
│                        └──────────────────┘                                  │
│                                                                              │
│  Secondary Buttons:                                                          │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │  Cancel          │  │  [Disabled]      │  │   Loading...     │          │
│  │                  │  │  Cancel          │  │   ●●●            │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
│                                                                              │
│  Danger Buttons:                                                             │
│  ┌──────────────────┐  ┌──────────────────┐                                 │
│  │  Delete          │  │  [Disabled]      │                                 │
│  │  Experiment      │  │  Delete          │                                 │
│  └──────────────────┘  │  Experiment      │                                 │
│                        └──────────────────┘                                  │
│                                                                              │
│  Icon Buttons:                                                               │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐                                 │
│  │ ✏️ │ │ 🗑️ │ │ 📥 │ │ 🔄 │ │ ⋮  │ │ ✓  │                                 │
│  └────┘ └────┘ └────┘ └────┘ └────┘ └────┘                                 │
│   Edit   Delete  Download Refresh More   Confirm                            │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Form Elements

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            FORM ELEMENTS                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Text Input (Default):                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Label                                                               │    │
│  │ ┌─────────────────────────────────────────────────────────────────┐ │    │
│  │ │ Placeholder text                                                │ │    │
│  │ └─────────────────────────────────────────────────────────────────┘ │    │
│  │ Helper text                                                         │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Text Input (Focused):                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Label                                                               │    │
│  │ ╔═════════════════════════════════════════════════════════════════╗ │    │
│  │ ║ Active input text                                               ║ │    │
│  │ ╚═════════════════════════════════════════════════════════════════╝ │    │
│  │ Helper text                                                         │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Text Input (Error):                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Label                                           ⚠                   │    │
│  │ ╔═════════════════════════════════════════════════════════════════╗ │    │
│  │ ║ Invalid input value                                             ║ │    │
│  │ ╚═════════════════════════════════════════════════════════════════╝ │    │
│  │ ⚠ This field is required                                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Select Dropdown:                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Label                                                               │    │
│  │ ┌─────────────────────────────────────────────────────────────────┐ │    │
│  │ │ Selected option                                           [▼] │ │    │
│  │ └─────────────────────────────────────────────────────────────────┘ │    │
│  │ Helper text                                                         │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Checkbox:                                                                   │
│  ┌────┐ ☐ Unchecked  ☑ Checked  ☒ Disabled                                  │
│  │ ✓  │                                                                      │
│  └────┘                                                                      │
│                                                                              │
│  Toggle Switch:                                                              │
│  ┌──────────┐  ┌──────────┐                                                  │
│  │ ○─────── │  │ ───────● │  Off / On                                       │
│  └──────────┘  └──────────┘                                                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Animation Guidelines

| Animation | Duration | Easing | Usage |
|-----------|----------|--------|-------|
| Fade In | 200ms | ease-out | Modal appearances, toast notifications |
| Slide In | 300ms | ease-out | Page transitions, drawer openings |
| Scale | 150ms | ease-in-out | Button clicks, card expansions |
| Progress | 300ms | linear | Progress bars, loading states |
| Pulse | 1500ms | ease-in-out | Live indicators, recording states |

### 8.3 Loading States

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            LOADING STATES                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Page Load:                                                                  │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                               │  │
│  │  │░░░░░░░░░│  │░░░░░░░░░│  │░░░░░░░░░│  Skeleton loaders            │  │
│  │  │░░░░░░░░░│  │░░░░░░░░░│  │░░░░░░░░░│  for cards                   │  │
│  │  │░░░░░░░░░│  │░░░░░░░░░│  │░░░░░░░░░│                               │  │
│  │  └─────────┘  └─────────┘  └─────────┘                               │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  Spinner (Small):    ○●○  (24px)                                            │
│  Spinner (Medium):   ○●○  (40px)                                            │
│  Spinner (Large):    ○●○  (64px)                                            │
│                                                                              │
│  Progress Bar:                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ ████████████████░░░░░░░░░░░░░░░░░░░░░░░░ 45%                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Step Progress:                                                              │
│  ✓ Step 1  ────  ● Step 2  ────  ○ Step 3                                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 9. Responsive Design

### 9.1 Breakpoint Specifications

| Breakpoint | Min Width | Max Width | Layout Changes |
|------------|-----------|-----------|----------------|
| Mobile (XS) | 0px | 599px | Single column, collapsed navigation |
| Mobile (SM) | 600px | 767px | Single column, collapsible sidebar |
| Tablet (MD) | 768px | 1023px | Two columns, persistent sidebar |
| Desktop (LG) | 1024px | 1439px | Full layout with sidebar |
| Large Desktop (XL) | 1440px | ∞ | Expanded layout, additional panels |

### 9.2 Responsive Behavior

#### Mobile Layout (< 768px)

```
┌─────────────────────────┐
│ [☰] Logo        [🔔][👤]│  Header (sticky)
├─────────────────────────┤
│                         │
│     Main Content        │
│     (Full Width)        │
│                         │
│                         │
│                         │
├─────────────────────────┤
│ [📊] [🔬] [☸️] [⚙️]      │  Bottom Navigation
└─────────────────────────┘
```

#### Tablet Layout (768px - 1023px)

```
┌────────────────────────────────────────┐
│ [☰] Logo     Dashboard Experiments...  │  Header
├──────────────┬─────────────────────────┤
│              │                         │
│   Sidebar    │    Main Content         │
│  (Collapsible│    (Flexible Width)     │
│   Drawer)    │                         │
│              │                         │
│              │                         │
└──────────────┴─────────────────────────┘
```

#### Desktop Layout (≥ 1024px)

```
┌─────────────────────────────────────────────────────────┐
│ [Logo] Dashboard Experiments Clusters Reports Settings  │  Header
├──────────────┬──────────────────────────────────────────┤
│              │                                          │
│   Sidebar    │          Main Content                    │
│  (Persistent)│          (Full Width)                    │
│              │                                          │
│  • Quick     │                                          │
│    Links     │                                          │
│              │                                          │
│  • Filters   │                                          │
│              │                                          │
└──────────────┴──────────────────────────────────────────┘
```

### 9.3 Component Responsive Rules

| Component | Mobile | Tablet | Desktop |
|-----------|--------|--------|---------|
| Navigation | Bottom bar | Top bar + drawer | Top bar + sidebar |
| Tables | Card list | Scrollable table | Full table |
| Forms | Single column | Two columns | Multi-column |
| Charts | Stacked | Side-by-side | Dashboard grid |
| Modals | Full screen | Centered (90%) | Centered (max 600px) |
| Cards | Stacked | 2-column grid | 3-4 column grid |

---

## 10. Accessibility

### 10.1 WCAG 2.1 Compliance Targets

| Level | Target | Requirements |
|-------|--------|--------------|
| A | Required | Basic accessibility (keyboard nav, alt text) |
| AA | Target | Enhanced accessibility (color contrast, focus indicators) |
| AAA | Aspirational | Highest accessibility (sign language, extended audio) |

### 10.2 Accessibility Features

#### Keyboard Navigation

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        KEYBOARD NAVIGATION MAP                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Global Shortcuts:                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Tab / Shift+Tab    │ Navigate between interactive elements        │    │
│  │  Enter / Space      │ Activate focused element                     │    │
│  │  Escape             │ Close modal/dropdown                         │    │
│  │  /                  │ Focus search bar                             │    │
│  │  ?                  │ Open keyboard shortcuts help                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Page-Specific Shortcuts:                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Experiments Page:                                                   │    │
│  │  │  n            │ Create new experiment                            │    │
│  │  │  r            │ Run selected experiment                          │    │
│  │  │  d            │ Delete selected experiment                       │    │
│  │  │  /            │ Filter experiments                               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Color Contrast Requirements

| Element | Minimum Ratio | Target Ratio |
|---------|---------------|--------------|
| Normal Text | 4.5:1 | 7:1 |
| Large Text (18px+) | 3:1 | 4.5:1 |
| UI Components | 3:1 | 4.5:1 |
| Focus Indicators | 3:1 | 4.5:1 |

#### Screen Reader Support

- All interactive elements have accessible names
- Form inputs have associated labels
- Icons have aria-labels or aria-hidden as appropriate
- Live regions for dynamic content updates
- Skip links for main content navigation

#### Focus Management

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           FOCUS STYLES                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Default Focus:                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  ┌─────────────────────────────────────────────────────────────┐   │    │
│  │  │  Input with 2px blue outline                                │   │    │
│  │  └─────────────────────────────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Focus Within Modal:                                                         │
│  • Trap focus inside modal                                                   │
│  • Return focus to trigger element on close                                  │
│                                                                              │
│  Focus Order:                                                                │
│  • Logical tab order following visual layout                                 │
│  • Skip navigation links at page top                                         │
│  • Focus indicators visible on all interactive elements                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.3 Assistive Technology Testing

| Technology | Testing Frequency | Tools |
|------------|-------------------|-------|
| Screen Readers | Each release | NVDA, JAWS, VoiceOver |
| Keyboard Only | Each sprint | Manual testing |
| Screen Magnification | Each release | ZoomText, browser zoom |
| Voice Control | Quarterly | Dragon NaturallySpeaking |

---

## 11. Appendix

### 11.1 Design File References

- Figma Design System: [Link to be added]
- Component Library: [Link to be added]
- Icon Set: Heroicons (https://heroicons.com/)
- Font Files: Inter (Google Fonts), JetBrains Mono (JetBrains)

### 11.2 Design Review Checklist

- [ ] All components follow design system
- [ ] Color contrast meets WCAG AA standards
- [ ] Keyboard navigation tested for all interactive elements
- [ ] Screen reader testing completed
- [ ] Responsive layouts tested at all breakpoints
- [ ] Loading states defined for all async operations
- [ ] Error states defined for all forms
- [ ] Empty states defined for all list views

### 11.3 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-15 | Chaos-Sec Team | Initial design document |

---

**End of Frontend Design Document**