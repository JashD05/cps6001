# Chaos-Sec

**Security Control Validation Platform for Kubernetes**

Chaos-Sec tests whether your Kubernetes security controls actually work by running controlled attacks and verifying that they are detected.

## What It Does

1. **Simulates attacks** — Creates temporary attacker pods in your cluster
2. **Tests controls** — Verifies network policies, RBAC, and firewalls block/allow as expected  
3. **Checks detection** — Confirms SIEM/security tools generate alerts for suspicious activity
4. **Generates reports** — Provides evidence that your security is working (or not)

## Quick Start

### Prerequisites

- **Go** 1.21+
- **Node.js** 18+
- **Docker** 24+
- **kubectl**
- **kind** (for local Kubernetes testing)

### 1. Start Services

```bash
# Start PostgreSQL, Redis, and Mock SIEM
docker compose up -d
```

Wait 30 seconds for services to start, then verify:
```bash
docker compose ps
```

### 2. Run Backend

```bash
cd backend
go mod download
go run ./cmd/backend/main.go migrate
go run ./cmd/backend/main.go serve
```

Backend API: http://localhost:8080

### 3. Run Frontend

In a new terminal:
```bash
cd frontend
npm install
npm run dev
```

Frontend dashboard: http://localhost:3000

### 4. Login

- **Email:** admin@chaos-sec.local
- **Password:** admin

### 5. (Optional) Create Test Cluster

```bash
kind create cluster --name chaos-sec-dev --config scripts/kind-config.yaml
```

## Development Commands

Use the Makefile for convenience:

```bash
make infra-up          # Start Docker services
make infra-down        # Stop Docker services
make backend-run       # Run backend server
make frontend-run      # Run frontend dev server
make test              # Run all tests
```

## Project Structure

```
cps6001-main/
├── backend/          # Go backend (Gin framework)
├── frontend/         # React + TypeScript frontend
├── docs/             # Full documentation
├── scripts/          # Setup and deployment scripts
├── docker-compose.yml
└── Makefile
```

## Common Issues

**Docker images fail to pull**
- Check Docker Desktop is running
- Retry: `docker compose up -d`

**Database connection refused**
- Make sure PostgreSQL is running: `docker compose ps`
- Wait for status to show "healthy"

**Port already in use**
- Backend uses port 8080
- Frontend uses port 3000
- Change in `docker-compose.yml` or `.env` if needed

## Documentation

See the `docs/` folder for detailed guides:
- `12-user-guide.md` — Using the dashboard
- `13-administrator-guide.md` — Installation and configuration
- `14-api-reference.md` — REST API documentation
- `15-troubleshooting-guide.md` — Common problems and solutions

## License

Academic project — Final Year Project. All rights reserved.