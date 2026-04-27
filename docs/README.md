# Chaos-Sec Documentation

> **An Orchestration Platform for Security Control Validation in Kubernetes Environments**

Welcome to the official documentation for **Chaos-Sec**, a Final Year Project focused on building an automated security validation platform using chaos engineering principles.

---

## 📚 Documentation Index

| # | Document | Description |
|---|----------|-------------|
| 01 | [Project Overview](./01-project-overview.md) | Introduction, problem statement, objectives, and scope |
| 02 | [Architecture](./02-architecture.md) | System architecture, components, and design diagrams |
| 03 | [Technical Specifications](./03-technical-specifications.md) | Technology stack, requirements, and constraints |
| 04 | [API Design](./04-api-design.md) | REST API endpoints, request/response formats |
| 05 | [Database Schema](./05-database-schema.md) | Data models, entity relationships, and schema design |
| 06 | [Frontend Design](./06-frontend-design.md) | Dashboard UI/UX design, wireframes, and user flows |
| 07 | [Security Considerations](./07-security-considerations.md) | Security measures, threat modeling, and best practices |
| 08 | [Implementation Roadmap](./08-implementation-roadmap.md) | Development phases, milestones, and timeline |
| 09 | [Testing Strategy](./09-testing-strategy.md) | Test plans, coverage strategies, and methodologies |
| 10 | [Deployment Guide](./10-deployment-guide.md) | Deployment architecture, CI/CD, and monitoring |
| 11 | [User Guide](./12-user-guide.md) | End-user guide for running experiments, viewing results, and using the dashboard |
| 13 | [Administrator Guide](./13-administrator-guide.md) | Installation, configuration, user management, monitoring, and security administration |
| 14 | [API Reference](./14-api-reference.md) | Complete REST API reference with request/response examples, authentication, and error codes |
| 15 | [Troubleshooting Guide](./15-troubleshooting-guide.md) | Diagnosing and resolving common issues across all platform components |

---

## 🎯 Document Categories

### Design & Planning (Phase 0)
These documents define the system before implementation:

- **01–08** — Project overview, architecture, technical specs, API design, database schema, frontend design, security, and roadmap

### Testing & Quality (Phase 5)
- **09** — Testing strategy and coverage plans

### Operations & Deployment (Phase 6)
- **10** — Infrastructure deployment, CI/CD pipelines, monitoring
- **13** — Day-to-day administration and configuration

### User & Developer Reference
- **11** — User guide for the web dashboard (experiments, templates, clusters, reports)
- **14** — Complete API reference for developers and integrators
- **15** — Troubleshooting guide for operators and support staff

---

## 🏗️ System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      Chaos-Sec Dashboard                        │
│                    (Web-Based Control Panel)                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Orchestration Engine (Go)                    │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │ Experiment  │  │  Kubernetes │  │    SIEM Integration     │ │
│  │   Manager   │  │   Client    │  │      (Mock/Real)        │ │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Target Kubernetes Cluster                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │  Security   │  │  Attacker   │  │    Application Pods     │ │
│  │   Controls  │  │    Pods     │  │      (Targets)          │ │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🔑 Key Features

| Feature | Description |
|---------|-------------|
| **Experiment Orchestration** | Create, schedule, and manage security experiments |
| **Attack Simulation** | Spawn attacker pods to simulate various threat scenarios |
| **Closed-Loop Validation** | Verify security alerts in SIEM systems |
| **Real-Time Monitoring** | Live dashboard showing experiment status and results |
| **Report Generation** | Automated reports with findings and recommendations |
| **Role-Based Access** | Secure access control for different user types |

---

## 👥 Audience Guide

| Role | Recommended Reading |
|------|-------------------|
| **End User** | [User Guide (11)](./12-user-guide.md), [FAQ in User Guide](./12-user-guide.md#11-faq) |
| **Operator** | [User Guide (11)](./12-user-guide.md), [Administrator Guide (12)](./13-administrator-guide.md), [Troubleshooting (14)](./15-troubleshooting-guide.md) |
| **Administrator** | [Administrator Guide (13)](./13-administrator-guide.md), [Deployment Guide (10)](./10-deployment-guide.md), [Troubleshooting (15)](./15-troubleshooting-guide.md) |
| **Developer / Integrator** | [API Reference (14)](./14-api-reference.md), [API Design (04)](./04-api-design.md), [Architecture (02)](./02-architecture.md) |
| **Security Reviewer** | [Security Considerations (07)](./07-security-considerations.md), [Penetration Testing (09)](./09-penetration-testing.md) |

---

## 🎓 Academic Context

This project is part of a **Final Year Project** (FYP) for an undergraduate degree program. The documentation serves as:

1. **Design specification** for the proposed system
2. **Implementation guide** for development phases
3. **Reference material** for project evaluation
4. **User manual** for system operation

---

## 📝 Document Version Control

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-XX | Project Author | Initial documentation release (01–10) |
| 1.1 | 2026-04-21 | Project Author | Added operational docs (11–14): user guide, admin guide, API reference, troubleshooting |

---

## 📄 License

This documentation is part of an academic project. All rights reserved.

---

**Last Updated:** 2026-04-21