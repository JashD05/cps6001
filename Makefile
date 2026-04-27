# ============================================================================
# Chaos-Sec: Makefile
# Development, build, test, and infrastructure automation
# ============================================================================

# ----------------------------------------------------------------------------
# Configuration
# ----------------------------------------------------------------------------
SHELL           := /bin/bash
.DEFAULT_GOAL   := help
DOCKER_REGISTRY ?= chaos-sec
DOCKER_TAG      ?= latest
BACKEND_DIR     := backend
FRONTEND_DIR    := frontend
SCRIPTS_DIR     := scripts
KIND_CLUSTER    := chaos-sec
KIND_CONFIG     := scripts/kind-config.yaml

# Colors for output
CYAN   := \033[0;36m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
RED    := \033[0;31m
RESET  := \033[0m

# Go settings
GO      := go
GOFLAGS := -v

# Database migration settings
MIGRATE      := migrate
MIGRATIONS   := backend/migrations
DATABASE_URL ?= postgres://chaossec_admin:chaossec_local_dev_password@localhost:5432/chaossec?sslmode=disable

# Docker compose settings
COMPOSE      := docker compose
COMPOSE_FILE := docker-compose.yml

# ----------------------------------------------------------------------------
# Backend Commands
# ----------------------------------------------------------------------------

.PHONY: backend-run
backend-run: ## Run the backend server locally
	@echo "$(CYAN)▶ Running backend server...$(RESET)"
	cd $(BACKEND_DIR) && $(GO) run ./cmd/backend

.PHONY: backend-test
backend-test: ## Run backend tests
	@echo "$(CYAN)▶ Running backend tests...$(RESET)"
	cd $(BACKEND_DIR) && $(GO) test $(GOFLAGS) ./...

.PHONY: backend-lint
backend-lint: ## Lint backend Go code
	@echo "$(CYAN)▶ Linting backend code...$(RESET)"
	cd $(BACKEND_DIR) && golangci-lint run ./...

.PHONY: backend-build
backend-build: ## Build the backend binary
	@echo "$(CYAN)▶ Building backend binary...$(RESET)"
	cd $(BACKEND_DIR) && $(GO) build -o bin/backend ./cmd/backend

.PHONY: backend-migrate
backend-migrate: ## Run backend database migrations (up)
	@echo "$(CYAN)▶ Running backend migrations...$(RESET)"
	$(MAKE) migrate-up

# ----------------------------------------------------------------------------
# Frontend Commands
# ----------------------------------------------------------------------------

.PHONY: frontend-run
frontend-run: ## Run the frontend dev server
	@echo "$(CYAN)▶ Running frontend dev server...$(RESET)"
	cd $(FRONTEND_DIR) && npm run dev

.PHONY: frontend-test
frontend-test: ## Run frontend tests
	@echo "$(CYAN)▶ Running frontend tests...$(RESET)"
	cd $(FRONTEND_DIR) && npm run test

.PHONY: frontend-lint
frontend-lint: ## Lint frontend code
	@echo "$(CYAN)▶ Linting frontend code...$(RESET)"
	cd $(FRONTEND_DIR) && npm run lint

.PHONY: frontend-build
frontend-build: ## Build the frontend for production
	@echo "$(CYAN)▶ Building frontend for production...$(RESET)"
	cd $(FRONTEND_DIR) && npm run build

# ----------------------------------------------------------------------------
# Infrastructure Commands (Docker Compose)
# ----------------------------------------------------------------------------

.PHONY: infra-up
infra-up: ## Start all infrastructure services
	@echo "$(CYAN)▶ Starting infrastructure services...$(RESET)"
	$(COMPOSE) -f $(COMPOSE_FILE) up -d
	@echo "$(GREEN)✔ Infrastructure services started$(RESET)"

.PHONY: infra-down
infra-down: ## Stop all infrastructure services
	@echo "$(CYAN)▶ Stopping infrastructure services...$(RESET)"
	$(COMPOSE) -f $(COMPOSE_FILE) down
	@echo "$(GREEN)✔ Infrastructure services stopped$(RESET)"

.PHONY: infra-logs
infra-logs: ## Tail infrastructure logs (svc=postgres|redis|rabbitmq|mock-siem|backend|frontend|prometheus|grafana)
	@echo "$(CYAN)▶ Tailing logs for: $(or $(svc),all services)$(RESET)"
	$(COMPOSE) -f $(COMPOSE_FILE) logs -f $(svc)

.PHONY: infra-ps
infra-ps: ## List running infrastructure services
	@echo "$(CYAN)▶ Infrastructure service status:$(RESET)"
	$(COMPOSE) -f $(COMPOSE_FILE) ps

# ----------------------------------------------------------------------------
# Kind (Kubernetes in Docker) Commands
# ----------------------------------------------------------------------------

.PHONY: kind-create
kind-create: ## Create a Kind Kubernetes cluster
	@echo "$(CYAN)▶ Creating Kind cluster '$(KIND_CLUSTER)'...$(RESET)"
	@if ! command -v kind >/dev/null 2>&1; then \
		echo "$(RED)✘ kind is not installed. Install from: https://kind.sigs.k8s.io/docs/user/quick-start/$ (RESET)"; \
		exit 1; \
	fi
	kind create cluster --name $(KIND_CLUSTER) --config $(KIND_CONFIG)
	@echo "$(GREEN)✔ Kind cluster '$(KIND_CLUSTER)' created$(RESET)"
	@echo "$(YELLOW)  To use: export KUBECONFIG=$$(kind get kubeconfig --name $(KIND_CLUSTER))$(RESET)"

.PHONY: kind-delete
kind-delete: ## Delete the Kind Kubernetes cluster
	@echo "$(CYAN)▶ Deleting Kind cluster '$(KIND_CLUSTER)'...$(RESET)"
	kind delete cluster --name $(KIND_CLUSTER)
	@echo "$(GREEN)✔ Kind cluster '$(KIND_CLUSTER)' deleted$(RESET)"

# ----------------------------------------------------------------------------
# Docker Build Commands
# ----------------------------------------------------------------------------

.PHONY: docker-build-backend
docker-build-backend: ## Build the backend Docker image
	@echo "$(CYAN)▶ Building backend Docker image...$(RESET)"
	docker build -t $(DOCKER_REGISTRY)/backend:$(DOCKER_TAG) \
		-f $(BACKEND_DIR)/Dockerfile $(BACKEND_DIR)
	@echo "$(GREEN)✔ Backend image built: $(DOCKER_REGISTRY)/backend:$(DOCKER_TAG)$(RESET)"

.PHONY: docker-build-frontend
docker-build-frontend: ## Build the frontend Docker image
	@echo "$(CYAN)▶ Building frontend Docker image...$(RESET)"
	docker build -t $(DOCKER_REGISTRY)/frontend:$(DOCKER_TAG) \
		-f $(FRONTEND_DIR)/Dockerfile $(FRONTEND_DIR)
	@echo "$(GREEN)✔ Frontend image built: $(DOCKER_REGISTRY)/frontend:$(DOCKER_TAG)$(RESET)"

.PHONY: docker-build
docker-build: docker-build-backend docker-build-frontend ## Build all Docker images

# ----------------------------------------------------------------------------
# Database Migration Commands
# ----------------------------------------------------------------------------

.PHONY: migrate-up
migrate-up: ## Run database migrations up
	@echo "$(CYAN)▶ Running migrations (up)...$(RESET)"
	@bash $(SCRIPTS_DIR)/migrate.sh up

.PHONY: migrate-down
migrate-down: ## Run database migrations down
	@echo "$(CYAN)▶ Running migrations (down)...$(RESET)"
	@bash $(SCRIPTS_DIR)/migrate.sh down

.PHONY: migrate-create
migrate-create: ## Create a new migration (name=your_migration_name)
	@echo "$(CYAN)▶ Creating new migration: $(name)...$(RESET)"
	@bash $(SCRIPTS_DIR)/migrate.sh create $(name)

.PHONY: migrate-status
migrate-status: ## Show migration status
	@echo "$(CYAN)▶ Checking migration status...$(RESET)"
	@bash $(SCRIPTS_DIR)/migrate.sh status

# ----------------------------------------------------------------------------
# Convenience Targets
# ----------------------------------------------------------------------------

.PHONY: dev
dev: ## Start both backend and frontend dev servers
	@echo "$(CYAN)▶ Starting development environment...$(RESET)"
	@echo "$(CYAN)  Starting backend in background...$(RESET)"
	@cd $(BACKEND_DIR) && $(GO) run ./cmd/backend &
	@BACKEND_PID=$$!; \
	echo "$(GREEN)  Backend PID: $$BACKEND_PID$(RESET)"; \
	echo "$(CYAN)  Starting frontend...$(RESET)"; \
	cd $(FRONTEND_DIR) && npm run dev; \
	kill $$BACKEND_PID 2>/dev/null

.PHONY: test
test: backend-test frontend-test ## Run all tests (backend + frontend)

.PHONY: lint
lint: backend-lint frontend-lint ## Run all linters (backend + frontend)

.PHONY: setup
setup: ## Run initial development setup
	@echo "$(CYAN)▶ Running development setup...$(RESET)"
	@bash $(SCRIPTS_DIR)/setup-dev.sh

.PHONY: clean
clean: ## Remove build artifacts and temp files
	@echo "$(CYAN)▶ Cleaning build artifacts...$(RESET)"
	rm -rf $(BACKEND_DIR)/bin
	rm -rf $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/build
	rm -rf $(FRONTEND_DIR)/node_modules/.cache
	@echo "$(GREEN)✔ Clean complete$(RESET)"

.PHONY: deps
deps: ## Install all dependencies
	@echo "$(CYAN)▶ Installing backend dependencies...$(RESET)"
	cd $(BACKEND_DIR) && $(GO) mod download
	@echo "$(CYAN)▶ Installing frontend dependencies...$(RESET)"
	cd $(FRONTEND_DIR) && npm ci
	@echo "$(GREEN)✔ Dependencies installed$(RESET)"

# ----------------------------------------------------------------------------
# Help
# ----------------------------------------------------------------------------

.PHONY: help
help: ## Show this help message
	@echo ""
	@echo "$(CYAN)╔══════════════════════════════════════════════════════════════╗$(RESET)"
	@echo "$(CYAN)║              Chaos-Sec Development Makefile                ║$(RESET)"
	@echo "$(CYAN)╚══════════════════════════════════════════════════════════════╝$(RESET)"
	@echo ""
	@echo "$(YELLOW)Usage:$(RESET) make [target] [variable=value]"
	@echo ""
	@echo "$(YELLOW)Variables:$(RESET)"
	@echo "  DOCKER_REGISTRY  Docker image registry prefix (default: chaos-sec)"
	@echo "  DOCKER_TAG       Docker image tag (default: latest)"
	@echo "  DATABASE_URL     Database connection string"
	@echo "  svc              Service name for infra-logs (e.g., svc=postgres)"
	@echo "  name             Migration name for migrate-create (e.g., name=add_users)"
	@echo ""
	@echo "$(YELLOW)Backend:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && /^backend-[^:]*:/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Frontend:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && /^frontend-[^:]*:/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Infrastructure:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && /^infra-[^:]*:/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Kubernetes (Kind):$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && /^kind-[^:]*:/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Docker:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && /^docker-build-[^:]*:/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Migrations:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && /^migrate-[^:]*:/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Convenience:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?## .*$$/ && !/^backend-/ && !/^frontend-/ && !/^infra-/ && !/^kind-/ && !/^docker-build-/ && !/^migrate-/ {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
